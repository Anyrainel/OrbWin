//! Thin launch/IPC adapter. The Go process owns naming state and survives the UI.
use crate::{engine, runtime};
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use std::{path::Path, process::Stdio, time::Duration};
use tokio::io::{AsyncReadExt, AsyncWriteExt};
#[path = "local_services_lifetime.rs"]
mod lifetime;
use lifetime::{LaunchOwner, Retirement, StartupMode};

static START_PENDING: std::sync::atomic::AtomicBool = std::sync::atomic::AtomicBool::new(false);
// Same fixed admission used by every existing RPC owner/worker. Test-only host
// retention can wait for these actual permits without inspecting native actors.
static RPC_SLOTS: tokio::sync::Semaphore = tokio::sync::Semaphore::const_new(16);

fn startup_failure(code: Option<i32>) -> &'static str {
    match code {
        Some(124) => "Local services require an explicit migration of their legacy saved state before starting. No new authority was created.",
        Some(123) => "Local services found an incomplete state transaction. Explicit recovery is required before starting; do not reset or retry the transaction.",
        Some(122) => "Local services could not verify the ownership or integrity of their private saved state. Inspect the installation before starting again.",
        _ => "Local service process could not start. Inspect its saved state before starting again.",
    }
}

// An uncertain detached start is never killed or replayed. Its exact child and
// launch lease move to a sole native waiter, independent of the invoking future.
struct ServeChild {
    child: std::process::Child,
    #[cfg(test)]
    terminal: Option<tokio::sync::oneshot::Sender<ServeTerminal>>,
}
type ServeLaunch = LaunchOwner<ServeChild, crate::runtime_cache::CachedExecutable, fn()>;
type ServeRetirement = Retirement<ServeChild, crate::runtime_cache::CachedExecutable>;
#[cfg(test)]
struct ServeTerminal {
    code: Option<i32>,
    success: bool,
}
fn retain_uncertain_start(item: ServeRetirement) {
    use std::sync::{mpsc, OnceLock};
    static WAITER: OnceLock<Result<mpsc::SyncSender<ServeRetirement>, ()>> = OnceLock::new();
    let waiter = WAITER.get_or_init(|| {
        let (sender, receiver) = mpsc::sync_channel::<ServeRetirement>(1);
        std::thread::Builder::new()
            .name("orbwin-service-retirement".into())
            .spawn(move || {
                while let Ok(mut item) = receiver.recv() {
                    if let Ok(status) = item.child.child.wait() {
                        #[cfg(test)]
                        let terminal = item.child.terminal.take();
                        drop(item.lease);
                        START_PENDING.store(false, std::sync::atomic::Ordering::Release);
                        #[cfg(test)]
                        if let Some(terminal) = terminal {
                            // Notification comes only from this original wait.
                            // It proves no family/native-raw-close property.
                            let _ = terminal.send(ServeTerminal {
                                code: status.code(),
                                success: status.success(),
                            });
                        }
                        #[cfg(not(test))]
                        let _ = status;
                    } else {
                        item.retain_unknown();
                        return;
                    }
                }
            })
            .map(|_| sender)
            .map_err(|_| ())
    });
    match waiter {
        Ok(sender) => {
            if let Err(error) = sender.try_send(item) {
                match error {
                    mpsc::TrySendError::Full(item) | mpsc::TrySendError::Disconnected(item) => {
                        item.retain_unknown()
                    }
                }
            }
        }
        Err(()) => item.retain_unknown(),
    }
}
impl ServeLaunch {
    fn reserve(lease: crate::runtime_cache::CachedExecutable) -> Result<Self, String> {
        START_PENDING.compare_exchange(false, true, std::sync::atomic::Ordering::AcqRel,
            std::sync::atomic::Ordering::Acquire).map_err(|_| "An earlier service start is still unconfirmed. Inspect its state before another start.")?;
        Ok(Self {
            child: None,
            lease: Some(lease),
            retire: |child, lease| retain_uncertain_start(ServeRetirement { child, lease }),
            release: || START_PENDING.store(false, std::sync::atomic::Ordering::Release),
        })
    }
}
#[derive(Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct WslTrustReview {
    distribution_id: String,
    #[serde(default)]
    machine_id: String,
    #[serde(default)]
    guest_sequence: u64,
    #[serde(default)]
    owner_sequence: u64,
}

#[derive(Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct LanSelection {
    container_id: String,
    started_at: String,
    host_port: u16,
    container_port: u16,
    protocol: LanProtocol,
    interface_guid: String,
    interface_luid: String,
    local_address: String,
    profile: LanProfile,
    client_networks: Vec<String>,
}

#[derive(Deserialize, Serialize)]
#[serde(rename_all = "lowercase")]
pub(crate) enum LanProtocol {
    Tcp,
    Udp,
}

#[derive(Deserialize, Serialize)]
#[serde(rename_all = "lowercase")]
pub(crate) enum LanProfile {
    Private,
    Domain,
}

#[derive(Deserialize, Serialize)]
#[serde(tag = "operation", rename_all = "kebab-case", deny_unknown_fields)]
pub(crate) enum Request {
    Snapshot,
    Preview,
    Retry,
    LanOptions {},
    LanReview {
        revision: u64,
        #[serde(rename = "lanSelection")]
        selection: LanSelection,
    },
    LanActivate {
        revision: u64,
        #[serde(rename = "lanReviewId")]
        review_id: String,
        #[serde(rename = "lanReviewSha256")]
        review_sha256: String,
    },
    LanRemove {
        revision: u64,
        #[serde(rename = "lanReviewId")]
        review_id: String,
        #[serde(rename = "lanReviewSha256")]
        review_sha256: String,
    },
    LanSettleRemove {
        revision: u64,
        #[serde(rename = "lanReviewId")]
        review_id: String,
        #[serde(rename = "lanReviewSha256")]
        review_sha256: String,
    },
    LanCleanup {
        revision: u64,
        #[serde(rename = "lanReviewId")]
        review_id: String,
        #[serde(rename = "lanReviewSha256")]
        review_sha256: String,
    },
    LanCleanupReview {
        #[serde(rename = "lanReviewId")]
        review_id: String,
        #[serde(rename = "lanReviewSha256")]
        review_sha256: String,
    },
    LanCancel {
        #[serde(rename = "lanOperationId")]
        operation_id: String,
    },
    LanReconcileReview {
        #[serde(rename = "lanReviewId")]
        review_id: String,
        #[serde(rename = "lanReviewSha256")]
        review_sha256: String,
    },
    LanSettleHelper {
        #[serde(rename = "lanOperationId")]
        operation_id: String,
    },
    LanRemoveReview {
        #[serde(rename = "lanLeaseSha256")]
        lease_sha256: String,
    },
    LanSettleRemoveReview {
        #[serde(rename = "lanLeaseSha256")]
        lease_sha256: String,
    },
    LanInspect {
        #[serde(rename = "lanLeaseSha256")]
        lease_sha256: String,
    },
    WslTrustDistributions,
    WslTrustReceipts,
    RetireWslTrustReceipt {
        revision: u64,
        #[serde(rename = "rootSha256")]
        root_sha256: String,
        #[serde(rename = "wslReview")]
        wsl_review: WslTrustReview,
    },
    WslTrustStatus {
        revision: u64,
        #[serde(rename = "rootSha256")]
        root_sha256: String,
        #[serde(rename = "wslReview")]
        wsl_review: WslTrustReview,
    },
    WslTrustInstall {
        revision: u64,
        #[serde(rename = "rootSha256")]
        root_sha256: String,
        #[serde(rename = "wslReview")]
        wsl_review: WslTrustReview,
    },
    WslTrustRemove {
        revision: u64,
        #[serde(rename = "rootSha256")]
        root_sha256: String,
        #[serde(rename = "wslReview")]
        wsl_review: WslTrustReview,
    },
    Configure {
        revision: u64,
        settings: Value,
    },
    CreateAuthority {
        revision: u64,
    },
    AdoptAuthority {
        revision: u64,
        #[serde(rename = "rootSha256")]
        root_sha256: String,
    },
    TrustStatus {
        revision: u64,
        #[serde(rename = "rootSha256")]
        root_sha256: String,
    },
    TrustInstall {
        revision: u64,
        #[serde(rename = "rootSha256")]
        root_sha256: String,
    },
    TrustRemove {
        revision: u64,
        #[serde(rename = "rootSha256")]
        root_sha256: String,
    },
    RotateAuthority {
        revision: u64,
        #[serde(rename = "rootSha256")]
        root_sha256: String,
        ports: Value,
    },
    ResumeAuthorityRotation {
        revision: u64,
        #[serde(rename = "rootSha256")]
        root_sha256: String,
        #[serde(rename = "newRootSha256")]
        new_root_sha256: String,
    },
    CancelAuthorityRotation {
        revision: u64,
        #[serde(rename = "rootSha256")]
        root_sha256: String,
        #[serde(rename = "newRootSha256")]
        new_root_sha256: String,
    },
}

fn services_asset() -> Result<(std::path::PathBuf, String), String> {
    use sha2::{Digest, Sha256};
    let manifest: Value = serde_json::from_slice(
        &std::fs::read(runtime::backend_dir().join("services-manifest.json"))
            .map_err(|e| e.to_string())?,
    )
    .map_err(|e| e.to_string())?;
    if manifest["schema"] != 1 || manifest["abi"] != 5 {
        return Err("Local service helper ABI is unsupported.".into());
    }
    for (name, source) in [
        (
            "service-tools/main_linux.go",
            include_bytes!("../../backend-tools/session/service-tools/main_linux.go").as_slice(),
        ),
        (
            "service-tools/main_other.go",
            include_bytes!("../../backend-tools/session/service-tools/main_other.go").as_slice(),
        ),
        (
            "service-tools/lan_udp_linux.go",
            include_bytes!("../../backend-tools/session/service-tools/lan_udp_linux.go").as_slice(),
        ),
        (
            "internal/landatagram/codec.go",
            include_bytes!("../../backend-tools/session/internal/landatagram/codec.go").as_slice(),
        ),
        (
            "internal/landatagram/session.go",
            include_bytes!("../../backend-tools/session/internal/landatagram/session.go")
                .as_slice(),
        ),
        (
            "internal/gateway/tls.go",
            include_bytes!("../../backend-tools/session/internal/gateway/tls.go").as_slice(),
        ),
        (
            "internal/gateway/copy.go",
            include_bytes!("../../backend-tools/session/internal/gateway/copy.go").as_slice(),
        ),
        (
            "internal/gateway/mux.go",
            include_bytes!("../../backend-tools/session/internal/gateway/mux.go").as_slice(),
        ),
        (
            "internal/streamcontrol/protocol.go",
            include_bytes!("../../backend-tools/session/internal/streamcontrol/protocol.go")
                .as_slice(),
        ),
        (
            "internal/streamcontrol/server.go",
            include_bytes!("../../backend-tools/session/internal/streamcontrol/server.go")
                .as_slice(),
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
        if manifest["sources"][name] != format!("{:x}", Sha256::digest(source)) {
            return Err("Local service sources changed; rebuild the bundled helper.".into());
        }
    }
    let path = runtime::backend_dir().join("orbwin-services");
    let hash = runtime::hash_file(&path, 16 << 20)?;
    runtime::require_component_pin("orbwin-services", &hash)?;
    if manifest["binarySha256"] != hash {
        return Err("Local service helper checksum differs.".into());
    }
    Ok((runtime::cli_path(&path)?, hash))
}

fn spawn_owned_client<O, F, T>(owner: O, work: F) -> tokio::task::JoinHandle<T>
where
    O: Send + 'static,
    F: std::future::Future<Output = T> + Send + 'static,
    T: Send + 'static,
{
    tokio::spawn(async move {
        let result = work.await;
        drop(owner);
        result
    })
}

async fn rpc(
    binary: &crate::runtime_cache::CachedExecutable,
    home: &Path,
    installation: &str,
    request: &Value,
) -> Result<(i32, Value), String> {
    // A dropped renderer future must not drop the executable lease while the
    // transient client still runs. The sole worker retains child+lease+slot
    // until positive Wait; a caller deadline does not cancel that worker.
    let permit = RPC_SLOTS
        .try_acquire()
        .map_err(|_| "Local service clients are still finishing; wait before retrying.")?;
    let deadline = rpc_deadline(request);
    let (binary, home, installation, request) = (
        binary.clone(),
        home.to_path_buf(),
        installation.to_owned(),
        request.clone(),
    );
    let active_binary = binary.clone();
    let worker = spawn_owned_client((permit, binary), async move {
        rpc_owned(&active_binary, &home, &installation, &request).await
    });
    tokio::time::timeout(Duration::from_secs(deadline + 5), worker).await
        .map_err(|_| "Local service client retirement is still pending. Reload its state before retrying a change.".to_string())?
        .map_err(|_| "Local service client outcome is unknown. Reload its state before retrying a change.".to_string())?
}

fn rpc_deadline(request: &Value) -> u64 {
    match request["operation"].as_str() {
        Some("wsl-trust-status" | "wsl-trust-install" | "wsl-trust-remove") => 155,
        _ => 45,
    }
}

async fn retire_rpc(child: &mut tokio::process::Child) {
    let _ = child.start_kill();
    if child.wait().await.is_err() {
        // This worker and its admission/origin lease remain owned as unknown.
        std::future::pending::<()>().await;
    }
}

async fn rpc_owned(
    binary: &crate::runtime_cache::CachedExecutable,
    home: &Path,
    installation: &str,
    request: &Value,
) -> Result<(i32, Value), String> {
    binary.recheck()?;
    let data = serde_json::to_vec(request).map_err(|e| e.to_string())?;
    if !lifetime::request_size_allowed(data.len()) {
        return Err("Local service settings exceed their size limit.".into());
    }
    let mut command = tokio::process::Command::new(binary);
    command
        .arg("local-services-call")
        .arg(home)
        .arg(installation)
        .arg("-")
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .kill_on_drop(true);
    #[cfg(windows)]
    command.creation_flags(0x08000000);
    let mut child = command.spawn().map_err(|e| e.to_string())?;
    let (Some(mut input), Some(mut output), Some(mut errors)) =
        (child.stdin.take(), child.stdout.take(), child.stderr.take())
    else {
        retire_rpc(&mut child).await;
        return Err("Local service client pipes are unavailable.".into());
    };
    let result = tokio::time::timeout(Duration::from_secs(rpc_deadline(request)), async {
        let write = async {
            input.write_all(&data).await.map_err(|e| e.to_string())?;
            input.shutdown().await.map_err(|e| e.to_string())
        };
        let read = async {
            let mut bytes = Vec::new();
            (&mut output)
                .take((1 << 20) + 1)
                .read_to_end(&mut bytes)
                .await
                .map_err(|e| e.to_string())?;
            Ok::<_, String>(bytes)
        };
        let diagnostics = async {
            let mut bytes = Vec::new();
            (&mut errors)
                .take(16385)
                .read_to_end(&mut bytes)
                .await
                .map_err(|e| e.to_string())?;
            Ok::<_, String>(bytes)
        };
        let (written, bytes, stderr, status) = tokio::join!(write, read, diagnostics, child.wait());
        written?;
        let bytes = bytes?;
        let stderr = stderr?;
        let status = status.map_err(|e| e.to_string())?;
        if bytes.len() > 1 << 20 || stderr.len() > 16384 {
            return Err("Local service client exceeded its output limit.".into());
        }
        let code = status.code().unwrap_or(-1);
        if code == 126 {
            return Ok((code, Value::Null));
        }
        if !status.success() {
            return Err(String::from_utf8_lossy(&stderr).trim().to_string());
        }
        let response: Value = serde_json::from_slice(&bytes).map_err(|e| e.to_string())?;
        if response["version"] != 1 {
            return Err("Local service response version is unsupported.".into());
        }
        Ok((code, response))
    })
    .await;
    let error = match result {
        Ok(Ok(value)) => return Ok(value),
        Ok(Err(error)) => error,
        Err(_) => "Local service request timed out. Reload its state before retrying a change."
            .to_string(),
    };
    // A kill request is not retirement. This worker intentionally keeps its
    // lease/admission if native Wait cannot settle within the caller deadline.
    retire_rpc(&mut child).await;
    Err(error)
}

struct ConnectedServices {
    binary: crate::runtime_cache::CachedExecutable,
    home: std::path::PathBuf,
    installation: String,
    current: Value,
    started_child: Option<ServeLaunch>,
}
impl ConnectedServices {
    fn product_handoff(&mut self) {
        if let Some(launch) = self.started_child.take() {
            launch.handoff();
        }
    }
    #[cfg(test)]
    fn require_actual_created(self) -> Result<Self, String> {
        if self.started_child.is_none() {
            return Err(
                "Qualification attached to an existing service; no change or stop is permitted."
                    .into(),
            );
        }
        Ok(self)
    }
    #[cfg(test)]
    async fn await_original_terminal(mut self, budget: Duration) -> Result<ServeTerminal, String> {
        let mut launch = self
            .started_child
            .take()
            .ok_or("No original created service is retained.")?;
        let (terminal, observed) = tokio::sync::oneshot::channel();
        let child = launch
            .child
            .as_mut()
            .ok_or("Original service child is missing.")?;
        child.terminal = Some(terminal);
        // Drop moves this SAME child + lease to the existing sole waiter. Caller
        // timeout/cancellation drops only the receiver, never the waited owner.
        drop(launch);
        let terminal = tokio::time::timeout(budget, observed)
            .await
            .map_err(|_| {
                "Original service terminal remains unknown; no restart or retry is permitted."
                    .to_string()
            })?
            .map_err(|_| {
                "Original service waiter did not provide a positive terminal result.".to_string()
            })?;
        if !lifetime::clean_original_exit(terminal.code, terminal.success) {
            return Err(
                "Original service exited unsuccessfully; qualification must stop without restart."
                    .into(),
            );
        }
        Ok(terminal)
    }
}
async fn connect_or_start(mode: StartupMode) -> Result<ConnectedServices, String> {
    let restriction = mode.arguments()?;
    static START: tokio::sync::Mutex<()> = tokio::sync::Mutex::const_new(());
    let target = engine::target()?;
    let binary = target.session().await?;
    let home = runtime::cli_path(target.home())?;
    let installation = target.daemon_id();
    let (asset, hash) = services_asset()?;
    let (trust_asset, trust_hash) = trust_asset()?;
    // session() already admitted these independently pinned bytes and retains
    // their no-write/no-delete native file lease. A passive snapshot need not
    // hash the complete immutable manager again.
    let binary_hash = runtime::component_binary_pin("orbwin-session.exe")?;
    let snapshot = json!({"version": 1, "operation": "snapshot"});
    let (mut code, mut current) = rpc(&binary, &home, installation, &snapshot).await?;
    if code != 126 {
        mode.check_existing()?;
    }
    let mut started_child = None;
    if code == 126 {
        let _guard = START.lock().await;
        (code, current) = rpc(&binary, &home, installation, &snapshot).await?;
        if code != 126 {
            mode.check_existing()?;
        }
        if code == 126 {
            let mut command = std::process::Command::new(&binary);
            command
                .arg("local-services-serve")
                .arg(&home)
                .arg(installation)
                .arg(target.daemon_id())
                .arg(asset)
                .arg(&hash)
                .arg(&trust_asset)
                .arg(&trust_hash)
                .args(&restriction)
                .stdin(Stdio::null())
                .stdout(Stdio::null())
                .stderr(Stdio::null());
            #[cfg(windows)]
            {
                use std::os::windows::process::CommandExt;
                command.creation_flags(0x08000008);
            }
            let mut child = ServeLaunch::reserve(binary.clone())?;
            binary.recheck()?;
            child.child = Some(ServeChild {
                child: command.spawn().map_err(|e| e.to_string())?,
                #[cfg(test)]
                terminal: None,
            });
            let mut ready = false;
            for _ in 0..30 {
                (code, current) = rpc(&binary, &home, installation, &snapshot).await?;
                if code != 126 {
                    ready = true;
                    break;
                }
                if let Some(status) = child
                    .child
                    .as_mut()
                    .ok_or("Missing owned service child")?
                    .child
                    .try_wait()
                    .map_err(|e| e.to_string())?
                {
                    return Err(startup_failure(status.code()).into());
                }
                tokio::time::sleep(Duration::from_millis(100)).await;
            }
            if !ready {
                return Err(
                    "Local service process is not responding; its state was preserved.".into(),
                );
            }
            // Keep the actual child generation through the native self-lease
            // acknowledgement. Thereafter Go owns its independent lifetime.
            started_child = Some(child);
        }
    }
    if mode.restricted() && started_child.is_none() {
        return Err("Qualification did not retain a newly created service.".into());
    }
    #[cfg(test)]
    if mode.restricted() {
        qualification::check_reply(code, &current)?;
    }
    if current["state"]["daemonId"] != target.daemon_id()
        || current["state"]["installation"] != installation
    {
        return Err("Local service ownership differs from the registered engine.".into());
    }
    if current["state"]["binarySha256"] != binary_hash
        || current["state"]["toolsSha256"] != hash
        || current["state"]["wslTrustSha256"] != trust_hash
    {
        return Err("Local services use another component build. Stop that owned service before updating its helpers.".into());
    }
    crate::sdk_identity::verify_snapshot(
        &binary,
        &current,
        started_child
            .as_ref()
            .and_then(|launch| launch.child.as_ref())
            .map(|owned| &owned.child),
    )?;
    Ok(ConnectedServices {
        binary,
        home,
        installation: installation.to_owned(),
        current,
        started_child,
    })
}

pub(crate) async fn invoke(request: Request) -> Result<Value, String> {
    let mut connection = connect_or_start(StartupMode::Ordinary).await?;
    // Keep existing production behavior: only a positive actual self-identity
    // ACK reaches here, then lifetime handoff precedes the ordinary request.
    connection.product_handoff();
    let ConnectedServices {
        binary,
        home,
        installation,
        current,
        ..
    } = connection;
    let mut encoded = serde_json::to_value(&request).map_err(|e| e.to_string())?;
    encoded["version"] = json!(1);
    if matches!(request, Request::Snapshot) {
        return Ok(current);
    }
    let (_, response) = rpc(&binary, &home, &installation, &encoded).await?;
    if response.get("state").is_some() {
        crate::sdk_identity::verify_snapshot(&binary, &response, None)?;
    }
    Ok(response)
}

fn trust_asset() -> Result<(std::path::PathBuf, String), String> {
    use sha2::{Digest, Sha256};
    let manifest: Value = serde_json::from_slice(
        &std::fs::read(runtime::backend_dir().join("trust-manifest.json"))
            .map_err(|e| e.to_string())?,
    )
    .map_err(|e| e.to_string())?;
    if manifest["schema"] != 1 || manifest["abi"] != 1 {
        return Err("Selected WSL trust helper ABI is unsupported.".into());
    }
    for (name, source) in [
        (
            "trust-helper/main_linux.go",
            include_bytes!("../../backend-tools/session/trust-helper/main_linux.go").as_slice(),
        ),
        (
            "trust-helper/main_other.go",
            include_bytes!("../../backend-tools/session/trust-helper/main_other.go").as_slice(),
        ),
        (
            "internal/wsltrust/protocol.go",
            include_bytes!("../../backend-tools/session/internal/wsltrust/protocol.go").as_slice(),
        ),
        (
            "internal/wsltrust/store_linux.go",
            include_bytes!("../../backend-tools/session/internal/wsltrust/store_linux.go")
                .as_slice(),
        ),
        (
            "internal/wsltrust/actions_linux.go",
            include_bytes!("../../backend-tools/session/internal/wsltrust/actions_linux.go")
                .as_slice(),
        ),
        (
            "internal/wsltrust/serve_linux.go",
            include_bytes!("../../backend-tools/session/internal/wsltrust/serve_linux.go")
                .as_slice(),
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
        if manifest["sources"][name] != format!("{:x}", Sha256::digest(source)) {
            return Err("Selected WSL trust sources changed; rebuild the bundled helper.".into());
        }
    }
    let path = runtime::backend_dir().join("orbwin-trust");
    let hash = runtime::hash_file(&path, 16 << 20)?;
    runtime::require_component_pin("orbwin-trust", &hash)?;
    if manifest["binarySha256"] != hash {
        return Err("Selected WSL trust helper checksum differs.".into());
    }
    Ok((runtime::cli_path(&path)?, hash))
}

#[cfg(test)]
mod lan_request_tests {
    #[test]
    fn private_state_startup_errors_require_the_correct_recovery() {
        assert!(super::startup_failure(Some(124)).contains("migration"));
        assert!(super::startup_failure(Some(123)).contains("incomplete state transaction"));
        assert!(super::startup_failure(Some(122)).contains("ownership or integrity"));
        for code in [None, Some(125), Some(1)] {
            assert_eq!(super::startup_failure(code), super::startup_failure(None));
        }
    }
    use super::*;

    fn selection() -> Value {
        json!({"containerId":"a".repeat(64),"startedAt":"2026-10-26T00:00:00Z",
            "hostPort":8080,"containerPort":80,"protocol":"tcp",
            "interfaceGuid":"{12345678-1234-1234-1234-123456789abc}","interfaceLuid":"123",
            "localAddress":"192.168.1.2","profile":"private","clientNetworks":["192.168.1.0/24"]})
    }

    #[test]
    fn selection_roundtrip_never_accepts_native_authority_fields() {
        let original = json!({"operation":"lan-review","revision":4,"lanSelection":selection()});
        let request: Request = serde_json::from_value(original.clone()).unwrap();
        assert_eq!(serde_json::to_value(request).unwrap(), original);
        for key in [
            "callerPid",
            "userSid",
            "workerPath",
            "helperPath",
            "generation",
            "ruleKeys",
            "acknowledged",
        ] {
            let mut injected = original.clone();
            injected["lanSelection"][key] = json!("untrusted");
            assert!(
                serde_json::from_value::<Request>(injected).is_err(),
                "{key}"
            );
        }
        for (key, value) in [
            ("protocol", json!("TCP")),
            ("profile", json!("public")),
            ("hostPort", json!(-1)),
            ("containerPort", json!(65536)),
        ] {
            let mut invalid = original.clone();
            invalid["lanSelection"][key] = value;
            assert!(serde_json::from_value::<Request>(invalid).is_err(), "{key}");
        }
    }

    #[test]
    fn closed_lan_operations_reject_unrelated_duplicate_and_trailing_fields() {
        for input in [
            r#"{"operation":"lan-options","revision":1}"#,
            r#"{"operation":"lan-activate","revision":1,"lanReviewId":"a","lanReviewSha256":"b","lanSelection":{}}"#,
            r#"{"operation":"lan-reconcile-review","revision":1,"lanReviewId":"a","lanReviewSha256":"b"}"#,
            r#"{"operation":"lan-cancel","lanOperationId":"a","lanOperationId":"b"}"#,
            r#"{"operation":"lan-inspect","lanLeaseSha256":"a","workerPath":"foreign"}"#,
            r#"{"operation":"lan-cleanup","revision":1,"lanReviewId":"a","lanReviewSha256":"b","callerPid":42}"#,
            r#"{"operation":"lan-settle-remove","revision":1,"lanReviewId":"a","lanReviewSha256":"b","deleted":true}"#,
            r#"{"operation":"lan-options"} {}"#,
        ] {
            assert!(serde_json::from_str::<Request>(input).is_err(), "{input}");
        }
        for input in [
            r#"{"operation":"lan-options"}"#,
            r#"{"operation":"lan-activate","revision":1,"lanReviewId":"a","lanReviewSha256":"b"}"#,
            r#"{"operation":"lan-remove","revision":1,"lanReviewId":"a","lanReviewSha256":"b"}"#,
            r#"{"operation":"lan-reconcile-review","lanReviewId":"a","lanReviewSha256":"b"}"#,
            r#"{"operation":"lan-cancel","lanOperationId":"a"}"#,
            r#"{"operation":"lan-settle-helper","lanOperationId":"a"}"#,
            r#"{"operation":"lan-remove-review","lanLeaseSha256":"a"}"#,
            r#"{"operation":"lan-settle-remove-review","lanLeaseSha256":"a"}"#,
            r#"{"operation":"lan-settle-remove","revision":1,"lanReviewId":"a","lanReviewSha256":"b"}"#,
            r#"{"operation":"lan-cleanup-review","lanReviewId":"a","lanReviewSha256":"b"}"#,
            r#"{"operation":"lan-cleanup","revision":1,"lanReviewId":"a","lanReviewSha256":"b"}"#,
            r#"{"operation":"lan-inspect","lanLeaseSha256":"a"}"#,
        ] {
            assert!(serde_json::from_str::<Request>(input).is_ok(), "{input}");
        }
        // Native identity, digest and lifetime validation stays with the Go owner.
    }
}

#[cfg(test)]
mod client_ownership_tests {
    use super::*;
    use std::sync::{
        atomic::{AtomicUsize, Ordering},
        Arc,
    };
    struct LeaseDrop(Arc<AtomicUsize>, Arc<tokio::sync::Notify>);
    impl Drop for LeaseDrop {
        fn drop(&mut self) {
            self.0.fetch_add(1, Ordering::SeqCst);
            self.1.notify_one();
        }
    }
    #[tokio::test]
    async fn caller_loss_keeps_client_lease_and_admission_until_positive_wait() {
        let slots = Arc::new(tokio::sync::Semaphore::new(1));
        let permit = slots.clone().try_acquire_owned().unwrap();
        let drops = Arc::new(AtomicUsize::new(0));
        let released = Arc::new(tokio::sync::Notify::new());
        let (finish, waiting) = tokio::sync::oneshot::channel::<bool>();
        let worker = spawn_owned_client(
            (permit, LeaseDrop(drops.clone(), released.clone())),
            async move {
                if waiting.await != Ok(true) {
                    std::future::pending::<()>().await;
                }
            },
        );
        drop(worker); // Caller cancellation detaches; it must not abort ownership.
        tokio::task::yield_now().await;
        assert_eq!(drops.load(Ordering::SeqCst), 0);
        assert!(slots.clone().try_acquire_owned().is_err());
        finish.send(true).unwrap();
        tokio::time::timeout(Duration::from_secs(1), released.notified())
            .await
            .unwrap();
        assert_eq!(drops.load(Ordering::SeqCst), 1);
        assert!(slots.try_acquire().is_ok());
    }
    #[tokio::test]
    async fn unknown_wait_retains_client_lease_and_refuses_another_admission() {
        let slots = Arc::new(tokio::sync::Semaphore::new(1));
        let permit = slots.clone().try_acquire_owned().unwrap();
        let drops = Arc::new(AtomicUsize::new(0));
        let released = Arc::new(tokio::sync::Notify::new());
        let (finish, waiting) = tokio::sync::oneshot::channel::<bool>();
        let (unknown, observed) = tokio::sync::oneshot::channel();
        let worker = spawn_owned_client(
            (permit, LeaseDrop(drops.clone(), released.clone())),
            async move {
                if waiting.await != Ok(true) {
                    let _ = unknown.send(());
                    std::future::pending::<()>().await;
                }
            },
        );
        let cleanup = worker.abort_handle();
        drop(worker);
        finish.send(false).unwrap();
        observed.await.unwrap();
        assert_eq!(drops.load(Ordering::SeqCst), 0);
        assert!(slots.try_acquire().is_err());
        // Only the inert fake future is aborted to release test resources.
        cleanup.abort();
        tokio::time::timeout(Duration::from_secs(1), released.notified())
            .await
            .unwrap();
    }
    struct FakeChild(std::sync::mpsc::Sender<(FakeChild, Option<LeaseDrop>)>);
    fn retain(child: FakeChild, lease: Option<LeaseDrop>) {
        let sender = child.0.clone();
        assert!(sender.send((child, lease)).is_ok());
    }
    #[test]
    fn detached_owner_moves_once_on_error_and_releases_only_on_handoff() {
        let drops = Arc::new(AtomicUsize::new(0));
        let released = Arc::new(tokio::sync::Notify::new());
        let clears = Arc::new(AtomicUsize::new(0));
        let (sender, receiver) = std::sync::mpsc::channel();
        let make = || {
            let clears = clears.clone();
            LaunchOwner {
                child: Some(FakeChild(sender.clone())),
                lease: Some(LeaseDrop(drops.clone(), released.clone())),
                retire: retain,
                release: move || {
                    clears.fetch_add(1, Ordering::SeqCst);
                },
            }
        };
        drop(make());
        assert_eq!(clears.load(Ordering::SeqCst), 0);
        assert_eq!(drops.load(Ordering::SeqCst), 0);
        let retained = receiver.try_recv().expect("one retirement transfer");
        assert!(receiver.try_recv().is_err());
        drop(retained); // The fake native waiter now confirms retirement.
        assert_eq!(drops.load(Ordering::SeqCst), 1);
        make().handoff();
        assert_eq!(clears.load(Ordering::SeqCst), 1);
        assert_eq!(drops.load(Ordering::SeqCst), 2);
        assert!(receiver.try_recv().is_err());
    }
}

#[cfg(test)]
#[path = "local_services_qualification.rs"]
mod qualification;
