//! Installed binaries are immutable application resources; runtime state lives
//! in the current user's LocalAppData and survives UI uninstall/replacement.
//! Only debug executables without packaged resources use repository artifacts.
use std::path::PathBuf;

// Several upstream Windows dependencies concatenate slash-separated suffixes.
// Win32 verbatim paths reject that syntax; pass ordinary drive-absolute paths
// at CLI/environment boundaries while retaining canonical paths for ownership.
pub(crate) fn cli_path(path: &std::path::Path) -> Result<PathBuf, String> {
    let text = path.to_str().ok_or("Backend path is not valid Unicode")?;
    let text = text.strip_prefix(r"\\?\").unwrap_or(text);
    if text.len() < 3
        || !text.as_bytes()[0].is_ascii_alphabetic()
        || text.as_bytes()[1] != b':'
        || !matches!(text.as_bytes()[2], b'\\' | b'/')
    {
        return Err("Backend state must use an absolute local Windows drive path.".into());
    }
    Ok(PathBuf::from(text))
}

pub(crate) fn packaged() -> Option<PathBuf> {
    let path = std::env::current_exe().ok()?.parent()?.join("runtime");
    path.is_dir().then_some(path)
}
pub(crate) fn backend_dir() -> PathBuf {
    if let Some(path) = packaged() {
        return path.join("backend");
    }
    #[cfg(debug_assertions)]
    {
        PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../artifacts/backend")
    }
    #[cfg(not(debug_assertions))]
    {
        std::env::current_exe()
            .unwrap_or_default()
            .with_file_name("runtime/backend")
    }
}
pub(crate) fn tools_dir() -> PathBuf {
    if let Some(path) = packaged() {
        return path.join("tools");
    }
    #[cfg(debug_assertions)]
    {
        PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../artifacts/runtime-tools")
    }
    #[cfg(not(debug_assertions))]
    {
        std::env::current_exe()
            .unwrap_or_default()
            .with_file_name("runtime/tools")
    }
}
pub(crate) fn state_dir() -> Result<PathBuf, String> {
    std::env::var_os("LOCALAPPDATA")
        .map(|path| PathBuf::from(path).join("OrbWin"))
        .ok_or("Windows did not provide LocalAppData for this user.".into())
}
pub(crate) fn identity_path() -> Result<PathBuf, String> {
    #[cfg(debug_assertions)]
    if packaged().is_none() {
        let development = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../artifacts/qualification/runtime-identity.json");
        if development.exists() {
            return Ok(development);
        }
    }
    Ok(state_dir()?.join("runtime.json"))
}
pub(crate) fn validate_home(home: &std::path::Path) -> Result<(), String> {
    let allowed = state_dir()?.join("home");
    if allowed.exists() && home == allowed.canonicalize().map_err(|e| e.to_string())? {
        return Ok(());
    }
    #[cfg(debug_assertions)]
    if packaged().is_none() {
        let artifacts = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../artifacts")
            .canonicalize()
            .map_err(|e| e.to_string())?;
        if home.starts_with(artifacts) {
            return Ok(());
        }
    }
    Err("Owned backend home is outside the product state directory.".into())
}
pub(crate) fn hash_file(path: &std::path::Path, limit: u64) -> Result<String, String> {
    use sha2::{Digest, Sha256};
    use std::io::Read;
    let mut file = std::fs::File::open(path)
        .map_err(|e| format!("Cannot read bundled component {}: {e}", path.display()))?;
    if file.metadata().map_err(|e| e.to_string())?.len() > limit {
        return Err("Bundled component exceeds its size limit.".into());
    }
    let mut hash = Sha256::new();
    let mut buffer = [0u8; 65536];
    let mut total = 0;
    loop {
        let n = file.read(&mut buffer).map_err(|e| e.to_string())?;
        if n == 0 {
            break;
        }
        total += n as u64;
        if total > limit {
            return Err("Bundled component grew beyond its size limit.".into());
        }
        hash.update(&buffer[..n]);
    }
    Ok(format!("{:x}", hash.finalize()))
}

pub(crate) fn write_json(path: &std::path::Path, value: &serde_json::Value) -> Result<(), String> {
    use std::io::Write;
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map_err(|e| e.to_string())?
        .as_nanos();
    let temporary = path.with_extension(format!("partial-{}-{stamp}", std::process::id()));
    let mut file = std::fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .open(&temporary)
        .map_err(|e| e.to_string())?;
    file.write_all(&serde_json::to_vec_pretty(value).map_err(|e| e.to_string())?)
        .map_err(|e| e.to_string())?;
    file.sync_all().map_err(|e| e.to_string())?;
    drop(file);
    std::fs::rename(temporary, path).map_err(|e| e.to_string())
}
pub(crate) fn docker() -> Result<PathBuf, String> {
    let lock: serde_json::Value =
        serde_json::from_str(include_str!("../../runtime-tools.lock.json"))
            .map_err(|e| e.to_string())?;
    let path = tools_dir().join("bin/docker.exe");
    let expected = lock["tools"]
        .as_array()
        .and_then(|tools| tools.iter().find(|t| t["path"] == "bin/docker.exe"))
        .ok_or("Docker tool pin is missing")?;
    if hash_file(&path, 128 << 20)? != expected["sha256"] {
        return Err("Bundled Docker CLI differs from its pinned checksum.".into());
    }
    Ok(path)
}

pub(crate) fn credentials() -> Result<PathBuf, String> {
    use sha2::{Digest, Sha256};
    let manifest: serde_json::Value = serde_json::from_slice(
        &std::fs::read(backend_dir().join("credentials-manifest.json"))
            .map_err(|_| "The bundled credential helper manifest is unavailable")?,
    )
    .map_err(|_| "The credential helper manifest is invalid")?;
    for (name, source) in [
        (
            "main.go",
            include_bytes!("../../backend-tools/credentials/main.go").as_slice(),
        ),
        (
            "vault_windows.go",
            include_bytes!("../../backend-tools/credentials/vault_windows.go").as_slice(),
        ),
        (
            "go.mod",
            include_bytes!("../../backend-tools/credentials/go.mod").as_slice(),
        ),
        (
            "go.sum",
            include_bytes!("../../backend-tools/credentials/go.sum").as_slice(),
        ),
    ] {
        if manifest["sources"][name] != format!("{:x}", Sha256::digest(source)) {
            return Err("The credential helper differs from this application. Rebuild or repair the bundle before using credentials.".into());
        }
    }
    let path = backend_dir().join("docker-credential-orbwin.exe");
    if hash_file(&path, 16 << 20)? != manifest["binarySha256"] {
        return Err("The bundled credential helper checksum differs.".into());
    }
    Ok(path)
}
pub(crate) fn client_path() -> Result<std::ffi::OsString, String> {
    credentials()?;
    let mut paths = vec![backend_dir()];
    paths.extend(std::env::split_paths(
        &std::env::var_os("PATH").unwrap_or_default(),
    ));
    std::env::join_paths(paths)
        .map_err(|_| "Cannot construct the private Docker client PATH".into())
}

pub(crate) fn backend() -> Result<(PathBuf, serde_json::Value), String> {
    let manifest: serde_json::Value = serde_json::from_slice(
        &std::fs::read(backend_dir().join("build-manifest.json"))
            .map_err(|e| format!("Bundled backend manifest is unavailable: {e}"))?,
    )
    .map_err(|e| e.to_string())?;
    validate_backend_manifest(&manifest)?;
    let path = backend_dir().join("rdd.exe");
    if hash_file(&path, 512 << 20)? != manifest["binarySha256"] {
        return Err("Bundled backend binary differs from its manifest.".into());
    }
    Ok((path, manifest))
}

pub(crate) fn validate_backend_manifest(manifest: &serde_json::Value) -> Result<(), String> {
    use sha2::{Digest, Sha256};
    let lock: serde_json::Value = serde_json::from_str(include_str!("../../upstream.lock.json"))
        .map_err(|e| e.to_string())?;
    if manifest["sourceCommit"] != lock["commit"] || manifest["distroEmbedded"] != true {
        return Err("Bundled backend source or guest image differs from this application.".into());
    }
    if manifest["guestPackagesLockSha256"]
        != format!(
            "{:x}",
            Sha256::digest(include_bytes!("../../guest-packages.lock.json"))
        )
    {
        return Err("Bundled guest package pins differ from this application.".into());
    }
    let patches: &[(&str, &[u8])] = &[
        (
            "patches/rdd/0001-instance-docker-pipe.patch",
            include_bytes!("../../patches/rdd/0001-instance-docker-pipe.patch"),
        ),
        (
            "patches/rdd/0002-orbwin-wsl-namespace.patch",
            include_bytes!("../../patches/rdd/0002-orbwin-wsl-namespace.patch"),
        ),
        (
            "patches/rdd/0003-owned-wsl-distro.patch",
            include_bytes!("../../patches/rdd/0003-owned-wsl-distro.patch"),
        ),
        (
            "patches/rdd/0004-guard-product-environment.patch",
            include_bytes!("../../patches/rdd/0004-guard-product-environment.patch"),
        ),
        (
            "patches/rdd/0005-windows-docker-paths.patch",
            include_bytes!("../../patches/rdd/0005-windows-docker-paths.patch"),
        ),
        (
            "patches/rdd/0006-sequential-provisioning-proof.patch",
            include_bytes!("../../patches/rdd/0006-sequential-provisioning-proof.patch"),
        ),
        (
            "patches/rdd/0007-wsl-shared-socket.patch",
            include_bytes!("../../patches/rdd/0007-wsl-shared-socket.patch"),
        ),
        (
            "patches/rdd/0008-windows-port-publication.patch",
            include_bytes!("../../patches/rdd/0008-windows-port-publication.patch"),
        ),
        (
            "patches/rdd/0009-kubernetes-version-selection.patch",
            include_bytes!("../../patches/rdd/0009-kubernetes-version-selection.patch"),
        ),
        (
            "patches/rdd/0010-single-provisioning-owner.patch",
            include_bytes!("../../patches/rdd/0010-single-provisioning-owner.patch"),
        ),
        (
            "patches/rdd/0011-offline-guest-packages.patch",
            include_bytes!("../../patches/rdd/0011-offline-guest-packages.patch"),
        ),
        (
            "patches/rdd/0012-networkless-sandbox-publication.patch",
            include_bytes!("../../patches/rdd/0012-networkless-sandbox-publication.patch"),
        ),
    ];
    let actual = manifest["patches"]
        .as_array()
        .ok_or("Backend patch manifest is missing")?;
    if actual.len() != patches.len()
        || patches.iter().any(|(path, bytes)| {
            !actual.iter().any(|entry| {
                entry["path"] == *path && entry["sha256"] == format!("{:x}", Sha256::digest(bytes))
            })
        })
    {
        return Err("Bundled backend patches differ from this application.".into());
    }
    Ok(())
}

fn verified_plugins() -> Result<PathBuf, String> {
    let lock: serde_json::Value =
        serde_json::from_str(include_str!("../../runtime-tools.lock.json"))
            .map_err(|e| e.to_string())?;
    for tool in lock["tools"].as_array().ok_or("Tool pins missing")? {
        let relative = tool["path"].as_str().ok_or("Tool path missing")?;
        if relative.starts_with("docker-cli-plugins/")
            && hash_file(&tools_dir().join(relative), 256 << 20)? != tool["sha256"]
        {
            return Err(format!("Bundled plugin checksum differs: {relative}"));
        }
    }
    tools_dir()
        .join("docker-cli-plugins")
        .canonicalize()
        .map_err(|e| e.to_string())
}
pub(crate) fn validate_client(config: &std::path::Path) -> Result<(), String> {
    let plugins = verified_plugins()?;
    let value:serde_json::Value=serde_json::from_slice(&std::fs::read(config.join("config.json")).map_err(|e|format!("Private Docker client configuration is unavailable. Resume setup before reading build history: {e}"))?).map_err(|e|e.to_string())?;
    if value["cliPluginsExtraDirs"] != serde_json::json!([plugins]) {
        return Err("Private Docker plugin configuration differs from this application. Resume setup before reading build history.".into());
    }
    Ok(())
}
pub(crate) fn configure_client(config: &std::path::Path) -> Result<(), String> {
    let plugins = verified_plugins()?;
    std::fs::create_dir_all(config).map_err(|e| e.to_string())?;
    let path = config.join("config.json");
    let mut value: serde_json::Value = if path.exists() {
        serde_json::from_slice(&std::fs::read(&path).map_err(|e| e.to_string())?)
            .map_err(|e| format!("Private Docker config is invalid; it was not changed: {e}"))?
    } else {
        serde_json::json!({})
    };
    let object = value
        .as_object_mut()
        .ok_or("Private Docker config is not an object; it was not changed")?;
    let expected = serde_json::json!([plugins]);
    let same_plugins = object.get("cliPluginsExtraDirs") == Some(&expected);
    object.insert("cliPluginsExtraDirs".into(), expected);
    let auth_changed = private_auth_defaults(&mut value)?;
    if same_plugins && !auth_changed {
        return Ok(());
    }
    write_json(&path, &value)
}

// Docker CLI otherwise auto-detects a helper from PATH, even for an empty
// private config. Avoid consulting another engine's store or a broken inherited
// Linux helper for anonymous pulls. Explicit private auth/helper settings win.
pub(crate) fn private_auth_defaults(value: &mut serde_json::Value) -> Result<bool, String> {
    let object = value
        .as_object_mut()
        .ok_or("Private client config must be an object")?;
    let has_auth = object
        .get("auths")
        .and_then(serde_json::Value::as_object)
        .is_some_and(|value| !value.is_empty());
    let has_helpers = object
        .get("credHelpers")
        .and_then(serde_json::Value::as_object)
        .is_some_and(|value| !value.is_empty());
    let has_store = object
        .get("credsStore")
        .and_then(serde_json::Value::as_str)
        .is_some_and(|value| !value.is_empty());
    if has_auth || has_helpers || has_store {
        return Ok(false);
    }
    object.insert(
        "auths".into(),
        serde_json::json!({"https://index.docker.io/v1/": {}}),
    );
    Ok(true)
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn anonymous_client_does_not_inherit_helpers_or_erase_private_auth() {
        let mut empty = serde_json::json!({"unknown": 9007199254740993u64});
        assert!(private_auth_defaults(&mut empty).unwrap());
        assert_eq!(empty["unknown"], 9007199254740993u64);
        assert!(!private_auth_defaults(&mut empty).unwrap());
        for value in [
            serde_json::json!({"credsStore":"explicit"}),
            serde_json::json!({"credHelpers":{"private.example":"explicit"}}),
            serde_json::json!({"auths":{"private.example":{"auth":"disposable-test-token"}}}),
        ] {
            let mut configured = value.clone();
            assert!(!private_auth_defaults(&mut configured).unwrap());
            assert_eq!(configured, value);
        }
    }
    #[test]
    fn cli_paths_keep_drive_ownership_without_verbatim_slash_semantics() {
        assert_eq!(
            cli_path(std::path::Path::new(r"\\?\F:\state\home")).unwrap(),
            PathBuf::from(r"F:\state\home")
        );
        for source in [
            r"\\?\UNC\server\share",
            r"\\server\share",
            r"C:relative",
            r"relative",
        ] {
            assert!(cli_path(std::path::Path::new(source)).is_err(), "{source}");
        }
    }
    #[test]
    fn atomic_identity_replacement_and_component_size_cap() {
        let stamp = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        let directory =
            std::env::temp_dir().join(format!("orbwin-state-{}-{stamp}", std::process::id()));
        std::fs::create_dir(&directory).unwrap();
        let identity = directory.join("runtime.json");
        write_json(
            &identity,
            &serde_json::json!({"daemonId":"before","unknown":{"number":9007199254740993u64}}),
        )
        .unwrap();
        let mut value: serde_json::Value =
            serde_json::from_slice(&std::fs::read(&identity).unwrap()).unwrap();
        value["daemonId"] = serde_json::json!("after");
        write_json(&identity, &value).unwrap();
        let reread: serde_json::Value =
            serde_json::from_slice(&std::fs::read(&identity).unwrap()).unwrap();
        assert_eq!(reread["daemonId"], "after");
        assert_eq!(reread["unknown"]["number"], 9007199254740993u64);
        assert_eq!(std::fs::read_dir(&directory).unwrap().count(), 1);
        assert!(hash_file(&identity, 1).is_err());
        std::fs::remove_file(identity).unwrap();
        std::fs::remove_dir(directory).unwrap();
    }
}
