//! Explicit selected-distro exports. A deterministic mountpoint and an intent
//! record precede mount changes. Linux flock serializes changes across app
//! processes; stat device/inode checks refuse foreign mounts. Never delete source
//! files, change a shell default or stop a distribution. Inspect is user-triggered
//! because invoking WSL may start that distribution. rdd owns the socket export.
use crate::{engine, process, runtime, wsl};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::path::{Path, PathBuf};

#[derive(Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct Export {
    name: String,
    source: String,
    signature: String,
    shared_source: String,
}
#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct Status {
    registration: String,
    export: Option<Export>,
    connected: bool,
    docker_command: String,
}

// A project handoff is a reviewed use of an existing export, never authority
// to connect/reconnect a different root. Bind the actual helper launch to the
// selected registration, while the Linux helper rechecks source/mount identity.
#[derive(Debug, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct ConnectionReview {
    #[serde(default)]
    pub(crate) registration: Option<String>,
    pub(crate) source: String,
    pub(crate) signature: String,
}
impl ConnectionReview {
    pub(crate) fn validate(&self) -> Result<(), String> {
        source_path(&self.source)?;
        if let Some(registration) = &self.registration {
            validate_registration(registration)?;
        }
        let parts: Vec<_> = self.signature.split(':').collect();
        if self.signature.len() > 64
            || parts.len() != 2
            || parts
                .iter()
                .any(|part| part.is_empty() || !part.bytes().all(|byte| byte.is_ascii_digit()))
        {
            return Err("Inspect the WSL project connection to review its source identity.".into());
        }
        Ok(())
    }
    fn verify_root(&self, source: &str, signature: &str) -> Result<(), String> {
        self.validate()?;
        if self.source != source || self.signature != signature {
            return Err("The selected WSL project connection changed after review. Inspect it again; no workflow was sent.".into());
        }
        Ok(())
    }
}
pub(crate) fn validate_registration(registration: &str) -> Result<(), String> {
    if registration.len() != 36
        || !registration.bytes().enumerate().all(|(at, byte)| {
            if [8, 13, 18, 23].contains(&at) {
                byte == b'-'
            } else {
                byte.is_ascii_hexdigit() && !byte.is_ascii_uppercase()
            }
        })
    {
        return Err("Select the reviewed ordinary WSL registration.".into());
    }
    Ok(())
}
#[cfg(windows)]
fn verify_registration(name: &str, registration: &str) -> Result<(), String> {
    let key = winreg::RegKey::predef(winreg::enums::HKEY_CURRENT_USER)
        .open_subkey(format!(
            "Software\\Microsoft\\Windows\\CurrentVersion\\Lxss\\{{{registration}}}"
        ))
        .map_err(|_| {
            "The reviewed WSL registration is absent; select the project distribution again."
        })?;
    let actual: String = key
        .get_value("DistributionName")
        .map_err(|_| "WSL registration name is unavailable")?;
    let version: u32 = key
        .get_value("Version")
        .map_err(|_| "WSL registration version is unavailable")?;
    if actual != name || version != 2 {
        return Err("The reviewed WSL registration changed; no workflow was sent.".into());
    }
    wsl::validate(name, &[name.to_owned()])
}
#[cfg(not(windows))]
fn verify_registration(_: &str, _: &str) -> Result<(), String> {
    Err("Reviewed WSL connections require the Windows controller.".into())
}

#[tauri::command]
pub(crate) async fn wsl_migration_project(
    name: String,
    registration: String,
) -> Result<Status, String> {
    // Read-only observation may start the selected distro. No connect/disconnect
    // action is exposed here; refuse registration drift on either side of read.
    validate_registration(&registration)?;
    verify_registration(&name, &registration)?;
    let result = wsl_project(name.clone(), "inspect".into(), String::new()).await?;
    verify_registration(&name, &registration)?;
    Ok(result)
}

fn linux_path(path: &Path) -> Result<String, String> {
    let normalized = runtime::cli_path(path)?;
    let text = normalized.to_str().ok_or("Tool path is not Unicode")?;
    Ok(format!(
        "/mnt/{}/{}",
        text[..1].to_ascii_lowercase(),
        text[3..].replace('\\', "/")
    ))
}
fn quote(text: &str) -> String {
    format!("'{}'", text.replace('\'', "'\"'\"'"))
}
fn source_path(source: &str) -> Result<(), String> {
    if !source.starts_with("/home/")
        || source.len() > 4096
        || source.chars().any(char::is_control)
        || source.split('/').any(|part| part == "..")
    {
        return Err(
            "Select an existing project directory below /home in this distribution.".into(),
        );
    }
    Ok(())
}

// Keep the same exclusive lock as connect/disconnect until native handoff. This
// observes the existing record without creating or changing a project export.
pub(crate) fn native_project_record(
    name: &str,
    source: &str,
    signature: &str,
) -> Result<std::fs::File, String> {
    use std::io::Read;
    ConnectionReview {
        registration: None,
        source: source.into(),
        signature: signature.into(),
    }
    .validate()?;
    let target = engine::target()?;
    let id = format!("{:x}", Sha256::digest(name.as_bytes()));
    let path = target
        .home()
        .join(".rdorbwin/wsl-projects")
        .join(format!("{id}.json"));
    let lock = native_record_handle(&path.with_extension("lock"), false).map_err(|_| {
        "The project is unavailable or another project action is running. Refresh its connection."
    })?;
    let mut bytes = Vec::new();
    let file = native_record_handle(&path, true)?;
    file.take(16385)
        .read_to_end(&mut bytes)
        .map_err(|_| "Cannot read the selected project record.")?;
    if bytes.len() > 16384 {
        return Err("Project record exceeds its size limit.".into());
    }
    let record: Export =
        serde_json::from_slice(&bytes).map_err(|_| "The selected project record is invalid.")?;
    verify_native_record(&record, name, source, signature, &id)?;
    Ok(lock)
}
fn native_record_handle(path: &Path, shared_read: bool) -> Result<std::fs::File, String> {
    let mut options = std::fs::OpenOptions::new();
    options.read(true);
    #[cfg(windows)]
    {
        use std::os::windows::fs::OpenOptionsExt;
        options
            .share_mode(if shared_read { 1 } else { 0 })
            .custom_flags(0x00200000);
    }
    #[cfg(not(windows))]
    let _ = shared_read;
    let file = options.open(path).map_err(|_| {
        "Refresh the selected project connection; its record is unavailable or being changed."
    })?;
    regular_native_record(&file)?;
    Ok(file)
}
fn regular_native_record(file: &std::fs::File) -> Result<(), String> {
    let metadata = file
        .metadata()
        .map_err(|_| "Cannot inspect the project record.")?;
    if !metadata.is_file() {
        return Err("The project record must be an ordinary file.".into());
    }
    #[cfg(windows)]
    {
        use std::os::windows::fs::MetadataExt;
        if metadata.file_attributes() & 0x400 != 0 {
            return Err("Project record reparse points are not supported.".into());
        }
    }
    Ok(())
}
fn verify_native_record(
    record: &Export,
    name: &str,
    source: &str,
    signature: &str,
    id: &str,
) -> Result<(), String> {
    if record.name != name
        || record.source != source
        || record.signature != signature
        || record.shared_source != format!("/mnt/wsl/orbwin/projects/{}", &id[..24])
    {
        return Err(
            "The project changed after inspection. Refresh its connection before opening it."
                .into(),
        );
    }
    Ok(())
}
async fn shell(name: &str, script: &str, args: &[&str]) -> Result<String, String> {
    let mut command = vec![
        "-d", name, "-u", "root", "--exec", "sh", "-c", script, "orbwin",
    ];
    command.extend_from_slice(args);
    process::read_command(&wsl::wsl_binary(), &command).await
}

const CHANGE: &str = r#"set -eu
action=$1; source=$2; signature=$3; target=$4
case "$target" in /mnt/wsl/orbwin/projects/[a-f0-9]*) ;; *) exit 70;; esac
if test "$action" = inspect; then
  if mountpoint -q "$target"; then
    test "$(stat -Lc '%d:%i' "$target")" = "$signature" || { echo 'Foreign mount at project export' >&2; exit 71; }
    echo connected
  else echo disconnected; fi
  exit 0
fi
test ! -L /mnt/wsl/orbwin
test -d /mnt/wsl/orbwin
exec 9>/mnt/wsl/orbwin/.projects.lock
flock -x -w 5 9
test ! -L /mnt/wsl/orbwin/projects
mkdir -p /mnt/wsl/orbwin/projects
test ! -L "$target"
if mountpoint -q "$target"; then
  test "$(stat -Lc '%d:%i' "$target")" = "$signature" || { echo 'Foreign mount at project export' >&2; exit 71; }
  if test "$action" = disconnect; then umount -- "$target"; echo disconnected; else echo connected; fi
elif test "$action" = connect; then
  test "$(stat -Lc '%d:%i' "$source")" = "$signature" || { echo 'Source changed; inspect before retrying' >&2; exit 72; }
  test -d "$source"
  mkdir -p "$target"
  mount --bind "$source" "$target"
  echo connected
else echo disconnected
fi"#;

pub(crate) fn tools() -> Result<(PathBuf, String), String> {
    let docker = runtime::linux_docker()?;
    let directory = docker
        .parent()
        .ok_or("Cached Linux toolset directory missing")?
        .to_path_buf();
    let linux = linux_path(&directory)?;
    Ok((directory, linux))
}

fn host_tool_directory(linux: &str) -> Result<PathBuf, String> {
    let bytes = linux.as_bytes();
    if !linux.starts_with("/mnt/")
        || bytes.len() < 8
        || !bytes[5].is_ascii_lowercase()
        || bytes[6] != b'/'
        || linux[7..]
            .chars()
            .any(|c| c.is_control() || c == '\\' || c == ':')
        || linux[7..]
            .split('/')
            .any(|part| part.is_empty() || part == "." || part == "..")
    {
        return Err(
            "Private Linux plugins must use one local Windows-drive tool directory.".into(),
        );
    }
    Ok(PathBuf::from(format!(
        "{}:\\{}",
        (bytes[5] as char).to_ascii_uppercase(),
        linux[7..].replace('/', "\\")
    )))
}

// Installed and development clients share the deterministic private cache.
// Prior public asset paths require explicit reconnect; a loose matching hash
// cannot keep that path immutable until Docker launches its plugin.
fn verify_private_plugins(private: &serde_json::Value) -> Result<(), String> {
    let choices = private["cliPluginsExtraDirs"]
        .as_array()
        .filter(|items| items.len() == 1)
        .ok_or("Private Linux plugins require one verified directory; reconnect explicitly.")?;
    let directory = host_tool_directory(
        choices[0]
            .as_str()
            .ok_or("Private Linux plugin path is invalid")?,
    )?
    .canonicalize()
    .map_err(|_| "Private Linux tool directory is unavailable; reconnect explicitly.")?;
    runtime::cli_path(&directory)?;
    let source = crate::runtime_cache::VerifiedCacheSource::for_linux_docker()?;
    let expected = crate::runtime_cache::expected_path(&source)?;
    if directory
        != expected
            .parent()
            .ok_or("Private Linux cache path is unavailable")?
    {
        return Err(
            "Private Linux tools use an older public path; reconnect explicitly before dispatch."
                .into(),
        );
    }
    let cached = crate::runtime_cache::prepare(source)?;
    if cached.parent() != Some(directory.as_path()) {
        return Err("Private Linux cache identity changed; reconnect explicitly.".into());
    }
    Ok(())
}

// The selected export remains authoritative. Preparation never mounts, starts an
// engine, rewrites a shell profile, or adopts a supplied internal export path.
// The Linux adapter checks source/export inode identity again as the default user
// immediately before dispatch; crossing a disconnect/replacement is an error.
fn verify_reviewed_daemon(request: &serde_json::Value, daemon: &str) -> Result<(), String> {
    if (request["kind"] == "mapping"
        || request
            .get("reviewedDaemon")
            .is_some_and(|value| !value.is_null()))
        && request["reviewedDaemon"].as_str() != Some(daemon)
    {
        return Err("The reviewed destination engine changed; refresh migration review.".into());
    }
    Ok(())
}

pub(crate) async fn prepare_workflow(
    name: &str,
    mut request: serde_json::Value,
) -> Result<(tokio::process::Command, Vec<u8>), String> {
    // Relative inputs from reopened group/service/build dialogs belong to the
    // reviewed root. Never reinterpret them using a later selected export.
    let connection: Option<ConnectionReview> = serde_json::from_value(
        request
            .get("connection")
            .cloned()
            .unwrap_or(serde_json::Value::Null),
    )
    .map_err(|_| "Invalid reviewed WSL project connection")?;
    if let Some(review) = &connection {
        review.validate()?;
        if let Some(registration) = &review.registration {
            verify_registration(name, registration)?;
        }
    }
    if let Some(object) = request.as_object_mut() {
        object.remove("connection");
    }
    let listing = process::read_command(&wsl::wsl_binary(), &["--list", "--quiet"]).await?;
    wsl::validate(name, &wsl::registered(&listing))?;
    let target = engine::target()?;
    engine::check(&target).await?;
    verify_reviewed_daemon(&request, target.daemon_id())?;
    let id = format!("{:x}", Sha256::digest(name.as_bytes()));
    let record_path = target
        .home()
        .join(".rdorbwin/wsl-projects")
        .join(format!("{id}.json"));
    use std::io::Read;
    let mut bytes = Vec::new();
    std::fs::File::open(&record_path)
        .map_err(|_| "Connect the intended WSL project before running its workflow.")?
        .take(16385)
        .read_to_end(&mut bytes)
        .map_err(|error| error.to_string())?;
    if bytes.len() > 16384 {
        return Err("Project ownership record exceeds its limit.".into());
    }
    let record: Export = serde_json::from_slice(&bytes).map_err(|error| error.to_string())?;
    let shared = format!("/mnt/wsl/orbwin/projects/{}", &id[..24]);
    source_path(&record.source)?;
    if let Some(review) = &connection {
        review.verify_root(&record.source, &record.signature)?;
    }
    if record.name != name
        || record.shared_source != shared
        || record.signature.len() > 64
        || !record
            .signature
            .bytes()
            .all(|byte| byte.is_ascii_digit() || byte == b':')
    {
        return Err("Project export ownership record differs.".into());
    }
    let (_, directory) = tokio::task::spawn_blocking(tools)
        .await
        .map_err(|error| error.to_string())??;
    let config = target.home().join(".rdorbwin/wsl-docker");
    let mut config_bytes = Vec::new();
    std::fs::File::open(config.join("config.json"))
        .map_err(|_| "Reconnect the selected project to prepare its private Linux client.")?
        .take((1 << 20) + 1)
        .read_to_end(&mut config_bytes)
        .map_err(|error| error.to_string())?;
    if config_bytes.len() > 1 << 20 {
        return Err("Private Linux client config exceeds its limit; it was preserved.".into());
    }
    let private: serde_json::Value =
        serde_json::from_slice(&config_bytes).map_err(|error| error.to_string())?;
    // A cancelled prepare may leave a bounded read-only hash worker. No Linux
    // workflow is spawned before its successful join; IPC remains responsive.
    tokio::task::spawn_blocking(move || verify_private_plugins(&private))
        .await
        .map_err(|error| error.to_string())??;
    let helper = runtime::project_helper()?;
    let credential = runtime::credentials()?;
    request["source"] = record.source.into();
    request["shared"] = shared.into();
    request["signature"] = record.signature.into();
    request["daemon"] = target.daemon_id().into();
    request["docker"] = format!("{directory}/docker").into();
    request["config"] = linux_path(&config)?.into();
    request["helperDirectory"] = linux_path(
        credential
            .parent()
            .ok_or("Credential helper directory is unavailable")?,
    )?
    .into();
    request["distribution"] = name.into();
    let mut command = tokio::process::Command::new(wsl::wsl_binary());
    if let Some(registration) = connection
        .as_ref()
        .and_then(|review| review.registration.as_ref())
    {
        verify_registration(name, registration)?;
        command.args([
            "--distribution-id",
            &format!("{{{registration}}}"),
            "--exec",
            &linux_path(&helper)?,
        ]);
    } else {
        command.args(["--distribution", name, "--exec", &linux_path(&helper)?]);
    }
    Ok((
        command,
        serde_json::to_vec(&request).map_err(|error| error.to_string())?,
    ))
}

#[tauri::command]
pub(crate) async fn wsl_project(
    name: String,
    action: String,
    source: String,
) -> Result<Status, String> {
    if !["inspect", "connect", "disconnect"].contains(&action.as_str()) {
        return Err("Unknown WSL project action".into());
    }
    let listing = process::read_command(&wsl::wsl_binary(), &["--list", "--quiet"]).await?;
    wsl::validate(&name, &wsl::registered(&listing))?;
    let registration = crate::wsl_tools::registration_for_name(&name)?;
    let target = engine::target()?;
    let id = format!("{:x}", Sha256::digest(name.as_bytes()));
    let shared = format!("/mnt/wsl/orbwin/projects/{}", &id[..24]);
    let record_path = target
        .home()
        .join(".rdorbwin/wsl-projects")
        .join(format!("{id}.json"));
    std::fs::create_dir_all(record_path.parent().unwrap()).map_err(|e| e.to_string())?;
    let mut lock_options = std::fs::OpenOptions::new();
    lock_options.create(true).truncate(false).write(true);
    #[cfg(windows)]
    {
        use std::os::windows::fs::OpenOptionsExt;
        lock_options.share_mode(0);
    }
    let _lock = lock_options
        .open(record_path.with_extension("lock"))
        .map_err(|e| format!("Another project operation may be running: {e}"))?;
    let mut record: Option<Export> = if record_path.exists() {
        let bytes = std::fs::read(&record_path).map_err(|e| e.to_string())?;
        if bytes.len() > 16384 {
            return Err("Project record exceeds its size limit".into());
        }
        Some(serde_json::from_slice(&bytes).map_err(|e| e.to_string())?)
    } else {
        None
    };
    if let Some(record) = &record {
        if record.name != name
            || record.shared_source != shared
            || record.signature.len() > 64
            || !record
                .signature
                .bytes()
                .all(|c| c.is_ascii_digit() || c == b':')
        {
            return Err("Project export ownership record is invalid".into());
        }
        source_path(&record.source)?;
    }
    if action == "connect" {
        source_path(&source)?;
        engine::check(&target).await?;
        target.rdd().await?;
        let prepared = shell(&name, r#"set -eu; source=$(readlink -f -- "$1"); test -d "$source"; printf '%s\n' "$source"; stat -Lc '%d:%i' "$source""#, &[&source]).await?;
        let lines: Vec<_> = prepared.lines().collect();
        if lines.len() != 2 {
            return Err("Could not resolve the selected project directory".into());
        }
        source_path(lines[0])?;
        if let Some(previous) = &record {
            if previous.source != lines[0] {
                return Err("Disconnect the previously selected project before sharing a different directory.".into());
            }
            if previous.signature != lines[1]
                && shell(
                    &name,
                    CHANGE,
                    &["inspect", &previous.source, &previous.signature, &shared],
                )
                .await?
                    == "connected"
            {
                return Err("The source directory was replaced while its previous export is mounted. Disconnect that export before reconnecting.".into());
            }
        }
        let next = Export {
            name: name.clone(),
            source: lines[0].into(),
            signature: lines[1].into(),
            shared_source: shared.clone(),
        };
        std::fs::create_dir_all(record_path.parent().unwrap()).map_err(|e| e.to_string())?;
        runtime::write_json(
            &record_path,
            &serde_json::to_value(&next).map_err(|e| e.to_string())?,
        )?;
        record = Some(next);
    }
    let connected = if let Some(record) = &record {
        shell(
            &name,
            CHANGE,
            &[&action, &record.source, &record.signature, &shared],
        )
        .await?
            == "connected"
    } else {
        false
    };
    if action == "disconnect" && !connected {
        if record_path.exists() {
            std::fs::remove_file(&record_path).map_err(|e| e.to_string())?;
        }
        record = None;
    }
    if action == "disconnect" {
        return Ok(Status {
            registration,
            export: record,
            connected,
            docker_command: String::new(),
        });
    }
    let (_, directory) = tokio::task::spawn_blocking(tools)
        .await
        .map_err(|e| e.to_string())??;
    let config = target.home().join(".rdorbwin/wsl-docker");
    let helper = runtime::credentials()?;
    let helper_directory = linux_path(
        helper
            .parent()
            .ok_or("Credential helper directory is missing")?,
    )?;
    let command = format!("env -u DOCKER_CONTEXT -u DOCKER_TLS_VERIFY -u DOCKER_CERT_PATH -u BUILDKIT_HOST PATH={}:\"$PATH\" BUILDX_BUILDER=default BUILDX_CONFIG={} {} --config {} --host unix:///mnt/wsl/orbwin/docker.sock", quote(&helper_directory), quote(&linux_path(&config.join("buildx"))?), quote(&format!("{directory}/docker")), quote(&linux_path(&config)?));
    if action == "connect" {
        // The native helper list is authoritative; do not nest its ownership
        // guard with the independent legacy WSL metadata lock.
        let keys = crate::registry::stored_registry_keys().await?;
        let home = target.home().to_path_buf();
        let plugin_directory = directory.clone();
        tokio::task::spawn_blocking(move || {
            crate::wsl_clients::configure_clients(&home, &plugin_directory, &keys)
        })
        .await
        .map_err(|_| {
            "Private WSL metadata transaction did not complete; inspect before retrying."
        })??;
        let docker = format!("{directory}/docker");
        let config = linux_path(&config)?;
        let actual = process::read_command(
            &wsl::wsl_binary(),
            &[
                "-d",
                &name,
                "--exec",
                "env",
                "-u",
                "DOCKER_CONTEXT",
                "-u",
                "DOCKER_TLS_VERIFY",
                "-u",
                "DOCKER_CERT_PATH",
                &docker,
                "--config",
                &config,
                "--host",
                "unix:///mnt/wsl/orbwin/docker.sock",
                "info",
                "--format",
                "{{.ID}}",
            ],
        )
        .await?;
        if actual != target.daemon_id() {
            return Err("WSL socket does not match the owned daemon; the project export was retained for inspection.".into());
        }
    }
    if crate::wsl_tools::registration_for_name(&name)? != registration {
        return Err(
            "The selected WSL registration changed during inspection. Refresh it again.".into(),
        );
    }
    Ok(Status {
        registration,
        export: record,
        connected,
        docker_command: command,
    })
}

#[cfg(all(test, windows))]
#[path = "wsl_native_record_tests.rs"]
mod native_record_tests;

#[cfg(test)]
mod draft_target_tests {
    use super::*;
    #[test]
    fn reviewed_target_is_exact_and_ordinary_legacy_requests_remain_supported() {
        for value in [
            serde_json::json!({"kind":"compose"}),
            serde_json::json!({"kind":"compose","reviewedDaemon":null}),
        ] {
            assert!(verify_reviewed_daemon(&value, "owned").is_ok());
        }
        assert!(verify_reviewed_daemon(
            &serde_json::json!({"kind":"compose","reviewedDaemon":"owned"}),
            "owned"
        )
        .is_ok());
        for value in [
            serde_json::json!({"kind":"compose","reviewedDaemon":"foreign"}),
            serde_json::json!({"kind":"compose","reviewedDaemon":42}),
            serde_json::json!({"kind":"mapping"}),
        ] {
            assert!(verify_reviewed_daemon(&value, "owned").is_err());
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn native_handoff_refuses_changed_project_record() {
        let id = format!("{:x}", Sha256::digest(b"Ubuntu"));
        let mut record = Export {
            name: "Ubuntu".into(),
            source: "/home/user/project".into(),
            signature: "12:34".into(),
            shared_source: format!("/mnt/wsl/orbwin/projects/{}", &id[..24]),
        };
        assert!(
            verify_native_record(&record, "Ubuntu", "/home/user/project", "12:34", &id).is_ok()
        );
        assert!(
            verify_native_record(&record, "Other", "/home/user/project", "12:34", &id).is_err()
        );
        assert!(
            verify_native_record(&record, "Ubuntu", "/home/user/new-project", "12:34", &id)
                .is_err()
        );
        assert!(
            verify_native_record(&record, "Ubuntu", "/home/user/project", "12:35", &id).is_err()
        );
        record.shared_source.push_str("-replaced");
        assert!(
            verify_native_record(&record, "Ubuntu", "/home/user/project", "12:34", &id).is_err()
        );
    }
    #[test]
    fn project_scope_and_shell_arguments() {
        for path in ["/", "/etc", "/home/a/../b", "/home/a\ncmd", "~/project"] {
            assert!(source_path(path).is_err());
        }
        assert!(source_path("/home/user/project with spaces/é").is_ok());
        assert_eq!(quote("a'b"), "'a'\"'\"'b'");
    }

    #[test]
    fn migration_connection_requires_exact_reviewed_root_and_registration() {
        let review = ConnectionReview {
            registration: Some("00000000-0000-4000-8000-000000000001".into()),
            source: "/home/user/project 例".into(),
            signature: "1:2".into(),
        };
        assert!(review.verify_root("/home/user/project 例", "1:2").is_ok());
        assert!(review
            .verify_root("/home/user/project 例-other", "1:2")
            .is_err());
        assert!(review.verify_root("/home/user/project 例", "1:3").is_err());
        for registration in ["Ubuntu", "../other", "00000000-0000-4000-8000-00000000000Z"] {
            assert!(ConnectionReview {
                registration: Some(registration.into()),
                source: review.source.clone(),
                signature: review.signature.clone(),
            }
            .validate()
            .is_err());
        }
        assert!(ConnectionReview {
            registration: review.registration,
            source: "/etc".into(),
            signature: review.signature,
        }
        .validate()
        .is_err());
    }

    #[test]
    fn shared_private_plugins_refuse_unpinned_and_ambiguous_paths() {
        for path in [
            "/mnt/wsl/other",
            "C:\\tools",
            "/mnt/c/../other",
            "/mnt/c/tools\\other",
            "/mnt/c/tools/./other",
            "/mnt/c/tools//other",
            "/mnt/c/tools:other",
            "//server/tools",
        ] {
            assert!(host_tool_directory(path).is_err(), "{path}");
        }
        assert!(verify_private_plugins(
            &serde_json::json!({"cliPluginsExtraDirs":["/mnt/c/one","/mnt/c/two"]})
        )
        .is_err());
        assert!(verify_private_plugins(&serde_json::json!({"cliPluginsExtraDirs":null})).is_err());
        #[cfg(windows)]
        {
            let stamp = std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos();
            let directory = std::env::temp_dir().join(format!("orbwin-plugin-refusal-{stamp}"));
            std::fs::create_dir(&directory).unwrap();
            std::fs::write(directory.join("docker"), b"changed public tool").unwrap();
            let result = verify_private_plugins(
                &serde_json::json!({"cliPluginsExtraDirs":[linux_path(&directory).unwrap()]}),
            );
            std::fs::remove_file(directory.join("docker")).unwrap();
            std::fs::remove_dir(&directory).unwrap();
            assert!(result.err().unwrap().contains("older public path"));
        }
    }

    #[tokio::test]
    #[ignore = "explicit real Ubuntu project export and pinned Linux client qualification"]
    async fn owned_wsl_project() {
        assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
        let name = "Ubuntu";
        let binary = wsl::wsl_binary();
        // Keep a real WSL client session open, as required by the supported
        // project workflow. Closing stdin releases this test-only client.
        let mut shell_client = std::process::Command::new(&binary);
        shell_client
            .args(["-d", name, "--exec", "cat"])
            .stdin(std::process::Stdio::piped())
            .stdout(std::process::Stdio::null())
            .stderr(std::process::Stdio::null());
        #[cfg(windows)]
        {
            use std::os::windows::process::CommandExt;
            shell_client.creation_flags(0x08000000);
        }
        let mut shell_client = shell_client.spawn().unwrap();
        let previous = wsl_project(name.into(), "inspect".into(), String::new())
            .await
            .unwrap();
        let source = if let Some(export) = previous.export {
            assert!(
                export.source.starts_with("/home/vanyr/.orbwin-proof."),
                "Refuse to replace a non-qualification export"
            );
            export.source
        } else {
            process::read_command(
                &binary,
                &[
                    "-d",
                    name,
                    "--exec",
                    "sh",
                    "-c",
                    "set -eu; mktemp -d \"$HOME/.orbwin-proof.XXXXXX\"",
                ],
            )
            .await
            .unwrap()
        };
        source_path(&source).unwrap();
        let write = |script: &'static str| {
            let source = source.clone();
            async move {
                process::read_command(
                    &wsl::wsl_binary(),
                    &["-d", name, "--exec", "sh", "-c", script, "orbwin", &source],
                )
                .await
            }
        };
        write(r#"set -eu; printf 'host-visible\n' > "$1/marker"; printf 'FROM alpine:3.21\nRUN printf build-ok > /build-proof\n' > "$1/Dockerfile""#).await.unwrap();
        let connected = wsl_project(name.into(), "connect".into(), source.clone())
            .await
            .unwrap();
        assert!(connected.connected);
        let share = &connected.export.as_ref().unwrap().shared_source;
        let (_, directory) = tools().unwrap();
        let docker = format!("{directory}/docker");
        let config = linux_path(
            &engine::target()
                .unwrap()
                .home()
                .join(".rdorbwin/wsl-docker"),
        )
        .unwrap();
        let run = |args: Vec<String>| {
            let docker = docker.clone();
            let config = config.clone();
            async move {
                let mut command = vec![
                    "-d",
                    name,
                    "--exec",
                    "env",
                    "-u",
                    "DOCKER_CONTEXT",
                    "-u",
                    "DOCKER_TLS_VERIFY",
                    "-u",
                    "DOCKER_CERT_PATH",
                    "-u",
                    "BUILDKIT_HOST",
                    "BUILDX_BUILDER=default",
                    &docker,
                    "--config",
                    &config,
                    "--host",
                    "unix:///mnt/wsl/orbwin/docker.sock",
                ];
                command.extend(args.iter().map(String::as_str));
                process::read_command(&wsl::wsl_binary(), &command).await
            }
        };
        let args = |values: &[&str]| {
            values
                .iter()
                .map(|value| (*value).to_string())
                .collect::<Vec<_>>()
        };
        let compose_file = format!("{source}/compose.json");
        let previous_projects = run(args(&[
            "ps",
            "-a",
            "--filter",
            &format!("label=com.docker.compose.project.working_dir={source}"),
            "--format",
            "{{.Label \"com.docker.compose.project\"}}",
        ]))
        .await
        .unwrap();
        for previous in previous_projects.lines().filter(|name| {
            name.starts_with("orbwin-wsl-") && name[11..].bytes().all(|byte| byte.is_ascii_digit())
        }) {
            run(args(&[
                "compose",
                "--project-name",
                previous,
                "--file",
                &compose_file,
                "down",
                "--timeout=1",
            ]))
            .await
            .unwrap();
        }
        assert!(run(args(&["compose", "version"]))
            .await
            .unwrap()
            .contains("5.6.1"));
        assert!(run(args(&["buildx", "version"]))
            .await
            .unwrap()
            .contains("0.38.1"));
        let mounted = run(args(&[
            "run",
            "--rm",
            "--mount",
            &format!("type=bind,source={share},target=/project"),
            "alpine:3.21",
            "sh",
            "-c",
            "cat /project/marker; printf container-visible > /project/returned",
        ]))
        .await
        .unwrap();
        assert!(mounted.contains("host-visible"));
        let project = format!("orbwin-wsl-{}", std::process::id());
        let manifest = serde_json::json!({"services":{"proof":{"image":"alpine:3.21", "command":["sh","-c","printf compose-visible > /project/from-compose; sleep 300"],"volumes":[{"type":"bind","source":share,"target":"/project"}],"healthcheck":{"test":["CMD","test","-f","/project/from-compose"],"interval":"1s","timeout":"1s","retries":5}}}}).to_string();
        process::read_command(
            &binary,
            &[
                "-d",
                name,
                "--exec",
                "sh",
                "-c",
                r#"printf '%s' "$2" > "$1/compose.json""#,
                "orbwin",
                &source,
                &manifest,
            ],
        )
        .await
        .unwrap();
        let compose_up = run(args(&[
            "compose",
            "--project-name",
            &project,
            "--file",
            &compose_file,
            "up",
            "-d",
            "--wait",
            "--wait-timeout",
            "7",
        ]))
        .await;
        let compose_down = run(args(&[
            "compose",
            "--project-name",
            &project,
            "--file",
            &compose_file,
            "down",
            "--timeout=1",
        ]))
        .await;
        compose_up.unwrap();
        compose_down.unwrap();
        assert_eq!(
            write(r#"cat "$1/from-compose""#).await.unwrap(),
            "compose-visible"
        );
        assert_eq!(
            write(r#"cat "$1/returned""#).await.unwrap(),
            "container-visible"
        );
        let tag = format!("orbwin-wsl-proof:{}", std::process::id());
        let build = run(args(&["buildx", "build", "--load", "--tag", &tag, &source])).await;
        if let Err(error) = build {
            let _ = wsl_project(name.into(), "disconnect".into(), String::new()).await;
            panic!("Linux build failed: {error}");
        }
        assert_eq!(
            run(args(&["run", "--rm", &tag, "cat", "/build-proof"]))
                .await
                .unwrap(),
            "build-ok"
        );
        run(args(&["image", "rm", &tag])).await.unwrap();
        // A service restart must bind the new dockerd inode, not a stale socket.
        engine::target().unwrap().rdd().await.unwrap();
        process::read_command(
            &binary,
            &[
                "-d",
                "lima-orbwin",
                "-u",
                "root",
                "--exec",
                "systemctl",
                "restart",
                "docker.service",
            ],
        )
        .await
        .unwrap();
        let id = run(args(&["info", "--format", "{{.ID}}"])).await.unwrap();
        assert_eq!(id, engine::target().unwrap().daemon_id());
        // Repeat connect must not stack mounts. Replacing a source must retain
        // the previous intent so its original mount remains disconnectable.
        assert!(
            wsl_project(name.into(), "connect".into(), source.clone())
                .await
                .unwrap()
                .connected
        );
        write(r#"set -eu; mv -- "$1" "$1.retained"; mkdir -- "$1""#)
            .await
            .unwrap();
        let replacement = wsl_project(name.into(), "connect".into(), source.clone()).await;
        assert!(replacement.err().unwrap().contains("replaced"));
        assert!(
            wsl_project(name.into(), "inspect".into(), String::new())
                .await
                .unwrap()
                .connected
        );
        assert!(
            !wsl_project(name.into(), "disconnect".into(), String::new())
                .await
                .unwrap()
                .connected
        );
        write(r#"set -eu; rmdir -- "$1"; mv -- "$1.retained" "$1"; test -f "$1/marker"; test -f "$1/returned"; rm -- "$1/marker" "$1/returned" "$1/Dockerfile" "$1/from-compose" "$1/compose.json"; rmdir -- "$1""#).await.unwrap();
        drop(shell_client.stdin.take());
        shell_client.wait().unwrap();
        // Reproduce the shared-bind disappearance case after a client distro
        // becomes idle. The direct dockerd listener must remain usable.
        tokio::time::sleep(std::time::Duration::from_secs(12)).await;
        assert_eq!(
            run(args(&["info", "--format", "{{.ID}}"])).await.unwrap(),
            engine::target().unwrap().daemon_id()
        );
    }
}
