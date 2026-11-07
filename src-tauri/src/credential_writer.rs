//! Credential writes retain their native config loan through creation, stdin,
//! output drainage and settlement. A caller may cancel its request, but cannot
//! drop the worker or clear an uncertain loan. Child output is never returned:
//! even a registry error can contain a password supplied over stdin.
use crate::{client_config::ClientConfigGuard, native_client::NativeSpec};
use std::time::Duration;
use tokio::sync::oneshot;

const OUTPUT_LIMIT: usize = 128 * 1024;
const RECOVERY: &str = "Credential settings require recovery; the pending write was retained.";

fn storage_error(stage: &'static str, error: crate::client_config::ClientConfigError) -> String {
    // This error type contains only a closed kind, constant stage and numeric
    // native metadata. Never apply this formatter to child/pipe error strings.
    format!("{RECOVERY} stage={stage}; {error}")
}

async fn discard(mut stream: impl tokio::io::AsyncRead + Unpin) -> Result<(), ()> {
    use tokio::io::AsyncReadExt;
    let mut chunk = [0u8; 8192];
    let mut count = 0usize;
    loop {
        let n = stream.read(&mut chunk).await.map_err(|_| ())?;
        if n == 0 {
            return Ok(());
        }
        count = count.checked_add(n).ok_or(())?;
        if count > OUTPUT_LIMIT {
            return Err(());
        }
    }
}

pub(crate) struct Completion {
    pub(crate) exit_code: u32,
}

// The UI mutation lease outlives the caller and all native creation/IO. Unknown
// execution ownership intentionally retains it; storage errors after positive
// execution retirement release admission while the config owner retains intent.
struct Lifetime<T> {
    token: Option<T>,
    execution_stopped: bool,
}
impl<T> Lifetime<T> {
    fn confirm_execution_stopped(&mut self) {
        self.execution_stopped = true;
    }
}
impl<T> Drop for Lifetime<T> {
    fn drop(&mut self) {
        if !self.execution_stopped {
            if let Some(token) = self.token.take() {
                std::mem::forget(token);
            }
        }
    }
}

/// `spec` must carry this guard's one-use admission and the held Docker image.
/// The registry adapter supplies only a validated auth command and owned env.
/// This module does not admit arbitrary programs, config paths or auth keys.
pub(crate) async fn execute<T: Send + 'static>(
    guard: ClientConfigGuard,
    spec: NativeSpec,
    input: Vec<u8>,
    deadline: Duration,
    external_cancel: &mut oneshot::Receiver<()>,
    lifetime: T,
) -> Result<Completion, String> {
    let (cancel, cancelled) = oneshot::channel();
    let worker = tokio::spawn(run(guard, spec, input, deadline, cancelled, lifetime));
    tokio::pin!(worker);
    // Dropping this request also drops `cancel`. The separately owned worker
    // receives that loss even while native creation is pending.
    let joined = tokio::select! {
        biased;
        _ = external_cancel => {
            drop(cancel);
            worker.await
        }
        result = &mut worker => {
            drop(cancel);
            result
        }
    };
    joined.map_err(|_| RECOVERY.to_owned())?
}

async fn run<T: Send + 'static>(
    mut guard: ClientConfigGuard,
    spec: NativeSpec,
    input: Vec<u8>,
    deadline: Duration,
    mut cancelled: oneshot::Receiver<()>,
    lifetime: T,
) -> Result<Completion, String> {
    use crate::native_client::NativeClient;
    use tokio::io::AsyncWriteExt;

    let mut lifetime = Lifetime {
        token: Some(lifetime),
        execution_stopped: true,
    };
    let cancellation = spec.cancel_handle();
    let expiry = tokio::time::sleep(deadline);
    tokio::pin!(expiry);
    let mut interrupted = deadline.is_zero()
        || !matches!(
            cancelled.try_recv(),
            Err(oneshot::error::TryRecvError::Empty)
        );
    if interrupted {
        cancellation.cancel();
    }
    lifetime.execution_stopped = false;
    let spawned = {
        let spawn = NativeClient::spawn(spec);
        tokio::pin!(spawn);
        tokio::select! {
            biased;
            _ = &mut cancelled, if !interrupted => {
                interrupted = true;
                cancellation.cancel();
                spawn.await
            }
            _ = &mut expiry, if !interrupted => {
                interrupted = true;
                cancellation.cancel();
                spawn.await
            }
            result = &mut spawn => result,
        }
    };
    let mut child = match spawned {
        Ok(child) => child,
        Err(mut error) => {
            lifetime.execution_stopped = error.no_child_created;
            // Only the native producer can certify that birth never happened.
            // All other outcomes leave guard Drop to retain intent/controller.
            if let Some(proof) = error.take_unstarted() {
                tokio::task::spawn_blocking(move || {
                    guard
                        .abort_unstarted(proof)
                        .map_err(|error| storage_error("unstarted-abort", error))?;
                    guard
                        .close()
                        .map_err(|error| storage_error("unstarted-close", error))
                })
                .await
                .map_err(|_| RECOVERY.to_owned())??;
                return Err(
                    "Credential command did not start; no credential input was sent.".into(),
                );
            }
            if let Some(proof) = error.take_stopped() {
                lifetime.confirm_execution_stopped();
                tokio::task::spawn_blocking(move || guard.release_failed_spawn(proof))
                    .await
                    .map_err(|_| RECOVERY.to_owned())?
                    .map_err(|error| storage_error("failed-spawn-owner-release", error))?;
            }
            return Err(RECOVERY.into());
        }
    };
    let mut code = None;
    {
        // These futures have no independent tasks. Scope exit relinquishes all
        // parent IO endpoints before consuming the native execution proof.
        let stdin = child.take_stdin().ok_or_else(|| RECOVERY.to_owned())?;
        let stdout = child.take_stdout().ok_or_else(|| RECOVERY.to_owned())?;
        let stderr = child.take_stderr().ok_or_else(|| RECOVERY.to_owned())?;
        let send = async move {
            let mut stdin = stdin;
            stdin.write_all(&input).await.map_err(|_| ())?;
            stdin.shutdown().await.map_err(|_| ())
        };
        if !interrupted {
            let work = async {
                let (_, _, _, code) =
                    tokio::try_join!(send, discard(stdout), discard(stderr), async {
                        child.wait().await.map_err(|_| ())
                    })?;
                Ok::<_, ()>(code)
            };
            tokio::pin!(work);
            let result = tokio::select! {
                biased;
                _ = &mut cancelled => Err(()),
                _ = &mut expiry => Err(()),
                result = &mut work => result,
            };
            match result {
                Ok(value) => code = Some(value),
                Err(()) => interrupted = true,
            }
        }
    }
    // Settlement admission is the cancellation cutoff. Already observed caller
    // loss/deadline fences the loan; after admission the blocking owner must
    // finish its file transaction even if this caller subsequently disappears.
    // Cancellation cannot roll back a registry/vault operation already sent.
    if !interrupted
        && (expiry.is_elapsed()
            || !matches!(
                cancelled.try_recv(),
                Err(oneshot::error::TryRecvError::Empty)
            ))
    {
        interrupted = true;
    }
    if interrupted {
        let killed = child.kill();
        let waited = tokio::time::timeout(Duration::from_secs(5), child.wait()).await;
        killed.map_err(|_| RECOVERY.to_owned())?;
        waited
            .map_err(|_| RECOVERY.to_owned())?
            .map_err(|_| RECOVERY.to_owned())?;
    }
    let retirement = child.into_root_exited().map_err(|_| RECOVERY.to_owned())?;
    tokio::task::spawn_blocking(move || {
        let proof = retirement.retire().map_err(|_| RECOVERY.to_owned())?;
        lifetime.confirm_execution_stopped();
        if interrupted {
            // Confirmed termination does not prove whether a registry or the
            // credential store committed. Preserve the interrupted loan.
            guard
                .release_interrupted(proof)
                .map_err(|error| storage_error("interrupted-owner-release", error))?;
            return Err(RECOVERY.to_owned());
        }
        guard
            .finish_writer(proof)
            .map_err(|error| storage_error("config-settlement", error))?;
        guard
            .close()
            .map_err(|error| storage_error("owner-close", error))?;
        Ok(Completion {
            exit_code: code.ok_or_else(|| RECOVERY.to_owned())?,
        })
    })
    .await
    .map_err(|_| RECOVERY.to_owned())?
}

#[cfg(test)]
mod tests {
    use super::*;
    use tokio::io::AsyncReadExt;

    #[test]
    fn lifetime_releases_only_after_execution_is_proven_stopped() {
        use std::sync::atomic::{AtomicUsize, Ordering};
        struct Token<'a>(&'a AtomicUsize);
        impl Drop for Token<'_> {
            fn drop(&mut self) {
                self.0.fetch_add(1, Ordering::SeqCst);
            }
        }
        let released = AtomicUsize::new(0);
        drop(Lifetime {
            token: Some(Token(&released)),
            execution_stopped: true,
        });
        assert_eq!(released.load(Ordering::SeqCst), 1);
        drop(Lifetime {
            token: Some(Token(&released)),
            execution_stopped: false,
        });
        assert_eq!(released.load(Ordering::SeqCst), 1);
        let mut retired = Lifetime {
            token: Some(Token(&released)),
            execution_stopped: false,
        };
        retired.confirm_execution_stopped();
        drop(retired);
        assert_eq!(released.load(Ordering::SeqCst), 2);
    }

    #[tokio::test]
    async fn output_at_limit_drains_but_continuous_output_is_bounded() {
        assert!(discard(tokio::io::repeat(b'x').take(OUTPUT_LIMIT as u64))
            .await
            .is_ok());
        let rejected =
            tokio::time::timeout(Duration::from_secs(1), discard(tokio::io::repeat(b'x')))
                .await
                .expect("an unbounded child stream must not block cancellation cleanup");
        assert_eq!(rejected, Err(()));
    }

    #[tokio::test]
    async fn output_failure_never_returns_child_bytes() {
        use std::{
            io,
            pin::Pin,
            task::{Context, Poll},
        };
        struct Failure;
        impl tokio::io::AsyncRead for Failure {
            fn poll_read(
                self: Pin<&mut Self>,
                _: &mut Context<'_>,
                _: &mut tokio::io::ReadBuf<'_>,
            ) -> Poll<io::Result<()>> {
                Poll::Ready(Err(io::Error::other("synthetic-credential-echo")))
            }
        }
        // Errors from the pipe are deliberately discarded along with child
        // output. They must not become user-visible credential error strings.
        assert_eq!(discard(Failure).await, Err(()));
    }
}
