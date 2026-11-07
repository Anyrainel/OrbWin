//! Closed read-only candidate observation. This grants no resource ownership or
//! future startup guarantee; the service repeats its native authority checks.
use serde::Deserialize;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "lowercase")]
enum Authority {
    Initialized,
    Fresh,
    Legacy,
    Unavailable,
}

#[derive(Debug, Deserialize)]
#[serde(deny_unknown_fields, rename_all = "camelCase")]
struct Observation {
    schema: u8,
    instance: String,
    authority: Authority,
    startup_compatible: bool,
}

const REFUSAL: &str = "The bundled backend could not verify a compatible private management authority. No service change was sent; the existing engine and data are retained.";

pub(super) fn validate(raw: &str, saved_service: bool) -> Result<(), String> {
    if raw.len() > 1024 {
        return Err(REFUSAL.into());
    }
    let observed: Observation = serde_json::from_str(raw).map_err(|_| REFUSAL.to_string())?;
    if observed.schema != 1 || observed.instance != "orbwin" {
        return Err(REFUSAL.into());
    }
    match (observed.authority, observed.startup_compatible, saved_service) {
        (Authority::Initialized, true, true) | (Authority::Fresh, true, false) => Ok(()),
        (Authority::Legacy, false, _) => Err("This existing engine requires the explicit private-authority migration before applying this backend. No service change was sent; the current engine and all data are retained. Resume setup after the authority migration is completed.".into()),
        _ => Err(REFUSAL.into()),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn observation(authority: &str, compatible: bool) -> String {
        format!(
            r#"{{"schema":1,"instance":"orbwin","authority":"{authority}","startupCompatible":{compatible}}}"#
        )
    }

    #[test]
    fn only_exact_compatible_existing_or_fresh_state_permits_lifecycle() {
        assert!(validate(&observation("initialized", true), true).is_ok());
        assert!(validate(&observation("fresh", true), false).is_ok());
        for (authority, compatible, saved) in [
            ("initialized", true, false),
            ("fresh", true, true),
            ("initialized", false, true),
            ("legacy", false, true),
            ("legacy", true, true),
            ("unavailable", false, false),
            ("unavailable", true, true),
        ] {
            assert!(validate(&observation(authority, compatible), saved).is_err());
        }
    }

    #[test]
    fn closed_protocol_rejects_missing_duplicate_extra_and_trailing_json() {
        let valid = observation("initialized", true);
        for raw in [
            String::new(),
            "{}".into(),
            format!("{valid}{{}}"),
            valid.replace(
                "\"authority\":\"initialized\"",
                "\"authority\":\"initialized\",\"authority\":\"fresh\"",
            ),
            valid.replace("\"schema\":1", "\"schema\":1,\"schema\":1"),
            valid.replace("\"schema\":1", "\"schema\":2"),
            valid.replace("\"schema\":1", "\"schema\":1,\"path\":\"foreign\""),
            valid.replace("orbwin", "orbwin-proof"),
            valid.replace("initialized", "ready"),
            valid.replace("true", "\"true\""),
            " ".repeat(1025),
        ] {
            assert!(validate(&raw, true).is_err(), "accepted {raw:?}");
        }
        assert!(validate(&format!(" \n{valid}\n"), true).is_ok());
    }
}
