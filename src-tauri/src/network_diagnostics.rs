//! Fixed read-only local network check. No renderer arguments, engine start,
//! proxy evaluation, arbitrary destination, raw schema error or support export.
use serde::{Deserialize, Serialize};

const LIMIT: usize = 16 * 1024;
// Keep one slot across component navigation and discarded UI completions. A
// renderer cannot accumulate hidden concurrent local listeners by reopening it.
static RUNNING: tokio::sync::Mutex<()> = tokio::sync::Mutex::const_new(());

#[derive(Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct Snapshot {
    schema: u8,
    loopback: Loopback,
    proxy: Proxy,
    complete: bool,
}
#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Loopback {
    version: u8,
    read_at: String,
    source: String,
    ipv4: Probe,
    ipv6: Probe,
    names: Vec<Name>,
}
#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Name {
    name: String,
    resolution: String,
    families: Vec<String>,
    non_loopback_answers: u8,
    first_family: Option<String>,
    native_dial: Probe,
    ipv4_only_dial: Probe,
}
#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Probe {
    state: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    error_class: Option<String>,
}
#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Proxy {
    version: u8,
    current_user: Configuration,
    win_http_default: Configuration,
}
#[derive(Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct Configuration {
    source: String,
    state: String,
    mode: String,
    auto_detect: bool,
    pac_configured: bool,
    manual_configured: bool,
    bypass_configured: bool,
    effective_proxy: String,
}

fn one_of(value: &str, allowed: &[&str]) -> bool {
    allowed.contains(&value)
}
fn utc_stamp(value: &str) -> bool {
    let bytes = value.as_bytes();
    if !(bytes.len() == 20 || (22..=30).contains(&bytes.len())) || bytes.last() != Some(&b'Z') {
        return false;
    }
    for (index, byte) in bytes.iter().enumerate().take(bytes.len() - 1) {
        let expected = match index {
            4 | 7 => Some(b'-'),
            10 => Some(b'T'),
            13 | 16 => Some(b':'),
            19 => Some(b'.'),
            _ => None,
        };
        if expected.map_or(!byte.is_ascii_digit(), |expected| *byte != expected) {
            return false;
        }
    }
    let number = |range: std::ops::Range<usize>| value[range].parse::<u32>().unwrap_or(0);
    let (year, month, day) = (number(0..4), number(5..7), number(8..10));
    let days = match month {
        2 if year % 4 == 0 && (year % 100 != 0 || year % 400 == 0) => 29,
        2 => 28,
        4 | 6 | 9 | 11 => 30,
        1 | 3 | 5 | 7 | 8 | 10 | 12 => 31,
        _ => 0,
    };
    year >= 1970
        && day != 0
        && day <= days
        && number(11..13) <= 23
        && number(14..16) <= 59
        && number(17..19) <= 59
}
fn valid_probe(probe: &Probe) -> bool {
    match probe.state.as_str() {
        "not-run" | "verified-owned-listener" => probe.error_class.is_none(),
        "failed" | "listen-unavailable" => probe.error_class.as_deref().is_some_and(|value| {
            one_of(
                value,
                &[
                    "canceled",
                    "timeout",
                    "not-found",
                    "unavailable",
                    "ownership-mismatch",
                ],
            )
        }),
        _ => false,
    }
}
fn valid_configuration(config: &Configuration, source: &str) -> bool {
    if config.source != source || config.effective_proxy != "not-evaluated" {
        return false;
    }
    if config.state == "unavailable" {
        return config.mode == "unknown"
            && !config.auto_detect
            && !config.pac_configured
            && !config.manual_configured
            && !config.bypass_configured;
    }
    let automatic = config.auto_detect || config.pac_configured;
    let expected = match (automatic, config.manual_configured) {
        (true, true) => "mixed-configured",
        (true, false) => "automatic-configured",
        (false, true) => "manual-configured",
        (false, false) => "direct-configured",
    };
    config.state == "observed" && config.mode == expected
}

fn parse(raw: &str) -> Result<Snapshot, String> {
    let invalid = || {
        "Network check returned an unsupported or malformed result. Retry after preparing the current backend tools.".to_string()
    };
    if raw.len() > LIMIT {
        return Err(invalid());
    }
    let result: Snapshot = serde_json::from_str(raw).map_err(|_| invalid())?;
    if result.schema != 1
        || result.loopback.version != 1
        || result.proxy.version != 1
        || result.loopback.source != "os-resolver-and-nonce-owned-loopback"
        || result.loopback.read_at.len() > 64
        || !utc_stamp(&result.loopback.read_at)
        || !valid_probe(&result.loopback.ipv4)
        || !valid_probe(&result.loopback.ipv6)
        || result.loopback.names.len() > 2
        || !valid_configuration(
            &result.proxy.current_user,
            "winhttp-current-user-configuration",
        )
        || !valid_configuration(
            &result.proxy.win_http_default,
            "winhttp-default-registry-configuration",
        )
        || result.complete
            && (result.proxy.current_user.state != "observed"
                || result.proxy.win_http_default.state != "observed")
    {
        return Err(invalid());
    }
    for (index, name) in result.loopback.names.iter().enumerate() {
        let expected = ["localhost", "network-check.orbwin.localhost"][index];
        if name.name != expected
            || name.families.len() > 16
            || name.non_loopback_answers > 16
            || !one_of(
                &name.resolution,
                &[
                    "observed",
                    "canceled",
                    "timeout",
                    "not-found",
                    "unavailable",
                    "invalid-answer-count",
                    "malformed-answer",
                    "non-loopback-answer",
                ],
            )
            || name
                .families
                .iter()
                .any(|value| !one_of(value, &["ipv4", "ipv6"]))
            || !valid_probe(&name.native_dial)
            || !valid_probe(&name.ipv4_only_dial)
        {
            return Err(invalid());
        }
        if name.resolution == "observed" || name.resolution == "non-loopback-answer" {
            if name.families.is_empty()
                || name.first_family.as_ref() != name.families.first()
                || usize::from(name.non_loopback_answers) > name.families.len()
                || (name.resolution == "observed") != (name.non_loopback_answers == 0)
            {
                return Err(invalid());
            }
        } else if name.first_family.is_some() {
            return Err(invalid());
        }
        if name.resolution != "observed"
            && (name.native_dial.state != "not-run" || name.ipv4_only_dial.state != "not-run")
        {
            return Err(invalid());
        }
    }
    if result.complete && result.loopback.names.len() != 2 {
        return Err(invalid());
    }
    Ok(result)
}

#[tauri::command]
pub(crate) async fn network_diagnostics() -> Result<Snapshot, String> {
    let _slot = RUNNING.try_lock().map_err(|_| {
        "A local network check is still finishing. Wait a moment and retry.".to_string()
    })?;
    let binary = crate::engine::session_binary().await.map_err(|_| "The verified network diagnostic tool is unavailable. Prepare the backend tools and retry.".to_string())?;
    let raw = crate::process::read_cached(&binary, &["network-diagnostics"]).await
        .map_err(|_| "Network checks could not finish within their bounded run. Retry; no network settings were changed.".to_string())?;
    parse(&raw)
}

#[cfg(test)]
mod tests {
    use super::*;
    fn fixture() -> serde_json::Value {
        let proxy = |source| serde_json::json!({"source":source,"state":"observed","mode":"direct-configured","autoDetect":false,"pacConfigured":false,"manualConfigured":false,"bypassConfigured":false,"effectiveProxy":"not-evaluated"});
        let probe = serde_json::json!({"state":"verified-owned-listener"});
        let name = |name| serde_json::json!({"name":name,"resolution":"observed","families":["ipv6","ipv4"],"nonLoopbackAnswers":0,"firstFamily":"ipv6","nativeDial":probe,"ipv4OnlyDial":probe});
        serde_json::json!({"schema":1,"loopback":{"version":1,"readAt":"2026-10-25T22:15:58Z","source":"os-resolver-and-nonce-owned-loopback","ipv4":probe,"ipv6":probe,"names":[name("localhost"),name("network-check.orbwin.localhost")]},"proxy":{"version":1,"currentUser":proxy("winhttp-current-user-configuration"),"winHttpDefault":proxy("winhttp-default-registry-configuration")},"complete":true})
    }
    #[test]
    fn accepts_known_result_and_partial_unknown_without_effective_claim() {
        let mut value = fixture();
        assert!(parse(&value.to_string()).is_ok());
        value["complete"] = false.into();
        value["proxy"]["currentUser"]["state"] = "unavailable".into();
        value["proxy"]["currentUser"]["mode"] = "unknown".into();
        assert!(parse(&value.to_string()).is_ok());
    }
    #[test]
    fn utc_timestamp_bounds_refuse_malformed_calendar_and_private_text() {
        for valid in ["2026-10-25T22:15:58Z", "2024-02-29T00:00:00.123456789Z"] {
            assert!(utc_stamp(valid));
        }
        for invalid in [
            "2026-02-29T00:00:00Z",
            "2026-13-01T00:00:00Z",
            "2026-01-00T00:00:00Z",
            "2026-01-01T24:00:00Z",
            "2026-01-01T00:00:00.Z",
            "2026-01-01T00:00:00+00:00",
            "私密-network",
            "2026-01-01T00:00:00.1234567890Z",
        ] {
            assert!(!utc_stamp(invalid), "{invalid}");
        }
    }
    #[test]
    fn rejects_unbounded_unknown_private_and_inconsistent_output() {
        for pointer in ["/schema", "/loopback/version", "/proxy/version"] {
            let mut value = fixture();
            *value.pointer_mut(pointer).unwrap() = 2.into();
            assert!(parse(&value.to_string()).is_err());
        }
        for (pointer, bad) in [
            ("/loopback/source", "private-network"),
            ("/loopback/readAt", "bad"),
            ("/loopback/names/0/name", "private.corp.example"),
            ("/loopback/names/0/firstFamily", "ipv4"),
            ("/proxy/currentUser/effectiveProxy", "effective"),
            ("/proxy/currentUser/mode", "automatic-configured"),
            ("/loopback/ipv4/state", "success"),
        ] {
            let mut value = fixture();
            *value.pointer_mut(pointer).unwrap() = bad.into();
            assert!(parse(&value.to_string()).is_err(), "{pointer}");
        }
        let mut value = fixture();
        value["proxy"]["currentUser"]["url"] = "http://secret".into();
        assert!(parse(&value.to_string()).is_err());
        let mut value = fixture();
        value["loopback"]["names"][0]["families"] = vec!["ipv6"; 17].into();
        assert!(parse(&value.to_string()).is_err());
        let mut value = fixture();
        value["loopback"]["names"][1]["name"] = "localhost".into();
        assert!(parse(&value.to_string()).is_err());
        let mut value = fixture();
        value["loopback"]["names"][0]["resolution"] = "non-loopback-answer".into();
        assert!(parse(&value.to_string()).is_err());
        assert!(parse(&" ".repeat(LIMIT + 1)).is_err());
        let raw = fixture().to_string();
        assert!(parse(&raw.replace("\"schema\":1", "\"schema\":1,\"schema\":1")).is_err());
        assert!(parse(&(raw + "{}")).is_err());
    }
}
