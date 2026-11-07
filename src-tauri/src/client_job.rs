//! A job owns only transient client processes, never rdd service/guest lifecycle.
//! Hold it until pipes are drained and the root is reaped. Closing the handle
//! kills assigned clients and descendants, including on desktop process loss.
//! Assignment happens after spawn, before sending any input; this is not atomic
//! process-creation containment (see architecture/client-processes.md). It can
//! fail (for example under an incompatible host job); callers must kill/reap
//! the client and report failure rather than run it without containment.
#[cfg(windows)]
pub struct ClientJob {
    _handle: std::os::windows::io::OwnedHandle,
    root: std::os::windows::io::OwnedHandle,
    writer_binding: Option<[u8; 32]>,
}

// Root Wait plus exact-job ActiveProcesses=0 establishes execution quiescence,
// not that every historical process object is signaled or pending IO is done.
// Only the producer mints this one-use capability. A writer-bound instance is a
// necessary input to the known-auth-writer owner's separate storage commit; it
// never grants arbitrary file adoption, flush success or intent clearance.
pub(crate) struct ExecutionQuiesced {
    #[cfg(windows)]
    job: ClientJob,
    #[cfg(not(windows))]
    _unavailable: (),
}

#[cfg(windows)]
impl ExecutionQuiesced {
    pub(crate) fn consume_for_writer(
        self,
        admission: &crate::client_config::WriterAdmission,
    ) -> Result<(), String> {
        if self.job.writer_binding != Some(admission.binding()) {
            return Err(
                "Execution quiescence does not match this configuration writer loan.".into(),
            );
        }
        Ok(())
    }
}

#[cfg(windows)]
static RETIREMENT_UNKNOWN: std::sync::atomic::AtomicBool =
    std::sync::atomic::AtomicBool::new(false);

// Bounded, read-only observations of the exact retained job. The accounting
// queries surround (and are not atomic with) the PID-list query. This record is
// diagnostic data, never a retirement or writer capability.
#[cfg(windows)]
#[derive(Debug)]
pub(crate) struct ClientJobObservation {
    pub(crate) active_before: u32,
    pub(crate) total_before: u32,
    pub(crate) pids: Vec<u32>,
    pub(crate) active_after: u32,
    pub(crate) total_after: u32,
}

#[cfg(windows)]
impl ExecutionQuiesced {
    pub(crate) fn observation(&self) -> Result<ClientJobObservation, String> {
        self.job.observation()
    }
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

    #[test]
    fn execution_capability_has_a_separate_positive_descendant_signal_wait() {
        let path = std::env::temp_dir().join(format!(
            "orbwin-job-retirement-{}-{}.pid",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        let release = path.with_extension("start");
        let shell = std::path::PathBuf::from(std::env::var_os("SystemRoot").unwrap())
            .join("System32/WindowsPowerShell/v1.0/powershell.exe");
        let mut child = Command::new(&shell).args(["-NoProfile", "-NonInteractive", "-Command", "$deadline = [DateTime]::UtcNow.AddSeconds(5); while (-not (Test-Path -LiteralPath $env:ORBWIN_JOB_TEST_RELEASE)) { if ([DateTime]::UtcNow -gt $deadline) { exit 70 }; Start-Sleep -Milliseconds 25 }; $start = New-Object System.Diagnostics.ProcessStartInfo; $start.FileName = Join-Path $PSHOME 'powershell.exe'; $start.Arguments = '-NoProfile -NonInteractive -Command \"Start-Sleep -Seconds 30\"'; $start.UseShellExecute = $false; $start.CreateNoWindow = $true; $p = [System.Diagnostics.Process]::Start($start); [IO.File]::WriteAllText($env:ORBWIN_JOB_TEST_PID, [string]$p.Id)"])
            .env("ORBWIN_JOB_TEST_PID", &path).env("ORBWIN_JOB_TEST_RELEASE", &release).stdin(Stdio::null()).stdout(Stdio::null()).stderr(Stdio::null()).creation_flags(0x08000000).spawn().unwrap();
        let job = ClientJob::attach_bound(child.as_raw_handle(), Some([7; 32])).unwrap();
        std::fs::write(&release, b"ready").unwrap();
        let deadline = Instant::now() + Duration::from_secs(10);
        loop {
            if let Some(status) = child.try_wait().unwrap() {
                assert!(status.success());
                break;
            }
            if Instant::now() >= deadline {
                let _ = child.kill();
                let _ = child.wait();
                panic!("inert root did not exit within the fixture deadline");
            }
            std::thread::sleep(Duration::from_millis(25));
        }
        let pid = std::fs::read_to_string(&path).unwrap().parse().unwrap();
        let descendant = unsafe {
            let raw = OpenProcess(PROCESS_SYNCHRONIZE, 0, pid);
            assert!(!raw.is_null());
            OwnedHandle::from_raw_handle(raw)
        };
        // Root Wait has completed, but its child is still active. Execution
        // settlement must observe the actual job, not infer inactivity from
        // root status. A separate process-handle Wait below proves full signal.
        assert_eq!(
            unsafe { WaitForSingleObject(descendant.as_raw_handle(), 0) },
            258
        );
        let retired = job.into_execution_quiesced().unwrap();
        assert_eq!(retired.job.writer_binding, Some([7; 32]));
        assert_eq!(
            unsafe { WaitForSingleObject(descendant.as_raw_handle(), 5000) },
            0
        );
        drop(retired);
        std::fs::remove_file(path).unwrap();
        std::fs::remove_file(release).unwrap();
    }
}

#[cfg(windows)]
impl ClientJob {
    pub(crate) fn observation(&self) -> Result<ClientJobObservation, String> {
        use std::os::windows::io::AsRawHandle;
        use windows_sys::Win32::System::JobObjects::*;
        #[repr(C)]
        struct ProcessIds {
            assigned: u32,
            listed: u32,
            ids: [usize; 256],
        }
        let accounting = || -> Result<JOBOBJECT_BASIC_ACCOUNTING_INFORMATION, String> {
            let mut info: JOBOBJECT_BASIC_ACCOUNTING_INFORMATION = unsafe { std::mem::zeroed() };
            if unsafe {
                QueryInformationJobObject(
                    self._handle.as_raw_handle(),
                    JobObjectBasicAccountingInformation,
                    (&mut info as *mut JOBOBJECT_BASIC_ACCOUNTING_INFORMATION).cast(),
                    std::mem::size_of_val(&info) as u32,
                    std::ptr::null_mut(),
                )
            } == 0
            {
                return Err("Client job accounting observation failed.".into());
            }
            Ok(info)
        };
        let before = accounting()?;
        let mut list = ProcessIds {
            assigned: 0,
            listed: 0,
            ids: [0; 256],
        };
        if unsafe {
            QueryInformationJobObject(
                self._handle.as_raw_handle(),
                JobObjectBasicProcessIdList,
                (&mut list as *mut ProcessIds).cast(),
                std::mem::size_of_val(&list) as u32,
                std::ptr::null_mut(),
            )
        } == 0
            || list.assigned != list.listed
            || list.listed as usize > list.ids.len()
        {
            return Err("Client job process-list observation is incomplete.".into());
        }
        let mut pids = Vec::new();
        for &id in &list.ids[..list.listed as usize] {
            let pid = u32::try_from(id).map_err(|_| "Client job process identifier is invalid.")?;
            if pid == 0 || pids.contains(&pid) {
                return Err("Client job process identifier is invalid.".into());
            }
            pids.push(pid);
        }
        let after = accounting()?;
        Ok(ClientJobObservation {
            active_before: before.ActiveProcesses,
            total_before: before.TotalProcesses,
            pids,
            active_after: after.ActiveProcesses,
            total_after: after.TotalProcesses,
        })
    }
    pub fn attach(process: std::os::windows::io::RawHandle) -> Result<Self, String> {
        Self::attach_bound(process, None)
    }
    pub(crate) fn attach_for_writer(
        process: std::os::windows::io::RawHandle,
        admission: &crate::client_config::WriterAdmission,
    ) -> Result<Self, String> {
        Self::attach_bound(process, Some(admission.binding()))
    }
    // Only the native CreateProcess JOB_LIST producer calls this while the
    // returned initial thread is suspended. This constructor never assigns a
    // process after creation and never resolves an ambient PID.
    pub(crate) fn from_atomic(
        handle: std::os::windows::io::OwnedHandle,
        process: std::os::windows::io::RawHandle,
        writer_binding: Option<[u8; 32]>,
    ) -> Result<Self, String> {
        use std::os::windows::io::{AsRawHandle, FromRawHandle, OwnedHandle};
        use windows_sys::Win32::Foundation::{DuplicateHandle, DUPLICATE_SAME_ACCESS};
        use windows_sys::Win32::System::{JobObjects::*, Threading::*};
        if RETIREMENT_UNKNOWN.load(std::sync::atomic::Ordering::Acquire) {
            return Err("A prior client tree has unconfirmed retirement.".into());
        }
        unsafe {
            let mut belongs = 0;
            let mut limits: JOBOBJECT_EXTENDED_LIMIT_INFORMATION = std::mem::zeroed();
            if IsProcessInJob(process, handle.as_raw_handle(), &mut belongs) == 0
                || belongs == 0
                || QueryInformationJobObject(
                    handle.as_raw_handle(),
                    JobObjectExtendedLimitInformation,
                    (&mut limits as *mut JOBOBJECT_EXTENDED_LIMIT_INFORMATION).cast(),
                    std::mem::size_of_val(&limits) as u32,
                    std::ptr::null_mut(),
                ) == 0
                || limits.BasicLimitInformation.LimitFlags != JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
            {
                return Err(
                    "The created client does not belong to the exact transient job.".into(),
                );
            }
            let mut root = std::ptr::null_mut();
            if DuplicateHandle(
                GetCurrentProcess(),
                process,
                GetCurrentProcess(),
                &mut root,
                0,
                0,
                DUPLICATE_SAME_ACCESS,
            ) == 0
            {
                return Err("Could not retain the created client process.".into());
            }
            Ok(Self {
                _handle: handle,
                root: OwnedHandle::from_raw_handle(root),
                writer_binding,
            })
        }
    }

    pub(crate) fn request_termination(&self) -> Result<(), String> {
        use std::os::windows::io::AsRawHandle;
        // This is only a kill request. Native root/job observations below are
        // still required before returning a configuration writer capability.
        if unsafe {
            windows_sys::Win32::System::JobObjects::TerminateJobObject(
                self._handle.as_raw_handle(),
                125,
            )
        } == 0
        {
            Err("Client termination could not be requested.".into())
        } else {
            Ok(())
        }
    }
    fn attach_bound(
        process: std::os::windows::io::RawHandle,
        writer_binding: Option<[u8; 32]>,
    ) -> Result<Self, String> {
        use std::os::windows::io::{AsRawHandle, FromRawHandle, OwnedHandle};
        use windows_sys::Win32::Foundation::{DuplicateHandle, DUPLICATE_SAME_ACCESS};
        use windows_sys::Win32::System::JobObjects::*;
        use windows_sys::Win32::System::Threading::GetCurrentProcess;
        if RETIREMENT_UNKNOWN.load(std::sync::atomic::Ordering::Acquire) {
            return Err(
                "A prior client tree has unconfirmed retirement; no new client was admitted."
                    .into(),
            );
        }
        // SAFETY: unnamed non-inheritable job; OwnedHandle uniquely owns the
        // returned handle. Both Win32 calls borrow valid handles/struct storage.
        unsafe {
            let raw = CreateJobObjectW(std::ptr::null(), std::ptr::null());
            if raw.is_null() {
                return Err(std::io::Error::last_os_error().to_string());
            }
            let handle = OwnedHandle::from_raw_handle(raw);
            let mut root = std::ptr::null_mut();
            if DuplicateHandle(
                GetCurrentProcess(),
                process,
                GetCurrentProcess(),
                &mut root,
                0,
                0,
                DUPLICATE_SAME_ACCESS,
            ) == 0
            {
                return Err("Could not retain the original client process handle.".into());
            }
            let root = OwnedHandle::from_raw_handle(root);
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
            Ok(Self {
                _handle: handle,
                root,
                writer_binding,
            })
        }
    }

    // Caller has relinquished/drained every stdio endpoint and positively waited
    // its original root. This independently requires that root handle signaled
    // and the actual job inactive. TerminateJobObject requests execution stop;
    // accounting zero does NOT wait all historical descendant process objects,
    // nor prove canceled pending IO/mappings/renames complete. The separate
    // pinned auth-writer + owner-new-file commit contract is still mandatory.
    // Legacy attach users retain their documented spawn/assignment window.
    pub(crate) fn into_execution_quiesced(self) -> Result<ExecutionQuiesced, String> {
        use std::os::windows::io::AsRawHandle;
        use std::time::{Duration, Instant};
        use windows_sys::Win32::System::JobObjects::*;
        use windows_sys::Win32::System::Threading::WaitForSingleObject;
        let result = (|| {
            // SAFETY: both handles are uniquely owned by this live ClientJob;
            // the accounting buffer is an aligned SDK structure of exact size.
            unsafe {
                if WaitForSingleObject(self.root.as_raw_handle(), 0) != 0 {
                    return Err("The original client process has not positively exited.".into());
                }
                let active = || -> Result<u32, String> {
                    let mut accounting: JOBOBJECT_BASIC_ACCOUNTING_INFORMATION = std::mem::zeroed();
                    if QueryInformationJobObject(
                        self._handle.as_raw_handle(),
                        JobObjectBasicAccountingInformation,
                        (&mut accounting as *mut JOBOBJECT_BASIC_ACCOUNTING_INFORMATION).cast(),
                        std::mem::size_of_val(&accounting) as u32,
                        std::ptr::null_mut(),
                    ) == 0
                    {
                        return Err("Client tree membership could not be observed.".into());
                    }
                    Ok(accounting.ActiveProcesses)
                };
                if active()? != 0 {
                    if TerminateJobObject(self._handle.as_raw_handle(), 125) == 0 {
                        return Err("Client descendants could not be retired.".into());
                    }
                    let deadline = Instant::now() + Duration::from_secs(2);
                    while active()? != 0 {
                        if Instant::now() >= deadline {
                            return Err("Client descendant retirement remains unconfirmed.".into());
                        }
                        std::thread::sleep(Duration::from_millis(10));
                    }
                }
                Ok(())
            }
        })();
        match result {
            Ok(()) => Ok(ExecutionQuiesced { job: self }),
            Err(error) => {
                RETIREMENT_UNKNOWN.store(true, std::sync::atomic::Ordering::Release);
                // Keep the actual job/root handles and their exclusion until
                // process exit. Unknown retirement cannot authorize a new loan.
                std::mem::forget(self);
                Err(error)
            }
        }
    }
}
