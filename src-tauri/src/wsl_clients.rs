//! Explicit local client setup only: no WSL process, export record, credential
//! helper, or engine call. WSL routing metadata is independent of the Windows
//! native master config: only owned vault server keys cross that boundary.
//! Cooperating callers serialize with registry::config_lock. Stable bounded read
//! handles and a final identity/byte comparison detect external changes; this is
//! not a path compare-and-swap against a hostile same-user process. Parent handles
//! prevent ordinary rename/reparse substitution during the Windows transaction.
//! Before replacement an exclusive, synced sibling preserves the ORIGINAL exact
//! bytes. On commit uncertainty retain that backup and report failure, never
//! silently restore over a potentially newer config. No-op creates no config or
//! recovery files (the existing registry lock mechanism may create its lock).
use serde::Serialize;
#[path = "wsl_clients_security.rs"]
mod security;
use std::{
    fs::{File, OpenOptions},
    io::{Read, Write},
    path::Path,
};

const LIMIT: u64 = 1 << 20;
static SEQUENCE: std::sync::atomic::AtomicU64 = std::sync::atomic::AtomicU64::new(0);

fn spaces(bytes: &[u8], at: &mut usize) {
    while matches!(bytes.get(*at), Some(b' ' | b'\t' | b'\r' | b'\n')) {
        *at += 1;
    }
}

// Validate duplicate keys at EVERY object level without converting numbers.
// IgnoredAny handles scalar JSON syntax; raw scalar bytes remain untouched.
fn unique_value(bytes: &[u8], at: &mut usize, depth: usize) -> Result<(), String> {
    if depth > 64 {
        return Err("Private config JSON nesting exceeds 64; it was preserved.".into());
    }
    spaces(bytes, at);
    let opening = bytes.get(*at).copied();
    if matches!(opening, Some(b'{' | b'[')) {
        let object = opening == Some(b'{');
        let close = if object { b'}' } else { b']' };
        *at += 1;
        let mut keys = std::collections::BTreeSet::new();
        spaces(bytes, at);
        if bytes.get(*at) == Some(&close) {
            *at += 1;
            return Ok(());
        }
        loop {
            if object {
                let mut stream =
                    serde_json::Deserializer::from_slice(&bytes[*at..]).into_iter::<String>();
                let key = stream
                    .next()
                    .ok_or("Private config key missing")?
                    .map_err(|_| "Private config key is invalid")?;
                *at += stream.byte_offset();
                if !keys.insert(key) {
                    return Err("Private config has duplicate keys; it was preserved.".into());
                }
                spaces(bytes, at);
                if bytes.get(*at) != Some(&b':') {
                    return Err("Private config separator missing".into());
                }
                *at += 1;
            }
            unique_value(bytes, at, depth + 1)?;
            spaces(bytes, at);
            match bytes.get(*at) {
                Some(value) if *value == close => {
                    *at += 1;
                    return Ok(());
                }
                Some(b',') => {
                    *at += 1;
                    spaces(bytes, at);
                }
                _ => return Err("Private config JSON is invalid; it was preserved.".into()),
            }
        }
    }
    let mut value =
        serde_json::Deserializer::from_slice(&bytes[*at..]).into_iter::<serde::de::IgnoredAny>();
    value
        .next()
        .ok_or("Private config value missing")?
        .map_err(|_| "Private config JSON is invalid; it was preserved.")?;
    *at += value.byte_offset();
    Ok(())
}

// Keep every unrelated byte, including unknown fields and numeric precision.
// Parsing bounded values only locates the one top-level field to replace.
fn replacement(bytes: &[u8], directory: &str) -> Result<Option<Vec<u8>>, String> {
    let mut validated = 0;
    unique_value(bytes, &mut validated, 0)?;
    spaces(bytes, &mut validated);
    if validated != bytes.len() {
        return Err("Private config contains trailing data; it was preserved.".into());
    }
    let mut at = 0;
    spaces(bytes, &mut at);
    if bytes.get(at) != Some(&b'{') {
        return Err("Private WSL config must be a JSON object; it was preserved.".into());
    }
    at += 1;
    let mut fields = std::collections::BTreeSet::new();
    let mut selected = None;
    let close;
    loop {
        spaces(bytes, &mut at);
        if bytes.get(at) == Some(&b'}') {
            close = at;
            at += 1;
            break;
        }
        let mut key = serde_json::Deserializer::from_slice(&bytes[at..]).into_iter::<String>();
        let key_value = key
            .next()
            .ok_or("Private config key missing")?
            .map_err(|_| "Private WSL config is invalid JSON; it was preserved.")?;
        at += key.byte_offset();
        if !fields.insert(key_value.clone()) {
            return Err("Private config has duplicate keys; it was preserved.".into());
        }
        spaces(bytes, &mut at);
        if bytes.get(at) != Some(&b':') {
            return Err("Private config field separator missing; it was preserved.".into());
        }
        at += 1;
        spaces(bytes, &mut at);
        let start = at;
        let mut value =
            serde_json::Deserializer::from_slice(&bytes[at..]).into_iter::<serde::de::IgnoredAny>();
        value
            .next()
            .ok_or("Private config value missing")?
            .map_err(|_| "Private WSL config is invalid JSON; it was preserved.")?;
        at += value.byte_offset();
        if key_value == "cliPluginsExtraDirs" {
            selected = Some((start, at));
        }
        spaces(bytes, &mut at);
        match bytes.get(at) {
            Some(b',') => {
                at += 1;
                spaces(bytes, &mut at);
                if bytes.get(at) == Some(&b'}') {
                    return Err("Private config has a trailing comma; it was preserved.".into());
                }
            }
            Some(b'}') => {
                close = at;
                at += 1;
                break;
            }
            _ => return Err("Private WSL config is invalid JSON; it was preserved.".into()),
        }
    }
    spaces(bytes, &mut at);
    if at != bytes.len() {
        return Err("Private config contains trailing data; it was preserved.".into());
    }
    let expected = serde_json::json!([directory]);
    let encoded = serde_json::to_vec(&expected).map_err(|e| e.to_string())?;
    let mut next = Vec::new();
    if let Some((start, end)) = selected {
        if serde_json::from_slice::<serde_json::Value>(&bytes[start..end])
            .ok()
            .as_ref()
            == Some(&expected)
        {
            return Ok(None);
        }
        next.extend_from_slice(&bytes[..start]);
        next.extend_from_slice(&encoded);
        next.extend_from_slice(&bytes[end..]);
    } else {
        next.extend_from_slice(&bytes[..close]);
        if !fields.is_empty() {
            next.push(b',');
        }
        next.extend_from_slice(b"\"cliPluginsExtraDirs\":");
        next.extend_from_slice(&encoded);
        next.extend_from_slice(&bytes[close..]);
    }
    Ok(Some(next))
}

// Locate object values without materializing unrelated values (including numbers
// outside serde_json::Value's range). The caller validates all JSON first.
fn object_fields(
    bytes: &[u8],
) -> Result<std::collections::BTreeMap<String, std::ops::Range<usize>>, String> {
    let mut at = 0;
    spaces(bytes, &mut at);
    if bytes.get(at) != Some(&b'{') {
        return Err(
            "Private registry settings must be JSON objects; configuration preserved.".into(),
        );
    }
    at += 1;
    let mut fields = std::collections::BTreeMap::new();
    loop {
        spaces(bytes, &mut at);
        if bytes.get(at) == Some(&b'}') {
            return Ok(fields);
        }
        let mut key = serde_json::Deserializer::from_slice(&bytes[at..]).into_iter::<String>();
        let name = key
            .next()
            .ok_or("Private config key missing")?
            .map_err(|_| "Private config key is invalid")?;
        at += key.byte_offset();
        spaces(bytes, &mut at);
        if bytes.get(at) != Some(&b':') {
            return Err("Private config separator missing".into());
        }
        at += 1;
        spaces(bytes, &mut at);
        let start = at;
        unique_value(bytes, &mut at, 0)?;
        fields.insert(name, start..at);
        spaces(bytes, &mut at);
        match bytes.get(at) {
            Some(b',') => at += 1,
            Some(b'}') => return Ok(fields),
            _ => return Err("Private config JSON is invalid".into()),
        }
    }
}

fn validate_json(bytes: &[u8]) -> Result<(), String> {
    let mut at = 0;
    unique_value(bytes, &mut at, 0)?;
    spaces(bytes, &mut at);
    if at != bytes.len() {
        return Err("Private config contains trailing data; it was preserved.".into());
    }
    Ok(())
}

fn insert_raw_field(bytes: &[u8], name: &str, value: &[u8]) -> Result<Vec<u8>, String> {
    let fields = object_fields(bytes)?;
    let mut next = Vec::new();
    if let Some(range) = fields.get(name) {
        next.extend_from_slice(&bytes[..range.start]);
        next.extend_from_slice(value);
        next.extend_from_slice(&bytes[range.end..]);
    } else {
        let close = bytes
            .iter()
            .rposition(|byte| *byte == b'}')
            .ok_or("Private config object missing")?;
        next.extend_from_slice(&bytes[..close]);
        if !fields.is_empty() {
            next.push(b',');
        }
        next.extend_from_slice(
            &serde_json::to_vec(name).map_err(|_| "Cannot encode private registry key")?,
        );
        next.push(b':');
        next.extend_from_slice(value);
        next.extend_from_slice(&bytes[close..]);
    }
    Ok(next)
}

// Preserve the existing connect behavior for anonymous Docker Hub requests:
// suppress ambient Linux helper autodetection only when no private auth routing
// exists. Explicit foreign stores/helpers and auth entries always win.
fn anonymous_replacement(bytes: &[u8]) -> Result<Option<Vec<u8>>, String> {
    validate_json(bytes)?;
    let fields = object_fields(bytes)?;
    for field in ["auths", "credHelpers"] {
        if let Some(range) = fields.get(field) {
            if !object_fields(&bytes[range.clone()])?.is_empty() {
                return Ok(None);
            }
        }
    }
    if let Some(range) = fields.get("credsStore") {
        if serde_json::from_slice::<String>(&bytes[range.clone()])
            .ok()
            .as_deref()
            != Some("")
        {
            return Ok(None);
        }
    }
    Ok(Some(insert_raw_field(
        bytes,
        "auths",
        br#"{"https://index.docker.io/v1/":{}}"#,
    )?))
}

fn registry_replacement(bytes: &[u8], key: &str) -> Result<Option<Vec<u8>>, String> {
    crate::registry::validate_stored_server(key)?;
    validate_json(bytes)?;
    let fields = object_fields(bytes)?;
    if let Some(auths) = fields.get("auths") {
        let auths = &bytes[auths.clone()];
        if let Some(entry) = object_fields(auths)?.get(key) {
            let entry = &auths[entry.clone()];
            let credentials = object_fields(entry)?;
            for field in [
                "auth",
                "identitytoken",
                "registrytoken",
                "username",
                "password",
            ] {
                if let Some(value) = credentials.get(field) {
                    // Empty JSON strings have one representation. Inspect the
                    // raw token without allocating/copying credential contents.
                    if &entry[value.clone()] != b"\"\"" {
                        return Err("This private WSL registry has existing file credentials. Review those credentials explicitly before changing its helper; configuration preserved.".into());
                    }
                }
            }
        }
    }
    let helpers = fields
        .get("credHelpers")
        .map(|range| &bytes[range.clone()])
        .unwrap_or(b"{}");
    let helper_fields = object_fields(helpers)?;
    if let Some(value) = helper_fields.get(key) {
        if serde_json::from_slice::<String>(&helpers[value.clone()])
            .ok()
            .as_deref()
            != Some("orbwin.exe")
        {
            return Err("This private WSL registry already uses another credential helper; configuration preserved.".into());
        }
        return Ok(None);
    }
    let helpers = insert_raw_field(helpers, key, br#""orbwin.exe""#)?;
    Ok(Some(insert_raw_field(bytes, "credHelpers", &helpers)?))
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct Refreshed {
    changed: bool,
}

fn regular(metadata: &std::fs::Metadata, directory: bool) -> Result<(), String> {
    #[cfg(windows)]
    {
        use std::os::windows::fs::MetadataExt;
        if metadata.file_attributes() & 0x400 != 0 {
            return Err("Private client paths must not contain reparse points.".into());
        }
    }
    if metadata.file_type().is_symlink()
        || if directory {
            !metadata.is_dir()
        } else {
            !metadata.is_file() || metadata.len() > LIMIT
        }
    {
        return Err("Private client config must be an existing regular file within 1 MiB, under real directories.".into());
    }
    Ok(())
}

fn parents(path: &Path) -> Result<Vec<File>, String> {
    #[cfg(windows)]
    crate::runtime::cli_path(path)?;
    if !path.is_absolute()
        || path.components().any(|c| {
            matches!(
                c,
                std::path::Component::ParentDir | std::path::Component::CurDir
            )
        })
    {
        return Err("Private client config requires a fixed absolute local path.".into());
    }
    let mut held = Vec::new();
    for parent in path
        .parent()
        .ok_or("Private config parent missing")?
        .ancestors()
        .collect::<Vec<_>>()
        .into_iter()
        .rev()
    {
        regular(
            &std::fs::symlink_metadata(parent)
                .map_err(|e| format!("Private config directory unavailable: {e}"))?,
            true,
        )?;
        let mut options = OpenOptions::new();
        options.read(true);
        #[cfg(windows)]
        {
            use std::os::windows::fs::OpenOptionsExt;
            options.share_mode(3).custom_flags(0x02200000); // backup semantics + open reparse point; deny delete
        }
        let file = options
            .open(parent)
            .map_err(|e| format!("Cannot hold private config directory: {e}"))?;
        regular(&file.metadata().map_err(|e| e.to_string())?, true)?;
        held.push(file);
    }
    Ok(held)
}

fn identity(file: &File) -> Result<String, String> {
    let metadata = file.metadata().map_err(|e| e.to_string())?;
    regular(&metadata, false)?;
    #[cfg(windows)]
    {
        use std::os::windows::io::AsRawHandle;
        use windows_sys::Win32::Storage::FileSystem::{
            GetFileInformationByHandle, BY_HANDLE_FILE_INFORMATION,
        };
        let mut info: BY_HANDLE_FILE_INFORMATION = unsafe { std::mem::zeroed() };
        if unsafe { GetFileInformationByHandle(file.as_raw_handle(), &mut info) } == 0 {
            return Err("Cannot verify private config file identity.".into());
        }
        Ok(format!(
            "{}:{}:{}:{}:{:?}",
            info.dwVolumeSerialNumber,
            info.nFileIndexHigh,
            info.nFileIndexLow,
            metadata.len(),
            metadata.modified()
        ))
    }
    #[cfg(not(windows))]
    {
        use std::os::unix::fs::MetadataExt;
        Ok(format!(
            "{}:{}:{}:{:?}",
            metadata.dev(),
            metadata.ino(),
            metadata.len(),
            metadata.modified()
        ))
    }
}

fn read(path: &Path) -> Result<(File, String, Vec<u8>), String> {
    regular(
        &std::fs::symlink_metadata(path).map_err(|e| {
            format!("Private WSL config unavailable. Connect explicitly for initial setup: {e}")
        })?,
        false,
    )?;
    let mut options = OpenOptions::new();
    options.read(true);
    #[cfg(windows)]
    {
        use std::os::windows::fs::OpenOptionsExt;
        options.share_mode(5).custom_flags(0x00200000); // deny in-place writes, permit atomic replacement
    }
    let mut file = options
        .open(path)
        .map_err(|e| format!("Cannot read private config: {e}"))?;
    let before = identity(&file)?;
    let mut bytes = Vec::new();
    (&mut file)
        .take(LIMIT + 1)
        .read_to_end(&mut bytes)
        .map_err(|e| e.to_string())?;
    if bytes.len() as u64 > LIMIT || identity(&file)? != before {
        return Err("Private config changed during its bounded read; retry explicitly.".into());
    }
    Ok((file, before, bytes))
}

fn exclusive(path: &Path, bytes: &[u8], policy: &security::Policy) -> Result<(), String> {
    let mut file = policy.create(path)?;
    file.write_all(bytes)
        .and_then(|_| file.sync_all())
        .map_err(|e| {
            format!(
                "Private refresh recovery write failed; partial file {} retained: {e}",
                path.file_name().unwrap_or_default().to_string_lossy()
            )
        })
}

#[cfg(test)]
fn update(
    path: &Path,
    verify: impl Fn() -> Result<String, String>,
    before_commit: impl FnOnce(),
) -> Result<Refreshed, String> {
    update_expected(path, verify, before_commit, None)
}

fn update_expected(
    path: &Path,
    verify: impl Fn() -> Result<String, String>,
    before_commit: impl FnOnce(),
    expected_old_sha: Option<&str>,
) -> Result<Refreshed, String> {
    update_transform(path, verify, before_commit, expected_old_sha, replacement)
}

fn update_transform(
    path: &Path,
    verify: impl Fn() -> Result<String, String>,
    before_commit: impl FnOnce(),
    expected_old_sha: Option<&str>,
    transform: impl Fn(&[u8], &str) -> Result<Option<Vec<u8>>, String>,
) -> Result<Refreshed, String> {
    let directory = verify()?; // all bundled pins must pass BEFORE any write
    let _parents = parents(path)?;
    let (_original, original_id, bytes) = read(path)?;
    if let Some(expected) = expected_old_sha {
        use sha2::{Digest, Sha256};
        if format!("{:x}", Sha256::digest(&bytes)) != expected {
            return Err("Qualification old config checksum differs; nothing was refreshed.".into());
        }
    }
    let Some(next) = transform(&bytes, &directory)? else {
        let (_, current_id, current_bytes) = read(path)?;
        if current_id != original_id || current_bytes != bytes {
            return Err("Private config changed during refresh; inspect before retrying.".into());
        }
        return Ok(Refreshed { changed: false });
    };
    if next.len() as u64 > LIMIT {
        return Err("Updated private config exceeds 1 MiB; original preserved.".into());
    }
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map_err(|e| e.to_string())?
        .as_nanos();
    let sequence = SEQUENCE.fetch_add(1, std::sync::atomic::Ordering::Relaxed);
    let basename = format!(
        ".orbwin-client-refresh-{}-{stamp}-{sequence}",
        std::process::id()
    );
    let backup = format!("{basename}.original.json");
    let backup_path = path.with_file_name(&backup);
    let policy = security::Policy::capture(&_original)?;
    exclusive(&backup_path, &bytes, &policy)?;
    let temporary = path.with_file_name(format!("{basename}.partial"));
    exclusive(&temporary, &next, &policy)
        .map_err(|e| format!("{e} Original retained in {backup}."))?;
    before_commit();
    let commit = || -> Result<(), String> {
        if verify()? != directory {
            return Err("Bundled Linux tool directory changed.".into());
        }
        let (_current, current_id, current_bytes) = read(path)?;
        if current_id != original_id
            || current_bytes != bytes
            || !policy.same(&security::Policy::capture(&_current)?)?
        {
            return Err("Private config changed before commit; it was not overwritten.".into());
        }
        std::fs::rename(&temporary, path)
            .map_err(|e| format!("Atomic private config replacement failed: {e}"))?;
        let (_, _, actual) = read(path)?;
        if actual != next {
            return Err("Private config changed after replacement; outcome uncertain.".into());
        }
        Ok(())
    };
    commit().map_err(|e| format!("{e} Original exact bytes retained in {backup}; inspect before retrying. Any remaining refresh temporary files were retained."))?;
    Ok(Refreshed { changed: true })
}

// Legacy WSL path ownership is still rooted in the attested engine home. This
// adaptation does not migrate it to NativeProfileWsl or claim native-origin
// qualification. Only this legacy lock is held here, never native master guard.
fn configure_metadata(home: &Path, directory: Option<&str>, keys: &[String]) -> Result<(), String> {
    let verify = || -> Result<String, String> {
        if let Some(expected) = directory {
            let (_, actual) = crate::wsl_integration::tools()?;
            if actual != expected {
                return Err(
                    "Bundled Linux tool directory changed; private WSL metadata preserved.".into(),
                );
            }
            Ok(actual)
        } else {
            Ok(String::new())
        }
    };
    verify()?;
    for key in keys {
        crate::registry::validate_stored_server(key)?;
    }
    // Hold and validate each parent before creating the next legacy directory.
    let _home = parents(&home.join(".orbwin-parent-check"))?;
    let root = home.join(".rdorbwin");
    match std::fs::create_dir(&root) {
        Ok(()) => {}
        Err(error) if error.kind() == std::io::ErrorKind::AlreadyExists => {}
        Err(error) => {
            return Err(format!(
                "Cannot prepare private WSL metadata directory: {error}"
            ))
        }
    }
    let _root = parents(&root.join(".orbwin-parent-check"))?;
    let lock_path = root.join("registry-config.lock");
    match std::fs::symlink_metadata(&lock_path) {
        Ok(metadata) => regular(&metadata, false)?,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
        Err(error) => return Err(format!("Private WSL metadata lock unavailable: {error}")),
    }
    let _lock = crate::registry::config_lock(home)?;
    let config_directory = root.join("wsl-docker");
    match std::fs::create_dir(&config_directory) {
        Ok(()) => {}
        Err(error) if error.kind() == std::io::ErrorKind::AlreadyExists => {}
        Err(error) => {
            return Err(format!(
                "Cannot prepare private WSL config directory: {error}"
            ))
        }
    }
    let path = config_directory.join("config.json");
    let held = parents(&path)?;
    let plan = |bytes: &[u8], _: &str| -> Result<Option<Vec<u8>>, String> {
        let mut next = bytes.to_vec();
        for key in keys {
            if let Some(updated) = registry_replacement(&next, key)? {
                next = updated;
            }
        }
        if let Some(directory) = directory {
            if let Some(updated) = anonymous_replacement(&next)? {
                next = updated;
            }
            if let Some(updated) = replacement(&next, directory)? {
                next = updated;
            }
        }
        Ok((next != bytes).then_some(next))
    };
    match std::fs::symlink_metadata(&path) {
        Ok(_) => {
            update_transform(&path, verify, || {}, None, plan)?;
        }
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {
            // Fresh metadata contains no passwords/tokens. Stage the entire
            // document, sync, then publish with atomic no-clobber hard-link.
            // An intervening creator wins; retain our stage for inspection.
            let initial = br#"{"auths":{"https://index.docker.io/v1/":{}}}"#;
            let next = plan(initial, "")?.unwrap_or_else(|| initial.to_vec());
            if next.len() as u64 > LIMIT {
                return Err("Private WSL config exceeds 1 MiB".into());
            }
            let sequence = SEQUENCE.fetch_add(1, std::sync::atomic::Ordering::Relaxed);
            let stamp = std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .map_err(|e| e.to_string())?
                .as_nanos();
            let temporary = path.with_file_name(format!(
                ".orbwin-wsl-metadata-{}-{stamp}-{sequence}.partial",
                std::process::id()
            ));
            let policy =
                security::Policy::capture(held.last().ok_or("Private WSL parent guard missing")?)?;
            exclusive(&temporary, &next, &policy)?;
            let (_, staged_id, staged_bytes) = read(&temporary)?;
            if staged_bytes != next {
                return Err("Private WSL metadata stage changed; retained for inspection.".into());
            }
            verify()?;
            std::fs::hard_link(&temporary, &path).map_err(|_| "Cannot publish fresh private WSL config without replacing another file. Metadata stage retained for inspection.".to_string())?;
            let (published, published_id, published_bytes) = read(&path)?;
            if published_id != staged_id
                || published_bytes != next
                || !policy.same(&security::Policy::capture(&published)?)?
            {
                return Err("Private WSL config changed after publication; metadata stage retained for inspection.".into());
            }
            std::fs::remove_file(&temporary).map_err(|_| {
                "Private WSL config published, but metadata stage could not be removed.".to_string()
            })?;
        }
        Err(error) => return Err(format!("Private WSL config unavailable: {error}")),
    }
    Ok(())
}

/// Explicit UI login prepares routing metadata before any password dispatch.
/// This legacy lock is released before the caller acquires the native master.
pub(crate) fn route_registry_helper(home: &Path, key: &str) -> Result<(), String> {
    crate::registry::validate_stored_server(key)?;
    configure_metadata(home, None, &[key.to_owned()])
}

/// Caller obtained the actual held helper's owned server list before entering
/// this transaction. No old Windows config or secret-bearing entry is imported.
pub(crate) fn configure_clients(
    home: &Path,
    directory: &str,
    keys: &[String],
) -> Result<(), String> {
    configure_metadata(home, Some(directory), keys)
}

fn refresh(expected_old_sha: Option<&str>) -> Result<Refreshed, String> {
    let target = crate::engine::target()?; // local attestation only; never contacts or starts an engine
    refresh_for_target(&target, expected_old_sha)
}

// Qualification uses its already checked concrete target for both updates;
// product refresh selects one target above and delegates to this same body.
fn refresh_for_target(
    target: &crate::engine::Target,
    expected_old_sha: Option<&str>,
) -> Result<Refreshed, String> {
    crate::wsl_integration::tools()?; // pins before even the existing lock mechanism may create its lock file
    let path = target.home().join(".rdorbwin/wsl-docker/config.json");
    let _parents = parents(&path)?; // existing directories BEFORE lock may create its lock file
    let lock_path = target.home().join(".rdorbwin/registry-config.lock");
    match std::fs::symlink_metadata(lock_path) {
        Ok(metadata) => regular(&metadata, false)?,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
        Err(error) => return Err(format!("Private client lock is unavailable: {error}")),
    }
    let _lock = crate::registry::config_lock(target.home())?;
    update_expected(
        &path,
        || crate::wsl_integration::tools().map(|(_, path)| path),
        || {},
        expected_old_sha,
    )
}

#[tauri::command]
pub(crate) async fn wsl_refresh_private_clients() -> Result<Refreshed, String> {
    tokio::task::spawn_blocking(|| refresh(None))
        .await
        .map_err(|e| e.to_string())?
}

#[cfg(all(test, windows))]
#[path = "wsl_clients_qualification.rs"]
mod qualification;

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;
    struct Fixture(PathBuf);
    impl Fixture {
        fn new(bytes: &[u8]) -> Self {
            let path = std::env::temp_dir().join(format!(
                "orbwin-wsl-clients-{}-{}-{}",
                std::process::id(),
                std::time::SystemTime::now()
                    .duration_since(std::time::UNIX_EPOCH)
                    .unwrap()
                    .as_nanos(),
                SEQUENCE.fetch_add(1, std::sync::atomic::Ordering::Relaxed)
            ));
            std::fs::create_dir(&path).unwrap();
            std::fs::write(path.join("config.json"), bytes).unwrap();
            Self(path)
        }
        fn path(&self) -> PathBuf {
            self.0.join("config.json")
        }
        fn run(&self) -> Result<Refreshed, String> {
            update(&self.path(), || Ok("/mnt/c/pinned/linux".into()), || {})
        }
    }
    impl Drop for Fixture {
        fn drop(&mut self) {
            std::fs::remove_dir_all(&self.0).unwrap();
        }
    }

    #[test]
    fn registry_raw_target_only_retains_unknown_precision_and_foreign_store() {
        let bytes = br#"{ "large":123456789012345678901234567890123456789, "decimal":0.12345678901234567890123456789, "credsStore":"foreign", "credHelpers":{ "other":"foreign", "target":"orbwin.exe" }, "auths":{"other":{"auth":"opaque","precise":1e400}} }"#;
        assert!(registry_replacement(bytes, "target").unwrap().is_none());
        let next = registry_replacement(bytes, "new.example").unwrap().unwrap();
        assert_eq!(
            next,
            String::from_utf8(bytes.to_vec())
                .unwrap()
                .replace(
                    "\"target\":\"orbwin.exe\" }",
                    "\"target\":\"orbwin.exe\" ,\"new.example\":\"orbwin.exe\"}"
                )
                .as_bytes()
        );
        assert!(registry_replacement(&next, "new.example")
            .unwrap()
            .is_none());
    }

    #[test]
    fn registry_raw_anonymous_default_preserves_explicit_routing() {
        let bytes = br#"{"unknown":1e400}"#;
        assert_eq!(
            anonymous_replacement(bytes).unwrap().unwrap(),
            br#"{"unknown":1e400,"auths":{"https://index.docker.io/v1/":{}}}"#
        );
        for bytes in [
            br#"{"auths":{"other":{"auth":"opaque"}},"unknown":1e400}"#.as_slice(),
            br#"{"credHelpers":{"other":"foreign"},"unknown":1e400}"#.as_slice(),
            br#"{"credsStore":"foreign","unknown":1e400}"#.as_slice(),
        ] {
            assert!(anonymous_replacement(bytes).unwrap().is_none());
        }
    }

    #[test]
    fn registry_raw_missing_helpers_and_escaped_matching_keys() {
        let bytes = br#"{ "unknown":1e400, "auths":{"target":{}} }"#;
        let next = registry_replacement(bytes, "target").unwrap().unwrap();
        assert_eq!(
            next,
            br#"{ "unknown":1e400, "auths":{"target":{}} ,"credHelpers":{"target":"orbwin.exe"}}"#
        );
        assert!(registry_replacement(
            br#"{"credHelpers":{"t\u0061rget":"orbwin\u002eexe"}}"#,
            "target"
        )
        .unwrap()
        .is_none());
        let next = registry_replacement(br#"{}"#, "https://index.docker.io/v1/")
            .unwrap()
            .unwrap();
        assert_eq!(
            next,
            br#"{"credHelpers":{"https://index.docker.io/v1/":"orbwin.exe"}}"#
        );
    }

    #[test]
    fn registry_raw_rejects_foreign_inline_ambiguous_and_malformed_before_change() {
        for bytes in [
            br#"{"credHelpers":{"target":"foreign"}}"#.as_slice(),
            br#"{"credHelpers":{"target":null}}"#.as_slice(),
            br#"{"credHelpers":[]}"#.as_slice(),
            br#"{"auths":{"target":{"auth":"opaque"}}}"#.as_slice(),
            br#"{"auths":{"target":{"identitytoken":"opaque"}}}"#.as_slice(),
            br#"{"auths":{"target":{"registrytoken":"opaque"}}}"#.as_slice(),
            br#"{"auths":{"target":{"password":"opaque"}}}"#.as_slice(),
            br#"{"auths":{"target":{"auth":false}}}"#.as_slice(),
            br#"{"auths":{"target":null}}"#.as_slice(),
            br#"{"auths":[]}"#.as_slice(),
            br#"{"credHelpers":{"target":"orbwin.exe","t\u0061rget":"foreign"}}"#.as_slice(),
            br#"{"other":{"duplicate":1,"duplicate":2}}"#.as_slice(),
            br#"{"other":{},}"#.as_slice(),
            br#"{} trailing"#.as_slice(),
            br#"[]"#.as_slice(),
        ] {
            assert!(
                registry_replacement(bytes, "target").is_err(),
                "{}",
                String::from_utf8_lossy(bytes)
            );
        }
        assert!(registry_replacement(
            br#"{"auths":{"target":{"auth":"","identitytoken":"","registrytoken":""}}}"#,
            "target"
        )
        .unwrap()
        .is_some());
        assert!(registry_replacement(b"{}", "\n").is_err());
    }

    #[test]
    fn preserves_credentials_unknown_fields_and_exact_backup() {
        let bytes = br#"{ "auths":{"private":{"auth":"opaque"}},"credsStore":"kept","credHelpers":{"private":"orbwin.exe"},"unknown":[3,true],"cliPluginsExtraDirs":["/old"] }"#;
        let f = Fixture::new(bytes);
        let result = f.run().unwrap();
        assert!(result.changed);
        let backup = std::fs::read_dir(&f.0)
            .unwrap()
            .map(|e| e.unwrap().path())
            .find(|p| p.to_string_lossy().ends_with(".original.json"))
            .unwrap();
        assert_eq!(std::fs::read(backup).unwrap(), bytes);
        let mut old: serde_json::Value = serde_json::from_slice(bytes).unwrap();
        old["cliPluginsExtraDirs"] = serde_json::json!(["/mnt/c/pinned/linux"]);
        assert_eq!(
            serde_json::from_slice::<serde_json::Value>(&std::fs::read(f.path()).unwrap()).unwrap(),
            old
        );
        let count = std::fs::read_dir(&f.0).unwrap().count();
        let unchanged = f.run().unwrap();
        assert!(!unchanged.changed);
        assert_eq!(std::fs::read_dir(&f.0).unwrap().count(), count);
    }

    #[test]
    fn invalid_missing_oversize_and_tool_mismatch_preserve_bytes() {
        for bytes in [
            b"{".to_vec(),
            b"[]".to_vec(),
            vec![b' '; LIMIT as usize + 1],
        ] {
            let f = Fixture::new(&bytes);
            assert!(f.run().is_err());
            assert_eq!(std::fs::read(f.path()).unwrap(), bytes);
            assert_eq!(std::fs::read_dir(&f.0).unwrap().count(), 1);
        }
        let f = Fixture::new(b"{}");
        assert!(update(
            &f.path(),
            || Err("pin mismatch".into()),
            || panic!("must not commit")
        )
        .unwrap_err()
        .contains("pin mismatch"));
        assert_eq!(std::fs::read_dir(&f.0).unwrap().count(), 1);
        std::fs::remove_file(f.path()).unwrap();
        assert!(f.run().is_err());
        assert_eq!(std::fs::read_dir(&f.0).unwrap().count(), 0);
    }

    #[test]
    fn qualification_old_hash_guard_refuses_before_recovery_writes() {
        let f = Fixture::new(b"{}");
        let error = update_expected(
            &f.path(),
            || Ok("/pinned".into()),
            || panic!("must not commit"),
            Some(&"0".repeat(64)),
        )
        .unwrap_err();
        assert!(error.contains("old config checksum differs"));
        assert_eq!(std::fs::read(f.path()).unwrap(), b"{}");
        assert_eq!(std::fs::read_dir(&f.0).unwrap().count(), 1);
    }

    #[test]
    fn preserves_raw_numbers_and_refuses_ambiguous_nested_configs() {
        let bytes = br#"{ "large":123456789012345678901234567890123456789, "decimal":0.12345678901234567890123456789, "auths":{"nested":{"precise":1e400}}, "cliPluginsExtraDirs":["/old"] }"#;
        let next = replacement(bytes, "/new").unwrap().unwrap();
        assert_eq!(
            String::from_utf8(next).unwrap(),
            String::from_utf8(bytes.to_vec())
                .unwrap()
                .replace("[\"/old\"]", "[\"/new\"]")
        );
        for bytes in [
            br#"{"auths":{},"auths":{}}"#.as_slice(),
            br#"{"cliPluginsExtraDirs":[],"cliPluginsExtraDirs":[]}"#.as_slice(),
            br#"{"auths":{"nested":{"auth":"first","auth":"second"}}}"#.as_slice(),
            br#"{"x":[{"a":1,"\u0061":2}]}"#.as_slice(),
            br#"{"auths":{},}"#.as_slice(),
        ] {
            let f = Fixture::new(bytes);
            assert!(f.run().is_err());
            assert_eq!(std::fs::read(f.path()).unwrap(), bytes);
            assert_eq!(std::fs::read_dir(&f.0).unwrap().count(), 1);
        }
    }

    #[test]
    fn no_op_preserves_exact_bytes_and_identity() {
        let bytes =
            b"{ \"cliPluginsExtraDirs\" : [\"/mnt/c/pinned/linux\"], \"unknown\":1.00000 }\n";
        let f = Fixture::new(bytes);
        let (_, before, _) = read(&f.path()).unwrap();
        assert!(!f.run().unwrap().changed);
        let (_, after, actual) = read(&f.path()).unwrap();
        assert_eq!(before, after);
        assert_eq!(actual, bytes);
        assert_eq!(std::fs::read_dir(&f.0).unwrap().count(), 1);
    }

    #[test]
    fn external_replacement_is_not_overwritten_and_original_retained() {
        let f = Fixture::new(b"{\"original\":true}");
        let error = update(
            &f.path(),
            || Ok("/mnt/c/pinned/linux".into()),
            || {
                let other = f.0.join("other.json");
                std::fs::write(&other, b"{\"external\":true}").unwrap();
                std::fs::rename(other, f.path()).unwrap();
            },
        )
        .unwrap_err();
        assert!(error.contains("changed before commit"));
        assert_eq!(std::fs::read(f.path()).unwrap(), b"{\"external\":true}");
        let backup = std::fs::read_dir(&f.0)
            .unwrap()
            .map(|e| e.unwrap().path())
            .find(|p| p.to_string_lossy().ends_with(".original.json"))
            .unwrap();
        assert_eq!(std::fs::read(backup).unwrap(), b"{\"original\":true}");
    }

    #[test]
    fn same_byte_replacement_is_refused_by_file_identity() {
        let f = Fixture::new(b"{}");
        let error = update(
            &f.path(),
            || Ok("/pinned".into()),
            || {
                let replacement = f.0.join("replacement.json");
                std::fs::write(&replacement, b"{}").unwrap();
                std::fs::rename(replacement, f.path()).unwrap();
            },
        )
        .unwrap_err();
        assert!(error.contains("changed before commit"));
        assert_eq!(std::fs::read(f.path()).unwrap(), b"{}");
    }

    #[test]
    fn tool_drift_before_commit_preserves_original() {
        let f = Fixture::new(b"{}");
        let calls = std::cell::Cell::new(0);
        let error = update(
            &f.path(),
            || {
                calls.set(calls.get() + 1);
                if calls.get() == 1 {
                    Ok("/pinned".into())
                } else {
                    Err("pin drift".into())
                }
            },
            || {},
        )
        .unwrap_err();
        assert!(error.contains("pin drift") && error.contains("Original exact bytes retained"));
        assert_eq!(std::fs::read(f.path()).unwrap(), b"{}");
    }

    #[cfg(windows)]
    #[test]
    fn open_transaction_denies_in_place_write_and_parent_replacement() {
        let f = Fixture::new(b"{}");
        let result = update(
            &f.path(),
            || Ok("/pinned".into()),
            || {
                assert!(OpenOptions::new().write(true).open(f.path()).is_err());
                assert!(std::fs::rename(&f.0, f.0.with_extension("moved")).is_err());
            },
        )
        .unwrap();
        assert!(result.changed);
    }

    #[cfg(windows)]
    #[test]
    fn backup_and_replacement_preserve_restrictive_explicit_owner_dacl() {
        use std::os::windows::fs::OpenOptionsExt;
        let f = Fixture::new(b"{\"auths\":{\"private\":{\"auth\":\"fixture-only\"}}}");
        security::restrict_owner_only(&f.path());
        let (original, _, _) = read(&f.path()).unwrap();
        let policy = security::Policy::capture(&original).unwrap();
        let directory = OpenOptions::new()
            .read(true)
            .custom_flags(0x02000000)
            .open(&f.0)
            .unwrap();
        assert!(
            !policy
                .same(&security::Policy::capture(&directory).unwrap())
                .unwrap(),
            "Fixture directory must remain broader than explicit owner-only file DACL"
        );
        drop(original);
        assert!(f.run().unwrap().changed);
        for path in std::fs::read_dir(&f.0).unwrap().map(|e| e.unwrap().path()) {
            let (file, _, _) = read(&path).unwrap();
            assert!(
                policy
                    .same(&security::Policy::capture(&file).unwrap())
                    .unwrap(),
                "Secret-bearing replacement/backup must retain original owner and exact ACL"
            );
        }
    }

    #[cfg(windows)]
    #[test]
    fn reparse_file_and_parent_are_refused() {
        use std::os::windows::fs::{symlink_dir, symlink_file};
        let f = Fixture::new(b"{}");
        let link = f.0.join("link.json");
        symlink_file(f.path(), &link).unwrap();
        assert!(update(&link, || Ok("/pinned".into()), || {}).is_err());
        let directory = f.0.join("linked-parent");
        symlink_dir(&f.0, &directory).unwrap();
        assert!(update(
            &directory.join("config.json"),
            || Ok("/pinned".into()),
            || {}
        )
        .is_err());
        std::fs::remove_dir(directory).unwrap();
        assert_eq!(std::fs::read(f.path()).unwrap(), b"{}");
    }
}
