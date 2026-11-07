//! One native recovery owner, held across review and consumed exactly once.
//! The renderer gets an opaque epoch, never paths, settings, or process proofs.
//! Detached workers finish checked release even when an invoke future disappears.
use crate::{client_config, runtime};
use serde::Serialize;
use std::sync::{Arc, Mutex};
use tokio::{sync::oneshot, task::AbortHandle};

#[derive(Default, Clone)]
pub(crate) struct Reviews(Arc<Mutex<Option<Review>>>);
enum Review<T = RecoveryOwner> {
    Preparing(String),
    Ready {
        epoch: String,
        owner: T,
        expiry: AbortHandle,
    },
    Restoring(String),
    Closing(String),
    ReleaseFailed {
        epoch: String,
        error: String,
    },
}
// One shared actual owner slot for writer recovery and explicitly selected
// bootstrap actions. Renderer selection is a closed action, never a path/proof.
enum RecoveryOwner {
    Writer(client_config::WriterRecovery),
    Empty(client_config::EmptyBootstrapReview),
    Import(client_config::LegacyImportReview),
}
impl RecoveryOwner {
    fn summary(&self) -> &'static str {
        match self {
            Self::Writer(v) => v.summary(),
            Self::Empty(v) => v.summary(),
            Self::Import(v) => v.summary(),
        }
    }
    fn close(self) -> Result<(), client_config::ClientConfigError> {
        match self {
            Self::Writer(v) => v.close(),
            Self::Empty(v) => v.cancel(),
            Self::Import(v) => v.cancel(),
        }
    }
    fn restore(self) -> Result<client_config::ClientConfigGuard, client_config::ClientConfigError> {
        match self {
            Self::Writer(v) => v.restore(),
            Self::Empty(v) => v.apply(),
            Self::Import(v) => v.apply(),
        }
    }
}
#[derive(Clone, Copy, serde::Deserialize)]
#[serde(rename_all = "kebab-case")]
pub(crate) enum BootstrapSelection {
    Empty,
    ImportCurrentSource,
    ResumeImport,
}
impl<T> Review<T> {
    fn epoch(&self) -> &str {
        match self {
            Self::Preparing(epoch)
            | Self::Restoring(epoch)
            | Self::Closing(epoch)
            | Self::ReleaseFailed { epoch, .. }
            | Self::Ready { epoch, .. } => epoch,
        }
    }
}
fn finish_release<T>(slot: &mut Option<Review<T>>, epoch: &str, result: &Result<(), String>) {
    if matches!(slot.as_ref(), Some(Review::Closing(current)) if current == epoch) {
        *slot = match result {
            Ok(()) => None,
            Err(error) => Some(Review::ReleaseFailed {
                epoch: epoch.to_owned(),
                error: format!("Recovery ownership could not be released. Close and reopen the OrbWin app before reviewing again. {error}"),
            }),
        };
    }
}
// A stale cancel/expiry cannot take a newer review or interrupt a restore. Move
// the actual owner out under the lock; release/restore it only after unlocking.
fn take_ready<T>(slot: &mut Option<Review<T>>, epoch: &str, restoring: bool) -> Option<T> {
    if !matches!(slot.as_ref(), Some(Review::Ready { epoch: current, .. }) if current == epoch) {
        return None;
    }
    let Some(Review::Ready { owner, expiry, .. }) = slot.take() else {
        unreachable!()
    };
    expiry.abort();
    if restoring {
        *slot = Some(Review::Restoring(epoch.to_owned()));
    } else {
        *slot = Some(Review::Closing(epoch.to_owned()));
    }
    Some(owner)
}
fn close_owner(owner: Option<RecoveryOwner>) -> Result<(), String> {
    owner.map_or(Ok(()), |owner| owner.close().map_err(|e| e.to_string()))
}
async fn release_ready(
    state: Reviews,
    epoch: String,
    owner: Option<RecoveryOwner>,
) -> Result<(), String> {
    let result = tokio::task::spawn_blocking(move || close_owner(owner))
        .await
        .map_err(|_| "Recovery release failed; pending records were retained.".to_string())
        .and_then(|value| value);
    let mut slot = state.0.lock().unwrap_or_else(|e| e.into_inner());
    finish_release(&mut slot, &epoch, &result);
    if let Some(Review::ReleaseFailed {
        epoch: failed,
        error,
    }) = slot.as_ref()
    {
        if failed == &epoch {
            return Err(error.clone());
        }
    }
    result
}
#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct Summary {
    epoch: String,
    summary: &'static str,
    expires_in_seconds: u32,
}

fn nonce() -> Result<String, String> {
    #[cfg(windows)]
    {
        #[link(name = "bcrypt")]
        unsafe extern "system" {
            fn BCryptGenRandom(
                algorithm: *mut std::ffi::c_void,
                buffer: *mut u8,
                length: u32,
                flags: u32,
            ) -> i32;
        }
        let mut bytes = [0u8; 32];
        if unsafe { BCryptGenRandom(std::ptr::null_mut(), bytes.as_mut_ptr(), 32, 2) } < 0 {
            return Err("Cannot create a private recovery review.".into());
        }
        Ok(bytes.iter().map(|b| format!("{b:02x}")).collect())
    }
    #[cfg(not(windows))]
    Err("Recovery requires the Windows desktop application.".into())
}

#[tauri::command]
pub(crate) async fn client_recovery_review(
    state: tauri::State<'_, Reviews>,
) -> Result<Option<Summary>, String> {
    review_selected(state.inner().clone(), None).await
}
#[tauri::command]
pub(crate) async fn client_bootstrap_review(
    state: tauri::State<'_, Reviews>,
    selection: BootstrapSelection,
) -> Result<Option<Summary>, String> {
    review_selected(state.inner().clone(), Some(selection)).await
}
async fn review_selected(
    state: Reviews,
    selection: Option<BootstrapSelection>,
) -> Result<Option<Summary>, String> {
    let epoch = nonce()?;
    {
        let mut slot = state.0.lock().map_err(|_| "Recovery owner unavailable.")?;
        if let Some(Review::ReleaseFailed { error, .. }) = slot.as_ref() {
            return Err(error.clone());
        }
        if slot.is_some() {
            return Err("Another recovery review or restore is active.".into());
        }
        *slot = Some(Review::Preparing(epoch.clone()));
    }
    let (send, receive) = oneshot::channel();
    tokio::spawn(async move {
        let prepared = tokio::task::spawn_blocking(move || {
            let docker = runtime::docker()?;
            let credentials = runtime::credentials()?;
            let prepared = match selection {
                None => client_config::prepare_writer_recovery(docker, credentials)
                    .map(RecoveryOwner::Writer),
                Some(BootstrapSelection::Empty) => {
                    client_config::prepare_empty_bootstrap(docker, credentials)
                        .map(RecoveryOwner::Empty)
                }
                Some(BootstrapSelection::ImportCurrentSource) => {
                    client_config::prepare_legacy_import(
                        docker,
                        credentials,
                        client_config::ImportAction::ReviewCurrentSource,
                    )
                    .map(RecoveryOwner::Import)
                }
                Some(BootstrapSelection::ResumeImport) => client_config::prepare_legacy_import(
                    docker,
                    credentials,
                    client_config::ImportAction::ResumeReviewedImport,
                )
                .map(RecoveryOwner::Import),
            };
            match prepared {
                Ok(owner) => Ok(Some(owner)),
                Err(error) if error.kind == client_config::ErrorKind::NoInterruptedWrite => {
                    Ok(None)
                }
                Err(error) => Err(error.to_string()),
            }
        })
        .await
        .unwrap_or_else(|_| {
            Err("Recovery review worker failed; pending records were retained.".into())
        });
        let result = match prepared {
            Ok(Some(owner)) => {
                let summary = owner.summary();
                let weak = Arc::downgrade(&state.0);
                let expiry_epoch = epoch.clone();
                let timer = tokio::spawn(async move {
                    tokio::time::sleep(std::time::Duration::from_secs(300)).await;
                    if let Some(state) = weak.upgrade() {
                        let owner = {
                            let mut slot = state.lock().unwrap_or_else(|e| e.into_inner());
                            if matches!(slot.as_ref(), Some(Review::Ready { epoch, .. }) if epoch == &expiry_epoch)
                            {
                                // This is the expiry task itself; dropping its
                                // handle must not abort the checked release below.
                                match slot.take() {
                                    Some(Review::Ready { owner, .. }) => {
                                        *slot = Some(Review::Closing(expiry_epoch.clone()));
                                        Some(owner)
                                    }
                                    _ => unreachable!(),
                                }
                            } else {
                                None
                            }
                        };
                        // Native release may block. Never do it on a reactor thread.
                        let result = release_ready(Reviews(state), expiry_epoch, owner).await;
                        if result.is_err() {
                            eprintln!("Recovery review expired; checked native release failed. Pending records retained.");
                        }
                    }
                });
                let mut slot = state.0.lock().unwrap_or_else(|e| e.into_inner());
                *slot = Some(Review::Ready {
                    epoch: epoch.clone(),
                    owner,
                    expiry: timer.abort_handle(),
                });
                Ok(Some(Summary {
                    epoch: epoch.clone(),
                    summary,
                    expires_in_seconds: 300,
                }))
            }
            Ok(None) => {
                let mut slot = state.0.lock().unwrap_or_else(|e| e.into_inner());
                if slot.as_ref().is_some_and(|v| v.epoch() == epoch) {
                    *slot = None;
                }
                Ok(None)
            }
            Err(error) => {
                let mut slot = state.0.lock().unwrap_or_else(|e| e.into_inner());
                if slot.as_ref().is_some_and(|v| v.epoch() == epoch) {
                    *slot = None;
                }
                Err(error)
            }
        };
        if send.send(result).is_err() {
            let owner = {
                let mut slot = state.0.lock().unwrap_or_else(|e| e.into_inner());
                take_ready(&mut slot, &epoch, false)
            };
            let _ = release_ready(state, epoch, owner).await;
        }
    });
    receive
        .await
        .map_err(|_| "Recovery review worker unavailable.")?
}

#[tauri::command]
pub(crate) async fn client_recovery_cancel(
    state: tauri::State<'_, Reviews>,
    epoch: String,
) -> Result<(), String> {
    let state = state.inner().clone();
    let owner = {
        let mut slot = state.0.lock().map_err(|_| "Recovery owner unavailable.")?;
        if let Some(Review::ReleaseFailed {
            epoch: failed,
            error,
        }) = slot.as_ref()
        {
            if failed == &epoch {
                return Err(error.clone());
            }
        }
        take_ready(&mut slot, &epoch, false)
    };
    if owner.is_none() {
        return Ok(());
    }
    let (send, receive) = oneshot::channel();
    tokio::spawn(async move {
        let _ = send.send(release_ready(state, epoch, owner).await);
    });
    receive
        .await
        .map_err(|_| "Recovery release worker unavailable.")?
}

#[tauri::command]
pub(crate) async fn client_recovery_restore(
    state: tauri::State<'_, Reviews>,
    epoch: String,
) -> Result<(), String> {
    let state = state.inner().clone();
    let owner = {
        let mut slot = state.0.lock().map_err(|_| "Recovery owner unavailable.")?;
        take_ready(&mut slot, &epoch, true)
            .ok_or("Recovery review expired or was already consumed. Review again.")?
    };
    let (send, receive) = oneshot::channel();
    tokio::spawn(async move {
        let result = tokio::task::spawn_blocking(move || {
            owner
                .restore()
                .and_then(|guard| guard.close())
                .map_err(|e| e.to_string())
        })
        .await
        .unwrap_or_else(|_| {
            Err("Recovery worker failed; retain pending records and review again.".into())
        });
        {
            let mut slot = state.0.lock().unwrap_or_else(|e| e.into_inner());
            if matches!(slot.as_ref(), Some(Review::Restoring(current)) if current == &epoch) {
                *slot = None;
            }
        }
        let _ = send.send(result);
    });
    receive.await.map_err(|_| "Recovery worker unavailable.")?
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicUsize, Ordering};
    struct Owner(Arc<AtomicUsize>);
    impl Drop for Owner {
        fn drop(&mut self) {
            self.0.fetch_add(1, Ordering::SeqCst);
        }
    }
    #[tokio::test]
    async fn stale_cancel_and_expiry_cannot_take_current_or_restoring_owner() {
        let drops = Arc::new(AtomicUsize::new(0));
        let timer = tokio::spawn(std::future::pending::<()>());
        let mut slot = Some(Review::Ready {
            epoch: "current".into(),
            owner: Owner(drops.clone()),
            expiry: timer.abort_handle(),
        });
        assert!(take_ready(&mut slot, "stale", false).is_none());
        assert_eq!(drops.load(Ordering::SeqCst), 0);
        let owner = take_ready(&mut slot, "current", true).expect("actual review owner");
        assert!(take_ready(&mut slot, "current", true).is_none());
        assert!(take_ready(&mut slot, "current", false).is_none());
        assert!(matches!(slot, Some(Review::Restoring(ref epoch)) if epoch == "current"));
        assert_eq!(drops.load(Ordering::SeqCst), 0);
        drop(owner);
        assert_eq!(drops.load(Ordering::SeqCst), 1);
        assert!(timer.await.unwrap_err().is_cancelled());
    }
    #[tokio::test]
    async fn cancel_releases_one_owner_and_aborts_expiry() {
        let drops = Arc::new(AtomicUsize::new(0));
        let timer = tokio::spawn(std::future::pending::<()>());
        let mut slot = Some(Review::Ready {
            epoch: "current".into(),
            owner: Owner(drops.clone()),
            expiry: timer.abort_handle(),
        });
        let owner = take_ready(&mut slot, "current", false);
        assert!(matches!(slot, Some(Review::Closing(ref epoch)) if epoch == "current"));
        drop(owner);
        assert!(take_ready(&mut slot, "current", false).is_none());
        assert_eq!(drops.load(Ordering::SeqCst), 1);
        assert!(timer.await.unwrap_err().is_cancelled());
    }
    #[test]
    fn failed_release_remains_visible_and_stale_completion_cannot_clear_it() {
        let mut slot: Option<Review<()>> = Some(Review::Closing("expired".into()));
        finish_release(
            &mut slot,
            "expired",
            &Err("checked native close failed".into()),
        );
        assert!(
            matches!(slot.as_ref(), Some(Review::ReleaseFailed { epoch, error })
            if epoch == "expired" && error.contains("OrbWin app") && error.contains("checked native close failed"))
        );
        assert!(take_ready(&mut slot, "expired", false).is_none());
        finish_release(&mut slot, "stale", &Ok(()));
        finish_release(&mut slot, "expired", &Ok(()));
        assert!(matches!(slot, Some(Review::ReleaseFailed { .. })));
        let mut known: Option<Review<()>> = Some(Review::Closing("known".into()));
        finish_release(&mut known, "known", &Ok(()));
        assert!(known.is_none());
    }
}
