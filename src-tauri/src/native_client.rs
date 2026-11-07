//! Atomic transient client creation. Never launches rdd services or native apps.
//! Parent IO uses Tokio's ordinary close contract; its Drop syscall result is
//! unobservable and is not configuration-writer authority. Root/job observation,
//! caller IO-task join and checked explicit child/process handles precede
//! ExecutionQuiesced. That capability proves owned execution stopped, not every
//! descendant process-object signal or pending kernel IO/durable file commit.

fn abandon_owner<T>(owner: T, latch: &std::sync::atomic::AtomicBool, kill: impl FnOnce(&T)) {
    let owner = std::mem::ManuallyDrop::new(owner);
    latch.store(true, std::sync::atomic::Ordering::Release);
    kill(&owner); // ManuallyDrop preserves ownership even if kill panics.
}
fn finish_owner<T>(
    owner: T,
    latch: &std::sync::atomic::AtomicBool,
    finish: impl FnOnce(&mut T) -> Result<(), String>,
) -> Result<(), String> {
    struct FailureLatch<'a>(&'a std::sync::atomic::AtomicBool, bool);
    impl Drop for FailureLatch<'_> {
        fn drop(&mut self) {
            if !self.1 {
                self.0.store(true, std::sync::atomic::Ordering::Release)
            }
        }
    }
    let mut pending = FailureLatch(latch, false);
    let mut owner = std::mem::ManuallyDrop::new(owner);
    let result = finish(&mut owner);
    if result.is_ok() {
        pending.1 = true;
        unsafe { std::mem::ManuallyDrop::drop(&mut owner) }
    }
    result
}
fn resume_after_journal(
    publish: impl FnOnce() -> Result<(), String>,
    cancelled: impl FnOnce() -> bool,
    resume: impl FnOnce() -> Result<(), String>,
) -> Result<(), String> {
    publish()?;
    if cancelled() {
        return Err("Native creation cancelled before first resume.".into());
    }
    resume()
}
// Closed, debug-only native producer qualification. No environment/IPC input,
// caller PID/path, proof factory or reset. All selected roots already belong to
// the actual creation-time job and remain suspended before this gate runs.
#[cfg(debug_assertions)]
pub(crate) mod qualification_failures {
    use std::sync::Mutex;
    #[derive(Clone, Copy)]
    pub(crate) enum Policy {
        KnownOnce,
        KnownTwice,
        RetainOnce,
    }
    #[derive(Clone, Copy, PartialEq, Eq)]
    pub(super) enum Action {
        CheckedRetirement,
        RetainOwner,
    }
    impl Policy {
        fn plan(self) -> (Action, u8) {
            match self {
                Self::KnownOnce => (Action::CheckedRetirement, 1),
                Self::KnownTwice => (Action::CheckedRetirement, 2),
                Self::RetainOnce => (Action::RetainOwner, 1),
            }
        }
    }
    static PLAN: Mutex<Option<(Action, u8)>> = Mutex::new(None);
    pub(crate) fn arm(policy: Policy) -> Result<(), &'static str> {
        let mut plan = PLAN.lock().map_err(|_| "Native fixture gate unavailable")?;
        if plan.is_some() {
            return Err("Native fixture already armed");
        }
        *plan = Some(policy.plan());
        Ok(())
    }
    pub(super) fn take() -> Result<Option<Action>, String> {
        let mut plan = PLAN.lock().map_err(|_| "Native fixture gate unavailable")?;
        if let Some((action, remaining)) = plan.as_mut() {
            if *remaining > 0 {
                *remaining -= 1;
                return Ok(Some(*action));
            }
        }
        Ok(None)
    }
    #[cfg(test)]
    mod tests {
        use super::*;
        #[test]
        fn fixture_failure_plans_are_closed_and_do_not_arm_native_dispatch() {
            assert!(matches!(
                Policy::KnownOnce.plan(),
                (Action::CheckedRetirement, 1)
            ));
            assert!(matches!(
                Policy::KnownTwice.plan(),
                (Action::CheckedRetirement, 2)
            ));
            assert!(matches!(
                Policy::RetainOnce.plan(),
                (Action::RetainOwner, 1)
            ));
            assert!(PLAN.lock().unwrap().is_none());
        }
    }
}

fn quote_arg(value: &[u16]) -> Result<Vec<u16>, String> {
    if value.len() > 32760 || value.contains(&0) {
        return Err("Client argument contains NUL.".into());
    }
    let mut out = vec![b'"' as u16];
    let mut slashes = 0;
    for &unit in value {
        if unit == b'\\' as u16 {
            slashes += 1;
            continue;
        }
        if unit == b'"' as u16 {
            out.extend(std::iter::repeat_n(b'\\' as u16, slashes * 2 + 1));
        } else {
            out.extend(std::iter::repeat_n(b'\\' as u16, slashes));
        }
        slashes = 0;
        out.push(unit);
    }
    out.extend(std::iter::repeat_n(b'\\' as u16, slashes * 2));
    out.push(b'"' as u16);
    Ok(out)
}
fn command_line(argv: &[Vec<u16>]) -> Result<Vec<u16>, String> {
    if argv.is_empty() {
        return Err("Explicit client argv is required.".into());
    }
    let mut out = Vec::new();
    for arg in argv {
        if !out.is_empty() {
            out.push(32)
        }
        out.extend(quote_arg(arg)?);
        if out.len() >= 32767 {
            return Err("Client command line exceeds Windows bounds.".into());
        }
    }
    out.push(0);
    Ok(out)
}
fn environment_block(mut entries: Vec<(Vec<u16>, Vec<u16>)>) -> Result<Vec<u16>, String> {
    if entries.len() > 512 {
        return Err("Client environment entry bound exceeded.".into());
    }
    let mut units = 2usize;
    for (key, value) in &entries {
        units = units
            .checked_add(key.len())
            .and_then(|n| n.checked_add(value.len()))
            .and_then(|n| n.checked_add(2))
            .ok_or("Client environment length overflow.")?;
        if units > 1 << 19 {
            return Err("Client environment exceeds 1MiB.".into());
        }
    }
    let mut seen = std::collections::BTreeSet::new();
    for (key, value) in &entries {
        // Windows preserves hidden per-drive working directories as =C: keys.
        // This is the only permitted leading/embedded equals key form.
        let drive_directory = key.len() == 3
            && key[0] == b'=' as u16
            && ((b'A' as u16..=b'Z' as u16).contains(&key[1])
                || (b'a' as u16..=b'z' as u16).contains(&key[1]))
            && key[2] == b':' as u16;
        if key.is_empty()
            || key.contains(&0)
            || (key.contains(&(b'=' as u16)) && !drive_directory)
            || value.contains(&0)
        {
            return Err("Client environment has malformed entry.".into());
        }
        let text =
            String::from_utf16(key).map_err(|_| "Client environment key is not valid Unicode.")?;
        if !seen.insert(text.to_uppercase()) {
            return Err("Duplicate client environment key.".into());
        }
    }
    entries.sort_by_cached_key(|(key, _)| String::from_utf16_lossy(key).to_uppercase());
    let mut out = Vec::new();
    for (key, value) in entries {
        out.extend(key);
        out.push(b'=' as u16);
        out.extend(value);
        out.push(0);
        if out.len() > 1 << 19 {
            return Err("Client environment exceeds 1MiB.".into());
        }
    }
    out.push(0);
    if out.len() == 1 {
        out.push(0)
    }
    Ok(out)
}

#[cfg(windows)]
mod native {
    use super::{
        abandon_owner, command_line, environment_block, finish_owner, resume_after_journal,
    };
    use crate::{
        client_config::WriterAdmission,
        client_job::{ClientJob, ExecutionQuiesced},
        runtime_cache::CachedExecutable,
    };
    use std::{
        ffi::{c_void, OsString},
        mem::{size_of, size_of_val, zeroed},
        os::windows::{
            ffi::OsStrExt,
            io::{AsRawHandle, BorrowedHandle, FromRawHandle, IntoRawHandle, OwnedHandle},
        },
        path::PathBuf,
        ptr::{null, null_mut},
        sync::atomic::{AtomicBool, AtomicUsize, Ordering},
        time::{Duration, Instant},
    };
    use tokio::net::windows::named_pipe::NamedPipeServer;
    use windows_sys::Win32::{
        Foundation::*,
        Security::{Authorization::*, *},
        Storage::FileSystem::*,
        System::{JobObjects::*, Pipes::*, Threading::*},
    };

    static UNKNOWN: AtomicBool = AtomicBool::new(false);
    static ACTIVE: AtomicUsize = AtomicUsize::new(0);
    const MAX_CLIENTS: usize = 32;
    // Constructed only beside the actual suspended root HANDLE observations.
    // No caller-supplied PID, path or JSON can construct this capability.
    pub(crate) struct NativeBirth {
        pid: u32,
        created: u64,
        session: u32,
    }
    impl NativeBirth {
        pub(crate) fn generation(self) -> (u32, u64, u32) {
            (self.pid, self.created, self.session)
        }
    }
    struct Permit;
    impl Permit {
        fn acquire() -> Result<Self, String> {
            if UNKNOWN.load(Ordering::Acquire) {
                return Err("Prior native client retirement is unknown.".into());
            }
            ACTIVE
                .fetch_update(Ordering::AcqRel, Ordering::Acquire, |n| {
                    (n < MAX_CLIENTS).then_some(n + 1)
                })
                .map_err(|_| "Native client admission budget exhausted.")?;
            if UNKNOWN.load(Ordering::Acquire) {
                ACTIVE.fetch_sub(1, Ordering::AcqRel);
                return Err("Native client admission became unknown.".into());
            }
            Ok(Self)
        }
    }
    impl Drop for Permit {
        fn drop(&mut self) {
            ACTIVE.fetch_sub(1, Ordering::AcqRel);
        }
    }
    fn error(stage: &str) -> String {
        format!(
            "Native client {stage} failed: {}",
            std::io::Error::last_os_error()
        )
    }
    fn own(raw: HANDLE) -> Result<OwnedHandle, String> {
        if raw.is_null() || raw == INVALID_HANDLE_VALUE {
            Err(error("handle admission"))
        } else {
            Ok(unsafe { OwnedHandle::from_raw_handle(raw) })
        }
    }
    fn close_checked(
        slot: &mut Option<OwnedHandle>,
        unknown: &mut Vec<usize>,
    ) -> Result<(), String> {
        if let Some(handle) = slot.take() {
            let raw = handle.into_raw_handle();
            if unsafe { CloseHandle(raw) } == 0 {
                unknown.push(raw as usize);
                UNKNOWN.store(true, Ordering::Release);
                return Err(error("checked handle close"));
            }
        }
        Ok(())
    }
    // Temporary pre-admission handles also use checked Close. A failed close
    // leaves a raw unknown handle un-retried until process exit.
    struct LocalHandle(Option<OwnedHandle>);
    impl LocalHandle {
        fn raw(&self) -> HANDLE {
            self.0.as_ref().unwrap().as_raw_handle()
        }
        fn into_raw(mut self) -> HANDLE {
            self.0.take().unwrap().into_raw_handle()
        }
        fn into_owned(mut self) -> OwnedHandle {
            self.0.take().unwrap()
        }
    }
    impl Drop for LocalHandle {
        fn drop(&mut self) {
            let mut unknown = Vec::new();
            let _ = close_checked(&mut self.0, &mut unknown);
        }
    }
    #[link(name = "bcrypt")]
    unsafe extern "system" {
        fn BCryptGenRandom(algorithm: HANDLE, buffer: *mut u8, length: u32, flags: u32) -> i32;
    }

    pub(crate) struct NativeSpec {
        executable: CachedExecutable,
        args: Vec<OsString>,
        environment: Vec<(OsString, OsString)>,
        cwd: Option<PathBuf>,
        writer_binding: Option<[u8; 32]>,
        writer_admission: Option<WriterAdmission>,
        cancellation: std::sync::Arc<AtomicBool>,
        created: std::sync::Arc<AtomicBool>,
        retired_failure: std::sync::Arc<AtomicBool>,
    }
    impl NativeSpec {
        pub(crate) fn new(
            executable: CachedExecutable,
            args: Vec<OsString>,
            environment: Vec<(OsString, OsString)>,
            cwd: Option<PathBuf>,
            writer: Option<WriterAdmission>,
        ) -> Self {
            let writer_binding = writer.as_ref().map(|admission| admission.binding());
            Self {
                executable,
                args,
                environment,
                cwd,
                writer_binding,
                writer_admission: writer,
                cancellation: std::sync::Arc::new(AtomicBool::new(false)),
                created: std::sync::Arc::new(AtomicBool::new(false)),
                retired_failure: std::sync::Arc::new(AtomicBool::new(false)),
            }
        }
        pub(crate) fn cancel_handle(&self) -> NativeCancellation {
            NativeCancellation(self.cancellation.clone())
        }
    }
    #[derive(Clone)]
    pub(crate) struct NativeCancellation(std::sync::Arc<AtomicBool>);
    impl NativeCancellation {
        pub(crate) fn cancel(&self) {
            self.0.store(true, Ordering::Release);
        }
    }
    pub(crate) struct NeverStartedClient {
        binding: [u8; 32],
    }
    impl NeverStartedClient {
        pub(crate) fn consume_for_writer(self, writer: &WriterAdmission) -> Result<(), String> {
            if self.binding == writer.binding() && self.binding != [0; 32] {
                Ok(())
            } else {
                Err("Unstarted native client does not match this writer admission.".into())
            }
        }
    }
    pub(crate) struct NativeSpawnError {
        pub(crate) message: String,
        unstarted: Option<NeverStartedClient>,
        pub(crate) no_child_created: bool,
        stopped: Option<StoppedWriterClient>,
        admission_stopped: Option<NativeAdmissionStopped>,
    }
    // Admission only: never writer settlement or generic kernel-IO authority.
    // The per-creation producer is the sole constructor. Taking/consuming this
    // token proves its creation continuation ended without executable readers.
    pub(crate) struct NativeAdmissionStopped {
        _private: (),
    }
    impl NativeAdmissionStopped {
        pub(crate) fn consume(self) {}
    }
    pub(crate) struct StoppedWriterClient {
        binding: [u8; 32],
    }
    impl StoppedWriterClient {
        pub(crate) fn consume_for_writer(self, writer: &WriterAdmission) -> Result<(), String> {
            if self.binding != [0; 32] && self.binding == writer.binding() {
                Ok(())
            } else {
                Err("Retired native creation does not match writer".into())
            }
        }
    }
    impl NativeSpawnError {
        pub(crate) fn take_admission_stopped(&mut self) -> Option<NativeAdmissionStopped> {
            self.admission_stopped.take()
        }
        pub(crate) fn take_stopped(&mut self) -> Option<StoppedWriterClient> {
            self.stopped.take()
        }
        pub(crate) fn take_unstarted(&mut self) -> Option<NeverStartedClient> {
            self.unstarted.take()
        }
    }
    impl std::fmt::Debug for NativeSpawnError {
        fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
            f.debug_struct("NativeSpawnError")
                .field("message", &self.message)
                .field("no_child_created", &self.no_child_created)
                .finish()
        }
    }
    impl std::fmt::Display for NativeSpawnError {
        fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
            f.write_str(&self.message)
        }
    }
    impl std::error::Error for NativeSpawnError {}
    #[derive(Default)]
    struct Pipes {
        stdin: Option<NamedPipeServer>,
        stdout: Option<NamedPipeServer>,
        stderr: Option<NamedPipeServer>,
    }
    struct Bundle {
        created: bool,
        process: Option<OwnedHandle>,
        thread: Option<OwnedHandle>,
        original_job: Option<OwnedHandle>,
        job: Option<ClientJob>,
        children: [Option<OwnedHandle>; 3],
        pipes: Pipes,
        executable: CachedExecutable,
        _permit: Permit,
        unknown: Vec<usize>,
        pid: u32,
    }
    impl Bundle {
        fn job_raw(&self) -> HANDLE {
            self.original_job
                .as_ref()
                .map(|h| h.as_raw_handle())
                .unwrap_or(null_mut())
        }
        fn request_kill(&self) -> Result<(), String> {
            if self.job_raw().is_null() {
                return Ok(());
            }
            if unsafe { TerminateJobObject(self.job_raw(), 125) } == 0 {
                Err(error("tree termination"))
            } else {
                Ok(())
            }
        }
        fn signaled(&self) -> Result<bool, String> {
            let h = self
                .process
                .as_ref()
                .ok_or("Original native client handle unavailable.")?;
            match unsafe { WaitForSingleObject(h.as_raw_handle(), 0) } {
                WAIT_OBJECT_0 => Ok(true),
                WAIT_TIMEOUT => Ok(false),
                _ => Err(error("root observation")),
            }
        }
        fn empty(&self) -> Result<bool, String> {
            let mut info: JOBOBJECT_BASIC_ACCOUNTING_INFORMATION = unsafe { zeroed() };
            if unsafe {
                QueryInformationJobObject(
                    self.job_raw(),
                    JobObjectBasicAccountingInformation,
                    (&mut info as *mut JOBOBJECT_BASIC_ACCOUNTING_INFORMATION).cast(),
                    size_of::<JOBOBJECT_BASIC_ACCOUNTING_INFORMATION>() as u32,
                    null_mut(),
                )
            } == 0
            {
                return Err(error("job membership observation"));
            }
            Ok(info.ActiveProcesses == 0)
        }
    }
    // An unexpected Drop must keep native authority and executable leases alive.
    // No background cleanup thread, retry, or bare job-close can create proof.
    struct Guard {
        bundle: Option<Bundle>,
    }
    impl Drop for Guard {
        fn drop(&mut self) {
            if let Some(bundle) = self.bundle.take() {
                abandon_owner(bundle, &UNKNOWN, |b| {
                    let _ = b.request_kill();
                });
            }
        }
    }
    impl Guard {
        fn fail(mut self, reason: String, retired: &AtomicBool) -> String {
            let bundle = self.bundle.take().unwrap();
            if UNKNOWN.load(Ordering::Acquire) {
                abandon_owner(bundle, &UNKNOWN, |b| {
                    let _ = b.request_kill();
                });
                return reason;
            }
            match checked_failure(bundle) {
                Ok(()) => {
                    retired.store(true, Ordering::Release);
                    reason
                }
                Err(e) => format!("{reason}; {e}"),
            }
        }
    }
    fn checked_failure(bundle: Bundle) -> Result<(), String> {
        // Preserve the whole bundle on every early/uncertain path. ManuallyDrop
        // prevents implicit field cleanup from discarding authority on error.
        finish_owner(bundle, &UNKNOWN, |bundle| {
            if bundle.created && bundle.process.is_none() {
                return Err(
                    "Created native root handle is unavailable; ownership retained.".into(),
                );
            }
            if bundle.process.is_some() {
                let _ = bundle.request_kill();
                let deadline = Instant::now() + Duration::from_secs(2);
                loop {
                    if bundle.signaled()? && bundle.empty()? {
                        break;
                    }
                    if Instant::now() >= deadline {
                        return Err("Native admission retirement unknown.".into());
                    }
                    std::thread::sleep(Duration::from_millis(10));
                }
            }
            let b: &mut Bundle = bundle;
            for child in &mut b.children {
                close_checked(child, &mut b.unknown)?
            }
            close_checked(&mut b.thread, &mut b.unknown)?;
            close_checked(&mut b.process, &mut b.unknown)?;
            if let Some(job) = b.job.take() {
                job.into_execution_quiesced()?;
            }
            close_checked(&mut b.original_job, &mut b.unknown)?;
            b.executable.check_held_origin()?;
            Ok(())
        })
    }

    struct Descriptor(*mut c_void);
    // Immutable LocalAlloc descriptor storage moves with this sole owner; Win32
    // borrows it only during synchronous pipe creation. Drop requires &mut self.
    unsafe impl Send for Descriptor {}
    unsafe impl Sync for Descriptor {}
    impl Drop for Descriptor {
        fn drop(&mut self) {
            unsafe {
                LocalFree(self.0);
            }
        }
    }
    fn sid_text(sid: PSID) -> Result<String, String> {
        if unsafe { IsValidSid(sid) } == 0 {
            return Err("Invalid native client SID.".into());
        }
        let mut text = null_mut();
        if unsafe { ConvertSidToStringSidW(sid, &mut text) } == 0 {
            return Err(error("SID encoding"));
        }
        let mut n = 0;
        while n < 184 && unsafe { *text.add(n) } != 0 {
            n += 1
        }
        let result = if n < 184 {
            String::from_utf16(unsafe { std::slice::from_raw_parts(text, n) })
                .map_err(|_| "SID encoding refused.".to_string())
        } else {
            Err("SID text bound exceeded.".into())
        };
        unsafe {
            LocalFree(text.cast());
        }
        result
    }
    fn user_descriptor() -> Result<(Descriptor, String, u32), String> {
        let mut thread = null_mut();
        if unsafe { OpenThreadToken(GetCurrentThread(), TOKEN_QUERY, 1, &mut thread) } != 0 {
            let mut handle = Some(own(thread)?);
            let mut unknown = Vec::new();
            close_checked(&mut handle, &mut unknown)?;
            return Err("Impersonated native client caller refused.".into());
        }
        if unsafe { GetLastError() } != ERROR_NO_TOKEN {
            return Err(error("effective token"));
        }
        let mut token = null_mut();
        if unsafe { OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &mut token) } == 0 {
            return Err(error("process token"));
        }
        let mut token = Some(own(token)?);
        let h = token.as_ref().unwrap().as_raw_handle();
        let result = (|| {
            let mut elevated = 0u32;
            let mut needed = 0;
            let mut session = 0u32;
            if unsafe {
                GetTokenInformation(
                    h,
                    TokenElevation,
                    (&mut elevated as *mut u32).cast(),
                    4,
                    &mut needed,
                )
            } == 0
                || needed != 4
                || elevated != 0
            {
                return Err("Ordinary native client token required.".into());
            }
            if unsafe {
                GetTokenInformation(
                    h,
                    TokenSessionId,
                    (&mut session as *mut u32).cast(),
                    4,
                    &mut needed,
                )
            } == 0
                || needed != 4
                || session == 0
            {
                return Err("Interactive native client token required.".into());
            }
            let mut bytes = [0u64; 256];
            if unsafe {
                GetTokenInformation(
                    h,
                    TokenUser,
                    bytes.as_mut_ptr().cast(),
                    size_of::<[u64; 256]>() as u32,
                    &mut needed,
                )
            } == 0
                || needed as usize > size_of::<[u64; 256]>()
                || needed < size_of::<TOKEN_USER>() as u32
            {
                return Err(error("token user"));
            }
            let user = unsafe { &*bytes.as_ptr().cast::<TOKEN_USER>() };
            let sid = bounded_sid(bytes.as_ptr() as usize, needed as usize, user.User.Sid)?;
            if !crate::persistent_security_policy::ordinary_sid(&sid) {
                return Err("Ordinary native user SID required.".into());
            }
            let text: Vec<u16> = crate::persistent_security_policy::pipe_sddl(&sid)
                .encode_utf16()
                .chain(Some(0))
                .collect();
            let mut descriptor = null_mut();
            if unsafe {
                ConvertStringSecurityDescriptorToSecurityDescriptorW(
                    text.as_ptr(),
                    1,
                    &mut descriptor,
                    null_mut(),
                )
            } == 0
            {
                return Err(error("pipe descriptor"));
            }
            Ok((Descriptor(descriptor), sid, session))
        })();
        let mut unknown = Vec::new();
        let close = close_checked(&mut token, &mut unknown);
        if close.is_err() {
            UNKNOWN.store(true, Ordering::Release);
        }
        match (result, close) {
            (Ok(v), Ok(())) => Ok(v),
            (Err(e), _) => Err(e),
            (_, Err(e)) => Err(e),
        }
    }
    fn bounded_sid(base: usize, length: usize, sid: PSID) -> Result<String, String> {
        let address = sid as usize;
        let off = crate::persistent_security_policy::sid_offset(base, length, address)
            .ok_or("Pipe SID outside descriptor.")?;
        let count = unsafe { *sid.cast::<u8>().add(1) } as usize;
        crate::persistent_security_policy::sid_size(length - off, count as u8)
            .ok_or("Pipe SID length refused.")?;
        sid_text(sid)
    }
    fn verify_private_pipe(handle: HANDLE, user: &str) -> Result<(), String> {
        let mut needed = 0;
        unsafe {
            GetKernelObjectSecurity(
                handle,
                OWNER_SECURITY_INFORMATION | DACL_SECURITY_INFORMATION,
                null_mut(),
                0,
                &mut needed,
            )
        };
        if needed < 20 || needed > 65536 {
            return Err(error("pipe security size"));
        }
        let queried = needed as usize;
        let mut storage = vec![0u64; (needed as usize + 7) / 8];
        let capacity = storage.len() * 8;
        let mut returned = 0;
        if unsafe {
            GetKernelObjectSecurity(
                handle,
                OWNER_SECURITY_INFORMATION | DACL_SECURITY_INFORMATION,
                storage.as_mut_ptr().cast(),
                capacity as u32,
                &mut returned,
            )
        } == 0
            || returned as usize > capacity
        {
            return Err(error("pipe security"));
        }
        let sd = storage.as_mut_ptr().cast();
        // A real named-pipe query succeeded with ReturnLength=0. Preserve the
        // sizing-query bound and validate the copied self-relative descriptor;
        // zero never supplies an owner/ACL/SID bounds authority.
        if unsafe { IsValidSecurityDescriptor(sd) } == 0 {
            return Err("Pipe private descriptor is invalid.".into());
        }
        let exact = unsafe { GetSecurityDescriptorLength(sd) } as usize;
        if exact < 20 || exact > queried || exact > capacity {
            return Err("Pipe private descriptor length refused.".into());
        }
        let mut owner = null_mut();
        let mut defaulted = 0;
        let mut present = 0;
        let mut acl = null_mut();
        let mut control = 0u16;
        let mut revision = 0;
        if unsafe { GetSecurityDescriptorOwner(sd, &mut owner, &mut defaulted) } == 0
            || unsafe { GetSecurityDescriptorDacl(sd, &mut present, &mut acl, &mut defaulted) } == 0
            || present == 0
            || acl.is_null()
            || unsafe { GetSecurityDescriptorControl(sd, &mut control, &mut revision) } == 0
            || !crate::persistent_security_policy::descriptor_control(control, revision)
        {
            return Err("Pipe private descriptor shape refused.".into());
        }
        let base = storage.as_ptr() as usize;
        if bounded_sid(base, exact, owner).map_err(|e| format!("Pipe owner: {e}"))? != user {
            return Err("Pipe owner differs.".into());
        }
        let off = (acl as usize)
            .checked_sub(base)
            .ok_or("Pipe ACL bounds refused.")?;
        if off > exact || exact - off < size_of::<ACL>() {
            return Err("Pipe ACL bounds refused.".into());
        }
        let total = unsafe { (*acl).AclSize } as usize;
        if total < size_of::<ACL>()
            || total > exact - off
            || unsafe { (*acl).AceCount } as usize
                != crate::persistent_security_policy::PIPE_ACE_COUNT
        {
            return Err("Pipe ACL scope refused.".into());
        }
        let mut trustees = Vec::new();
        for i in 0..crate::persistent_security_policy::PIPE_ACE_COUNT as u32 {
            let mut p = null_mut();
            if unsafe { GetAce(acl, i, &mut p) } == 0 || p.is_null() {
                return Err(error("pipe ACE"));
            }
            let offset = (p as usize)
                .checked_sub(acl as usize)
                .ok_or("Pipe ACE bounds refused.")?;
            if offset > total || total - offset < 16 {
                return Err("Pipe ACE bounds refused.".into());
            }
            let ace = unsafe { &*p.cast::<ACCESS_ALLOWED_ACE>() };
            let size = ace.Header.AceSize as usize;
            if size < 16
                || size > total - offset
                || !crate::persistent_security_policy::allowed_ace(
                    ace.Header.AceType,
                    ace.Header.AceFlags,
                    ace.Mask,
                )
            {
                return Err("Pipe ACE differs from closed scope.".into());
            }
            trustees.push(
                bounded_sid(p as usize, size, unsafe { p.cast::<u8>().add(8) }.cast())
                    .map_err(|e| format!("Pipe trustee {i}: {e}"))?,
            )
        }
        if !crate::persistent_security_policy::trustees_match(user, &trustees) {
            return Err("Pipe trustees differ.".into());
        }
        Ok(())
    }
    fn prepare_pipe(
        descriptor: &Descriptor,
        user: &str,
        input: bool,
    ) -> Result<(NamedPipeServer, LocalHandle), String> {
        let mut nonce = [0u8; 32];
        if unsafe { BCryptGenRandom(null_mut(), nonce.as_mut_ptr(), 32, 2) } < 0 {
            return Err("Native pipe nonce unavailable.".into());
        }
        let text = format!(
            r"\\.\pipe\orbwin-native-{}",
            nonce.iter().map(|b| format!("{b:02x}")).collect::<String>()
        );
        let name: Vec<u16> = text.encode_utf16().chain(Some(0)).collect();
        let mut attrs = SECURITY_ATTRIBUTES {
            nLength: size_of::<SECURITY_ATTRIBUTES>() as u32,
            lpSecurityDescriptor: descriptor.0,
            bInheritHandle: 0,
        };
        let direction = if input {
            PIPE_ACCESS_OUTBOUND
        } else {
            PIPE_ACCESS_INBOUND
        };
        let raw = unsafe {
            CreateNamedPipeW(
                name.as_ptr(),
                // CreateNamedPipe grants FILE_GENERIC_READ/WRITE (including
                // READ_CONTROL) from the direction. READ_CONTROL is not an
                // additional dwOpenMode flag; passing it returns ERROR_INVALID_PARAMETER.
                direction | FILE_FLAG_FIRST_PIPE_INSTANCE | FILE_FLAG_OVERLAPPED,
                PIPE_TYPE_BYTE | PIPE_READMODE_BYTE | PIPE_WAIT | PIPE_REJECT_REMOTE_CLIENTS,
                1,
                65536,
                65536,
                0,
                &attrs,
            )
        };
        if raw.is_null() || raw == INVALID_HANDLE_VALUE {
            return Err(error("private pipe server creation"));
        }
        let server = LocalHandle(Some(own(raw)?));
        verify_private_pipe(server.raw(), user)?;
        let raw = server.into_raw();
        let server = match unsafe { NamedPipeServer::from_raw_handle(raw) } {
            Ok(p) => p,
            Err(e) => {
                UNKNOWN.store(true, Ordering::Release);
                return Err(format!("Tokio pipe registration unknown: {e}"));
            }
        };
        attrs.bInheritHandle = 1;
        let access = if input { GENERIC_READ } else { GENERIC_WRITE };
        let raw = unsafe {
            CreateFileW(
                name.as_ptr(),
                access,
                0,
                &attrs,
                OPEN_EXISTING,
                FILE_ATTRIBUTE_NORMAL | SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION,
                null_mut(),
            )
        };
        if raw.is_null() || raw == INVALID_HANDLE_VALUE {
            return Err(error("private pipe child endpoint creation"));
        }
        let child = LocalHandle(Some(own(raw)?));
        Ok((server, child))
    }
    async fn pipe_pair(
        descriptor: &Descriptor,
        user: &str,
        input: bool,
    ) -> Result<(NamedPipeServer, OwnedHandle), String> {
        let (server, child) = prepare_pipe(descriptor, user, input)?;
        tokio::time::timeout(Duration::from_secs(2), server.connect())
            .await
            .map_err(|_| "Native pipe self-connect timed out.")?
            .map_err(|e| e.to_string())?;
        let mut client_pid = 0;
        let mut server_pid = 0;
        if unsafe { GetNamedPipeClientProcessId(server.as_raw_handle(), &mut client_pid) } == 0
            || unsafe { GetNamedPipeServerProcessId(child.raw(), &mut server_pid) } == 0
            || client_pid != unsafe { GetCurrentProcessId() }
            || server_pid != client_pid
        {
            return Err("Native pipe endpoints do not belong to this process.".into());
        }
        verify_private_pipe(server.as_raw_handle(), user)?;
        Ok((server, child.into_owned()))
    }
    struct Attributes {
        storage: Vec<usize>,
        ptr: LPPROC_THREAD_ATTRIBUTE_LIST,
    }
    impl Drop for Attributes {
        fn drop(&mut self) {
            unsafe { DeleteProcThreadAttributeList(self.ptr) }
        }
    }
    impl Attributes {
        fn new() -> Result<Self, String> {
            let mut bytes = 0;
            unsafe { InitializeProcThreadAttributeList(null_mut(), 2, 0, &mut bytes) };
            if bytes == 0 || bytes > 65536 {
                return Err(error("attribute bounds"));
            }
            let mut storage = vec![0usize; (bytes + size_of::<usize>() - 1) / size_of::<usize>()];
            let ptr = storage.as_mut_ptr().cast();
            if unsafe { InitializeProcThreadAttributeList(ptr, 2, 0, &mut bytes) } == 0 {
                return Err(error("attribute initialization"));
            }
            Ok(Self { storage, ptr })
        }
    }
    include!("native_inspection.rs");
    include!("native_persistent_setup.rs");
    pub(crate) struct NativeClient {
        guard: Guard,
    }
    pub(crate) struct NativeRetirement {
        guard: Guard,
    }
    #[cfg(test)]
    fn spawn_future_is_send(spec: NativeSpec) {
        fn assert_send<T: Send>(_: T) {}
        assert_send(NativeClient::spawn(spec));
    }
    impl NativeClient {
        pub(crate) async fn spawn(spec: NativeSpec) -> Result<Self, NativeSpawnError> {
            let created = spec.created.clone();
            let binding = spec.writer_binding;
            let retired = spec.retired_failure.clone();
            Self::spawn_inner(spec).await.map_err(|message| {
                let no_child_created = !created.load(Ordering::Acquire);
                NativeSpawnError {
                    message,
                    no_child_created,
                    admission_stopped: if no_child_created || retired.load(Ordering::Acquire) {
                        Some(NativeAdmissionStopped { _private: () })
                    } else {
                        None
                    },
                    stopped: if !no_child_created && retired.load(Ordering::Acquire) {
                        binding.map(|binding| StoppedWriterClient { binding })
                    } else {
                        None
                    },
                    unstarted: if no_child_created && !UNKNOWN.load(Ordering::Acquire) {
                        binding.map(|binding| NeverStartedClient { binding })
                    } else {
                        None
                    },
                }
            })
        }
        async fn spawn_inner(mut spec: NativeSpec) -> Result<Self, String> {
            if spec.cancellation.load(Ordering::Acquire) {
                return Err("Native creation cancelled before admission.".into());
            }
            // Validate actual held Docker, fixed auth/config/target argv and
            // owned helper/plugin environment before admission or any birth.
            // Rejection can produce authentic writer-bound NeverStarted only
            // after this creation continuation has ended with known no-birth.
            if let Some(writer) = &spec.writer_admission {
                writer.validate_invocation(&spec.executable, &spec.args, &spec.environment)?;
            }
            let permit = Permit::acquire()?;
            spec.executable.check_held_origin()?;
            let mut argv = vec![spec.executable.path().as_os_str().encode_wide().collect()];
            argv.extend(spec.args.iter().map(|v| v.encode_wide().collect()));
            let mut command = command_line(&argv)?;
            let mut environment = environment_block(
                spec.environment
                    .iter()
                    .map(|(k, v)| (k.encode_wide().collect(), v.encode_wide().collect()))
                    .collect(),
            )?;
            let application: Vec<u16> = spec
                .executable
                .path()
                .as_os_str()
                .encode_wide()
                .chain(Some(0))
                .collect();
            if !spec.executable.path().is_absolute()
                || application.len() > 32761
                || application[..application.len() - 1].contains(&0)
            {
                return Err("Closed cached executable path required.".into());
            }
            let cwd: Option<Vec<u16>> = spec
                .cwd
                .as_ref()
                .map(|p| p.as_os_str().encode_wide().chain(Some(0)).collect());
            if let Some(v) = &cwd {
                if v.len() > 32761
                    || v[..v.len() - 1].contains(&0)
                    || !spec.cwd.as_ref().unwrap().is_absolute()
                {
                    return Err("Absolute bounded client working directory required.".into());
                }
            }
            let (descriptor, user, session) = user_descriptor()?;
            let job = LocalHandle(Some(own(unsafe { CreateJobObjectW(null(), null()) })?));
            let mut limits: JOBOBJECT_EXTENDED_LIMIT_INFORMATION = unsafe { zeroed() };
            limits.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;
            if unsafe {
                SetInformationJobObject(
                    job.raw(),
                    JobObjectExtendedLimitInformation,
                    (&limits as *const JOBOBJECT_EXTENDED_LIMIT_INFORMATION).cast(),
                    size_of::<JOBOBJECT_EXTENDED_LIMIT_INFORMATION>() as u32,
                )
            } == 0
            {
                return Err(error("job limits"));
            }
            let mut guard = Guard {
                bundle: Some(Bundle {
                    created: false,
                    process: None,
                    thread: None,
                    original_job: Some(job.into_owned()),
                    job: None,
                    children: [None, None, None],
                    pipes: Pipes::default(),
                    executable: spec.executable,
                    _permit: permit,
                    unknown: Vec::new(),
                    pid: 0,
                }),
            };
            for index in 0..3 {
                if spec.cancellation.load(Ordering::Acquire) {
                    return Err(guard.fail(
                        "Native creation cancelled before dispatch.".into(),
                        &spec.retired_failure,
                    ));
                }
                match pipe_pair(&descriptor, &user, index == 0).await {
                    Ok((pipe, child)) => {
                        let b = guard.bundle.as_mut().unwrap();
                        b.children[index] = Some(child);
                        match index {
                            0 => b.pipes.stdin = Some(pipe),
                            1 => b.pipes.stdout = Some(pipe),
                            _ => b.pipes.stderr = Some(pipe),
                        }
                    }
                    Err(e) => return Err(guard.fail(e, &spec.retired_failure)),
                }
            }
            let result = (|| {
                let b = guard.bundle.as_mut().unwrap();
                let handles: [HANDLE; 3] =
                    std::array::from_fn(|i| b.children[i].as_ref().unwrap().as_raw_handle());
                let job = b.job_raw();
                let attributes = Attributes::new()?;
                if unsafe {
                    UpdateProcThreadAttribute(
                        attributes.ptr,
                        0,
                        PROC_THREAD_ATTRIBUTE_JOB_LIST as usize,
                        (&job as *const HANDLE).cast(),
                        size_of::<HANDLE>(),
                        null_mut(),
                        null(),
                    )
                } == 0
                    || unsafe {
                        UpdateProcThreadAttribute(
                            attributes.ptr,
                            0,
                            PROC_THREAD_ATTRIBUTE_HANDLE_LIST as usize,
                            handles.as_ptr().cast(),
                            size_of_val(&handles),
                            null_mut(),
                            null(),
                        )
                    } == 0
                {
                    return Err(error("closed attributes"));
                }
                let mut startup: STARTUPINFOEXW = unsafe { zeroed() };
                startup.StartupInfo.cb = size_of::<STARTUPINFOEXW>() as u32;
                startup.StartupInfo.dwFlags = STARTF_USESTDHANDLES;
                startup.StartupInfo.hStdInput = handles[0];
                startup.StartupInfo.hStdOutput = handles[1];
                startup.StartupInfo.hStdError = handles[2];
                startup.lpAttributeList = attributes.ptr;
                let mut process: PROCESS_INFORMATION = unsafe { zeroed() };
                b.executable.check_held_origin()?;
                if UNKNOWN.load(Ordering::Acquire) {
                    return Err("Native admission became unknown before dispatch.".into());
                }
                if spec.cancellation.load(Ordering::Acquire) {
                    return Err("Native creation cancelled before dispatch.".into());
                }
                if unsafe {
                    CreateProcessW(
                        application.as_ptr(),
                        command.as_mut_ptr(),
                        null(),
                        null(),
                        1,
                        CREATE_NO_WINDOW
                            | CREATE_SUSPENDED
                            | CREATE_UNICODE_ENVIRONMENT
                            | EXTENDED_STARTUPINFO_PRESENT,
                        environment.as_mut_ptr().cast(),
                        cwd.as_ref().map_or(null(), |v| v.as_ptr()),
                        &startup.StartupInfo,
                        &mut process,
                    )
                } == 0
                {
                    return Err(error("atomic creation"));
                }
                spec.created.store(true, Ordering::Release);
                b.created = true;
                b.process = Some(own(process.hProcess)?);
                b.thread = Some(own(process.hThread)?);
                b.pid = process.dwProcessId;
                let mut duplicate = null_mut();
                if unsafe {
                    DuplicateHandle(
                        GetCurrentProcess(),
                        job,
                        GetCurrentProcess(),
                        &mut duplicate,
                        0,
                        0,
                        DUPLICATE_SAME_ACCESS,
                    )
                } == 0
                {
                    return Err(error("job capability duplicate"));
                }
                b.job = match ClientJob::from_atomic(
                    own(duplicate)?,
                    b.process.as_ref().unwrap().as_raw_handle(),
                    spec.writer_binding,
                ) {
                    Ok(job) => Some(job),
                    Err(e) => {
                        UNKNOWN.store(true, Ordering::Release);
                        return Err(e);
                    }
                };
                let mut creation: FILETIME = unsafe { zeroed() };
                let mut exit: FILETIME = unsafe { zeroed() };
                let mut kernel: FILETIME = unsafe { zeroed() };
                let mut user_time: FILETIME = unsafe { zeroed() };
                if unsafe {
                    GetProcessTimes(
                        process.hProcess,
                        &mut creation,
                        &mut exit,
                        &mut kernel,
                        &mut user_time,
                    )
                } == 0
                    || unsafe { GetProcessId(process.hProcess) } != process.dwProcessId
                {
                    return Err(error("created identity"));
                }
                let created =
                    ((creation.dwHighDateTime as u64) << 32) | (creation.dwLowDateTime as u64);
                b.executable.verify_original_process(unsafe {
                    std::os::windows::io::BorrowedHandle::borrow_raw(process.hProcess)
                })?;
                for child in &mut b.children {
                    close_checked(child, &mut b.unknown)?
                }
                #[cfg(debug_assertions)]
                if let Some(action) = super::qualification_failures::take()? {
                    if action == super::qualification_failures::Action::RetainOwner {
                        // Real native owner remains retained: no wait/retirement
                        // observation or known-stop token can be produced.
                        UNKNOWN.store(true, Ordering::Release);
                        return Err(
                            "Native fixture retained created owner before retirement.".into()
                        );
                    }
                    return Err("Native fixture failed after creation before first resume.".into());
                }
                resume_after_journal(
                    || match spec.writer_admission.as_mut() {
                        Some(writer) => writer.publish_birth(NativeBirth {
                            pid: process.dwProcessId,
                            created,
                            session,
                        }),
                        None => Ok(()),
                    },
                    || spec.cancellation.load(Ordering::Acquire),
                    || {
                        if unsafe { ResumeThread(process.hThread) } == 1 {
                            Ok(())
                        } else {
                            Err(error("initial suspension"))
                        }
                    },
                )?;
                close_checked(&mut b.thread, &mut b.unknown)?;
                Ok(())
            })();
            match result {
                Ok(()) => Ok(Self { guard }),
                Err(e) => Err(guard.fail(e, &spec.retired_failure)),
            }
        }
        pub(crate) fn id(&self) -> u32 {
            self.guard.bundle.as_ref().unwrap().pid
        }
        #[cfg(debug_assertions)]
        pub(crate) fn qualification_admission_state() -> (usize, bool) {
            // Diagnostic observation only, never an admission/retirement grant.
            (
                ACTIVE.load(Ordering::Acquire),
                UNKNOWN.load(Ordering::Acquire),
            )
        }
        // Read-only observation of an already retained native process against
        // this exact job. A Boolean is not a retirement or writer grant.
        pub(crate) fn contains_process(&self, process: BorrowedHandle<'_>) -> Result<bool, String> {
            let mut member = 0;
            if unsafe {
                IsProcessInJob(
                    process.as_raw_handle(),
                    self.guard.bundle.as_ref().unwrap().job_raw(),
                    &mut member,
                )
            } == 0
            {
                return Err(error("retained member observation"));
            }
            Ok(member != 0)
        }
        // Snapshot only; this does not settle ownership or expose/duplicate the job.
        pub(crate) fn job_observation(
            &self,
        ) -> Result<crate::client_job::ClientJobObservation, String> {
            self.guard
                .bundle
                .as_ref()
                .unwrap()
                .job
                .as_ref()
                .ok_or("Native client job is unavailable.")?
                .observation()
        }
        pub(crate) fn kill(&self) -> Result<(), String> {
            self.guard.bundle.as_ref().unwrap().request_kill()
        }
        pub(crate) fn try_wait(&self) -> Result<Option<u32>, String> {
            let b = self.guard.bundle.as_ref().unwrap();
            if !b.signaled()? {
                return Ok(None);
            }
            let mut code = 0;
            if unsafe { GetExitCodeProcess(b.process.as_ref().unwrap().as_raw_handle(), &mut code) }
                == 0
            {
                return Err(error("exit code"));
            }
            Ok(Some(code))
        }
        pub(crate) async fn wait(&self) -> Result<u32, String> {
            loop {
                if let Some(code) = self.try_wait()? {
                    return Ok(code);
                }
                tokio::time::sleep(Duration::from_millis(10)).await
            }
        }
        pub(crate) fn take_stdin(&mut self) -> Option<NamedPipeServer> {
            self.guard.bundle.as_mut().unwrap().pipes.stdin.take()
        }
        pub(crate) fn take_stdout(&mut self) -> Option<NamedPipeServer> {
            self.guard.bundle.as_mut().unwrap().pipes.stdout.take()
        }
        pub(crate) fn take_stderr(&mut self) -> Option<NamedPipeServer> {
            self.guard.bundle.as_mut().unwrap().pipes.stderr.take()
        }
        pub(crate) fn into_root_exited(mut self) -> Result<NativeRetirement, String> {
            if !self.guard.bundle.as_ref().unwrap().signaled()? {
                return Err("Original client root has not exited.".into());
            }
            Ok(NativeRetirement {
                guard: Guard {
                    bundle: self.guard.bundle.take(),
                },
            })
        }
    }
    impl NativeRetirement {
        // Caller is the sole IO join worker. No renderer Boolean or status DTO
        // substitutes for relinquishing every endpoint. The returned opaque
        // execution capability does not assert all descendant objects signaled;
        // known-writer data settlement additionally requires its owner's file
        // commit and authentic scoped admission. Readers grant no writable data.
        pub(crate) fn retire(mut self) -> Result<ExecutionQuiesced, String> {
            let b = self.guard.bundle.as_mut().unwrap();
            if !b.signaled()? {
                return Err("Native root is not positively exited.".into());
            }
            let job = b.job.take().ok_or("Atomic client job unavailable.")?;
            let retired = job.into_execution_quiesced()?;
            for child in &mut b.children {
                close_checked(child, &mut b.unknown)?
            }
            close_checked(&mut b.thread, &mut b.unknown)?;
            close_checked(&mut b.process, &mut b.unknown)?;
            close_checked(&mut b.original_job, &mut b.unknown)?;
            b.executable.check_held_origin()?;
            let bundle = self.guard.bundle.take().unwrap();
            drop(bundle);
            Ok(retired)
        }
    }
}
#[cfg(windows)]
pub(crate) use native::{
    InspectionClient, NativeAdmissionStopped, NativeBirth, NativeCancellation, NativeClient,
    NativeRetirement, NativeSpawnError, NativeSpec, NeverStartedClient, PersistentSetupClient,
    StoppedWriterClient,
};

#[cfg(test)]
mod pure_tests {
    use super::*;
    #[test]
    fn writer_birth_failure_or_cancellation_never_reaches_first_resume() {
        use std::cell::Cell;
        for publish_fails in [false, true] {
            for cancelled in [false, true] {
                let published = Cell::new(false);
                let resumed = Cell::new(false);
                let result = resume_after_journal(
                    || {
                        if publish_fails {
                            Err("flush/readback refused".into())
                        } else {
                            published.set(true);
                            Ok(())
                        }
                    },
                    || {
                        assert!(published.get());
                        cancelled
                    },
                    || {
                        assert!(published.get());
                        resumed.set(true);
                        Ok(())
                    },
                );
                assert_eq!(result.is_ok(), !publish_fails && !cancelled);
                assert_eq!(resumed.get(), !publish_fails && !cancelled);
            }
        }
        assert!(
            resume_after_journal(|| Ok(()), || false, || Err("ResumeThread failed".into()))
                .is_err()
        );
    }
    fn wide(s: &str) -> Vec<u16> {
        s.encode_utf16().collect()
    }
    struct InertOwner(std::sync::Arc<std::sync::atomic::AtomicUsize>);
    impl Drop for InertOwner {
        fn drop(&mut self) {
            self.0.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
        }
    }
    #[test]
    fn native_failed_admission_and_unexpected_drop_preserve_actual_owner() {
        use std::sync::{
            atomic::{AtomicBool, AtomicUsize, Ordering},
            Arc,
        };
        let drops = Arc::new(AtomicUsize::new(0));
        let unknown = AtomicBool::new(false);
        let kills = AtomicUsize::new(0);
        abandon_owner(InertOwner(drops.clone()), &unknown, |_| {
            kills.fetch_add(1, Ordering::SeqCst);
        });
        assert!(unknown.load(Ordering::SeqCst));
        assert_eq!(drops.load(Ordering::SeqCst), 0);
        assert_eq!(kills.load(Ordering::SeqCst), 1);
        let unknown = AtomicBool::new(false);
        assert!(finish_owner(InertOwner(drops.clone()), &unknown, |_| Err(
            "checked close unknown".into()
        ))
        .is_err());
        assert!(unknown.load(Ordering::SeqCst));
        assert_eq!(drops.load(Ordering::SeqCst), 0);
        let unknown = AtomicBool::new(false);
        assert!(finish_owner(InertOwner(drops.clone()), &unknown, |_| Ok(())).is_ok());
        assert!(!unknown.load(Ordering::SeqCst));
        assert_eq!(drops.load(Ordering::SeqCst), 1);
        let unknown = AtomicBool::new(false);
        let panicked = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
            let _ = finish_owner(InertOwner(drops.clone()), &unknown, |_| {
                panic!("inert retirement fault")
            });
        }));
        assert!(panicked.is_err());
        assert!(unknown.load(Ordering::SeqCst));
        assert_eq!(drops.load(Ordering::SeqCst), 1);
    }
    #[test]
    fn native_quote_preserves_empty_quotes_slashes_and_unicode() {
        for (input, expected) in [
            ("", "\"\""),
            ("one two", "\"one two\""),
            ("a\"b", "\"a\\\"b\""),
            ("tail\\", "\"tail\\\\\""),
            ("λ", "\"λ\""),
        ] {
            assert_eq!(quote_arg(&wide(input)).unwrap(), wide(expected))
        }
        assert!(quote_arg(&[0]).is_err());
        assert_eq!(quote_arg(&[0xd800]).unwrap(), vec![34, 0xd800, 34]);
    }
    #[test]
    fn native_explicit_environment_rejects_ambiguity_and_frames_eof() {
        assert_eq!(environment_block(vec![]).unwrap(), vec![0, 0]);
        assert_eq!(
            environment_block(vec![(wide("B"), wide("2")), (wide("A"), wide("1"))]).unwrap(),
            wide("A=1\0B=2\0\0")
        );
        assert!(
            environment_block(vec![(wide("Path"), wide("a")), (wide("PATH"), wide("b"))]).is_err()
        );
        assert!(environment_block(vec![(wide("A=B"), wide("c"))]).is_err());
        assert_eq!(
            environment_block(vec![
                (wide("PATH"), wide("C:\\tools")),
                (wide("=C:"), wide("C:\\work")),
            ])
            .unwrap(),
            wide("=C:=C:\\work\0PATH=C:\\tools\0\0")
        );
        for malformed in ["=", "=C", "=1:", "=CC:", "=C:="] {
            assert!(environment_block(vec![(wide(malformed), wide("x"))]).is_err());
        }
        assert!(
            environment_block(vec![(wide("=C:"), wide("a")), (wide("=c:"), wide("b"))]).is_err()
        );
        assert!(command_line(&[vec![1; 32767]]).is_err());
    }
    #[cfg(windows)]
    #[test]
    fn native_atomic_windows_abi_matches_closed_sdk_contract() {
        use windows_sys::Win32::System::Threading::*;
        assert_eq!(std::mem::size_of::<STARTUPINFOEXW>(), 112);
        assert_eq!(std::mem::offset_of!(STARTUPINFOEXW, lpAttributeList), 104);
        assert_eq!(std::mem::size_of::<PROCESS_INFORMATION>(), 24);
        assert_eq!(PROC_THREAD_ATTRIBUTE_JOB_LIST, 0x2000d);
        assert_eq!(PROC_THREAD_ATTRIBUTE_HANDLE_LIST, 0x20002);
    }
}
