//! Dormant fixed same-image observation transport. No UI command or lifecycle.
use crate::runtime_inspection::{Inspection, Unavailable};

#[cfg(windows)]
pub(crate) struct InspectionExecutable {
    executable: crate::runtime_cache::CachedExecutable,
}
#[cfg(windows)]
impl InspectionExecutable {
    pub(crate) fn into_cache(self) -> crate::runtime_cache::CachedExecutable {
        self.executable
    }
}

// Enabling requires a new independently qualified producer/package epoch.
// Neither mutable manifest fields, environment nor renderer input can set this.
const QUALIFIED_INSPECTION_SCHEMA: Option<u8> = None;

pub(crate) async fn inspect() -> Result<Inspection, Unavailable> {
    if QUALIFIED_INSPECTION_SCHEMA != Some(2) {
        return Err(Unavailable);
    }
    #[cfg(windows)]
    {
        let (send, receive) = tokio::sync::oneshot::channel();
        // Dropping the caller/receiver never cancels this original owner worker.
        tokio::spawn(async move {
            let result = worker().await;
            let _ = send.send(result);
        });
        match tokio::time::timeout(std::time::Duration::from_secs(35), receive).await {
            Ok(Ok(result)) => result,
            _ => Err(Unavailable),
        }
    }
    #[cfg(not(windows))]
    {
        Err(Unavailable)
    }
}

use crate::runtime_inspection_transport_io::{drain, send_request};
use crate::runtime_inspection_transport_model::{STDERR_LIMIT, STDOUT_LIMIT};

#[cfg(windows)]
async fn worker() -> Result<Inspection, Unavailable> {
    // Inert executable-cache preparation is explicitly effectful and distinct
    // from the subsequent Go FILE_OPEN-only observation. No management identity,
    // client config, service start, selected path or root is needed.
    let (executable, manifest) = tokio::task::spawn_blocking(crate::runtime::backend)
        .await
        .map_err(|_| Unavailable)?
        .map_err(|_| Unavailable)?;
    if manifest["runtimeInspectionSchema"].as_u64() != Some(2) {
        return Err(Unavailable);
    }
    let package = InspectionExecutable { executable };
    let mut child = crate::native_client::InspectionClient::spawn(package)
        .await
        .map_err(|_| Unavailable)?;
    let input = child.take_stdin().ok_or(Unavailable)?;
    let output = child.take_stdout().ok_or(Unavailable)?;
    let error = child.take_stderr().ok_or(Unavailable)?;
    // Owned tasks, buffers and pipe handles outlive the caller's wait budget.
    // Never abort them. A dropped JoinHandle detaches instead of cancelling IO.
    let input = tokio::spawn(send_request(input));
    let output = tokio::spawn(drain(output, STDOUT_LIMIT));
    let error = tokio::spawn(drain(error, STDERR_LIMIT));
    let exit = loop {
        match child.try_wait().map_err(|_| Unavailable)? {
            Some(exit) => break exit,
            None => tokio::time::sleep(std::time::Duration::from_millis(10)).await,
        }
    };
    // Join all original workers even on an earlier worker's failure. Task panic
    // is unknown and leaves the native guard retained, never generic cleanup.
    let input = input.await;
    let output = output.await;
    let error = error.await;
    if input.is_err() || output.is_err() || error.is_err() {
        return Err(Unavailable);
    }
    let input = input.unwrap();
    let output = output.unwrap();
    let error = error.unwrap();
    // Natural original exit + all IO joins + exact own-job empty, then checked
    // explicit handles/cache settlement. This cannot authorize a writer action.
    child.retire().map_err(|_| Unavailable)?;
    let (output, error) = (output?, error?);
    if !crate::runtime_inspection_transport_model::decode_allowed(
        input.is_ok(),
        [true, true],
        Some(exit),
        &output,
        &error,
    ) {
        return Err(Unavailable);
    }
    crate::runtime_inspection::decode(output.bytes(), true)
}
