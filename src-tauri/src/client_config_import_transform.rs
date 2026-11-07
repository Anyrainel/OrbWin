use super::*;
#[path = "client_config_import_typed.rs"]
mod typed;
#[derive(Clone, serde::Deserialize, serde::Serialize)]
#[serde(rename_all = "PascalCase")]
pub struct ReviewedSelectors {
    pub old_plugins: String,
    pub current_plugins: String,
    pub profile: String,
}
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Refusal {
    Shape,
    Secret,
    Routing,
}
type ImportResult<T> = std::result::Result<T, Refusal>;

// Original bounded raw adapter. No Value/float conversion or unknown key
// collapse: RawValue validates JSON and retains scalar/container lexemes.
// Mirror actual Go pre-increment accounting, including nodes == 65536.
fn bounded(raw: &str, depth: usize, nodes: &mut usize) -> ImportResult<()> {
    if depth > 64 || *nodes > 65536 {
        return Err(Refusal::Shape);
    }
    *nodes += 1;
    match raw.as_bytes().first() {
        Some(b'{') => {
            for (_, v) in raw_fields(raw.as_bytes()).map_err(|_| Refusal::Shape)? {
                bounded(v.get(), depth + 1, nodes)?
            }
        }
        Some(b'[') => {
            for v in serde_json::from_str::<Vec<Box<serde_json::value::RawValue>>>(raw)
                .map_err(|_| Refusal::Shape)?
            {
                bounded(v.get(), depth + 1, nodes)?
            }
        }
        _ => {}
    }
    Ok(())
}
fn windows_path(s: &str) -> bool {
    let s = s.strip_prefix(r"\\?\").unwrap_or(s);
    let b = s.as_bytes();
    if b.len() < 3
        || !b[0].is_ascii_alphabetic()
        || &b[1..3] != b":\\"
        || s.contains('/')
        || s.chars().any(char::is_control)
    {
        return false;
    }
    s[3..].split('\\').all(|v| {
        !v.is_empty()
            && v != "."
            && v != ".."
            && !v.contains([':', '<', '>', '|', '?', '*', '"'])
            && v.trim_end_matches([' ', '.']) == v
    })
}
pub(super) fn legacy_selector(s: &str) -> bool {
    windows_path(s) && s.ends_with(r"\docker-cli-plugins")
}
fn valid_selectors(s: &ReviewedSelectors) -> bool {
    if !windows_path(&s.profile)
        || !windows_path(&s.old_plugins)
        || !s.old_plugins.ends_with(r"\docker-cli-plugins")
    {
        return false;
    }
    let prefix = format!(r"{}\.orbwin-private\runtime\docker-", s.profile);
    let Some(pin) = s
        .current_plugins
        .strip_prefix(&prefix)
        .and_then(|v| v.strip_suffix(r"\docker-cli-plugins"))
    else {
        return false;
    };
    pin.len() == 64
        && pin
            .bytes()
            .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
}
fn registry_address(s: &str) -> bool {
    if s.is_empty()
        || s.len() > 2048
        || s.trim() != s
        || s.contains(['\\', '@'])
        || s.chars().any(|c| c.is_control() || c.is_whitespace())
    {
        return false;
    }
    // Match the reviewed Go net/url classifier's supported HTTP(S)/host forms.
    let mut text = s;
    if let Some((scheme, tail)) = s.split_once("://") {
        if !scheme.eq_ignore_ascii_case("http") && !scheme.eq_ignore_ascii_case("https") {
            return false;
        };
        text = tail
    }
    if let Some((before, fragment)) = text.split_once('#') {
        if !fragment.is_empty() {
            return false;
        };
        text = before
    }
    if let Some((before, query)) = text.split_once('?') {
        if !query.is_empty() {
            return false;
        };
        text = before
    }
    let mut i = 0;
    let b = text.as_bytes();
    while i < b.len() {
        if b[i] == b'%' {
            if i + 2 >= b.len() || !b[i + 1].is_ascii_hexdigit() || !b[i + 2].is_ascii_hexdigit() {
                return false;
            };
            i += 3
        } else {
            i += 1
        }
    }
    let authority = text.split('/').next().unwrap_or("");
    if authority.is_empty() || authority.ends_with(':') {
        return false;
    }
    let (host, port) = if let Some(tail) = authority.strip_prefix('[') {
        let Some((ip, rest)) = tail.split_once(']') else {
            return false;
        };
        if ip.parse::<std::net::Ipv6Addr>().is_err() {
            return false;
        }
        if rest.is_empty() {
            (ip, None)
        } else if let Some(p) = rest.strip_prefix(':') {
            (ip, Some(p))
        } else {
            return false;
        }
    } else {
        match authority.rsplit_once(':') {
            Some((h, p)) => (h, Some(p)),
            None => (authority, None),
        }
    };
    if host.is_empty() {
        return false;
    }
    if host.parse::<std::net::IpAddr>().is_err()
        && !host.split('.').all(|v| {
            !v.is_empty()
                && !v.starts_with('-')
                && !v.ends_with('-')
                && v.bytes().all(|c| c.is_ascii_alphanumeric() || c == b'-')
        })
    {
        return false;
    }
    if let Some(port) = port {
        if port.is_empty()
            || !port.bytes().all(|c| c.is_ascii_digit())
            || !port.parse::<u32>().is_ok_and(|p| (1..=65535).contains(&p))
        {
            return false;
        }
    }
    true
}
fn string(raw: &str) -> ImportResult<String> {
    serde_json::from_str(raw).map_err(|_| Refusal::Shape)
}

// Original import-only lexical policy; structural JSON stays with serde/raw_fields.
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

// Match the Go parser's global schema alias/duplicate pass BEFORE any routing
// refusal. This validates schema positions, not opaque user-data objects.
fn docker_schema_fields(fields: &[(String, Box<serde_json::value::RawValue>)]) -> ImportResult<()> {
    known_fields(fields, ROOT_FIELDS).map_err(|_| Refusal::Shape)?;
    for (name, raw) in fields {
        if !["auths", "credHelpers"].contains(&name.as_str()) || !raw.get().starts_with('{') {
            continue;
        }
        let entries = raw_fields(raw.get().as_bytes()).map_err(|_| Refusal::Shape)?;
        let mut names = std::collections::BTreeSet::new();
        for (key, value) in entries {
            if !names.insert(key) {
                return Err(Refusal::Shape);
            }
            if name == "auths" && value.get().starts_with('{') {
                let auth = raw_fields(value.get().as_bytes()).map_err(|_| Refusal::Shape)?;
                known_fields(
                    &auth,
                    &[
                        "username",
                        "password",
                        "auth",
                        "serveraddress",
                        "identitytoken",
                        "registrytoken",
                    ],
                )
                .map_err(|_| Refusal::Shape)?;
            }
        }
    }
    Ok(())
}

// Pure bytes in/out only. Selector strings are reviewed DATA, never caps.
pub fn transform(source: &[u8], s: &ReviewedSelectors) -> ImportResult<Vec<u8>> {
    if !valid_selectors(s) {
        return Err(Refusal::Routing);
    }
    if source.len() > CONFIG_BUDGET {
        return Err(Refusal::Shape);
    }
    if !valid_unicode_json_strings(source) {
        return Err(Refusal::Shape);
    }
    let raw: Box<serde_json::value::RawValue> =
        serde_json::from_slice(source).map_err(|_| Refusal::Shape)?;
    bounded(raw.get(), 0, &mut 0)?;
    let fields = raw_fields(source).map_err(|_| Refusal::Shape)?;
    docker_schema_fields(&fields)?;
    let get = |key: &str| fields.iter().find(|(k, _)| k == key).map(|(_, v)| v.get());
    let dirs = get("cliPluginsExtraDirs").and_then(|v| serde_json::from_str::<Vec<String>>(v).ok());
    if dirs.as_deref() != Some(&[s.old_plugins.clone()]) {
        return Err(Refusal::Routing);
    }
    if let Some(raw) = get("credsStore") {
        let store = string(raw).map_err(|_| Refusal::Routing)?;
        if store != "" && store != "orbwin" {
            return Err(Refusal::Routing);
        }
    }
    if let Some(raw) = get("credHelpers") {
        let mut helpers = raw_fields(raw.as_bytes()).map_err(|_| Refusal::Shape)?;
        helpers.sort_by(|a, b| a.0.cmp(&b.0));
        let mut keys = std::collections::BTreeSet::new();
        for (key, value) in helpers {
            if !keys.insert(key.clone()) {
                return Err(Refusal::Shape);
            };
            if !registry_address(&key) || string(value.get()).ok().as_deref() != Some("orbwin") {
                return Err(Refusal::Routing);
            }
        }
    }
    if let Some(raw) = get("auths").filter(|v| *v != "null") {
        let mut hosts = raw_fields(raw.as_bytes()).map_err(|_| Refusal::Shape)?;
        hosts.sort_by(|a, b| a.0.cmp(&b.0));
        let mut keys = std::collections::BTreeSet::new();
        for (key, value) in hosts {
            if !keys.insert(key.clone()) {
                return Err(Refusal::Shape);
            };
            if !registry_address(&key) {
                return Err(Refusal::Routing);
            };
            if value.get() == "null" {
                continue;
            }
            let auth = raw_fields(value.get().as_bytes()).map_err(|_| Refusal::Shape)?;
            let names = [
                "username",
                "serveraddress",
                "auth",
                "password",
                "identitytoken",
                "registrytoken",
            ];
            known_fields(&auth, &names).map_err(|_| Refusal::Shape)?;
            for key in names {
                let Some((_, value)) = auth.iter().find(|(name, _)| name == key) else {
                    continue;
                };
                if value.get() == "null" {
                    continue;
                }
                let v = string(value.get())?;
                match key {
                    "username" => {}
                    "serveraddress" => {
                        if !v.is_empty() && !registry_address(&v) {
                            return Err(Refusal::Routing);
                        }
                    }
                    _ => {
                        if !v.is_empty() {
                            return Err(Refusal::Secret);
                        }
                    }
                }
            }
        }
    }
    if let Some(raw) = get("currentContext") {
        let c = string(raw).map_err(|_| Refusal::Routing)?;
        if !["", "default", "rancher-desktop-orbwin"].contains(&c.as_str()) {
            return Err(Refusal::Routing);
        }
    }
    for key in ["plugins", "aliases"] {
        if let Some(raw) = get(key) {
            if !raw_fields(raw.as_bytes())
                .map_err(|_| Refusal::Routing)?
                .is_empty()
            {
                return Err(Refusal::Routing);
            }
        }
    }
    let mut out = Vec::from(&b"{"[..]);
    let mut first = true;
    let mut store = false;
    for (key, raw) in &fields {
        if key == "currentContext" {
            continue;
        }
        if !first {
            out.push(b',')
        };
        first = false;
        out.extend(go_json(key).map_err(|_| Refusal::Shape)?);
        out.push(b':');
        match key.as_str() {
            "cliPluginsExtraDirs" => {
                out.extend(go_json(&[&s.current_plugins]).map_err(|_| Refusal::Shape)?)
            }
            "credsStore" => {
                out.extend(br#""orbwin""#);
                store = true
            }
            _ => out.extend(raw.get().as_bytes()),
        }
    }
    if !store {
        if !first {
            out.push(b',')
        };
        out.extend(br#""credsStore":"orbwin""#)
    };
    out.push(b'}');
    if out.len() > CONFIG_BUDGET {
        return Err(Refusal::Shape);
    }
    let raw: Box<serde_json::value::RawValue> =
        serde_json::from_slice(&out).map_err(|_| Refusal::Shape)?;
    bounded(raw.get(), 0, &mut 0)?;
    if parse_config(&out, Path::new(&s.current_plugins)).is_err() {
        return Err(Refusal::Shape);
    }
    if !typed::typed_target(&out) {
        return Err(Refusal::Shape);
    }
    Ok(out)
}
