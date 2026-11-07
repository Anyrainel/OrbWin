//! Fixed staging decisions only. No filesystem/native ownership is issued here.

pub(crate) const ROLE_NAMES: [&str; 4] = [
    "rdd.exe",
    "rdd-forwarder.exe",
    "rdd-candidate.json",
    "rdd-candidate.sig",
];
pub(crate) const ROLE_LIMITS: [u64; 4] = [512 << 20, 4 << 20, 4096, 88];

pub(crate) fn valid_size(role: usize, size: u64) -> bool {
    role < ROLE_NAMES.len() && size > 0 && size <= ROLE_LIMITS[role] && (role != 3 || size == 88)
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum SlotLookup {
    Opened,
    Missing,
    Refused,
}
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum SlotAction {
    OpenExisting,
    CreateExclusive,
    Refuse,
}
pub(crate) fn slot_action(lookup: SlotLookup) -> SlotAction {
    match lookup {
        SlotLookup::Opened => SlotAction::OpenExisting,
        SlotLookup::Missing => SlotAction::CreateExclusive,
        SlotLookup::Refused => SlotAction::Refuse,
    }
}
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum CaptureAction {
    OpenOnly,
    PublishNonReplacing,
}

// The actual constructor establishes freshness from native outcomes. This only
// fixes the subsequent four actions; an existing incomplete row stops the loop.
pub(crate) fn capture_roles<E>(
    action: CaptureAction,
    mut capture: impl FnMut(usize, CaptureAction) -> Result<(), E>,
) -> Result<(), E> {
    for role in 0..ROLE_NAMES.len() {
        capture(role, action)?;
    }
    Ok(())
}

#[derive(Clone, Copy, PartialEq, Eq)]
enum MemoPhase {
    Fresh,
    Preparing,
    Ready,
    Refused,
}
pub(crate) struct Memo<T> {
    phase: MemoPhase,
    held: Option<T>,
    error: String,
}
impl<T> Default for Memo<T> {
    fn default() -> Self {
        Self {
            phase: MemoPhase::Fresh,
            held: None,
            error: "Candidate staging did not complete.".into(),
        }
    }
}
impl<T> Memo<T> {
    pub(crate) fn cached(&self) -> Option<Result<&T, &str>> {
        match self.phase {
            MemoPhase::Fresh => None,
            MemoPhase::Ready => Some(self.held.as_ref().ok_or(self.error.as_str())),
            MemoPhase::Preparing | MemoPhase::Refused => Some(Err(&self.error)),
        }
    }
    pub(crate) fn begin(&mut self) -> bool {
        if self.phase != MemoPhase::Fresh {
            return false;
        }
        self.phase = MemoPhase::Preparing;
        true
    }
    pub(crate) fn refuse(&mut self, error: String) {
        self.error = error;
        self.phase = MemoPhase::Refused;
        // Keep any prior successful owner even when a replay recheck failed.
    }
    pub(crate) fn admit(&mut self, held: T) -> bool {
        if self.phase != MemoPhase::Preparing || self.held.is_some() {
            if self.held.is_none() {
                self.held = Some(held);
            }
            self.phase = MemoPhase::Refused;
            return false;
        }
        self.held = Some(held);
        self.phase = MemoPhase::Ready;
        true
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::{
        atomic::{AtomicUsize, Ordering},
        Arc, Mutex,
    };

    #[test]
    fn existing_missing_or_conflicting_role_never_publishes() {
        for cut in 0..ROLE_NAMES.len() {
            for reason in ["missing", "conflicting"] {
                let mut opened = Vec::new();
                let mut published = 0;
                let result = capture_roles(CaptureAction::OpenOnly, |role, action| {
                    opened.push(role);
                    if action == CaptureAction::PublishNonReplacing {
                        published += 1;
                    }
                    if role == cut {
                        Err(reason)
                    } else {
                        Ok(())
                    }
                });
                assert_eq!(result, Err(reason));
                assert_eq!(opened, (0..=cut).collect::<Vec<_>>());
                assert_eq!(published, 0);
            }
        }
    }

    #[test]
    fn native_missing_selects_only_exclusive_create() {
        assert_eq!(slot_action(SlotLookup::Opened), SlotAction::OpenExisting);
        assert_eq!(slot_action(SlotLookup::Refused), SlotAction::Refuse);
        // Two contenders can both observe Missing. Only the native CREATE
        // winner reaches the fixed publish sequence; collision is terminal.
        for create_succeeded in [false, true] {
            assert_eq!(
                slot_action(SlotLookup::Missing),
                SlotAction::CreateExclusive
            );
            let mut published = 0;
            if create_succeeded {
                capture_roles(CaptureAction::PublishNonReplacing, |_, action| {
                    assert_eq!(action, CaptureAction::PublishNonReplacing);
                    published += 1;
                    Ok::<_, ()>(())
                })
                .unwrap();
            }
            assert_eq!(published, if create_succeeded { 4 } else { 0 });
        }
    }

    #[test]
    fn fixed_metadata_bounds() {
        assert!(!valid_size(2, 0));
        assert!(valid_size(2, 1));
        assert!(valid_size(2, 4096));
        assert!(!valid_size(2, 4097));
        for size in [0, 87, 89] {
            assert!(!valid_size(3, size));
        }
        assert!(valid_size(3, 88));
        assert!(!valid_size(4, 1));
        assert!(valid_size(0, 512 << 20));
        assert!(!valid_size(0, (512 << 20) + 1));
        assert!(valid_size(1, 4 << 20));
        assert!(!valid_size(1, (4 << 20) + 1));
    }

    #[test]
    fn memo_prearms_failure_and_never_retries() {
        let mut memo = Memo::<()>::default();
        assert!(memo.cached().is_none());
        assert!(memo.begin());
        assert!(memo.cached().unwrap().is_err());
        memo.refuse("original refusal".into());
        assert_eq!(memo.cached(), Some(Err("original refusal")));
        assert!(!memo.begin());
        assert!(!memo.admit(()));
        assert!(memo.cached().unwrap().is_err());
    }

    #[test]
    fn memo_retains_owner_on_success_and_failed_recheck() {
        let owner = Arc::new(());
        let weak = Arc::downgrade(&owner);
        let mut memo = Memo::default();
        assert!(memo.begin());
        assert!(memo.admit(owner.clone()));
        drop(owner);
        assert!(weak.upgrade().is_some());
        assert!(memo.cached().unwrap().is_ok());
        assert!(!memo.begin());
        memo.refuse("recheck unknown".into());
        assert!(memo.cached().unwrap().is_err());
        assert!(weak.upgrade().is_some());
    }

    #[test]
    fn panic_poison_prevents_waiter_from_reentering() {
        let memo = Arc::new(Mutex::new(Memo::<()>::default()));
        let effects = Arc::new(AtomicUsize::new(0));
        let actor_memo = memo.clone();
        let actor_effects = effects.clone();
        assert!(std::thread::spawn(move || {
            let mut guard = actor_memo.lock().unwrap();
            assert!(guard.begin());
            actor_effects.fetch_add(1, Ordering::SeqCst);
            panic!("lost preparation reply");
        })
        .join()
        .is_err());
        assert!(memo.lock().is_err());
        assert_eq!(effects.load(Ordering::SeqCst), 1);
    }
}
