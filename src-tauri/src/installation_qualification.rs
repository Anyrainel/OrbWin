//! Sequential, explicitly opted-in clean provisioning. Only the fixed proof
//! guest is used; product and other engines are never stopped by this test.
use super::*;
use std::sync::{Arc, Mutex};

#[tokio::test]
#[ignore = "clean setup/cancel/resume proof; product guest must already be stopped"]
async fn owned_first_run_setup() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let system = PathBuf::from(std::env::var_os("SystemRoot").unwrap()).join("System32/wsl.exe");
    let running = process::read_command(&system, &["--list", "--running", "--quiet"])
        .await
        .unwrap();
    assert!(!running.lines().any(|line|line.trim().eq_ignore_ascii_case("lima-orbwin")),"Stop only the verified product backend before running a sequential proof; this test will not stop it.");
    let directory = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../artifacts/setup-proof");
    std::fs::create_dir_all(&directory).unwrap();
    let directory = directory.canonicalize().unwrap();
    let location = SetupLocation {
        directory: directory.clone(),
        instance: "orbwin-proof",
        vm: "orbwin-proof",
        endpoint: "npipe:////./pipe/docker-rancher-desktop-orbwin-proof",
    };
    let home = directory.join("home");
    let had_guest = registration(&home, "lima-orbwin-proof").unwrap();
    let messages = Arc::new(Mutex::new(Vec::<String>::new()));
    let (sender, mut receiver) = oneshot::channel();
    let cancel = Arc::new(Mutex::new(Some(sender)));
    let captured = messages.clone();
    let trigger = cancel.clone();
    let output = Channel::new(move |body| {
        let event: serde_json::Value = body.deserialize()?;
        if event["kind"] == "data" {
            let bytes: Vec<u8> = serde_json::from_value(event["bytes"].clone())?;
            let text = String::from_utf8_lossy(&bytes).into_owned();
            if !had_guest && text.contains("Starting the OrbWin management service") {
                if let Some(sender) = trigger.lock().unwrap().take() {
                    let _ = sender.send(());
                }
            }
            eprintln!("{text}");
            captured.lock().unwrap().push(text);
        }
        Ok(())
    });
    let cancelled_result = if !had_guest {
        Some(
            setup_at(&location, &output, &mut receiver)
                .await
                .map(|outcome| outcome.success),
        )
    } else {
        None
    };
    if let Some(result) = &cancelled_result {
        assert!(
            result
                .as_ref()
                .is_err_and(|error| error.contains("cancelled")),
            "{result:?}"
        );
        assert!(!directory.join("runtime.json").exists());
    }
    let (_sender, mut receiver) = oneshot::channel();
    let messages2 = messages.clone();
    let output = Channel::new(move |body| {
        let event: serde_json::Value = body.deserialize()?;
        if event["kind"] == "data" {
            let bytes: Vec<u8> = serde_json::from_value(event["bytes"].clone())?;
            let text = String::from_utf8_lossy(&bytes).into_owned();
            eprintln!("{text}");
            messages2.lock().unwrap().push(text);
        }
        Ok(())
    });
    let result = setup_at(&location, &output, &mut receiver)
        .await
        .map(|outcome| outcome.success);
    let mut resume = None;
    let mut smoke = None;
    if result == Ok(true) {
        let docker = runtime::docker().unwrap();
        let config = home.join(".rdorbwin-proof/docker");
        let value = process::read_command(
            &docker,
            &[
                "--config",
                config.to_str().unwrap(),
                "--host",
                location.endpoint,
                "info",
                "--format",
                "{{json .}}",
            ],
        )
        .await
        .unwrap();
        let info: serde_json::Value = serde_json::from_str(&value).unwrap();
        assert_eq!(info["OSType"], "linux");
        let identity: serde_json::Value =
            serde_json::from_slice(&std::fs::read(directory.join("runtime.json")).unwrap())
                .unwrap();
        assert_eq!(info["ID"], identity["daemonId"]);
        smoke = Some(info);
        resume = Some(
            setup_at(&location, &output, &mut receiver)
                .await
                .map(|outcome| outcome.success),
        );
    }
    // Preserve the owned guest/disks for inspection; only stop its service.
    let (binary, _) = runtime::backend().unwrap();
    let stop = if registration(&home, "lima-orbwin-proof").is_ok() {
        rdd(
            &binary,
            &location,
            &["service", "stop", "--timeout=60s"],
            None,
            &output,
            &mut receiver,
            75,
        )
        .await
    } else {
        Err("Guest ownership changed; no stop sent".into())
    };
    std::fs::write(directory.join("result.json"),serde_json::to_vec_pretty(&serde_json::json!({"hadGuest":had_guest,"cancelled":cancelled_result,"setup":result,"resume":resume,"engine":smoke,"stop":stop,"messages":*messages.lock().unwrap()})).unwrap()).unwrap();
    assert_eq!(result, Ok(true));
    assert_eq!(resume, Some(Ok(true)));
    assert!(stop.is_ok());
}
