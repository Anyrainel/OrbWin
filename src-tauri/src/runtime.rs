//! Installed binaries are immutable application resources; runtime state lives
//! in the current user's LocalAppData and survives UI uninstall/replacement.
//! Only debug executables without packaged resources use repository artifacts.
use std::path::PathBuf;

pub(crate) fn component_binary_pin(filename: &str) -> Result<String, String> {
    const PINS: &[(&str, Option<&str>)] =
        include!(concat!(env!("OUT_DIR"), "/component_binary_pins.rs"));
    PINS.iter()
        .find(|(name, _)| *name == filename)
        .and_then(|(_, pin)| *pin)
        .map(str::to_owned)
        .ok_or_else(|| {
            "This desktop build has no trusted pin for the requested runtime component.".into()
        })
}

pub(crate) fn require_component_pin(filename: &str, expected: &str) -> Result<(), String> {
    if component_binary_pin(filename)? != expected {
        return Err("The selected runtime component differs from the trusted inputs of this desktop build. Rebuild a coherent bundle before continuing.".into());
    }
    Ok(())
}

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
    let local = crate::runtime_cache::native_local_appdata()?;
    let root = cli_path(&local)?
        .canonicalize()
        .map_err(|_| "Cannot resolve LocalAppData")?;
    let state = root.join("OrbWinData");
    let resolved = if state.exists() {
        state
            .canonicalize()
            .map_err(|_| "Cannot resolve the engine state directory")?
    } else {
        state.clone()
    };
    let executable =
        std::env::current_exe().map_err(|_| "Cannot resolve the application location")?;
    let installation = executable
        .parent()
        .ok_or("Application directory is missing")?
        .canonicalize()
        .map_err(|_| "Cannot resolve the application directory")?;
    separate_state(&installation, &resolved)?;
    Ok(state)
}
fn separate_state(installation: &std::path::Path, state: &std::path::Path) -> Result<(), String> {
    if installation.starts_with(state) || state.starts_with(installation) {
        return Err("The application installation overlaps OrbWinData. Reinstall OrbWin in a separate folder before preparing its engine. Existing data was retained.".into());
    }
    Ok(())
}
#[derive(Debug, PartialEq, Eq)]
enum IdentityMode {
    Product,
    FixedQualification,
}

fn identity_mode(
    debug: bool,
    packaged: bool,
    choice: Option<&std::ffi::OsStr>,
) -> Result<IdentityMode, String> {
    if !debug || packaged {
        return Ok(IdentityMode::Product);
    }
    match choice{
        None=>Ok(IdentityMode::Product),
        Some(value) if value=="reviewed-fixed-proof"=>Ok(IdentityMode::FixedQualification),
        _=>Err("Invalid debug qualification identity choice. Omit ORBWIN_QUALIFICATION_IDENTITY for product status; only reviewed-fixed-proof selects the fixed historical proof.".into()),
    }
}
fn selected_identity_mode() -> Result<IdentityMode, String> {
    #[cfg(debug_assertions)]
    {
        identity_mode(
            true,
            packaged().is_some(),
            std::env::var_os("ORBWIN_QUALIFICATION_IDENTITY").as_deref(),
        )
    }
    #[cfg(not(debug_assertions))]
    {
        Ok(IdentityMode::Product)
    }
}
pub(crate) fn identity_path() -> Result<PathBuf, String> {
    if selected_identity_mode()? == IdentityMode::FixedQualification {
        return Ok(PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../artifacts/qualification/runtime-identity.json"));
    }
    Ok(state_dir()?.join("runtime.json"))
}
pub(crate) fn validate_home(home: &std::path::Path) -> Result<(), String> {
    let allowed = state_dir()?.join("home");
    if allowed.exists() && home == allowed.canonicalize().map_err(|e| e.to_string())? {
        return Ok(());
    }
    if selected_identity_mode()? == IdentityMode::FixedQualification {
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
pub(crate) fn tool_pin(relative: &str) -> Result<String, String> {
    if !matches!(
        relative,
        "bin/docker.exe"
            | "docker-cli-plugins/docker-compose.exe"
            | "docker-cli-plugins/docker-buildx.exe"
            | "linux/docker"
            | "linux/docker-compose"
            | "linux/docker-buildx"
    ) {
        return Err("Unsupported fixed runtime tool role.".into());
    }
    let lock: serde_json::Value =
        serde_json::from_str(include_str!("../../runtime-tools.lock.json"))
            .map_err(|e| e.to_string())?;
    let expected = lock["tools"]
        .as_array()
        .and_then(|tools| tools.iter().find(|t| t["path"] == relative))
        .and_then(|tool| tool["sha256"].as_str())
        .ok_or("Runtime tool pin is missing")?;
    Ok(expected.to_owned())
}
pub(crate) fn docker() -> Result<crate::runtime_cache::CachedExecutable, String> {
    crate::runtime_cache::prepare(crate::runtime_cache::VerifiedCacheSource::for_windows_docker()?)
}
pub(crate) fn linux_docker() -> Result<crate::runtime_cache::CachedExecutable, String> {
    crate::runtime_cache::prepare(crate::runtime_cache::VerifiedCacheSource::for_linux_docker()?)
}

pub(crate) fn migration_docker() -> Result<(PathBuf, String), String> {
    let binary = linux_docker()?;
    Ok((binary.path().to_path_buf(), tool_pin("linux/docker")?))
}

pub(crate) fn credentials() -> Result<crate::runtime_cache::CachedExecutable, String> {
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
    require_component_pin(
        "docker-credential-orbwin.exe",
        manifest["binarySha256"]
            .as_str()
            .ok_or("Credential helper manifest has no binary digest.")?,
    )?;
    crate::runtime_cache::prepare(
        crate::runtime_cache::VerifiedCacheSource::for_credentials_verified_manifest(
            manifest["binarySha256"]
                .as_str()
                .ok_or("Credential helper manifest has no binary digest.")?,
        )?,
    )
}
pub(crate) fn maintenance_image() -> Result<(PathBuf, String), String> {
    let lock: serde_json::Value =
        serde_json::from_str(include_str!("../../maintenance-image.lock.json"))
            .map_err(|error| error.to_string())?;
    if lock["schema"] != 1
        || lock["tagFree"] != true
        || lock["os"] != "linux"
        || lock["architecture"] != "amd64"
    {
        return Err("Bundled maintenance image pin is unsupported.".into());
    }
    let path = backend_dir().join("orbwin-maintenance.tar");
    let hash = hash_file(&path, 8 << 20)?;
    if lock["sha256"] != hash
        || std::fs::metadata(&path)
            .map_err(|error| error.to_string())?
            .len()
            != lock["bytes"]
                .as_u64()
                .ok_or("Maintenance archive length pin is missing")?
    {
        return Err(
            "Bundled maintenance image differs; repair this installation before using file tools."
                .into(),
        );
    }
    Ok((path, hash))
}
pub(crate) fn guest_file_helper() -> Result<(PathBuf, String), String> {
    let manifest: serde_json::Value = serde_json::from_slice(
        &std::fs::read(backend_dir().join("fs-manifest.json"))
            .map_err(|error| error.to_string())?,
    )
    .map_err(|error| error.to_string())?;
    if manifest["schema"] != 1 || manifest["abi"] != 1 {
        return Err("Guest file helper ABI is unsupported.".into());
    }
    for (name, source) in [
        (
            "fs-helper/main_linux.go",
            include_bytes!("../../backend-tools/session/fs-helper/main_linux.go").as_slice(),
        ),
        (
            "fs-helper/main_other.go",
            include_bytes!("../../backend-tools/session/fs-helper/main_other.go").as_slice(),
        ),
        (
            "fs-helper/volume_linux.go",
            include_bytes!("../../backend-tools/session/fs-helper/volume_linux.go").as_slice(),
        ),
        (
            "go.mod",
            include_bytes!("../../backend-tools/session/go.mod").as_slice(),
        ),
        (
            "go.sum",
            include_bytes!("../../backend-tools/session/go.sum").as_slice(),
        ),
    ] {
        use sha2::{Digest, Sha256};
        if manifest["sources"][name] != format!("{:x}", Sha256::digest(source)) {
            return Err(
                "Guest file helper sources changed; rebuild it before editing data.".into(),
            );
        }
    }
    let path = backend_dir().join("orbwin-fs");
    let hash = hash_file(&path, 16 << 20)?;
    require_component_pin("orbwin-fs", &hash)?;
    if manifest["binarySha256"] != hash {
        return Err("Guest file helper checksum differs.".into());
    }
    Ok((path, hash))
}
pub(crate) fn client_path() -> Result<std::ffi::OsString, String> {
    let credential = credentials()?;
    let mut paths = vec![credential
        .parent()
        .ok_or("Credential helper directory missing")?
        .to_path_buf()];
    paths.extend(std::env::split_paths(
        &std::env::var_os("PATH").unwrap_or_default(),
    ));
    std::env::join_paths(paths)
        .map_err(|_| "Cannot construct the private Docker client PATH".into())
}

pub(crate) fn project_helper() -> Result<crate::runtime_cache::CachedExecutable, String> {
    let manifest: serde_json::Value = serde_json::from_slice(
        &std::fs::read(backend_dir().join("project-manifest.json"))
            .map_err(|error| error.to_string())?,
    )
    .map_err(|error| error.to_string())?;
    if manifest["schema"] != 1 || manifest["abi"] != 1 {
        return Err("WSL project helper ABI is unsupported.".into());
    }
    for (name, source) in [
        (
            "project-client/mapping.go",
            include_bytes!("../../backend-tools/session/project-client/mapping.go").as_slice(),
        ),
        (
            "project-client/mapping_linux.go",
            include_bytes!("../../backend-tools/session/project-client/mapping_linux.go")
                .as_slice(),
        ),
        (
            "project-client/mapping_review_linux.go",
            include_bytes!("../../backend-tools/session/project-client/mapping_review_linux.go")
                .as_slice(),
        ),
        (
            "project-client/config.go",
            include_bytes!("../../backend-tools/session/project-client/config.go").as_slice(),
        ),
        (
            "project-client/main_linux.go",
            include_bytes!("../../backend-tools/session/project-client/main_linux.go").as_slice(),
        ),
        (
            "project-client/main_other.go",
            include_bytes!("../../backend-tools/session/project-client/main_other.go").as_slice(),
        ),
        (
            "go.mod",
            include_bytes!("../../backend-tools/session/go.mod").as_slice(),
        ),
        (
            "go.sum",
            include_bytes!("../../backend-tools/session/go.sum").as_slice(),
        ),
    ] {
        use sha2::{Digest, Sha256};
        if manifest["sources"][name] != format!("{:x}", Sha256::digest(source)) {
            return Err(
                "WSL project helper sources changed; rebuild before dispatching workflows.".into(),
            );
        }
    }
    require_component_pin(
        "orbwin-project",
        manifest["binarySha256"]
            .as_str()
            .ok_or("Project helper manifest has no binary digest.")?,
    )?;
    crate::runtime_cache::prepare(
        crate::runtime_cache::VerifiedCacheSource::for_project_verified_manifest(
            manifest["binarySha256"]
                .as_str()
                .ok_or("Project helper manifest has no binary digest.")?,
        )?,
    )
}

pub(crate) fn backend(
) -> Result<(crate::runtime_cache::CachedExecutable, serde_json::Value), String> {
    crate::runtime_cache::ordinary_backend()
}

// Same existing provenance checks for ordinary startup and candidate staging.
// This reads source inputs only; it never materializes the ordinary cache.
pub(crate) fn read_verified_backend_manifest() -> Result<serde_json::Value, String> {
    let manifest: serde_json::Value = serde_json::from_slice(
        &std::fs::read(backend_dir().join("build-manifest.json"))
            .map_err(|e| format!("Bundled backend manifest is unavailable: {e}"))?,
    )
    .map_err(|e| e.to_string())?;
    validate_backend_manifest(&manifest)?;
    let path = backend_dir().join("rdd.exe");
    require_component_pin(
        "rdd.exe",
        manifest["binarySha256"]
            .as_str()
            .ok_or("Bundled backend manifest has no binary digest.")?,
    )?;
    if hash_file(&path, 512 << 20)? != manifest["binarySha256"] {
        return Err("Bundled backend binary differs from its manifest.".into());
    }
    Ok(manifest)
}

// Dormant Rust-only byte staging. No renderer invoke or backend process starts.
pub(crate) fn prepare_modern_backend_candidate(
) -> Result<crate::runtime_cache::PreparedModernBackendCandidate, String> {
    crate::runtime_cache::prepare_modern_backend_candidate()
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
        (
            "patches/rdd/0013-typed-service-status.patch",
            include_bytes!("../../patches/rdd/0013-typed-service-status.patch"),
        ),
        (
            "patches/rdd/0014-host-http2-floor.patch",
            include_bytes!("../../patches/rdd/0014-host-http2-floor.patch"),
        ),
        (
            "patches/rdd/0015-lan-publication-observation.patch",
            include_bytes!("../../patches/rdd/0015-lan-publication-observation.patch"),
        ),
        (
            "patches/rdd/0016-lan-publication-owner.patch",
            include_bytes!("../../patches/rdd/0016-lan-publication-owner.patch"),
        ),
        (
            "patches/rdd/0017-private-engine-proxy.patch",
            include_bytes!("../../patches/rdd/0017-private-engine-proxy.patch"),
        ),
        (
            "patches/rdd/0018-private-runtime-authority.patch",
            include_bytes!("../../patches/rdd/0018-private-runtime-authority.patch"),
        ),
        (
            "patches/rdd/0019-runtime-custody-and-inspection-foundations.patch",
            include_bytes!("../../patches/rdd/0019-runtime-custody-and-inspection-foundations.patch"),
        ),
        (
            "patches/rdd/0020-fresh-setup-early-entry-and-io-custody.patch",
            include_bytes!("../../patches/rdd/0020-fresh-setup-early-entry-and-io-custody.patch"),
        ),
        (
            "patches/rdd/0021-modern-candidate-native-read-custody.patch",
            include_bytes!("../../patches/rdd/0021-modern-candidate-native-read-custody.patch"),
        ),
        (
            "patches/rdd/0022-fresh-original-birth-and-guest-session-codec.patch",
            include_bytes!("../../patches/rdd/0022-fresh-original-birth-and-guest-session-codec.patch"),
        ),
        (
            "patches/rdd/0023-fresh-original-pipe-dial-custody.patch",
            include_bytes!("../../patches/rdd/0023-fresh-original-pipe-dial-custody.patch"),
        ),
    ];
    // The embedded byte allowlist must cover the complete lock in application
    // order. Otherwise a newly registered patch could silently remain outside
    // runtime attestation while an older bundled backend was still accepted.
    let expected = lock["patches"]
        .as_array()
        .ok_or("Application patch lock is missing")?;
    if expected.len() != patches.len()
        || expected
            .iter()
            .zip(patches)
            .any(|(path, (embedded, _))| path.as_str() != Some(*embedded))
    {
        return Err("Application patch attestation differs from its source lock.".into());
    }
    let actual = manifest["patches"]
        .as_array()
        .ok_or("Backend patch manifest is missing")?;
    if actual.len() != patches.len()
        || actual.iter().zip(patches).any(|(entry, (path, bytes))| {
            entry["path"] != *path || entry["sha256"] != format!("{:x}", Sha256::digest(bytes))
        })
    {
        return Err("Bundled backend patches differ from this application.".into());
    }
    crate::runtime_winio_provenance::validate_portable_winio(manifest)?;
    Ok(())
}

#[cfg(test)]
mod tests {
    #[test]
    fn identity_selection_is_product_default_and_fixed_debug_optin_only() {
        use super::{identity_mode, IdentityMode};
        use std::ffi::OsStr;
        assert_eq!(
            identity_mode(true, false, None).unwrap(),
            IdentityMode::Product
        );
        assert_eq!(
            identity_mode(true, false, Some(OsStr::new("reviewed-fixed-proof"))).unwrap(),
            IdentityMode::FixedQualification
        );
        for debug in [false, true] {
            for packaged in [false, true] {
                if !debug || packaged {
                    for value in [
                        None,
                        Some(OsStr::new("reviewed-fixed-proof")),
                        Some(OsStr::new("C:\\caller\\identity.json")),
                        Some(OsStr::new("invalid")),
                    ] {
                        assert_eq!(
                            identity_mode(debug, packaged, value).unwrap(),
                            IdentityMode::Product
                        )
                    }
                }
            }
        }
        for value in [
            "",
            "1",
            "true",
            "reviewed-fixed-proof ",
            "C:\\caller\\identity.json",
        ] {
            assert!(identity_mode(true, false, Some(OsStr::new(value))).is_err())
        }
    }
    #[test]
    fn component_execution_requires_a_desktop_build_pin_and_closed_role() {
        for name in ["rdd.exe", "orbwin-session.exe"] {
            // Builds without backend artifacts deliberately have no pin; they
            // must still refuse a caller's fabricated provenance digest.
            assert!(super::require_component_pin(name, &"0".repeat(64)).is_err());
            if let Ok(pin) = super::component_binary_pin(name) {
                assert_eq!(pin.len(), 64);
                assert!(pin
                    .bytes()
                    .all(|byte| byte.is_ascii_hexdigit() && !byte.is_ascii_uppercase()));
                assert!(super::require_component_pin(name, &pin).is_ok());
                let mut changed = pin.into_bytes();
                changed[0] = if changed[0] == b'0' { b'1' } else { b'0' };
                assert!(
                    super::require_component_pin(name, std::str::from_utf8(&changed).unwrap())
                        .is_err()
                );
            }
        }
        for name in [
            "",
            "../rdd.exe",
            "RDD.EXE",
            "runtime/rdd.exe",
            "orbwin-lan-helper.exe",
        ] {
            assert!(super::component_binary_pin(name).is_err());
        }
    }
    use super::*;
    #[test]
    fn backend_attestation_covers_the_entire_ordered_patch_lock() {
        use sha2::{Digest, Sha256};
        let lock: serde_json::Value =
            serde_json::from_str(include_str!("../../upstream.lock.json")).unwrap();
        let root = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("..");
        let mut manifest = serde_json::json!({
            "sourceCommit":lock["commit"], "distroEmbedded":true,
            "guestPackagesLockSha256":format!("{:x}",Sha256::digest(include_bytes!("../../guest-packages.lock.json"))),
            "patches":lock["patches"].as_array().unwrap().iter().map(|path| {
                let path = path.as_str().unwrap();
                serde_json::json!({"path":path,"sha256":format!("{:x}",Sha256::digest(std::fs::read(root.join(path)).unwrap()))})
            }).collect::<Vec<_>>()
        });
        validate_backend_manifest(&manifest).unwrap();
        let correct = manifest.clone();
        // Source-only adoption must not silently accept a prior backend bundle.
        manifest["patches"].as_array_mut().unwrap().pop();
        assert!(validate_backend_manifest(&manifest).is_err());
        manifest = correct.clone();
        manifest["patches"].as_array_mut().unwrap().swap(0, 1);
        assert!(validate_backend_manifest(&manifest).is_err());
        manifest = correct.clone();
        manifest["patches"][1] = manifest["patches"][0].clone();
        assert!(validate_backend_manifest(&manifest).is_err());
        manifest = correct;
        manifest["patches"][12]["sha256"] = "0".repeat(64).into();
        assert!(validate_backend_manifest(&manifest).is_err());
    }
    #[test]
    fn installed_resources_and_persistent_state_must_be_separate() {
        for (installation, state) in [
            ("C:/Local/OrbWin", "C:/Local/OrbWinData"),
            ("C:/Programs/OrbWin", "C:/Local/OrbWinData"),
        ] {
            separate_state(
                std::path::Path::new(installation),
                std::path::Path::new(state),
            )
            .unwrap();
        }
        for (installation, state) in [
            ("C:/Local/OrbWinData", "C:/Local/OrbWinData"),
            ("C:/Local", "C:/Local/OrbWinData"),
            ("C:/Local/OrbWinData/app", "C:/Local/OrbWinData"),
        ] {
            assert!(separate_state(
                std::path::Path::new(installation),
                std::path::Path::new(state)
            )
            .is_err());
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
