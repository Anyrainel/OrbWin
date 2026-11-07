use std::path::PathBuf;
#[derive(Debug, PartialEq, Eq)]
pub(super) struct InvalidLogDirectory;

// Native count excludes NUL on success. Reject required-size/truncated outputs
// rather than interpreting uninitialized or partial buffers. C: is explicitly
// rooted: PathBuf::join alone would otherwise create a drive-relative target.
pub(super) fn explorer_path(buffer: &[u16], written: u32) -> Result<PathBuf, InvalidLogDirectory> {
    let count = written as usize;
    if count == 0 || count >= buffer.len() || count > 32767 || buffer[count] != 0 {
        return Err(InvalidLogDirectory);
    }
    let text = String::from_utf16(&buffer[..count]).map_err(|_| InvalidLogDirectory)?;
    let bytes = text.as_bytes();
    if bytes.len() < 2
        || !bytes[0].is_ascii_alphabetic()
        || bytes[1] != b':'
        || text
            .chars()
            .any(|c| c.is_control() || matches!(c, '"' | '/' | '\0'))
        || text[2..].contains(':')
        || (bytes.len() > 2 && bytes[2] != b'\\')
        || text[2..]
            .split('\\')
            .any(|part| part == "." || part == "..")
        || (bytes.len() > 3 && text[3..].split('\\').any(str::is_empty))
    {
        return Err(InvalidLogDirectory);
    }
    Ok(PathBuf::from(format!(
        "{}\\explorer.exe",
        text.trim_end_matches('\\')
    )))
}

#[cfg(test)]
mod tests {
    use super::*;
    fn directory(value: &str) -> (Vec<u16>, u32) {
        let mut buffer: Vec<u16> = value.encode_utf16().collect();
        let count = buffer.len() as u32;
        buffer.push(0);
        (buffer, count)
    }
    #[test]
    fn system_directory_is_absolute_even_when_windows_is_at_drive_root() {
        for (input, wanted) in [
            ("C:", r"C:\explorer.exe"),
            (r"D:\", r"D:\explorer.exe"),
            (r"C:\Windows", r"C:\Windows\explorer.exe"),
            (r"C:\Windows ü", r"C:\Windows ü\explorer.exe"),
        ] {
            let (buffer, count) = directory(input);
            assert_eq!(
                explorer_path(&buffer, count).unwrap(),
                PathBuf::from(wanted)
            );
        }
    }
    #[test]
    fn native_directory_failure_or_malformed_output_never_selects_an_executable() {
        for value in [
            "",
            r"Windows",
            r"C:Windows",
            r"\\server\Windows",
            r"C:\..\Windows",
            r"C:\.\Windows",
            r"C:\Windows:other",
            "C:\\Windows\0other",
            r"C:/Windows",
            r#"C:\"Windows"#,
            r"C:\\Windows",
            r"C:\Windows\",
        ] {
            let (buffer, count) = directory(value);
            assert!(explorer_path(&buffer, count).is_err(), "{value:?}");
        }
        let (mut buffer, count) = directory(r"C:\Windows");
        for bad in [0, buffer.len() as u32, 32768, u32::MAX] {
            assert!(explorer_path(&buffer, bad).is_err());
        }
        buffer[count as usize] = 1;
        assert!(explorer_path(&buffer, count).is_err());
        let (mut buffer, count) = directory(r"C:\Windows");
        buffer[3] = 0xd800;
        assert!(explorer_path(&buffer, count).is_err());
    }
}
