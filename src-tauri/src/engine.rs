//! Read-only Docker adapter. Connect only to the fixed product pipe after a
//! successful owned-guest proof recorded its daemon ID and exact build. Never
//! autostart rdd or fall back to a default context. Inventory is returned only
//! after all reads succeed and identity is rechecked; rdd/Moby own all state.
use crate::process;
use serde::{Deserialize, Serialize};
use std::path::PathBuf;

pub const ENDPOINT: &str = "npipe:////./pipe/docker-rancher-desktop-orbwin";
#[derive(Deserialize, Clone)]
#[serde(rename_all = "camelCase")]
struct Identity {
    endpoint: String,
    home: PathBuf,
    daemon_id: String,
    source_commit: String,
    binary_sha256: String,
    identity_verified: bool,
}

#[derive(Clone)]
pub(crate) struct Target {
    identity: Identity,
    pub(crate) docker: crate::runtime_cache::CachedExecutable,
    _credentials: crate::runtime_cache::CachedExecutable,
    pub(crate) config: PathBuf,
}
impl Target {
    pub(crate) fn daemon_id(&self) -> &str {
        &self.identity.daemon_id
    }
    pub(crate) async fn session(&self) -> Result<crate::runtime_cache::CachedExecutable, String> {
        session_binary().await
    }
    pub(crate) fn home(&self) -> &std::path::Path {
        &self.identity.home
    }
    pub(crate) async fn client_snapshot(
        &self,
    ) -> Result<crate::client_config::ClientConfigSnapshot, String> {
        let docker = self.docker.clone();
        let credentials = self._credentials.clone();
        tokio::task::spawn_blocking(move || crate::client_config::reader(docker, credentials))
            .await
            .map_err(|_| "Private client configuration admission did not complete.".to_string())?
            .map_err(|e| e.to_string())
    }
    pub(crate) async fn rdd(&self) -> Result<crate::runtime_cache::CachedExecutable, String> {
        let target = self.clone();
        tokio::task::spawn_blocking(move || target.verified_rdd())
            .await
            .map_err(|e| e.to_string())?
    }
    fn verified_rdd(&self) -> Result<crate::runtime_cache::CachedExecutable, String> {
        crate::runtime::require_component_pin("rdd.exe", &self.identity.binary_sha256)?;
        // The cache hashes source bytes against this independent pin once,
        // then continuously retains the immutable native image/namespace.
        self.verify_registration()?;
        crate::runtime_cache::prepare(
            crate::runtime_cache::VerifiedCacheSource::for_rdd_qualified_identity(
                &self.identity.binary_sha256,
            )?,
        )
    }
    #[cfg(windows)]
    fn verify_registration(&self) -> Result<(), String> {
        use winreg::{enums::HKEY_CURRENT_USER, RegKey};
        let registry = RegKey::predef(HKEY_CURRENT_USER)
            .open_subkey("Software\\Microsoft\\Windows\\CurrentVersion\\Lxss")
            .map_err(|e| e.to_string())?;
        let home = self.home().canonicalize().map_err(|e| e.to_string())?;
        let mut count = 0;
        for key in registry.enum_keys() {
            let child = registry
                .open_subkey(key.map_err(|e| e.to_string())?)
                .map_err(|e| e.to_string())?;
            let distribution: String = child
                .get_value("DistributionName")
                .map_err(|e| e.to_string())?;
            if distribution.eq_ignore_ascii_case("lima-orbwin") {
                count += 1;
                let base: String = child.get_value("BasePath").map_err(|e| e.to_string())?;
                let base = PathBuf::from(base)
                    .canonicalize()
                    .map_err(|e| e.to_string())?;
                if base == home || !base.starts_with(&home) {
                    return Err("The OrbWin WSL registration is not owned by this recorded home. No lifecycle change was sent.".into());
                }
            }
        }
        if count != 1 {
            return Err("Expected one recorded owned guest. First-run provisioning is not qualified in this development adapter.".into());
        }
        Ok(())
    }
    #[cfg(not(windows))]
    fn verify_registration(&self) -> Result<(), String> {
        Err("WSL lifecycle requires Windows.".into())
    }
}

// Shared source/binary attestation is independent of engine identity. Fixed
// host-only diagnostics may use the same SDK while the engine is unavailable;
// workload callers must still obtain and verify their Target before dispatch.
pub(crate) async fn session_binary() -> Result<crate::runtime_cache::CachedExecutable, String> {
    tokio::task::spawn_blocking(|| {
        use sha2::{Digest, Sha256};
        use std::io::Read;
        let manifest_path = crate::runtime::backend_dir().join("session-manifest.json");
        let mut bytes = Vec::new();
        std::fs::File::open(manifest_path)
            .map_err(|e| e.to_string())?
            .take((1 << 20) + 1)
            .read_to_end(&mut bytes)
            .map_err(|e| e.to_string())?;
        if bytes.len() > 1 << 20 {
            return Err("Session helper manifest exceeds its size limit.".into());
        }
        let manifest: serde_json::Value =
            serde_json::from_slice(&bytes).map_err(|e| e.to_string())?;
        crate::runtime::require_component_pin(
            "orbwin-session.exe",
            manifest["binarySha256"]
                .as_str()
                .ok_or("Session helper manifest has no binary digest.")?,
        )?;
        let expected_sources = include!(concat!(env!("OUT_DIR"), "/session_sources.rs"));
        if manifest["sources"].as_object().map(|sources| sources.len())
            != Some(expected_sources.len())
        {
            return Err(
                "Session helper source inventory changed. Rebuild it before continuing.".into(),
            );
        }
        for (name, source) in expected_sources {
            if manifest["sources"][name] != format!("{:x}", Sha256::digest(source)) {
                return Err("Session helper source changed. Rebuild it before continuing.".into());
            }
        }
        crate::runtime_cache::prepare(
            crate::runtime_cache::VerifiedCacheSource::for_session_verified_manifest(
                manifest["binarySha256"]
                    .as_str()
                    .ok_or("Session helper manifest has no binary digest.")?,
            )?,
        )
    })
    .await
    .map_err(|e| e.to_string())?
}
pub(crate) fn target() -> Result<Target, String> {
    let read = |path| {
        std::fs::read(path).map_err(|e| format!("Owned backend evidence is unavailable: {e}"))
    };
    let identity: Identity = serde_json::from_slice(&read(crate::runtime::identity_path()?)?)
        .map_err(|e| e.to_string())?;
    let manifest: serde_json::Value = serde_json::from_slice(&read(
        crate::runtime::backend_dir().join("build-manifest.json"),
    )?)
    .map_err(|e| e.to_string())?;
    crate::runtime::validate_backend_manifest(&manifest)?;
    let lock: serde_json::Value = serde_json::from_str(include_str!("../../upstream.lock.json"))
        .map_err(|e| e.to_string())?;
    if !identity.identity_verified
        || identity.endpoint != ENDPOINT
        || identity.daemon_id.is_empty()
        || manifest["sourceCommit"] != identity.source_commit
        || lock["commit"] != identity.source_commit
        || manifest["binarySha256"] != identity.binary_sha256
    {
        return Err("Owned backend identity is missing or differs from this build. Re-run the isolated proof.".into());
    }
    let home = identity.home.canonicalize().map_err(|e| e.to_string())?;
    crate::runtime::validate_home(&home)?;
    Ok(Target {
        config: home.join(".rdorbwin/docker"),
        identity,
        docker: crate::runtime::docker()?,
        _credentials: crate::runtime::credentials()?,
    })
}

pub(crate) async fn command(target: &Target, args: &[&str]) -> Result<String, String> {
    let snapshot = target.client_snapshot().await?;
    let environment = docker_environment(target, &snapshot)?;
    let mut scoped: Vec<std::ffi::OsString> = vec![
        "--config".into(),
        snapshot.path().as_os_str().to_owned(),
        "--host".into(),
        ENDPOINT.into(),
    ];
    scoped.extend(args.iter().map(std::ffi::OsString::from));
    crate::client_worker::read(
        target.docker.clone(),
        scoped,
        environment,
        Some(snapshot),
        std::time::Duration::from_secs(10),
    )
    .await
}

fn docker_environment(
    target: &Target,
    snapshot: &crate::client_config::ClientConfigSnapshot,
) -> Result<Vec<(std::ffi::OsString, std::ffi::OsString)>, String> {
    docker_environment_for_paths(target, snapshot.path(), snapshot.buildx_path())
}

pub(crate) fn docker_environment_for_paths(
    target: &Target,
    config: &std::path::Path,
    buildx: &std::path::Path,
) -> Result<Vec<(std::ffi::OsString, std::ffi::OsString)>, String> {
    docker_environment_for_sources(
        &target.docker,
        &target._credentials,
        ENDPOINT,
        config,
        buildx,
    )
}

fn docker_environment_for_sources(
    docker: &crate::runtime_cache::CachedExecutable,
    credentials: &crate::runtime_cache::CachedExecutable,
    endpoint: &str,
    config: &std::path::Path,
    buildx: &std::path::Path,
) -> Result<Vec<(std::ffi::OsString, std::ffi::OsString)>, String> {
    use std::ffi::OsString;
    credentials.check_held_origin()?;
    docker.check_held_origin()?;
    let mut paths = vec![
        credentials
            .parent()
            .ok_or("Held credential helper directory unavailable")?
            .to_path_buf(),
        docker
            .parent()
            .ok_or("Held Docker directory unavailable")?
            .to_path_buf(),
    ];
    paths.extend(std::env::split_paths(
        &std::env::var_os("PATH").unwrap_or_default(),
    ));
    let path = std::env::join_paths(paths)
        .map_err(|_| "Cannot construct held client helper search path")?;
    // Replaced selectors include plugin discovery and buildx state. Inherited
    // values cannot redirect this explicitly targeted diagnostic.
    let overrides: Vec<(OsString, OsString)> = vec![
        ("PATH".into(), path),
        // Go's Windows helper lookup otherwise checks the working directory
        // before PATH. Pin the suffix search so the retained helper wins.
        ("NoDefaultCurrentDirectoryInExePath".into(), "1".into()),
        ("PATHEXT".into(), ".EXE".into()),
        ("DOCKER_HOST".into(), endpoint.into()),
        ("DOCKER_CONFIG".into(), config.as_os_str().to_owned()),
        ("BUILDX_CONFIG".into(), buildx.as_os_str().to_owned()),
        (
            "ORBWIN_DOCKER_MANAGED_PLUGINS".into(),
            docker
                .parent()
                .ok_or("Cached Docker directory unavailable")?
                .join("docker-cli-plugins")
                .into_os_string(),
        ),
    ];
    Ok(scoped_environment(std::env::vars_os(), overrides))
}

// Setup has no recorded Target until provisioning and daemon identity checks
// finish. Admit only the two fixed setup endpoints, read info with a real held
// private snapshot, and leave all identity/publication decisions to setup.
fn setup_endpoint(endpoint: &str) -> Result<(), String> {
    if endpoint == ENDPOINT || endpoint == "npipe:////./pipe/docker-rancher-desktop-orbwin-proof" {
        Ok(())
    } else {
        Err("Setup info requires the fixed OrbWin or qualification endpoint.".into())
    }
}

pub(crate) async fn setup_info(
    docker: crate::runtime_cache::CachedExecutable,
    credentials: crate::runtime_cache::CachedExecutable,
    snapshot: crate::client_config::ClientConfigSnapshot,
    endpoint: &str,
) -> Result<serde_json::Value, String> {
    setup_endpoint(endpoint)?;
    snapshot.check().map_err(|error| error.to_string())?;
    let environment = docker_environment_for_sources(
        &docker,
        &credentials,
        endpoint,
        snapshot.path(),
        snapshot.buildx_path(),
    )?;
    let args = vec![
        "--config".into(),
        snapshot.path().as_os_str().to_owned(),
        "--host".into(),
        endpoint.into(),
        "info".into(),
        "--format".into(),
        "{{json .}}".into(),
    ];
    let raw = crate::client_worker::read(
        docker,
        args,
        environment,
        Some(snapshot),
        std::time::Duration::from_secs(10),
    )
    .await?;
    let info: serde_json::Value = serde_json::from_str(&raw)
        .map_err(|_| "Setup received an invalid engine info response.".to_string())?;
    validate_setup_info(&info)?;
    Ok(info)
}

fn validate_setup_info(info: &serde_json::Value) -> Result<(), String> {
    if info["OSType"] != "linux" || !info["ID"].as_str().is_some_and(|id| !id.is_empty()) {
        return Err("Setup endpoint did not return a Linux engine with a daemon ID. No identity was published.".into());
    }
    Ok(())
}

fn scoped_environment(
    inherited: impl IntoIterator<Item = (std::ffi::OsString, std::ffi::OsString)>,
    overrides: Vec<(std::ffi::OsString, std::ffi::OsString)>,
) -> Vec<(std::ffi::OsString, std::ffi::OsString)> {
    let mut values: Vec<_> = inherited
        .into_iter()
        .filter(|(key, _)| {
            !matches!(
                key.to_string_lossy().to_ascii_uppercase().as_str(),
                "PATH"
                    | "NODEFAULTCURRENTDIRECTORYINEXEPATH"
                    | "PATHEXT"
                    | "DOCKER_HOST"
                    | "DOCKER_CONFIG"
                    | "DOCKER_CONTEXT"
                    | "DOCKER_TLS_VERIFY"
                    | "DOCKER_CERT_PATH"
                    | "DOCKER_API_VERSION"
                    | "DOCKER_AUTH_CONFIG"
                    | "BUILDKIT_HOST"
                    | "BUILDX_BUILDER"
                    | "BUILDX_CONFIG"
                    | "ORBWIN_DOCKER_MANAGED_PLUGINS"
            )
        })
        .collect();
    values.extend(overrides);
    values
}

pub(crate) async fn check(target: &Target) -> Result<serde_json::Value, String> {
    let raw = command(target, &["info", "--format", "{{json .}}"]).await?;
    let info: serde_json::Value =
        serde_json::from_str(&raw).map_err(|e| format!("Invalid engine response: {e}"))?;
    validate_info(&info, &target.identity.daemon_id)?;
    Ok(info)
}

#[derive(Deserialize, Serialize, Clone)]
#[serde(rename_all = "camelCase")]
pub(crate) struct Publication {
    container_id: String,
    port: u16,
    protocol: String,
    published: bool,
}
pub(crate) async fn publications(target: &Target) -> Result<Vec<Publication>, String> {
    let helper = target.session().await?;
    let raw = process::read_cached(&helper, &["port-status", target.daemon_id()]).await?;
    serde_json::from_str(&raw).map_err(|e| format!("Invalid publication snapshot: {e}"))
}
pub(crate) async fn publication_failure(target: &Target) -> Result<Option<String>, String> {
    let entries = publications(target).await?;
    let missing: Vec<_> = entries
        .iter()
        .filter(|entry| !entry.published)
        .map(|entry| format!("{}/{}", entry.port, entry.protocol))
        .collect();
    Ok((!missing.is_empty()).then(|| format!("Windows publication is unavailable for {}. Containers may be running. Inspect the conflicting service or choose another port; no other engine was stopped.", missing.join(", "))))
}

fn validate_info(info: &serde_json::Value, daemon_id: &str) -> Result<(), String> {
    if info["ID"] != daemon_id || info["OSType"] != "linux" {
        return Err("The endpoint does not match the verified owned Linux engine. No inventory was accepted.".into());
    }
    Ok(())
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Status {
    pub available: bool,
    pub endpoint: &'static str,
    pub message: String,
    pub version: Option<String>,
    pub diagnostic: Option<String>,
    pub setup_available: bool,
    pub upgrade_available: bool,
    pub resources: Option<serde_json::Value>,
}
pub async fn status() -> Status {
    let result = async {
        let target = target()?;
        let info = check(&target).await?;
        Ok::<_, String>((
            info["ServerVersion"].as_str().map(str::to_string),
            serde_json::json!({"cpus":info["NCPU"],"memoryBytes":info["MemTotal"]}),
        ))
    }
    .await;
    match result {
        Ok((version, resources)) => Status {
            available: true,
            endpoint: ENDPOINT,
            message: "Connected to OrbWin's verified Linux engine.".into(),
            version,
            diagnostic: None,
            setup_available: false,
            upgrade_available: false,
            resources: Some(resources),
        },
        Err(detail) => {
            let preparation_issue = crate::installation::preparation_issue();
            Status {
                available: false,
                endpoint: ENDPOINT,
                message: if preparation_issue.is_some() {
                    "The engine's registered home cannot be used by this installation. Open diagnostics and use the original installation to manage it. OrbWin has not adopted or removed its data.".into()
                } else if detail.contains("failed to connect")
                    || detail.contains("error during connect")
                {
                    "The owned OrbWin engine is offline. Refresh when it is running.".into()
                } else {
                    "The owned backend could not be verified. Open diagnostics for the next step."
                        .into()
                },
                version: None,
                diagnostic: Some(preparation_issue.unwrap_or(detail)),
                setup_available: crate::installation::available(),
                upgrade_available: crate::installation::upgrade_available(),
                resources: None,
            }
        }
    }
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DiskInfo {
    path: String,
    file_length_bytes: u64,
    host_capacity: Option<crate::host_capacity::HostCapacity>,
    host_capacity_error: Option<String>,
}
pub async fn disk_info() -> Result<DiskInfo, String> {
    let target = target()?;
    tokio::task::spawn_blocking(move || {
        target.verified_rdd()?;
        let base = target
            .home()
            .join(".rdorbwin/lima/orbwin")
            .canonicalize()
            .map_err(|e| e.to_string())?;
        let path = base
            .join("ext4.vhdx")
            .canonicalize()
            .map_err(|e| format!("The owned virtual disk could not be measured: {e}"))?;
        if !path.starts_with(&base) {
            return Err("The virtual disk path is outside the owned guest directory.".into());
        }
        let metadata = path.metadata().map_err(|e| e.to_string())?;
        if !metadata.is_file() {
            return Err("The owned virtual disk is not a regular file.".into());
        }
        let (host_capacity, host_capacity_error) = match crate::host_capacity::query(&base) {
            Ok(value) => (Some(value), None),
            Err(error) => (None, Some(error)),
        };
        Ok(DiskInfo {
            path: path.display().to_string(),
            file_length_bytes: metadata.len(),
            host_capacity,
            host_capacity_error,
        })
    })
    .await
    .map_err(|e| e.to_string())?
}

#[derive(Serialize)]
pub struct Inventory {
    containers: Vec<serde_json::Value>,
    images: Vec<serde_json::Value>,
    volumes: Vec<serde_json::Value>,
    networks: Vec<serde_json::Value>,
}
fn parse_rows(raw: &str) -> Result<Vec<serde_json::Value>, String> {
    raw.lines()
        .filter(|line| !line.trim().is_empty())
        .map(|line| {
            let row: serde_json::Value = serde_json::from_str(line)
                .map_err(|e| format!("Inventory response was invalid: {e}"))?;
            if !row.is_object() {
                return Err("Inventory response contained a non-object row".into());
            }
            Ok(row)
        })
        .collect()
}
pub async fn inventory() -> Result<Inventory, String> {
    let target = target()?;
    check(&target).await?;
    let (containers, images, volumes, networks) = tokio::try_join!(
        command(
            &target,
            &["ps", "-a", "--no-trunc", "--format", "{{json .}}"]
        ),
        command(
            &target,
            &["image", "ls", "--no-trunc", "--format", "{{json .}}"]
        ),
        command(&target, &["volume", "ls", "--format", "{{json .}}"]),
        command(
            &target,
            &["network", "ls", "--no-trunc", "--format", "{{json .}}"]
        )
    )?;
    let mut snapshot = Inventory {
        containers: parse_rows(&containers)?,
        images: parse_rows(&images)?,
        volumes: parse_rows(&volumes)?,
        networks: parse_rows(&networks)?,
    };
    let publication = publications(&target).await;
    for container in &mut snapshot.containers {
        let id = container["ID"].as_str().unwrap_or("").to_string();
        match &publication {
            Ok(entries) => {
                container["OrbWinHostPorts"] = serde_json::json!(entries
                    .iter()
                    .filter(|entry| entry.container_id == id)
                    .collect::<Vec<_>>())
            }
            Err(error) => {
                container["OrbWinHostPorts"] = serde_json::json!([]);
                container["OrbWinPortError"] = serde_json::json!(error);
            }
        }
    }
    check(&target).await?;
    Ok(snapshot)
}

#[cfg(test)]
mod tests {
    #[test]
    fn managed_diagnostic_environment_replaces_case_insensitive_selectors() {
        use std::ffi::OsString;
        let inherited = [
            ("docker_host", "foreign"),
            ("Docker_Context", "foreign"),
            ("Docker_Auth_Config", "foreign"),
            ("ORBWIN_DOCKER_MANAGED_PLUGINS", "foreign"),
            ("BUILDX_CONFIG", "foreign"),
            ("Path", "foreign"),
            ("NoDefaultCurrentDirectoryInExePath", "foreign"),
            ("pathext", ".CMD"),
            ("=C:", "C:\\work"),
            ("APP_TEST", "keep"),
        ]
        .into_iter()
        .map(|(k, v)| (OsString::from(k), OsString::from(v)));
        let result = super::scoped_environment(
            inherited,
            vec![
                ("DOCKER_HOST".into(), "owned".into()),
                ("PATH".into(), "held".into()),
            ],
        );
        assert_eq!(
            result,
            vec![
                ("=C:".into(), "C:\\work".into()),
                ("APP_TEST".into(), "keep".into()),
                ("DOCKER_HOST".into(), "owned".into()),
                ("PATH".into(), "held".into()),
            ]
        );
    }
    #[tokio::test]
    #[ignore = "read-only capacity of the existing owned data volume"]
    async fn owned_host_capacity() {
        assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
        let value = super::disk_info().await.unwrap();
        let data = serde_json::to_value(value).unwrap();
        assert!(data["hostCapacityError"].is_null(), "{data}");
        let host = &data["hostCapacity"];
        let available = host["availableBytes"].as_u64().unwrap();
        let total = host["totalBytes"].as_u64().unwrap();
        let free = host["freeBytes"].as_u64().unwrap();
        assert!(total > 0 && available <= total && available <= free);
        assert!(data["fileLengthBytes"].as_u64().unwrap() > 0);
        let file = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../artifacts/qualification/p2-host-capacity-native-20261025.json");
        std::fs::write(file, serde_json::to_vec_pretty(&data).unwrap()).unwrap();
    }
    use super::*;
    #[test]
    fn setup_info_accepts_only_owned_or_fixed_proof_endpoint_and_linux_identity() {
        assert!(setup_endpoint(ENDPOINT).is_ok());
        assert!(setup_endpoint("npipe:////./pipe/docker-rancher-desktop-orbwin-proof").is_ok());
        for endpoint in [
            "",
            "unix:///var/run/docker.sock",
            "tcp://127.0.0.1:2375",
            "npipe:////./pipe/docker_engine",
            "npipe:////./pipe/docker-rancher-desktop-orbwin-other",
        ] {
            assert!(setup_endpoint(endpoint).is_err());
        }
        assert!(validate_setup_info(
            &serde_json::json!({"ID":"observed-daemon", "OSType":"linux"})
        )
        .is_ok());
        for info in [
            serde_json::Value::Null,
            serde_json::json!([]),
            serde_json::json!({"OSType":"linux"}),
            serde_json::json!({"ID":"", "OSType":"linux"}),
            serde_json::json!({"ID":1, "OSType":"linux"}),
            serde_json::json!({"ID":"observed", "OSType":"windows"}),
        ] {
            assert!(validate_setup_info(&info).is_err());
        }
    }
    #[test]
    fn rejects_foreign_or_non_linux_daemons() {
        assert!(validate_info(
            &serde_json::json!({"ID":"foreign","OSType":"linux"}),
            "owned"
        )
        .is_err());
        assert!(validate_info(
            &serde_json::json!({"ID":"owned","OSType":"windows"}),
            "owned"
        )
        .is_err());
        assert!(
            validate_info(&serde_json::json!({"ID":"owned","OSType":"linux"}), "owned").is_ok()
        );
    }
    #[test]
    fn inventory_preserves_unknown_fields_and_rejects_partial_parse() {
        let rows = parse_rows("{\"ID\":\"one\",\"future\":{\"nested\":true}}\n").unwrap();
        assert_eq!(rows[0]["future"]["nested"], true);
        assert!(parse_rows("{\"ID\":\"one\"}\ninvalid").is_err());
        assert!(parse_rows("[]").is_err());
        assert!(parse_rows("").unwrap().is_empty());
    }
}
