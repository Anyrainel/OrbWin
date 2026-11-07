//! Bounded metadata/data transforms only. Native callers supply all authority.
use super::*;
use sha2::{Digest, Sha256};
pub(super) const BUDGET: usize = 16 * 1024;
#[derive(Clone, Debug, PartialEq, Eq, serde::Deserialize, serde::Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(super) struct V2 {
    pub schema: u32,
    pub kind: String,
    pub binding: String,
    pub current_volume: u32,
    pub current_high: u32,
    pub current_low: u32,
    #[serde(rename = "currentSdSha256")]
    pub current_sd: String,
    #[serde(rename = "currentSha256")]
    pub current_sha: String,
    #[serde(rename = "restoreSha256")]
    pub restore_sha: String,
    pub intent_body: String,
    pub target_selector: String,
    #[serde(rename = "publishedBaselineSha256")]
    pub published_baseline_sha: String,
    pub previous_volume: u32,
    pub previous_high: u32,
    pub previous_low: u32,
    #[serde(rename = "previousSdSha256")]
    pub previous_sd: String,
    #[serde(rename = "previousSha256")]
    pub previous_sha: String,
    #[serde(rename = "previousRestoreSha256")]
    pub previous_restore_sha: String,
    pub stage_present: bool,
    pub stage_volume: u32,
    pub stage_high: u32,
    pub stage_low: u32,
    #[serde(rename = "stageSdSha256")]
    pub stage_sd: String,
    #[serde(rename = "stageSha256")]
    pub stage_sha: String,
}
pub(super) struct Record {
    pub base: RestoreIntent,
    pub v2: Option<V2>,
}
pub(super) fn hash(bytes: &[u8]) -> String {
    format!("{:x}", Sha256::digest(bytes))
}
pub(super) fn selector(value: &str, profile: &Path) -> bool {
    if value.encode_utf16().count() > 1024 {
        return false;
    }
    let prefix = profile
        .join(".orbwin-private")
        .join("runtime")
        .join("docker-");
    let Some(prefix) = prefix.to_str() else {
        return false;
    };
    let suffix = format!("{}docker-cli-plugins", std::path::MAIN_SEPARATOR);
    let Some(key) = value
        .strip_prefix(prefix)
        .and_then(|v| v.strip_suffix(&suffix))
    else {
        return false;
    };
    digest_hex(key).is_ok_and(|v| v != [0; 32])
}
fn selector_shape(value: &str) -> bool {
    // Separators and the fixed namespace are separate projection-friendly data.
    let suffix = format!(
        "{}{}{}runtime{}docker-",
        std::path::MAIN_SEPARATOR,
        ".orbwin-private",
        std::path::MAIN_SEPARATOR,
        std::path::MAIN_SEPARATOR
    );
    let Some(at) = value.find(&suffix) else {
        return false;
    };
    at >= 3 && selector(value, Path::new(&value[..at]))
}
impl V2 {
    pub fn base(&self) -> RestoreIntent {
        RestoreIntent {
            schema: self.schema,
            kind: self.kind.clone(),
            binding: self.binding.clone(),
            current_volume: self.current_volume,
            current_high: self.current_high,
            current_low: self.current_low,
            current_sd_sha256: self.current_sd.clone(),
            current_sha256: self.current_sha.clone(),
            restore_sha256: self.restore_sha.clone(),
            intent_body: self.intent_body.clone(),
        }
    }
    pub fn from_base(base: &RestoreIntent, target_selector: String) -> Self {
        Self {
            schema: 2,
            kind: base.kind.clone(),
            binding: base.binding.clone(),
            current_volume: base.current_volume,
            current_high: base.current_high,
            current_low: base.current_low,
            current_sd: base.current_sd_sha256.clone(),
            current_sha: base.current_sha256.clone(),
            restore_sha: base.restore_sha256.clone(),
            intent_body: base.intent_body.clone(),
            target_selector,
            published_baseline_sha: String::new(),
            previous_volume: 0,
            previous_high: 0,
            previous_low: 0,
            previous_sd: String::new(),
            previous_sha: String::new(),
            previous_restore_sha: String::new(),
            stage_present: false,
            stage_volume: 0,
            stage_high: 0,
            stage_low: 0,
            stage_sd: String::new(),
            stage_sha: String::new(),
        }
    }
    pub fn bytes(&self) -> Result<Vec<u8>, ClientConfigError> {
        let bytes = go_json(self)?;
        parse(&bytes)?;
        Ok(bytes)
    }
    pub fn same_authority(&self, other: &Self) -> bool {
        let mut a = self.clone();
        let mut b = other.clone();
        a.restore_sha.clear();
        b.restore_sha.clear();
        a.target_selector.clear();
        b.target_selector.clear();
        a == b
    }
}
pub(super) fn parse(bytes: &[u8]) -> Result<Record, ClientConfigError> {
    if bytes.len() > BUDGET {
        return Err(err(ErrorKind::RecoveryRequired));
    }
    let value = checked_json(bytes)?;
    let (base, v2) = if value.get("schema") == Some(&serde_json::json!(1)) {
        if bytes.len() > 8192 {
            return Err(err(ErrorKind::RecoveryRequired));
        }
        (
            serde_json::from_slice::<RestoreIntent>(bytes)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?,
            None,
        )
    } else {
        let v: V2 = serde_json::from_slice(bytes).map_err(|_| err(ErrorKind::RecoveryRequired))?;
        if v.schema != 2 || !selector_shape(&v.target_selector) {
            return Err(err(ErrorKind::RecoveryRequired));
        }
        if !v.published_baseline_sha.is_empty() {
            digest_hex(&v.published_baseline_sha)?;
            if v.previous_sha.is_empty() {
                return Err(err(ErrorKind::RecoveryRequired));
            }
        }
        if v.previous_sha.is_empty() {
            if (v.previous_volume, v.previous_high, v.previous_low) != (0, 0, 0)
                || !v.previous_sd.is_empty()
                || !v.previous_restore_sha.is_empty()
            {
                return Err(err(ErrorKind::RecoveryRequired));
            }
        } else {
            digest_hex(&v.previous_sha)?;
            digest_hex(&v.previous_sd)?;
            digest_hex(&v.previous_restore_sha)?;
        }
        if v.stage_present {
            digest_hex(&v.stage_sd)?;
            digest_hex(&v.stage_sha)?;
        } else if (v.stage_volume, v.stage_high, v.stage_low) != (0, 0, 0)
            || !v.stage_sd.is_empty()
            || !v.stage_sha.is_empty()
        {
            return Err(err(ErrorKind::RecoveryRequired));
        }
        (v.base(), Some(v))
    };
    if base.kind != "restore-original" || digest_hex(&base.binding)? == [0; 32] {
        return Err(err(ErrorKind::RecoveryRequired));
    }
    digest_hex(&base.current_sd_sha256)?;
    digest_hex(&base.current_sha256)?;
    digest_hex(&base.restore_sha256)?;
    if parse_recovery_intent(base.intent_body.as_bytes())?.binding != base.binding {
        return Err(err(ErrorKind::RecoveryRequired));
    }
    Ok(Record { base, v2 })
}
pub(super) fn incomplete(bytes: &[u8]) -> bool {
    bytes.len() <= BUDGET && incomplete_record(bytes)
}
pub(super) fn owned_selector(bytes: &[u8], profile: &Path) -> Result<String, ClientConfigError> {
    let fields = raw_fields(bytes)?;
    known_fields(&fields, ROOT_FIELDS)?;
    let field = fields
        .iter()
        .find(|(name, _)| name == "cliPluginsExtraDirs")
        .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
    let paths: Vec<String> =
        serde_json::from_str(field.1.get()).map_err(|_| err(ErrorKind::RecoveryRequired))?;
    if paths.len() != 1 || !selector(&paths[0], profile) {
        return Err(err(ErrorKind::RecoveryRequired));
    }
    parse_config(bytes, Path::new(&paths[0]))?;
    Ok(paths[0].clone())
}
pub(super) fn target(
    bytes: &[u8],
    plugins: &Path,
    profile: &Path,
) -> Result<Vec<u8>, ClientConfigError> {
    if !plugins.to_str().is_some_and(|v| selector(v, profile)) {
        return Err(err(ErrorKind::RecoveryRequired));
    }
    let next = refresh_managed_field(bytes, plugins, profile)?.unwrap_or_else(|| bytes.to_vec());
    parse_config(&next, plugins)?;
    Ok(next)
}
pub(super) fn published(
    base: &RestoreIntent,
    v2: Option<&V2>,
    prior: bool,
    strict: bool,
    hash: &str,
) -> bool {
    strict
        && (hash == base.restore_sha256
            || v2.is_some_and(|v| {
                v.binding == base.binding
                    && v.intent_body == base.intent_body
                    && !v.previous_sha.is_empty()
                    && !v.published_baseline_sha.is_empty()
                    && prior
                    && hash == v.published_baseline_sha
            }))
}
pub(super) struct DataPlan {
    pub baseline: Vec<u8>,
    pub target: Vec<u8>,
    pub needs_publish: bool,
}
// This is called by the production native coordinator, not just predicates in
// tests. It returns bytes only; prior/strict are native observations, no grants.
pub(super) fn data_plan(
    baseline: &[u8],
    current: &[u8],
    profile: &Path,
    plugins: &Path,
    from_original: bool,
    record: Option<&Record>,
    prior: bool,
    strict: bool,
    authority: impl FnOnce() -> Result<(), ClientConfigError>,
) -> Result<DataPlan, ClientConfigError> {
    if !from_original {
        if !strict {
            return Err(err(ErrorKind::RecoveryRequired));
        }
        owned_selector(current, profile)?;
    }
    authority()?;
    let baseline = if from_original {
        baseline
    } else {
        let record = record.ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
        if !published(
            &record.base,
            record.v2.as_ref(),
            prior,
            strict,
            &hash(current),
        ) {
            return Err(err(ErrorKind::RecoveryRequired));
        }
        current
    };
    let target = target(baseline, plugins, profile)?;
    if let Some(record) = record {
        if let Some(v) = record.v2.as_ref() {
            let old = target_bytes(baseline, &v.target_selector, profile)?;
            if hash(&old) != record.base.restore_sha256 {
                return Err(err(ErrorKind::RecoveryRequired));
            }
        }
    }
    Ok(DataPlan {
        baseline: baseline.to_vec(),
        needs_publish: from_original || target != current,
        target,
    })
}
fn target_bytes(bytes: &[u8], plugins: &str, profile: &Path) -> Result<Vec<u8>, ClientConfigError> {
    target(bytes, Path::new(plugins), profile)
}
pub(super) fn next_prefix(bytes: &[u8], expected: &V2, profile: &Path) -> bool {
    if !incomplete(bytes) {
        return false;
    }
    let mut v = expected.clone();
    v.restore_sha = "0".repeat(64);
    v.target_selector = profile
        .join(".orbwin-private")
        .join("runtime")
        .join(format!("docker-{}", "0".repeat(64)))
        .join("docker-cli-plugins")
        .to_str()
        .unwrap_or("")
        .into();
    let Ok(template) = go_json(&v) else {
        return false;
    };
    if template.len() > BUDGET || bytes.len() >= template.len() {
        return false;
    }
    let find = |needle: &[u8]| {
        template
            .windows(needle.len())
            .position(|p| p == needle)
            .map(|i| i + needle.len())
    };
    let Some(digest) = find(br#""restoreSha256":""#) else {
        return false;
    };
    let Some(start) = find(br#""targetSelector":"#) else {
        return false;
    };
    let prefix = profile
        .join(".orbwin-private")
        .join("runtime")
        .join("docker-");
    let Ok(prefix) = go_json(prefix.to_str().unwrap_or("")) else {
        return false;
    };
    let hex = start + prefix.len() - 1;
    let mut nonzero = false;
    for (i, b) in bytes.iter().copied().enumerate() {
        if (digest..digest + 64).contains(&i) || (hex..hex + 64).contains(&i) {
            if !b.is_ascii_digit() && !(b'a'..=b'f').contains(&b) {
                return false;
            }
            if i >= hex {
                nonzero |= b != b'0';
            }
        } else if template[i] != b {
            return false;
        }
    }
    bytes.len() < hex + 64 || nonzero
}
pub(super) fn validate_next(
    bytes: &[u8],
    expected: &V2,
    baseline: &[u8],
    profile: &Path,
) -> Result<bool, ClientConfigError> {
    if incomplete(bytes) {
        return if next_prefix(bytes, expected, profile) {
            Ok(true)
        } else {
            Err(err(ErrorKind::RecoveryRequired))
        };
    }
    let v = parse(bytes)?
        .v2
        .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
    if !v.same_authority(expected)
        || !selector(&v.target_selector, profile)
        || hash(&target_bytes(baseline, &v.target_selector, profile)?) != v.restore_sha
    {
        return Err(err(ErrorKind::RecoveryRequired));
    }
    Ok(false)
}
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(super) enum Step {
    Durable,
    Recheck,
    Publish,
}
pub(super) fn transition(
    mut call: impl FnMut(Step) -> Result<(), ClientConfigError>,
) -> Result<(), ClientConfigError> {
    for step in [Step::Durable, Step::Recheck, Step::Publish] {
        call(step)?;
    }
    Ok(())
}
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(super) enum CompletedStep {
    Check,
    Cleanup,
    Refresh,
}
pub(super) fn completed(
    mut call: impl FnMut(CompletedStep) -> Result<(), ClientConfigError>,
) -> Result<(), ClientConfigError> {
    for step in [
        CompletedStep::Check,
        CompletedStep::Cleanup,
        CompletedStep::Refresh,
    ] {
        call(step)?;
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    fn plugin(profile: &Path, key: char) -> PathBuf {
        profile
            .join(".orbwin-private")
            .join("runtime")
            .join(format!("docker-{}", key.to_string().repeat(64)))
            .join("docker-cli-plugins")
    }
    fn intent() -> String {
        go_json(&RecoveryIntent {
            schema: 3,
            kind: "docker-writer".into(),
            mutation: "registry-entries-v2".into(),
            binding: "1".repeat(64),
            keys_sha256: "2".repeat(64),
            base_sha256: "3".repeat(64),
            base_volume: 1,
            base_high: 2,
            base_low: 3,
            base_sd_sha256: "4".repeat(64),
            original_volume: 5,
            original_high: 6,
            original_low: 7,
            original_sd_sha256: "5".repeat(64),
            original_sha256: "3".repeat(64),
        })
        .map(|b| String::from_utf8(b).unwrap())
        .unwrap()
    }
    fn config(path: &Path) -> Vec<u8> {
        let mut b=serde_json::to_vec(&serde_json::json!({"auths":{"kept":{"auth":"synthetic-fixture"}},"credsStore":"orbwin","cliPluginsExtraDirs":[path]})).unwrap();
        b.pop();
        b.extend(br#", "unknownHuge":1e9999}"#);
        b
    }
    fn base(current: &[u8], restored: &[u8]) -> RestoreIntent {
        RestoreIntent {
            schema: 1,
            kind: "restore-original".into(),
            binding: "1".repeat(64),
            current_volume: 8,
            current_high: 9,
            current_low: 10,
            current_sd_sha256: "6".repeat(64),
            current_sha256: hash(current),
            restore_sha256: hash(restored),
            intent_body: intent(),
        }
    }
    fn linked(base: &RestoreIntent, path: &Path) -> V2 {
        let mut v = V2::from_base(base, path.to_str().unwrap().into());
        v.previous_volume = 1;
        v.previous_high = 2;
        v.previous_low = 3;
        v.previous_sd = "7".repeat(64);
        v.previous_sha = "8".repeat(64);
        v.previous_restore_sha = "9".repeat(64);
        v
    }
    #[test]
    fn closed_codec_has_exact_go_field_order_and_no_embedded_config() {
        let profile = Path::new("C:\\Users\\A&B\u{2028}");
        let p = plugin(profile, 'a');
        let body = config(&p);
        let v = linked(&base(&body, &body), &p);
        let encoded = v.bytes().unwrap();
        let record = parse(&encoded).unwrap();
        assert_eq!(record.v2.unwrap(), v);
        let fields = raw_fields(&encoded).unwrap();
        assert_eq!(
            fields.iter().map(|(n, _)| n.as_str()).collect::<Vec<_>>(),
            [
                "schema",
                "kind",
                "binding",
                "currentVolume",
                "currentHigh",
                "currentLow",
                "currentSdSha256",
                "currentSha256",
                "restoreSha256",
                "intentBody",
                "targetSelector",
                "publishedBaselineSha256",
                "previousVolume",
                "previousHigh",
                "previousLow",
                "previousSdSha256",
                "previousSha256",
                "previousRestoreSha256",
                "stagePresent",
                "stageVolume",
                "stageHigh",
                "stageLow",
                "stageSdSha256",
                "stageSha256"
            ]
        );
        let text = String::from_utf8(encoded.clone()).unwrap();
        assert!(text.contains("\\u0026") && text.contains("\\u2028"));
        assert!(!text.contains("synthetic-fixture") && !text.contains("unknownHuge"));
        for bad in [
            text.replace("\"schema\":2", "\"schema\":3"),
            text.replace("\"stagePresent\":false", "\"stagePresent\":null"),
            text.replace("\"stagePresent\":false", "\"StagePresent\":false"),
            text.replace(
                "\"stagePresent\":false",
                "\"stagePresent\":false,\"stagePresent\":false",
            ),
            text.replace(
                "\"stagePresent\":false",
                "\"unknown\":0,\"stagePresent\":false",
            ),
        ] {
            assert!(parse(bad.as_bytes()).is_err());
        }
        let mut missing: Value = serde_json::from_slice(&encoded).unwrap();
        missing.as_object_mut().unwrap().remove("previousSha256");
        assert!(parse(&serde_json::to_vec(&missing).unwrap()).is_err());
        let old = go_json(&base(&body, &body)).unwrap();
        assert!(parse(&old).unwrap().v2.is_none());
    }
    #[test]
    fn codec_matches_actual_go_recovery_v2_bytes_golden() {
        // Actual current Go recoveryV2Bytes, invoked by a pure source overlay:
        // artifacts/qualification/p2-successor-codec-pure-20261026/proof.json.
        // No native provider/private config or role execution produced this.
        let profile = Path::new("C:\\Users\\A&B\u{2028}");
        let mut base = base(b"", b"");
        base.current_sha256 = "a".repeat(64);
        base.restore_sha256 = "b".repeat(64);
        let mut v = linked(&base, &plugin(profile, 'a'));
        v.published_baseline_sha = "c".repeat(64);
        v.stage_present = true;
        v.stage_volume = 4;
        v.stage_high = 5;
        v.stage_low = 6;
        v.stage_sd = "a".repeat(64);
        v.stage_sha = "b".repeat(64);
        let bytes = v.bytes().unwrap();
        assert_eq!(bytes.len(), 1924);
        assert_eq!(
            hash(&bytes),
            "fe06a5b09f0301a93c864ed128487f2a13e156e2560429a04c5a9e570c359092"
        );
    }
    #[test]
    fn actual_data_coordinator_admits_published_b_before_constructing_c_or_d() {
        let profile = Path::new("C:\\Users\\fixture");
        let a = plugin(profile, 'a');
        let b = plugin(profile, 'b');
        let c = plugin(profile, 'c');
        let d = plugin(profile, 'd');
        let original = config(&a);
        let published_b = target(&original, &b, profile).unwrap();
        let record = Record {
            base: base(b"corrupt-current", &published_b),
            v2: None,
        };
        let plan = data_plan(
            &[],
            &published_b,
            profile,
            &c,
            false,
            Some(&record),
            false,
            true,
            || Ok(()),
        )
        .unwrap();
        assert_eq!(plan.baseline, published_b);
        assert_eq!(plan.target, target(&published_b, &c, profile).unwrap());
        assert!(
            data_plan(
                &[],
                &original,
                profile,
                &c,
                false,
                Some(&record),
                true,
                true,
                || Ok(())
            )
            .is_err(),
            "recorded prestate is not a published baseline"
        );
        let old_base = base(&published_b, &plan.target);
        let mut v = linked(&old_base, &c);
        v.published_baseline_sha = hash(&published_b);
        let record = Record {
            base: v.base(),
            v2: Some(v),
        };
        let upgrade = data_plan(
            &[],
            &published_b,
            profile,
            &d,
            false,
            Some(&record),
            true,
            true,
            || Ok(()),
        )
        .unwrap();
        assert_eq!(upgrade.target, target(&published_b, &d, profile).unwrap());
        for (prior, strict) in [(false, true), (true, false)] {
            assert!(data_plan(
                &[],
                &published_b,
                profile,
                &d,
                false,
                Some(&record),
                prior,
                strict,
                || Ok(())
            )
            .is_err());
        }
        assert!(data_plan(
            &[],
            &published_b,
            profile,
            &d,
            false,
            Some(&record),
            true,
            true,
            || Err(err(ErrorKind::RecoveryRequired))
        )
        .is_err());
        let from_original = data_plan(
            &original,
            b"not-json-writer-output",
            profile,
            &d,
            true,
            Some(&Record {
                base: base(b"not-json-writer-output", &published_b),
                v2: None,
            }),
            true,
            false,
            || Ok(()),
        )
        .unwrap();
        assert_eq!(
            from_original.target,
            target(&original, &d, profile).unwrap()
        );
        assert!(String::from_utf8(from_original.target)
            .unwrap()
            .contains("1e9999"));
    }
    #[test]
    fn next_every_exact_producer_prefix_is_bound_to_old_current_and_stage() {
        let profile = Path::new("C:\\Users\\A&B\u{2028}");
        let p = plugin(profile, 'a');
        let body = config(&p);
        let mut expected = linked(&base(&body, &body), &p);
        expected.stage_present = true;
        expected.stage_volume = 4;
        expected.stage_high = 5;
        expected.stage_low = 6;
        expected.stage_sd = "a".repeat(64);
        expected.stage_sha = "b".repeat(64);
        let encoded = expected.bytes().unwrap();
        for end in 0..encoded.len() {
            assert_eq!(
                validate_next(&encoded[..end], &expected, &body, profile).unwrap(),
                true,
                "prefix {end}"
            );
        }
        assert!(!validate_next(&encoded, &expected, &body, profile).unwrap());
        let mut wrong = expected.clone();
        wrong.current_low += 1;
        let bytes = wrong.bytes().unwrap();
        assert!(validate_next(&bytes[..bytes.len() - 1], &expected, &body, profile).is_err());
        assert!(validate_next(&bytes, &expected, &body, profile).is_err());
        for bytes in [
            br#"{"schema":2, "kind":"#.as_slice(),
            b"[",
            b"\"",
            br#"{"schema":2,"kind":"restore-original"}"#,
        ] {
            assert!(validate_next(bytes, &expected, &body, profile).is_err());
        }
        let mut stale = expected.clone();
        let older = plugin(profile, 'b');
        stale.target_selector = older.to_str().unwrap().into();
        stale.restore_sha = hash(&target(&body, &older, profile).unwrap());
        assert!(!validate_next(&stale.bytes().unwrap(), &expected, &body, profile).unwrap());
        stale.restore_sha = "f".repeat(64);
        assert!(validate_next(&stale.bytes().unwrap(), &expected, &body, profile).is_err());
    }
    #[test]
    fn bounded_metadata_preserves_large_escape_prefixes_without_recursive_journals() {
        let profile = PathBuf::from(format!("C:\\{}", "<&\u{2028}".repeat(260)));
        let p = plugin(&profile, 'a');
        let body = config(&p);
        let mut base = base(&body, &body);
        let padding = 4096 - base.intent_body.len();
        base.intent_body = format!("{}{}", "\t".repeat(padding), base.intent_body);
        assert_eq!(base.intent_body.len(), 4096);
        let v = linked(&base, &p);
        let encoded = v.bytes().unwrap();
        assert!(encoded.len() > 8192 && encoded.len() <= 15_501 && encoded.len() <= BUDGET);
        for end in [8193, encoded.len() - 1] {
            assert!(next_prefix(&encoded[..end], &v, &profile));
        }
        assert!(parse(&vec![b' '; BUDGET + 1]).is_err());
        assert!(!next_prefix(&vec![b' '; BUDGET + 1], &v, &profile));
    }
    #[test]
    fn actual_order_coordinators_stop_before_later_effects_on_each_failure() {
        for fail in 0..3 {
            let mut visited = Vec::new();
            assert!(transition(|step| {
                visited.push(step);
                if visited.len() - 1 == fail {
                    Err(err(ErrorKind::RecoveryRequired))
                } else {
                    Ok(())
                }
            })
            .is_err());
            assert_eq!(visited.len(), fail + 1);
        }
        let mut visited = Vec::new();
        transition(|step| {
            visited.push(step);
            Ok(())
        })
        .unwrap();
        assert_eq!(visited, [Step::Durable, Step::Recheck, Step::Publish]);
        for fail in 0..3 {
            let mut visited = Vec::new();
            assert!(completed(|step| {
                visited.push(step);
                if visited.len() - 1 == fail {
                    Err(err(ErrorKind::RecoveryRequired))
                } else {
                    Ok(())
                }
            })
            .is_err());
            assert_eq!(visited.len(), fail + 1);
        }
        let mut visited = Vec::new();
        completed(|step| {
            visited.push(step);
            Ok(())
        })
        .unwrap();
        assert_eq!(
            visited,
            [
                CompletedStep::Check,
                CompletedStep::Cleanup,
                CompletedStep::Refresh
            ]
        );
    }
}
