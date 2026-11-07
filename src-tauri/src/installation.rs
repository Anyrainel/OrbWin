//! Product setup delegates ownership, classification, publication and lifetime
//! to the fixed Go role. The separate opted-in qualification path remains below.
#[cfg(test)]
use crate::process;
use crate::{
    engine,
    operations::{self, Outcome, Output},
    runtime,
};
use std::path::{Path, PathBuf};
use tauri::ipc::Channel;
use tokio::{process::Command, sync::oneshot};

#[cfg(test)]
#[path = "installation_qualification.rs"]
mod qualification;

#[path = "installation_preflight.rs"]
mod private_authority_preflight;

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
    preparation_mode().is_some()
}

pub(crate) fn upgrade_available() -> bool {
    preparation_mode() == Some(true)
}

// Keep ownership refusal visible even before a runtime identity exists. A fresh
// installed application must not describe somebody else's registration as a
// missing component, or suggest adopting/deleting that engine as recovery.
pub(crate) fn preparation_issue() -> Option<String> {
    let directory = match runtime::state_dir() {
        Ok(directory) => directory,
        Err(error) => return Some(error),
    };
    if runtime::identity_path().ok().as_deref() != Some(directory.join("runtime.json").as_path()) {
        return None;
    }
    registration(&directory.join("home"), "lima-orbwin").err()
}

fn preparation_mode() -> Option<bool> {
    let Ok(directory) = runtime::state_dir() else {
        return None;
    };
    // A development proof is not permission to prepare an installed home over
    // its registration. Installed setup uses only its own state directory.
    let identity = runtime::identity_path().ok()?;
    if identity.exists() && identity != directory.join("runtime.json") {
        return None;
    }
    if !runtime::backend_dir().join("rdd.exe").is_file()
        || registration(&directory.join("home"), "lima-orbwin").is_err()
    {
        return None;
    }
    if !identity.exists() {
        return Some(false);
    }
    let old: serde_json::Value = serde_json::from_slice(&std::fs::read(identity).ok()?).ok()?;
    let home = runtime::cli_path(&directory.join("home")).ok()?;
    validate_prior_identity(&old, &home, engine::ENDPOINT).ok()?;
    let manifest: serde_json::Value = serde_json::from_slice(
        &std::fs::read(runtime::backend_dir().join("build-manifest.json")).ok()?,
    )
    .ok()?;
    runtime::validate_backend_manifest(&manifest).ok()?;
    if old["sourceCommit"] != manifest["sourceCommit"] {
        return None;
    }
    (old["binarySha256"] != manifest["binarySha256"]).then_some(true)
}

fn validate_prior_identity(
    identity: &serde_json::Value,
    home: &Path,
    endpoint: &str,
) -> Result<(), String> {
    let digest = |key: &str, length| {
        identity[key]
            .as_str()
            .is_some_and(|s| s.len() == length && s.bytes().all(|b| b.is_ascii_hexdigit()))
    };
    let recorded_home = identity["home"]
        .as_str()
        .map(PathBuf::from)
        .and_then(|p| p.canonicalize().ok());
    if identity["identityVerified"] != true
        || identity["endpoint"] != endpoint
        || !identity["daemonId"]
            .as_str()
            .is_some_and(|s| !s.is_empty() && s.len() <= 256)
        || !digest("sourceCommit", 40)
        || !digest("binarySha256", 64)
        || recorded_home != Some(home.canonicalize().map_err(|e| e.to_string())?)
    {
        return Err("The existing engine identity does not match this owned home and endpoint. No service change was sent.".into());
    }
    Ok(())
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
    binary: &crate::runtime_cache::CachedExecutable,
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
    binary.recheck()?;
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
    decisions: &operations::SetupDecisionControl,
) -> Result<Outcome, String> {
    setup_product(output, cancelled, decisions).await
}

async fn wait_setup_choice(
    mut observer: crate::private_setup_client::SetupObserver,
    nonce: String,
    output: &Channel<Output>,
    cancelled: &mut oneshot::Receiver<()>,
    decisions: &operations::SetupDecisionControl,
    operation_deadline: std::time::Instant,
) -> Result<(crate::private_setup_client::SetupObserver, bool), String> {
    let mut response = decisions.pending(&nonce)?;
    let mut leaving = output
        .send(Output::SetupDecision {
            reason: "prior-app-absent",
            nonce,
        })
        .is_err();
    let choice_deadline = std::time::Instant::now() + std::time::Duration::from_secs(5 * 60);
    let pause_deadline;
    loop {
        if leaving {
            // The original write task is awaited even if its viewer disappears.
            // No shared registry mutex is held across this blocking pipe I/O.
            let work = tokio::task::spawn_blocking(move || {
                let result = observer.request_pause();
                (observer, result)
            });
            let (returned, result) = work.await.map_err(|_| {
                "Setup pause delivery did not settle; check status before retrying.".to_owned()
            })?;
            observer = returned;
            result?;
            pause_deadline = operation_deadline
                .min(std::time::Instant::now() + std::time::Duration::from_secs(75));
            break;
        }
        match observer.poll(operation_deadline)? {
            Some(crate::private_setup_client::ObservationData::SetupPaused) => {
                return Ok((observer, false))
            }
            Some(_) => return Err("Unexpected event while waiting for a setup decision.".into()),
            None => {}
        }
        tokio::select! {biased;
            _=&mut *cancelled=>leaving=true,
            _=tokio::time::sleep_until(tokio::time::Instant::from_std(choice_deadline.min(operation_deadline)))=>leaving=true,
            reply=response.response()=>{
                match reply {
                    Ok(nonce)=>{
                        // Once claimed, delivery may have effects. Never abort
                        // or kill on cancellation while this original write runs.
                        let work=tokio::task::spawn_blocking(move || {let result=observer.continue_original_setup(&nonce);(observer,result)});
                        let (returned,result)=work.await.map_err(|_|"Setup decision delivery did not settle; the engine may continue starting. Check status.".to_owned())?;
                        observer=returned;result?;return Ok((observer,true));
                    }
                    Err(_)=>leaving=true,
                }
            },
            _=tokio::time::sleep(std::time::Duration::from_millis(100))=>{},
        }
    }
    // Leave remains incomplete until the actual Go positive settlement event.
    // EOF/deadline/malformed output is unknown, not a successful pause.
    loop {
        match observer.poll(pause_deadline)? {
            Some(crate::private_setup_client::ObservationData::SetupPaused) => {
                return Ok((observer, false))
            }
            Some(_) => {
                return Err("Setup changed while pausing; check status before retrying.".into())
            }
            None => tokio::time::sleep(std::time::Duration::from_millis(100)).await,
        }
    }
}

#[derive(Debug, PartialEq, Eq)]
enum SetupDetach {
    Cancelled,
    Deadline,
}

// Once birth is dispatched, cancellation must settle the ORIGINAL task. Never
// abort it or drop its returned persistent observer before it actually returns.
async fn settle_setup_birth<T: Send + 'static>(
    mut birth: tokio::task::JoinHandle<Result<T, String>>,
    cancelled: &mut oneshot::Receiver<()>,
    deadline: std::time::Instant,
    stop: &std::sync::atomic::AtomicBool,
) -> Result<(T, Option<SetupDetach>), String> {
    let detached = if deadline <= std::time::Instant::now() {
        Some(SetupDetach::Deadline)
    } else {
        tokio::select! {biased;
            _=&mut *cancelled=>Some(SetupDetach::Cancelled),
            _=tokio::time::sleep_until(tokio::time::Instant::from_std(deadline))=>Some(SetupDetach::Deadline),
            result=&mut birth=>return result.map_err(|_|"Engine startup could not be confirmed. Its state is unknown. Check status and setup diagnostics before retrying.".to_owned())?.map(|value|(value,None)),
        }
    };
    stop.store(true, std::sync::atomic::Ordering::Release);
    let value=birth.await.map_err(|_|"Engine startup could not be confirmed after waiting stopped. Its state is unknown. Check status and setup diagnostics before retrying.".to_owned())??;
    Ok((value, detached))
}

async fn setup_product(
    output: &Channel<Output>,
    cancelled: &mut oneshot::Receiver<()>,
    decisions: &operations::SetupDecisionControl,
) -> Result<Outcome, String> {
    setup_admission_not_cancelled(cancelled)?;
    stage(
        output,
        "Verifying bundled components before requesting OrbWin setup…",
    )?;
    let deadline = std::time::Instant::now() + std::time::Duration::from_secs(20 * 60);
    // Only the existing executable cache/private host log are admitted here.
    // Go alone selects/creates the engine home, fence, service and runtime record.
    let stop = std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false));
    let preparing = stop.clone();
    let birth = tokio::spawn(async move {
        let (binary,log)=tokio::task::spawn_blocking(move || {
        let (binary, _manifest) = runtime::backend()?;
        if preparing.load(std::sync::atomic::Ordering::Acquire)
            || std::time::Instant::now() >= deadline
        {
            return Err("Setup waiting stopped while checking the bundled components. Check status before retrying.".into());
        }
        let log = crate::runtime_cache::create_private_setup_log()?;
        if preparing.load(std::sync::atomic::Ordering::Acquire)
            || std::time::Instant::now() >= deadline
        {
            return Err(
                "Setup waiting stopped while preparing the diagnostic log. Check status before retrying."
                    .into(),
            );
        }
        Ok::<_,String>((binary,log))
      }).await.map_err(|_|"Setup cache preparation worker unavailable.".to_owned())??;
        crate::private_setup_client::SetupObserver::start(binary, log).await
    });
    let (mut observer, detached) = settle_setup_birth(birth, cancelled, deadline, &stop).await?;
    if detached.is_some() {
        drop(observer); // EOF: pre-admission cancellation or post-admission DATA detach.
        return Err("Setup observation was cancelled or timed out while process creation settled. The backend may be running or recovering; inspect status before retrying. Data and diagnostic logs are retained.".into());
    }
    loop {
        // No mutex or blocking receive is held across the cancellation select.
        match observer.poll(deadline) {
            Ok(Some(crate::private_setup_client::ObservationData::StartupAdmitted)) => {
                if stage(
                    output,
                    "Starting the engine. Guest components may take several minutes to download…",
                )
                .is_err()
                {
                    drop(observer);
                    return Err("Setup viewer disconnected after dispatch. The backend may continue; inspect status and recovery before retrying. Data and logs are retained.".into());
                }
            }
            Ok(Some(crate::private_setup_client::ObservationData::SetupIncomplete { stage })) => {
                // DATA reports this observation attempt failed. Dropping the
                // observer closes input only; the original child/cache custody
                // remains with its sole actual-exit waiter. Never cancel/kill.
                drop(observer);
                return Err(stage.message().into());
            }
            Ok(Some(crate::private_setup_client::ObservationData::RuntimePublished)) => break,
            Ok(Some(crate::private_setup_client::ObservationData::DecisionRequired { nonce })) => {
                let (returned, continued) =
                    wait_setup_choice(observer, nonce, output, cancelled, decisions, deadline)
                        .await?;
                observer = returned;
                if !continued {
                    return Ok(setup_paused_outcome(output));
                }
                stage(output,"Continuing the original setup. Guest components may take several minutes to download…")?;
            }
            Ok(Some(crate::private_setup_client::ObservationData::SetupPaused)) => {
                return Ok(setup_paused_outcome(output));
            }
            Ok(None) => {}
            Err(_) => {
                drop(observer);
                return Err("Setup observation was lost or timed out. The backend may be running or recovering; inspect status before retrying. Data and logs are retained.".into());
            }
        }
        tokio::select! {biased;
            _=&mut *cancelled=>{drop(observer);return Err("Setup observation cancelled. The engine may continue starting; inspect status and recovery before retrying. Data and logs are retained.".into())},
            _=tokio::time::sleep(std::time::Duration::from_millis(100))=>{},
        }
    }
    drop(observer); // Publication is DATA; this closes observation, never the service.
    if stage(
        output,
        "Engine setup finished. Checking the desktop connection…",
    )
    .is_err()
    {
        return Err("The engine finished setup, but its window disconnected before the desktop connection check. Check engine status; data and logs were retained.".into());
    }
    let validation = async {
        let target = tokio::task::spawn_blocking(engine::target)
            .await
            .map_err(|_| "The desktop connection check did not finish.".to_owned())??;
        engine::check(&target).await
    };
    let checked = tokio::select! {biased;
        _=&mut *cancelled=>return Err("The engine finished setup; the desktop connection check was cancelled. Engine data and logs were retained. Check status before retrying.".into()),
        _=tokio::time::sleep_until(tokio::time::Instant::from_std(deadline))=>return Err("The engine finished setup; the desktop connection check timed out. Engine data and logs were retained. Check status before retrying.".into()),
        result=validation=>result,
    };
    checked.map_err(|_|"The engine finished setup, but OrbWin could not check the desktop connection. Engine data and logs were retained. Open diagnostics; if private Docker settings need recovery, use Settings > Registries > Private Docker settings, then check status again.".to_owned())?;
    Ok(Outcome {
        success: true,
        mutation: true,
        interrupted: false,
        exit_code: Some(0),
        message: "OrbWin is ready. Closing the app leaves the backend running.".into(),
    })
}

fn setup_paused_outcome(output: &Channel<Output>) -> Outcome {
    let _ = output.send(Output::SetupPaused);
    Outcome{success:false,mutation:true,interrupted:true,exit_code:Some(0),message:"Setup is paused. Existing files and logs were kept. Resume when you are ready, or open diagnostics.".into()}
}

pub(crate) async fn prepare_update(
    output: &Channel<Output>,
    cancelled: &mut oneshot::Receiver<()>,
) -> Result<Outcome, String> {
    let directory = runtime::state_dir()?;
    let home = runtime::cli_path(&directory.join("home"))?;
    let mut options = std::fs::OpenOptions::new();
    options.read(true).write(true).create(true).truncate(false);
    #[cfg(windows)]
    {
        use std::os::windows::fs::OpenOptionsExt;
        options.share_mode(0);
    }
    let _lock = options.open(directory.join("setup.lock"))
        .map_err(|_| "Another setup is active or its lock is inaccessible. Wait and retry before preparing an update.")?;
    let target = engine::target()?;
    let binary = target.rdd().await?;
    if target.home().canonicalize().map_err(|e| e.to_string())?
        != home.canonicalize().map_err(|e| e.to_string())?
    {
        return Err("Application update preparation requires the installed engine home. No service change was sent.".into());
    }
    match std::fs::OpenOptions::new()
        .read(true)
        .write(true)
        .open(r"\\.\pipe\docker-rancher-desktop-orbwin")
    {
        Ok(connection) => {
            drop(connection);
            engine::check(&target).await?;
        }
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
        Err(_) => {
            return Err(
                "The engine endpoint is busy or inaccessible. No service change was sent.".into(),
            )
        }
    }
    let location = SetupLocation {
        directory,
        instance: "orbwin",
        vm: "orbwin",
        endpoint: engine::ENDPOINT,
    };
    stage(output,"Stopping the owned backend for application file replacement. Docker and Kubernetes workloads are interrupted; persistent data is retained…")?;
    rdd(
        &binary,
        &location,
        &["service", "stop", "--timeout=60s"],
        None,
        output,
        cancelled,
        70,
    )
    .await?;
    // The content-addressed cache remains retained and is never replaced by
    // an installer. Test the packaged application resource being replaced.
    executable_released(&runtime::backend_dir().join("rdd.exe"))?;
    Ok(Outcome {success:true,mutation:true,interrupted:false,exit_code:Some(0),message:"OrbWin is ready for application replacement. Close OrbWin, then run the installer or uninstaller. Engine data and registration are retained.".into()})
}

fn executable_released(binary: &Path) -> Result<(), String> {
    let mut probe = std::fs::OpenOptions::new();
    probe.write(true);
    #[cfg(windows)]
    {
        use std::os::windows::fs::OpenOptionsExt;
        probe.share_mode(0);
    }
    drop(probe.open(binary).map_err(|_|"The backend stopped, but Windows still holds an application file open. Close active OrbWin clients and retry preparation before running the installer. Data is retained.")?);
    Ok(())
}

#[cfg(test)]
async fn setup_at(
    location: &SetupLocation,
    output: &Channel<Output>,
    cancelled: &mut oneshot::Receiver<()>,
) -> Result<Outcome, String> {
    // Exact existing qualification scope; no product fallback reaches this writer.
    if location.instance != "orbwin-proof"
        || location.vm != "orbwin-proof"
        || location.endpoint != "npipe:////./pipe/docker-rancher-desktop-orbwin-proof"
    {
        return Err(
            "Only the explicit isolated qualification instance can use this setup transaction."
                .into(),
        );
    }
    setup_admission_not_cancelled(cancelled)?;
    stage(
        output,
        "Verifying bundled components and Windows Subsystem for Linux…",
    )?;
    let (binary, manifest) = tokio::task::spawn_blocking(runtime::backend)
        .await
        .map_err(|e| e.to_string())??;
    let docker = runtime::docker()?;
    let credential_lease = runtime::credentials()?;
    let system = std::env::var_os("SystemRoot").unwrap_or_else(|| "C:\\Windows".into());
    process::read_command(
        &PathBuf::from(system).join("System32/wsl.exe"),
        &["--version"],
    )
    .await?;
    let directory = runtime::cli_path(&location.directory)?;
    let home = directory.join("home");
    let service_paths = crate::backend_paths::read_setup(&binary, &home, location.instance).await?;
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
    let registered = registration(&home, &distro)?;
    let identity = directory.join("runtime.json");
    let prior: Option<serde_json::Value> = if identity.exists() {
        let old = serde_json::from_slice(&std::fs::read(&identity).map_err(|e| e.to_string())?)
            .map_err(|e| e.to_string())?;
        validate_prior_identity(&old, &home, location.endpoint)?;
        if old["sourceCommit"] != manifest["sourceCommit"] {
            return Err("This upstream-version transition has not been qualified. No service change was sent; retain the existing engine and use a compatible OrbWin build.".into());
        }
        if !registered {
            return Err("The recorded engine registration is missing. Existing data was retained; inspect diagnostics before setup.".into());
        }
        Some(old)
    } else {
        None
    };
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
        if prior_home.as_ref() != Some(&home)
            || prior["instance"] != location.instance
            || prior["schema"] != 1
            || prior["product"] != "OrbWin"
        {
            return Err(
                "Setup intent does not match the owned home. No backend request was sent.".into(),
            );
        }
    } else {
        if prior.is_some() {
            return Err(
                "The existing engine has no matching setup intent. No service change was sent."
                    .into(),
            );
        }
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
    setup_admission_not_cancelled(cancelled)?;
    // Admit the actual private client before any service change. Do not create
    // a public master here: that would itself trigger legacy-import refusal.
    // Existing public data remains untouched and requires explicit review.
    let client_snapshot = {
        let docker = docker.clone();
        let credentials = credential_lease.clone();
        tokio::task::spawn_blocking(move || crate::client_config::reader(docker, credentials))
            .await
            .map_err(|_| "Setup private client admission did not complete.".to_string())?
            .map_err(|error| {
                if error.kind == crate::client_config::ErrorKind::RequiresLegacyReview {
                    "Existing OrbWin Docker settings require migration review before setup. Open Settings > Registries > Private Docker settings and choose Review previous OrbWin settings for import. The settings were retained and no service change was sent.".to_string()
                } else {
                    error.to_string()
                }
            })?
    };
    // The selected Windows candidate requires a private authority.init bound to
    // its saved arguments. Observe it BEFORE any Stop/Create/Start, including
    // Resume with a live old engine or an absent publication. This read neither
    // initializes legacy state nor claims that future startup cannot fail.
    // The separate fixed qualification instance retains its management-only
    // upstream path; it is not a product authority or legacy migration bypass.
    if location.instance == "orbwin" {
        stage(output, "Checking the bundled backend's management authority compatibility before any service change…")?;
        let observed = rdd(
            &binary,
            location,
            &["--instance", "orbwin", "service", "private-authority-preflight", "--json"],
            None,
            output,
            cancelled,
            15,
        )
        .await
        .map_err(|_| "The read-only authority compatibility check did not complete. No service change was sent; retain the existing engine and inspect setup diagnostics before resuming.".to_string())?;
        // Missing authority files cannot erase a recorded engine or an actual
        // owned WSL registration and authorize a fresh default configuration.
        private_authority_preflight::validate(
            &observed,
            service_paths.args_file.exists() || prior.is_some() || registered,
        )?;
    }
    if prior
        .as_ref()
        .is_some_and(|old| old["binarySha256"] != manifest["binarySha256"])
    {
        // A listening endpoint must still identify the recorded engine before
        // replacement. Only a genuinely absent pipe permits offline resume;
        // busy/denied/malformed/foreign endpoints fail closed.
        let pipe = format!(r"\\.\pipe\docker-rancher-desktop-{}", location.instance);
        match std::fs::OpenOptions::new().read(true).write(true).open(pipe) {
            Ok(connection) => {
                drop(connection);
                let actual = setup_engine_info(&docker, &credential_lease, &client_snapshot, location.endpoint, cancelled).await?;
                let old = prior.as_ref().ok_or("Missing previous engine identity")?;
                if actual["ID"].as_str() != Some(old["daemonId"].as_str().ok_or("Missing daemon ID")?) {
                    return Err("The live endpoint differs from the recorded Linux engine. No service change was sent.".into());
                }
            }
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => {},
            Err(e) => return Err(format!("Cannot verify the existing endpoint before backend replacement: {e}. No service change was sent.")),
        }
        registration(&home, &distro)?;
        stage(output, "Applying the bundled backend: stopping only the owned management service. Workloads are interrupted; disks and identity are retained…")?;
        rdd(
            &binary,
            location,
            &["service", "stop", "--timeout=60s"],
            None,
            output,
            cancelled,
            75,
        )
        .await?;
    }
    if !service_paths.args_file.exists() {
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
    let config = service_paths.config;
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
        // New installations select the qualified 1.35 release: upstream's 1.34
        // default exceeds the bundled kubectl 1.36 client's supported skew.
        // This creation-only choice never upgrades an existing App or cluster.
        let app=serde_json::to_vec(&serde_json::json!({"apiVersion":"app.rancherdesktop.io/v1alpha1","kind":"App","metadata":{"name":"app"},"spec":{"running":true,"namespace":"rancher-desktop","containerEngine":{"name":"moby"},"kubernetes":{"enabled":false,"version":"1.35.3"},"application":{"addPath":"manual"}}})).map_err(|e|e.to_string())?;
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
    let info = setup_engine_info(
        &docker,
        &credential_lease,
        &client_snapshot,
        location.endpoint,
        cancelled,
    )
    .await?;
    let id = info["ID"]
        .as_str()
        .filter(|id| !id.is_empty())
        .ok_or("The owned engine did not return a daemon ID")?;
    if info["OSType"] != "linux" {
        return Err(
            "The owned endpoint did not return a Linux engine; no identity was published.".into(),
        );
    }
    let current: Option<serde_json::Value> = if identity.exists() {
        Some(
            serde_json::from_slice(&std::fs::read(&identity).map_err(|e| e.to_string())?)
                .map_err(|e| e.to_string())?,
        )
    } else {
        None
    };
    if current != prior {
        return Err("The stored engine identity changed during setup. No identity was replaced; inspect current status before resuming.".into());
    }
    if let Some(old) = prior {
        if old["daemonId"] != id {
            return Err("The existing daemon ID changed; no identity was replaced.".into());
        }
    }
    // This request releases its actual generation reference before publication.
    // Other independent readers may still own it; no global-reader quiescence
    // is required or inferred. A last-owner native close fault is observable.
    client_snapshot.close().map_err(|_| "Setup read the engine, but private client cleanup could not be confirmed. Existing data was retained and no new identity was published. Close and reopen OrbWin before resuming.".to_string())?;
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

async fn setup_engine_info(
    docker: &crate::runtime_cache::CachedExecutable,
    credentials: &crate::runtime_cache::CachedExecutable,
    snapshot: &crate::client_config::ClientConfigSnapshot,
    endpoint: &str,
    cancelled: &mut oneshot::Receiver<()>,
) -> Result<serde_json::Value, String> {
    // Dropping the caller future signals cancellation to the detached atomic
    // worker. Its snapshot clone remains owned through positive native release,
    // including when this setup transaction has already returned cancellation.
    tokio::select! {
        biased;
        _ = cancelled => Err("Setup cancelled during the engine identity read. No identity was published; inspect status before resuming.".into()),
        info = engine::setup_info(docker.clone(), credentials.clone(), snapshot.clone(), endpoint) => info,
    }
}

fn setup_admission_not_cancelled(cancelled: &mut oneshot::Receiver<()>) -> Result<(), String> {
    if matches!(
        cancelled.try_recv(),
        Err(oneshot::error::TryRecvError::Empty)
    ) {
        Ok(())
    } else {
        Err("Setup cancelled before private client admission. Existing settings and engine data were retained; no service change was sent.".into())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn setup_birth_cancellation_awaits_original_result_before_detach() {
        let (cancel, mut cancelled) = oneshot::channel();
        let (release, ready) = oneshot::channel();
        let (started, seen) = oneshot::channel();
        let birth = tokio::spawn(async move {
            started.send(()).unwrap();
            ready.await.unwrap();
            Ok::<_, String>("original birth result")
        });
        seen.await.unwrap();
        cancel.send(()).unwrap();
        let stopped = std::sync::Arc::new(std::sync::atomic::AtomicBool::new(false));
        let stop = stopped.clone();
        let settlement = tokio::spawn(async move {
            settle_setup_birth(
                birth,
                &mut cancelled,
                std::time::Instant::now() + std::time::Duration::from_secs(10),
                &stop,
            )
            .await
        });
        tokio::time::timeout(std::time::Duration::from_secs(1), async {
            while !stopped.load(std::sync::atomic::Ordering::Acquire) {
                tokio::task::yield_now().await
            }
        })
        .await
        .unwrap();
        assert!(!settlement.is_finished());
        release.send(()).unwrap();
        assert_eq!(
            settlement.await.unwrap().unwrap(),
            ("original birth result", Some(SetupDetach::Cancelled))
        );
    }
    #[tokio::test]
    async fn setup_birth_deadline_still_settles_and_panic_is_unknown() {
        let (_cancel, mut cancelled) = oneshot::channel();
        let birth = tokio::spawn(async { Ok::<_, String>(17) });
        assert_eq!(
            settle_setup_birth(
                birth,
                &mut cancelled,
                std::time::Instant::now(),
                &std::sync::atomic::AtomicBool::new(false)
            )
            .await
            .unwrap(),
            (17, Some(SetupDetach::Deadline))
        );
        let (_cancel, mut cancelled) = oneshot::channel();
        let birth = tokio::spawn(async {
            panic!("inert birth panic");
            #[allow(unreachable_code)]
            Ok::<(), String>(())
        });
        assert!(settle_setup_birth(
            birth,
            &mut cancelled,
            std::time::Instant::now() + std::time::Duration::from_secs(10),
            &std::sync::atomic::AtomicBool::new(false)
        )
        .await
        .unwrap_err()
        .contains("unknown"));
    }

    #[test]
    fn setup_private_admission_refuses_pending_or_disconnected_cancellation() {
        let (sender, mut receiver) = oneshot::channel();
        assert!(setup_admission_not_cancelled(&mut receiver).is_ok());
        sender.send(()).unwrap();
        assert!(setup_admission_not_cancelled(&mut receiver).is_err());
        assert!(setup_admission_not_cancelled(&mut receiver).is_err());
        let (sender, mut receiver) = oneshot::channel::<()>();
        drop(sender);
        assert!(setup_admission_not_cancelled(&mut receiver).is_err());
    }

    #[cfg(windows)]
    #[test]
    fn replacement_probe_rejects_mapped_images_and_never_changes_file_contents() {
        assert!(executable_released(&std::env::current_exe().unwrap()).is_err());
        let path = std::env::temp_dir().join(format!(
            "orbwin-replacement-probe-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        std::fs::write(&path, b"retained bytes").unwrap();
        executable_released(&path).unwrap();
        assert_eq!(std::fs::read(&path).unwrap(), b"retained bytes");
        std::fs::remove_file(path).unwrap();
    }

    #[test]
    fn backend_replacement_requires_the_recorded_home_endpoint_and_identity() {
        let stamp = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        let home =
            std::env::temp_dir().join(format!("orbwin-upgrade-{}-{stamp}", std::process::id()));
        std::fs::create_dir(&home).unwrap();
        let valid = serde_json::json!({"home":home,"endpoint":engine::ENDPOINT,"identityVerified":true,"daemonId":"original-engine","sourceCommit":"a".repeat(40),"binarySha256":"b".repeat(64)});
        validate_prior_identity(&valid, &home, engine::ENDPOINT).unwrap();
        for (key, value) in [
            ("home", serde_json::json!(home.join("foreign"))),
            (
                "endpoint",
                serde_json::json!("npipe:////./pipe/dockerDesktopLinuxEngine"),
            ),
            ("identityVerified", serde_json::json!(false)),
            ("daemonId", serde_json::json!("")),
            ("sourceCommit", serde_json::json!("unverified")),
            ("binarySha256", serde_json::json!("not-a-digest")),
        ] {
            let mut invalid = valid.clone();
            invalid[key] = value;
            assert!(
                validate_prior_identity(&invalid, &home, engine::ENDPOINT).is_err(),
                "{key}"
            );
        }
        std::fs::remove_dir(&home).unwrap();
    }
}
