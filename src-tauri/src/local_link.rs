// The renderer can request only a numeric loopback HTTP(S) port. It cannot pass
// an arbitrary scheme, command, executable, file path, or shell argument.
fn loopback_url(port: u16, scheme: Option<&str>) -> Result<String, String> {
    if port == 0 {
        return Err("Choose a published port between 1 and 65535.".into());
    }
    let scheme = scheme.unwrap_or("http");
    if !matches!(scheme, "http" | "https") {
        return Err("Only HTTP or HTTPS localhost links are supported.".into());
    }
    Ok(format!("{scheme}://127.0.0.1:{port}/"))
}

#[tauri::command]
pub async fn open_local_port(port: u16, scheme: Option<String>) -> Result<(), String> {
    let address = loopback_url(port, scheme.as_deref())?;
    #[cfg(windows)]
    return tauri::async_runtime::spawn_blocking(move || {
        use windows_sys::Win32::UI::{Shell::ShellExecuteW, WindowsAndMessaging::SW_SHOWNORMAL};
        let verb: Vec<u16> = "open\0".encode_utf16().collect();
        let url: Vec<u16> = format!("{address}\0").encode_utf16().collect();
        // Both strings are owned, NUL-terminated, and remain alive for the call.
        let result = unsafe {
            ShellExecuteW(
                std::ptr::null_mut(),
                verb.as_ptr(),
                url.as_ptr(),
                std::ptr::null(),
                std::ptr::null(),
                SW_SHOWNORMAL,
            )
        } as isize;
        if result <= 32 {
            Err(format!(
                "Windows could not open your browser (code {result}). Copy {address} to a browser."
            ))
        } else {
            Ok(())
        }
    })
    .await
    .map_err(|error| error.to_string())?;
    #[cfg(not(windows))]
    Err("Opening a local service requires the Windows desktop application.".into())
}

#[cfg(test)]
mod tests {
    use super::loopback_url;
    #[test]
    fn only_numeric_loopback_http_or_https() {
        assert_eq!(loopback_url(80, None).unwrap(), "http://127.0.0.1:80/");
        assert_eq!(
            loopback_url(65535, Some("https")).unwrap(),
            "https://127.0.0.1:65535/"
        );
        assert!(loopback_url(0, Some("http")).is_err());
        for scheme in [
            "",
            "HTTP",
            "file",
            "javascript",
            "https://example.com",
            "https?x=1",
            "https/other",
            "https@foreign",
            "https\0",
        ] {
            assert!(loopback_url(443, Some(scheme)).is_err(), "{scheme}");
        }
    }
}
