//! UI lifetime only. rdd owns backend/workload lifetime independently.
//! Startup preference authority is one exact HKCU Run value, never a second file.
use crate::desktop_policy::{self as policy, StartupValue};
use serde::Serialize;
use std::sync::Mutex;
use tauri::Manager;

#[derive(Clone, Default)]
struct TrayObservation {
    available: bool,
    issue: Option<String>,
}
#[derive(Default)]
pub(crate) struct Desktop {
    tray: Mutex<TrayObservation>,
}
#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct Preferences {
    enabled: bool,
    tray_available: bool,
    tray_issue: Option<String>,
}
static STARTUP_LOCK: Mutex<()> = Mutex::new(());

fn set_tray_issue(app: &tauri::AppHandle, issue: String) {
    let desktop = app.state::<Desktop>();
    if let Ok(mut state) = desktop.tray.lock() {
        state.issue = Some(issue);
    };
}
fn open(app: &tauri::AppHandle) {
    let result = (|| {
        let window = app
            .get_webview_window("main")
            .ok_or("Main OrbWin window is unavailable")?;
        window.show().map_err(|_| "Cannot show the OrbWin window")?;
        window
            .unminimize()
            .map_err(|_| "Cannot restore the OrbWin window")?;
        window
            .set_focus()
            .map_err(|_| "Cannot focus the OrbWin window")
    })();
    if let Err(error) = result {
        set_tray_issue(app, error.into());
    }
}
pub(crate) fn setup(app: &mut tauri::App) -> Result<(), Box<dyn std::error::Error>> {
    #[cfg(windows)]
    let result = (|| -> Result<(), String> {
        use tauri::{
            menu::{Menu, MenuItem},
            tray::TrayIconBuilder,
        };
        let open = MenuItem::with_id(app, "orbwin-open", "Open OrbWin", true, None::<&str>)
            .map_err(|e| e.to_string())?;
        let quit = MenuItem::with_id(app, "orbwin-quit-ui", "Quit UI", true, None::<&str>)
            .map_err(|e| e.to_string())?;
        let menu = Menu::with_items(app, &[&open, &quit]).map_err(|e| e.to_string())?;
        let icon = app
            .default_window_icon()
            .ok_or("Application icon unavailable; tray was not created")?
            .clone();
        TrayIconBuilder::with_id("orbwin-desktop")
            .icon(icon)
            .tooltip("OrbWin · Quit UI leaves workloads running")
            .menu(&menu)
            .on_menu_event(|app, event| match event.id().as_ref() {
                "orbwin-open" => self::open(app),
                "orbwin-quit-ui" => app.exit(0),
                _ => (),
            })
            .build(app)
            .map_err(|e| e.to_string())?;
        Ok(())
    })();
    #[cfg(not(windows))]
    let result: Result<(), String> = Err("The desktop tray requires Windows.".into());
    let available = result.is_ok();
    {
        let desktop = app.state::<Desktop>();
        let mut tray = desktop
            .tray
            .lock()
            .map_err(|_| "Desktop tray state unavailable")?;
        tray.available = available;
        tray.issue = result.err().map(|_| "Tray creation failed. OrbWin remains visible; sign-in launch is unavailable until a working tray is created.".into());
    }
    let args: Vec<String> = std::env::args_os()
        .skip(1)
        .map(|arg| arg.to_string_lossy().into_owned())
        .collect();
    if policy::hide_initial(&args, available) {
        if let Some(window) = app.get_webview_window("main") {
            if window.hide().is_err() {
                open(app.handle());
                set_tray_issue(
                    app.handle(),
                    "Could not hide the initial window; OrbWin remains visible.".into(),
                );
            }
        }
    } else if policy::launch(&args) != policy::Launch::Manual || !available {
        open(app.handle());
        if policy::launch(&args) == policy::Launch::Invalid {
            set_tray_issue(
                app.handle(),
                "Unrecognized launch arguments were ignored; OrbWin remains visible.".into(),
            );
        }
    }
    Ok(())
}
fn own_value() -> Result<StartupValue, String> {
    let executable =
        std::env::current_exe().map_err(|_| "Cannot resolve this application executable")?;
    let executable = executable
        .canonicalize()
        .map_err(|_| "Cannot verify this application executable")?;
    let executable = crate::runtime::cli_path(&executable)?;
    policy::expected_value(
        executable
            .to_str()
            .ok_or("Application path is not Unicode")?,
    )
}

#[cfg(windows)]
mod startup {
    #[cfg(test)]
    mod qualification {
        include!("desktop_registry_qualification.rs");
    }
    use super::*;
    use policy::StartupStore;
    include!("desktop_startup_lock.rs");
    use winreg::{
        enums::{HKEY_CURRENT_USER, KEY_QUERY_VALUE, KEY_SET_VALUE, REG_SZ},
        RegKey, RegValue,
    };
    const KEY: &str = r"Software\Microsoft\Windows\CurrentVersion\Run";
    const VALUE: &str = "OrbWin";
    // Fixed leaf and fixed maximum allocation, including malformed foreign values.
    fn read(key: &RegKey) -> Result<Option<StartupValue>, String> {
        use windows_sys::Win32::{
            Foundation::{ERROR_FILE_NOT_FOUND, ERROR_SUCCESS},
            System::Registry::RegQueryValueExW,
        };
        let name: Vec<u16> = VALUE.encode_utf16().chain(Some(0)).collect();
        let mut bytes = vec![0u8; 8192];
        let mut len = bytes.len() as u32;
        let mut kind = 0;
        let result = unsafe {
            RegQueryValueExW(
                key.raw_handle(),
                name.as_ptr(),
                std::ptr::null(),
                &mut kind,
                bytes.as_mut_ptr(),
                &mut len,
            )
        };
        if result == ERROR_FILE_NOT_FOUND {
            return Ok(None);
        }
        if result != ERROR_SUCCESS || len as usize > bytes.len() {
            return Err("Cannot read the bounded OrbWin sign-in entry.".into());
        }
        bytes.truncate(len as usize);
        Ok(Some(StartupValue { kind, bytes }))
    }
    struct RegistryStore {
        key: RegKey,
    }
    impl StartupStore for RegistryStore {
        fn read(&mut self) -> Result<Option<StartupValue>, String> {
            read(&self.key)
        }
        fn write(&mut self, value: Option<&StartupValue>) -> Result<(), String> {
            match value {
                Some(value) => self.key.set_raw_value(VALUE, &RegValue { vtype: REG_SZ, bytes: value.bytes.clone() }),
                None => self.key.delete_value(VALUE),
            }.map_err(|_| "Sign-in registration write failed or its result is uncertain. Read the setting again; no automatic retry or rollback was made.".into())
        }
    }
    fn current() -> Result<Option<StartupValue>, String> {
        let key =
            match RegKey::predef(HKEY_CURRENT_USER).open_subkey_with_flags(KEY, KEY_QUERY_VALUE) {
                Ok(key) => key,
                Err(e) if e.kind() == std::io::ErrorKind::NotFound => return Ok(None),
                Err(_) => return Err("Windows denied reading sign-in registration.".into()),
            };
        read(&key)
    }
    pub(super) fn enabled(expected: &StartupValue) -> Result<bool, String> {
        let _lock = startup_lock()?;
        policy::is_enabled(current()?.as_ref(), expected)
    }
    fn store(create: bool) -> Result<Option<RegistryStore>, String> {
        let hkcu = RegKey::predef(HKEY_CURRENT_USER);
        let flags = KEY_QUERY_VALUE | KEY_SET_VALUE;
        let key = match hkcu.open_subkey_with_flags(KEY, flags) {
            Ok(key) => key,
            Err(e) if e.kind() == std::io::ErrorKind::NotFound && !create => return Ok(None),
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
                hkcu.create_subkey_with_flags(KEY, flags)
                    .map_err(|_| "Cannot create the per-user sign-in key")?
                    .0
            }
            Err(_) => return Err("Windows denied changing sign-in registration.".into()),
        };
        Ok(Some(RegistryStore { key }))
    }
    pub(super) fn set(expected: &StartupValue, enabled: bool) -> Result<(), String> {
        let _lock = startup_lock()?;
        // Do not require SET_VALUE for a no-op or foreign-value refusal. The
        // write-capable adapter below rereads ownership before any mutation.
        if policy::is_enabled(current()?.as_ref(), expected)? == enabled {
            return Ok(());
        }
        if let Some(mut store) = store(enabled)? {
            policy::change(&mut store, expected, enabled)?;
        }
        Ok(())
    }
    pub(super) fn cleanup(expected: &StartupValue) -> Result<policy::Cleanup, String> {
        let _lock = startup_lock()?;
        match policy::is_enabled(current()?.as_ref(), expected) {
            Err(_) => return Ok(policy::Cleanup::ForeignPreserved),
            Ok(false) => return Ok(policy::Cleanup::RemovedOrAbsent),
            Ok(true) => {}
        }
        match store(false)? {
            Some(mut store) => policy::cleanup(&mut store, expected),
            None => Ok(policy::Cleanup::RemovedOrAbsent),
        }
    }
}
#[cfg(not(windows))]
mod startup {
    use super::*;
    pub(super) fn enabled(_: &StartupValue) -> Result<bool, String> {
        Err("Sign-in registration requires Windows.".into())
    }
    pub(super) fn set(_: &StartupValue, _: bool) -> Result<(), String> {
        Err("Sign-in registration requires Windows.".into())
    }
    pub(super) fn cleanup(_: &StartupValue) -> Result<policy::Cleanup, String> {
        Err("Sign-in cleanup requires Windows.".into())
    }
}
fn preferences(tray: TrayObservation) -> Result<Preferences, String> {
    Ok(Preferences {
        enabled: startup::enabled(&own_value()?)?,
        tray_available: tray.available,
        tray_issue: tray.issue,
    })
}
#[tauri::command]
pub(crate) async fn desktop_preferences(
    state: tauri::State<'_, Desktop>,
) -> Result<Preferences, String> {
    let tray = state
        .tray
        .lock()
        .map_err(|_| "Desktop tray state unavailable")?
        .clone();
    tokio::task::spawn_blocking(move || {
        let _guard = STARTUP_LOCK
            .lock()
            .map_err(|_| "Sign-in registration is busy")?;
        preferences(tray)
    })
    .await
    .map_err(|_| "Sign-in read did not complete")?
}
#[tauri::command]
pub(crate) async fn desktop_set_startup(
    state: tauri::State<'_, Desktop>,
    enabled: bool,
) -> Result<Preferences, String> {
    let tray = state
        .tray
        .lock()
        .map_err(|_| "Desktop tray state unavailable")?
        .clone();
    if enabled && !tray.available {
        return Err("A working tray is required before enabling sign-in launch.".into());
    }
    tokio::task::spawn_blocking(move || {
        let _guard = STARTUP_LOCK.lock().map_err(|_| "Sign-in registration is busy")?;
        startup::set(&own_value()?, enabled)?;
        preferences(tray).map_err(|_| "Sign-in registration may have changed, but its final read failed. Read it again before retrying.".to_string())
    }).await.map_err(|_| "Sign-in request did not complete. Its result may be unknown; read the setting again.")?
}

/// Closed maintenance operation, dispatched before Tauri/WebView/tray setup.
/// Direct invocation is equivalent to removing only this copy's exact owned
/// preference. No privilege increase, arbitrary path/key/args, or backend call.
/// 0: removed/absent, 10: foreign preserved, 21: failure/outcome unknown.
pub(crate) fn early_maintenance() -> Option<i32> {
    let args: Vec<String> = std::env::args_os()
        .skip(1)
        .map(|arg| arg.to_string_lossy().into_owned())
        .collect();
    if !policy::uninstall_mode(&args) {
        return None;
    }
    let result = (|| {
        let _guard = STARTUP_LOCK
            .lock()
            .map_err(|_| "Sign-in registration is busy")?;
        startup::cleanup(&own_value()?)
    })();
    Some(match result {
        Ok(policy::Cleanup::RemovedOrAbsent) => 0,
        Ok(policy::Cleanup::ForeignPreserved) => 10,
        Err(_) => 21,
    })
}
