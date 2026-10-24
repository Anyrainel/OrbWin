//! Open a user's registered WSL distribution in a new console window.
//!
//! Invariants: the renderer supplies only a distribution name; it is validated
//! against the names `wsl.exe --list --quiet` reports right now and never reaches
//! a command interpreter. ShellExecute launches the fixed WSL executable with
//! a separately quoted registered name. OrbWin's and other engines' distros are
//! refused. Nothing is started, stopped, registered or modified beyond what an
//! interactive `wsl -d <name>` shell itself does. The console is detached; its
//! lifetime belongs to the user.
use crate::process;
use std::path::PathBuf;

pub(crate) fn wsl_binary() -> PathBuf {
    let root = std::env::var_os("SystemRoot").unwrap_or_else(|| "C:\\Windows".into());
    PathBuf::from(root).join("System32/wsl.exe")
}

/// Names from `--list --quiet`: one per line, possibly with NUL/BOM/CR residue.
pub(crate) fn registered(output: &str) -> Vec<String> {
    output
        .replace(['\u{feff}', '\0', '\r'], "")
        .lines()
        .map(|line| line.trim().trim_start_matches('*').trim().to_string())
        .filter(|line| !line.is_empty())
        .collect()
}

fn protected(name: &str) -> bool {
    let lower = name.to_lowercase();
    lower.starts_with("lima-orbwin")
        || lower.starts_with("orbwin")
        || lower.starts_with("rancher-desktop")
        || lower.starts_with("lima-rd")
        || lower.starts_with("docker-desktop")
}

pub(crate) fn validate(name: &str, names: &[String]) -> Result<(), String> {
    if name.is_empty()
        || name.len() > 128
        || name.starts_with('-')
        || name.chars().any(char::is_control)
    {
        return Err("Invalid distribution name.".into());
    }
    if protected(name) {
        return Err(
            "This distribution belongs to a container engine; OrbWin does not open shells in it."
                .into(),
        );
    }
    if !names.iter().any(|candidate| candidate == name) {
        return Err("That distribution is not registered. Refresh the list and retry.".into());
    }
    Ok(())
}

#[tauri::command]
pub async fn wsl_open_terminal(name: String) -> Result<(), String> {
    let binary = wsl_binary();
    let listing = process::read_command(&binary, &["--list", "--quiet"]).await?;
    validate(&name, &registered(&listing))?;
    #[cfg(windows)]
    {
        tauri::async_runtime::spawn_blocking(move || launch_terminal(&binary, &name))
            .await
            .map_err(|error| error.to_string())?
    }
    #[cfg(not(windows))]
    {
        let _ = binary;
        Err("Opening a WSL terminal requires Windows.".into())
    }
}

#[cfg(windows)]
fn quote_argument(value: &str) -> String {
    let mut result = String::from("\"");
    let mut slashes = 0;
    for ch in value.chars() {
        if ch == '\\' {
            slashes += 1;
            continue;
        }
        result.push_str(&"\\".repeat(if ch == '"' { slashes * 2 + 1 } else { slashes }));
        result.push(ch);
        slashes = 0;
    }
    result.push_str(&"\\".repeat(slashes * 2));
    result.push('"');
    result
}

#[cfg(windows)]
fn launch_terminal(binary: &std::path::Path, name: &str) -> Result<(), String> {
    // Windows Terminal owns the interactive PTY. Its launcher does not depend
    // on inheriting a console input handle from this GUI process.
    let terminal = std::env::var_os("LOCALAPPDATA")
        .map(PathBuf::from)
        .map(|path| path.join("Microsoft/WindowsApps/wt.exe"));
    if let Some(terminal) = terminal.filter(|path| path.is_file()) {
        // WT has its own semicolon command separator, independently of host
        // shell quoting. Never let a registered name become another WT action.
        if name.contains(';') {
            return Err("This distribution name contains a Windows Terminal command separator. Run the copied WSL command in your own terminal.".into());
        }
        let mut child = std::process::Command::new(terminal)
            .args(["--window","new","new-tab","--title",&format!("{name} — OrbWin"),"--suppressApplicationTitle"])
            .arg(binary).arg("--distribution").arg(name)
            .stdin(std::process::Stdio::null()).stdout(std::process::Stdio::null()).stderr(std::process::Stdio::null())
            .spawn().map_err(|error|format!("Windows Terminal could not be launched: {error}. Run the copied command in your terminal."))?;
        std::thread::sleep(std::time::Duration::from_millis(250));
        if let Some(status) = child.try_wait().map_err(|error| error.to_string())? {
            if !status.success() {
                return Err(format!("Windows Terminal rejected the launch ({status}). Run the copied WSL command in your terminal."));
            }
        }
        return Ok(());
    }
    use std::os::windows::ffi::OsStrExt;
    use windows_sys::Win32::{
        Foundation::{CloseHandle, WAIT_FAILED, WAIT_OBJECT_0},
        System::Threading::{GetExitCodeProcess, WaitForSingleObject},
        UI::{
            Shell::{
                ShellExecuteExW, SEE_MASK_NOASYNC, SEE_MASK_NOCLOSEPROCESS, SHELLEXECUTEINFOW,
            },
            WindowsAndMessaging::SW_SHOWNORMAL,
        },
    };
    let file: Vec<u16> = binary.as_os_str().encode_wide().chain(Some(0)).collect();
    let parameters: Vec<u16> = format!("--distribution {}\0", quote_argument(name))
        .encode_utf16()
        .collect();
    let verb: Vec<u16> = "open\0".encode_utf16().collect();
    // Let Windows supply an actual interactive console. Inheriting the GUI's
    // stdin through Command::spawn caused WSL shells to receive EOF and exit.
    let mut launch = SHELLEXECUTEINFOW {
        cbSize: std::mem::size_of::<SHELLEXECUTEINFOW>() as u32,
        fMask: SEE_MASK_NOCLOSEPROCESS | SEE_MASK_NOASYNC,
        lpVerb: verb.as_ptr(),
        lpFile: file.as_ptr(),
        lpParameters: parameters.as_ptr(),
        nShow: SW_SHOWNORMAL,
        ..Default::default()
    };
    // All pointer-backed strings outlive this synchronous call. No host shell
    // or arbitrary renderer-supplied executable is invoked.
    if unsafe { ShellExecuteExW(&mut launch) } == 0 {
        return Err(format!(
            "Windows could not launch WSL: {}. Run the copied command in your terminal.",
            std::io::Error::last_os_error()
        ));
    }
    if launch.hProcess.is_null() {
        return Ok(());
    }
    let result = unsafe { WaitForSingleObject(launch.hProcess, 250) };
    let mut exit = 0;
    let failure = if result == WAIT_OBJECT_0 {
        unsafe { GetExitCodeProcess(launch.hProcess, &mut exit) };
        Some(format!("The WSL terminal exited immediately (code {exit}). Run the copied command in your terminal to inspect its startup error."))
    } else if result == WAIT_FAILED {
        Some(format!(
            "The terminal launch could not be checked: {}",
            std::io::Error::last_os_error()
        ))
    } else {
        None
    };
    // The user owns this interactive client. Closing our handle never stops it.
    unsafe { CloseHandle(launch.hProcess) };
    failure.map_or(Ok(()), Err)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_utf16_residue_and_default_marker() {
        let names = registered("\u{feff}Ubuntu\r\n*Debian\r\n\0\r\n");
        assert_eq!(names, ["Ubuntu", "Debian"]);
    }
    #[test]
    fn refuses_engine_unregistered_and_option_like_names() {
        let names = vec!["Ubuntu".to_string(), "lima-orbwin".into(), "-x".into()];
        assert!(validate("Ubuntu", &names).is_ok());
        assert!(validate("lima-orbwin", &names).is_err());
        assert!(validate("Missing", &names).is_err());
        assert!(validate("-x", &names).is_err());
    }
    #[cfg(windows)]
    #[test]
    fn registered_distribution_is_one_windows_argument() {
        use windows_sys::Win32::{Foundation::LocalFree, UI::Shell::CommandLineToArgvW};
        for value in [
            "Ubuntu",
            "Ubuntu 24.04",
            "name\"quoted",
            "ends\\",
            "x\\\" -d foreign",
            "例 distro",
        ] {
            let command: Vec<u16> = format!("wsl.exe {}\0", quote_argument(value))
                .encode_utf16()
                .collect();
            let mut count = 0;
            let args = unsafe { CommandLineToArgvW(command.as_ptr(), &mut count) };
            assert!(!args.is_null());
            assert_eq!(count, 2);
            let second = unsafe { *args.add(1) };
            let mut len = 0;
            while unsafe { *second.add(len) } != 0 {
                len += 1;
            }
            let actual =
                String::from_utf16_lossy(unsafe { std::slice::from_raw_parts(second, len) });
            unsafe { LocalFree(args.cast()) };
            assert_eq!(actual, value);
        }
    }
}
