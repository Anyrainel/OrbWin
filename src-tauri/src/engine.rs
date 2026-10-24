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
    pub(crate) docker: PathBuf,
    pub(crate) config: PathBuf,
}
impl Target {
    pub(crate) fn daemon_id(&self) -> &str {
        &self.identity.daemon_id
    }
    pub(crate) async fn session(&self) -> Result<PathBuf, String> {
        tokio::task::spawn_blocking(|| {
            use sha2::{Digest, Sha256};
            let path = crate::runtime::backend_dir().join("orbwin-session.exe");
            let manifest: serde_json::Value = serde_json::from_slice(
                &std::fs::read(crate::runtime::backend_dir().join("session-manifest.json"))
                    .map_err(|e| e.to_string())?,
            )
            .map_err(|e| e.to_string())?;
            for (name, source) in [
                (
                    "main.go",
                    include_bytes!("../../backend-tools/session/main.go").as_slice(),
                ),
                (
                    "go.mod",
                    include_bytes!("../../backend-tools/session/go.mod").as_slice(),
                ),
                (
                    "kube.go",
                    include_bytes!("../../backend-tools/session/kube.go").as_slice(),
                ),
                (
                    "publication.go",
                    include_bytes!("../../backend-tools/session/publication.go").as_slice(),
                ),
                (
                    "go.sum",
                    include_bytes!("../../backend-tools/session/go.sum").as_slice(),
                ),
            ] {
                if manifest["sources"][name] != format!("{:x}", Sha256::digest(source)) {
                    return Err(
                        "Session helper source changed. Rebuild it before opening a terminal."
                            .into(),
                    );
                }
            }
            let bytes =
                std::fs::read(&path).map_err(|e| format!("Build the session helper first: {e}"))?;
            if bytes.len() > 64 * 1024 * 1024
                || manifest["binarySha256"] != format!("{:x}", Sha256::digest(bytes))
            {
                return Err("Session helper binary differs from its build manifest.".into());
            }
            Ok(path)
        })
        .await
        .map_err(|e| e.to_string())?
    }
    pub(crate) fn home(&self) -> &std::path::Path {
        &self.identity.home
    }
    pub(crate) async fn rdd(&self) -> Result<PathBuf, String> {
        let target = self.clone();
        tokio::task::spawn_blocking(move || target.verified_rdd())
            .await
            .map_err(|e| e.to_string())?
    }
    fn verified_rdd(&self) -> Result<PathBuf, String> {
        use sha2::{Digest, Sha256};
        let path = crate::runtime::backend_dir().join("rdd.exe");
        use std::io::Read;
        let mut file = std::fs::File::open(&path)
            .map_err(|e| format!("Cannot read owned backend binary: {e}"))?;
        if file.metadata().map_err(|e| e.to_string())?.len() > 512 * 1024 * 1024 {
            return Err("Backend binary exceeds the development size limit.".into());
        }
        let mut hash = Sha256::new();
        let mut buffer = [0u8; 65536];
        loop {
            let n = file.read(&mut buffer).map_err(|e| e.to_string())?;
            if n == 0 {
                break;
            }
            hash.update(&buffer[..n]);
        }
        if format!("{:x}", hash.finalize()) != self.identity.binary_sha256 {
            return Err("Backend binary no longer matches its qualified identity. Rebuild and requalify before lifecycle changes.".into());
        }
        self.verify_registration()?;
        Ok(path)
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
    })
}

pub(crate) async fn command(target: &Target, args: &[&str]) -> Result<String, String> {
    let config = target
        .config
        .to_str()
        .ok_or("Invalid client configuration path")?;
    let mut scoped = vec!["--config", config, "--host", ENDPOINT];
    scoped.extend_from_slice(args);
    process::read_command(&target.docker, &scoped).await
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
    let raw = process::read_command(&helper, &["port-status", target.daemon_id()]).await?;
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
            resources: Some(resources),
        },
        Err(detail) => Status {
            available: false,
            endpoint: ENDPOINT,
            message: if detail.contains("failed to connect")
                || detail.contains("error during connect")
            {
                "The owned OrbWin engine is offline. Refresh when it is running.".into()
            } else {
                "The owned backend could not be verified. Open diagnostics for the next step."
                    .into()
            },
            version: None,
            diagnostic: Some(detail),
            setup_available: crate::installation::available(),
            resources: None,
        },
    }
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DiskInfo {
    path: String,
    file_length_bytes: u64,
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
        Ok(DiskInfo {
            path: path.display().to_string(),
            file_length_bytes: metadata.len(),
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
    use super::*;
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
