//! Per-session clients for the Docker SDK helper. No host console or host shell.
//! rdd/Moby own workloads. Two clients maximum; 8 KiB input writes, 64 KiB
//! acknowledged output, 15-second stalled-viewer deadline. Closing kills/reaps
//! only this SDK client; its exec process may remain in the container. The helper
//! closes its hijacked connection on stdin EOF and never restarts an engine.
use crate::engine;
use serde::{Deserialize, Serialize};
use std::{
    sync::{atomic::Ordering, Arc},
    time::Duration,
};
use tauri::ipc::Channel;
use tokio::sync::{mpsc, oneshot};
#[path = "session_control.rs"]
mod control;
#[cfg(test)]
use control::await_creation;
pub use control::Sessions;
use control::{reserve, Opening, Reservation, Session};
#[cfg(test)]
use std::sync::{atomic::AtomicBool, Mutex};
#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum Shell {
    Sh,
    Bash,
    Debug,
}
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
pub struct PodTarget {
    namespace: String,
    name: String,
    uid: String,
    #[serde(default)]
    container: String,
}
enum ExecTarget {
    Container(String),
    Pod(PodTarget),
}
#[derive(Clone, Serialize)]
#[serde(tag = "kind", rename_all = "camelCase")]
pub enum Event {
    Ready,
    Data { bytes: Vec<u8> },
    Exit { code: Option<u32>, message: String },
}
fn size(cols: u16, rows: u16) -> Result<(), String> {
    if !(10..=400).contains(&cols) || !(2..=150).contains(&rows) {
        return Err("Terminal dimensions are outside supported bounds.".into());
    }
    Ok(())
}
fn valid_id(id: &str) -> bool {
    id.len() == 36 && id.bytes().all(|c| c.is_ascii_hexdigit() || c == b'-')
}
pub async fn open(
    state: &Sessions,
    id: String,
    container: String,
    shell: Shell,
    cols: u16,
    rows: u16,
    output: Channel<Event>,
) -> Result<(), String> {
    open_request(
        state,
        id,
        shell,
        cols,
        rows,
        output,
        ExecTarget::Container(container),
    )
    .await
}
pub async fn open_pod(
    state: &Sessions,
    id: String,
    pod: PodTarget,
    shell: Shell,
    cols: u16,
    rows: u16,
    output: Channel<Event>,
) -> Result<(), String> {
    open_request(state, id, shell, cols, rows, output, ExecTarget::Pod(pod)).await
}
async fn open_request(
    state: &Sessions,
    id: String,
    shell: Shell,
    cols: u16,
    rows: u16,
    output: Channel<Event>,
    request: ExecTarget,
) -> Result<(), String> {
    if !valid_id(&id) {
        return Err("Invalid terminal session ID.".into());
    }
    match &request {
        ExecTarget::Container(container)
            if container.len() != 64 || !container.bytes().all(|c| c.is_ascii_hexdigit()) =>
        {
            return Err("Select a container with a full engine ID.".into())
        }
        ExecTarget::Pod(pod) => {
            let name = |name: &str, max: usize| {
                !name.is_empty()
                    && name.len() <= max
                    && name.as_bytes()[0].is_ascii_alphanumeric()
                    && name
                        .bytes()
                        .all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || b"-.".contains(&c))
            };
            if !name(&pod.namespace, 63)
                || !name(&pod.name, 253)
                || pod.uid.is_empty()
                || pod.uid.len() > 128
                || pod.uid.chars().any(char::is_control)
                || (!pod.container.is_empty() && !name(&pod.container, 63))
            {
                return Err("Select a current pod, namespace, UID and container.".into());
            }
        }
        _ => {}
    }
    size(cols, rows)?;
    if matches!(shell, Shell::Debug) && matches!(request, ExecTarget::Pod(_)) {
        return Err(
            "Debug tools currently target local Docker containers. Use the pod's own shell here."
                .into(),
        );
    }
    let (session, receiver) = Session::control();
    let reservation = reserve(state.0.clone(), id, session.clone())?;
    let (reply, received) = oneshot::channel();
    let worker_session = session.clone();
    tokio::spawn(async move {
        let mut reply = Some(reply);
        let result = work(
            reservation,
            worker_session.clone(),
            receiver,
            shell,
            cols,
            rows,
            output.clone(),
            request,
            &mut reply,
        )
        .await;
        worker_session.finished.store(true, Ordering::Release);
        if let Some(reply) = reply {
            let _ = reply.send(result.clone());
        }
        if let Err(error) = result {
            let _ = output.send(Event::Exit {
                code: None,
                message: error,
            });
        }
    });
    // The request owns only this cancellation signal. The detached worker keeps
    // creation, pipes and the exact cache/job capabilities through retirement.
    let mut opening = Opening(session, true);
    let result = received
        .await
        .map_err(|_| "Terminal worker did not settle startup.".to_string())?;
    if result.is_ok() {
        opening.1 = false;
    }
    result
}

// The registry contains controls only; it cannot own its detached worker or
// Inner. No Arc cycle keeps successful sessions alive. Reservation is released
// only before native dispatch or after ExecutionQuiesced. Unknown cleanup keeps
// the actual reservation and native owner retained, never just an error flag.
#[allow(clippy::too_many_arguments)]
async fn work(
    mut reservation: Reservation,
    session: Arc<Session>,
    receiver: mpsc::Receiver<Vec<u8>>,
    shell: Shell,
    cols: u16,
    rows: u16,
    output: Channel<Event>,
    request: ExecTarget,
    reply: &mut Option<oneshot::Sender<Result<(), String>>>,
) -> Result<(), String> {
    let target = engine::target()?;
    engine::check(&target).await?;
    let binary = target.session().await?;
    let debugging = matches!(shell, Shell::Debug);
    let mut initial = serde_json::json!({"kind":"open","daemonId":target.daemon_id(),"shell":match shell { Shell::Sh | Shell::Debug => "sh", Shell::Bash => "bash" },"cols":cols,"rows":rows});
    if debugging {
        initial["mode"] = serde_json::json!("debug");
        initial["session"] = serde_json::json!(reservation.id);
    }
    match request {
        ExecTarget::Container(container) => initial["container"] = serde_json::json!(container),
        ExecTarget::Pod(pod) => {
            let rdd = target.rdd().await?;
            let paths = crate::backend_paths::read(&rdd, target.home()).await?;
            let config = paths.k3s_config;
            if !config.is_file() {
                return Err("The owned cluster has no kubeconfig yet. Enable Kubernetes and wait for readiness.".into());
            }
            initial["mode"] = serde_json::json!("kubernetes");
            initial["namespace"] = serde_json::json!(pod.namespace);
            initial["pod"] = serde_json::json!(pod.name);
            initial["podUid"] = serde_json::json!(pod.uid);
            initial["containerName"] = serde_json::json!(pod.container);
            initial["kubeconfig"] = serde_json::json!(config);
        }
    }
    if session.closed.load(Ordering::Acquire) || reservation.inner.shutdown.load(Ordering::Acquire)
    {
        return Err("Terminal opening cancelled.".into());
    }
    #[cfg(windows)]
    {
        use crate::native_client::NativeSpec;
        use tokio::io::{AsyncReadExt, AsyncWriteExt};
        let mut environment: Vec<_> = std::env::vars_os()
            .filter(|(key, _)| {
                !matches!(
                    key.to_string_lossy().to_ascii_uppercase().as_str(),
                    "ORBWIN_MAINTENANCE_IMAGE"
                        | "ORBWIN_MAINTENANCE_IMAGE_SHA256"
                        | "ORBWIN_TRANSIENT_DIR"
                )
            })
            .collect();
        if debugging {
            let (archive, hash) = tokio::task::spawn_blocking(crate::runtime::maintenance_image)
                .await
                .map_err(|_| "Maintenance image verification did not complete.")??;
            environment.push(("ORBWIN_MAINTENANCE_IMAGE".into(), archive.into()));
            environment.push(("ORBWIN_MAINTENANCE_IMAGE_SHA256".into(), hash.into()));
        }
        environment.push((
            "ORBWIN_TRANSIENT_DIR".into(),
            target.home().join(".rdorbwin/transients").into(),
        ));
        let spec = NativeSpec::new(binary, vec![], environment, None, None);
        let mut child = control::spawn_reserved(&mut reservation, &session, spec).await?;
        // A missing endpoint is an ownership failure, not a reason to free a
        // slot. NativeClient's guard retains its real resources on early error.
        let mut stdin = child.take_stdin().ok_or("Missing terminal input")?;
        let mut stdout = child.take_stdout().ok_or("Missing terminal output")?;
        let mut stderr = child.take_stderr().ok_or("Missing terminal diagnostics")?;
        let mut initial = serde_json::to_vec(&initial).map_err(|e| e.to_string())?;
        initial.push(b'\n');
        let mut diagnostics = Vec::new();
        let mut code = None;
        let mut reason = session
            .closed
            .load(Ordering::Acquire)
            .then_some("Terminal opening cancelled.");
        {
            let mut receiver = receiver;
            let writer = async {
                stdin.write_all(&initial).await.map_err(|e| e.to_string())?;
                if let Some(reply) = reply.take() {
                    if reply.send(Ok(())).is_err() {
                        session.close();
                    }
                }
                while let Some(bytes) = receiver.recv().await {
                    // Retain one write future through other IO progress: never
                    // restart a partially completed JSON frame after select.
                    tokio::time::timeout(Duration::from_secs(5), stdin.write_all(&bytes))
                        .await
                        .map_err(|_| "Terminal input stalled.".to_string())?
                        .map_err(|e| e.to_string())?;
                }
                Ok::<(), String>(())
            };
            let reader = async {
                let mut buffer = [0u8; 4096];
                loop {
                    let limit = output_credit(&session).await?;
                    let n = stdout
                        .read(&mut buffer[..limit])
                        .await
                        .map_err(|e| e.to_string())?;
                    if n == 0 {
                        return Ok::<(), String>(());
                    }
                    *session
                        .outstanding
                        .lock()
                        .map_err(|_| "Terminal credit is unavailable")? += n;
                    output
                        .send(Event::Data {
                            bytes: buffer[..n].to_vec(),
                        })
                        .map_err(|e| e.to_string())?;
                }
            };
            let errors = async {
                let mut buffer = [0u8; 1024];
                let mut ready = false;
                loop {
                    let n = stderr.read(&mut buffer).await.map_err(|e| e.to_string())?;
                    if n == 0 {
                        return Ok::<(), String>(());
                    }
                    let count = n.min(8192usize.saturating_sub(diagnostics.len()));
                    diagnostics.extend_from_slice(&buffer[..count]);
                    const READY: &[u8] = b"ORBWIN_SESSION_READY\n";
                    if !ready && diagnostics.starts_with(READY) {
                        diagnostics.drain(..READY.len());
                        ready = true;
                        output.send(Event::Ready).map_err(|e| e.to_string())?;
                    }
                }
            };
            tokio::pin!(writer, reader, errors);
            let (mut out_done, mut err_done) = (false, false);
            // A root that exits while a descendant holds pipes gets a bounded
            // drain window; retirement subsequently stops the exact whole job.
            let mut drain_deadline = None;
            while reason.is_none() {
                tokio::select! {
                    biased;
                    _ = session.cancellation.notified() => { reason = Some("Terminal client disconnected."); break; }
                    _ = async { match drain_deadline { Some(at) => tokio::time::sleep_until(at).await, None => std::future::pending().await } } => {
                        reason = Some("Terminal output did not close after client exit."); break;
                    }
                    result = &mut writer, if code.is_none() => { reason = Some(if result.is_err() { "Terminal input failed." } else { "Terminal input closed." }); break; }
                    result = child.wait(), if code.is_none() => {
                        match result { Ok(value) => { code = Some(value); drain_deadline = Some(tokio::time::Instant::now() + Duration::from_secs(5)); }, Err(_) => { reason = Some("Terminal client wait failed."); break; } }
                    }
                    result = &mut reader, if !out_done => {
                        out_done = true; if result.is_err() { reason = Some("Terminal output failed or viewer stalled."); break; }
                    }
                    result = &mut errors, if !err_done => {
                        err_done = true; if result.is_err() { reason = Some("Terminal diagnostics failed."); break; }
                    }
                }
                if code.is_some() && out_done && err_done {
                    break;
                }
            }
        }
        // Closing stdin is the helper's existing graceful close/EOF protocol.
        // No task can still use these pipes before the execution proof is made.
        drop(stdin);
        drop(stdout);
        drop(stderr);
        if code.is_none() {
            code = tokio::time::timeout(Duration::from_secs(2), child.wait())
                .await
                .ok()
                .and_then(Result::ok);
        }
        if code.is_none() {
            let stop = child.kill();
            let waited = tokio::time::timeout(Duration::from_secs(5), child.wait()).await;
            stop?;
            code = Some(waited.map_err(|_| "Terminal client termination remains unconfirmed.")??);
        }
        let retirement = child.into_root_exited()?;
        let proof = tokio::task::spawn_blocking(move || retirement.retire())
            .await
            .map_err(|_| "Terminal execution quiescence did not settle.")??;
        drop(proof);
        reservation.armed = true;
        if let Some(reply) = reply.take() {
            let _ = reply.send(Err(reason
                .unwrap_or("Terminal exited before connecting.")
                .into()));
        }
        let message = if debugging && reason.is_some() {
            "Debug client disconnected. The target was not stopped. Disposable tools expire within 20 seconds if immediate cleanup was unavailable."
        } else if reason.is_some() {
            "Terminal client disconnected. The container and its workloads were not stopped; an exec process may remain inside it."
        } else {
            "Terminal client exited."
        };
        let detail = String::from_utf8_lossy(&diagnostics);
        let _ = output.send(Event::Exit {
            code,
            message: format!("{message} {} {detail}", reason.unwrap_or_default()),
        });
        Ok(())
    }
    #[cfg(not(windows))]
    {
        let _ = (binary, initial, debugging, receiver, output, reply);
        Err("Native terminal clients require Windows.".into())
    }
}

async fn output_credit(session: &Session) -> Result<usize, String> {
    // notify_one stores a permit across the read/check/await boundary; the
    // short counter lock is never held across IO or an async wait.
    let deadline = tokio::time::Instant::now() + Duration::from_secs(15);
    output_credit_until(session, deadline).await
}
async fn output_credit_until(
    session: &Session,
    deadline: tokio::time::Instant,
) -> Result<usize, String> {
    loop {
        let outstanding = *session
            .outstanding
            .lock()
            .map_err(|_| "Terminal credit is unavailable")?;
        if outstanding < 65536 {
            return Ok(4096.min(65536 - outstanding));
        }
        tokio::time::timeout_at(deadline, session.credit.notified())
            .await
            .map_err(|_| "Terminal viewer stopped acknowledging output.".to_string())?;
    }
}

fn get(state: &Sessions, id: &str) -> Result<Arc<Session>, String> {
    state
        .0
        .slots
        .lock()
        .map_err(|_| "Session registry is unavailable")?
        .get(id)
        .cloned()
        .ok_or("Terminal session is not connected.".into())
}
pub fn close(state: &Sessions, id: &str) -> Result<(), String> {
    get(state, id)?.close();
    Ok(())
}
pub fn acknowledge(state: &Sessions, id: &str, bytes: usize) -> Result<(), String> {
    let session = get(state, id)?;
    let mut outstanding = session
        .outstanding
        .lock()
        .map_err(|_| "Terminal credit is unavailable")?;
    *outstanding = outstanding.saturating_sub(bytes.min(65536));
    session.credit.notify_one();
    Ok(())
}
pub async fn input(state: &Sessions, id: &str, bytes: Vec<u8>) -> Result<(), String> {
    if bytes.len() > 8192 {
        return Err("Terminal input is limited to 8 KiB per write.".into());
    }
    let session = get(state, id)?;
    // JSON byte arrays expand to four characters per byte. Split a permitted
    // 8 KiB write so even 0xff input stays below the SDK's 32 KiB scanner bound.
    // Reserve every queue entry together: a busy result never accepts a prefix.
    session.send_frames(
        bytes
            .chunks(4096)
            .map(|part| serde_json::json!({"kind":"input","data":part}))
            .collect(),
    )
}
pub async fn resize(state: &Sessions, id: &str, cols: u16, rows: u16) -> Result<(), String> {
    size(cols, rows)?;
    let session = get(state, id)?;
    session.send(serde_json::json!({"kind":"resize","cols":cols,"rows":rows}))
}
#[cfg(test)]
#[path = "terminal_qualification.rs"]
mod qualification;
#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn validates_dimensions_and_session_ids() {
        assert!(size(80, 24).is_ok());
        assert!(size(0, 24).is_err());
        assert!(size(80, 1000).is_err());
        assert!(!valid_id("../../outside"));
    }

    #[test]
    fn terminal_reservations_survive_close_until_owner_settlement() {
        let state = Sessions::default();
        let (a, _a_rx) = Session::control();
        let (b, _b_rx) = Session::control();
        let first = reserve(state.0.clone(), "first".into(), a.clone()).unwrap();
        let second = reserve(state.0.clone(), "second".into(), b.clone()).unwrap();
        assert!(reserve(state.0.clone(), "third".into(), b.clone()).is_err());
        close(&state, "first").unwrap();
        assert!(a.closed.load(Ordering::Acquire));
        assert!(reserve(state.0.clone(), "third".into(), b.clone()).is_err());
        drop(first);
        let third = reserve(state.0.clone(), "third".into(), b.clone()).unwrap();
        assert!(reserve(state.0.clone(), "third".into(), b).is_err());
        drop(second);
        drop(third);
        assert!(state.0.slots.lock().unwrap().is_empty());
    }

    #[tokio::test]
    async fn terminal_opening_loss_and_shutdown_signal_without_releasing_slots() {
        let state = Sessions::default();
        let (session, _receiver) = Session::control();
        let reservation = reserve(state.0.clone(), "opening".into(), session.clone()).unwrap();
        drop(Opening(session.clone(), true));
        tokio::time::timeout(Duration::from_millis(100), session.cancellation.notified())
            .await
            .unwrap();
        assert!(session.send(serde_json::json!({"kind":"resize"})).is_err());
        assert_eq!(state.0.slots.lock().unwrap().len(), 1);
        drop(state);
        assert!(reservation.inner.shutdown.load(Ordering::Acquire));
        assert_eq!(reservation.inner.slots.lock().unwrap().len(), 1);
        drop(reservation);
    }

    #[tokio::test]
    async fn terminal_cancellation_continues_the_same_creation_future() {
        struct Finished(Arc<AtomicBool>);
        impl Drop for Finished {
            fn drop(&mut self) {
                self.0.store(true, Ordering::Release);
            }
        }
        let (session, _receiver) = Session::control();
        let cancelled = Arc::new(AtomicBool::new(false));
        let finished = Arc::new(AtomicBool::new(false));
        let (gate, release) = oneshot::channel();
        let creation_finished = finished.clone();
        let cancel_observed = cancelled.clone();
        session.close(); // Cancellation can precede the first creation poll.
        let worker = tokio::spawn(async move {
            let creation = async move {
                let _finished = Finished(creation_finished);
                release.await.unwrap();
                42
            };
            await_creation(creation, &session, || {
                cancel_observed.store(true, Ordering::Release)
            })
            .await
        });
        tokio::time::timeout(Duration::from_secs(1), async {
            while !cancelled.load(Ordering::Acquire) {
                tokio::task::yield_now().await;
            }
        })
        .await
        .unwrap();
        assert!(!finished.load(Ordering::Acquire));
        assert!(!worker.is_finished());
        gate.send(()).unwrap();
        assert_eq!(worker.await.unwrap(), 42);
        assert!(finished.load(Ordering::Acquire));
    }

    #[tokio::test]
    async fn terminal_input_frames_fit_sdk_scanner_and_queue_admits_atomically() {
        let state = Sessions::default();
        let (session, mut receiver) = Session::control();
        let _reservation = reserve(state.0.clone(), "input".into(), session.clone()).unwrap();
        input(&state, "input", vec![255; 8192]).await.unwrap();
        let mut received = Vec::new();
        for _ in 0..2 {
            let frame = receiver.try_recv().unwrap();
            assert!(frame.len() < 32768);
            let value: serde_json::Value = serde_json::from_slice(&frame).unwrap();
            received.extend(serde_json::from_value::<Vec<u8>>(value["data"].clone()).unwrap());
        }
        assert_eq!(received, vec![255; 8192]);
        assert!(input(&state, "input", vec![0; 8193]).await.is_err());
        for _ in 0..7 {
            resize(&state, "input", 80, 24).await.unwrap();
        }
        assert!(input(&state, "input", vec![255; 8192]).await.is_err());
        assert_eq!(receiver.len(), 7); // No accepted prefix of the failed write.
        resize(&state, "input", 80, 24).await.unwrap();
        assert!(resize(&state, "input", 80, 24).await.is_err());
    }

    #[tokio::test]
    async fn terminal_output_credit_bounds_and_expired_stall_deadline() {
        let state = Sessions::default();
        let (session, _receiver) = Session::control();
        let _reservation = reserve(state.0.clone(), "credit".into(), session.clone()).unwrap();
        *session.outstanding.lock().unwrap() = 65530;
        assert_eq!(output_credit(&session).await.unwrap(), 6);
        *session.outstanding.lock().unwrap() = 65536;
        assert!(output_credit_until(&session, tokio::time::Instant::now())
            .await
            .is_err());
        acknowledge(&state, "credit", 17).unwrap();
        assert_eq!(output_credit(&session).await.unwrap(), 17);
        acknowledge(&state, "credit", usize::MAX).unwrap();
        assert_eq!(output_credit(&session).await.unwrap(), 4096);
        assert_eq!(*session.outstanding.lock().unwrap(), 0);
    }
}
