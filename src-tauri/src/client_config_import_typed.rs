use super::*;
const AUTH: &[&str] = &[
    "username",
    "password",
    "auth",
    "serveraddress",
    "identitytoken",
    "registrytoken",
];
const PROXY: &[&str] = &["httpProxy", "httpsProxy", "noProxy", "ftpProxy", "allProxy"];
fn string_null(raw: &str) -> bool {
    raw == "null" || serde_json::from_str::<String>(raw).is_ok()
}
fn string_map(raw: &str) -> bool {
    if raw == "null" {
        return true;
    };
    let Ok(fields) = raw_fields(raw.as_bytes()) else {
        return false;
    };
    fields.iter().all(|(_, v)| string_null(v.get()))
}
fn string_array(raw: &str) -> bool {
    if raw == "null" {
        return true;
    };
    let Ok(values) = serde_json::from_str::<Vec<Box<serde_json::value::RawValue>>>(raw) else {
        return false;
    };
    values.iter().all(|v| string_null(v.get()))
}
fn typed_struct(raw: &str, known: &[&str]) -> bool {
    if raw == "null" {
        return true;
    };
    let Ok(fields) = raw_fields(raw.as_bytes()) else {
        return false;
    };
    if known_fields(&fields, known).is_err() {
        return false;
    };
    fields
        .iter()
        .all(|(k, v)| !known.contains(&k.as_str()) || string_null(v.get()))
}
// Do not collapse data-map duplicates: Docker retains a first type error even
// when a later occurrence has a valid type. Unknown opaque fields remain raw.
fn map_values(raw: &str, check: impl Fn(&str) -> bool) -> bool {
    if raw == "null" {
        return true;
    };
    let Ok(fields) = raw_fields(raw.as_bytes()) else {
        return false;
    };
    fields.iter().all(|(_, v)| check(v.get()))
}
pub(super) fn typed_target(bytes: &[u8]) -> bool {
    let Ok(fields) = raw_fields(bytes) else {
        return false;
    };
    if known_fields(&fields, ROOT_FIELDS).is_err() {
        return false;
    }
    fields.iter().all(|(key, value)| {
        let raw = value.get();
        match key.as_str() {
            "psFormat"
            | "imagesFormat"
            | "networksFormat"
            | "pluginsFormat"
            | "volumesFormat"
            | "statsFormat"
            | "detachKeys"
            | "credsStore"
            | "serviceInspectFormat"
            | "servicesFormat"
            | "tasksFormat"
            | "secretFormat"
            | "configFormat"
            | "nodesFormat"
            | "currentContext" => string_null(raw),
            "HttpHeaders" | "credHelpers" | "aliases" | "features" => string_map(raw),
            "pruneFilters" | "cliPluginsExtraDirs" => string_array(raw),
            "auths" => map_values(raw, |v| typed_struct(v, AUTH)),
            "proxies" => map_values(raw, |v| typed_struct(v, PROXY)),
            "plugins" => map_values(raw, string_map),
            _ => true,
        }
    })
}
