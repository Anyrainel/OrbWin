//! Explicit native handoff of an existing reviewed user project. Fixed native
//! paths and a clean-environment guest script; no engine/trust changes.
//! Native tools keep their ordinary
//! user environment; OrbWin does not configure an agent or copy credentials.
use serde::Deserialize;
use std::path::PathBuf;

#[derive(Clone, Copy, Deserialize)]
#[serde(rename_all = "camelCase")]
pub(crate) enum Tool {
    Explorer,
    Code,
}

fn authority(name: &str) -> Result<(), String> {
    if name.is_empty()
        || name.len() > 128
        || name.starts_with('-')
        || name == "."
        || name == ".."
        || name.ends_with('.')
        || !name
            .bytes()
            .all(|c| c.is_ascii_alphanumeric() || b"._-".contains(&c))
    {
        return Err("Open this distribution from your own editor; its name is outside the supported native launch format.".into());
    }
    Ok(())
}
fn project_argument(name: &str, source: &str, tool: Tool) -> Result<String, String> {
    authority(name)?;
    if !source.starts_with("/home/")
        || source.len() > 4096
        || source.chars().any(char::is_control)
        || source
            .split('/')
            .skip(1)
            .any(|part| part.is_empty() || part == "." || part == "..")
    {
        return Err("Refresh an existing canonical project below /home before opening it.".into());
    }
    match tool {
        Tool::Explorer => {
            if source.contains(['\\', ':', '*', '?', '"', '<', '>', '|'])
                || source.split('/').any(|part| part.ends_with([' ', '.']))
                || source.split('/').any(|part| {
                    let stem = part.split('.').next().unwrap_or("").to_ascii_uppercase();
                    matches!(stem.as_str(), "CON" | "PRN" | "AUX" | "NUL")
                        || stem
                            .strip_prefix("COM")
                            .or_else(|| stem.strip_prefix("LPT"))
                            .is_some_and(|digit| {
                                matches!(
                                    digit,
                                    "1" | "2"
                                        | "3"
                                        | "4"
                                        | "5"
                                        | "6"
                                        | "7"
                                        | "8"
                                        | "9"
                                        | "¹"
                                        | "²"
                                        | "³"
                                )
                            })
                })
            {
                return Err("This Linux path cannot be represented safely in Explorer. Open it with a WSL-aware editor.".into());
            }
            Ok(format!(
                r"\\wsl.localhost\{}{}",
                name,
                source.replace('/', "\\")
            ))
        }
        Tool::Code => {
            let mut path = String::new();
            for byte in source.bytes() {
                if byte.is_ascii_alphanumeric() || b"/-._~".contains(&byte) {
                    path.push(byte as char);
                } else {
                    path.push_str(&format!("%{byte:02X}"));
                }
            }
            Ok(format!("vscode-remote://wsl+{name}{path}"))
        }
    }
}

#[cfg(windows)]
pub(crate) fn registration_for_name(name: &str) -> Result<String, String> {
    let root = winreg::RegKey::predef(winreg::enums::HKEY_CURRENT_USER)
        .open_subkey("Software\\Microsoft\\Windows\\CurrentVersion\\Lxss")
        .map_err(|_| "WSL registration is unavailable.")?;
    let mut found = Vec::new();
    for (index, key) in root.enum_keys().enumerate() {
        if index >= 512 {
            return Err("WSL registration inventory exceeds its limit.".into());
        }
        let key = key.map_err(|_| "Cannot enumerate WSL registrations.")?;
        let entry = root
            .open_subkey(&key)
            .map_err(|_| "WSL registration changed during observation.")?;
        let raw = registry_value(&entry, "DistributionName", 1, 1024)?;
        if raw.len() < 2 || raw.len() % 2 != 0 {
            return Err("WSL registration name is malformed.".into());
        }
        let wide: Vec<u16> = raw
            .chunks_exact(2)
            .map(|b| u16::from_le_bytes([b[0], b[1]]))
            .collect();
        if wide.last() != Some(&0) || wide[..wide.len() - 1].contains(&0) {
            return Err("WSL registration name is malformed.".into());
        }
        let actual = String::from_utf16(&wide[..wide.len() - 1])
            .map_err(|_| "WSL registration name is not valid Unicode.")?;
        if actual == name {
            if key.len() != 38
                || !key.starts_with('{')
                || !key.ends_with('}')
                || !key.as_bytes()[1..37].iter().enumerate().all(|(at, byte)| {
                    if [8, 13, 18, 23].contains(&at) {
                        *byte == b'-'
                    } else {
                        byte.is_ascii_hexdigit()
                    }
                })
            {
                return Err("The selected WSL registration identity is malformed.".into());
            }
            let version_bytes = registry_value(&entry, "Version", 4, 4)?;
            let version = u32::from_le_bytes(
                version_bytes
                    .try_into()
                    .map_err(|_| "WSL registration version is malformed.")?,
            );
            if version != 2 {
                return Err("Project tools require a user WSL 2 distribution.".into());
            }
            found.push(key[1..37].to_ascii_lowercase());
        }
    }
    if found.len() != 1 {
        return Err(
            "The selected WSL registration is missing or ambiguous. Refresh the list.".into(),
        );
    }
    Ok(found.remove(0))
}
#[cfg(windows)]
fn registry_value(
    key: &winreg::RegKey,
    name: &str,
    expected_type: u32,
    limit: usize,
) -> Result<Vec<u8>, String> {
    let name: Vec<u16> = name.encode_utf16().chain(Some(0)).collect();
    let mut bytes = vec![0u8; limit];
    let mut size = limit as u32;
    let mut kind = 0;
    let result = unsafe {
        windows_sys::Win32::System::Registry::RegQueryValueExW(
            key.raw_handle() as _,
            name.as_ptr(),
            std::ptr::null(),
            &mut kind,
            bytes.as_mut_ptr(),
            &mut size,
        )
    };
    if result != 0 || kind != expected_type || size as usize > limit {
        return Err("WSL registry metadata is unavailable, malformed or exceeds its limit.".into());
    }
    bytes.truncate(size as usize);
    Ok(bytes)
}
#[cfg(not(windows))]
pub(crate) fn registration_for_name(_: &str) -> Result<String, String> {
    Err("WSL registration requires Windows.".into())
}

fn native_base(path: &std::path::Path) -> Result<PathBuf, String> {
    crate::runtime::cli_path(path)
        .map_err(|_| "Native program directories must be absolute local Windows paths.".into())
}

#[tauri::command]
pub(crate) async fn open_wsl_project(
    name: String,
    source: String,
    signature: String,
    registration: String,
    tool: Tool,
) -> Result<(), String> {
    let argument = project_argument(&name, &source, tool)?;
    let wsl_binary = native_base(&crate::wsl::wsl_binary())?;
    crate::wsl_integration::validate_registration(&registration)?;
    crate::wsl::validate(&name, &[name.clone()])?;
    if registration_for_name(&name)? != registration {
        return Err(
            "The inspected WSL registration changed. Refresh the project before opening it.".into(),
        );
    }
    let listing = crate::process::read_command(&wsl_binary, &["--list", "--quiet"]).await?;
    crate::wsl::validate(&name, &crate::wsl::registered(&listing))?;
    #[cfg(windows)]
    {
        let selected = format!("{{{registration}}}");
        let lock_name = name.clone();
        let lock_source = source.clone();
        let lock_signature = signature.clone();
        let _lock = tauri::async_runtime::spawn_blocking(move || {
            crate::wsl_integration::native_project_record(&lock_name, &lock_source, &lock_signature)
        })
        .await
        .map_err(|_| "Project inspection failed.")??;
        // Fixed script, positional data only; ordinary default user, never root.
        crate::process::read_command(&wsl_binary, &["--distribution-id", &selected, "--exec", "/usr/bin/env", "-i", "PATH=/usr/bin:/bin", "/bin/sh", "-c", r#"set -eu; test "$(/usr/bin/id -u)" -ne 0; test -d "$1"; test "$(/usr/bin/readlink -f -- "$1")" = "$1"; test "$(/usr/bin/stat -Lc '%d:%i' -- "$1")" = "$2""#, "orbwin-open-project", &source, &signature]).await?;
        if registration_for_name(&name)? != registration {
            return Err("The WSL registration changed. Refresh before opening the project.".into());
        }
        let program = match tool {
            Tool::Explorer => native_base(&PathBuf::from(
                std::env::var_os("SystemRoot").ok_or("Windows system directory is unavailable.")?,
            ))?
            .join("explorer.exe"),
            Tool::Code => {
                let mut candidates = Vec::new();
                for (variable, suffix) in [
                    ("ProgramFiles", "Microsoft VS Code/Code.exe"),
                    ("LOCALAPPDATA", "Programs/Microsoft VS Code/Code.exe"),
                ] {
                    if let Some(base) = std::env::var_os(variable) {
                        candidates.push(native_base(&PathBuf::from(base))?.join(suffix));
                    }
                }
                candidates.into_iter().find(|p| p.is_file()).ok_or("Microsoft VS Code was not found in a standard installation. Open the project from your editor; OrbWin does not install one automatically.")?
            }
        };
        let mut command = std::process::Command::new(program);
        if matches!(tool, Tool::Code) {
            command.arg("--folder-uri");
        }
        command
            .arg(argument)
            .env_remove("ELECTRON_RUN_AS_NODE")
            .stdin(std::process::Stdio::null())
            .stdout(std::process::Stdio::null())
            .stderr(std::process::Stdio::null())
            .spawn()
            .map_err(|e| format!("The project tool could not be launched: {e}"))?;
        Ok(())
    }
    #[cfg(not(windows))]
    {
        let _ = (argument, signature);
        Err("Native project tools require Windows.".into())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn native_arguments_preserve_exact_project_and_refuse_ambiguous_paths() {
        assert_eq!(
            project_argument("Ubuntu", "/home/you/a b/雪#?%", Tool::Code).unwrap(),
            "vscode-remote://wsl+Ubuntu/home/you/a%20b/%E9%9B%AA%23%3F%25"
        );
        assert_eq!(
            project_argument("Ubuntu-24.04", "/home/you/project", Tool::Explorer).unwrap(),
            r"\\wsl.localhost\Ubuntu-24.04\home\you\project"
        );
        for path in [
            "/etc",
            "/home/you/../other",
            "/home/you//a",
            "/home/you/a:stream",
            "/home/you/a\\b",
            "/home/you/a.",
            "/home/you/con.txt",
            "/home/you/COM1",
            "/home/you/COM¹",
            "/home/you/lpt².txt",
            "/home/you/LPT³",
        ] {
            assert!(
                project_argument("Ubuntu", path, Tool::Explorer).is_err(),
                "{path}"
            );
        }
        for name in ["-d", "Ubuntu;new-tab", "a/b", "a\\b", "a\"b"] {
            assert!(project_argument(name, "/home/you/project", Tool::Code).is_err());
        }
        for name in [".", "..", "Ubuntu."] {
            assert!(project_argument(name, "/home/you/project", Tool::Explorer).is_err());
        }
        for base in ["relative", "C:relative", r"\\server\share"] {
            assert!(native_base(std::path::Path::new(base)).is_err());
        }
        assert!(native_base(std::path::Path::new(r"C:\Windows\System32\wsl.exe")).is_ok());
    }
}
