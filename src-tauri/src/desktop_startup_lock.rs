// Included only in the Windows startup adapter. Same token user, all sessions
// and cooperating OrbWin copies share this lock. It is not same-SID authentication.
struct StartupLock(windows_sys::Win32::Foundation::HANDLE);
impl Drop for StartupLock {
    fn drop(&mut self) {
        unsafe {
            windows_sys::Win32::System::Threading::ReleaseMutex(self.0);
            windows_sys::Win32::Foundation::CloseHandle(self.0);
        }
    }
}
fn startup_lock() -> Result<StartupLock, String> {
    named_lock("OrbWin.Startup.v1", 5000)
}
fn named_lock(prefix: &str, timeout: u32) -> Result<StartupLock, String> {
    wait_named_lock(named_lock_handle(prefix)?, timeout)
}
fn named_lock_handle(prefix: &str) -> Result<windows_sys::Win32::Foundation::HANDLE, String> {
    use windows_sys::Win32::{
        Foundation::{CloseHandle, LocalFree},
        Security::{
            Authorization::{
                ConvertSidToStringSidW, ConvertStringSecurityDescriptorToSecurityDescriptorW,
            },
            GetTokenInformation, TokenUser, SECURITY_ATTRIBUTES, TOKEN_QUERY, TOKEN_USER,
        },
        System::Threading::{CreateMutexExW, GetCurrentProcess, OpenProcessToken},
    };
    // Fixed bounded aligned storage; never allocate from an untrusted reported size.
    let mut token = std::ptr::null_mut();
    if unsafe { OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &mut token) } == 0 {
        return Err("Cannot identify the current user for sign-in registration.".into());
    }
    let mut buffer = [0u64; 256];
    let mut needed = 0;
    let ok = unsafe {
        GetTokenInformation(
            token,
            TokenUser,
            buffer.as_mut_ptr().cast(),
            std::mem::size_of_val(&buffer) as u32,
            &mut needed,
        )
    };
    unsafe {
        CloseHandle(token);
    }
    if ok == 0
        || needed as usize > std::mem::size_of_val(&buffer)
        || needed < std::mem::size_of::<TOKEN_USER>() as u32
    {
        return Err("Cannot read the current user for sign-in registration.".into());
    }
    let user = unsafe { &*buffer.as_ptr().cast::<TOKEN_USER>() };
    let mut sid_string = std::ptr::null_mut();
    if unsafe { ConvertSidToStringSidW(user.User.Sid, &mut sid_string) } == 0 {
        return Err("Cannot identify the sign-in ownership lock.".into());
    }
    let mut length = 0;
    while length < 184 && unsafe { *sid_string.add(length) } != 0 {
        length += 1;
    }
    let sid = if length < 184 {
        String::from_utf16(unsafe { std::slice::from_raw_parts(sid_string, length) }).ok()
    } else {
        None
    };
    unsafe {
        LocalFree(sid_string.cast());
    }
    let sid = sid.ok_or("Cannot bound the sign-in ownership identity")?;
    // Current user and SYSTEM only; existing objects keep their own descriptor.
    // Request minimal rights on BOTH new and preexisting objects (CreateMutexW
    // would request MUTEX_ALL_ACCESS when opening a preexisting object).
    let descriptor: Vec<u16> = format!("D:P(A;;0x00100001;;;{sid})(A;;0x00100001;;;SY)")
        .encode_utf16()
        .chain(Some(0))
        .collect();
    let mut security = std::ptr::null_mut();
    if unsafe {
        ConvertStringSecurityDescriptorToSecurityDescriptorW(
            descriptor.as_ptr(),
            1,
            &mut security,
            std::ptr::null_mut(),
        )
    } == 0
    {
        return Err("Cannot protect the sign-in ownership lock.".into());
    }
    let attributes = SECURITY_ATTRIBUTES {
        nLength: std::mem::size_of::<SECURITY_ATTRIBUTES>() as u32,
        lpSecurityDescriptor: security,
        bInheritHandle: 0,
    };
    let name: Vec<u16> = format!("Global\\{prefix}.{sid}")
        .encode_utf16()
        .chain(Some(0))
        .collect();
    let handle = unsafe { CreateMutexExW(&attributes, name.as_ptr(), 0, 0x00100001) };
    unsafe {
        LocalFree(security);
    }
    if handle.is_null() {
        return Err("Cannot open the cross-session sign-in lock. No value was changed.".into());
    }
    Ok(handle)
}
// Consumes the handle on every result; successful guard remains on this thread.
fn wait_named_lock(
    handle: windows_sys::Win32::Foundation::HANDLE,
    timeout: u32,
) -> Result<StartupLock, String> {
    use windows_sys::Win32::{
        Foundation::{CloseHandle, WAIT_ABANDONED, WAIT_OBJECT_0, WAIT_TIMEOUT},
        System::Threading::{ReleaseMutex, WaitForSingleObject},
    };
    let result = unsafe { WaitForSingleObject(handle, timeout) };
    if result == WAIT_OBJECT_0 {
        return Ok(StartupLock(handle));
    }
    if result == WAIT_ABANDONED {
        unsafe {
            ReleaseMutex(handle);
        }
    }
    unsafe {
        CloseHandle(handle);
    }
    Err(match result {
        WAIT_ABANDONED => "A previous sign-in operation was interrupted. Read the setting again before retrying; this request made no registry change.",
        WAIT_TIMEOUT => "Sign-in registration is busy in another OrbWin process. This request made no registry change.",
        _ => "Cannot acquire the sign-in ownership lock. This request made no registry change.",
    }.into())
}
