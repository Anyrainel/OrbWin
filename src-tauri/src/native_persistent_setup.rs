// Included in native_client::native. Fixed persistent role; NEVER a transient job.
mod persistent_setup {
    use super::*;
    use std::fs::File;
    include!("native_persistent_security.rs");
    struct PipeFrame {
        nonce: [u8; 32],
        name: Vec<u16>,
        attrs: SECURITY_ATTRIBUTES,
        server: HANDLE,
        server_reply: HANDLE,
        server_error: u32,
        server_entered: bool,
        server_returned: bool,
        child: HANDLE,
        child_reply: HANDLE,
        child_error: u32,
        child_entered: bool,
        child_returned: bool,
        client_pid: u32,
        server_pid: u32,
        managed: bool,
        close_entered: bool,
    }
    // Sole boxed frame moves with its creation continuation; no aliasing/native calls in parallel.
    unsafe impl Send for PipeFrame {}
    impl PipeFrame {
        fn new() -> Self {
            Self {
                nonce: [0; 32],
                name: Vec::new(),
                attrs: unsafe { zeroed() },
                server: null_mut(),
                server_reply: null_mut(),
                server_error: 0,
                server_entered: false,
                server_returned: false,
                child: null_mut(),
                child_reply: null_mut(),
                child_error: 0,
                child_entered: false,
                child_returned: false,
                client_pid: 0,
                server_pid: 0,
                managed: false,
                close_entered: false,
            }
        }
    }
    struct Frame {
        pi: PROCESS_INFORMATION,
        parent_member: i32,
        child_member: i32,
        windows: Vec<u16>,
        application: Vec<u16>,
        command: Vec<u16>,
        environment: Vec<u16>,
        cwd: Vec<u16>,
        handles: [HANDLE; 3],
        startup: STARTUPINFOEXW,
        attributes: Vec<usize>,
        attribute_ptr: LPPROC_THREAD_ATTRIBUTE_LIST,
        attribute_bytes: usize,
        attribute_initialized: bool,
        attribute_init_entered: bool,
        attribute_probe: i32,
        attribute_probe_error: u32,
        delete_entered: bool,
        pipes: [PipeFrame; 2],
        log_child: HANDLE,
        log_reply: HANDLE,
        log_result: i32,
        log_error: u32,
        log_entered: bool,
        log_returned: bool,
        acquisition: crate::persistent_setup_model::AcquisitionState,
        log_close: bool,
        thread_close: bool,
        process_close: bool,
        launch: crate::persistent_setup_model::LaunchState,
        exit_code: u32,
    }
    unsafe impl Send for Frame {}
    struct Owner {
        frame: Box<Frame>,
        pipes: [Option<NamedPipeServer>; 2],
        log: File,
        security: Box<Security>,
        executable: CachedExecutable,
        _permit: Permit,
    }
    // Sole creation continuation, then sole observation worker; no public raw access.
    unsafe impl Send for Owner {}
    struct Guard(Option<Owner>);
    fn park(owner: Owner) -> ! {
        let owner = std::mem::ManuallyDrop::new(owner);
        loop {
            std::hint::black_box(&owner);
            std::thread::park_timeout(Duration::from_secs(3600));
        }
    }
    impl Drop for Guard {
        fn drop(&mut self) {
            if let Some(mut o) = self.0.take() {
                o.frame.acquisition.fence(&UNKNOWN);
                park(o)
            }
        }
    }
    fn raw_close(h: &mut HANDLE, entered: &mut bool) -> Result<(), String> {
        if h.is_null() {
            return Ok(());
        }
        if *h == INVALID_HANDLE_VALUE || *entered {
            return Err("Persistent raw close unknown.".into());
        }
        *entered = true;
        if unsafe { CloseHandle(*h) } == 0 {
            return Err(error("persistent checked close"));
        }
        *h = null_mut();
        Ok(())
    }
    fn close_known(o: &mut Owner) -> Result<(), String> {
        let f = &mut o.frame;
        if o.security.unknown || f.acquisition.unknown {
            return Err("Persistent acquisition unknown.".into());
        }
        if f.attribute_init_entered && !f.attribute_initialized && !f.delete_entered {
            return Err("Persistent attribute initialization unknown.".into());
        }
        for (i, p) in f.pipes.iter().enumerate() {
            if p.managed && o.pipes[i].is_none() && !f.launch.entered {
                return Err("Managed registration uncertainty retained.".into());
            }
        }
        for p in &mut f.pipes {
            raw_close(&mut p.child, &mut p.close_entered)?;
            if !p.managed && !p.server.is_null() {
                return Err("Persistent raw pipe ownership unresolved.".into());
            }
        }
        raw_close(&mut f.log_child, &mut f.log_close)?;
        raw_close(&mut f.pi.hThread, &mut f.thread_close)?;
        if f.attribute_initialized {
            if f.delete_entered {
                return Err("Persistent attribute delete unknown.".into());
            }
            f.delete_entered = true;
            unsafe { DeleteProcThreadAttributeList(f.attribute_ptr) };
            f.attribute_initialized = false;
        }
        o.executable.check_held_origin()?;
        o.security.close()?;
        raw_close(&mut f.pi.hProcess, &mut f.process_close)?;
        Ok(())
    }
    impl Guard {
        // SAME original Guard owns partial native graph before spawn. No suffix,
        // query, retry or wait for keeper after a failed close.
        fn retain_and_report(mut self, reason: String) -> String {
            self.0.as_mut().unwrap().frame.acquisition.fence(&UNKNOWN);
            let retained = self;
            tokio::spawn(async move {
                loop {
                    std::hint::black_box(&retained);
                    tokio::time::sleep(Duration::from_secs(3600)).await;
                }
            });
            reason
        }
        fn fail(mut self, e: String) -> String {
            let owner = self.0.as_mut().unwrap();
            if crate::persistent_setup_model::failure_route(
                owner.frame.launch.entered,
                owner.frame.acquisition.unknown,
                owner.security.unknown,
            ) == crate::persistent_setup_model::FailureRoute::RetainAndReport
            {
                owner.frame.launch.fail();
                return self.retain_and_report(e);
            }
            if let Err(close_error) = close_known(owner) {
                // Fence BEFORE even allocating bounded diagnostic strings.
                owner.frame.acquisition.fence(&UNKNOWN);
                let reason = crate::persistent_setup_model::close_failure_error(&e, &close_error);
                return self.retain_and_report(reason);
            }
            self.0.take();
            e
        }
    }
    pub(crate) struct PersistentSetupClient {
        guard: Guard,
    }
    impl PersistentSetupClient {
        pub(crate) async fn spawn(
            package: crate::private_setup_client::SetupPackage,
            log: File,
        ) -> Result<Self, String> {
            let executable = package.into_cache();
            executable.check_held_origin()?;
            crate::runtime_cache::check_private_setup_log(&log)?;
            let permit = Permit::acquire()?;
            let mut guard = Guard(Some(Owner {
                frame: Box::new(Frame {
                    pi: unsafe { zeroed() },
                    parent_member: 2,
                    child_member: 2,
                    windows: vec![0; 32768],
                    application: Vec::new(),
                    command: Vec::new(),
                    environment: Vec::new(),
                    cwd: Vec::new(),
                    handles: [null_mut(); 3],
                    startup: unsafe { zeroed() },
                    attributes: Vec::new(),
                    attribute_ptr: null_mut(),
                    attribute_bytes: 0,
                    attribute_initialized: false,
                    attribute_init_entered: false,
                    attribute_probe: 0,
                    attribute_probe_error: 0,
                    delete_entered: false,
                    pipes: [PipeFrame::new(), PipeFrame::new()],
                    log_child: null_mut(),
                    log_reply: null_mut(),
                    log_result: 0,
                    log_error: 0,
                    log_entered: false,
                    log_returned: false,
                    acquisition: crate::persistent_setup_model::AcquisitionState::default(),
                    log_close: false,
                    thread_close: false,
                    process_close: false,
                    launch: crate::persistent_setup_model::LaunchState::default(),
                    exit_code: 259,
                }),
                pipes: [None, None],
                log,
                security: Box::new(Security::new()),
                executable,
                _permit: permit,
            }));
            let result = async {
                let o = guard.0.as_mut().unwrap();
                o.security.capture()?;
                let f = &mut o.frame;
                if unsafe { IsProcessInJob(GetCurrentProcess(), null_mut(), &mut f.parent_member) }
                    == 0
                    || f.parent_member != 0
                {
                    return Err("Persistent setup requires independent parent job context.".into());
                }
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
                    return Err("Fixed cached setup image required.".into());
                }
                f.command = command_line(&[
                    o.executable.path().as_os_str().encode_wide().collect(),
                    "private-runtime-operation".encode_utf16().collect(),
                    "setup".encode_utf16().collect(),
                ])?;
                let n = unsafe {
                    windows_sys::Win32::System::SystemInformation::GetWindowsDirectoryW(
                        f.windows.as_mut_ptr(),
                        f.windows.len() as u32,
                    )
                } as usize;
                if n == 0 || n >= f.windows.len() || f.windows[..n].contains(&0) {
                    return Err("Native Windows environment unavailable.".into());
                }
                let windows = OsString::from(
                    String::from_utf16(&f.windows[..n])
                        .map_err(|_| "Native Windows directory invalid.")?,
                );
                let system = PathBuf::from(&windows).join("System32");
                f.environment = environment_block(
                    crate::private_setup_client::environment(vec![
                        ("SystemRoot".into(), windows.clone()),
                        ("WINDIR".into(), windows),
                        ("PATH".into(), system.into_os_string()),
                        ("GOTRACEBACK".into(), "none".into()),
                    ])?
                    .into_iter()
                    .map(|(k, v)| (k.encode_wide().collect(), v.encode_wide().collect()))
                    .collect(),
                )?;
                f.cwd = o
                    .executable
                    .path()
                    .parent()
                    .ok_or("Setup cache parent absent.")?
                    .as_os_str()
                    .encode_wide()
                    .chain(Some(0))
                    .collect();
                for i in 0..2 {
                    let p = &mut f.pipes[i];
                    if unsafe { BCryptGenRandom(null_mut(), p.nonce.as_mut_ptr(), 32, 2) } < 0 {
                        return Err("Persistent pipe nonce unavailable.".into());
                    }
                    p.name = format!(
                        r"\\.\pipe\orbwin-setup-{}",
                        p.nonce
                            .iter()
                            .map(|b| format!("{b:02x}"))
                            .collect::<String>()
                    )
                    .encode_utf16()
                    .chain(Some(0))
                    .collect();
                    p.attrs = SECURITY_ATTRIBUTES {
                        nLength: size_of::<SECURITY_ATTRIBUTES>() as u32,
                        lpSecurityDescriptor: o.security.descriptor_owned,
                        bInheritHandle: 0,
                    };
                    p.server_entered = true;
                    p.server_reply = unsafe {
                        CreateNamedPipeW(
                            p.name.as_ptr(),
                            (if i == 0 {
                                PIPE_ACCESS_OUTBOUND
                            } else {
                                PIPE_ACCESS_INBOUND
                            }) | FILE_FLAG_FIRST_PIPE_INSTANCE
                                | FILE_FLAG_OVERLAPPED,
                            PIPE_TYPE_BYTE
                                | PIPE_READMODE_BYTE
                                | PIPE_WAIT
                                | PIPE_REJECT_REMOTE_CLIENTS,
                            1,
                            65536,
                            65536,
                            0,
                            &p.attrs,
                        )
                    };
                    p.server_error = unsafe { GetLastError() };
                    p.server_returned = true;
                    match crate::persistent_setup_model::file_reply(
                        p.server_reply as usize,
                        p.server_error,
                    ) {
                        crate::persistent_setup_model::HandleReply::Owned => {
                            p.server = p.server_reply
                        }
                        crate::persistent_setup_model::HandleReply::NoOutput => {
                            return Err("Persistent pipe server creation refused.".into())
                        }
                        crate::persistent_setup_model::HandleReply::Unknown => {
                            f.acquisition.fail();
                            return Err("Persistent pipe server reply unknown.".into());
                        }
                    }
                    o.security.verify_pipe(p.server)?;
                    // Transfer into pinned Tokio/mio managed IO. On registration error do not
                    // retry raw close: API may already have consumed its handle.
                    p.managed = true;
                    o.pipes[i] = Some(
                        unsafe { NamedPipeServer::from_raw_handle(p.server) }
                            .map_err(|_| "Persistent pipe registration unknown.")?,
                    );
                    p.attrs.bInheritHandle = 1;
                    p.child_entered = true;
                    p.child_reply = unsafe {
                        CreateFileW(
                            p.name.as_ptr(),
                            if i == 0 { GENERIC_READ } else { GENERIC_WRITE },
                            0,
                            &p.attrs,
                            OPEN_EXISTING,
                            FILE_ATTRIBUTE_NORMAL | SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION,
                            null_mut(),
                        )
                    };
                    p.child_error = unsafe { GetLastError() };
                    p.child_returned = true;
                    match crate::persistent_setup_model::file_reply(
                        p.child_reply as usize,
                        p.child_error,
                    ) {
                        crate::persistent_setup_model::HandleReply::Owned => {
                            p.child = p.child_reply
                        }
                        crate::persistent_setup_model::HandleReply::NoOutput => {
                            return Err("Persistent child pipe creation refused.".into())
                        }
                        crate::persistent_setup_model::HandleReply::Unknown => {
                            f.acquisition.fail();
                            return Err("Persistent child pipe reply unknown.".into());
                        }
                    }
                    o.pipes[i]
                        .as_ref()
                        .unwrap()
                        .connect()
                        .await
                        .map_err(|_| "Persistent self-pipe connect unknown.")?;
                    if unsafe { GetNamedPipeClientProcessId(p.server, &mut p.client_pid) } == 0
                        || unsafe { GetNamedPipeServerProcessId(p.child, &mut p.server_pid) } == 0
                        || p.client_pid != unsafe { GetCurrentProcessId() }
                        || p.server_pid != p.client_pid
                    {
                        return Err("Persistent pipe peer unknown.".into());
                    }
                    f.handles[i] = p.child;
                }
                f.log_entered = true;
                f.log_result = unsafe {
                    DuplicateHandle(
                        GetCurrentProcess(),
                        o.log.as_raw_handle(),
                        GetCurrentProcess(),
                        &mut f.log_reply,
                        0,
                        1,
                        DUPLICATE_SAME_ACCESS,
                    )
                };
                f.log_error = unsafe { GetLastError() };
                f.log_returned = true;
                match crate::persistent_setup_model::duplicate_reply(
                    f.log_result,
                    f.log_reply as usize,
                    f.log_error,
                ) {
                    crate::persistent_setup_model::HandleReply::Owned => f.log_child = f.log_reply,
                    crate::persistent_setup_model::HandleReply::NoOutput => {
                        return Err("Persistent log duplicate refused.".into())
                    }
                    crate::persistent_setup_model::HandleReply::Unknown => {
                        f.acquisition.fail();
                        return Err("Persistent log duplicate reply unknown.".into());
                    }
                }
                f.handles[2] = f.log_child;
                f.attribute_probe = unsafe {
                    InitializeProcThreadAttributeList(null_mut(), 1, 0, &mut f.attribute_bytes)
                };
                f.attribute_probe_error = unsafe { GetLastError() };
                if f.attribute_probe != 0 || f.attribute_probe_error != ERROR_INSUFFICIENT_BUFFER {
                    return Err("Persistent attribute size probe refused.".into());
                }
                if f.attribute_bytes == 0 || f.attribute_bytes > 65536 {
                    return Err("Persistent attribute bounds.".into());
                }
                f.attributes = vec![0; f.attribute_bytes.div_ceil(size_of::<usize>())];
                f.attribute_ptr = f.attributes.as_mut_ptr().cast();
                f.attribute_init_entered = true;
                if unsafe {
                    InitializeProcThreadAttributeList(f.attribute_ptr, 1, 0, &mut f.attribute_bytes)
                } == 0
                {
                    return Err(error("persistent attributes"));
                }
                f.attribute_initialized = true;
                if unsafe {
                    UpdateProcThreadAttribute(
                        f.attribute_ptr,
                        0,
                        PROC_THREAD_ATTRIBUTE_HANDLE_LIST as usize,
                        f.handles.as_ptr().cast(),
                        size_of_val(&f.handles),
                        null_mut(),
                        null(),
                    )
                } == 0
                {
                    return Err(error("persistent handle whitelist"));
                }
                f.startup.StartupInfo.cb = size_of::<STARTUPINFOEXW>() as u32;
                f.startup.StartupInfo.dwFlags = STARTF_USESTDHANDLES;
                f.startup.StartupInfo.hStdInput = f.handles[0];
                f.startup.StartupInfo.hStdOutput = f.handles[1];
                f.startup.StartupInfo.hStdError = f.handles[2];
                f.startup.lpAttributeList = f.attribute_ptr;
                o.executable.check_held_origin()?;
                o.security.recheck_dispatch_thread()?;
                if !f.launch.enter() {
                    return Err("Persistent launch state refused.".into());
                }
                if unsafe {
                    CreateProcessW(
                        f.application.as_ptr(),
                        f.command.as_mut_ptr(),
                        null(),
                        null(),
                        1,
                        CREATE_NO_WINDOW
                            | CREATE_NEW_PROCESS_GROUP
                            | CREATE_UNICODE_ENVIRONMENT
                            | EXTENDED_STARTUPINFO_PRESENT,
                        f.environment.as_mut_ptr().cast(),
                        f.cwd.as_ptr(),
                        &f.startup.StartupInfo,
                        &mut f.pi,
                    )
                } == 0
                {
                    return Err(error("persistent original creation"));
                }
                if f.pi.hProcess.is_null()
                    || f.pi.hProcess == INVALID_HANDLE_VALUE
                    || f.pi.hThread.is_null()
                    || f.pi.hThread == INVALID_HANDLE_VALUE
                {
                    return Err("Persistent partial birth retained.".into());
                }
                o.executable.verify_original_process(unsafe {
                    BorrowedHandle::borrow_raw(f.pi.hProcess)
                })?;
                if unsafe { IsProcessInJob(f.pi.hProcess, null_mut(), &mut f.child_member) } == 0
                    || f.child_member != 0
                {
                    return Err("Persistent child job context unknown.".into());
                }
                for p in &mut f.pipes {
                    raw_close(&mut p.child, &mut p.close_entered)?
                }
                raw_close(&mut f.log_child, &mut f.log_close)?;
                raw_close(&mut f.pi.hThread, &mut f.thread_close)?;
                if !f.launch.admit() {
                    return Err("Persistent initial frame not admitted.".into());
                }
                Ok(())
            }
            .await;
            match result {
                Ok(()) => Ok(Self { guard }),
                Err(e) => Err(guard.fail(e)),
            }
        }
        pub(crate) fn take_input(&mut self) -> Option<NamedPipeServer> {
            if !self
                .guard
                .0
                .as_ref()
                .unwrap()
                .frame
                .launch
                .may_send_initial()
            {
                return None;
            }
            self.guard.0.as_mut().unwrap().pipes[0].take()
        }
        pub(crate) fn take_output(&mut self) -> Option<NamedPipeServer> {
            self.guard.0.as_mut().unwrap().pipes[1].take()
        }
        pub(crate) fn try_wait(&mut self) -> Result<Option<u32>, String> {
            let f = &mut self.guard.0.as_mut().unwrap().frame;
            match unsafe { WaitForSingleObject(f.pi.hProcess, 0) } {
                WAIT_TIMEOUT => Ok(None),
                WAIT_OBJECT_0 => {
                    if unsafe { GetExitCodeProcess(f.pi.hProcess, &mut f.exit_code) } == 0 {
                        Err(error("persistent original exit"))
                    } else {
                        Ok(Some(f.exit_code))
                    }
                }
                _ => Err(error("persistent original wait")),
            }
        }
        pub(crate) fn retire(mut self) -> Result<(), String> {
            let o = self.guard.0.as_mut().unwrap();
            if unsafe { WaitForSingleObject(o.frame.pi.hProcess, 0) } != WAIT_OBJECT_0 {
                return Err("Persistent process not terminal.".into());
            }
            close_known(o)?;
            self.guard.0.take();
            Ok(())
        }
    }
}
pub(crate) use persistent_setup::PersistentSetupClient;
