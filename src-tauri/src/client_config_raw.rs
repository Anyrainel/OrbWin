use super::*;
fn bounded(raw: &str, depth: usize, nodes: &mut usize) -> Result<(), ClientConfigError> {
    if depth > 64 || *nodes > 65536 {
        return Err(err(ErrorKind::RoutingRequiresReview));
    }
    *nodes += 1;
    match raw.as_bytes().first() {
        Some(b'{') => {
            for (_, v) in
                raw_fields(raw.as_bytes()).map_err(|_| err(ErrorKind::RoutingRequiresReview))?
            {
                bounded(v.get(), depth + 1, nodes)?
            }
        }
        Some(b'[') => {
            for v in serde_json::from_str::<Vec<Box<serde_json::value::RawValue>>>(raw)
                .map_err(|_| err(ErrorKind::RoutingRequiresReview))?
            {
                bounded(v.get(), depth + 1, nodes)?
            }
        }
        _ => {}
    }
    Ok(())
}

fn valid_unicode_json_strings(bytes: &[u8]) -> bool {
    if std::str::from_utf8(bytes).is_err() {
        return false;
    };
    let mut inside = false;
    let mut i = 0;
    let hex = |at: usize| -> Option<u16> {
        if at + 4 > bytes.len() {
            return None;
        };
        let mut n = 0;
        for &b in &bytes[at..at + 4] {
            n = n * 16
                + match b {
                    b'0'..=b'9' => (b - b'0') as u16,
                    b'a'..=b'f' => (b - b'a' + 10) as u16,
                    b'A'..=b'F' => (b - b'A' + 10) as u16,
                    _ => return None,
                }
        }
        Some(n)
    };
    while i < bytes.len() {
        match bytes[i] {
            b'"' => inside = !inside,
            b'\\' if inside => {
                i += 1;
                if i >= bytes.len() {
                    return false;
                };
                if bytes[i] == b'u' {
                    let Some(n) = hex(i + 1) else { return false };
                    i += 4;
                    if (0xd800..=0xdbff).contains(&n) {
                        if i + 6 >= bytes.len() || bytes[i + 1] != b'\\' || bytes[i + 2] != b'u' {
                            return false;
                        };
                        let Some(low) = hex(i + 3) else { return false };
                        if !(0xdc00..=0xdfff).contains(&low) {
                            return false;
                        };
                        i += 6
                    } else if (0xdc00..=0xdfff).contains(&n) {
                        return false;
                    }
                }
            }
            _ => {}
        };
        i += 1
    }
    !inside
}

pub(super) fn validate(bytes: &[u8]) -> Result<(), ClientConfigError> {
    if bytes.len() > CONFIG_BUDGET || !valid_unicode_json_strings(bytes) {
        return Err(err(ErrorKind::RoutingRequiresReview));
    }
    let raw: Box<serde_json::value::RawValue> =
        serde_json::from_slice(bytes).map_err(|_| err(ErrorKind::RoutingRequiresReview))?;
    bounded(raw.get(), 0, &mut 0)
}

#[cfg(test)]
mod adoption_tests {
    use super::super::{merge_registry_auths, parse_config, refresh_managed_field, CONFIG_BUDGET};
    use std::path::Path;

    // Synthetic DATA only; no native resource, constructor, role or file access.
    fn fixture(plugins: &Path, extra: &str) -> Vec<u8> {
        format!(
            r#"{{"credsStore":"orbwin","cliPluginsExtraDirs":{}{extra}}}"#,
            serde_json::to_string(&[plugins]).unwrap()
        )
        .into_bytes()
    }

    #[test]
    fn ordinary_raw_contract_boundaries() {
        let plugins = Path::new("ordinary-raw-test-plugins");
        let body = |extra: &str| fixture(plugins, extra);
        let arrays = |n: usize| {
            body(&format!(
                r#", "opaque":{}0{}"#,
                "[".repeat(n),
                "]".repeat(n)
            ))
        };
        // root/store/plugin-array/plugin-string/opaque-array = five values.
        let nodes = |n: usize| body(&format!(r#", "opaque":[{}0]"#, "0,".repeat(n - 1)));
        let overhead = body(r#", "opaque":"""#).len();
        let sized = |extra: usize| {
            body(&format!(
                r#", "opaque":"{}""#,
                "x".repeat(CONFIG_BUDGET - overhead + extra)
            ))
        };
        let mut invalid = body(r#", "opaque":"invalid-byte""#);
        let at = invalid
            .windows(b"invalid-byte".len())
            .position(|v| v == b"invalid-byte")
            .unwrap();
        invalid.splice(at..at + b"invalid-byte".len(), [0xff]);
        let cases = [
            ("depth64", arrays(63), true),
            ("depth65", arrays(64), false),
            ("count65537", nodes(65532), true),
            ("count65538", nodes(65533), false),
            ("oneMiB", sized(0), true),
            ("overOneMiB", sized(1), false),
            ("validPair", body(r#", "opaque":"\uD800\uDC00""#), true),
            ("literalReplacement", body(",\"opaque\":\"\u{fffd}\""), true),
            ("escapedLiteral", body(r#", "opaque":"\\uD800""#), true),
            ("unpairedValue", body(r#", "opaque":"\uD800""#), false),
            ("unpairedKey", body(r#", "\uDC00":0"#), false),
            ("invalidUTF8", invalid, false),
        ];
        for (name, data, accept) in cases {
            let original = data.clone();
            if name == "oneMiB" {
                assert_eq!(data.len(), CONFIG_BUDGET)
            }
            assert_eq!(parse_config(&data, plugins).is_ok(), accept, "{name}");
            assert_eq!(data, original, "{name}: input mutated");
        }
    }

    #[test]
    fn ordinary_raw_recovery_preservation() {
        let profile = Path::new("ordinary-raw-test-profile");
        let old = profile
            .join(".orbwin-private")
            .join("runtime")
            .join(format!("docker-{}", "a".repeat(64)))
            .join("docker-cli-plugins");
        let next = profile
            .join(".orbwin-private")
            .join("runtime")
            .join(format!("docker-{}", "b".repeat(64)))
            .join("docker-cli-plugins");
        let extras = r#", "auths":{"kept.example":{ "auth":"synthetic-original", "opaque":1e9999 },"target.example":{"auth":"old"}},"opaque":{ "n":1e9999,"n":123456789012345678901234567890 },"opaque":[ 1 , 2 ],"currentContext":"keep-context","plugins":{"x":{"hooks":"keep"}},"HttpHeaders":false"#;
        let base = fixture(&old, extras);
        let original = base.clone();
        let incoming = fixture(
            &old,
            r#", "auths":{"kept.example":{"auth":"untrusted-change"},"target.example":{"auth":"synthetic-new"}},"opaque":false"#,
        );
        let keys = vec!["target.example".to_owned()];
        let merged = merge_registry_auths(&base, &incoming, &old, &keys).unwrap();
        let preserved = [
            r#"{ "auth":"synthetic-original", "opaque":1e9999 }"#,
            r#"{ "n":1e9999,"n":123456789012345678901234567890 }"#,
            r#""opaque":[ 1 , 2 ]"#,
            r#""currentContext":"keep-context""#,
            r#""plugins":{"x":{"hooks":"keep"}}"#,
            r#""HttpHeaders":false"#,
        ];
        let merged_text = std::str::from_utf8(&merged).unwrap();
        for raw in preserved {
            assert!(merged_text.contains(raw), "unrelated ordinary data lost")
        }
        assert!(merged_text.contains(r#""auth":"synthetic-new""#));
        assert_eq!(
            merge_registry_auths(&base, &base, &old, &keys).unwrap(),
            base
        );
        let refreshed = refresh_managed_field(&base, &next, profile)
            .unwrap()
            .unwrap();
        parse_config(&refreshed, &next).unwrap();
        let text = std::str::from_utf8(&refreshed).unwrap();
        for raw in preserved {
            assert!(
                text.contains(raw),
                "refresh changed unrelated ordinary data"
            )
        }
        assert_eq!(base, original);
        let bad = [
            fixture(&old, r#", "opaque":"\uD800""#),
            fixture(
                &old,
                &format!(r#", "opaque":{}0{}"#, "[".repeat(64), "]".repeat(64)),
            ),
            [base.as_slice(), vec![b' '; CONFIG_BUDGET].as_slice()].concat(),
        ];
        for data in bad {
            let original = data.clone();
            assert!(merge_registry_auths(&data, &data, &old, &keys).is_err());
            assert!(refresh_managed_field(&data, &next, profile).is_err());
            assert_eq!(data, original)
        }
    }
}
