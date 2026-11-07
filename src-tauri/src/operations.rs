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
use std::{
    collections::HashMap,
    process::Stdio,
    sync::{Arc, Mutex},
    time::Duration,
};
use tauri::ipc::Channel;
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt},
    process::Command,
    sync::oneshot,
};

#[derive(Default, Clone)]
pub struct Operations(Arc<Mutex<HashMap<String, Running>>>);
struct Running {
    cancel: Option<oneshot::Sender<()>>,
    mutation: bool,
    setup: bool,
    decision: Option<PendingSetupDecision>,
}
struct PendingSetupDecision {
    nonce: String,
    sender: oneshot::Sender<String>,
}

#[derive(Clone)]
pub(crate) struct SetupDecisionControl {
    state: Operations,
    id: String,
}
pub(crate) struct SetupDecisionWait {
    control: SetupDecisionControl,
    nonce: String,
    receiver: oneshot::Receiver<String>,
}
fn setup_decision_nonce(nonce: &str) -> bool {
    nonce.len() == 32
        && nonce
            .bytes()
            .all(|c| c.is_ascii_digit() || (b'a'..=b'f').contains(&c))
}
fn setup_operation_id(id: &str) -> bool {
    id.len() == 36
        && id.bytes().enumerate().all(|(i, c)| {
            if [8, 13, 18, 23].contains(&i) {
                c == b'-'
            } else {
                c.is_ascii_digit() || (b'a'..=b'f').contains(&c)
            }
        })
}
impl SetupDecisionControl {
    pub(crate) fn pending(&self, nonce: &str) -> Result<SetupDecisionWait, String> {
        if !setup_operation_id(&self.id) || !setup_decision_nonce(nonce) {
            return Err("Invalid setup decision.".into());
        }
        let (sender, receiver) = oneshot::channel();
        let mut jobs = self
            .state
            .0
            .lock()
            .map_err(|_| "Operation registry is unavailable")?;
        let job = jobs.get_mut(&self.id).ok_or("Setup is no longer active")?;
        if !job.setup || job.cancel.is_none() || job.decision.is_some() {
            return Err("Setup is not waiting for this decision.".into());
        }
        job.decision = Some(PendingSetupDecision {
            nonce: nonce.into(),
            sender,
        });
        Ok(SetupDecisionWait {
            control: self.clone(),
            nonce: nonce.into(),
            receiver,
        })
    }
}
impl SetupDecisionWait {
    pub(crate) async fn response(&mut self) -> Result<String, String> {
        (&mut self.receiver)
            .await
            .map_err(|_| "Setup decision was closed.".into())
    }
}
impl Drop for SetupDecisionWait {
    fn drop(&mut self) {
        if let Ok(mut jobs) = self.control.state.0.lock() {
            if let Some(job) = jobs.get_mut(&self.control.id) {
                if job.decision.as_ref().is_some_and(|d| d.nonce == self.nonce) {
                    job.decision.take();
                }
            }
        }
    }
}

pub fn setup_decide(state: &Operations, id: &str, nonce: &str) -> Result<(), String> {
    if !setup_operation_id(id) || !setup_decision_nonce(nonce) {
        return Err("Invalid setup decision.".into());
    }
    let sender = {
        let mut jobs = state
            .0
            .lock()
            .map_err(|_| "Operation registry is unavailable")?;
        let job = jobs.get_mut(id).ok_or("Setup is no longer active")?;
        if !job.setup
            || job.cancel.is_none()
            || job.decision.as_ref().is_none_or(|d| d.nonce != nonce)
        {
            return Err("This setup decision is no longer available.".into());
        }
        job.decision
            .take()
            .ok_or("Setup decision was already used")?
            .sender
    };
    sender
        .send(nonce.into())
        .map_err(|_| "Setup stopped waiting for this decision.".into())
}
#[derive(Clone)]
pub(crate) struct Lease {
    _owner: Arc<LeaseOwner>,
}
struct LeaseOwner {
    state: Operations,
    id: String,
}
impl Drop for LeaseOwner {
    fn drop(&mut self) {
        if let Ok(mut jobs) = self.state.0.lock() {
            jobs.remove(&self.id);
        }
    }
}

#[derive(Debug, serde::Deserialize, serde::Serialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct WslMappingReview {
    file: String,
    sha256: String,
    contents: String,
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
    VolumeSize {
        target: String,
        #[serde(rename = "createdAt")]
        created_at: String,
    },
    RecoverTools,
    WslCompose {
        #[serde(default, rename = "reviewedDaemon")]
        reviewed_daemon: Option<String>,
        #[serde(default, rename = "mappingReview")]
        mapping_review: Option<WslMappingReview>,
        distribution: String,
        #[serde(default)]
        connection: Option<crate::wsl_integration::ConnectionReview>,
        file: String,
        #[serde(default)]
        overrides: Vec<String>,
        #[serde(default, rename = "envFiles")]
        env_files: Vec<String>,
        project: String,
        #[serde(default)]
        directory: String,
        #[serde(default)]
        profiles: Vec<String>,
        #[serde(default)]
        services: Vec<String>,
        action: ComposeAction,
    },
    WslBuild {
        distribution: String,
        #[serde(default)]
        connection: Option<crate::wsl_integration::ConnectionReview>,
        directory: String,
        tag: String,
    },
    WslSaveMapping {
        distribution: String,
        connection: crate::wsl_integration::ConnectionReview,
        file: String,
        contents: String,
        target: String,
    },
    VolumeTransfers,
    VolumeClones,
    DiscardCloneStaging {
        transaction: String,
    },
    CloneVolume {
        volume: String,
        name: String,
    },
    DiscardVolumeTransfer {
        transaction: String,
    },
    ExportVolume {
        volume: String,
        destination: String,
    },
    ExportImage {
        image: String,
        reference: String,
        destination: String,
    },
    ReviewImageImport {
        source: String,
    },
    ReviewDesktopMigration,
    MigrationWslSources,
    ReviewRancherMigration {
        registration: String,
        #[serde(default)]
        machine: String,
    },
    ExportRancherMigrationImage {
        source: String,
        registration: String,
        machine: String,
        image: String,
        reference: String,
        destination: String,
    },
    ReviewDesktopMigrationVolume {
        source: String,
        volume: String,
    },
    ReviewRancherMigrationVolume {
        source: String,
        registration: String,
        machine: String,
        volume: String,
    },
    ExportRancherMigrationVolume {
        source: String,
        registration: String,
        machine: String,
        volume: String,
        fingerprint: String,
        destination: String,
    },
    ExportDesktopMigrationVolume {
        source: String,
        volume: String,
        fingerprint: String,
        destination: String,
    },
    RecoverDesktopMigrationVolume {
        transaction: String,
    },
    ExportDesktopMigrationImage {
        source: String,
        image: String,
        reference: String,
        destination: String,
    },
    ImportImages {
        source: String,
        sha256: String,
        tags: Vec<ImageImportChoice>,
    },
    ImageImportHistory,
    MigrationVolumeHistory,
    MigrationImageHistory,
    ReviewMigrationDestination {
        source: String,
        #[serde(rename = "sourceVolume")]
        source_volume: String,
        destination: String,
    },
    ImageImportRecord {
        transaction: String,
    },
    RestoreVolume {
        source: String,
        name: String,
    },
    SaveVolumeFile {
        volume: String,
        path: String,
        expected: String,
        text: String,
    },
    CopyFile {
        resource: FileResource,
        target: String,
        path: String,
        destination: String,
    },
    Files {
        #[serde(default)]
        resource: FileResource,
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
    ConfigureEngineProxy {
        review: crate::engine_proxy::Change,
    },
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
    EngineStats,
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
        #[serde(default)]
        driver: String,
        #[serde(default)]
        subnet: String,
        #[serde(default)]
        internal: bool,
    },
    Run {
        image: String,
        name: String,
        command: Vec<String>,
        #[serde(default)]
        options: RunOptions,
    },
    UpdateContainerLimits {
        target: String,
        cpus: String,
        #[serde(rename = "memoryMiB")]
        memory_mib: String,
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
        #[serde(default)]
        overrides: Vec<String>,
        #[serde(default, rename = "envFiles")]
        env_files: Vec<String>,
        #[serde(default)]
        profiles: Vec<String>,
        #[serde(default)]
        services: Vec<String>,
        #[serde(default)]
        directory: String,
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
#[serde(rename_all = "camelCase")]
pub enum FileResource {
    #[default]
    Container,
    Image,
    Volume,
}
#[derive(Deserialize, Debug, Default)]
#[serde(default, deny_unknown_fields)]
pub struct RunOptions {
    ports: Vec<String>,
    environment: Vec<String>,
    volumes: Vec<String>,
    restart: String,
    network: String,
    cpus: String,
    #[serde(rename = "memoryMiB")]
    memory_mib: String,
}
#[derive(Deserialize, Serialize, Debug)]
#[serde(rename_all = "camelCase")]
pub enum ComposeAction {
    Validate,
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
#[derive(Deserialize, Serialize, Debug, Clone)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub struct ImageImportChoice {
    reference: String,
    config_id: String,
    expected_id: String,
    apply: bool,
    replace: bool,
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

// Bounded decimal limits; do not coerce malformed/zero inputs into defaults.
// Blank means omit on Run, leave unchanged on Update. Memory is a hard RAM
// limit; swap policy remains Docker-owned and can cause an update to refuse.
fn container_limit_args(cpus: &str, memory_mib: &str) -> Result<Vec<String>, String> {
    let mut args = Vec::new();
    if !cpus.is_empty() {
        if cpus.len() > 9 {
            return Err(
                "CPU limit must be 0.01 to 1024, with at most three decimal places.".into(),
            );
        }
        let parts: Vec<_> = cpus.split('.').collect();
        if parts.len() > 2
            || parts[0].is_empty()
            || !parts[0].bytes().all(|c| c.is_ascii_digit())
            || (parts[0].len() > 1 && parts[0].starts_with('0'))
            || (parts.len() == 2
                && (parts[1].is_empty()
                    || parts[1].len() > 3
                    || !parts[1].bytes().all(|c| c.is_ascii_digit())))
        {
            return Err("CPU limit must be a decimal number with at most three decimal places, for example 0.5.".into());
        }
        let whole = parts[0].parse::<u64>().map_err(|_| "Invalid CPU limit.")?;
        let fraction = if parts.len() == 2 {
            format!("{:0<3}", parts[1])
                .parse::<u64>()
                .map_err(|_| "Invalid CPU limit.")?
        } else {
            0
        };
        let milli = whole
            .checked_mul(1000)
            .and_then(|v| v.checked_add(fraction))
            .ok_or("CPU limit exceeds its range.")?;
        if !(10..=1_024_000).contains(&milli) {
            return Err("CPU limit must be between 0.01 and 1024 CPUs.".into());
        }
        args.extend(["--cpus".into(), cpus.into()]);
    }
    if !memory_mib.is_empty() {
        if memory_mib.len() > 7
            || !memory_mib.bytes().all(|c| c.is_ascii_digit())
            || memory_mib.starts_with('0')
        {
            return Err("Memory limit must be a whole number of MiB, from 6 to 1048576.".into());
        }
        let mib = memory_mib
            .parse::<u64>()
            .map_err(|_| "Invalid memory limit.")?;
        if !(6..=1_048_576).contains(&mib) {
            return Err("Memory limit must be between 6 and 1048576 MiB.".into());
        }
        args.extend(["--memory".into(), (mib * 1024 * 1024).to_string()]);
    }
    Ok(args)
}
impl Request {
    fn is_management_read(&self) -> bool {
        matches!(self, Self::ReadSettings | Self::ReadClusterVersions)
    }
    fn is_management_api(&self) -> bool {
        self.is_management_read() || matches!(self, Self::ConfigureEngineProxy { .. })
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
            Self::VolumeSize {
                target: id,
                created_at,
            } => {
                target(Resource::Volume, id)?;
                if created_at.is_empty()
                    || created_at.len() > 128
                    || created_at.chars().any(char::is_control)
                {
                    return Err("Select a current volume creation identity.".into());
                }
                mutation = false;
                seconds = 40;
                vec!["volume-size".into(), id.clone(), created_at.clone()]
            }
            Self::RecoverTools => {
                seconds = 30;
                vec!["recover-tools".into()]
            }
            Self::WslCompose {
                reviewed_daemon,
                mapping_review,
                distribution,
                connection,
                file,
                overrides,
                env_files,
                project,
                directory,
                profiles,
                services,
                action,
            } => {
                value(distribution)?;
                if let Some(review) = connection {
                    review.validate()?;
                }
                name(project)?;
                if reviewed_daemon.is_some() || mapping_review.is_some() {
                    let expected = reviewed_daemon
                        .as_ref()
                        .ok_or("A saved mapping requires its reviewed target.")?;
                    value(expected)?;
                    if connection
                        .as_ref()
                        .and_then(|c| c.registration.as_ref())
                        .is_none()
                        || !matches!(action, ComposeAction::Validate | ComposeAction::Up)
                    {
                        return Err(
                            "Migration drafts require a reviewed registration and Validate/Up."
                                .into(),
                        );
                    }
                }
                let mapping_review = if let Some(review) = mapping_review {
                    if review.file.is_empty()
                        || review.file.len() > 240
                        || review.file.contains(['/', '\\'])
                        || review.file.chars().any(char::is_control)
                        || !review.file.to_ascii_lowercase().ends_with(".json")
                        || review.sha256.len() != 64
                        || !review
                            .sha256
                            .bytes()
                            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
                    {
                        return Err("Invalid bounded saved WSL mapping review.".into());
                    }
                    Some(
                        serde_json::json!({"file":review.file,"sha256":review.sha256,"contents":crate::migration_mapping::canonical(&review.contents)?}),
                    )
                } else {
                    None
                };
                if overrides.len() > 15
                    || env_files.len() > 16
                    || profiles.len() > 32
                    || services.len() > 64
                {
                    return Err("WSL Compose inputs exceed their limits.".into());
                }
                for path in std::iter::once(file)
                    .chain(overrides)
                    .chain(env_files)
                    .chain(std::iter::once(directory))
                {
                    if path.len() > 4096
                        || path.chars().any(char::is_control)
                        || path.contains('\\')
                    {
                        return Err("Use original Linux project paths or paths relative to the selected root.".into());
                    }
                }
                if file.is_empty() || env_files.iter().any(String::is_empty) {
                    return Err("Select the WSL project's Compose file.".into());
                }
                for selection in profiles.iter().chain(services) {
                    name(selection)?;
                }
                if matches!(action, ComposeAction::Validate | ComposeAction::Down)
                    && !services.is_empty()
                {
                    return Err("WSL Down/Validate cannot be scoped to services.".into());
                }
                mutation = !matches!(action, ComposeAction::Validate | ComposeAction::Logs);
                seconds = 1815;
                input=Some(serde_json::to_vec(&serde_json::json!({"kind":"compose","reviewedDaemon":reviewed_daemon,"mappingReview":mapping_review,"connection":connection,"file":file,"overrides":overrides,"envFiles":env_files,"project":project,"directory":directory,"profiles":profiles,"services":services,"action":action})).map_err(|error|error.to_string())?);
                vec![]
            }
            Self::WslSaveMapping {
                distribution,
                connection,
                file,
                contents,
                target,
            } => {
                value(distribution)?;
                connection.validate()?;
                if connection.registration.is_none() {
                    return Err(
                        "Inspect the project distribution registration before saving.".into(),
                    );
                }
                if file.is_empty()
                    || file.len() > 240
                    || file.contains(['/', '\\'])
                    || file.chars().any(char::is_control)
                    || !file.to_ascii_lowercase().ends_with(".json")
                {
                    return Err("Choose a new JSON filename in the connected project root, without directories.".into());
                }
                value(target)?;
                let mapping = crate::migration_mapping::canonical(contents)?;
                seconds = 50;
                input = Some(serde_json::to_vec(&serde_json::json!({"kind":"mapping","connection":connection,"file":file,"mapping":mapping,"reviewedDaemon":target})).map_err(|error|error.to_string())?);
                vec![]
            }
            Self::WslBuild {
                distribution,
                connection,
                directory,
                tag,
            } => {
                value(distribution)?;
                if let Some(review) = connection {
                    review.validate()?;
                }
                value(tag)?;
                if directory.len() > 4096
                    || directory.chars().any(char::is_control)
                    || directory.contains('\\')
                    || tag.starts_with('-')
                {
                    return Err("Select a Linux project context and explicit image tag.".into());
                }
                seconds = 1815;
                input = Some(
                    serde_json::to_vec(
                        &serde_json::json!({"kind":"build","connection":connection,"directory":directory,"tag":tag}),
                    )
                    .map_err(|error| error.to_string())?,
                );
                vec![]
            }
            Self::VolumeTransfers => {
                mutation = false;
                seconds = 30;
                vec!["volume-transfers".into()]
            }
            Self::DiscardVolumeTransfer { transaction } => {
                if transaction.len() != 32
                    || !transaction
                        .bytes()
                        .all(|byte| byte.is_ascii_hexdigit() && !byte.is_ascii_uppercase())
                {
                    return Err("Select a recorded pending transfer.".into());
                }
                seconds = 45;
                vec!["discard-volume-transfer".into(), transaction.clone()]
            }
            Self::ReviewDesktopMigration => {
                mutation = false;
                seconds = 105;
                vec!["migration-review".into(), "docker-desktop".into()]
            }
            Self::MigrationWslSources => {
                mutation = false;
                seconds = 20;
                vec!["migration-wsl-sources".into()]
            }
            Self::ReviewRancherMigration {
                registration,
                machine,
            } => {
                migration_wsl_identity(registration, machine, true)?;
                mutation = false;
                seconds = 105;
                vec![
                    "migration-rancher-review".into(),
                    registration.clone(),
                    machine.clone(),
                ]
            }
            Self::ExportRancherMigrationImage {
                source,
                registration,
                machine,
                image,
                reference,
                destination,
            } => {
                migration_wsl_identity(registration, machine, false)?;
                value(source)?;
                if source.len() > 256 {
                    return Err("Select the reviewed Rancher source.".into());
                }
                target(Resource::Image, image)?;
                value(reference)?;
                if reference.len() > 512
                    || !std::path::Path::new(destination)
                        .extension()
                        .and_then(|extension| extension.to_str())
                        .is_some_and(|extension| extension.eq_ignore_ascii_case("tar"))
                {
                    return Err("Choose a reviewed source image and new .tar destination.".into());
                }
                mutation = false;
                seconds = 1815;
                vec![
                    "migration-rancher-image-export".into(),
                    source.clone(),
                    registration.clone(),
                    machine.clone(),
                    image.clone(),
                    reference.clone(),
                    host_destination(destination)?,
                ]
            }
            Self::ReviewRancherMigrationVolume {
                source,
                registration,
                machine,
                volume,
            } => {
                migration_wsl_identity(registration, machine, false)?;
                value(source)?;
                name(volume)?;
                if source.len() > 256 {
                    return Err("Select the reviewed source daemon.".into());
                }
                mutation = false;
                seconds = 105;
                vec![
                    "migration-rancher-volume-review".into(),
                    source.clone(),
                    registration.clone(),
                    machine.clone(),
                    volume.clone(),
                ]
            }
            Self::ExportRancherMigrationVolume {
                source,
                registration,
                machine,
                volume,
                fingerprint,
                destination,
            } => {
                migration_wsl_identity(registration, machine, false)?;
                value(source)?;
                name(volume)?;
                if source.len() > 256
                    || fingerprint.len() != 64
                    || !fingerprint
                        .bytes()
                        .all(|byte| byte.is_ascii_hexdigit() && !byte.is_ascii_uppercase())
                {
                    return Err("Review the selected source volume before export.".into());
                }
                mutation = false;
                seconds = 1815;
                vec![
                    "migration-rancher-volume-export".into(),
                    source.clone(),
                    registration.clone(),
                    machine.clone(),
                    volume.clone(),
                    fingerprint.clone(),
                    host_destination(destination)?,
                ]
            }
            Self::ReviewDesktopMigrationVolume { source, volume } => {
                value(source)?;
                name(volume)?;
                if source.len() > 256 {
                    return Err("Select the reviewed source daemon.".into());
                }
                mutation = false;
                seconds = 105;
                vec![
                    "migration-volume-review".into(),
                    "docker-desktop".into(),
                    source.clone(),
                    volume.clone(),
                ]
            }
            Self::ExportDesktopMigrationVolume {
                source,
                volume,
                fingerprint,
                destination,
            } => {
                value(source)?;
                name(volume)?;
                if source.len() > 256
                    || fingerprint.len() != 64
                    || !fingerprint
                        .bytes()
                        .all(|byte| byte.is_ascii_hexdigit() && !byte.is_ascii_uppercase())
                {
                    return Err("Review the selected source volume before export.".into());
                }
                mutation = false;
                seconds = 1815;
                vec![
                    "migration-volume-export".into(),
                    "docker-desktop".into(),
                    source.clone(),
                    volume.clone(),
                    fingerprint.clone(),
                    host_destination(destination)?,
                ]
            }
            Self::RecoverDesktopMigrationVolume { transaction } => {
                if transaction.len() != 32
                    || !transaction
                        .bytes()
                        .all(|byte| byte.is_ascii_hexdigit() && !byte.is_ascii_uppercase())
                {
                    return Err("Enter the exact recorded source export transaction ID.".into());
                }
                mutation = false;
                seconds = 60;
                vec!["migration-volume-recover".into(), transaction.clone()]
            }
            Self::ExportDesktopMigrationImage {
                source,
                image,
                reference,
                destination,
            } => {
                value(source)?;
                if source.len() > 256 {
                    return Err("Select the reviewed source daemon.".into());
                }
                target(Resource::Image, image)?;
                value(reference)?;
                if reference.len() > 512
                    || !std::path::Path::new(destination)
                        .extension()
                        .and_then(|extension| extension.to_str())
                        .is_some_and(|extension| extension.eq_ignore_ascii_case("tar"))
                {
                    return Err(
                        "Select a reviewed image reference and new .tar destination.".into(),
                    );
                }
                mutation = false;
                seconds = 1815;
                vec![
                    "migration-image-export".into(),
                    "docker-desktop".into(),
                    source.clone(),
                    image.clone(),
                    reference.clone(),
                    host_destination(destination)?,
                ]
            }
            Self::ReviewImageImport { source } => {
                mutation = false;
                seconds = 1815;
                vec!["image-import-review".into(), image_archive_source(source)?]
            }
            Self::ImportImages {
                source,
                sha256,
                tags,
            } => {
                if sha256.len() != 64
                    || !sha256.bytes().all(|byte| byte.is_ascii_hexdigit())
                    || tags.len() > 512
                {
                    return Err(
                        "Review the image archive and bounded tag choices before importing.".into(),
                    );
                }
                let source = image_archive_source(source)?;
                input = Some(
                    serde_json::to_vec(
                        &serde_json::json!({"source":source,"sha256":sha256,"tags":tags}),
                    )
                    .map_err(|error| error.to_string())?,
                );
                if input.as_ref().is_some_and(|input| input.len() > 512 * 1024) {
                    return Err("Image import review exceeds its input budget.".into());
                }
                seconds = 1815;
                vec!["import-images".into()]
            }
            Self::MigrationVolumeHistory => {
                mutation = false;
                seconds = 30;
                vec!["migration-volume-history".into()]
            }
            Self::MigrationImageHistory => {
                mutation = false;
                seconds = 30;
                vec!["migration-image-history".into()]
            }
            Self::ReviewMigrationDestination {
                source,
                source_volume,
                destination,
            } => {
                value(source)?;
                if source.len() > 256 {
                    return Err("Select the reviewed source daemon.".into());
                }
                name(source_volume)?;
                name(destination)?;
                mutation = false;
                seconds = 40;
                vec![
                    "migration-destination-review".into(),
                    source.clone(),
                    source_volume.clone(),
                    destination.clone(),
                ]
            }
            Self::ImageImportHistory => {
                mutation = false;
                seconds = 30;
                vec!["image-import-history".into()]
            }
            Self::ImageImportRecord { transaction } => {
                if transaction.len() != 32
                    || !transaction
                        .bytes()
                        .all(|byte| byte.is_ascii_hexdigit() && !byte.is_ascii_uppercase())
                {
                    return Err("Select a recorded image import transaction.".into());
                }
                mutation = false;
                seconds = 30;
                vec!["image-import-record".into(), transaction.clone()]
            }
            Self::ExportImage {
                image,
                reference,
                destination,
            } => {
                target(Resource::Image, image)?;
                value(reference)?;
                if !std::path::Path::new(destination)
                    .extension()
                    .and_then(|extension| extension.to_str())
                    .is_some_and(|extension| extension.eq_ignore_ascii_case("tar"))
                {
                    return Err("Choose a new .tar image archive destination.".into());
                }
                mutation = false;
                seconds = 1815;
                vec![
                    "image-archive".into(),
                    "export".into(),
                    image.clone(),
                    reference.clone(),
                    host_destination(destination)?,
                ]
            }
            Self::ExportVolume {
                volume,
                destination,
            } => {
                name(volume)?;
                mutation = false;
                seconds = 1815;
                vec![
                    "volume-transfer".into(),
                    "export".into(),
                    volume.clone(),
                    host_destination(destination)?,
                ]
            }
            Self::VolumeClones => {
                mutation = false;
                seconds = 20;
                vec!["volume-clones".into()]
            }
            Self::DiscardCloneStaging { transaction } => {
                if transaction.len() != 32
                    || !transaction
                        .chars()
                        .all(|value| value.is_ascii_hexdigit() && !value.is_ascii_uppercase())
                {
                    return Err("Select a recorded clone snapshot.".into());
                }
                seconds = 45;
                vec!["discard-clone-staging".into(), transaction.clone()]
            }
            Self::CloneVolume {
                volume,
                name: selected,
            } => {
                name(volume)?;
                name(selected)?;
                if volume == selected {
                    return Err("Choose a distinct new volume name for the clone.".into());
                }
                seconds = 3630;
                vec![
                    "volume-transfer".into(),
                    "clone".into(),
                    volume.clone(),
                    selected.clone(),
                ]
            }
            Self::RestoreVolume {
                source,
                name: selected,
            } => {
                name(selected)?;
                let source =
                    crate::runtime::cli_path(std::path::Path::new(&host_path(source, false)?))?;
                let source = source.to_str().ok_or("Package path is not valid Unicode")?;
                if !crate::mounts::windows_source(source) {
                    return Err("Choose a portable package on a local Windows drive.".into());
                }
                seconds = 1815;
                vec![
                    "volume-transfer".into(),
                    "restore".into(),
                    source.into(),
                    selected.clone(),
                ]
            }
            Self::SaveVolumeFile {
                volume,
                path,
                expected,
                text,
            } => {
                name(volume)?;
                if !path.starts_with('/')
                    || path.len() > 2048
                    || path.contains(['\0', '\\'])
                    || expected.len() != 64
                    || !expected.bytes().all(|byte| byte.is_ascii_hexdigit())
                    || text.len() > 256 * 1024
                    || text.contains('\0')
                {
                    return Err(
                        "Select a bounded text file with its preview version before saving.".into(),
                    );
                }
                seconds = 90;
                input=Some(serde_json::to_vec(&serde_json::json!({"volume":volume,"path":path,"expected":expected,"text":text})).map_err(|error|error.to_string())?);
                vec!["save-volume-file".into()]
            }
            Self::CopyFile {
                resource,
                target: selected,
                path,
                destination,
            } => {
                target(
                    match resource {
                        FileResource::Container => Resource::Container,
                        FileResource::Image => Resource::Image,
                        FileResource::Volume => Resource::Volume,
                    },
                    selected,
                )?;
                if !path.starts_with('/') || path.len() > 2048 || path.contains(['\0', '\\']) {
                    return Err("Select an absolute Linux file path.".into());
                }
                mutation = false;
                seconds = 615;
                vec![
                    "files".into(),
                    match resource {
                        FileResource::Container => "container",
                        FileResource::Image => "image",
                        FileResource::Volume => "volume",
                    }
                    .into(),
                    selected.clone(),
                    path.clone(),
                    "copy".into(),
                    host_destination(destination)?,
                ]
            }
            Self::Files {
                resource,
                target: selected,
                path,
                action,
            } => {
                target(
                    match resource {
                        FileResource::Container => Resource::Container,
                        FileResource::Image => Resource::Image,
                        FileResource::Volume => Resource::Volume,
                    },
                    selected,
                )?;
                if !path.starts_with('/') || path.len() > 2048 || path.contains(['\0', '\\']) {
                    return Err("Use an absolute Linux container path (maximum 2048 bytes).".into());
                }
                mutation = false;
                seconds = 20;
                vec![
                    "files".into(),
                    match resource {
                        FileResource::Container => "container",
                        FileResource::Image => "image",
                        FileResource::Volume => "volume",
                    }
                    .into(),
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
            Self::ConfigureEngineProxy { review } => {
                seconds = 30;
                input = Some(crate::engine_proxy::patch(review)?);
                vec![
                    "kubectl".into(),
                    "patch".into(),
                    "apps.app.rancherdesktop.io".into(),
                    "app".into(),
                    "--type=json".into(),
                    "--patch-file=-".into(),
                    "--output=name".into(),
                    "--request-timeout=15s".into(),
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
            Self::Stats { .. } | Self::EngineStats => {
                mutation = false;
                let mut args = vec![
                    "stats".into(),
                    "--no-stream".into(),
                    "--no-trunc".into(),
                    "--format={{json .}}".into(),
                ];
                if let Self::Stats { target: id } = self {
                    target(Resource::Container, id)?;
                    args.push(id.clone());
                }
                args
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
            Self::CreateNetwork {
                name: n,
                driver,
                subnet,
                internal,
            } => {
                name(n)?;
                if !driver.is_empty() && driver != "bridge" {
                    return Err("Bridge is the supported network driver.".into());
                }
                let mut args = vec!["network".into(), "create".into(), "--driver=bridge".into()];
                if !subnet.is_empty() {
                    let (address, prefix) = subnet
                        .split_once('/')
                        .ok_or("Use a canonical IPv4 subnet, such as 172.30.0.0/24.")?;
                    let ip = address
                        .parse::<std::net::Ipv4Addr>()
                        .map_err(|_| "Invalid IPv4 subnet address.")?;
                    let bits = prefix
                        .parse::<u32>()
                        .map_err(|_| "Invalid IPv4 subnet prefix.")?;
                    if !(1..=30).contains(&bits) || prefix != bits.to_string() {
                        return Err("Use an IPv4 subnet prefix from 1 to 30.".into());
                    }
                    let mask = u32::MAX << (32 - bits);
                    if u32::from(ip) & mask != u32::from(ip)
                        || ip.is_loopback()
                        || ip.is_multicast()
                        || ip.is_unspecified()
                        || ip.is_link_local()
                    {
                        return Err(
                            "Use a canonical unicast IPv4 network address; host bits must be zero."
                                .into(),
                        );
                    }
                    args.extend(["--subnet".into(), subnet.clone()]);
                }
                if *internal {
                    args.push("--internal".into());
                }
                args.extend(["--".into(), n.clone()]);
                args
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
                args.extend(container_limit_args(&options.cpus, &options.memory_mib)?);
                if !matches!(
                    options.restart.as_str(),
                    "" | "no" | "on-failure" | "always" | "unless-stopped"
                ) {
                    return Err("Unsupported restart policy.".into());
                }
                if !options.restart.is_empty() {
                    args.extend(["--restart".into(), options.restart.clone()]);
                }
                if !options.network.is_empty() {
                    name(&options.network)?;
                    if matches!(options.network.as_str(), "host" | "none") {
                        return Err("Choose an existing bridge network; host and none are not supported in Run.".into());
                    }
                    args.extend(["--network".into(), options.network.clone()]);
                }
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
            Self::UpdateContainerLimits {
                target: id,
                cpus,
                memory_mib,
            } => {
                target(Resource::Container, id)?;
                let limits = container_limit_args(cpus, memory_mib)?;
                if limits.is_empty() {
                    return Err(
                        "Enter at least one new limit; blank fields leave that setting unchanged."
                            .into(),
                    );
                }
                let mut args = vec!["container".into(), "update".into()];
                args.extend(limits);
                args.extend(["--".into(), id.clone()]);
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
                overrides,
                env_files,
                profiles,
                services,
                directory,
                project,
                action,
            } => {
                name(project)?;
                let path = host_path(file, false)?;
                if overrides.len() > 15
                    || env_files.len() > 16
                    || profiles.len() > 16
                    || services.len() > 64
                {
                    return Err(
                        "Use at most 15 override files, 16 env files, 16 profiles and 64 services."
                            .into(),
                    );
                }
                if !services.is_empty()
                    && matches!(action, ComposeAction::Down | ComposeAction::Validate)
                {
                    return Err("Down and Validate apply to the entire project; clear the service selection first.".into());
                }
                for service in services {
                    name(service)?;
                }
                seconds = 600;
                let mut args = vec![
                    "compose".into(),
                    "--project-name".into(),
                    project.clone(),
                    "--file".into(),
                    path,
                ];
                // Preserve merge order; Compose resolves relative paths from the
                // first file. Arguments never pass through a shell.
                for file in overrides {
                    args.extend(["--file".into(), host_path(file, false)?]);
                }
                for file in env_files {
                    args.extend(["--env-file".into(), host_path(file, false)?]);
                }
                for profile in profiles {
                    name(profile)?;
                    args.extend(["--profile".into(), profile.clone()]);
                }
                if !directory.is_empty() {
                    args.extend(["--project-directory".into(), host_path(directory, true)?]);
                }
                args.extend(
                    match action {
                        ComposeAction::Validate => {
                            mutation = false;
                            seconds = 30;
                            vec!["config", "--quiet"]
                        }
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
                args.extend(services.iter().cloned());
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
fn migration_wsl_identity(registration: &str, machine: &str, initial: bool) -> Result<(), String> {
    let guid = registration.len() == 36
        && registration.bytes().enumerate().all(|(index, byte)| {
            if [8, 13, 18, 23].contains(&index) {
                byte == b'-'
            } else {
                byte.is_ascii_hexdigit() && !byte.is_ascii_uppercase()
            }
        });
    let observed = machine.len() == 32
        && machine
            .bytes()
            .all(|byte| byte.is_ascii_hexdigit() && !byte.is_ascii_uppercase());
    if !guid || !(observed || (initial && machine.is_empty())) {
        return Err("Select an ordinary WSL registration and review its machine identity.".into());
    }
    Ok(())
}

fn image_archive_source(raw: &str) -> Result<String, String> {
    let path = crate::runtime::cli_path(std::path::Path::new(&host_path(raw, false)?))?;
    let source = path
        .to_str()
        .ok_or("Image archive path is not valid Unicode")?;
    if source.len() > 4096
        || !crate::mounts::windows_source(source)
        || !path
            .extension()
            .and_then(|extension| extension.to_str())
            .is_some_and(|extension| extension.eq_ignore_ascii_case("tar"))
    {
        return Err("Choose a plain .tar image archive on a local Windows drive.".into());
    }
    Ok(source.into())
}
pub(crate) fn host_destination(raw: &str) -> Result<String, String> {
    value(raw)?;
    let path = std::path::Path::new(raw);
    if !path.is_absolute() {
        return Err("Select a new absolute Windows file destination.".into());
    }
    match path.symlink_metadata() {
        Ok(_) => return Err("Copy never overwrites an existing destination.".into()),
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
        Err(error) => return Err(error.to_string()),
    }
    let parent = path.parent().ok_or("Select a destination directory.")?;
    let parent = crate::runtime::cli_path(std::path::Path::new(&host_path(
        parent.to_str().ok_or("Invalid path")?,
        true,
    )?))?;
    parent
        .join(path.file_name().ok_or("Select a file name")?)
        .to_str()
        .map(String::from)
        .ok_or("Destination is not valid Unicode".into())
}

#[derive(Serialize, Clone)]
#[serde(tag = "kind", rename_all = "camelCase")]
pub enum Output {
    Ready,
    SetupDecision {
        reason: &'static str,
        nonce: String,
    },
    SetupPaused,
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
        job.decision.take();
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
    let mut plan = request.plan()?;
    let setup_control = SetupDecisionControl {
        state: state.clone(),
        id: id.clone(),
    };
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
                setup: matches!(request, Request::Setup),
                decision: None,
            },
        );
    }
    let _lease = Lease {
        _owner: Arc::new(LeaseOwner {
            state: state.clone(),
            id,
        }),
    };
    output
        .send(Output::Ready)
        .map_err(|_| "The output viewer disconnected")?;
    if matches!(request, Request::Setup) {
        return crate::installation::setup(&output, &mut cancelled, &setup_control).await;
    }
    if matches!(request, Request::PrepareUpdate) {
        return crate::installation::prepare_update(&output, &mut cancelled).await;
    }
    if matches!(
        request,
        Request::RegistryList | Request::RegistryLogin { .. } | Request::RegistryForget { .. }
    ) {
        return crate::registry::run(&request, &output, &mut cancelled, _lease.clone()).await;
    }
    let target = engine::target()?;
    if matches!(request, Request::ConfigureEngineProxy { .. }) {
        if !crate::engine_proxy::capability()? {
            return Err("This backend build does not include engine proxy configuration.".into());
        }
        engine::check(&target).await?;
    }
    let management = matches!(
        request,
        Request::Engine { .. } | Request::ConfigureCluster { .. }
    ) || request.is_management_api()
        || request.is_cluster();
    let stopping = matches!(request, Request::Engine { running: false });
    let wsl_workflow = match &request {
        Request::WslCompose { distribution, .. }
        | Request::WslBuild { distribution, .. }
        | Request::WslSaveMapping { distribution, .. } => Some(distribution.as_str()),
        _ => None,
    };
    let file_read = matches!(request, Request::Files { .. } | Request::CopyFile { .. });
    let host_metadata_recovery = matches!(
        request,
        Request::VolumeClones
            | Request::DiscardCloneStaging { .. }
            | Request::ImageImportHistory
            | Request::MigrationVolumeHistory
            | Request::MigrationImageHistory
            | Request::ImageImportRecord { .. }
            | Request::RecoverDesktopMigrationVolume { .. }
            | Request::MigrationWslSources
    );
    let sdk_operation = file_read
        || matches!(
            request,
            Request::VolumeSize { .. }
                | Request::RecoverTools
                | Request::VolumeTransfers
                | Request::DiscardVolumeTransfer { .. }
                | Request::SaveVolumeFile { .. }
                | Request::ExportVolume { .. }
                | Request::RestoreVolume { .. }
                | Request::ExportImage { .. }
                | Request::ReviewDesktopMigration
                | Request::MigrationWslSources
                | Request::ReviewRancherMigration { .. }
                | Request::ExportRancherMigrationImage { .. }
                | Request::ReviewRancherMigrationVolume { .. }
                | Request::ExportRancherMigrationVolume { .. }
                | Request::ExportDesktopMigrationImage { .. }
                | Request::ReviewDesktopMigrationVolume { .. }
                | Request::ExportDesktopMigrationVolume { .. }
                | Request::RecoverDesktopMigrationVolume { .. }
                | Request::ReviewImageImport { .. }
                | Request::ImportImages { .. }
                | Request::ImageImportHistory
                | Request::MigrationVolumeHistory
                | Request::MigrationImageHistory
                | Request::ReviewMigrationDestination { .. }
                | Request::ImageImportRecord { .. }
                | Request::CloneVolume { .. }
                | Request::VolumeClones
                | Request::DiscardCloneStaging { .. }
        );
    let maintenance_archive = if sdk_operation
        && (file_read
            || matches!(
                request,
                Request::SaveVolumeFile { .. }
                    | Request::ExportVolume { .. }
                    | Request::RestoreVolume { .. }
                    | Request::CloneVolume { .. }
                    | Request::ImportImages { .. }
            )) {
        Some(
            tokio::task::spawn_blocking(crate::runtime::maintenance_image)
                .await
                .map_err(|error| error.to_string())??,
        )
    } else {
        None
    };
    let mut executable_lease = None;
    let binary = if wsl_workflow.is_some() {
        Some(crate::wsl::wsl_binary())
    } else if sdk_operation {
        if !host_metadata_recovery {
            engine::check(&target).await?;
        }
        let lease = target.session().await?;
        let path = lease.path().to_path_buf();
        executable_lease = Some(lease);
        Some(path)
    } else if management {
        let lease = target.rdd().await?;
        let path = lease.path().to_path_buf();
        executable_lease = Some(lease);
        Some(path)
    } else {
        engine::check(&target).await?;
        None
    };
    // Cancellation before dispatch must never issue the mutation.
    if cancelled.try_recv().is_ok() {
        return Err("Cancelled before dispatch. No operation was sent.".into());
    }
    if let Some(lease) = &executable_lease {
        lease.recheck()?;
    }
    let direct_docker = wsl_workflow.is_none() && !sdk_operation && !management;
    let client_result = if direct_docker {
        // Publication returns an immutable private generation; its master lock
        // has been released before any long-lived stream can start.
        let snapshot = target.client_snapshot().await?;
        let mut environment =
            engine::docker_environment_for_paths(&target, snapshot.path(), snapshot.buildx_path())?;
        environment.push(("BUILDX_BUILDER".into(), "default".into()));
        if matches!(request, Request::Compose { .. }) {
            environment.retain(|(key, _)| {
                !matches!(
                    key.to_string_lossy().to_ascii_uppercase().as_str(),
                    "COMPOSE_FILE"
                        | "COMPOSE_PROFILES"
                        | "COMPOSE_PROJECT_NAME"
                        | "COMPOSE_ENV_FILES"
                        | "COMPOSE_REMOVE_ORPHANS"
                )
            });
            environment.push(("COMPOSE_PROFILES".into(), "".into()));
            environment.push(("COMPOSE_REMOVE_ORPHANS".into(), "false".into()));
        }
        let mut args = vec![
            "--config".into(),
            snapshot.path().as_os_str().to_owned(),
            "--host".into(),
            engine::ENDPOINT.into(),
        ];
        args.extend(plan.args.iter().map(std::ffi::OsString::from));
        crate::operation_worker::run(
            target.docker.clone(),
            args,
            environment,
            Some(snapshot),
            plan.input.take(),
            output.clone(),
            &mut cancelled,
            plan.seconds,
            Some(_lease.clone()),
        )
        .await?
    } else {
        let mut command = Command::new(binary.ok_or("Missing non-Docker client executable")?);
        if let Some((archive, hash)) = maintenance_archive {
            command
                .env("ORBWIN_MAINTENANCE_IMAGE", archive)
                .env("ORBWIN_MAINTENANCE_IMAGE_SHA256", hash);
        }
        if matches!(
            request,
            Request::ReviewRancherMigration { .. }
                | Request::ExportRancherMigrationImage { .. }
                | Request::ReviewRancherMigrationVolume { .. }
                | Request::ExportRancherMigrationVolume { .. }
                | Request::RecoverDesktopMigrationVolume { .. }
        ) {
            let (binary, hash) = crate::runtime::migration_docker()?;
            command
                .env("ORBWIN_MIGRATION_DOCKER", binary)
                .env("ORBWIN_MIGRATION_DOCKER_SHA256", hash);
        }
        if let Some(distribution) = wsl_workflow {
            let data: serde_json::Value = serde_json::from_slice(
                plan.input
                    .as_deref()
                    .ok_or("WSL workflow input is missing")?,
            )
            .map_err(|error| error.to_string())?;
            let (prepared, input) =
                crate::wsl_integration::prepare_workflow(distribution, data).await?;
            command = prepared;
            plan.input = Some(input);
            if cancelled.try_recv().is_ok() {
                return Err("Cancelled before WSL dispatch; no workflow was sent.".into());
            }
        } else if management {
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
            if request.is_cluster() || request.is_management_api() {
                let rdd = executable_lease
                    .as_ref()
                    .ok_or("Missing verified backend image")?;
                let paths = tokio::select! {
                    biased;
                    _ = &mut cancelled => return Err("Cancelled before dispatch. No operation was sent.".into()),
                    result = crate::backend_paths::read(rdd, target.home()) => result?,
                };
                if cancelled.try_recv().is_ok() {
                    return Err("Cancelled before dispatch. No operation was sent.".into());
                }
                if request.is_cluster() {
                    let config = paths.k3s_config;
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
                } else {
                    // `rdd ctl` would auto-start the management service. Reads and the
                    // one proxy CAS use only the existing API and fail if offline.
                    command
                        .arg("kubectl")
                        .arg("--kubeconfig")
                        .arg(paths.config)
                        .args(&plan.args[1..]);
                }
            }
        } else if sdk_operation {
            if matches!(
                request,
                Request::SaveVolumeFile { .. }
                    | Request::ExportVolume { .. }
                    | Request::RestoreVolume { .. }
                    | Request::CloneVolume { .. }
                    | Request::ExportDesktopMigrationVolume { .. }
                    | Request::ExportRancherMigrationVolume { .. }
            ) {
                // Source helpers contain no shell or registry dependency.
                let (binary, hash) = crate::runtime::guest_file_helper()?;
                command
                    .env("ORBWIN_FS_HELPER", binary)
                    .env("ORBWIN_FS_HELPER_SHA256", hash);
            }
            command.env(
                "ORBWIN_TRANSIENT_DIR",
                target.home().join(".rdorbwin/transients"),
            );
            if matches!(request, Request::MigrationWslSources) {
                command.arg("migration-wsl-sources");
            } else if matches!(
                request,
                Request::ReviewRancherMigration { .. }
                    | Request::ExportRancherMigrationImage { .. }
                    | Request::ReviewRancherMigrationVolume { .. }
                    | Request::ExportRancherMigrationVolume { .. }
            ) {
                command
                    .arg(&plan.args[0])
                    .arg(target.daemon_id())
                    .args(&plan.args[1..]);
            } else if matches!(
                request,
                Request::ReviewDesktopMigration
                    | Request::ExportDesktopMigrationImage { .. }
                    | Request::ReviewDesktopMigrationVolume { .. }
                    | Request::ExportDesktopMigrationVolume { .. }
            ) {
                command
                    .args(&plan.args[..2])
                    .arg(target.daemon_id())
                    .args(&plan.args[2..]);
            } else if matches!(request, Request::ExportImage { .. }) {
                command
                    .arg("image-archive")
                    .args(&plan.args[1..])
                    .arg(target.daemon_id());
            } else if matches!(
                request,
                Request::ExportVolume { .. }
                    | Request::RestoreVolume { .. }
                    | Request::CloneVolume { .. }
            ) {
                command
                    .arg("volume-transfer")
                    .args(&plan.args[1..])
                    .arg(target.daemon_id());
            } else {
                command
                    .arg(if file_read {
                        "files"
                    } else if matches!(request, Request::ReviewImageImport { .. }) {
                        "image-import-review"
                    } else if matches!(request, Request::ImportImages { .. }) {
                        "import-images"
                    } else if matches!(request, Request::MigrationVolumeHistory) {
                        "migration-volume-history"
                    } else if matches!(request, Request::MigrationImageHistory) {
                        "migration-image-history"
                    } else if matches!(request, Request::ReviewMigrationDestination { .. }) {
                        "migration-destination-review"
                    } else if matches!(request, Request::ImageImportHistory) {
                        "image-import-history"
                    } else if matches!(request, Request::ImageImportRecord { .. }) {
                        "image-import-record"
                    } else if matches!(request, Request::RecoverDesktopMigrationVolume { .. }) {
                        "migration-volume-recover"
                    } else if matches!(request, Request::SaveVolumeFile { .. }) {
                        "save-volume-file"
                    } else if matches!(request, Request::VolumeTransfers) {
                        "volume-transfers"
                    } else if matches!(request, Request::VolumeSize { .. }) {
                        "volume-size"
                    } else if matches!(request, Request::VolumeClones) {
                        "volume-clones"
                    } else if matches!(request, Request::DiscardCloneStaging { .. }) {
                        "discard-clone-staging"
                    } else if matches!(request, Request::DiscardVolumeTransfer { .. }) {
                        "discard-volume-transfer"
                    } else {
                        "recover-tools"
                    })
                    .arg(target.daemon_id())
                    .args(&plan.args[1..]);
            }
        }
        if wsl_workflow.is_none()
            && !sdk_operation
            && !request.is_cluster()
            && !request.is_management_api()
        {
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
        run_client(
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
        .await?
    };
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
        let readiness_lease = target.rdd().await?;
        readiness_lease.recheck()?;
        let mut readiness = Command::new(&readiness_lease);
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
    if interrupted
        && matches!(
            request,
            Request::CopyFile { .. }
                | Request::ExportVolume { .. }
                | Request::RestoreVolume { .. }
                | Request::CloneVolume { .. }
                | Request::ImportImages { .. }
        )
    {
        let cleanup_lease = target.session().await?;
        cleanup_lease.recheck()?;
        let mut cleanup = Command::new(&cleanup_lease);
        cleanup
            .env(
                "ORBWIN_TRANSIENT_DIR",
                target.home().join(".rdorbwin/transients"),
            )
            .args(["recover-tools", target.daemon_id()]);
        let (_keepalive, mut cleanup_cancel) = oneshot::channel();
        match run_client(cleanup,None,&output,&mut cleanup_cancel,15,true).await {
            Ok(result) if !result.interrupted && result.exit_code==Some(0)=>{},
            _=>reason.push_str(". Disposable tools cleanup could not be confirmed; use Recover abandoned tools in Settings"),
        }
        if matches!(request, Request::CloneVolume { .. }) {
            reason.push_str(". Source data was not changed; saved clone staging and an isolated pending destination may remain. Review clone snapshots and restore recovery before retrying");
        } else if matches!(request, Request::RestoreVolume { .. }) {
            reason.push_str(". A pending destination may remain; retain its ownership receipt and do not attach it to workloads until verified");
        } else if matches!(request, Request::ExportVolume { .. }) {
            reason.push_str(". Source data was not changed; a named partial export may remain at the selected destination");
        }
    }
    if interrupted
        && matches!(
            request,
            Request::ExportImage { .. }
                | Request::ExportDesktopMigrationImage { .. }
                | Request::ExportRancherMigrationImage { .. }
        )
    {
        reason.push_str(". Source images and tags were not changed. A named partial archive or a completed archive with an unconfirmed acknowledgement may remain; inspect the selected destination before retrying");
    }
    if interrupted && matches!(request, Request::ImportImages { .. }) {
        reason.push_str(". Isolated image references and accepted tag changes may remain. Inspect image import history and its exact receipt before retrying; cancellation does not roll back tags or delete images");
    }
    if interrupted
        && matches!(
            request,
            Request::ExportDesktopMigrationVolume { .. }
                | Request::ExportRancherMigrationVolume { .. }
                | Request::RecoverDesktopMigrationVolume { .. }
        )
    {
        reason.push_str(". Source volume data was not changed. Source export tools and a named partial or unconfirmed completed package may remain. Use the exact transaction ID printed in the source maintenance receipt with Recover source export tools; recovery never resumes or rolls back a transfer");
    }
    let identity = if stopping
        || request.is_management_read()
        || matches!(request, Request::ConfigureEngineProxy { .. })
        || host_metadata_recovery
    {
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
    } else if !interrupted
        && exit_code == Some(0)
        && matches!(request, Request::ConfigureEngineProxy { .. })
    {
        "Proxy intent was stored. The backend applies it separately; read current proxy status before relying on it.".into()
    } else if interrupted {
        if matches!(request, Request::CopyFile { .. }) {
            format!("{reason}. The source was not modified. A complete destination or an explicitly named partial file may remain; inspect the selected directory before retrying. Existing destination files are never overwritten.")
        } else {
            format!(
                "{reason}. {}",
                if plan.mutation {
                    "The operation may already have taken effect. Refresh before retrying."
                } else {
                    "The engine and workloads were not stopped."
                }
            )
        }
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

pub(crate) fn send(
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
#[path = "operations_inventory_qualification.rs"]
mod inventory_qualification;
#[cfg(test)]
#[path = "operations_qualification.rs"]
mod qualification;
#[cfg(test)]
#[path = "registry_client_qualification.rs"]
mod registry_client_qualification;

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn container_limits_are_bounded_and_never_silently_discard_invalid_values() {
        assert_eq!(
            container_limit_args("0.125", "128").unwrap(),
            ["--cpus", "0.125", "--memory", "134217728"]
        );
        for cpu in ["0.01", "0.1", "1", "1.001", "1024"] {
            assert!(container_limit_args(cpu, "").is_ok(), "{cpu}");
        }
        for cpu in [
            "0",
            "0.001",
            "-1",
            ".5",
            "1.",
            "01",
            "1e2",
            "NaN",
            "1024.001",
            "1.0000",
            "1\n",
            "--help",
            "999999999999999999999999",
        ] {
            assert!(container_limit_args(cpu, "128").is_err(), "{cpu}");
        }
        for memory in ["6", "512", "1048576"] {
            assert!(container_limit_args("", memory).is_ok());
        }
        for memory in [
            "0",
            "5",
            "-1",
            "1.5",
            "512m",
            "01",
            "1048577",
            "128\n",
            "--help",
            "9999999999999999999999",
        ] {
            assert!(container_limit_args("1", memory).is_err(), "{memory}");
        }
        let id = "a".repeat(64);
        let update = |cpus: &str, memory: &str| Request::UpdateContainerLimits {
            target: id.clone(),
            cpus: cpus.into(),
            memory_mib: memory.into(),
        };
        assert!(update("", "").plan().is_err());
        let plan = update("0.5", "").plan().unwrap();
        assert!(plan.mutation);
        assert_eq!(
            plan.args,
            ["container", "update", "--cpus", "0.5", "--", &id]
        );
        assert!(Request::UpdateContainerLimits {
            target: "reused-name".into(),
            cpus: "1".into(),
            memory_mib: "".into()
        }
        .plan()
        .is_err());
        let run = Request::Run {
            image: "example:latest".into(),
            name: "example".into(),
            command: vec![],
            options: RunOptions {
                cpus: "0.5".into(),
                memory_mib: "64".into(),
                ..Default::default()
            },
        }
        .plan()
        .unwrap();
        assert!(run.args.windows(2).any(|pair| pair == ["--cpus", "0.5"]));
        assert!(run
            .args
            .windows(2)
            .any(|pair| pair == ["--memory", "67108864"]));
        assert!(run.args.ends_with(&["--".into(), "example:latest".into()]));
    }
    #[test]
    fn creation_options_are_explicit_and_validated() {
        let network = |subnet: &str, driver: &str| Request::CreateNetwork {
            name: "test-network".into(),
            driver: driver.into(),
            subnet: subnet.into(),
            internal: true,
        };
        let plan = network("172.30.0.0/24", "bridge").plan().unwrap();
        assert_eq!(
            plan.args,
            [
                "network",
                "create",
                "--driver=bridge",
                "--subnet",
                "172.30.0.0/24",
                "--internal",
                "--",
                "test-network"
            ]
        );
        for subnet in [
            "172.30.0.1/24",
            "172.30.0.0/33",
            "172.30.0.0/024",
            "::/64",
            "127.0.0.0/8",
            "224.0.0.0/4",
            "0.0.0.0/0",
            "169.254.0.0/16",
            "--help",
            "1.2.3.0/24\n",
        ] {
            assert!(network(subnet, "bridge").plan().is_err(), "{subnet}");
        }
        for driver in ["host", "overlay", "macvlan", "--help"] {
            assert!(network("", driver).plan().is_err());
        }
        assert!(network("", "").plan().is_ok());
        let run = |restart: &str, network: &str| Request::Run {
            image: "example:latest".into(),
            name: "test-container".into(),
            command: vec!["--literal-command".into()],
            options: RunOptions {
                restart: restart.into(),
                network: network.into(),
                ..Default::default()
            },
        };
        for restart in ["", "no", "on-failure", "always", "unless-stopped"] {
            let plan = run(restart, "project_default").plan().unwrap();
            assert!(plan
                .args
                .windows(2)
                .any(|args| args == ["--network", "project_default"]));
            assert!(plan.args.ends_with(&[
                "--".into(),
                "example:latest".into(),
                "--literal-command".into()
            ]));
        }
        for restart in ["on-failure:3", "--privileged", "always\n", "invalid"] {
            assert!(run(restart, "").plan().is_err());
        }
        for selected in [
            "host",
            "none",
            "container:abc",
            "--privileged",
            "test network",
        ] {
            assert!(run("", selected).plan().is_err());
        }
    }
    #[test]
    fn wsl_draft_review_is_bounded_typed_and_not_a_new_guest_file_api() {
        let request = serde_json::json!({"kind":"wslCompose","distribution":"Ubuntu","connection":{"registration":"11111111-1111-1111-1111-111111111111","source":"/home/user/project","signature":"1:2"},"file":"compose.yaml","overrides":["mapping.json"],"project":"sample","action":"validate","reviewedDaemon":"owned","mappingReview":{"file":"mapping.json","sha256":"a".repeat(64),"contents":r#"{"services":{"web":{"volumes":[{"type":"volume","source":"orbwin_migration_test_1","target":"/data","read_only":false,"volume":{"nocopy":true}}]}},"volumes":{"orbwin_migration_test_1":{"external":true,"name":"restored"}}}"#}});
        let plan = serde_json::from_value::<Request>(request.clone())
            .unwrap()
            .plan()
            .unwrap();
        assert!(!plan.mutation);
        let input: serde_json::Value =
            serde_json::from_slice(plan.input.as_ref().unwrap()).unwrap();
        assert_eq!(input["mappingReview"]["file"], "mapping.json");
        assert_eq!(input["reviewedDaemon"], "owned");
        for (pointer, value) in [
            ("/mappingReview/file", serde_json::json!("../outside.json")),
            ("/mappingReview/sha256", serde_json::json!("A".repeat(64))),
            ("/mappingReview/contents", serde_json::json!("{}")),
            ("/reviewedDaemon", serde_json::Value::Null),
            ("/connection/registration", serde_json::Value::Null),
            ("/action", serde_json::json!("down")),
        ] {
            let mut changed = request.clone();
            *changed.pointer_mut(pointer).unwrap() = value;
            assert!(
                serde_json::from_value::<Request>(changed)
                    .unwrap()
                    .plan()
                    .is_err(),
                "{pointer}"
            );
        }
    }
    #[test]
    fn wsl_mapping_requires_registered_root_and_new_relative_filename() {
        let request = |file: &str, registration: Option<String>| {
            Request::WslSaveMapping {
            distribution: "Ubuntu".into(),
            connection: crate::wsl_integration::ConnectionReview { registration, source:"/home/user/project".into(),signature:"1:2".into() },
            file:file.into(),target:"owned".into(),
            contents: serde_json::json!({"services":{"web":{"volumes":[{"type":"volume","source":"orbwin_migration_test_1","target":"/data","read_only":true,"volume":{"nocopy":true}}]}},"volumes":{"orbwin_migration_test_1":{"external":true,"name":"restored"}}}).to_string(),
        }
        };
        let registration = Some("00000000-0000-4000-8000-000000000001".into());
        let plan = request("mapping 例.json", registration.clone())
            .plan()
            .unwrap();
        assert!(plan.mutation);
        assert_eq!(plan.seconds, 50);
        let input: serde_json::Value =
            serde_json::from_slice(plan.input.as_ref().unwrap()).unwrap();
        assert_eq!(input["kind"], "mapping");
        assert_eq!(input["reviewedDaemon"], "owned");
        assert!(request("mapping.json", None).plan().is_err());
        for file in ["../escape.json", "nested/map.json", "wrong.yml"] {
            assert!(request(file, registration.clone()).plan().is_err());
        }
    }
    #[test]
    fn volume_size_plan_is_readonly_and_requires_current_selection() {
        let plan = Request::VolumeSize {
            target: "data".into(),
            created_at: "2026-10-25T00:00:00Z".into(),
        }
        .plan()
        .unwrap();
        assert!(!plan.mutation);
        assert_eq!(plan.seconds, 40);
        assert_eq!(plan.args, ["volume-size", "data", "2026-10-25T00:00:00Z"]);
        for (target, created_at) in [("--all", "now"), ("data", ""), ("data", "bad\nvalue")] {
            assert!(Request::VolumeSize {
                target: target.into(),
                created_at: created_at.into()
            }
            .plan()
            .is_err());
        }
    }
    #[test]
    fn stats_plans_are_bounded_readonly_and_target_explicitly() {
        let engine = Request::EngineStats.plan().unwrap();
        assert!(!engine.mutation);
        assert_eq!(
            engine.args,
            ["stats", "--no-stream", "--no-trunc", "--format={{json .}}"]
        );
        let id = "a".repeat(64);
        let container = Request::Stats { target: id.clone() }.plan().unwrap();
        assert!(!container.mutation);
        assert_eq!(container.args.last(), Some(&id));
        assert!(Request::Stats {
            target: "--all".into()
        }
        .plan()
        .is_err());
    }
    #[test]
    fn migration_adapter_plans_pin_source_kind_and_validate_reviewed_inputs() {
        let review = Request::ReviewDesktopMigration.plan().unwrap();
        assert!(!review.mutation);
        assert_eq!(review.seconds, 105);
        assert_eq!(review.args, ["migration-review", "docker-desktop"]);
        let destination = std::env::temp_dir()
            .join(format!("orbwin-migration-plan-{}.tar", std::process::id()))
            .to_str()
            .unwrap()
            .to_owned();
        let request = |source: &str, image: &str| Request::ExportDesktopMigrationImage {
            source: source.into(),
            image: image.into(),
            reference: "example/probe:reviewed".into(),
            destination: destination.clone(),
        };
        let image = format!("sha256:{}", "a".repeat(64));
        let plan = request("reviewed-source", &image).plan().unwrap();
        assert!(!plan.mutation);
        assert_eq!(plan.seconds, 1815);
        assert_eq!(
            &plan.args[..5],
            [
                "migration-image-export",
                "docker-desktop",
                "reviewed-source",
                image.as_str(),
                "example/probe:reviewed"
            ]
        );
        assert!(request("source\nchanged", &image).plan().is_err());
        assert!(request(&"a".repeat(257), &image).plan().is_err());
        assert!(request("reviewed-source", "partial").plan().is_err());
    }
    #[test]
    fn source_volume_maintenance_requires_exact_review_and_recovery_tokens() {
        let read = Request::ReviewDesktopMigrationVolume {
            source: "reviewed-source".into(),
            volume: "selected".into(),
        }
        .plan()
        .unwrap();
        assert_eq!(
            read.args,
            [
                "migration-volume-review",
                "docker-desktop",
                "reviewed-source",
                "selected"
            ]
        );
        assert!(!read.mutation);
        for token in ["", "--all", "../foreign", "ABCDEF", &"a".repeat(31)] {
            assert!(Request::RecoverDesktopMigrationVolume {
                transaction: token.into()
            }
            .plan()
            .is_err());
        }
        let cleanup = Request::RecoverDesktopMigrationVolume {
            transaction: "a".repeat(32),
        }
        .plan()
        .unwrap();
        assert_eq!(cleanup.args, ["migration-volume-recover", &"a".repeat(32)]);
        assert!(!cleanup.mutation);
        let history = Request::MigrationVolumeHistory.plan().unwrap();
        assert_eq!(history.args, ["migration-volume-history"]);
        assert!(!history.mutation);
        assert_eq!(history.seconds, 30);
        let image_history = Request::MigrationImageHistory.plan().unwrap();
        assert_eq!(image_history.args, ["migration-image-history"]);
        assert!(!image_history.mutation);
        assert_eq!(image_history.seconds, 30);
        let destination: Request = serde_json::from_str(r#"{"kind":"reviewMigrationDestination","source":"source","sourceVolume":"data","destination":"restored"}"#).unwrap();
        let destination = destination.plan().unwrap();
        assert_eq!(
            destination.args,
            ["migration-destination-review", "source", "data", "restored"]
        );
        assert!(!destination.mutation);
        assert_eq!(destination.seconds, 40);
        assert!(Request::ReviewMigrationDestination {
            source: "source".into(),
            source_volume: "../data".into(),
            destination: "restored".into()
        }
        .plan()
        .is_err());
        let request = Request::ExportDesktopMigrationVolume {
            source: "source".into(),
            volume: "selected".into(),
            fingerprint: "partial".into(),
            destination: "C:\\new.orbvolume".into(),
        };
        assert!(request.plan().is_err());
    }
    #[test]
    fn rancher_source_requires_guid_and_frozen_machine_for_export() {
        let registration = "00000000-0000-4000-8000-000000000001";
        assert_eq!(
            Request::MigrationWslSources.plan().unwrap().args,
            ["migration-wsl-sources"]
        );
        let read = Request::ReviewRancherMigration {
            registration: registration.into(),
            machine: String::new(),
        }
        .plan()
        .unwrap();
        assert!(!read.mutation);
        assert_eq!(read.args, ["migration-rancher-review", registration, ""]);
        assert!(migration_wsl_identity("Ubuntu", "", true).is_err());
        assert!(migration_wsl_identity(registration, "", false).is_err());
        assert!(migration_wsl_identity(registration, &"c".repeat(32), false).is_ok());
    }
    #[test]
    fn rancher_volume_plan_requires_frozen_source_and_new_destination() {
        let registration = "00000000-0000-4000-8000-000000000001";
        let read = Request::ReviewRancherMigrationVolume {
            source: "source".into(),
            registration: registration.into(),
            machine: "c".repeat(32),
            volume: "data".into(),
        }
        .plan()
        .unwrap();
        assert!(!read.mutation);
        assert_eq!(
            read.args,
            [
                "migration-rancher-volume-review",
                "source",
                registration,
                &"c".repeat(32),
                "data"
            ]
        );
        let plan = Request::ExportRancherMigrationVolume {
            source: "source".into(),
            registration: registration.into(),
            machine: "c".repeat(32),
            volume: "data".into(),
            fingerprint: "b".repeat(64),
            destination: "C:\\new.orbvolume".into(),
        }
        .plan()
        .unwrap();
        assert!(!plan.mutation);
        assert_eq!(plan.seconds, 1815);
        assert_eq!(plan.args[0], "migration-rancher-volume-export");
        assert!(Request::ReviewRancherMigrationVolume {
            source: "source".into(),
            registration: "Ubuntu".into(),
            machine: "c".repeat(32),
            volume: "data".into()
        }
        .plan()
        .is_err());
        assert!(Request::ExportRancherMigrationVolume {
            source: "source".into(),
            registration: registration.into(),
            machine: "".into(),
            volume: "data".into(),
            fingerprint: "b".repeat(64),
            destination: "C:\\new.orbvolume".into()
        }
        .plan()
        .is_err());
    }
    #[test]
    fn compose_file_sets_preserve_order_and_single_file_compatibility() {
        let base = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("Cargo.toml");
        let override_file = base.with_file_name("tauri.conf.json");
        let request: Request = serde_json::from_value(serde_json::json!({
            "kind": "compose", "file": base, "project": "proof", "action": "validate"
        }))
        .unwrap();
        let plan = request.plan().unwrap();
        assert!(!plan.mutation);
        assert_eq!(plan.seconds, 30);
        assert_eq!(&plan.args[plan.args.len() - 2..], ["config", "--quiet"]);
        let plan = Request::Compose {
            env_files: vec![],
            services: vec![],
            directory: String::new(),
            file: base.to_str().unwrap().into(),
            overrides: vec![
                override_file.to_str().unwrap().into(),
                base.to_str().unwrap().into(),
            ],
            profiles: vec!["dev".into(), "tools.test".into()],
            project: "proof".into(),
            action: ComposeAction::Down,
        }
        .plan()
        .unwrap();
        assert!(plan.mutation);
        assert_eq!(
            &plan.args[3..],
            [
                "--file",
                &host_path(base.to_str().unwrap(), false).unwrap(),
                "--file",
                &host_path(override_file.to_str().unwrap(), false).unwrap(),
                "--file",
                &host_path(base.to_str().unwrap(), false).unwrap(),
                "--profile",
                "dev",
                "--profile",
                "tools.test",
                "down",
                "--timeout=30",
            ]
        );
    }
    #[test]
    fn compose_env_files_are_ordered_existing_paths_with_legacy_defaults() {
        let root = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"));
        let base = root.join("Cargo.toml").to_string_lossy().into_owned();
        let second = root.join("src/main.rs").to_string_lossy().into_owned();
        let request = |files: Vec<String>| {
            serde_json::from_value::<Request>(serde_json::json!({"kind":"compose","file":base,"project":"env-proof","action":"validate","envFiles":files})).unwrap()
        };
        let plan = request(vec![base.clone(), second.clone()]).plan().unwrap();
        let pairs: Vec<_> = plan
            .args
            .windows(2)
            .filter(|pair| pair[0] == "--env-file")
            .map(|pair| pair[1].clone())
            .collect();
        assert_eq!(
            pairs,
            [
                host_path(&base, false).unwrap(),
                host_path(&second, false).unwrap()
            ]
        );
        assert!(!plan.mutation);
        for files in [
            vec![base.clone(); 17],
            vec!["--help".into()],
            vec!["relative.env".into()],
            vec![root.to_string_lossy().into_owned()],
            vec![root.join("absent.env").to_string_lossy().into_owned()],
        ] {
            assert!(request(files).plan().is_err());
        }
    }
    #[test]
    fn compose_rejects_invalid_file_sets_and_profiles_before_dispatch() {
        let base = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("Cargo.toml");
        let file = base.to_str().unwrap().to_string();
        for (overrides, profiles) in [
            (vec![file.clone(); 16], vec![]),
            (vec![], vec!["dev".into(); 17]),
            (vec!["relative.yaml".into()], vec![]),
            (
                vec![base
                    .with_file_name("missing-compose.yaml")
                    .to_str()
                    .unwrap()
                    .into()],
                vec![],
            ),
            (vec![], vec!["--help".into()]),
            (vec![], vec!["*".into()]),
            (vec![], vec!["bad\nprofile".into()]),
        ] {
            assert!(Request::Compose {
                env_files: vec![],
                services: vec![],
                directory: String::new(),
                file: file.clone(),
                overrides,
                profiles,
                project: "proof".into(),
                action: ComposeAction::Up
            }
            .plan()
            .is_err());
        }
    }
    #[test]
    fn compose_working_directory_is_explicit_and_stale_paths_fail_before_dispatch() {
        let root = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"));
        let base = root.join("Cargo.toml");
        let request = |directory: String, file: String| -> Request {
            serde_json::from_value(serde_json::json!({
                "kind": "compose", "file": file, "directory": directory,
                "project": "proof", "action": "validate"
            }))
            .unwrap()
        };
        let plan = request(root.to_str().unwrap().into(), base.to_str().unwrap().into())
            .plan()
            .unwrap();
        let option = plan
            .args
            .iter()
            .position(|arg| arg == "--project-directory")
            .unwrap();
        assert_eq!(
            plan.args[option + 1],
            host_path(root.to_str().unwrap(), true).unwrap()
        );
        for directory in [
            "relative".to_string(),
            base.to_str().unwrap().into(),
            root.join("missing").to_str().unwrap().into(),
        ] {
            assert!(request(directory, base.to_str().unwrap().into())
                .plan()
                .is_err());
        }
        assert!(request(
            String::new(),
            root.join("moved-compose.yaml").to_str().unwrap().into()
        )
        .plan()
        .is_err());
    }
    const ID: &str = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa";
    #[test]
    fn file_copy_requires_a_new_absolute_destination_and_retains_existing_data() {
        let base = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"));
        assert!(host_destination("relative.bin").is_err());
        assert!(host_destination(base.join("Cargo.toml").to_str().unwrap()).is_err());
        assert!(host_destination(base.to_str().unwrap()).is_err());
        assert!(host_destination(base.join("missing-parent/new.bin").to_str().unwrap()).is_err());
        let destination = base.join("new-copy-destination.bin");
        let plan = Request::CopyFile {
            resource: FileResource::Container,
            target: ID.into(),
            path: "/large.bin".into(),
            destination: destination.to_str().unwrap().into(),
        }
        .plan()
        .unwrap();
        assert!(!plan.mutation);
        assert_eq!(plan.seconds, 615);
        assert!(!destination.exists());
    }
    #[test]
    fn compose_service_scope_cannot_narrow_down_or_inject_options() {
        let file = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("Cargo.toml");
        let plan = |action: &str, services: Vec<&str>| {
            serde_json::from_value::<Request>(serde_json::json!({
                "kind":"compose", "file":file, "project":"scope", "action":action, "services":services
            })).unwrap().plan()
        };
        let logs = plan("logs", vec!["web", "worker"]).unwrap();
        assert!(!logs.mutation);
        assert_eq!(&logs.args[logs.args.len() - 2..], ["web", "worker"]);
        assert!(plan("up", vec!["web"]).unwrap().mutation);
        for services in [vec!["--help"], vec!["bad\nservice"], vec!["web"; 65]] {
            assert!(plan("logs", services).is_err());
        }
        for action in ["down", "validate"] {
            assert!(plan(action, vec!["web"]).is_err());
        }
    }
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

    #[cfg(windows)]
    #[tokio::test]
    async fn native_blocked_input_deadline_reaps_the_acknowledged_client() {
        let fixture = crate::process_fixture::Fixture::new();
        let command: Command = fixture.command("sleep").into();
        let (_sender, mut cancelled) = oneshot::channel();
        let task = tokio::spawn(async move {
            let output = Channel::new(|_| Ok(()));
            let input = vec![b'x'; 2 * 1024 * 1024];
            client_tree(command, Some(&input), &output, &mut cancelled, 2).await
        });
        let handle = crate::process_fixture::retain_running_process(&fixture).await;
        let result = tokio::time::timeout(Duration::from_secs(5), task)
            .await
            .expect("native blocked input bypassed deadline")
            .unwrap()
            .unwrap();
        assert!(result.interrupted && result.reason.contains("time limit"));
        crate::process_fixture::assert_exited(&handle);
    }

    #[cfg(windows)]
    #[tokio::test]
    async fn native_cancel_reaps_the_acknowledged_client() {
        let fixture = crate::process_fixture::Fixture::new();
        let command: Command = fixture.command("sleep").into();
        let (sender, mut cancelled) = oneshot::channel();
        let task = tokio::spawn(async move {
            let output = Channel::new(|_| Ok(()));
            client_tree(command, None, &output, &mut cancelled, 15).await
        });
        let handle = crate::process_fixture::retain_running_process(&fixture).await;
        sender.send(()).unwrap();
        let result = tokio::time::timeout(Duration::from_secs(5), task)
            .await
            .expect("native cancellation did not settle")
            .unwrap()
            .unwrap();
        assert!(result.interrupted && result.reason.contains("cancelled"));
        crate::process_fixture::assert_exited(&handle);
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
    fn proxy_change_is_one_private_stdin_cas_without_lifecycle_or_status_commands() {
        let value = serde_json::json!({"kind":"configureEngineProxy","review":{"uid":"owned-app","resourceVersion":"42","generation":9,"config":{"version":1,"mode":"url","url":"https://private.proxy:8443","noProxy":[".private.test"]}}});
        let request: Request = serde_json::from_value(value.clone()).unwrap();
        assert!(request.is_management_api());
        assert!(!request.is_management_read());
        assert!(!request.is_cluster());
        let plan = request.plan().unwrap();
        assert!(plan.mutation);
        assert_eq!(plan.seconds, 30);
        assert_eq!(
            plan.args,
            [
                "kubectl",
                "patch",
                "apps.app.rancherdesktop.io",
                "app",
                "--type=json",
                "--patch-file=-",
                "--output=name",
                "--request-timeout=15s"
            ]
        );
        assert!(!format!("{request:?}").contains("private.proxy"));
        assert!(!plan.args.join(" ").contains("private"));
        let patch: serde_json::Value =
            serde_json::from_slice(plan.input.as_ref().unwrap()).unwrap();
        assert_eq!(patch[0]["path"], "/metadata/uid");
        assert_eq!(patch[1]["path"], "/metadata/resourceVersion");
        assert_eq!(patch[2]["path"], "/metadata/generation");
        assert_eq!(patch[3]["path"], "/spec/networkProxy");
        for scope in ["kubeconfig", "endpoint", "command"] {
            let mut changed = value.clone();
            changed["review"][scope] = "foreign".into();
            assert!(serde_json::from_value::<Request>(changed).is_err());
        }
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
                resource: FileResource::Container,
                target: ID.into(),
                path: path.into(),
                action: FileAction::List
            }
            .plan()
            .is_err());
        }
        let plan = Request::Files {
            resource: FileResource::Container,
            target: ID.into(),
            path: "/space ü/file".into(),
            action: FileAction::Read,
        }
        .plan()
        .unwrap();
        assert!(!plan.mutation);
        assert_eq!(plan.seconds, 20);
        assert_eq!(
            plan.args,
            ["files", "container", ID, "/space ü/file", "read"]
        );
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
                setup: false,
                decision: None,
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
                setup: false,
                decision: None,
            },
        );
        let lease = Lease {
            _owner: Arc::new(LeaseOwner {
                state: state.clone(),
                id: "test".into(),
            }),
        };
        let worker_lease = lease.clone();
        drop(lease);
        assert!(state.0.lock().unwrap().values().any(|job| job.mutation));
        drop(worker_lease);
        assert!(state.0.lock().unwrap().is_empty());
    }

    #[tokio::test]
    async fn setup_decision_targets_only_original_uuid_nonce_once() {
        let state = Operations::default();
        let id = "11111111-1111-4111-8111-111111111111";
        let nonce = "22222222222222222222222222222222";
        let (sender, _receiver) = oneshot::channel();
        state.0.lock().unwrap().insert(
            id.into(),
            Running {
                cancel: Some(sender),
                mutation: true,
                setup: true,
                decision: None,
            },
        );
        let control = SetupDecisionControl {
            state: state.clone(),
            id: id.into(),
        };
        let mut wait = control.pending(nonce).unwrap();
        assert!(control.pending(nonce).is_err());
        assert!(setup_decide(&state, "33333333-3333-4333-8333-333333333333", nonce).is_err());
        assert!(setup_decide(&state, id, "44444444444444444444444444444444").is_err());
        assert!(state.0.lock().unwrap()[id].decision.is_some());
        setup_decide(&state, id, nonce).unwrap();
        assert_eq!(wait.response().await.unwrap(), nonce);
        assert!(setup_decide(&state, id, nonce).is_err());
        assert!(state.0.lock().unwrap()[id].mutation);
        drop(wait);
        let mut wait = control.pending("55555555555555555555555555555555").unwrap();
        cancel(&state, id).unwrap();
        assert!(wait.response().await.is_err());
        assert!(setup_decide(&state, id, "55555555555555555555555555555555").is_err());
    }

    #[test]
    fn setup_decision_unregistered_nonsetup_and_dropped_wait_refuse() {
        let state = Operations::default();
        let id = "11111111-1111-4111-8111-111111111111";
        let nonce = "22222222222222222222222222222222";
        let control = SetupDecisionControl {
            state: state.clone(),
            id: id.into(),
        };
        assert!(control.pending(nonce).is_err());
        let (sender, _receiver) = oneshot::channel();
        state.0.lock().unwrap().insert(
            id.into(),
            Running {
                cancel: Some(sender),
                mutation: true,
                setup: false,
                decision: None,
            },
        );
        assert!(control.pending(nonce).is_err());
        state.0.lock().unwrap().get_mut(id).unwrap().setup = true;
        drop(control.pending(nonce).unwrap());
        assert!(setup_decide(&state, id, nonce).is_err());
        assert!(!setup_operation_id("------------------------------------"));
        assert!(!setup_decision_nonce("2222222222222222222222222222222A"));
    }
}
