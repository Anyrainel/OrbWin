//! A job owns only transient client processes, never rdd service/guest lifecycle.
//! Hold it until pipes are drained and the root is reaped. Closing the handle
//! kills assigned clients and descendants, including on desktop process loss.
//! Assignment happens immediately after spawn, before sending any input. It can
//! fail (for example under an incompatible host job); callers must kill/reap
//! the client and report failure rather than run it without containment.
#[cfg(windows)]
pub struct ClientJob {
    _handle: std::os::windows::io::OwnedHandle,
}

#[cfg(all(test, windows))]
mod tests {
    use super::*;
    use std::{
        os::windows::{
            io::{AsRawHandle, FromRawHandle, OwnedHandle},
            process::CommandExt,
        },
        process::{Command, Stdio},
        time::{Duration, Instant},
    };
    use windows_sys::Win32::System::Threading::{
        OpenProcess, WaitForSingleObject, PROCESS_SYNCHRONIZE,
    };

    #[test]
    fn closing_job_reaps_client_tree_without_desktop_cleanup() {
        let path = std::env::temp_dir().join(format!(
            "orbwin-job-{}-{}.pid",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        let shell = std::path::PathBuf::from(std::env::var_os("SystemRoot").unwrap())
            .join("System32/WindowsPowerShell/v1.0/powershell.exe");
        // Fixed script; the output path is an environment value, never code.
        let mut child = Command::new(&shell).args(["-NoProfile", "-NonInteractive", "-Command", "$p = Start-Process -FilePath (Join-Path $PSHOME 'powershell.exe') -ArgumentList '-NoProfile','-NonInteractive','-Command','Start-Sleep -Seconds 30' -WindowStyle Hidden -PassThru; [IO.File]::WriteAllText($env:ORBWIN_JOB_TEST_PID, [string]$p.Id); Start-Sleep -Seconds 30"])
            .env("ORBWIN_JOB_TEST_PID", &path).stdin(Stdio::null()).stdout(Stdio::null()).stderr(Stdio::null()).creation_flags(0x08000000).spawn().unwrap();
        let job = ClientJob::attach(child.as_raw_handle()).unwrap();
        let deadline = Instant::now() + Duration::from_secs(10);
        let pid = loop {
            if let Ok(text) = std::fs::read_to_string(&path) {
                if let Ok(pid) = text.parse::<u32>() {
                    break pid;
                }
            }
            assert!(
                Instant::now() < deadline,
                "client did not create child PID record"
            );
            std::thread::sleep(Duration::from_millis(25));
        };
        // SAFETY: PID was returned by the spawned client. Own the handle before
        // dropping the job so PID reuse cannot affect the termination check.
        let descendant = unsafe {
            let raw = OpenProcess(PROCESS_SYNCHRONIZE, 0, pid);
            assert!(!raw.is_null());
            OwnedHandle::from_raw_handle(raw)
        };
        drop(job);
        assert!(child.wait().is_ok());
        // WAIT_OBJECT_0 proves the descendant exited, rather than only its root.
        assert_eq!(
            unsafe { WaitForSingleObject(descendant.as_raw_handle(), 5000) },
            0
        );
        std::fs::remove_file(path).unwrap();
    }
}

#[cfg(windows)]
impl ClientJob {
    pub fn attach(process: std::os::windows::io::RawHandle) -> Result<Self, String> {
        use std::os::windows::io::{AsRawHandle, FromRawHandle, OwnedHandle};
        use windows_sys::Win32::System::JobObjects::*;
        // SAFETY: unnamed non-inheritable job; OwnedHandle uniquely owns the
        // returned handle. Both Win32 calls borrow valid handles/struct storage.
        unsafe {
            let raw = CreateJobObjectW(std::ptr::null(), std::ptr::null());
            if raw.is_null() {
                return Err(std::io::Error::last_os_error().to_string());
            }
            let handle = OwnedHandle::from_raw_handle(raw);
            let mut limits: JOBOBJECT_EXTENDED_LIMIT_INFORMATION = std::mem::zeroed();
            limits.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;
            if SetInformationJobObject(
                handle.as_raw_handle(),
                JobObjectExtendedLimitInformation,
                (&limits as *const JOBOBJECT_EXTENDED_LIMIT_INFORMATION).cast(),
                std::mem::size_of_val(&limits) as u32,
            ) == 0
                || AssignProcessToJobObject(handle.as_raw_handle(), process) == 0
            {
                return Err(format!(
                    "Could not contain client process: {}",
                    std::io::Error::last_os_error()
                ));
            }
            Ok(Self { _handle: handle })
        }
    }
}
