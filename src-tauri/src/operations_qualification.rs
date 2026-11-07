//! Explicit opt-in tests of the actual native operation adapter against the
//! recorded owned development engine. Only uniquely named test resources are
//! removed; no global lifecycle, context, prune or competitor operation.
#[path = "migration_application_qualification.rs"]
mod migration_application_qualification;
#[path = "migration_draft_qualification.rs"]
mod migration_draft_qualification;

use super::*;
use std::sync::{
    atomic::{AtomicU64, Ordering},
    Arc,
};

static SEQUENCE: AtomicU64 = AtomicU64::new(1);

#[tokio::test]
#[ignore = "new WSL project mapping write/refusal/validation under the existing qualification root"]
async fn owned_wsl_mapping_save() {
    use sha2::Digest;
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let registration = "231810b0-fa21-48a8-b0a7-86f9a1360029";
    let observed = serde_json::to_value(
        crate::wsl_integration::wsl_migration_project("Ubuntu".into(), registration.into())
            .await
            .unwrap(),
    )
    .unwrap();
    assert_eq!(observed["connected"], true);
    let source = observed["export"]["source"].as_str().unwrap().to_string();
    let signature = observed["export"]["signature"]
        .as_str()
        .unwrap()
        .to_string();
    assert_eq!(source, "/home/vanyr/orbwin-native-proof-20261024");
    let target = engine::target().unwrap();
    let identity = engine::check(&target).await.unwrap();
    assert_eq!(identity["ID"], "d9c00194-ea9d-439a-8561-6a0ec580fbd5");
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_nanos();
    let filename = format!("orbwin-wsl-map-{stamp} 例.json");
    let base_name = format!("orbwin-wsl-map-base-{stamp}.json");
    let path = format!("{source}/{filename}");
    let base_path = format!("{source}/{base_name}");
    let destination = "1d6214631053b7caea902b26cee50318521892c6d1e131ff6970677fcabef447";
    let contents=serde_json::json!({"services":{"web":{"volumes":[{"type":"volume","source":"orbwin_migration_test_1","target":"/data","read_only":true,"volume":{"nocopy":true}}]}},"volumes":{"orbwin_migration_test_1":{"external":true,"name":destination}}}).to_string();
    let canonical = crate::migration_mapping::canonical(&contents).unwrap();
    let digest = format!("{:x}", sha2::Sha256::digest(canonical.as_bytes()));
    let request = |file: String, expected: String, daemon: String| Request::WslSaveMapping {
        distribution: "Ubuntu".into(),
        connection: crate::wsl_integration::ConnectionReview {
            registration: Some(registration.into()),
            source: source.clone(),
            signature: expected,
        },
        file,
        contents: contents.clone(),
        target: daemon,
    };
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../artifacts/qualification/p2-wsl-mapping-native-records-20261025.json"),
    };
    let binary = crate::wsl::wsl_binary();
    let guid = format!("{{{registration}}}");
    let uid =
        crate::process::read_command(&binary, &["--distribution-id", &guid, "--exec", "id", "-u"])
            .await
            .unwrap();
    assert_eq!(uid.trim(), "1000");
    let mut created = false;
    let mut base_created = false;
    let base=serde_json::json!({"services":{"web":{"image":"alpine:3.21@sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30","network_mode":"none","command":["true"]}}}).to_string();
    let result: Result<(), String> = async {
        assert!(harness
            .call(request(
                format!("wrong-{stamp}.json"),
                "0:0".into(),
                target.daemon_id().into()
            ))
            .await
            .is_err());
        assert!(harness
            .call(request(
                format!("wrong-daemon-{stamp}.json"),
                signature.clone(),
                "changed".into()
            ))
            .await
            .is_err());
        // Mark intent before dispatch so any uncertain committed file is audited.
        created = true;
        let text = harness
            .pass(request(
                filename.clone(),
                signature.clone(),
                target.daemon_id().into(),
            ))
            .await?;
        let receipt: serde_json::Value =
            serde_json::from_str(&text).map_err(|error| error.to_string())?;
        if receipt["path"] != path
            || receipt["signature"] != signature
            || receipt["sha256"] != digest
        {
            return Err("Saved mapping receipt differs".into());
        }
        let read = crate::process::read_command(
            &binary,
            &["--distribution-id", &guid, "--exec", "cat", &path],
        )
        .await?;
        if read != canonical {
            return Err("Saved bytes differ".into());
        }
        let permissions = crate::process::read_command(
            &binary,
            &[
                "--distribution-id",
                &guid,
                "--exec",
                "stat",
                "-c",
                "%u:%a",
                &path,
            ],
        )
        .await?;
        if permissions.trim() != "1000:600" {
            return Err(format!("Mapping ownership differs: {permissions}"));
        }
        let (outcome, diagnostic) = harness
            .call(request(
                filename.clone(),
                signature.clone(),
                target.daemon_id().into(),
            ))
            .await?;
        if outcome.success || !diagnostic.contains("existing files are retained") {
            return Err("Existing mapping was not refused".into());
        }
        let retained = crate::process::read_command(
            &binary,
            &["--distribution-id", &guid, "--exec", "cat", &path],
        )
        .await?;
        if retained != canonical {
            return Err("Existing mapping was modified".into());
        }
        crate::process::read_command(
            &binary,
            &[
                "--distribution-id",
                &guid,
                "--exec",
                "sh",
                "-c",
                r#"set -euC; umask 077; printf '%s' "$2" > "$1""#,
                "orbwin",
                &base_path,
                &base,
            ],
        )
        .await?;
        base_created = true;
        let text = harness
            .pass(Request::WslCompose {
                reviewed_daemon: None,
                mapping_review: None,
                distribution: "Ubuntu".into(),
                connection: Some(crate::wsl_integration::ConnectionReview {
                    registration: Some(registration.into()),
                    source: source.clone(),
                    signature: signature.clone(),
                }),
                file: base_path.clone(),
                overrides: vec![path.clone()],
                env_files: vec![],
                project: format!("orbwin-wsl-map-{stamp}"),
                directory: source.clone(),
                profiles: vec![],
                services: vec![],
                action: ComposeAction::Validate,
            })
            .await?;
        if text.trim() != "Compose configuration and selected project paths validated." {
            return Err("Mapping validation did not return the bounded confirmation".into());
        }
        Ok(())
    }
    .await;
    let mut cleanup = vec![];
    for (file, expected, owned) in [
        (&path, digest.clone(), created),
        (
            &base_path,
            format!("{:x}", sha2::Sha256::digest(base.as_bytes())),
            base_created,
        ),
    ] {
        if owned {
            if let Err(error)=crate::process::read_command(&binary,&["--distribution-id",&guid,"--exec","sh","-c",r#"set -eu; test ! -L "$1"; test "$(stat -Lc '%d:%i' "$1")" = "$2"; test ! -L "$3"; actual=$(sha256sum -- "$3"); test "${actual%% *}" = "$4"; rm -- "$3""#,"orbwin",&source,&signature,file,&expected]).await{cleanup.push(error)}
        }
    }
    let evidence = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../artifacts/qualification/p2-wsl-mapping-native-20261025.json");
    std::fs::write(evidence,serde_json::to_vec_pretty(&serde_json::json!({"result":result,"cleanupErrors":cleanup,"root":source,"signature":signature,"filename":filename,"uid":uid,"sha256":digest})).unwrap()).unwrap();
    assert!(
        result.is_ok() && cleanup.is_empty(),
        "result: {result:?}; cleanup: {cleanup:?}"
    );
}

#[tokio::test]
#[ignore = "read-only selected ordinary owned volume size and stale-selection refusal"]
async fn owned_volume_size() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let target = engine::target().unwrap();
    let before = engine::check(&target).await.unwrap();
    assert_eq!(before["ID"], "d9c00194-ea9d-439a-8561-6a0ec580fbd5");
    let name = "1d6214631053b7caea902b26cee50318521892c6d1e131ff6970677fcabef447";
    let inspected = engine::command(&target, &["volume", "inspect", name])
        .await
        .unwrap();
    let details: serde_json::Value = serde_json::from_str(&inspected).unwrap();
    let created = details[0]["CreatedAt"].as_str().unwrap().to_string();
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../artifacts/qualification/p2-volume-size-native-20261025.json"),
    };
    let output = harness
        .pass(Request::VolumeSize {
            target: name.into(),
            created_at: created.clone(),
        })
        .await
        .unwrap();
    let value: serde_json::Value = serde_json::from_str(&output).unwrap();
    assert_eq!(value["name"], name);
    assert_eq!(value["createdAt"], created);
    assert_eq!(value["bytes"], 0);
    assert_eq!(value["references"], 0);
    let (outcome, output) = harness
        .call(Request::VolumeSize {
            target: name.into(),
            created_at: "stale".into(),
        })
        .await
        .unwrap();
    assert!(!outcome.success && !outcome.mutation);
    assert!(output.contains("volume selection changed"));
    assert_eq!(before["ID"], engine::check(&target).await.unwrap()["ID"]);
}

#[tokio::test]
#[ignore = "read-only owned stats admission cancellation and next-read recovery"]
async fn owned_activity_cancel() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let target = engine::target().unwrap();
    let before = engine::check(&target).await.unwrap();
    assert_eq!(before["ID"], "d9c00194-ea9d-439a-8561-6a0ec580fbd5");
    let state = Operations::default();
    let job_id = id();
    let (ready_tx, ready_rx) = oneshot::channel();
    let ready = Mutex::new(Some(ready_tx));
    let channel = Channel::new(move |body| {
        let event: serde_json::Value = body.deserialize()?;
        if event["kind"] == "ready" {
            if let Some(sender) = ready.lock().unwrap().take() {
                let _ = sender.send(());
            }
        }
        Ok(())
    });
    let cancelling = async {
        tokio::time::timeout(Duration::from_secs(10), ready_rx)
            .await
            .unwrap()
            .unwrap();
        cancel(&state, &job_id).unwrap();
    };
    let (outcome, ()) = tokio::time::timeout(Duration::from_secs(20), async {
        tokio::join!(
            run(&state, job_id.clone(), Request::EngineStats, channel),
            cancelling
        )
    })
    .await
    .unwrap();
    match &outcome {
        Ok(outcome) => assert!(outcome.interrupted && !outcome.success && !outcome.mutation),
        Err(message) => assert_eq!(message, "Cancelled before dispatch. No operation was sent."),
    }
    assert!(state.0.lock().unwrap().is_empty());
    let next = run(&state, id(), Request::EngineStats, Channel::new(|_| Ok(())))
        .await
        .unwrap();
    assert!(next.success && !next.mutation);
    assert!(state.0.lock().unwrap().is_empty());
    assert_eq!(before["ID"], engine::check(&target).await.unwrap()["ID"]);
    let path = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../artifacts/qualification/p2-activity-cancel-native-20261025.json");
    std::fs::write(path, serde_json::to_vec_pretty(&serde_json::json!({"scope":"cancel after admission, before stats payload; not evidence of cancelling a collecting CLI", "cancelled":outcome,"nextRead":next,"registryEmpty":true,"daemon":before["ID"]})).unwrap()).unwrap();
}

#[tokio::test]
#[ignore = "read-only engine and selected-container stats on the recorded owned engine"]
async fn owned_activity_stats() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let target = engine::target().unwrap();
    let before = engine::check(&target).await.unwrap();
    assert_eq!(before["ID"], "d9c00194-ea9d-439a-8561-6a0ec580fbd5");
    let root =
        std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../artifacts/qualification");
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: root.join("p2-activity-native-20261025.json"),
    };
    let (outcome, output) = harness.call(Request::EngineStats).await.unwrap();
    assert!(outcome.success && !outcome.mutation);
    let rows: Vec<serde_json::Value> = output
        .lines()
        .map(|line| serde_json::from_str(line).unwrap())
        .collect();
    assert!(
        !rows.is_empty(),
        "This proof expects the existing running Kubernetes containers"
    );
    let id = rows[0]["ID"].as_str().unwrap().to_string();
    assert_eq!(id.len(), 64);
    let (outcome, selected) = harness
        .call(Request::Stats { target: id.clone() })
        .await
        .unwrap();
    assert!(outcome.success && !outcome.mutation);
    let row: serde_json::Value = serde_json::from_str(selected.trim()).unwrap();
    assert_eq!(row["ID"], id);
    assert_eq!(before["ID"], engine::check(&target).await.unwrap()["ID"]);
}

#[tokio::test]
#[ignore = "explicit owned-engine ordered Compose env-file lifecycle proof"]
async fn owned_compose_env_files() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    assert!(
        std::env::var("ORBWIN_COMPOSE_ENVFILE_PROOF").is_err(),
        "Do not override an existing launcher variable"
    );
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_nanos();
    let project = format!("orbwin-env-{stamp}");
    let root =
        std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../artifacts/qualification");
    let fixture = root.join(format!("compose-env-{stamp}"));
    std::fs::create_dir(&fixture).unwrap();
    let base = fixture.join("compose 例.json");
    let first = fixture.join("first 例.env");
    let last = fixture.join("last 例.env");
    std::fs::write(&first, "ORBWIN_COMPOSE_ENVFILE_PROOF=first\n").unwrap();
    std::fs::write(&last, "ORBWIN_COMPOSE_ENVFILE_PROOF=last\n").unwrap();
    std::fs::write(
        fixture.join(".env"),
        "ORBWIN_COMPOSE_ENVFILE_PROOF=hidden\n",
    )
    .unwrap();
    std::fs::write(&base,serde_json::to_vec_pretty(&serde_json::json!({"services":{"probe":{"image":"alpine:3.21@sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30","command":["sleep","120"],"network_mode":"none","labels":{"dev.orbwin.qualification":stamp.to_string(),"dev.orbwin.env-proof":"${ORBWIN_COMPOSE_ENVFILE_PROOF:?select env files}"}}}})).unwrap()).unwrap();
    let files = vec![
        first.to_string_lossy().into_owned(),
        last.to_string_lossy().into_owned(),
    ];
    let request = |action, env_files: Vec<String>| Request::Compose {
        file: base.to_string_lossy().into_owned(),
        env_files,
        overrides: vec![],
        profiles: vec![],
        services: vec![],
        directory: fixture.to_string_lossy().into_owned(),
        project: project.clone(),
        action,
    };
    let target = engine::target().unwrap();
    engine::check(&target).await.unwrap();
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: root.join(format!("compose-env-{stamp}-records.json")),
    };
    let filter = format!("label=com.docker.compose.project={project}");
    let result: Result<(), String> = async {
        let (outcome, text) = harness
            .call(request(ComposeAction::Validate, files.clone()))
            .await?;
        if !outcome.success || !text.trim().is_empty() || outcome.mutation {
            return Err("Env validation was not quiet".into());
        }
        let members = engine::command(&target, &["ps", "-aq", "--filter", &filter]).await?;
        if !members.is_empty() {
            return Err("Validation created a workload".into());
        }
        for (selected, expected) in [
            (files.clone(), "last"),
            (files.iter().rev().cloned().collect(), "first"),
        ] {
            harness.pass(request(ComposeAction::Up, selected)).await?;
            let members =
                engine::command(&target, &["ps", "-aq", "--no-trunc", "--filter", &filter]).await?;
            let ids: Vec<_> = members.lines().collect();
            if ids.len() != 1 {
                return Err("Env proof service count differs".into());
            }
            let text = engine::command(&target, &["inspect", ids[0]]).await?;
            let value: serde_json::Value =
                serde_json::from_str(&text).map_err(|error| error.to_string())?;
            if value[0]["Config"]["Labels"]["dev.orbwin.env-proof"] != expected
                || value[0]["Config"]["Labels"]["dev.orbwin.qualification"] != stamp.to_string()
            {
                return Err("Ordered env selection or fixture owner differs".into());
            }
        }
        if harness
            .call(request(
                ComposeAction::Validate,
                vec![fixture.join("missing.env").to_string_lossy().into_owned()],
            ))
            .await
            .is_ok()
        {
            return Err("Missing env file reached dispatch".into());
        }
        Ok(())
    }
    .await;
    let cleanup = harness.pass(request(ComposeAction::Down, files)).await;
    let members = engine::command(&target, &["ps", "-aq", "--filter", &filter])
        .await
        .unwrap();
    std::fs::write(root.join("p2-compose-env-windows-native-20261025.json"),serde_json::to_vec_pretty(&serde_json::json!({"success":result.is_ok()&&cleanup.is_ok()&&members.is_empty(),"failure":result.as_ref().err(),"cleanupFailure":cleanup.as_ref().err(),"project":project,"fixture":fixture,"quietValidation":true,"orderedAndReversedSelectionVerified":result.is_ok(),"missingEnvFileRefusedBeforeDispatch":result.is_ok(),"containersRemain":!members.is_empty(),"records":harness.evidence})).unwrap()).unwrap();
    assert!(
        result.is_ok() && cleanup.is_ok() && members.is_empty(),
        "result {result:?}; cleanup {cleanup:?}"
    );
}

#[tokio::test]
#[ignore = "explicit recorded owned target; observes local source receipts only"]
async fn owned_source_volume_history() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let root =
        std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../artifacts/qualification");
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: root.join("p2-source-history-native-records-20261025.json"),
    };
    let text = harness.pass(Request::MigrationVolumeHistory).await.unwrap();
    let value: serde_json::Value = serde_json::from_str(&text).unwrap();
    let records = value["records"].as_array().unwrap();
    assert!(records.len() <= 256);
    for record in records {
        assert!(record["transaction"].as_str().unwrap().len() == 32);
        assert!(record["uncertain"].is_boolean());
        assert!(record.get("export").is_none());
        assert!(record.get("assetSha").is_none());
    }
    std::fs::write(
        root.join("p2-source-history-native-20261025.json"),
        serde_json::to_vec_pretty(&value).unwrap(),
    )
    .unwrap();
}

#[tokio::test]
#[ignore = "explicit existing connected qualification project; no mount changes or distro termination"]
async fn owned_wsl_migration_handoff() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let registration = std::env::var("ORBWIN_QUALIFY_WSL_REGISTRATION")
        .expect("explicit ordinary WSL registration required");
    let name = "Ubuntu";
    let observed = serde_json::to_value(
        crate::wsl_integration::wsl_migration_project(name.into(), registration.clone())
            .await
            .unwrap(),
    )
    .unwrap();
    assert_eq!(
        observed["connected"], true,
        "Preserve existing selection; reconnect explicitly outside this proof if needed"
    );
    let source = observed["export"]["source"].as_str().unwrap().to_owned();
    let signature = observed["export"]["signature"].as_str().unwrap().to_owned();
    assert_eq!(
        source, "/home/vanyr/orbwin-native-proof-20261024",
        "Only the existing qualification project may receive fixture files"
    );
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_nanos();
    let project = format!("orbwin-wsl-migration-{stamp}");
    let fixture = format!("{source}/migration-review-{stamp}");
    let root =
        std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../artifacts/qualification");
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: root.join(format!("wsl-migration-review-{stamp}.json")),
    };
    let binary = crate::wsl::wsl_binary();
    let uid = crate::process::read_command(
        &binary,
        &[
            "--distribution-id",
            &format!("{{{registration}}}"),
            "--exec",
            "id",
            "-u",
        ],
    )
    .await
    .unwrap();
    assert_ne!(
        uid, "0",
        "Workflow qualification requires the ordinary default user"
    );
    let base = serde_json::json!({"services":{"probe":{"image":"rancher/mirrored-library-busybox:1.37.0","command":["true"],"ports":["127.0.0.1:${ORBWIN_ENV_FILE_PROOF:?select explicit env files}:80"],"labels":{"dev.orbwin.qualification":stamp.to_string()},"volumes":["data:/data"]}},"volumes":{"data":{"name":format!("{project}-data"),"labels":{"dev.orbwin.qualification":stamp.to_string()}}}}).to_string();
    let overrides = serde_json::json!({"services":{"probe":{"command":["false"]}}}).to_string();
    crate::process::read_command(&binary, &["--distribution-id", &format!("{{{registration}}}"), "--exec", "sh", "-c", r#"set -eu; mkdir -- "$1"; printf '%s' "$2" > "$1/base 例.json"; printf '%s' "$3" > "$1/override 例.json"; printf 'ORBWIN_ENV_FILE_PROOF=invalid-port\n' > "$1/first 例.env"; printf 'ORBWIN_ENV_FILE_PROOF=17999\n' > "$1/last 例.env"; chmod 600 "$1/base 例.json" "$1/override 例.json" "$1/first 例.env" "$1/last 例.env""#, "orbwin", &fixture, &base, &overrides]).await.unwrap();
    let request = |registration: String, expected: String| Request::WslCompose {
        reviewed_daemon: None,
        mapping_review: None,
        env_files: vec![
            format!("{fixture}/first 例.env"),
            format!("{fixture}/last 例.env"),
        ],
        distribution: name.into(),
        connection: Some(crate::wsl_integration::ConnectionReview {
            registration: Some(registration),
            source: expected,
            signature: signature.clone(),
        }),
        file: format!("{fixture}/base 例.json"),
        overrides: vec![format!("{fixture}/override 例.json")],
        directory: fixture.clone(),
        project: project.clone(),
        profiles: vec![],
        services: vec![],
        action: ComposeAction::Validate,
    };
    harness
        .pass(request(registration.clone(), source.clone()))
        .await
        .unwrap();
    assert!(harness
        .call(request(registration.clone(), format!("{source}-changed")))
        .await
        .err()
        .expect("stale project root must refuse")
        .contains("changed after review"));
    assert!(harness
        .call(request(
            "00000000-0000-4000-8000-000000000001".into(),
            source.clone()
        ))
        .await
        .err()
        .expect("absent WSL registration must refuse")
        .contains("registration"));
    let relative = |expected: String, snapshot: String| Request::WslCompose {
        reviewed_daemon: None,
        mapping_review: None,
        env_files: vec!["first 例.env".into(), "last 例.env".into()],
        distribution: name.into(),
        connection: Some(crate::wsl_integration::ConnectionReview {
            registration: None,
            source: expected,
            signature: snapshot,
        }),
        file: "base 例.json".into(),
        overrides: vec!["override 例.json".into()],
        directory: format!("migration-review-{stamp}"),
        project: project.clone(),
        profiles: vec![],
        services: vec![],
        action: ComposeAction::Validate,
    };
    harness
        .pass(relative(source.clone(), signature.clone()))
        .await
        .unwrap();
    let mut reversed = relative(source.clone(), signature.clone());
    if let Request::WslCompose { env_files, .. } = &mut reversed {
        env_files.reverse();
    }
    let (outcome, _) = harness.call(reversed).await.unwrap();
    assert!(
        !outcome.success,
        "Reversed env files should select the invalid port"
    );
    let mut missing = relative(source.clone(), signature.clone());
    if let Request::WslCompose { env_files, .. } = &mut missing {
        *env_files = vec!["absent.env".into()];
    }
    let (outcome, _) = harness.call(missing).await.unwrap();
    assert!(!outcome.success, "Missing explicit env file must refuse");
    assert!(harness
        .call(relative(format!("{source}-old"), signature.clone()))
        .await
        .err()
        .expect("stale relative root must refuse")
        .contains("changed after review"));
    assert!(harness
        .call(relative(source.clone(), "1:1".into()))
        .await
        .err()
        .expect("same-path replacement must refuse")
        .contains("changed after review"));
    assert!(harness
        .call(Request::WslBuild {
            distribution: name.into(),
            connection: Some(crate::wsl_integration::ConnectionReview {
                registration: None,
                source: source.clone(),
                signature: "1:1".into()
            }),
            directory: ".".into(),
            tag: format!("orbwin-root-refusal:{stamp}")
        })
        .await
        .err()
        .expect("stale build review must refuse")
        .contains("changed after review"));
    let payload = serde_json::json!({"kind":"compose", "connection":{"registration":registration,"source":source,"signature":signature}, "file":format!("{fixture}/base 例.json"), "overrides":[format!("{fixture}/override 例.json")], "directory":fixture,"project":project,"profiles":[],"services":[],"action":"validate"});
    let (command, wire) = crate::wsl_integration::prepare_workflow(name, payload)
        .await
        .unwrap();
    let args: Vec<_> = command
        .as_std()
        .get_args()
        .map(|arg| arg.to_string_lossy().to_string())
        .collect();
    assert_eq!(args[0], "--distribution-id");
    assert_eq!(args[1], format!("{{{registration}}}"));
    assert!(!args.iter().any(|arg| arg == "root"));
    let wire: serde_json::Value = serde_json::from_slice(&wire).unwrap();
    assert!(wire.get("connection").is_none());
    assert_eq!(wire["source"], source);
    let after = serde_json::to_value(
        crate::wsl_integration::wsl_migration_project(name.into(), registration.clone())
            .await
            .unwrap(),
    )
    .unwrap();
    assert_eq!(observed["export"], after["export"]);
    let target = engine::target().unwrap();
    let members = engine::command(
        &target,
        &[
            "ps",
            "-a",
            "--filter",
            &format!("label=com.docker.compose.project={project}"),
            "--format",
            "{{.ID}}",
        ],
    )
    .await
    .unwrap();
    assert!(members.is_empty());
    std::fs::write(root.join("p2-compose-env-wsl-native-20261025.json"), serde_json::to_vec_pretty(&serde_json::json!({"schema":1,"success":true,"registration":registration,"distribution":name,"source":source,"signature":signature,"fixture":fixture,"project":project,"uid":uid,"orderedEnvFilesValidated":true,"reversedEnvFilesRefused":true,"missingEnvFileRefused":true,"connectionPreserved":true,"targetMembersCreated":false,"staleRootRefused":true,"absentRegistrationRefused":true,"relativeInputsValidated":true,"relativeRootDriftRefused":true,"samePathIdentityChangeRefused":true,"staleBuildRefused":true,"helperUsesRegistration":true,"reviewRemovedFromWire":true,"records":harness.evidence})).unwrap()).unwrap();
}

#[tokio::test]
#[ignore = "headless owned-engine WSL original-project Compose/build proof; never terminates the user distro"]
async fn owned_wsl_original_paths() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let distribution = "Ubuntu";
    let binary = crate::wsl::wsl_binary();
    let previous = serde_json::to_value(
        crate::wsl_integration::wsl_project(distribution.into(), "inspect".into(), String::new())
            .await
            .unwrap(),
    )
    .unwrap();
    assert!(
        previous["export"].is_null(),
        "Preserve the currently selected WSL project; this proof needs an unused selection"
    );
    let uid = crate::process::read_command(&binary, &["-d", distribution, "--exec", "id", "-u"])
        .await
        .unwrap();
    assert_ne!(uid, "0", "Proof requires the ordinary default WSL user");
    let mut keeper = std::process::Command::new(&binary);
    keeper
        .args(["-d", distribution, "--exec", "cat"])
        .stdin(std::process::Stdio::piped())
        .stdout(std::process::Stdio::null())
        .stderr(std::process::Stdio::null());
    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        keeper.creation_flags(0x08000000);
    }
    let mut keeper = keeper.spawn().unwrap();
    let source=crate::process::read_command(&binary,&["-d",distribution,"--exec","sh","-c",r#"set -eu; root=$(mktemp -d "$HOME/.orbwin-p2.XXXXXX"); mkdir "$root/project ü"; printf '%s' "$root/project ü""#]).await.unwrap();
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_nanos();
    let project = format!("orbwin-wsl-p2-{stamp}");
    let tag = format!("orbwin-wsl-build:{stamp}");
    let root = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("..");
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: root.join(format!(
            "artifacts/qualification/wsl-original-paths-{stamp}.json"
        )),
    };
    let image =
        "alpine:3.21@sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30";
    let base=serde_json::json!({"services":{"app":{"image":image,"command":["sh","-c","cat /project/marker; cat /read-only; printf engine-visible > /project/from-engine; sleep 300"],"volumes":[{"type":"bind","source":"./data","target":"/project"},{"type":"bind","source":format!("{source}/readonly ü.txt"),"target":"/read-only","read_only":true}],"healthcheck":{"test":["CMD","test","-f","/project/from-engine"],"interval":"1s","timeout":"1s","retries":10}},"worker":{"image":image,"profiles":["worker"],"command":["sleep","300"],"volumes":["persistent:/persistent"]}},"volumes":{"persistent":{}}}).to_string();
    let overrides =
        serde_json::json!({"services":{"app":{"environment":{"SOURCE_HINT":source}}}}).to_string();
    crate::process::read_command(&binary,&["-d",distribution,"--exec","sh","-c",r#"set -eu; mkdir "$1/data"; printf 'source-visible\n' > "$1/data/marker"; printf 'read-only-visible\n' > "$1/readonly ü.txt"; chmod 640 "$1/readonly ü.txt"; printf '%s' "$2" > "$1/compose.json"; printf '%s' "$3" > "$1/override.json"; printf 'FROM %s\nRUN printf build-proof > /built\n' "$4" > "$1/Dockerfile""#,"orbwin",&source,&base,&overrides,image]).await.unwrap();
    crate::wsl_integration::wsl_project(distribution.into(), "connect".into(), source.clone())
        .await
        .unwrap();
    let request = |action, services: Vec<&str>| Request::WslCompose {
        reviewed_daemon: None,
        mapping_review: None,
        env_files: vec![],
        connection: None,
        distribution: distribution.into(),
        file: format!("{source}/compose.json"),
        overrides: vec!["override.json".into()],
        project: project.clone(),
        directory: String::new(),
        profiles: vec!["worker".into()],
        services: services.into_iter().map(String::from).collect(),
        action,
    };
    let target = engine::target().unwrap();
    let result: Result<(), String> = async {
        harness
            .pass(request(ComposeAction::Validate, vec![]))
            .await?;
        harness
            .pass(request(ComposeAction::Up, vec!["app"]))
            .await?;
        let members = engine::command(
            &target,
            &[
                "ps",
                "-a",
                "--no-trunc",
                "--quiet",
                "--filter",
                &format!("label=com.docker.compose.project={project}"),
            ],
        )
        .await?;
        if members.lines().count() != 1 {
            return Err("Selected WSL service scope started unexpected members".into());
        }
        let inspected = engine::command(&target, &["inspect", members.trim()]).await?;
        let inspected: serde_json::Value =
            serde_json::from_str(&inspected).map_err(|error| error.to_string())?;
        let mounts = inspected[0]["Mounts"]
            .as_array()
            .ok_or("No WSL bind metadata")?;
        if !mounts
            .iter()
            .any(|mount| mount["Destination"] == "/read-only" && mount["RW"] == false)
        {
            return Err("Original absolute WSL file bind lost read-only scope".into());
        }
        if !inspected[0]["Config"]["Env"]
            .as_array()
            .ok_or("No WSL app environment")?
            .iter()
            .any(|value| value.as_str() == Some(&format!("SOURCE_HINT={source}")))
        {
            return Err("Application environment was rewritten during path adaptation".into());
        }
        let visible = crate::process::read_command(
            &binary,
            &[
                "-d",
                distribution,
                "--exec",
                "cat",
                &format!("{source}/data/from-engine"),
            ],
        )
        .await?;
        if visible != "engine-visible" {
            return Err("Engine writes were not visible in the original project".into());
        }
        let logs = harness
            .pass(request(ComposeAction::Logs, vec!["app"]))
            .await?;
        if !logs.contains("source-visible") || !logs.contains("read-only-visible") {
            return Err("Mapped WSL sources were not visible to the service".into());
        }
        harness
            .pass(request(ComposeAction::Up, vec!["worker"]))
            .await?;
        harness
            .pass(Request::WslBuild {
                distribution: distribution.into(),
                connection: None,
                directory: ".".into(),
                tag: tag.clone(),
            })
            .await?;
        harness.pass(request(ComposeAction::Down, vec![])).await?;
        let data = engine::command(
            &target,
            &["volume", "inspect", &format!("{project}_persistent")],
        )
        .await?;
        let data: serde_json::Value =
            serde_json::from_str(&data).map_err(|error| error.to_string())?;
        if data[0]["Labels"]["dev.orbwin.wsl.project-source"] != source {
            return Err("WSL volume ownership/preservation differed".into());
        }
        crate::wsl_integration::wsl_project(
            distribution.into(),
            "disconnect".into(),
            String::new(),
        )
        .await?;
        if harness
            .call(request(ComposeAction::Validate, vec![]))
            .await
            .is_ok()
        {
            return Err("Disconnected WSL workflow reached dispatch".into());
        }
        Ok(())
    }
    .await;
    // No distro termination, engine restart or global cleanup. The selected
    // fixture is retained on an unexpected failure for explicit inspection.
    if result.is_ok() {
        let _ = harness
            .pass(Request::Remove {
                resource: Resource::Volume,
                target: format!("{project}_persistent"),
            })
            .await;
        let _ = engine::command(&target, &["image", "rm", &tag]).await;
        crate::process::read_command(&binary,&["-d",distribution,"--exec","sh","-c",r#"set -eu; root=$(dirname -- "$1"); case "$root" in /home/*/.orbwin-p2.??????) ;; *) exit 70;; esac; test ! -L "$root"; rm -r -- "$root""#,"orbwin",&source]).await.unwrap();
    }
    drop(keeper.stdin.take());
    let _ = keeper.wait();
    std::fs::write(&harness.evidence,serde_json::to_vec_pretty(&serde_json::json!({"success":result.is_ok(),"failure":result.as_ref().err(),"source":source,"uid":uid,"records":harness.records})).unwrap()).unwrap();
    result.unwrap();
}

#[tokio::test]
#[ignore = "owned portable-volume export/restore preservation proof"]
async fn owned_portable_volume() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_nanos();
    let name = format!("orbwin-portable-{stamp}");
    let restored = format!("{name}-restored");
    let invalid = format!("{name}-invalid");
    let pending = format!("{name}-pending");
    let root = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("..");
    let attributes_binary = root.join("artifacts/qualification/volume-attributes");
    let compiler = std::process::Command::new(root.join(".tools/go1.27.3/go/bin/go.exe"))
        .args(["build", "-trimpath", "-o"])
        .arg(&attributes_binary)
        .arg(root.join("tests/workloads/volume-attributes/main.go"))
        .env("GOOS", "linux")
        .env("GOARCH", "amd64")
        .env("CGO_ENABLED", "0")
        .output()
        .unwrap();
    assert!(
        compiler.status.success(),
        "{}",
        String::from_utf8_lossy(&compiler.stderr)
    );
    let package = root.join(format!("artifacts/qualification/portable ü {stamp}.orbvol"));
    let package_path = package.to_str().unwrap().to_string();
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: root.join(format!(
            "artifacts/qualification/portable-volume-{stamp}.json"
        )),
    };
    let target = engine::target().unwrap();
    engine::check(&target).await.unwrap();
    harness
        .pass(Request::CreateVolume { name: name.clone() })
        .await
        .unwrap();
    let container=harness.pass(Request::Run{image:"alpine:3.21@sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30".into(),name:name.clone(),command:vec!["sleep".into(),"900".into()],options:RunOptions{volumes:vec![format!("{name}:/data")],..Default::default()}}).await.unwrap().trim().to_string();
    let result:Result<(),String>=async {
        harness.pass(Request::Exec{target:container.clone(),command:vec!["sh".into(),"-c".into(),"mkdir /data/project; printf 'retained text ü\\n' > /data/project/例.txt; dd if=/dev/zero of=/data/binary bs=1024 count=300; ln /data/project/例.txt /data/hard; ln -s project/例.txt /data/linked; chown 1234:2345 /data/project/例.txt; chmod 640 /data/project/例.txt; chmod 750 /data/project".into()]}).await?;
        let active=harness.call(Request::ExportVolume{volume:name.clone(),destination:package_path.clone()}).await?;
        if active.0.success||package.exists(){return Err("Export accepted active writable user".into());}
        harness.pass(Request::Container{target:container.clone(),action:ContainerAction::Pause}).await?;
        if harness.call(Request::ExportVolume{volume:name.clone(),destination:package_path.clone()}).await?.0.success{return Err("Export accepted a paused writer".into());}
        harness.pass(Request::Container{target:container.clone(),action:ContainerAction::Unpause}).await?;
        harness.pass(Request::Container{target:container.clone(),action:ContainerAction::Stop}).await?;
        engine::command(&target,&["cp",attributes_binary.to_str().unwrap(),&format!("{container}:/tmp/volume-attributes")]).await?;
        harness.pass(Request::Container{target:container.clone(),action:ContainerAction::Start}).await?;
        harness.pass(Request::Exec{target:container.clone(),command:vec!["/tmp/volume-attributes".into(),"add".into()]}).await?;
        harness.pass(Request::Container{target:container.clone(),action:ContainerAction::Stop}).await?;
        let attributed=harness.call(Request::ExportVolume{volume:name.clone(),destination:package_path.clone()}).await?;
        if attributed.0.success||package.exists(){return Err("Export silently dropped an attribute omitted by Moby archives".into());}
        harness.pass(Request::Container{target:container.clone(),action:ContainerAction::Start}).await?;
        harness.pass(Request::Exec{target:container.clone(),command:vec!["/tmp/volume-attributes".into(),"remove".into()]}).await?;
        harness.pass(Request::Container{target:container.clone(),action:ContainerAction::Stop}).await?;
        harness.pass(Request::ExportVolume{volume:name.clone(),destination:package_path.clone()}).await?;
        if harness.call(Request::ExportVolume{volume:name.clone(),destination:package_path.clone()}).await.is_ok(){return Err("Existing export destination reached dispatch".into());}
        let restored_result=harness.pass(Request::RestoreVolume{source:package_path.clone(),name:restored.clone()}).await?;
        let receipt:serde_json::Value=serde_json::from_str(restored_result.lines().find(|line|line.starts_with('{')).ok_or("Restore returned no receipt")?).map_err(|error|error.to_string())?;
        if receipt["complete"]!=true||receipt["volume"]!=restored{return Err("Restore did not report complete owned destination".into());}
        let completed_discard=harness.call(Request::DiscardVolumeTransfer{transaction:receipt["transaction"].as_str().ok_or("No completed transaction identity")?.into()}).await?;
        if completed_discard.0.success{return Err("Recovery removed a completed destination".into());}
        let (cancelled,_)=harness.call_until(Request::RestoreVolume{source:package_path.clone(),name:pending.clone()},Some("Importing into isolated pending volume:")).await?;
        if !cancelled.interrupted||cancelled.success{return Err("Restore cancellation was not reported truthfully".into());}
        let recovery=harness.pass(Request::VolumeTransfers).await?;
        let recovery:serde_json::Value=serde_json::from_str(&recovery).map_err(|error|error.to_string())?;
        let recovery=recovery.as_array().ok_or("No recovery records")?.iter().find(|record|record["volume"]==pending).ok_or("Interrupted restore was not recorded")?;
        if recovery["active"]!=false||recovery["owned"]!=true||recovery["exists"]!=true||recovery["phase"]=="complete"{return Err(format!("Interrupted restore has unsafe recovery state: {recovery}"));}
        let transaction=recovery["transaction"].as_str().ok_or("No recovery transaction")?.to_string();
        let attached=harness.pass(Request::Run{image:"alpine:3.21@sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30".into(),name:format!("{pending}-reader"),command:vec!["sleep".into(),"60".into()],options:RunOptions{volumes:vec![format!("{pending}:/data:ro")],..Default::default()}}).await?.trim().to_string();
        let refused_attached=harness.call(Request::DiscardVolumeTransfer{transaction:transaction.clone()}).await?;
        harness.pass(Request::Container{target:attached.clone(),action:ContainerAction::Stop}).await?;
        let refused_stopped=harness.call(Request::DiscardVolumeTransfer{transaction:transaction.clone()}).await?;
        harness.pass(Request::Remove{resource:Resource::Container,target:attached}).await?;
        if refused_attached.0.success||refused_stopped.0.success{return Err("Recovery deleted a referenced pending volume".into());}
        harness.pass(Request::DiscardVolumeTransfer{transaction:transaction.clone()}).await?;
        if engine::command(&target,&["volume","inspect",&pending]).await.is_ok(){return Err("Discard retained its pending destination".into());}
        if harness.call(Request::DiscardVolumeTransfer{transaction}).await?.0.success{return Err("Discard repeated a completed deletion".into());}
        let collision=harness.call(Request::RestoreVolume{source:package_path.clone(),name:name.clone()}).await?;
        if collision.0.success{return Err("Restore overwrote the existing source volume".into());}
        let bytes=std::fs::read(&package).map_err(|error|error.to_string())?;
        let mut corrupt=bytes.clone();*corrupt.last_mut().ok_or("Empty package")?^=1;
        let bad=package.with_extension("corrupt");std::fs::write(&bad,corrupt).map_err(|error|error.to_string())?;
        let refused=harness.call(Request::RestoreVolume{source:bad.to_str().unwrap().into(),name:invalid.clone()}).await?;
        std::fs::remove_file(&bad).map_err(|error|error.to_string())?;
        if refused.0.success || engine::command(&target,&["volume","inspect",&invalid]).await.is_ok(){return Err("Invalid package created a destination volume".into());}
        harness.pass(Request::Container{target:container.clone(),action:ContainerAction::Start}).await?;
        let preserved=harness.pass(Request::Exec{target:container.clone(),command:vec!["sh".into(),"-c".into(),"cat /data/project/例.txt; stat -c '%u:%g:%a' /data/project/例.txt; test /data/hard -ef /data/project/例.txt; test \"$(readlink /data/linked)\" = project/例.txt".into()]}).await?;
        if !preserved.contains("retained text ü")||!preserved.contains("1234:2345:640"){return Err("Source content/metadata changed after export/restore".into());}
        Ok(())
    }.await;
    let _ = harness
        .call(Request::Container {
            target: container.clone(),
            action: ContainerAction::Stop,
        })
        .await;
    let removed = harness
        .pass(Request::Remove {
            resource: Resource::Container,
            target: container,
        })
        .await;
    if removed.is_ok() {
        let _ = harness
            .pass(Request::Remove {
                resource: Resource::Volume,
                target: name,
            })
            .await;
    }
    // Destination removal remains exact and requires its per-transfer ownership label.
    for destination in [&restored, &pending] {
        if let Ok(output) = engine::command(&target, &["volume", "inspect", destination]).await {
            let volumes: serde_json::Value = serde_json::from_str(&output).unwrap();
            if volumes[0]["Labels"]["dev.orbwin.transfer"]
                .as_str()
                .is_some_and(|value| value.len() == 32)
            {
                let _ = harness
                    .pass(Request::Remove {
                        resource: Resource::Volume,
                        target: destination.clone(),
                    })
                    .await;
            }
        }
    }
    if package.exists() {
        std::fs::remove_file(package).unwrap();
    }
    std::fs::write(&harness.evidence,serde_json::to_vec_pretty(&serde_json::json!({"success":result.is_ok(),"failure":result.as_ref().err(),"records":harness.records})).unwrap()).unwrap();
    result.unwrap();
    removed.unwrap();
}
fn id() -> String {
    format!(
        "00000000-0000-4000-8000-{:012x}",
        SEQUENCE.fetch_add(1, Ordering::Relaxed)
    )
}

#[tokio::test]
#[ignore = "owned-engine full-copy volume clone and cancellation recovery proof"]
async fn owned_volume_clone() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_nanos();
    let source = format!("orbwin-clone-{stamp}");
    let destination = format!("{source}-copy");
    let pending = format!("{source}-pending");
    let root = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("..");
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: root.join(format!("artifacts/qualification/volume-clone-{stamp}.json")),
    };
    let target = engine::target().unwrap();
    engine::check(&target).await.unwrap();
    harness
        .pass(Request::CreateVolume {
            name: source.clone(),
        })
        .await
        .unwrap();
    let image =
        "alpine:3.21@sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30";
    let container = harness
        .pass(Request::Run {
            image: image.into(),
            name: source.clone(),
            command: vec!["sleep".into(), "900".into()],
            options: RunOptions {
                volumes: vec![format!("{source}:/data")],
                ..Default::default()
            },
        })
        .await
        .unwrap()
        .trim()
        .to_string();
    let mut reader = None;
    let result:Result<(),String>=Box::pin(async {
        harness.pass(Request::Exec{target:container.clone(),command:vec!["sh".into(),"-c".into(),"mkdir /data/project; printf 'clone retained ü\\n' > /data/project/例.txt; dd if=/dev/zero of=/data/binary bs=1024 count=4096; ln /data/project/例.txt /data/hard; ln -s project/例.txt /data/linked; chown 1234:2345 /data/project/例.txt; chmod 640 /data/project/例.txt".into()]}).await?;
        if harness.call(Request::CloneVolume{volume:source.clone(),name:destination.clone()}).await?.0.success{return Err("Clone accepted an active writable source".into());}
        if engine::command(&target,&["volume","inspect",&destination]).await.is_ok(){return Err("Active-source refusal created a destination".into());}
        harness.pass(Request::Container{target:container.clone(),action:ContainerAction::Pause}).await?;
        if harness.call(Request::CloneVolume{volume:source.clone(),name:destination.clone()}).await?.0.success{return Err("Clone accepted a paused writer".into());}
        harness.pass(Request::Container{target:container.clone(),action:ContainerAction::Unpause}).await?;
        harness.pass(Request::Container{target:container.clone(),action:ContainerAction::Stop}).await?;
        let cloned=harness.pass(Request::CloneVolume{volume:source.clone(),name:destination.clone()}).await?;
        let cloned:serde_json::Value=serde_json::from_str(cloned.lines().find(|line|line.starts_with('{')).ok_or("Clone returned no result JSON")?).map_err(|e|e.to_string())?;
        if cloned["complete"]!=true || cloned["volume"]!=destination || cloned["copyOnWrite"]!=false || cloned["snapshotRetained"]!=false{return Err(format!("Clone completion/staging/copy semantics differ: {cloned}"));}
        if harness.call(Request::CloneVolume{volume:source.clone(),name:destination.clone()}).await?.0.success{return Err("Clone overwrote an existing destination".into());}
        let check=harness.pass(Request::Run{image:image.into(),name:format!("{source}-reader"),command:vec!["sleep".into(),"900".into()],options:RunOptions{volumes:vec![format!("{destination}:/data:ro")],..Default::default()}}).await?.trim().to_string();reader=Some(check.clone());
        let verified=harness.pass(Request::Exec{target:check,command:vec!["sh".into(),"-c".into(),"cat /data/project/例.txt; stat -c '%u:%g:%a' /data/project/例.txt; test /data/hard -ef /data/project/例.txt; test \"$(readlink /data/linked)\" = project/例.txt; test \"$(wc -c < /data/binary)\" = 4194304".into()]}).await?;
        if !verified.contains("clone retained ü")||!verified.contains("1234:2345:640"){return Err("Cloned bytes/metadata were not preserved".into());}
        let (cancelled,_)=harness.call_until(Request::CloneVolume{volume:source.clone(),name:pending.clone()},Some("Importing into isolated pending volume:")).await?;
        if !cancelled.interrupted || cancelled.success{return Err("Clone cancellation was not reported truthfully".into());}
        let jobs=harness.pass(Request::VolumeClones).await?;let jobs:serde_json::Value=serde_json::from_str(&jobs).map_err(|e|e.to_string())?;
        let job=jobs["records"].as_array().ok_or("No clone recovery rows")?.iter().find(|value|value["volume"]==pending).ok_or("Canceled clone receipt missing")?;
        if job["active"]!=false || job["snapshotPresent"]!=true || job["phase"]!="restoring"{return Err(format!("Canceled clone recovery differs: {job}"));}
        let token=job["transaction"].as_str().ok_or("Clone token missing")?.to_string();
        let snapshot=job["snapshotPath"].as_str().ok_or("Saved snapshot path missing")?;
        if !std::path::Path::new(snapshot).is_file(){return Err("Saved clone snapshot absent".into());}
        harness.pass(Request::DiscardCloneStaging{transaction:token}).await?;
        if std::path::Path::new(snapshot).exists(){return Err("Owned staging cleanup retained the snapshot".into());}
        if engine::command(&target,&["volume","inspect",&pending]).await.is_err(){return Err("Staging cleanup removed a pending destination".into());}
        let transfers=harness.pass(Request::VolumeTransfers).await?;let transfers:serde_json::Value=serde_json::from_str(&transfers).map_err(|e|e.to_string())?;
        let transfer=transfers.as_array().ok_or("No restore recovery rows")?.iter().find(|value|value["volume"]==pending).ok_or("Pending clone destination has no restore receipt")?;
        let transfer=transfer["transaction"].as_str().ok_or("Pending restore transaction missing")?.to_string();
        harness.pass(Request::DiscardVolumeTransfer{transaction:transfer}).await?;
        harness.pass(Request::Container{target:container.clone(),action:ContainerAction::Start}).await?;
        let preserved=harness.pass(Request::Exec{target:container.clone(),command:vec!["cat".into(),"/data/project/例.txt".into()]}).await?;
        if !preserved.contains("clone retained ü"){return Err("Clone changed source content".into());}Ok(())
    }).await;
    for owned in reader.iter().chain(std::iter::once(&container)) {
        let _ = harness
            .call(Request::Container {
                target: owned.clone(),
                action: ContainerAction::Stop,
            })
            .await;
        let _ = harness
            .call(Request::Remove {
                resource: Resource::Container,
                target: owned.clone(),
            })
            .await;
    }
    let mut cleanup = vec![];
    for volume in [&source, &destination, &pending] {
        if let Ok(value) = engine::command(&target, &["volume", "inspect", volume]).await {
            let value: serde_json::Value = serde_json::from_str(&value).unwrap();
            if volume != &source
                && !value[0]["Labels"]["dev.orbwin.transfer"]
                    .as_str()
                    .is_some_and(|value| value.len() == 32)
            {
                cleanup.push(format!("Ownership differs for {volume}"));
                continue;
            }
            if let Err(error) = harness
                .pass(Request::Remove {
                    resource: Resource::Volume,
                    target: volume.clone(),
                })
                .await
            {
                cleanup.push(error);
            }
        }
    }
    std::fs::write(&harness.evidence,serde_json::to_vec_pretty(&serde_json::json!({"success":result.is_ok()&&cleanup.is_empty(),"failure":result.as_ref().err(),"cleanupErrors":cleanup,"records":harness.records})).unwrap()).unwrap();
    assert!(
        result.is_ok() && cleanup.is_empty(),
        "Result: {result:?}; cleanup: {cleanup:?}; evidence: {}",
        harness.evidence.display()
    );
}

#[tokio::test]
#[ignore = "build and Compose workflow proof on the recorded owned engine"]
async fn owned_build_compose_workflow() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_millis();
    let project = format!("orbwin-compose-{stamp}");
    let tag = format!("orbwin-build:{stamp}");
    let alias = format!("orbwin-build-alias:{stamp}");
    let root = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("..");
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: root.join(format!(
            "artifacts/qualification/build-compose-{stamp}.json"
        )),
    };
    let file = root
        .join("tests/workloads/compose.yaml")
        .to_str()
        .unwrap()
        .to_string();
    let mut built = false;
    let mut tagged = false;
    let result: Result<(), String> = async {
        harness
            .pass(Request::Build {
                directory: root.join("tests/workloads/build").to_str().unwrap().into(),
                tag: tag.clone(),
            })
            .await?;
        built = true;
        let target = engine::target()?;
        let image =
            engine::command(&target, &["image", "inspect", "--format={{.Id}}", &tag]).await?;
        harness
            .pass(Request::Tag {
                target: image.trim().into(),
                tag: alias.clone(),
            })
            .await?;
        tagged = true;
        harness
            .pass(Request::Inspect {
                resource: Resource::Image,
                target: image.trim().into(),
            })
            .await?;
        harness
            .pass(Request::Compose {
                env_files: vec![],
                services: vec![],
                directory: String::new(),
                overrides: vec![],
                profiles: vec![],
                file: file.clone(),
                project: project.clone(),
                action: ComposeAction::Up,
            })
            .await?;
        let filter = format!("label=com.docker.compose.project={project}");
        let redis = engine::command(
            &target,
            &[
                "ps",
                "--no-trunc",
                "--quiet",
                "--filter",
                &filter,
                "--filter",
                "label=com.docker.compose.service=redis",
            ],
        )
        .await?;
        let redis = redis.trim().to_string();
        super::target(Resource::Container, &redis)?;
        harness
            .pass(Request::Exec {
                target: redis.clone(),
                command: vec![
                    "redis-cli".into(),
                    "set".into(),
                    "orbwin-adapter-proof".into(),
                    project.clone(),
                ],
            })
            .await?;
        harness
            .pass(Request::Compose {
                env_files: vec![],
                services: vec![],
                directory: String::new(),
                overrides: vec![],
                profiles: vec![],
                file: file.clone(),
                project: project.clone(),
                action: ComposeAction::Restart,
            })
            .await?;
        harness
            .pass(Request::Compose {
                env_files: vec![],
                services: vec![],
                directory: String::new(),
                overrides: vec![],
                profiles: vec![],
                file: file.clone(),
                project: project.clone(),
                action: ComposeAction::Up,
            })
            .await?;
        let value = harness
            .pass(Request::Exec {
                target: redis.clone(),
                command: vec![
                    "redis-cli".into(),
                    "get".into(),
                    "orbwin-adapter-proof".into(),
                ],
            })
            .await?;
        if value.trim() != project {
            return Err("Compose volume data did not survive restart".into());
        }
        harness
            .pass(Request::Compose {
                env_files: vec![],
                services: vec![],
                directory: String::new(),
                overrides: vec![],
                profiles: vec![],
                file: file.clone(),
                project: project.clone(),
                action: ComposeAction::Logs,
            })
            .await?;
        harness
            .pass(Request::Compose {
                env_files: vec![],
                services: vec![],
                directory: String::new(),
                overrides: vec![],
                profiles: vec![],
                file: file.clone(),
                project: project.clone(),
                action: ComposeAction::Stop,
            })
            .await?;
        harness
            .pass(Request::Compose {
                env_files: vec![],
                services: vec![],
                directory: String::new(),
                overrides: vec![],
                profiles: vec![],
                file: file.clone(),
                project: project.clone(),
                action: ComposeAction::Start,
            })
            .await?;
        Ok(())
    }
    .await;
    let mut cleanup_errors = vec![];
    if let Err(e) = harness
        .pass(Request::Compose {
            env_files: vec![],
            services: vec![],
            directory: String::new(),
            overrides: vec![],
            profiles: vec![],
            file,
            project: project.clone(),
            action: ComposeAction::Down,
        })
        .await
    {
        cleanup_errors.push(e);
    }
    for suffix in ["redis-data", "postgres-data"] {
        if let Err(e) = harness
            .pass(Request::Remove {
                resource: Resource::Volume,
                target: format!("{project}_{suffix}"),
            })
            .await
        {
            cleanup_errors.push(e);
        }
    }
    if tagged {
        if let Err(e) = harness.pass(Request::Untag { reference: alias }).await {
            cleanup_errors.push(e);
        }
    }
    if built {
        if let Err(e) = harness.pass(Request::Untag { reference: tag }).await {
            cleanup_errors.push(e);
        }
    }
    assert!(
        result.is_ok() && cleanup_errors.is_empty(),
        "result: {result:?}; cleanup: {cleanup_errors:?}; evidence: {}",
        harness.evidence.display()
    );
}
struct Harness {
    state: Operations,
    records: Vec<serde_json::Value>,
    evidence: std::path::PathBuf,
}

#[tokio::test]
#[ignore = "owned-engine reviewed image import, tag conflict consent and previous-content preservation"]
async fn owned_image_import() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_nanos();
    let root = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("..");
    let directory = root.join(format!("artifacts/qualification/image-import-{stamp}"));
    std::fs::create_dir(&directory).unwrap();
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: directory.join("proof.json"),
    };
    let target = engine::target().unwrap();
    engine::check(&target).await.unwrap();
    let source =
        "alpine:3.21@sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30";
    let alternate = "alpine:3.23";
    let inspect_target = &target;
    let inspect = move |value: String| async move {
        let output = engine::command(inspect_target, &["image", "inspect", &value]).await?;
        serde_json::from_str::<serde_json::Value>(&output).map_err(|error| error.to_string())
    };
    let before = inspect(source.into()).await.unwrap();
    let other = inspect(alternate.into()).await.unwrap();
    let source_id = before[0]["Id"].as_str().unwrap().to_string();
    let other_id = other[0]["Id"].as_str().unwrap().to_string();
    assert_ne!(source_id, other_id);
    let tag = format!("orbwin-image-import:{stamp}");
    harness
        .pass(Request::Tag {
            target: source_id.clone(),
            tag: tag.clone(),
        })
        .await
        .unwrap();
    let archive = directory.join("review 例.tar");
    let mut owned = std::collections::BTreeMap::<String, String>::new();
    let mut receipts = vec![];
    let result: Result<(), String> = Box::pin(async {
        harness
            .pass(Request::ExportImage {
                image: source_id.clone(),
                reference: tag.clone(),
                destination: archive.to_str().unwrap().into(),
            })
            .await?;
        harness
            .pass(Request::Tag {
                target: other_id.clone(),
                tag: tag.clone(),
            })
            .await?;
        let reviewed = harness
            .pass(Request::ReviewImageImport {
                source: archive.to_str().unwrap().into(),
            })
            .await?;
        let reviewed: serde_json::Value =
            serde_json::from_str(&reviewed).map_err(|e| e.to_string())?;
        let config = reviewed["images"][0]["id"]
            .as_str()
            .ok_or("No reviewed config")?
            .to_string();
        let reference = reviewed["tags"][0]["reference"]
            .as_str()
            .ok_or("No reviewed original tag")?
            .to_string();
        if reviewed["tags"][0]["currentId"] != other_id {
            return Err("Review did not bind the existing conflict identity".into());
        }
        let request = |apply, replace, expected: String| Request::ImportImages {
            source: archive.to_str().unwrap().into(),
            sha256: reviewed["sha256"].as_str().unwrap().into(),
            tags: vec![ImageImportChoice {
                reference: reference.clone(),
                config_id: config.clone(),
                expected_id: expected,
                apply,
                replace,
            }],
        };
        if harness
            .call(request(true, false, other_id.clone()))
            .await?
            .0
            .success
        {
            return Err("Existing image tag was replaced without consent".into());
        }
        harness
            .pass(Request::Tag {
                target: source_id.clone(),
                tag: tag.clone(),
            })
            .await?;
        if harness
            .call(request(true, true, other_id.clone()))
            .await?
            .0
            .success
        {
            return Err("Stale image tag review was accepted".into());
        }
        harness
            .pass(Request::Tag {
                target: other_id.clone(),
                tag: tag.clone(),
            })
            .await?;
        for (apply, expected) in [
            (false, other_id.clone()),
            (true, other_id.clone()),
            (true, String::new()),
        ] {
            if expected.is_empty() {
                let current = inspect(tag.clone()).await?;
                owned.insert(reference.clone(), current[0]["Id"].as_str().unwrap().into());
                harness
                    .pass(Request::Untag {
                        reference: tag.clone(),
                    })
                    .await?;
                owned.remove(&reference);
            }
            let output = harness
                .pass(request(
                    apply,
                    apply && !expected.is_empty(),
                    expected.clone(),
                ))
                .await?;
            let accepted: serde_json::Value = serde_json::from_str(
                output
                    .lines()
                    .find(|line| line.starts_with('{'))
                    .ok_or("No import acknowledgement")?,
            )
            .map_err(|e| e.to_string())?;
            if accepted["complete"] != true {
                return Err("Image import was not verified complete".into());
            }
            for stage in accepted["images"]
                .as_array()
                .ok_or("No retained image references")?
            {
                let config = stage["configId"].as_str().unwrap();
                let id = accepted["verified"][config]
                    .as_str()
                    .ok_or("No verified staged identity")?;
                owned.insert(stage["reference"].as_str().unwrap().into(), id.into());
            }
            if let Some(previous) = accepted["previousReferences"].as_object() {
                for (original, backup) in previous {
                    owned.insert(
                        backup.as_str().unwrap().into(),
                        accepted["previous"][original].as_str().unwrap().into(),
                    );
                }
            }
            let current = inspect(tag.clone()).await?;
            if !apply && current[0]["Id"] != other_id {
                return Err("Skipped conflict was changed".into());
            }
            if apply {
                let id = accepted["verified"][&config].as_str().unwrap();
                if current[0]["Id"] != id {
                    return Err("Reviewed original tag does not point to verified image".into());
                }
                owned.insert(reference.clone(), id.into());
                if !expected.is_empty() {
                    let backup = accepted["previousReferences"][&reference]
                        .as_str()
                        .ok_or("Previous image has no retained reference")?;
                    if inspect(backup.into()).await?[0]["Id"] != other_id {
                        return Err("Previous image content was not preserved".into());
                    }
                }
            }
            let token = accepted["transaction"].as_str().unwrap().to_string();
            let detail = harness
                .pass(Request::ImageImportRecord {
                    transaction: token.clone(),
                })
                .await?;
            let detail: serde_json::Value =
                serde_json::from_str(&detail).map_err(|e| e.to_string())?;
            if detail["phase"] != "complete" || detail["active"] != false {
                return Err("Import receipt did not settle after SDK exit".into());
            }
            receipts.push(token);
        }
        let after = inspect(source.into()).await?;
        let other_after = inspect(alternate.into()).await?;
        if after[0]["Id"] != before[0]["Id"]
            || after[0]["RootFS"] != before[0]["RootFS"]
            || other_after[0]["Id"] != other[0]["Id"]
            || other_after[0]["RootFS"] != other[0]["RootFS"]
        {
            return Err("Source/previous base identity or layers changed".into());
        }
        Ok(())
    })
    .await;
    let mut cleanup_errors = vec![];
    // Only exact references acknowledged by these unique fixture transactions.
    // Conflicting identities remain for inspection rather than forced deletion.
    for (reference, id) in &owned {
        match inspect(reference.clone()).await {
            Ok(current) if current[0]["Id"] == *id => {
                if let Err(error) = harness
                    .pass(Request::Untag {
                        reference: reference.clone(),
                    })
                    .await
                {
                    cleanup_errors.push(error)
                }
            }
            _ => cleanup_errors.push(format!("Fixture reference owner unconfirmed: {reference}")),
        }
    }
    // If an early test failure preceded import acknowledgement, remove only the
    // original unique alias when it still points to one of the two known bases.
    if !owned.keys().any(|reference| reference.ends_with(&tag)) {
        if let Ok(current) = inspect(tag.clone()).await {
            if current[0]["Id"] == source_id || current[0]["Id"] == other_id {
                if let Err(error) = harness
                    .pass(Request::Untag {
                        reference: tag.clone(),
                    })
                    .await
                {
                    cleanup_errors.push(error)
                }
            }
        }
    }
    std::fs::write(&harness.evidence,serde_json::to_vec_pretty(&serde_json::json!({"success":result.is_ok()&&cleanup_errors.is_empty(),"failure":result.as_ref().err(),"cleanupErrors":cleanup_errors,"records":harness.records,"receipts":receipts,"cleanedAcknowledgedReferences":owned})).unwrap()).unwrap();
    assert!(
        result.is_ok() && cleanup_errors.is_empty(),
        "result: {result:?}; cleanup: {cleanup_errors:?}; evidence: {}",
        harness.evidence.display()
    );
}

#[tokio::test]
#[ignore = "owned-engine reviewed image export; no import, prune or source deletion"]
async fn owned_image_export() {
    use sha2::Digest;
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_nanos();
    let root = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("..");
    let directory = root.join(format!("artifacts/qualification/image-export-{stamp}"));
    std::fs::create_dir(&directory).unwrap();
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: directory.join("proof.json"),
    };
    let target = engine::target().unwrap();
    engine::check(&target).await.unwrap();
    let source =
        "alpine:3.21@sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30";
    let before: serde_json::Value = serde_json::from_str(
        &engine::command(&target, &["image", "inspect", source])
            .await
            .unwrap(),
    )
    .unwrap();
    let image = before[0]["Id"].as_str().unwrap().to_string();
    let tag = format!("orbwin-image-export:{stamp}");
    harness
        .pass(Request::Tag {
            target: image.clone(),
            tag: tag.clone(),
        })
        .await
        .unwrap();
    let archive = directory.join("export 例.tar");
    let cancelled_archive = directory.join("interrupted.tar");
    let result: Result<(), String> = Box::pin(async {
        let (outcome, output) = harness
            .call(Request::ExportImage {
                image: image.clone(),
                reference: tag.clone(),
                destination: archive.to_str().unwrap().into(),
            })
            .await?;
        if !outcome.success || outcome.mutation {
            return Err(format!("Image export was not a successful read: {output}"));
        }
        let accepted: serde_json::Value = serde_json::from_str(
            output
                .lines()
                .find(|line| line.starts_with('{'))
                .ok_or("No image export acknowledgement")?,
        )
        .map_err(|e| e.to_string())?;
        let bytes = std::fs::read(&archive).map_err(|e| e.to_string())?;
        if accepted["complete"] != true
            || accepted["image"] != image
            || accepted["bytes"] != bytes.len()
            || accepted["sha256"] != format!("{:x}", sha2::Sha256::digest(&bytes))
        {
            return Err("Image archive acknowledgement/checksum differs".into());
        }
        let archived_tags = accepted["images"]
            .as_array()
            .ok_or("Archive has no verified images")?
            .iter()
            .flat_map(|image| image["tags"].as_array().into_iter().flatten())
            .any(|value| value.as_str() == Some(&tag));
        if !archived_tags {
            return Err("Selected source tag was not preserved in the archive".into());
        }
        if harness
            .call(Request::ExportImage {
                image: image.clone(),
                reference: tag.clone(),
                destination: archive.to_str().unwrap().into(),
            })
            .await
            .is_ok()
        {
            return Err("Existing archive destination was accepted by native preflight".into());
        }
        if std::fs::read(&archive).map_err(|e| e.to_string())? != bytes {
            return Err("Existing image archive was altered".into());
        }
        let wrong = directory.join("wrong-target.tar");
        if harness
            .call(Request::ExportImage {
                image: format!("sha256:{}", "0".repeat(64)),
                reference: tag.clone(),
                destination: wrong.to_str().unwrap().into(),
            })
            .await?
            .0
            .success
            || wrong.exists()
        {
            return Err("Changed image identity was exported".into());
        }
        // A late cancellation may race completed publication; record the actual
        // outcome, never assume that killing the client rolled a file back.
        let _ = harness
            .call_until(
                Request::ExportImage {
                    image: image.clone(),
                    reference: tag.clone(),
                    destination: cancelled_archive.to_str().unwrap().into(),
                },
                Some("Saving image through exclusive partial file:"),
            )
            .await?;
        let after: serde_json::Value =
            serde_json::from_str(&engine::command(&target, &["image", "inspect", source]).await?)
                .map_err(|e| e.to_string())?;
        if after[0]["Id"] != before[0]["Id"] || after[0]["RootFS"] != before[0]["RootFS"] {
            return Err("Source image identity/layers changed during export".into());
        }
        Ok(())
    })
    .await;
    let mut cleanup_errors = vec![];
    // Remove only the unique alias if it still resolves to the exact fixture ID.
    match engine::command(&target, &["image", "inspect", &tag]).await {
        Ok(output) => match serde_json::from_str::<serde_json::Value>(&output) {
            Ok(inspected) if inspected[0]["Id"] == image => {
                if let Err(error) = harness
                    .pass(Request::Untag {
                        reference: tag.clone(),
                    })
                    .await
                {
                    cleanup_errors.push(error);
                }
            }
            _ => cleanup_errors
                .push("Unique export alias owner could not be confirmed; retained".into()),
        },
        Err(error) => cleanup_errors.push(error),
    }
    std::fs::write(&harness.evidence,serde_json::to_vec_pretty(&serde_json::json!({"success":result.is_ok()&&cleanup_errors.is_empty(),"failure":result.as_ref().err(),"cleanupErrors":cleanup_errors,"records":harness.records,"archiveRetained":archive,"cancellationDestination":cancelled_archive})).unwrap()).unwrap();
    assert!(
        result.is_ok() && cleanup_errors.is_empty(),
        "result: {result:?}; cleanup: {cleanup_errors:?}; evidence: {}",
        harness.evidence.display()
    );
}

#[tokio::test]
#[ignore = "explicit owned-engine Phase 2 Compose override/profile qualification"]
async fn owned_compose_profiles() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_nanos();
    let project = format!("orbwin-profiles-{stamp}");
    let root = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("..");
    let fixture = root.join("tests/workloads/compose-profiles");
    let request = |action, profiles: Vec<String>| Request::Compose {
        env_files: vec![],
        services: vec![],
        directory: fixture.to_str().unwrap().into(),
        file: fixture.join("compose.yaml").to_str().unwrap().into(),
        overrides: vec!["first override.yaml", "last override.yaml"]
            .into_iter()
            .map(|name| fixture.join(name).to_str().unwrap().into())
            .collect(),
        profiles,
        project: project.clone(),
        action,
    };
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: root.join(format!(
            "artifacts/qualification/compose-profiles-{stamp}.json"
        )),
    };
    let filter = format!("label=com.docker.compose.project={project}");
    let target = engine::target().unwrap();
    engine::check(&target).await.unwrap();
    let service = |name: &str| format!("label=com.docker.compose.service={name}");
    let result: Result<(), String> = async {
        let (validation, output) = harness
            .call(request(ComposeAction::Validate, vec![]))
            .await?;
        if !validation.success || validation.mutation || !output.trim().is_empty() {
            return Err(format!("Validation was not a quiet read: {output}"));
        }
        let containers = engine::command(&target, &["ps", "-aq", "--filter", &filter]).await?;
        if !containers.trim().is_empty() {
            return Err("Validation created workloads".into());
        }
        let (invalid, _) = harness
            .call(Request::Compose {
                env_files: vec![],
                services: vec![],
                directory: String::new(),
                file: fixture.join("invalid.yaml").to_str().unwrap().into(),
                overrides: vec![],
                profiles: vec![],
                project: project.clone(),
                action: ComposeAction::Validate,
            })
            .await?;
        if invalid.success || invalid.mutation {
            return Err("Invalid configuration was accepted".into());
        }
        harness.pass(request(ComposeAction::Up, vec![])).await?;
        let app = engine::command(
            &target,
            &[
                "ps",
                "--no-trunc",
                "-q",
                "--filter",
                &filter,
                "--filter",
                &service("app"),
            ],
        )
        .await?;
        let app = app.trim().to_string();
        super::target(Resource::Container, &app)?;
        for name in ["debug", "hidden"] {
            if !engine::command(
                &target,
                &["ps", "-aq", "--filter", &filter, "--filter", &service(name)],
            )
            .await?
            .trim()
            .is_empty()
            {
                return Err(format!("Unselected profile service {name} was created"));
            }
        }
        let marker = harness
            .pass(Request::Exec {
                target: app.clone(),
                command: vec!["printenv".into(), "MERGE_MARKER".into()],
            })
            .await?;
        if marker.trim() != "last" {
            return Err(format!("Merge order differed: {marker}"));
        }
        harness
            .pass(Request::Exec {
                target: app,
                command: vec![
                    "sh".into(),
                    "-c".into(),
                    "printf retained > /data/sentinel".into(),
                ],
            })
            .await?;
        for action in [
            ComposeAction::Up,
            ComposeAction::Restart,
            ComposeAction::Stop,
            ComposeAction::Start,
            ComposeAction::Down,
            ComposeAction::Up,
        ] {
            harness.pass(request(action, vec!["dev".into()])).await?;
        }
        let debug = engine::command(
            &target,
            &[
                "ps",
                "-q",
                "--filter",
                &filter,
                "--filter",
                &service("debug"),
            ],
        )
        .await?;
        if debug.trim().is_empty() {
            return Err("Selected profile service was not running".into());
        }
        let hidden = engine::command(
            &target,
            &[
                "ps",
                "-aq",
                "--filter",
                &filter,
                "--filter",
                &service("hidden"),
            ],
        )
        .await?;
        if !hidden.trim().is_empty() {
            return Err("Hidden profile activated".into());
        }
        let app = engine::command(
            &target,
            &[
                "ps",
                "--no-trunc",
                "-q",
                "--filter",
                &filter,
                "--filter",
                &service("app"),
            ],
        )
        .await?;
        let marker = harness
            .pass(Request::Exec {
                target: app.trim().into(),
                command: vec!["cat".into(), "/data/sentinel".into()],
            })
            .await?;
        if marker != "retained" {
            return Err("Down/up lost named-volume data".into());
        }
        Ok(())
    }
    .await;
    // Attempt exact-project cleanup even after a partially accepted up.
    let cleanup = harness
        .pass(request(
            ComposeAction::Down,
            vec!["dev".into(), "hidden".into()],
        ))
        .await;
    let volume = format!("{project}_data");
    let volume_cleanup = if cleanup.is_ok() {
        harness
            .pass(Request::Remove {
                resource: Resource::Volume,
                target: volume,
            })
            .await
            .map(|_| ())
    } else {
        Err("Retained volume after failed project cleanup".into())
    };
    assert!(
        result.is_ok() && cleanup.is_ok() && volume_cleanup.is_ok(),
        "result: {result:?}; cleanup: {cleanup:?}; volume: {volume_cleanup:?}; evidence: {}",
        harness.evidence.display()
    );
}

#[tokio::test]
#[ignore = "explicit owned-engine Compose service/failure/preservation journey"]
async fn owned_compose_journey() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_nanos();
    let project = format!("orbwin-journey-{stamp}");
    let root = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("..")
        .canonicalize()
        .unwrap();
    let fixture = root.join("tests/workloads/compose-journey");
    let evidence = root.join(format!(
        "artifacts/qualification/compose-journey-{stamp}.json"
    ));
    let conflict_file = root.join(format!(
        "artifacts/qualification/compose-conflict-{stamp}.yaml"
    ));
    let listener = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
    let port = listener.local_addr().unwrap().port();
    std::fs::write(
        &conflict_file,
        format!("services:\n  app:\n    ports: [\"127.0.0.1:{port}:80\"]\n"),
    )
    .unwrap();
    let request = |action: &str, services: Vec<&str>, extra: Option<&std::path::Path>| -> Request {
        serde_json::from_value(serde_json::json!({
            "kind":"compose", "file":fixture.join("compose.yaml"), "project":project,
            "directory":fixture, "overrides":extra.map(|path|vec![path]).unwrap_or_default(),
            "profiles":["workers"], "services":services, "action":action
        }))
        .unwrap()
    };
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence,
    };
    let target = engine::target().unwrap();
    engine::check(&target).await.unwrap();
    let filter = format!("label=com.docker.compose.project={project}");
    let app_filter = "label=com.docker.compose.service=app";
    let app_args = [
        "ps",
        "-aq",
        "--no-trunc",
        "--filter",
        &filter,
        "--filter",
        app_filter,
    ];
    let find_app = || engine::command(&target, &app_args);
    let result: Result<(), String> = async {
        harness.pass(request("validate", vec![], None)).await?;
        harness.pass(request("up", vec!["app"], None)).await?;
        let all = engine::command(&target, &["ps", "-aq", "--filter", &filter]).await?;
        if all.lines().count()!=1 { return Err("Selected app created other services".into()); }
        let app = find_app().await?.trim().to_string();
        harness.pass(Request::Exec {target: app.clone(), command: vec!["sh".into(),"-c".into(),"printf preserved > /data/sentinel".into()]}).await?;
        harness.pass(request("up", vec!["worker"], None)).await?;
        let logs = harness.pass(request("logs", vec!["app"], None)).await?;
        if !logs.contains("app-only-log") || logs.contains("worker-only-log") { return Err(format!("Service-filtered logs differed: {logs}")); }
        let logs = harness.pass(request("logs", vec![], None)).await?;
        if !logs.contains("app-only-log") || !logs.contains("worker-only-log") { return Err("Aggregate logs omitted a service".into()); }
        let (unknown, _) = harness.call(request("up", vec!["not-a-service"], None)).await?;
        if unknown.success || find_app().await?.trim()!=app { return Err("Unknown service changed app identity".into()); }
        let stale: Request = serde_json::from_value(serde_json::json!({"kind":"compose", "file":fixture.join("moved.yaml"), "project":project,"action":"up"})).unwrap();
        if harness.call(stale).await.is_ok() { return Err("Moved file reached dispatch".into()); }
        // Another explicitly targeted client recreates app; UI identity is not authoritative.
        let file = host_path(fixture.join("compose.yaml").to_str().unwrap(), false)?;
        engine::command(&target, &["compose","--project-name",&project,"--file",&file,"up","--detach","--force-recreate","app"]).await?;
        let replacement = find_app().await?.trim().to_string();
        if replacement==app { return Err("External recreation did not replace the container".into()); }
        let marker = harness.pass(Request::Exec {target:replacement,command:vec!["cat".into(),"/data/sentinel".into()]}).await?;
        if marker!="preserved" { return Err("External recreation lost volume marker".into()); }
        let (conflict, output) = harness.call(request("up", vec!["app"], Some(&conflict_file))).await?;
        if conflict.success { return Err(format!("Occupied host port was accepted: {output}")); }
        drop(listener);
        harness.pass(request("up", vec!["app"], None)).await?;
        let waiting = fixture.join("wait.yaml");
        let (cancelled, output) = harness.call_until(request("up", vec!["app"], Some(&waiting)), Some("Waiting")).await?;
        if !cancelled.interrupted || !cancelled.mutation || cancelled.success { return Err(format!("Readiness cancellation not uncertain: {output}")); }
        let app = find_app().await?.trim().to_string();
        if app.is_empty() { return Err("Client cancellation removed app".into()); }
        harness.pass(Request::Exec {target:app.clone(),command:vec!["touch".into(),"/data/ready".into()]}).await?;
        harness.pass(request("up", vec!["app"], Some(&waiting))).await?;
        let marker = harness.pass(Request::Exec {target:app,command:vec!["cat".into(),"/data/sentinel".into()]}).await?;
        if marker!="preserved" { return Err("Failure/cancellation/retry lost volume marker".into()); }
        Ok(())
    }.await;
    let cleanup = harness.pass(request("down", vec![], None)).await;
    let volume_cleanup = if cleanup.is_ok() {
        harness
            .pass(Request::Remove {
                resource: Resource::Volume,
                target: format!("{project}_data"),
            })
            .await
            .map(|_| ())
    } else {
        Err("Retained volume after failed cleanup".into())
    };
    if cleanup.is_ok() {
        std::fs::remove_file(&conflict_file).unwrap();
    }
    assert!(
        result.is_ok() && cleanup.is_ok() && volume_cleanup.is_ok(),
        "result: {result:?}; cleanup: {cleanup:?}; volume: {volume_cleanup:?}; evidence: {}",
        harness.evidence.display()
    );
}

#[tokio::test]
#[ignore = "owned standalone image/volume file read and preservation proof"]
async fn owned_standalone_files() {
    use sha2::Digest;
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_nanos();
    let name = format!("orbwin-files-{stamp}");
    let root = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("..");
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: root.join(format!(
            "artifacts/qualification/standalone-files-{stamp}.json"
        )),
    };
    let target = engine::target().unwrap();
    engine::check(&target).await.unwrap();
    let image =
        "alpine:3.21@sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30";
    let image_id = engine::command(&target, &["image", "inspect", "--format={{.Id}}", image])
        .await
        .unwrap()
        .trim()
        .to_string();
    harness
        .pass(Request::CreateVolume { name: name.clone() })
        .await
        .unwrap();
    let container = harness
        .pass(Request::Run {
            image: image.into(),
            name: name.clone(),
            command: vec!["sleep".into(), "300".into()],
            options: RunOptions {
                volumes: vec![format!("{name}:/data")],
                ..Default::default()
            },
        })
        .await
        .unwrap()
        .trim()
        .to_string();
    let result:Result<(),String>=async {
        harness.pass(Request::Exec{target:container.clone(),command:vec!["sh".into(),"-c".into(),"printf retained > /data/marker; ln -s /etc /data/absolute; head -c 1 /dev/zero > /data/binary; printf binary >> /data/binary; dd if=/dev/zero of=/data/large bs=1024 count=300".into()]}).await?;
        let volume_read=|path:&str,action|Request::Files{resource:FileResource::Volume,target:name.clone(),path:path.into(),action};
        let copy_path=root.join(format!("artifacts/qualification/copied binary ü {stamp}.bin"));
        let copied=harness.pass(Request::CopyFile{resource:FileResource::Volume,target:name.clone(),path:"/large".into(),destination:copy_path.to_str().unwrap().into()}).await?;
        let metadata:serde_json::Value=serde_json::from_str(copied.lines().find(|line|line.starts_with('{')).ok_or("Copy returned no receipt")?).map_err(|error|error.to_string())?;
        let bytes=std::fs::read(&copy_path).map_err(|error|error.to_string())?;
        if bytes.len()!=300*1024 || bytes.iter().any(|byte|*byte!=0) || metadata["bytes"]!=300*1024 {return Err("Binary volume copy differed".into());}
        if harness.call(Request::CopyFile{resource:FileResource::Volume,target:name.clone(),path:"/marker".into(),destination:copy_path.to_str().unwrap().into()}).await.is_ok(){return Err("Existing destination reached copy dispatch".into());}
        std::fs::remove_file(&copy_path).map_err(|error|error.to_string())?;
        let output=harness.pass(volume_read("/marker",FileAction::Read)).await?;
        let value:serde_json::Value=serde_json::from_str(&output).map_err(|e|e.to_string())?;
        if value["text"]!="retained"{return Err(format!("Standalone volume returned wrong data: {output}"));}
        let output=harness.pass(volume_read("/",FileAction::List)).await?;
        let value:serde_json::Value=serde_json::from_str(&output).map_err(|e|e.to_string())?;
        if !value["entries"].as_array().ok_or("No file entries")?.iter().any(|entry|entry["name"]=="absolute"&&entry["kind"]=="symlink"){return Err("Volume listing lost symbolic link metadata".into());}
        for path in ["/absolute/passwd","/binary","/large"]{
            let (outcome,_) = harness.call(volume_read(path,FileAction::Read)).await?;
            if outcome.success{return Err(format!("Unsupported file preview succeeded: {path}"));}
        }
        let output=harness.pass(Request::Files{resource:FileResource::Image,target:image_id,path:"/etc/alpine-release".into(),action:FileAction::Read}).await?;
        let value:serde_json::Value=serde_json::from_str(&output).map_err(|e|e.to_string())?;
        if !value["text"].as_str().unwrap_or("").starts_with("3.21."){return Err("Image mount returned unexpected release data".into());}
        let original_hash=format!("{:x}",sha2::Sha256::digest(b"retained"));
        let attempted=harness.call(Request::SaveVolumeFile{volume:name.clone(),path:"/marker".into(),expected:original_hash.clone(),text:"must-not-write".into()}).await?;
        if attempted.0.success{return Err("Active writer volume edit succeeded".into());}
        harness.pass(Request::Exec{target:container.clone(),command:vec!["sh".into(),"-c".into(),"chown 1234:2345 /data/marker; chmod 640 /data/marker".into()]}).await?;
        harness.pass(Request::Container{target:container.clone(),action:ContainerAction::Stop}).await?;
        let changed="  changed text with spaces \n";
        let saved=harness.pass(Request::SaveVolumeFile{volume:name.clone(),path:"/marker".into(),expected:original_hash.clone(),text:changed.into()}).await?;
        let receipt:serde_json::Value=serde_json::from_str(&saved).map_err(|error|error.to_string())?;
        if receipt["committed"]!=true{return Err(format!("No file commit receipt: {saved}"));}
        let copied=harness.pass(volume_read(receipt["backup"].as_str().ok_or("No backup path")?,FileAction::Read)).await?;
        let backup:serde_json::Value=serde_json::from_str(&copied).map_err(|error|error.to_string())?;
        if backup["text"]!="retained"{return Err("Displaced file version was not retained".into());}
        let stale=harness.call(Request::SaveVolumeFile{volume:name.clone(),path:"/marker".into(),expected:original_hash.clone(),text:"stale overwrite".into()}).await?;
        if stale.0.success{return Err("Stale file edit succeeded".into());}
        let current=harness.pass(volume_read("/marker",FileAction::Read)).await?;
        let current:serde_json::Value=serde_json::from_str(&current).map_err(|error|error.to_string())?;
        if current["text"]!=changed{return Err("Saved text whitespace changed or stale edit overwrote it".into());}
        harness.pass(Request::SaveVolumeFile{volume:name.clone(),path:"/marker".into(),expected:receipt["sha256"].as_str().ok_or("No new file hash")?.into(),text:"retained".into()}).await?;
        harness.pass(Request::Container{target:container.clone(),action:ContainerAction::Start}).await?;
        let attributes=harness.pass(Request::Exec{target:container.clone(),command:vec!["stat".into(),"-c".into(),"%u:%g:%a".into(),"/data/marker".into()]}).await?;
        if attributes.trim()!="1234:2345:640"{return Err(format!("File owner/mode changed: {attributes}"));}
        harness.pass(Request::Container{target:container.clone(),action:ContainerAction::Stop}).await?;

        harness.pass(volume_read("/marker",FileAction::Read)).await?;
        let mapped=harness.pass(Request::Files{resource:FileResource::Container,target:container.clone(),path:"/data/marker".into(),action:FileAction::Read}).await?;
        let mapped:serde_json::Value=serde_json::from_str(&mapped).map_err(|error|error.to_string())?;
        if mapped["editSource"]["volume"]!=name || mapped["editSource"]["path"]!="/marker" {return Err(format!("Stopped container did not map its editable volume: {mapped}"));}
        let helpers=engine::command(&target,&["ps","-aq","--filter","label=dev.orbwin.transient=files"]).await?;
        if !helpers.trim().is_empty(){return Err(format!("Normal file reads retained tools helpers: {helpers}"));}
        Ok(())
    }.await;
    let _ = harness
        .call(Request::Container {
            target: container.clone(),
            action: ContainerAction::Stop,
        })
        .await;
    let removed = harness
        .pass(Request::Remove {
            resource: Resource::Container,
            target: container,
        })
        .await;
    let volume_removed = if removed.is_ok() {
        harness
            .pass(Request::Remove {
                resource: Resource::Volume,
                target: name,
            })
            .await
    } else {
        Err("Retained volume after failed container cleanup".into())
    };
    assert!(
        result.is_ok() && removed.is_ok() && volume_removed.is_ok(),
        "result: {result:?}; container: {removed:?}; volume: {volume_removed:?}; evidence: {}",
        harness.evidence.display()
    );
}

#[tokio::test]
#[ignore = "Windows bind mount runtime proof; wait for explicit host availability"]
async fn owned_windows_bind_mounts() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_nanos();
    let root = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("..")
        .canonicalize()
        .unwrap();
    let name = format!("orbwin-bind-{stamp}");
    let source = root.join(format!("artifacts/qualification/bind source {stamp}.txt"));
    let file = std::fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .open(&source)
        .unwrap();
    drop(file);
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: root.join(format!("artifacts/qualification/bind-{stamp}.json")),
    };
    let mut container = None;
    let result: Result<(), String> = async {
        let created = harness.pass(Request::Run {
            image: "alpine:3.21@sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30".into(),
            name: name.clone(), command: vec!["sleep".into(), "300".into()],
            options: RunOptions { volumes: vec![
                format!("{}:/fixture:ro", root.join("tests/workloads/bind source").display()),
                format!("{}:/writable.txt", source.display()),
                format!("{}:/readonly.txt:ro", source.display()),
            ], ..Default::default() },
        }).await?;
        let target = created.trim().to_string(); container = Some(target.clone());
        let value = harness.pass(Request::Exec { target: target.clone(), command: vec!["cat".into(), "/fixture/hello.txt".into()] }).await?;
        if value.trim() != "orbwin-windows-bind-proof" { return Err(format!("Wrong directory mount content: {value}")); }
        let (readonly, _) = harness.call(Request::Exec { target: target.clone(), command: vec!["sh".into(), "-c".into(), "printf forbidden >> /readonly.txt".into()] }).await?;
        if readonly.success || readonly.exit_code == Some(0) { return Err("Read-only host mount accepted a write".into()); }
        harness.pass(Request::Exec { target: target.clone(), command: vec!["sh".into(), "-c".into(), "printf orbwin-file-mount-proof > /writable.txt".into()] }).await?;
        if std::fs::read_to_string(&source).map_err(|e| e.to_string())? != "orbwin-file-mount-proof" {
            return Err("Container file write was not visible on Windows".into());
        }
        harness.pass(Request::Container { target: target.clone(), action: ContainerAction::Restart }).await?;
        let value = harness.pass(Request::Exec { target, command: vec!["cat".into(), "/writable.txt".into()] }).await?;
        if value != "orbwin-file-mount-proof" { return Err("Host file data did not survive container restart".into()); }
        Ok(())
    }.await;
    let mut cleanup = Ok(());
    if let Some(target) = container {
        cleanup = async {
            harness
                .pass(Request::Container {
                    target: target.clone(),
                    action: ContainerAction::Stop,
                })
                .await?;
            harness
                .pass(Request::Remove {
                    resource: Resource::Container,
                    target,
                })
                .await?;
            Ok::<(), String>(())
        }
        .await;
    }
    if cleanup.is_ok() {
        std::fs::remove_file(&source).unwrap();
    }
    assert!(
        result.is_ok() && cleanup.is_ok(),
        "result: {result:?}; cleanup: {cleanup:?}; evidence: {}",
        harness.evidence.display()
    );
}
impl Harness {
    // Heap-bound the adapter future so long scenario tables do not multiply its
    // debug-build stack storage at each await site on Windows test threads.
    fn call(
        &mut self,
        request: Request,
    ) -> std::pin::Pin<Box<dyn std::future::Future<Output = Result<(Outcome, String), String>> + '_>>
    {
        Box::pin(self.call_until(request, None))
    }
    async fn call_until(
        &mut self,
        request: Request,
        cancel_at: Option<&str>,
    ) -> Result<(Outcome, String), String> {
        eprintln!("owned qualification: {request:?}");
        let text = Arc::new(Mutex::new(String::new()));
        let received = text.clone();
        let reached = Arc::new(tokio::sync::Notify::new());
        let notify = reached.clone();
        let marker = cancel_at.map(String::from);
        let channel = Channel::new(move |body| {
            let event: serde_json::Value = body.deserialize()?;
            if event["kind"] == "data" {
                let bytes: Vec<u8> = serde_json::from_value(event["bytes"].clone())?;
                let mut text = received.lock().unwrap();
                text.push_str(&String::from_utf8_lossy(&bytes));
                if marker.as_ref().is_some_and(|marker| text.contains(marker)) {
                    notify.notify_one();
                }
            }
            Ok(())
        });
        let description = format!("{request:?}");
        let operation_id = id();
        let mut job = Box::pin(run(&self.state, operation_id.clone(), request, channel));
        let result = if cancel_at.is_some() {
            tokio::select! {
                result = &mut job => result,
                _ = reached.notified() => {
                    cancel(&self.state, &operation_id)?;
                    job.await
                }
            }
        } else {
            job.await
        };
        let output = text.lock().unwrap().clone();
        self.records.push(serde_json::json!({ "request": description, "outcome": result.as_ref().ok(), "error": result.as_ref().err(), "output": output }));
        std::fs::write(
            &self.evidence,
            serde_json::to_vec_pretty(&self.records).unwrap(),
        )
        .map_err(|e| e.to_string())?;
        result.map(|outcome| (outcome, output))
    }
    async fn pass(&mut self, request: Request) -> Result<String, String> {
        let (outcome, output) = self.call(request).await?;
        if !outcome.success {
            return Err(format!("{}\n{output}", outcome.message));
        }
        Ok(output)
    }
}

#[tokio::test]
#[ignore = "requires ORBWIN_QUALIFY_OWNED=1 and the verified owned running engine"]
async fn owned_resource_operations() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_millis();
    let name = format!("orbwin-operations-{stamp}");
    let root = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("..");
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: root.join(format!("artifacts/qualification/operations-{stamp}.json")),
    };
    let mut container = None;
    let mut network = None;
    let mut volume = false;
    let result: Result<(), String> = async {
        harness.pass(Request::CreateVolume { name: name.clone() }).await?; volume = true;
        harness.pass(Request::Inspect { resource: Resource::Volume, target: name.clone() }).await?;
        network = Some(harness.pass(Request::CreateNetwork { name: name.clone(), driver: String::new(), subnet: String::new(), internal: false }).await?.trim().to_string());
        let network_id = network.clone().unwrap();
        harness.pass(Request::Inspect { resource: Resource::Network, target: network_id.clone() }).await?;
        let created = harness.pass(Request::Run { image: "alpine:3.21@sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30".into(), name: name.clone(), command: vec!["sh".into(), "-c".into(), "echo orbwin-log-proof; sleep 300".into()], options: RunOptions { ports: vec!["8080".into()], environment: vec!["ORBWIN_PROOF=container-environment".into()], volumes: vec![format!("{name}:/proof-data")], ..Default::default() } }).await?;
        let container_id = created.trim().to_string(); container = Some(container_id.clone());
        let details: serde_json::Value = serde_json::from_str(&harness.pass(Request::Inspect { resource: Resource::Container, target: container_id.clone() }).await?).map_err(|e| e.to_string())?;
        if details[0]["HostConfig"]["PortBindings"]["8080/tcp"][0]["HostIp"] != "127.0.0.1" { return Err("Default publication escaped localhost".into()); }
        if !details[0]["Config"]["Env"].as_array().ok_or("Missing environment")?.iter().any(|entry| entry == "ORBWIN_PROOF=container-environment") { return Err("Requested environment was not applied".into()); }
        harness.pass(Request::Exec { target: container_id.clone(), command: vec!["sh".into(), "-c".into(), "printf preserved > /proof-data/sentinel".into()] }).await?;
        let (attached_volume, _) = harness.call(Request::Remove { resource: Resource::Volume, target: name.clone() }).await?;
        if attached_volume.success { return Err("Attached volume removal unexpectedly succeeded".into()); }
        harness.pass(Request::ConnectNetwork { network: network_id.clone(), container: container_id.clone(), disconnect: false }).await?;
        let (_, refused) = harness.call(Request::Remove { resource: Resource::Network, target: network_id.clone() }).await?;
        if !refused.contains("active endpoints") { return Err(format!("Expected in-use network rejection: {refused}")); }
        harness.pass(Request::ConnectNetwork { network: network_id, container: container_id.clone(), disconnect: true }).await?;
        let value = harness.pass(Request::Exec { target: container_id.clone(), command: vec!["printf".into(), "orbwin-exec-proof".into()] }).await?;
        if value != "orbwin-exec-proof" { return Err(format!("Unexpected exec result: {value}")); }
        let (failure, _) = harness.call(Request::Exec { target: container_id.clone(), command: vec!["sh".into(), "-c".into(), "exit 7".into()] }).await?;
        if failure.exit_code != Some(7) || failure.success { return Err("Container command exit status was lost".into()); }
        let logs = harness.pass(Request::Logs { target: container_id.clone(), follow: false }).await?;
        if !logs.contains("orbwin-log-proof") { return Err("Expected log line missing".into()); }
        harness.pass(Request::Stats { target: container_id.clone() }).await?;
        for action in [ContainerAction::Pause, ContainerAction::Unpause, ContainerAction::Restart, ContainerAction::Stop, ContainerAction::Start] {
            harness.pass(Request::Container { action, target: container_id.clone() }).await?;
        }
        let preserved = harness.pass(Request::Exec { target: container_id.clone(), command: vec!["cat".into(), "/proof-data/sentinel".into()] }).await?;
        if preserved != "preserved" { return Err("Named-volume data did not survive the lifecycle sequence".into()); }
        let (refused, _) = harness.call(Request::Remove { resource: Resource::Container, target: container_id.clone() }).await?;
        if refused.success { return Err("Running container removal unexpectedly succeeded".into()); }
        // Cancel follow after the first actual log payload, so cancellation
        // exercises a dispatched client rather than just preflight rejection.
        let job_id = id(); let state = &harness.state;
        let (ready_tx, ready_rx) = oneshot::channel(); let ready = Mutex::new(Some(ready_tx));
        let channel = Channel::new(move |body| { let event: serde_json::Value = body.deserialize()?; if event["kind"] == "data" { if let Some(sender) = ready.lock().unwrap().take() { let _ = sender.send(()); } } Ok(()) });
        let running = run(state, job_id.clone(), Request::Logs { target: container_id, follow: true }, channel);
        let cancel_client = async { tokio::time::timeout(Duration::from_secs(20), ready_rx).await.map_err(|_| "Follow emitted no output")?.map_err(|_| "Follow ended before output")?; cancel(state, &job_id) };
        let (outcome, cancelled) = tokio::join!(running, cancel_client); cancelled?;
        let outcome = outcome?;
        harness.records.push(serde_json::json!({ "case": "cancel active log follow", "outcome": outcome }));
        if !outcome.interrupted || outcome.success { return Err("Active follow cancellation was not reflected".into()); }
        Ok(())
    }.await;
    let mut cleanup_errors = vec![];
    if let Some(container) = container {
        if let Err(e) = harness
            .pass(Request::Container {
                target: container.clone(),
                action: ContainerAction::Stop,
            })
            .await
        {
            cleanup_errors.push(e);
        }
        if let Err(e) = harness
            .pass(Request::Remove {
                resource: Resource::Container,
                target: container,
            })
            .await
        {
            cleanup_errors.push(e);
        }
    }
    if let Some(network) = network {
        if let Err(e) = harness
            .pass(Request::Remove {
                resource: Resource::Network,
                target: network,
            })
            .await
        {
            cleanup_errors.push(e);
        }
    }
    if volume {
        if let Err(e) = harness
            .pass(Request::Remove {
                resource: Resource::Volume,
                target: name,
            })
            .await
        {
            cleanup_errors.push(e);
        }
    }
    assert!(
        result.is_ok() && cleanup_errors.is_empty(),
        "Result: {result:?}; cleanup: {cleanup_errors:?}; evidence: {}",
        harness.evidence.display()
    );
}

#[tokio::test]
#[ignore = "explicit owned engine stop/start proof; requires no running OrbWin containers"]
async fn owned_management_cycle() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_millis();
    let root = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("..");
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: root.join(format!("artifacts/qualification/management-{stamp}.json")),
    };
    let target = engine::target().unwrap();
    let before = engine::check(&target).await.unwrap();
    let mut command = Command::new(&target.docker);
    command
        .arg("--config")
        .arg(&target.config)
        .arg("--host")
        .arg(engine::ENDPOINT)
        .args(["ps", "--quiet"]);
    let containers = command.output().await.unwrap();
    assert!(
        containers.status.success() && containers.stdout.is_empty(),
        "Refusing to interrupt running OrbWin containers"
    );
    let config = harness.pass(Request::ReadSettings).await.unwrap();
    let app: serde_json::Value = serde_json::from_str(&config).unwrap();
    assert_eq!(app["spec"]["running"], true);
    let stopped = harness.pass(Request::Engine { running: false }).await;
    // Always attempt restoration even if stop failed after committing.
    let started = harness.pass(Request::Engine { running: true }).await;
    assert!(
        stopped.is_ok() && started.is_ok(),
        "stop: {stopped:?}; start: {started:?}"
    );
    let after = engine::check(&target).await.unwrap();
    assert_eq!(before["ID"], after["ID"]);
}

#[tokio::test]
#[ignore = "optional owned Kubernetes enable/workload/storage/delete/disable proof"]
async fn owned_cluster_workflow() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_millis();
    let name = format!("orbwin-cluster-{stamp}");
    let root = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("..");
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: root.join(format!("artifacts/qualification/cluster-{stamp}.json")),
    };
    let config: serde_json::Value =
        serde_json::from_str(&harness.pass(Request::ReadSettings).await.unwrap()).unwrap();
    let was_enabled = config["spec"]["kubernetes"]["enabled"]
        .as_bool()
        .unwrap_or(false);
    let file = root.join(format!(
        "artifacts/qualification/cluster-manifest-{stamp}.json"
    ));
    let manifest = serde_json::json!({ "apiVersion":"v1", "kind":"List", "items":[
        {"apiVersion":"v1", "kind":"PersistentVolumeClaim", "metadata":{"name":name,"namespace":"default"},"spec":{"accessModes":["ReadWriteOnce"],"resources":{"requests":{"storage":"16Mi"}}}},
        {"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":name,"namespace":"default"},"spec":{"replicas":1,"selector":{"matchLabels":{"app":name}},"template":{"metadata":{"labels":{"app":name}},"spec":{"containers":[{"name":"server","image":"redis:7.4.2-alpine@sha256:02419de7eddf55aa5bcf49efb74e88fa8d931b4d77c07eff8a6b2144472b6952","command":["sh","-c","echo orbwin-cluster-proof; echo preserved > /data/proof; exec redis-server --dir /data --appendonly yes"],"ports":[{"containerPort":6379}],"readinessProbe":{"tcpSocket":{"port":6379},"initialDelaySeconds":1,"periodSeconds":1},"volumeMounts":[{"name":"data","mountPath":"/data"}]}],"volumes":[{"name":"data","persistentVolumeClaim":{"claimName":name}}]}}}},
        {"apiVersion":"v1","kind":"Service","metadata":{"name":name,"namespace":"default"},"spec":{"selector":{"app":name},"ports":[{"port":80,"targetPort":6379}]}}
    ]});
    std::fs::write(&file, serde_json::to_vec_pretty(&manifest).unwrap()).unwrap();
    let result: Result<(), String> = async {
        if !was_enabled {
            harness
                .pass(Request::ConfigureCluster {
                    enabled: true,
                    version: None,
                })
                .await?;
        }
        harness
            .pass(Request::ClusterApply {
                file: file.to_str().unwrap().into(),
            })
            .await?;
        harness
            .pass(Request::ClusterWait {
                resource: ClusterResource::Deployments,
                namespace: "default".into(),
                name: name.clone(),
                deleted: false,
            })
            .await?;
        harness
            .pass(Request::ClusterWait {
                resource: ClusterResource::PersistentVolumeClaims,
                namespace: "default".into(),
                name: name.clone(),
                deleted: false,
            })
            .await?;
        for resource in [
            ClusterResource::Deployments,
            ClusterResource::Services,
            ClusterResource::PersistentVolumeClaims,
        ] {
            let output = harness
                .pass(Request::Cluster {
                    action: ClusterAction::Inspect,
                    resource,
                    namespace: "default".into(),
                    name: name.clone(),
                })
                .await?;
            let object: serde_json::Value =
                serde_json::from_str(&output).map_err(|e| e.to_string())?;
            if object["metadata"]["name"] != name {
                return Err("Cluster returned a different resource".into());
            }
        }
        let pods: serde_json::Value = serde_json::from_str(
            &harness
                .pass(Request::Cluster {
                    action: ClusterAction::List,
                    resource: ClusterResource::Pods,
                    namespace: "default".into(),
                    name: "".into(),
                })
                .await?,
        )
        .map_err(|e| e.to_string())?;
        let pod = pods["items"]
            .as_array()
            .ok_or("No pod list")?
            .iter()
            .find(|pod| pod["metadata"]["labels"]["app"] == name)
            .ok_or("Test deployment has no pod")?;
        let logs = harness
            .pass(Request::Cluster {
                action: ClusterAction::Logs,
                resource: ClusterResource::Pods,
                namespace: "default".into(),
                name: pod["metadata"]["name"]
                    .as_str()
                    .ok_or("Pod has no name")?
                    .into(),
            })
            .await?;
        if !logs.contains("orbwin-cluster-proof") {
            return Err("Cluster log output missing".into());
        }
        let (wrong, _) = harness
            .call(Request::ClusterDelete {
                resource: ClusterResource::Services,
                namespace: "default".into(),
                name: name.clone(),
                uid: "00000000-0000-4000-8000-000000000000".into(),
            })
            .await?;
        if wrong.success {
            return Err("Wrong UID deletion unexpectedly succeeded".into());
        }
        harness
            .pass(Request::Cluster {
                action: ClusterAction::Inspect,
                resource: ClusterResource::Services,
                namespace: "default".into(),
                name: name.clone(),
            })
            .await?;
        Ok(())
    }
    .await;
    if result.is_err() {
        for resource in [
            ClusterResource::Pods,
            ClusterResource::Events,
            ClusterResource::PersistentVolumeClaims,
        ] {
            let _ = harness
                .call(Request::Cluster {
                    action: ClusterAction::List,
                    resource,
                    namespace: "default".into(),
                    name: "".into(),
                })
                .await;
        }
    }
    let mut cleanup_errors = vec![];
    for resource in [
        ClusterResource::Deployments,
        ClusterResource::Services,
        ClusterResource::PersistentVolumeClaims,
    ] {
        // Cleanup discovers the UID of only the exact unique test name. A
        // failed/partial apply must never turn cleanup into a broad deletion.
        match harness
            .call(Request::Cluster {
                action: ClusterAction::Inspect,
                resource,
                namespace: "default".into(),
                name: name.clone(),
            })
            .await
        {
            Ok((outcome, output)) if outcome.success => {
                match serde_json::from_str::<serde_json::Value>(&output) {
                    Ok(object) => {
                        if let Some(uid) = object["metadata"]["uid"].as_str() {
                            if let Err(e) = harness
                                .pass(Request::ClusterDelete {
                                    resource,
                                    namespace: "default".into(),
                                    name: name.clone(),
                                    uid: uid.into(),
                                })
                                .await
                            {
                                cleanup_errors.push(e);
                            }
                            if let Err(e) = harness
                                .pass(Request::ClusterWait {
                                    resource,
                                    namespace: "default".into(),
                                    name: name.clone(),
                                    deleted: true,
                                })
                                .await
                            {
                                cleanup_errors.push(e);
                            }
                        } else {
                            cleanup_errors.push("Cleanup object has no UID".into());
                        }
                    }
                    Err(e) => cleanup_errors.push(e.to_string()),
                }
            }
            Ok((_, output)) if output.contains("NotFound") || output.contains("not found") => {}
            Ok((outcome, output)) => cleanup_errors.push(format!("{}\n{output}", outcome.message)),
            Err(e) => cleanup_errors.push(e),
        }
    }
    if !was_enabled {
        if let Err(e) = harness
            .pass(Request::ConfigureCluster {
                enabled: false,
                version: None,
            })
            .await
        {
            cleanup_errors.push(e);
        }
    }
    assert!(
        result.is_ok() && cleanup_errors.is_empty(),
        "result: {result:?}; cleanup: {cleanup_errors:?}; evidence: {}",
        harness.evidence.display()
    );
}
