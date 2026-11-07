//! Native file actions select an existing local copy through an OS dialog.
//! Only fixed Explorer/Notepad binaries run. No shell association or command is
//! accepted, and no automatic upload follows an editor save. Engine-private
//! paths are never derived/exposed by this flow. The user owns the local copy.
use std::path::{Path, PathBuf};

fn selected_copy(path: &Path, edit: bool) -> Result<PathBuf, String> {
    let resolved = path.canonicalize().map_err(|error| error.to_string())?;
    let text = resolved
        .to_str()
        .ok_or("The selected path is not valid Unicode.")?;
    if !crate::mounts::windows_source(text)
        || text.chars().any(char::is_control)
        || text.strip_prefix(r"\\?\").unwrap_or(text)[3..].contains(':')
    {
        return Err("Choose a regular file on a local Windows drive.".into());
    }
    let metadata = resolved.metadata().map_err(|error| error.to_string())?;
    if !metadata.is_file() {
        return Err("Choose an existing regular file.".into());
    }
    if edit {
        use std::io::Read;
        let mut bytes = Vec::new();
        std::fs::File::open(&resolved)
            .map_err(|error| error.to_string())?
            .take(256 * 1024 + 1)
            .read_to_end(&mut bytes)
            .map_err(|error| error.to_string())?;
        if bytes.len() > 256 * 1024 || bytes.contains(&0) || std::str::from_utf8(&bytes).is_err() {
            return Err("Notepad integration supports UTF-8 text up to 256 KiB. Reveal other files in Explorer and choose your own tool.".into());
        }
    }
    Ok(resolved)
}

#[tauri::command]
pub async fn open_selected_copy(window: tauri::WebviewWindow, edit: bool) -> Result<bool, String> {
    let dialog = rfd::AsyncFileDialog::new()
        .set_parent(&window)
        .set_title(if edit {
            "Open a local text copy in Notepad (no automatic upload)"
        } else {
            "Reveal a local file in Explorer"
        });
    let Some(file) = dialog.pick_file().await else {
        return Ok(false);
    };
    let selected = selected_copy(file.path(), edit)?;
    launch_copy(selected, edit).await?;
    Ok(true)
}

#[tauri::command]
pub async fn open_local_copy(path: String, edit: bool) -> Result<(), String> {
    let selected = selected_copy(Path::new(&path), edit)?;
    launch_copy(selected, edit).await
}

async fn launch_copy(selected: PathBuf, edit: bool) -> Result<(), String> {
    #[cfg(windows)]
    {
        let root =
            std::env::var_os("SystemRoot").ok_or("Windows system directory is unavailable.")?;
        let path = crate::runtime::cli_path(&selected)?;
        let mut command = std::process::Command::new(PathBuf::from(root).join(if edit {
            "System32/notepad.exe"
        } else {
            "explorer.exe"
        }));
        if edit {
            command.arg(path);
        } else {
            let mut select = std::ffi::OsString::from("/select,");
            select.push(path);
            command.arg(select);
        }
        command
            .spawn()
            .map_err(|error| format!("Could not open the selected local copy: {error}"))?;
        Ok(())
    }
    #[cfg(not(windows))]
    {
        let _ = selected;
        Err("Native file tools require Windows.".into())
    }
}

#[cfg(all(test, windows))]
mod tests {
    use super::*;
    #[test]
    fn native_editor_refuses_binary_and_large_copies() {
        let directory =
            std::env::temp_dir().join(format!("orbwin-native-files-{}", std::process::id()));
        std::fs::create_dir(&directory).unwrap();
        let file = directory.join("text ü.txt");
        std::fs::write(&file, b"ordinary text\n").unwrap();
        assert!(selected_copy(&file, true).is_ok());
        std::fs::write(&file, b"binary\0data").unwrap();
        assert!(selected_copy(&file, true).is_err());
        assert!(selected_copy(&file, false).is_ok());
        std::fs::write(&file, vec![b'x'; 256 * 1024 + 1]).unwrap();
        assert!(selected_copy(&file, true).is_err());
        assert!(selected_copy(&directory, false).is_err());
        std::fs::remove_file(file).unwrap();
        std::fs::remove_dir(directory).unwrap();
    }
}
