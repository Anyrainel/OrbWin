// Ignored native fixture. Only the fixed disposable HKCU qualification subtree
// is reachable. Production read/write/policy adapters remain unchanged.
use super::*;
use sha2::{Digest, Sha256};
use std::{fs, io::Write, path::PathBuf};
use winreg::enums::{KEY_READ, KEY_WRITE, REG_CREATED_NEW_KEY, REG_DWORD, REG_EXPAND_SZ};

const NONCE: &str = "a73cd9812c864f3aaff7ab229e460d35";
const FIXTURE: &str = r"Software\OrbWin\Qualification\a73cd9812c864f3aaff7ab229e460d35";
const MARKER: &str = "QualificationOwner";
type Check<T> = Result<T, String>;
fn require(value: bool, message: &str) -> Check<()> {
    if value {
        Ok(())
    } else {
        Err(message.into())
    }
}
fn io<T>(result: std::io::Result<T>) -> Check<T> {
    result.map_err(|e| e.to_string())
}
fn marker() -> RegValue<'static> {
    RegValue {
        vtype: REG_SZ,
        bytes: NONCE
            .encode_utf16()
            .chain(Some(0))
            .flat_map(u16::to_le_bytes)
            .collect::<Vec<_>>()
            .into(),
    }
}
// RegSetValueExW requires terminated string data. Keep the oversized case
// independent of the separate malformed-string case: 4096 UTF16 A + NUL.
fn oversized_string_bytes() -> Vec<u8> {
    std::iter::repeat_n(0x0041u16, 4096)
        .chain(Some(0))
        .flat_map(u16::to_le_bytes)
        .collect()
}
#[test]
fn oversized_fixture_is_valid_terminated_utf16_above_production_read_limit() {
    let bytes = oversized_string_bytes();
    assert_eq!(bytes.len(), 8194);
    let units: Vec<u16> = bytes
        .chunks_exact(2)
        .map(|v| u16::from_le_bytes([v[0], v[1]]))
        .collect();
    assert_eq!(units.last(), Some(&0));
    assert!(!units[..units.len() - 1].contains(&0));
    assert_eq!(
        String::from_utf16(&units[..units.len() - 1]).unwrap(),
        "A".repeat(4096)
    );
}
fn raw_value(value: &RegValue<'_>) -> StartupValue {
    StartupValue {
        kind: value.vtype.clone() as u32,
        bytes: value.bytes.to_vec(),
    }
}
// Larger fixture-only read limit accommodates the intentional 8194-byte case.
// Never allocate based on a registry length and never read arbitrary value names.
fn fixture_raw(key: &RegKey, field: &str) -> Check<Option<StartupValue>> {
    require(
        field == MARKER || field == VALUE,
        "Unexpected fixture field",
    )?;
    use windows_sys::Win32::{
        Foundation::{ERROR_FILE_NOT_FOUND, ERROR_SUCCESS},
        System::Registry::RegQueryValueExW,
    };
    let name: Vec<_> = field.encode_utf16().chain(Some(0)).collect();
    let mut bytes = vec![0u8; 16384];
    let mut length = bytes.len() as u32;
    let mut kind = 0;
    let code = unsafe {
        RegQueryValueExW(
            key.raw_handle(),
            name.as_ptr(),
            std::ptr::null(),
            &mut kind,
            bytes.as_mut_ptr(),
            &mut length,
        )
    };
    if code == ERROR_FILE_NOT_FOUND {
        return Ok(None);
    }
    require(
        code == ERROR_SUCCESS && length as usize <= bytes.len(),
        "Cannot bound fixture registry value",
    )?;
    bytes.truncate(length as usize);
    Ok(Some(StartupValue { kind, bytes }))
}
fn guard(key: &RegKey, expected: Option<&StartupValue>) -> Check<()> {
    let info = io(key.query_info())?;
    require(
        info.sub_keys == 0
            && info.values == 1 + u32::from(expected.is_some())
            && info.max_value_name_len <= 64
            && info.max_value_len <= 16384,
        "Unknown fixture content retained",
    )?;
    require(
        fixture_raw(key, MARKER)? == Some(raw_value(&marker())),
        "Fixture owner marker changed; retain",
    )?;
    require(
        fixture_raw(key, VALUE)?.as_ref() == expected,
        "Fixture value changed; retain",
    )?;
    let after = io(key.query_info())?;
    require(
        after.sub_keys == 0 && after.values == info.values,
        "Fixture changed during guard",
    )
}
fn observed(expected: Option<&StartupValue>) -> Check<()> {
    let key =
        io(RegKey::predef(HKEY_CURRENT_USER).open_subkey_with_flags(FIXTURE, KEY_QUERY_VALUE))?;
    guard(&key, expected)
}
fn store(read_only: bool) -> Check<RegistryStore> {
    let flags = KEY_QUERY_VALUE | if read_only { 0 } else { KEY_SET_VALUE };
    let key = io(RegKey::predef(HKEY_CURRENT_USER).open_subkey_with_flags(FIXTURE, flags))?;
    Ok(RegistryStore { key })
}
#[derive(Clone, Copy, serde::Serialize)]
struct Parents {
    product_created: bool,
    qualification_created: bool,
}
fn create(journal: &mut Journal) -> Check<Parents> {
    let hkcu = RegKey::predef(HKEY_CURRENT_USER);
    let software = io(hkcu.open_subkey_with_flags("Software", KEY_READ | KEY_WRITE))?;
    // Durable parent/leaf dispositions precede each subsequent operation. A
    // partial creation remains recorded; no fallback adopts or recursively removes it.
    let (product, pd) = io(software.create_subkey_with_flags("OrbWin", KEY_READ | KEY_WRITE))?;
    journal.receipt["creation"]["productCreated"] = (pd == REG_CREATED_NEW_KEY).into();
    journal.save()?;
    let (parent, qd) = io(product.create_subkey_with_flags("Qualification", KEY_READ | KEY_WRITE))?;
    journal.receipt["creation"]["qualificationCreated"] = (qd == REG_CREATED_NEW_KEY).into();
    journal.save()?;
    // RegCreateKeyEx disposition is the admission guard; an existing empty key
    // is foreign and never receives a marker.
    let (key, nd) = io(parent.create_subkey_with_flags(NONCE, KEY_READ | KEY_WRITE))?;
    require(
        nd == REG_CREATED_NEW_KEY,
        "Nonce key already exists; no adoption or overwrite",
    )?;
    journal.receipt["creation"]["nonceCreated"] = true.into();
    journal.save()?;
    let info = io(key.query_info())?;
    require(
        info.values == 0 && info.sub_keys == 0,
        "New fixture unexpectedly populated; retain",
    )?;
    io(key.set_raw_value(MARKER, &marker()))?;
    guard(&key, None)?;
    observed(None)?;
    Ok(Parents {
        product_created: pd == REG_CREATED_NEW_KEY,
        qualification_created: qd == REG_CREATED_NEW_KEY,
    })
}
fn cleanup(parents: Parents) -> Check<()> {
    let hkcu = RegKey::predef(HKEY_CURRENT_USER);
    let software = io(hkcu.open_subkey_with_flags("Software", KEY_READ | KEY_WRITE))?;
    let product = io(software.open_subkey_with_flags("OrbWin", KEY_READ | KEY_WRITE))?;
    let parent = io(product.open_subkey_with_flags("Qualification", KEY_READ | KEY_WRITE))?;
    let child = io(parent.open_subkey_with_flags(NONCE, KEY_READ | KEY_WRITE))?;
    guard(&child, None)?;
    io(child.delete_value(MARKER))?;
    let empty = io(child.query_info())?;
    require(
        empty.values == 0 && empty.sub_keys == 0,
        "Fixture not empty; retain",
    )?;
    drop(child);
    io(parent.delete_subkey(NONCE))?;
    if parents.qualification_created {
        let empty = io(parent.query_info())?;
        require(
            empty.values == 0 && empty.sub_keys == 0,
            "Created parent adopted; retain",
        )?;
        drop(parent);
        io(product.delete_subkey("Qualification"))?;
    } else {
        drop(parent)
    }
    if parents.product_created {
        let empty = io(product.query_info())?;
        require(
            empty.values == 0 && empty.sub_keys == 0,
            "Created product parent adopted; retain",
        )?;
        drop(product);
        io(software.delete_subkey("OrbWin"))?;
    } else {
        drop(product)
    }
    match hkcu.open_subkey_with_flags(FIXTURE, KEY_QUERY_VALUE) {
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(()),
        _ => Err("Fixture removal not confirmed".into()),
    }
}
fn real_directories(path: &std::path::Path) -> Check<()> {
    use std::os::windows::fs::MetadataExt;
    for part in path.ancestors() {
        let metadata = io(fs::symlink_metadata(part))?;
        require(
            metadata.is_dir() && metadata.file_attributes() & 0x400 == 0,
            "Fixture artifact parent is redirected",
        )?;
    }
    Ok(())
}
struct Journal {
    file: fs::File,
    receipt: serde_json::Value,
}
impl Journal {
    fn save(&mut self) -> Check<()> {
        use std::io::{Seek, SeekFrom};
        io(self.file.set_len(0))?;
        io(self.file.seek(SeekFrom::Start(0)))?;
        io(self
            .file
            .write_all(&serde_json::to_vec_pretty(&self.receipt).map_err(|e| e.to_string())?))?;
        io(self.file.sync_all())
    }
    fn step(&mut self, name: &str, action: impl FnOnce() -> Check<()>) -> Check<()> {
        self.receipt["pending"] = name.into();
        self.save()?;
        action()?;
        self.receipt["steps"]
            .as_array_mut()
            .unwrap()
            .push(name.into());
        self.receipt["pending"] = serde_json::Value::Null;
        self.save()
    }
}
#[test]
#[ignore = "Explicit disposable HKCU qualification subtree only; requires frozen native/source attestation and root review; never HKCU Run"]
fn owned_startup_registry_guarded_operations() {
    assert_eq!(
        std::env::var("ORBWIN_QUALIFY_DESKTOP_REGISTRY").as_deref(),
        Ok("1")
    );
    assert_eq!(
        std::env::var("ORBWIN_QUALIFY_DESKTOP_REGISTRY_NONCE").as_deref(),
        Ok(NONCE)
    );
    let folder = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../artifacts/qualification")
        .join(format!("p2-desktop-registry-{NONCE}"));
    // Exclusive artifact folder; no rerun can silently adopt the previous key.
    real_directories(folder.parent().unwrap()).unwrap();
    fs::create_dir(&folder).expect("Fresh fixture artifact folder required");
    real_directories(&folder).unwrap();
    use std::os::windows::fs::OpenOptionsExt;
    let file = fs::OpenOptions::new()
        .share_mode(windows_sys::Win32::Storage::FileSystem::FILE_SHARE_READ)
        .write(true)
        .create_new(true)
        .open(folder.join("proof.json"))
        .unwrap();
    let mut journal = Journal {
        file,
        receipt: serde_json::json!({"schema":1,"nonce":NONCE,"registryPath":FIXTURE,"scope":"disposable qualification subtree only; no Run or backend","steps":[],"pending":null,"success":false,"cleaned":false,"parents":null,"creation":{},"sourceHashes":{"desktop":format!("{:x}",Sha256::digest(include_bytes!("desktop.rs"))),"policy":format!("{:x}",Sha256::digest(include_bytes!("desktop_policy.rs"))),"fixture":format!("{:x}",Sha256::digest(include_bytes!("desktop_registry_qualification.rs"))),"lock":format!("{:x}",Sha256::digest(include_bytes!("desktop_startup_lock.rs")))}}),
    };
    journal.save().unwrap();
    let result = (|| -> Check<()> {
        let _lock = named_lock(
            "OrbWin.Qualification.Startup.a73cd9812c864f3aaff7ab229e460d35",
            5000,
        )?;
        let expected = own_value()?;

        journal.receipt["pending"] = "create-exclusive-nonce-key-and-marker".into();
        journal.save()?;
        let parents = Some(create(&mut journal)?);
        journal.receipt["steps"]
            .as_array_mut()
            .unwrap()
            .push("create-exclusive-nonce-key-and-marker".into());
        journal.receipt["pending"] = serde_json::Value::Null;
        journal.save()?;
        journal.receipt["parents"] = serde_json::to_value(parents.unwrap()).unwrap();
        journal.save()?;
        journal.step("absent-enabled-noop-disabled", || {
            observed(None)?;
            let mut tx = store(false)?;
            guard(&tx.key, None)?;
            require(
                policy::change(&mut tx, &expected, true)?,
                "Expected enabled change",
            )?;
            drop(tx);
            observed(Some(&expected))?;
            let mut tx = store(false)?;
            guard(&tx.key, Some(&expected))?;
            require(
                !policy::change(&mut tx, &expected, true)?,
                "Expected enabled no-op",
            )?;
            drop(tx);
            observed(Some(&expected))?;
            let mut tx = store(false)?;
            require(
                policy::change(&mut tx, &expected, false)?,
                "Expected disabled change",
            )?;
            drop(tx);
            observed(None)?;
            let mut tx = store(false)?;
            require(
                !policy::change(&mut tx, &expected, false)?,
                "Expected disabled no-op",
            )?;
            drop(tx);
            observed(None)
        })?;
        let foreign_command =
            policy::expected_value(&format!(r"C:\OrbWinQualification\{NONCE}\foreign.exe"))?;
        for (name, value) in [
            (
                "foreign-sz",
                RegValue {
                    vtype: REG_SZ,
                    bytes: foreign_command.bytes.into(),
                },
            ),
            (
                "foreign-expand-type",
                RegValue {
                    vtype: REG_EXPAND_SZ,
                    bytes: expected.bytes.as_slice().into(),
                },
            ),
            (
                "foreign-dword",
                RegValue {
                    vtype: REG_DWORD,
                    bytes: 1u32.to_le_bytes().to_vec().into(),
                },
            ),
            (
                "malformed-sz",
                RegValue {
                    vtype: REG_SZ,
                    bytes: (&[65][..]).into(),
                },
            ),
            (
                "oversize-sz",
                RegValue {
                    vtype: REG_SZ,
                    bytes: oversized_string_bytes().into(),
                },
            ),
        ] {
            let foreign = raw_value(&value);
            journal.receipt["case"] = serde_json::json!({"name":name,"kind":foreign.kind,"bytes":foreign.bytes.len(),"sha256":format!("{:x}",Sha256::digest(&foreign.bytes))});
            journal.save()?;
            journal.step(&format!("preserve-{name}"), || {
                let setup = store(false)?;
                guard(&setup.key, None)?;
                io(setup.key.set_raw_value(VALUE, &value))?;
                drop(setup);
                observed(Some(&foreign))?;
                for enabled in [true, false] {
                    let mut tx = store(false)?;
                    guard(&tx.key, Some(&foreign))?;
                    require(
                        policy::change(&mut tx, &expected, enabled).is_err(),
                        "Foreign value was accepted",
                    )?;
                    drop(tx);
                    observed(Some(&foreign))?;
                }
                let mut tx = store(false)?;
                if foreign.bytes.len() > 8192 {
                    require(
                        policy::cleanup(&mut tx, &expected).is_err(),
                        "Oversize cleanup accepted",
                    )?;
                } else {
                    require(
                        policy::cleanup(&mut tx, &expected)? == policy::Cleanup::ForeignPreserved,
                        "Foreign cleanup accepted",
                    )?;
                }
                drop(tx);
                observed(Some(&foreign))?;
                // Fixture teardown only: known synthetic bytes under owned nonce.
                let remove = store(false)?;
                guard(&remove.key, Some(&foreign))?;
                io(remove.key.delete_value(VALUE))?;
                drop(remove);
                observed(None)
            })?;
        }
        journal.step("readonly-handle-write-failure-preserves-absence", || {
            let mut tx = store(true)?;
            guard(&tx.key, None)?;
            require(
                policy::change(&mut tx, &expected, true).is_err(),
                "Read-only handle unexpectedly wrote",
            )?;
            drop(tx);
            observed(None)
        })?;
        journal.step("cross-process-contention-and-abandonment", mutex_cases)?;
        journal.step(
            "cleanup-exact-empty-nonce-and-created-empty-parents",
            || cleanup(parents.unwrap()),
        )?;
        journal.receipt["cleaned"] = true.into();
        Ok(())
    })();
    journal.receipt["success"] = result.is_ok().into();
    if let Err(error) = &result {
        journal.receipt["error"] = error.clone().into();
        journal.receipt["recovery"]="Retain the exact nonce key and evidence. Do not retry, adopt, clear unknown values, change ACLs, or recursively delete. Root must review the pending outcome.".into();
    }
    journal.save().unwrap();
    println!(
        "DESKTOP_REGISTRY_FIXTURE nonce={NONCE} success={} cleaned={} proof={}",
        result.is_ok(),
        journal.receipt["cleaned"],
        folder.join("proof.json").display()
    );
    assert!(
        result.is_ok(),
        "Disposable registry fixture failed; inspect retained receipt, no automatic recovery."
    );
}

const MUTEX_PREFIX: &str = "OrbWin.Qualification.Startup.Child.a73cd9812c864f3aaff7ab229e460d35";
#[test]
#[ignore = "Child of exact opted-in disposable fixture only; no Run key"]
fn startup_mutex_child() {
    assert_eq!(
        std::env::var("ORBWIN_QUALIFY_DESKTOP_REGISTRY").as_deref(),
        Ok("1")
    );
    assert_eq!(
        std::env::var("ORBWIN_QUALIFY_DESKTOP_REGISTRY_NONCE").as_deref(),
        Ok(NONCE)
    );
    assert_eq!(
        std::env::var("ORBWIN_QUALIFY_DESKTOP_REGISTRY_CHILD").as_deref(),
        Ok("1")
    );
    let _lock = named_lock(MUTEX_PREFIX, 5000).unwrap();
    let folder = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../artifacts/qualification")
        .join(format!("p2-desktop-registry-{NONCE}"));
    real_directories(&folder).unwrap();
    let mut ready = fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .open(folder.join("mutex-child-ready"))
        .unwrap();
    ready.write_all(NONCE.as_bytes()).unwrap();
    ready.sync_all().unwrap();
    std::thread::sleep(std::time::Duration::from_secs(30));
}
fn mutex_cases() -> Check<()> {
    let mut child = std::process::Command::new(io(std::env::current_exe())?)
        .args([
            "--ignored",
            "--exact",
            "desktop::startup::qualification::startup_mutex_child",
            "--nocapture",
        ])
        .env("ORBWIN_QUALIFY_DESKTOP_REGISTRY_CHILD", "1")
        .stdout(std::process::Stdio::null())
        .stderr(std::process::Stdio::null())
        .spawn()
        .map_err(|e| e.to_string())?;
    // Always reap the child, even if the bounded signal wait/check fails.
    let result = (|| -> Check<()> {
        let signal = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../artifacts/qualification")
            .join(format!("p2-desktop-registry-{NONCE}/mutex-child-ready"));
        let start = std::time::Instant::now();
        loop {
            match fs::symlink_metadata(&signal) {
                Ok(meta) => {
                    use std::os::windows::fs::MetadataExt;
                    require(
                        meta.is_file()
                            && meta.file_attributes() & 0x400 == 0
                            && meta.len() == NONCE.len() as u64,
                        "Invalid child signal",
                    )?;
                    require(
                        io(fs::read(&signal))? == NONCE.as_bytes(),
                        "Foreign child signal",
                    )?;
                    break;
                }
                Err(e) if e.kind() == std::io::ErrorKind::NotFound => (),
                Err(e) => return Err(e.to_string()),
            }
            require(
                start.elapsed() < std::time::Duration::from_secs(3),
                "Mutex child readiness timeout",
            )?;
            require(
                io(child.try_wait())?.is_none(),
                "Mutex child exited before readiness",
            )?;
            std::thread::sleep(std::time::Duration::from_millis(20));
        }
        let error = named_lock(MUTEX_PREFIX, 100)
            .err()
            .ok_or("Contending process acquired held mutex")?;
        require(error.contains("busy"), "Unexpected contention result")?;
        // Open the same native object BEFORE terminating its owner, so Windows
        // retains the abandoned object instead of destroying/recreating it.
        let handle = named_lock_handle(MUTEX_PREFIX)?;
        if let Err(error) = child.kill() {
            unsafe {
                windows_sys::Win32::Foundation::CloseHandle(handle);
            }
            return Err(error.to_string());
        }
        if let Err(error) = child.wait() {
            unsafe {
                windows_sys::Win32::Foundation::CloseHandle(handle);
            }
            return Err(error.to_string());
        }
        let error = wait_named_lock(handle, 1000)
            .err()
            .ok_or("Abandoned mutex was accepted")?;
        require(error.contains("interrupted"), "Unexpected abandoned result")?;
        // Abandonment guard released ownership before error; fresh acquire works.
        drop(named_lock(MUTEX_PREFIX, 100)?);
        Ok(())
    })();
    if io(child.try_wait())?.is_none() {
        let _ = child.kill();
        let _ = child.wait();
    }
    result
}
