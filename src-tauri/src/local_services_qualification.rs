//! Ignored native product-entry qualification; never part of ordinary dispatch.
//! Root must separately admit an exclusive/pristine current package before run.
//! The opt-in is an accident guard, not a grant. Go owns native fresh admission.
use super::*;
use lifetime::{ExpectedServer, ReplyError, StartupMode};

pub(super) fn check_reply(code: i32, response: &Value) -> Result<(), String> {
    let error = match response.get("error") {
        None => ReplyError::Absent,
        Some(Value::String(value)) => ReplyError::Text(value),
        Some(_) => ReplyError::Malformed,
    };
    if !lifetime::successful_reply(code, response.get("version").and_then(Value::as_u64), error) {
        return Err("Restricted SDK reply is unsuccessful or malformed; do not resend.".into());
    }
    Ok(())
}
fn original_child(connection: &ConnectedServices) -> Result<&std::process::Child, String> {
    connection
        .started_child
        .as_ref()
        .and_then(|launch| launch.child.as_ref())
        .map(|owned| &owned.child)
        .ok_or_else(|| "No original created SDK child is retained.".into())
}
fn generation(state: &Value) -> Result<ExpectedServer, String> {
    let identity = state
        .get("selfIdentity")
        .ok_or("SDK ACK identity is missing.")?;
    let pid = identity
        .get("pid")
        .and_then(Value::as_u64)
        .and_then(|value| u32::try_from(value).ok())
        .ok_or("SDK ACK PID is invalid.")?;
    let creation = identity
        .get("creationTime")
        .and_then(Value::as_str)
        .ok_or("SDK ACK generation is invalid.")?;
    ExpectedServer::new(pid, creation).ok_or_else(|| "SDK ACK generation is noncanonical.".into())
}
fn actual_generation(connection: &ConnectedServices) -> Result<ExpectedServer, String> {
    let child = original_child(connection)?;
    // This is the SAME retained actual launch, not an attached DTO or PID lookup.
    crate::sdk_identity::verify_snapshot(&connection.binary, &connection.current, Some(child))?;
    let expected = generation(&connection.current["state"])?;
    if child.id() != expected.pid {
        return Err("SDK ACK does not identify the retained original child.".into());
    }
    Ok(expected)
}
fn expected_json(expected: &ExpectedServer) -> Value {
    json!({"pid": expected.pid, "creationTime": expected.creation_time})
}
fn bound_state<'a>(
    connection: &ConnectedServices,
    expected: &ExpectedServer,
    code: i32,
    response: &'a Value,
) -> Result<&'a Value, String> {
    check_reply(code, response)?;
    let state = response
        .get("state")
        .filter(|value| value.is_object())
        .ok_or("Restricted SDK state is missing.")?;
    if state.get("selfIdentity") != connection.current["state"].get("selfIdentity")
        || generation(state)? != *expected
        || original_child(connection)?.id() != expected.pid
    {
        return Err("Restricted SDK reply identifies another original generation.".into());
    }
    for key in [
        "daemonId",
        "installation",
        "binarySha256",
        "toolsSha256",
        "wslTrustSha256",
    ] {
        if state.get(key).is_none() || state[key] != connection.current["state"][key] {
            return Err("Restricted SDK reply changed its target or component binding.".into());
        }
    }
    Ok(state)
}
fn live_state<'a>(
    connection: &ConnectedServices,
    expected: &ExpectedServer,
    code: i32,
    response: &'a Value,
) -> Result<&'a Value, String> {
    let state = bound_state(connection, expected, code, response)?;
    crate::sdk_identity::verify_snapshot(
        &connection.binary,
        response,
        Some(original_child(connection)?),
    )?;
    Ok(state)
}
fn disabled_state<'a>(
    state: &Value,
    response: &'a Value,
    revision: u64,
    settings: &Value,
) -> Result<&'a str, String> {
    let fingerprint = response
        .get("restrictedStateSha256")
        .and_then(Value::as_str);
    if !lifetime::disabled_reply_matches(
        state.get("revision").and_then(Value::as_u64),
        revision,
        state
            .get("settings")
            .and_then(|settings| settings.get("enabled"))
            .and_then(Value::as_bool),
        state.get("settings") == Some(settings),
        fingerprint,
    ) {
        return Err(
            "Disabled SDK settings/revision/fingerprint differ; no replay is permitted.".into(),
        );
    }
    fingerprint.ok_or_else(|| "Disabled SDK fingerprint is missing.".into())
}
async fn stop_original(
    connection: ConnectedServices,
    expected: &ExpectedServer,
    revision: u64,
    settings: &Value,
) -> Result<(), String> {
    // Authenticate the still-live SAME original immediately before stop. The Go
    // client and server compare expectedServer before request write/dispatch.
    if actual_generation(&connection)? != *expected {
        return Err("Pre-stop original generation changed.".into());
    }
    let request = json!({"version": 1, "operation": "stop-process", "expectedServer": expected_json(expected)});
    let (code, response) = rpc(
        &connection.binary,
        &connection.home,
        &connection.installation,
        &request,
    )
    .await?;
    // The server may already be terminal when its RPC client returns. Do not
    // require a still-live Wait0 here: bind this reply to the previously verified
    // original and then demand its original sole-waiter terminal below. A reply
    // alone never provides retirement or permission to restart.
    let state = bound_state(&connection, expected, code, &response)?;
    if state.get("revision").and_then(Value::as_u64) != Some(revision)
        || state.get("settings") != Some(settings)
    {
        return Err("Stop reply changed the disabled SDK state; outcome is unknown.".into());
    }
    connection
        .await_original_terminal(Duration::from_secs(60))
        .await?;
    Ok(())
}

// This selector itself is native and MUST NOT be used by the std-only harness.
#[tokio::test]
#[ignore = "Root-only: reviewed exclusive/pristine current package; native runs are paused"]
async fn owned_current_product_disabled_roundtrip() {
    assert_eq!(
        std::env::var("ORBWIN_QUALIFY_CURRENT_SDK_PRODUCT_ENTRY").as_deref(),
        Ok("1"),
        "Explicit qualification opt-in required before any target/cache/start effect."
    );
    // Catch only ordinary Rust task unwind at this existing ownership boundary.
    // A dropped launch still goes to its sole waiter; RPC workers keep permits.
    // This is not a universal raw-output/panic or host-termination guarantee.
    let failed = match tokio::spawn(disabled_roundtrip()).await {
        Ok(Ok(())) => return,
        Ok(Err(error)) => error,
        Err(error) => format!("SDK qualification task did not return normally: {error}"),
    };
    // A single-selector libtest PE must not exit merely because its async case
    // returned Err while a background original owner still needs this host.
    let _ = std::io::Write::write_all(&mut std::io::stderr(), b"SDK qualification failed; retaining this test process until its existing original server waiter and RPC workers settle. No retry or restart will occur.\n");
    let _settled_clients = retain_failure_host().await;
    // Keep all16 actual admission permits through the FAILED panic boundary.
    panic!("SDK qualification failed after existing owner settlement: {failed}");
}
/// Failure-only same-PE barrier. These are the ACTUAL source admission owners,
/// not process-existence guesses or permission to create another service.
async fn retain_failure_host() -> tokio::sync::SemaphorePermit<'static> {
    loop {
        // Existing workers release these permits only after their own fixed
        // normal-return settlement. A pending worker keeps this await pending.
        let clients = match RPC_SLOTS.acquire_many(16).await {
            Ok(clients) => clients,
            Err(_) => {
                // No code closes this static semaphore. A contradictory result
                // cannot certify settlement; retain the host without new work.
                return std::future::pending().await;
            }
        };
        if !START_PENDING.load(std::sync::atomic::Ordering::Acquire) {
            // Positive original wait or a no-child reservation released the
            // existing gate. No family/raw-close property follows from it.
            return clients;
        }
        drop(clients);
        // Only an in-process atomic is observed; no native liveness/PID polling.
        // Unknown waiter settlement leaves START_PENDING sticky and keeps PE up.
        tokio::time::sleep(Duration::from_millis(100)).await;
    }
}

async fn disabled_roundtrip() -> Result<(), String> {
    let first = connect_or_start(StartupMode::RequireNew)
        .await?
        .require_actual_created()?;
    let first_generation = actual_generation(&first)?;
    let initial = &first.current["state"];
    if initial.get("revision").and_then(Value::as_u64) != Some(0)
        || initial["settings"]["enabled"] != false
    {
        return Err("Fresh restricted SDK did not return its initial disabled state.".into());
    }
    let mut settings = initial
        .get("settings")
        .filter(|value| value.is_object())
        .cloned()
        .ok_or("Initial SDK settings are unavailable.")?;
    // Fixed disabled selectors refer to no discovered actor and enable no LAN or
    // authority. 360 real preferences exercise the actual stdin worker >32 KiB.
    settings["preferences"] = Value::Array(
        (1..=360u32)
            .map(|index| {
                json!({
                    "selector": {"containerId": format!("{index:064x}")}, "disabled": true,
                })
            })
            .collect(),
    );
    settings["enabled"] = json!(false);
    let configure = json!({"version": 1, "operation": "configure", "revision": 0,
        "settings": settings, "expectedServer": expected_json(&first_generation)});
    let stdin_bytes = serde_json::to_vec(&configure)
        .map_err(|error| error.to_string())?
        .len();
    if stdin_bytes <= 32768 || !lifetime::request_size_allowed(stdin_bytes) {
        return Err(
            "Fixed disabled Configure does not exercise the admitted large stdin path.".into(),
        );
    }
    let (code, response) = rpc(&first.binary, &first.home, &first.installation, &configure).await?;
    let state = live_state(&first, &first_generation, code, &response)?;
    let fingerprint = disabled_state(state, &response, 1, &settings)?.to_owned();
    stop_original(first, &first_generation, 1, &settings).await?;

    // Only the positive original waiter above can reach a second start. Actual
    // Go exclusive lock + exact raw inventory fingerprint authorizes admission;
    // this string never creates a native owner or proves historical absence.
    let second = connect_or_start(StartupMode::RequireDisabled(fingerprint.clone()))
        .await?
        .require_actual_created()?;
    let second_generation = actual_generation(&second)?;
    if second_generation == first_generation {
        return Err("Restart did not supply a distinct original SDK generation.".into());
    }
    let persisted = disabled_state(&second.current["state"], &second.current, 1, &settings)?;
    if persisted != fingerprint {
        return Err("Restarted disabled inventory fingerprint changed.".into());
    }
    stop_original(second, &second_generation, 1, &settings).await?;
    println!(
        "SDK_PRODUCT_ENTRY_QUALIFICATION {}",
        json!({"schema": 1, "preferences": 360,
        "stdinBytes": stdin_bytes, "revision": 1, "restrictedStateSha256": fingerprint,
        "first": expected_json(&first_generation), "second": expected_json(&second_generation),
        "originalServerTerminals": 2})
    );
    Ok(())
}
