//! Fixed native-private Docker configuration owner, separate from SDK state.
//! Source only: product integration and native qualification are separate gates.
//! Native object/SD helpers are copied from runtime_cache.rs; immutable code
//! policy stays unchanged. Only data-file admission accepts exact inherited FA.
use serde_json::Value;
#[path = "client_config_import.rs"]
mod legacy_import;
#[path = "client_config_successor.rs"]
mod successor;
#[cfg(debug_assertions)]
#[path = "client_config_qualification.rs"]
pub(crate) mod successor_qualification;
use std::ffi::OsString;
use std::path::{Path, PathBuf};
#[path = "client_config_raw.rs"]
mod raw_contract;
const CONFIG_BUDGET: usize = 1 << 20;
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum ErrorKind {
    Unavailable,
    RequiresLegacyReview,
    BootstrapChoiceRequired,
    LegacyImportSecrets,
    LegacyImportShape,
    LegacyImportRouting,
    RecoveryRequired,
    NoInterruptedWrite,
    RoutingRequiresReview,
    Busy,
}
#[derive(Debug)]
pub(crate) struct ClientConfigError {
    pub(crate) kind: ErrorKind,
    // Constant stages and numeric native status only; never SID/path/config data.
    diagnostic: Option<NativeDiagnostic>,
}
#[derive(Clone, Copy, Debug)]
struct NativeDiagnostic {
    stage: &'static str,
    system_error: Option<u32>,
    // File SD control/count/flags OR/AND only; no SID/path/SD/data bytes.
    security: Option<[u32; 4]>,
}
impl std::fmt::Display for ClientConfigError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self.kind{
            ErrorKind::BootstrapChoiceRequired=>return f.write_str("Private client initialization needs an explicit Empty or Import choice in OrbWin desktop. Existing settings were retained."),
            ErrorKind::LegacyImportSecrets=>return f.write_str("Historical OrbWin settings contain nonempty inline credential fields. Automatic import does not remove or convert those credentials. Source settings were retained; no credential helper was run."),
            ErrorKind::LegacyImportShape=>return f.write_str("Historical OrbWin settings do not match the supported import data shape or bounds. Source settings were retained."),
            ErrorKind::LegacyImportRouting=>return f.write_str("Historical OrbWin settings contain unsupported context, plugin, alias, or credential routes. Source settings were retained; no helper or plugin was run."),
            _=>{}
        }
        if self.kind == ErrorKind::NoInterruptedWrite {
            return f.write_str("No interrupted private settings write found.");
        }
        write!(
            f,
            "Private Docker client configuration {:?}; existing bytes retained",
            self.kind
        )?;
        if let Some(diagnostic) = self.diagnostic {
            write!(f, "; stage={}", diagnostic.stage)?;
            if let Some(code) = diagnostic.system_error {
                write!(f, "; SystemError=0x{code:08x}")?;
            }
            if let Some(m) = diagnostic.security {
                write!(f, "; fileControl=0x{:04x}; fileACECount={}; fileFlagsOR=0x{:02x}; fileFlagsAND=0x{:02x}", m[0],m[1],m[2],m[3])?;
            }
        }
        Ok(())
    }
}
impl std::error::Error for ClientConfigError {}
fn err(kind: ErrorKind) -> ClientConfigError {
    ClientConfigError {
        kind,
        diagnostic: None,
    }
}
// TOKEN_GROUPS starts with a DWORD count; nonzero counts are rejected without
// dereferencing untrusted variable SID pointers. The backing store is aligned.
fn no_restricting_sids(groups: &[u64], returned: u32) -> bool {
    returned >= 4
        && returned as usize <= std::mem::size_of_val(groups)
        && groups
            .first()
            .is_some_and(|value| value.to_ne_bytes()[..4] == [0; 4])
}
// Intent clearing is deliberately outside this barrier. Each failed step stops
// settlement; no caller may publish a healthy writer state before all five pass.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum WriterCommitStep {
    AdmitWritable,
    Flush,
    RecheckFlushed,
    CloseWritable,
    AdmitReadOnly,
}
fn writer_durability_sequence(
    mut step: impl FnMut(WriterCommitStep) -> Result<(), ClientConfigError>,
) -> Result<(), ClientConfigError> {
    for action in [
        WriterCommitStep::AdmitWritable,
        WriterCommitStep::Flush,
        WriterCommitStep::RecheckFlushed,
        WriterCommitStep::CloseWritable,
        WriterCommitStep::AdmitReadOnly,
    ] {
        step(action)?;
    }
    Ok(())
}
fn routing(value: &Value, plugins: &Path) -> Result<(), ClientConfigError> {
    let object = value
        .as_object()
        .ok_or_else(|| err(ErrorKind::RoutingRequiresReview))?;
    if object.get("cliPluginsExtraDirs") != Some(&serde_json::json!([plugins]))
        || object.get("credsStore") != Some(&Value::String("orbwin".into()))
    {
        return Err(err(ErrorKind::RoutingRequiresReview));
    }
    if let Some(helpers) = object.get("credHelpers") {
        if !helpers
            .as_object()
            .is_some_and(|m| m.values().all(|v| v == "orbwin"))
        {
            return Err(err(ErrorKind::RoutingRequiresReview));
        }
    }
    Ok(())
}

// Docker uses encoding/json case-insensitive tagged-field matching. Check raw
// keys before Value can collapse duplicates, including escaped aliases.
const ROOT_FIELDS: &[&str] = &[
    "auths",
    "HttpHeaders",
    "psFormat",
    "imagesFormat",
    "networksFormat",
    "pluginsFormat",
    "volumesFormat",
    "statsFormat",
    "detachKeys",
    "credsStore",
    "credHelpers",
    "serviceInspectFormat",
    "servicesFormat",
    "tasksFormat",
    "secretFormat",
    "configFormat",
    "nodesFormat",
    "pruneFilters",
    "proxies",
    "currentContext",
    "cliPluginsExtraDirs",
    "plugins",
    "aliases",
    "features",
];
fn checked_json(bytes: &[u8]) -> Result<Value, ClientConfigError> {
    use serde::de::{DeserializeSeed, Error as _, MapAccess, SeqAccess, Visitor};
    struct Seed(bool);
    struct Checked(bool);
    impl<'de> DeserializeSeed<'de> for Seed {
        type Value = Value;
        fn deserialize<D: serde::Deserializer<'de>>(self, d: D) -> Result<Value, D::Error> {
            d.deserialize_any(Checked(self.0))
        }
    }
    impl<'de> Visitor<'de> for Checked {
        type Value = Value;
        fn expecting(&self, f: &mut std::fmt::Formatter) -> std::fmt::Result {
            f.write_str("unique JSON data")
        }
        fn visit_bool<E: serde::de::Error>(self, v: bool) -> Result<Value, E> {
            Ok(Value::Bool(v))
        }
        fn visit_unit<E: serde::de::Error>(self) -> Result<Value, E> {
            Ok(Value::Null)
        }
        fn visit_i64<E: serde::de::Error>(self, v: i64) -> Result<Value, E> {
            Ok(Value::from(v))
        }
        fn visit_u64<E: serde::de::Error>(self, v: u64) -> Result<Value, E> {
            Ok(Value::from(v))
        }
        fn visit_f64<E: serde::de::Error>(self, v: f64) -> Result<Value, E> {
            serde_json::Number::from_f64(v)
                .map(Value::Number)
                .ok_or_else(|| E::custom("number refused"))
        }
        fn visit_str<E: serde::de::Error>(self, v: &str) -> Result<Value, E> {
            Ok(Value::String(v.into()))
        }
        fn visit_string<E: serde::de::Error>(self, v: String) -> Result<Value, E> {
            Ok(Value::String(v))
        }
        fn visit_seq<A: SeqAccess<'de>>(self, mut a: A) -> Result<Value, A::Error> {
            let mut out = Vec::new();
            while let Some(v) = a.next_element_seed(Seed(false))? {
                out.push(v)
            }
            Ok(Value::Array(out))
        }
        fn visit_map<A: MapAccess<'de>>(self, mut a: A) -> Result<Value, A::Error> {
            let mut out = serde_json::Map::new();
            while let Some(key) = a.next_key::<String>()? {
                if out.contains_key(&key) {
                    return Err(A::Error::custom("duplicate configuration key"));
                }
                if self.0
                    && ROOT_FIELDS
                        .iter()
                        .any(|canonical| canonical.eq_ignore_ascii_case(&key) && *canonical != key)
                {
                    return Err(A::Error::custom("case-fold configuration alias"));
                }
                let value = a.next_value_seed(Seed(false))?;
                out.insert(key, value);
            }
            Ok(Value::Object(out))
        }
    }
    let mut d = serde_json::Deserializer::from_slice(bytes);
    let value = Seed(true)
        .deserialize(&mut d)
        .map_err(|_| err(ErrorKind::RoutingRequiresReview))?;
    d.end().map_err(|_| err(ErrorKind::RoutingRequiresReview))?;
    Ok(value)
}
fn raw_fields(
    bytes: &[u8],
) -> Result<Vec<(String, Box<serde_json::value::RawValue>)>, ClientConfigError> {
    use serde::de::{MapAccess, Visitor};
    struct Fields;
    impl<'de> Visitor<'de> for Fields {
        type Value = Vec<(String, Box<serde_json::value::RawValue>)>;
        fn expecting(&self, f: &mut std::fmt::Formatter) -> std::fmt::Result {
            f.write_str("configuration object")
        }
        fn visit_map<A: MapAccess<'de>>(self, mut access: A) -> Result<Self::Value, A::Error> {
            let mut fields = Vec::new();
            while let Some(key) = access.next_key::<String>()? {
                let value = access.next_value::<Box<serde_json::value::RawValue>>()?;
                fields.push((key, value))
            }
            Ok(fields)
        }
    }
    let mut d = serde_json::Deserializer::from_slice(bytes);
    let fields = serde::Deserializer::deserialize_map(&mut d, Fields)
        .map_err(|_| err(ErrorKind::RoutingRequiresReview))?;
    d.end().map_err(|_| err(ErrorKind::RoutingRequiresReview))?;
    Ok(fields)
}
// All Docker typed field names here are ASCII. Go encoding/json uses Unicode
// SimpleFold: the two non-ASCII runes folding into ASCII are long-s and Kelvin.
// Keep this schema adapter in sync with the selected Go/Docker JSON contract.
fn docker_field_eq(canonical: &str, key: &str) -> bool {
    let mut actual = key.chars();
    for expected in canonical.bytes() {
        let Some(value) = actual.next() else {
            return false;
        };
        let folded = match value {
            '\u{017f}' => 's',
            '\u{212a}' => 'k',
            value if value.is_ascii() => value.to_ascii_lowercase(),
            _ => return false,
        };
        if folded as u8 != expected.to_ascii_lowercase() {
            return false;
        }
    }
    actual.next().is_none()
}
fn known_fields(
    fields: &[(String, Box<serde_json::value::RawValue>)],
    known: &[&str],
) -> Result<(), ClientConfigError> {
    let mut seen = std::collections::BTreeSet::new();
    for (key, _) in fields {
        if let Some(canonical) = known.iter().find(|field| docker_field_eq(field, key)) {
            if *canonical != key || !seen.insert(*canonical) {
                return Err(err(ErrorKind::RoutingRequiresReview));
            }
        }
    }
    Ok(())
}
fn parse_config(bytes: &[u8], plugins: &Path) -> Result<(), ClientConfigError> {
    if bytes.len() > CONFIG_BUDGET {
        return Err(err(ErrorKind::Unavailable));
    }
    raw_contract::validate(bytes)?;
    let fields = raw_fields(bytes)?;
    known_fields(&fields, ROOT_FIELDS)?;
    let get = |key: &str| {
        fields
            .iter()
            .find(|(name, _)| name == key)
            .map(|(_, value)| value.get())
    };
    if get("credsStore")
        .and_then(|raw| serde_json::from_str::<String>(raw).ok())
        .as_deref()
        != Some("orbwin")
    {
        return Err(err(ErrorKind::RoutingRequiresReview));
    }
    let dirs = get("cliPluginsExtraDirs")
        .and_then(|raw| serde_json::from_str::<Vec<String>>(raw).ok())
        .ok_or_else(|| err(ErrorKind::RoutingRequiresReview))?;
    if dirs
        != [plugins
            .to_str()
            .ok_or_else(|| err(ErrorKind::Unavailable))?]
    {
        return Err(err(ErrorKind::RoutingRequiresReview));
    }
    if let Some(raw) = get("credHelpers") {
        let helpers = raw_fields(raw.as_bytes())?;
        let mut names = std::collections::BTreeSet::new();
        for (name, value) in helpers {
            if !names.insert(name)
                || serde_json::from_str::<String>(value.get()).ok().as_deref() != Some("orbwin")
            {
                return Err(err(ErrorKind::RoutingRequiresReview));
            }
        }
    }
    if let Some(raw) = get("auths") {
        if raw != "null" {
            let mut hosts = std::collections::BTreeSet::new();
            for (host, raw) in raw_fields(raw.as_bytes())? {
                if !hosts.insert(host) {
                    return Err(err(ErrorKind::RoutingRequiresReview));
                };
                if raw.get() == "null" {
                    continue;
                };
                let auth = raw_fields(raw.get().as_bytes())?;
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
                )?;
                for (name, value) in auth {
                    if [
                        "username",
                        "password",
                        "auth",
                        "serveraddress",
                        "identitytoken",
                        "registrytoken",
                    ]
                    .contains(&name.as_str())
                        && value.get() != "null"
                        && serde_json::from_str::<String>(value.get()).is_err()
                    {
                        return Err(err(ErrorKind::RoutingRequiresReview));
                    }
                }
            }
        }
    }
    Ok(())
}
// Registry writer authority: only command-bound auth entries may change.
// Retain every unrelated baseline RawValue verbatim, including unknown numbers
// and duplicate lexemes inside data-only objects. This is not a restore schema.
fn merge_registry_auths(
    base: &[u8],
    incoming: &[u8],
    plugins: &Path,
    keys: &[String],
) -> Result<Vec<u8>, ClientConfigError> {
    parse_config(base, plugins)?;
    parse_config(incoming, plugins)?;
    if base == incoming {
        return Ok(base.to_vec());
    }
    if keys.is_empty() {
        return Err(err(ErrorKind::RecoveryRequired));
    }
    let original = raw_fields(base)?;
    let changed = raw_fields(incoming)?;
    let incoming_auth = changed
        .iter()
        .find(|(key, _)| key == "auths")
        .map(|(_, v)| v.get());
    if incoming_auth.is_none() {
        return Err(err(ErrorKind::RecoveryRequired));
    }
    let auth = merge_auth_entries(
        original
            .iter()
            .find(|(key, _)| key == "auths")
            .map(|(_, v)| v.get()),
        incoming_auth.unwrap(),
        keys,
    )?;
    let mut out = vec![b'{'];
    let mut first = true;
    let mut emitted = false;
    for (key, value) in original {
        if !first {
            out.push(b',');
        }
        first = false;
        out.extend(serde_json::to_vec(&key).map_err(|_| err(ErrorKind::RecoveryRequired))?);
        out.push(b':');
        if key == "auths" {
            out.extend(&auth);
            emitted = true;
        } else {
            out.extend(value.get().as_bytes());
        }
    }
    if !emitted {
        if !first {
            out.push(b',');
        }
        out.extend(b"\"auths\":");
        out.extend(&auth);
    }
    out.push(b'}');
    parse_config(&out, plugins)?;
    Ok(out)
}
fn merge_auth_entries(
    base: Option<&str>,
    incoming: &str,
    keys: &[String],
) -> Result<Vec<u8>, ClientConfigError> {
    let mut original = match base {
        None | Some("null") => Vec::new(),
        Some(raw) => raw_fields(raw.as_bytes())?,
    };
    let changed = if incoming == "null" {
        Vec::new()
    } else {
        raw_fields(incoming.as_bytes())?
    };
    // Only command-bound entries may be replaced/deleted/added. Every other
    // registry RawValue, including unknown nested fields, stays verbatim.
    original.retain(|(key, _)| !keys.contains(key));
    original.extend(changed.into_iter().filter(|(key, _)| keys.contains(key)));
    let mut out = vec![b'{'];
    for (i, (key, value)) in original.iter().enumerate() {
        if i != 0 {
            out.push(b',');
        }
        out.extend(serde_json::to_vec(key).map_err(|_| err(ErrorKind::RecoveryRequired))?);
        out.push(b':');
        out.extend(value.get().as_bytes());
    }
    out.push(b'}');
    Ok(out)
}
// Match Go encoding/json's compact UTF-8 array encoding, including its HTML
// and line-separator escapes, so schema-2 key digests are cross-owner metadata.
fn writer_keys_json(keys: &[String]) -> Result<Vec<u8>, ClientConfigError> {
    go_json(keys)
}
fn go_json<T: serde::Serialize + ?Sized>(value: &T) -> Result<Vec<u8>, ClientConfigError> {
    let json = serde_json::to_string(value).map_err(|_| err(ErrorKind::RecoveryRequired))?;
    Ok(json
        .replace('&', "\\u0026")
        .replace('<', "\\u003c")
        .replace('>', "\\u003e")
        .replace('\u{2028}', "\\u2028")
        .replace('\u{2029}', "\\u2029")
        .into_bytes())
}
fn registry_keys(keys: &[String]) -> Result<Vec<String>, ClientConfigError> {
    if keys.len() > 8
        || keys
            .iter()
            .any(|k| k.is_empty() || k.len() > 2048 || k.chars().any(char::is_control))
    {
        return Err(err(ErrorKind::RoutingRequiresReview));
    }
    let mut keys = keys.to_vec();
    keys.sort();
    keys.dedup();
    Ok(keys)
}
fn validate_registry_login(
    config: &Path,
    keys: &[String],
    args: &[OsString],
) -> Result<(), ClientConfigError> {
    let args: Option<Vec<&str>> = args.iter().map(|v| v.to_str()).collect();
    let args = args.ok_or_else(|| err(ErrorKind::RoutingRequiresReview))?;
    if args.len() != 9
        || args[0] != "--config"
        || Some(args[1]) != config.to_str()
        || args[2] != "--host"
        || args[3] != "npipe:////./pipe/docker-rancher-desktop-orbwin"
        || args[4] != "login"
        || args[5] != "--username"
        || args[7] != "--password-stdin"
        || args[6].is_empty()
        || args[6].len() > 512
        || args[6].chars().any(char::is_control)
    {
        return Err(err(ErrorKind::RoutingRequiresReview));
    }
    let server = args[8];
    // This API is for the existing UI's validated lowercase host[:port] shape;
    // the general CLI parser has its own complete selected-client key adapter.
    let (host, port) = server
        .split_once(':')
        .map_or((server, None), |(h, p)| (h, Some(p)));
    if server.is_empty()
        || server.len() > 253
        || !server.is_ascii()
        || server != server.to_ascii_lowercase()
        || host.split('.').any(|label| {
            label.is_empty()
                || label.len() > 63
                || !label.as_bytes()[0].is_ascii_alphanumeric()
                || !label.as_bytes()[label.len() - 1].is_ascii_alphanumeric()
                || !label
                    .bytes()
                    .all(|b| b.is_ascii_alphanumeric() || b == b'-')
        })
        || port.is_some_and(|v| !v.parse::<u16>().is_ok_and(|p| p > 0))
    {
        return Err(err(ErrorKind::RoutingRequiresReview));
    }
    let key = if server == "docker.io" {
        "https://index.docker.io/v1/"
    } else {
        server
    };
    if keys != [key] {
        return Err(err(ErrorKind::RoutingRequiresReview));
    }
    Ok(())
}
fn refresh_managed_field(
    bytes: &[u8],
    plugins: &Path,
    profile: &Path,
) -> Result<Option<Vec<u8>>, ClientConfigError> {
    if parse_config(bytes, plugins).is_ok() {
        return Ok(None);
    }
    let fields = raw_fields(bytes)?;
    known_fields(&fields, ROOT_FIELDS)?;
    let raw = fields
        .iter()
        .find(|(name, _)| name == "cliPluginsExtraDirs")
        .ok_or_else(|| err(ErrorKind::RoutingRequiresReview))?;
    let old: Vec<String> =
        serde_json::from_str(raw.1.get()).map_err(|_| err(ErrorKind::RoutingRequiresReview))?;
    if old.len() != 1 {
        return Err(err(ErrorKind::RoutingRequiresReview));
    }
    let prefix = profile
        .join(".orbwin-private")
        .join("runtime")
        .join("docker-")
        .to_str()
        .ok_or_else(|| err(ErrorKind::Unavailable))?
        .to_string();
    let suffix = format!("{}docker-cli-plugins", std::path::MAIN_SEPARATOR);
    let key = old[0]
        .strip_prefix(&prefix)
        .and_then(|tail| tail.strip_suffix(&suffix))
        .ok_or_else(|| err(ErrorKind::RoutingRequiresReview))?;
    if key.len() != 64
        || key
            .bytes()
            .any(|b| !(b.is_ascii_digit() || (b'a'..=b'f').contains(&b)))
        || key.bytes().all(|b| b == b'0')
    {
        return Err(err(ErrorKind::RoutingRequiresReview));
    }
    parse_config(bytes, Path::new(&old[0]))?;
    let mut out = vec![b'{'];
    for (index, (name, raw)) in fields.into_iter().enumerate() {
        if index > 0 {
            out.push(b',')
        };
        out.extend(go_json(&name)?);
        out.push(b':');
        if name == "cliPluginsExtraDirs" {
            out.extend(go_json(&[plugins])?)
        } else {
            out.extend(raw.get().as_bytes())
        }
    }
    out.push(b'}');
    parse_config(&out, plugins)?;
    Ok(Some(out))
}
fn defaults(plugins: &Path) -> Value {
    serde_json::json!({"auths":{},"credsStore":"orbwin","cliPluginsExtraDirs":[plugins]})
}
const FRESH_MARKER: &[u8] = br#"{"schema":1,"kind":"fresh-client"}"#;
const FRESH_RECORD_BUDGET: usize = 8192;
#[cfg(debug_assertions)]
pub(crate) mod fresh_qualification {
    use std::sync::atomic::{AtomicBool, AtomicU8, Ordering};
    #[derive(Clone, Copy, PartialEq, Eq)]
    #[repr(u8)]
    pub(crate) enum Fault {
        MarkerPrefix = 1,
        MarkerFull = 2,
        ClientFull = 3,
        ControllerFull = 4,
        DockerFull = 5,
        InitPrefix = 6,
        InitFull = 7,
        ConfigPrefix = 8,
        ConfigFull = 9,
        MarkerRemoved = 10,
        HoldBeforeClient = 11,
        HoldBeforeController = 12,
        HoldAfterController = 13,
        EmptyMarker = 14,
    }
    static ONCE: AtomicBool = AtomicBool::new(false);
    static ARMED: AtomicU8 = AtomicU8::new(0);
    static OBSERVED: AtomicU8 = AtomicU8::new(0);
    pub(crate) fn arm(fault: Fault) -> Result<(), &'static str> {
        if ONCE.swap(true, Ordering::AcqRel) {
            return Err("Fresh checkpoint already armed");
        }
        ARMED.store(fault as u8, Ordering::Release);
        Ok(())
    }
    pub(crate) fn observed() -> bool {
        OBSERVED.load(Ordering::Acquire) != 0
    }
    pub(super) fn prefix(name: &str) -> Option<u8> {
        let code = ARMED.load(Ordering::Acquire);
        let yes = matches!(
            (code, name),
            (1, "client.create-intent")
                | (14, "client.create-intent")
                | (6, "client.init")
                | (8, "config.json")
        );
        if yes
            && ARMED
                .compare_exchange(code, 0, Ordering::AcqRel, Ordering::Acquire)
                .is_ok()
        {
            Some(code)
        } else {
            None
        }
    }
    pub(super) fn checkpoint(event: Fault) -> Option<(u8, bool)> {
        let code = ARMED.load(Ordering::Acquire);
        let selected = code == event as u8
            || matches!(
                (code, event),
                (11, Fault::MarkerFull) | (12, Fault::ClientFull) | (13, Fault::ControllerFull)
            );
        if selected
            && code != 0
            && ARMED
                .compare_exchange(code, 0, Ordering::AcqRel, Ordering::Acquire)
                .is_ok()
        {
            Some((code, (11..=13).contains(&code)))
        } else {
            None
        }
    }
    pub(super) fn reached(code: u8, hold: bool) {
        OBSERVED.store(code, Ordering::Release);
        if hold {
            std::thread::sleep(std::time::Duration::from_secs(1));
            return;
        }
        println!("FRESH_PRODUCER_CUT {code}");
        std::process::exit(117)
    }
    #[cfg(test)]
    mod tests {
        use super::*;
        #[test]
        fn closed_checkpoints_have_no_path_or_ambient_configuration() {
            assert_eq!(ARMED.load(Ordering::Acquire), 0);
            for name in ["writer.intent", "../client.init", "config.stage", ""] {
                assert!(prefix(name).is_none());
            }
            for event in [
                Fault::MarkerFull,
                Fault::ClientFull,
                Fault::ControllerFull,
                Fault::DockerFull,
                Fault::InitFull,
                Fault::ConfigFull,
                Fault::MarkerRemoved,
            ] {
                assert!(checkpoint(event).is_none());
            }
            assert!(!observed());
            assert!(!ONCE.load(Ordering::Acquire));
        }
    }
}
fn fresh_marker(bytes: &[u8]) -> bool {
    FRESH_MARKER.starts_with(bytes)
}
#[derive(Clone, Copy)]
enum FreshByte {
    Exact(u8),
    Hash,
}
fn fresh_prefix_compatible(bytes: &[u8], pattern: &[FreshByte]) -> bool {
    let mut hash_count = 0;
    let mut hash_seen = 0;
    let mut nonzero = false;
    for (i, expected) in pattern.iter().enumerate() {
        if matches!(expected, FreshByte::Hash) {
            hash_count += 1;
        }
        let Some(actual) = bytes.get(i).copied() else {
            continue;
        };
        match expected {
            FreshByte::Exact(value) if *value != actual => return false,
            FreshByte::Hash => {
                if !actual.is_ascii_digit() && !(b'a'..=b'f').contains(&actual) {
                    return false;
                }
                hash_seen += 1;
                nonzero |= actual != b'0';
            }
            _ => {}
        }
    }
    hash_count != 64 || hash_seen != 64 || nonzero
}
// Only finite compact producer encodings, never generic JSON EOF. An input may
// end inside a known key/value; unused fields still have a valid completion.
fn fresh_field_prefix(bytes: &[u8], fields: &[Vec<Vec<FreshByte>>]) -> bool {
    fn visit(
        bytes: &[u8],
        fields: &[Vec<Vec<FreshByte>>],
        used: usize,
        out: &mut Vec<FreshByte>,
    ) -> bool {
        if used.count_ones() as usize == fields.len() {
            out.push(FreshByte::Exact(b'}'));
            let valid = bytes.len() <= out.len() && fresh_prefix_compatible(bytes, out);
            out.pop();
            return valid;
        }
        for (index, alternatives) in fields.iter().enumerate() {
            if used & (1 << index) != 0 {
                continue;
            }
            for field in alternatives {
                let prior = out.len();
                if used != 0 {
                    out.push(FreshByte::Exact(b','));
                }
                out.extend(field);
                if fresh_prefix_compatible(bytes, out)
                    && (bytes.len() <= out.len() || visit(bytes, fields, used | (1 << index), out))
                {
                    out.truncate(prior);
                    return true;
                }
                out.truncate(prior);
            }
        }
        false
    }
    bytes.len() <= FRESH_RECORD_BUDGET
        && bytes.first().is_none_or(|b| *b == b'{')
        && visit(bytes, fields, 0, &mut vec![FreshByte::Exact(b'{')])
}
fn fresh_exact_field(name: &str, value: &Value) -> Vec<FreshByte> {
    let mut bytes = serde_json::to_vec(name).unwrap();
    bytes.push(b':');
    bytes.extend(serde_json::to_vec(value).unwrap());
    bytes.into_iter().map(FreshByte::Exact).collect()
}
fn fresh_init_bytes(bytes: &[u8], expected: &Value) -> bool {
    if bytes.len() > FRESH_RECORD_BUDGET {
        return false;
    }
    if let Ok(value) = checked_json(bytes) {
        return value == *expected;
    }
    let fields: Vec<_> = expected
        .as_object()
        .unwrap()
        .iter()
        .map(|(key, value)| vec![fresh_exact_field(key, value)])
        .collect();
    fresh_field_prefix(bytes, &fields)
}
fn fresh_default_bytes(bytes: &[u8], profile: &Path) -> bool {
    if bytes.len() > FRESH_RECORD_BUDGET {
        return false;
    }
    let selector = |key: &str| {
        profile
            .join(".orbwin-private")
            .join("runtime")
            .join(format!("docker-{key}"))
            .join("docker-cli-plugins")
    };
    if let Ok(value) = checked_json(bytes) {
        let Some(root) = value.as_object() else {
            return false;
        };
        if root.len() != 3
            || root.get("auths") != Some(&serde_json::json!({}))
            || root.get("credsStore").and_then(Value::as_str) != Some("orbwin")
        {
            return false;
        }
        let Some(paths) = root.get("cliPluginsExtraDirs").and_then(Value::as_array) else {
            return false;
        };
        if paths.len() != 1 {
            return false;
        }
        let Some(path) = paths[0].as_str() else {
            return false;
        };
        let candidate = Path::new(path);
        let Some(parent) = candidate.parent() else {
            return false;
        };
        let Some(directory) = parent.file_name().and_then(|v| v.to_str()) else {
            return false;
        };
        let Some(key) = directory.strip_prefix("docker-") else {
            return false;
        };
        return digest_hex(key).is_ok_and(|hash| hash != [0; 32])
            && selector(key).to_str() == Some(path);
    }
    let mut paths = Vec::new();
    for go in [false, true] {
        let encode = |key: &str| -> Vec<u8> {
            let value = vec![selector(key)];
            if go {
                go_json(&value).unwrap()
            } else {
                serde_json::to_vec(&value).unwrap()
            }
        };
        let a = encode(&"a".repeat(64));
        let b = encode(&"b".repeat(64));
        if a.len() != b.len() {
            return false;
        }
        let mut field: Vec<_> =
            br#""cliPluginsExtraDirs":"#.iter().copied().map(FreshByte::Exact).collect();
        let mut wildcards = 0;
        field.extend(a.iter().zip(&b).map(|(a, b)| {
            if a != b {
                wildcards += 1;
                FreshByte::Hash
            } else {
                FreshByte::Exact(*a)
            }
        }));
        if wildcards != 64 {
            return false;
        }
        paths.push(field);
    }
    fresh_field_prefix(
        bytes,
        &[
            vec![fresh_exact_field("auths", &serde_json::json!({}))],
            vec![fresh_exact_field(
                "credsStore",
                &serde_json::json!("orbwin"),
            )],
            paths,
        ],
    )
}
fn native_component(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 192
        && value != "."
        && value != ".."
        && !value.ends_with(['.', ' '])
        && !value
            .chars()
            .any(|c| c.is_control() || r#"\/:<>"|?*"#.contains(c))
}
fn component(value: &str) -> bool {
    native_component(value)
}
// Qualification-only interruption seam. Release builds exclude both arming and
// hooks. Normal debug runs remain unarmed; no environment/IPC/path/body control
// reaches this module. The native producer still owns every actual FILE_OBJECT.
#[cfg(debug_assertions)]
pub(crate) mod qualification_faults {
    use std::sync::atomic::{AtomicU8, Ordering};
    #[derive(Clone, Copy)]
    #[repr(u8)]
    pub(crate) enum Record {
        Prepare = 1,
        Original = 2,
        Intent = 3,
        Birth = 4,
        Recovery = 5,
        Stage = 6,
    }
    impl Record {
        fn name(self) -> &'static str {
            match self {
                Self::Prepare => "writer.prepare",
                Self::Original => "writer.original",
                Self::Intent => "writer.intent",
                Self::Birth => "writer.birth",
                Self::Recovery => "recovery.intent",
                Self::Stage => "config.stage",
            }
        }
    }
    #[derive(Clone, Copy)]
    pub(crate) enum Fault {
        Prefix(Record),
        Removed(Record),
    }
    impl Fault {
        fn code(self) -> u8 {
            match self {
                Self::Prefix(record) => record as u8,
                Self::Removed(record) => 16 + record as u8,
            }
        }
    }
    static ARMED: AtomicU8 = AtomicU8::new(0);
    pub(crate) fn arm(fault: Fault) -> Result<(), &'static str> {
        ARMED
            .compare_exchange(0, fault.code(), Ordering::AcqRel, Ordering::Acquire)
            .map(|_| ())
            .map_err(|_| "Private writer fixture already armed")
    }
    fn select(code: u8, name: &str, prefix: bool) -> Option<Fault> {
        for record in [
            Record::Prepare,
            Record::Original,
            Record::Intent,
            Record::Birth,
            Record::Recovery,
            Record::Stage,
        ] {
            let fault = if prefix {
                Fault::Prefix(record)
            } else {
                Fault::Removed(record)
            };
            if code == fault.code() && name == record.name() {
                return Some(fault);
            }
        }
        None
    }
    fn selected(name: &str, prefix: bool) -> Option<Fault> {
        select(ARMED.load(Ordering::Acquire), name, prefix)
    }
    pub(super) fn prefix(name: &str) -> Option<Fault> {
        selected(name, true)
    }
    pub(super) fn removed(name: &str) -> Option<Fault> {
        selected(name, false)
    }
    pub(super) fn exit_after_cut(fault: Fault) -> ! {
        use std::io::Write;
        if ARMED
            .compare_exchange(fault.code(), 255, Ordering::AcqRel, Ordering::Acquire)
            .is_err()
        {
            std::process::exit(125);
        }
        println!("WRITER_FIXTURE_CUT {}", fault.code());
        if std::io::stdout().flush().is_err() {
            std::process::exit(125);
        }
        // Intentionally skip destructors. The qualification models creator
        // process loss, not an error result that the live owner can settle.
        std::process::exit(117)
    }
    #[cfg(test)]
    mod tests {
        use super::*;
        #[test]
        fn closed_fixture_selector_never_arms_global_state_or_accepts_paths() {
            for record in [
                Record::Prepare,
                Record::Original,
                Record::Intent,
                Record::Birth,
                Record::Recovery,
                Record::Stage,
            ] {
                assert!(select(Fault::Prefix(record).code(), record.name(), true).is_some());
                assert!(select(Fault::Removed(record).code(), record.name(), false).is_some());
                assert!(select(Fault::Prefix(record).code(), record.name(), false).is_none());
                assert!(select(Fault::Removed(record).code(), record.name(), true).is_none());
                for path in ["../writer.prepare", "config.json", "controller.lock", ""] {
                    assert!(select(Fault::Prefix(record).code(), path, true).is_none());
                }
            }
            assert!(select(0, "writer.prepare", true).is_none());
            assert!(select(255, "writer.prepare", true).is_none());
            assert_eq!(ARMED.load(Ordering::Acquire), 0);
        }
    }
}
#[derive(serde::Deserialize, serde::Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct RecoveryIntent {
    schema: u32,
    kind: String,
    mutation: String,
    binding: String,
    keys_sha256: String,
    base_sha256: String,
    base_volume: u32,
    base_high: u32,
    base_low: u32,
    base_sd_sha256: String,
    original_volume: u32,
    original_high: u32,
    original_low: u32,
    original_sd_sha256: String,
    original_sha256: String,
}
#[derive(serde::Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct RecoveryBirth {
    schema: u32,
    kind: String,
    binding: String,
    intent_sha256: String,
    intent_body: String,
    root_pid: u32,
    root_created: u64,
    root_session: u32,
}
#[derive(Clone, serde::Deserialize, serde::Serialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct RestoreIntent {
    schema: u32,
    kind: String,
    binding: String,
    current_volume: u32,
    current_high: u32,
    current_low: u32,
    current_sd_sha256: String,
    current_sha256: String,
    restore_sha256: String,
    intent_body: String,
}
#[cfg(debug_assertions)]
fn historical_qualification_bytes(base: &RestoreIntent) -> Result<Vec<u8>, ClientConfigError> {
    // The live plan remains schema 2. Only this closed historical-format
    // injection changes the cloned wire discriminator, before actual creation.
    let mut old = base.clone();
    old.schema = 1;
    let encoded = go_json(&old)?;
    let parsed = successor::parse(&encoded)?;
    if parsed.base.schema != 1 || parsed.v2.is_some() || go_json(&parsed.base)? != encoded {
        return Err(err(ErrorKind::RecoveryRequired));
    }
    Ok(encoded)
}
#[derive(serde::Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct RecoveryPrepare {
    schema: u32,
    kind: String,
    mutation: String,
    binding: String,
    keys_sha256: String,
    base_sha256: String,
    base_volume: u32,
    base_high: u32,
    base_low: u32,
    base_sd_sha256: String,
}
// A local selector replacement never lends the config to a child. This closed
// record is independent of schema 3 writer authority; it permits explicit KEEP
// CURRENT review, not reconstruction of an original that was never preserved.
struct LocalReplaceIntent {
    base: [u8; 32],
    next: Option<[u8; 32]>,
}
fn parse_local_replace(bytes: &[u8]) -> Result<LocalReplaceIntent, ClientConfigError> {
    if bytes.len() > 4096 {
        return Err(err(ErrorKind::RecoveryRequired));
    }
    let value = checked_json(bytes)?;
    let map = value
        .as_object()
        .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
    if !(map.len() == 3 || map.len() == 4)
        || map.keys().any(|key| {
            !matches!(
                key.as_str(),
                "schema" | "kind" | "baseSha256" | "nextSha256"
            )
        })
        || map.get("schema").and_then(Value::as_u64) != Some(1)
        || map.get("kind").and_then(Value::as_str) != Some("config-replace")
    {
        return Err(err(ErrorKind::RecoveryRequired));
    }
    let base = digest_hex(
        map.get("baseSha256")
            .and_then(Value::as_str)
            .ok_or_else(|| err(ErrorKind::RecoveryRequired))?,
    )?;
    let next = map
        .get("nextSha256")
        .map(|value| {
            digest_hex(
                value
                    .as_str()
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?,
            )
        })
        .transpose()?;
    Ok(LocalReplaceIntent { base, next })
}
fn local_replace_current(
    record: &LocalReplaceIntent,
    bytes: &[u8],
) -> Result<(), ClientConfigError> {
    use sha2::{Digest, Sha256};
    let hash: [u8; 32] = Sha256::digest(bytes).into();
    if record.next.is_some() && hash != record.base && Some(hash) != record.next {
        return Err(err(ErrorKind::RecoveryRequired));
    }
    Ok(())
}
fn local_replace_producer(base: &str, next: Option<&str>) -> Result<Vec<u8>, ClientConfigError> {
    digest_hex(base)?;
    if let Some(next) = next {
        digest_hex(next)?;
    }
    let mut fields = std::collections::BTreeMap::from([
        ("schema", serde_json::json!(1)),
        ("kind", serde_json::json!("config-replace")),
        ("baseSha256", serde_json::json!(base)),
    ]);
    if let Some(next) = next {
        fields.insert("nextSha256", serde_json::json!(next));
    }
    go_json(&fields)
}
fn local_eof_prefix(bytes: &[u8], current: &str) -> bool {
    if bytes.len() > 4096 || digest_hex(current).is_err() || !incomplete_record(bytes) {
        return false;
    }
    for next in [None, Some("0".repeat(64))] {
        let Ok(template) = local_replace_producer(current, next.as_deref()) else {
            return false;
        };
        if bytes.len() >= template.len() {
            continue;
        }
        let start = if next.is_some() {
            let needle = br#""nextSha256":""#;
            template
                .windows(needle.len())
                .position(|b| b == needle)
                .map(|i| i + needle.len())
        } else {
            None
        };
        if bytes.iter().copied().enumerate().all(|(i, b)| {
            if start.is_some_and(|start| (start..start + 64).contains(&i)) {
                b.is_ascii_digit() || (b'a'..=b'f').contains(&b)
            } else {
                b == template[i]
            }
        }) {
            return true;
        }
    }
    false
}
#[derive(Clone, Copy, Default)]
struct RecoverySiblings {
    prepare: bool,
    original: bool,
    birth: bool,
    recovery: bool,
    next: bool,
}
impl RecoverySiblings {
    fn any(self) -> bool {
        self.prepare || self.original || self.birth || self.recovery || self.next
    }
}
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum RecoveryRoute {
    Writer,
    LocalFull,
    LocalEof,
}
fn recovery_route(
    intent: Option<&[u8]>,
    siblings: RecoverySiblings,
    current: &str,
) -> Result<RecoveryRoute, ClientConfigError> {
    if siblings.next && !siblings.recovery {
        return Err(err(ErrorKind::RecoveryRequired));
    }
    let Some(bytes) = intent else {
        return Ok(RecoveryRoute::Writer);
    };
    if incomplete_record(bytes) {
        if siblings.any() {
            return Ok(RecoveryRoute::Writer);
        }
        return if local_eof_prefix(bytes, current) {
            Ok(RecoveryRoute::LocalEof)
        } else {
            Err(err(ErrorKind::RecoveryRequired))
        };
    }
    if parse_local_replace(bytes).is_ok() {
        return if siblings.any() {
            Err(err(ErrorKind::RecoveryRequired))
        } else {
            Ok(RecoveryRoute::LocalFull)
        };
    }
    Ok(RecoveryRoute::Writer)
}
fn local_eof_plan(
    intent: &[u8],
    current: &[u8],
    profile: &Path,
    plugins: &Path,
    strict: bool,
    authority: impl FnOnce() -> Result<(), ClientConfigError>,
) -> Result<Vec<u8>, ClientConfigError> {
    if !strict {
        return Err(err(ErrorKind::RecoveryRequired));
    }
    authority()?;
    successor::owned_selector(current, profile)?;
    if !local_eof_prefix(intent, &successor::hash(current)) {
        return Err(err(ErrorKind::RecoveryRequired));
    }
    successor::target(current, plugins, profile)
}
fn digest_hex(value: &str) -> Result<[u8; 32], ClientConfigError> {
    if value.len() != 64
        || !value
            .bytes()
            .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
    {
        return Err(err(ErrorKind::RecoveryRequired));
    }
    let mut out = [0; 32];
    for (i, byte) in out.iter_mut().enumerate() {
        *byte = u8::from_str_radix(&value[i * 2..i * 2 + 2], 16)
            .map_err(|_| err(ErrorKind::RecoveryRequired))?;
    }
    Ok(out)
}
fn parse_recovery_intent(bytes: &[u8]) -> Result<RecoveryIntent, ClientConfigError> {
    if bytes.len() > 4096 {
        return Err(err(ErrorKind::RecoveryRequired));
    }
    let value: RecoveryIntent =
        serde_json::from_slice(bytes).map_err(|_| err(ErrorKind::RecoveryRequired))?;
    if value.schema != 3
        || value.kind != "docker-writer"
        || !matches!(
            value.mutation.as_str(),
            "registry-entries-v2" | "root-unchanged-v1"
        )
        || digest_hex(&value.binding)? == [0; 32]
        || digest_hex(&value.keys_sha256)? == [0; 32]
        || value.base_sha256 != value.original_sha256
    {
        return Err(err(ErrorKind::RecoveryRequired));
    }
    for hash in [
        &value.base_sha256,
        &value.base_sd_sha256,
        &value.original_sd_sha256,
        &value.original_sha256,
    ] {
        digest_hex(hash)?;
    }
    Ok(value)
}
fn validate_recovery_birth(
    birth: &RecoveryBirth,
    intent: &[u8],
    binding: &str,
) -> Result<(), ClientConfigError> {
    use sha2::{Digest, Sha256};
    if birth.schema != 3
        || birth.kind != "docker-writer-birth"
        || birth.binding != binding
        || digest_hex(&birth.intent_sha256)? != <[u8; 32]>::from(Sha256::digest(intent))
        || birth.intent_body.as_bytes() != intent
        || birth.root_pid == 0
        || birth.root_created == 0
    {
        return Err(err(ErrorKind::RecoveryRequired));
    }
    Ok(())
}
fn parse_birth_record(bytes: &[u8]) -> Result<Option<RecoveryBirth>, ClientConfigError> {
    if bytes.len() > 8192 {
        return Err(err(ErrorKind::RecoveryRequired));
    }
    match serde_json::from_slice(bytes) {
        Ok(value) => Ok(Some(value)),
        Err(error) if error.is_eof() && incomplete_record(bytes) => Ok(None),
        Err(_) => Err(err(ErrorKind::RecoveryRequired)),
    }
}
fn incomplete_record(bytes: &[u8]) -> bool {
    let first = bytes.iter().copied().find(|b| !b.is_ascii_whitespace());
    if first.is_some_and(|b| b != b'{') {
        return false;
    }
    serde_json::from_slice::<Value>(bytes).is_err_and(|e| e.is_eof())
}
// No renderer hash/path constructor: both executable capabilities come from
// the existing sealed fixed-role cache publisher and stay held for this guard.
pub(crate) struct WriterAdmission {
    binding: [u8; 32],
    invocation: Option<WriterInvocation>,
    #[cfg(windows)]
    birth: Option<native::BirthPublisher>,
}
struct WriterInvocation {
    docker: crate::runtime_cache::CachedExecutable,
    args: Vec<OsString>,
    credentials: crate::runtime_cache::CachedExecutable,
    config: PathBuf,
    plugins: PathBuf,
}
impl WriterAdmission {
    #[cfg(windows)]
    pub(crate) fn publish_birth(
        &mut self,
        birth: crate::native_client::NativeBirth,
    ) -> Result<(), String> {
        self.birth
            .take()
            .ok_or("Private writer birth already consumed")?
            .publish(birth)
            .map_err(|_| "Private writer birth persistence failed".to_owned())?;
        // The consumed publisher and all duplicated data/parent caps have now
        // been dropped. Native quarantine must be checked after that boundary,
        // never inferred from an earlier successful record write.
        native::writer_resources_known()
            .map_err(|_| "Private writer native resources remain unknown".to_owned())
    }
    pub(crate) fn binding(&self) -> [u8; 32] {
        self.binding
    }
    pub(crate) fn validate_invocation(
        &self,
        executable: &crate::runtime_cache::CachedExecutable,
        args: &[OsString],
        env: &[(OsString, OsString)],
    ) -> Result<(), String> {
        let expected = self
            .invocation
            .as_ref()
            .ok_or("Private writer invocation unavailable")?;
        expected.docker.check_held_origin()?;
        expected.credentials.check_held_origin()?;
        executable.check_held_origin()?;
        if expected.docker.path() != executable.path() || expected.args != args {
            return Err("Private writer invocation does not match its admission".into());
        }
        validate_writer_environment(
            env,
            &expected.config,
            &expected.plugins,
            expected.docker.path(),
            expected.credentials.path(),
        )
    }
}
// Validate the complete managed selector set independently of caller-supplied
// strings. Ambient entries are permitted but cannot shadow owned selectors.
fn validate_writer_environment(
    env: &[(OsString, OsString)],
    config: &Path,
    plugins: &Path,
    docker: &Path,
    credentials: &Path,
) -> Result<(), String> {
    let mut entries = std::collections::BTreeMap::new();
    for (key, value) in env {
        let key = key.to_str().ok_or("Invalid writer environment")?;
        let drive = key.len() == 3
            && key.as_bytes()[0] == b'='
            && key.as_bytes()[1].is_ascii_alphabetic()
            && key.as_bytes()[2] == b':';
        if key.is_empty()
            || key.contains('\0')
            || (key.contains('=') && !drive)
            || value.as_encoded_bytes().contains(&0)
            || entries
                .insert(key.to_uppercase(), value.as_os_str())
                .is_some()
        {
            return Err("Ambiguous writer environment".into());
        }
    }
    for (key, value) in [
        ("DOCKER_CONFIG", config.as_os_str()),
        (
            "DOCKER_HOST",
            std::ffi::OsStr::new("npipe:////./pipe/docker-rancher-desktop-orbwin"),
        ),
        ("ORBWIN_DOCKER_MANAGED_PLUGINS", plugins.as_os_str()),
        ("PATHEXT", std::ffi::OsStr::new(".EXE")),
        (
            "NODEFAULTCURRENTDIRECTORYINEXEPATH",
            std::ffi::OsStr::new("1"),
        ),
    ] {
        if entries.get(key).copied() != Some(value) {
            return Err("Writer selector differs from admission".into());
        }
    }
    for key in [
        "DOCKER_CONTEXT",
        "DOCKER_TLS_VERIFY",
        "DOCKER_CERT_PATH",
        "DOCKER_API_VERSION",
        "DOCKER_AUTH_CONFIG",
    ] {
        if entries.contains_key(key) {
            return Err("Foreign writer selector".into());
        }
    }
    let path = entries.get("PATH").ok_or("Missing helper path")?;
    let mut paths = std::env::split_paths(path);
    if paths.next().as_deref() != credentials.parent() || paths.next().as_deref() != docker.parent()
    {
        return Err("Foreign writer helper path".into());
    }
    Ok(())
}
impl std::fmt::Debug for WriterAdmission {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str("WriterAdmission(private native loan)")
    }
}
pub(crate) struct ClientConfigGuard {
    #[cfg(windows)]
    inner: Option<Box<native::Owned>>,
    path: PathBuf,
}
// The backend owns this capability; a renderer receives only its own opaque
// epoch token and a fixed summary. Drop/expiry keeps the pending evidence.
pub(crate) struct WriterRecovery {
    guard: Option<ClientConfigGuard>,
    #[cfg(windows)]
    fresh: Option<Box<native::FreshInitialization>>,
    expires: std::time::Instant,
}
impl WriterRecovery {
    pub(crate) fn close(mut self) -> Result<(), ClientConfigError> {
        #[cfg(windows)]
        if let Some(mut fresh) = self.fresh.take() {
            return fresh.release();
        }
        #[cfg(windows)]
        if let Some(mut guard) = self.guard.take() {
            if let Some(inner) = guard.inner.take() {
                return native::release_pending(inner);
            }
        }
        Ok(())
    }
    pub(crate) fn summary(&self) -> &'static str {
        #[cfg(windows)]
        if self.fresh.is_some() {
            return "Finish initializing private Docker settings using empty credential settings and the current managed selector. No credential command was started. Existing user settings or unknown files will not be replaced.";
        }
        #[cfg(windows)]
        if let Some(inner) = self.guard.as_ref().and_then(|guard| guard.inner.as_ref()) {
            if inner.is_completed_birth() {
                return "Finalize the committed private Docker settings and refresh their managed selector. Registry or credential changes may already have completed; no credential command will be replayed.";
            }
            if inner.is_local_replace() {
                return "Keep the validated current private Docker settings and discard the captured unpublished replacement stage. Then refresh managed settings. No credential command was started by this replacement.";
            }
            if inner.recovery_preserves_source() {
                return "Finish the interrupted preparation of private Docker settings while preserving the unchanged source settings. No credential command was resumed. Any unpublished recovery stage will be discarded.";
            }
            if !inner.has_recovery_original() {
                return "Keep the validated committed private Docker settings, finish interrupted recovery, and refresh the managed selector. Registry authentication and credential-store changes may already have completed; refresh credentials afterward.";
            }
        }
        "Restore the preserved private Docker settings, discarding any unpublished recovery stage. Registry authentication and credential-store changes may already have completed; refresh credentials afterward."
    }
    pub(crate) fn restore(mut self) -> Result<ClientConfigGuard, ClientConfigError> {
        if std::time::Instant::now() >= self.expires {
            return Err(err(ErrorKind::RecoveryRequired));
        }
        #[cfg(windows)]
        if let Some(fresh) = self.fresh.take() {
            return fresh.apply();
        }
        let mut guard = self
            .guard
            .take()
            .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
        #[cfg(windows)]
        if let Err(error) = guard
            .inner
            .as_mut()
            .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
            .restore_reviewed()
        {
            if let Some(inner) = guard.inner.take() {
                let _ = native::release_pending(inner);
            }
            return Err(error);
        }
        #[cfg(not(windows))]
        return Err(err(ErrorKind::Unavailable));
        Ok(guard)
    }
}
impl Drop for WriterRecovery {
    fn drop(&mut self) {
        #[cfg(windows)]
        if let Some(mut fresh) = self.fresh.take() {
            let _ = fresh.release();
        }
        #[cfg(windows)]
        if let Some(mut guard) = self.guard.take() {
            if let Some(inner) = guard.inner.take() {
                let _ = native::release_pending(inner);
            }
        }
    }
}
pub(crate) fn prepare_writer_recovery(
    docker: crate::runtime_cache::CachedExecutable,
    credentials: crate::runtime_cache::CachedExecutable,
) -> Result<WriterRecovery, ClientConfigError> {
    #[cfg(windows)]
    {
        let fresh = native::prepare_fresh(docker.clone(), credentials.clone())?;
        if fresh.is_some() {
            return Ok(WriterRecovery {
                guard: None,
                fresh,
                expires: std::time::Instant::now() + std::time::Duration::from_secs(300),
            });
        }
        Ok(WriterRecovery {
            guard: Some(native::acquire_mode(docker, credentials, true)?),
            fresh: None,
            expires: std::time::Instant::now() + std::time::Duration::from_secs(300),
        })
    }
    #[cfg(not(windows))]
    {
        let _ = (docker, credentials);
        Err(err(ErrorKind::Unavailable))
    }
}
impl std::fmt::Debug for ClientConfigGuard {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str("ClientConfigGuard(retained private native state)")
    }
}
impl ClientConfigGuard {
    pub(crate) fn release_failed_spawn(
        mut self,
        proof: crate::native_client::StoppedWriterClient,
    ) -> Result<(), ClientConfigError> {
        #[cfg(windows)]
        {
            let inner = self
                .inner
                .take()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            native::release_failed_spawn(inner, proof)
        }
        #[cfg(not(windows))]
        {
            let _ = proof;
            Err(err(ErrorKind::Unavailable))
        }
    }
    pub(crate) fn release_interrupted(
        mut self,
        proof: crate::client_job::ExecutionQuiesced,
    ) -> Result<(), ClientConfigError> {
        #[cfg(windows)]
        {
            let inner = self
                .inner
                .take()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            native::release_interrupted(inner, proof)
        }
        #[cfg(not(windows))]
        {
            let _ = proof;
            Err(err(ErrorKind::Unavailable))
        }
    }
    pub(crate) fn path(&self) -> &Path {
        &self.path
    }
    pub(crate) fn check(&self) -> Result<(), ClientConfigError> {
        #[cfg(windows)]
        {
            self.inner
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                .check()
        }
        #[cfg(not(windows))]
        {
            Err(err(ErrorKind::Unavailable))
        }
    }
    pub(crate) fn read_config_bytes(&self) -> Result<Vec<u8>, ClientConfigError> {
        self.check()?;
        #[cfg(windows)]
        {
            Ok(self
                .inner
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                .body
                .clone())
        }
        #[cfg(not(windows))]
        {
            Err(err(ErrorKind::Unavailable))
        }
    }
    pub(crate) fn read_config(&self) -> Result<Vec<u8>, ClientConfigError> {
        self.read_config_bytes()
    }
    pub(crate) fn replace_config(&mut self, bytes: &[u8]) -> Result<(), ClientConfigError> {
        #[cfg(windows)]
        {
            self.inner
                .as_mut()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                .replace_config(bytes)
        }
        #[cfg(not(windows))]
        {
            let _ = bytes;
            Err(err(ErrorKind::Unavailable))
        }
    }
    pub(crate) fn close(mut self) -> Result<(), ClientConfigError> {
        #[cfg(windows)]
        {
            if let Some(inner) = self.inner.take() {
                return native::settle(inner);
            }
        }
        Ok(())
    }
    /// Empty-scope admission retained for unstarted fixtures; root cannot change.
    pub(crate) fn begin_writer(&mut self) -> Result<WriterAdmission, ClientConfigError> {
        self.begin_writer_admitted(Vec::new(), Vec::new())
    }
    /// Keys must be derived from the exact selected Docker command's registry
    /// normalization. Settlement preserves every other registry RawValue.
    pub(crate) fn begin_registry_writer(
        &mut self,
        keys: &[String],
        docker_args: &[OsString],
    ) -> Result<WriterAdmission, ClientConfigError> {
        let keys = registry_keys(keys)?;
        validate_registry_login(&self.path, &keys, docker_args)?;
        self.begin_writer_admitted(keys, docker_args.to_vec())
    }
    fn begin_writer_admitted(
        &mut self,
        keys: Vec<String>,
        args: Vec<OsString>,
    ) -> Result<WriterAdmission, ClientConfigError> {
        #[cfg(windows)]
        {
            let result = self
                .inner
                .as_mut()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                .begin_writer(keys, args, self.path.clone());
            // No admission escaped on error, so no native producer can have
            // created user code. Preserve pending records, release only known
            // resources; the native uncertainty latch retains unsafe owners.
            if result.is_err() {
                if let Some(inner) = self.inner.take() {
                    let _ = native::release_pending(inner);
                }
            }
            result
        }
        #[cfg(not(windows))]
        {
            let _ = (keys, args);
            Err(err(ErrorKind::Unavailable))
        }
    }
    // Integration requires the exact admitted execution proof and natural root
    // completion with joined IO. Unknown/cancellation/drop retains writer.intent.
    pub(crate) fn abort_unstarted(
        &mut self,
        proof: crate::native_client::NeverStartedClient,
    ) -> Result<(), ClientConfigError> {
        #[cfg(windows)]
        {
            let result = self
                .inner
                .as_mut()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                .abort_unstarted(proof);
            if result.is_err()
                && self
                    .inner
                    .as_ref()
                    .is_some_and(|inner| inner.writer_stopped())
            {
                let _ = native::release_pending(self.inner.take().unwrap());
            }
            result
        }
        #[cfg(not(windows))]
        {
            let _ = proof;
            Err(err(ErrorKind::Unavailable))
        }
    }
    pub(crate) fn finish_writer(
        &mut self,
        retired: crate::client_job::ExecutionQuiesced,
    ) -> Result<(), ClientConfigError> {
        #[cfg(windows)]
        {
            let result = self
                .inner
                .as_mut()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                .finish_writer(retired);
            if result.is_err()
                && self
                    .inner
                    .as_ref()
                    .is_some_and(|inner| inner.writer_stopped())
            {
                let _ = native::release_pending(self.inner.take().unwrap());
            }
            result
        }
        #[cfg(not(windows))]
        {
            let _ = retired;
            Err(err(ErrorKind::Unavailable))
        }
    }
}
impl Drop for ClientConfigGuard {
    fn drop(&mut self) {
        #[cfg(windows)]
        if let Some(inner) = self.inner.take() {
            let _ = native::settle(inner);
        }
    }
}
#[derive(Clone)]
pub(crate) struct ClientConfigSnapshot {
    #[cfg(windows)]
    inner: Option<std::sync::Arc<native::Snapshot>>,
    path: PathBuf,
    buildx: PathBuf,
}
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum SnapshotRelease {
    Shared,
    Retired,
}
fn release_snapshot_arc<T>(
    arc: std::sync::Arc<T>,
    close: impl FnOnce(T) -> Result<(), ClientConfigError>,
) -> Result<SnapshotRelease, ClientConfigError> {
    match std::sync::Arc::into_inner(arc) {
        Some(graph) => {
            close(graph)?;
            Ok(SnapshotRelease::Retired)
        }
        None => Ok(SnapshotRelease::Shared),
    }
}
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum SnapshotCloseStep {
    Config,
    Init,
    Ancestor(usize),
    Token,
    Roles,
}
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
enum TargetPublishStep {
    Arm,
    Attempt,
    CloseStage,
    Reopen,
    Verify,
    Complete,
}
fn target_publish_sequence(
    mut call: impl FnMut(TargetPublishStep) -> Result<(), ClientConfigError>,
) -> Result<(), ClientConfigError> {
    for step in [
        TargetPublishStep::Arm,
        TargetPublishStep::Attempt,
        TargetPublishStep::CloseStage,
        TargetPublishStep::Reopen,
        TargetPublishStep::Verify,
        TargetPublishStep::Complete,
    ] {
        call(step)?;
    }
    Ok(())
}
fn snapshot_close_sequence(
    ancestors: usize,
    mut call: impl FnMut(SnapshotCloseStep) -> Result<(), ClientConfigError>,
) -> Result<(), ClientConfigError> {
    call(SnapshotCloseStep::Config)?;
    call(SnapshotCloseStep::Init)?;
    for index in (0..ancestors).rev() {
        call(SnapshotCloseStep::Ancestor(index))?;
    }
    call(SnapshotCloseStep::Token)?;
    call(SnapshotCloseStep::Roles)
}
impl ClientConfigSnapshot {
    pub(crate) fn close(mut self) -> Result<SnapshotRelease, ClientConfigError> {
        #[cfg(windows)]
        {
            let arc = self
                .inner
                .take()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            release_snapshot_arc(arc, |mut snapshot| snapshot.release())
        }
        #[cfg(not(windows))]
        {
            Err(err(ErrorKind::Unavailable))
        }
    }
    pub(crate) fn path(&self) -> &Path {
        &self.path
    }
    pub(crate) fn buildx_path(&self) -> &Path {
        &self.buildx
    }
    pub(crate) fn check(&self) -> Result<(), ClientConfigError> {
        #[cfg(windows)]
        {
            self.inner
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                .check()
        }
        #[cfg(not(windows))]
        {
            Err(err(ErrorKind::Unavailable))
        }
    }
}
impl Drop for ClientConfigSnapshot {
    fn drop(&mut self) {
        #[cfg(windows)]
        if let Some(arc) = self.inner.take() {
            let _ = release_snapshot_arc(arc, |mut snapshot| snapshot.release());
        }
    }
}
impl std::fmt::Debug for ClientConfigSnapshot {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str("ClientConfigSnapshot(immutable private generation)")
    }
}
pub(crate) fn reader(
    docker: crate::runtime_cache::CachedExecutable,
    credentials: crate::runtime_cache::CachedExecutable,
) -> Result<ClientConfigSnapshot, ClientConfigError> {
    #[cfg(windows)]
    {
        let mut owner = acquire(docker, credentials)?;
        let snapshot = owner
            .inner
            .as_mut()
            .ok_or_else(|| err(ErrorKind::Unavailable))?
            .snapshot()?;
        owner.close()?;
        Ok(snapshot)
    }
    #[cfg(not(windows))]
    {
        let _ = (docker, credentials);
        Err(err(ErrorKind::Unavailable))
    }
}
pub(crate) fn acquire(
    docker: crate::runtime_cache::CachedExecutable,
    credentials: crate::runtime_cache::CachedExecutable,
) -> Result<ClientConfigGuard, ClientConfigError> {
    #[cfg(windows)]
    {
        native::acquire(docker, credentials)
    }
    #[cfg(not(windows))]
    {
        let _ = (docker, credentials);
        Err(err(ErrorKind::Unavailable))
    }
}
// Typed desktop-only selection. Data/paths/native identities never cross this
// boundary; the existing coordinator owns epoch/expiry/cancel UI serialization.
#[cfg(windows)]
pub(crate) struct LegacyImportReview {
    inner: Option<Box<native::import_owner::Owner>>,
    expires: std::time::Instant,
}
#[cfg(windows)]
pub(crate) use legacy_import::ImportAction;
#[cfg(windows)]
pub(crate) fn prepare_legacy_import(
    docker: crate::runtime_cache::CachedExecutable,
    credentials: crate::runtime_cache::CachedExecutable,
    action: ImportAction,
) -> Result<LegacyImportReview, ClientConfigError> {
    Ok(LegacyImportReview {
        inner: Some(native::import_owner::Owner::prepare(
            docker,
            credentials,
            action,
        )?),
        expires: std::time::Instant::now() + std::time::Duration::from_secs(300),
    })
}
#[cfg(windows)]
impl LegacyImportReview {
    pub(crate) fn summary(&self) -> &'static str {
        self.inner.as_ref().expect("live import review").summary()
    }
    pub(crate) fn cancel(mut self) -> Result<(), ClientConfigError> {
        if let Some(mut owner) = self.inner.take() {
            owner.release()?
        };
        Ok(())
    }
    pub(crate) fn apply(mut self) -> Result<ClientConfigGuard, ClientConfigError> {
        if std::time::Instant::now() >= self.expires {
            return Err(err(ErrorKind::RecoveryRequired));
        };
        self.inner
            .take()
            .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
            .apply()
    }
}
#[cfg(windows)]
pub(crate) struct EmptyBootstrapReview {
    inner: Option<Box<native::FreshInitialization>>,
    expires: std::time::Instant,
}
#[cfg(windows)]
pub(crate) fn prepare_empty_bootstrap(
    docker: crate::runtime_cache::CachedExecutable,
    credentials: crate::runtime_cache::CachedExecutable,
) -> Result<EmptyBootstrapReview, ClientConfigError> {
    Ok(EmptyBootstrapReview {
        inner: Some(
            native::prepare_fresh_mode(docker, credentials, true)?
                .ok_or_else(|| err(ErrorKind::NoInterruptedWrite))?,
        ),
        expires: std::time::Instant::now() + std::time::Duration::from_secs(300),
    })
}
#[cfg(windows)]
impl EmptyBootstrapReview {
    pub(crate) fn summary(&self) -> &'static str {
        "Initialize the private OrbWin client with empty credential settings. Historical OrbWin client settings must be absent; no source is imported."
    }
    pub(crate) fn cancel(mut self) -> Result<(), ClientConfigError> {
        if let Some(mut owner) = self.inner.take() {
            owner.release()?
        };
        Ok(())
    }
    pub(crate) fn apply(mut self) -> Result<ClientConfigGuard, ClientConfigError> {
        if std::time::Instant::now() >= self.expires {
            return Err(err(ErrorKind::RecoveryRequired));
        };
        self.inner
            .take()
            .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
            .apply()
    }
}
#[cfg(windows)]
mod native {
    include!("client_config_import_native.rs");
    use super::*;
    use sha2::{Digest, Sha256};
    use std::{
        ffi::c_void,
        fs::File,
        io::{Read, Seek, SeekFrom, Write},
        mem::{size_of, zeroed},
        os::windows::{
            ffi::OsStrExt,
            io::{AsRawHandle, FromRawHandle, IntoRawHandle},
        },
        ptr::{null, null_mut},
    };
    use windows_sys::Win32::{
        Foundation::*,
        Security::{Authorization::*, *},
        Storage::FileSystem::*,
        System::{RemoteDesktop::ProcessIdToSessionId, Threading::*},
        UI::Shell::{FOLDERID_LocalAppData, FOLDERID_Profile, SHGetKnownFolderPath},
    };
    const READ_CONTROL_MASK: u32 = 0x20000;
    // Set only by actual uncertain native resource producers. Never execution
    // quiescence, persisted state or a caller-controlled recovery bypass.
    static RESOURCE_UNKNOWN: std::sync::atomic::AtomicBool =
        std::sync::atomic::AtomicBool::new(false);
    fn retain_unknown() {
        RESOURCE_UNKNOWN.store(true, std::sync::atomic::Ordering::Release);
    }
    pub(super) fn writer_resources_known() -> Result<(), ClientConfigError> {
        if RESOURCE_UNKNOWN.load(std::sync::atomic::Ordering::Acquire) {
            Err(err(ErrorKind::RecoveryRequired))
        } else {
            Ok(())
        }
    }
    const SYNCHRONIZE_MASK: u32 = 0x100000;
    const DELETE_MASK: u32 = 0x10000;
    const FILE_OPEN: u32 = 1;
    const FILE_CREATE: u32 = 2;
    const FILE_OPEN_IF: u32 = 3;
    const OBJ_CASE_INSENSITIVE: u32 = 0x40;
    const OBJ_DONT_REPARSE: u32 = 0x1000;
    const FILE_DIRECTORY_FILE: u32 = 1;
    const FILE_NON_DIRECTORY_FILE: u32 = 0x40;
    const FILE_SYNCHRONOUS_IO_NONALERT: u32 = 0x20;
    const NT_OPEN_REPARSE: u32 = 0x200000;
    const FILE_PERSISTENT_ACLS: u32 = 0x8;
    #[repr(C)]
    struct Unicode {
        length: u16,
        maximum: u16,
        buffer: *mut u16,
    }
    #[repr(C)]
    struct Attributes {
        length: u32,
        root: HANDLE,
        name: *mut Unicode,
        flags: u32,
        descriptor: *mut c_void,
        qos: *mut c_void,
    }
    #[repr(C)]
    struct IoStatus {
        status: usize,
        information: usize,
    }
    #[repr(C)]
    struct Rename {
        replace: u8,
        root: HANDLE,
        length: u32,
        name: [u16; 1],
    }
    #[link(name = "ntdll")]
    unsafe extern "system" {
        fn NtCreateFile(
            out: *mut HANDLE,
            access: u32,
            attributes: *mut Attributes,
            status: *mut IoStatus,
            allocation: *const i64,
            file_attributes: u32,
            share: u32,
            disposition: u32,
            options: u32,
            ea: *const c_void,
            ea_length: u32,
        ) -> i32;
        fn NtSetInformationFile(
            file: HANDLE,
            status: *mut IoStatus,
            information: *mut c_void,
            length: u32,
            class: u32,
        ) -> i32;
    }
    #[link(name = "ole32")]
    unsafe extern "system" {
        fn CoTaskMemFree(p: *const c_void);
    }
    fn bad(stage: &'static str) -> NativeDiagnostic {
        NativeDiagnostic {
            stage,
            system_error: None,
            security: None,
        }
    }
    fn system(stage: &'static str) -> NativeDiagnostic {
        NativeDiagnostic {
            stage,
            system_error: Some(unsafe { GetLastError() }),
            security: None,
        }
    }
    fn explain(kind: ErrorKind, diagnostic: NativeDiagnostic) -> ClientConfigError {
        ClientConfigError {
            kind,
            diagnostic: Some(diagnostic),
        }
    }
    fn raw(file: &File) -> HANDLE {
        file.as_raw_handle()
    }
    fn own(h: HANDLE) -> Result<File, NativeDiagnostic> {
        if h.is_null() || h == INVALID_HANDLE_VALUE {
            Err(system("native handle"))
        } else {
            Ok(unsafe { File::from_raw_handle(h) })
        }
    }
    fn wide(path: &Path) -> Result<Vec<u16>, NativeDiagnostic> {
        let v: Vec<u16> = path.as_os_str().encode_wide().collect();
        if v.len() > 32760 || v.contains(&0) {
            return Err(bad("path encoding"));
        }
        Ok(v.into_iter().chain(Some(0)).collect())
    }
    fn sid_at(
        base: *const u8,
        length: usize,
        p: *const c_void,
    ) -> Result<Vec<u8>, NativeDiagnostic> {
        let start = base as usize;
        let address = p as usize;
        if p.is_null()
            || address < start
            || address - start > length
            || length - (address - start) < 8
        {
            return Err(bad("SID bounds"));
        }
        let count = unsafe { *(p.cast::<u8>().add(1)) } as usize;
        let len = 8 + count * 4;
        if count > 15
            || len > length - (address - start)
            || unsafe { IsValidSid(p.cast_mut()) } == 0
            || unsafe { GetLengthSid(p.cast_mut()) } as usize != len
        {
            return Err(bad("SID shape"));
        }
        Ok(unsafe { std::slice::from_raw_parts(p.cast::<u8>(), len) }.to_vec())
    }
    fn sid_text(sid: &[u8]) -> Result<String, NativeDiagnostic> {
        let mut p = null_mut();
        if unsafe { ConvertSidToStringSidW(sid.as_ptr().cast_mut().cast(), &mut p) } == 0 {
            return Err(bad("SID encoding"));
        }
        let mut n = 0;
        while n < 184 && unsafe { *p.add(n) } != 0 {
            n += 1
        }
        let result = if n < 184 {
            String::from_utf16(unsafe { std::slice::from_raw_parts(p, n) })
                .map_err(|_| bad("SID encoding"))
        } else {
            Err(bad("SID bounds"))
        };
        unsafe {
            LocalFree(p.cast());
        }
        result
    }
    struct User {
        sid: Vec<u8>,
        text: String,
        token: File,
        session: u32,
    }
    impl User {
        fn current() -> Result<Self, NativeDiagnostic> {
            let mut thread = null_mut();
            if unsafe { OpenThreadToken(GetCurrentThread(), TOKEN_QUERY, 1, &mut thread) } != 0 {
                unsafe {
                    CloseHandle(thread);
                }
                return Err(bad("impersonated caller"));
            }
            if unsafe { GetLastError() } != ERROR_NO_TOKEN {
                return Err(bad("effective token"));
            }
            let mut h = null_mut();
            if unsafe { OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &mut h) } == 0 {
                return Err(system("process token"));
            }
            let token = own(h)?;
            let mut elevated = 0u32;
            let mut needed = 0;
            if unsafe {
                GetTokenInformation(
                    raw(&token),
                    TokenElevation,
                    (&mut elevated as *mut u32).cast(),
                    4,
                    &mut needed,
                )
            } == 0
                || needed != 4
                || elevated != 0
            {
                return Err(bad("ordinary caller"));
            }
            // TokenHasRestrictions means "ever filtered", so rejects an ordinary
            // UAC limited token. Restricting SIDs are the actual extra access-check
            // boundary; query the documented bounded TOKEN_GROUPS contract instead.
            for class in [TokenIsAppContainer] {
                let mut value = 0u32;
                if unsafe {
                    GetTokenInformation(
                        raw(&token),
                        class,
                        (&mut value as *mut u32).cast(),
                        4,
                        &mut needed,
                    )
                } == 0
                    || needed != 4
                    || value != 0
                {
                    return Err(bad("ordinary unrestricted token"));
                }
            }
            let mut restricted = [0u64; 256];
            if unsafe {
                GetTokenInformation(
                    raw(&token),
                    TokenRestrictedSids,
                    restricted.as_mut_ptr().cast(),
                    std::mem::size_of_val(&restricted) as u32,
                    &mut needed,
                )
            } == 0
            {
                return Err(system("restricting SID query"));
            }
            if !no_restricting_sids(&restricted, needed) {
                return Err(bad("restricting SID token/bounds"));
            }
            let mut session = 0u32;
            if unsafe {
                GetTokenInformation(
                    raw(&token),
                    TokenSessionId,
                    (&mut session as *mut u32).cast(),
                    4,
                    &mut needed,
                )
            } == 0
                || needed != 4
                || session == 0
            {
                return Err(bad("interactive token session"));
            }
            let mut buffer = [0u64; 256];
            if unsafe {
                GetTokenInformation(
                    raw(&token),
                    TokenUser,
                    buffer.as_mut_ptr().cast(),
                    size_of::<[u64; 256]>() as u32,
                    &mut needed,
                )
            } == 0
                || needed as usize > size_of::<[u64; 256]>()
                || needed < size_of::<TOKEN_USER>() as u32
            {
                return Err(bad("token user"));
            }
            let user = unsafe { &*buffer.as_ptr().cast::<TOKEN_USER>() };
            let sid = sid_at(buffer.as_ptr().cast(), needed as usize, user.User.Sid)?;
            let text = sid_text(&sid)?;
            if !(text.starts_with("S-1-5-21-") || text.starts_with("S-1-12-1-")) {
                return Err(bad("ordinary account"));
            }
            Ok(Self {
                sid,
                text,
                token,
                session,
            })
        }
        fn profile(&self) -> Result<PathBuf, NativeDiagnostic> {
            let mut p = null_mut();
            if unsafe { SHGetKnownFolderPath(&FOLDERID_Profile, 0, raw(&self.token), &mut p) } < 0
                || p.is_null()
            {
                return Err(bad("native profile"));
            }
            let mut n = 0;
            while n < 1024 && unsafe { *p.add(n) } != 0 {
                n += 1
            }
            let result = if n < 1024 {
                String::from_utf16(unsafe { std::slice::from_raw_parts(p, n) })
                    .map(PathBuf::from)
                    .map_err(|_| bad("profile encoding"))
            } else {
                Err(bad("profile bounds"))
            };
            unsafe {
                CoTaskMemFree(p.cast());
            }
            result
        }
    }
    struct Descriptor {
        data: *mut c_void,
    }
    impl Drop for Descriptor {
        fn drop(&mut self) {
            unsafe {
                LocalFree(self.data);
            }
        }
    }
    impl Descriptor {
        fn private(user: &User, directory: bool) -> Result<Self, NativeDiagnostic> {
            let inherit = if directory { "OICI" } else { "" };
            let text: Vec<u16> = format!(
                "O:{}D:P(A;{inherit};FA;;;{})(A;{inherit};FA;;;SY)(A;{inherit};FA;;;BA)",
                user.text, user.text
            )
            .encode_utf16()
            .chain(Some(0))
            .collect();
            let mut p = null_mut();
            if unsafe {
                ConvertStringSecurityDescriptorToSecurityDescriptorW(
                    text.as_ptr(),
                    1,
                    &mut p,
                    null_mut(),
                )
            } == 0
            {
                return Err(bad("private descriptor"));
            }
            Ok(Self { data: p })
        }
    }
    struct Security {
        bytes: Vec<u32>,
        length: usize,
    }
    impl Security {
        fn read(file: &File) -> Result<Self, NativeDiagnostic> {
            let mut n = 0;
            unsafe {
                GetKernelObjectSecurity(
                    raw(file),
                    OWNER_SECURITY_INFORMATION | DACL_SECURITY_INFORMATION,
                    null_mut(),
                    0,
                    &mut n,
                );
            }
            if !(20..=65536).contains(&n) {
                return Err(bad("descriptor bounds"));
            }
            let mut bytes = vec![0u32; (n as usize).div_ceil(4)];
            let capacity = n;
            if unsafe {
                GetKernelObjectSecurity(
                    raw(file),
                    OWNER_SECURITY_INFORMATION | DACL_SECURITY_INFORMATION,
                    bytes.as_mut_ptr().cast(),
                    capacity,
                    &mut n,
                )
            } == 0
                || n > capacity
                || n < 20
            {
                return Err(bad("descriptor read"));
            }
            Ok(Self {
                bytes,
                length: n as usize,
            })
        }
        // Canonical Data-file SD query. GetKernelObjectSecurity loses inherited
        // ACE flags on the qualified Windows file path. Every capture/recheck,
        // including strict metadata files, must use this same SE_FILE_OBJECT
        // representation for policy and retained descriptor hashes.
        fn read_file(file: &File) -> Result<Self, NativeDiagnostic> {
            let mut descriptor = null_mut();
            let code = unsafe {
                GetSecurityInfo(
                    raw(file),
                    SE_FILE_OBJECT,
                    OWNER_SECURITY_INFORMATION | DACL_SECURITY_INFORMATION,
                    null_mut(),
                    null_mut(),
                    null_mut(),
                    null_mut(),
                    &mut descriptor,
                )
            };
            if code != 0 {
                if !descriptor.is_null() {
                    unsafe { LocalFree(descriptor) };
                }
                return Err(NativeDiagnostic {
                    stage: "file descriptor query",
                    system_error: Some(code),
                    security: None,
                });
            }
            if descriptor.is_null() {
                return Err(bad("file descriptor absent"));
            }
            let result = (|| {
                if unsafe { IsValidSecurityDescriptor(descriptor) } == 0 {
                    return Err(bad("file descriptor invalid"));
                }
                let mut control = 0u16;
                let mut revision = 0u32;
                if unsafe { GetSecurityDescriptorControl(descriptor, &mut control, &mut revision) }
                    == 0
                    || control & SE_SELF_RELATIVE == 0
                {
                    return Err(bad("file descriptor relative format"));
                }
                let length = unsafe { GetSecurityDescriptorLength(descriptor) } as usize;
                if !(20..=65536).contains(&length) {
                    return Err(bad("file descriptor bounds"));
                }
                let mut bytes = vec![0u32; length.div_ceil(4)];
                unsafe {
                    std::ptr::copy_nonoverlapping(
                        descriptor.cast::<u8>(),
                        bytes.as_mut_ptr().cast::<u8>(),
                        length,
                    )
                };
                Ok(Self { bytes, length })
            })();
            unsafe { LocalFree(descriptor) };
            result
        }
        fn numeric_metadata(&self) -> Result<[u32; 4], NativeDiagnostic> {
            let (_, acl, control) = self.owner_acl()?;
            let aces = self.aces(acl)?;
            Ok([
                control as u32,
                aces.len() as u32,
                aces.iter().fold(0, |v, (_, flags, _, _)| v | *flags as u32),
                aces.iter()
                    .fold(0xff, |v, (_, flags, _, _)| v & *flags as u32),
            ])
        }
        fn descriptor(&self) -> *mut c_void {
            self.bytes.as_ptr().cast_mut().cast()
        }
        fn owner_acl(&self) -> Result<(Vec<u8>, *mut ACL, u16), NativeDiagnostic> {
            let mut owner = null_mut();
            let mut defaulted = 0;
            let mut present = 0;
            let mut acl = null_mut();
            let mut control = 0;
            let mut revision = 0;
            if unsafe { GetSecurityDescriptorOwner(self.descriptor(), &mut owner, &mut defaulted) }
                == 0
                || unsafe {
                    GetSecurityDescriptorDacl(
                        self.descriptor(),
                        &mut present,
                        &mut acl,
                        &mut defaulted,
                    )
                } == 0
                || present == 0
                || acl.is_null()
                || unsafe {
                    GetSecurityDescriptorControl(self.descriptor(), &mut control, &mut revision)
                } == 0
            {
                return Err(bad("descriptor shape"));
            }
            let sid = sid_at(self.bytes.as_ptr().cast(), self.length, owner)?;
            let base = self.bytes.as_ptr() as usize;
            let offset = (acl as usize)
                .checked_sub(base)
                .ok_or_else(|| bad("ACL bounds"))?;
            if offset > self.length || self.length - offset < size_of::<ACL>() {
                return Err(bad("ACL bounds"));
            }
            let size = unsafe { (*acl).AclSize } as usize;
            if size < size_of::<ACL>()
                || size > self.length - offset
                || unsafe { (*acl).AceCount } > 256
            {
                return Err(bad("ACL bounds"));
            }
            Ok((sid, acl, control))
        }
        fn aces(&self, acl: *mut ACL) -> Result<Vec<(u8, u8, u32, Vec<u8>)>, NativeDiagnostic> {
            let mut out = Vec::new();
            for i in 0..unsafe { (*acl).AceCount } as u32 {
                let mut p = null_mut();
                if unsafe { GetAce(acl, i, &mut p) } == 0 || p.is_null() {
                    return Err(bad("ACE read"));
                }
                let base = acl as usize;
                let offset = (p as usize)
                    .checked_sub(base)
                    .ok_or_else(|| bad("ACE bounds"))?;
                let total = unsafe { (*acl).AclSize } as usize;
                if offset > total || total - offset < 8 {
                    return Err(bad("ACE bounds"));
                }
                let ace = unsafe { &*p.cast::<ACCESS_ALLOWED_ACE>() };
                let size = ace.Header.AceSize as usize;
                if size < 16
                    || size > total - offset
                    || ace.Header.AceType > 1
                    || ace.Header.AceFlags & !0x1f != 0
                {
                    return Err(bad("ACE shape"));
                }
                let sid = sid_at(p.cast(), size, unsafe { p.cast::<u8>().add(8) }.cast())?;
                out.push((ace.Header.AceType, ace.Header.AceFlags, ace.Mask, sid));
            }
            Ok(out)
        }
        fn trusted(sid: &[u8], user: &User) -> Result<bool, NativeDiagnostic> {
            if sid == user.sid {
                return Ok(true);
            }
            let text = sid_text(sid)?;
            Ok(matches!(
                text.as_str(),
                "S-1-5-18"
                    | "S-1-5-32-544"
                    | "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"
            ))
        }
        fn integrity(&self, user: &User) -> Result<(), NativeDiagnostic> {
            let (owner, acl, _) = self.owner_acl()?;
            if !Self::trusted(&owner, user)? {
                return Err(bad("ancestor owner policy"));
            }
            for (kind, flags, mask, sid) in self.aces(acl)? {
                if flags & 0x08 != 0 || kind == 1 || Self::trusted(&sid, user)? {
                    continue;
                }
                // Exact unchanged Go directory checkReadObject bad-mask. APPEND
                // is not added; capability ACEs receive no policy exemption.
                let bad_mask = 0x10000
                    | 0x40000
                    | 0x80000
                    | 0x40
                    | 0x2
                    | 0x100
                    | 0x10
                    | 0x10000000
                    | 0x40000000;
                if mask & bad_mask != 0 {
                    return Err(bad("ancestor integrity policy"));
                }
            }
            Ok(())
        }
        fn private(&self, user: &User, directory: bool) -> Result<(), NativeDiagnostic> {
            let (owner, acl, control) = self.owner_acl()?;
            if owner != user.sid || control & SE_DACL_PROTECTED == 0 {
                return Err(bad("private owner/protection"));
            }
            let mut actual = Vec::new();
            for (kind, flags, mask, sid) in self.aces(acl)? {
                if kind != 0 || flags != if directory { 3 } else { 0 } || mask != 0x1f01ff {
                    return Err(bad("private ACE policy"));
                }
                actual.push(sid_text(&sid)?)
            }
            actual.sort();
            let mut expected = vec![user.text.clone(), "S-1-5-18".into(), "S-1-5-32-544".into()];
            expected.sort();
            if actual != expected {
                return Err(bad("private principal set"));
            }
            Ok(())
        }
        fn hash(&self) -> [u8; 32] {
            Sha256::digest(unsafe {
                std::slice::from_raw_parts(self.bytes.as_ptr().cast::<u8>(), self.length)
            })
            .into()
        }
    }
    #[derive(Clone, Copy, PartialEq, Eq)]
    struct Identity {
        volume: u32,
        high: u32,
        low: u32,
        attributes: u32,
        links: u32,
        size: u64,
        write: u64,
    }
    fn identity(file: &File, directory: bool) -> Result<Identity, NativeDiagnostic> {
        let mut info: BY_HANDLE_FILE_INFORMATION = unsafe { zeroed() };
        let mut path = [0u16; 1024];
        let mut flags = 0;
        if unsafe { GetFileInformationByHandle(raw(file), &mut info) } == 0
            || unsafe { GetFileType(raw(file)) } != FILE_TYPE_DISK
            || info.dwFileAttributes & FILE_ATTRIBUTE_REPARSE_POINT != 0
            || (info.dwFileAttributes & FILE_ATTRIBUTE_DIRECTORY != 0) != directory
            || (!directory && info.nNumberOfLinks != 1)
        {
            return Err(bad("object kind/identity"));
        }
        let n = unsafe {
            GetFinalPathNameByHandleW(raw(file), path.as_mut_ptr(), path.len() as u32, 2)
        } as usize;
        if n == 0
            || n >= path.len()
            || !String::from_utf16_lossy(&path[..n]).starts_with(r"\Device\HarddiskVolume")
            || unsafe {
                GetVolumeInformationByHandleW(
                    raw(file),
                    null_mut(),
                    0,
                    null_mut(),
                    null_mut(),
                    &mut flags,
                    null_mut(),
                    0,
                )
            } == 0
            || flags & FILE_PERSISTENT_ACLS == 0
        {
            return Err(bad("local persistent volume"));
        }
        Ok(Identity {
            volume: info.dwVolumeSerialNumber,
            high: info.nFileIndexHigh,
            low: info.nFileIndexLow,
            attributes: info.dwFileAttributes,
            links: info.nNumberOfLinks,
            size: if directory {
                0
            } else {
                (u64::from(info.nFileSizeHigh) << 32) | u64::from(info.nFileSizeLow)
            },
            write: if directory {
                0
            } else {
                (u64::from(info.ftLastWriteTime.dwHighDateTime) << 32)
                    | u64::from(info.ftLastWriteTime.dwLowDateTime)
            },
        })
    }
    struct Held {
        file: CheckedFile,
        id: Identity,
        security: [u8; 32],
        directory: bool,
        private: bool,
    }
    impl Held {
        fn capture(
            file: File,
            user: &User,
            directory: bool,
            private: bool,
        ) -> Result<Self, NativeDiagnostic> {
            let captured = (|| {
                let id = identity(&file, directory)?;
                let sd = Security::read(&file)?;
                if private {
                    sd.private(user, directory)?
                } else {
                    sd.integrity(user)?
                }
                Ok((id, sd.hash()))
            })();
            let (id, security) = match captured {
                Ok(value) => value,
                Err(cause) => {
                    let _ = close_file(file);
                    return Err(cause);
                }
            };
            Ok(Self {
                file: CheckedFile(Some(file)),
                id,
                security,
                directory,
                private,
            })
        }
        fn recheck(&self, user: &User) -> Result<(), NativeDiagnostic> {
            if identity(&self.file, self.directory)? != self.id {
                return Err(bad("retained identity changed"));
            }
            let sd = Security::read(&self.file)?;
            if sd.hash() != self.security {
                return Err(bad("retained descriptor changed"));
            }
            if self.private {
                sd.private(user, self.directory)
            } else {
                sd.integrity(user)
            }
        }
    }
    enum RelativeError {
        Sharing,
        Missing,
        Refused(NativeDiagnostic),
    }
    // MS-FSA sharing checks ignore metadata-only existing opens. TRAVERSE is
    // the directory form of FILE_EXECUTE and participates without listing.
    pub(super) fn retained_directory_access(list: bool) -> u32 {
        FILE_TRAVERSE | if list { FILE_LIST_DIRECTORY } else { 0 }
    }
    fn relative_status(
        parent: &File,
        name: &str,
        directory: bool,
        disposition: u32,
        descriptor: Option<&Descriptor>,
        write: bool,
    ) -> Result<File, RelativeError> {
        relative_status_access(
            parent,
            name,
            directory,
            disposition,
            descriptor,
            write,
            write,
            false,
            false,
        )
    }
    fn relative_status_access(
        parent: &File,
        name: &str,
        directory: bool,
        disposition: u32,
        descriptor: Option<&Descriptor>,
        write: bool,
        remove: bool,
        list: bool,
        broad_share: bool,
    ) -> Result<File, RelativeError> {
        if !native_component(name) {
            return Err(RelativeError::Refused(bad("relative component")));
        }
        let mut text: Vec<u16> = name.encode_utf16().collect();
        let length = u16::try_from(text.len() * 2)
            .map_err(|_| RelativeError::Refused(bad("relative name bound")))?;
        let mut unicode = Unicode {
            length,
            maximum: length,
            buffer: text.as_mut_ptr(),
        };
        let mut oa = Attributes {
            length: size_of::<Attributes>() as u32,
            root: raw(parent),
            name: &mut unicode,
            flags: OBJ_CASE_INSENSITIVE | OBJ_DONT_REPARSE,
            descriptor: descriptor.map_or(null_mut(), |d| d.data),
            qos: null_mut(),
        };
        let mut io = IoStatus {
            status: 0,
            information: 0,
        };
        let mut h = null_mut();
        let access = READ_CONTROL_MASK
            | FILE_READ_ATTRIBUTES
            | SYNCHRONIZE_MASK
            | if directory {
                retained_directory_access(name == "snapshots" || list)
            } else {
                FILE_GENERIC_READ
            }
            | if matches!(name, "config.stage" | "recovery.next") {
                DELETE_MASK
            } else {
                0
            }
            | if write {
                FILE_GENERIC_WRITE | if remove { DELETE_MASK } else { 0 }
            } else {
                0
            };
        let options = NT_OPEN_REPARSE
            | FILE_SYNCHRONOUS_IO_NONALERT
            | if directory {
                FILE_DIRECTORY_FILE
            } else {
                FILE_NON_DIRECTORY_FILE
            };
        let status = unsafe {
            NtCreateFile(
                &mut h,
                access,
                &mut oa,
                &mut io,
                null(),
                0,
                if name == "controller.lock" {
                    0
                } else if broad_share {
                    FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE
                } else {
                    FILE_SHARE_READ | if directory { FILE_SHARE_WRITE } else { 0 }
                },
                disposition,
                options,
                null(),
                0,
            )
        };
        import_owner::relative_output(status, h)
    }
    fn relative_diagnostic(cause: RelativeError) -> NativeDiagnostic {
        match cause {
            RelativeError::Refused(cause) => cause,
            RelativeError::Sharing => NativeDiagnostic {
                stage: "relative sharing",
                system_error: Some(0xc0000043),
                security: None,
            },
            RelativeError::Missing => NativeDiagnostic {
                stage: "relative missing",
                system_error: Some(0xc0000034),
                security: None,
            },
        }
    }
    fn relative(
        parent: &File,
        name: &str,
        directory: bool,
        disposition: u32,
        descriptor: Option<&Descriptor>,
        write: bool,
    ) -> Result<File, NativeDiagnostic> {
        relative_status(parent, name, directory, disposition, descriptor, write).map_err(|cause| {
            match cause {
                RelativeError::Refused(cause) => cause,
                RelativeError::Sharing => NativeDiagnostic {
                    stage: "relative sharing",
                    system_error: Some(0xc0000043),
                    security: None,
                },
                RelativeError::Missing => NativeDiagnostic {
                    stage: "relative missing",
                    system_error: Some(0xc0000034),
                    security: None,
                },
            }
        })
    }
    fn profile_chain(user: &User) -> Result<(PathBuf, Vec<Held>), NativeDiagnostic> {
        let profile = user.profile()?;
        let text = profile.to_str().ok_or_else(|| bad("profile encoding"))?;
        if text.len() < 4
            || text.as_bytes()[1] != b':'
            || text.as_bytes()[2] != b'\\'
            || !text.as_bytes()[0].is_ascii_alphabetic()
        {
            return Err(bad("native local profile shape"));
        }
        let parts: Vec<&str> = text[3..].split('\\').collect();
        if parts.len() > 16 || parts.iter().any(|p| !native_component(p)) {
            return Err(bad("profile components"));
        }
        let root = PathBuf::from(&text[..3]);
        let wide_root = wide(&root)?;
        let file = own(unsafe {
            CreateFileW(
                wide_root.as_ptr(),
                READ_CONTROL_MASK | FILE_READ_ATTRIBUTES | retained_directory_access(false),
                FILE_SHARE_READ | FILE_SHARE_WRITE,
                null(),
                OPEN_EXISTING,
                FILE_FLAG_BACKUP_SEMANTICS | FILE_FLAG_OPEN_REPARSE_POINT,
                null_mut(),
            )
        })?;
        let mut ancestors = vec![Held::capture(file, user, true, false)?];
        for part in parts {
            let file = relative(
                &ancestors.last().ok_or_else(|| bad("profile parent"))?.file,
                part,
                true,
                FILE_OPEN,
                None,
                false,
            )?;
            ancestors.push(Held::capture(file, user, true, false)?)
        }
        Ok((profile, ancestors))
    }
    fn rename(parent: &File, stage: &File, name: &str) -> Result<(), NativeDiagnostic> {
        if !component(name) {
            return Err(bad("publish name"));
        }
        let utf16: Vec<u16> = name.encode_utf16().collect();
        let bytes = 20 + utf16.len() * 2;
        let mut buffer = vec![0usize; bytes.div_ceil(size_of::<usize>())];
        let header = buffer.as_mut_ptr().cast::<Rename>();
        unsafe {
            (*header).replace = 1;
            (*header).root = raw(parent);
            (*header).length = (utf16.len() * 2) as u32;
            std::ptr::copy_nonoverlapping(
                utf16.as_ptr(),
                buffer.as_mut_ptr().cast::<u8>().add(20).cast::<u16>(),
                utf16.len(),
            );
        }
        let mut io = IoStatus {
            status: 0,
            information: 0,
        };
        let status = unsafe {
            NtSetInformationFile(
                raw(stage),
                &mut io,
                buffer.as_mut_ptr().cast(),
                bytes as u32,
                10,
            )
        };
        if status < 0 {
            return Err(NativeDiagnostic {
                stage: "conditional publish",
                system_error: Some(status as u32),
                security: None,
            });
        }
        Ok(())
    }

    #[link(name = "bcrypt")]
    unsafe extern "system" {
        fn BCryptGenRandom(algorithm: HANDLE, buffer: *mut u8, length: u32, flags: u32) -> i32;
    }
    struct CheckedFile(Option<File>);
    impl std::ops::Deref for CheckedFile {
        type Target = File;
        fn deref(&self) -> &File {
            self.0.as_ref().unwrap()
        }
    }
    impl Drop for CheckedFile {
        fn drop(&mut self) {
            if let Some(file) = self.0.take() {
                if RESOURCE_UNKNOWN.load(std::sync::atomic::Ordering::Acquire) {
                    std::mem::forget(file);
                    return;
                }
                let _ = close_file(file);
            }
        }
    }
    trait TakeFile {
        fn take_file(self) -> File;
    }
    impl TakeFile for File {
        fn take_file(self) -> File {
            self
        }
    }
    impl TakeFile for CheckedFile {
        fn take_file(mut self) -> File {
            self.0.take().unwrap()
        }
    }
    struct Data {
        file: CheckedFile,
        id: Identity,
        security: [u8; 32],
        writer: bool,
    }
    impl Data {
        fn capture(file: File, user: &User) -> Result<Self, NativeDiagnostic> {
            Self::capture_class(file, user, false)
        }
        fn capture_writer(file: File, user: &User) -> Result<Self, NativeDiagnostic> {
            Self::capture_class(file, user, true)
        }
        // Reopen suffix of an already attempted native publication. A failed
        // identity/security observation retains the actual FILE_OBJECT instead
        // of dropping it before the owning publication fence can be reported.
        fn capture_published(file: File, user: &User) -> Result<Self, NativeDiagnostic> {
            let observed = (|| {
                let id = identity(&file, false)?;
                let sd = Security::read_file(&file)?;
                sd.private(user, false)?;
                Ok((id, sd.hash()))
            })();
            match observed {
                Ok((id, security)) => Ok(Self {
                    file: CheckedFile(Some(file)),
                    id,
                    security,
                    writer: false,
                }),
                Err(cause) => {
                    retain_unknown();
                    std::mem::forget(file);
                    Err(cause)
                }
            }
        }
        fn capture_class(file: File, user: &User, writer: bool) -> Result<Self, NativeDiagnostic> {
            let captured = (|| {
                let id = identity(&file, false)?;
                let sd = Security::read_file(&file)?;
                if writer {
                    if let Err(mut failure) = data_policy(&sd, user) {
                        failure.security = sd.numeric_metadata().ok();
                        return Err(failure);
                    }
                } else {
                    sd.private(user, false)?
                };
                Ok((id, sd.hash()))
            })();
            let (id, security) = match captured {
                Ok(value) => value,
                Err(cause) => {
                    let _ = close_file(file);
                    return Err(cause);
                }
            };
            Ok(Self {
                file: CheckedFile(Some(file)),
                id,
                security,
                writer,
            })
        }
        fn check(&self, user: &User) -> Result<(), NativeDiagnostic> {
            if identity(&self.file, false)? != self.id {
                return Err(bad("data identity changed"));
            };
            let sd = Security::read_file(&self.file)?;
            if self.writer {
                data_policy(&sd, user)?
            } else {
                sd.private(user, false)?
            };
            if sd.hash() != self.security {
                return Err(bad("data descriptor changed"));
            };
            Ok(())
        }
    }
    fn data_policy(sd: &Security, user: &User) -> Result<(), NativeDiagnostic> {
        let (owner, acl, control) = sd.owner_acl()?;
        if owner != user.sid {
            return Err(bad("data owner"));
        }
        let aces = sd.aces(acl)?;
        let inherited = aces.iter().all(|(_, flags, _, _)| *flags == 0x10);
        if (inherited && control & SE_DACL_PROTECTED != 0)
            || (!inherited && control & SE_DACL_PROTECTED == 0)
        {
            return Err(bad("data DACL protection"));
        }
        let mut actual = Vec::new();
        for (kind, flags, mask, sid) in aces {
            if kind != 0 || mask != 0x1f01ff || flags != if inherited { 0x10 } else { 0 } {
                return Err(bad("data ACE"));
            };
            actual.push(sid_text(&sid)?)
        }
        actual.sort();
        let mut expected = vec![user.text.clone(), "S-1-5-18".into(), "S-1-5-32-544".into()];
        expected.sort();
        if actual != expected {
            return Err(bad("data principals"));
        };
        Ok(())
    }
    fn close_file(file: impl TakeFile) -> Result<(), NativeDiagnostic> {
        let h = file.take_file().into_raw_handle();
        if unsafe { CloseHandle(h) } == 0 {
            retain_unknown();
            return Err(bad("owned handle close unknown"));
        };
        Ok(())
    }
    fn body(file: &File) -> Result<Vec<u8>, NativeDiagnostic> {
        let id = identity(file, false)?;
        if id.size > CONFIG_BUDGET as u64 {
            return Err(bad("config budget"));
        };
        let mut out = Vec::new();
        let mut reader = &*file;
        reader
            .seek(SeekFrom::Start(0))
            .map_err(|_| bad("data rewind"))?;
        reader
            .take(CONFIG_BUDGET as u64 + 1)
            .read_to_end(&mut out)
            .map_err(|_| bad("config read"))?;
        if out.len() > CONFIG_BUDGET {
            return Err(bad("config budget"));
        };
        Ok(out)
    }
    fn known_local(user: &User) -> Result<PathBuf, NativeDiagnostic> {
        let mut p = null_mut();
        if unsafe { SHGetKnownFolderPath(&FOLDERID_LocalAppData, 0, raw(&user.token), &mut p) } < 0
            || p.is_null()
        {
            return Err(bad("native local app data"));
        }
        let mut n = 0;
        while n < 1024 && unsafe { *p.add(n) } != 0 {
            n += 1
        }
        let result = if n < 1024 {
            String::from_utf16(unsafe { std::slice::from_raw_parts(p, n) })
                .map(PathBuf::from)
                .map_err(|_| bad("local app data encoding"))
        } else {
            Err(bad("local app data bounds"))
        };
        unsafe { CoTaskMemFree(p.cast()) };
        result
    }
    // Only a fixed native-home denial probe. Neither bytes nor absence grant
    // import authority, and no arbitrary legacy path is accepted by this API.
    fn legacy_absent(user: &User) -> Result<(), ClientConfigError> {
        let local = known_local(user).map_err(|cause| explain(ErrorKind::Unavailable, cause))?;
        let text = local.to_str().ok_or_else(|| err(ErrorKind::Unavailable))?;
        if text.len() < 4
            || text.as_bytes()[1] != b':'
            || text.as_bytes()[2] != b'\\'
            || !text.as_bytes()[0].is_ascii_alphabetic()
        {
            return Err(err(ErrorKind::Unavailable));
        }
        let parts: Vec<&str> = text[3..].split('\\').collect();
        if parts.len() > 16 || parts.iter().any(|p| !native_component(p)) {
            return Err(err(ErrorKind::Unavailable));
        }
        let volume = wide(Path::new(&text[..3])).map_err(|_| err(ErrorKind::Unavailable))?;
        let file = own(unsafe {
            CreateFileW(
                volume.as_ptr(),
                READ_CONTROL_MASK | FILE_READ_ATTRIBUTES,
                FILE_SHARE_READ | FILE_SHARE_WRITE,
                null(),
                OPEN_EXISTING,
                FILE_FLAG_BACKUP_SEMANTICS | FILE_FLAG_OPEN_REPARSE_POINT,
                null_mut(),
            )
        })
        .map_err(|_| err(ErrorKind::Unavailable))?;
        identity(&file, true).map_err(|_| err(ErrorKind::Unavailable))?;
        let mut held = vec![file];
        // Public ancestry is metadata-only DENIAL evidence, never accepted state,
        // contents, identity/config authority or an old-publisher exclusion proof.
        // Walk every component without following reparses/network redirection.
        for part in parts
            .into_iter()
            .chain(["OrbWinData", "home", ".rdorbwin", "docker"])
        {
            match relative_status(held.last().unwrap(), part, true, FILE_OPEN, None, false) {
                Ok(file) => {
                    identity(&file, true).map_err(|_| err(ErrorKind::Unavailable))?;
                    held.push(file)
                }
                Err(RelativeError::Missing) => return Ok(()),
                Err(_) => return Err(err(ErrorKind::Unavailable)),
            }
        }
        Err(err(ErrorKind::RequiresLegacyReview))
    }
    fn after_retirement_open(parent: &File, writable: bool) -> Result<File, NativeDiagnostic> {
        let deadline = std::time::Instant::now() + std::time::Duration::from_millis(250);
        loop {
            match relative_status_access(
                parent,
                "config.json",
                false,
                FILE_OPEN,
                None,
                writable,
                false,
                false,
                false,
            ) {
                Ok(file) => return Ok(file),
                Err(RelativeError::Sharing) if std::time::Instant::now() < deadline => {
                    std::thread::sleep(std::time::Duration::from_millis(10))
                }
                Err(_) => return Err(bad("writer read-only reopen")),
            }
        }
    }
    fn optional(parent: &File, name: &str) -> Result<Option<File>, NativeDiagnostic> {
        match relative_status(parent, name, false, FILE_OPEN, None, false) {
            Ok(f) => Ok(Some(f)),
            Err(RelativeError::Missing) => Ok(None),
            Err(_) => Err(bad("fixed record open")),
        }
    }
    #[cfg(debug_assertions)]
    fn fresh_proof_parent(parent: &File) {
        let checked = (|| -> Result<(), NativeDiagnostic> {
            let expected = ".orbwin-private";
            let Some(nonce) = expected.strip_prefix(".orbwin-fresh-proof-") else {
                return Err(bad("fresh checkpoint namespace"));
            };
            if nonce.len() != 32
                || !nonce
                    .bytes()
                    .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
            {
                return Err(bad("fresh checkpoint nonce"));
            }
            let user = User::current()?;
            identity(parent, true)?;
            Security::read(parent)?.private(&user, true)?;
            let mut path = [0u16; 1024];
            let n = unsafe {
                GetFinalPathNameByHandleW(raw(parent), path.as_mut_ptr(), path.len() as u32, 2)
            } as usize;
            if n == 0 || n >= path.len() {
                return Err(bad("fresh checkpoint parent"));
            }
            let path =
                String::from_utf16(&path[..n]).map_err(|_| bad("fresh checkpoint encoding"))?;
            if path.split('\\').filter(|part| *part == expected).count() != 1 {
                return Err(bad("fresh checkpoint actual parent"));
            }
            Ok(())
        })();
        if checked.is_err() {
            eprintln!("FRESH_CHECKPOINT_NATIVE_NAMESPACE_REFUSED");
            std::process::exit(125);
        }
    }
    #[cfg(debug_assertions)]
    fn fresh_checkpoint(parent: &File, event: fresh_qualification::Fault) {
        if let Some((code, hold)) = fresh_qualification::checkpoint(event) {
            fresh_proof_parent(parent);
            if event == fresh_qualification::Fault::MarkerRemoved
                && absent(parent, "client.create-intent", false).is_err()
            {
                eprintln!("FRESH_CHECKPOINT_MARKER_ABSENCE_REFUSED");
                std::process::exit(125);
            }
            fresh_qualification::reached(code, hold);
        }
    }
    #[cfg(debug_assertions)]
    fn successor_proof_parent(parent: &File, user: &User) {
        let checked = (|| -> Result<(), NativeDiagnostic> {
            let expected = ".orbwin-private";
            let nonce = expected
                .strip_prefix(".orbwin-successor-proof-")
                .ok_or_else(|| bad("successor checkpoint namespace"))?;
            if nonce.len() != 32
                || !nonce
                    .bytes()
                    .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
            {
                return Err(bad("successor checkpoint nonce"));
            }
            identity(parent, true)?;
            Security::read(parent)?.private(user, true)?;
            let mut path = [0u16; 1024];
            let count = unsafe {
                GetFinalPathNameByHandleW(raw(parent), path.as_mut_ptr(), path.len() as u32, 2)
            } as usize;
            if count == 0 || count >= path.len() {
                return Err(bad("successor checkpoint parent"));
            }
            let path = String::from_utf16(&path[..count])
                .map_err(|_| bad("successor checkpoint encoding"))?;
            let parts: Vec<_> = path.split('\\').collect();
            let indices: Vec<_> = parts
                .iter()
                .enumerate()
                .filter(|(_, part)| **part == expected)
                .map(|(n, _)| n)
                .collect();
            if indices.len() != 1
                || !matches!(&parts[indices[0] + 1..], ["client"] | ["client", "docker"])
            {
                return Err(bad("successor checkpoint native parent"));
            }
            Ok(())
        })();
        if checked.is_err() {
            eprintln!("SUCCESSOR_CHECKPOINT_NATIVE_NAMESPACE_REFUSED");
            std::process::exit(125);
        }
    }
    #[cfg(debug_assertions)]
    fn successor_checkpoint(parent: &File, user: &User, event: successor_qualification::Fault) {
        if successor_qualification::selected(event) {
            successor_proof_parent(parent, user);
            successor_qualification::reached(event);
        }
    }
    #[cfg(debug_assertions)]
    fn successor_lost(parent: &File, user: &User, event: successor_qualification::Fault) -> bool {
        if !successor_qualification::selected(event) {
            return false;
        }
        successor_proof_parent(parent, user);
        successor_qualification::consume(event)
    }
    fn create_data(
        parent: &File,
        name: &str,
        bytes: &[u8],
        user: &User,
    ) -> Result<Data, NativeDiagnostic> {
        if bytes.len() > CONFIG_BUDGET {
            return Err(bad("record budget"));
        }
        let descriptor = Descriptor::private(user, false)?;
        let mut file = relative(parent, name, false, FILE_CREATE, Some(&descriptor), true)?;
        #[cfg(debug_assertions)]
        if let Some((fault, count)) = successor_qualification::selected_prefix(name, bytes) {
            successor_proof_parent(parent, user);
            let cut = (|| {
                identity(&file, false)?;
                Security::read_file(&file)?.private(user, false)?;
                if count >= bytes.len() {
                    return Err(bad("closed prefix bound"));
                }
                file.write_all(&bytes[..count])
                    .map_err(|_| bad("closed prefix write"))?;
                file.sync_all().map_err(|_| bad("closed prefix flush"))?;
                if body(&file)? != bytes[..count] {
                    return Err(bad("closed prefix readback"));
                }
                Ok(())
            })();
            if let Err(cause) = cut {
                let _ = close_file(file);
                return Err(cause);
            }
            successor_qualification::reached(fault);
        }
        #[cfg(debug_assertions)]
        if name == "recovery.next"
            && successor_qualification::selected(successor_qualification::Fault::NextPrefix)
        {
            successor_proof_parent(parent, user);
            let count = bytes.len() / 2;
            let cut = (|| {
                identity(&file, false)?;
                Security::read_file(&file)?.private(user, false)?;
                if count == 0 || count >= bytes.len() {
                    return Err(bad("successor prefix bound"));
                }
                file.write_all(&bytes[..count])
                    .map_err(|_| bad("successor prefix write"))?;
                file.sync_all().map_err(|_| bad("successor prefix flush"))?;
                if body(&file)? != bytes[..count] {
                    return Err(bad("successor prefix readback"));
                }
                Ok(())
            })();
            if let Err(cause) = cut {
                let _ = close_file(file);
                return Err(cause);
            }
            successor_qualification::reached(successor_qualification::Fault::NextPrefix);
        }
        #[cfg(debug_assertions)]
        if let Some(code) = fresh_qualification::prefix(name) {
            fresh_proof_parent(parent);
            let count = if code == 14 {
                0
            } else {
                (bytes.len() / 2).max(1).min(bytes.len() - 1)
            };
            let cut = (|| {
                identity(&file, false)?;
                Security::read_file(&file)?.private(user, false)?;
                file.write_all(&bytes[..count])
                    .map_err(|_| bad("fresh prefix write"))?;
                file.sync_all().map_err(|_| bad("fresh prefix flush"))?;
                if body(&file)? != bytes[..count] {
                    return Err(bad("fresh prefix readback"));
                }
                Ok(())
            })();
            if let Err(cause) = cut {
                let _ = close_file(file);
                return Err(cause);
            }
            fresh_qualification::reached(code, false);
        }
        #[cfg(debug_assertions)]
        if let Some(fault) = qualification_faults::prefix(name) {
            let count = bytes.len() / 2;
            if count == 0 || count >= bytes.len() {
                let _ = close_file(file);
                return Err(bad("fixture prefix budget"));
            }
            let cut = (|| {
                identity(&file, false)?;
                Security::read_file(&file)?.private(user, false)?;
                file.write_all(&bytes[..count])
                    .map_err(|_| bad("fixture prefix write"))?;
                file.sync_all().map_err(|_| bad("fixture prefix flush"))?;
                if body(&file)? != bytes[..count] {
                    return Err(bad("fixture prefix readback"));
                }
                Ok(())
            })();
            if let Err(cause) = cut {
                let _ = close_file(file);
                return Err(cause);
            }
            qualification_faults::exit_after_cut(fault);
        }
        let written = (|| {
            file.write_all(bytes).map_err(|_| bad("record write"))?;
            file.sync_all().map_err(|_| bad("record flush"))?;
            let before = identity(&file, false)?;
            if body(&file)? != bytes {
                return Err(bad("record readback"));
            };
            Ok(before)
        })();
        let before = match written {
            Ok(id) => id,
            Err(e) => {
                let _ = close_file(file);
                return Err(e);
            }
        };
        close_file(file)?;
        let reader = relative(parent, name, false, FILE_OPEN, None, false)?;
        let data = Data::capture(reader, user)?;
        if (
            before.volume,
            before.high,
            before.low,
            before.attributes,
            before.links,
            before.size,
        ) != (
            data.id.volume,
            data.id.high,
            data.id.low,
            data.id.attributes,
            data.id.links,
            data.id.size,
        ) || body(&data.file)? != bytes
        {
            let _ = close_file(data.file);
            return Err(bad("record immutable readback"));
        }
        Ok(data)
    }
    fn erase(parent: &File, name: &str, user: &User) -> Result<(), NativeDiagnostic> {
        let file = relative(parent, name, false, FILE_OPEN, None, true)?;
        let data = Data::capture(file, user)?;
        let disposition = FILE_DISPOSITION_INFO { DeleteFile: true };
        if unsafe {
            SetFileInformationByHandle(
                raw(&data.file),
                FileDispositionInfo,
                (&disposition as *const FILE_DISPOSITION_INFO).cast(),
                size_of::<FILE_DISPOSITION_INFO>() as u32,
            )
        } == 0
        {
            retain_unknown();
            std::mem::forget(data);
            return Err(bad("record disposal unknown"));
        }
        close_file(data.file)?;
        disposed_absent(parent, name, false)
    }
    fn erase_exact(
        parent: &File,
        name: &str,
        user: &User,
        expected: (Identity, [u8; 32]),
    ) -> Result<(), NativeDiagnostic> {
        let file = relative(parent, name, false, FILE_OPEN, None, true)?;
        let data = Data::capture(file, user)?;
        if (data.id, data.security) != expected {
            let _ = close_file(data.file);
            return Err(bad("record cleanup identity changed"));
        }
        let disposition = FILE_DISPOSITION_INFO { DeleteFile: true };
        if unsafe {
            SetFileInformationByHandle(
                raw(&data.file),
                FileDispositionInfo,
                (&disposition as *const FILE_DISPOSITION_INFO).cast(),
                size_of::<FILE_DISPOSITION_INFO>() as u32,
            )
        } == 0
        {
            retain_unknown();
            std::mem::forget(data);
            return Err(bad("record cleanup disposition unknown"));
        }
        #[cfg(debug_assertions)]
        if name == "config.stage"
            && successor_lost(
                parent,
                user,
                successor_qualification::Fault::LostStageDisposition,
            )
        {
            retain_unknown();
            let mut info: FILE_STANDARD_INFO = unsafe { std::mem::zeroed() };
            let observed = unsafe {
                GetFileInformationByHandleEx(
                    raw(&data.file),
                    FileStandardInfo,
                    (&mut info as *mut FILE_STANDARD_INFO).cast(),
                    size_of::<FILE_STANDARD_INFO>() as u32,
                )
            } != 0
                && info.DeletePending;
            std::mem::forget(data);
            if observed {
                println!("ACTUAL_STAGE_DELETE_PENDING");
            }
            return Err(bad("injected lost stage disposition return"));
        }
        close_file(data.file)?;
        disposed_absent(parent, name, false)?;
        #[cfg(debug_assertions)]
        if let Some(fault) = qualification_faults::removed(name) {
            qualification_faults::exit_after_cut(fault);
        }
        Ok(())
    }
    #[derive(PartialEq, Eq)]
    struct FreshWitness {
        controller: Option<(Identity, [u8; 32])>,
        init: Option<(Identity, [u8; 32], Vec<u8>)>,
        docker: Option<(Identity, [u8; 32])>,
        config: Option<(Identity, [u8; 32], Vec<u8>)>,
    }
    #[derive(Default)]
    struct FreshTree {
        controller: Option<(Identity, [u8; 32])>,
        init: Option<Data>,
        docker: Option<Held>,
        config: Option<Data>,
    }
    fn fresh_failure<T>(_: T) -> ClientConfigError {
        err(ErrorKind::RecoveryRequired)
    }
    fn fresh_binding(user: &User, id: Identity) -> Value {
        serde_json::json!({"schema":1,"product":"orbwin","ownerSid":user.text,
            "rootVolume":id.volume,"rootHigh":id.high,"rootLow":id.low,"admission":"fresh"})
    }
    fn fresh_optional(
        parent: &File,
        name: &str,
        directory: bool,
        broad: bool,
    ) -> Result<Option<File>, ClientConfigError> {
        match relative_status_access(
            parent, name, directory, FILE_OPEN, None, false, false, false, broad,
        ) {
            Ok(file) => Ok(Some(file)),
            Err(RelativeError::Missing) => Ok(None),
            Err(_) => Err(err(ErrorKind::RecoveryRequired)),
        }
    }
    fn fresh_record(
        parent: &File,
        name: &str,
        user: &User,
        broad: bool,
    ) -> Result<Option<Data>, ClientConfigError> {
        fresh_optional(parent, name, false, broad)?
            .map(|f| Data::capture(f, user).map_err(fresh_failure))
            .transpose()
    }
    fn fresh_read(data: &Data) -> Result<Vec<u8>, ClientConfigError> {
        if data.id.size > FRESH_RECORD_BUDGET as u64 {
            return Err(err(ErrorKind::RecoveryRequired));
        }
        body(&data.file).map_err(fresh_failure)
    }
    fn fresh_inventory(
        parent: &File,
        name: &str,
        held: &Held,
        user: &User,
        allowed: fn(&str) -> bool,
        budget: usize,
    ) -> Result<Vec<String>, ClientConfigError> {
        // LIST_DIRECTORY is granted only to this temporary read capability.
        let file = relative_status_access(
            parent, name, true, FILE_OPEN, None, false, false, true, true,
        )
        .map_err(fresh_failure)?;
        let listed = Held::capture(file, user, true, true).map_err(fresh_failure)?;
        let result = (|| {
            held.recheck(user).map_err(fresh_failure)?;
            if (listed.id, listed.security) != (held.id, held.security) {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let mut names =
                directory_names(&listed.file, allowed, budget).map_err(fresh_failure)?;
            names.sort();
            if names.windows(2).any(|pair| pair[0] == pair[1]) {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            Ok(names)
        })();
        close_file(listed.file).map_err(fresh_failure)?;
        result
    }
    impl FreshTree {
        fn capture(
            private: &File,
            root: &Held,
            user: &User,
            profile: &Path,
            owner: Option<&Data>,
            broad: bool,
        ) -> Result<Self, ClientConfigError> {
            let root_names = fresh_inventory(
                private,
                "client",
                root,
                user,
                |n| matches!(n, "controller.lock" | "client.init" | "docker"),
                3,
            )?;
            let mut tree = Self::default();
            if let Some(owner) = owner {
                owner.check(user).map_err(fresh_failure)?;
                if !fresh_read(owner)?.is_empty() {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                tree.controller = Some((owner.id, owner.security));
            } else if let Some(controller) =
                fresh_record(&root.file, "controller.lock", user, true)?
            {
                let empty = fresh_read(&controller)?.is_empty();
                tree.controller = Some((controller.id, controller.security));
                close_file(controller.file).map_err(fresh_failure)?;
                if !empty {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
            }
            tree.init = fresh_record(&root.file, "client.init", user, broad)?;
            if let Some(init) = tree.init.as_ref() {
                if !fresh_init_bytes(&fresh_read(init)?, &fresh_binding(user, root.id)) {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
            }
            if let Some(file) = fresh_optional(&root.file, "docker", true, broad)? {
                tree.docker = Some(Held::capture(file, user, true, true).map_err(fresh_failure)?);
                let docker = tree.docker.as_ref().unwrap();
                let docker_names = fresh_inventory(
                    &root.file,
                    "docker",
                    docker,
                    user,
                    |n| n == "config.json",
                    1,
                )?;
                tree.config = fresh_record(&docker.file, "config.json", user, broad)?;
                if docker_names.contains(&"config.json".to_owned()) != tree.config.is_some() {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                if let Some(config) = tree.config.as_ref() {
                    if !fresh_default_bytes(&fresh_read(config)?, profile) {
                        return Err(err(ErrorKind::RecoveryRequired));
                    }
                }
            }
            for (name, present) in [
                ("controller.lock", tree.controller.is_some()),
                ("client.init", tree.init.is_some()),
                ("docker", tree.docker.is_some()),
            ] {
                if root_names.contains(&name.to_owned()) != present {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
            }
            Ok(tree)
        }
        fn witness(&self) -> Result<FreshWitness, ClientConfigError> {
            let record = |data: &Data| Ok((data.id, data.security, fresh_read(data)?));
            Ok(FreshWitness {
                controller: self.controller,
                init: self.init.as_ref().map(record).transpose()?,
                docker: self.docker.as_ref().map(|d| (d.id, d.security)),
                config: self.config.as_ref().map(record).transpose()?,
            })
        }
        fn close(&mut self) -> Result<(), ClientConfigError> {
            for data in [&mut self.config, &mut self.init] {
                if let Some(data) = data.take() {
                    close_file(data.file).map_err(fresh_failure)?;
                }
            }
            if let Some(directory) = self.docker.take() {
                close_file(directory.file).map_err(fresh_failure)?;
            }
            Ok(())
        }
        fn retain(&mut self) {
            if let Some(data) = self.config.take() {
                std::mem::forget(data);
            }
            if let Some(data) = self.init.take() {
                std::mem::forget(data);
            }
            if let Some(data) = self.docker.take() {
                std::mem::forget(data);
            }
        }
    }
    // Only this initializer can own an incomplete client tree. No ordinary
    // lender, writer or native process producer accepts this capability.
    pub(super) struct FreshInitialization {
        user: Option<User>,
        ancestors: Vec<Held>, // authenticated profile, private parent, client
        owner: Option<Data>,
        marker: Option<Data>,
        marker_bytes: Vec<u8>,
        tree: FreshTree,
        witness: FreshWitness,
        profile: PathBuf,
        plugins: PathBuf,
        docker: Option<crate::runtime_cache::CachedExecutable>,
        credentials: Option<crate::runtime_cache::CachedExecutable>,
    }
    impl FreshInitialization {
        fn user(&self) -> &User {
            self.user.as_ref().unwrap()
        }
        fn private(&self) -> &File {
            &self.ancestors[self.ancestors.len() - 2].file
        }
        fn root(&self) -> &Held {
            self.ancestors.last().unwrap()
        }
        fn check(&self) -> Result<(), ClientConfigError> {
            writer_resources_known()?;
            let user = User::current().map_err(fresh_failure)?;
            if user.sid != self.user().sid {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            for held in &self.ancestors {
                held.recheck(&user).map_err(fresh_failure)?;
            }
            for role in [&self.docker, &self.credentials] {
                role.as_ref()
                    .unwrap()
                    .check_held_origin()
                    .map_err(fresh_failure)?;
            }
            let marker = self
                .marker
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            marker.check(&user).map_err(fresh_failure)?;
            if fresh_read(marker)? != self.marker_bytes || !fresh_marker(&self.marker_bytes) {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let mut current = FreshTree::capture(
                self.private(),
                self.root(),
                &user,
                &self.profile,
                self.owner.as_ref(),
                false,
            )?;
            let unchanged = current.witness()? == self.witness;
            current.close()?;
            if !unchanged {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            Ok(())
        }
        fn retain(&mut self) {
            self.tree.retain();
            if let Some(data) = self.marker.take() {
                std::mem::forget(data);
            }
            for held in self.ancestors.drain(..) {
                std::mem::forget(held);
            }
            if let Some(data) = self.owner.take() {
                std::mem::forget(data);
            }
            if let Some(user) = self.user.take() {
                std::mem::forget(user)
            }
            if let Some(role) = self.docker.take() {
                std::mem::forget(role)
            }
            if let Some(role) = self.credentials.take() {
                std::mem::forget(role)
            }
        }
        pub(super) fn release(&mut self) -> Result<(), ClientConfigError> {
            if writer_resources_known().is_err() {
                self.retain();
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let result = (|| {
                self.tree.close()?;
                if let Some(data) = self.marker.take() {
                    close_file(data.file).map_err(fresh_failure)?;
                }
                while let Some(held) = self.ancestors.pop() {
                    close_file(held.file).map_err(fresh_failure)?;
                }
                if let Some(user) = self.user.take() {
                    close_file(user.token).map_err(fresh_failure)?
                }
                if let Some(data) = self.owner.take() {
                    close_file(data.file).map_err(fresh_failure)?;
                }
                self.docker.take();
                self.credentials.take();
                Ok(())
            })();
            if result.is_err() {
                self.retain();
            }
            result
        }
        pub(super) fn apply(mut self: Box<Self>) -> Result<ClientConfigGuard, ClientConfigError> {
            self.check()?;
            legacy_absent(self.user())?;
            // Marker remains the ordinary-admission fence across every deletion,
            // CREATE_NEW and flush/readback below, including a partial marker.
            if self.tree.docker.is_none() {
                let sd = Descriptor::private(self.user(), true).map_err(fresh_failure)?;
                let file = relative(
                    &self.root().file,
                    "docker",
                    true,
                    FILE_CREATE,
                    Some(&sd),
                    false,
                )
                .map_err(fresh_failure)?;
                self.tree.docker =
                    Some(Held::capture(file, self.user(), true, true).map_err(fresh_failure)?);
            }
            let expected_init = fresh_binding(self.user(), self.root().id);
            let init_bytes = serde_json::to_vec(&expected_init).map_err(fresh_failure)?;
            if self
                .tree
                .init
                .as_ref()
                .map(fresh_read)
                .transpose()?
                .as_deref()
                != Some(init_bytes.as_slice())
            {
                if let Some(old) = self.tree.init.take() {
                    let expected = (old.id, old.security);
                    close_file(old.file).map_err(fresh_failure)?;
                    erase_exact(&self.root().file, "client.init", self.user(), expected)
                        .map_err(fresh_failure)?;
                }
                self.tree.init = Some(
                    create_data(&self.root().file, "client.init", &init_bytes, self.user())
                        .map_err(fresh_failure)?,
                );
            }
            let config_bytes =
                serde_json::to_vec(&defaults(&self.plugins)).map_err(fresh_failure)?;
            if self
                .tree
                .config
                .as_ref()
                .map(fresh_read)
                .transpose()?
                .as_deref()
                != Some(config_bytes.as_slice())
            {
                if let Some(old) = self.tree.config.take() {
                    let expected = (old.id, old.security);
                    close_file(old.file).map_err(fresh_failure)?;
                    erase_exact(
                        &self.tree.docker.as_ref().unwrap().file,
                        "config.json",
                        self.user(),
                        expected,
                    )
                    .map_err(fresh_failure)?;
                }
                self.tree.config = Some(
                    create_data(
                        &self.tree.docker.as_ref().unwrap().file,
                        "config.json",
                        &config_bytes,
                        self.user(),
                    )
                    .map_err(fresh_failure)?,
                );
            }
            self.witness = self.tree.witness()?;
            self.check()?;
            if checked_json(&fresh_read(self.tree.init.as_ref().unwrap())?)? != expected_init {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            parse_config(&config_bytes, &self.plugins)?;
            legacy_absent(self.user())?;
            let marker = self.marker.take().unwrap();
            let expected = (marker.id, marker.security);
            close_file(marker.file).map_err(fresh_failure)?;
            writer_resources_known()?;
            erase_exact(
                self.private(),
                "client.create-intent",
                self.user(),
                expected,
            )
            .map_err(fresh_failure)?;
            #[cfg(debug_assertions)]
            {
                fresh_checkpoint(self.private(), fresh_qualification::Fault::MarkerRemoved);
            }
            let mut ancestors = std::mem::take(&mut self.ancestors);
            ancestors.push(self.tree.docker.take().unwrap());
            let path = self
                .profile
                .join(".orbwin-private")
                .join("client")
                .join("docker");
            let owned = Box::new(Owned {
                user: self.user.take().unwrap(),
                ancestors,
                owner: self.owner.take(),
                init: self.tree.init.take(),
                config: self.tree.config.take(),
                intent: None,
                body: config_bytes,
                plugins: self.plugins.clone(),
                docker: self.docker.take().unwrap(),
                credentials: self.credentials.take().unwrap(),
                loaned: false,
                poison: false,
                binding: None,
                writer_base: None,
                writer_original: None,
                writer_prepare: None,
                writer_birth: Default::default(),
                recovery_record: None,
                recovery_next: None,
                completed_birth: false,
                publication_unknown: false,
                recovery_stage: None,
                recovery_stage_hash: None,
                recovery_preserves_source: false,
                local_replace_hash: None,
                local_eof: false,
                partial_prepare: false,
                partial_prepare_hash: None,
                partial_original: false,
                partial_original_hash: None,
                writer_keys: Vec::new(),
                writer_stopped: false,
            });
            if let Err(error) = owned.check() {
                let _ = release_pending(owned);
                return Err(error);
            }
            Ok(ClientConfigGuard {
                inner: Some(owned),
                path,
            })
        }
    }
    impl Drop for FreshInitialization {
        fn drop(&mut self) {
            if std::thread::panicking() {
                retain_unknown();
                self.retain();
                return;
            }
            let _ = self.release();
        }
    }
    pub(super) fn prepare_fresh(
        docker: crate::runtime_cache::CachedExecutable,
        credentials: crate::runtime_cache::CachedExecutable,
    ) -> Result<Option<Box<FreshInitialization>>, ClientConfigError> {
        prepare_fresh_mode(docker, credentials, false)
    }
    pub(super) fn prepare_fresh_mode(
        docker: crate::runtime_cache::CachedExecutable,
        credentials: crate::runtime_cache::CachedExecutable,
        explicit_empty: bool,
    ) -> Result<Option<Box<FreshInitialization>>, ClientConfigError> {
        writer_resources_known()?;
        docker.check_held_origin().map_err(fresh_failure)?;
        credentials.check_held_origin().map_err(fresh_failure)?;
        if docker.path().file_name() != Some(std::ffi::OsStr::new("docker.exe"))
            || credentials.path().file_name()
                != Some(std::ffi::OsStr::new("docker-credential-orbwin.exe"))
        {
            return Err(err(ErrorKind::RecoveryRequired));
        }
        let plugins = docker
            .path()
            .parent()
            .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
            .join("docker-cli-plugins");
        let user = User::current().map_err(fresh_failure)?;
        let (profile, mut ancestors) = profile_chain(&user).map_err(fresh_failure)?;
        let private = match relative_status(
            &ancestors.last().unwrap().file,
            ".orbwin-private",
            true,
            FILE_OPEN,
            None,
            false,
        ) {
            Ok(f) => f,
            Err(RelativeError::Missing) => return Ok(None),
            Err(_) => return Err(err(ErrorKind::RecoveryRequired)),
        };
        ancestors.push(Held::capture(private, &user, true, true).map_err(fresh_failure)?);
        // Before the new bootstrap effect, every actual resource belongs to a
        // strong early graph. It is valid before root/controller construction.
        let mut early = import_owner::FreshPrelude {
            fresh: Some(Box::new(FreshInitialization {
                user: Some(user),
                ancestors,
                owner: None,
                marker: None,
                marker_bytes: Vec::new(),
                tree: FreshTree::default(),
                witness: FreshWitness {
                    controller: None,
                    init: None,
                    docker: None,
                    config: None,
                },
                profile,
                plugins,
                docker: Some(docker),
                credentials: Some(credentials),
            })),
            bootstrap: None,
        };
        let fresh = early.fresh.as_mut().unwrap();
        let private = &fresh.ancestors.last().unwrap().file;
        let Some(marker) = fresh_record(private, "client.create-intent", fresh.user(), true)?
        else {
            return Ok(None);
        };
        fresh.marker_bytes = fresh_read(&marker)?;
        fresh.marker = Some(marker);
        if legacy_import::marker_kind(&fresh.marker_bytes) == legacy_import::MarkerKind::Shared
            && !explicit_empty
        {
            return Err(err(ErrorKind::BootstrapChoiceRequired));
        }
        if !fresh_marker(&fresh.marker_bytes) {
            return Err(err(ErrorKind::RecoveryRequired));
        }
        let marker = fresh.marker.take().unwrap();
        let marker_expected = (marker.id, marker.security);
        close_file(marker.file).map_err(fresh_failure)?;
        early.bootstrap = Some(import_owner::Bootstrap::claim(
            &fresh.ancestors.last().unwrap().file,
            fresh.user(),
            false,
        )?);
        legacy_absent(fresh.user())?;
        let private = &fresh.ancestors.last().unwrap().file;
        let existing = fresh_optional(private, "client", true, true)?;
        let root = existing
            .map(|f| Held::capture(f, fresh.user(), true, true).map_err(fresh_failure))
            .transpose()?;
        let mut tree = if let Some(root) = root.as_ref() {
            FreshTree::capture(private, root, fresh.user(), &fresh.profile, None, true)?
        } else {
            FreshTree::default()
        };
        fresh.witness = tree.witness()?;
        tree.close()?;
        let sd = Descriptor::private(fresh.user(), true).map_err(fresh_failure)?;
        let root = if let Some(root) = root {
            root
        } else {
            Held::capture(
                relative(private, "client", true, FILE_OPEN_IF, Some(&sd), false)
                    .map_err(fresh_failure)?,
                fresh.user(),
                true,
                true,
            )
            .map_err(fresh_failure)?
        };
        fresh.ancestors.push(root);
        let file = relative(
            &fresh.root().file,
            "controller.lock",
            false,
            FILE_OPEN_IF,
            Some(&Descriptor::private(fresh.user(), false).map_err(fresh_failure)?),
            false,
        )
        .map_err(fresh_failure)?;
        fresh.owner = Some(Data::capture(file, fresh.user()).map_err(fresh_failure)?);
        let owner = fresh.owner.as_ref().unwrap();
        let actual = (owner.id, owner.security);
        if fresh.witness.controller.is_some_and(|v| v != actual) {
            return Err(err(ErrorKind::RecoveryRequired));
        };
        fresh.witness.controller = Some(actual);
        let stable = Held::capture(
            relative(fresh.private(), "client", true, FILE_OPEN, None, false)
                .map_err(fresh_failure)?,
            fresh.user(),
            true,
            true,
        )
        .map_err(fresh_failure)?;
        if (stable.id, stable.security) != (fresh.root().id, fresh.root().security) {
            return Err(err(ErrorKind::RecoveryRequired));
        }
        let broad = fresh.ancestors.pop().unwrap();
        fresh.ancestors.push(stable);
        close_file(broad.file).map_err(fresh_failure)?;
        // The actual controller now excludes other cooperative writers; reduce
        // the same FILE_OBJECT before any by-name reopen, with no marker gap.
        fresh.marker = Some(early.bootstrap.take().unwrap().into_fresh(
            fresh.private(),
            fresh.user(),
            marker_expected,
            &fresh.marker_bytes,
        )?);
        fresh.tree = FreshTree::capture(
            fresh.private(),
            fresh.root(),
            fresh.user(),
            &fresh.profile,
            fresh.owner.as_ref(),
            false,
        )?;
        if fresh.tree.witness()? != fresh.witness {
            return Err(err(ErrorKind::RecoveryRequired));
        }
        fresh.check()?;
        Ok(Some(early.ready()))
    }

    struct BoundRestorePlan {
        complete_birth: bool,
        strict: bool,
        profile: PathBuf,
        plan: successor::DataPlan,
        base: RestoreIntent,
        old: Option<successor::Record>,
        expected: Option<successor::V2>,
    }
    pub(super) struct Owned {
        user: User,
        ancestors: Vec<Held>,
        owner: Option<Data>,
        init: Option<Data>,
        config: Option<Data>,
        intent: Option<Data>,
        pub(super) body: Vec<u8>,
        pub(super) plugins: PathBuf,
        docker: crate::runtime_cache::CachedExecutable,
        credentials: crate::runtime_cache::CachedExecutable,
        loaned: bool,
        poison: bool,
        binding: Option<[u8; 32]>,
        writer_base: Option<(Identity, [u8; 32], [u8; 32])>,
        // Exact durable pre-loan bytes; read-only native capability, never child-inherited.
        writer_original: Option<Data>,
        writer_prepare: Option<Data>,
        writer_birth: std::sync::Arc<std::sync::Mutex<Option<Data>>>,
        recovery_record: Option<Data>,
        recovery_next: Option<Data>,
        completed_birth: bool,
        publication_unknown: bool,
        recovery_stage: Option<Data>,
        recovery_stage_hash: Option<[u8; 32]>,
        recovery_preserves_source: bool,
        local_replace_hash: Option<[u8; 32]>,
        local_eof: bool,
        partial_prepare: bool,
        partial_prepare_hash: Option<[u8; 32]>,
        partial_original: bool,
        partial_original_hash: Option<[u8; 32]>,
        writer_keys: Vec<String>,
        writer_stopped: bool,
    }
    pub(super) struct BirthPublisher {
        parent: Held,
        intent: Data,
        original: Data,
        binding: [u8; 32],
        record: std::sync::Arc<std::sync::Mutex<Option<Data>>>,
    }
    impl BirthPublisher {
        pub(super) fn publish(
            self,
            birth: crate::native_client::NativeBirth,
        ) -> Result<(), ClientConfigError> {
            let user = User::current().map_err(|_| err(ErrorKind::RecoveryRequired))?;
            self.parent
                .recheck(&user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            self.intent
                .check(&user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            self.original
                .check(&user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let (pid, created, session) = birth.generation();
            if pid == 0 || created == 0 || session != user.session {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let bytes = serde_json::to_vec(&serde_json::json!({
                "schema":3,"kind":"docker-writer-birth","binding":hex(&self.binding),
                "intentSha256":hex(&Sha256::digest(body(&self.intent.file).map_err(|_|err(ErrorKind::RecoveryRequired))?)),
                "intentBody":String::from_utf8(body(&self.intent.file).map_err(|_|err(ErrorKind::RecoveryRequired))?).map_err(|_|err(ErrorKind::RecoveryRequired))?,
                "rootPid":pid,"rootCreated":created,"rootSession":session
            })).map_err(|_|err(ErrorKind::RecoveryRequired))?;
            let mut slot = self
                .record
                .lock()
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            if slot.is_some() {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            *slot = Some(
                create_data(&self.parent.file, "writer.birth", &bytes, &user)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?,
            );
            drop(slot);
            close_file(self.intent.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
            close_file(self.original.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
            close_file(self.parent.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
            Ok(())
        }
    }
    fn hex(bytes: &[u8]) -> String {
        bytes.iter().map(|b| format!("{b:02x}")).collect()
    }
    fn stopped_generation(birth: &RecoveryBirth) -> Result<(), ClientConfigError> {
        let handle = unsafe {
            OpenProcess(
                PROCESS_QUERY_LIMITED_INFORMATION | SYNCHRONIZE_MASK,
                0,
                birth.root_pid,
            )
        };
        if handle.is_null() {
            // ERROR_INVALID_PARAMETER is the documented nonexistent-process case.
            // Access denial, protected process and all unknown errors refuse review.
            return if unsafe { GetLastError() } == ERROR_INVALID_PARAMETER {
                Ok(())
            } else {
                Err(err(ErrorKind::RecoveryRequired))
            };
        }
        let file = own(handle).map_err(|_| err(ErrorKind::RecoveryRequired))?;
        let result = (|| {
            let (mut created, mut exit, mut kernel, mut user): (
                FILETIME,
                FILETIME,
                FILETIME,
                FILETIME,
            ) = unsafe { zeroed() };
            if unsafe {
                GetProcessTimes(raw(&file), &mut created, &mut exit, &mut kernel, &mut user)
            } == 0
            {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let actual = ((created.dwHighDateTime as u64) << 32) | created.dwLowDateTime as u64;
            if actual != birth.root_created {
                return Ok(());
            }
            let mut session = 0;
            if unsafe { GetProcessId(raw(&file)) } != birth.root_pid
                || unsafe { ProcessIdToSessionId(birth.root_pid, &mut session) } == 0
                || session != birth.root_session
            {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            if unsafe { WaitForSingleObject(raw(&file), 0) } != WAIT_OBJECT_0 {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            Ok(())
        })();
        close_file(file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
        result
    }
    // Checked release changes no disk bytes and grants no health/adoption. Every
    // remaining capability is retained if any CloseHandle result is uncertain.
    pub(super) fn release_pending(mut owned: Box<Owned>) -> Result<(), ClientConfigError> {
        if owned.publication_unknown {
            Box::leak(owned);
            return Err(err(ErrorKind::RecoveryRequired));
        }
        if RESOURCE_UNKNOWN.load(std::sync::atomic::Ordering::Acquire) {
            Box::leak(owned);
            return Err(err(ErrorKind::RecoveryRequired));
        }
        let result = (|| {
            for data in [
                &mut owned.config,
                &mut owned.intent,
                &mut owned.writer_original,
                &mut owned.writer_prepare,
                &mut owned.recovery_record,
                &mut owned.recovery_next,
                &mut owned.recovery_stage,
                &mut owned.init,
            ] {
                if let Some(data) = data.take() {
                    close_file(data.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
                }
            }
            let birth = owned
                .writer_birth
                .lock()
                .map_err(|_| err(ErrorKind::RecoveryRequired))?
                .take();
            if let Some(data) = birth {
                close_file(data.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
            }
            while let Some(held) = owned.ancestors.pop() {
                close_file(held.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
            }
            if let Some(data) = owned.owner.take() {
                close_file(data.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
            }
            Ok(())
        })();
        if result.is_err() {
            Box::leak(owned);
        }
        result
    }
    pub(super) fn release_interrupted(
        owned: Box<Owned>,
        proof: crate::client_job::ExecutionQuiesced,
    ) -> Result<(), ClientConfigError> {
        let Some(binding) = owned.binding else {
            Box::leak(owned);
            return Err(err(ErrorKind::RecoveryRequired));
        };
        let admission = WriterAdmission {
            binding,
            invocation: None,
            birth: None,
        };
        if proof.consume_for_writer(&admission).is_err()
            || owned.origin_check().is_err()
            || owned.durable_original().is_err()
        {
            Box::leak(owned);
            return Err(err(ErrorKind::RecoveryRequired));
        }
        release_pending(owned)
    }
    pub(super) fn release_failed_spawn(
        owned: Box<Owned>,
        proof: crate::native_client::StoppedWriterClient,
    ) -> Result<(), ClientConfigError> {
        let Some(binding) = owned.binding else {
            Box::leak(owned);
            return Err(err(ErrorKind::RecoveryRequired));
        };
        let admission = WriterAdmission {
            binding,
            invocation: None,
            birth: None,
        };
        if proof.consume_for_writer(&admission).is_err()
            || owned.origin_check().is_err()
            || owned.durable_original().is_err()
        {
            Box::leak(owned);
            return Err(err(ErrorKind::RecoveryRequired));
        }
        release_pending(owned)
    }
    impl Owned {
        pub(super) fn writer_stopped(&self) -> bool {
            self.writer_stopped
        }
        pub(super) fn has_recovery_original(&self) -> bool {
            self.writer_original.is_some()
        }
        pub(super) fn recovery_preserves_source(&self) -> bool {
            self.recovery_preserves_source
        }
        pub(super) fn is_completed_birth(&self) -> bool {
            self.completed_birth
        }
        pub(super) fn is_local_replace(&self) -> bool {
            self.local_replace_hash.is_some()
        }
        fn validate_local_replace(&self) -> Result<(), ClientConfigError> {
            self.origin_check()?;
            for name in [
                "writer.original",
                "writer.prepare",
                "writer.birth",
                "recovery.intent",
                "recovery.next",
                "snapshot.intent",
            ] {
                self.checked_absent(self.parent(), name)?;
            }
            let intent = self
                .intent
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            intent
                .check(&self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let bytes = body(&intent.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
            if self.local_replace_hash != Some(Sha256::digest(&bytes).into()) {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let record = if self.local_eof {
                None
            } else {
                Some(parse_local_replace(&bytes)?)
            };
            let current = self
                .config
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            current
                .check(&self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            Security::read_file(&current.file)
                .and_then(|sd| sd.private(&self.user, false))
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            if body(&current.file).map_err(|_| err(ErrorKind::RecoveryRequired))? != self.body {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            if let Some(record) = record.as_ref() {
                local_replace_current(record, &self.body)?;
            } else if !local_eof_prefix(&bytes, &successor::hash(&self.body)) {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let profile = self
                .user
                .profile()
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let refreshed = refresh_managed_field(&self.body, &self.plugins, &profile)?;
            parse_config(refreshed.as_deref().unwrap_or(&self.body), &self.plugins)?;
            if self.local_eof {
                successor::owned_selector(&self.body, &profile)?;
                let private = &self.ancestors[self.ancestors.len() - 3].file;
                for name in ["client.create-intent", "client.import-intent"] {
                    self.checked_absent(private, name)?;
                }
                self.checked_absent(self.directory(), "config.stage")?;
                if self.recovery_stage.is_some() {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
            }
            if let Some(stage) = self.recovery_stage.as_ref() {
                stage
                    .check(&self.user)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                Security::read_file(&stage.file)
                    .and_then(|sd| sd.private(&self.user, false))
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                if self.recovery_stage_hash
                    != Some(
                        Sha256::digest(
                            body(&stage.file).map_err(|_| err(ErrorKind::RecoveryRequired))?,
                        )
                        .into(),
                    )
                {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
            } else if optional(self.directory(), "config.stage")
                .map_err(|_| err(ErrorKind::RecoveryRequired))?
                .is_some()
            {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            Ok(())
        }
        fn validate_partial_prepare(&self) -> Result<(), ClientConfigError> {
            self.origin_check()?;
            if !self.partial_prepare {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            for name in [
                "writer.intent",
                "writer.original",
                "writer.birth",
                "recovery.intent",
                "recovery.next",
                "snapshot.intent",
            ] {
                if optional(self.parent(), name)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?
                    .is_some()
                {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
            }
            if optional(self.directory(), "config.stage")
                .map_err(|_| err(ErrorKind::RecoveryRequired))?
                .is_some()
            {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let prepare = self
                .writer_prepare
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            prepare
                .check(&self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let bytes = body(&prepare.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
            if bytes.len() > 4096
                || !incomplete_record(&bytes)
                || self.partial_prepare_hash != Some(Sha256::digest(&bytes).into())
            {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let config = self
                .config
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            config
                .check(&self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            Security::read_file(&config.file)
                .and_then(|sd| sd.private(&self.user, false))
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            if body(&config.file).map_err(|_| err(ErrorKind::RecoveryRequired))? != self.body {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let profile = self
                .user
                .profile()
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let next = refresh_managed_field(&self.body, &self.plugins, &profile)?;
            parse_config(next.as_deref().unwrap_or(&self.body), &self.plugins)
        }
        fn prepare_local_eof_plan(&self) -> Result<Vec<u8>, ClientConfigError> {
            let profile = self
                .user
                .profile()
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let current = self
                .config
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            let strict = Security::read_file(&current.file)
                .and_then(|sd| sd.private(&self.user, false))
                .is_ok();
            let intent = self
                .intent
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            let bytes = body(&intent.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
            local_eof_plan(&bytes, &self.body, &profile, &self.plugins, strict, || {
                self.validate_local_replace()
            })
        }
        fn recovery_records(&self) -> Result<RecoveryIntent, ClientConfigError> {
            self.origin_check()?;
            let birth = self
                .writer_birth
                .lock()
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let born = if let Some(birth) = birth.as_ref() {
                birth
                    .check(&self.user)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                let bytes = body(&birth.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
                parse_birth_record(&bytes)?
            } else {
                None
            };
            let restore_record = if let Some(record) = self.recovery_record.as_ref() {
                record
                    .check(&self.user)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                let bytes = self.recovery_bytes(record)?;
                if bytes.len() > successor::BUDGET {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                if successor::incomplete(&bytes) {
                    None
                } else {
                    Some(successor::parse(&bytes)?)
                }
            } else {
                None
            };
            let prepare = if let Some(record) = self.writer_prepare.as_ref() {
                record
                    .check(&self.user)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                let bytes = body(&record.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
                if bytes.len() > 4096 {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                let value: RecoveryPrepare =
                    serde_json::from_slice(&bytes).map_err(|_| err(ErrorKind::RecoveryRequired))?;
                if value.schema != 3 || value.kind != "docker-writer-prepare" {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                Some(value)
            } else {
                None
            };
            let actual_intent = if let Some(intent) = self.intent.as_ref() {
                intent
                    .check(&self.user)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                Some(body(&intent.file).map_err(|_| err(ErrorKind::RecoveryRequired))?)
            } else {
                None
            };
            let incomplete_intent = actual_intent
                .as_ref()
                .is_some_and(|bytes| incomplete_record(bytes));
            if incomplete_intent && (born.is_some() || prepare.is_none()) {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let bytes = if let Some(bytes) = actual_intent.as_ref().filter(|_| !incomplete_intent) {
                bytes.clone()
            } else if let Some(born) = born.as_ref() {
                born.intent_body.as_bytes().to_vec()
            } else if let Some(record) = restore_record.as_ref() {
                record.base.intent_body.as_bytes().to_vec()
            } else if let Some(prepare) = prepare.as_ref() {
                let original = self.writer_original.as_ref();
                serde_json::to_vec(&serde_json::json!({"schema":3,"kind":"docker-writer","mutation":prepare.mutation,
                    "binding":prepare.binding,"keysSha256":prepare.keys_sha256,"baseSha256":prepare.base_sha256,
                    "baseVolume":prepare.base_volume,"baseHigh":prepare.base_high,"baseLow":prepare.base_low,"baseSdSha256":prepare.base_sd_sha256,
                    "originalVolume":original.map_or(0,|v|v.id.volume),"originalHigh":original.map_or(0,|v|v.id.high),"originalLow":original.map_or(0,|v|v.id.low),
                    "originalSdSha256":original.map_or_else(||prepare.base_sd_sha256.clone(),|v|hex(&v.security)),"originalSha256":prepare.base_sha256
                })).map_err(|_|err(ErrorKind::RecoveryRequired))?
            } else {
                return Err(err(ErrorKind::RecoveryRequired));
            };
            let value = parse_recovery_intent(&bytes)?;
            if let Some(prepare) = prepare.as_ref() {
                if prepare.mutation != value.mutation
                    || prepare.binding != value.binding
                    || prepare.keys_sha256 != value.keys_sha256
                    || prepare.base_sha256 != value.base_sha256
                    || prepare.base_sd_sha256 != value.base_sd_sha256
                    || (prepare.base_volume, prepare.base_high, prepare.base_low)
                        != (value.base_volume, value.base_high, value.base_low)
                {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
            }
            if let Some(saved) = self.writer_original.as_ref() {
                saved
                    .check(&self.user)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                let original = body(&saved.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
                if (saved.id.volume, saved.id.high, saved.id.low)
                    != (
                        value.original_volume,
                        value.original_high,
                        value.original_low,
                    )
                    || saved.security != digest_hex(&value.original_sd_sha256)?
                {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                if <[u8; 32]>::from(Sha256::digest(&original))
                    != digest_hex(&value.original_sha256)?
                {
                    let current = self
                        .config
                        .as_ref()
                        .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
                    if born.is_some()
                        || (actual_intent.is_some() && !incomplete_intent)
                        || prepare.is_none()
                        || (current.id.volume, current.id.high, current.id.low)
                            != (value.base_volume, value.base_high, value.base_low)
                        || current.security != digest_hex(&value.base_sd_sha256)?
                        || <[u8; 32]>::from(Sha256::digest(&self.body))
                            != digest_hex(&value.base_sha256)?
                        || original.len() >= self.body.len()
                        || !self.body.starts_with(&original)
                    {
                        return Err(err(ErrorKind::RecoveryRequired));
                    }
                    Security::read_file(&current.file)
                        .and_then(|sd| sd.private(&self.user, false))
                        .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                }
            } else if self.intent.is_some()
                || (born.is_none() && prepare.is_none() && restore_record.is_none())
            {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let config = self
                .config
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            config
                .check(&self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            if body(&config.file).map_err(|_| err(ErrorKind::RecoveryRequired))? != self.body {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            if let Some(record) = restore_record.as_ref() {
                let restore = &record.base;
                if restore.binding != value.binding || restore.intent_body.as_bytes() != bytes {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                digest_hex(&restore.current_sd_sha256)?;
                digest_hex(&restore.current_sha256)?;
                digest_hex(&restore.restore_sha256)?;
                let hash = <[u8; 32]>::from(Sha256::digest(&self.body));
                let prior = (config.id.volume, config.id.high, config.id.low)
                    == (
                        restore.current_volume,
                        restore.current_high,
                        restore.current_low,
                    )
                    && config.security == digest_hex(&restore.current_sd_sha256)?
                    && hash == digest_hex(&restore.current_sha256)?;
                if !prior || self.writer_original.is_none() {
                    Security::read_file(&config.file)
                        .and_then(|sd| sd.private(&self.user, false))
                        .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                    let unchanged_source = prior
                        && born.is_none()
                        && prepare.is_some()
                        && (config.id.volume, config.id.high, config.id.low)
                            == (value.base_volume, value.base_high, value.base_low)
                        && config.security == digest_hex(&value.base_sd_sha256)?
                        && hash == digest_hex(&value.base_sha256)?;
                    if !unchanged_source
                        && !successor::published(
                            restore,
                            record.v2.as_ref(),
                            prior,
                            true,
                            &hex(&hash),
                        )
                    {
                        return Err(err(ErrorKind::RecoveryRequired));
                    }
                }
            }
            if let Some(stage) = self.recovery_stage.as_ref() {
                stage
                    .check(&self.user)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                if self.recovery_stage_hash
                    != Some(
                        Sha256::digest(
                            body(&stage.file).map_err(|_| err(ErrorKind::RecoveryRequired))?,
                        )
                        .into(),
                    )
                {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
            }
            if let Some(born) = born.as_ref() {
                validate_recovery_birth(born, &bytes, &value.binding)?;
                stopped_generation(born)?;
                if self.writer_original.is_none() {
                    // Birth is removed last. Missing original+intent can only
                    // finalize an owner-authored (three-ACE) committed config.
                    Security::read_file(&config.file)
                        .and_then(|sd| sd.private(&self.user, false))
                        .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                    let profile = self
                        .user
                        .profile()
                        .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                    successor::owned_selector(&self.body, &profile)?;
                }
            } else {
                // Only schema 3 promises that absence precedes first user resume.
                // Require the original native config as an independent birth cutpoint.
                if restore_record.is_none()
                    && ((config.id.volume, config.id.high, config.id.low)
                        != (value.base_volume, value.base_high, value.base_low)
                        || config.security != digest_hex(&value.base_sd_sha256)?
                        || <[u8; 32]>::from(Sha256::digest(&self.body))
                            != digest_hex(&value.base_sha256)?)
                {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
            }
            Ok(value)
        }
        fn recovery_bytes(&self, data: &Data) -> Result<Vec<u8>, ClientConfigError> {
            data.check(&self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            if data.id.size > successor::BUDGET as u64 {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            body(&data.file).map_err(|_| err(ErrorKind::RecoveryRequired))
        }
        fn prepare_restore_plan(&self) -> Result<BoundRestorePlan, ClientConfigError> {
            let intent = self.recovery_records()?;
            let born = self
                .writer_birth
                .lock()
                .map_err(|_| err(ErrorKind::RecoveryRequired))?
                .as_ref()
                .map(|data| {
                    body(&data.file)
                        .map_err(|_| err(ErrorKind::RecoveryRequired))
                        .and_then(|bytes| parse_birth_record(&bytes))
                })
                .transpose()?
                .flatten();
            let complete_birth = born.is_some();
            let actual_intent = self
                .intent
                .as_ref()
                .map(|data| body(&data.file).map_err(|_| err(ErrorKind::RecoveryRequired)))
                .transpose()?;
            let pre_intent = !complete_birth
                && self.writer_prepare.is_some()
                && actual_intent
                    .as_ref()
                    .is_none_or(|bytes| incomplete_record(bytes));
            let from_original = self.writer_original.is_some() || pre_intent;
            let baseline = if self.writer_original.is_some() && !self.partial_original {
                self.durable_original()?
            } else {
                self.body.clone()
            };
            let profile = self
                .user
                .profile()
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let old_bytes = self
                .recovery_record
                .as_ref()
                .map(|d| self.recovery_bytes(d))
                .transpose()?;
            let old_record = old_bytes
                .as_ref()
                .filter(|bytes| !successor::incomplete(bytes))
                .map(|bytes| successor::parse(bytes))
                .transpose()?;
            let current = self
                .config
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            let strict = Security::read_file(&current.file)
                .and_then(|sd| sd.private(&self.user, false))
                .is_ok();
            let prior = old_record
                .as_ref()
                .is_some_and(|record| self.current_matches_restore(&record.base));
            let plan = successor::data_plan(
                &baseline,
                &self.body,
                &profile,
                &self.plugins,
                from_original,
                old_record.as_ref(),
                prior,
                strict,
                || self.recovery_records().map(|_| ()),
            )?;
            let intent_body = if let Some(bytes) =
                actual_intent.as_ref().filter(|b| !incomplete_record(b))
            {
                String::from_utf8(bytes.clone()).map_err(|_| err(ErrorKind::RecoveryRequired))?
            } else if let Some(birth) = born.as_ref() {
                birth.intent_body.clone()
            } else if let Some(old) = old_record.as_ref() {
                old.base.intent_body.clone()
            } else {
                String::from_utf8(go_json(&intent)?)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?
            };
            let current = self.config.as_ref().unwrap();
            let base = RestoreIntent {
                schema: 2,
                kind: "restore-original".into(),
                binding: intent.binding.clone(),
                current_volume: current.id.volume,
                current_high: current.id.high,
                current_low: current.id.low,
                current_sd_sha256: hex(&current.security),
                current_sha256: successor::hash(&self.body),
                restore_sha256: successor::hash(&plan.target),
                intent_body,
            };
            let expected = old_record
                .as_ref()
                .map(|old| {
                    self.successor_witness(old, old_bytes.as_ref().unwrap(), &base, !from_original)
                })
                .transpose()?;
            if let Some(next) = self.recovery_next.as_ref() {
                let expected = expected
                    .as_ref()
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
                successor::validate_next(
                    &self.recovery_bytes(next)?,
                    expected,
                    &plan.baseline,
                    &profile,
                )?;
            }
            Ok(BoundRestorePlan {
                complete_birth,
                strict,
                profile,
                plan,
                base,
                old: old_record,
                expected,
            })
        }
        fn current_matches_restore(&self, record: &RestoreIntent) -> bool {
            self.config.as_ref().is_some_and(|d| {
                (d.id.volume, d.id.high, d.id.low)
                    == (
                        record.current_volume,
                        record.current_high,
                        record.current_low,
                    )
                    && hex(&d.security) == record.current_sd_sha256
                    && successor::hash(&self.body) == record.current_sha256
            })
        }
        fn checked_absent(&self, parent: &File, name: &str) -> Result<(), ClientConfigError> {
            match relative_status(parent, name, false, FILE_OPEN, None, false) {
                Err(RelativeError::Missing) => Ok(()),
                Ok(file) => {
                    close_file(file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
                    Err(err(ErrorKind::RecoveryRequired))
                }
                Err(_) => Err(err(ErrorKind::RecoveryRequired)),
            }
        }
        fn recovery_fingerprint(
            &self,
        ) -> Result<Vec<(Identity, [u8; 32], Vec<u8>)>, ClientConfigError> {
            self.origin_check()?;
            let mut values = Vec::new();
            for (data, parent, name) in [
                (&self.config, self.directory(), "config.json"),
                (&self.intent, self.parent(), "writer.intent"),
                (&self.writer_original, self.parent(), "writer.original"),
                (&self.writer_prepare, self.parent(), "writer.prepare"),
                (&self.recovery_record, self.parent(), "recovery.intent"),
                (&self.recovery_stage, self.directory(), "config.stage"),
            ] {
                if let Some(d) = data {
                    d.check(&self.user)
                        .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                    values.push((
                        d.id,
                        d.security,
                        body(&d.file).map_err(|_| err(ErrorKind::RecoveryRequired))?,
                    ));
                } else {
                    self.checked_absent(parent, name)?;
                }
            }
            let birth = self
                .writer_birth
                .lock()
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            if let Some(d) = birth.as_ref() {
                d.check(&self.user)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                values.push((
                    d.id,
                    d.security,
                    body(&d.file).map_err(|_| err(ErrorKind::RecoveryRequired))?,
                ));
            } else {
                self.checked_absent(self.parent(), "writer.birth")?;
            }
            self.checked_absent(self.parent(), "snapshot.intent")?;
            Ok(values)
        }
        fn successor_witness(
            &self,
            old: &successor::Record,
            old_bytes: &[u8],
            base: &RestoreIntent,
            published: bool,
        ) -> Result<successor::V2, ClientConfigError> {
            let record = self
                .recovery_record
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            record
                .check(&self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let mut next = successor::V2::from_base(
                base,
                self.plugins
                    .to_str()
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                    .into(),
            );
            next.previous_volume = record.id.volume;
            next.previous_high = record.id.high;
            next.previous_low = record.id.low;
            next.previous_sd = hex(&record.security);
            next.previous_sha = successor::hash(old_bytes);
            next.previous_restore_sha = old.base.restore_sha256.clone();
            if published {
                next.published_baseline_sha = successor::hash(&self.body);
            }
            if let Some(stage) = self.recovery_stage.as_ref() {
                stage
                    .check(&self.user)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                next.stage_present = true;
                next.stage_volume = stage.id.volume;
                next.stage_high = stage.id.high;
                next.stage_low = stage.id.low;
                next.stage_sd = hex(&stage.security);
                next.stage_sha = successor::hash(
                    &body(&stage.file).map_err(|_| err(ErrorKind::RecoveryRequired))?,
                );
            }
            Ok(next)
        }
        fn ensure_successor(
            &mut self,
            encoded: &[u8],
            expected: &successor::V2,
            baseline: &[u8],
            profile: &Path,
        ) -> Result<(), ClientConfigError> {
            if let Some(next) = self.recovery_next.as_ref() {
                next.check(&self.user)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                let bytes = self.recovery_bytes(next)?;
                let partial = successor::validate_next(&bytes, expected, baseline, profile)?;
                if partial || bytes != encoded {
                    let next = self.recovery_next.take().unwrap();
                    let id = (next.id, next.security);
                    close_file(next.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
                    erase_exact(self.parent(), "recovery.next", &self.user, id)
                        .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                }
            }
            if self.recovery_next.is_none() {
                self.recovery_next = Some(
                    create_data(self.parent(), "recovery.next", encoded, &self.user)
                        .map_err(|_| err(ErrorKind::RecoveryRequired))?,
                );
            }
            #[cfg(debug_assertions)]
            successor_checkpoint(
                self.parent(),
                &self.user,
                successor_qualification::Fault::NextDurable,
            );
            Ok(())
        }
        fn publish_successor(&mut self, encoded: &[u8]) -> Result<(), ClientConfigError> {
            self.origin_check()?;
            let old = self
                .recovery_record
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            let next = self
                .recovery_next
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            old.check(&self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            next.check(&self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let expected = (next.id, next.security);
            if self.recovery_bytes(next)? != encoded {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            // Cooperative fixed-name replacement under this actual exclusive
            // controller. NT rename is not a destination-ID compare-and-swap.
            #[cfg(debug_assertions)]
            successor_checkpoint(
                self.parent(),
                &self.user,
                successor_qualification::Fault::NextBeforeRename,
            );
            self.publication_unknown = true;
            let old = self.recovery_record.take().unwrap();
            if close_file(old.file).is_err() {
                self.publication_unknown = true;
                return Err(err(ErrorKind::RecoveryRequired));
            }
            if rename(
                self.parent(),
                &self.recovery_next.as_ref().unwrap().file,
                "recovery.intent",
            )
            .is_err()
            {
                self.publication_unknown = true;
                return Err(err(ErrorKind::RecoveryRequired));
            }
            #[cfg(debug_assertions)]
            if successor_lost(
                self.parent(),
                &self.user,
                successor_qualification::Fault::LostMetadataRename,
            ) {
                retain_unknown();
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let next = self.recovery_next.take().unwrap();
            if close_file(next.file).is_err() {
                self.publication_unknown = true;
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let verified = (|| {
                let d = Data::capture_published(
                    relative(
                        self.parent(),
                        "recovery.intent",
                        false,
                        FILE_OPEN,
                        None,
                        false,
                    )
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?,
                    &self.user,
                )
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                self.recovery_record = Some(d);
                let d = self.recovery_record.as_ref().unwrap();
                let same = (d.id, d.security) == expected && self.recovery_bytes(d)? == encoded;
                if !same {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                disposed_absent(self.parent(), "recovery.next", false)
                    .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?;
                self.retained_origin_check()?;
                Ok(())
            })();
            if verified.is_ok() {
                self.publication_unknown = false;
                #[cfg(debug_assertions)]
                successor_checkpoint(
                    self.parent(),
                    &self.user,
                    successor_qualification::Fault::NextPublished,
                );
            }
            verified
        }
        fn validate_completed_birth(&self) -> Result<(), ClientConfigError> {
            self.recovery_records()?;
            for name in [
                "writer.intent",
                "writer.original",
                "recovery.intent",
                "recovery.next",
                "snapshot.intent",
            ] {
                self.checked_absent(self.parent(), name)?;
            }
            self.checked_absent(self.directory(), "config.stage")?;
            let birth = self
                .writer_birth
                .lock()
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let d = birth
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            if parse_birth_record(&body(&d.file).map_err(|_| err(ErrorKind::RecoveryRequired))?)?
                .is_none()
            {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let current = self
                .config
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            Security::read_file(&current.file)
                .and_then(|sd| sd.private(&self.user, false))
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            successor::owned_selector(
                &self.body,
                &self
                    .user
                    .profile()
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?,
            )?;
            Ok(())
        }
        fn finish_completed_birth(&mut self) -> Result<(), ClientConfigError> {
            let profile = self
                .user
                .profile()
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let target = successor::target(&self.body, &self.plugins, &profile)?;
            successor::completed(|step| match step {
                successor::CompletedStep::Check => self.validate_completed_birth(),
                successor::CompletedStep::Cleanup => {
                    self.clear_writer_prepare()?;
                    self.clear_writer_birth()?;
                    self.completed_birth = false;
                    self.loaned = false;
                    self.poison = false;
                    Ok(())
                }
                successor::CompletedStep::Refresh => {
                    if target != self.body {
                        self.replace_config(&target)?;
                    }
                    self.check()
                }
            })
        }
        fn load_recovery(&mut self) -> Result<(), ClientConfigError> {
            self.completed_birth = false;
            self.partial_prepare = false;
            self.partial_original = false;
            self.partial_original_hash = None;
            self.recovery_preserves_source = false;
            self.intent = optional(self.parent(), "writer.intent")
                .map_err(|_| err(ErrorKind::RecoveryRequired))?
                .map(|f| Data::capture(f, &self.user))
                .transpose()
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            self.writer_original = optional(self.parent(), "writer.original")
                .map_err(|_| err(ErrorKind::RecoveryRequired))?
                .map(|f| Data::capture(f, &self.user))
                .transpose()
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            self.writer_prepare = optional(self.parent(), "writer.prepare")
                .map_err(|_| err(ErrorKind::RecoveryRequired))?
                .map(|f| Data::capture(f, &self.user))
                .transpose()
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            if let Some(file) = optional(self.parent(), "writer.birth")
                .map_err(|_| err(ErrorKind::RecoveryRequired))?
            {
                *self
                    .writer_birth
                    .lock()
                    .map_err(|_| err(ErrorKind::RecoveryRequired))? = Some(
                    Data::capture(file, &self.user)
                        .map_err(|_| err(ErrorKind::RecoveryRequired))?,
                );
            }
            self.recovery_record = optional(self.parent(), "recovery.intent")
                .map_err(|_| err(ErrorKind::RecoveryRequired))?
                .map(|f| Data::capture(f, &self.user))
                .transpose()
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            self.recovery_next = optional(self.parent(), "recovery.next")
                .map_err(|_| err(ErrorKind::RecoveryRequired))?
                .map(|f| Data::capture(f, &self.user))
                .transpose()
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            if self.recovery_next.is_some() {
                let old = self
                    .recovery_record
                    .as_ref()
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
                successor::parse(&self.recovery_bytes(old)?)?;
            }
            self.recovery_stage = optional(self.directory(), "config.stage")
                .map_err(|_| err(ErrorKind::RecoveryRequired))?
                .map(|f| Data::capture(f, &self.user))
                .transpose()
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            if let Some(stage) = self.recovery_stage.as_ref() {
                self.recovery_stage_hash = Some(
                    Sha256::digest(
                        body(&stage.file).map_err(|_| err(ErrorKind::RecoveryRequired))?,
                    )
                    .into(),
                );
            }
            let intent_bytes = self
                .intent
                .as_ref()
                .map(|d| body(&d.file).map_err(|_| err(ErrorKind::RecoveryRequired)))
                .transpose()?;
            let siblings = RecoverySiblings {
                prepare: self.writer_prepare.is_some(),
                original: self.writer_original.is_some(),
                birth: self
                    .writer_birth
                    .lock()
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?
                    .is_some(),
                recovery: self.recovery_record.is_some(),
                next: self.recovery_next.is_some(),
            };
            match recovery_route(
                intent_bytes.as_deref(),
                siblings,
                &successor::hash(&self.body),
            )? {
                RecoveryRoute::Writer => {}
                route => {
                    self.local_eof = route == RecoveryRoute::LocalEof;
                    self.local_replace_hash =
                        Some(Sha256::digest(intent_bytes.as_ref().unwrap()).into());
                    if self.local_eof {
                        self.prepare_local_eof_plan()?;
                    } else {
                        self.validate_local_replace()?;
                    }
                    self.loaned = true;
                    self.poison = true;
                    return Ok(());
                }
            }
            if let Some(prepare) = self.writer_prepare.as_ref() {
                let bytes = body(&prepare.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
                if incomplete_record(&bytes) {
                    self.partial_prepare = true;
                    self.partial_prepare_hash = Some(Sha256::digest(&bytes).into());
                    self.validate_partial_prepare()?;
                    self.recovery_preserves_source = true;
                    self.loaned = true;
                    self.poison = true;
                    return Ok(());
                }
            }
            if self.intent.is_none()
                && self.writer_original.is_none()
                && self.recovery_record.is_none()
                && self.recovery_next.is_none()
                && self.recovery_stage.is_none()
            {
                let complete = self
                    .writer_birth
                    .lock()
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?
                    .as_ref()
                    .map(|d| {
                        body(&d.file)
                            .map_err(|_| err(ErrorKind::RecoveryRequired))
                            .and_then(|b| parse_birth_record(&b))
                    })
                    .transpose()?
                    .flatten()
                    .is_some();
                if complete {
                    self.validate_completed_birth()?;
                    self.completed_birth = true;
                    self.loaned = true;
                    self.poison = true;
                    return Ok(());
                }
            }
            let value = self.recovery_records()?;
            if self.writer_original.is_none() {
                let current = self.config.as_ref().unwrap();
                self.recovery_preserves_source =
                    (current.id.volume, current.id.high, current.id.low)
                        == (value.base_volume, value.base_high, value.base_low)
                        && current.security == digest_hex(&value.base_sd_sha256)?
                        && <[u8; 32]>::from(Sha256::digest(&self.body))
                            == digest_hex(&value.base_sha256)?;
            }
            self.binding = Some(digest_hex(&value.binding)?);
            if let Some(saved) = self.writer_original.as_ref() {
                let hash = <[u8; 32]>::from(Sha256::digest(
                    body(&saved.file).map_err(|_| err(ErrorKind::RecoveryRequired))?,
                ));
                if hash != digest_hex(&value.original_sha256)? {
                    self.partial_original = true;
                    self.partial_original_hash = Some(hash);
                    self.recovery_preserves_source = true;
                }
                self.writer_base = Some((
                    saved.id,
                    saved.security,
                    digest_hex(&value.original_sha256)?,
                ));
            }
            self.prepare_restore_plan()?;
            self.loaned = true;
            self.poison = true;
            Ok(())
        }
        pub(super) fn restore_reviewed(&mut self) -> Result<(), ClientConfigError> {
            if self.completed_birth {
                return self.finish_completed_birth();
            }
            if self.local_eof {
                let target = self.prepare_local_eof_plan()?;
                return successor::completed(|step| match step {
                    successor::CompletedStep::Check => {
                        if self.prepare_local_eof_plan()? != target {
                            return Err(err(ErrorKind::RecoveryRequired));
                        }
                        Ok(())
                    }
                    successor::CompletedStep::Cleanup => {
                        self.clear_writer_intent()?;
                        self.local_replace_hash = None;
                        self.local_eof = false;
                        self.loaned = false;
                        self.poison = false;
                        Ok(())
                    }
                    successor::CompletedStep::Refresh => {
                        if target != self.body {
                            self.replace_config(&target)?;
                        }
                        let current = self
                            .config
                            .as_ref()
                            .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
                        Security::read_file(&current.file)
                            .and_then(|sd| sd.private(&self.user, false))
                            .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                        if body(&current.file).map_err(|_| err(ErrorKind::RecoveryRequired))?
                            != target
                        {
                            return Err(err(ErrorKind::RecoveryRequired));
                        }
                        disposed_absent(self.directory(), "config.stage", false)
                            .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?;
                        parse_config(&target, &self.plugins)?;
                        self.check()
                    }
                });
            }
            if self.is_local_replace() {
                self.validate_local_replace()?;
                let profile = self
                    .user
                    .profile()
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                let refresh = refresh_managed_field(&self.body, &self.plugins, &profile)?;
                if let Some(stage) = self.recovery_stage.take() {
                    let expected = (stage.id, stage.security);
                    close_file(stage.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
                    erase_exact(self.directory(), "config.stage", &self.user, expected)
                        .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                }
                self.validate_local_replace()?;
                self.clear_writer_intent()?;
                self.local_replace_hash = None;
                self.local_eof = false;
                self.loaned = false;
                self.poison = false;
                // A fresh ordinary transaction journals any current-selector
                // update while the same exclusive controller remains held.
                if let Some(next) = refresh {
                    self.replace_config(&next)?;
                }
                let current = self
                    .config
                    .as_ref()
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
                Security::read_file(&current.file)
                    .and_then(|sd| sd.private(&self.user, false))
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                if body(&current.file).map_err(|_| err(ErrorKind::RecoveryRequired))? != self.body {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                parse_config(&self.body, &self.plugins)?;
                return self.check();
            }
            if self.partial_prepare {
                self.validate_partial_prepare()?;
                let profile = self
                    .user
                    .profile()
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                let refresh = refresh_managed_field(&self.body, &self.plugins, &profile)?;
                self.clear_writer_prepare()?;
                self.partial_prepare = false;
                self.partial_prepare_hash = None;
                self.loaned = false;
                self.poison = false;
                if refresh.is_none() {
                    return self.check();
                }
                // Replace the interrupted prepare from retained native source
                // metadata before any selector mutation. This admission never
                // escapes to a process producer; no native client is created.
                let config_path = profile
                    .join(".orbwin-private")
                    .join("client")
                    .join("docker");
                let admission = self.begin_writer(Vec::new(), Vec::new(), config_path)?;
                drop(admission);
                if RESOURCE_UNKNOWN.load(std::sync::atomic::Ordering::Acquire) {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                self.config = Some(
                    Data::capture_writer(
                        after_retirement_open(self.directory(), false)
                            .map_err(|_| err(ErrorKind::RecoveryRequired))?,
                        &self.user,
                    )
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?,
                );
                self.load_recovery()?;
                return self.restore_reviewed();
            }
            let BoundRestorePlan {
                complete_birth,
                strict,
                profile,
                plan,
                base,
                old: old_record,
                expected,
            } = self.prepare_restore_plan()?;
            let restored = plan.target.clone();
            if let Some(old) = old_record.as_ref() {
                let expected = expected
                    .as_ref()
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
                if old.base.restore_sha256 != base.restore_sha256 || self.recovery_next.is_some() {
                    let sources = self.recovery_fingerprint()?;
                    let encoded = expected.bytes()?;
                    successor::transition(|step| match step {
                        successor::Step::Durable => {
                            self.ensure_successor(&encoded, expected, &plan.baseline, &profile)
                        }
                        successor::Step::Recheck => {
                            self.recovery_records()?;
                            if self.recovery_fingerprint()? != sources {
                                return Err(err(ErrorKind::RecoveryRequired));
                            }
                            Ok(())
                        }
                        successor::Step::Publish => self.publish_successor(&encoded),
                    })?;
                }
            } else {
                if self.recovery_next.is_some() {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                if let Some(partial) = self.recovery_record.take() {
                    let expected = (partial.id, partial.security);
                    close_file(partial.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
                    erase_exact(self.parent(), "recovery.intent", &self.user, expected)
                        .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                }
                let encoded = successor::V2::from_base(
                    &base,
                    self.plugins
                        .to_str()
                        .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                        .into(),
                )
                .bytes()?;
                #[cfg(debug_assertions)]
                let encoded = if successor_qualification::historical() {
                    // Historical-format injection at the actual producer, not
                    // an observed old binary or a source-authority constructor.
                    successor_proof_parent(self.parent(), &self.user);
                    historical_qualification_bytes(&base)?
                } else {
                    encoded
                };
                self.recovery_record = Some(
                    create_data(self.parent(), "recovery.intent", &encoded, &self.user)
                        .map_err(|_| err(ErrorKind::RecoveryRequired))?,
                );
                #[cfg(debug_assertions)]
                successor_checkpoint(
                    self.parent(),
                    &self.user,
                    successor_qualification::Fault::HistoricalJournal,
                );
            }
            let record = self
                .recovery_record
                .take()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            let expected = (record.id, record.security);
            if self.partial_original {
                // This is an incomplete pre-loan copy, never a post-child
                // baseline. Keep the exact unchanged native source until this
                // durable recovery journal exists; retire the partial copy
                // before publishing any replacement of that source.
                if self.intent.is_some() {
                    self.clear_writer_intent()?;
                }
                let saved = self
                    .writer_original
                    .take()
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
                saved
                    .check(&self.user)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                if self.partial_original_hash
                    != Some(
                        Sha256::digest(
                            body(&saved.file).map_err(|_| err(ErrorKind::RecoveryRequired))?,
                        )
                        .into(),
                    )
                {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                let saved_expected = (saved.id, saved.security);
                close_file(saved.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
                erase_exact(self.parent(), "writer.original", &self.user, saved_expected)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                self.partial_original = false;
                self.partial_original_hash = None;
                self.writer_base = None;
            }
            // Explicit restore owns this exact native stage under the durable
            // recovery record. It never sweeps a directory or follows a path.
            if let Some(stage) = self.recovery_stage.take() {
                let stage_expected = (stage.id, stage.security);
                stage
                    .check(&self.user)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                close_file(stage.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
                erase_exact(self.directory(), "config.stage", &self.user, stage_expected)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                #[cfg(debug_assertions)]
                successor_checkpoint(
                    self.parent(),
                    &self.user,
                    successor_qualification::Fault::StageRetired,
                );
            }
            if plan.needs_publish && !(self.body == restored && strict) {
                let config = self.config.take().unwrap();
                self.config = Some(self.commit_registry_merge(config, &restored)?);
            }
            self.body = restored;
            self.origin_check()?;
            self.config
                .as_ref()
                .unwrap()
                .check(&self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            if body(&self.config.as_ref().unwrap().file)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?
                != self.body
            {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            if self.writer_original.is_some() {
                self.durable_original()?;
            }
            record
                .check(&self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            if self.intent.is_some() {
                self.clear_writer_intent()?;
            }
            if self.writer_original.is_some() {
                self.clear_writer_original()?;
            }
            self.clear_writer_prepare()?;
            #[cfg(debug_assertions)]
            successor_checkpoint(
                self.parent(),
                &self.user,
                successor_qualification::Fault::PublishedOriginalRemoved,
            );
            if !complete_birth {
                self.clear_writer_birth()?;
            }
            close_file(record.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
            erase_exact(self.parent(), "recovery.intent", &self.user, expected)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            if complete_birth {
                self.clear_writer_birth()?;
            }
            self.loaned = false;
            self.poison = false;
            self.binding = None;
            self.writer_base = None;
            self.check()
        }
        fn parent(&self) -> &File {
            &self.ancestors[self.ancestors.len() - 2].file
        }
        fn directory(&self) -> &File {
            &self.ancestors[self.ancestors.len() - 1].file
        }
        fn core_check(&self) -> Result<(), ClientConfigError> {
            if self.poison {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            self.origin_check()
        }
        // Poison blocks public reuse, not the authenticated owner's final
        // provenance checks while completing an already bound settlement.
        fn origin_check(&self) -> Result<(), ClientConfigError> {
            if self.publication_unknown {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            self.retained_origin_check()
        }
        // Used only inside the active native publication's final verification;
        // it never clears uncertainty after a failed/returned publication.
        fn retained_origin_check(&self) -> Result<(), ClientConfigError> {
            // Poison may be bypassed only for authenticated final checks, never
            // an actually unknown native resource close elsewhere in this owner
            // domain. NoStart/restore/settle must retain journals and admission.
            if RESOURCE_UNKNOWN.load(std::sync::atomic::Ordering::Acquire) {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let user = User::current().map_err(|cause| explain(ErrorKind::Unavailable, cause))?;
            if user.sid != self.user.sid {
                return Err(err(ErrorKind::Unavailable));
            }
            for a in &self.ancestors {
                a.recheck(&user).map_err(|_| err(ErrorKind::Unavailable))?
            }
            for data in [&self.owner, &self.init].into_iter().flatten() {
                data.check(&user).map_err(|_| err(ErrorKind::Unavailable))?
            }
            self.docker
                .check_held_origin()
                .map_err(|_| err(ErrorKind::Unavailable))?;
            self.credentials
                .check_held_origin()
                .map_err(|_| err(ErrorKind::Unavailable))?;
            Ok(())
        }
        pub(super) fn check(&self) -> Result<(), ClientConfigError> {
            self.core_check()?;
            if self.loaned {
                return Err(err(ErrorKind::RecoveryRequired));
            };
            self.config
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                .check(&self.user)
                .map_err(|_| err(ErrorKind::Unavailable))
        }

        pub(super) fn replace_config(&mut self, bytes: &[u8]) -> Result<(), ClientConfigError> {
            self.check()?;
            parse_config(bytes, &self.plugins)?;
            if bytes == self.body {
                return Ok(());
            }
            self.poison = true;
            let intent_bytes = local_replace_producer(
                &successor::hash(&self.body),
                Some(&successor::hash(bytes)),
            )?;
            self.intent = Some(
                create_data(self.parent(), "writer.intent", &intent_bytes, &self.user)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?,
            );
            let old = self
                .config
                .take()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            old.check(&self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let old_id = old.id;
            close_file(old.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let target = Data::capture_writer(
                relative(
                    self.directory(),
                    "config.json",
                    false,
                    FILE_OPEN,
                    None,
                    false,
                )
                .map_err(|_| err(ErrorKind::RecoveryRequired))?,
                &self.user,
            )
            .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            if target.id != old_id
                || body(&target.file).map_err(|_| err(ErrorKind::RecoveryRequired))? != self.body
            {
                let _ = close_file(target.file);
                return Err(err(ErrorKind::RecoveryRequired));
            }
            close_file(target.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
            // Fixed stage is CREATE_NEW. Crash/unknown leaves stage+intent and
            // owner retained; ordinary open never sweeps or retries either.
            let stage = create_data(self.directory(), "config.stage", bytes, &self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let current = self.publish_config_stage(stage, bytes)?;
            self.config = Some(current);
            self.body = bytes.to_vec();
            self.clear_writer_intent()?;
            self.poison = false;
            self.check()
        }
        pub(super) fn begin_writer(
            &mut self,
            keys: Vec<String>,
            args: Vec<OsString>,
            config_path: PathBuf,
        ) -> Result<WriterAdmission, ClientConfigError> {
            self.check()?;
            self.writer_stopped = false;
            let mut binding = [0u8; 32];
            if unsafe { BCryptGenRandom(null_mut(), binding.as_mut_ptr(), 32, 2) } < 0
                || binding == [0; 32]
            {
                return Err(err(ErrorKind::Unavailable));
            }
            self.poison = true;
            self.binding = Some(binding);
            self.writer_keys = keys;
            let original = self
                .config
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            self.writer_base = Some((
                original.id,
                original.security,
                Sha256::digest(&self.body).into(),
            ));
            // Persist the exact original BEFORE intent publication and before releasing
            // config.json. CREATE_NEW refuses any orphan rather than overwriting it.
            // create_data writes, flushes, reads back, positively closes the writer,
            // and independently captures a private read-only native capability.
            original
                .check(&self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            if body(&original.file).map_err(|_| err(ErrorKind::RecoveryRequired))? != self.body {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let base_id = original.id;
            let base_sd = original.security;
            let prepare=serde_json::to_vec(&serde_json::json!({"schema":3,"kind":"docker-writer-prepare","mutation":"registry-entries-v2",
                "binding":hex(&binding),"keysSha256":hex(&Sha256::digest(writer_keys_json(&self.writer_keys)?)),
                "baseSha256":hex(&Sha256::digest(&self.body)),"baseVolume":base_id.volume,"baseHigh":base_id.high,"baseLow":base_id.low,"baseSdSha256":hex(&base_sd)
            })).map_err(|_|err(ErrorKind::RecoveryRequired))?;
            self.writer_prepare = Some(
                create_data(self.parent(), "writer.prepare", &prepare, &self.user)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?,
            );
            self.writer_original = Some(
                create_data(self.parent(), "writer.original", &self.body, &self.user)
                    .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?,
            );
            let saved = self.writer_original.as_ref().unwrap();
            let hex = |v: &[u8]| v.iter().map(|b| format!("{b:02x}")).collect::<String>();
            let bytes = serde_json::to_vec(&serde_json::json!({
                "schema":3,"kind":"docker-writer","mutation":"registry-entries-v2",
                "keysSha256":format!("{:x}", Sha256::digest(writer_keys_json(&self.writer_keys)?)),
                "binding":hex(&binding), "baseSha256":format!("{:x}",Sha256::digest(&self.body)),
                "baseVolume":base_id.volume,"baseHigh":base_id.high,"baseLow":base_id.low,"baseSdSha256":hex(&base_sd),
                "originalVolume":saved.id.volume,"originalHigh":saved.id.high,"originalLow":saved.id.low,
                "originalSdSha256":hex(&saved.security),"originalSha256":format!("{:x}",Sha256::digest(&self.body))
            })).map_err(|_|err(ErrorKind::RecoveryRequired))?;
            if bytes.len() > 4096 {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let intent = create_data(self.parent(), "writer.intent", &bytes, &self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            self.intent = Some(intent);
            let birth = BirthPublisher {
                parent: duplicate_held(&self.ancestors[self.ancestors.len() - 2], &self.user)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?,
                intent: Data::capture(
                    self.intent
                        .as_ref()
                        .unwrap()
                        .file
                        .try_clone()
                        .map_err(|_| err(ErrorKind::RecoveryRequired))?,
                    &self.user,
                )
                .map_err(|_| err(ErrorKind::RecoveryRequired))?,
                original: Data::capture(
                    self.writer_original
                        .as_ref()
                        .unwrap()
                        .file
                        .try_clone()
                        .map_err(|_| err(ErrorKind::RecoveryRequired))?,
                    &self.user,
                )
                .map_err(|_| err(ErrorKind::RecoveryRequired))?,
                binding,
                record: self.writer_birth.clone(),
            };
            self.durable_original()?;
            self.origin_check()?;
            self.config
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                .check(&self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            self.loaned = true;
            let config = self
                .config
                .take()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            close_file(config.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
            self.poison = false;
            Ok(WriterAdmission {
                binding,
                birth: Some(birth),
                invocation: Some(WriterInvocation {
                    docker: self.docker.clone(),
                    credentials: self.credentials.clone(),
                    config: config_path,
                    plugins: self.plugins.clone(),
                    args,
                }),
            })
        }
        // The held exact bytes are authoritative for merge, not the in-memory
        // cached body. Recheck identity, private SD and digest at every settlement.
        fn durable_original(&self) -> Result<Vec<u8>, ClientConfigError> {
            let saved = self
                .writer_original
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            saved
                .check(&self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let bytes = body(&saved.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let (_, _, digest) = self
                .writer_base
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            if <[u8; 32]>::from(Sha256::digest(&bytes)) != digest {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            Ok(bytes)
        }
        fn clear_writer_intent(&mut self) -> Result<(), ClientConfigError> {
            let data = self
                .intent
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            data.check(&self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let expected = (data.id, data.security);
            let data = self.intent.take().unwrap();
            close_file(data.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
            erase_exact(self.parent(), "writer.intent", &self.user, expected)
                .map_err(|_| err(ErrorKind::RecoveryRequired))
        }
        fn clear_writer_original(&mut self) -> Result<(), ClientConfigError> {
            let data = self
                .writer_original
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            let expected = (data.id, data.security);
            data.check(&self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let (_, _, hash) = self
                .writer_base
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            let bytes = body(&data.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
            if <[u8; 32]>::from(Sha256::digest(&bytes)) != hash {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let data = self.writer_original.take().unwrap();
            close_file(data.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
            erase_exact(self.parent(), "writer.original", &self.user, expected)
                .map_err(|_| err(ErrorKind::RecoveryRequired))
        }
        fn clear_writer_birth(&mut self) -> Result<(), ClientConfigError> {
            let data = self
                .writer_birth
                .lock()
                .map_err(|_| err(ErrorKind::RecoveryRequired))?
                .take();
            if let Some(data) = data {
                data.check(&self.user)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                let expected = (data.id, data.security);
                close_file(data.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
                erase_exact(self.parent(), "writer.birth", &self.user, expected)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            } else if optional(self.parent(), "writer.birth")
                .map_err(|_| err(ErrorKind::RecoveryRequired))?
                .is_some()
            {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            Ok(())
        }
        fn clear_writer_prepare(&mut self) -> Result<(), ClientConfigError> {
            if let Some(data) = self.writer_prepare.take() {
                data.check(&self.user)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                let expected = (data.id, data.security);
                close_file(data.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
                erase_exact(self.parent(), "writer.prepare", &self.user, expected)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            } else if optional(self.parent(), "writer.prepare")
                .map_err(|_| err(ErrorKind::RecoveryRequired))?
                .is_some()
            {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            Ok(())
        }
        pub(super) fn abort_unstarted(
            &mut self,
            proof: crate::native_client::NeverStartedClient,
        ) -> Result<(), ClientConfigError> {
            self.core_check()?;
            if !self.loaned {
                return Err(err(ErrorKind::RecoveryRequired));
            };
            self.poison = true;
            let admission = WriterAdmission {
                invocation: None,
                birth: None,
                binding: self
                    .binding
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?,
            };
            proof
                .consume_for_writer(&admission)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            self.writer_stopped = true;
            let baseline = self.durable_original()?;
            let (original, sd, hash) = self
                .writer_base
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
            let file = relative(
                self.directory(),
                "config.json",
                false,
                FILE_OPEN,
                None,
                false,
            )
            .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let data = Data::capture_writer(file, &self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let bytes = body(&data.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
            if data.id != original
                || data.security != sd
                || <[u8; 32]>::from(Sha256::digest(&bytes)) != hash
                || bytes != baseline
            {
                let _ = close_file(data.file);
                return Err(err(ErrorKind::RecoveryRequired));
            }
            self.intent
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                .check(&self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            self.origin_check()?;
            data.check(&self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            self.durable_original()?;
            self.clear_writer_intent()?;
            self.clear_writer_original()?;
            self.clear_writer_prepare()?;
            self.clear_writer_birth()?;
            self.config = Some(data);
            self.loaned = false;
            self.binding = None;
            self.writer_base = None;
            self.poison = false;
            self.check()
        }
        fn publish_config_stage(
            &mut self,
            stage: Data,
            bytes: &[u8],
        ) -> Result<Data, ClientConfigError> {
            self.origin_check()?;
            if self.recovery_stage.is_some() || self.config.is_some() {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let expected = (stage.id, stage.security);
            self.recovery_stage = Some(stage);
            self.recovery_stage_hash = Some(Sha256::digest(bytes).into());
            #[cfg(debug_assertions)]
            successor_checkpoint(
                self.parent(),
                &self.user,
                successor_qualification::Fault::HistoricalStagePrepared,
            );
            target_publish_sequence(|step| {
                match step {
                    TargetPublishStep::Arm => self.publication_unknown = true,
                    TargetPublishStep::Attempt => {
                        rename(
                            self.directory(),
                            &self.recovery_stage.as_ref().unwrap().file,
                            "config.json",
                        )
                        .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?;
                        #[cfg(debug_assertions)]
                        if successor_lost(
                            self.parent(),
                            &self.user,
                            successor_qualification::Fault::LostTargetRename,
                        ) {
                            retain_unknown();
                            return Err(err(ErrorKind::RecoveryRequired));
                        }
                    }
                    TargetPublishStep::CloseStage => {
                        let stage = self.recovery_stage.take().unwrap();
                        close_file(stage.file)
                            .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?;
                    }
                    TargetPublishStep::Reopen => {
                        let file = after_retirement_open(self.directory(), false)
                            .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?;
                        self.config = Some(
                            Data::capture_published(file, &self.user)
                                .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?,
                        );
                    }
                    TargetPublishStep::Verify => {
                        self.retained_origin_check()?;
                        let data = self.config.as_ref().unwrap();
                        data.check(&self.user)
                            .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?;
                        if (data.id, data.security) != expected
                            || body(&data.file)
                                .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?
                                != bytes
                        {
                            return Err(err(ErrorKind::RecoveryRequired));
                        }
                    }
                    TargetPublishStep::Complete => {
                        self.recovery_stage_hash = None;
                        self.publication_unknown = false;
                    }
                }
                Ok(())
            })?;
            Ok(self.config.take().unwrap())
        }
        fn commit_registry_merge(
            &mut self,
            current: Data,
            bytes: &[u8],
        ) -> Result<Data, ClientConfigError> {
            current
                .check(&self.user)
                .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?;
            close_file(current.file)
                .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?;
            let stage = create_data(self.directory(), "config.stage", bytes, &self.user)
                .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?;
            self.publish_config_stage(stage, bytes)
        }
        pub(super) fn finish_writer(
            &mut self,
            retired: crate::client_job::ExecutionQuiesced,
        ) -> Result<(), ClientConfigError> {
            self.core_check()?;
            if !self.loaned {
                return Err(err(ErrorKind::RecoveryRequired));
            };
            self.poison = true;
            let admission = WriterAdmission {
                invocation: None,
                birth: None,
                binding: self
                    .binding
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?,
            };
            retired
                .consume_for_writer(&admission)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            self.writer_stopped = true;
            self.intent
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                .check(&self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let mut writable: Option<Data> = None;
            let mut readonly: Option<Data> = None;
            let mut expected = None;
            let mut bytes = Vec::new();
            writer_durability_sequence(|step| {
                match step {
                    WriterCommitStep::AdmitWritable => {
                        let f = after_retirement_open(self.directory(), true)
                            .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?;
                        let data = Data::capture_writer(f, &self.user)
                            .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?;
                        bytes = body(&data.file)
                            .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?;
                        parse_config(&bytes, &self.plugins)?;
                        expected = Some((data.id, data.security));
                        writable = Some(data);
                    }
                    WriterCommitStep::Flush => {
                        writable.as_ref().unwrap().file.sync_all().map_err(|e| {
                            explain(
                                ErrorKind::RecoveryRequired,
                                NativeDiagnostic {
                                    stage: "writer owner flush",
                                    system_error: e.raw_os_error().map(|v| v as u32),
                                    security: None,
                                },
                            )
                        })?
                    }
                    WriterCommitStep::RecheckFlushed => {
                        let data = writable.as_ref().unwrap();
                        data.check(&self.user)
                            .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?;
                        if body(&data.file)
                            .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?
                            != bytes
                        {
                            return Err(explain(
                                ErrorKind::RecoveryRequired,
                                bad("writer flushed bytes changed"),
                            ));
                        }
                    }
                    WriterCommitStep::CloseWritable => close_file(writable.take().unwrap().file)
                        .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?,
                    WriterCommitStep::AdmitReadOnly => {
                        let f = after_retirement_open(self.directory(), false)
                            .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?;
                        let data = Data::capture_writer(f, &self.user)
                            .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?;
                        let (id, sd) = expected.unwrap();
                        if data.id != id
                            || data.security != sd
                            || body(&data.file)
                                .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?
                                != bytes
                        {
                            return Err(explain(
                                ErrorKind::RecoveryRequired,
                                bad("writer flushed reopen changed"),
                            ));
                        }
                        readonly = Some(data);
                    }
                }
                Ok(())
            })?;
            let config = readonly.unwrap();
            let original = self.durable_original()?;
            let merged = merge_registry_auths(&original, &bytes, &self.plugins, &self.writer_keys)?;
            // Always sever the final config FILE_OBJECT from incoming child
            // references, including a byte-identical/no-op child completion.
            let config = self.commit_registry_merge(config, &merged)?;
            self.config = Some(config);
            self.body = merged;
            self.origin_check()?;
            self.config
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                .check(&self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            self.intent
                .as_ref()
                .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                .check(&self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            self.durable_original()?;
            self.clear_writer_intent()?;
            // Cleanup begins only after final native owner/config/intent checks.
            // Intent first, original last: a crash leaves either pending intent or
            // an orphan original; both refuse ordinary startup. No disk flag is
            // execution quiescence or authority to automatically resume a loan.
            self.clear_writer_original()?;
            self.clear_writer_prepare()?;
            self.clear_writer_birth()?;
            self.loaned = false;
            self.binding = None;
            self.writer_base = None;
            self.poison = false;
            self.check()
        }
    }

    pub(super) struct Snapshot {
        user: Option<User>,
        ancestors: Vec<Held>,
        config: Option<Held>,
        init: Option<Held>,
        expected: [u8; 32],
        docker: Option<crate::runtime_cache::CachedExecutable>,
        credentials: Option<crate::runtime_cache::CachedExecutable>,
    }
    impl Snapshot {
        pub(super) fn check(&self) -> Result<(), ClientConfigError> {
            writer_resources_known()?;
            let user = User::current().map_err(|cause| explain(ErrorKind::Unavailable, cause))?;
            if self.user.as_ref().is_none_or(|owner| owner.sid != user.sid) {
                return Err(err(ErrorKind::Unavailable));
            }
            for held in &self.ancestors {
                held.recheck(&user)
                    .map_err(|_| err(ErrorKind::Unavailable))?;
            }
            for held in [&self.config, &self.init] {
                held.as_ref()
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                    .recheck(&user)
                    .map_err(|_| err(ErrorKind::Unavailable))?;
            }
            for role in [&self.docker, &self.credentials] {
                role.as_ref()
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                    .check_held_origin()
                    .map_err(|_| err(ErrorKind::Unavailable))?;
            }
            Ok(())
        }
        fn retain(&mut self) {
            if let Some(value) = self.config.take() {
                std::mem::forget(value);
            }
            if let Some(value) = self.init.take() {
                std::mem::forget(value);
            }
            for value in self.ancestors.drain(..) {
                std::mem::forget(value);
            }
            if let Some(value) = self.user.take() {
                std::mem::forget(value);
            }
            if let Some(value) = self.docker.take() {
                std::mem::forget(value);
            }
            if let Some(value) = self.credentials.take() {
                std::mem::forget(value);
            }
        }
        pub(super) fn release(&mut self) -> Result<(), ClientConfigError> {
            if writer_resources_known().is_err() {
                self.retain();
                return Err(err(ErrorKind::RecoveryRequired));
            }
            #[cfg(debug_assertions)]
            let snapshot_loss = successor_qualification::selected(
                successor_qualification::Fault::LostSnapshotConfigClose,
            );
            #[cfg(debug_assertions)]
            if snapshot_loss {
                let user = self
                    .user
                    .as_ref()
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
                let client = self
                    .ancestors
                    .get(
                        self.ancestors
                            .len()
                            .checked_sub(4)
                            .ok_or_else(|| err(ErrorKind::RecoveryRequired))?,
                    )
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?;
                successor_proof_parent(&client.file, user);
                self.ancestors
                    .last()
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                    .recheck(user)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            }
            let result = snapshot_close_sequence(self.ancestors.len(), |step| {
                match step {
                    SnapshotCloseStep::Config => {
                        if let Some(value) = self.config.take() {
                            close_file(value.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
                            #[cfg(debug_assertions)]
                            if snapshot_loss
                                && successor_qualification::consume(
                                    successor_qualification::Fault::LostSnapshotConfigClose,
                                )
                            {
                                retain_unknown();
                                return Err(err(ErrorKind::RecoveryRequired));
                            }
                        }
                    }
                    SnapshotCloseStep::Init => {
                        if let Some(value) = self.init.take() {
                            close_file(value.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
                        }
                    }
                    SnapshotCloseStep::Ancestor(_) => {
                        if let Some(value) = self.ancestors.pop() {
                            close_file(value.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
                        }
                    }
                    SnapshotCloseStep::Token => {
                        if let Some(user) = self.user.take() {
                            close_file(user.token).map_err(|_| err(ErrorKind::RecoveryRequired))?;
                        }
                    }
                    SnapshotCloseStep::Roles => {
                        self.docker.take();
                        self.credentials.take();
                    }
                }
                Ok(())
            });
            if result.is_err() {
                self.retain();
            }
            result
        }
    }
    impl Drop for Snapshot {
        fn drop(&mut self) {
            let _ = self.release();
        }
    }
    static SNAPSHOTS: std::sync::OnceLock<
        std::sync::Mutex<std::collections::BTreeMap<String, std::sync::Weak<Snapshot>>>,
    > = std::sync::OnceLock::new();

    fn exclusive_delete(parent: &File, name: &str, directory: bool) -> Result<File, RelativeError> {
        if !native_component(name) {
            return Err(RelativeError::Refused(bad("relative component")));
        };
        let mut chars: Vec<u16> = name.encode_utf16().collect();
        let n = u16::try_from(chars.len() * 2)
            .map_err(|_| RelativeError::Refused(bad("relative name bound")))?;
        let mut unicode = Unicode {
            length: n,
            maximum: n,
            buffer: chars.as_mut_ptr(),
        };
        let mut oa = Attributes {
            length: size_of::<Attributes>() as u32,
            root: raw(parent),
            name: &mut unicode,
            flags: OBJ_CASE_INSENSITIVE | OBJ_DONT_REPARSE,
            descriptor: null_mut(),
            qos: null_mut(),
        };
        let mut io = IoStatus {
            status: 0,
            information: 0,
        };
        let mut h = null_mut();
        let result = unsafe {
            NtCreateFile(
                &mut h,
                FILE_GENERIC_READ
                    | READ_CONTROL_MASK
                    | FILE_READ_ATTRIBUTES
                    | DELETE_MASK
                    | SYNCHRONIZE_MASK,
                &mut oa,
                &mut io,
                null(),
                0,
                0,
                FILE_OPEN,
                NT_OPEN_REPARSE
                    | FILE_SYNCHRONOUS_IO_NONALERT
                    | if directory {
                        FILE_DIRECTORY_FILE
                    } else {
                        FILE_NON_DIRECTORY_FILE
                    },
                null(),
                0,
            )
        };
        if result != 0 {
            return Err(if result as u32 == 0xc0000043 {
                RelativeError::Sharing
            } else if result as u32 == 0xc0000034 {
                RelativeError::Missing
            } else {
                RelativeError::Refused(NativeDiagnostic {
                    stage: "relative native open/create",
                    system_error: Some(result as u32),
                    security: None,
                })
            });
        };
        own(h).map_err(RelativeError::Refused)
    }
    fn dispose_exact(file: impl TakeFile) -> Result<(), NativeDiagnostic> {
        let file = file.take_file();
        let disposition = FILE_DISPOSITION_INFO { DeleteFile: true };
        if unsafe {
            SetFileInformationByHandle(
                raw(&file),
                FileDispositionInfo,
                (&disposition as *const FILE_DISPOSITION_INFO).cast(),
                size_of::<FILE_DISPOSITION_INFO>() as u32,
            )
        } == 0
        {
            retain_unknown();
            std::mem::forget(file);
            return Err(bad("snapshot disposal unknown"));
        };
        close_file(file)
    }
    fn absent(parent: &File, name: &str, directory: bool) -> Result<(), NativeDiagnostic> {
        match relative_status(parent, name, directory, FILE_OPEN, None, false) {
            Err(RelativeError::Missing) => Ok(()),
            _ => Err(bad("snapshot absence unknown")),
        }
    }
    // Only after an actual mutation attempt and positive close. Unlike a
    // pre-effect absence probe, an unexplained suffix cannot release ownership.
    fn disposed_absent(parent: &File, name: &str, directory: bool) -> Result<(), NativeDiagnostic> {
        match relative_status(parent, name, directory, FILE_OPEN, None, false) {
            Err(RelativeError::Missing) => Ok(()),
            Ok(file) => {
                retain_unknown();
                std::mem::forget(file);
                Err(bad("disposal absence unverified"))
            }
            Err(_) => {
                retain_unknown();
                Err(bad("disposal absence unknown"))
            }
        }
    }
    fn directory_names(
        parent: &File,
        allowed: fn(&str) -> bool,
        budget: usize,
    ) -> Result<Vec<String>, NativeDiagnostic> {
        let mut words = Box::new([0u64; 8192]);
        let buffer =
            unsafe { std::slice::from_raw_parts_mut(words.as_mut_ptr().cast::<u8>(), 65536) };
        let mut names = Vec::new();
        let mut class = FileIdBothDirectoryRestartInfo;
        for _ in 0..4 {
            if unsafe {
                GetFileInformationByHandleEx(
                    raw(parent),
                    class,
                    buffer.as_mut_ptr().cast(),
                    buffer.len() as u32,
                )
            } == 0
            {
                if unsafe { GetLastError() } == ERROR_NO_MORE_FILES {
                    return Ok(names);
                };
                return Err(bad("snapshot enumeration"));
            };
            class = FileIdBothDirectoryInfo;
            let mut at = 0usize;
            loop {
                if at + 104 > buffer.len() {
                    return Err(bad("snapshot entry"));
                };
                let next = u32::from_le_bytes(buffer[at..at + 4].try_into().unwrap()) as usize;
                let len = u32::from_le_bytes(buffer[at + 60..at + 64].try_into().unwrap()) as usize;
                if len < 2 || len % 2 != 0 || len > 510 || at + 104 + len > buffer.len() {
                    return Err(bad("snapshot name"));
                };
                let units: Vec<u16> = buffer[at + 104..at + 104 + len]
                    .chunks_exact(2)
                    .map(|x| u16::from_le_bytes([x[0], x[1]]))
                    .collect();
                let name = String::from_utf16(&units).map_err(|_| bad("snapshot encoding"))?;
                if name != "." && name != ".." {
                    if !allowed(&name) || names.len() >= budget {
                        return Err(bad("snapshot inventory"));
                    };
                    names.push(name)
                }
                if next == 0 {
                    break;
                };
                if next < 104 + len || next % 8 != 0 || at + next >= buffer.len() {
                    return Err(bad("snapshot next entry"));
                };
                at += next;
            }
            buffer.fill(0);
        }
        Err(bad("snapshot enumeration bound"))
    }
    fn generation_names(parent: &File) -> Result<Vec<String>, NativeDiagnostic> {
        directory_names(parent, generation_name, 32)
    }
    fn complete_generation(parent: &File) -> Result<(), NativeDiagnostic> {
        let mut names = directory_names(
            parent,
            |name| matches!(name, "config.json" | "generation.init"),
            2,
        )?;
        names.sort();
        if names != ["config.json", "generation.init"] {
            return Err(bad("snapshot incomplete/unknown children"));
        };
        Ok(())
    }
    fn generation_name(name: &str) -> bool {
        name.len() == 68
            && name.starts_with("cfg-")
            && name[4..]
                .bytes()
                .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
    }
    fn manifest(
        root: Identity,
        generation: Identity,
        config: Identity,
        sha: &str,
        user: &User,
    ) -> Value {
        serde_json::json!({"schema":1,"product":"orbwin","ownerSid":user.text,"rootVolume":root.volume,"rootHigh":root.high,"rootLow":root.low,"generationVolume":generation.volume,"generationHigh":generation.high,"generationLow":generation.low,"configVolume":config.volume,"configHigh":config.high,"configLow":config.low,"configSha256":sha})
    }
    fn duplicate_held(h: &Held, user: &User) -> Result<Held, NativeDiagnostic> {
        Held::capture(
            h.file
                .try_clone()
                .map_err(|_| bad("snapshot retained ancestor"))?,
            user,
            h.directory,
            h.private,
        )
    }
    impl Owned {
        fn evict_inactive(
            &mut self,
            parent: &File,
            names: &[String],
            current: &str,
        ) -> Result<(), ClientConfigError> {
            for name in names {
                if name == current {
                    continue;
                }
                let directory = match exclusive_delete(parent, name, true) {
                    Ok(file) => Held::capture(file, &self.user, true, true)
                        .map_err(|_| err(ErrorKind::RecoveryRequired))?,
                    Err(RelativeError::Sharing) => continue,
                    Err(_) => return Err(err(ErrorKind::RecoveryRequired)),
                };
                complete_generation(&directory.file)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                let config = match exclusive_delete(&directory.file, "config.json", false) {
                    Ok(file) => Held::capture(file, &self.user, false, true)
                        .map_err(|_| err(ErrorKind::RecoveryRequired))?,
                    Err(RelativeError::Sharing) => continue,
                    Err(_) => return Err(err(ErrorKind::RecoveryRequired)),
                };
                let init = match exclusive_delete(&directory.file, "generation.init", false) {
                    Ok(file) => Held::capture(file, &self.user, false, true)
                        .map_err(|_| err(ErrorKind::RecoveryRequired))?,
                    Err(RelativeError::Sharing) => continue,
                    Err(_) => return Err(err(ErrorKind::RecoveryRequired)),
                };
                let bytes = body(&config.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
                let sha = format!("{:x}", Sha256::digest(&bytes));
                let expected = manifest(
                    self.ancestors[self.ancestors.len() - 2].id,
                    directory.id,
                    config.id,
                    &sha,
                    &self.user,
                );
                if name != &format!("cfg-{sha}")
                    || checked_json(
                        &body(&init.file).map_err(|_| err(ErrorKind::RecoveryRequired))?,
                    )? != expected
                {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                // Exclusive DELETE-capable native directory+file opens are the actual
                // inactivity proof, not Weak counts, PID guesses or missing snapshots.
                // No reader using this protocol can remain or enter while held.
                self.poison = true;
                self.intent=Some(create_data(self.parent(),"snapshot.intent",&serde_json::to_vec(&serde_json::json!({"schema":1,"kind":"evict-derived-snapshot","generation":name,"binding":expected})).unwrap(),&self.user).map_err(|_|err(ErrorKind::RecoveryRequired))?);
                dispose_exact(config.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
                disposed_absent(&directory.file, "config.json", false)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                dispose_exact(init.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
                disposed_absent(&directory.file, "generation.init", false)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                // Directory deletion refuses any unrecorded extra child. Such a
                // partial/unknown state retains intent+owner and is never swept.
                dispose_exact(directory.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
                disposed_absent(parent, name, true)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                close_file(
                    self.intent
                        .take()
                        .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                        .file,
                )
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                erase(self.parent(), "snapshot.intent", &self.user)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                self.poison = false;
                return Ok(());
            }
            Err(err(ErrorKind::Busy))
        }
        pub(super) fn snapshot(&mut self) -> Result<ClientConfigSnapshot, ClientConfigError> {
            self.check()?;
            let sha = format!("{:x}", Sha256::digest(&self.body));
            let name = format!("cfg-{sha}");
            let root_id = self.ancestors[self.ancestors.len() - 2].id;
            let key = format!(
                "{}:{}:{}:{}:{sha}",
                self.user.text, root_id.volume, root_id.high, root_id.low
            );
            let table =
                SNAPSHOTS.get_or_init(|| std::sync::Mutex::new(std::collections::BTreeMap::new()));
            let mut table = table.lock().map_err(|_| err(ErrorKind::Unavailable))?;
            table.retain(|_, entry| entry.strong_count() > 0);
            if let Some(snapshot) = table.get(&key).and_then(|v| v.upgrade()) {
                snapshot.check()?;
                let profile = self
                    .user
                    .profile()
                    .map_err(|_| err(ErrorKind::Unavailable))?;
                return Ok(ClientConfigSnapshot {
                    inner: Some(snapshot),
                    path: profile
                        .join(".orbwin-private")
                        .join("client")
                        .join("snapshots")
                        .join(name),
                    buildx: profile
                        .join(".orbwin-private")
                        .join("client")
                        .join("docker")
                        .join("buildx"),
                });
            }
            if table.len() >= 32 {
                return Err(err(ErrorKind::Busy));
            }
            let descriptor =
                Descriptor::private(&self.user, true).map_err(|_| err(ErrorKind::Unavailable))?;
            match relative_status(self.parent(), "snapshots", true, FILE_OPEN, None, false) {
                Ok(file) => {
                    let existing = Held::capture(file, &self.user, true, true)
                        .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                    if matches!(
                        relative_status(&existing.file, &name, true, FILE_OPEN, None, false),
                        Err(RelativeError::Missing)
                    ) {
                        let mut names = generation_names(&existing.file)
                            .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                        names.sort();
                        if names.len() >= 32 {
                            self.evict_inactive(&existing.file, &names, &name)?
                        }
                    }
                }
                Err(RelativeError::Missing) => {}
                Err(_) => return Err(err(ErrorKind::RecoveryRequired)),
            }
            self.poison = true;
            self.intent = Some(
                create_data(
                    self.parent(),
                    "snapshot.intent",
                    b"{\"schema\":1,\"kind\":\"snapshot\"}",
                    &self.user,
                )
                .map_err(|_| err(ErrorKind::RecoveryRequired))?,
            );
            let snapshots = Held::capture(
                relative(
                    self.parent(),
                    "snapshots",
                    true,
                    FILE_OPEN_IF,
                    Some(&descriptor),
                    false,
                )
                .map_err(|_| err(ErrorKind::RecoveryRequired))?,
                &self.user,
                true,
                true,
            )
            .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let existing = relative_status(&snapshots.file, &name, true, FILE_OPEN, None, false);
            let fresh = matches!(existing, Err(RelativeError::Missing));
            if fresh {
                let names = generation_names(&snapshots.file)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                if names.len() >= 32 {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
            }
            let directory = if fresh {
                relative(
                    &snapshots.file,
                    &name,
                    true,
                    FILE_CREATE,
                    Some(&descriptor),
                    false,
                )
                .map_err(|_| err(ErrorKind::RecoveryRequired))?
            } else {
                existing.map_err(|_| err(ErrorKind::RecoveryRequired))?
            };
            let generation = Held::capture(directory, &self.user, true, true)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            if fresh {
                let created = create_data(&generation.file, "config.json", &self.body, &self.user)
                    .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                close_file(created.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
            }
            let config = Held::capture(
                relative(
                    &generation.file,
                    "config.json",
                    false,
                    FILE_OPEN,
                    None,
                    false,
                )
                .map_err(|_| err(ErrorKind::RecoveryRequired))?,
                &self.user,
                false,
                true,
            )
            .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            if body(&config.file).map_err(|_| err(ErrorKind::RecoveryRequired))? != self.body {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let binding = manifest(root_id, generation.id, config.id, &sha, &self.user);
            if fresh {
                let created = create_data(
                    &generation.file,
                    "generation.init",
                    &serde_json::to_vec(&binding).unwrap(),
                    &self.user,
                )
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
                close_file(created.file).map_err(|_| err(ErrorKind::RecoveryRequired))?;
            }
            let init = Held::capture(
                relative(
                    &generation.file,
                    "generation.init",
                    false,
                    FILE_OPEN,
                    None,
                    false,
                )
                .map_err(|_| err(ErrorKind::RecoveryRequired))?,
                &self.user,
                false,
                true,
            )
            .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            if checked_json(&body(&init.file).map_err(|_| err(ErrorKind::RecoveryRequired))?)?
                != binding
            {
                return Err(err(ErrorKind::RecoveryRequired));
            }
            let mut ancestors = Vec::new();
            for h in &self.ancestors {
                ancestors.push(
                    duplicate_held(h, &self.user).map_err(|_| err(ErrorKind::RecoveryRequired))?,
                )
            }
            ancestors.push(snapshots);
            ancestors.push(generation);
            let user = User::current().map_err(|_| err(ErrorKind::RecoveryRequired))?;
            let snapshot = std::sync::Arc::new(Snapshot {
                user: Some(user),
                ancestors,
                config: Some(config),
                init: Some(init),
                expected: Sha256::digest(&self.body).into(),
                docker: Some(self.docker.clone()),
                credentials: Some(self.credentials.clone()),
            });
            snapshot.check()?;
            close_file(
                self.intent
                    .take()
                    .ok_or_else(|| err(ErrorKind::RecoveryRequired))?
                    .file,
            )
            .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            erase(self.parent(), "snapshot.intent", &self.user)
                .map_err(|_| err(ErrorKind::RecoveryRequired))?;
            self.poison = false;
            table.insert(key, std::sync::Arc::downgrade(&snapshot));
            let profile = self
                .user
                .profile()
                .map_err(|_| err(ErrorKind::Unavailable))?;
            Ok(ClientConfigSnapshot {
                inner: Some(snapshot),
                path: profile
                    .join(".orbwin-private")
                    .join("client")
                    .join("snapshots")
                    .join(name),
                buildx: profile
                    .join(".orbwin-private")
                    .join("client")
                    .join("docker")
                    .join("buildx"),
            })
        }
    }
    pub(super) fn settle(mut owned: Box<Owned>) -> Result<(), ClientConfigError> {
        if owned.poison || owned.loaned || owned.check().is_err() {
            Box::leak(owned);
            return Err(err(ErrorKind::RecoveryRequired));
        }
        // Drop each record in explicit order. On first uncertain close, leak all
        // remaining resources including the exclusive controller FILE_OBJECT.
        if let Some(data) = owned.config.take() {
            if close_file(data.file).is_err() {
                Box::leak(owned);
                return Err(err(ErrorKind::RecoveryRequired));
            }
        }
        if let Some(data) = owned.init.take() {
            if close_file(data.file).is_err() {
                Box::leak(owned);
                return Err(err(ErrorKind::RecoveryRequired));
            }
        }
        while let Some(h) = owned.ancestors.pop() {
            if close_file(h.file).is_err() {
                Box::leak(owned);
                return Err(err(ErrorKind::RecoveryRequired));
            }
        }
        if let Some(owner) = owned.owner.take() {
            if close_file(owner.file).is_err() {
                Box::leak(owned);
                return Err(err(ErrorKind::RecoveryRequired));
            }
        }
        Ok(())
    }
    pub(super) fn acquire(
        docker: crate::runtime_cache::CachedExecutable,
        credentials: crate::runtime_cache::CachedExecutable,
    ) -> Result<ClientConfigGuard, ClientConfigError> {
        acquire_mode(docker, credentials, false)
    }
    pub(super) fn acquire_mode(
        docker: crate::runtime_cache::CachedExecutable,
        credentials: crate::runtime_cache::CachedExecutable,
        recovery: bool,
    ) -> Result<ClientConfigGuard, ClientConfigError> {
        if RESOURCE_UNKNOWN.load(std::sync::atomic::Ordering::Acquire) {
            return Err(err(ErrorKind::RecoveryRequired));
        }
        docker
            .check_held_origin()
            .map_err(|_| explain(ErrorKind::Unavailable, bad("runtime capability origin")))?;
        credentials
            .check_held_origin()
            .map_err(|_| explain(ErrorKind::Unavailable, bad("runtime capability origin")))?;
        if docker.path().file_name() != Some(std::ffi::OsStr::new("docker.exe"))
            || credentials.path().file_name()
                != Some(std::ffi::OsStr::new("docker-credential-orbwin.exe"))
        {
            return Err(err(ErrorKind::Unavailable));
        }
        let plugins = docker
            .path()
            .parent()
            .ok_or_else(|| err(ErrorKind::Unavailable))?
            .join("docker-cli-plugins");
        let user = User::current().map_err(|cause| explain(ErrorKind::Unavailable, cause))?;
        let (profile, mut ancestors) =
            profile_chain(&user).map_err(|cause| explain(ErrorKind::Unavailable, cause))?;
        let dir_sd = Descriptor::private(&user, true)
            .map_err(|cause| explain(ErrorKind::Unavailable, cause))?;
        let private = relative_status(
            &ancestors.last().unwrap().file,
            ".orbwin-private",
            true,
            if recovery { FILE_OPEN } else { FILE_OPEN_IF },
            Some(&dir_sd),
            false,
        )
        .map_err(|cause| match cause {
            RelativeError::Missing if recovery => err(ErrorKind::NoInterruptedWrite),
            cause => explain(ErrorKind::Unavailable, relative_diagnostic(cause)),
        })?;
        ancestors.push(
            Held::capture(private, &user, true, true)
                .map_err(|cause| explain(ErrorKind::Unavailable, cause))?,
        );
        let private_parent = &ancestors.last().unwrap().file;
        if optional(private_parent, "client.create-intent")
            .map_err(|cause| explain(ErrorKind::Unavailable, cause))?
            .is_some()
        {
            return Err(err(ErrorKind::RecoveryRequired));
        }
        let existing = relative_status(private_parent, "client", true, FILE_OPEN, None, false);
        let fresh = matches!(existing, Err(RelativeError::Missing));
        if recovery && fresh {
            return Err(err(ErrorKind::NoInterruptedWrite));
        }
        if fresh {
            legacy_absent(&user)?;
            let intent = create_data(
                private_parent,
                "client.create-intent",
                b"{\"schema\":1,\"kind\":\"fresh-client\"}",
                &user,
            )
            .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?;
            close_file(intent.file).map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?;
            #[cfg(debug_assertions)]
            fresh_checkpoint(private_parent, fresh_qualification::Fault::MarkerFull);
        }
        let client = if fresh {
            relative(
                private_parent,
                "client",
                true,
                FILE_CREATE,
                Some(&dir_sd),
                false,
            )
            .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?
        } else {
            existing.map_err(|cause| explain(ErrorKind::Unavailable, relative_diagnostic(cause)))?
        };
        ancestors.push(
            Held::capture(client, &user, true, true)
                .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?,
        );
        #[cfg(debug_assertions)]
        if fresh {
            fresh_checkpoint(
                &ancestors.last().unwrap().file,
                fresh_qualification::Fault::ClientFull,
            );
        }
        let parent = &ancestors.last().unwrap().file;
        let owner_file = relative(
            parent,
            "controller.lock",
            false,
            if fresh { FILE_CREATE } else { FILE_OPEN },
            Some(
                &Descriptor::private(&user, false)
                    .map_err(|cause| explain(ErrorKind::Unavailable, cause))?,
            ),
            true,
        )
        .map_err(|cause| explain(ErrorKind::Unavailable, cause))?;
        let owner = Data::capture(owner_file, &user)
            .map_err(|cause| explain(ErrorKind::Unavailable, cause))?;
        // From the first exclusive owner onward, any failure retains the owner and
        // namespace. Existing incomplete initialization is never freshened.
        let mut owned = Box::new(Owned {
            user,
            ancestors,
            owner: Some(owner),
            init: None,
            config: None,
            intent: None,
            body: Vec::new(),
            plugins,
            docker,
            credentials,
            loaned: false,
            poison: true,
            binding: None,
            writer_base: None,
            writer_original: None,
            writer_prepare: None,
            writer_birth: Default::default(),
            recovery_record: None,
            recovery_next: None,
            completed_birth: false,
            publication_unknown: false,
            recovery_stage: None,
            recovery_stage_hash: None,
            recovery_preserves_source: false,
            local_replace_hash: None,
            local_eof: false,
            partial_prepare: false,
            partial_prepare_hash: None,
            partial_original: false,
            partial_original_hash: None,
            writer_keys: Vec::new(),
            writer_stopped: false,
        });
        let mut pending_refusal = false;
        #[cfg(debug_assertions)]
        if fresh {
            fresh_checkpoint(
                &owned.ancestors.last().unwrap().file,
                fresh_qualification::Fault::ControllerFull,
            );
        }
        let mut pending_present = false;
        let result = (|| {
            let parent = &owned.ancestors.last().unwrap().file;
            for name in [
                "client.import-intent",
                "client.import-original",
                "client.import-source",
                "client.import-target",
            ] {
                if optional(parent, name)
                    .map_err(|cause| explain(ErrorKind::Unavailable, cause))?
                    .is_some()
                {
                    pending_refusal = true;
                    return Err(err(ErrorKind::RequiresLegacyReview));
                }
            }
            for name in [
                "writer.intent",
                "writer.original",
                "writer.prepare",
                "writer.birth",
                "snapshot.intent",
                "recovery.intent",
                "recovery.next",
            ] {
                if optional(parent, name)
                    .map_err(|cause| explain(ErrorKind::Unavailable, cause))?
                    .is_some()
                {
                    pending_present = true;
                    if !recovery || name == "snapshot.intent" {
                        pending_refusal = true;
                        return Err(err(ErrorKind::RecoveryRequired));
                    }
                }
            }
            let directory = relative(
                parent,
                "docker",
                true,
                if fresh { FILE_CREATE } else { FILE_OPEN },
                if fresh { Some(&dir_sd) } else { None },
                false,
            )
            .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?;
            owned.ancestors.push(
                Held::capture(directory, &owned.user, true, true)
                    .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?,
            );
            #[cfg(debug_assertions)]
            if fresh {
                fresh_checkpoint(owned.directory(), fresh_qualification::Fault::DockerFull);
            }
            let root_id = owned.ancestors[owned.ancestors.len() - 2].id;
            let binding = serde_json::json!({"schema":1,"product":"orbwin","ownerSid":owned.user.text,"rootVolume":root_id.volume,"rootHigh":root_id.high,"rootLow":root_id.low,"admission":"fresh"});
            let init = if fresh {
                create_data(
                    owned.parent(),
                    "client.init",
                    &serde_json::to_vec(&binding).unwrap(),
                    &owned.user,
                )
                .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?
            } else {
                Data::capture(
                    relative(owned.parent(), "client.init", false, FILE_OPEN, None, false)
                        .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?,
                    &owned.user,
                )
                .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?
            };
            let init_body =
                body(&init.file).map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?;
            if checked_json(&init_body)
                .map_err(|_| explain(ErrorKind::RecoveryRequired, bad("initialization JSON")))?
                != binding
                && legacy_import::parse_completed(
                    &init_body,
                    &owned.user.text,
                    &legacy_import::Fid {
                        volume: root_id.volume,
                        high: root_id.high,
                        low: root_id.low,
                    },
                    &profile,
                )
                .is_err()
            {
                return Err(explain(
                    ErrorKind::RecoveryRequired,
                    bad("initialization binding mismatch"),
                ));
            }
            owned.init = Some(init);
            #[cfg(debug_assertions)]
            if fresh {
                fresh_checkpoint(owned.parent(), fresh_qualification::Fault::InitFull);
            }
            let config = if fresh {
                create_data(
                    owned.directory(),
                    "config.json",
                    &serde_json::to_vec(&defaults(&owned.plugins)).unwrap(),
                    &owned.user,
                )
                .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?
            } else {
                Data::capture_writer(
                    relative(
                        owned.directory(),
                        "config.json",
                        false,
                        FILE_OPEN,
                        None,
                        false,
                    )
                    .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?,
                    &owned.user,
                )
                .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?
            };
            owned.body =
                body(&config.file).map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?;
            owned.config = Some(config);
            #[cfg(debug_assertions)]
            if fresh {
                fresh_checkpoint(owned.directory(), fresh_qualification::Fault::ConfigFull);
            }
            if recovery {
                if !pending_present {
                    return Err(err(ErrorKind::NoInterruptedWrite));
                }
                owned.load_recovery()?;
                return Ok(());
            }
            if fresh {
                erase(
                    &owned.ancestors[owned.ancestors.len() - 3].file,
                    "client.create-intent",
                    &owned.user,
                )
                .map_err(|cause| explain(ErrorKind::RecoveryRequired, cause))?
            }
            if fresh {
                parse_config(&owned.body, &owned.plugins)?
            } else if let Some(next) = refresh_managed_field(&owned.body, &owned.plugins, &profile)?
            {
                owned.poison = false;
                owned.replace_config(&next)?;
            }
            owned.poison = false;
            owned.check()
        })();
        if let Err(error) = result {
            if pending_refusal || recovery {
                release_pending(owned)?;
            } else {
                Box::leak(owned);
            }
            return Err(error);
        }
        Ok(ClientConfigGuard {
            path: profile
                .join(".orbwin-private")
                .join("client")
                .join("docker"),
            inner: Some(owned),
        })
    }
}

#[cfg(test)]
mod tests {
    #[cfg(debug_assertions)]
    #[test]
    fn historical_qualification_encoder_uses_closed_schema1_without_changing_live_plan() {
        let intent = serde_json::json!({"schema":3,"kind":"docker-writer","mutation":"registry-entries-v2",
            "binding":"1".repeat(64),"keysSha256":"2".repeat(64),"baseSha256":"3".repeat(64),
            "baseVolume":1,"baseHigh":2,"baseLow":3,"baseSdSha256":"4".repeat(64),
            "originalVolume":5,"originalHigh":6,"originalLow":7,"originalSdSha256":"5".repeat(64),"originalSha256":"3".repeat(64)});
        let base = RestoreIntent {
            schema: 2,
            kind: "restore-original".into(),
            binding: "1".repeat(64),
            current_volume: 1,
            current_high: 2,
            current_low: 3,
            current_sd_sha256: "2".repeat(64),
            current_sha256: "3".repeat(64),
            restore_sha256: "4".repeat(64),
            intent_body: String::from_utf8(go_json(&intent).unwrap()).unwrap(),
        };
        let live = go_json(&base).unwrap();
        let bytes = historical_qualification_bytes(&base).unwrap();
        let fields = raw_fields(&bytes).unwrap();
        assert_eq!(fields.len(), 10);
        let mut parsed: RestoreIntent = serde_json::from_slice(&bytes).unwrap();
        assert_eq!(parsed.schema, 1);
        assert!(successor::parse(&bytes).unwrap().v2.is_none());
        parsed.schema = 2;
        assert_eq!(
            go_json(&parsed).unwrap(),
            live,
            "every bound field remains unchanged"
        );
        assert_eq!(
            base.schema, 2,
            "historical injection cannot mutate live plan"
        );
        let mut invalid = base.clone();
        invalid.intent_body = "{}".into();
        assert!(historical_qualification_bytes(&invalid).is_err());
        let hex: String = bytes.iter().map(|b| format!("{b:02x}")).collect();
        println!("HISTORICAL_CODEC_GOLDEN_HEX {hex}");
    }
    #[test]
    fn local_eof_actual_dispatch_preserves_every_independent_writer_route() {
        let hash = "a".repeat(64);
        let mut siblings = Vec::new();
        for field in 0..5 {
            let mut s = RecoverySiblings::default();
            match field {
                0 => s.prepare = true,
                1 => s.original = true,
                2 => s.birth = true,
                3 => s.recovery = true,
                _ => {
                    s.next = true;
                    s.recovery = true;
                }
            }
            siblings.push(s);
        }
        for next in [None, Some("b".repeat(64))] {
            let bytes = local_replace_producer(&hash, next.as_deref()).unwrap();
            assert_eq!(
                recovery_route(Some(&bytes), RecoverySiblings::default(), &hash).unwrap(),
                RecoveryRoute::LocalFull
            );
            for cut in 0..bytes.len() {
                assert!(local_eof_prefix(&bytes[..cut], &hash));
                assert_eq!(
                    recovery_route(Some(&bytes[..cut]), RecoverySiblings::default(), &hash)
                        .unwrap(),
                    RecoveryRoute::LocalEof
                );
                for s in &siblings {
                    assert_eq!(
                        recovery_route(Some(&bytes[..cut]), *s, &hash).unwrap(),
                        RecoveryRoute::Writer
                    );
                }
            }
            for s in &siblings {
                assert!(recovery_route(Some(&bytes), *s, &hash).is_err());
            }
            assert!(!local_eof_prefix(
                &bytes[..bytes.len() - 1],
                &"c".repeat(64)
            ));
        }
        assert!(recovery_route(
            Some(b"{"),
            RecoverySiblings {
                next: true,
                ..Default::default()
            },
            &hash
        )
        .is_err());
        let writer = recovery_intent_fixture();
        for cut in 0..writer.len() {
            assert_eq!(
                recovery_route(
                    Some(&writer[..cut]),
                    RecoverySiblings {
                        prepare: true,
                        original: true,
                        ..Default::default()
                    },
                    &hash
                )
                .unwrap(),
                RecoveryRoute::Writer
            );
        }
        for wrong in [
            br#"{"baseSha256": "#.as_slice(),
            br#"{"kind":"config-replace","baseSha256":"#,
            b"[",
            b"\"",
            br#"{"schema":1,"kind":"config-replace"}"#,
        ] {
            assert!(!local_eof_prefix(wrong, &hash));
        }
    }
    #[test]
    fn local_eof_called_plan_requires_native_check_and_preserves_current_raw_values() {
        let profile = Path::new("C:\\Users\\fixture");
        let plugins = |key: char| {
            profile
                .join(".orbwin-private")
                .join("runtime")
                .join(format!("docker-{}", key.to_string().repeat(64)))
                .join("docker-cli-plugins")
        };
        let a = plugins('a');
        let c = plugins('c');
        let mut body = serde_json::to_vec(&defaults(&a)).unwrap();
        body.pop();
        body.extend(br#", "unrelated":1e9999}"#);
        let full = local_replace_producer(&successor::hash(&body), Some(&"b".repeat(64))).unwrap();
        let partial = &full[..full.len() - 1];
        let mut checked = false;
        let target = local_eof_plan(partial, &body, profile, &c, true, || {
            checked = true;
            Ok(())
        })
        .unwrap();
        assert!(checked);
        assert!(String::from_utf8(target.clone())
            .unwrap()
            .contains("1e9999"));
        parse_config(&target, &c).unwrap();
        assert!(
            local_eof_plan(partial, &body, profile, &c, false, || panic!(
                "strict native current is required first"
            ))
            .is_err()
        );
        assert!(
            local_eof_plan(partial, &body, profile, &c, true, || Err(err(
                ErrorKind::RecoveryRequired
            )))
            .is_err()
        );
        assert!(local_eof_plan(
            partial,
            &body,
            Path::new("C:\\Users\\other"),
            &c,
            true,
            || Ok(())
        )
        .is_err());
        assert!(!local_eof_prefix(
            &vec![b' '; 4097],
            &successor::hash(&body)
        ));
        let sequence = [
            successor::CompletedStep::Check,
            successor::CompletedStep::Cleanup,
            successor::CompletedStep::Refresh,
        ];
        for failure in 0..sequence.len() {
            let mut calls = Vec::new();
            assert!(successor::completed(|step| {
                calls.push(step);
                if calls.len() == failure + 1 {
                    Err(err(ErrorKind::RecoveryRequired))
                } else {
                    Ok(())
                }
            })
            .is_err());
            assert_eq!(calls, &sequence[..=failure]);
        }
    }
    #[test]
    fn target_publication_arms_before_attempt_and_no_suffix_failure_releases_graph() {
        let expected = [
            TargetPublishStep::Arm,
            TargetPublishStep::Attempt,
            TargetPublishStep::CloseStage,
            TargetPublishStep::Reopen,
            TargetPublishStep::Verify,
            TargetPublishStep::Complete,
        ];
        for failure in 1..expected.len() - 1 {
            let mut unknown = false;
            let mut held_stage = true;
            let mut called = Vec::new();
            let result = target_publish_sequence(|step| {
                called.push(step);
                if step == TargetPublishStep::Arm {
                    unknown = true;
                }
                if step == TargetPublishStep::Attempt {
                    assert!(unknown && held_stage);
                }
                if called.len() - 1 == failure {
                    return Err(err(ErrorKind::RecoveryRequired));
                }
                if step == TargetPublishStep::CloseStage {
                    held_stage = false;
                }
                if step == TargetPublishStep::Complete {
                    unknown = false;
                }
                Ok(())
            });
            assert!(result.is_err());
            assert!(unknown);
            assert_eq!(called, &expected[..=failure]);
            if failure == 1 {
                assert!(held_stage, "attempt failure retains the actual stage slot");
            }
        }
        let mut unknown = false;
        target_publish_sequence(|step| {
            match step {
                TargetPublishStep::Arm => unknown = true,
                TargetPublishStep::Attempt
                | TargetPublishStep::CloseStage
                | TargetPublishStep::Reopen
                | TargetPublishStep::Verify => assert!(unknown),
                TargetPublishStep::Complete => unknown = false,
            }
            Ok(())
        })
        .unwrap();
        assert!(!unknown);
    }
    #[test]
    fn snapshot_arc_release_has_one_atomic_final_winner_and_no_weak_resurrection() {
        use std::sync::{
            atomic::{AtomicUsize, Ordering},
            Arc, Barrier,
        };
        struct Payload(Arc<AtomicUsize>);
        impl Drop for Payload {
            fn drop(&mut self) {
                self.0.fetch_add(1, Ordering::SeqCst);
            }
        }
        let drops = Arc::new(AtomicUsize::new(0));
        let graph = Arc::new(Payload(drops.clone()));
        let weak = Arc::downgrade(&graph);
        let barrier = Arc::new(Barrier::new(16));
        let retired = Arc::new(AtomicUsize::new(0));
        let mut workers = Vec::new();
        for _ in 0..16 {
            let actual = graph.clone();
            let barrier = barrier.clone();
            let retired = retired.clone();
            workers.push(std::thread::spawn(move || {
                barrier.wait();
                release_snapshot_arc(actual, |_actual_graph| {
                    retired.fetch_add(1, Ordering::SeqCst);
                    Ok(())
                })
                .unwrap()
            }));
        }
        drop(graph);
        let outcomes: Vec<_> = workers.into_iter().map(|w| w.join().unwrap()).collect();
        assert_eq!(
            outcomes
                .iter()
                .filter(|v| **v == SnapshotRelease::Retired)
                .count(),
            1
        );
        assert_eq!(retired.load(Ordering::SeqCst), 1);
        assert_eq!(drops.load(Ordering::SeqCst), 1);
        assert!(weak.upgrade().is_none());
        let graph = Arc::new(1);
        let weak = Arc::downgrade(&graph);
        assert_eq!(
            release_snapshot_arc(graph, |_| {
                assert!(weak.upgrade().is_none());
                Ok(())
            })
            .unwrap(),
            SnapshotRelease::Retired
        );
    }
    #[test]
    fn snapshot_shared_release_does_not_close_another_reader_and_final_error_is_reported() {
        use std::sync::Arc;
        let graph = Arc::new(7);
        let other = graph.clone();
        assert_eq!(
            release_snapshot_arc(graph, |_| panic!(
                "shared release must not close live graph"
            ))
            .unwrap(),
            SnapshotRelease::Shared
        );
        assert!(release_snapshot_arc(other, |actual| {
            assert_eq!(actual, 7);
            Err(err(ErrorKind::RecoveryRequired))
        })
        .is_err());
        let expected = [
            SnapshotCloseStep::Config,
            SnapshotCloseStep::Init,
            SnapshotCloseStep::Ancestor(2),
            SnapshotCloseStep::Ancestor(1),
            SnapshotCloseStep::Ancestor(0),
            SnapshotCloseStep::Token,
            SnapshotCloseStep::Roles,
        ];
        for failure in 0..expected.len() {
            let mut called = Vec::new();
            assert!(snapshot_close_sequence(3, |step| {
                called.push(step);
                if called.len() == failure + 1 {
                    Err(err(ErrorKind::RecoveryRequired))
                } else {
                    Ok(())
                }
            })
            .is_err());
            assert_eq!(called, &expected[..=failure]);
        }
        let mut called = Vec::new();
        snapshot_close_sequence(3, |step| {
            called.push(step);
            Ok(())
        })
        .unwrap();
        assert_eq!(called, expected);
    }
    #[cfg(windows)]
    #[test]
    fn retained_config_directories_participate_in_sharing_without_implicit_listing() {
        use windows_sys::Win32::Storage::FileSystem::{FILE_LIST_DIRECTORY, FILE_TRAVERSE};
        assert_ne!(
            super::native::retained_directory_access(false) & FILE_TRAVERSE,
            0
        );
        assert_eq!(
            super::native::retained_directory_access(false) & FILE_LIST_DIRECTORY,
            0
        );
        assert_ne!(
            super::native::retained_directory_access(true) & FILE_LIST_DIRECTORY,
            0
        );
    }
    #[test]
    fn fresh_initialization_recognizes_only_literal_marker_and_native_bound_init_prefixes() {
        for end in 0..=FRESH_MARKER.len() {
            assert!(fresh_marker(&FRESH_MARKER[..end]));
        }
        for bad in [
            br#"{"schema":2}"#.as_slice(),
            br#"{"kind":"fresh-client"}"#,
            b" ",
            b"[]",
            b"null",
            br#"{"schema":1,"kind":"fresh-client"} "#,
        ] {
            assert!(!fresh_marker(bad));
        }
        let expected = serde_json::json!({"schema":1,"product":"orbwin","ownerSid":"S-1-5-21-123","rootVolume":42,"rootHigh":1,"rootLow":2,"admission":"fresh"});
        let fields: Vec<_> = expected
            .as_object()
            .unwrap()
            .iter()
            .map(|(key, value)| {
                format!(
                    "{}:{}",
                    serde_json::to_string(key).unwrap(),
                    serde_json::to_string(value).unwrap()
                )
            })
            .collect();
        fn permute(fields: &[String], used: usize, parts: &mut Vec<String>, expected: &Value) {
            if parts.len() == fields.len() {
                let bytes = format!("{{{}}}", parts.join(",")).into_bytes();
                assert!(fresh_init_bytes(&bytes, expected));
                assert!(fresh_init_bytes(&bytes[..bytes.len() - 1], expected));
                return;
            }
            for (i, field) in fields.iter().enumerate() {
                if used & (1 << i) == 0 {
                    parts.push(field.clone());
                    permute(fields, used | (1 << i), parts, expected);
                    parts.pop();
                }
            }
        }
        permute(&fields, 0, &mut Vec::new(), &expected);
        let bytes = serde_json::to_vec(&expected).unwrap();
        for end in 0..=bytes.len() {
            assert!(fresh_init_bytes(&bytes[..end], &expected), "prefix {end}");
        }
        for change in [
            "rootVolume",
            "rootHigh",
            "rootLow",
            "schema",
            "ownerSid",
            "product",
            "admission",
        ] {
            let mut wrong = expected.clone();
            wrong[change] = serde_json::json!("foreign");
            assert!(!fresh_init_bytes(
                &serde_json::to_vec(&wrong).unwrap(),
                &expected
            ));
        }
        assert!(!fresh_init_bytes(br#"{"schema":1,"schema":1}"#, &expected));
        assert!(!fresh_init_bytes(br#"{"schema": 1,"rootHigh":"#, &expected));
    }
    #[test]
    fn fresh_defaults_prefixes_do_not_guess_credentials_or_foreign_data() {
        let profile = Path::new("C:\\Users\\A&B\u{2028}");
        let plugin = profile
            .join(".orbwin-private")
            .join("runtime")
            .join(format!("docker-{}", "a".repeat(64)))
            .join("docker-cli-plugins");
        let expected = defaults(&plugin);
        for bytes in [
            serde_json::to_vec(&expected).unwrap(),
            go_json(&expected).unwrap(),
        ] {
            for end in 0..=bytes.len() {
                assert!(fresh_default_bytes(&bytes[..end], profile), "prefix {end}");
            }
        }
        for wrong in [
            serde_json::json!({"auths":{"private":{"auth":"secret"}},"credsStore":"orbwin","cliPluginsExtraDirs":[plugin]}),
            serde_json::json!({"auths":{},"credsStore":"foreign","cliPluginsExtraDirs":[plugin]}),
            serde_json::json!({"auths":{},"credsStore":"orbwin","cliPluginsExtraDirs":["C:\\foreign"]}),
            serde_json::json!({"auths":{},"credsStore":"orbwin","cliPluginsExtraDirs":[plugin],"userData":true}),
            serde_json::json!({"Auths":{},"credsStore":"orbwin","cliPluginsExtraDirs":[plugin]}),
        ] {
            assert!(!fresh_default_bytes(
                &serde_json::to_vec(&wrong).unwrap(),
                profile
            ));
        }
        let canonical = plugin.to_str().unwrap();
        for alias in [
            canonical.replace("\\runtime\\", "\\.\\runtime\\"),
            canonical.replace("\\runtime\\", "\\\\runtime\\"),
            canonical.replace('\\', "/"),
        ] {
            #[cfg(windows)]
            assert_eq!(Path::new(&alias), plugin.as_path());
            let mut wrong = expected.clone();
            wrong["cliPluginsExtraDirs"] = serde_json::json!([alias]);
            assert!(!fresh_default_bytes(
                &serde_json::to_vec(&wrong).unwrap(),
                profile
            ));
        }
        let long_profile = PathBuf::from(format!(
            "C:\\Users\\{}",
            vec!["\u{2028}".repeat(64); 12].join("\\")
        ));
        let long_plugin = long_profile
            .join(".orbwin-private")
            .join("runtime")
            .join(format!("docker-{}", "b".repeat(64)))
            .join("docker-cli-plugins");
        let long_bytes = go_json(&defaults(&long_plugin)).unwrap();
        assert!(long_bytes.len() > 4096 && long_bytes.len() <= FRESH_RECORD_BUDGET);
        for end in [long_bytes.len() / 2, long_bytes.len() - 1, long_bytes.len()] {
            assert!(fresh_default_bytes(&long_bytes[..end], &long_profile));
        }
        let zero = profile
            .join(".orbwin-private")
            .join("runtime")
            .join(format!("docker-{}", "0".repeat(64)))
            .join("docker-cli-plugins");
        let bytes = serde_json::to_vec(&defaults(&zero)).unwrap();
        assert!(!fresh_default_bytes(&bytes, profile));
        assert!(!fresh_default_bytes(&bytes[..bytes.len() - 1], profile));
        for bad in [
            br#"{"auths":{"name":"#.as_slice(),
            br#"{"auths":{},"auths":{}"#,
            b" ",
            b"[]",
            br#"{"auths": {},"cliPlugins"#,
        ] {
            assert!(!fresh_default_bytes(bad, profile));
        }
    }
    #[test]
    fn local_replace_closed_cross_owner_records() {
        use sha2::{Digest, Sha256};
        let base = b"unchanged owned config";
        let next = b"published owned config";
        let record = serde_json::json!({"schema":1,"kind":"config-replace",
            "baseSha256":format!("{:x}",Sha256::digest(base)),
            "nextSha256":format!("{:x}",Sha256::digest(next))});
        let bytes = serde_json::to_vec(&record).unwrap();
        let parsed = parse_local_replace(&bytes).unwrap();
        assert!(local_replace_current(&parsed, base).is_ok());
        assert!(local_replace_current(&parsed, next).is_ok());
        assert!(local_replace_current(&parsed, b"unbound current").is_err());
        let mut go = record.clone();
        go.as_object_mut().unwrap().remove("nextSha256");
        let parsed = parse_local_replace(&serde_json::to_vec(&go).unwrap()).unwrap();
        // The Go producer never recorded its target. This is KEEP CURRENT,
        // subject to independent native provenance and config validation.
        assert!(local_replace_current(&parsed, next).is_ok());
        assert!(parsed.next.is_none());
        assert!(parse_recovery_intent(&bytes).is_err());
    }
    #[test]
    fn local_replace_refuses_unbound_or_ambiguous_records() {
        let hash = "a".repeat(64);
        let base = format!(r#"{{"schema":1,"kind":"config-replace","baseSha256":"{hash}"}}"#);
        for invalid in [
            base.replace("\"schema\":1", "\"schema\":2"),
            base.replace("config-replace", "docker-writer"),
            base.replace(&hash, &"A".repeat(64)),
            base.replace(&hash, "abc"),
            base.replace("}", ",\"nextSha256\":null}"),
            base.replace("}", ",\"next_sha256\":\"x\"}"),
            base.replace("}", ",\"schema\":1}"),
            base.replace("}", ",\"extra\":false}"),
            base[..base.len() - 1].to_string(),
            format!("{base} null"),
        ] {
            assert!(
                parse_local_replace(invalid.as_bytes()).is_err(),
                "{invalid}"
            );
        }
        assert!(parse_local_replace(&vec![b' '; 4097]).is_err());
    }
    use super::*;
    fn recovery_intent_fixture() -> Vec<u8> {
        serde_json::to_vec(&serde_json::json!({"schema":3,"kind":"docker-writer","mutation":"registry-entries-v2",
            "binding":"01".repeat(32),"keysSha256":"02".repeat(32),"baseSha256":"03".repeat(32),
            "baseVolume":1,"baseHigh":2,"baseLow":3,"baseSdSha256":"04".repeat(32),
            "originalVolume":1,"originalHigh":2,"originalLow":4,"originalSdSha256":"05".repeat(32),"originalSha256":"03".repeat(32)
        })).unwrap()
    }
    #[test]
    fn recovery_protocol_rejects_legacy_unknown_duplicate_and_unbound_intents() {
        let valid = recovery_intent_fixture();
        assert!(parse_recovery_intent(&valid).is_ok());
        for (field, value) in [
            ("schema", serde_json::json!(2)),
            ("unknown", serde_json::json!(true)),
            ("binding", serde_json::json!("00".repeat(32))),
            ("originalSha256", serde_json::json!("09".repeat(32))),
            ("baseSdSha256", serde_json::json!("AB".repeat(32))),
        ] {
            let mut corrupted: Value = serde_json::from_slice(&valid).unwrap();
            corrupted[field] = value;
            assert!(
                parse_recovery_intent(&serde_json::to_vec(&corrupted).unwrap()).is_err(),
                "{field}"
            );
        }
        let mut duplicate = valid.clone();
        duplicate.pop();
        duplicate.extend_from_slice(b",\"schema\":3}");
        assert!(parse_recovery_intent(&duplicate).is_err());
        assert!(parse_recovery_intent(&vec![b' '; 4097]).is_err());
    }
    #[test]
    fn recovery_birth_binds_exact_intent_and_native_generation_metadata() {
        use sha2::{Digest, Sha256};
        let intent = recovery_intent_fixture();
        let binding = "01".repeat(32);
        let value = serde_json::json!({"schema":3,"kind":"docker-writer-birth","binding":binding,
            "intentSha256":format!("{:x}",Sha256::digest(&intent)),"intentBody":String::from_utf8(intent.clone()).unwrap(),
            "rootPid":42,"rootCreated":99,"rootSession":3});
        let parse = |value: &Value| serde_json::from_value::<RecoveryBirth>(value.clone()).unwrap();
        assert!(validate_recovery_birth(&parse(&value), &intent, &binding).is_ok());
        for (field, replacement) in [
            ("rootPid", serde_json::json!(0)),
            ("rootCreated", serde_json::json!(0)),
            ("binding", serde_json::json!("09".repeat(32))),
            ("intentBody", serde_json::json!("{}")),
            ("intentSha256", serde_json::json!("00".repeat(32))),
        ] {
            let mut corrupted = value.clone();
            corrupted[field] = replacement;
            assert!(
                validate_recovery_birth(&parse(&corrupted), &intent, &binding).is_err(),
                "{field}"
            );
        }
        let mut drift = intent.clone();
        drift.push(b' ');
        assert!(validate_recovery_birth(&parse(&value), &drift, &binding).is_err());
        let mut unknown = value;
        unknown["stopped"] = serde_json::json!(true);
        assert!(serde_json::from_value::<RecoveryBirth>(unknown).is_err());
    }
    #[test]
    fn incomplete_birth_is_only_a_protocol_cutpoint_not_a_stopped_process_flag() {
        let bytes=br#"{"schema":3,"kind":"docker-writer-birth","binding":"x","intentSha256":"x","intentBody":"{}","rootPid":42,"rootCreated":99,"rootSession":3}"#;
        for end in 0..bytes.len() {
            assert!(
                parse_birth_record(&bytes[..end]).unwrap().is_none(),
                "prefix {end}"
            );
        }
        assert!(parse_birth_record(bytes).unwrap().is_some());
        for malformed in [b"null".as_slice(), b"{}", b"{\"stopped\":true}", b"garbage"] {
            assert!(parse_birth_record(malformed).is_err());
        }
        assert!(parse_birth_record(&vec![b' '; 8193]).is_err());
        for malformed in [b"[".as_slice(), b"\"", b"nu", b"1e"] {
            assert!(!incomplete_record(malformed));
        }
    }
    #[test]
    fn managed_refresh_matches_go_json_html_and_separator_escaping() {
        let profile = Path::new(r"C:\Users\A&B<j>");
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
        let mut bytes=serde_json::to_vec(&serde_json::json!({"auths":{},"credsStore":"orbwin","cliPluginsExtraDirs":[old],"x<&>\u{2028}\u{2029}":{"raw":"<&>"}})).unwrap();
        bytes.pop();
        bytes.extend_from_slice(br#", "unknownHuge":1e9999}"#);
        let refreshed = refresh_managed_field(&bytes, &next, profile)
            .unwrap()
            .unwrap();
        let text = String::from_utf8(refreshed).unwrap();
        assert!(text.contains(r"A\u0026B\u003cj\u003e"));
        assert!(text.contains(r"x\u003c\u0026\u003e\u2028\u2029"));
        assert!(text.contains(r#"{"raw":"<&>"}"#));
        assert!(text.contains("1e9999"));
    }
    #[test]
    fn writer_key_digest_bytes_match_go_metadata_encoding() {
        let keys = vec!["a<&>".to_string(), "é\u{2028}\u{2029}".to_string()];
        assert_eq!(
            writer_keys_json(&keys).unwrap(),
            r#"["a\u003c\u0026\u003e","é\u2028\u2029"]"#.as_bytes()
        );
        assert_eq!(writer_keys_json(&[]).unwrap(), b"[]");
    }
    #[test]
    fn registry_auth_merge_preserves_unrelated_raw_values_after_typed_child_save() {
        let base=br#"{"credsStore":"orbwin","cliPluginsExtraDirs":["pinned"],"auths":{"old":{}},"unknown":1e9999,"unknown":123456789012345678901234567890,"HttpHeaders":{"User-Agent":"keep","X":"a","X":"b"},"nested":{"same":1,"same":2}}"#;
        let child=br#"{"credsStore":"orbwin","cliPluginsExtraDirs":["pinned"],"auths":{"new":{}},"HttpHeaders":{"X":"b"},"psFormat":"unowned-change"}"#;
        let merged =
            merge_registry_auths(base, child, Path::new("pinned"), &["new".into()]).unwrap();
        let text = std::str::from_utf8(&merged).unwrap();
        for exact in [
            "\"unknown\":1e9999",
            "\"unknown\":123456789012345678901234567890",
            "\"User-Agent\":\"keep\"",
            "\"X\":\"a\",\"X\":\"b\"",
            "\"same\":1,\"same\":2",
            "\"new\":{}",
        ] {
            assert!(text.contains(exact), "{exact}");
        }
        assert!(!text.contains("unowned-change"));
        assert!(merge_registry_auths(
            base,
            br#"{"credsStore":"orbwin","cliPluginsExtraDirs":["pinned"]}"#,
            Path::new("pinned"),
            &["new".into()]
        )
        .is_err());
        assert!(merge_registry_auths(
            base,
            br#"{"credsStore":"foreign","cliPluginsExtraDirs":["pinned"],"auths":{}}"#,
            Path::new("pinned"),
            &["new".into()]
        )
        .is_err());
    }
    #[test]
    fn registry_entry_authority_retains_other_raw_credentials() {
        let base=br#"{"credsStore":"orbwin","cliPluginsExtraDirs":["pinned"],"auths":{"untouched":{"auth":"opaque","future":1e9999,"same":1,"same":2},"target":{"old":"replace"},"legacy-target":{"old":"delete"}}}"#;
        let child=br#"{"credsStore":"orbwin","cliPluginsExtraDirs":["pinned"],"auths":{"untouched":{"auth":"opaque"},"target":{"auth":"new"},"unowned":{"auth":"injected"}}}"#;
        let merged = merge_registry_auths(
            base,
            child,
            Path::new("pinned"),
            &["target".into(), "legacy-target".into()],
        )
        .unwrap();
        let text = std::str::from_utf8(&merged).unwrap();
        assert!(text.contains(r#""untouched":{"auth":"opaque","future":1e9999,"same":1,"same":2}"#));
        assert!(text.contains(r#""target":{"auth":"new"}"#));
        assert!(!text.contains("unowned") && !text.contains("legacy-target"));
        assert!(merge_registry_auths(base, child, Path::new("pinned"), &[]).is_err());
    }
    #[test]
    fn ui_registry_grant_is_bound_to_owned_target_and_exact_login_shape() {
        let prefix = [
            "--config",
            "private",
            "--host",
            "npipe:////./pipe/docker-rancher-desktop-orbwin",
            "login",
            "--username",
            "user",
            "--password-stdin",
            "docker.io",
        ];
        let mut args: Vec<OsString> = prefix.iter().map(OsString::from).collect();
        assert!(validate_registry_login(
            Path::new("private"),
            &["https://index.docker.io/v1/".into()],
            &args
        )
        .is_ok());
        assert!(
            validate_registry_login(Path::new("private"), &["unrelated".into()], &args).is_err()
        );
        args[1] = "foreign".into();
        assert!(validate_registry_login(
            Path::new("private"),
            &["https://index.docker.io/v1/".into()],
            &args
        )
        .is_err());
        args[1] = "private".into();
        args[8] = "localhost:5000".into();
        assert!(
            validate_registry_login(Path::new("private"), &["localhost:5000".into()], &args)
                .is_ok()
        );
        args[7] = "--password".into();
        assert!(
            validate_registry_login(Path::new("private"), &["localhost:5000".into()], &args)
                .is_err()
        );
    }
    #[test]
    fn writer_commit_faults_never_reach_later_barriers() {
        let expected = [
            WriterCommitStep::AdmitWritable,
            WriterCommitStep::Flush,
            WriterCommitStep::RecheckFlushed,
            WriterCommitStep::CloseWritable,
            WriterCommitStep::AdmitReadOnly,
        ];
        for failure in 0..expected.len() {
            let mut seen = Vec::new();
            assert!(writer_durability_sequence(|stage| {
                seen.push(stage);
                if stage == expected[failure] {
                    Err(err(ErrorKind::RecoveryRequired))
                } else {
                    Ok(())
                }
            })
            .is_err());
            assert_eq!(seen, expected[..=failure]);
        }
        let mut seen = Vec::new();
        writer_durability_sequence(|stage| {
            seen.push(stage);
            Ok(())
        })
        .unwrap();
        assert_eq!(seen, expected);
    }
    #[test]
    fn restricting_sid_contract_rejects_nonempty_and_malformed_native_results() {
        assert!(no_restricting_sids(&[0; 256], 4));
        assert!(no_restricting_sids(&[0; 256], 2048));
        for length in [0, 3, 2049, u32::MAX] {
            assert!(!no_restricting_sids(&[0; 256], length));
        }
        assert!(!no_restricting_sids(&[], 4));
        assert!(!no_restricting_sids(&[1; 256], 4));
        assert!(!no_restricting_sids(&[u64::MAX; 256], 2048));
    }
    #[test]
    fn raw_docker_aliases_and_duplicates_never_authorize_routing() {
        let plugins = Path::new("fixed-cached-plugins");
        let good = serde_json::to_vec(&defaults(plugins)).unwrap();
        assert!(parse_config(&good, plugins).is_ok());
        for bytes in [
            br#"{"credsStore":"orbwin","CredsStore":"foreign"}"#.as_slice(),
            br#"{"credsStore":"orbwin","credsStore":"foreign"}"#,
            br#"{"cliPluginsExtraDirs":[],"cliPluginsextraDirs":["foreign"]}"#,
            br#"{"auths":{},"AUTHS":{}}"#,
            br#"{"auths":{"registry":{"auth":"a","auth":"b"}}}"#,
        ] {
            assert!(parse_config(bytes, plugins).is_err())
        }
    }
    #[test]
    fn fixed_product_routing_refuses_ambient_helper_selection() {
        let plugins = Path::new("fixed-cached-plugins");
        let mut v = defaults(plugins);
        v["credHelpers"] = serde_json::json!({"foreign":"another"});
        assert!(routing(&v, plugins).is_err());
        v["credHelpers"] = serde_json::json!({"owned":"orbwin"});
        assert!(routing(&v, plugins).is_ok());
        v["cliPluginsExtraDirs"] = serde_json::json!(["loose"]);
        assert!(routing(&v, plugins).is_err());
    }
    #[test]
    fn unrelated_config_values_are_not_extension_authority() {
        let v = checked_json(br#"{"unknown":{"selected":"preserved-data"},"auths":{}}"#).unwrap();
        assert_eq!(v["unknown"]["selected"], "preserved-data");
        assert!(routing(&v, Path::new("fixed")).is_err())
    }
    #[test]
    fn unknown_raw_lexemes_and_header_keys_are_preserved() {
        let plugins = Path::new("fixed");
        let bytes=br#"{"credsStore":"orbwin","cliPluginsExtraDirs":["fixed"],"unknown":1e9999,"huge":1234567890123456789012345678901234567890,"HttpHeaders":{"CreDsStoRe":"header-data"},"other":{"credsStore":1e9999,"same":1,"same":2},"auths":{"registry":{"username":"u","unrelated":1e9999}}}"#;
        assert!(parse_config(bytes, plugins).is_ok());
        assert!(std::str::from_utf8(bytes).unwrap().contains("1e9999"));
        let alias=br#"{"credsStore":"orbwin","cliPluginsExtraDirs":["fixed"],"auths":{"registry":{"username":"u","UserName":"v"}}}"#;
        assert!(parse_config(alias, plugins).is_err());
    }
    #[test]
    fn managed_field_refresh_is_only_private_product_shape_and_preserves_raw_data() {
        let profile = Path::new("native-profile");
        let old = profile
            .join(".orbwin-private")
            .join("runtime")
            .join(format!("docker-{}", "a".repeat(64)))
            .join("docker-cli-plugins");
        let current = profile
            .join(".orbwin-private")
            .join("runtime")
            .join(format!("docker-{}", "b".repeat(64)))
            .join("docker-cli-plugins");
        let mut bytes = serde_json::to_vec(&defaults(&old)).unwrap();
        bytes.pop();
        bytes.extend(br#", "unknown":1e9999,"authsExtra":{"credsStore":"inert-data"}}"#);
        let refreshed = refresh_managed_field(&bytes, &current, profile)
            .unwrap()
            .unwrap();
        assert!(parse_config(&refreshed, &current).is_ok());
        assert!(String::from_utf8(refreshed).unwrap().contains("1e9999"));
        for path in [
            "foreign/plugins",
            "native-profile/.orbwin-private/runtime/docker-../docker-cli-plugins",
            "native-profile/.orbwin-private/runtime/docker-000/docker-cli-plugins",
        ] {
            let v = serde_json::to_vec(&defaults(Path::new(path))).unwrap();
            assert!(refresh_managed_field(&v, &current, profile).is_err())
        }
        let mut v = defaults(&old);
        v["credsStore"] = serde_json::json!("foreign");
        assert!(
            refresh_managed_field(&serde_json::to_vec(&v).unwrap(), &current, profile).is_err()
        );
        let mut v = defaults(&old);
        v["cliPluginsExtraDirs"] = serde_json::json!([old, current]);
        assert!(
            refresh_managed_field(&serde_json::to_vec(&v).unwrap(), &current, profile).is_err()
        );
    }
    #[test]
    fn docker_unicode_simple_fold_aliases_are_schema_scoped() {
        let plugins = Path::new("fixed");
        for bytes in ["{\"credſStore\":\"foreign\",\"credsStore\":\"orbwin\",\"cliPluginsExtraDirs\":[\"fixed\"]}","{\"credsStore\":\"orbwin\",\"cliPluginsExtraDirs\":[\"fixed\"],\"auths\":{\"r\":{\"identitytoKen\":\"x\"}}}"]{assert!(parse_config(bytes.as_bytes(),plugins).is_err())}
        assert!(parse_config("{\"credsStore\":\"orbwin\",\"cliPluginsExtraDirs\":[\"fixed\"],\"HttpHeaders\":{\"credſStore\":\"inert\"}}".as_bytes(),plugins).is_ok());
    }
    #[test]
    fn writer_environment_binds_private_selectors_and_helper_graph() {
        let config = Path::new("private");
        let plugins = Path::new("pinned");
        let docker = Path::new("docker").join("docker.exe");
        let credentials = Path::new("helper").join("docker-credential-orbwin.exe");
        let path = std::env::join_paths([Path::new("helper"), Path::new("docker")]).unwrap();
        let env: Vec<(OsString, OsString)> = vec![
            ("DOCKER_CONFIG".into(), "private".into()),
            (
                "DOCKER_HOST".into(),
                "npipe:////./pipe/docker-rancher-desktop-orbwin".into(),
            ),
            ("ORBWIN_DOCKER_MANAGED_PLUGINS".into(), "pinned".into()),
            ("PATHEXT".into(), ".EXE".into()),
            ("NoDefaultCurrentDirectoryInExePath".into(), "1".into()),
            ("PATH".into(), path),
            ("=C:".into(), "C:\\work".into()),
        ];
        let check = |env: &[(OsString, OsString)]| {
            validate_writer_environment(env, config, plugins, &docker, &credentials)
        };
        assert!(check(&env).is_ok());
        for entry in [
            ("path", "foreign"),
            ("DOCKER_CONTEXT", "foreign"),
            ("DOCKER_AUTH_CONFIG", "opaque"),
            ("=bad", "foreign"),
        ] {
            let mut changed = env.clone();
            changed.push((entry.0.into(), entry.1.into()));
            assert!(check(&changed).is_err());
        }
        for (index, value) in [
            (0, "public"),
            (2, "foreign"),
            (3, ".BAT"),
            (4, "0"),
            (5, "foreign"),
        ] {
            let mut changed = env.clone();
            changed[index].1 = value.into();
            assert!(check(&changed).is_err());
        }
        #[cfg(windows)]
        {
            use std::os::windows::ffi::OsStringExt;
            let mut changed = env.clone();
            changed.push(("AMBIENT".into(), OsString::from_wide(&[0xD800])));
            assert!(check(&changed).is_ok());
        }
    }
}
