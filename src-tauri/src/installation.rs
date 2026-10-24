//! Explicit first-run CLI transaction. rdd is the sole guest orchestrator.
//! Intent precedes startup; identity follows ownership and actual readiness.
//! Cancellation retains data and may leave rdd converging: Resume is idempotent.
use crate::{
    engine,
    operations::{self, Outcome, Output},
    process, runtime,
};
use std::path::{Path, PathBuf};
use tauri::ipc::Channel;
use tokio::{process::Command, sync::oneshot};

#[cfg(test)]
#[path = "installation_qualification.rs"]
mod qualification;

#[cfg(windows)]
pub(crate) fn registration(home: &Path, name: &str) -> Result<bool, String> {
    use winreg::{enums::HKEY_CURRENT_USER, RegKey};
    let registry = match RegKey::predef(HKEY_CURRENT_USER)
        .open_subkey("Software\\Microsoft\\Windows\\CurrentVersion\\Lxss")
    {
        Ok(key) => key,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Ok(false),
        Err(e) => return Err(e.to_string()),
    };
    let mut found = false;
    for key in registry.enum_keys() {
        let key = registry
            .open_subkey(key.map_err(|e| e.to_string())?)
            .map_err(|e| e.to_string())?;
        let distribution: String = key
            .get_value("DistributionName")
            .map_err(|e| e.to_string())?;
        if !distribution.eq_ignore_ascii_case(name) {
            continue;
        }
        if found {
            return Err("Duplicate engine registration; no operation was sent.".into());
        }
        found = true;
        let base: String = key.get_value("BasePath").map_err(|e| e.to_string())?;
        let base = PathBuf::from(base)
            .canonicalize()
            .map_err(|e| e.to_string())?;
        let home=home.canonicalize().map_err(|_|"An existing OrbWin registration belongs to a different home. No operation was sent.".to_string())?;
        if base == home || !base.starts_with(home) {
            return Err("An existing OrbWin registration belongs to a different home. No operation was sent.".into());
        }
    }
    Ok(found)
}
#[cfg(not(windows))]
pub(crate) fn registration(_: &Path, _: &str) -> Result<bool, String> {
    Err("Setup requires Windows.".into())
}

pub(crate) fn available() -> bool {
    let Ok(directory) = runtime::state_dir() else {
        return false;
    };
    !runtime::identity_path().is_ok_and(|p| p.exists())
        && runtime::backend_dir().join("rdd.exe").is_file()
        && registration(&directory.join("home"), "lima-orbwin").is_ok()
}

fn stage(output: &Channel<Output>, message: &str) -> Result<(), String> {
    output
        .send(Output::Data {
            stream: "stdout",
            bytes: format!("{message}\n").into_bytes(),
        })
        .map_err(|_| "Setup viewer disconnected; inspect status before resuming.".into())
}

async fn rdd(
    binary: &Path,
    location: &SetupLocation,
    args: &[&str],
    input: Option<&[u8]>,
    output: &Channel<Output>,
    cancelled: &mut oneshot::Receiver<()>,
    seconds: u64,
) -> Result<String, String> {
    if cancelled.try_recv().is_ok() {
        return Err("Setup cancelled before dispatch. Backend work may still be finishing from an earlier step.".into());
    }
    let home = runtime::cli_path(&location.directory.join("home"))?;
    let short = home.join(format!(".rd{}", location.instance));
    let mut command = Command::new(binary);
    command
        .args(args)
        .env("USERPROFILE", &home)
        .env("HOME", &home)
        .env("RDD_INSTANCE", location.instance)
        .env("RDD_DEVELOPER_MODE", "1")
        .env("RDD_KEEP_LOGS", "1")
        .env_remove("RDD_LOG_DIR")
        .env_remove("LIMA_HOME")
        .env("DOCKER_CONFIG", short.join("docker"))
        .env("KUBECONFIG", short.join("kube-client.config"))
        .env("DOCKER_HOST", location.endpoint)
        .env_remove("DOCKER_CONTEXT")
        .env_remove("DOCKER_TLS_VERIFY")
        .env_remove("DOCKER_CERT_PATH");
    let text = std::sync::Arc::new(std::sync::Mutex::new(String::new()));
    let received = text.clone();
    let viewer = output.clone();
    let channel = Channel::new(move |body| {
        let event: serde_json::Value = body.deserialize()?;
        if event["kind"] == "data" {
            let bytes: Vec<u8> = serde_json::from_value(event["bytes"].clone())?;
            let stream = if event["stream"] == "stdout" {
                "stdout"
            } else {
                "stderr"
            };
            if stream == "stdout" {
                let mut text = received.lock().unwrap();
                if text.len() + bytes.len() <= 128 * 1024 {
                    text.push_str(&String::from_utf8_lossy(&bytes));
                }
            }
            viewer.send(Output::Data { stream, bytes })?;
        } else if event["kind"] == "truncated" {
            viewer.send(Output::Truncated)?;
        }
        Ok(())
    });
    let result = operations::client(command, input, &channel, cancelled, seconds).await?;
    if result.interrupted || result.exit_code != Some(0) {
        return Err(format!("Setup client stopped: {} (exit {:?}). The backend may still be applying changes. Data and logs are retained; inspect status and Resume setup.",result.reason,result.exit_code));
    }
    let value = text.lock().map_err(|_| "Setup output unavailable")?.clone();
    Ok(value)
}

struct SetupLocation {
    directory: PathBuf,
    instance: &'static str,
    vm: &'static str,
    endpoint: &'static str,
}
pub(crate) async fn setup(
    output: &Channel<Output>,
    cancelled: &mut oneshot::Receiver<()>,
) -> Result<Outcome, String> {
    let location = SetupLocation {
        directory: runtime::state_dir()?,
        instance: "orbwin",
        vm: "orbwin",
        endpoint: engine::ENDPOINT,
    };
    setup_at(&location, output, cancelled).await
}

async fn setup_at(
    location: &SetupLocation,
    output: &Channel<Output>,
    cancelled: &mut oneshot::Receiver<()>,
) -> Result<Outcome, String> {
    stage(
        output,
        "Verifying bundled components and Windows Subsystem for Linux…",
    )?;
    let (binary, manifest) = tokio::task::spawn_blocking(runtime::backend)
        .await
        .map_err(|e| e.to_string())??;
    let docker = runtime::docker()?;
    let system = std::env::var_os("SystemRoot").unwrap_or_else(|| "C:\\Windows".into());
    process::read_command(
        &PathBuf::from(system).join("System32/wsl.exe"),
        &["--version"],
    )
    .await?;
    let directory = runtime::cli_path(&location.directory)?;
    let home = directory.join("home");
    let short = home.join(format!(".rd{}", location.instance));
    let service_dir = home.join(format!(
        "AppData/Local/rancher-desktop-{}",
        location.instance
    ));
    let distro = format!("lima-{}", location.vm);
    std::fs::create_dir_all(&directory).map_err(|e| e.to_string())?;
    #[cfg(windows)]
    use std::os::windows::fs::OpenOptionsExt;
    let mut lock_options = std::fs::OpenOptions::new();
    lock_options.read(true).write(true).create(true);
    #[cfg(windows)]
    lock_options.share_mode(0);
    let _setup_lock = lock_options
        .open(directory.join("setup.lock"))
        .map_err(|e| {
            format!("Another setup may be active, or the setup lock is inaccessible: {e}")
        })?;
    registration(&home, &distro)?;
    let intent = directory.join("setup-intent.json");
    if intent.exists() {
        let prior: serde_json::Value =
            serde_json::from_slice(&std::fs::read(&intent).map_err(|e| e.to_string())?)
                .map_err(|e| e.to_string())?;
        let prior_home = prior["home"]
            .as_str()
            .map(PathBuf::from)
            .map(|p| runtime::cli_path(&p))
            .transpose()?;
        if prior_home.as_ref() != Some(&home) || prior["instance"] != location.instance {
            return Err(
                "Setup intent does not match the owned home. No backend request was sent.".into(),
            );
        }
    } else {
        if home.exists()
            && std::fs::read_dir(&home)
                .map_err(|e| e.to_string())?
                .next()
                .is_some()
        {
            return Err("The proposed home already contains data without an OrbWin setup intent. It was left untouched.".into());
        }
        runtime::write_json(
            &intent,
            &serde_json::json!({"schema":1,"product":"OrbWin","instance":location.instance,"home":home}),
        )?;
    }
    std::fs::create_dir_all(&home).map_err(|e| e.to_string())?;
    runtime::configure_client(&short.join("docker"))?;
    if !service_dir.join("args.json").exists() {
        stage(output, "Creating the private rdd service configuration…")?;
        rdd(
            &binary,
            location,
            &["service", "create", "--controllers=*,-path-management"],
            None,
            output,
            cancelled,
            60,
        )
        .await?;
    }
    stage(output, "Starting the OrbWin management service…")?;
    rdd(
        &binary,
        location,
        &[
            "service",
            "start",
            "--controllers=*,-path-management",
            "--timeout=90s",
        ],
        None,
        output,
        cancelled,
        110,
    )
    .await?;
    // The management service is now ready. Read it explicitly without autostart.
    let config = service_dir.join("config.yaml");
    let existing = rdd(
        &binary,
        location,
        &[
            "kubectl",
            "--kubeconfig",
            config.to_str().ok_or("Invalid config path")?,
            "get",
            "apps/app",
            "--ignore-not-found",
            "--output=name",
            "--request-timeout=10s",
        ],
        None,
        output,
        cancelled,
        30,
    )
    .await?;
    if existing.trim().is_empty() {
        stage(
            output,
            "Provisioning the owned WSL guest. rdd may download and install guest components…",
        )?;
        let app=serde_json::to_vec(&serde_json::json!({"apiVersion":"app.rancherdesktop.io/v1alpha1","kind":"App","metadata":{"name":"app"},"spec":{"running":true,"namespace":"rancher-desktop","containerEngine":{"name":"moby"},"kubernetes":{"enabled":false},"application":{"addPath":"manual"}}})).map_err(|e|e.to_string())?;
        rdd(
            &binary,
            location,
            &["ctl", "create", "-f", "-"],
            Some(&app),
            output,
            cancelled,
            60,
        )
        .await?;
    } else {
        stage(output, "Resuming the owned backend configuration…")?;
        rdd(
            &binary,
            location,
            &["set", "running=true", "--timeout=840s"],
            None,
            output,
            cancelled,
            900,
        )
        .await?;
    }
    stage(
        output,
        "Waiting for guest provisioning and the Docker engine…",
    )?;
    rdd(
        &binary,
        location,
        &[
            "ctl",
            "wait",
            "apps/app",
            "--for=condition=ContainerEngineReady=True",
            "--timeout=840s",
        ],
        None,
        output,
        cancelled,
        900,
    )
    .await?;
    rdd(
        &binary,
        location,
        &[
            "ctl",
            "wait-condition",
            &format!("limavms/{}", location.vm),
            "Running=True",
            "--reason=Started",
            "--since=startup",
            "--namespace=rancher-desktop",
            "--timeout=180s",
        ],
        None,
        output,
        cancelled,
        200,
    )
    .await?;
    if !registration(&home, &distro)? {
        return Err(
            "Provisioning did not create the expected owned guest. Logs were retained.".into(),
        );
    }
    let client_config = short.join("docker");
    let info = process::read_command(
        &docker,
        &[
            "--config",
            client_config.to_str().ok_or("Invalid client config")?,
            "--host",
            location.endpoint,
            "info",
            "--format",
            "{{json .}}",
        ],
    )
    .await?;
    let info: serde_json::Value = serde_json::from_str(&info).map_err(|e| e.to_string())?;
    let id = info["ID"]
        .as_str()
        .filter(|id| !id.is_empty())
        .ok_or("The owned engine did not return a daemon ID")?;
    if info["OSType"] != "linux" {
        return Err(
            "The owned endpoint did not return a Linux engine; no identity was published.".into(),
        );
    }
    let identity = directory.join("runtime.json");
    if identity.exists() {
        let old: serde_json::Value =
            serde_json::from_slice(&std::fs::read(&identity).map_err(|e| e.to_string())?)
                .map_err(|e| e.to_string())?;
        if old["daemonId"] != id {
            return Err("The existing daemon ID changed; no identity was replaced.".into());
        }
    }
    let record = serde_json::json!({"endpoint":location.endpoint,"home":home,"daemonId":id,"sourceCommit":manifest["sourceCommit"],"binarySha256":manifest["binarySha256"],"identityVerified":true});
    runtime::write_json(&identity, &record)?;
    stage(
        output,
        "Setup complete. The owned guest, endpoint and daemon identity were verified.",
    )?;
    Ok(Outcome {
        success: true,
        mutation: true,
        interrupted: false,
        exit_code: Some(0),
        message: "OrbWin is ready. Closing the app leaves the backend running.".into(),
    })
}
