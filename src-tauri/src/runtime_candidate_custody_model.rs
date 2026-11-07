//! Candidate-only DATA policy. No OS calls, includes, hidden initialization or native ownership.
pub(super) const HANDLE_LIMIT: usize = 64;
pub(super) const ALLOCATION_LIMIT: usize = 16;
pub(super) const FRAME_LIMIT: usize = 128;
pub(super) const PATH_LIMIT: usize = 32760;
pub(super) const PROFILE_LIMIT: usize = 16;
pub(super) const IO_LIMIT: usize = 65536;
pub(super) const SD_LIMIT: usize = 65536;
pub(super) const MANIFEST_LIMIT: u64 = 4 << 20;
pub(super) const PENDING: u32 = 0x103;
pub(super) const MISSING: u32 = 0xc0000034;
pub(super) const EOF: u32 = 0xc0000011;
pub(super) const SENTINEL: usize = usize::MAX - 0x1111;

/// Two closed acquisition modes; never arbitrary paths/flags or a six-role framework.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(super) enum Mode {
    Candidate4,
    OrdinaryRdd2,
}
impl Mode {
    pub(super) fn roles(self) -> usize {
        match self {
            Self::Candidate4 => 4,
            Self::OrdinaryRdd2 => 2,
        }
    }
}
/// READ21 rejects length-including-NUL >=1040. Native query buffers are a separate cap.
pub(super) fn downstream_path_units(units_without_nul: usize) -> bool {
    units_without_nul.checked_add(1).is_some_and(|n| n < 1040)
}
pub(super) fn downstream_child(parent: &str, component: &str) -> bool {
    if parent.is_empty()
        || parent.ends_with('\\')
        || component.is_empty()
        || component.contains(['\\', '/'])
    {
        return false;
    }
    parent
        .encode_utf16()
        .count()
        .checked_add(1)
        .and_then(|n| n.checked_add(component.encode_utf16().count()))
        .is_some_and(downstream_path_units)
}

pub(super) fn process_tuple_matches(
    actual: (u32, u64, u32),
    expected: Option<(u32, u64, u32)>,
    current_session: u32,
) -> bool {
    actual.0 != 0
        && actual.1 != 0
        && actual.2 != 0
        && actual.2 == current_session
        && expected.is_none_or(|e| e == actual)
}

/// The real producer publishes its actual Arc owner here before native entry.
pub(super) struct RetainedAttempt<T> {
    owner: Option<std::sync::Arc<T>>,
}
impl<T> Default for RetainedAttempt<T> {
    fn default() -> Self {
        Self { owner: None }
    }
}
impl<T> RetainedAttempt<T> {
    pub(super) fn captured(&self) -> Option<std::sync::Arc<T>> {
        self.owner.clone()
    }
    pub(super) fn publish(&mut self, owner: std::sync::Arc<T>) -> bool {
        if self.owner.is_some() {
            return false;
        }
        self.owner = Some(owner);
        true
    }
}

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(super) enum Completion {
    Complete,
    Missing,
    Refused,
    Unknown,
}
pub(super) fn open_completion(
    raw: u32,
    handle: usize,
    status: usize,
    information: usize,
    disposition: u32,
) -> Completion {
    if raw == PENDING || status as u32 == PENDING {
        return Completion::Unknown;
    }
    if raw == 0 {
        return if handle != 0
            && handle != usize::MAX
            && status as u32 == 0
            && match disposition {
                1 => information == 1,
                2 => information == 2,
                3 => information == 1 || information == 2,
                _ => false,
            } {
            Completion::Complete
        } else {
            Completion::Unknown
        };
    }
    if handle != 0 {
        return Completion::Unknown;
    }
    if raw == MISSING {
        if (status == SENTINEL && information == SENTINEL)
            || (status as u32 == MISSING && matches!(information, 0 | 5))
        {
            Completion::Missing
        } else {
            Completion::Unknown
        }
    } else {
        Completion::Refused
    }
}
pub(super) fn duplicate_completion(
    success: bool,
    output: usize,
    original: usize,
    aliases_live: bool,
) -> Completion {
    if !success {
        return if output == 0 {
            Completion::Refused
        } else {
            Completion::Unknown
        };
    }
    if output == 0 || output == usize::MAX || output == original || aliases_live {
        Completion::Unknown
    } else {
        Completion::Complete
    }
}
pub(super) fn rename_completion(
    raw: u32,
    status: usize,
    information: usize,
    length: usize,
) -> Completion {
    if raw == 0 && status as u32 == 0 && information <= length {
        Completion::Complete
    } else {
        Completion::Unknown
    }
}
pub(super) fn io_completion(
    raw: u32,
    status: usize,
    information: usize,
    requested: usize,
    reading: bool,
) -> Completion {
    if raw == PENDING || status as u32 == PENDING {
        return Completion::Unknown;
    }
    if raw == 0 && status as u32 == 0 && information <= requested {
        return Completion::Complete;
    }
    if reading && raw == EOF && status as u32 == EOF && information == 0 {
        return Completion::Complete;
    }
    Completion::Unknown
}
#[derive(Default, Debug)]
pub(super) struct CloseMemo {
    pub(super) attempted: bool,
    pub(super) complete: bool,
}
impl CloseMemo {
    pub(super) fn arm(&mut self) -> bool {
        if self.attempted {
            return false;
        }
        self.attempted = true;
        true
    }
    pub(super) fn finish(&mut self, success: bool) {
        self.complete = success;
    }
}
pub(super) fn room(used: usize, limit: usize) -> bool {
    used < limit
}
pub(super) fn asset_size(index: usize, bytes: u64) -> bool {
    bytes > 0
        && match index {
            0 => bytes <= 512 << 20,
            1 => bytes <= 4 << 20,
            2 => bytes <= 4096,
            3 => bytes == 88,
            _ => false,
        }
}
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(super) struct Identity {
    pub(super) volume: u32,
    pub(super) high: u32,
    pub(super) low: u32,
    pub(super) volume64: u64,
    pub(super) id128: [u8; 16],
    pub(super) attributes: u32,
    pub(super) links: u32,
    pub(super) size: u64,
    pub(super) write: u64,
}
pub(super) fn same_object(left: Identity, right: Identity) -> bool {
    left.volume == right.volume
        && left.high == right.high
        && left.low == right.low
        && left.volume64 == right.volume64
        && left.id128 == right.id128
}
pub(super) fn first_readonly_matches(writer: Identity, reader: Identity) -> bool {
    same_object(writer, reader)
        && writer.attributes == reader.attributes
        && writer.links == reader.links
        && writer.size == reader.size
}
#[derive(Clone, Debug)]
pub(super) struct Ace {
    pub(super) kind: u8,
    pub(super) flags: u8,
    pub(super) mask: u32,
    pub(super) sid: Vec<u8>,
}
// Numeric SID representation for closed-policy comparisons. Caller SDDL uses
// authority5 only; this is not a general ConvertSidToStringSidW API replacement.
pub(super) fn sid_text(sid: &[u8]) -> Option<String> {
    if sid.len() < 8 || sid[0] != 1 || sid[1] > 15 || sid.len() != 8 + usize::from(sid[1]) * 4 {
        return None;
    }
    let authority = sid[2..8].iter().fold(0u64, |n, b| (n << 8) | u64::from(*b));
    let mut text = format!("S-1-{authority}");
    for chunk in sid[8..].chunks_exact(4) {
        text.push_str(&format!("-{}", u32::from_le_bytes(chunk.try_into().ok()?)));
    }
    Some(text)
}
fn trusted(sid: &[u8], user: &[u8]) -> bool {
    sid == user
        || matches!(
            sid_text(sid).as_deref(),
            Some(
                "S-1-5-18"
                    | "S-1-5-32-544"
                    | "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"
            )
        )
}
pub(super) fn descriptor_header_allowed(
    owner: &[u8],
    user: &[u8],
    control: u16,
    private: bool,
) -> bool {
    if sid_text(owner).is_none() || sid_text(user).is_none() {
        return false;
    }
    if private {
        owner == user && control & 0x1000 != 0
    } else {
        trusted(owner, user)
    }
}
/// Same existing cache decisions; extraction/native custody is deliberately outside this DATA function.
pub(super) fn descriptor_allowed(
    owner: &[u8],
    user: &[u8],
    control: u16,
    aces: &[Ace],
    private: bool,
    directory: bool,
) -> bool {
    if !descriptor_header_allowed(owner, user, control, private)
        || aces
            .iter()
            .any(|a| a.kind > 1 || a.flags & !0x1f != 0 || sid_text(&a.sid).is_none())
    {
        return false;
    }
    if !private {
        return trusted(owner, user)
            && aces.iter().all(|a| {
                a.flags & 8 != 0 || a.kind == 1 || trusted(&a.sid, user) || a.mask & 0x500d0152 == 0
            });
    }
    if owner != user
        || control & 0x1000 == 0
        || aces.len() != 3
        || aces
            .iter()
            .any(|a| a.kind != 0 || a.flags != if directory { 3 } else { 0 } || a.mask != 0x1f01ff)
    {
        return false;
    }
    let mut actual: Vec<String> = aces.iter().filter_map(|a| sid_text(&a.sid)).collect();
    actual.sort();
    let mut expected = vec![
        sid_text(user).unwrap(),
        "S-1-5-18".into(),
        "S-1-5-32-544".into(),
    ];
    expected.sort();
    actual == expected
}

#[cfg(test)]
mod tests {
    use super::*;
    fn sid(parts: &[u32]) -> Vec<u8> {
        let mut s = vec![1, parts.len() as u8, 0, 0, 0, 0, 0, 5];
        for p in parts {
            s.extend(p.to_le_bytes());
        }
        s
    }
    fn ace(sid: Vec<u8>, mask: u32) -> Ace {
        Ace {
            kind: 0,
            flags: 0,
            mask,
            sid,
        }
    }
    #[test]
    fn pending_and_partial_outputs_never_become_missing_or_complete() {
        for (raw, h, s, i, d) in [
            (PENDING, 0, SENTINEL, SENTINEL, 1),
            (MISSING, 42, SENTINEL, SENTINEL, 1),
            (0, 0, 0, 1, 1),
            (0, 42, PENDING as usize, 1, 1),
            (0, 42, 0, 2, 1),
            (0, 42, 0, 1, 2),
        ] {
            assert_eq!(open_completion(raw, h, s, i, d), Completion::Unknown);
        }
        assert_eq!(
            open_completion(MISSING, 0, SENTINEL, SENTINEL, 1),
            Completion::Missing
        );
        assert_eq!(
            open_completion(MISSING, 0, MISSING as usize, 0, 1),
            Completion::Missing
        );
        assert_eq!(open_completion(MISSING, 0, 0, 2, 1), Completion::Unknown);
        assert_eq!(
            open_completion(0xc0000022, 0, SENTINEL, SENTINEL, 1),
            Completion::Refused
        );
        assert_eq!(open_completion(0, 42, 0, 1, 1), Completion::Complete);
        assert_eq!(open_completion(0, 42, 0, 2, 2), Completion::Complete);
    }
    #[test]
    fn rename_and_stream_completion_are_exact_and_bounded() {
        for (raw, s, i) in [
            (PENDING, 0, 0),
            (0, PENDING as usize, 0),
            (0, 0, 129),
            (0xc0000022, 0, 0),
        ] {
            assert_eq!(rename_completion(raw, s, i, 128), Completion::Unknown);
        }
        assert_eq!(rename_completion(0, 0, 0, 128), Completion::Complete);
        assert_eq!(rename_completion(0, 0, 128, 128), Completion::Complete);
        assert_eq!(io_completion(0, 0, 65, 64, true), Completion::Unknown);
        assert_eq!(
            io_completion(EOF, EOF as usize, 0, 64, true),
            Completion::Complete
        );
        assert_eq!(
            io_completion(EOF, EOF as usize, 0, 64, false),
            Completion::Unknown
        );
    }
    #[test]
    fn close_is_prearmed_and_failed_or_panicked_attempt_is_not_replayed() {
        let mut c = CloseMemo::default();
        assert!(c.arm());
        assert!(!c.complete);
        assert!(!c.arm());
        c.finish(false);
        assert!(!c.arm());
        assert!(!c.complete);
        let mut c = CloseMemo::default();
        assert!(c.arm());
        c.finish(true);
        assert!(c.complete);
        assert!(!c.arm());
    }
    #[test]
    fn bounds_refuse_before_overflow_and_keep_fixed_four_roles() {
        for limit in [HANDLE_LIMIT, ALLOCATION_LIMIT, FRAME_LIMIT] {
            assert!(room(limit - 1, limit));
            assert!(!room(limit, limit));
            assert!(!room(usize::MAX, limit));
        }
        assert_eq!(MANIFEST_LIMIT, 4 << 20);
        for i in 0..4 {
            assert!(!asset_size(i, 0));
        }
        assert!(asset_size(0, 512 << 20));
        assert!(!asset_size(0, (512 << 20) + 1));
        assert!(asset_size(1, 4 << 20));
        assert!(!asset_size(1, (4 << 20) + 1));
        assert!(asset_size(2, 4096));
        assert!(!asset_size(2, 4097));
        assert!(asset_size(3, 88));
        assert!(!asset_size(3, 87));
        assert!(!asset_size(3, 89));
        assert!(!asset_size(4, 88));
    }
    #[test]
    fn only_first_writer_handoff_can_change_timestamp() {
        let a = Identity {
            volume: 1,
            high: 2,
            low: 3,
            volume64: 1,
            id128: [3; 16],
            attributes: 0x20,
            links: 1,
            size: 88,
            write: 4,
        };
        let mut b = a;
        b.write = 5;
        assert!(first_readonly_matches(a, b));
        assert_ne!(a, b);
        let baseline = b;
        assert_eq!(baseline, b);
        b.low += 1;
        assert!(!first_readonly_matches(a, b));
        b = a;
        b.size += 1;
        assert!(!first_readonly_matches(a, b));
        b = a;
        b.links += 1;
        assert!(!first_readonly_matches(a, b));
    }
    #[test]
    fn integrity_keeps_existing_append_and_ignored_ace_semantics() {
        let user = sid(&[21, 1]);
        let foreign = sid(&[21, 9]);
        assert!(descriptor_allowed(&user, &user, 0, &[], false, true));
        assert!(descriptor_allowed(
            &user,
            &user,
            0,
            &[ace(foreign.clone(), 4)],
            false,
            true
        ));
        assert!(!descriptor_allowed(
            &user,
            &user,
            0,
            &[ace(foreign.clone(), 2)],
            false,
            true
        ));
        for flags in [8, 0x18] {
            let mut a = ace(foreign.clone(), u32::MAX);
            a.flags = flags;
            assert!(descriptor_allowed(&user, &user, 0, &[a], false, true));
        }
        let mut a = ace(foreign.clone(), 2);
        a.flags = 0x10;
        assert!(!descriptor_allowed(&user, &user, 0, &[a], false, true));
        let mut a = ace(foreign, 2);
        a.kind = 1;
        assert!(descriptor_allowed(&user, &user, 0, &[a], false, true));
        let mut a = ace(user.clone(), 0);
        a.kind = 2;
        assert!(!descriptor_allowed(&user, &user, 0, &[a], false, true));
    }
    #[test]
    fn private_policy_is_protected_exact_principal_multiset() {
        let user = sid(&[21, 1]);
        let mut aces = vec![
            ace(sid(&[32, 544]), 0x1f01ff),
            ace(user.clone(), 0x1f01ff),
            ace(sid(&[18]), 0x1f01ff),
        ];
        assert!(descriptor_allowed(&user, &user, 0x9000, &aces, true, false));
        assert!(!descriptor_allowed(&user, &user, 0, &aces, true, false));
        assert!(!descriptor_allowed(&user, &user, 0x1000, &aces, true, true));
        for a in &mut aces {
            a.flags = 3;
        }
        assert!(descriptor_allowed(&user, &user, 0x1000, &aces, true, true));
        aces[0].sid = user.clone();
        assert!(!descriptor_allowed(&user, &user, 0x1000, &aces, true, true));
    }
    #[test]
    fn bounded_sid_numeric_values_preserve_authority_and_subauthorities() {
        assert_eq!(sid_text(&sid(&[32, 544])).as_deref(), Some("S-1-5-32-544"));
        let mut s = sid(&[u32::MAX]);
        s[2..8].copy_from_slice(&[1, 2, 3, 4, 5, 6]);
        assert_eq!(sid_text(&s).unwrap(), "S-1-1108152157446-4294967295");
        s[0] = 2;
        assert!(sid_text(&s).is_none());
        assert!(sid_text(&[1; 7]).is_none());
    }
    #[test]
    fn actual_arc_owner_survives_external_drop_and_failed_attempt_reentry() {
        let mut memo = RetainedAttempt::default();
        let owner = std::sync::Arc::new(vec![42u8; 64]);
        let weak = std::sync::Arc::downgrade(&owner);
        assert!(memo.publish(owner.clone()));
        drop(owner);
        assert!(weak.upgrade().is_some());
        assert!(!memo.publish(std::sync::Arc::new(vec![99])));
        assert_eq!(memo.captured().unwrap()[0], 42);
    }
    #[test]
    fn poisoned_real_mutex_keeps_the_prepublished_graph() {
        let memo = std::sync::Mutex::new(RetainedAttempt::default());
        let owner = std::sync::Arc::new(vec![7u8; 32]);
        let weak = std::sync::Arc::downgrade(&owner);
        memo.lock().unwrap().publish(owner);
        let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            let _guard = memo.lock().unwrap();
            panic!("after original graph publication");
        }));
        assert!(result.is_err());
        assert!(memo.lock().is_err());
        assert_eq!(weak.upgrade().unwrap()[0], 7);
    }
    #[test]
    fn all_trusted_principals_and_foreign_large_authority_keep_same_decisions() {
        let user = sid(&[21, 1]);
        for owner in [
            sid(&[18]),
            sid(&[32, 544]),
            sid(&[
                80, 956008885, 3418522649, 1831038044, 1853292631, 2271478464,
            ]),
        ] {
            assert!(descriptor_allowed(
                &owner,
                &user,
                0,
                &[ace(owner.clone(), u32::MAX)],
                false,
                true
            ));
            assert!(!descriptor_allowed(&owner, &user, 0x1000, &[], true, true));
        }
        let mut foreign = sid(&[u32::MAX]);
        foreign[2..8].copy_from_slice(&[1, 2, 3, 4, 5, 6]);
        assert!(descriptor_allowed(
            &user,
            &user,
            0,
            &[ace(foreign.clone(), 4)],
            false,
            true
        ));
        assert!(!descriptor_allowed(&foreign, &user, 0, &[], false, true));
        let mut a = ace(foreign.clone(), 2);
        assert!(!descriptor_allowed(
            &user,
            &user,
            0,
            &[a.clone()],
            false,
            true
        ));
        a.kind = 1;
        assert!(descriptor_allowed(
            &user,
            &user,
            0,
            &[a.clone()],
            false,
            true
        ));
        a.flags = 0x20;
        assert!(!descriptor_allowed(&user, &user, 0, &[a], false, true));
    }
    #[test]
    fn downstream_read21_uses_strict_nul_bound_and_utf16_not_utf8_bytes() {
        assert!(downstream_path_units(1038));
        assert!(!downstream_path_units(1039));
        assert!(!downstream_path_units(1040));
        assert!(!downstream_path_units(usize::MAX));
        let parent = format!("\\Device\\{}", "x".repeat(1010));
        assert!(downstream_child(&parent, "rdd.exe"));
        assert!(!downstream_child(
            &format!("{parent}{}", "x".repeat(30)),
            "rdd.exe"
        ));
        assert!(!downstream_child("\\Device\\", "rdd.exe"));
        assert!(!downstream_child("\\Device\\x", "../rdd.exe"));
        assert!(downstream_path_units(
            "😀".repeat(519).encode_utf16().count()
        ));
        assert!(!downstream_path_units(
            "😀".repeat(520).encode_utf16().count()
        ));
    }
    #[test]
    fn only_two_closed_modes_select_roles_without_metadata_for_ordinary() {
        assert_eq!(Mode::Candidate4.roles(), 4);
        assert_eq!(Mode::OrdinaryRdd2.roles(), 2);
    }

    #[test]
    fn original_process_tuple_requires_nonzero_same_session_and_expected_generation() {
        let actual = (7, 1234, 2);
        assert!(process_tuple_matches(actual, None, 2));
        assert!(process_tuple_matches(actual, Some(actual), 2));
        for bad in [
            (0, 1234, 2),
            (7, 0, 2),
            (7, 1234, 0),
            (8, 1234, 2),
            (7, 1235, 2),
            (7, 1234, 3),
        ] {
            assert!(!process_tuple_matches(bad, Some(actual), 2));
        }
        assert!(!process_tuple_matches(actual, None, 3));
    }

    #[test]
    fn duplicate_original_output_is_never_a_borrowed_or_live_alias_closer() {
        for (success, out, original, live) in [
            (true, 0, 7, false),
            (true, usize::MAX, 7, false),
            (true, 7, 7, false),
            (true, 8, 7, true),
            (false, 8, 7, false),
        ] {
            assert_eq!(
                duplicate_completion(success, out, original, live),
                Completion::Unknown
            );
        }
        assert_eq!(
            duplicate_completion(false, 0, 7, false),
            Completion::Refused
        );
        assert_eq!(
            duplicate_completion(true, 8, 7, false),
            Completion::Complete
        );
    }
    #[test]
    fn full_file_id_prevents_low64_collision_from_becoming_same_object() {
        let a = Identity {
            volume: 1,
            high: 2,
            low: 3,
            volume64: 1,
            id128: [7; 16],
            attributes: 0x20,
            links: 1,
            size: 88,
            write: 4,
        };
        let mut b = a;
        b.id128[15] ^= 1;
        assert_eq!((a.volume, a.high, a.low), (b.volume, b.high, b.low));
        assert_ne!(a, b);
        assert!(!same_object(a, b));
        assert!(!first_readonly_matches(a, b));
        b = a;
        b.volume64 ^= 1 << 48;
        assert_ne!(a, b);
        assert!(!same_object(a, b));
        assert!(!first_readonly_matches(a, b));
        b = a;
        b.write += 1;
        assert!(same_object(a, b));
        assert!(first_readonly_matches(a, b));
        assert_ne!(a, b);
    }
}
