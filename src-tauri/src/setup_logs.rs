//! Fixed existing setup-log directory only. No caller path or setup effect.
use serde::Serialize;
use std::path::PathBuf;

#[derive(Clone, Copy, Debug, PartialEq, Eq, Serialize)]
#[serde(tag = "code", rename_all = "kebab-case")]
pub(crate) enum LogAccessError {
    Missing,
    Unavailable,
    OpenFailed,
}

#[path = "setup_log_path.rs"]
mod path;

#[cfg(windows)]
fn native_explorer() -> Result<PathBuf, LogAccessError> {
    #[link(name = "kernel32")]
    unsafe extern "system" {
        fn GetSystemWindowsDirectoryW(buffer: *mut u16, size: u32) -> u32;
    }
    let mut buffer = vec![0u16; 32768];
    let count = unsafe { GetSystemWindowsDirectoryW(buffer.as_mut_ptr(), buffer.len() as u32) };
    path::explorer_path(&buffer, count).map_err(|_| LogAccessError::Unavailable)
}

#[cfg(windows)]
fn open_existing() -> Result<(), LogAccessError> {
    let lease = crate::runtime_cache::existing_private_setup_logs()
        .map_err(|_| LogAccessError::Unavailable)?
        .ok_or(LogAccessError::Missing)?;
    // Keep original native leases outside unwind scope, then checked-release
    // them after any normal/panic launch result. No navigation confirmation.
    let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        let program = native_explorer()?;
        let directory = lease.path().map_err(|_| LogAccessError::Unavailable)?;
        let argument =
            crate::runtime::cli_path(directory).map_err(|_| LogAccessError::Unavailable)?;
        std::process::Command::new(program)
            .arg(argument)
            .spawn()
            .map(|_| ())
            .map_err(|_| LogAccessError::OpenFailed)
    }));
    lease.close().map_err(|_| LogAccessError::Unavailable)?;
    result.map_err(|_| LogAccessError::Unavailable)?
}

#[tauri::command]
pub(crate) async fn open_setup_logs() -> Result<(), LogAccessError> {
    #[cfg(windows)]
    {
        tokio::task::spawn_blocking(open_existing)
            .await
            .map_err(|_| LogAccessError::Unavailable)?
    }
    #[cfg(not(windows))]
    {
        Err(LogAccessError::Unavailable)
    }
}
