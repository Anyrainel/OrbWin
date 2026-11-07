//! A bounded desired-state patch for rdd's sole App owner. The desktop neither
//! restarts dockerd nor writes status, and never retries an uncertain mutation.
use serde::{Deserialize, Serialize};

#[derive(Clone, Deserialize, Serialize)]
#[serde(rename_all = "lowercase")]
pub(crate) enum Mode {
    None,
    Url,
    Auto,
}

#[derive(Clone, Deserialize, Serialize)]
#[serde(deny_unknown_fields, rename_all = "camelCase")]
pub(crate) struct Config {
    pub version: u8,
    pub mode: Mode,
    pub url: String,
    pub no_proxy: Vec<String>,
}

#[derive(Clone, Deserialize)]
#[serde(deny_unknown_fields, rename_all = "camelCase")]
pub(crate) struct Change {
    pub uid: String,
    pub resource_version: String,
    pub generation: u64,
    pub config: Config,
}
impl std::fmt::Debug for Change {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str("EngineProxyReview(private values omitted)")
    }
}

const BAD: &str =
    "The proxy review is invalid or no longer current. Read the configuration and review it again.";
fn ascii(value: &str, max: usize) -> bool {
    !value.is_empty() && value.len() <= max && value.bytes().all(|b| b.is_ascii_graphic())
}
fn port(value: &str) -> bool {
    value
        .parse::<u16>()
        .is_ok_and(|n| n > 0 && n.to_string() == value)
}
fn host(value: &str) -> bool {
    if let Ok(ip) = value.parse::<std::net::IpAddr>() {
        return ip.to_string() == value
            && !ip.is_unspecified()
            && !ip.is_multicast()
            && !matches!(ip, std::net::IpAddr::V6(v) if v.to_ipv4_mapped().is_some());
    }
    ascii(value, 253)
        && value == value.to_ascii_lowercase()
        && value.split('.').all(|part| {
            !part.is_empty()
                && part.len() <= 63
                && !part.starts_with('-')
                && !part.ends_with('-')
                && part
                    .bytes()
                    .all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || b == b'-')
        })
}
fn host_port(value: &str) -> bool {
    if let Some(rest) = value.strip_prefix('[') {
        return rest.split_once(']').is_some_and(|(ip, rest)| {
            ip.contains(':')
                && host(ip)
                && (rest.is_empty() || rest.strip_prefix(':').is_some_and(port))
        });
    }
    match value.rsplit_once(':') {
        Some((h, p)) => !h.contains(':') && host(h) && port(p),
        None => host(value),
    }
}
fn bypass(value: &str) -> bool {
    if !ascii(value, 512) {
        return false;
    }
    if value == "*" {
        return true;
    }
    if let Some((address, bits)) = value.split_once('/') {
        let Ok(ip) = address.parse::<std::net::IpAddr>() else {
            return false;
        };
        let Ok(bits) = bits.parse::<u32>() else {
            return false;
        };
        if ip.to_string() != address || format!("{bits}") != value.rsplit('/').next().unwrap_or("")
        {
            return false;
        }
        return match ip {
            std::net::IpAddr::V4(ip) => {
                bits <= 32
                    && u32::from(ip)
                        & if bits == 0 {
                            0
                        } else {
                            u32::MAX << (32 - bits)
                        }
                        == u32::from(ip)
            }
            std::net::IpAddr::V6(ip) => {
                bits <= 128
                    && ip.to_ipv4_mapped().is_none()
                    && u128::from(ip)
                        & if bits == 0 {
                            0
                        } else {
                            u128::MAX << (128 - bits)
                        }
                        == u128::from(ip)
            }
        };
    }
    if value.parse::<std::net::IpAddr>().is_ok() {
        return host(value);
    }
    host_port(
        value
            .strip_prefix("*.")
            .or_else(|| value.strip_prefix('.'))
            .unwrap_or(value),
    )
}
impl Config {
    fn valid(&self) -> bool {
        if self.version != 1 || self.no_proxy.len() > 128 {
            return false;
        }
        if matches!(self.mode, Mode::Url) {
            let Some(authority) = self
                .url
                .strip_prefix("http://")
                .or_else(|| self.url.strip_prefix("https://"))
            else {
                return false;
            };
            let authority = authority.strip_suffix('/').unwrap_or(authority);
            if !ascii(&self.url, 2048) || !host_port(authority) {
                return false;
            }
        } else if !self.url.is_empty() {
            return false;
        }
        let mut seen = std::collections::HashSet::new();
        self.no_proxy
            .iter()
            .all(|entry| bypass(entry) && seen.insert(entry))
            && serde_json::to_vec(self).is_ok_and(|bytes| bytes.len() <= 16 * 1024)
    }
}

pub(crate) fn capability() -> Result<bool, String> {
    let lock: serde_json::Value = serde_json::from_str(include_str!("../../upstream.lock.json"))
        .map_err(|_| "Backend capability record is unavailable.")?;
    let supported = lock["patches"].as_array().is_some_and(|patches| {
        patches
            .iter()
            .any(|p| p.as_str() == Some("patches/rdd/0017-private-engine-proxy.patch"))
    });
    if supported {
        crate::engine::target()?;
    }
    Ok(supported)
}

pub(crate) fn patch(change: &Change) -> Result<Vec<u8>, String> {
    if !ascii(&change.uid, 128)
        || !ascii(&change.resource_version, 128)
        || change.generation == 0
        || change.generation > 9_007_199_254_740_991
        || !change.config.valid()
    {
        return Err(BAD.into());
    }
    serde_json::to_vec(&serde_json::json!([
        {"op":"test","path":"/metadata/uid","value":change.uid},
        {"op":"test","path":"/metadata/resourceVersion","value":change.resource_version},
        {"op":"test","path":"/metadata/generation","value":change.generation},
        {"op":"add","path":"/spec/networkProxy","value":change.config}
    ]))
    .map_err(|_| BAD.into())
}

#[cfg(test)]
mod tests {
    use super::*;
    fn change() -> Change {
        serde_json::from_value(serde_json::json!({"uid":"app-owned","resourceVersion":"123","generation":7,"config":{"version":1,"mode":"url","url":"https://proxy.example:8443","noProxy":[".example.test","192.168.1.0/24"]}})).unwrap()
    }
    #[test]
    fn desired_patch_tests_identity_and_version_before_one_fixed_spec_write() {
        let parsed: serde_json::Value = serde_json::from_slice(&patch(&change()).unwrap()).unwrap();
        let ops = parsed.as_array().unwrap();
        assert_eq!(ops.len(), 4);
        for (i, path) in [
            "/metadata/uid",
            "/metadata/resourceVersion",
            "/metadata/generation",
        ]
        .iter()
        .enumerate()
        {
            assert_eq!(ops[i]["op"], "test");
            assert_eq!(ops[i]["path"], *path);
        }
        assert_eq!(ops[3]["path"], "/spec/networkProxy");
        assert_eq!(ops[3]["op"], "add");
        assert_eq!(ops[3]["value"]["url"], "https://proxy.example:8443");
    }
    #[test]
    fn untrusted_urls_bypass_and_review_identity_cannot_extend_the_write() {
        for url in [
            "http://user:secret@host",
            "http://@host",
            "http://proxy/path",
            "http://proxy?x",
            "http://proxy#fragment",
            "http://proxy:080",
            "http://proxy\\foreign",
            "http://[::ffff:127.0.0.1]:8080",
            "http://0.0.0.0",
            "http://Proxy.EXAMPLE",
        ] {
            let mut c = change();
            c.config.url = url.into();
            assert!(patch(&c).is_err());
        }
        for entry in [
            "example test",
            "192.168.1.1/24",
            "10.0.0.0/08",
            "example:0",
            "../foreign",
        ] {
            let mut c = change();
            c.config.no_proxy = vec![entry.into()];
            assert!(patch(&c).is_err());
        }
        let mut c = change();
        c.generation = 0;
        assert!(patch(&c).is_err());
        let mut c = change();
        c.config.no_proxy = vec!["example".into(), "example".into()];
        assert!(patch(&c).is_err());
        let mut c = change();
        c.config.mode = Mode::Auto;
        assert!(patch(&c).is_err());
        c.config.url.clear();
        assert!(patch(&c).is_ok());
        assert!(!format!("{:?}", change()).contains("proxy.example"));
    }
}
