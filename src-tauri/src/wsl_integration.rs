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
    export: Option<Export>,
    connected: bool,
    docker_command: String,
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

fn tools() -> Result<(PathBuf, String), String> {
    let lock: serde_json::Value =
        serde_json::from_str(include_str!("../../runtime-tools.lock.json"))
            .map_err(|e| e.to_string())?;
    for tool in lock["tools"].as_array().ok_or("Tool pins missing")? {
        let path = tool["path"].as_str().ok_or("Tool path missing")?;
        if path.starts_with("linux/")
            && runtime::hash_file(&runtime::tools_dir().join(path), 128 << 20)? != tool["sha256"]
        {
            return Err(
                "Pinned Linux tool checksum differs. Repair the application resources.".into(),
            );
        }
    }
    let directory = runtime::tools_dir()
        .join("linux")
        .canonicalize()
        .map_err(|e| e.to_string())?;
    let linux = linux_path(&directory)?;
    Ok((directory, linux))
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
            export: record,
            connected,
            docker_command: String::new(),
        });
    }
    let (_, directory) = tokio::task::spawn_blocking(tools)
        .await
        .map_err(|e| e.to_string())??;
    let config = target.home().join(".rdorbwin/wsl-docker");
    let command = format!("env -u DOCKER_CONTEXT -u DOCKER_TLS_VERIFY -u DOCKER_CERT_PATH -u BUILDKIT_HOST BUILDX_BUILDER=default BUILDX_CONFIG={} {} --config {} --host unix:///mnt/wsl/orbwin/docker.sock", quote(&linux_path(&config.join("buildx"))?), quote(&format!("{directory}/docker")), quote(&linux_path(&config)?));
    if action == "connect" {
        std::fs::create_dir_all(&config).map_err(|e| e.to_string())?;
        let file = config.join("config.json");
        let mut value = if file.exists() {
            use std::io::Read;
            let mut bytes = Vec::new();
            std::fs::File::open(&file)
                .map_err(|e| e.to_string())?
                .take((1 << 20) + 1)
                .read_to_end(&mut bytes)
                .map_err(|e| e.to_string())?;
            if bytes.len() > 1 << 20 {
                return Err("Private WSL client config exceeds 1 MiB. It was preserved.".into());
            }
            serde_json::from_slice::<serde_json::Value>(&bytes).map_err(|e| {
                format!("Private WSL client config is invalid and was preserved: {e}")
            })?
        } else {
            serde_json::json!({})
        };
        if !value.is_object() {
            return Err("Private WSL client config must be an object. It was preserved.".into());
        }
        value["cliPluginsExtraDirs"] = serde_json::json!([directory]);
        runtime::private_auth_defaults(&mut value)?;
        runtime::write_json(&file, &value)?;
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
    Ok(Status {
        export: record,
        connected,
        docker_command: command,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn project_scope_and_shell_arguments() {
        for path in ["/", "/etc", "/home/a/../b", "/home/a\ncmd", "~/project"] {
            assert!(source_path(path).is_err());
        }
        assert!(source_path("/home/user/project with spaces/é").is_ok());
        assert_eq!(quote("a'b"), "'a'\"'\"'b'");
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
