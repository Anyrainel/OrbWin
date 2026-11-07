//! Fixed, native subprocess fixtures. This module exists only in Windows tests;
//! no renderer, shell program or installed binary can invoke these modes.
use std::{
    io::Write,
    path::PathBuf,
    process::{Command, Stdio},
    sync::atomic::{AtomicU64, Ordering},
    time::{Duration, SystemTime, UNIX_EPOCH},
};
static NEXT_FIXTURE: AtomicU64 = AtomicU64::new(0);

pub(crate) struct Fixture {
    directory: PathBuf,
}
impl Fixture {
    pub(crate) fn new() -> Self {
        let directory = std::env::temp_dir().join(format!(
            "orbwin-native-process-{}-{}-{}",
            std::process::id(),
            SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap()
                .as_nanos(),
            NEXT_FIXTURE.fetch_add(1, Ordering::Relaxed)
        ));
        std::fs::create_dir(&directory).unwrap();
        Self { directory }
    }
    pub(crate) fn pid_path(&self) -> PathBuf {
        self.directory.join("root.pid")
    }
    pub(crate) fn command(&self, mode: &str) -> Command {
        assert!(["sleep", "output", "failure"].contains(&mode));
        let mut command = Command::new(std::env::current_exe().unwrap());
        command
            .args([
                "--ignored",
                "--exact",
                "process_fixture::native_child",
                "--nocapture",
            ])
            .env("ORBWIN_NATIVE_PROCESS_FIXTURE", mode)
            .env("ORBWIN_NATIVE_PROCESS_PID", self.pid_path())
            .stdin(Stdio::null())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped());
        command
    }
}
impl Drop for Fixture {
    fn drop(&mut self) {
        // Never recurse into a computed path. Only these fixture-owned entries
        // are removed; unknown contents keep the unique directory for review.
        let _ = std::fs::remove_file(self.pid_path());
        let _ = std::fs::remove_dir(&self.directory);
    }
}

#[test]
#[ignore = "fixed subprocess fixture, invoked by process ownership tests"]
fn native_child() {
    let Ok(mode) = std::env::var("ORBWIN_NATIVE_PROCESS_FIXTURE") else {
        return;
    };
    assert!(["sleep", "output", "failure"].contains(&mode.as_str()));
    let path =
        PathBuf::from(std::env::var_os("ORBWIN_NATIVE_PROCESS_PID").expect("fixture PID path"));
    let mut file = std::fs::OpenOptions::new()
        .create_new(true)
        .write(true)
        .open(path)
        .unwrap();
    writeln!(file, "{}", std::process::id()).unwrap();
    file.sync_all().unwrap();
    drop(file);
    match mode.as_str() {
        "output" => {
            std::io::stdout().write_all(&vec![b'x'; 140_000]).unwrap();
            std::io::stdout().flush().unwrap();
        }
        "failure" => {
            std::io::stderr()
                .write_all(b"native expected failure")
                .unwrap();
            std::io::stderr().flush().unwrap();
            std::process::exit(7);
        }
        "sleep" => (),
        _ => unreachable!(),
    }
    // Bound even a fixture whose test owner dies before it can reap its child.
    std::thread::sleep(Duration::from_secs(20));
}

pub(crate) async fn retain_running_process(fixture: &Fixture) -> std::os::windows::io::OwnedHandle {
    use std::os::windows::io::FromRawHandle;
    use windows_sys::Win32::System::Threading::{OpenProcess, PROCESS_SYNCHRONIZE};
    let deadline = tokio::time::Instant::now() + Duration::from_secs(5);
    let started = std::time::Instant::now();
    loop {
        if let Ok(text) = std::fs::read_to_string(fixture.pid_path()) {
            if let Some(pid) = text
                .strip_suffix('\n')
                .and_then(|text| text.parse::<u32>().ok())
            {
                // Keep the handle while the fixture is known alive. Later wait
                // assertions cannot accidentally observe a reused numeric PID.
                let raw = unsafe { OpenProcess(PROCESS_SYNCHRONIZE, 0, pid) };
                assert!(!raw.is_null(), "could not retain fixture process");
                eprintln!(
                    "Acknowledged native fixture in {} ms",
                    started.elapsed().as_millis()
                );
                return unsafe { std::os::windows::io::OwnedHandle::from_raw_handle(raw) };
            }
        }
        assert!(
            tokio::time::Instant::now() < deadline,
            "native fixture did not acknowledge startup"
        );
        tokio::time::sleep(Duration::from_millis(10)).await;
    }
}
pub(crate) fn assert_exited(handle: &std::os::windows::io::OwnedHandle) {
    use std::os::windows::io::AsRawHandle;
    assert_eq!(
        unsafe {
            windows_sys::Win32::System::Threading::WaitForSingleObject(handle.as_raw_handle(), 1000)
        },
        0,
        "owned fixture process survived completion"
    );
}
