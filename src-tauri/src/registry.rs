//! Docker authenticates registries; a pinned helper owns only OrbWin vault keys.
//! Secrets travel through stdin and never enter arguments, debug text or output.
//! Login output is untrusted (a registry can echo credentials), so discard it.
//! Cancellation is uncertain: authentication/storage may already have committed.
use crate::{
    engine,
    operations::{Lease, Outcome, Output, Request},
    runtime,
};
use serde::Deserialize;
use std::{
    ffi::OsString,
    fmt,
    path::Path,
    sync::{Arc, Mutex},
    time::Duration,
};
use tauri::ipc::Channel;
use tokio::sync::oneshot;

#[derive(Deserialize)]
#[serde(transparent)]
pub struct Secret(pub String);
impl fmt::Debug for Secret {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        formatter.write_str("<redacted>")
    }
}

pub(crate) fn login_target(server: &str) -> Result<(String, String), String> {
    let server = server.to_ascii_lowercase();
    if matches!(
        server.as_str(),
        "" | "docker.io" | "index.docker.io" | "https://index.docker.io/v1/"
    ) {
        return Ok(("docker.io".into(), "https://index.docker.io/v1/".into()));
    }
    if !server.is_ascii()
        || server.len() > 253
        || server.chars().any(char::is_control)
        || server.trim() != server
    {
        return Err("Enter a registry host with an optional port, or docker.io.".into());
    }
    let parts: Vec<_> = server.split(':').collect();
    if parts.len() > 2
        || parts[0].split('.').any(|label| {
            label.is_empty()
                || label.len() > 63
                || !label.as_bytes()[0].is_ascii_alphanumeric()
                || !label.as_bytes()[label.len() - 1].is_ascii_alphanumeric()
                || !label
                    .bytes()
                    .all(|c| c.is_ascii_alphanumeric() || c == b'-')
        })
    {
        return Err("Enter a registry host with an optional port; paths and URL credentials are not supported.".into());
    }
    if parts.len() == 2 && !parts[1].parse::<u16>().is_ok_and(|port| port > 0) {
        return Err("Registry port must be 1–65535.".into());
    }
    Ok((server.clone(), server))
}
pub(crate) fn validate_login(server: &str, username: &str, secret: &Secret) -> Result<(), String> {
    login_target(server)?;
    if username.is_empty() || username.len() > 512 || username.chars().any(char::is_control) {
        return Err("Enter a username without control characters (maximum 512 bytes).".into());
    }
    if secret.0.is_empty() || secret.0.len() > 2560 || secret.0.contains(['\r', '\n', '\0']) {
        return Err("Enter a password or token of 1–2560 bytes without line breaks.".into());
    }
    Ok(())
}
pub(crate) fn validate_stored_server(server: &str) -> Result<(), String> {
    if server.is_empty()
        || server.len() > 2048
        || server.trim() != server
        || server.chars().any(char::is_control)
    {
        return Err("Select a valid returned registry entry.".into());
    }
    Ok(())
}

pub(crate) fn config_lock(home: &Path) -> Result<std::fs::File, String> {
    let directory = home.join(".rdorbwin");
    std::fs::create_dir_all(&directory)
        .map_err(|_| "Cannot prepare private credential settings")?;
    let mut options = std::fs::OpenOptions::new();
    options.create(true).truncate(false).write(true);
    #[cfg(windows)]
    {
        use std::os::windows::fs::OpenOptionsExt;
        options.share_mode(0);
    }
    options
        .open(directory.join("registry-config.lock"))
        .map_err(|_| {
            "Private credential settings are busy. Wait for the other operation and retry.".into()
        })
}
// List/erase address only the owned Windows vault namespace. They do not need
// a config snapshot and remain usable when the private config requires review.
// Login stages WSL routing first, releases that legacy metadata lock, then
// acquires the independent native master owner. These locks are never nested.
const LIST_LIMIT: usize = 1 << 20;
const HELPER_FAILURE: &str = "The Windows credential operation failed. Refresh saved entries before retrying; a credential mutation may already have completed.";

#[derive(Default)]
struct ListCapture {
    bytes: Vec<u8>,
    invalid: bool,
}
impl ListCapture {
    fn append(&mut self, bytes: &[u8]) {
        if self.invalid || bytes.len() > LIST_LIMIT.saturating_sub(self.bytes.len()) {
            self.invalid = true;
            return;
        }
        self.bytes.extend_from_slice(bytes);
    }
}

fn list_metadata(bytes: &[u8]) -> Result<Vec<u8>, String> {
    if bytes.len() > LIST_LIMIT {
        return Err("Saved registry list exceeds its size limit.".into());
    }
    // Parsing errors can quote untrusted helper output: never return them.
    let entries: std::collections::BTreeMap<String, String> = serde_json::from_slice(bytes)
        .map_err(|_| "The saved registry list has an invalid shape.")?;
    if entries.len() > 256 {
        return Err("Saved registry list exceeds 256 entries.".into());
    }
    for (server, username) in &entries {
        validate_stored_server(server)?;
        // Match the held helper's stored username protocol. UI login has a
        // stricter input rule, but CLI-created metadata can contain escaped
        // controls and must still be visible/removable. JSON and React text
        // rendering preserve these as data; NUL is rejected by the helper too.
        if username.is_empty() || username.len() > 512 || username.contains('\0') {
            return Err("The saved registry list contains an invalid username.".into());
        }
    }
    serde_json::to_vec(&entries).map_err(|_| "Cannot prepare saved registry metadata.".into())
}

// No subprocess discovery is needed by the vault helper. Its exact held image
// owns protocol handling; ambient Docker/config selectors have no authority.
fn helper_environment() -> Vec<(OsString, OsString)> {
    std::env::vars_os()
        .filter(|(key, _)| {
            let key = key.to_string_lossy().to_ascii_uppercase();
            !key.starts_with("DOCKER_")
                && !key.starts_with("BUILDX_")
                && key != "BUILDKIT_HOST"
                && key != "ORBWIN_DOCKER_MANAGED_PLUGINS"
        })
        .collect()
}

async fn helper_command(
    action: &str,
    input: Option<Vec<u8>>,
    mutation: bool,
    cancelled: &mut oneshot::Receiver<()>,
    lease: Option<Lease>,
) -> Result<(crate::operations::ClientResult, ListCapture), String> {
    let helper = tokio::task::spawn_blocking(runtime::credentials)
        .await
        .map_err(|_| "Credential helper admission did not complete.")??;
    let captured = Arc::new(Mutex::new(ListCapture::default()));
    let incoming = captured.clone();
    let channel = Channel::new(move |body| {
        let event: serde_json::Value = body.deserialize()?;
        if !mutation && event["kind"] == "data" && event["stream"] == "stdout" {
            let bytes: Vec<u8> = serde_json::from_value(event["bytes"].clone())?;
            if let Ok(mut captured) = incoming.lock() {
                captured.append(&bytes);
            }
        }
        Ok(())
    });
    let result = crate::operation_worker::run(
        helper,
        vec![action.into()],
        helper_environment(),
        None,
        input,
        channel,
        cancelled,
        120,
        lease,
    )
    .await
    .map_err(|_| HELPER_FAILURE.to_owned())?;
    let captured = Arc::try_unwrap(captured)
        .map_err(|_| "Saved registry metadata remains borrowed.")?
        .into_inner()
        .map_err(|_| "Saved registry metadata is unavailable.")?;
    Ok((result, captured))
}

// WSL connects copy only validated owned server names, outside their config lock.
// No master config admission is needed to inspect the helper's owned namespace.
pub(crate) async fn stored_registry_keys() -> Result<Vec<String>, String> {
    let (_sender, mut cancelled) = oneshot::channel();
    let (result, captured) = helper_command("list", None, false, &mut cancelled, None).await?;
    if result.interrupted || result.exit_code != Some(0) {
        return Err(HELPER_FAILURE.into());
    }
    if captured.invalid {
        return Err("Saved registry list exceeds its size limit.".into());
    }
    let metadata = list_metadata(&captured.bytes)?;
    let entries: std::collections::BTreeMap<String, String> =
        serde_json::from_slice(&metadata).map_err(|_| "Saved registry metadata is unavailable.")?;
    Ok(entries.into_keys().collect())
}

pub(crate) async fn run(
    request: &Request,
    output: &Channel<Output>,
    cancelled: &mut oneshot::Receiver<()>,
    lease: Lease,
) -> Result<Outcome, String> {
    if let Request::RegistryLogin {
        server,
        username,
        secret,
    } = request
    {
        validate_login(server, username, secret)?;
        let (server, key) = login_target(server)?;
        let target = engine::target()?;
        let helper = tokio::task::spawn_blocking(runtime::credentials)
            .await
            .map_err(|_| "Credential helper admission did not complete.")??;
        let home = target.home().to_path_buf();
        let route = key.clone();
        let metadata_lease = lease.clone();
        tokio::task::spawn_blocking(move || {
            let _lease = metadata_lease;
            crate::wsl_clients::route_registry_helper(&home, &route)
        })
        .await
        .map_err(|_| "Private WSL credential routing did not complete.")??;
        let docker = target.docker.clone();
        let admission_lease = lease.clone();
        let mut guard = tokio::task::spawn_blocking(move || {
            let _lease = admission_lease;
            crate::client_config::acquire(docker, helper)
        })
        .await
        .map_err(|_| "Private credential configuration admission did not complete.")?
        .map_err(|e| e.to_string())?;
        let args = vec![
            "--config".into(),
            guard.path().as_os_str().to_owned(),
            "--host".into(),
            engine::ENDPOINT.into(),
            "login".into(),
            "--username".into(),
            username.into(),
            "--password-stdin".into(),
            server.into(),
        ];
        let environment = engine::docker_environment_for_paths(
            &target,
            guard.path(),
            &guard.path().join("buildx"),
        )?;
        let admission = guard
            .begin_registry_writer(&[key], &args)
            .map_err(|e| e.to_string())?;
        let spec = crate::native_client::NativeSpec::new(
            target.docker.clone(),
            args,
            environment,
            None,
            Some(admission),
        );
        let completion = crate::credential_writer::execute(
            guard,
            spec,
            secret.0.as_bytes().to_vec(),
            Duration::from_secs(120),
            cancelled,
            Some(lease),
        )
        .await?;
        let success = completion.exit_code == 0;
        return Ok(Outcome {
            success, mutation: true, interrupted: false, exit_code: Some(completion.exit_code as i32),
            message: if success {
                "Registry authentication completed and credentials were saved in the OrbWin Windows store."
            } else {
                "Registry authentication or credential storage failed. Check the registry, credentials and network, then refresh saved entries. Existing credentials were not intentionally removed."
            }.into(),
        });
    }
    let (action, input, mutation) = match request {
        Request::RegistryList => ("list", None, false),
        Request::RegistryForget { server } => {
            validate_stored_server(server)?;
            ("erase", Some(server.as_bytes().to_vec()), true)
        }
        _ => return Err("Unsupported registry operation".into()),
    };
    let (result, captured) =
        helper_command(action, input, mutation, cancelled, Some(lease)).await?;
    let success = !result.interrupted && result.exit_code == Some(0);
    if success && !mutation {
        if captured.invalid {
            return Err("Saved registry list exceeds its size limit.".into());
        }
        let metadata = list_metadata(&captured.bytes)?;
        output
            .send(Output::Data {
                stream: "stdout",
                bytes: metadata,
            })
            .map_err(|_| "The output viewer disconnected")?;
    }
    Ok(Outcome {
        success, mutation, interrupted: result.interrupted, exit_code: result.exit_code,
        message: if success {
            if mutation { "Saved OrbWin credentials were removed. Workloads were not stopped." }
            else { "Saved registry list read." }
        } else if result.interrupted {
            "Credential command stopped. Authentication or storage may already have completed; refresh the saved registry list."
        } else { HELPER_FAILURE }.into(),
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn registry_targets_are_explicit_and_secrets_redacted() {
        assert_eq!(
            login_target("Docker.IO").unwrap().1,
            "https://index.docker.io/v1/"
        );
        assert_eq!(login_target("localhost:5000").unwrap().0, "localhost:5000");
        for value in [
            "--help",
            "registry/path",
            "https://user:token@example.com",
            "localhost:0",
            "localhost:65536",
            "registry\n",
            "..example",
            "[::1]:5000",
        ] {
            assert!(login_target(value).is_err())
        }
        let secret = Secret("disposable-synthetic-secret".into());
        assert!(!format!("{secret:?}").contains(&secret.0));
        validate_login("docker.io", "proof-user", &secret).unwrap();
        assert!(validate_login("docker.io", "proof-user", &Secret("token\n".into())).is_err());
    }
    #[test]
    fn registry_metadata_is_bounded_and_rejects_non_metadata_shapes() {
        assert_eq!(
            list_metadata(br#"{"registry.example":"user"}"#).unwrap(),
            br#"{"registry.example":"user"}"#
        );
        for value in [
            br#"{"registry.example":{"Secret":"untrusted"}}"#.as_slice(),
            br#"{"registry.example":"user\u0000"}"#,
            br#"{"registry.example":""}"#,
            br#"{" registry.example":"user"}"#,
            b"untrusted-child-output",
        ] {
            let error = list_metadata(value).unwrap_err();
            assert!(!error.contains("untrusted"));
        }
        let mut captured = ListCapture::default();
        captured.append(&vec![b'x'; LIST_LIMIT]);
        assert!(!captured.invalid);
        captured.append(b"x");
        assert!(captured.invalid);
        assert_eq!(captured.bytes.len(), LIST_LIMIT);
        captured.append(b"discarded");
        assert_eq!(captured.bytes.len(), LIST_LIMIT);
        let many: std::collections::BTreeMap<_, _> = (0..257)
            .map(|i| (format!("r{i}.example"), "user"))
            .collect();
        assert!(list_metadata(&serde_json::to_vec(&many).unwrap()).is_err());
        assert_eq!(
            list_metadata(br#"{"registry.example":"cli\nuser"}"#).unwrap(),
            br#"{"registry.example":"cli\nuser"}"#
        );
    }

    #[test]
    fn helper_list_channel_releases_capture_after_owned_delivery() {
        let captured = Arc::new(Mutex::new(ListCapture::default()));
        let incoming = captured.clone();
        let channel = Channel::new(move |body| {
            let event: serde_json::Value = body.deserialize()?;
            if event["kind"] == "data" && event["stream"] == "stdout" {
                let bytes: Vec<u8> = serde_json::from_value(event["bytes"].clone())?;
                incoming.lock().unwrap().append(&bytes);
            }
            Ok(())
        });
        channel
            .send(Output::Data {
                stream: "stdout",
                bytes: br#"{"registry.example":"user"}"#.to_vec(),
            })
            .unwrap();
        drop(channel);
        let captured = Arc::try_unwrap(captured)
            .ok()
            .unwrap()
            .into_inner()
            .unwrap();
        assert_eq!(
            list_metadata(&captured.bytes).unwrap(),
            br#"{"registry.example":"user"}"#
        );
    }
}
