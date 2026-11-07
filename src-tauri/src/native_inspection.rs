// Included inside native_client::native to reuse the existing private pipe,
// cache and admission APIs. This closed profile never calls a kill path.
mod inspection_owner {
    use super::*;
    use windows_sys::Win32::System::SystemInformation::GetWindowsDirectoryW;

    struct Frame {
        process: PROCESS_INFORMATION,
        job: HANDLE,
        parent_member: i32,
        windows: Vec<u16>,
        application: Vec<u16>,
        command: Vec<u16>,
        environment: Vec<u16>,
        cwd: Vec<u16>,
        handles: [HANDLE; 3],
        job_list: [HANDLE; 1],
        startup: STARTUPINFOEXW,
        attributes: Option<std::mem::ManuallyDrop<Attributes>>,
        child_member: i32,
        accounting: JOBOBJECT_BASIC_ACCOUNTING_INFORMATION,
        exit_code: u32,
        limits: JOBOBJECT_EXTENDED_LIMIT_INFORMATION,
        create_entered: bool,
        close_attempted: [bool; 3], // thread, job, original process last
    }
    unsafe impl Send for Frame {}
    struct Owner {
        frame: Box<Frame>,
        children: [Option<OwnedHandle>; 3],
        pipes: Pipes,
        executable: CachedExecutable,
        _permit: Permit,
        unknown: Vec<usize>,
    }
    // Sole owner moves between the creation continuation and one IO join worker.
    // Every native pointer targets its boxed frame or stable Vec allocation;
    // no concurrent mutation or public raw-handle access is possible.
    unsafe impl Send for Owner {}

    struct InspectionGuard(Option<Owner>);
    impl Drop for InspectionGuard {
        fn drop(&mut self) {
            if let Some(owner) = self.0.take() {
                abandon_owner(owner, &UNKNOWN, |_| {});
            }
        }
    }
    fn close_raw(slot: &mut HANDLE, attempted: &mut bool) -> Result<(), String> {
        if slot.is_null() {
            return Ok(());
        }
        if *attempted {
            return Err("Inspection native close is unknown.".into());
        }
        *attempted = true;
        if unsafe { CloseHandle(*slot) } == 0 {
            return Err(error("inspection checked close"));
        }
        *slot = null_mut();
        Ok(())
    }
    fn close_known(owner: &mut Owner) -> Result<(), String> {
        for child in &mut owner.children {
            close_checked(child, &mut owner.unknown)?;
        }
        let f = &mut owner.frame;
        close_raw(&mut f.process.hThread, &mut f.close_attempted[0])?;
        // Attributes have no pending native operation after a known return.
        // Delete's void normal return is not a raw CloseHandle certificate.
        if let Some(attributes) = f.attributes.as_mut() {
            unsafe { DeleteProcThreadAttributeList(attributes.ptr) };
            attributes.ptr = null_mut();
            // Keep original backing in the frame until the void delete returned.
            drop(std::mem::take(&mut attributes.storage));
        }
        f.attributes = None;
        close_raw(&mut f.job, &mut f.close_attempted[1])?;
        close_raw(&mut f.process.hProcess, &mut f.close_attempted[2])?;
        owner.executable.check_held_origin()
    }
    impl InspectionGuard {
        fn fail(mut self, message: String) -> String {
            let owner = self.0.take().unwrap();
            // Entered false/partial creation is never converted to no-birth.
            if owner.frame.create_entered || UNKNOWN.load(Ordering::Acquire) {
                abandon_owner(owner, &UNKNOWN, |_| {});
            } else {
                let _ = finish_owner(owner, &UNKNOWN, close_known);
            }
            message
        }
    }

    pub(crate) struct InspectionClient {
        guard: InspectionGuard,
    }
    impl InspectionClient {
        // Fixed capability input only. The transport obtains this from the
        // independently pinned bundle; no renderer argv/env/kill option exists.
        pub(crate) async fn spawn(
            package: crate::runtime_inspection_transport::InspectionExecutable,
        ) -> Result<Self, String> {
            let executable = package.into_cache();
            let permit = Permit::acquire()?;
            executable.check_held_origin()?;
            let frame = Box::new(Frame {
                process: unsafe { zeroed() },
                job: null_mut(),
                parent_member: 2,
                windows: vec![0; 32768],
                application: Vec::new(),
                command: Vec::new(),
                environment: Vec::new(),
                cwd: Vec::new(),
                handles: [null_mut(); 3],
                job_list: [null_mut()],
                startup: unsafe { zeroed() },
                attributes: None,
                child_member: 2,
                accounting: unsafe { zeroed() },
                exit_code: 259,
                limits: unsafe { zeroed() },
                create_entered: false,
                close_attempted: [false; 3],
            });
            let mut guard = InspectionGuard(Some(Owner {
                frame,
                executable,
                _permit: permit,
                children: [None, None, None],
                pipes: Pipes::default(),
                unknown: Vec::new(),
            }));
            let preparation = async {
                let o = guard.0.as_mut().unwrap();
                let f = &mut o.frame;
                if unsafe { IsProcessInJob(GetCurrentProcess(), null_mut(), &mut f.parent_member) }
                    == 0
                    || f.parent_member != 0
                {
                    return Err("Inspection requires an independent parent job context.".into());
                }
                let n =
                    unsafe { GetWindowsDirectoryW(f.windows.as_mut_ptr(), f.windows.len() as u32) }
                        as usize;
                if n == 0 || n >= f.windows.len() || f.windows[..n].contains(&0) {
                    return Err("Inspection Windows environment unavailable.".into());
                }
                f.environment = environment_block(vec![
                    (
                        "SystemRoot".encode_utf16().collect(),
                        f.windows[..n].to_vec(),
                    ),
                    ("WINDIR".encode_utf16().collect(), f.windows[..n].to_vec()),
                    (
                        "GOTRACEBACK".encode_utf16().collect(),
                        "none".encode_utf16().collect(),
                    ),
                    (
                        "RDD_INSTANCE".encode_utf16().collect(),
                        "orbwin".encode_utf16().collect(),
                    ),
                    (
                        "RDD_LOG_LEVEL".encode_utf16().collect(),
                        "error".encode_utf16().collect(),
                    ),
                ])?;
                f.application = o
                    .executable
                    .path()
                    .as_os_str()
                    .encode_wide()
                    .chain(Some(0))
                    .collect();
                if !o.executable.path().is_absolute()
                    || f.application.len() > 32761
                    || f.application[..f.application.len() - 1].contains(&0)
                {
                    return Err("Inspection cached image path invalid.".into());
                }
                f.command = command_line(&[
                    f.application[..f.application.len() - 1].to_vec(),
                    "--orbwin-runtime-inspect".encode_utf16().collect(),
                ])?;
                f.cwd = o
                    .executable
                    .path()
                    .parent()
                    .ok_or("Inspection cache directory unavailable.")?
                    .as_os_str()
                    .encode_wide()
                    .chain(Some(0))
                    .collect();
                let (descriptor, user, _session) = user_descriptor()?;
                f.job = unsafe { CreateJobObjectW(null(), null()) };
                if f.job.is_null() || f.job == INVALID_HANDLE_VALUE {
                    return Err(error("inspection job"));
                }
                f.limits.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_ACTIVE_PROCESS;
                f.limits.BasicLimitInformation.ActiveProcessLimit = 1;
                if unsafe {
                    SetInformationJobObject(
                        f.job,
                        JobObjectExtendedLimitInformation,
                        (&f.limits as *const JOBOBJECT_EXTENDED_LIMIT_INFORMATION).cast(),
                        size_of::<JOBOBJECT_EXTENDED_LIMIT_INFORMATION>() as u32,
                    )
                } == 0
                {
                    return Err(error("inspection non-kill job limit"));
                }
                for index in 0..3 {
                    let (pipe, child) = pipe_pair(&descriptor, &user, index == 0).await?;
                    o.children[index] = Some(child);
                    match index {
                        0 => o.pipes.stdin = Some(pipe),
                        1 => o.pipes.stdout = Some(pipe),
                        _ => o.pipes.stderr = Some(pipe),
                    }
                }
                f.handles =
                    std::array::from_fn(|i| o.children[i].as_ref().unwrap().as_raw_handle());
                f.job_list = [f.job];
                f.attributes = Some(std::mem::ManuallyDrop::new(Attributes::new()?));
                let attributes = f.attributes.as_ref().unwrap().ptr;
                if unsafe {
                    UpdateProcThreadAttribute(
                        attributes,
                        0,
                        PROC_THREAD_ATTRIBUTE_JOB_LIST as usize,
                        f.job_list.as_ptr().cast(),
                        size_of_val(&f.job_list),
                        null_mut(),
                        null(),
                    )
                } == 0
                    || unsafe {
                        UpdateProcThreadAttribute(
                            attributes,
                            0,
                            PROC_THREAD_ATTRIBUTE_HANDLE_LIST as usize,
                            f.handles.as_ptr().cast(),
                            size_of_val(&f.handles),
                            null_mut(),
                            null(),
                        )
                    } == 0
                {
                    return Err(error("inspection atomic attributes"));
                }
                f.startup.StartupInfo.cb = size_of::<STARTUPINFOEXW>() as u32;
                f.startup.StartupInfo.dwFlags = STARTF_USESTDHANDLES;
                f.startup.StartupInfo.hStdInput = f.handles[0];
                f.startup.StartupInfo.hStdOutput = f.handles[1];
                f.startup.StartupInfo.hStdError = f.handles[2];
                f.startup.lpAttributeList = attributes;
                o.executable.check_held_origin()?;
                if UNKNOWN.load(Ordering::Acquire) {
                    return Err("Inspection admission became unknown.".into());
                }
                f.create_entered = true;
                if unsafe {
                    CreateProcessW(
                        f.application.as_ptr(),
                        f.command.as_mut_ptr(),
                        null(),
                        null(),
                        1,
                        CREATE_NO_WINDOW
                            | CREATE_SUSPENDED
                            | CREATE_UNICODE_ENVIRONMENT
                            | EXTENDED_STARTUPINFO_PRESENT,
                        f.environment.as_mut_ptr().cast(),
                        f.cwd.as_ptr(),
                        &f.startup.StartupInfo,
                        &mut f.process,
                    )
                } == 0
                {
                    return Err(error("inspection original creation"));
                }
                if f.process.hProcess.is_null()
                    || f.process.hProcess == INVALID_HANDLE_VALUE
                    || f.process.hThread.is_null()
                    || f.process.hThread == INVALID_HANDLE_VALUE
                {
                    return Err("Inspection partial creation retained.".into());
                }
                // Use the actual original HANDLE, never reopen a numeric PID.
                o.executable.verify_original_process(unsafe {
                    BorrowedHandle::borrow_raw(f.process.hProcess)
                })?;
                if unsafe { IsProcessInJob(f.process.hProcess, f.job, &mut f.child_member) } == 0
                    || f.child_member != 1
                {
                    return Err("Inspection atomic job binding unknown.".into());
                }
                for child in &mut o.children {
                    close_checked(child, &mut o.unknown)?;
                }
                if unsafe { ResumeThread(f.process.hThread) } != 1 {
                    return Err(error("inspection first resume"));
                }
                close_raw(&mut f.process.hThread, &mut f.close_attempted[0])?;
                Ok(())
            }
            .await;
            match preparation {
                Ok(()) => Ok(Self { guard }),
                Err(e) => Err(guard.fail(e)),
            }
        }
        pub(crate) fn take_stdin(&mut self) -> Option<NamedPipeServer> {
            self.guard.0.as_mut().unwrap().pipes.stdin.take()
        }
        pub(crate) fn take_stdout(&mut self) -> Option<NamedPipeServer> {
            self.guard.0.as_mut().unwrap().pipes.stdout.take()
        }
        pub(crate) fn take_stderr(&mut self) -> Option<NamedPipeServer> {
            self.guard.0.as_mut().unwrap().pipes.stderr.take()
        }
        pub(crate) fn try_wait(&mut self) -> Result<Option<u32>, String> {
            let f = &mut self.guard.0.as_mut().unwrap().frame;
            match unsafe { WaitForSingleObject(f.process.hProcess, 0) } {
                WAIT_TIMEOUT => Ok(None),
                WAIT_OBJECT_0 => {
                    if unsafe { GetExitCodeProcess(f.process.hProcess, &mut f.exit_code) } == 0 {
                        Err(error("inspection exit"))
                    } else {
                        Ok(Some(f.exit_code))
                    }
                }
                _ => Err(error("inspection root wait")),
            }
        }
        // Sole transport calls this only after all original IO worker joins.
        pub(crate) fn retire(mut self) -> Result<(), String> {
            let owner = self.guard.0.take().unwrap();
            finish_owner(owner, &UNKNOWN, |o| {
                let f = &mut o.frame;
                if unsafe { WaitForSingleObject(f.process.hProcess, 0) } != WAIT_OBJECT_0 {
                    return Err("Inspection root is not terminal.".into());
                }
                if unsafe {
                    QueryInformationJobObject(
                        f.job,
                        JobObjectBasicAccountingInformation,
                        (&mut f.accounting as *mut JOBOBJECT_BASIC_ACCOUNTING_INFORMATION).cast(),
                        size_of::<JOBOBJECT_BASIC_ACCOUNTING_INFORMATION>() as u32,
                        null_mut(),
                    )
                } == 0
                    || f.accounting.ActiveProcesses != 0
                {
                    return Err("Inspection job retirement unknown.".into());
                }
                close_known(o)
            })
        }
    }
}
pub(crate) use inspection_owner::InspectionClient;
