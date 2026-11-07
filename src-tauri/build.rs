use sha2::{Digest, Sha256};

// The running desktop owns these pins. Runtime manifests are provenance inputs,
// not a trust anchor for executable bytes that an ordinary account can replace.
fn component_binary_pins() {
    let root = std::path::PathBuf::from(std::env::var("CARGO_MANIFEST_DIR").unwrap());
    println!("cargo:rerun-if-env-changed=ORBWIN_COMPONENT_INPUT_DIR");
    let directory = match std::env::var_os("ORBWIN_COMPONENT_INPUT_DIR") {
        Some(value) => {
            let path = std::path::PathBuf::from(value);
            assert!(
                path.is_absolute(),
                "ORBWIN_COMPONENT_INPUT_DIR must be absolute"
            );
            path
        }
        None => root.join("../artifacts/backend"),
    };
    let release = std::env::var("PROFILE").unwrap() == "release";
    println!("cargo:rerun-if-env-changed=ORBWIN_TOOLS_INPUT_DIR");
    let tools_directory = match std::env::var_os("ORBWIN_TOOLS_INPUT_DIR") {
        Some(value) => {
            let path = std::path::PathBuf::from(value);
            assert!(
                path.is_absolute(),
                "ORBWIN_TOOLS_INPUT_DIR must be absolute"
            );
            path
        }
        None => root.join("../artifacts/runtime-tools"),
    };
    let mut source = String::from("&[");
    for (name, limit) in [
        ("rdd.exe", 512 * 1024 * 1024u64),
        ("rdd-forwarder.exe", 4 * 1024 * 1024),
        ("orbwin-session.exe", 256 * 1024 * 1024u64),
        ("lan/assets.json", 4096),
        ("lan/assets.sig", 88),
        ("lan/orbwin-lan-helper.exe", 64 * 1024 * 1024),
        ("lan/orbwin-lan-worker.exe", 64 * 1024 * 1024),
        ("docker-credential-orbwin.exe", 16 * 1024 * 1024),
        ("orbwin-project", 16 * 1024 * 1024),
        ("orbwin-fs", 16 * 1024 * 1024),
        ("orbwin-services", 16 * 1024 * 1024),
        ("orbwin-trust", 16 * 1024 * 1024),
        ("tools/bin/docker.exe", 128 * 1024 * 1024),
        (
            "tools/docker-cli-plugins/docker-compose.exe",
            256 * 1024 * 1024,
        ),
        (
            "tools/docker-cli-plugins/docker-buildx.exe",
            256 * 1024 * 1024,
        ),
        ("tools/linux/docker", 128 * 1024 * 1024),
        ("tools/linux/docker-compose", 256 * 1024 * 1024),
        ("tools/linux/docker-buildx", 256 * 1024 * 1024),
    ] {
        let path = match name.strip_prefix("tools/") {
            Some(relative) => tools_directory.join(relative),
            None => directory.join(name),
        };
        println!("cargo:rerun-if-changed={}", path.display());
        let pin = match std::fs::File::open(&path) {
            Ok(mut file) => {
                use std::io::Read;
                let metadata = file.metadata().expect("component metadata");
                assert!(
                    metadata.is_file() && metadata.len() > 0 && metadata.len() <= limit,
                    "invalid build-time component size: {name}"
                );
                assert!(
                    name != "lan/assets.sig" || metadata.len() == 88,
                    "build-time LAN signature must be exactly 88 bytes"
                );
                let mut hasher = Sha256::new();
                let copied = std::io::copy(&mut (&mut file).take(limit + 1), &mut hasher)
                    .expect("hash build-time component");
                assert_eq!(
                    copied,
                    metadata.len(),
                    "component changed while hashing: {name}"
                );
                format!("Some({:?})", format!("{:x}", hasher.finalize()))
            }
            Err(error) if !release && error.kind() == std::io::ErrorKind::NotFound => {
                // Source-only debug builds remain possible, but cannot launch
                // a component until rebuilt with actual reviewed build inputs.
                "None".into()
            }
            Err(error) => panic!("cannot pin build-time component {name}: {error}"),
        };
        source.push_str(&format!("({name:?}, {pin}),"));
    }
    // Optional candidate metadata never turns an ordinary older release into a
    // candidate build. Staging requires BOTH independent pins at runtime; it
    // cannot select expected bytes from a mutable manifest.
    let mut candidate_metadata_present = 0;
    for (name, limit, exact) in [
        ("rdd-candidate.json", 4096u64, None),
        ("rdd-candidate.sig", 88u64, Some(88u64)),
    ] {
        let path = directory.join(name);
        println!("cargo:rerun-if-changed={}", path.display());
        let pin = match std::fs::File::open(&path) {
            Ok(mut file) => {
                candidate_metadata_present += 1;
                use std::io::Read;
                let metadata = file.metadata().expect("candidate metadata");
                assert!(
                    metadata.is_file()
                        && metadata.len() > 0
                        && metadata.len() <= limit
                        && exact.map_or(true, |size| metadata.len() == size),
                    "invalid build-time candidate metadata size: {name}"
                );
                let mut hasher = Sha256::new();
                let copied = std::io::copy(&mut (&mut file).take(limit + 1), &mut hasher)
                    .expect("hash build-time candidate metadata");
                assert_eq!(copied, metadata.len(), "candidate metadata changed: {name}");
                format!("Some({:?})", format!("{:x}", hasher.finalize()))
            }
            Err(error) if error.kind() == std::io::ErrorKind::NotFound => "None".into(),
            Err(error) => panic!("cannot pin build-time candidate metadata {name}: {error}"),
        };
        source.push_str(&format!("({name:?}, {pin}),"));
    }
    assert!(
        candidate_metadata_present == 0 || candidate_metadata_present == 2,
        "candidate metadata must be both present or both absent"
    );
    source.push(']');
    std::fs::write(
        std::path::PathBuf::from(std::env::var("OUT_DIR").unwrap())
            .join("component_binary_pins.rs"),
        source,
    )
    .expect("write component pins");
}

fn main() {
    component_binary_pins();
    let session = std::path::PathBuf::from(std::env::var("CARGO_MANIFEST_DIR").unwrap())
        .join("../backend-tools/session");
    let mut files = Vec::new();
    println!("cargo:rerun-if-changed=../scripts/session-source-folders.txt");
    for folder in include_str!("../scripts/session-source-folders.txt").lines() {
        let directory = session.join(folder);
        println!("cargo:rerun-if-changed={}", directory.display());
        for entry in std::fs::read_dir(directory).unwrap() {
            let name = entry.unwrap().file_name().to_string_lossy().into_owned();
            if name.ends_with(".go")
                && !name.ends_with("_test.go")
                && !name.starts_with('.')
                && !name.starts_with('_')
            {
                files.push(if folder == "." {
                    name
                } else {
                    format!("{folder}/{name}")
                });
            }
        }
    }
    files.extend([
        "go.mod".into(),
        "go.sum".into(),
        "lan-helper/lan.manifest".into(),
        "lan-helper/lan.rc".into(),
    ]);
    files.sort();
    let mut source = String::from("[");
    for name in files {
        let path = session.join(&name).canonicalize().unwrap();
        println!("cargo:rerun-if-changed={}", path.display());
        source.push_str(&format!(
            "({name:?}, include_bytes!({:?}).as_slice()),",
            path.to_string_lossy()
        ));
    }
    source.push(']');
    std::fs::write(
        std::path::PathBuf::from(std::env::var("OUT_DIR").unwrap()).join("session_sources.rs"),
        source,
    )
    .unwrap();
    println!("cargo:rerun-if-changed=../upstream.lock.json");
    println!("cargo:rerun-if-changed=../runtime-tools.lock.json");
    println!("cargo:rerun-if-changed=../maintenance-image.lock.json");
    println!("cargo:rerun-if-changed=../guest-packages.lock.json");
    tauri_build::build()
}
