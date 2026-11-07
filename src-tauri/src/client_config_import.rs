//! Closed import DATA policy/codec. Native owners independently supply all caps.
use super::*;
use sha2::{Digest, Sha256};
#[path = "client_config_import_prefix.rs"]
mod prefix;
#[path = "client_config_import_transform.rs"]
mod transform;
pub(super) const MARKER: &[u8] = br#"{"schema":1,"kind":"legacy-import"}"#;
pub(super) const BUDGET: usize = 16 * 1024;
pub(super) const POLICY: &str = "legacy-transform-v3";
pub(super) const SOURCE_NAMESPACE: &str = "orbwin-public-app-config-v1";
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(super) enum MarkerKind {
    Shared,
    Fresh,
    Import,
    Unsupported,
}
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum ImportAction {
    ResumeReviewedImport,
    ReviewCurrentSource,
}
pub(super) fn marker_kind(bytes: &[u8]) -> MarkerKind {
    match (FRESH_MARKER.starts_with(bytes), MARKER.starts_with(bytes)) {
        (true, true) => MarkerKind::Shared,
        (true, false) => MarkerKind::Fresh,
        (false, true) => MarkerKind::Import,
        _ => MarkerKind::Unsupported,
    }
}
#[derive(Clone, Debug, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(super) struct Fid {
    pub volume: u32,
    pub high: u32,
    pub low: u32,
}
#[derive(Clone, Debug, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(super) struct Witness {
    pub id: Fid,
    pub sd_sha256: String,
    pub body_sha256: String,
    pub bytes: u64,
}
#[derive(Clone, Debug, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(super) struct DirectoryWitness {
    pub id: Fid,
    pub sd_sha256: String,
}
#[derive(Clone, Debug, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(super) struct Scratch {
    pub docker: Option<DirectoryWitness>,
    pub init: Option<Witness>,
    pub config: Option<Witness>,
}
#[derive(Clone, Debug, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(super) struct Intent {
    pub schema: u32,
    pub kind: String,
    pub policy: String,
    pub binding: String,
    pub owner_sid: String,
    pub root: Fid,
    pub source_namespace: String,
    pub source: Witness,
    pub old_selector: String,
    pub scratch: Scratch,
}
#[derive(Clone, Debug, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(super) struct SourceCopy {
    pub schema: u32,
    pub kind: String,
    pub binding: String,
    pub root: Fid,
    pub intent_sha256: String,
    pub original: Witness,
}
#[derive(Clone, Debug, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(super) struct Target {
    pub schema: u32,
    pub kind: String,
    pub binding: String,
    pub root: Fid,
    pub source_copy_sha256: String,
    pub selector: String,
    pub target_sha256: String,
    pub target_bytes: u64,
}
#[derive(Clone, Debug, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(super) struct CompletedInit {
    pub schema: u32,
    pub product: String,
    pub admission: String,
    pub policy: String,
    pub owner_sid: String,
    pub root_volume: u32,
    pub root_high: u32,
    pub root_low: u32,
    pub binding: String,
    pub source_namespace: String,
    pub source: Witness,
    pub initial: Witness,
    pub initial_selector: String,
}
pub(super) fn hash(bytes: &[u8]) -> String {
    format!("{:x}", Sha256::digest(bytes))
}
fn digest(s: &str) -> bool {
    digest_hex(s).is_ok()
}
fn binding(s: &str) -> bool {
    digest_hex(s).is_ok_and(|v| v != [0; 32])
}
fn witness(v: &Witness) -> bool {
    v.bytes <= CONFIG_BUDGET as u64 && digest(&v.sd_sha256) && digest(&v.body_sha256)
}
pub(super) fn encode<T: serde::Serialize>(value: &T) -> Result<Vec<u8>, ClientConfigError> {
    let sorted = serde_json::to_value(value).map_err(|_| err(ErrorKind::RecoveryRequired))?;
    let b = go_json(&sorted)?;
    if b.len() > BUDGET {
        return Err(err(ErrorKind::RecoveryRequired));
    };
    Ok(b)
}
// New protocol has one canonical producer encoding. Existing generic metadata
// parsers/schemas remain untouched; unknown/duplicate/missing fields refuse here.
fn decode<T: serde::de::DeserializeOwned + serde::Serialize>(
    b: &[u8],
) -> Result<T, ClientConfigError> {
    if b.len() > BUDGET {
        return Err(err(ErrorKind::RecoveryRequired));
    };
    checked_json(b)?;
    let v = serde_json::from_slice(b).map_err(|_| err(ErrorKind::RecoveryRequired))?;
    if encode(&v)? != b {
        return Err(err(ErrorKind::RecoveryRequired));
    };
    Ok(v)
}
pub(super) fn parse_intent(b: &[u8]) -> Result<Intent, ClientConfigError> {
    let v: Intent = decode(b)?;
    if v.schema != 1
        || v.kind != "legacy-client-import"
        || v.policy != POLICY
        || v.source_namespace != SOURCE_NAMESPACE
        || !binding(&v.binding)
        || v.owner_sid.is_empty()
        || !witness(&v.source)
        || v.source.bytes == 0
        || v.scratch.init.as_ref().is_some_and(|w| !witness(w))
        || v.scratch.config.as_ref().is_some_and(|w| !witness(w))
        || (v.scratch.docker.is_none() && v.scratch.config.is_some())
        || v.scratch
            .docker
            .as_ref()
            .is_some_and(|d| !digest(&d.sd_sha256))
    {
        return Err(err(ErrorKind::RecoveryRequired));
    };
    Ok(v)
}
pub(super) fn parse_source_copy(
    b: &[u8],
    intent: &Intent,
    intent_bytes: &[u8],
) -> Result<SourceCopy, ClientConfigError> {
    let v: SourceCopy = decode(b)?;
    if v.schema != 1
        || v.kind != "legacy-source-copy"
        || v.binding != intent.binding
        || v.root != intent.root
        || v.intent_sha256 != hash(intent_bytes)
        || !witness(&v.original)
        || v.original.bytes != intent.source.bytes
        || v.original.body_sha256 != intent.source.body_sha256
    {
        return Err(err(ErrorKind::RecoveryRequired));
    };
    Ok(v)
}
pub(super) fn parse_target(
    b: &[u8],
    intent: &Intent,
    source_copy_bytes: &[u8],
    original: &[u8],
    profile: &Path,
) -> Result<Target, ClientConfigError> {
    let v: Target = decode(b)?;
    if v.schema != 1
        || v.kind != "legacy-import-target"
        || v.binding != intent.binding
        || v.root != intent.root
        || v.source_copy_sha256 != hash(source_copy_bytes)
        || !successor::selector(&v.selector, profile)
    {
        return Err(err(ErrorKind::RecoveryRequired));
    };
    let target = target(original, &intent.old_selector, &v.selector, profile)?;
    if v.target_sha256 != hash(&target) || v.target_bytes != target.len() as u64 {
        return Err(err(ErrorKind::RecoveryRequired));
    };
    Ok(v)
}
pub(super) fn parse_completed(
    b: &[u8],
    owner_sid: &str,
    root: &Fid,
    profile: &Path,
) -> Result<CompletedInit, ClientConfigError> {
    let v: CompletedInit = decode(b)?;
    if v.schema != 2
        || v.product != "orbwin"
        || v.admission != "legacy-import"
        || v.policy != POLICY
        || v.owner_sid != owner_sid
        || v.source_namespace != SOURCE_NAMESPACE
        || (v.root_volume, v.root_high, v.root_low) != (root.volume, root.high, root.low)
        || !binding(&v.binding)
        || !witness(&v.source)
        || v.source.bytes == 0
        || !witness(&v.initial)
        || v.initial.bytes == 0
        || !successor::selector(&v.initial_selector, profile)
    {
        return Err(err(ErrorKind::RecoveryRequired));
    };
    Ok(v)
}
pub(super) fn captured_old_selector(bytes: &[u8]) -> Result<String, ClientConfigError> {
    let fields = raw_fields(bytes)?;
    known_fields(&fields, ROOT_FIELDS)?;
    let raw = fields
        .iter()
        .find(|(k, _)| k == "cliPluginsExtraDirs")
        .ok_or_else(|| err(ErrorKind::RoutingRequiresReview))?;
    let dirs: Vec<String> =
        serde_json::from_str(raw.1.get()).map_err(|_| err(ErrorKind::RoutingRequiresReview))?;
    if dirs.len() != 1 {
        return Err(err(ErrorKind::RoutingRequiresReview));
    };
    Ok(dirs[0].clone())
}
// The SAME function classifies historical stage bytes and creates a new target.
// Selectors are data only; CURRENT native runtime caps authorize publication.
pub(super) fn target(
    original: &[u8],
    old: &str,
    new: &str,
    profile: &Path,
) -> Result<Vec<u8>, ClientConfigError> {
    transform::transform(
        original,
        &transform::ReviewedSelectors {
            old_plugins: old.into(),
            current_plugins: new.into(),
            profile: profile
                .to_str()
                .ok_or_else(|| err(ErrorKind::Unavailable))?
                .into(),
        },
    )
    .map_err(|cause| {
        err(match cause {
            transform::Refusal::Shape => ErrorKind::LegacyImportShape,
            transform::Refusal::Secret => ErrorKind::LegacyImportSecrets,
            transform::Refusal::Routing => ErrorKind::LegacyImportRouting,
        })
    })
}
pub(super) fn uncommitted_prefix(bytes: &[u8], current: &Intent) -> bool {
    prefix::uncommitted(bytes, current)
}
pub(super) fn target_prefix(
    bytes: &[u8],
    current: &Target,
    original: &[u8],
    old: &str,
    profile: &Path,
) -> bool {
    prefix::target(bytes, current, original, old, profile)
}
// Verify the actual owning graph BEFORE transferring a newly admitted record
// out of its pending slot. Errors/unwind leave the slot and live attempt owned.
// This is sequencing only: the native callback supplies all real authority.
pub(super) fn verified_transfer<S, T>(
    state: &mut S,
    verify: impl FnOnce(&S) -> Result<(), ClientConfigError>,
    take: impl FnOnce(&mut S) -> T,
) -> Result<T, ClientConfigError> {
    verify(state)?;
    Ok(take(state))
}
// Native callsites capture their real returned handle into an owning slot
// BEFORE classifying return status. An error with a valid output, or success
// with no valid output, is uncertain; only error+no output is a known refusal.
pub(super) fn native_output<T, E>(
    success: bool,
    output: Option<T>,
    unknown: impl FnOnce(Option<T>) -> E,
    known_refusal: impl FnOnce() -> E,
) -> Result<T, E> {
    match (success, output) {
        (true, Some(cap)) => Ok(cap),
        (false, None) => Err(known_refusal()),
        (_, output) => Err(unknown(output)),
    }
}

#[cfg(test)]
#[path = "client_config_import_cut_tests.rs"]
mod cut_tests;
#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn native_partial_outputs_are_owned_before_classification_and_never_known_busy() {
        use std::{cell::Cell, rc::Rc};
        struct Cap(Rc<Cell<usize>>);
        impl Drop for Cap {
            fn drop(&mut self) {
                self.0.set(self.0.get() + 1)
            }
        }
        for (success, present) in [(false, false), (false, true), (true, false), (true, true)] {
            let closed = Rc::new(Cell::new(0));
            let mut retained = None;
            let mut unknown = 0;
            let mut refused = 0;
            let output = present.then(|| Cap(closed.clone()));
            let result = native_output(
                success,
                output,
                |cap| {
                    unknown += 1;
                    retained = cap;
                    err(ErrorKind::RecoveryRequired)
                },
                || {
                    refused += 1;
                    err(ErrorKind::Busy)
                },
            );
            assert_eq!(unknown, usize::from(success != present));
            assert_eq!(refused, usize::from(!success && !present));
            assert_eq!(closed.get(), 0);
            if success && present {
                assert!(result.is_ok());
                drop(result);
                assert_eq!(closed.get(), 1)
            } else {
                assert!(result.is_err());
                assert_eq!(retained.is_some(), present);
                drop(retained);
                assert_eq!(closed.get(), usize::from(present))
            }
        }
    }
    #[test]
    fn suffix_transfer_never_drops_pending_before_verification_or_on_unwind() {
        use std::{cell::Cell, rc::Rc};
        struct Resource(Rc<Cell<usize>>);
        impl Drop for Resource {
            fn drop(&mut self) {
                self.0.set(self.0.get() + 1)
            }
        }
        struct State {
            pending: Option<Resource>,
            live: bool,
        }
        for panic in [false, true] {
            let closes = Rc::new(Cell::new(0));
            let mut state = State {
                pending: Some(Resource(closes.clone())),
                live: true,
            };
            let outcome = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
                verified_transfer(
                    &mut state,
                    |s| {
                        assert!(s.pending.is_some());
                        assert!(s.live);
                        if panic {
                            panic!("final graph observation failed")
                        };
                        Err(err(ErrorKind::RecoveryRequired))
                    },
                    |s| {
                        s.live = false;
                        s.pending.take().unwrap()
                    },
                )
            }));
            assert!(outcome.is_err() || outcome.unwrap().is_err());
            assert!(state.pending.is_some());
            assert!(state.live);
            assert_eq!(closes.get(), 0);
            drop(state);
            assert_eq!(closes.get(), 1);
        }
        let closes = Rc::new(Cell::new(0));
        let mut state = State {
            pending: Some(Resource(closes.clone())),
            live: true,
        };
        let value = verified_transfer(
            &mut state,
            |s| {
                assert!(s.pending.is_some());
                assert!(s.live);
                Ok(())
            },
            |s| {
                s.live = false;
                s.pending.take().unwrap()
            },
        )
        .unwrap();
        assert!(!state.live);
        assert!(state.pending.is_none());
        assert_eq!(closes.get(), 0);
        drop(value);
        assert_eq!(closes.get(), 1);
    }
    fn witness(seed: &str) -> Witness {
        Witness {
            id: Fid {
                volume: 1,
                high: 2,
                low: u32::MAX,
            },
            sd_sha256: seed.repeat(64),
            body_sha256: seed.repeat(64),
            bytes: 1048576,
        }
    }
    fn intent(old: &str, seed: &str) -> Intent {
        Intent {
            schema: 1,
            kind: "legacy-client-import".into(),
            policy: POLICY.into(),
            binding: seed.repeat(64),
            owner_sid: "S-1-5-21-test".into(),
            root: Fid {
                volume: 1,
                high: 2,
                low: 3,
            },
            source_namespace: SOURCE_NAMESPACE.into(),
            source: witness(seed),
            old_selector: old.into(),
            scratch: Scratch {
                docker: Some(DirectoryWitness {
                    id: Fid {
                        volume: 2,
                        high: 3,
                        low: 4,
                    },
                    sd_sha256: "5".repeat(64),
                }),
                init: Some(witness("6")),
                config: Some(witness("7")),
            },
        }
    }
    #[test]
    fn explicit_uncommitted_restart_accepts_only_exact_producer_prefix_not_source_continuity() {
        let mut current = intent(r"D:\new\docker-cli-plugins", "2");
        current.source.id.low = 17;
        current.source.bytes = 37;
        for old in [
            r"C:\old\docker-cli-plugins",
            r"\\?\Z:\old&name\docker-cli-plugins",
            "C:\\你好😀\\docker-cli-plugins",
        ] {
            let prior = intent(old, "3");
            let encoded = encode(&prior).unwrap();
            for n in 0..encoded.len() {
                assert!(
                    uncommitted_prefix(&encoded[..n], &current),
                    "old={old:?} cut={n} prefix={:?}",
                    String::from_utf8_lossy(&encoded[..n])
                );
            }
            assert!(!uncommitted_prefix(&encoded, &current));
            let mut bad = encoded.clone();
            bad[1] = b' ';
            assert!(!uncommitted_prefix(&bad[..bad.len() - 1], &current));
            let mut other_root = prior.clone();
            other_root.root.low += 1;
            assert!(!uncommitted_prefix(
                &encode(&other_root).unwrap()[..encoded.len() - 1],
                &current
            ));
        }
        for bad in [br#"{"schema":1"#.as_slice(),br#"{"binding":"X"#,br#"{"binding":"0000000000000000000000000000000000000000000000000000000000000000","kind":"legacy-client-import","oldSelector":"C:\\x\\docker-cli-plugins","ownerSid":"other"#]{assert!(!uncommitted_prefix(bad,&current))}
    }
    #[test]
    fn every_historical_target_prefix_is_disposable_only_before_stage_or_publication() {
        let profile = Path::new(r"C:\Users\test");
        let old = r"C:\old\docker-cli-plugins";
        let original = format!(
            r#"{{"auths":{{}},"credsStore":"orbwin","cliPluginsExtraDirs":[{}],"unknown":1e9999}}"#,
            serde_json::to_string(old).unwrap()
        );
        let mut c = Target {
            schema: 1,
            kind: "legacy-import-target".into(),
            binding: "1".repeat(64),
            root: Fid {
                volume: 1,
                high: 2,
                low: 3,
            },
            source_copy_sha256: "4".repeat(64),
            selector: format!(
                r"{}\.orbwin-private\runtime\docker-{}\docker-cli-plugins",
                profile.display(),
                "c".repeat(64)
            ),
            target_sha256: String::new(),
            target_bytes: 0,
        };
        let output = target(original.as_bytes(), old, &c.selector, profile).unwrap();
        c.target_sha256 = hash(&output);
        c.target_bytes = output.len() as u64;
        let encoded = encode(&c).unwrap();
        let mut d = c.clone();
        d.selector = d.selector.replace(&"c".repeat(64), &"d".repeat(64));
        for n in 0..encoded.len() {
            assert!(
                target_prefix(&encoded[..n], &d, original.as_bytes(), old, profile),
                "cut {n}"
            )
        }
        assert!(!target_prefix(
            &encoded,
            &d,
            original.as_bytes(),
            old,
            profile
        ));
        let mut unknown = c.clone();
        unknown.kind = "different".into();
        assert!(!target_prefix(
            &encode(&unknown).unwrap(),
            &d,
            original.as_bytes(),
            old,
            profile
        ));
    }
    #[test]
    fn completed_init_golden_is_closed_and_bound_to_actual_native_facts() {
        let bytes = include_bytes!("../../fixtures/completed-import.json");
        let profile = Path::new(r"C:\Users\test");
        let root = Fid {
            volume: 1,
            high: 2,
            low: 3,
        };
        let parsed = parse_completed(bytes, "S-1-5-21-test", &root, profile).unwrap();
        assert_eq!(encode(&parsed).unwrap(), bytes);
        for n in 0..bytes.len() {
            assert!(
                parse_completed(&bytes[..n], "S-1-5-21-test", &root, profile).is_err(),
                "cut {n}"
            )
        }
        assert!(parse_completed(bytes, "other", &root, profile).is_err());
        assert!(parse_completed(
            bytes,
            "S-1-5-21-test",
            &Fid {
                volume: 1,
                high: 2,
                low: 4
            },
            profile
        )
        .is_err());
        assert!(
            parse_completed(bytes, "S-1-5-21-test", &root, Path::new(r"D:\Users\test")).is_err()
        );
        let text = std::str::from_utf8(bytes).unwrap();
        for bad in [
            text.replace("\"schema\":2", "\"schema\":2,\"unknown\":0"),
            text.replace("\"schema\":2", "\"schema\":2,\"schema\":2"),
            text.replace("\"volume\":4", "\"volume\":null"),
            text.replace("\"rootVolume\":1", "\"rootVolume\":\"1\""),
            text.replace(&"1".repeat(64), &"0".repeat(64)),
        ] {
            assert!(parse_completed(bad.as_bytes(), "S-1-5-21-test", &root, profile).is_err())
        }
    }
    #[test]
    fn every_marker_cut_has_explicit_disjoint_or_choice_class() {
        for literal in [FRESH_MARKER, MARKER] {
            for n in 0..=literal.len() {
                let b = &literal[..n];
                let both = FRESH_MARKER.starts_with(b) && MARKER.starts_with(b);
                assert_eq!(marker_kind(b) == MarkerKind::Shared, both);
                if !both {
                    assert_eq!(
                        marker_kind(b),
                        if literal == FRESH_MARKER {
                            MarkerKind::Fresh
                        } else {
                            MarkerKind::Import
                        }
                    )
                }
            }
        }
        for bad in [
            b" ".as_slice(),
            b"{}",
            b"{\"kind\":\"fresh-client\",\"schema\":1}",
            b"{\"schema\":1,\"kind\":\"other\"}",
        ] {
            assert_eq!(marker_kind(bad), MarkerKind::Unsupported)
        }
    }
}
