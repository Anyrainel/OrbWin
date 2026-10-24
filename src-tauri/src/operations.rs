//! Typed, explicitly targeted Docker clients; rdd/Moby retain state ownership.
//! One mutation at a time, at most four clients including reads. No shell, global
//! selectors, force removal, volume deletion with containers or broad prune.
//! Lifecycle requests set rdd's desired state; no UI-side supervisor is added.
//! Validate before spawning, verify engine before/after.
//! Cancellation/deadline kills and reaps the CLI, never the daemon. A mutation
//! may already have committed: return an uncertain result and require refresh.
//! Output drains concurrently with a shared 2 MiB IPC budget; excess is discarded
//! while the command finishes. Renderer disconnect kills only its CLI client.
use crate::engine;
use serde::{Deserialize, Serialize};
use std::{collections::HashMap, process::Stdio, sync::Mutex, time::Duration};
use tauri::ipc::Channel;
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt},
    process::Command,
    sync::oneshot,
};

#[derive(Default)]
pub struct Operations(Mutex<HashMap<String, Running>>);
struct Running {
    cancel: Option<oneshot::Sender<()>>,
    mutation: bool,
}
struct Lease<'a> {
    state: &'a Operations,
    id: String,
}
impl Drop for Lease<'_> {
    fn drop(&mut self) {
        if let Ok(mut jobs) = self.state.0.lock() {
            jobs.remove(&self.id);
        }
    }
}

#[derive(Deserialize, Debug)]
#[serde(tag = "kind", rename_all = "camelCase", deny_unknown_fields)]
pub enum Request {
    Setup,
    PrepareUpdate,
    RegistryList,
    RegistryLogin {
        server: String,
        username: String,
        secret: crate::registry::Secret,
    },
    RegistryForget {
        server: String,
    },
    DiskUsage,
    Files {
        target: String,
        path: String,
        action: FileAction,
    },
    BuildHistory {
        action: BuildHistoryAction,
        reference: String,
    },
    ClusterLogs {
        namespace: String,
        name: String,
        follow: bool,
    },
    ClusterForward {
        resource: ClusterResource,
        namespace: String,
        name: String,
        local: String,
        remote: String,
    },
    ClusterExec {
        namespace: String,
        name: String,
        command: Vec<String>,
    },
    Untag {
        reference: String,
    },
    ClusterWait {
        resource: ClusterResource,
        namespace: String,
        name: String,
        #[serde(default)]
        deleted: bool,
    },
    Engine {
        running: bool,
    },
    ReadSettings,
    ReadClusterVersions,
    ConfigureCluster {
        enabled: bool,
        #[serde(default)]
        version: Option<String>,
    },
    Cluster {
        action: ClusterAction,
        resource: ClusterResource,
        namespace: String,
        name: String,
    },
    ClusterApply {
        file: String,
    },
    ClusterDelete {
        resource: ClusterResource,
        namespace: String,
        name: String,
        uid: String,
    },
    Inspect {
        resource: Resource,
        target: String,
    },
    Container {
        action: ContainerAction,
        target: String,
    },
    Remove {
        resource: Resource,
        target: String,
    },
    Logs {
        target: String,
        follow: bool,
    },
    Stats {
        target: String,
    },
    Pull {
        image: String,
    },
    Tag {
        target: String,
        tag: String,
    },
    CreateVolume {
        name: String,
    },
    CreateNetwork {
        name: String,
    },
    Run {
        image: String,
        name: String,
        command: Vec<String>,
        #[serde(default)]
        options: RunOptions,
    },
    Exec {
        target: String,
        command: Vec<String>,
    },
    Build {
        directory: String,
        tag: String,
    },
    Compose {
        file: String,
        project: String,
        action: ComposeAction,
    },
    ConnectNetwork {
        network: String,
        container: String,
        disconnect: bool,
    },
}
#[derive(Deserialize, Debug, Default)]
#[serde(default, deny_unknown_fields)]
pub struct RunOptions {
    ports: Vec<String>,
    environment: Vec<String>,
    volumes: Vec<String>,
}
#[derive(Deserialize, Debug)]
#[serde(rename_all = "camelCase")]
pub enum ComposeAction {
    Up,
    Down,
    Start,
    Stop,
    Restart,
    Pull,
    Logs,
}
#[derive(Deserialize, Debug)]
#[serde(rename_all = "camelCase")]
pub enum BuildHistoryAction {
    List,
    Inspect,
    Logs,
}
#[derive(Deserialize, Debug)]
#[serde(rename_all = "camelCase")]
pub enum ClusterAction {
    List,
    Inspect,
    Logs,
}
#[derive(Deserialize, Debug, Clone, Copy)]
#[serde(rename_all = "camelCase")]
pub enum ClusterResource {
    Events,
    Pods,
    Services,
    Deployments,
    PersistentVolumeClaims,
}
impl ClusterResource {
    fn cli(&self) -> &'static str {
        match self {
            Self::Events => "events",
            Self::Pods => "pods",
            Self::Services => "services",
            Self::Deployments => "deployments",
            Self::PersistentVolumeClaims => "persistentvolumeclaims",
        }
    }
}
#[derive(Deserialize, Debug, Clone, Copy)]
#[serde(rename_all = "camelCase")]
pub enum Resource {
    Container,
    Image,
    Volume,
    Network,
}
#[derive(Deserialize, Debug)]
#[serde(rename_all = "camelCase")]
pub enum ContainerAction {
    Start,
    Stop,
    Restart,
    Pause,
    Unpause,
}

#[derive(Deserialize, Debug)]
#[serde(rename_all = "camelCase")]
pub enum FileAction {
    List,
    Read,
}

struct Plan {
    args: Vec<String>,
    mutation: bool,
    seconds: u64,
    input: Option<Vec<u8>>,
}
fn value(value: &str) -> Result<(), String> {
    if value.is_empty()
        || value.len() > 1024
        || value.starts_with('-')
        || value.chars().any(char::is_control)
    {
        return Err("Enter a nonempty value without control characters or a leading dash (maximum 1024 bytes).".into());
    }
    Ok(())
}
fn name(value: &str) -> Result<(), String> {
    if value.is_empty()
        || value.len() > 128
        || !value.as_bytes()[0].is_ascii_alphanumeric()
        || !value
            .bytes()
            .all(|c| c.is_ascii_alphanumeric() || b"_.-".contains(&c))
    {
        return Err("Names must begin with a letter or digit and contain only letters, digits, dots, underscores or dashes (maximum 128 bytes).".into());
    }
    Ok(())
}
fn target(resource: Resource, target: &str) -> Result<(), String> {
    match resource {
        Resource::Volume => name(target),
        _ => {
            let raw = target.strip_prefix("sha256:").unwrap_or(target);
            if raw.len() != 64 || !raw.bytes().all(|c| c.is_ascii_hexdigit()) {
                return Err("Select a resource with a full engine ID, then retry.".into());
            }
            Ok(())
        }
    }
}
fn command_values(command: &[String]) -> Result<(), String> {
    if command.len() > 128 || command.iter().any(|v| v.len() > 8192 || v.contains('\0')) {
        return Err("Command exceeds argument limits or contains a NUL byte.".into());
    }
    Ok(())
}
impl Request {
    fn is_management_read(&self) -> bool {
        matches!(self, Self::ReadSettings | Self::ReadClusterVersions)
    }
    fn is_cluster(&self) -> bool {
        matches!(
            self,
            Self::Cluster { .. }
                | Self::ClusterLogs { .. }
                | Self::ClusterApply { .. }
                | Self::ClusterDelete { .. }
                | Self::ClusterWait { .. }
                | Self::ClusterForward { .. }
                | Self::ClusterExec { .. }
        )
    }
    fn plan(&self) -> Result<Plan, String> {
        let mut mutation = true;
        let mut seconds = 120;
        let mut input = None;
        let args: Vec<String> = match self {
            Self::PrepareUpdate => vec![],
            Self::RegistryList => {
                mutation = false;
                vec![]
            }
            Self::RegistryLogin {
                server,
                username,
                secret,
            } => {
                crate::registry::validate_login(server, username, secret)?;
                vec![]
            }
            Self::RegistryForget { server } => {
                crate::registry::validate_stored_server(server)?;
                vec![]
            }
            Self::DiskUsage => {
                mutation = false;
                vec!["system".into(), "df".into(), "--format=json".into()]
            }
            Self::Files {
                target: selected,
                path,
                action,
            } => {
                target(Resource::Container, selected)?;
                if !path.starts_with('/') || path.len() > 2048 || path.contains(['\0', '\\']) {
                    return Err("Use an absolute Linux container path (maximum 2048 bytes).".into());
                }
                mutation = false;
                seconds = 20;
                vec![
                    "files".into(),
                    selected.clone(),
                    path.clone(),
                    match action {
                        FileAction::List => "list",
                        FileAction::Read => "read",
                    }
                    .into(),
                ]
            }
            Self::BuildHistory { action, reference } => {
                mutation = false;
                let mut args = vec![
                    "buildx".into(),
                    "--builder=default".into(),
                    "history".into(),
                ];
                match action {
                    BuildHistoryAction::List => {
                        args.extend(["ls".into(), "--format=json".into(), "--no-trunc".into()])
                    }
                    BuildHistoryAction::Inspect | BuildHistoryAction::Logs => {
                        // The pinned Buildx lists builder/node/id but its default
                        // driver lookup accepts the leaf ID. No builder switching.
                        let leaf = reference.rsplit('/').next().unwrap_or("");
                        if leaf.is_empty()
                            || leaf.len() > 128
                            || !leaf
                                .bytes()
                                .all(|c| c.is_ascii_alphanumeric() || b"_-".contains(&c))
                        {
                            return Err("Select a valid returned build reference.".into());
                        }
                        if matches!(action, BuildHistoryAction::Inspect) {
                            args.extend(["inspect".into(), "--format=json".into(), leaf.into()]);
                        } else {
                            args.extend(["logs".into(), "--progress=plain".into(), leaf.into()]);
                        }
                    }
                }
                args
            }
            Self::Setup => {
                seconds = 1800;
                vec![]
            }
            Self::ClusterLogs {
                namespace,
                name: object,
                follow,
            } => {
                name(namespace)?;
                name(object)?;
                mutation = false;
                seconds = if *follow { 3600 } else { 60 };
                let mut args = vec![
                    "kubectl".into(),
                    "--namespace".into(),
                    namespace.clone(),
                    "--request-timeout=0".into(),
                    "logs".into(),
                    object.clone(),
                    "--all-containers=true".into(),
                    "--prefix=true".into(),
                    "--tail=500".into(),
                    "--timestamps=true".into(),
                ];
                if *follow {
                    args.push("--follow=true".into());
                }
                args
            }
            Self::ClusterForward {
                resource,
                namespace,
                name: object,
                local,
                remote,
            } => {
                name(namespace)?;
                name(object)?;
                mutation = false;
                seconds = 3600;
                if !matches!(resource, ClusterResource::Pods | ClusterResource::Services) {
                    return Err("Port forwarding requires a pod or service.".into());
                }
                let local = local
                    .parse::<u16>()
                    .map_err(|_| "Local port must be 0–65535 (0 allocates a port).")?;
                let remote = remote
                    .parse::<u16>()
                    .map_err(|_| "Remote port must be 1–65535.")?;
                if remote == 0 {
                    return Err("Remote port must be 1–65535.".into());
                }
                let ports = if local == 0 {
                    format!(":{remote}")
                } else {
                    format!("{local}:{remote}")
                };
                vec![
                    "kubectl".into(),
                    "--namespace".into(),
                    namespace.clone(),
                    "port-forward".into(),
                    "--address=127.0.0.1".into(),
                    format!("{}/{object}", resource.cli()),
                    ports,
                ]
            }
            Self::ClusterExec {
                namespace,
                name: object,
                command,
            } => {
                name(namespace)?;
                name(object)?;
                command_values(command)?;
                if command.is_empty() {
                    return Err("Enter a container executable and optional arguments.".into());
                }
                let mut args = vec![
                    "kubectl".into(),
                    "--namespace".into(),
                    namespace.clone(),
                    "exec".into(),
                    object.clone(),
                    "--".into(),
                ];
                args.extend(command.clone());
                args
            }
            Self::Untag { reference } => {
                value(reference)?;
                vec!["image".into(), "rm".into(), reference.clone()]
            }
            Self::Engine { running } => {
                seconds = 240;
                vec![
                    "set".into(),
                    format!("running={running}"),
                    "--timeout=210s".into(),
                ]
            }
            Self::ReadSettings => {
                mutation = false;
                vec![
                    "kubectl".into(),
                    "get".into(),
                    "apps/app".into(),
                    "--output=json".into(),
                    "--request-timeout=10s".into(),
                ]
            }
            Self::ReadClusterVersions => {
                mutation = false;
                vec![
                    "kubectl".into(),
                    "get".into(),
                    "configmap/k3s-versions".into(),
                    "--namespace=rancher-desktop".into(),
                    "--output=json".into(),
                    "--request-timeout=10s".into(),
                ]
            }
            Self::ConfigureCluster { enabled, version } => {
                seconds = 900;
                let mut args = vec![
                    "set".into(),
                    format!("kubernetes.enabled={enabled}"),
                    "--timeout=840s".into(),
                ];
                if let Some(version) = version {
                    let parts: Vec<_> = version.split('.').collect();
                    if parts.len() != 3
                        || parts.iter().any(|part| {
                            part.is_empty()
                                || part.len() > 4
                                || !part.bytes().all(|c| c.is_ascii_digit())
                        })
                    {
                        return Err(
                            "Select a concrete Kubernetes version from the backend catalog.".into(),
                        );
                    }
                    args.insert(2, format!("kubernetes.version={version}"));
                }
                args
            }
            Self::Cluster {
                action,
                resource,
                namespace,
                name: object,
            } => {
                mutation = false;
                let mut args = vec!["kubectl".into(), "--request-timeout=30s".into()];
                if !namespace.is_empty() {
                    name(namespace)?;
                    args.extend(["--namespace".into(), namespace.clone()]);
                }
                match action {
                    ClusterAction::List => {
                        args.extend(["get".into(), resource.cli().into(), "--output=json".into()]);
                        if namespace.is_empty() {
                            args.push("--all-namespaces".into());
                        }
                    }
                    ClusterAction::Inspect => {
                        name(namespace)?;
                        name(object)?;
                        args.extend([
                            "get".into(),
                            resource.cli().into(),
                            object.clone(),
                            "--output=json".into(),
                        ]);
                    }
                    ClusterAction::Logs => {
                        name(namespace)?;
                        name(object)?;
                        if !matches!(resource, ClusterResource::Pods) {
                            return Err("Logs require a pod.".into());
                        }
                        args.extend([
                            "logs".into(),
                            object.clone(),
                            "--all-containers=true".into(),
                            "--tail=500".into(),
                            "--timestamps=true".into(),
                        ]);
                    }
                }
                args
            }
            Self::ClusterApply { file } => {
                seconds = 180;
                vec![
                    "kubectl".into(),
                    "--request-timeout=120s".into(),
                    "apply".into(),
                    "--filename".into(),
                    host_path(file, false)?,
                ]
            }
            Self::ClusterDelete {
                resource,
                namespace,
                name: object,
                uid,
            } => {
                name(namespace)?;
                name(object)?;
                if uid.len() != 36 || !uid.bytes().all(|c| c.is_ascii_hexdigit() || c == b'-') {
                    return Err(
                        "Selected Kubernetes resource has no valid UID. Refresh before retrying."
                            .into(),
                    );
                }
                let api = if matches!(resource, ClusterResource::Deployments) {
                    "/apis/apps/v1"
                } else {
                    "/api/v1"
                };
                let path = format!("{api}/namespaces/{namespace}/{}/{object}", resource.cli());
                input = Some(serde_json::to_vec(&serde_json::json!({"apiVersion":"v1", "kind":"DeleteOptions", "preconditions":{"uid":uid}, "propagationPolicy":"Foreground"})).map_err(|e| e.to_string())?);
                vec![
                    "kubectl".into(),
                    "--request-timeout=60s".into(),
                    "delete".into(),
                    format!("--raw={path}"),
                    "--filename=-".into(),
                ]
            }
            Self::ClusterWait {
                resource,
                namespace,
                name: object,
                deleted,
            } => {
                name(namespace)?;
                name(object)?;
                mutation = false;
                seconds = 150;
                let condition = if *deleted {
                    "delete"
                } else {
                    match resource {
                        ClusterResource::Pods => "condition=Ready",
                        ClusterResource::Deployments => "condition=Available",
                        ClusterResource::PersistentVolumeClaims => "jsonpath={.status.phase}=Bound",
                        ClusterResource::Services | ClusterResource::Events => {
                            return Err("Services have no readiness condition.".into())
                        }
                    }
                };
                vec![
                    "kubectl".into(),
                    "--namespace".into(),
                    namespace.clone(),
                    "wait".into(),
                    format!("{}/{object}", resource.cli()),
                    format!("--for={condition}"),
                    "--timeout=120s".into(),
                ]
            }
            Self::Inspect {
                resource,
                target: id,
            } => {
                target(*resource, id)?;
                mutation = false;
                vec![resource.cli().into(), "inspect".into(), id.clone()]
            }
            Self::Container { action, target: id } => {
                target(Resource::Container, id)?;
                let verb = match action {
                    ContainerAction::Start => "start",
                    ContainerAction::Stop => "stop",
                    ContainerAction::Restart => "restart",
                    ContainerAction::Pause => "pause",
                    ContainerAction::Unpause => "unpause",
                };
                vec!["container".into(), verb.into(), id.clone()]
            }
            Self::Remove {
                resource,
                target: id,
            } => {
                target(*resource, id)?;
                vec![resource.cli().into(), "rm".into(), id.clone()]
            }
            Self::Logs { target: id, follow } => {
                target(Resource::Container, id)?;
                mutation = false;
                seconds = 600;
                let mut args = vec!["logs".into(), "--timestamps".into(), "--tail=500".into()];
                if *follow {
                    args.push("--follow".into());
                }
                args.push(id.clone());
                args
            }
            Self::Stats { target: id } => {
                target(Resource::Container, id)?;
                mutation = false;
                vec![
                    "stats".into(),
                    "--no-stream".into(),
                    "--format={{json .}}".into(),
                    id.clone(),
                ]
            }
            Self::Pull { image } => {
                value(image)?;
                seconds = 600;
                vec!["image".into(), "pull".into(), image.clone()]
            }
            Self::Tag { target: id, tag } => {
                target(Resource::Image, id)?;
                value(tag)?;
                vec!["image".into(), "tag".into(), id.clone(), tag.clone()]
            }
            Self::CreateVolume { name: n } => {
                name(n)?;
                vec!["volume".into(), "create".into(), n.clone()]
            }
            Self::CreateNetwork { name: n } => {
                name(n)?;
                vec!["network".into(), "create".into(), n.clone()]
            }
            Self::Run {
                image,
                name: n,
                command,
                options,
            } => {
                value(image)?;
                name(n)?;
                command_values(command)?;
                seconds = 600;
                let mut args = vec![
                    "container".into(),
                    "run".into(),
                    "--detach".into(),
                    "--name".into(),
                    n.clone(),
                ];
                if options.ports.len() + options.environment.len() + options.volumes.len() > 128 {
                    return Err("Too many container settings (maximum 128).".into());
                }
                for port in &options.ports {
                    args.extend(["--publish".into(), publish(port)?]);
                }
                for environment in &options.environment {
                    let (key, _) = environment
                        .split_once('=')
                        .ok_or("Environment entries must be NAME=value.")?;
                    if key.is_empty()
                        || !key.bytes().all(|c| c.is_ascii_alphanumeric() || c == b'_')
                        || environment.len() > 8192
                        || environment.contains('\0')
                    {
                        return Err("Invalid environment entry.".into());
                    }
                    args.extend(["--env".into(), environment.clone()]);
                }
                for volume in &options.volumes {
                    if crate::mounts::windows_source(volume) {
                        args.extend(["--mount".into(), crate::mounts::bind(volume)?]);
                        continue;
                    }
                    let parts: Vec<_> = volume.split(':').collect();
                    if !(2..=3).contains(&parts.len()) || (parts.len() == 3 && parts[2] != "ro") {
                        return Err(
                            "Use named-volume:/container/path or named-volume:/container/path:ro."
                                .into(),
                        );
                    }
                    name(parts[0])?;
                    if !parts[1].starts_with('/')
                        || parts[1].contains(',')
                        || parts[1].chars().any(char::is_control)
                    {
                        return Err("Volume targets must be absolute Linux paths without commas or control characters.".into());
                    }
                    args.extend([
                        "--mount".into(),
                        format!(
                            "type=volume,source={},target={}{}",
                            parts[0],
                            parts[1],
                            if parts.len() == 3 { ",readonly" } else { "" }
                        ),
                    ]);
                }
                args.extend(["--".into(), image.clone()]);
                args.extend(command.clone());
                args
            }
            Self::Exec {
                target: id,
                command,
            } => {
                target(Resource::Container, id)?;
                command_values(command)?;
                if command.is_empty() {
                    return Err("Enter an executable and optional arguments.".into());
                }
                let mut args = vec!["container".into(), "exec".into(), id.clone()];
                args.extend(command.clone());
                args
            }
            Self::Build { directory, tag } => {
                value(tag)?;
                let path = host_path(directory, true)?;
                seconds = 900;
                vec![
                    "buildx".into(),
                    "build".into(),
                    "--builder=default".into(),
                    "--load".into(),
                    "--progress=plain".into(),
                    "--tag".into(),
                    tag.clone(),
                    path,
                ]
            }
            Self::Compose {
                file,
                project,
                action,
            } => {
                name(project)?;
                let path = host_path(file, false)?;
                seconds = 600;
                let mut args = vec![
                    "compose".into(),
                    "--project-name".into(),
                    project.clone(),
                    "--file".into(),
                    path,
                ];
                args.extend(
                    match action {
                        ComposeAction::Up => vec!["up", "--detach", "--wait", "--wait-timeout=120"],
                        ComposeAction::Down => vec!["down", "--timeout=30"],
                        ComposeAction::Start => vec!["start", "--wait", "--wait-timeout=120"],
                        ComposeAction::Stop => vec!["stop", "--timeout=30"],
                        ComposeAction::Restart => vec!["restart", "--timeout=30"],
                        ComposeAction::Pull => vec!["pull"],
                        ComposeAction::Logs => {
                            mutation = false;
                            vec!["logs", "--no-color", "--tail=500"]
                        }
                    }
                    .into_iter()
                    .map(String::from),
                );
                args
            }
            Self::ConnectNetwork {
                network,
                container,
                disconnect,
            } => {
                target(Resource::Network, network)?;
                target(Resource::Container, container)?;
                vec![
                    "network".into(),
                    if *disconnect {
                        "disconnect".into()
                    } else {
                        "connect".into()
                    },
                    network.clone(),
                    container.clone(),
                ]
            }
        };
        Ok(Plan {
            args,
            mutation,
            seconds,
            input,
        })
    }
}
impl Resource {
    fn cli(self) -> &'static str {
        match self {
            Self::Container => "container",
            Self::Image => "image",
            Self::Volume => "volume",
            Self::Network => "network",
        }
    }
}
fn publish(raw: &str) -> Result<String, String> {
    let (mapping, protocol) = raw.rsplit_once('/').unwrap_or((raw, "tcp"));
    if !["tcp", "udp"].contains(&protocol) {
        return Err("Windows publication supports TCP and UDP.".into());
    }
    let parts: Vec<_> = mapping.split(':').collect();
    let (host, source, target) =
        match parts.as_slice() {
            [target] => ("127.0.0.1", "", *target),
            [source, target] => ("127.0.0.1", *source, *target),
            [host, source, target] => (*host, *source, *target),
            _ => return Err(
                "Use container-port, host-port:container-port or IPv4:host-port:container-port."
                    .into(),
            ),
        };
    let address = host
        .parse::<std::net::Ipv4Addr>()
        .map_err(|_| "Published host address must be IPv4.")?;
    if address != std::net::Ipv4Addr::LOCALHOST && address != std::net::Ipv4Addr::UNSPECIFIED {
        return Err(
            "Windows publication supports 127.0.0.1 or 0.0.0.0; host access is localhost only."
                .into(),
        );
    }
    if target.parse::<u16>().ok().filter(|p| *p > 0).is_none()
        || (!source.is_empty() && source.parse::<u16>().is_err())
    {
        return Err(
            "Ports must be in 1–65535; an empty or zero host port allocates a dynamic port.".into(),
        );
    }
    Ok(format!("{host}:{source}:{target}/{protocol}"))
}
fn host_path(raw: &str, directory: bool) -> Result<String, String> {
    value(raw)?;
    let path = std::path::Path::new(raw);
    if !path.is_absolute() {
        return Err("Select an absolute project path.".into());
    }
    let path = path
        .canonicalize()
        .map_err(|e| format!("Project path is unavailable: {e}"))?;
    if (directory && !path.is_dir()) || (!directory && !path.is_file()) {
        return Err("Project path has the wrong file type.".into());
    }
    path.to_str()
        .map(String::from)
        .ok_or("Project path is not valid Unicode".into())
}

#[derive(Serialize, Clone)]
#[serde(tag = "kind", rename_all = "camelCase")]
pub enum Output {
    Ready,
    Data {
        stream: &'static str,
        bytes: Vec<u8>,
    },
    Truncated,
}
#[derive(Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Outcome {
    pub exit_code: Option<i32>,
    pub interrupted: bool,
    pub mutation: bool,
    pub success: bool,
    pub message: String,
}

pub fn cancel(state: &Operations, id: &str) -> Result<(), String> {
    let mut jobs = state
        .0
        .lock()
        .map_err(|_| "Operation registry is unavailable")?;
    if let Some(job) = jobs.get_mut(id) {
        if let Some(sender) = job.cancel.take() {
            let _ = sender.send(());
        }
    }
    Ok(())
}

pub async fn run(
    state: &Operations,
    id: String,
    request: Request,
    output: Channel<Output>,
) -> Result<Outcome, String> {
    if id.len() != 36 || !id.bytes().all(|c| c.is_ascii_hexdigit() || c == b'-') {
        return Err("Invalid operation ID".into());
    }
    let plan = request.plan()?;
    let (sender, mut cancelled) = oneshot::channel();
    {
        let mut jobs = state
            .0
            .lock()
            .map_err(|_| "Operation registry is unavailable")?;
        if jobs.contains_key(&id)
            || jobs.len() >= 4
            || (plan.mutation && jobs.values().any(|job| job.mutation))
        {
            return Err(
                "Another operation is active. Wait for it to finish before retrying.".into(),
            );
        }
        jobs.insert(
            id.clone(),
            Running {
                cancel: Some(sender),
                mutation: plan.mutation,
            },
        );
    }
    let _lease = Lease { state, id };
    output
        .send(Output::Ready)
        .map_err(|_| "The output viewer disconnected")?;
    if matches!(request, Request::Setup) {
        return crate::installation::setup(&output, &mut cancelled).await;
    }
    if matches!(request, Request::PrepareUpdate) {
        return crate::installation::prepare_update(&output, &mut cancelled).await;
    }
    if matches!(
        request,
        Request::RegistryList | Request::RegistryLogin { .. } | Request::RegistryForget { .. }
    ) {
        return crate::registry::run(&request, &output, &mut cancelled).await;
    }
    let target = engine::target()?;
    if matches!(request, Request::Build { .. } | Request::Compose { .. }) {
        crate::runtime::configure_client(&target.config)?;
    }
    if matches!(request, Request::BuildHistory { .. }) {
        crate::runtime::validate_client(&target.config)?;
    }
    let management = matches!(
        request,
        Request::Engine { .. } | Request::ConfigureCluster { .. }
    ) || request.is_management_read()
        || request.is_cluster();
    let stopping = matches!(request, Request::Engine { running: false });
    let file_read = matches!(request, Request::Files { .. });
    let binary = if file_read {
        engine::check(&target).await?;
        target.session().await?
    } else if management {
        target.rdd().await?
    } else {
        engine::check(&target).await?;
        target.docker.clone()
    };
    // Cancellation before dispatch must never issue the mutation.
    if cancelled.try_recv().is_ok() {
        return Err("Cancelled before dispatch. No operation was sent.".into());
    }
    let mut command = Command::new(binary);
    if management {
        command
            .env("USERPROFILE", target.home())
            .env("HOME", target.home())
            .env("RDD_INSTANCE", "orbwin")
            .env("RDD_DEVELOPER_MODE", "1")
            .env("RDD_KEEP_LOGS", "1")
            .env_remove("RDD_LOG_DIR")
            .env_remove("LIMA_HOME")
            .env(
                "KUBECONFIG",
                target.home().join(".rdorbwin/kube-client.config"),
            );
        if request.is_cluster() {
            let config = target
                .home()
                .join("AppData/Local/rancher-desktop-orbwin/k3s.yaml");
            if !config.is_file() {
                return Err("The owned workload cluster has no mirrored kubeconfig. Enable Kubernetes and wait for setup to finish.".into());
            }
            // This file is generated by the owned guest probe. Never use the
            // management API's config, a global context, or a renderer path.
            command
                .arg("kubectl")
                .arg("--kubeconfig")
                .arg(config)
                .args(&plan.args[1..]);
        } else if request.is_management_read() {
            // `rdd ctl` would auto-start the management service. A read must
            // instead use the existing owned API config and fail if offline.
            command
                .arg("kubectl")
                .arg("--kubeconfig")
                .arg(
                    target
                        .home()
                        .join("AppData/Local/rancher-desktop-orbwin/config.yaml"),
                )
                .args(&plan.args[1..]);
        }
    } else if file_read {
        command
            .arg("files")
            .arg(target.daemon_id())
            .args(&plan.args[1..]);
    } else {
        command
            .env("PATH", crate::runtime::client_path()?)
            .arg("--config")
            .arg(&target.config)
            .arg("--host")
            .arg(engine::ENDPOINT);
    }
    if !file_read && !request.is_cluster() && !request.is_management_read() {
        command.args(&plan.args);
    }
    command
        .env("DOCKER_HOST", engine::ENDPOINT)
        .env("DOCKER_CONFIG", &target.config)
        .env_remove("DOCKER_CONTEXT")
        .env_remove("DOCKER_TLS_VERIFY")
        .env_remove("DOCKER_CERT_PATH")
        .env_remove("BUILDKIT_HOST")
        .env("BUILDX_BUILDER", "default")
        .env("BUILDX_CONFIG", target.config.join("buildx"))
        .stdin(if plan.input.is_some() {
            Stdio::piped()
        } else {
            Stdio::null()
        })
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .kill_on_drop(true);
    #[cfg(windows)]
    command.creation_flags(0x08000000);
    let client_result = run_client(
        command,
        plan.input.as_deref(),
        &output,
        &mut cancelled,
        plan.seconds,
        !matches!(
            request,
            Request::Engine { .. } | Request::ConfigureCluster { .. }
        ),
    )
    .await?;
    let client_result = if client_result.exit_code == Some(0)
        && !client_result.interrupted
        && matches!(request, Request::Engine { running: true })
    {
        // A freshly started management service can retain an already-true App
        // Settled condition when `running=true` changes no generation. Require
        // Lima's current-service startup, as first-run setup already does.
        // This is a bounded observation of rdd, never a second reconciler.
        output
            .send(Output::Data {
                stream: "stdout",
                bytes: b"Waiting for the current backend startup and guest connection...\n"
                    .to_vec(),
            })
            .map_err(|_| "The output viewer disconnected")?;
        let mut readiness = Command::new(target.rdd().await?);
        readiness
            .args([
                "ctl",
                "wait-condition",
                "limavms/orbwin",
                "Running=True",
                "--reason=Started",
                "--since=startup",
                "--namespace=rancher-desktop",
                "--timeout=180s",
            ])
            .env("USERPROFILE", target.home())
            .env("HOME", target.home())
            .env("RDD_INSTANCE", "orbwin")
            .env("RDD_DEVELOPER_MODE", "1")
            .env("RDD_KEEP_LOGS", "1")
            .env_remove("RDD_LOG_DIR")
            .env_remove("LIMA_HOME")
            .stdin(Stdio::null())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .kill_on_drop(true);
        #[cfg(windows)]
        readiness.creation_flags(0x08000000);
        run_client(readiness, None, &output, &mut cancelled, 190, false).await?
    } else {
        client_result
    };
    let ClientResult {
        exit_code,
        mut interrupted,
        mut reason,
    } = client_result;
    let identity = if stopping || request.is_management_read() {
        Ok(serde_json::Value::Null)
    } else {
        engine::check(&target).await
    };
    let publication_error = if identity.is_ok()
        && !interrupted
        && exit_code == Some(0)
        && matches!(
            request,
            Request::Engine { running: true } | Request::ConfigureCluster { .. }
        ) {
        output
            .send(Output::Data {
                stream: "stdout",
                bytes: b"Verifying Windows port publication (up to 30 seconds)...\n".to_vec(),
            })
            .map_err(|_| "The output viewer disconnected")?;
        match crate::readiness::publication(||engine::publication_failure(&target), &mut cancelled, Duration::from_secs(30), Duration::from_secs(1)).await {
            crate::readiness::Publication::Ready => None,
            crate::readiness::Publication::Unavailable(message) => Some(message),
            crate::readiness::Publication::Failed(error) => Some(format!("The backend changed, but Windows publication could not be verified: {error}. Refresh and inspect before retrying.")),
            crate::readiness::Publication::Cancelled => {
                interrupted=true;
                reason="Cancelled while verifying Windows publication".into();
                None
            },
        }
    } else {
        None
    };
    let success =
        identity.is_ok() && publication_error.is_none() && !interrupted && exit_code == Some(0);
    let message = if let Err(e) = identity {
        format!("Engine could not be verified after the operation: {e}. Refresh and inspect before retrying; the operation may have taken effect.")
    } else if let Some(error) = publication_error {
        error
    } else if interrupted {
        format!(
            "{reason}. {}",
            if plan.mutation {
                "The operation may already have taken effect. Refresh before retrying."
            } else {
                "The engine and workloads were not stopped."
            }
        )
    } else if exit_code == Some(0) {
        "Command completed.".into()
    } else {
        let code = exit_code
            .map(|code| code.to_string())
            .unwrap_or_else(|| "unknown".into());
        format!("Engine client exited with code {code}. Review output and refresh state before retrying.")
    };
    Ok(Outcome {
        exit_code,
        interrupted,
        mutation: plan.mutation,
        success,
        message,
    })
}
pub(crate) struct ClientResult {
    pub exit_code: Option<i32>,
    pub interrupted: bool,
    pub reason: String,
}

// One bounded runner for normal operations and first-run CLI requests. It owns
// only child clients. rdd/Moby retain lifecycle and persistence authority.
pub(crate) async fn client(
    command: Command,
    input: Option<&[u8]>,
    output: &Channel<Output>,
    cancelled: &mut oneshot::Receiver<()>,
    seconds: u64,
) -> Result<ClientResult, String> {
    run_client(command, input, output, cancelled, seconds, false).await
}

pub(crate) async fn client_tree(
    command: Command,
    input: Option<&[u8]>,
    output: &Channel<Output>,
    cancelled: &mut oneshot::Receiver<()>,
    seconds: u64,
) -> Result<ClientResult, String> {
    run_client(command, input, output, cancelled, seconds, true).await
}

async fn run_client(
    mut command: Command,
    input: Option<&[u8]>,
    output: &Channel<Output>,
    cancelled: &mut oneshot::Receiver<()>,
    seconds: u64,
    _contain_tree: bool,
) -> Result<ClientResult, String> {
    command
        .stdin(if input.is_some() {
            Stdio::piped()
        } else {
            Stdio::null()
        })
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .kill_on_drop(true);
    #[cfg(windows)]
    command.creation_flags(0x08000000);
    let mut child = command
        .spawn()
        .map_err(|e| format!("Could not start engine client: {e}"))?;
    #[cfg(windows)]
    let _job = if _contain_tree {
        match crate::client_job::ClientJob::attach(
            child.raw_handle().ok_or("Missing client process handle")?,
        ) {
            Ok(job) => Some(job),
            Err(error) => {
                let _ = child.kill().await;
                let _ = child.wait().await;
                return Err(error);
            }
        }
    } else {
        None
    };
    let stdin = child.stdin.take();
    let mut input_done = input.is_none();
    let write_input = async move {
        if let Some(input) = input {
            let mut stdin = stdin.ok_or("Missing client input")?;
            stdin
                .write_all(input)
                .await
                .map_err(|e| format!("Could not send operation input: {e}"))?;
        }
        Ok::<(), String>(())
    };
    tokio::pin!(write_input);
    let mut stdout = child.stdout.take().ok_or("Missing stdout")?;
    let mut stderr = child.stderr.take().ok_or("Missing stderr")?;
    let mut out = [0u8; 8192];
    let mut err = [0u8; 8192];
    let (mut out_open, mut err_open) = (true, true);
    let mut remaining = 2 * 1024 * 1024;
    let deadline = tokio::time::sleep(Duration::from_secs(seconds));
    tokio::pin!(deadline);
    let mut interrupted = false;
    let mut reason = String::new();
    let mut status = None;
    loop {
        tokio::select! {
            _ = &mut *cancelled => { interrupted = true; reason = "Client command cancelled".into(); break; }
            _ = &mut deadline => { interrupted = true; reason = "Client command reached its time limit".into(); break; }
            result = &mut write_input, if !input_done => {
                input_done = true;
                if let Err(error) = result { interrupted = true; reason = error; break; }
            }
            result = child.wait(), if status.is_none() => {
                match result { Ok(value) => status = Some(value), Err(e) => { interrupted = true; reason = format!("Could not wait for client: {e}"); break; } }
            }
            result = stdout.read(&mut out), if out_open => {
                match result { Ok(0) => out_open = false, Ok(n) => { if let Err(e) = send(output, "stdout", &out[..n], &mut remaining) { interrupted = true; reason = e; break; } }, Err(e) => { interrupted = true; reason = e.to_string(); break; } }
            }
            result = stderr.read(&mut err), if err_open => {
                match result { Ok(0) => err_open = false, Ok(n) => { if let Err(e) = send(output, "stderr", &err[..n], &mut remaining) { interrupted = true; reason = e; break; } }, Err(e) => { interrupted = true; reason = e.to_string(); break; } }
            }
        }
        if status.is_some() && !out_open && !err_open {
            break;
        }
    }
    if interrupted {
        let _ = child.kill().await;
        let _ = child.wait().await;
    }

    Ok(ClientResult {
        exit_code: status.and_then(|s| s.code()),
        interrupted,
        reason,
    })
}

fn send(
    channel: &Channel<Output>,
    stream: &'static str,
    bytes: &[u8],
    remaining: &mut usize,
) -> Result<(), String> {
    if *remaining == 0 {
        return Ok(());
    }
    let n = bytes.len().min(*remaining);
    *remaining -= n;
    channel
        .send(Output::Data {
            stream,
            bytes: bytes[..n].to_vec(),
        })
        .map_err(|_| "The output viewer disconnected".to_string())?;
    if *remaining == 0 {
        channel
            .send(Output::Truncated)
            .map_err(|_| "The output viewer disconnected".to_string())?;
    }
    Ok(())
}

#[cfg(test)]
#[path = "operations_qualification.rs"]
mod qualification;

#[cfg(test)]
mod tests {
    use super::*;
    const ID: &str = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
    #[cfg(windows)]
    #[tokio::test]
    async fn client_deadline_covers_a_blocked_stdin_write() {
        let root = std::env::var_os("SystemRoot").unwrap();
        let mut command = Command::new(
            std::path::PathBuf::from(root).join("System32/WindowsPowerShell/v1.0/powershell.exe"),
        );
        command.args([
            "-NoProfile",
            "-NonInteractive",
            "-Command",
            "Start-Sleep -Seconds 60",
        ]);
        let (_sender, mut cancelled) = oneshot::channel();
        let output = Channel::new(|_| Ok(()));
        let input = vec![b'x'; 2 * 1024 * 1024];
        let result = tokio::time::timeout(
            Duration::from_secs(5),
            client(command, Some(&input), &output, &mut cancelled, 1),
        )
        .await
        .expect("blocked stdin bypassed the deadline")
        .unwrap();
        assert!(result.interrupted);
        assert!(result.reason.contains("time limit"));
    }
    #[test]
    fn published_ports_are_local_by_default_and_validate_explicit_scope() {
        assert_eq!(publish("80").unwrap(), "127.0.0.1::80/tcp");
        assert_eq!(publish("8080:80").unwrap(), "127.0.0.1:8080:80/tcp");
        assert_eq!(
            publish("0.0.0.0:8080:80/udp").unwrap(),
            "0.0.0.0:8080:80/udp"
        );
        for port in [
            "0",
            "-1",
            "65536",
            "8080:0",
            "8080:80/unknown",
            "8080:80/sctp",
            "192.168.1.2:8080:80",
            "bad:8080:80",
        ] {
            assert!(publish(port).is_err(), "{port}");
        }
    }
    #[test]
    fn cluster_deletion_carries_uid_and_configuration_reads_do_not_autostart() {
        let uid = "00000000-0000-4000-8000-000000000001";
        let plan = Request::ClusterDelete {
            resource: ClusterResource::Deployments,
            namespace: "default".into(),
            name: "sample".into(),
            uid: uid.into(),
        }
        .plan()
        .unwrap();
        assert!(plan
            .args
            .contains(&"--raw=/apis/apps/v1/namespaces/default/deployments/sample".into()));
        let body: serde_json::Value = serde_json::from_slice(&plan.input.unwrap()).unwrap();
        assert_eq!(body["preconditions"]["uid"], uid);
        let read = Request::ReadSettings.plan().unwrap();
        assert_eq!(read.args[0], "kubectl");
        assert!(!read.mutation);
        let catalog = Request::ReadClusterVersions;
        assert!(catalog.is_management_read());
        assert!(!catalog.is_cluster());
        let read = catalog.plan().unwrap();
        assert!(!read.mutation);
        assert!(read.args.contains(&"configmap/k3s-versions".into()));
        assert!(read.args.contains(&"--namespace=rancher-desktop".into()));
        for version in ["latest", "1.34.6;command", "1.34", "-1.34.6", "1.34.6\n"] {
            assert!(Request::ConfigureCluster {
                enabled: true,
                version: Some(version.into())
            }
            .plan()
            .is_err());
        }
        let plan = Request::ConfigureCluster {
            enabled: true,
            version: Some("1.35.3".into()),
        }
        .plan()
        .unwrap();
        assert!(plan.args.contains(&"kubernetes.version=1.35.3".into()));
    }
    #[test]
    fn streaming_output_has_a_shared_budget_and_announces_truncation_once() {
        let events = std::sync::Arc::new(Mutex::new(Vec::<serde_json::Value>::new()));
        let captured = events.clone();
        let channel = Channel::new(move |body| {
            captured.lock().unwrap().push(body.deserialize()?);
            Ok(())
        });
        let mut remaining = 10;
        send(&channel, "stdout", b"12345678", &mut remaining).unwrap();
        send(&channel, "stderr", b"abcdef", &mut remaining).unwrap();
        send(&channel, "stdout", b"discarded", &mut remaining).unwrap();
        let events = events.lock().unwrap();
        assert_eq!(events.len(), 3);
        assert_eq!(events[1]["bytes"].as_array().unwrap().len(), 2);
        assert_eq!(events[2]["kind"], "truncated");
    }
    #[test]
    fn rejects_option_injection_and_partial_ids() {
        for n in ["", "--help", "../foreign", "a b", "a\nb"] {
            assert!(Request::CreateVolume { name: n.into() }.plan().is_err());
        }
        assert!(Request::Pull {
            image: "--all-tags".into()
        }
        .plan()
        .is_err());
        assert!(Request::Remove {
            resource: Resource::Container,
            target: "short-id".into()
        }
        .plan()
        .is_err());
    }
    #[test]
    fn file_reads_are_bounded_nonmutating_and_reject_host_paths() {
        for path in ["relative", "C:\\Windows", "/bad\0file", "/bad\\file"] {
            assert!(Request::Files {
                target: ID.into(),
                path: path.into(),
                action: FileAction::List
            }
            .plan()
            .is_err());
        }
        let plan = Request::Files {
            target: ID.into(),
            path: "/space ü/file".into(),
            action: FileAction::Read,
        }
        .plan()
        .unwrap();
        assert!(!plan.mutation);
        assert_eq!(plan.seconds, 20);
        assert_eq!(plan.args, ["files", ID, "/space ü/file", "read"]);
    }
    #[test]
    fn destructive_operations_never_force_or_remove_attached_volumes() {
        let plan = Request::Remove {
            resource: Resource::Container,
            target: ID.into(),
        }
        .plan()
        .unwrap();
        assert_eq!(plan.args, ["container", "rm", ID]);
        assert!(plan.mutation);
        assert!(Request::Remove {
            resource: Resource::Image,
            target: format!("sha256:{ID}")
        }
        .plan()
        .is_ok());
    }
    #[test]
    fn arbitrary_container_commands_are_separate_arguments_without_host_shell() {
        let command = vec!["printf".into(), "$(touch /host); hello".into()];
        let plan = Request::Exec {
            target: ID.into(),
            command: command.clone(),
        }
        .plan()
        .unwrap();
        assert_eq!(&plan.args[3..], &command);
        assert!(Request::Exec {
            target: ID.into(),
            command: vec![]
        }
        .plan()
        .is_err());
    }
    #[test]
    fn lease_and_cancellation_release_registry() {
        let state = Operations::default();
        let (sender, mut receiver) = oneshot::channel();
        state.0.lock().unwrap().insert(
            "test".into(),
            Running {
                cancel: Some(sender),
                mutation: true,
            },
        );
        cancel(&state, "test").unwrap();
        assert!(receiver.try_recv().is_ok());
        assert!(state.0.lock().unwrap().contains_key("test"));
        let (sender, _) = oneshot::channel();
        state.0.lock().unwrap().insert(
            "test".into(),
            Running {
                cancel: Some(sender),
                mutation: true,
            },
        );
        drop(Lease {
            state: &state,
            id: "test".into(),
        });
        assert!(state.0.lock().unwrap().is_empty());
    }
}
