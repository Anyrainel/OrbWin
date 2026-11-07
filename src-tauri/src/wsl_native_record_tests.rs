#![cfg(all(test, windows))]

use super::native_record_handle;
use std::fs::{self, File, OpenOptions};
use std::io::{self, Read, Write};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{SystemTime, UNIX_EPOCH};

static FIXTURE_SEQUENCE: AtomicU64 = AtomicU64::new(0);
const CONTENT: &[u8] = b"orbwin-native-record-fixture\n";

// This fixture owns one newly created directory and two fixed child names.
// Cleanup never enumerates, recursively removes, follows a reparse root, or
// removes a preexisting directory. Failed cleanup preserves evidence visibly.
struct Fixture {
    root: PathBuf,
    canonical_root: PathBuf,
}

impl Fixture {
    fn new() -> Self {
        let temporary = std::env::temp_dir();
        assert!(
            temporary.is_absolute(),
            "test temporary directory must be absolute"
        );
        let nonce = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("fixture clock")
            .as_nanos();
        let root = temporary.join(format!(
            "orbwin-wsl-native-record-{}-{nonce}-{}",
            std::process::id(),
            FIXTURE_SEQUENCE.fetch_add(1, Ordering::Relaxed)
        ));
        // No reuse: failure here creates no cleanup owner.
        fs::create_dir(&root).expect("create unique native-record test directory");
        let canonical_root = fs::canonicalize(&root).expect("resolve new test directory");
        let fixture = Self {
            root,
            canonical_root,
        };
        let mut file = OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(fixture.record())
            .expect("create exact fixture record");
        file.write_all(CONTENT).expect("write fixture record");
        file.flush().expect("flush fixture record");
        drop(file);
        fixture
    }

    fn record(&self) -> PathBuf {
        self.root.join("record.json")
    }
    fn link(&self) -> PathBuf {
        self.root.join("record-link.json")
    }

    fn cleanup(&self) -> io::Result<()> {
        use std::os::windows::fs::MetadataExt;
        let metadata = fs::symlink_metadata(&self.root)?;
        if !metadata.is_dir()
            || metadata.file_attributes() & 0x400 != 0
            || fs::canonicalize(&self.root)? != self.canonical_root
        {
            return Err(io::Error::other(
                "fixture directory identity/path changed; preserved",
            ));
        }
        for path in [self.link(), self.record()] {
            match fs::symlink_metadata(&path) {
                Ok(metadata) if metadata.is_file() || metadata.file_type().is_symlink() => {
                    fs::remove_file(&path)?;
                }
                Ok(_) => return Err(io::Error::other("unexpected fixture child type; preserved")),
                Err(error) if error.kind() == io::ErrorKind::NotFound => {}
                Err(error) => return Err(error),
            }
        }
        fs::remove_dir(&self.root)
    }
}

impl Drop for Fixture {
    fn drop(&mut self) {
        if let Err(error) = self.cleanup() {
            if std::thread::panicking() {
                eprintln!(
                    "Native-record fixture retained at {}: {error}",
                    self.root.display()
                );
            } else {
                panic!(
                    "Native-record fixture cleanup failed at {}: {error}",
                    self.root.display()
                );
            }
        }
    }
}

fn assert_sharing_violation<T: std::fmt::Debug>(result: io::Result<T>, context: &str) {
    let error = result.expect_err(context);
    assert_eq!(error.raw_os_error(), Some(32), "{context}: {error}");
}

fn read_exact_fixture(path: &Path) -> Vec<u8> {
    let mut bytes = Vec::new();
    File::open(path).unwrap().read_to_end(&mut bytes).unwrap();
    bytes
}

#[test]
fn shared_record_handle_reads_real_bytes_allows_readers_and_refuses_write_delete() {
    let fixture = Fixture::new();
    let mut first = native_record_handle(&fixture.record(), true).expect("open reviewed record");
    let second =
        native_record_handle(&fixture.record(), true).expect("share another reviewed read");
    let ordinary_reader = File::open(fixture.record()).expect("share an ordinary read");
    let mut content = Vec::new();
    first
        .read_to_end(&mut content)
        .expect("read exact record bytes");
    assert_eq!(content, CONTENT);
    assert_sharing_violation(
        OpenOptions::new().write(true).open(fixture.record()),
        "record writer must be refused while reviewed read exists",
    );
    assert_sharing_violation(
        fs::remove_file(fixture.record()),
        "record deletion must be refused while reviewed read exists",
    );
    assert!(native_record_handle(&fixture.record(), false).is_err());
    drop(ordinary_reader);
    drop(second);
    drop(first);
    let mut writer = OpenOptions::new()
        .append(true)
        .open(fixture.record())
        .expect("dropping reviewed readers releases write denial");
    writer.write_all(b"after-drop\n").unwrap();
    drop(writer);
    assert_eq!(
        read_exact_fixture(&fixture.record()),
        [CONTENT, b"after-drop\n"].concat()
    );
    fs::remove_file(fixture.record()).expect("dropping readers releases delete denial");
}

#[test]
fn existing_writer_prevents_reviewed_read_until_writer_is_dropped() {
    let fixture = Fixture::new();
    let writer = OpenOptions::new()
        .write(true)
        .open(fixture.record())
        .unwrap();
    assert!(native_record_handle(&fixture.record(), true).is_err());
    drop(writer);
    let reviewed = native_record_handle(&fixture.record(), true)
        .expect("record read succeeds after existing writer closes");
    drop(reviewed);
    assert_eq!(read_exact_fixture(&fixture.record()), CONTENT);
}

#[test]
fn exclusive_record_lock_refuses_other_handles_and_releases_on_drop() {
    let fixture = Fixture::new();
    let lock = native_record_handle(&fixture.record(), false).expect("acquire exclusive lock");
    assert!(native_record_handle(&fixture.record(), false).is_err());
    assert!(native_record_handle(&fixture.record(), true).is_err());
    assert_sharing_violation(
        File::open(fixture.record()),
        "exclusive lock refuses an ordinary reader",
    );
    assert_sharing_violation(
        OpenOptions::new().write(true).open(fixture.record()),
        "exclusive lock refuses an ordinary writer",
    );
    assert_sharing_violation(
        fs::remove_file(fixture.record()),
        "exclusive lock refuses deletion",
    );
    drop(lock);
    let reacquired =
        native_record_handle(&fixture.record(), false).expect("reacquire dropped lock");
    drop(reacquired);
    let reviewed = native_record_handle(&fixture.record(), true).expect("read after lock drop");
    drop(reviewed);
    fs::remove_file(fixture.record()).expect("lock release permits exact file cleanup");
}

#[test]
fn native_record_handle_refuses_directory_without_retaining_a_handle() {
    let fixture = Fixture::new();
    assert!(native_record_handle(&fixture.root, true).is_err());
    assert!(native_record_handle(&fixture.root, false).is_err());
    // Fixture Drop's nonrecursive remove_dir proves rejected opens released
    // any acquired handle; its exact regular file is still untouched here.
    assert_eq!(read_exact_fixture(&fixture.record()), CONTENT);
}

#[test]
fn native_record_handle_refuses_reparse_file_without_touching_target() {
    let fixture = Fixture::new();
    if let Err(error) = std::os::windows::fs::symlink_file(fixture.record(), fixture.link()) {
        match error.raw_os_error() {
            Some(1 | 5 | 50 | 1314) => {
                eprintln!("SKIP native reparse fixture: OS/filesystem does not permit an unprivileged symlink: {error}");
                return;
            }
            _ => panic!("unexpected symlink fixture failure: {error}"),
        }
    }
    assert!(fs::symlink_metadata(fixture.link())
        .unwrap()
        .file_type()
        .is_symlink());
    assert!(native_record_handle(&fixture.link(), true).is_err());
    assert!(native_record_handle(&fixture.link(), false).is_err());
    fs::remove_file(fixture.link()).expect("rejected reparse opens release the link handle");
    assert_eq!(read_exact_fixture(&fixture.record()), CONTENT);
}
