//! Read-only capacity of an already resolved data directory. No disk handles,
//! privilege changes or engine operations. Quota-limited caller capacity and
//! physical free bytes are distinct; neither measures guest filesystem space.
use serde::Serialize;
use std::path::Path;

#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct HostCapacity {
    directory: String,
    available_bytes: u64,
    total_bytes: u64,
    free_bytes: u64,
}

fn measured(
    directory: &Path,
    available: u64,
    total: u64,
    free: u64,
) -> Result<HostCapacity, String> {
    const EXACT_JS_INTEGER: u64 = 9_007_199_254_740_991;
    if total == 0
        || available > total
        || available > free
        || [available, total, free]
            .iter()
            .any(|value| *value > EXACT_JS_INTEGER)
    {
        return Err("Windows returned an unsupported host capacity measurement.".into());
    }
    Ok(HostCapacity {
        directory: directory.display().to_string(),
        available_bytes: available,
        total_bytes: total,
        free_bytes: free,
    })
}

#[cfg(windows)]
pub fn query(directory: &Path) -> Result<HostCapacity, String> {
    use std::os::windows::ffi::OsStrExt;
    use windows_sys::Win32::Storage::FileSystem::GetDiskFreeSpaceExW;
    let mut path: Vec<u16> = directory.as_os_str().encode_wide().collect();
    if path.is_empty() || path.contains(&0) {
        return Err("Invalid host capacity directory.".into());
    }
    if path.last() != Some(&(b'\\' as u16)) {
        path.push(b'\\' as u16);
    }
    path.push(0);
    let (mut available, mut total, mut free) = (0u64, 0u64, 0u64);
    // All pointers are valid for this synchronous call; path is NUL-terminated.
    if unsafe { GetDiskFreeSpaceExW(path.as_ptr(), &mut available, &mut total, &mut free) } == 0 {
        return Err(format!(
            "Host capacity could not be measured: {}",
            std::io::Error::last_os_error()
        ));
    }
    measured(directory, available, total, free)
}

#[cfg(not(windows))]
pub fn query(_: &Path) -> Result<HostCapacity, String> {
    Err("Host capacity requires Windows.".into())
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn quota_capacity_must_not_be_confused_with_physical_free_space() {
        let value = measured(Path::new("data"), 10, 20, 100).unwrap();
        assert_eq!(value.available_bytes, 10);
        assert_eq!(value.free_bytes, 100);
        for (available, total, free) in [
            (1, 0, 1),
            (2, 1, 2),
            (2, 3, 1),
            (1, 9_007_199_254_740_992, 2),
        ] {
            assert!(measured(Path::new("data"), available, total, free).is_err());
        }
    }
    #[cfg(windows)]
    #[test]
    fn reads_existing_directory_and_refuses_missing_unicode_directory() {
        let root = std::env::temp_dir();
        assert!(query(&root).is_ok());
        let absent = root.join(format!("orbwin-missing-capacity-例-{}", std::process::id()));
        assert!(!absent.exists());
        assert!(query(&absent).is_err());
    }
}
