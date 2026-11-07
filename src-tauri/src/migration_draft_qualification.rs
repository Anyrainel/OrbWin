//! Ignored, file-bound fixture adapter for migration draft qualification. There
//! is no production IPC/command surface and no resource/lifecycle mutation.
use super::*;
use serde::Deserialize;
use std::path::{Path, PathBuf};

#[derive(Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Envelope {
    schema: u8,
    nonce: String,
    expected_target: String,
    step: Step,
}
#[derive(Deserialize)]
#[serde(tag = "kind", rename_all = "camelCase", deny_unknown_fields)]
enum Step {
    ReviewSource,
    InspectConnection {
        name: String,
        registration: String,
        source: String,
        signature: String,
    },
    InspectMapping {
        path: String,
        sha256: String,
        contents: String,
    },
    Validate {
        request: serde_json::Value,
    },
}
fn fixture_path(path: &str, root: &Path) -> Result<PathBuf, String> {
    let candidate = PathBuf::from(path);
    let metadata = std::fs::symlink_metadata(&candidate).map_err(|e| e.to_string())?;
    #[cfg(windows)]
    {
        use std::os::windows::fs::MetadataExt;
        if metadata.file_attributes() & 0x400 != 0 {
            return Err("fixture reparse file refused".into());
        }
    }
    if !metadata.is_file() {
        return Err("fixture must be regular".into());
    }
    let canonical = candidate.canonicalize().map_err(|e| e.to_string())?;
    if !canonical.starts_with(root) {
        return Err("fixture is outside its nonce directory".into());
    }
    Ok(canonical)
}
#[tokio::test]
#[ignore = "explicit reviewed nonce migration fixture only; no setup/Up/engine lifecycle"]
async fn owned_migration_draft_step() {
    use sha2::{Digest, Sha256};
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    assert_eq!(std::env::var("ORBWIN_QUALIFY_DRAFT").as_deref(), Ok("1"));
    let nonce = std::env::var("ORBWIN_QUALIFY_DRAFT_NONCE").expect("reviewed fixture nonce");
    assert!(
        nonce.len() == 32
            && nonce
                .bytes()
                .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
    );
    let workspace = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("..")
        .canonicalize()
        .unwrap();
    let parent = workspace
        .join("artifacts/qualification")
        .canonicalize()
        .unwrap();
    let named_folder = parent.join(format!("orbwin-draft-{nonce}"));
    let folder = named_folder.canonicalize().unwrap();
    assert_eq!(folder, named_folder, "fixture parent must not redirect");
    let input = PathBuf::from(
        std::env::var("ORBWIN_QUALIFY_DRAFT_REQUEST").expect("exact reviewed request file"),
    );
    let input = fixture_path(input.to_str().unwrap(), &folder).unwrap();
    use std::io::Read;
    let mut bytes = Vec::new();
    std::fs::File::open(&input)
        .unwrap()
        .take((1 << 20) + 1)
        .read_to_end(&mut bytes)
        .unwrap();
    assert!(bytes.len() <= 1 << 20);
    assert_eq!(
        format!("{:x}", Sha256::digest(&bytes)),
        std::env::var("ORBWIN_QUALIFY_DRAFT_REQUEST_SHA256").unwrap()
    );
    let envelope: Envelope = serde_json::from_slice(&bytes).unwrap();
    assert_eq!(envelope.schema, 1);
    assert_eq!(envelope.nonce, nonce);
    assert_eq!(
        envelope.expected_target,
        "d9c00194-ea9d-439a-8561-6a0ec580fbd5"
    );
    let target = engine::target().unwrap();
    assert_eq!(
        engine::check(&target).await.unwrap()["ID"],
        envelope.expected_target
    );
    let evidence = input.with_extension("response.json");
    assert!(!evidence.exists());
    let native_records = input.with_extension("native-records.json");
    std::fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .open(&native_records)
        .unwrap();
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: native_records,
    };
    let project = format!("orbwin-draft-{}", envelope.nonce);
    let wsl_root = "/home/vanyr/orbwin-native-proof-20261024";
    let registration = "231810b0-fa21-48a8-b0a7-86f9a1360029";
    let result:Result<serde_json::Value,String>=async {match envelope.step {
  Step::ReviewSource=>{let (outcome,text)=harness.call(Request::ReviewDesktopMigration).await?;Ok(serde_json::json!({"success":outcome.success,"mutation":outcome.mutation,"stdout":text}))},
  Step::InspectConnection{name,registration:chosen,source,signature}=>{
   if name!="Ubuntu"||chosen!=registration||source!=wsl_root{return Err("unapproved WSL fixture connection".into());}
   let value=serde_json::to_value(crate::wsl_integration::wsl_migration_project(name,chosen).await?).map_err(|e|e.to_string())?;
   if value["connected"]!=true||value["export"]["source"]!=source||value["export"]["signature"]!=signature{return Err("connected project changed".into());}Ok(value)
  },
  Step::InspectMapping{path,sha256,contents}=>{
   fixture_path(&path,&folder)?;crate::migration_mapping::inspect(path,sha256,contents,envelope.expected_target.clone()).await?;Ok(serde_json::json!({"success":true,"mutation":false}))
  },
  Step::Validate{request}=>{
   let request:Request=serde_json::from_value(request).map_err(|e|e.to_string())?;
   match &request {
    Request::Compose{file,overrides,env_files,directory,profiles,services,project:name,action:ComposeAction::Validate,..}=>{
     if name!=&project||!env_files.is_empty()||!profiles.is_empty()||!services.is_empty()||Path::new(directory).canonicalize().ok().as_ref()!=Some(&folder){return Err("unexpected Windows fixture inputs".into());}
     for path in std::iter::once(file).chain(overrides){fixture_path(path,&folder)?;}
    },
    Request::WslCompose{distribution,connection:Some(connection),reviewed_daemon:Some(daemon),mapping_review:Some(mapping_review),file,overrides,env_files,directory,profiles,services,project:name,action:ComposeAction::Validate,..}=>{
     if distribution!="Ubuntu"||connection.registration.as_deref()!=Some(registration)||connection.source!=wsl_root||daemon!=&envelope.expected_target||name!=&project||!env_files.is_empty()||!profiles.is_empty()||!services.is_empty(){return Err("unexpected WSL fixture inputs".into());}
     let child=format!("{wsl_root}/{project}/");let mapping=format!("{wsl_root}/{project}.json");
     if directory!=&format!("{wsl_root}/{project}")||mapping_review.file!=format!("{project}.json")||!file.starts_with(&child)||overrides.len()!=1||overrides[0]!=mapping{return Err("WSL fixture paths differ".into());}
    },
    _=>return Err("only typed fixture Compose Validate is admitted".into()),
   }
   let (outcome,text)=harness.call(request).await?;Ok(serde_json::json!({"success":outcome.success,"mutation":outcome.mutation,"stdout":text}))
  }
 }}.await;
    let response = serde_json::json!({"schema":1,"nonce":envelope.nonce,"result":result,"targetAfter":engine::check(&target).await.unwrap()["ID"]});
    use std::io::Write;
    let mut output = std::fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .open(&evidence)
        .unwrap();
    output
        .write_all(&serde_json::to_vec_pretty(&response).unwrap())
        .unwrap();
    output.sync_all().unwrap();
}
