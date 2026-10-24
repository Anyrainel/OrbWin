use super::*;
use crate::operations::{self, ContainerAction, Operations, Request, Resource, RunOptions};

async fn operation(state: &Operations, id: &str, request: Request) -> Result<String, String> {
    let output = Arc::new(Mutex::new(String::new()));
    let received = output.clone();
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
    let outcome = operations::run(state, id.into(), request, channel).await?;
    let output = output.lock().unwrap().clone();
    if !outcome.success {
        return Err(format!("{}\n{output}", outcome.message));
    }
    Ok(output)
}
async fn until(mut ready: impl FnMut() -> bool) -> Result<(), String> {
    tokio::time::timeout(Duration::from_secs(25), async {
        while !ready() {
            tokio::time::sleep(Duration::from_millis(50)).await;
        }
    })
    .await
    .map_err(|_| "Terminal condition timed out".into())
}
// The SDK attaches directly to the Linux TTY. Input remains ordinary UTF-8/VT.
fn keys(text: &str) -> Vec<u8> {
    text.as_bytes().to_vec()
}
fn channel(
    state: &Arc<Sessions>,
    id: &'static str,
    output: Arc<Mutex<String>>,
    exit: Arc<Mutex<Option<Option<u32>>>>,
    ack: bool,
) -> Channel<Event> {
    let weak = Arc::downgrade(state);
    let runtime = tokio::runtime::Handle::current();
    let answered = AtomicBool::new(false);
    Channel::new(move |body| {
        let event: serde_json::Value = body.deserialize()?;
        if event["kind"] == "data" {
            let bytes: Vec<u8> = serde_json::from_value(event["bytes"].clone())?;
            output
                .lock()
                .unwrap()
                .push_str(&String::from_utf8_lossy(&bytes));
            // ConPTY asks its terminal emulator for the initial cursor position.
            // xterm answers this in production; emulate that response here.
            if output.lock().unwrap().contains("\x1b[6n") && !answered.swap(true, Ordering::AcqRel)
            {
                if let Some(state) = weak.upgrade() {
                    runtime.spawn(async move {
                        let _ = input(&state, id, b"\x1b[1;1R".to_vec()).await;
                    });
                }
            }
            if ack {
                if let Some(state) = weak.upgrade() {
                    let _ = acknowledge(&state, id, bytes.len());
                }
            }
        } else if event["kind"] == "exit" {
            *exit.lock().unwrap() = Some(event["code"].as_u64().map(|c| c as u32));
        }
        Ok(())
    })
}

#[tokio::test]
#[ignore = "real SDK interactive exec/backpressure proof on the owned engine"]
async fn owned_interactive_terminal() {
    assert_eq!(std::env::var("ORBWIN_QUALIFY_OWNED").as_deref(), Ok("1"));
    // This opt-in test runs alone in its own process. Deliberately wrong
    // inherited selectors must not redirect either adapter or SDK helper.
    std::env::set_var("DOCKER_HOST", "tcp://127.0.0.1:1");
    std::env::set_var("DOCKER_CONTEXT", "orbwin-intentionally-absent-context");
    std::env::set_var("DOCKER_TLS_VERIFY", "1");
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_millis();
    let name = format!("orbwin-terminal-{stamp}");
    let operations = Operations::default();
    let container = operation(&operations, "00000000-0000-4000-8000-000000000001", Request::Run { image: "alpine:3.21@sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30".into(), name, command: vec!["sleep".into(), "300".into()], options: RunOptions::default() }).await.unwrap().trim().to_string();
    let sessions = Arc::new(Sessions::default());
    let output = Arc::new(Mutex::new(String::new()));
    let exit = Arc::new(Mutex::new(None));
    let first = "00000000-0000-4000-8000-000000000101";
    let result: Result<(), String> = async {
        open(
            &sessions,
            first.into(),
            container.clone(),
            Shell::Sh,
            80,
            24,
            channel(&sessions, first, output.clone(), exit.clone(), true),
        )
        .await?;
        until(|| output.lock().unwrap().contains("/ #")).await?;
        input(
            &sessions,
            first,
            keys("printf '%s%s\\n' orbwin- tty-proof; stty size\r"),
        )
        .await?;
        until(|| {
            let output = output.lock().unwrap();
            output.contains("orbwin-tty-proof") && output.contains("24 80")
        })
        .await?;
        resize(&sessions, first, 111, 37).await?;
        input(&sessions, first, keys("stty size\r")).await?;
        until(|| output.lock().unwrap().contains("37 111")).await?;
        input(&sessions, first, keys("sleep 30\r")).await?;
        tokio::time::sleep(Duration::from_millis(300)).await;
        input(&sessions, first, keys("\x03")).await?;
        input(
            &sessions,
            first,
            keys("printf '%s%s\\n' recovered- after-interrupt\r"),
        )
        .await?;
        until(|| output.lock().unwrap().contains("recovered-after-interrupt")).await?;
        input(&sessions, first, keys("exit 7\r")).await?;
        until(|| exit.lock().unwrap().is_some()).await?;
        if *exit.lock().unwrap() != Some(Some(7)) {
            return Err(format!(
                "Lost shell exit status: {:?}",
                *exit.lock().unwrap()
            ));
        }
        let second = "00000000-0000-4000-8000-000000000102";
        let stalled_output = Arc::new(Mutex::new(String::new()));
        let stalled_exit = Arc::new(Mutex::new(None));
        open(
            &sessions,
            second.into(),
            container.clone(),
            Shell::Sh,
            80,
            24,
            channel(
                &sessions,
                second,
                stalled_output.clone(),
                stalled_exit.clone(),
                false,
            ),
        )
        .await?;
        until(|| stalled_output.lock().unwrap().contains("/ #")).await?;
        input(&sessions, second, keys("yes x\r")).await?;
        until(|| stalled_exit.lock().unwrap().is_some()).await?;
        if stalled_output.lock().unwrap().len() > 66000 {
            return Err("Unacknowledged output exceeded its bound".into());
        }
        let target = engine::target()?;
        let running = engine::command(
            &target,
            &["inspect", "--format={{.State.Running}}", &container],
        )
        .await?;
        if running.trim() != "true" {
            return Err("Terminal disconnect stopped the container".into());
        }
        Ok(())
    }
    .await;
    for id in [first, "00000000-0000-4000-8000-000000000102"] {
        let _ = close(&sessions, id);
    }
    let stopped = operation(
        &operations,
        "00000000-0000-4000-8000-000000000002",
        Request::Container {
            action: ContainerAction::Stop,
            target: container.clone(),
        },
    )
    .await;
    let removed = operation(
        &operations,
        "00000000-0000-4000-8000-000000000003",
        Request::Remove {
            resource: Resource::Container,
            target: container,
        },
    )
    .await;
    let evidence = std::path::PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join(format!("../artifacts/qualification/terminal-{stamp}.json"));
    std::fs::write(evidence, serde_json::to_vec_pretty(&serde_json::json!({"result": result, "stop": stopped, "remove": removed, "output": *output.lock().unwrap(), "exit": *exit.lock().unwrap()})).unwrap()).unwrap();
    assert!(
        result.is_ok() && stopped.is_ok() && removed.is_ok(),
        "result: {result:?}; stop: {stopped:?}; remove: {removed:?}; output: {}",
        output.lock().unwrap()
    );
}
