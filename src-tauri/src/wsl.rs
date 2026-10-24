//! Open a user's registered WSL distribution in a new console window.
//!
//! Invariants: the renderer supplies only a distribution name; it is validated
//! against the names `wsl.exe --list --quiet` reports right now and never reaches
//! a shell (argv only). OrbWin's own engine distro and other engines' distros are
//! refused. Nothing is started, stopped, registered or modified beyond what an
//! interactive `wsl -d <name>` shell itself does. The console is detached; its
//! lifetime belongs to the user.
use crate::process;
use std::path::PathBuf;

fn wsl_binary() -> PathBuf {
    let root = std::env::var_os("SystemRoot").unwrap_or_else(|| "C:\\Windows".into());
    PathBuf::from(root).join("System32/wsl.exe")
}

/// Names from `--list --quiet`: one per line, possibly with NUL/BOM/CR residue.
fn registered(output: &str) -> Vec<String> {
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

fn validate(name: &str, names: &[String]) -> Result<(), String> {
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
        use std::os::windows::process::CommandExt;
        const CREATE_NEW_CONSOLE: u32 = 0x0000_0010;
        std::process::Command::new(&binary)
            .arg("-d")
            .arg(&name)
            .creation_flags(CREATE_NEW_CONSOLE)
            .spawn()
            .map(drop)
            .map_err(|e| format!("Could not open a terminal: {e}"))
    }
    #[cfg(not(windows))]
    {
        let _ = binary;
        Err("Opening a WSL terminal requires Windows.".into())
    }
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
}
