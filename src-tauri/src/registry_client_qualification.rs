//! Explicit synthetic loopback credential qualification; no unrelated vault Get.
//! Root registers/runs only after reviewing the fixture's exclusive mutation lane.
use super::*;
use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use std::{
    path::{Path, PathBuf},
    sync::Arc,
};

fn require(ok: bool, message: &str) -> Result<(), String> {
    if ok {
        Ok(())
    } else {
        Err(message.into())
    }
}
fn digest(bytes: &[u8]) -> String {
    format!("{:x}", Sha256::digest(bytes))
}
fn read(path: &Path) -> Result<Vec<u8>, String> {
    use std::io::Read;
    let meta = std::fs::symlink_metadata(path).map_err(|e| e.to_string())?;
    require(
        meta.is_file() && !meta.file_type().is_symlink() && meta.len() <= 1 << 20,
        "Profile is not a bounded regular file",
    )?;
    let mut bytes = Vec::new();
    std::fs::File::open(path)
        .map_err(|e| e.to_string())?
        .take((1 << 20) + 1)
        .read_to_end(&mut bytes)
        .map_err(|e| e.to_string())?;
    require(bytes.len() <= 1 << 20, "Profile exceeded limit")?;
    Ok(bytes)
}
fn canonical(bytes: &[u8], server: &str) -> Result<Value, String> {
    let value: Value = serde_json::from_slice(bytes).map_err(|_| "Invalid profile JSON")?;
    require(value.is_object(), "Profile must be an object")?;
    require(
        serde_json::to_vec_pretty(&value).map_err(|e| e.to_string())? == bytes,
        "Require canonical profile before synthetic login; existing bytes retained",
    )?;
    require(
        value["credHelpers"][server].is_null() && value["auths"][server].is_null(),
        "Selected fixture server already has metadata",
    )?;
    Ok(value)
}
fn without_fixture(
    mut current: Value,
    original: &Value,
    server: &str,
    helper: &str,
) -> Result<Value, String> {
    let object = current.as_object_mut().ok_or("Profile became non-object")?;
    if let Some(helpers) = object.get_mut("credHelpers") {
        let helpers = helpers
            .as_object_mut()
            .ok_or("Helper metadata became invalid")?;
        if let Some(value) = helpers.get(server) {
            require(
                value == helper,
                "Fixture route changed ownership; retain metadata",
            )?;
            helpers.remove(server);
        }
        if helpers.is_empty() && original.get("credHelpers").is_none() {
            object.remove("credHelpers");
        }
    }
    if let Some(auths) = object.get_mut("auths") {
        let auths = auths
            .as_object_mut()
            .ok_or("Auth metadata became invalid")?;
        if let Some(value) = auths.get(server) {
            require(
                value.as_object().is_some_and(|v| v.is_empty()),
                "Fixture file auth contains unexpected values; retain metadata",
            )?;
            auths.remove(server);
        }
        if auths.is_empty() && original.get("auths").is_none() {
            object.remove("auths");
        }
    }
    require(
        current == *original,
        "Unrelated profile metadata changed; restoration refused",
    )?;
    Ok(current)
}
struct Native {
    state: Operations,
    records: Vec<Value>,
    path: PathBuf,
}
impl Native {
    fn save(&self) -> Result<(), String> {
        std::fs::write(
            &self.path,
            serde_json::to_vec_pretty(&self.records).map_err(|e| e.to_string())?,
        )
        .map_err(|e| e.to_string())
    }
    async fn call(&mut self, request: Request) -> Result<(Outcome, String), String> {
        let description = format!("{request:?}");
        let list = matches!(request, Request::RegistryList);
        self.records
            .push(json!({"phase":"intent","request":description}));
        self.save()?;
        let output = Arc::new(Mutex::new(String::new()));
        let incoming = output.clone();
        let channel = Channel::new(move |body| {
            let event: Value = body.deserialize()?;
            if event["kind"] == "data" {
                let bytes: Vec<u8> = serde_json::from_value(event["bytes"].clone())?;
                incoming
                    .lock()
                    .unwrap()
                    .push_str(&String::from_utf8_lossy(&bytes));
            }
            Ok(())
        });
        let id = format!(
            "{:08x}-0000-4000-8000-{:012x}",
            std::process::id(),
            self.records.len()
        );
        let result = Box::pin(run(&self.state, id, request, channel)).await;
        let text = output.lock().map_err(|_| "Output lock poisoned")?.clone();
        // RegistryList exposes unrelated registry/user names: retain only its digest.
        self.records.push(json!({"request":description,"outcome":result.as_ref().ok(),"error":result.as_ref().err(),"outputSha256":digest(text.as_bytes()),"listMetadataOmitted":list}));
        self.save()?;
        result.map(|value| (value, text))
    }
    async fn list(&mut self) -> Result<Value, String> {
        let (outcome, text) = self.call(Request::RegistryList).await?;
        require(outcome.success, "Registry list failed")?;
        let value: Value = serde_json::from_str(&text).map_err(|_| "Registry list is invalid")?;
        require(value.is_object(), "Registry list must be an object")?;
        Ok(value)
    }
    async fn child(
        &mut self,
        node: &str,
        script: &Path,
        mode: &str,
        state: &Path,
    ) -> Result<(), String> {
        let script = crate::runtime::cli_path(script)?;
        let state = crate::runtime::cli_path(state)?;
        self.records
            .push(json!({"phase":"client-intent","mode":mode}));
        self.save()?;
        let mut command = tokio::process::Command::new(node);
        command.args([
            script.as_os_str(),
            std::ffi::OsStr::new(mode),
            state.as_os_str(),
        ]);
        let (_tx, mut receiver) = oneshot::channel();
        let output = Channel::new(|_| Ok(()));
        let result = client_tree(command, None, &output, &mut receiver, 240).await?;
        self.records.push(json!({"phase":"client-result","mode":mode,"exitCode":result.exit_code,"interrupted":result.interrupted}));
        self.save()?;
        require(
            result.exit_code == Some(0) && !result.interrupted,
            "Private client stage failed; inspect its minimized evidence",
        )
    }
}

#[tokio::test]
#[ignore = "synthetic loopback registry/native credentials/current CLI and Dev Containers; exclusive root lane"]
async fn owned_private_clients() {
    let result = qualify().await;
    assert!(result.is_ok(), "{result:?}");
}
async fn qualify() -> Result<(), String> {
    require(
        std::env::var("ORBWIN_QUALIFY_PRIVATE_CLIENTS").as_deref() == Ok("1")
            && std::env::var("ORBWIN_QUALIFY_OWNED").as_deref() == Ok("1"),
        "Explicit private-client opt-in required",
    )?;
    let state =
        PathBuf::from(std::env::var("ORBWIN_PRIVATE_CLIENT_STATE").map_err(|e| e.to_string())?);
    let root = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("..")
        .canonicalize()
        .map_err(|e| e.to_string())?;
    let state = state.canonicalize().map_err(|e| e.to_string())?;
    require(
        state.starts_with(root.join("artifacts/qualification")),
        "Fixture state is outside qualification root",
    )?;
    let fixture: Value =
        serde_json::from_slice(&read(&state)?).map_err(|_| "Invalid fixture state")?;
    let nonce = fixture["nonce"].as_str().ok_or("Missing nonce")?;
    require(
        nonce.len() == 32 && nonce.bytes().all(|b| b.is_ascii_hexdigit()),
        "Invalid nonce",
    )?;
    let server = fixture["server"].as_str().ok_or("Missing server")?;
    let port = server
        .strip_prefix("127.0.0.1:")
        .ok_or("Only loopback synthetic registry supported")?
        .parse::<u16>()
        .map_err(|_| "Invalid port")?;
    require(port > 0, "Invalid fixture port")?;
    let username = format!("orbwin-proof-{}", &nonce[..12]);
    require(fixture["username"] == username, "Fixture username differs")?;
    let node = std::env::var("ORBWIN_QUALIFY_NODE").map_err(|e| e.to_string())?;
    require(Path::new(&node).is_absolute(), "Node path must be absolute")?;
    let owned_target = engine::target()?;
    require(
        engine::check(&owned_target).await?["ID"] == "d9c00194-ea9d-439a-8561-6a0ec580fbd5",
        "Owned identity drift",
    )?;
    let home = owned_target.home().to_path_buf();
    let windows = home.join(".rdorbwin/docker/config.json");
    let linux = home.join(".rdorbwin/wsl-docker/config.json");
    let client_profile = PathBuf::from(
        fixture["privateConfig"]
            .as_str()
            .ok_or("Missing explicit client profile")?,
    )
    .canonicalize()
    .map_err(|e| e.to_string())?;
    require(
        Some(client_profile)
            == windows
                .parent()
                .map(|v| v.canonicalize())
                .transpose()
                .map_err(|e| e.to_string())?,
        "Client profile differs from native product profile",
    )?;
    let (windows_bytes, linux_bytes, windows_original, linux_original) = {
        let _lock = crate::registry::config_lock(&home)?;
        let win = read(&windows)?;
        let lin = read(&linux)?;
        let a = canonical(&win, server)?;
        let b = canonical(&lin, server)?;
        (win, lin, a, b)
    };
    let evidence = state
        .parent()
        .ok_or("Missing fixture directory")?
        .join("native-credentials.json");
    require(
        !evidence.exists(),
        "Native credential evidence already exists",
    )?;
    let mut native = Native {
        state: Operations::default(),
        path: evidence,
        records: vec![
            json!({"phase":"baseline","nonce":nonce,"server":server,"windowsSha256":digest(&windows_bytes),"wslSha256":digest(&linux_bytes)}),
        ],
    };
    native.save()?;
    let baseline = native.list().await?;
    require(
        baseline[server].is_null(),
        "Fixture server has existing saved credentials; no login sent",
    )?;
    let container = fixture["registryContainer"]
        .as_str()
        .ok_or("Missing registry container")?;
    target(Resource::Container, container)?;
    let (outcome, text) = native
        .call(Request::Inspect {
            resource: Resource::Container,
            target: container.into(),
        })
        .await?;
    require(outcome.success, "Registry fixture inspect failed")?;
    let inspected: Value = serde_json::from_str(&text).map_err(|_| "Invalid registry inspect")?;
    let inspected = inspected[0].clone();
    require(
        inspected["Id"] == container
            && inspected["Config"]["Labels"]["dev.orbwin.qualification"] == nonce
            && inspected["Image"] == fixture["registryImageId"]
            && inspected["State"]["Running"] == true,
        "Registry fixture identity differs",
    )?;
    require(
        inspected["NetworkSettings"]["Ports"]["5000/tcp"][0]["HostIp"] == "127.0.0.1"
            && inspected["NetworkSettings"]["Ports"]["5000/tcp"][0]["HostPort"] == port.to_string(),
        "Registry port differs from synthetic target",
    )?;
    let script = root.join("scripts/qualify-private-clients.mjs");
    let mut staged_profile_hashes: Option<(String, String)> = None;
    let result: Result<(), String> = async {
        let failed = native
            .call(Request::RegistryLogin {
                server: server.into(),
                username: username.clone(),
                secret: crate::registry::Secret("orbwin-wrong-disposable-only".into()),
            })
            .await?;
        require(!failed.0.success, "Wrong synthetic password was accepted")?;
        let logged = native
            .call(Request::RegistryLogin {
                server: server.into(),
                username: username.clone(),
                secret: crate::registry::Secret("orbwin-disposable-proof-only".into()),
            })
            .await?;
        require(logged.0.success, "Synthetic login failed")?;
        require(
            native.list().await?[server] == username,
            "Synthetic saved username differs",
        )?;
        {
            let _lock = crate::registry::config_lock(&home)?;
            let win_bytes = read(&windows)?;
            let lin_bytes = read(&linux)?;
            let win: Value =
                serde_json::from_slice(&win_bytes).map_err(|_| "Invalid Windows profile")?;
            let lin: Value =
                serde_json::from_slice(&lin_bytes).map_err(|_| "Invalid WSL profile")?;
            require(
                win["credHelpers"][server] == "orbwin"
                    && lin["credHelpers"][server] == "orbwin.exe",
                "Synthetic helper route missing",
            )?;
            without_fixture(win, &windows_original, server, "orbwin")?;
            without_fixture(lin, &linux_original, server, "orbwin.exe")?;
            staged_profile_hashes = Some((digest(&win_bytes), digest(&lin_bytes)));
        }
        native
            .child(&node, &script, "authenticated", &state)
            .await?;
        let forgot = native
            .call(Request::RegistryForget {
                server: server.into(),
            })
            .await?;
        require(forgot.0.success, "Synthetic Forget failed")?;
        require(
            native.list().await? == baseline,
            "Unrelated registry metadata changed or synthetic key retained",
        )?;
        native.child(&node, &script, "forgotten", &state).await?;
        Ok(())
    }
    .await;
    let mut cleanup = Vec::<String>::new();
    match native.list().await {
        Ok(list) => {
            if !list[server].is_null() {
                if list[server] == username {
                    match native
                        .call(Request::RegistryForget {
                            server: server.into(),
                        })
                        .await
                    {
                        Ok((outcome, _)) if outcome.success => (),
                        _ => cleanup.push("Synthetic Forget cleanup failed".into()),
                    }
                } else {
                    cleanup.push("Saved fixture key changed username; erase refused".into());
                }
            }
        }
        Err(e) => cleanup.push(e),
    };
    if cleanup.is_empty() {
        let restored: Result<(), String> = (|| {
            let _lock = crate::registry::config_lock(&home)?;
            let win_bytes = read(&windows)?;
            let lin_bytes = read(&linux)?;
            if let Some((win_hash, lin_hash)) = &staged_profile_hashes {
                require(
                    digest(&win_bytes) == *win_hash && digest(&lin_bytes) == *lin_hash,
                    "Profile bytes changed after login; restoration refused",
                )?;
            }
            let win: Value =
                serde_json::from_slice(&win_bytes).map_err(|_| "Invalid Windows config")?;
            let lin: Value =
                serde_json::from_slice(&lin_bytes).map_err(|_| "Invalid WSL config")?;
            without_fixture(win, &windows_original, server, "orbwin")?;
            without_fixture(lin, &linux_original, server, "orbwin.exe")?;
            crate::runtime::write_json(&windows, &windows_original)?;
            crate::runtime::write_json(&linux, &linux_original)?;
            require(
                read(&windows)? == windows_bytes && read(&linux)? == linux_bytes,
                "Profile byte restoration differs",
            )?;
            Ok(())
        })();
        if let Err(e) = restored {
            cleanup.push(e);
        }
    }
    match native.list().await {
        Ok(list) => {
            if list != baseline {
                cleanup.push("Final registry list metadata differs".into())
            }
        }
        Err(e) => cleanup.push(e),
    };
    native.records.push(json!({"phase":"complete","success":result.is_ok()&&cleanup.is_empty(),"error":result.as_ref().err(),"cleanupErrors":cleanup}));
    native.save()?;
    result?;
    require(cleanup.is_empty(), "Credential/config cleanup incomplete")
}

#[test]
fn fixture_metadata_restoration_refuses_foreign_changes() {
    let original =
        json!({"auths":{},"unknown":9007199254740993u64,"credHelpers":{"other.example":"other"}});
    let mut current = original.clone();
    current["credHelpers"]["127.0.0.1:12345"] = json!("orbwin");
    current["auths"]["127.0.0.1:12345"] = json!({});
    assert_eq!(
        without_fixture(current.clone(), &original, "127.0.0.1:12345", "orbwin").unwrap(),
        original
    );
    current["unknown"] = json!(9007199254740992u64);
    assert!(without_fixture(current, &original, "127.0.0.1:12345", "orbwin").is_err());
    let original = json!({"auths":{}});
    for (helper, auth) in [
        ("other", json!({})),
        ("orbwin", json!({"auth":"synthetic-unexpected"})),
    ] {
        let current =
            json!({"auths":{"127.0.0.1:12345":auth},"credHelpers":{"127.0.0.1:12345":helper}});
        assert!(without_fixture(current, &original, "127.0.0.1:12345", "orbwin").is_err());
    }
    let bytes = serde_json::to_vec_pretty(&original).unwrap();
    assert!(canonical(&bytes, "127.0.0.1:12345").is_ok());
    assert!(canonical(b"{\"auths\":{}}", "127.0.0.1:12345").is_err());
}
