//! Docker authenticates registries; a pinned helper owns only OrbWin vault keys.
//! Secrets travel through stdin and never enter arguments, debug text or output.
//! Login output is untrusted (a registry can echo credentials), so discard it.
//! Cancellation is uncertain: authentication/storage may already have committed.
use crate::{
    engine,
    operations::{self, Outcome, Output, Request},
    runtime,
};
use serde::Deserialize;
use std::{fmt, path::Path};
use tauri::ipc::Channel;
use tokio::{process::Command, sync::oneshot};

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

fn route_helper_to(value: &mut serde_json::Value, key: &str, helper: &str) -> Result<(), String> {
    let object = value
        .as_object_mut()
        .ok_or("Private Docker configuration must be an object")?;
    if object
        .get("auths")
        .and_then(|v| v.get(key))
        .is_some_and(|entry| {
            ["auth", "identitytoken", "registrytoken"]
                .iter()
                .any(|field| entry[*field].as_str().is_some_and(|s| !s.is_empty()))
        })
    {
        return Err("This private registry has existing file credentials. Use its current client or remove those credentials explicitly before changing its store.".into());
    }
    let helpers = object
        .entry("credHelpers")
        .or_insert_with(|| serde_json::json!({}))
        .as_object_mut()
        .ok_or("Private registry helper configuration is invalid")?;
    if helpers.get(key).is_some_and(|value| value != helper) {
        return Err("This private registry already uses another credential helper; its configuration was retained.".into());
    }
    helpers.insert(key.into(), serde_json::json!(helper));
    Ok(())
}
fn route_helper(value: &mut serde_json::Value, key: &str) -> Result<(), String> {
    route_helper_to(value, key, "orbwin")
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
fn read_config(path: &Path) -> Result<serde_json::Value, String> {
    use std::io::Read;
    if !path.exists() {
        return Ok(serde_json::json!({}));
    }
    let mut bytes = Vec::new();
    std::fs::File::open(path)
        .map_err(|_| "Cannot read private credential settings")?
        .take((1 << 20) + 1)
        .read_to_end(&mut bytes)
        .map_err(|_| "Cannot read private credential settings")?;
    if bytes.len() > 1 << 20 {
        return Err("Private credential settings exceed 1 MiB; they were preserved.".into());
    }
    serde_json::from_slice(&bytes)
        .map_err(|_| "Private credential settings are invalid; they were preserved.".into())
}
// Caller holds config_lock. Copy routing metadata only; WSL invokes the exact
// bundled Windows helper through explicit PATH and normal WSL interop.
pub(crate) fn sync_wsl(home: &Path, value: &mut serde_json::Value) -> Result<(), String> {
    let windows = read_config(&home.join(".rdorbwin/docker/config.json"))?;
    if let Some(helpers) = windows["credHelpers"].as_object() {
        for (key, helper) in helpers {
            if helper == "orbwin" {
                validate_stored_server(key)?;
                route_helper_to(value, key, "orbwin.exe")?;
            }
        }
    }
    Ok(())
}
fn configure(config: &Path, key: &str) -> Result<(), String> {
    let home = config
        .parent()
        .and_then(Path::parent)
        .ok_or("Private profile path is invalid")?;
    let _lock = config_lock(home)?;
    runtime::configure_client(config)?;
    let path = config.join("config.json");
    let mut value = read_config(&path)?;
    route_helper(&mut value, key)?;
    let wsl = home.join(".rdorbwin/wsl-docker");
    std::fs::create_dir_all(&wsl).map_err(|_| "Cannot prepare private WSL credential routing")?;
    let mut linux = read_config(&wsl.join("config.json"))?;
    route_helper_to(&mut linux, key, "orbwin.exe")?;
    // No credentials have been sent. A partial metadata write is safe to retry;
    // each client still targets the same owned helper and independent engine.
    runtime::write_json(&wsl.join("config.json"), &linux)?;
    runtime::write_json(&path, &value)
}

pub(crate) async fn run(
    request: &Request,
    output: &Channel<Output>,
    cancelled: &mut oneshot::Receiver<()>,
) -> Result<Outcome, String> {
    let target = engine::target()?;
    let helper = tokio::task::spawn_blocking(runtime::credentials)
        .await
        .map_err(|e| e.to_string())??;
    if cancelled.try_recv().is_ok() {
        return Err("Cancelled before dispatch. No credential operation was sent.".into());
    }
    let login = matches!(request, Request::RegistryLogin { .. });
    let mutation = !matches!(request, Request::RegistryList);
    let mut command = Command::new(if login { target.docker.clone() } else { helper });
    let input = match request {
        Request::RegistryLogin {
            server,
            username,
            secret,
        } => {
            let (server, key) = login_target(server)?;
            let config = target.config.clone();
            tokio::task::spawn_blocking(move || configure(&config, &key))
                .await
                .map_err(|e| e.to_string())??;
            command
                .arg("--config")
                .arg(&target.config)
                .arg("--host")
                .arg(engine::ENDPOINT)
                .args(["login", "--username", username, "--password-stdin", &server]);
            Some(secret.0.as_bytes())
        }
        Request::RegistryForget { server } => {
            command.arg("erase");
            Some(server.as_bytes())
        }
        Request::RegistryList => {
            command.arg("list");
            None
        }
        _ => return Err("Unsupported registry operation".into()),
    };
    command
        .env("PATH", runtime::client_path()?)
        .env("DOCKER_CONFIG", &target.config)
        .env("DOCKER_HOST", engine::ENDPOINT)
        .env_remove("DOCKER_CONTEXT")
        .env_remove("DOCKER_TLS_VERIFY")
        .env_remove("DOCKER_CERT_PATH");
    let discarded = Channel::new(|_| Ok(()));
    let result = operations::client_tree(
        command,
        input,
        if login { &discarded } else { output },
        cancelled,
        120,
    )
    .await?;
    let success = !result.interrupted && result.exit_code == Some(0);
    let message = if success {
        if login {
            "Registry authentication completed and credentials were saved in the OrbWin Windows store."
        } else if mutation {
            "Saved OrbWin credentials were removed. Workloads were not stopped."
        } else {
            "Saved registry list read."
        }
    } else if result.interrupted {
        "Credential command stopped. Authentication or storage may already have completed; refresh the saved registry list."
    } else if login {
        "Registry authentication or credential storage failed. Check the registry, credentials and network, then refresh saved entries. Existing credentials were not intentionally removed."
    } else {
        "The Windows credential operation failed. Refresh and inspect the selected entry before retrying."
    };
    Ok(Outcome {
        success,
        mutation,
        interrupted: result.interrupted,
        exit_code: result.exit_code,
        message: message.into(),
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
    fn private_helper_routes_preserve_other_settings_and_refuse_credential_hijacking() {
        let mut config = serde_json::json!({"unknown":9007199254740993u64,"credsStore":"explicit","credHelpers":{"other.example":"other"},"auths":{"untouched.example":{"auth":"synthetic-existing"}}});
        route_helper(&mut config, "registry.example").unwrap();
        assert_eq!(config["unknown"], 9007199254740993u64);
        assert_eq!(config["credsStore"], "explicit");
        assert_eq!(
            config["auths"]["untouched.example"]["auth"],
            "synthetic-existing"
        );
        assert_eq!(config["credHelpers"]["other.example"], "other");
        let before = config.clone();
        assert!(route_helper(&mut config, "other.example").is_err());
        assert!(route_helper(&mut config, "untouched.example").is_err());
        assert_eq!(config, before);
    }
    #[test]
    fn wsl_routes_only_product_metadata_and_refuses_foreign_settings() {
        let directory = std::env::temp_dir().join(format!(
            "orbwin-registry-routing-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        let config = directory.join(".rdorbwin/docker");
        std::fs::create_dir_all(&config).unwrap();
        std::fs::write(config.join("config.json"),r#"{"credHelpers":{"owned.example":"orbwin","foreign.example":"foreign"},"auths":{"secret.example":{"auth":"synthetic-only"}}}"#).unwrap();
        let _lock = config_lock(&directory).unwrap();
        #[cfg(windows)]
        assert!(config_lock(&directory).is_err());
        let mut linux = serde_json::json!({"unknown":9007199254740993u64});
        sync_wsl(&directory, &mut linux).unwrap();
        assert_eq!(
            linux,
            serde_json::json!({"unknown":9007199254740993u64,"credHelpers":{"owned.example":"orbwin.exe"}})
        );
        let mut foreign = serde_json::json!({"credHelpers":{"owned.example":"another"}});
        let before = foreign.clone();
        assert!(sync_wsl(&directory, &mut foreign).is_err());
        assert_eq!(foreign, before);
        drop(_lock);
        std::fs::remove_dir_all(directory).unwrap();
    }
}
