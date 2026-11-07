//! Existing launch ownership mechanics, shared by production and inert tests.
//! No native owner is manufactured here; the production retire function is fixed.
pub(super) struct LaunchOwner<C, L, F: Fn()> {
    pub(super) child: Option<C>,
    pub(super) lease: Option<L>,
    pub(super) retire: fn(C, Option<L>),
    pub(super) release: F,
}
impl<C, L, F: Fn()> LaunchOwner<C, L, F> {
    pub(super) fn handoff(mut self) {
        // Existing positive ACK path: Go independently holds its own resources.
        self.child.take();
        self.lease.take();
    }
}
impl<C, L, F: Fn()> Drop for LaunchOwner<C, L, F> {
    fn drop(&mut self) {
        let Some(child) = self.child.take() else {
            (self.release)();
            return;
        };
        (self.retire)(child, self.lease.take());
    }
}
/// Existing sole-waiter payload. Unknown settlement keeps these actual values;
/// it never produces a release/terminal receipt from a boolean or timeout.
pub(super) struct Retirement<C, L> {
    pub(super) child: C,
    pub(super) lease: Option<L>,
}
impl<C, L> Retirement<C, L> {
    pub(super) fn retain_unknown(self) {
        std::mem::forget(self);
    }
}
pub(super) const RPC_INPUT_LIMIT: usize = (128 << 10) + 4096;
pub(super) fn request_size_allowed(bytes: usize) -> bool {
    bytes <= RPC_INPUT_LIMIT
}

pub(super) fn clean_original_exit(code: Option<i32>, success: bool) -> bool {
    success && code == Some(0)
}

/// Closed startup restrictions. Ordinary production callers keep their original
/// argv; the cfg(test) choices only restrict the same product start path.
pub(super) enum StartupMode {
    Ordinary,
    #[cfg(test)]
    RequireNew,
    #[cfg(test)]
    RequireDisabled(String),
}
impl StartupMode {
    pub(super) fn arguments(&self) -> Result<Vec<&str>, &'static str> {
        match self {
            Self::Ordinary => Ok(Vec::new()),
            #[cfg(test)]
            Self::RequireNew => Ok(vec!["--require-new-sdk-state"]),
            #[cfg(test)]
            Self::RequireDisabled(fingerprint) if valid_fingerprint(fingerprint) => {
                Ok(vec!["--require-disabled-sdk-state", fingerprint])
            }
            #[cfg(test)]
            Self::RequireDisabled(_) => Err("Disabled SDK fingerprint is not canonical."),
        }
    }
    pub(super) fn check_existing(&self) -> Result<(), &'static str> {
        match self {
            Self::Ordinary => Ok(()),
            #[cfg(test)]
            _ => Err("Qualification found an existing service; no mutation or stop is permitted."),
        }
    }
    pub(super) fn restricted(&self) -> bool {
        !matches!(self, Self::Ordinary)
    }
}

/// Comparison DATA from an ACK already checked against the same original child.
/// This value supplies no native process access or retirement permission.
#[cfg(test)]
#[derive(Clone, PartialEq, Eq)]
pub(super) struct ExpectedServer {
    pub(super) pid: u32,
    pub(super) creation_time: String,
}
#[cfg(test)]
impl ExpectedServer {
    pub(super) fn new(pid: u32, creation: &str) -> Option<Self> {
        let value = creation.parse::<u64>().ok()?;
        if pid == 0 || value == 0 || value.to_string() != creation {
            return None;
        }
        Some(Self {
            pid,
            creation_time: creation.to_owned(),
        })
    }
}
#[cfg(test)]
pub(super) fn valid_fingerprint(value: &str) -> bool {
    value.len() == 64
        && value
            .bytes()
            .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
}
#[cfg(test)]
pub(super) enum ReplyError<'a> {
    Absent,
    Text(&'a str),
    Malformed,
}
#[cfg(test)]
pub(super) fn successful_reply(code: i32, version: Option<u64>, error: ReplyError<'_>) -> bool {
    code == 0 && version == Some(1) && matches!(error, ReplyError::Absent | ReplyError::Text(""))
}
#[cfg(test)]
pub(super) fn disabled_reply_matches(
    actual_revision: Option<u64>,
    expected_revision: u64,
    enabled: Option<bool>,
    exact_settings: bool,
    fingerprint: Option<&str>,
) -> bool {
    actual_revision == Some(expected_revision)
        && expected_revision != 0
        && enabled == Some(false)
        && exact_settings
        && fingerprint.is_some_and(valid_fingerprint)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::{
        atomic::{AtomicUsize, Ordering},
        mpsc, Arc,
    };
    struct Payload {
        retired: mpsc::Sender<(Payload, Option<Lease>)>,
        drops: Arc<AtomicUsize>,
    }
    impl Drop for Payload {
        fn drop(&mut self) {
            self.drops.fetch_add(1, Ordering::SeqCst);
        }
    }
    struct Lease(Arc<AtomicUsize>);
    impl Drop for Lease {
        fn drop(&mut self) {
            self.0.fetch_add(1, Ordering::SeqCst);
        }
    }
    fn retire(child: Payload, lease: Option<Lease>) {
        let sender = child.retired.clone();
        sender
            .send((child, lease))
            .unwrap_or_else(|_| panic!("test receiver absent"));
    }
    #[test]
    fn dropped_owner_moves_exact_payload_and_lease_once_without_releasing_gate() {
        let (sender, receiver) = mpsc::channel();
        let children = Arc::new(AtomicUsize::new(0));
        let leases = Arc::new(AtomicUsize::new(0));
        let gates = Arc::new(AtomicUsize::new(0));
        let g = gates.clone();
        drop(LaunchOwner {
            child: Some(Payload {
                retired: sender,
                drops: children.clone(),
            }),
            lease: Some(Lease(leases.clone())),
            retire,
            release: move || {
                g.fetch_add(1, Ordering::SeqCst);
            },
        });
        assert_eq!(gates.load(Ordering::SeqCst), 0);
        assert_eq!(children.load(Ordering::SeqCst), 0);
        assert_eq!(leases.load(Ordering::SeqCst), 0);
        let retained = receiver.try_recv().expect("one original payload");
        assert!(receiver.try_recv().is_err());
        drop(retained);
        assert_eq!(children.load(Ordering::SeqCst), 1);
        assert_eq!(leases.load(Ordering::SeqCst), 1);
    }
    #[test]
    fn positive_handoff_preserves_existing_no_waiter_path_and_releases_gate() {
        let (sender, receiver) = mpsc::channel();
        let children = Arc::new(AtomicUsize::new(0));
        let leases = Arc::new(AtomicUsize::new(0));
        let gates = Arc::new(AtomicUsize::new(0));
        let g = gates.clone();
        LaunchOwner {
            child: Some(Payload {
                retired: sender,
                drops: children.clone(),
            }),
            lease: Some(Lease(leases.clone())),
            retire,
            release: move || {
                g.fetch_add(1, Ordering::SeqCst);
            },
        }
        .handoff();
        assert_eq!(gates.load(Ordering::SeqCst), 1);
        assert_eq!(children.load(Ordering::SeqCst), 1);
        assert_eq!(leases.load(Ordering::SeqCst), 1);
        assert!(receiver.try_recv().is_err());
    }
    #[test]
    fn failed_or_unreturned_settlement_retains_the_same_original_payload() {
        let (sender, receiver) = mpsc::channel();
        let children = Arc::new(AtomicUsize::new(0));
        let leases = Arc::new(AtomicUsize::new(0));
        let child = Payload {
            retired: sender,
            drops: children.clone(),
        };
        let weak = Arc::downgrade(&child.drops);
        drop(LaunchOwner {
            child: Some(child),
            lease: Some(Lease(leases.clone())),
            retire,
            release: || panic!("must not release before waiter"),
        });
        let retained = receiver.try_recv().unwrap();
        // An unknown native waiter must retain, not drop/retry this payload.
        Retirement {
            child: retained.0,
            lease: retained.1,
        }
        .retain_unknown();
        assert_eq!(children.load(Ordering::SeqCst), 0);
        assert_eq!(leases.load(Ordering::SeqCst), 0);
        assert!(weak.upgrade().is_some());
    }
    #[test]
    fn reservation_without_child_releases_once_and_drops_only_its_lease() {
        let leases = Arc::new(AtomicUsize::new(0));
        let gates = Arc::new(AtomicUsize::new(0));
        let g = gates.clone();
        let owner: LaunchOwner<Payload, Lease, _> = LaunchOwner {
            child: None,
            lease: Some(Lease(leases.clone())),
            retire,
            release: move || {
                g.fetch_add(1, Ordering::SeqCst);
            },
        };
        drop(owner);
        assert_eq!(gates.load(Ordering::SeqCst), 1);
        assert_eq!(leases.load(Ordering::SeqCst), 1);
    }
    #[test]
    fn stdin_request_policy_accepts_more_than_32k_and_refuses_true_limit_overflow() {
        assert!(request_size_allowed(32769));
        assert!(request_size_allowed(RPC_INPUT_LIMIT));
        assert!(!request_size_allowed(RPC_INPUT_LIMIT + 1));
        assert!(!request_size_allowed(usize::MAX));
    }
    #[test]
    fn a_success_boolean_or_missing_exit_code_is_not_positive_original_terminal() {
        assert!(clean_original_exit(Some(0), true));
        for (code, success) in [
            (None, true),
            (Some(1), true),
            (Some(0), false),
            (None, false),
        ] {
            assert!(!clean_original_exit(code, success));
        }
    }
    #[test]
    fn closed_startup_modes_preserve_product_argv_and_refuse_attached_qualification() {
        assert!(StartupMode::Ordinary.arguments().unwrap().is_empty());
        assert!(StartupMode::Ordinary.check_existing().is_ok());
        assert!(!StartupMode::Ordinary.restricted());
        assert_eq!(
            StartupMode::RequireNew.arguments().unwrap(),
            ["--require-new-sdk-state"]
        );
        assert!(StartupMode::RequireNew.check_existing().is_err());
        let fingerprint = "a".repeat(64);
        let mode = StartupMode::RequireDisabled(fingerprint.clone());
        assert_eq!(
            mode.arguments().unwrap(),
            ["--require-disabled-sdk-state", &fingerprint]
        );
        assert!(mode.check_existing().is_err());
        for bad in [
            String::new(),
            "a".repeat(63),
            "a".repeat(65),
            "A".repeat(64),
            "g".repeat(64),
            "0".repeat(63) + "\n",
        ] {
            assert!(StartupMode::RequireDisabled(bad).arguments().is_err());
        }
    }
    #[test]
    fn expected_generation_preserves_full_uint64_and_rejects_noncanonical_input() {
        let generation = ExpectedServer::new(u32::MAX, "18446744073709551615").unwrap();
        assert_eq!(generation.creation_time, "18446744073709551615");
        assert_eq!(generation.pid, u32::MAX);
        assert!(ExpectedServer::new(0, "1").is_none());
        for value in [
            "",
            "0",
            "01",
            "+1",
            "-1",
            "1.0",
            " 1",
            "1 ",
            "18446744073709551616",
        ] {
            assert!(ExpectedServer::new(1, value).is_none(), "{value}");
        }
    }
    #[test]
    fn outer_success_never_hides_response_error_or_missing_version() {
        assert!(successful_reply(0, Some(1), ReplyError::Absent));
        assert!(successful_reply(0, Some(1), ReplyError::Text("")));
        assert!(!successful_reply(
            0,
            Some(1),
            ReplyError::Text("not applied")
        ));
        assert!(!successful_reply(0, Some(1), ReplyError::Malformed));
        assert!(!successful_reply(126, Some(1), ReplyError::Absent));
        assert!(!successful_reply(0, None, ReplyError::Absent));
        assert!(!successful_reply(0, Some(2), ReplyError::Absent));
    }
    #[test]
    fn disabled_reply_requires_exact_settings_revision_and_actor_fingerprint() {
        let sha = "a".repeat(64);
        assert!(valid_fingerprint(&"0".repeat(64))); // Go accepts any canonical 32-byte digest.
        assert!(disabled_reply_matches(
            Some(1),
            1,
            Some(false),
            true,
            Some(&sha)
        ));
        for (revision, expected, enabled, exact, fingerprint) in [
            (Some(0), 0, Some(false), true, Some(sha.as_str())),
            (Some(2), 1, Some(false), true, Some(sha.as_str())),
            (None, 1, Some(false), true, Some(sha.as_str())),
            (Some(1), 1, Some(true), true, Some(sha.as_str())),
            (Some(1), 1, Some(false), false, Some(sha.as_str())),
            (Some(1), 1, Some(false), true, None),
            (Some(1), 1, Some(false), true, Some("not-a-fingerprint")),
        ] {
            assert!(!disabled_reply_matches(
                revision,
                expected,
                enabled,
                exact,
                fingerprint
            ));
        }
    }
}
