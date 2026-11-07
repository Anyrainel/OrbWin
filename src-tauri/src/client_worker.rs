//! Bounded cached diagnostics. A detached worker owns creation, IO, the native
//! descendant tree and the immutable config generation. Caller loss signals
//! cancellation; it cannot drop creation or release a generation prematurely.
use crate::{client_config::ClientConfigSnapshot, runtime_cache::CachedExecutable};
use std::{ffi::OsString, time::Duration};

const LIMIT: u64 = 128 * 1024;

struct SnapshotOwner {
    snapshot: Option<ClientConfigSnapshot>,
    settled: bool,
}
impl Drop for SnapshotOwner {
    fn drop(&mut self) {
        if !self.settled {
            // An uncertain tree may still read its generation. Keep the actual
            // native file/directory capabilities, not merely its pathname.
            if let Some(snapshot) = self.snapshot.take() {
                std::mem::forget(snapshot);
            }
        }
    }
}

pub(crate) async fn read(
    executable: CachedExecutable,
    args: Vec<OsString>,
    environment: Vec<(OsString, OsString)>,
    snapshot: Option<ClientConfigSnapshot>,
    deadline: Duration,
) -> Result<String, String> {
    #[cfg(windows)]
    {
        let (cancel, cancelled) = tokio::sync::oneshot::channel();
        let worker = tokio::spawn(run(
            executable,
            args,
            environment,
            snapshot,
            deadline,
            cancelled,
        ));
        let result = worker
            .await
            .map_err(|_| "Diagnostic worker did not settle its native ownership.".to_string())?;
        drop(cancel);
        result
    }
    #[cfg(not(windows))]
    {
        let _ = (executable, args, environment, snapshot, deadline);
        Err("Cached native diagnostics require Windows.".into())
    }
}

#[cfg(windows)]
async fn run(
    executable: CachedExecutable,
    args: Vec<OsString>,
    environment: Vec<(OsString, OsString)>,
    snapshot: Option<ClientConfigSnapshot>,
    deadline: Duration,
    mut cancelled: tokio::sync::oneshot::Receiver<()>,
) -> Result<String, String> {
    use crate::native_client::{NativeClient, NativeSpec};
    use tokio::{io::AsyncReadExt, time::timeout};
    let mut owner = SnapshotOwner {
        snapshot,
        settled: true,
    };
    if let Some(snapshot) = &owner.snapshot {
        snapshot.check().map_err(|e| e.to_string())?;
    }
    if !matches!(
        cancelled.try_recv(),
        Err(tokio::sync::oneshot::error::TryRecvError::Empty)
    ) {
        return Err("Diagnostic caller disconnected before dispatch.".into());
    }
    if deadline.is_zero() {
        return Err("Diagnostic timed out before dispatch.".into());
    }
    let spec = NativeSpec::new(executable, args, environment, None, None);
    let cancellation = spec.cancel_handle();
    let expiry = tokio::time::sleep(deadline);
    tokio::pin!(expiry);
    let mut interrupted = None;
    // After this point, a panic or arbitrary future drop retains the snapshot.
    owner.settled = false;
    let spawned = {
        let spawn = NativeClient::spawn(spec);
        tokio::pin!(spawn);
        tokio::select! {
            biased;
            _ = &mut cancelled => {
                interrupted = Some("Diagnostic caller disconnected.".to_string());
                cancellation.cancel();
                spawn.await
            }
            _ = &mut expiry => {
                interrupted = Some(format!("Diagnostic timed out after {} ms", deadline.as_millis()));
                cancellation.cancel();
                spawn.await
            }
            result = &mut spawn => result,
        }
    };
    let mut child = match spawned {
        Ok(child) => child,
        Err(mut error) => {
            // The producer alone knows whether a process ever existed. Unknown
            // cleanup keeps its native owner and this generation pinned.
            if let Some(stopped) = error.take_admission_stopped() {
                stopped.consume();
                owner.settled = true;
            }
            return Err(interrupted.unwrap_or(error.message));
        }
    };
    drop(child.take_stdin()); // Always EOF, including an empty diagnostic input.
    async fn collect(reader: impl tokio::io::AsyncRead + Unpin) -> Result<Vec<u8>, String> {
        let mut bytes = Vec::new();
        reader
            .take(LIMIT + 1)
            .read_to_end(&mut bytes)
            .await
            .map_err(|e| e.to_string())?;
        if bytes.len() as u64 > LIMIT {
            return Err("Diagnostic output exceeded the size limit".into());
        }
        Ok(bytes)
    }
    let stdout = child
        .take_stdout()
        .ok_or("Missing native diagnostic stdout")?;
    let stderr = child
        .take_stderr()
        .ok_or("Missing native diagnostic stderr")?;
    let mut captured = None;
    {
        // IO futures are polled only by this worker. Leaving this scope drops
        // their endpoints synchronously; there are no detached IO tasks which
        // could still access the generation during retirement.
        let out = collect(stdout);
        let err = collect(stderr);
        if interrupted.is_none() {
            let result = {
                let work = async {
                    let (out, err, code) = tokio::try_join!(out, err, child.wait(),)?;
                    Ok::<_, String>((out, err, code))
                };
                tokio::pin!(work);
                tokio::select! {
                    result = &mut work => result,
                    _ = &mut cancelled => Err("Diagnostic caller disconnected.".into()),
                    _ = &mut expiry => Err(format!("Diagnostic timed out after {} ms", deadline.as_millis())),
                }
            };
            match result {
                Ok(value) => captured = Some(value),
                Err(error) => interrupted = Some(error),
            }
        }
    }
    if interrupted.is_some() {
        // Termination is a request. Require the original handle to become
        // signaled, and join every IO task before requesting tree retirement.
        let kill = child.kill();
        let waited = timeout(Duration::from_secs(5), child.wait()).await;
        kill?;
        waited.map_err(|_| "Diagnostic termination remains unconfirmed.".to_string())??;
    }
    let retirement = child.into_root_exited()?;
    let proof = tokio::task::spawn_blocking(move || retirement.retire())
        .await
        .map_err(|_| "Diagnostic tree retirement did not settle.".to_string())??;
    drop(proof);
    owner.settled = true;
    if let Some(snapshot) = &owner.snapshot {
        snapshot.check().map_err(|e| e.to_string())?;
    }
    if let Some(snapshot) = owner.snapshot.take() {
        // The actual native tree is retired. Shared means another real reader
        // retains the immutable generation; it is not a quiescence certificate.
        // If this is the last owner, propagate checked native close failure.
        snapshot.close().map_err(|error| error.to_string())?;
    }
    if let Some(error) = interrupted {
        return Err(error);
    }
    let (out, err, code) = captured.ok_or("Diagnostic output unavailable")?;
    if code != 0 {
        return Err(format!(
            "Diagnostic exited with code {code}: {}",
            crate::process::decode(&err)
        ));
    }
    Ok(crate::process::decode(&out))
}
