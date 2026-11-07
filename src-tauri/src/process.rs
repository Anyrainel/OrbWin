//! Bounded read-only diagnostic subprocesses. The child is owned by this future;
//! timeout kills/reaps it; cancellation uses Tokio's kill-on-drop behavior.
//! Output never exceeds 128 KiB
//! per stream. No shell or renderer arguments cross this boundary. This runner
//! must not be used for rdd's long-lived service/guest lifecycle.
use std::{path::Path, process::Stdio, time::Duration};
use tokio::{io::AsyncReadExt, process::Command, time::timeout};

const LIMIT: u64 = 128 * 1024;

pub(crate) fn decode(bytes: &[u8]) -> String {
    // WSL emits UTF-16LE when redirected on Windows, with or without a BOM.
    let utf16 =
        bytes.starts_with(&[0xff, 0xfe]) || (bytes.len() > 3 && bytes[1] == 0 && bytes[3] == 0);
    if utf16 {
        let start = if bytes.starts_with(&[0xff, 0xfe]) {
            2
        } else {
            0
        };
        let units: Vec<u16> = bytes[start..]
            .chunks_exact(2)
            .map(|pair| u16::from_le_bytes([pair[0], pair[1]]))
            .collect();
        String::from_utf16_lossy(&units).trim().to_string()
    } else {
        String::from_utf8_lossy(bytes).trim().to_string()
    }
}

pub async fn read_command(binary: &Path, args: &[&str]) -> Result<String, String> {
    read_with_deadline(binary, args, Duration::from_secs(10)).await
}

// Closed cached SDK diagnostics use the atomic transient owner. OS-supplied
// WSL tools keep their separate system-code admission/runner contract.
pub(crate) async fn read_cached(
    binary: &crate::runtime_cache::CachedExecutable,
    args: &[&str],
) -> Result<String, String> {
    let environment = std::env::vars_os()
        .filter(|(key, _)| {
            !matches!(
                key.to_string_lossy().to_ascii_uppercase().as_str(),
                "DOCKER_CONTEXT" | "DOCKER_TLS_VERIFY" | "DOCKER_CERT_PATH"
            )
        })
        .collect();
    crate::client_worker::read(
        binary.clone(),
        args.iter().map(std::ffi::OsString::from).collect(),
        environment,
        None,
        Duration::from_secs(10),
    )
    .await
}

async fn bounded_read(reader: impl tokio::io::AsyncRead + Unpin) -> Result<Vec<u8>, String> {
    let mut bytes = Vec::new();
    reader
        .take(LIMIT + 1)
        .read_to_end(&mut bytes)
        .await
        .map_err(|e| e.to_string())?;
    if bytes.len() as u64 > LIMIT {
        return Err("Diagnostic output exceeded the size limit".into());
    }
    Ok(bytes)
}

async fn read_with_deadline(
    binary: &Path,
    args: &[&str],
    deadline: Duration,
) -> Result<String, String> {
    let mut command = Command::new(binary);
    // An inherited Docker context/TLS selector must never redirect a scoped
    // engine probe. WSL diagnostics do not use these variables.
    command
        .env_remove("DOCKER_CONTEXT")
        .env_remove("DOCKER_TLS_VERIFY")
        .env_remove("DOCKER_CERT_PATH");
    command
        .args(args)
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .kill_on_drop(true);
    read_prepared_command(command, deadline).await
}

async fn read_prepared_command(mut command: Command, deadline: Duration) -> Result<String, String> {
    command.kill_on_drop(true);
    #[cfg(windows)]
    command.creation_flags(0x08000000); // CREATE_NO_WINDOW
    let mut child = command
        .spawn()
        .map_err(|e| format!("Could not start diagnostic: {e}"))?;
    let stdout = child.stdout.take().ok_or("Missing diagnostic stdout")?;
    let stderr = child.stderr.take().ok_or("Missing diagnostic stderr")?;
    let result = timeout(deadline, async {
        // Drain both pipes concurrently; a child writing stderr must not block
        // while stdout is being read. Bound reads before allocating strings.
        let (out, err, status) =
            tokio::try_join!(bounded_read(stdout), bounded_read(stderr), async {
                child.wait().await.map_err(|e| e.to_string())
            })?;
        if !status.success() {
            return Err(format!("Diagnostic exited with {status}: {}", decode(&err)));
        }
        Ok(decode(&out))
    })
    .await;
    match result {
        Ok(Ok(value)) => Ok(value),
        Ok(Err(error)) => {
            let _ = child.kill().await;
            let _ = child.wait().await;
            Err(error)
        }
        Err(_) => {
            let _ = child.kill().await;
            let _ = child.wait().await;
            Err(format!(
                "Diagnostic timed out after {} ms",
                deadline.as_millis()
            ))
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn decodes_wsl_and_utf8() {
        assert_eq!(decode(b"ready\r\n"), "ready");
        let encoded: Vec<_> = "Ubuntu\r\n"
            .encode_utf16()
            .flat_map(u16::to_le_bytes)
            .collect();
        assert_eq!(decode(&encoded), "Ubuntu");
        let mut bom = vec![0xff, 0xfe];
        bom.extend(encoded);
        assert_eq!(decode(&bom), "Ubuntu");
    }

    #[cfg(windows)]
    fn powershell() -> std::path::PathBuf {
        std::path::PathBuf::from(std::env::var_os("SystemRoot").unwrap())
            .join("System32/WindowsPowerShell/v1.0/powershell.exe")
    }

    #[cfg(windows)]
    #[tokio::test]
    async fn timeout_kills_and_reaps_child() {
        let result = read_with_deadline(
            &powershell(),
            &[
                "-NoProfile",
                "-NonInteractive",
                "-Command",
                "Start-Sleep -Seconds 30",
            ],
            Duration::from_millis(100),
        )
        .await;
        assert!(result.unwrap_err().contains("timed out"));
    }

    #[cfg(windows)]
    #[tokio::test]
    async fn excessive_output_fails_without_waiting_for_deadline() {
        let start = std::time::Instant::now();
        let error = read_command(
            &powershell(),
            &[
                "-NoProfile",
                "-NonInteractive",
                "-Command",
                "[Console]::Out.Write(('x' * 140000)); Start-Sleep -Seconds 30",
            ],
        )
        .await
        .unwrap_err();
        assert!(error.contains("size limit"), "{error}");
        assert!(start.elapsed() < Duration::from_secs(8));
    }

    #[cfg(windows)]
    #[tokio::test]
    async fn preserves_failure_output() {
        let error = read_command(
            &powershell(),
            &[
                "-NoProfile",
                "-NonInteractive",
                "-Command",
                "[Console]::Error.Write('expected failure'); exit 7",
            ],
        )
        .await
        .unwrap_err();
        assert!(error.contains("expected failure"));
        assert!(error.contains('7'));
    }

    #[cfg(windows)]
    #[tokio::test]
    async fn native_timeout_reaps_the_acknowledged_process() {
        let fixture = crate::process_fixture::Fixture::new();
        let command = fixture.command("sleep");
        let task = tokio::spawn(read_prepared_command(
            command.into(),
            Duration::from_secs(2),
        ));
        let handle = crate::process_fixture::retain_running_process(&fixture).await;
        let error = task.await.unwrap().unwrap_err();
        assert!(error.contains("2000 ms"), "{error}");
        crate::process_fixture::assert_exited(&handle);
    }

    #[cfg(windows)]
    #[tokio::test]
    async fn native_cancellation_kills_the_acknowledged_process() {
        let fixture = crate::process_fixture::Fixture::new();
        let task = tokio::spawn(read_prepared_command(
            fixture.command("sleep").into(),
            Duration::from_secs(15),
        ));
        let handle = crate::process_fixture::retain_running_process(&fixture).await;
        task.abort();
        assert!(task.await.unwrap_err().is_cancelled());
        crate::process_fixture::assert_exited(&handle);
    }

    #[cfg(windows)]
    #[tokio::test]
    async fn native_output_limit_and_failure_status() {
        let output = crate::process_fixture::Fixture::new();
        let start = std::time::Instant::now();
        let error = read_prepared_command(output.command("output").into(), Duration::from_secs(10))
            .await
            .unwrap_err();
        assert!(error.contains("size limit"), "{error}");
        assert!(start.elapsed() < Duration::from_secs(8));
        let failure = crate::process_fixture::Fixture::new();
        let error =
            read_prepared_command(failure.command("failure").into(), Duration::from_secs(10))
                .await
                .unwrap_err();
        assert!(
            error.contains("native expected failure") && error.contains('7'),
            "{error}"
        );
    }
}
