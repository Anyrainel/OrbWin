//! Attended, explicitly authorized private-config setup qualification only.
//! The caller must freeze/hash the test binary, retain globals/project originals
//! and all engine baselines, and review BOTH expected hashes before executing.
//! This test never calls WSL or any engine; actual WSL dispatch is a separate proof.
use super::*;
use sha2::{Digest, Sha256};
use std::path::PathBuf;

const ROOTS_LIMIT: usize = 8192;

// Ordered fields define the one compact wire record shared with the wrapper.
// Path strings are DATA, never a caller-selected route or native ownership.
#[derive(serde::Serialize, serde::Deserialize, PartialEq, Eq)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
struct QualificationRoots {
    schema: u8,
    nonce: String,
    local_app_data: String,
    profile: String,
}

fn qualification_nonce() -> String {
    let nonce = std::env::var("ORBWIN_QUALIFY_WSL_CLIENT_NONCE")
        .unwrap_or_else(|_| panic!("Explicit reviewed roots nonce required"));
    assert!(
        nonce.len() == 32
            && nonce
                .bytes()
                .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b)),
        "Reviewed roots nonce format refused"
    );
    nonce
}

fn valid_root_text(text: &str) -> bool {
    let bytes = text.as_bytes();
    text.len() >= 4
        && text.len() <= 4096
        && text.encode_utf16().count() < 1024
        && bytes[0].is_ascii_alphabetic()
        && bytes[1] == b':'
        && bytes[2] == b'\\'
        && !text.chars().any(char::is_control)
        && !text[3..].contains(['/', ':', '<', '>', '"', '|', '?', '*'])
        && text[3..].split('\\').all(|part| {
            !part.is_empty() && part != "." && part != ".." && !part.ends_with(['.', ' '])
        })
}

fn root_text(path: &std::path::Path) -> String {
    let ordinary = crate::runtime::cli_path(path)
        .unwrap_or_else(|_| panic!("Native roots path conversion refused"));
    let text = ordinary
        .to_str()
        .unwrap_or_else(|| panic!("Native roots Unicode encoding refused"));
    assert!(valid_root_text(text), "Native roots path shape refused");
    text.to_owned()
}

fn native_roots(nonce: &str) -> QualificationRoots {
    // No target/cache/tools/config lookup: only the existing own-user native
    // KnownFolder path resolver followed by pure cli_path/string conversion.
    let (local, profile) = crate::runtime_cache::qualification_native_folders()
        .unwrap_or_else(|_| panic!("Native qualification roots unavailable"));
    QualificationRoots {
        schema: 1,
        nonce: nonce.to_owned(),
        local_app_data: root_text(&local),
        profile: root_text(&profile),
    }
}

fn roots_json(roots: &QualificationRoots) -> String {
    let text = serde_json::to_string(roots)
        .unwrap_or_else(|_| panic!("Native roots record encoding refused"));
    assert!(
        text.len() <= ROOTS_LIMIT,
        "Native roots record bound refused"
    );
    text
}

fn expected_roots(nonce: &str) -> QualificationRoots {
    let text = std::env::var("ORBWIN_QUALIFY_WSL_CLIENT_EXPECTED_ROOTS")
        .unwrap_or_else(|_| panic!("Explicit reviewed expected roots required"));
    assert!(
        text.len() <= ROOTS_LIMIT,
        "Expected roots record bound refused"
    );
    let roots: QualificationRoots = serde_json::from_str(&text)
        .unwrap_or_else(|_| panic!("Closed expected roots record refused"));
    assert!(
        roots.schema == 1
            && roots.nonce == nonce
            && valid_root_text(&roots.local_app_data)
            && valid_root_text(&roots.profile),
        "Expected roots schema, nonce or path shape refused"
    );
    // Derived serde rejects duplicate/missing/unknown fields. Re-encoding also
    // refuses reordered/escaped/whitespace variants of this controlled record.
    assert!(
        roots_json(&roots) == text,
        "Canonical expected roots required"
    );
    roots
}

fn require_current_roots(expected: &QualificationRoots) {
    assert!(
        native_roots(&expected.nonce) == *expected,
        "Current native roots differ from reviewed DATA"
    );
}

fn require_target_roots(target: &crate::engine::Target, roots: &QualificationRoots) {
    let home = crate::runtime::cli_path(target.home())
        .unwrap_or_else(|_| panic!("Selected target home conversion refused"));
    let expected_home = PathBuf::from(&roots.local_app_data)
        .join("OrbWinData")
        .join("home");
    assert!(
        home == expected_home,
        "Selected target home differs from reviewed roots"
    );
    assert!(
        target.daemon_id() == "d9c00194-ea9d-439a-8561-6a0ec580fbd5",
        "Selected target daemon differs from the fixed owned identity"
    );
}

#[test]
#[ignore = "Queries only existing own-user native KnownFolders; requires explicit reviewed query opt-in and nonce"]
fn owned_private_client_roots() {
    assert!(
        matches!(
            std::env::var("ORBWIN_QUALIFY_WSL_CLIENT_ROOTS").as_deref(),
            Ok("1")
        ),
        "Explicit reviewed native roots query required"
    );
    let nonce = qualification_nonce();
    let roots = native_roots(&nonce);
    let record = roots_json(&roots);
    // Separate the one marker line from any libtest progress prefix.
    println!("\nWSL_PRIVATE_CLIENT_ROOTS {record}");
}

fn expected(name: &str) -> String {
    let value =
        std::env::var(name).unwrap_or_else(|_| panic!("Explicit reviewed checksum required"));
    assert!(
        value.len() == 64
            && value
                .bytes()
                .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
    );
    value
}

#[test]
#[ignore = "Writes only the explicitly reviewed owned private WSL plugin directory; requires root runtime lane and retained baselines"]
fn owned_private_client_refresh() {
    assert!(
        matches!(
            std::env::var("ORBWIN_QUALIFY_WSL_CLIENT_REFRESH").as_deref(),
            Ok("1")
        ),
        "Explicit reviewed private client refresh required"
    );
    let nonce = qualification_nonce();
    let roots = expected_roots(&nonce);
    let old_sha = expected("ORBWIN_QUALIFY_WSL_CLIENT_OLD_SHA256");
    let lock_sha = expected("ORBWIN_QUALIFY_WSL_CLIENT_LOCK_SHA256");
    assert_eq!(
        format!(
            "{:x}",
            Sha256::digest(include_bytes!("../../runtime-tools.lock.json"))
        ),
        lock_sha,
        "Frozen native embedded tool lock differs"
    );
    require_current_roots(&roots); // Before target's executable-cache effects.
    let target = crate::engine::target()
        .unwrap_or_else(|_| panic!("Owned qualification target unavailable"));
    require_target_roots(&target, &roots); // Before parents/private reads/writes.
    let path = target.home().join(".rdorbwin/wsl-docker/config.json");
    let _parents =
        parents(&path).unwrap_or_else(|_| panic!("Reviewed private client parents unavailable"));
    let (_, _, old) =
        read(&path).unwrap_or_else(|_| panic!("Reviewed private client original unavailable"));
    assert_eq!(format!("{:x}", Sha256::digest(&old)), old_sha);
    let (_, directory) = crate::wsl_integration::tools()
        .unwrap_or_else(|_| panic!("Reviewed private client tool pins unavailable"));
    let next = replacement(&old, &directory)
        .unwrap_or_else(|_| panic!("Reviewed private client replacement refused"))
        .expect("Qualification expects an actual version-path refresh");
    // The same transaction as the product command, with one additional old-byte
    // guard under its existing registry lock; no test path/tool overrides.
    require_current_roots(&roots);
    require_target_roots(&target, &roots);
    let result = refresh_for_target(&target, Some(&old_sha))
        .unwrap_or_else(|_| panic!("Reviewed private client refresh failed; retain evidence"));
    assert!(result.changed);
    let (_, _, actual) =
        read(&path).unwrap_or_else(|_| panic!("Refreshed private client bytes unavailable"));
    assert!(
        actual == next,
        "Only the selected plugin value may change; all other bytes must match"
    );
    let new_sha = format!("{:x}", Sha256::digest(&actual));
    require_current_roots(&roots);
    require_target_roots(&target, &roots);
    let unchanged = refresh_for_target(&target, Some(&new_sha))
        .unwrap_or_else(|_| panic!("Reviewed private client idempotence failed; retain evidence"));
    assert!(!unchanged.changed);
    let (_, _, repeated) =
        read(&path).unwrap_or_else(|_| panic!("Repeated private client bytes unavailable"));
    assert!(repeated == actual, "Repeated private client bytes differ");
    require_current_roots(&roots); // Late drift fails acceptance after effects.
    require_target_roots(&target, &roots);
    println!("WSL_PRIVATE_CLIENT_REFRESH changed=true idempotent=true oldSha256={old_sha} newSha256={new_sha} embeddedLockSha256={lock_sha}");
}
