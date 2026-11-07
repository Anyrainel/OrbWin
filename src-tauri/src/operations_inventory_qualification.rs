//! Explicit opt-in production-adapter qualification. Root serializes engine use.
//! Only nonce-owned full-ID fixtures are stopped/removed; no force/prune/lifecycle.
use super::*;
use serde_json::{json, Value};
use std::{path::PathBuf, sync::Arc};

struct Proof {
    state: Operations,
    path: PathBuf,
    events: Vec<Value>,
}
impl Proof {
    fn save(&self) -> Result<(), String> {
        std::fs::write(
            &self.path,
            serde_json::to_vec_pretty(&self.events).map_err(|e| e.to_string())?,
        )
        .map_err(|e| e.to_string())
    }
    async fn call(&mut self, request: Request) -> Result<(Outcome, String), String> {
        let output = Arc::new(Mutex::new(String::new()));
        let received = output.clone();
        let channel = Channel::new(move |body| {
            let event: Value = body.deserialize()?;
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
        self.events
            .push(json!({"phase":"intent","request":description}));
        self.save()?;
        let operation = format!(
            "{:08x}-0000-4000-8000-{:012x}",
            std::process::id(),
            self.events.len()
        );
        let result = Box::pin(run(&self.state, operation, request, channel)).await;
        let text = output.lock().map_err(|_| "Output lock poisoned")?.clone();
        self.events.push(json!({"request":description,"outcome":result.as_ref().ok(),"error":result.as_ref().err(),"output":text}));
        self.save()?;
        result.map(|outcome| (outcome, text))
    }
    async fn pass(&mut self, request: Request) -> Result<String, String> {
        let (outcome, text) = self.call(request).await?;
        if !outcome.success {
            return Err(format!("{} {text}", outcome.message));
        }
        Ok(text)
    }
    async fn inspect(&mut self, resource: Resource, id: &str) -> Result<Value, String> {
        let text = self
            .pass(Request::Inspect {
                resource,
                target: id.into(),
            })
            .await?;
        let value: Value = serde_json::from_str(&text).map_err(|e| e.to_string())?;
        value
            .as_array()
            .and_then(|v| if v.len() == 1 { v.first() } else { None })
            .cloned()
            .ok_or("Expected one inspect result".into())
    }
    async fn exec(&mut self, id: &str, command: &[&str]) -> Result<String, String> {
        self.pass(Request::Exec {
            target: id.into(),
            command: command.iter().map(|v| (*v).into()).collect(),
        })
        .await
    }
}
fn require(condition: bool, message: &str) -> Result<(), String> {
    if condition {
        Ok(())
    } else {
        Err(message.into())
    }
}
fn full_id(text: &str) -> Result<String, String> {
    let id = text.trim();
    require(
        id.len() == 64 && id.bytes().all(|c| c.is_ascii_hexdigit()),
        "Expected full fixture ID",
    )?;
    Ok(id.into())
}
async fn inventory() -> Result<Value, String> {
    serde_json::to_value(engine::inventory().await?).map_err(|e| e.to_string())
}
fn rows<'a>(inventory: &'a Value, kind: &str) -> Result<&'a Vec<Value>, String> {
    inventory[kind]
        .as_array()
        .ok_or_else(|| format!("Missing {kind} inventory"))
}
fn fixture_container(value: &Value, name: &str, image: &str, marker: &str) -> Result<(), String> {
    require(
        value["Name"] == format!("/{name}")
            && value["Image"] == image
            && value["HostConfig"]["Privileged"] == false,
        "Container identity/security differs",
    )?;
    require(
        value["Mounts"].as_array().is_some_and(|v| v.is_empty()),
        "Fixture unexpectedly has mounts",
    )?;
    require(
        value["Config"]["Env"]
            .as_array()
            .is_some_and(|v| v.iter().any(|v| v == marker)),
        "Container ownership nonce missing",
    )
}
fn fixture_network(value: &Value, name: &str, subnet: &str) -> Result<(), String> {
    require(
        value["Name"] == name
            && value["Driver"] == "bridge"
            && value["Internal"] == true
            && value["Scope"] == "local",
        "Network identity/options differ",
    )?;
    require(
        value["IPAM"]["Config"]
            .as_array()
            .is_some_and(|v| v.iter().any(|v| v["Subnet"] == subnet)),
        "Network subnet differs",
    )
}
fn stat(text: &str, name: &str) -> Result<u64, String> {
    text.lines()
        .find_map(|line| {
            line.split_once(' ')
                .filter(|(key, _)| *key == name)
                .map(|(_, value)| value.trim().parse::<u64>())
        })
        .ok_or_else(|| format!("Missing cgroup counter {name}"))?
        .map_err(|e| e.to_string())
}

#[tokio::test]
#[ignore = "explicit owned engine network/restart/CPU/RAM fixtures; root owns serial engine lane"]
async fn owned_inventory_network_limits() {
    let result = qualify().await;
    assert!(result.is_ok(), "{result:?}");
}
async fn qualify() -> Result<(), String> {
    require(
        std::env::var("ORBWIN_QUALIFY_OWNED").as_deref() == Ok("1"),
        "Explicit owned-engine opt-in required",
    )?;
    let nonce = std::env::var("ORBWIN_INVENTORY_NONCE").map_err(|e| e.to_string())?;
    require(
        nonce.len() == 32 && nonce.bytes().all(|b| b.is_ascii_hexdigit()),
        "Invalid qualification nonce",
    )?;
    let image = std::env::var("ORBWIN_QUALIFY_FIXTURE_IMAGE").map_err(|e| e.to_string())?;
    target(Resource::Image, &image)?;
    require(
        image.starts_with("sha256:"),
        "Use an immutable full image digest",
    )?;
    let subnet = std::env::var("ORBWIN_QUALIFY_SUBNET").map_err(|e| e.to_string())?;
    let base = format!("orbwin-inventory-{nonce}");
    let root = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../artifacts/qualification");
    let folder = root.join(&base);
    require(
        folder.is_dir(),
        "Orchestrator must create evidence folder before execution",
    )?;
    let path = folder.join("native-records.json");
    require(!path.exists(), "Native proof path already exists")?;
    let target_engine = engine::target()?;
    let identity = engine::check(&target_engine).await?;
    require(
        identity["ID"] == "d9c00194-ea9d-439a-8561-6a0ec580fbd5",
        "Owned daemon identity drift",
    )?;
    let mut proof = Proof {
        state: Operations::default(),
        path,
        events: vec![
            json!({"nonce":nonce,"daemon":identity["ID"],"image":image,"subnet":subnet,"started":std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).map_err(|e|e.to_string())?.as_secs()}),
        ],
    };
    proof.save()?;
    let image_value = proof.inspect(Resource::Image, &image).await?;
    require(
        image_value["Id"] == image
            && image_value["Os"] == "linux"
            && image_value["Architecture"] == "amd64",
        "Cached fixture image/platform differs",
    )?;
    require(
        image_value["Config"]["Volumes"].is_null()
            || image_value["Config"]["Volumes"]
                .as_object()
                .is_some_and(|v| v.is_empty()),
        "Fixture image declares anonymous volumes",
    )?;
    let initial = inventory().await?;
    require(
        !rows(&initial, "containers")?
            .iter()
            .any(|v| v["Names"].as_str().is_some_and(|n| n.starts_with(&base)))
            && !rows(&initial, "networks")?
                .iter()
                .any(|v| v["Name"].as_str().is_some_and(|n| n.starts_with(&base))),
        "Fixture namespace already exists",
    )?;
    let network_name = format!("{base}-net");
    let overlap_name = format!("{base}-overlap");
    let marker = format!("ORBWIN_QUALIFICATION_NONCE={nonce}");
    let mut container_names = Vec::<String>::new();
    let mut network_names = Vec::<String>::new();
    let result: Result<(), String> = async {
        network_names.push(network_name.clone());
        let network = full_id(&proof.pass(Request::CreateNetwork { name:network_name.clone(), driver:"bridge".into(), subnet:subnet.clone(), internal:true }).await?)?;
        fixture_network(&proof.inspect(Resource::Network,&network).await?, &network_name, &subnet)?;
        let duplicate = proof.call(Request::CreateNetwork { name:network_name.clone(), driver:"bridge".into(), subnet:String::new(), internal:true }).await?;
        require(!duplicate.0.success,"Duplicate network name was accepted")?;
        network_names.push(overlap_name.clone());
        let overlap = proof.call(Request::CreateNetwork { name:overlap_name.clone(), driver:"bridge".into(), subnet:subnet.clone(), internal:true }).await?;
        require(!overlap.0.success,"Overlapping subnet was accepted")?;
        for policy in ["no","on-failure","unless-stopped","always"] {
            let name = format!("{base}-{policy}"); container_names.push(name.clone());
            let id = full_id(&proof.pass(Request::Run { image:image.clone(), name:name.clone(), command:vec!["sleep".into(),"120".into()], options:RunOptions { environment:vec![marker.clone()], network:network_name.clone(), restart:policy.into(), cpus:"0.1".into(), memory_mib:"32".into(), ..Default::default() } }).await?)?;
            let value = proof.inspect(Resource::Container,&id).await?; fixture_container(&value,&name,&image,&marker)?;
            require(value["HostConfig"]["RestartPolicy"]["Name"] == policy && value["HostConfig"]["NanoCpus"] == 100_000_000u64 && value["HostConfig"]["Memory"] == 33_554_432u64 && value["NetworkSettings"]["Networks"][&network_name]["NetworkID"] == network,"Run options differ")?;
            // Verify the immutable cached BusyBox applets and timeout syntax
            // before starting any busy/allocation workload.
            proof.exec(&id,&["/bin/busybox","timeout","-s","KILL","2","/bin/busybox","true"]).await?;
            proof.exec(&id,&["/bin/busybox","awk","BEGIN{exit 0}"]).await?;
            proof.pass(Request::Container { target:id.clone(), action:ContainerAction::Stop }).await?;
        }
        let cpu_name = format!("{base}-cpu"); container_names.push(cpu_name.clone());
        let cpu = full_id(&proof.pass(Request::Run { image:image.clone(),name:cpu_name.clone(),command:["/bin/busybox","timeout","-s","KILL","120","/bin/busybox","sh","-c","while :; do :; done"].iter().map(|v|(*v).into()).collect(),options:RunOptions {environment:vec![marker.clone()],network:network_name.clone(),cpus:"0.1".into(),memory_mib:"32".into(),..Default::default()} }).await?)?;
        fixture_container(&proof.inspect(Resource::Container,&cpu).await?,&cpu_name,&image,&marker)?;
        let quota = proof.exec(&cpu,&["cat","/sys/fs/cgroup/cpu.max"]).await?;
        let fields:Vec<_> = quota.split_whitespace().collect(); require(fields.len()==2 && fields[0].parse::<u64>().ok().zip(fields[1].parse::<u64>().ok()).is_some_and(|(quota,period)|quota.checked_mul(10)==Some(period)),"CPU cgroup quota differs or v2 unavailable")?;
        let before = stat(&proof.exec(&cpu,&["cat","/sys/fs/cgroup/cpu.stat"]).await?,"nr_throttled")?;
        tokio::time::sleep(Duration::from_secs(3)).await;
        let after = stat(&proof.exec(&cpu,&["cat","/sys/fs/cgroup/cpu.stat"]).await?,"nr_throttled")?;
        require(after>before,"CPU workload did not show quota throttling")?;
        let before_update = proof.inspect(Resource::Container,&cpu).await?;
        proof.pass(Request::UpdateContainerLimits {target:cpu.clone(),cpus:"0.2".into(),memory_mib:String::new()}).await?;
        let updated = proof.inspect(Resource::Container,&cpu).await?;
        require(updated["HostConfig"]["NanoCpus"]==200_000_000u64 && updated["HostConfig"]["Memory"]==before_update["HostConfig"]["Memory"] && updated["State"]["StartedAt"]==before_update["State"]["StartedAt"],"CPU update changed blank RAM or restarted fixture")?;
        let updated_quota=proof.exec(&cpu,&["cat","/sys/fs/cgroup/cpu.max"]).await?;
        let fields:Vec<_>=updated_quota.split_whitespace().collect();require(fields.len()==2&&fields[0].parse::<u64>().ok().zip(fields[1].parse::<u64>().ok()).is_some_and(|(quota,period)|quota.checked_mul(5)==Some(period)),"Updated CPU cgroup quota differs")?;
        proof.pass(Request::UpdateContainerLimits {target:cpu.clone(),cpus:String::new(),memory_mib:"24".into()}).await?;
        let updated = proof.inspect(Resource::Container,&cpu).await?;
        require(updated["HostConfig"]["NanoCpus"]==200_000_000u64 && updated["HostConfig"]["Memory"]==25_165_824u64 && updated["State"]["StartedAt"]==before_update["State"]["StartedAt"],"RAM update changed blank CPU, restarted fixture or did not apply")?;
        require(proof.exec(&cpu,&["cat","/sys/fs/cgroup/memory.max"]).await?.trim()=="25165824","Updated RAM cgroup limit differs")?;
        proof.pass(Request::Container {target:cpu,action:ContainerAction::Stop}).await?;
        let oom_name=format!("{base}-oom");container_names.push(oom_name.clone());
        // Init waits for an explicit arm marker. No allocation before cgroup bounds are verified.
        let oom=full_id(&proof.pass(Request::Run {image:image.clone(),name:oom_name.clone(),command:["/bin/busybox","timeout","-s","KILL","90","/bin/busybox","sh","-c","while [ ! -f /tmp/armed ]; do sleep 1; done; exec /bin/busybox awk 'BEGIN{s=\"0123456789abcdef\"; for(i=0;i<24;i++)s=s s; print length(s)}'"].iter().map(|v|(*v).into()).collect(),options:RunOptions{environment:vec![marker.clone()],network:network_name.clone(),cpus:"0.1".into(),memory_mib:"32".into(),..Default::default()}}).await?)?;
        fixture_container(&proof.inspect(Resource::Container,&oom).await?,&oom_name,&image,&marker)?;
        let memory=proof.exec(&oom,&["cat","/sys/fs/cgroup/memory.max"]).await?;
        let swap=proof.exec(&oom,&["cat","/sys/fs/cgroup/memory.swap.max"]).await?;
        require(memory.trim().parse::<u64>().ok()==Some(33_554_432) && swap.trim().parse::<u64>().ok().is_some_and(|v|v<=33_554_432),"Refuse OOM: memory/swap is unbounded or cgroup v2 unavailable")?;
        proof.exec(&oom,&["touch","/tmp/armed"]).await?;
        let mut killed=false;
        // Forty observations plus native client deadlines, not a promised 10s
        // wall-clock bound. Init self-exits at 90s even if this harness is lost.
        for _ in 0..40 { let value=proof.inspect(Resource::Container,&oom).await?; if value["State"]["Running"]==false { require(value["State"]["OOMKilled"]==true && value["State"]["ExitCode"]==137,"Bounded allocation ended without expected OOM evidence")?; killed=true;break; } tokio::time::sleep(Duration::from_millis(250)).await; }
        require(killed,"Bounded OOM probe did not finish in forty observations")?;
        Ok(())
    }.await;
    let mut cleanup_errors = Vec::<String>::new();
    // Discover uncertain accepted creations only by exact nonce names, then
    // verify immutable image + env ownership/mounts before touching full IDs.
    match inventory().await {
        Ok(current) => {
            for row in rows(&current, "containers")? {
                let Some(name) = row["Names"]
                    .as_str()
                    .filter(|n| container_names.iter().any(|v| v.as_str() == *n))
                else {
                    continue;
                };
                let cleanup: Result<(), String> = async {
                    let id = full_id(row["ID"].as_str().ok_or("Missing container ID")?)?;
                    let value = proof.inspect(Resource::Container, &id).await?;
                    fixture_container(&value, name, &image, &marker)?;
                    if value["State"]["Running"] == true {
                        proof
                            .pass(Request::Container {
                                target: id.clone(),
                                action: ContainerAction::Stop,
                            })
                            .await?;
                    }
                    let retained = proof.inspect(Resource::Container, &id).await?;
                    fixture_container(&retained, name, &image, &marker)?;
                    require(
                        retained["State"]["Running"] == false,
                        "Fixture still running",
                    )?;
                    proof
                        .pass(Request::Remove {
                            resource: Resource::Container,
                            target: id,
                        })
                        .await?;
                    Ok(())
                }
                .await;
                if let Err(error) = cleanup {
                    cleanup_errors.push(error);
                }
            }
            let current = inventory().await?;
            for row in rows(&current, "networks")? {
                let Some(name) = row["Name"]
                    .as_str()
                    .filter(|n| network_names.iter().any(|v| v.as_str() == *n))
                else {
                    continue;
                };
                let cleanup: Result<(), String> = async {
                    let id = full_id(row["ID"].as_str().ok_or("Missing network ID")?)?;
                    let value = proof.inspect(Resource::Network, &id).await?;
                    fixture_network(&value, name, &subnet)?;
                    require(
                        value["Containers"]
                            .as_object()
                            .is_some_and(|v| v.is_empty()),
                        "Network has unexpected attachments; retain it",
                    )?;
                    proof
                        .pass(Request::Remove {
                            resource: Resource::Network,
                            target: id,
                        })
                        .await?;
                    Ok(())
                }
                .await;
                if let Err(error) = cleanup {
                    cleanup_errors.push(error);
                }
            }
        }
        Err(error) => cleanup_errors.push(error),
    }
    let identity_after = engine::check(&target_engine).await?;
    proof.events.push(json!({"result":result.as_ref().err(),"cleanupErrors":cleanup_errors,"daemonAfter":identity_after["ID"],"success":result.is_ok()&&cleanup_errors.is_empty()}));
    proof.save()?;
    result?;
    require(
        cleanup_errors.is_empty(),
        "Fixture cleanup incomplete; inspect retained evidence",
    )
}
