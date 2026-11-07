//! Failure-only launch DATA; no process/file/native authority is constructed.
#[derive(Default)]
pub(crate) struct LaunchState {
    pub(crate) entered: bool,
    admitted: bool,
    unknown: bool,
}
impl LaunchState {
    pub(crate) fn enter(&mut self) -> bool {
        if self.entered || self.unknown {
            return false;
        }
        self.entered = true;
        true
    }
    pub(crate) fn admit(&mut self) -> bool {
        if !self.entered || self.unknown || self.admitted {
            return false;
        }
        self.admitted = true;
        true
    }
    pub(crate) fn fail(&mut self) {
        self.unknown = true
    }
    pub(crate) fn may_send_initial(&self) -> bool {
        self.entered && self.admitted && !self.unknown
    }
}
#[derive(Default)]
pub(crate) struct IoSettlement {
    eof: bool,
    writer_joined: bool,
    terminal: bool,
    unknown: bool,
}
impl IoSettlement {
    pub(crate) fn end_output(&mut self) {
        self.eof = true
    }
    pub(crate) fn writer_join(&mut self, positive: bool) {
        if !self.eof || !positive {
            self.unknown = true
        } else {
            self.writer_joined = true
        }
    }
    pub(crate) fn terminal(&mut self) {
        self.terminal = true
    }
    pub(crate) fn may_release(&self) -> bool {
        self.eof && self.writer_joined && self.terminal && !self.unknown
    }
}
#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn partial_birth_never_becomes_unstarted_or_admitted() {
        for cut in 0..4 {
            let mut s = LaunchState::default();
            assert!(!s.may_send_initial());
            assert!(s.enter());
            if cut < 3 {
                s.fail();
                assert!(!s.admit());
                assert!(!s.may_send_initial())
            } else {
                assert!(s.admit());
                assert!(s.may_send_initial())
            }
            assert!(!s.enter());
            assert!(!s.admit())
        }
    }
    #[test]
    fn eof_before_wait_and_writer_join_before_release() {
        let mut s = IoSettlement::default();
        s.terminal();
        assert!(!s.may_release());
        s.end_output();
        assert!(!s.may_release());
        s.writer_join(true);
        assert!(s.may_release());
        let mut bad = IoSettlement::default();
        bad.writer_join(true);
        bad.end_output();
        bad.terminal();
        assert!(!bad.may_release())
    }
    #[test]
    fn failed_worker_join_is_sticky() {
        let mut s = IoSettlement::default();
        s.end_output();
        s.writer_join(false);
        s.writer_join(true);
        s.terminal();
        assert!(!s.may_release())
    }
    #[test]
    fn ui_loss_does_not_own_original_worker() {
        use std::sync::{
            atomic::{AtomicBool, Ordering},
            mpsc, Arc,
        };
        let (tx, rx) = mpsc::channel::<Vec<u8>>();
        let exited = Arc::new(AtomicBool::new(false));
        let child = exited.clone();
        let work = std::thread::spawn(move || {
            let mut s = IoSettlement::default();
            assert!(rx.recv().is_err());
            s.end_output();
            s.writer_join(true);
            assert!(!s.may_release());
            child.store(true, Ordering::Release);
            s.terminal();
            assert!(s.may_release());
        });
        drop(tx);
        work.join().unwrap();
        assert!(exited.load(Ordering::Acquire))
    }
}

// Native reply DATA classification only: callers still need actual fixed call.
#[derive(Debug, PartialEq, Eq)]
pub(crate) enum HandleReply {
    Owned,
    NoOutput,
    Unknown,
}
fn valid_handle(h: usize) -> bool {
    h != 0 && h < usize::MAX - 16
}
fn terminal_error(e: u32) -> bool {
    matches!(e, 5 | 6 | 8 | 14 | 87 | 1450)
}
pub(crate) fn duplicate_reply(result: i32, h: usize, error: u32) -> HandleReply {
    if result != 0 {
        if valid_handle(h) {
            HandleReply::Owned
        } else {
            HandleReply::Unknown
        }
    } else if h == 0 && terminal_error(error) {
        HandleReply::NoOutput
    } else {
        HandleReply::Unknown
    }
}
pub(crate) fn file_reply(h: usize, error: u32) -> HandleReply {
    if valid_handle(h) {
        HandleReply::Owned
    } else if h == usize::MAX && terminal_error(error) {
        HandleReply::NoOutput
    } else {
        HandleReply::Unknown
    }
}
#[derive(Default)]
pub(crate) struct AcquisitionState {
    pub(crate) unknown: bool,
}
impl AcquisitionState {
    pub(crate) fn fail(&mut self) {
        self.unknown = true
    }
    pub(crate) fn fence(&mut self, latch: &std::sync::atomic::AtomicBool) {
        self.unknown = true;
        latch.store(true, std::sync::atomic::Ordering::Release);
    }
}
#[cfg(test)]
mod acquisition_tests {
    use super::*;
    #[test]
    fn duplicate_partial_false_never_owns_close_slot() {
        for h in [1, 42, usize::MAX - 16, usize::MAX - 1, usize::MAX] {
            assert_eq!(duplicate_reply(0, h, 5), HandleReply::Unknown)
        }
        for e in [0, 996, 997, 0xffff_ffff] {
            assert_eq!(duplicate_reply(0, 0, e), HandleReply::Unknown)
        }
        assert_eq!(duplicate_reply(0, 0, 5), HandleReply::NoOutput);
        assert_eq!(duplicate_reply(2, 42, 997), HandleReply::Owned);
        assert_eq!(duplicate_reply(1, 0, 0), HandleReply::Unknown);
        assert_eq!(duplicate_reply(1, usize::MAX, 0), HandleReply::Unknown)
    }
    #[test]
    fn returned_handle_invalid_failure_is_distinct_from_null() {
        assert_eq!(file_reply(usize::MAX, 5), HandleReply::NoOutput);
        assert_eq!(file_reply(0, 5), HandleReply::Unknown);
        assert_eq!(file_reply(usize::MAX, 997), HandleReply::Unknown);
        assert_eq!(file_reply(42, 997), HandleReply::Owned)
    }
    #[test]
    fn direct_unknown_park_fences_future_admission() {
        let latch = std::sync::atomic::AtomicBool::new(false);
        let mut a = AcquisitionState::default();
        a.fence(&latch);
        assert!(a.unknown && latch.load(std::sync::atomic::Ordering::Acquire));
        // A later no-output DATA reply cannot clear original unknown/fence.
        assert_eq!(duplicate_reply(0, 0, 5), HandleReply::NoOutput);
        assert!(latch.load(std::sync::atomic::Ordering::Acquire));
    }
    #[test]
    fn uncertainty_stops_previous_owned_suffix() {
        let mut a = AcquisitionState::default();
        let mut closes = 0;
        let replies = [
            HandleReply::Owned,
            HandleReply::Unknown,
            HandleReply::NoOutput,
        ];
        for reply in replies {
            if reply == HandleReply::Unknown {
                a.fail()
            }
            if !a.unknown && reply == HandleReply::NoOutput {
                closes += 1
            }
        }
        assert!(a.unknown);
        assert_eq!(closes, 0)
    }
}

// Failure routing DATA only. AttemptClose is NOT a retirement/no-birth fact.
#[derive(Debug, PartialEq, Eq)]
pub(crate) enum FailureRoute {
    RetainAndReport,
    AttemptCheckedPrebirthClose,
}
pub(crate) fn failure_route(
    entered: bool,
    acquisition_unknown: bool,
    security_unknown: bool,
) -> FailureRoute {
    if entered || acquisition_unknown || security_unknown {
        FailureRoute::RetainAndReport
    } else {
        FailureRoute::AttemptCheckedPrebirthClose
    }
}
#[cfg(test)]
mod failure_dispatch_tests {
    use super::*;
    #[test]
    fn security_failure_reports_without_dropping_original_owner() {
        use std::sync::{
            atomic::{AtomicUsize, Ordering},
            mpsc, Arc,
        };
        struct Original(Arc<AtomicUsize>);
        impl Drop for Original {
            fn drop(&mut self) {
                self.0.fetch_add(1, Ordering::SeqCst);
            }
        }
        let dropped = Arc::new(AtomicUsize::new(0));
        let original = Original(dropped.clone());
        let (tx, rx) = mpsc::sync_channel(1);
        let (done, finish) = mpsc::channel();
        let keeper = std::thread::spawn(move || {
            assert_eq!(
                failure_route(false, false, true),
                FailureRoute::RetainAndReport
            );
            tx.send("security state unknown").unwrap();
            // Dummy test-only object; production keeper has no release command.
            let _ = finish.recv();
            drop(original);
        });
        assert_eq!(
            rx.recv_timeout(std::time::Duration::from_secs(2)).unwrap(),
            "security state unknown"
        );
        assert_eq!(dropped.load(Ordering::SeqCst), 0);
        done.send(()).unwrap();
        keeper.join().unwrap();
        assert_eq!(dropped.load(Ordering::SeqCst), 1);
    }
    #[test]
    fn checked_prebirth_close_remains_separate_from_retention() {
        assert_eq!(
            failure_route(false, false, false),
            FailureRoute::AttemptCheckedPrebirthClose
        );
        for (birth, acq, security) in [
            (true, false, false),
            (false, true, false),
            (false, false, true),
            (true, true, true),
        ] {
            assert_eq!(
                failure_route(birth, acq, security),
                FailureRoute::RetainAndReport
            )
        }
    }
}

#[cfg(test)]
mod close_failure_delivery_tests {
    use super::*;
    #[test]
    fn failed_close_preserves_cached_prefix_and_reports_without_suffix() {
        use std::sync::{
            atomic::{AtomicUsize, Ordering},
            mpsc, Arc,
        };
        struct Original {
            closed: [bool; 3],
            attempted: [bool; 3],
            drops: Arc<AtomicUsize>,
        }
        impl Drop for Original {
            fn drop(&mut self) {
                self.drops.fetch_add(1, Ordering::SeqCst);
            }
        }
        let drops = Arc::new(AtomicUsize::new(0));
        let mut original = Original {
            closed: [false; 3],
            attempted: [false; 3],
            drops: drops.clone(),
        };
        let fence = Arc::new(std::sync::atomic::AtomicBool::new(false));
        let mut acquisition = AcquisitionState::default();
        for i in 0..3 {
            original.attempted[i] = true;
            if i == 1 {
                acquisition.fence(&fence);
                break;
            }
            original.closed[i] = true;
        }
        assert_eq!(original.closed, [true, false, false]);
        assert_eq!(original.attempted, [true, true, false]);
        let (tx, rx) = mpsc::sync_channel(1);
        let (stop, finish) = mpsc::channel();
        let keeper = std::thread::spawn(move || {
            // Test-only release; native producer has no release command.
            tx.send((original.closed, original.attempted,close_failure_error("Ordinary native client token required.","Persistent security primary token CloseHandle unknown; original graph retained."))).unwrap();
            let _ = finish.recv();
            drop(original);
        });
        let snapshot = rx.recv_timeout(std::time::Duration::from_secs(2)).unwrap();
        assert_eq!(
            (snapshot.0, snapshot.1),
            ([true, false, false], [true, true, false])
        );
        assert!(snapshot
            .2
            .contains("Ordinary native client token required."));
        assert!(snapshot
            .2
            .contains("cleanup: Persistent security primary token CloseHandle"));
        assert!(fence.load(Ordering::Acquire));
        assert_eq!(drops.load(Ordering::Acquire), 0);
        stop.send(()).unwrap();
        keeper.join().unwrap();
        assert_eq!(drops.load(Ordering::Acquire), 1);
    }
}

// Internal diagnostics only, never native lifecycle/permission classification.
pub(crate) fn close_failure_error(original: &str, cleanup: &str) -> String {
    let original: String = original.chars().take(512).collect();
    let cleanup: String = cleanup.chars().take(512).collect();
    format!(
        "{original}; cleanup: {cleanup}. Cleanup could not be confirmed; further setup is blocked."
    )
}
#[cfg(test)]
mod cleanup_error_tests {
    use super::*;
    #[test]
    fn actual_cleanup_reason_and_native_code_are_preserved_and_bounded() {
        let text = close_failure_error(
            "Ordinary native client token required.",
            "Native client persistent checked close failed: The handle is invalid. (os error 6)",
        );
        assert!(text.contains("Ordinary native client token required."));
        assert!(text.contains("cleanup:"));
        assert!(text.contains("os error 6"));
        assert!(text.contains("Cleanup could not be confirmed"));
        assert!(text.contains("further setup is blocked"));
        let long = "\u{03bb}".repeat(900);
        let bounded = close_failure_error(&long, &long);
        assert_eq!(bounded.matches('\u{03bb}').count(), 1024);
        assert!(bounded.chars().count() < 1200);
    }
}
