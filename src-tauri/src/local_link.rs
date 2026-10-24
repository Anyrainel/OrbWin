// The renderer can request only a numeric loopback HTTP port. It cannot pass
// an arbitrary scheme, command, executable, file path, or shell argument.
#[tauri::command]
pub async fn open_local_port(port: u16) -> Result<(), String> {
    if port == 0 {
        return Err("Choose a published port between 1 and 65535.".into());
    }
    #[cfg(windows)]
    return tauri::async_runtime::spawn_blocking(move || {
        use windows_sys::Win32::UI::{Shell::ShellExecuteW, WindowsAndMessaging::SW_SHOWNORMAL};
        let verb: Vec<u16> = "open\0".encode_utf16().collect();
        let url: Vec<u16> = format!("http://127.0.0.1:{port}/\0").encode_utf16().collect();
        // Both strings are owned, NUL-terminated, and remain alive for the call.
        let result = unsafe { ShellExecuteW(std::ptr::null_mut(), verb.as_ptr(), url.as_ptr(), std::ptr::null(), std::ptr::null(), SW_SHOWNORMAL) } as isize;
        if result <= 32 { Err(format!("Windows could not open your browser (code {result}). Copy http://127.0.0.1:{port}/ to a browser.")) } else { Ok(()) }
    }).await.map_err(|error| error.to_string())?;
    #[cfg(not(windows))]
    Err("Opening a local service requires the Windows desktop application.".into())
}
