//! Pure DATA observations only. No native owner/role constructor or I/O.
use super::*;
fn fixture() -> (Intent, Vec<u8>, SourceCopy) {
    let source=br#"{"auths":{},"credsStore":"orbwin","cliPluginsExtraDirs":["C:\\old\\docker-cli-plugins"],"unknown":1e9999,"unknown":{"raw":true}}"#.to_vec();
    let w = Witness {
        id: Fid {
            volume: 1,
            high: u32::MAX,
            low: 3,
        },
        sd_sha256: "2".repeat(64),
        body_sha256: hash(&source),
        bytes: source.len() as u64,
    };
    let intent = Intent {
        schema: 1,
        kind: "legacy-client-import".into(),
        policy: POLICY.into(),
        binding: "1".repeat(64),
        owner_sid: "S-1-5-21-test".into(),
        root: Fid {
            volume: 1,
            high: 2,
            low: 3,
        },
        source_namespace: SOURCE_NAMESPACE.into(),
        source: w.clone(),
        old_selector: r"C:\old\docker-cli-plugins".into(),
        scratch: Scratch {
            docker: None,
            init: None,
            config: None,
        },
    };
    let mut original = w;
    original.id.low = 17;
    original.sd_sha256 = "4".repeat(64);
    let copy = SourceCopy {
        schema: 1,
        kind: "legacy-source-copy".into(),
        binding: intent.binding.clone(),
        root: intent.root.clone(),
        intent_sha256: hash(&encode(&intent).unwrap()),
        original,
    };
    (intent, source, copy)
}
#[test]
fn full_intent_and_source_copy_are_independent_closed_records_at_every_cut() {
    let (intent, original, copy) = fixture();
    let intent_bytes = encode(&intent).unwrap();
    let copy_bytes = encode(&copy).unwrap();
    assert_eq!(parse_intent(&intent_bytes).unwrap(), intent);
    assert_eq!(
        parse_source_copy(&copy_bytes, &intent, &intent_bytes).unwrap(),
        copy
    );
    for n in 0..intent_bytes.len() {
        assert!(parse_intent(&intent_bytes[..n]).is_err())
    }
    for n in 0..copy_bytes.len() {
        assert!(parse_source_copy(&copy_bytes[..n], &intent, &intent_bytes).is_err())
    }
    for mutation in 0..5 {
        let mut bad = copy.clone();
        match mutation {
            0 => bad.original.body_sha256 = "0".repeat(64),
            1 => bad.original.bytes += 1,
            2 => bad.binding = "3".repeat(64),
            3 => bad.root.low += 1,
            _ => bad.intent_sha256 = "4".repeat(64),
        }
        assert!(parse_source_copy(&encode(&bad).unwrap(), &intent, &intent_bytes).is_err())
    }
    // The private original's identity/descriptor are independent of the public
    // source. Their real native verification belongs to the owner, not this DTO.
    assert_ne!(copy.original.id, intent.source.id);
    assert_ne!(copy.original.sd_sha256, intent.source.sd_sha256);
    assert_eq!(copy.original.body_sha256, hash(&original));
    let text = String::from_utf8(intent_bytes.clone()).unwrap();
    for bad in [
        text.replace("\"schema\":1", "\"schema\":1,\"other\":0"),
        text.replace("\"schema\":1", "\"schema\":1,\"schema\":1"),
        text.replace("\"volume\":1", "\"volume\":null"),
    ] {
        assert!(parse_intent(bad.as_bytes()).is_err())
    }
}
#[test]
fn historical_target_uses_same_strict_transform_and_exact_raw_preservation() {
    let (intent, original, copy) = fixture();
    let copy_bytes = encode(&copy).unwrap();
    let profile = Path::new(r"C:\Users\test");
    let selector = format!(
        r"{}\.orbwin-private\runtime\docker-{}\docker-cli-plugins",
        profile.display(),
        "c".repeat(64)
    );
    let output = target(&original, &intent.old_selector, &selector, profile).unwrap();
    assert!(std::str::from_utf8(&output)
        .unwrap()
        .contains(r#""unknown":1e9999,"unknown":{"raw":true}"#));
    let record = Target {
        schema: 1,
        kind: "legacy-import-target".into(),
        binding: intent.binding.clone(),
        root: intent.root.clone(),
        source_copy_sha256: hash(&copy_bytes),
        selector,
        target_sha256: hash(&output),
        target_bytes: output.len() as u64,
    };
    let bytes = encode(&record).unwrap();
    assert_eq!(
        parse_target(&bytes, &intent, &copy_bytes, &original, profile).unwrap(),
        record
    );
    for n in 0..bytes.len() {
        assert!(parse_target(&bytes[..n], &intent, &copy_bytes, &original, profile).is_err())
    }
    for mutation in 0..5 {
        let mut bad = record.clone();
        match mutation {
            0 => bad.target_sha256 = "0".repeat(64),
            1 => bad.target_bytes += 1,
            2 => bad.source_copy_sha256 = "3".repeat(64),
            3 => bad.selector = bad.selector.replace(r"C:\Users\test", r"D:\Users\test"),
            _ => bad.binding = "4".repeat(64),
        }
        assert!(parse_target(
            &encode(&bad).unwrap(),
            &intent,
            &copy_bytes,
            &original,
            profile
        )
        .is_err())
    }
    let changed = String::from_utf8(original.clone()).unwrap().replace(
        "\"auths\":{}",
        "\"auths\":{\"registry.example\":{\"auth\":\"sensitive-fixture\"}}",
    );
    assert!(parse_target(&bytes, &intent, &copy_bytes, changed.as_bytes(), profile).is_err());
    assert_eq!(original, fixture().1);
}
#[test]
fn import_only_refusals_preserve_raw_source_and_explain_known_secret_without_echo() {
    let (intent, source, _) = fixture();
    let profile = Path::new(r"C:\Users\test");
    let selector = format!(
        r"{}\.orbwin-private\runtime\docker-{}\docker-cli-plugins",
        profile.display(),
        "d".repeat(64)
    );
    let base = String::from_utf8(source).unwrap();
    for (input, kind) in [
        (
            base.replace(
                "\"auths\":{}",
                "\"auths\":{\"registry.example\":{\"password\":\"sensitive-fixture\"}}",
            ),
            ErrorKind::LegacyImportSecrets,
        ),
        (
            base.replace("\"credsStore\":\"orbwin\"", "\"credsStore\":\"foreign\""),
            ErrorKind::LegacyImportRouting,
        ),
        (
            base.replace(
                "\"auths\":{}",
                "\"auths\":{},\"HttpHeaders\":{\"first\":17,\"first\":\"valid\"}",
            ),
            ErrorKind::LegacyImportShape,
        ),
    ] {
        let before = input.as_bytes().to_vec();
        let error = target(input.as_bytes(), &intent.old_selector, &selector, profile).unwrap_err();
        assert_eq!(error.kind, kind);
        assert!(!error.to_string().contains("sensitive-fixture"));
        assert_eq!(before, input.as_bytes());
    }
}
