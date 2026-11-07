//! DATA-only recognizers for two exact sorted compact producer shapes. Never a
//! source continuity witness, permission, historical runtime or execution grant.
use super::*;
fn offset(bytes: &[u8], field: &[u8]) -> Option<usize> {
    bytes
        .windows(field.len())
        .position(|v| v == field)
        .map(|i| i + field.len())
}
fn replace_hash(bytes: &[u8], start: usize, fallback: &str) -> Option<String> {
    let available = bytes.len().saturating_sub(start).min(64);
    let mut result = fallback.as_bytes().to_vec();
    for n in 0..available {
        let b = bytes[start + n];
        if !b.is_ascii_digit() && !(b'a'..=b'f').contains(&b) {
            return None;
        };
        result[n] = b;
    }
    String::from_utf8(result).ok()
}
fn string_candidate(bytes: &[u8], fallback: &str) -> Option<String> {
    let fallback_bytes = go_json(&fallback).ok()?;
    if fallback_bytes.starts_with(bytes) {
        return Some(fallback.into());
    }
    if bytes.first() != Some(&b'"') {
        return None;
    }
    let mut stream = serde_json::Deserializer::from_slice(bytes).into_iter::<String>();
    if let Some(Ok(s)) = stream.next() {
        let n = stream.byte_offset();
        return (go_json(&s).ok()? == bytes[..n] && transform::legacy_selector(&s)).then_some(s);
    }
    let mut completed = bytes.to_vec();
    let mut i = 1;
    while i < completed.len() {
        if completed[i] < 0x20 || completed[i] == b'"' {
            return None;
        }
        if completed[i] != b'\\' {
            i += 1;
            continue;
        }
        i += 1;
        if i == completed.len() {
            completed.push(b'\\');
            break;
        }
        match completed[i] {
            b'\\' => i += 1,
            b'u' => {
                let count = (completed.len() - i - 1).min(4);
                let partial = &completed[i + 1..i + 1 + count];
                let allowed = [b"2028".as_slice(), b"2029", b"0026"];
                let suffix = allowed.iter().find(|v| v.starts_with(partial))?;
                if count < 4 {
                    completed.extend_from_slice(&suffix[count..]);
                    break;
                }
                i += 5;
            }
            _ => return None,
        }
    }
    // The only possible invalid UTF-8 here is a cut inside one scalar. Complete
    // that scalar minimally, then let serde + exact canonical reencoding decide.
    if let Err(e) = std::str::from_utf8(&completed) {
        if e.error_len().is_some() {
            return None;
        };
        let start = e.valid_up_to();
        let lead = completed[start];
        let width = match lead {
            0xc2..=0xdf => 2,
            0xe0..=0xef => 3,
            0xf0..=0xf4 => 4,
            _ => return None,
        };
        while completed.len() - start < width {
            let at = completed.len() - start;
            completed.push(if at == 1 && lead == 0xe0 {
                0xa0
            } else if at == 1 && lead == 0xf0 {
                0x90
            } else {
                0x80
            })
        }
    }
    completed.push(b'"');
    let text: String = serde_json::from_slice(&completed).ok()?;
    if go_json(&text).ok()? != completed {
        return None;
    }
    for suffix in [
        "",
        r"\docker-cli-plugins",
        r"x\docker-cli-plugins",
        r"C:\docker-cli-plugins",
        r":\docker-cli-plugins",
        r"?\C:\docker-cli-plugins",
        r"\?\C:\docker-cli-plugins",
        r"\C:\docker-cli-plugins",
    ] {
        let candidate = format!("{text}{suffix}");
        if transform::legacy_selector(&candidate) && go_json(&candidate).ok()?.starts_with(bytes) {
            return Some(candidate);
        }
    }
    None
}
struct Cursor<'a> {
    bytes: &'a [u8],
    at: usize,
    stopped: bool,
}
impl Cursor<'_> {
    fn fixed(&mut self, s: &[u8]) -> Option<()> {
        if self.stopped {
            return Some(());
        }
        let tail = &self.bytes[self.at..];
        if tail.len() < s.len() {
            if !s.starts_with(tail) {
                return None;
            };
            self.at = self.bytes.len();
            self.stopped = true;
            return Some(());
        }
        if !tail.starts_with(s) {
            return None;
        };
        self.at += s.len();
        Some(())
    }
    fn hex(&mut self) -> Option<String> {
        if self.stopped {
            return Some("1".repeat(64));
        }
        let value = replace_hash(self.bytes, self.at, &"1".repeat(64))?;
        let n = (self.bytes.len() - self.at).min(64);
        self.at += n;
        if n < 64 {
            self.stopped = true
        };
        Some(value)
    }
    fn number(&mut self, min: u64, max: u64) -> Option<u64> {
        if self.stopped {
            return Some(min);
        }
        let start = self.at;
        while self.bytes.get(self.at).is_some_and(u8::is_ascii_digit) {
            self.at += 1
        }
        if self.at == start {
            if self.at == self.bytes.len() {
                self.stopped = true;
                return Some(min);
            }
            return None;
        }
        let raw = &self.bytes[start..self.at];
        if raw.len() > 1 && raw[0] == b'0' {
            return None;
        }
        let value = std::str::from_utf8(raw).ok()?.parse::<u64>().ok()?;
        if value < min || value > max {
            return None;
        }
        if self.at == self.bytes.len() {
            self.stopped = true
        }
        Some(value)
    }
}
fn observed_witness(bytes: &[u8]) -> Option<Witness> {
    let mut c = Cursor {
        bytes,
        at: 0,
        stopped: false,
    };
    c.fixed(br#"{"bodySha256":""#)?;
    let body_sha256 = c.hex()?;
    c.fixed(br#"","bytes":"#)?;
    let bytes = c.number(1, CONFIG_BUDGET as u64)?;
    c.fixed(b",\"id\":{\"high\":")?;
    let high = c.number(0, u32::MAX as u64)? as u32;
    c.fixed(b",\"low\":")?;
    let low = c.number(0, u32::MAX as u64)? as u32;
    c.fixed(b",\"volume\":")?;
    let volume = c.number(0, u32::MAX as u64)? as u32;
    c.fixed(b"},\"sdSha256\":\"")?;
    let sd_sha256 = c.hex()?;
    c.fixed(b"\"}")?;
    Some(Witness {
        id: Fid { volume, high, low },
        sd_sha256,
        body_sha256,
        bytes,
    })
}
pub(super) fn uncommitted(bytes: &[u8], current: &Intent) -> bool {
    if bytes.len() > BUDGET || parse_intent(bytes).is_ok() {
        return false;
    }
    let mut candidate = current.clone();
    let Some(binding) = replace_hash(bytes, 12, &candidate.binding) else {
        return false;
    };
    candidate.binding = binding;
    let Ok(first) = encode(&candidate) else {
        return false;
    };
    let Some(old_at) = offset(&first, br#""oldSelector":"#) else {
        return false;
    };
    if bytes.len() > old_at {
        let Some(old) = string_candidate(&bytes[old_at..], &candidate.old_selector) else {
            return false;
        };
        candidate.old_selector = old;
    }
    let Ok(with_old) = encode(&candidate) else {
        return false;
    };
    let Some(source_at) = offset(&with_old, br#""source":"#) else {
        return false;
    };
    if bytes.len() > source_at {
        let Some(source) = observed_witness(&bytes[source_at..]) else {
            return false;
        };
        candidate.source = source;
    }
    encode(&candidate).is_ok_and(|full| bytes.len() < full.len() && full.starts_with(bytes))
}
pub(super) fn target(
    bytes: &[u8],
    current: &Target,
    original: &[u8],
    old: &str,
    profile: &Path,
) -> bool {
    let mut candidate = current.clone();
    let Ok(first) = encode(&candidate) else {
        return false;
    };
    let Some(at) = offset(&first, br#""selector":"#) else {
        return false;
    };
    let Ok(prefix) = go_json(&format!(
        r"{}\.orbwin-private\runtime\docker-",
        profile.to_string_lossy()
    )) else {
        return false;
    };
    let pin_at = at + prefix.len() - 1;
    if bytes.len() > pin_at {
        let Some(pin) = replace_hash(bytes, pin_at, &"1".repeat(64)) else {
            return false;
        };
        candidate.selector = format!(
            r"{}\.orbwin-private\runtime\docker-{}\docker-cli-plugins",
            profile.to_string_lossy(),
            pin
        );
    }
    let Ok(target) = super::target(original, old, &candidate.selector, profile) else {
        return false;
    };
    candidate.target_sha256 = hash(&target);
    candidate.target_bytes = target.len() as u64;
    encode(&candidate).is_ok_and(|full| bytes.len() < full.len() && full.starts_with(bytes))
}
