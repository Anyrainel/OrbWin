//! UI mount arguments only; this does not rewrite arbitrary Docker API traffic.
//! Existing host paths are canonicalized before mapping the drive into WSL's
//! default /mnt layout, matching pinned rdd pkg/guestexec/hostpath.go. No directory creation, WSL command or global setting
//! change occurs here. UNC/device shares require a separate supported contract.
use std::path::Path;

pub(crate) fn windows_source(path: &str) -> bool {
    // canonicalize() and native file pickers can return verbatim drive paths.
    // Recognize only the local-drive form; verbatim UNC/device paths still fail.
    let path = path.strip_prefix(r"\\?\").unwrap_or(path);
    let bytes = path.as_bytes();
    bytes.len() >= 3
        && bytes[0].is_ascii_alphabetic()
        && bytes[1] == b':'
        && matches!(bytes[2], b'\\' | b'/')
}

fn drive_path(path: &str) -> Result<String, String> {
    let path = path.strip_prefix(r"\\?\").unwrap_or(path);
    if !windows_source(path) || path.contains(',') || path.chars().any(char::is_control) {
        return Err(
            "Use an existing local Windows drive path; UNC and device paths are not supported."
                .into(),
        );
    }
    let suffix = path[3..].replace('\\', "/");
    if suffix
        .split('/')
        .any(|part| matches!(part, "." | "..") || part.contains(':'))
    {
        return Err("The resolved mount source is not a local drive path.".into());
    }
    Ok(format!(
        "/mnt/{}/{}",
        path[..1].to_ascii_lowercase(),
        suffix
    ))
}

pub(crate) fn bind(specification: &str) -> Result<String, String> {
    let (specification, readonly) = specification
        .strip_suffix(":ro")
        .map_or((specification, false), |spec| (spec, true));
    let (source, target) = specification
        .rsplit_once(":/")
        .ok_or("Use C:\\existing path:/container/path[:ro].")?;
    if target.is_empty() || target.contains([',', ':']) || target.chars().any(char::is_control) {
        return Err("Bind targets must be absolute Linux paths without commas, colons or control characters.".into());
    }
    if !windows_source(source) {
        return Err("Bind source must be an absolute local Windows drive path.".into());
    }
    let resolved = Path::new(source)
        .canonicalize()
        .map_err(|e| format!("Mount source must already exist and be accessible: {e}"))?;
    let metadata = resolved.metadata().map_err(|e| e.to_string())?;
    if !metadata.is_file() && !metadata.is_dir() {
        return Err("Mount source must be a regular file or directory.".into());
    }
    let guest = drive_path(
        resolved
            .to_str()
            .ok_or("Mount source is not valid Unicode.")?,
    )?;
    Ok(format!(
        "type=bind,source={guest},target=/{}{}",
        target,
        if readonly { ",readonly" } else { "" }
    ))
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn maps_drive_case_spaces_unicode_and_resolved_windows_prefix() {
        assert_eq!(
            drive_path(r"F:\Codes\bind source").unwrap(),
            "/mnt/f/Codes/bind source"
        );
        assert_eq!(
            drive_path(r"\\?\C:\work\例.txt").unwrap(),
            "/mnt/c/work/例.txt"
        );
        assert_eq!(drive_path("D:/folder/file").unwrap(), "/mnt/d/folder/file");
    }
    #[test]
    fn rejects_shares_devices_relative_paths_and_mount_option_injection() {
        for source in [
            r"\\server\share",
            r"\\.\pipe\docker",
            r"C:relative",
            r"C:\a\..\b",
            r"C:\a,readonly",
            "C:\\a\n",
            r"C:\file:stream",
        ] {
            assert!(drive_path(source).is_err(), "{source}");
        }
        assert!(bind(r"C:\absent:/data,readonly").is_err());
        assert!(bind(r"C:\absent:/data:rw").is_err());
    }
    #[test]
    fn existing_host_file_is_mapped_without_creating_missing_sources() {
        let stamp = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_nanos();
        let source =
            std::env::temp_dir().join(format!("orbwin-mount-{}-{stamp}.txt", std::process::id()));
        let fixture = std::fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(&source)
            .unwrap();
        drop(fixture);
        let specification = format!("{}:/data/file:ro", source.display());
        let result = bind(&specification);
        let canonical_specification =
            format!("{}:/data/file:ro", source.canonicalize().unwrap().display());
        assert!(windows_source(&canonical_specification));
        assert_eq!(bind(&canonical_specification), result);
        std::fs::remove_file(&source).unwrap();
        let argument = result.unwrap();
        assert!(argument.starts_with("type=bind,source=/mnt/"));
        assert!(argument.ends_with(",target=/data/file,readonly"));
        assert!(bind(&specification).is_err());
    }
}
