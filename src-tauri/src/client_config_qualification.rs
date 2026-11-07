//! Closed, unarmed process-cut hooks for separately reviewed native fixtures.
//! This module is excluded from release builds. It creates no native authority.
use std::sync::atomic::{AtomicU8, Ordering};

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
#[repr(u8)]
pub(crate) enum Fault {
    HistoricalJournal = 1,
    PublishedOriginalRemoved = 2,
    NextPrefix = 3,
    NextDurable = 4,
    NextBeforeRename = 5,
    NextPublished = 6,
    HistoricalStagePrepared = 7,
    StageRetired = 8,
    LocalEmpty = 9,
    LocalEarly = 10,
    LocalBase = 11,
    LocalLate = 12,
    WriterIntentPrefix = 13,
    LostMetadataRename = 14,
    LostTargetRename = 15,
    LostStageDisposition = 16,
    LostSnapshotConfigClose = 17,
}
static ARMED: AtomicU8 = AtomicU8::new(0);
pub(crate) fn arm(fault: Fault) -> Result<(), &'static str> {
    ARMED
        .compare_exchange(0, fault as u8, Ordering::AcqRel, Ordering::Acquire)
        .map(|_| ())
        .map_err(|_| "Recovery fixture already armed")
}
pub(super) fn selected(fault: Fault) -> bool {
    ARMED.load(Ordering::Acquire) == fault as u8
}
pub(super) fn historical() -> bool {
    selected(Fault::HistoricalJournal) || selected(Fault::HistoricalStagePrepared)
}
pub(super) fn consume(fault: Fault) -> bool {
    if ARMED
        .compare_exchange(fault as u8, 255, Ordering::AcqRel, Ordering::Acquire)
        .is_ok()
    {
        println!("INJECTED_LOST_RETURN {}", fault as u8);
        true
    } else {
        false
    }
}
pub(super) fn prefix_count(fault: Fault, name: &str, bytes: &[u8]) -> Option<usize> {
    if name != "writer.intent" {
        return None;
    }
    if fault == Fault::WriterIntentPrefix {
        return super::parse_recovery_intent(bytes)
            .ok()
            .map(|_| bytes.len() / 2);
    }
    if !matches!(
        fault,
        Fault::LocalEmpty | Fault::LocalEarly | Fault::LocalBase | Fault::LocalLate
    ) || super::parse_local_replace(bytes).is_err()
    {
        return None;
    }
    let needle = b"\"baseSha256\":\"";
    let start = bytes.windows(needle.len()).position(|w| w == needle)? + needle.len();
    Some(match fault {
        Fault::LocalEmpty => 0,
        Fault::LocalEarly => start + 8,
        Fault::LocalBase => start + 65,
        Fault::LocalLate => bytes.len() - 1,
        _ => unreachable!(),
    })
}
pub(super) fn selected_prefix(name: &str, bytes: &[u8]) -> Option<(Fault, usize)> {
    [
        Fault::LocalEmpty,
        Fault::LocalEarly,
        Fault::LocalBase,
        Fault::LocalLate,
        Fault::WriterIntentPrefix,
    ]
    .into_iter()
    .find_map(|f| {
        selected(f)
            .then(|| prefix_count(f, name, bytes).map(|n| (f, n)))
            .flatten()
    })
}
pub(super) fn reached(fault: Fault) -> ! {
    use std::io::Write;
    if ARMED
        .compare_exchange(fault as u8, 255, Ordering::AcqRel, Ordering::Acquire)
        .is_err()
    {
        std::process::exit(125);
    }
    println!("SUCCESSOR_FIXTURE_CUT {}", fault as u8);
    if std::io::stdout().flush().is_err() {
        std::process::exit(125);
    }
    // Real process loss at an actual producer boundary; intentionally no Drop.
    std::process::exit(117)
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn closed_codes_are_unique_and_historical_modes_are_explicit() {
        let modes = [
            Fault::HistoricalJournal,
            Fault::PublishedOriginalRemoved,
            Fault::NextPrefix,
            Fault::NextDurable,
            Fault::NextBeforeRename,
            Fault::NextPublished,
            Fault::HistoricalStagePrepared,
            Fault::StageRetired,
            Fault::LocalEmpty,
            Fault::LocalEarly,
            Fault::LocalBase,
            Fault::LocalLate,
            Fault::WriterIntentPrefix,
            Fault::LostMetadataRename,
            Fault::LostTargetRename,
            Fault::LostStageDisposition,
            Fault::LostSnapshotConfigClose,
        ];
        let mut codes: Vec<_> = modes.into_iter().map(|f| f as u8).collect();
        codes.sort();
        codes.dedup();
        assert_eq!(codes.len(), 17);
        assert!(!historical());
        for fault in modes {
            assert!(!selected(fault));
        }
    }
    #[test]
    fn actual_prefix_selector_is_closed_and_matches_local_producer_prefixes() {
        let hash = "a".repeat(64);
        for next in [None, Some("b".repeat(64))] {
            let bytes = super::super::local_replace_producer(&hash, next.as_deref()).unwrap();
            for mode in [
                Fault::LocalEmpty,
                Fault::LocalEarly,
                Fault::LocalBase,
                Fault::LocalLate,
            ] {
                let n = prefix_count(mode, "writer.intent", &bytes).unwrap();
                assert!(n < bytes.len());
                assert!(super::super::local_eof_prefix(&bytes[..n], &hash));
                assert!(prefix_count(mode, "writer.original", &bytes).is_none());
            }
            assert!(prefix_count(Fault::WriterIntentPrefix, "writer.intent", &bytes).is_none());
        }
        assert!(prefix_count(Fault::LocalLate, "writer.intent", b"{}").is_none());
    }
}
