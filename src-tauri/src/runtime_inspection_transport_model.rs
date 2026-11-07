//! Pure bounded DATA accumulation. No native ownership or termination grant.

pub(super) const STDOUT_LIMIT: usize = 16_384;
pub(super) const STDERR_LIMIT: usize = 65_536;

pub(super) struct Capture {
    bytes: Vec<u8>,
    limit: usize,
    overflow: bool,
}
impl Capture {
    pub(super) fn new(limit: usize) -> Self {
        Self {
            bytes: Vec::new(),
            limit,
            overflow: false,
        }
    }
    pub(super) fn push(&mut self, chunk: &[u8]) {
        let remaining = self.limit.saturating_sub(self.bytes.len());
        self.overflow |= chunk.len() > remaining;
        self.bytes
            .extend_from_slice(&chunk[..remaining.min(chunk.len())]);
    }
    pub(super) fn bytes(&self) -> &[u8] {
        &self.bytes
    }
    pub(super) fn overflow(&self) -> bool {
        self.overflow
    }
}

pub(super) fn decode_allowed(
    input: bool,
    eof: [bool; 2],
    exit: Option<u32>,
    stdout: &Capture,
    stderr: &Capture,
) -> bool {
    input
        && eof == [true, true]
        && exit == Some(0)
        && !stdout.overflow
        && !stderr.overflow
        && stderr.bytes.is_empty()
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn captures_bound_every_split_and_keep_overflow_sticky() {
        for split in 0..=33 {
            let mut c = Capture::new(32);
            c.push(&[b'a'; 33][..split]);
            c.push(&[b'a'; 33][split..]);
            assert_eq!(c.bytes(), &[b'a'; 32]);
            assert!(c.overflow());
            c.push(&[]);
            c.push(&[b'b'; 8192]);
            assert_eq!(c.bytes().len(), 32);
            assert!(c.overflow());
        }
        let mut zero = Capture::new(0);
        zero.push(b"x");
        assert!(zero.bytes().is_empty());
        assert!(zero.overflow());
    }
    #[test]
    fn eof_exit_and_empty_stderr_are_independent_requirements() {
        let mut out = Capture::new(STDOUT_LIMIT);
        out.push(b"{}\n");
        let mut err = Capture::new(STDERR_LIMIT);
        assert!(decode_allowed(true, [true, true], Some(0), &out, &err));
        for eof in [[false, false], [true, false], [false, true]] {
            assert!(!decode_allowed(true, eof, Some(0), &out, &err));
        }
        for exit in [None, Some(1), Some(2), Some(259)] {
            assert!(!decode_allowed(true, [true, true], exit, &out, &err));
        }
        assert!(!decode_allowed(false, [true, true], Some(0), &out, &err));
        err.push(b"warning");
        assert!(!decode_allowed(true, [true, true], Some(0), &out, &err));
        let mut out = Capture::new(1);
        out.push(b"{}");
        assert!(!decode_allowed(
            true,
            [true, true],
            Some(0),
            &out,
            &Capture::new(STDERR_LIMIT)
        ));
    }
}
