//! Pure startup ownership and transition policy; no OS, tray or backend calls.
#[derive(Clone, Debug, PartialEq, Eq)]
pub(crate) struct StartupValue {
    pub kind: u32,
    pub bytes: Vec<u8>,
}
pub(crate) fn expected_value(executable: &str) -> Result<StartupValue, String> {
    let bytes = executable.as_bytes();
    if bytes.len() < 7
        || !bytes[0].is_ascii_alphabetic()
        || bytes[1] != b':'
        || !matches!(bytes[2], b'\\' | b'/')
        || executable.chars().any(|c| c == '"' || c.is_control())
        || executable[2..].contains(':')
        || executable.split(['\\', '/']).any(|p| p == "." || p == "..")
        || !executable.to_ascii_lowercase().ends_with(".exe")
    {
        return Err("Sign-in launch requires this application's absolute local executable.".into());
    }
    let command = format!("\"{executable}\" --background");
    let utf16: Vec<u16> = command.encode_utf16().chain(Some(0)).collect();
    if utf16.len() - 1 > 260 {
        return Err("Application path exceeds the sign-in command limit.".into());
    }
    Ok(StartupValue {
        kind: 1,
        bytes: utf16.into_iter().flat_map(u16::to_le_bytes).collect(),
    })
}
pub(crate) fn is_enabled(
    value: Option<&StartupValue>,
    expected: &StartupValue,
) -> Result<bool, String> {
    match value {
        None => Ok(false),
        Some(value) if value == expected => Ok(true),
        Some(_) => Err("Another or modified command owns OrbWin's sign-in entry. It was not changed. Review Startup Apps or the entry before trying again.".into()),
    }
}
/// Caller holds the cross-process ownership lock throughout this operation.
/// Ordinary Windows value writes are immediate, never a registry transaction.
pub(crate) trait StartupStore {
    fn read(&mut self) -> Result<Option<StartupValue>, String>;
    fn write(&mut self, value: Option<&StartupValue>) -> Result<(), String>;
}
fn replace<T: StartupStore>(
    store: &mut T,
    before: Option<&StartupValue>,
    after: Option<&StartupValue>,
) -> Result<(), String> {
    if store.read()?.as_ref() != before {
        return Err(
            "Sign-in entry changed before writing. It was not changed by this request.".into(),
        );
    }
    store.write(after)?;
    let observed = store.read().map_err(|_| "Sign-in registration may have changed, but its final read failed. Read it again before retrying; no automatic rollback was made.".to_string())?;
    if observed.as_ref() != after {
        return Err("Sign-in write could not be confirmed. Read the setting again; no automatic retry or rollback was made.".into());
    }
    Ok(())
}
pub(crate) fn change<T: StartupStore>(
    store: &mut T,
    expected: &StartupValue,
    enabled: bool,
) -> Result<bool, String> {
    let current = store.read()?;
    if is_enabled(current.as_ref(), expected)? == enabled {
        return Ok(false);
    }
    replace(store, current.as_ref(), enabled.then_some(expected))?;
    Ok(true)
}
#[derive(Debug, PartialEq, Eq)]
pub(crate) enum Cleanup {
    RemovedOrAbsent,
    ForeignPreserved,
}
pub(crate) fn cleanup<T: StartupStore>(
    store: &mut T,
    expected: &StartupValue,
) -> Result<Cleanup, String> {
    let value = store.read()?;
    match is_enabled(value.as_ref(), expected) {
        Err(_) => Ok(Cleanup::ForeignPreserved),
        Ok(false) => Ok(Cleanup::RemovedOrAbsent),
        Ok(true) => {
            replace(store, value.as_ref(), None)?;
            Ok(Cleanup::RemovedOrAbsent)
        }
    }
}
pub(crate) fn uninstall_mode(args: &[String]) -> bool {
    matches!(args,[arg] if arg == "--uninstall-owned-startup")
}
#[derive(Debug, PartialEq, Eq)]
pub(crate) enum Launch {
    Manual,
    Background,
    Invalid,
}
pub(crate) fn launch(args: &[String]) -> Launch {
    match args {
        [] => Launch::Manual,
        [arg] if arg == "--background" => Launch::Background,
        _ => Launch::Invalid,
    }
}
pub(crate) fn hide_initial(args: &[String], tray_created: bool) -> bool {
    tray_created && launch(args) == Launch::Background
}

#[cfg(test)]
mod tests {
    use super::*;
    #[derive(Default)]
    struct Mock {
        current: Option<StartupValue>,
        reads: usize,
        writes: usize,
        fail: Option<&'static str>,
        changed: Option<StartupValue>,
    }
    impl StartupStore for Mock {
        fn read(&mut self) -> Result<Option<StartupValue>, String> {
            self.reads += 1;
            if self.fail == Some("read") || self.fail == Some("postread") && self.reads == 3 {
                return Err("denied".into());
            }
            if self.reads == 2 && self.changed.is_some() {
                self.current = self.changed.take();
            }
            Ok(self.current.clone())
        }
        fn write(&mut self, v: Option<&StartupValue>) -> Result<(), String> {
            self.writes += 1;
            if self.fail == Some("write") {
                return Err("write failed".into());
            }
            self.current = v.cloned();
            if self.fail == Some("ack") {
                return Err("write acknowledgement lost".into());
            }
            if self.fail == Some("replace-after") {
                self.current = Some(expected_value(r"C:\Foreign\a.exe").unwrap());
            }
            Ok(())
        }
    }
    fn expected() -> StartupValue {
        expected_value(r"C:\Example User\orbwin.exe").unwrap()
    }
    #[test]
    fn quoting_and_run_command_utf16_boundary_include_non_bmp() {
        let prefix = r"C:\";
        let suffix = ".exe";
        // quoted path + space + --background = 15 extra UTF16 units.
        let overhead = format!("\"{prefix}{suffix}\" --background")
            .encode_utf16()
            .count();
        let exact = format!("{prefix}{}{suffix}", "a".repeat(260 - overhead));
        assert!(expected_value(&exact).is_ok());
        assert!(
            expected_value(&format!("{prefix}{}b{suffix}", "a".repeat(260 - overhead))).is_err()
        );
        let exact_unicode = format!("{prefix}{}😀{suffix}", "a".repeat(258 - overhead));
        assert!(expected_value(&exact_unicode).is_ok());
        assert!(expected_value(&exact_unicode.replace("😀", "😀b")).is_err());
        for path in [
            r"relative.exe",
            r"\\server\share\a.exe",
            "C:\\a\" --evil.exe",
            "C:\\a\n.exe",
            "C:\\a.exe:stream",
            "C:\\x\\..\\a.exe",
            "C:\\a.cmd",
        ] {
            assert!(expected_value(path).is_err(), "{path}");
        }
        let value = expected_value(r"F:\例 project & safe\orbwin.exe").unwrap();
        let units: Vec<_> = value
            .bytes
            .chunks_exact(2)
            .map(|v| u16::from_le_bytes([v[0], v[1]]))
            .collect();
        assert_eq!(
            String::from_utf16(&units[..units.len() - 1]).unwrap(),
            "\"F:\\例 project & safe\\orbwin.exe\" --background"
        );
        assert_eq!(units.last(), Some(&0));
    }
    #[test]
    fn absent_owned_transitions_repeat_without_writes() {
        let own = expected();
        let mut store = Mock::default();
        assert_eq!(change(&mut store, &own, false), Ok(false));
        assert_eq!(change(&mut store, &own, true), Ok(true));
        assert_eq!(change(&mut store, &own, true), Ok(false));
        assert_eq!(change(&mut store, &own, false), Ok(true));
        assert_eq!(change(&mut store, &own, false), Ok(false));
        assert_eq!(store.writes, 2);
    }
    #[test]
    fn foreign_values_and_changes_before_write_are_preserved() {
        let own = expected();
        let mut altered = own.clone();
        altered.bytes.pop();
        for foreign in [
            expected_value(r"C:\Other\a.exe").unwrap(),
            StartupValue {
                kind: 2,
                bytes: own.bytes.clone(),
            },
            altered,
            StartupValue {
                kind: 1,
                bytes: vec![0; 8193],
            },
        ] {
            for enable in [true, false] {
                let mut store = Mock {
                    current: Some(foreign.clone()),
                    ..Default::default()
                };
                assert!(change(&mut store, &own, enable).is_err());
                assert_eq!(store.writes, 0);
                assert_eq!(cleanup(&mut store, &own), Ok(Cleanup::ForeignPreserved));
            }
            let mut store = Mock {
                changed: Some(foreign.clone()),
                ..Default::default()
            };
            assert!(change(&mut store, &own, true).is_err());
            assert_eq!(store.writes, 0);
            assert_eq!(store.current, Some(foreign.clone()));
            let mut store = Mock {
                current: Some(own.clone()),
                changed: Some(foreign.clone()),
                ..Default::default()
            };
            assert!(cleanup(&mut store, &own).is_err());
            assert_eq!(store.writes, 0);
            assert_eq!(store.current, Some(foreign));
        }
    }
    #[test]
    fn immediate_write_errors_or_unconfirmed_outcomes_never_retry_or_rollback() {
        let own = expected();
        for fail in ["read", "write", "ack", "postread", "replace-after"] {
            let mut store = Mock {
                fail: Some(fail),
                ..Default::default()
            };
            assert!(change(&mut store, &own, true).is_err());
            assert!(store.writes <= 1);
            if fail == "ack" || fail == "postread" {
                assert_eq!(store.current, Some(own.clone()));
            }
            let mut store = Mock {
                current: Some(own.clone()),
                fail: Some(fail),
                ..Default::default()
            };
            assert!(cleanup(&mut store, &own).is_err());
            assert!(store.writes <= 1);
        }
    }
    #[test]
    fn exact_modes_and_successful_tray_are_required() {
        assert_eq!(launch(&[]), Launch::Manual);
        assert!(hide_initial(&["--background".into()], true));
        assert!(!hide_initial(&["--background".into()], false));
        assert!(uninstall_mode(&["--uninstall-owned-startup".into()]));
        for args in [
            vec!["--BACKGROUND".into()],
            vec!["--background".into(), "--unknown".into()],
            vec!["--uninstall-owned-startup".into(), "C:\\foreign.exe".into()],
        ] {
            assert!(!hide_initial(&args, true));
            assert!(!uninstall_mode(&args));
            assert_eq!(launch(&args), Launch::Invalid);
        }
    }
}
