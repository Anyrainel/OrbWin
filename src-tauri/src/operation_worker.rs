//! Streaming Docker clients. The detached worker owns the immutable config,
//! creation future, native job and all IO until ExecutionQuiesced. Losing the
//! request signals cancellation; it never drops an in-flight native creation.
use crate::{
    client_config::ClientConfigSnapshot,
    operations::{ClientResult, Output},
    runtime_cache::CachedExecutable,
};
use std::ffi::OsString;
use tauri::ipc::Channel;
use tokio::sync::oneshot;

struct SnapshotOwner<T> {
    snapshot: Option<ClientConfigSnapshot>,
    settled: bool,
    lease: Option<T>,
}
impl<T> Drop for SnapshotOwner<T> {
    fn drop(&mut self) {
        if !self.settled {
            // Unknown ownership keeps native generation capabilities, including
            // helper images, alive. A pathname alone cannot preserve them.
            if let Some(lease) = self.lease.take() {
                std::mem::forget(lease);
            }
            if let Some(snapshot) = self.snapshot.take() {
                std::mem::forget(snapshot);
            }
        }
    }
}

#[allow(clippy::too_many_arguments)]
pub(crate) async fn run<T: Send + 'static>(
    executable: CachedExecutable,
    args: Vec<OsString>,
    environment: Vec<(OsString, OsString)>,
    snapshot: Option<ClientConfigSnapshot>,
    input: Option<Vec<u8>>,
    output: Channel<Output>,
    cancelled: &mut oneshot::Receiver<()>,
    seconds: u64,
    lease: Option<T>,
) -> Result<ClientResult, String> {
    #[cfg(windows)]
    {
        let (cancel, signal) = oneshot::channel();
        let mut worker = tokio::spawn(work(
            executable,
            args,
            environment,
            snapshot,
            input,
            output,
            signal,
            seconds,
            lease,
        ));
        // The sender also signals through Drop if this request future is lost.
        let result = tokio::select! {
            biased;
            _ = cancelled => {
                drop(cancel);
                worker.await
            }
            result = &mut worker => { drop(cancel); result }
        };
        result.map_err(|_| "Operation worker did not settle native ownership.".to_string())?
    }
    #[cfg(not(windows))]
    {
        let _ = (
            executable,
            args,
            environment,
            snapshot,
            input,
            output,
            cancelled,
            seconds,
            lease,
        );
        Err("Native Docker operation clients require Windows.".into())
    }
}

#[cfg(windows)]
#[allow(clippy::too_many_arguments)]
async fn work<T: Send + 'static>(
    executable: CachedExecutable,
    args: Vec<OsString>,
    environment: Vec<(OsString, OsString)>,
    snapshot: Option<ClientConfigSnapshot>,
    input: Option<Vec<u8>>,
    output: Channel<Output>,
    mut cancelled: oneshot::Receiver<()>,
    seconds: u64,
    lease: Option<T>,
) -> Result<ClientResult, String> {
    use crate::native_client::{NativeClient, NativeSpec};
    use std::time::Duration;
    use tokio::io::{AsyncReadExt, AsyncWriteExt};
    let mut owner = SnapshotOwner {
        snapshot,
        settled: true,
        lease,
    };
    if let Some(snapshot) = &owner.snapshot {
        snapshot.check().map_err(|e| e.to_string())?;
    }
    if !matches!(
        cancelled.try_recv(),
        Err(oneshot::error::TryRecvError::Empty)
    ) {
        return Ok(ClientResult {
            exit_code: None,
            interrupted: true,
            reason: "Client command cancelled before dispatch".into(),
        });
    }
    if seconds == 0 {
        return Ok(ClientResult {
            exit_code: None,
            interrupted: true,
            reason: "Client command reached its time limit before dispatch".into(),
        });
    }
    let spec = NativeSpec::new(executable, args, environment, None, None);
    let cancellation = spec.cancel_handle();
    let deadline = tokio::time::sleep(Duration::from_secs(seconds));
    tokio::pin!(deadline);
    let mut reason = None;
    owner.settled = false;
    let spawned = {
        let creation = NativeClient::spawn(spec);
        tokio::pin!(creation);
        tokio::select! {
            biased;
            _ = &mut cancelled => {
                reason = Some("Client command cancelled".to_string());
                cancellation.cancel();
                creation.await
            }
            _ = &mut deadline => {
                reason = Some("Client command reached its time limit".to_string());
                cancellation.cancel();
                creation.await
            }
            result = &mut creation => result,
        }
    };
    let mut child = match spawned {
        Ok(child) => child,
        Err(mut error) => {
            if let Some(stopped) = error.take_admission_stopped() {
                stopped.consume();
                owner.settled = true;
            }
            if let Some(reason) = reason {
                return Ok(ClientResult {
                    exit_code: None,
                    interrupted: true,
                    reason,
                });
            }
            return Err(error.message);
        }
    };
    let mut status = None;
    {
        // No detached IO tasks: this worker owns every endpoint and polls input,
        // both streams, cancellation, expiry and root exit concurrently. Leaving
        // this scope synchronously releases all IO futures before retirement.
        let stdin = child.take_stdin();
        let write_input = async move {
            let mut stdin = stdin.ok_or("Missing client input")?;
            if let Some(input) = input {
                stdin
                    .write_all(&input)
                    .await
                    .map_err(|e| format!("Could not send operation input: {e}"))?;
            }
            stdin
                .shutdown()
                .await
                .map_err(|e| format!("Could not close operation input: {e}"))?;
            Ok::<_, String>(())
        };
        tokio::pin!(write_input);
        let mut input_done = false;
        let mut stdout = child
            .take_stdout()
            .ok_or("Missing native operation stdout")?;
        let mut stderr = child
            .take_stderr()
            .ok_or("Missing native operation stderr")?;
        let (mut out_open, mut err_open) = (true, true);
        let (mut out, mut err) = ([0u8; 8192], [0u8; 8192]);
        let mut remaining = 2 * 1024 * 1024;
        while reason.is_none() {
            tokio::select! {
                biased;
                _ = &mut cancelled => reason = Some("Client command cancelled".into()),
                _ = &mut deadline => reason = Some("Client command reached its time limit".into()),
                result = &mut write_input, if !input_done => {
                    input_done = true;
                    if let Err(error) = result { reason = Some(error); }
                }
                result = child.wait(), if status.is_none() => {
                    match result { Ok(value) => status = Some(value), Err(error) => reason = Some(error) }
                }
                result = stdout.read(&mut out), if out_open => {
                    match result {
                        Ok(0) => out_open = false,
                        Ok(n) => if let Err(error) = crate::operations::send(&output, "stdout", &out[..n], &mut remaining) { reason = Some(error); },
                        Err(error) => reason = Some(error.to_string()),
                    }
                }
                result = stderr.read(&mut err), if err_open => {
                    match result {
                        Ok(0) => err_open = false,
                        Ok(n) => if let Err(error) = crate::operations::send(&output, "stderr", &err[..n], &mut remaining) { reason = Some(error); },
                        Err(error) => reason = Some(error.to_string()),
                    }
                }
            }
            if input_done && status.is_some() && !out_open && !err_open {
                break;
            }
        }
    }
    if reason.is_some() {
        let killed = child.kill();
        let waited = tokio::time::timeout(Duration::from_secs(5), child.wait()).await;
        killed?;
        waited.map_err(|_| "Operation client termination remains unconfirmed.".to_string())??;
    }
    let retirement = child.into_root_exited()?;
    let proof = tokio::task::spawn_blocking(move || retirement.retire())
        .await
        .map_err(|_| "Operation execution quiescence did not settle.".to_string())??;
    drop(proof);
    owner.settled = true;
    if let Some(snapshot) = &owner.snapshot {
        snapshot.check().map_err(|e| e.to_string())?;
    }
    Ok(ClientResult {
        exit_code: status.map(|value| value as i32),
        interrupted: reason.is_some(),
        reason: reason.unwrap_or_default(),
    })
}
