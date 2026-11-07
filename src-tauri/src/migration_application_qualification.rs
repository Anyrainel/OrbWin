//! Reviewed fixture bridge to the real operation adapter, not a product command.
//! The external fixture owner prepares/imports resources, checks application data
//! and performs exact cleanup. This process owns only one Validate/Up/Stop client.
//! It journals intent before dispatch and never retries or compensates an unknown
//! mutation. A failed/terminated process requires the owner to inspect its journal
//! and engine state before cleanup. No engine lifecycle or Compose Down is admitted.
use super::*;
use serde::Deserialize;
use sha2::{Digest, Sha256};
use std::{
    io::{Read, Write},
    path::{Path, PathBuf},
};

const TARGET: &str = "d9c00194-ea9d-439a-8561-6a0ec580fbd5";

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Envelope {
    schema: u8,
    nonce: String,
    expected_target: String,
    base_sha256: String,
    mapping_sha256: String,
    action: Action,
}
#[derive(Deserialize, Debug)]
#[serde(rename_all = "camelCase")]
enum Action {
    Validate,
    Up,
    Stop,
}

fn hex(value: &str, len: usize) -> bool {
    value.len() == len
        && value
            .bytes()
            .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
}
fn parse(bytes: &[u8], nonce: &str) -> Result<Envelope, String> {
    let envelope: Envelope = serde_json::from_slice(bytes).map_err(|e| e.to_string())?;
    if envelope.schema != 1
        || envelope.nonce != nonce
        || !hex(nonce, 32)
        || envelope.expected_target != TARGET
        || !hex(&envelope.base_sha256, 64)
        || !hex(&envelope.mapping_sha256, 64)
    {
        return Err("Unapproved migration application fixture identity".into());
    }
    Ok(envelope)
}
// Single direct children only. Every component below the repository is checked
// for redirection; this is a cooperating-user fixture, not hostile-user CAS.
fn direct_file(folder: &Path, name: &str, digest: &str) -> Result<Vec<u8>, String> {
    if !hex(digest, 64)
        || name.is_empty()
        || name.contains(['/', '\\', ':'])
        || name == "."
        || name == ".."
    {
        return Err("Invalid direct fixture file".into());
    }
    let path = folder.join(name);
    let metadata = std::fs::symlink_metadata(&path).map_err(|e| e.to_string())?;
    #[cfg(windows)]
    {
        use std::os::windows::fs::MetadataExt;
        if metadata.file_attributes() & 0x400 != 0 {
            return Err("Fixture reparse file refused".into());
        }
    }
    if !metadata.is_file() || metadata.file_type().is_symlink() || metadata.len() > 1 << 20 {
        return Err("Fixture file is not bounded regular data".into());
    }
    let mut bytes = Vec::new();
    std::fs::File::open(&path)
        .map_err(|e| e.to_string())?
        .take((1 << 20) + 1)
        .read_to_end(&mut bytes)
        .map_err(|e| e.to_string())?;
    if bytes.len() > 1 << 20 || format!("{:x}", Sha256::digest(&bytes)) != digest {
        return Err("Reviewed fixture file changed".into());
    }
    Ok(bytes)
}
fn write_new(path: &Path, value: &serde_json::Value) -> Result<(), String> {
    let mut file = std::fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .open(path)
        .map_err(|e| e.to_string())?;
    file.write_all(&serde_json::to_vec_pretty(value).map_err(|e| e.to_string())?)
        .map_err(|e| e.to_string())?;
    file.sync_all().map_err(|e| e.to_string())
}

#[test]
fn application_bridge_refuses_other_actions_targets_and_ambiguous_envelopes() {
    let nonce = "a".repeat(32);
    let valid = serde_json::json!({"schema":1,"nonce":nonce,"expectedTarget":TARGET,
        "baseSha256":"b".repeat(64),"mappingSha256":"c".repeat(64),"action":"up"});
    assert!(parse(&serde_json::to_vec(&valid).unwrap(), &nonce).is_ok());
    for (key, value) in [
        ("action", "down"),
        ("action", "pull"),
        ("expectedTarget", "foreign"),
        ("nonce", "different"),
        ("baseSha256", "ABC"),
        ("command", "up"),
    ] {
        let mut changed = valid.clone();
        changed[key] = value.into();
        assert!(parse(&serde_json::to_vec(&changed).unwrap(), &nonce).is_err());
    }
    let duplicate =
        serde_json::to_string(&valid)
            .unwrap()
            .replacen('{', "{\"action\":\"stop\",", 1);
    assert!(parse(duplicate.as_bytes(), &nonce).is_err());
    assert!(parse(format!("{} trailing", valid).as_bytes(), &nonce).is_err());
}

#[tokio::test]
#[ignore = "reviewed external nonce fixture only; real Compose Validate/Up/Stop, never engine lifecycle"]
async fn owned_migration_application_step() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    assert_eq!(
        std::env::var("ORBWIN_QUALIFY_APPLICATION").as_deref(),
        Ok("1")
    );
    let nonce = std::env::var("ORBWIN_QUALIFY_APPLICATION_NONCE").unwrap();
    assert!(hex(&nonce, 32));
    let workspace = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("..")
        .canonicalize()
        .unwrap();
    let mut folder = workspace;
    for component in [
        "artifacts",
        "qualification",
        &format!("orbwin-project-{nonce}"),
    ] {
        folder.push(component);
        let metadata = std::fs::symlink_metadata(&folder).unwrap();
        assert!(metadata.is_dir() && !metadata.file_type().is_symlink());
        #[cfg(windows)]
        {
            use std::os::windows::fs::MetadataExt;
            assert_eq!(
                metadata.file_attributes() & 0x400,
                0,
                "redirected fixture directory"
            );
        }
    }
    let name = std::env::var("ORBWIN_QUALIFY_APPLICATION_REQUEST").unwrap();
    let digest = std::env::var("ORBWIN_QUALIFY_APPLICATION_REQUEST_SHA256").unwrap();
    let bytes = direct_file(&folder, &name, &digest).unwrap();
    let envelope = parse(&bytes, &nonce).unwrap();
    direct_file(&folder, "base.json", &envelope.base_sha256).unwrap();
    direct_file(&folder, "restored.compose.json", &envelope.mapping_sha256).unwrap();
    let target = engine::target().unwrap();
    assert_eq!(engine::check(&target).await.unwrap()["ID"], TARGET);
    let input = folder.join(&name);
    let intent = input.with_extension("intent.json");
    let response = input.with_extension("response.json");
    let records = input.with_extension("native-records.json");
    assert!(!response.exists() && !records.exists());
    write_new(
        &intent,
        &serde_json::json!({"schema":1,"nonce":nonce,"requestSha256":digest,
        "action":format!("{:?}",envelope.action),"phase":"dispatch-intent","target":TARGET}),
    )
    .unwrap();
    // Reserve output before dispatch. Partial/absent response does not imply that
    // a mutation failed to commit; only the external owner may resolve cleanup.
    std::fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .open(&records)
        .unwrap();
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: records,
    };
    let action = match envelope.action {
        Action::Validate => ComposeAction::Validate,
        Action::Up => ComposeAction::Up,
        Action::Stop => ComposeAction::Stop,
    };
    let result = harness
        .call(Request::Compose {
            file: folder.join("base.json").to_string_lossy().into_owned(),
            overrides: vec![folder
                .join("restored.compose.json")
                .to_string_lossy()
                .into_owned()],
            env_files: vec![],
            profiles: vec![],
            services: vec![],
            directory: folder.to_string_lossy().into_owned(),
            project: format!("orbwin-project-{nonce}"),
            action,
        })
        .await;
    let files_unchanged = direct_file(&folder, "base.json", &envelope.base_sha256).is_ok()
        && direct_file(&folder, "restored.compose.json", &envelope.mapping_sha256).is_ok();
    let target_after = engine::check(&target).await;
    let successful = result.as_ref().is_ok_and(|(outcome, _)| outcome.success)
        && files_unchanged
        && target_after
            .as_ref()
            .is_ok_and(|value| value["ID"] == TARGET);
    write_new(&response, &serde_json::json!({"schema":1,"nonce":nonce,"success":successful,
        "result":result.as_ref().map(|(outcome,text)|serde_json::json!({"outcome":outcome,"stdout":text})),
        "filesUnchanged":files_unchanged,"targetAfter":target_after})).unwrap();
    assert!(
        successful,
        "Actual Rust operation did not qualify; inspect {} before cleanup",
        response.display()
    );
}
