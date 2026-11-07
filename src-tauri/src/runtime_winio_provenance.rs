//! Build provenance DATA only. No filesystem, process or native capability.
//! The producer owns source reconstruction and actual Go metadata collection.
use serde_json::Value;
use sha2::{Digest, Sha256};
use std::collections::{BTreeMap, BTreeSet};

const MODULE: &str = "github.com/Microsoft/go-winio";
const MAIN: &str = "github.com/rancher-sandbox/rancher-desktop-daemon";
const VERSION: &str = "v0.6.4";
const RECIPES: &[&str] = &[
    "scripts/winio-build-input.mjs",
    "scripts/winio-build-input-fs.mjs",
    "scripts/lima-wsl2-build-input.mjs",
    "scripts/build-backend.mjs",
    "scripts/build-identity.mjs",
    "scripts/collect-notices.mjs",
];
const REQUIRED: &[&str] = &[
    "file.go",
    "file_original_close_state.go",
    "file_original_iocp_windows.go",
    "pipe.go",
    "pipe_dial_state.go",
    "pipe_original_dial_windows.go",
];
const LISTS: &[&str] = &["GoFiles", "CgoFiles", "CompiledGoFiles", "EmbedFiles"];
type Check<T> = Result<T, ()>;

fn require(value: bool) -> Check<()> {
    if value {
        Ok(())
    } else {
        Err(())
    }
}
fn closed(value: &Value, keys: &[&str]) -> Check<()> {
    let map = value.as_object().ok_or(())?;
    require(map.len() == keys.len() && keys.iter().all(|key| map.contains_key(*key)))
}
fn text(value: &Value) -> Check<&str> {
    value.as_str().ok_or(())
}
fn hash(value: &Value) -> Check<&str> {
    let value = text(value)?;
    require(
        value.len() == 64
            && value
                .bytes()
                .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
            && value.bytes().any(|b| b != b'0'),
    )?;
    Ok(value)
}
fn source_name(value: &str) -> Check<()> {
    require(
        !value.is_empty()
            && value.len() <= 1024
            && !value.contains(['\\', ':'])
            && value
                .split('/')
                .all(|part| !part.is_empty() && part != "." && part != "..")
            && !value.chars().any(char::is_control),
    )
}
// Compare recorded build paths only; never resolve or reopen them at runtime.
fn path(value: &Value) -> Check<String> {
    let value = text(value)?;
    require(value.len() >= 3 && value.len() < 32760 && !value.chars().any(char::is_control))?;
    let normalized = value.replace('\\', "/");
    require(
        normalized.as_bytes()[0].is_ascii_alphabetic()
            && normalized.as_bytes()[1] == b':'
            && normalized.as_bytes()[2] == b'/',
    )?;
    require(
        normalized[3..]
            .split('/')
            .all(|p| !p.is_empty() && p != "." && p != ".."),
    )?;
    Ok(normalized.to_ascii_lowercase())
}
fn array(value: &Value, bound: usize) -> Check<&Vec<Value>> {
    let values = value.as_array().ok_or(())?;
    require(values.len() <= bound)?;
    Ok(values)
}
fn module(value: &Value, root: &str) -> Check<()> {
    closed(value, &["Path", "Version", "Dir", "Replace"])?;
    require(
        value["Path"] == MODULE && value["Version"] == VERSION && path(&value["Dir"])? == root,
    )?;
    closed(&value["Replace"], &["Path", "Dir"])?;
    require(path(&value["Replace"]["Path"])? == root && path(&value["Replace"]["Dir"])? == root)
}
fn rows(value: &Value) -> Check<BTreeMap<String, (u64, String)>> {
    let mut out = BTreeMap::new();
    let mut folded = BTreeSet::new();
    for row in array(value, 4096)? {
        closed(row, &["path", "sha256", "bytes"])?;
        let name = text(&row["path"])?;
        source_name(name)?;
        let bytes = row["bytes"].as_u64().ok_or(())?;
        require(bytes <= 16 << 20 && folded.insert(name.to_ascii_lowercase()))?;
        require(
            out.insert(name.to_owned(), (bytes, hash(&row["sha256"])?.to_owned()))
                .is_none(),
        )?;
    }
    Ok(out)
}
fn validate(manifest: &Value, lock_bytes: &[u8], expected_bytes: &[u8]) -> Check<()> {
    require(lock_bytes.len() <= 2 << 20 && expected_bytes.len() <= 64 << 10)?;
    let lock: Value = serde_json::from_slice(lock_bytes).map_err(|_| ())?;
    let expected: Value = serde_json::from_slice(expected_bytes).map_err(|_| ())?;
    closed(
        &lock,
        &[
            "schema",
            "role",
            "reviewPeerSHA256",
            "sourceReceiptSHA256",
            "profile",
        ],
    )?;
    require(lock["schema"] == 1 && lock["role"] == "root-rdd-windows-amd64")?;
    let profile = &lock["profile"];
    require(
        profile["schema"] == 1
            && profile["module"] == MODULE
            && profile["version"] == VERSION
            && profile["mainModule"] == MAIN,
    )?;
    require(
        array(&profile["baseline"], 111)?.len() == 111
            && array(&profile["patched"], 118)?.len() == 118,
    )?;
    let reviewed = rows(&profile["patched"])?;
    closed(
        &expected,
        &[
            "schema",
            "lockSHA256",
            "module",
            "version",
            "upstreamZipSHA256",
            "patchedProjectionSHA256",
            "patchSHA256",
            "recipeHashes",
            "offerFiles",
            "reviewPeerSHA256",
            "sourceReceiptSHA256",
        ],
    )?;
    require(
        expected["schema"] == 1 && expected["module"] == MODULE && expected["version"] == VERSION,
    )?;
    require(hash(&expected["lockSHA256"])? == format!("{:x}", Sha256::digest(lock_bytes)))?;
    for key in [
        "upstreamZipSHA256",
        "patchedProjectionSHA256",
        "patchSHA256",
    ] {
        hash(&profile[key])?;
        require(expected[key] == profile[key])?;
    }
    for key in ["reviewPeerSHA256", "sourceReceiptSHA256"] {
        hash(&lock[key])?;
        require(expected[key] == lock[key])?;
    }
    closed(&profile["recipeHashes"], RECIPES)?;
    for key in RECIPES {
        hash(&profile["recipeHashes"][*key])?;
    }
    require(expected["recipeHashes"] == profile["recipeHashes"])?;
    closed(
        &expected["offerFiles"],
        &[
            "LICENSE",
            "README.md",
            "manifest.json",
            "upstream.zip",
            "winio-custody.patch",
        ],
    )?;
    for value in expected["offerFiles"].as_object().ok_or(())?.values() {
        hash(value)?;
    }
    require(
        expected["offerFiles"]["LICENSE"] == profile["licenseSHA256"]
            && expected["offerFiles"]["upstream.zip"] == profile["upstreamZipSHA256"]
            && expected["offerFiles"]["winio-custody.patch"] == profile["patchSHA256"],
    )?;
    require(manifest.get("portableWinio") == Some(&expected))?;
    require(
        text(&manifest["winioRuntimeOfferSHA256"])?
            == format!("{:x}", Sha256::digest(expected_bytes)),
    )?;
    // Four producer recipes are also present in the existing buildScripts map.
    for name in &RECIPES[..4] {
        let short = name.strip_prefix("scripts/").ok_or(())?;
        require(manifest["buildScripts"][short] == profile["recipeHashes"][*name])?;
    }
    let effective = &manifest["winioEffective"];
    closed(effective, &["before", "after", "checked"])?;
    for capture in [&effective["before"], &effective["after"]] {
        closed(capture, &["record", "rawModuleSHA256", "rawPackagesSHA256"])?;
        hash(&capture["rawModuleSHA256"])?;
        hash(&capture["rawPackagesSHA256"])?;
    }
    let original = &effective["before"]["record"];
    require(original == &effective["after"]["record"])?;
    closed(
        original,
        &[
            "artifact",
            "main",
            "goos",
            "goarch",
            "goamd64",
            "cgo",
            "tags",
            "package",
            "modMode",
            "modfile",
            "modSHA256",
            "sumSHA256",
            "sourceRoot",
            "winioRoot",
            "selectedModule",
            "packages",
        ],
    )?;
    let checked = &effective["checked"];
    closed(
        checked,
        &[
            "artifact",
            "main",
            "goos",
            "goarch",
            "goamd64",
            "cgo",
            "tags",
            "package",
            "modMode",
            "modfile",
            "modSHA256",
            "sumSHA256",
            "sourceRoot",
            "winioRoot",
            "selectedVersion",
            "packages",
            "files",
            "patchedProjectionSHA256",
        ],
    )?;
    for (key, value) in [
        ("artifact", "rdd.exe"),
        ("main", MAIN),
        ("goos", "windows"),
        ("goarch", "amd64"),
        ("goamd64", "v1"),
        ("cgo", "1"),
        ("package", "./cmd/rdd"),
        ("modMode", "readonly"),
    ] {
        require(original[key] == value && checked[key] == value)?;
    }
    let tags = array(&original["tags"], 2)?;
    require(
        tags.iter().any(|v| v == "sqlite_omit_load_extension")
            && tags
                .iter()
                .all(|v| v == "sqlite_omit_load_extension" || v == "with_distro"),
    )?;
    require(
        tags.len()
            == tags
                .iter()
                .map(Value::as_str)
                .collect::<BTreeSet<_>>()
                .len()
            && original["tags"] == checked["tags"],
    )?;
    for key in ["modfile", "sourceRoot", "winioRoot"] {
        require(path(&original[key])? == path(&checked[key])?)?;
    }
    for key in ["modSHA256", "sumSHA256"] {
        hash(&original[key])?;
        require(original[key] == checked[key])?;
    }
    require(
        checked["selectedVersion"] == VERSION
            && checked["patchedProjectionSHA256"] == profile["patchedProjectionSHA256"],
    )?;
    let root = path(&checked["winioRoot"])?;
    module(&original["selectedModule"], &root)?;
    let files = rows(&checked["files"])?;
    require(REQUIRED.iter().all(|name| files.contains_key(*name)))?;
    for (name, row) in &files {
        require(!name.ends_with("_test.go") && reviewed.get(name) == Some(row))?;
    }
    let raw_packages = array(&original["packages"], 128)?;
    let packages = array(&checked["packages"], 128)?;
    require(!packages.is_empty() && raw_packages.len() == packages.len())?;
    let mut imported = BTreeSet::new();
    let mut used = BTreeSet::new();
    for package in packages {
        closed(
            package,
            &[
                "ImportPath",
                "Dir",
                "GoFiles",
                "CgoFiles",
                "CompiledGoFiles",
                "EmbedFiles",
            ],
        )?;
        let import = text(&package["ImportPath"])?;
        require(
            (import == MODULE || import.starts_with(&(MODULE.to_owned() + "/")))
                && imported.insert(import),
        )?;
        let directory = path(&package["Dir"])?;
        require(directory == root || directory.starts_with(&(root.clone() + "/")))?;
        let raw = raw_packages
            .iter()
            .find(|p| p["ImportPath"] == import)
            .ok_or(())?;
        closed(
            raw,
            &[
                "ImportPath",
                "Dir",
                "Module",
                "GoFiles",
                "CgoFiles",
                "CompiledGoFiles",
                "EmbedFiles",
            ],
        )?;
        module(&raw["Module"], &root)?;
        require(path(&raw["Dir"])? == directory)?;
        let mut selected = BTreeMap::new();
        for list in LISTS {
            let mut names = BTreeSet::new();
            for name in array(&package[*list], 512)? {
                let name = text(name)?;
                source_name(name)?;
                require(files.contains_key(name) && names.insert(name))?;
                used.insert(name);
            }
            // Physical metadata projection is producer-owned; bind every list
            // to its corresponding actual Go record, not an independent DTO.
            let raw_list = array(&raw[*list], 512)?;
            require(raw_list.len() == names.len())?;
            let mut projected = BTreeSet::new();
            for raw_name in raw_list {
                let raw_name = text(raw_name)?;
                let absolute = if raw_name.as_bytes().get(1) == Some(&b':') {
                    path(&Value::String(raw_name.to_owned()))?
                } else {
                    source_name(raw_name)?;
                    directory.clone() + "/" + &raw_name.to_ascii_lowercase()
                };
                let relative = absolute.strip_prefix(&(root.clone() + "/")).ok_or(())?;
                require(projected.insert(relative.to_owned()))?;
            }
            require(projected == names.iter().map(|n| n.to_ascii_lowercase()).collect())?;
            selected.insert(*list, names);
        }
        require(
            !selected["CompiledGoFiles"].is_empty()
                && (!selected["GoFiles"].is_empty() || !selected["CgoFiles"].is_empty()),
        )?;
        require(
            selected["GoFiles"]
                .union(&selected["CgoFiles"])
                .all(|name| selected["CompiledGoFiles"].contains(name)),
        )?;
    }
    require(imported.contains(MODULE) && used == files.keys().map(String::as_str).collect())
}

pub(crate) fn validate_portable_winio(manifest: &Value) -> Result<(), String> {
    validate(
        manifest,
        include_bytes!("../../winio-custody.lock.json"),
        include_bytes!("../../winio-runtime-offer.json"),
    )
    .map_err(|()| {
        "Bundled backend Windows transport provenance differs from this application.".to_owned()
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn fixture() -> Value {
        let lock: Value =
            serde_json::from_slice(include_bytes!("../../winio-custody.lock.json")).unwrap();
        let portable: Value =
            serde_json::from_slice(include_bytes!("../../winio-runtime-offer.json")).unwrap();
        let profile = &lock["profile"];
        let selected = json!({"Path":MODULE,"Version":VERSION,"Dir":"F:/fixture/winio",
            "Replace":{"Path":"F:/fixture/winio","Dir":"F:/fixture/winio"}});
        let files: Vec<_> = REQUIRED
            .iter()
            .map(|name| {
                profile["patched"]
                    .as_array()
                    .unwrap()
                    .iter()
                    .find(|row| row["path"] == *name)
                    .unwrap()
                    .clone()
            })
            .collect();
        let package = json!({"ImportPath":MODULE,"Dir":"F:/fixture/winio",
            "GoFiles":REQUIRED,"CgoFiles":[],"CompiledGoFiles":REQUIRED,"EmbedFiles":[]});
        let mut raw_package = package.clone();
        raw_package["Module"] = selected.clone();
        raw_package["CompiledGoFiles"] = json!(REQUIRED
            .iter()
            .map(|name| format!("F:\\fixture\\winio\\{name}"))
            .collect::<Vec<_>>());
        let record = json!({"artifact":"rdd.exe","main":MAIN,"goos":"windows","goarch":"amd64",
            "goamd64":"v1","cgo":"1","tags":["sqlite_omit_load_extension"],"package":"./cmd/rdd",
            "modMode":"readonly","modfile":"F:/fixture/input/rdd-lima.mod","modSHA256":"1".repeat(64),
            "sumSHA256":"2".repeat(64),"sourceRoot":"F:/fixture/rdd","winioRoot":"F:/fixture/winio",
            "selectedModule":selected,"packages":[raw_package]});
        let mut checked = record.clone();
        let checked_map = checked.as_object_mut().unwrap();
        checked_map.remove("selectedModule");
        checked_map.insert("selectedVersion".into(), json!(VERSION));
        checked_map.insert("packages".into(), json!([package]));
        checked_map.insert("files".into(), json!(files));
        checked_map.insert(
            "patchedProjectionSHA256".into(),
            profile["patchedProjectionSHA256"].clone(),
        );
        let capture = json!({"record":record,"rawModuleSHA256":"3".repeat(64),"rawPackagesSHA256":"4".repeat(64)});
        let mut build_scripts = json!({});
        for name in &RECIPES[..4] {
            build_scripts[name.strip_prefix("scripts/").unwrap()] =
                profile["recipeHashes"][*name].clone();
        }
        json!({"portableWinio":portable,"buildScripts":build_scripts,
            "winioRuntimeOfferSHA256":format!("{:x}",Sha256::digest(include_bytes!("../../winio-runtime-offer.json"))),
            "winioEffective":{"before":capture,"after":capture,"checked":checked}})
    }

    #[test]
    fn paired_source_offer_and_actual_projection_shape_pass() {
        assert!(validate_portable_winio(&fixture()).is_ok());
        let mut embedded = fixture();
        for stage in ["before", "after"] {
            embedded["winioEffective"][stage]["record"]["tags"] =
                json!(["sqlite_omit_load_extension", "with_distro"]);
        }
        embedded["winioEffective"]["checked"]["tags"] =
            json!(["sqlite_omit_load_extension", "with_distro"]);
        assert!(validate_portable_winio(&embedded).is_ok());
    }

    #[test]
    fn missing_extra_or_modified_portable_offer_refuses() {
        for change in 0..6 {
            let mut value = fixture();
            match change {
                0 => {
                    value.as_object_mut().unwrap().remove("portableWinio");
                }
                1 => value["portableWinio"]["extra"] = json!(true),
                2 => value["portableWinio"]["offerFiles"]["README.md"] = json!("5".repeat(64)),
                3 => value["portableWinio"]["lockSHA256"] = json!("5".repeat(64)),
                4 => value["buildScripts"]["winio-build-input.mjs"] = json!("5".repeat(64)),
                _ => value["winioRuntimeOfferSHA256"] = json!("5".repeat(64)),
            }
            assert!(
                validate_portable_winio(&value).is_err(),
                "mutation {change}"
            );
        }
    }

    #[test]
    fn changed_capture_wrong_target_or_unreviewed_bytes_refuse() {
        for change in 0..7 {
            let mut value = fixture();
            let effective = &mut value["winioEffective"];
            match change {
                0 => effective["after"]["record"]["cgo"] = json!("0"),
                1 => effective["checked"]["goos"] = json!("linux"),
                2 => effective["checked"]["files"][0]["sha256"] = json!("5".repeat(64)),
                3 => {
                    effective["checked"]["files"]
                        .as_array_mut()
                        .unwrap()
                        .remove(0);
                }
                4 => {
                    effective["checked"]["packages"][0]["CompiledGoFiles"] = json!([]);
                }
                5 => {
                    effective["before"]["rawModuleSHA256"] = json!("0".repeat(64));
                }
                _ => {
                    effective["checked"]["unexpected"] = json!(true);
                }
            };
            assert!(
                validate_portable_winio(&value).is_err(),
                "mutation {change}"
            );
        }
    }

    #[test]
    fn metadata_package_and_file_lists_cannot_be_independent_claims() {
        for change in 0..4 {
            let mut value = fixture();
            for stage in ["before", "after"] {
                let package = &mut value["winioEffective"][stage]["record"]["packages"][0];
                match change {
                    0 => package["Module"]["Replace"]["Dir"] = json!("F:/other/winio"),
                    1 => package["CompiledGoFiles"] = json!([]),
                    2 => package["GoFiles"][0] = json!("../outside.go"),
                    _ => package["ImportPath"] = json!("other/module"),
                }
            }
            assert!(
                validate_portable_winio(&value).is_err(),
                "mutation {change}"
            );
        }
    }
}
