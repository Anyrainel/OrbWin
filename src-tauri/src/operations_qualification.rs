//! Explicit opt-in tests of the actual native operation adapter against the
//! recorded owned development engine. Only uniquely named test resources are
//! removed; no global lifecycle, context, prune or competitor operation.
use super::*;
use std::sync::{
    atomic::{AtomicU64, Ordering},
    Arc,
};

static SEQUENCE: AtomicU64 = AtomicU64::new(1);
fn id() -> String {
    format!(
        "00000000-0000-4000-8000-{:012x}",
        SEQUENCE.fetch_add(1, Ordering::Relaxed)
    )
}

#[tokio::test]
#[ignore = "build and Compose workflow proof on the recorded owned engine"]
async fn owned_build_compose_workflow() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_millis();
    let project = format!("orbwin-compose-{stamp}");
    let tag = format!("orbwin-build:{stamp}");
    let alias = format!("orbwin-build-alias:{stamp}");
    let root = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("..");
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: root.join(format!(
            "artifacts/qualification/build-compose-{stamp}.json"
        )),
    };
    let file = root
        .join("tests/workloads/compose.yaml")
        .to_str()
        .unwrap()
        .to_string();
    let mut built = false;
    let mut tagged = false;
    let result: Result<(), String> = async {
        harness
            .pass(Request::Build {
                directory: root.join("tests/workloads/build").to_str().unwrap().into(),
                tag: tag.clone(),
            })
            .await?;
        built = true;
        let target = engine::target()?;
        let image =
            engine::command(&target, &["image", "inspect", "--format={{.Id}}", &tag]).await?;
        harness
            .pass(Request::Tag {
                target: image.trim().into(),
                tag: alias.clone(),
            })
            .await?;
        tagged = true;
        harness
            .pass(Request::Inspect {
                resource: Resource::Image,
                target: image.trim().into(),
            })
            .await?;
        harness
            .pass(Request::Compose {
                file: file.clone(),
                project: project.clone(),
                action: ComposeAction::Up,
            })
            .await?;
        let filter = format!("label=com.docker.compose.project={project}");
        let redis = engine::command(
            &target,
            &[
                "ps",
                "--no-trunc",
                "--quiet",
                "--filter",
                &filter,
                "--filter",
                "label=com.docker.compose.service=redis",
            ],
        )
        .await?;
        let redis = redis.trim().to_string();
        super::target(Resource::Container, &redis)?;
        harness
            .pass(Request::Exec {
                target: redis.clone(),
                command: vec![
                    "redis-cli".into(),
                    "set".into(),
                    "orbwin-adapter-proof".into(),
                    project.clone(),
                ],
            })
            .await?;
        harness
            .pass(Request::Compose {
                file: file.clone(),
                project: project.clone(),
                action: ComposeAction::Restart,
            })
            .await?;
        harness
            .pass(Request::Compose {
                file: file.clone(),
                project: project.clone(),
                action: ComposeAction::Up,
            })
            .await?;
        let value = harness
            .pass(Request::Exec {
                target: redis.clone(),
                command: vec![
                    "redis-cli".into(),
                    "get".into(),
                    "orbwin-adapter-proof".into(),
                ],
            })
            .await?;
        if value.trim() != project {
            return Err("Compose volume data did not survive restart".into());
        }
        harness
            .pass(Request::Compose {
                file: file.clone(),
                project: project.clone(),
                action: ComposeAction::Logs,
            })
            .await?;
        harness
            .pass(Request::Compose {
                file: file.clone(),
                project: project.clone(),
                action: ComposeAction::Stop,
            })
            .await?;
        harness
            .pass(Request::Compose {
                file: file.clone(),
                project: project.clone(),
                action: ComposeAction::Start,
            })
            .await?;
        Ok(())
    }
    .await;
    let mut cleanup_errors = vec![];
    if let Err(e) = harness
        .pass(Request::Compose {
            file,
            project: project.clone(),
            action: ComposeAction::Down,
        })
        .await
    {
        cleanup_errors.push(e);
    }
    for suffix in ["redis-data", "postgres-data"] {
        if let Err(e) = harness
            .pass(Request::Remove {
                resource: Resource::Volume,
                target: format!("{project}_{suffix}"),
            })
            .await
        {
            cleanup_errors.push(e);
        }
    }
    if tagged {
        if let Err(e) = harness.pass(Request::Untag { reference: alias }).await {
            cleanup_errors.push(e);
        }
    }
    if built {
        if let Err(e) = harness.pass(Request::Untag { reference: tag }).await {
            cleanup_errors.push(e);
        }
    }
    assert!(
        result.is_ok() && cleanup_errors.is_empty(),
        "result: {result:?}; cleanup: {cleanup_errors:?}; evidence: {}",
        harness.evidence.display()
    );
}
struct Harness {
    state: Operations,
    records: Vec<serde_json::Value>,
    evidence: std::path::PathBuf,
}

#[tokio::test]
#[ignore = "Windows bind mount runtime proof; wait for explicit host availability"]
async fn owned_windows_bind_mounts() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_nanos();
    let root = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("..")
        .canonicalize()
        .unwrap();
    let name = format!("orbwin-bind-{stamp}");
    let source = root.join(format!("artifacts/qualification/bind source {stamp}.txt"));
    let file = std::fs::OpenOptions::new()
        .write(true)
        .create_new(true)
        .open(&source)
        .unwrap();
    drop(file);
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: root.join(format!("artifacts/qualification/bind-{stamp}.json")),
    };
    let mut container = None;
    let result: Result<(), String> = async {
        let created = harness.pass(Request::Run {
            image: "alpine:3.21@sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30".into(),
            name: name.clone(), command: vec!["sleep".into(), "300".into()],
            options: RunOptions { volumes: vec![
                format!("{}:/fixture:ro", root.join("tests/workloads/bind source").display()),
                format!("{}:/writable.txt", source.display()),
                format!("{}:/readonly.txt:ro", source.display()),
            ], ..Default::default() },
        }).await?;
        let target = created.trim().to_string(); container = Some(target.clone());
        let value = harness.pass(Request::Exec { target: target.clone(), command: vec!["cat".into(), "/fixture/hello.txt".into()] }).await?;
        if value.trim() != "orbwin-windows-bind-proof" { return Err(format!("Wrong directory mount content: {value}")); }
        let (readonly, _) = harness.call(Request::Exec { target: target.clone(), command: vec!["sh".into(), "-c".into(), "printf forbidden >> /readonly.txt".into()] }).await?;
        if readonly.success || readonly.exit_code == Some(0) { return Err("Read-only host mount accepted a write".into()); }
        harness.pass(Request::Exec { target: target.clone(), command: vec!["sh".into(), "-c".into(), "printf orbwin-file-mount-proof > /writable.txt".into()] }).await?;
        if std::fs::read_to_string(&source).map_err(|e| e.to_string())? != "orbwin-file-mount-proof" {
            return Err("Container file write was not visible on Windows".into());
        }
        harness.pass(Request::Container { target: target.clone(), action: ContainerAction::Restart }).await?;
        let value = harness.pass(Request::Exec { target, command: vec!["cat".into(), "/writable.txt".into()] }).await?;
        if value != "orbwin-file-mount-proof" { return Err("Host file data did not survive container restart".into()); }
        Ok(())
    }.await;
    let mut cleanup = Ok(());
    if let Some(target) = container {
        cleanup = async {
            harness
                .pass(Request::Container {
                    target: target.clone(),
                    action: ContainerAction::Stop,
                })
                .await?;
            harness
                .pass(Request::Remove {
                    resource: Resource::Container,
                    target,
                })
                .await?;
            Ok::<(), String>(())
        }
        .await;
    }
    if cleanup.is_ok() {
        std::fs::remove_file(&source).unwrap();
    }
    assert!(
        result.is_ok() && cleanup.is_ok(),
        "result: {result:?}; cleanup: {cleanup:?}; evidence: {}",
        harness.evidence.display()
    );
}
impl Harness {
    async fn call(&mut self, request: Request) -> Result<(Outcome, String), String> {
        eprintln!("owned qualification: {request:?}");
        let text = Arc::new(Mutex::new(String::new()));
        let received = text.clone();
        let channel = Channel::new(move |body| {
            let event: serde_json::Value = body.deserialize()?;
            if event["kind"] == "data" {
                let bytes: Vec<u8> = serde_json::from_value(event["bytes"].clone())?;
                received
                    .lock()
                    .unwrap()
                    .push_str(&String::from_utf8_lossy(&bytes));
            }
            Ok(())
        });
        let description = format!("{request:?}");
        let result = run(&self.state, id(), request, channel).await;
        let output = text.lock().unwrap().clone();
        self.records.push(serde_json::json!({ "request": description, "outcome": result.as_ref().ok(), "error": result.as_ref().err(), "output": output }));
        std::fs::write(
            &self.evidence,
            serde_json::to_vec_pretty(&self.records).unwrap(),
        )
        .map_err(|e| e.to_string())?;
        result.map(|outcome| (outcome, output))
    }
    async fn pass(&mut self, request: Request) -> Result<String, String> {
        let (outcome, output) = self.call(request).await?;
        if !outcome.success {
            return Err(format!("{}\n{output}", outcome.message));
        }
        Ok(output)
    }
}

#[tokio::test]
#[ignore = "requires ORBWIN_QUALIFY_OWNED=1 and the verified owned running engine"]
async fn owned_resource_operations() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_millis();
    let name = format!("orbwin-operations-{stamp}");
    let root = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("..");
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: root.join(format!("artifacts/qualification/operations-{stamp}.json")),
    };
    let mut container = None;
    let mut network = None;
    let mut volume = false;
    let result: Result<(), String> = async {
        harness.pass(Request::CreateVolume { name: name.clone() }).await?; volume = true;
        harness.pass(Request::Inspect { resource: Resource::Volume, target: name.clone() }).await?;
        network = Some(harness.pass(Request::CreateNetwork { name: name.clone() }).await?.trim().to_string());
        let network_id = network.clone().unwrap();
        harness.pass(Request::Inspect { resource: Resource::Network, target: network_id.clone() }).await?;
        let created = harness.pass(Request::Run { image: "alpine:3.21@sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30".into(), name: name.clone(), command: vec!["sh".into(), "-c".into(), "echo orbwin-log-proof; sleep 300".into()], options: RunOptions { ports: vec!["8080".into()], environment: vec!["ORBWIN_PROOF=container-environment".into()], volumes: vec![format!("{name}:/proof-data")] } }).await?;
        let container_id = created.trim().to_string(); container = Some(container_id.clone());
        let details: serde_json::Value = serde_json::from_str(&harness.pass(Request::Inspect { resource: Resource::Container, target: container_id.clone() }).await?).map_err(|e| e.to_string())?;
        if details[0]["HostConfig"]["PortBindings"]["8080/tcp"][0]["HostIp"] != "127.0.0.1" { return Err("Default publication escaped localhost".into()); }
        if !details[0]["Config"]["Env"].as_array().ok_or("Missing environment")?.iter().any(|entry| entry == "ORBWIN_PROOF=container-environment") { return Err("Requested environment was not applied".into()); }
        harness.pass(Request::Exec { target: container_id.clone(), command: vec!["sh".into(), "-c".into(), "printf preserved > /proof-data/sentinel".into()] }).await?;
        let (attached_volume, _) = harness.call(Request::Remove { resource: Resource::Volume, target: name.clone() }).await?;
        if attached_volume.success { return Err("Attached volume removal unexpectedly succeeded".into()); }
        harness.pass(Request::ConnectNetwork { network: network_id.clone(), container: container_id.clone(), disconnect: false }).await?;
        let (_, refused) = harness.call(Request::Remove { resource: Resource::Network, target: network_id.clone() }).await?;
        if !refused.contains("active endpoints") { return Err(format!("Expected in-use network rejection: {refused}")); }
        harness.pass(Request::ConnectNetwork { network: network_id, container: container_id.clone(), disconnect: true }).await?;
        let value = harness.pass(Request::Exec { target: container_id.clone(), command: vec!["printf".into(), "orbwin-exec-proof".into()] }).await?;
        if value != "orbwin-exec-proof" { return Err(format!("Unexpected exec result: {value}")); }
        let (failure, _) = harness.call(Request::Exec { target: container_id.clone(), command: vec!["sh".into(), "-c".into(), "exit 7".into()] }).await?;
        if failure.exit_code != Some(7) || failure.success { return Err("Container command exit status was lost".into()); }
        let logs = harness.pass(Request::Logs { target: container_id.clone(), follow: false }).await?;
        if !logs.contains("orbwin-log-proof") { return Err("Expected log line missing".into()); }
        harness.pass(Request::Stats { target: container_id.clone() }).await?;
        for action in [ContainerAction::Pause, ContainerAction::Unpause, ContainerAction::Restart, ContainerAction::Stop, ContainerAction::Start] {
            harness.pass(Request::Container { action, target: container_id.clone() }).await?;
        }
        let preserved = harness.pass(Request::Exec { target: container_id.clone(), command: vec!["cat".into(), "/proof-data/sentinel".into()] }).await?;
        if preserved != "preserved" { return Err("Named-volume data did not survive the lifecycle sequence".into()); }
        let (refused, _) = harness.call(Request::Remove { resource: Resource::Container, target: container_id.clone() }).await?;
        if refused.success { return Err("Running container removal unexpectedly succeeded".into()); }
        // Cancel follow after the first actual log payload, so cancellation
        // exercises a dispatched client rather than just preflight rejection.
        let job_id = id(); let state = &harness.state;
        let (ready_tx, ready_rx) = oneshot::channel(); let ready = Mutex::new(Some(ready_tx));
        let channel = Channel::new(move |body| { let event: serde_json::Value = body.deserialize()?; if event["kind"] == "data" { if let Some(sender) = ready.lock().unwrap().take() { let _ = sender.send(()); } } Ok(()) });
        let running = run(state, job_id.clone(), Request::Logs { target: container_id, follow: true }, channel);
        let cancel_client = async { tokio::time::timeout(Duration::from_secs(20), ready_rx).await.map_err(|_| "Follow emitted no output")?.map_err(|_| "Follow ended before output")?; cancel(state, &job_id) };
        let (outcome, cancelled) = tokio::join!(running, cancel_client); cancelled?;
        let outcome = outcome?;
        harness.records.push(serde_json::json!({ "case": "cancel active log follow", "outcome": outcome }));
        if !outcome.interrupted || outcome.success { return Err("Active follow cancellation was not reflected".into()); }
        Ok(())
    }.await;
    let mut cleanup_errors = vec![];
    if let Some(container) = container {
        if let Err(e) = harness
            .pass(Request::Container {
                target: container.clone(),
                action: ContainerAction::Stop,
            })
            .await
        {
            cleanup_errors.push(e);
        }
        if let Err(e) = harness
            .pass(Request::Remove {
                resource: Resource::Container,
                target: container,
            })
            .await
        {
            cleanup_errors.push(e);
        }
    }
    if let Some(network) = network {
        if let Err(e) = harness
            .pass(Request::Remove {
                resource: Resource::Network,
                target: network,
            })
            .await
        {
            cleanup_errors.push(e);
        }
    }
    if volume {
        if let Err(e) = harness
            .pass(Request::Remove {
                resource: Resource::Volume,
                target: name,
            })
            .await
        {
            cleanup_errors.push(e);
        }
    }
    assert!(
        result.is_ok() && cleanup_errors.is_empty(),
        "Result: {result:?}; cleanup: {cleanup_errors:?}; evidence: {}",
        harness.evidence.display()
    );
}

#[tokio::test]
#[ignore = "explicit owned engine stop/start proof; requires no running OrbWin containers"]
async fn owned_management_cycle() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_millis();
    let root = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("..");
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: root.join(format!("artifacts/qualification/management-{stamp}.json")),
    };
    let target = engine::target().unwrap();
    let before = engine::check(&target).await.unwrap();
    let mut command = Command::new(&target.docker);
    command
        .arg("--config")
        .arg(&target.config)
        .arg("--host")
        .arg(engine::ENDPOINT)
        .args(["ps", "--quiet"]);
    let containers = command.output().await.unwrap();
    assert!(
        containers.status.success() && containers.stdout.is_empty(),
        "Refusing to interrupt running OrbWin containers"
    );
    let config = harness.pass(Request::ReadSettings).await.unwrap();
    let app: serde_json::Value = serde_json::from_str(&config).unwrap();
    assert_eq!(app["spec"]["running"], true);
    let stopped = harness.pass(Request::Engine { running: false }).await;
    // Always attempt restoration even if stop failed after committing.
    let started = harness.pass(Request::Engine { running: true }).await;
    assert!(
        stopped.is_ok() && started.is_ok(),
        "stop: {stopped:?}; start: {started:?}"
    );
    let after = engine::check(&target).await.unwrap();
    assert_eq!(before["ID"], after["ID"]);
}

#[tokio::test]
#[ignore = "optional owned Kubernetes enable/workload/storage/delete/disable proof"]
async fn owned_cluster_workflow() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_millis();
    let name = format!("orbwin-cluster-{stamp}");
    let root = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("..");
    let mut harness = Harness {
        state: Operations::default(),
        records: vec![],
        evidence: root.join(format!("artifacts/qualification/cluster-{stamp}.json")),
    };
    let config: serde_json::Value =
        serde_json::from_str(&harness.pass(Request::ReadSettings).await.unwrap()).unwrap();
    let was_enabled = config["spec"]["kubernetes"]["enabled"]
        .as_bool()
        .unwrap_or(false);
    let file = root.join(format!(
        "artifacts/qualification/cluster-manifest-{stamp}.json"
    ));
    let manifest = serde_json::json!({ "apiVersion":"v1", "kind":"List", "items":[
        {"apiVersion":"v1", "kind":"PersistentVolumeClaim", "metadata":{"name":name,"namespace":"default"},"spec":{"accessModes":["ReadWriteOnce"],"resources":{"requests":{"storage":"16Mi"}}}},
        {"apiVersion":"apps/v1","kind":"Deployment","metadata":{"name":name,"namespace":"default"},"spec":{"replicas":1,"selector":{"matchLabels":{"app":name}},"template":{"metadata":{"labels":{"app":name}},"spec":{"containers":[{"name":"server","image":"redis:7.4.2-alpine@sha256:02419de7eddf55aa5bcf49efb74e88fa8d931b4d77c07eff8a6b2144472b6952","command":["sh","-c","echo orbwin-cluster-proof; echo preserved > /data/proof; exec redis-server --dir /data --appendonly yes"],"ports":[{"containerPort":6379}],"readinessProbe":{"tcpSocket":{"port":6379},"initialDelaySeconds":1,"periodSeconds":1},"volumeMounts":[{"name":"data","mountPath":"/data"}]}],"volumes":[{"name":"data","persistentVolumeClaim":{"claimName":name}}]}}}},
        {"apiVersion":"v1","kind":"Service","metadata":{"name":name,"namespace":"default"},"spec":{"selector":{"app":name},"ports":[{"port":80,"targetPort":6379}]}}
    ]});
    std::fs::write(&file, serde_json::to_vec_pretty(&manifest).unwrap()).unwrap();
    let result: Result<(), String> = async {
        if !was_enabled {
            harness
                .pass(Request::ConfigureCluster { enabled: true })
                .await?;
        }
        harness
            .pass(Request::ClusterApply {
                file: file.to_str().unwrap().into(),
            })
            .await?;
        harness
            .pass(Request::ClusterWait {
                resource: ClusterResource::Deployments,
                namespace: "default".into(),
                name: name.clone(),
                deleted: false,
            })
            .await?;
        harness
            .pass(Request::ClusterWait {
                resource: ClusterResource::PersistentVolumeClaims,
                namespace: "default".into(),
                name: name.clone(),
                deleted: false,
            })
            .await?;
        for resource in [
            ClusterResource::Deployments,
            ClusterResource::Services,
            ClusterResource::PersistentVolumeClaims,
        ] {
            let output = harness
                .pass(Request::Cluster {
                    action: ClusterAction::Inspect,
                    resource,
                    namespace: "default".into(),
                    name: name.clone(),
                })
                .await?;
            let object: serde_json::Value =
                serde_json::from_str(&output).map_err(|e| e.to_string())?;
            if object["metadata"]["name"] != name {
                return Err("Cluster returned a different resource".into());
            }
        }
        let pods: serde_json::Value = serde_json::from_str(
            &harness
                .pass(Request::Cluster {
                    action: ClusterAction::List,
                    resource: ClusterResource::Pods,
                    namespace: "default".into(),
                    name: "".into(),
                })
                .await?,
        )
        .map_err(|e| e.to_string())?;
        let pod = pods["items"]
            .as_array()
            .ok_or("No pod list")?
            .iter()
            .find(|pod| pod["metadata"]["labels"]["app"] == name)
            .ok_or("Test deployment has no pod")?;
        let logs = harness
            .pass(Request::Cluster {
                action: ClusterAction::Logs,
                resource: ClusterResource::Pods,
                namespace: "default".into(),
                name: pod["metadata"]["name"]
                    .as_str()
                    .ok_or("Pod has no name")?
                    .into(),
            })
            .await?;
        if !logs.contains("orbwin-cluster-proof") {
            return Err("Cluster log output missing".into());
        }
        let (wrong, _) = harness
            .call(Request::ClusterDelete {
                resource: ClusterResource::Services,
                namespace: "default".into(),
                name: name.clone(),
                uid: "00000000-0000-4000-8000-000000000000".into(),
            })
            .await?;
        if wrong.success {
            return Err("Wrong UID deletion unexpectedly succeeded".into());
        }
        harness
            .pass(Request::Cluster {
                action: ClusterAction::Inspect,
                resource: ClusterResource::Services,
                namespace: "default".into(),
                name: name.clone(),
            })
            .await?;
        Ok(())
    }
    .await;
    if result.is_err() {
        for resource in [
            ClusterResource::Pods,
            ClusterResource::Events,
            ClusterResource::PersistentVolumeClaims,
        ] {
            let _ = harness
                .call(Request::Cluster {
                    action: ClusterAction::List,
                    resource,
                    namespace: "default".into(),
                    name: "".into(),
                })
                .await;
        }
    }
    let mut cleanup_errors = vec![];
    for resource in [
        ClusterResource::Deployments,
        ClusterResource::Services,
        ClusterResource::PersistentVolumeClaims,
    ] {
        // Cleanup discovers the UID of only the exact unique test name. A
        // failed/partial apply must never turn cleanup into a broad deletion.
        match harness
            .call(Request::Cluster {
                action: ClusterAction::Inspect,
                resource,
                namespace: "default".into(),
                name: name.clone(),
            })
            .await
        {
            Ok((outcome, output)) if outcome.success => {
                match serde_json::from_str::<serde_json::Value>(&output) {
                    Ok(object) => {
                        if let Some(uid) = object["metadata"]["uid"].as_str() {
                            if let Err(e) = harness
                                .pass(Request::ClusterDelete {
                                    resource,
                                    namespace: "default".into(),
                                    name: name.clone(),
                                    uid: uid.into(),
                                })
                                .await
                            {
                                cleanup_errors.push(e);
                            }
                            if let Err(e) = harness
                                .pass(Request::ClusterWait {
                                    resource,
                                    namespace: "default".into(),
                                    name: name.clone(),
                                    deleted: true,
                                })
                                .await
                            {
                                cleanup_errors.push(e);
                            }
                        } else {
                            cleanup_errors.push("Cleanup object has no UID".into());
                        }
                    }
                    Err(e) => cleanup_errors.push(e.to_string()),
                }
            }
            Ok((_, output)) if output.contains("NotFound") || output.contains("not found") => {}
            Ok((outcome, output)) => cleanup_errors.push(format!("{}\n{output}", outcome.message)),
            Err(e) => cleanup_errors.push(e),
        }
    }
    if !was_enabled {
        if let Err(e) = harness
            .pass(Request::ConfigureCluster { enabled: false })
            .await
        {
            cleanup_errors.push(e);
        }
    }
    assert!(
        result.is_ok() && cleanup_errors.is_empty(),
        "result: {result:?}; cleanup: {cleanup_errors:?}; evidence: {}",
        harness.evidence.display()
    );
}
