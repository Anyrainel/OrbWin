//! Shared PURE pipe policy. Neither adapter can acquire authority from this DATA.
pub(crate) const PIPE_ACE_COUNT: usize = 3;
pub(crate) fn ordinary_sid(s: &str) -> bool {
    s.starts_with("S-1-5-21-") || s.starts_with("S-1-12-1-")
}
pub(crate) fn pipe_sddl(s: &str) -> String {
    format!("O:{s}D:P(A;;FA;;;{s})(A;;FA;;;SY)(A;;FA;;;BA)")
}
pub(crate) fn sid_offset(base: usize, length: usize, address: usize) -> Option<usize> {
    let off = address.checked_sub(base)?;
    if off > length || length - off < 8 {
        None
    } else {
        Some(off)
    }
}
pub(crate) fn sid_size(remaining: usize, count: u8) -> Option<usize> {
    let size = 8 + usize::from(count) * 4;
    if size > 68 || size > remaining {
        None
    } else {
        Some(size)
    }
}
pub(crate) fn descriptor_control(control: u16, revision: u32) -> bool {
    control & 0x9000 == 0x9000 && revision == 1
}
pub(crate) fn allowed_ace(kind: u8, flags: u8, mask: u32) -> bool {
    kind == 0 && flags == 0 && mask == 0x001f01ff
}
pub(crate) fn trustees_match(user: &str, trustees: &[String]) -> bool {
    let mut actual = trustees.to_vec();
    actual.sort();
    let mut expected = vec![user.to_string(), "S-1-5-18".into(), "S-1-5-32-544".into()];
    expected.sort();
    actual == expected
}
pub(crate) fn local_free_succeeded(result: usize, _error: u32) -> bool {
    result == 0
}
// Failure-only once-free ledger. Native result/error cells remain in original owner.
#[derive(Default)]
pub(crate) struct FreeState {
    entered: bool,
    closed: bool,
    unknown: bool,
}
impl FreeState {
    pub(crate) fn begin(&mut self) -> bool {
        if self.entered || self.closed || self.unknown {
            return false;
        }
        self.entered = true;
        true
    }
    pub(crate) fn finish(&mut self, result: usize, error: u32) -> bool {
        if !self.entered || self.closed || self.unknown || !local_free_succeeded(result, error) {
            self.unknown = true;
            return false;
        }
        self.closed = true;
        true
    }
}
pub(crate) fn thread_token_absent(result: i32, raw: usize, error: u32) -> bool {
    result == 0 && raw == 0 && error == 1008
}
pub(crate) fn scalar_token_query(result: i32, count: u32) -> bool {
    result != 0 && count == 4
}
pub(crate) fn user_token_query(result: i32, count: u32, capacity: usize, minimum: usize) -> bool {
    result != 0 && count as usize <= capacity && count as usize >= minimum
}
pub(crate) fn failed_allocation_size_consistent(result: i32, size: u32) -> bool {
    result != 0 || size == 0
}
#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn original_ordinary_sid_scope() {
        assert!(ordinary_sid("S-1-5-21-1-2-3-1001"));
        assert!(ordinary_sid("S-1-12-1-1-2-3-4"));
        for s in ["S-1-5-18", "S-1-5-32-544", "S-1-5-21", "s-1-5-21-1", ""] {
            assert!(!ordinary_sid(s))
        }
    }
    #[test]
    fn exact_descriptor_and_closed_trustees() {
        let u = "S-1-5-21-1-1001";
        assert_eq!(
            pipe_sddl(u),
            format!("O:{u}D:P(A;;FA;;;{u})(A;;FA;;;SY)(A;;FA;;;BA)")
        );
        assert!(trustees_match(
            u,
            &["S-1-5-32-544".into(), u.into(), "S-1-5-18".into()]
        ));
        assert!(!trustees_match(u, &[u.into(), u.into(), "S-1-5-18".into()]));
        assert!(!trustees_match(u, &[u.into(), "S-1-5-18".into()]));
        assert!(descriptor_control(0x9000, 1));
        assert!(!descriptor_control(0x8000, 1));
        assert!(!descriptor_control(0x9000, 2));
        assert!(allowed_ace(0, 0, 0x1f01ff));
        assert!(!allowed_ace(1, 0, 0x1f01ff));
        assert!(!allowed_ace(0, 1, 0x1f01ff));
        assert!(!allowed_ace(0, 0, 0x1301bf));
    }
    #[test]
    fn local_free_success_ignores_stale_error() {
        assert!(local_free_succeeded(0, 997));
        assert!(!local_free_succeeded(42, 0));
    }
    #[test]
    fn bounded_sid_header_and_subauthorities() {
        assert_eq!(sid_offset(100, 20, 99), None);
        assert_eq!(sid_offset(100, 7, 100), None);
        assert_eq!(sid_offset(100, 20, 112), Some(12));
        assert_eq!(sid_size(68, 15), Some(68));
        assert_eq!(sid_size(68, 16), None);
        assert_eq!(sid_size(11, 1), None);
    }
    #[test]
    fn partial_or_panicked_free_never_retries() {
        let mut bad = FreeState::default();
        assert!(bad.begin());
        assert!(!bad.finish(42, 0));
        assert!(!bad.begin());
        assert!(!bad.finish(0, 0));
        let mut cut = FreeState::default();
        assert!(cut.begin());
        assert!(!cut.begin());
        let mut good = FreeState::default();
        assert!(good.begin());
        assert!(good.finish(0, 997));
        assert!(!good.begin());
        assert!(!good.finish(0, 0));
    }
    #[test]
    fn actual_thread_token_absence_and_query_counts() {
        assert!(thread_token_absent(0, 0, 1008));
        for (r, h, e) in [(0, 42, 1008), (1, 0, 1008), (0, 0, 5), (0, 0, 997)] {
            assert!(!thread_token_absent(r, h, e))
        }
        assert!(scalar_token_query(2, 4));
        for (r, n) in [(0, 4), (1, 0), (1, 8)] {
            assert!(!scalar_token_query(r, n))
        }
        assert!(user_token_query(2, 16, 2048, 16));
        for (r, n) in [(0, 16), (1, 0), (1, 2049)] {
            assert!(!user_token_query(r, n, 2048, 16))
        }
        assert!(failed_allocation_size_consistent(0, 0));
        assert!(!failed_allocation_size_consistent(0, 64));
    }
}
