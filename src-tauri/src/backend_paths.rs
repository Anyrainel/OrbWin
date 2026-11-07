//! Version-specific rdd routing data, never an authority or lifecycle grant.
//! `service paths` does not start the service; its normal Cobra initialization
//! still briefly binds and closes an ephemeral loopback listener.
use crate::runtime_cache::CachedExecutable;
use serde::Deserialize;
use std::{
    ffi::OsString,
    path::{Path, PathBuf},
    time::Duration,
};

const OUTPUT_LIMIT: usize = 128 * 1024;

#[derive(Debug)]
pub(crate) struct BackendPaths {
    pub(crate) args_file: PathBuf,
    pub(crate) config: PathBuf,
    pub(crate) k3s_config: PathBuf,
}

// Serde rejects duplicate required fields, missing fields, and non-string
// values. Other rdd path keys may evolve independently and are ignored.
#[derive(Deserialize)]
struct WirePaths {
    dir: String,
    args_file: String,
    config: String,
    k3s_config: String,
}

pub(crate) async fn read(
    executable: &CachedExecutable,
    home: &Path,
) -> Result<BackendPaths, String> {
    read_setup(executable, home, "orbwin").await
}

// Setup already owns a verified cached image before guest registration exists.
// Do not add target.rdd(), engine checks, or a legacy-directory fallback here.
pub(crate) async fn read_setup(
    executable: &CachedExecutable,
    home: &Path,
    instance: &str,
) -> Result<BackendPaths, String> {
    let args = arguments(instance)?;
    let environment = environment(std::env::vars_os(), home, instance)?;
    let output = crate::client_worker::read(
        executable.clone(),
        args,
        environment,
        None,
        Duration::from_secs(10),
    )
    .await?;
    parse(&output)
}

fn arguments(instance: &str) -> Result<Vec<OsString>, String> {
    if !matches!(instance, "orbwin" | "orbwin-proof") {
        return Err("Unsupported backend path instance.".into());
    }
    // The backend's early instance parser only examines the first argument.
    Ok(["--instance", instance, "service", "paths", "-o", "json"]
        .into_iter()
        .map(OsString::from)
        .collect())
}

fn environment(
    inherited: impl IntoIterator<Item = (OsString, OsString)>,
    home: &Path,
    instance: &str,
) -> Result<Vec<(OsString, OsString)>, String> {
    arguments(instance)?;
    let mut environment: Vec<_> = inherited
        .into_iter()
        .filter(|(key, _)| {
            let key = key.to_string_lossy().to_ascii_uppercase();
            !matches!(
                key.as_str(),
                "HOME"
                    | "USERPROFILE"
                    | "RDD_INSTANCE"
                    | "RDD_LOG_DIR"
                    | "RDD_DEVELOPER_MODE"
                    | "RDD_KEEP_LOGS"
                    | "LIMA_HOME"
                    | "KUBECONFIG"
                    | "ORBWIN_DOCKER_MANAGED_PLUGINS"
            ) && !key.starts_with("DOCKER_")
                && !key.starts_with("BUILDX_")
                && !key.starts_with("BUILDKIT_")
        })
        .collect();
    let short = home.join(format!(".rd{instance}"));
    environment.extend([
        ("HOME".into(), home.as_os_str().to_owned()),
        ("USERPROFILE".into(), home.as_os_str().to_owned()),
        ("RDD_INSTANCE".into(), instance.into()),
        ("RDD_DEVELOPER_MODE".into(), "1".into()),
        ("RDD_KEEP_LOGS".into(), "1".into()),
        (
            "DOCKER_CONFIG".into(),
            short.join("docker").into_os_string(),
        ),
        (
            "KUBECONFIG".into(),
            short.join("kube-client.config").into_os_string(),
        ),
    ]);
    Ok(environment)
}

fn parse(output: &str) -> Result<BackendPaths, String> {
    if output.len() > OUTPUT_LIMIT {
        return Err("Backend path response exceeds its bound.".into());
    }
    let wire: WirePaths = serde_json::from_str(output)
        .map_err(|error| format!("Invalid backend path response: {error}"))?;
    let dir = local_path(&wire.dir)?;
    let child = |value: &str, name: &str| -> Result<PathBuf, String> {
        let value = local_path(value)?;
        let expected = format!("{}\\{name}", dir.trim_end_matches('\\'));
        if !value.eq_ignore_ascii_case(&expected) {
            return Err(format!(
                "Backend path {name} is not its fixed directory child."
            ));
        }
        Ok(PathBuf::from(value))
    };
    Ok(BackendPaths {
        args_file: child(&wire.args_file, "args.json")?,
        config: child(&wire.config, "config.yaml")?,
        k3s_config: child(&wire.k3s_config, "k3s.yaml")?,
    })
}

// Lexical Windows validation works before directories/files exist. It does not
// canonicalize, inspect ACLs, bind an owner, or prove that a path is safe to open.
fn local_path(value: &str) -> Result<String, String> {
    let value = value.replace('/', "\\");
    let bytes = value.as_bytes();
    if bytes.len() < 3
        || !bytes[0].is_ascii_alphabetic()
        || bytes[1] != b':'
        || bytes[2] != b'\\'
        || value.encode_utf16().count() >= 32767
        || value
            .chars()
            .any(|c| c.is_control() || matches!(c, '<' | '>' | '"' | '|' | '?' | '*'))
    {
        return Err("Backend paths must be absolute local Windows paths.".into());
    }
    if value.len() > 3 {
        for component in value[3..].split('\\') {
            let stem = component
                .split('.')
                .next()
                .unwrap_or("")
                .to_ascii_uppercase();
            let numbered_device = stem
                .strip_prefix("COM")
                .or_else(|| stem.strip_prefix("LPT"))
                .is_some_and(|suffix| {
                    matches!(
                        suffix,
                        "1" | "2" | "3" | "4" | "5" | "6" | "7" | "8" | "9" | "¹" | "²" | "³"
                    )
                });
            if component.is_empty()
                || component == "."
                || component == ".."
                || component.ends_with([' ', '.'])
                || component.contains(':')
                || matches!(stem.as_str(), "CON" | "PRN" | "AUX" | "NUL")
                || numbered_device
            {
                return Err("Backend paths contain an unsupported Windows component.".into());
            }
        }
    }
    Ok(value)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn response(dir: &str) -> String {
        serde_json::json!({"dir":dir,"args_file":format!("{dir}\\args.json"),
            "config":format!("{dir}\\config.yaml"),"k3s_config":format!("{dir}\\k3s.yaml"),
            "log_dir":"ignored upstream path", "future":{"allowed":true}})
        .to_string()
    }

    #[test]
    fn accepts_private_native_profile_and_fresh_unicode_paths_without_io() {
        for dir in [
            r"C:\Users\Native\.orbwin-private\authority\instance",
            r"D:\Fresh\用户 space\instance",
        ] {
            let paths = parse(&response(dir)).unwrap();
            assert_eq!(paths.args_file, PathBuf::from(format!("{dir}\\args.json")));
            assert_eq!(paths.config, PathBuf::from(format!("{dir}\\config.yaml")));
            assert_eq!(paths.k3s_config, PathBuf::from(format!("{dir}\\k3s.yaml")));
        }
        assert!(parse(&response("C:/Fresh/instance")).is_ok());
    }

    #[test]
    fn requires_unique_typed_complete_object() {
        let valid = response(r"C:\Fresh\instance");
        for key in ["dir", "args_file", "config", "k3s_config"] {
            let mut object: serde_json::Value = serde_json::from_str(&valid).unwrap();
            object.as_object_mut().unwrap().remove(key);
            assert!(parse(&object.to_string()).is_err(), "missing {key}");
            object[key] = serde_json::json!(42);
            assert!(parse(&object.to_string()).is_err(), "non-string {key}");
            let duplicate = format!("{{\"{key}\":\"C:\\\\x\",{}", &valid[1..]);
            assert!(parse(&duplicate).is_err(), "duplicate {key}");
        }
        for output in [
            "null",
            "[]",
            "{}",
            "",
            "{",
            &format!("{valid} {{}}"),
            &" ".repeat(OUTPUT_LIMIT + 1),
        ] {
            assert!(parse(output).is_err());
        }
    }

    #[test]
    fn rejects_nonlocal_ambiguous_and_device_paths() {
        for dir in [
            r"relative",
            r"C:relative",
            r"\rooted",
            r"\\server\share\x",
            r"\\?\C:\x",
            r"\\.\C:\x",
            r"C:\a\..\b",
            r"C:\a\.\b",
            r"C:\a\\b",
            "C:\\a\\",
            "C:\\a.\\b",
            "C:\\a \\b",
            r"C:\a:b",
            r"C:\CON\b",
            r"C:\nul.txt\b",
            r"C:\COM1\b",
            r"C:\lpt²\b",
            "C:\\a\nb",
            "C:\\a\0b",
            r"C:\a?b",
            r"C:\a|b",
        ] {
            assert!(parse(&response(dir)).is_err(), "{dir:?}");
        }
        assert!(local_path(&format!("C:\\{}", "a".repeat(32767))).is_err());
    }

    #[test]
    fn children_cannot_escape_or_change_role() {
        let valid = response(r"C:\Fresh\instance");
        for (key, bad) in [
            ("args_file", r"C:\Fresh\instance\config.yaml"),
            ("config", r"C:\Other\config.yaml"),
            ("k3s_config", r"C:\Fresh\instance\nested\k3s.yaml"),
            ("config", r"C:\Fresh\instance-other\config.yaml"),
            ("config", r"C:\Fresh\instance\..\config.yaml"),
        ] {
            let mut object: serde_json::Value = serde_json::from_str(&valid).unwrap();
            object[key] = bad.into();
            assert!(parse(&object.to_string()).is_err(), "{key} {bad}");
        }
    }

    #[test]
    fn argv_has_early_fixed_instance_and_no_lifecycle_command() {
        for instance in ["orbwin", "orbwin-proof"] {
            assert_eq!(
                arguments(instance).unwrap(),
                ["--instance", instance, "service", "paths", "-o", "json"].map(OsString::from)
            );
        }
        for instance in ["", "other", "--help", "OR BWIN", "orbwin "] {
            assert!(arguments(instance).is_err());
        }
    }

    #[test]
    fn diagnostic_environment_scrubs_selectors_case_insensitively_and_keeps_system() {
        let keys = [
            "hOmE",
            "userprofile",
            "rDd_InStAnCe",
            "RDD_LOG_DIR",
            "lima_home",
            "kubeconfig",
            "Docker_Host",
            "DOCKER_CONTEXT",
            "docker_config",
            "DOCKER_TLS_VERIFY",
            "DOCKER_CERT_PATH",
            "DOCKER_API_VERSION",
            "BUILDX_BUILDER",
            "BUILDKIT_HOST",
            "ORBWIN_DOCKER_MANAGED_PLUGINS",
            "RDD_DEVELOPER_MODE",
            "rdd_keep_logs",
        ];
        let mut inherited: Vec<_> = keys
            .into_iter()
            .map(|k| (k.into(), "hostile".into()))
            .collect();
        inherited.extend([
            ("SystemRoot".into(), r"C:\Windows".into()),
            ("PATH".into(), "preserved".into()),
        ]);
        let home = Path::new(r"C:\Native\OrbWinData\home");
        let env = environment(inherited, home, "orbwin").unwrap();
        assert!(!env.iter().any(|(_, v)| v == "hostile"));
        let find = |key: &str| {
            env.iter()
                .find(|(k, _)| k == key)
                .map(|(_, v)| v.clone())
                .unwrap()
        };
        assert_eq!(find("HOME"), home.as_os_str());
        assert_eq!(find("USERPROFILE"), home.as_os_str());
        assert_eq!(find("RDD_INSTANCE"), "orbwin");
        assert_eq!(find("DOCKER_CONFIG"), home.join(".rdorbwin/docker"));
        assert_eq!(
            find("KUBECONFIG"),
            home.join(".rdorbwin/kube-client.config")
        );
        assert_eq!(find("SystemRoot"), r"C:\Windows");
        assert_eq!(find("PATH"), "preserved");
        for (index, (key, _)) in env.iter().enumerate() {
            assert!(!env[index + 1..].iter().any(|(other, _)| key
                .to_string_lossy()
                .eq_ignore_ascii_case(&other.to_string_lossy())));
        }
        let proof = environment([], home, "orbwin-proof").unwrap();
        assert!(proof
            .iter()
            .any(|(k, v)| k == "RDD_INSTANCE" && v == "orbwin-proof"));
        assert!(environment([], home, "rancher-desktop").is_err());
    }
}
