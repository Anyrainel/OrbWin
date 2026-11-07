//! Writes only a reviewed, narrow Compose volume override to a new user-selected
//! file, then rechecks its bounded bytes and narrow semantics before a later
//! explicit operation. Never edits source files or starts Compose. Creation is exclusive;
//! a failed write retains its partial file and reports the path for inspection.
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::{
    collections::{BTreeMap, BTreeSet},
    io::{Read, Write},
    path::Path,
};

#[derive(Deserialize, Serialize, PartialEq, Eq)]
#[serde(deny_unknown_fields)]
struct Mapping {
    services: BTreeMap<String, Service>,
    volumes: BTreeMap<String, External>,
}
#[derive(Deserialize, Serialize, PartialEq, Eq)]
#[serde(deny_unknown_fields)]
struct Service {
    volumes: Vec<Mount>,
}
#[derive(Deserialize, Serialize, PartialEq, Eq)]
#[serde(deny_unknown_fields)]
struct Mount {
    #[serde(rename = "type")]
    kind: String,
    source: String,
    target: String,
    read_only: bool,
    volume: NoCopy,
}
#[derive(Deserialize, Serialize, PartialEq, Eq)]
#[serde(deny_unknown_fields)]
struct NoCopy {
    nocopy: bool,
}
#[derive(Deserialize, Serialize, PartialEq, Eq)]
#[serde(deny_unknown_fields)]
struct External {
    external: bool,
    name: String,
}
fn key(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 128
        && value.as_bytes()[0].is_ascii_alphanumeric()
        && value
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || b"_.-".contains(&byte))
}
fn file_name(value: &str) -> bool {
    let base = value
        .split('.')
        .next()
        .unwrap_or("")
        .trim_end_matches([' ', '.'])
        .to_ascii_uppercase();
    !value.contains(':')
        && !value.chars().any(char::is_control)
        && value.encode_utf16().count() <= 240
        && value.to_ascii_lowercase().ends_with(".json")
        && ![
            "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7",
            "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
            "COM¹", "COM²", "COM³", "LPT¹", "LPT²", "LPT³",
        ]
        .contains(&base.as_str())
}
fn reviewed(contents: &str) -> Result<Mapping, String> {
    if contents.len() > 128 * 1024 {
        return Err("Mapping draft exceeds 128 KiB.".into());
    }
    let value: Mapping = serde_json::from_str(contents)
        .map_err(|error| format!("Invalid mapping draft: {error}"))?;
    if value.services.is_empty()
        || value.services.len() > 128
        || value.volumes.is_empty()
        || value.volumes.len() > 128
    {
        return Err("Mapping requires a bounded service and volume selection.".into());
    }
    let mut used = BTreeSet::new();
    let mut mounts = 0;
    for (service, entry) in &value.services {
        if !key(service) || entry.volumes.is_empty() {
            return Err("Invalid mapped service.".into());
        }
        let mut targets = BTreeSet::new();
        for mount in &entry.volumes {
            mounts += 1;
            if mounts > 256
                || mount.kind != "volume"
                || !mount.volume.nocopy
                || !value.volumes.contains_key(&mount.source)
                || !mount.source.starts_with("orbwin_migration_")
                || !key(&mount.source)
                || !mount.target.starts_with('/')
                || mount.target == "/"
                || mount.target.contains("//")
                || mount.target.ends_with('/')
                || mount.target.len() > 4096
                || mount.target.chars().any(char::is_control)
                || mount
                    .target
                    .split('/')
                    .any(|part| part == "." || part == "..")
                || mount.target.replace("$$", "").contains('$')
                || !targets.insert(&mount.target)
            {
                return Err("Mapping contains an unsupported or repeated volume mount.".into());
            }
            used.insert(&mount.source);
        }
    }
    let mut destinations = BTreeSet::new();
    for (alias, external) in &value.volumes {
        if !used.contains(alias)
            || !external.external
            || !key(&external.name)
            || !destinations.insert(&external.name)
        {
            return Err("Mapping requires distinct existing external volume destinations.".into());
        }
    }
    Ok(value)
}
pub(crate) fn canonical(contents: &str) -> Result<String, String> {
    let value = reviewed(contents)?;
    let contents = serde_json::to_string_pretty(&value).map_err(|error| error.to_string())?;
    if contents.len() > 256 * 1024 {
        return Err("Serialized mapping draft exceeds 256 KiB.".into());
    }
    Ok(contents)
}
fn write_new(path: &Path, bytes: &[u8]) -> Result<(), String> {
    let mut file = std::fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .open(path)
        .map_err(|error| {
            format!("New mapping file could not be created (existing files are retained): {error}")
        })?;
    file.write_all(bytes)
        .and_then(|_| file.sync_all())
        .map_err(|error| {
            format!(
                "Mapping write failed. Inspect the retained file {} before retrying: {error}",
                path.display()
            )
        })
}

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct SavedMapping {
    path: String,
    sha256: String,
    target: String,
}

fn open_mapping(path: &Path) -> Result<std::fs::File, String> {
    let mut options = std::fs::OpenOptions::new();
    options.read(true);
    #[cfg(windows)]
    {
        use std::os::windows::fs::OpenOptionsExt;
        // Hold read access while denying concurrent write/delete. Inspect the
        // final reparse point itself rather than following a substituted link.
        options.share_mode(1).custom_flags(0x00200000);
    }
    let file = options
        .open(path)
        .map_err(|error| format!("Mapping file is unavailable: {error}"))?;
    let metadata = file.metadata().map_err(|error| error.to_string())?;
    #[cfg(windows)]
    {
        use std::os::windows::fs::MetadataExt;
        if metadata.file_attributes() & 0x400 != 0 {
            return Err("Mapping file is a reparse point; select a regular file.".into());
        }
    }
    if !metadata.is_file() || metadata.len() > 256 * 1024 {
        return Err("Mapping file must be a regular file within 256 KiB.".into());
    }
    Ok(file)
}

fn inspect_mapping(path: &Path, expected_sha: &str, contents: &str) -> Result<(), String> {
    if expected_sha.len() != 64
        || !expected_sha
            .bytes()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
    {
        return Err("Mapping checksum is invalid.".into());
    }
    let expected = reviewed(contents)?;
    let mut file = open_mapping(path)?;
    let mut bytes = Vec::new();
    (&mut file)
        .take(256 * 1024 + 1)
        .read_to_end(&mut bytes)
        .map_err(|error| error.to_string())?;
    if bytes.len() > 256 * 1024 || format!("{:x}", Sha256::digest(&bytes)) != expected_sha {
        return Err(
            "Saved mapping bytes changed. Review and save a new mapping; nothing was overwritten."
                .into(),
        );
    }
    let text = std::str::from_utf8(&bytes).map_err(|_| "Mapping is not UTF-8")?;
    let disk: Mapping = serde_json::from_str(text).map_err(|error| error.to_string())?;
    let compact = serde_json::to_string(&disk).map_err(|error| error.to_string())?;
    if reviewed(&compact)? != expected {
        return Err(
            "Saved mapping does not match the reviewed source mounts and destinations.".into(),
        );
    }
    Ok(())
}

// Inspection admits an existing final file. Resolve only its parent: resolving
// the file itself would follow a substituted reparse point before open_mapping
// can reject it. Save still uses the exclusive new-destination validator.
fn inspect_existing_mapping(raw: &str, expected_sha: &str, contents: &str) -> Result<(), String> {
    let path = crate::runtime::cli_path(Path::new(raw))?;
    let name = path
        .file_name()
        .and_then(|name| name.to_str())
        .filter(|name| file_name(name))
        .ok_or(
            "Choose a regular .json mapping filename without device or alternate-stream syntax.",
        )?;
    let parent = path
        .parent()
        .ok_or("Select the mapping directory.")?
        .canonicalize()
        .map_err(|error| format!("Mapping directory is unavailable: {error}"))?;
    if !parent.is_dir() {
        return Err("Mapping parent must be a directory.".into());
    }
    let selected = crate::runtime::cli_path(&parent)?.join(name);
    inspect_mapping(&selected, expected_sha, contents)
}

pub(crate) async fn inspect(
    path: String,
    expected_sha: String,
    contents: String,
    target_id: String,
) -> Result<(), String> {
    let target = crate::engine::target()?;
    if target.daemon_id() != target_id {
        return Err("The saved draft targets another engine. Refresh migration review.".into());
    }
    tokio::task::spawn_blocking(move || inspect_existing_mapping(&path, &expected_sha, &contents))
        .await
        .map_err(|error| error.to_string())?
}

pub(crate) async fn save(
    contents: String,
    target_id: String,
) -> Result<Option<SavedMapping>, String> {
    let value = reviewed(&contents)?;
    let target = crate::engine::target()?;
    let identity = crate::engine::check(&target).await?;
    if identity["ID"].as_str() != Some(target_id.as_str()) {
        return Err("The reviewed destination engine changed; refresh migration review.".into());
    }
    // Existing external volumes only. No implicit create and no driver options.
    let mut args = vec!["volume", "inspect"];
    for volume in value.volumes.values() {
        args.push(&volume.name);
    }
    let inspected = crate::engine::command(&target, &args).await?;
    let volumes: Vec<serde_json::Value> =
        serde_json::from_str(&inspected).map_err(|error| error.to_string())?;
    if volumes.len() != value.volumes.len()
        || value.volumes.values().any(|selected| {
            !volumes.iter().any(|volume| {
                volume["Name"] == selected.name
                    && volume["Driver"] == "local"
                    && volume["Scope"] == "local"
                    && (volume["Options"].is_null()
                        || volume["Options"]
                            .as_object()
                            .is_some_and(|options| options.is_empty()))
            })
        })
    {
        return Err("Choose existing ordinary OrbWin volumes without driver options.".into());
    }
    let Some(file) = rfd::AsyncFileDialog::new()
        .set_title("Save reviewed Compose volume mapping")
        .set_file_name("orbwin-volume-map.compose.json")
        .add_filter("Compose JSON", &["json"])
        .save_file()
        .await
    else {
        return Ok(None);
    };
    crate::engine::check(&target).await?;
    let current = crate::engine::command(&target, &args).await?;
    let current: Vec<serde_json::Value> =
        serde_json::from_str(&current).map_err(|error| error.to_string())?;
    if current != volumes {
        return Err("Destination volume metadata changed while choosing the file; review the mapping again.".into());
    }
    if !file
        .path()
        .file_name()
        .and_then(|name| name.to_str())
        .is_some_and(file_name)
    {
        return Err(
            "Choose a regular new .json filename, without device or alternate-stream syntax."
                .into(),
        );
    }
    let destination = crate::operations::host_destination(
        file.path()
            .to_str()
            .ok_or("Choose a Unicode Windows path.")?,
    )?;
    let bytes = serde_json::to_vec_pretty(&value).map_err(|error| error.to_string())?;
    if bytes.len() > 256 * 1024 {
        return Err("Serialized mapping draft exceeds 256 KiB.".into());
    }
    tokio::task::spawn_blocking(move || {
        write_new(Path::new(&destination), &bytes)?;
        Ok(Some(SavedMapping {
            path: destination,
            sha256: format!("{:x}", Sha256::digest(&bytes)),
            target: target_id,
        }))
    })
    .await
    .map_err(|error| error.to_string())?
}

#[cfg(test)]
mod tests {
    use super::*;
    fn draft() -> serde_json::Value {
        serde_json::json!({"services":{"web":{"volumes":[{"type":"volume","source":"orbwin_migration_test_1","target":"/data","read_only":false,"volume":{"nocopy":true}}]}},"volumes":{"orbwin_migration_test_1":{"external":true,"name":"restored"}}})
    }
    #[test]
    fn mapping_cannot_add_commands_create_volumes_or_interpolate_paths() {
        assert!(reviewed(&draft().to_string()).is_ok());
        for (pointer, replacement) in [
            ("/services/web/command", serde_json::json!(["sh"])),
            (
                "/volumes/orbwin_migration_test_1/external",
                serde_json::json!(false),
            ),
            (
                "/services/web/volumes/0/volume/nocopy",
                serde_json::json!(false),
            ),
            (
                "/services/web/volumes/0/target",
                serde_json::json!("/${ENV}"),
            ),
            (
                "/services/web/volumes/0/target",
                serde_json::json!("/../data"),
            ),
        ] {
            let mut value = draft();
            if pointer.ends_with("/command") {
                value["services"]["web"]["command"] = replacement;
            } else {
                *value.pointer_mut(pointer).unwrap() = replacement;
            }
            assert!(reviewed(&value.to_string()).is_err());
        }
        let mut value = draft();
        value["services"]["web"]["volumes"][0]["target"] = serde_json::json!("/$${ENV}");
        assert!(reviewed(&value.to_string()).is_ok());
    }
    #[test]
    fn mapping_reopen_checks_actual_bytes_and_reviewed_semantics() {
        let root = std::env::temp_dir().join(format!(
            "orbwin-map-reopen-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        std::fs::create_dir(&root).unwrap();
        let path = root.join("mapping.json");
        let reviewed_draft = draft().to_string();
        let bytes = serde_json::to_vec_pretty(&reviewed(&reviewed_draft).unwrap()).unwrap();
        let sha = format!("{:x}", Sha256::digest(&bytes));
        write_new(&path, &bytes).unwrap();
        assert!(inspect_existing_mapping(path.to_str().unwrap(), &sha, &reviewed_draft).is_ok());
        assert!(
            crate::operations::host_destination(path.to_str().unwrap()).is_err(),
            "save-new and reopen intentionally have different existence requirements"
        );
        #[cfg(windows)]
        {
            let link = root.join("selected-link.json");
            std::os::windows::fs::symlink_file(&path, &link).unwrap();
            assert!(
                inspect_existing_mapping(link.to_str().unwrap(), &sha, &reviewed_draft)
                    .unwrap_err()
                    .contains("reparse")
            );
            std::fs::remove_file(link).unwrap();
        }
        for selected in [
            "relative.json",
            r"C:relative.json",
            r"\\server\share\mapping.json",
        ] {
            assert!(inspect_existing_mapping(selected, &sha, &reviewed_draft).is_err());
        }
        assert!(inspect_existing_mapping(
            &format!("{}:stream.json", path.display()),
            &sha,
            &reviewed_draft
        )
        .is_err());
        assert_ne!(
            sha,
            format!("{:x}", Sha256::digest(reviewed_draft.as_bytes()))
        );
        let mut foreign = draft();
        foreign["volumes"]["orbwin_migration_test_1"]["name"] = serde_json::json!("another");
        assert!(inspect_mapping(&path, &sha, &foreign.to_string())
            .unwrap_err()
            .contains("does not match"));
        assert!(inspect_mapping(&path, &"f".repeat(64), &reviewed_draft).is_err());
        let mut changed = bytes.clone();
        changed.push(b'\n');
        std::fs::write(&path, &changed).unwrap();
        assert!(inspect_mapping(&path, &sha, &reviewed_draft)
            .unwrap_err()
            .contains("bytes changed"));
        // A changed-byte checksum is insufficient when the document can inject
        // other Compose behavior; schema and semantic checks still apply.
        let hostile = br#"{"services":{"web":{"command":"hostile"}},"volumes":{}}"#;
        std::fs::write(&path, hostile).unwrap();
        assert!(inspect_mapping(
            &path,
            &format!("{:x}", Sha256::digest(hostile)),
            &reviewed_draft
        )
        .is_err());
        std::fs::write(&path, vec![b' '; 256 * 1024 + 1]).unwrap();
        assert!(inspect_mapping(&path, &sha, &reviewed_draft).is_err());
        std::fs::remove_file(&path).unwrap();
        assert!(inspect_mapping(&path, &sha, &reviewed_draft).is_err());
        assert!(open_mapping(&root).is_err());
        std::fs::remove_dir(root).unwrap();
    }
    #[cfg(windows)]
    #[test]
    fn mapping_read_handle_denies_writers_and_delete_until_recheck_finishes() {
        let path = std::env::temp_dir().join(format!(
            "orbwin-map-share-{}-{}.json",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        write_new(&path, b"original").unwrap();
        let file = open_mapping(&path).unwrap();
        assert!(std::fs::OpenOptions::new().write(true).open(&path).is_err());
        assert!(std::fs::remove_file(&path).is_err());
        drop(file);
        std::fs::remove_file(&path).unwrap();
    }
    #[test]
    fn existing_mapping_file_is_never_overwritten() {
        assert!(file_name("mapping 例.JSON"));
        for name in [
            "draft.json:stream.json",
            "CON.json",
            "NUL.json",
            "LPT1.json",
            "draft.yml",
        ] {
            assert!(!file_name(name));
        }
        let root = std::env::temp_dir().join(format!(
            "orbwin-mapping-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        std::fs::create_dir(&root).unwrap();
        let file = root.join("mapping 例.json");
        write_new(&file, b"first").unwrap();
        assert!(write_new(&file, b"changed").is_err());
        assert_eq!(std::fs::read(&file).unwrap(), b"first");
        std::fs::remove_file(file).unwrap();
        std::fs::remove_dir(root).unwrap();
    }
}
