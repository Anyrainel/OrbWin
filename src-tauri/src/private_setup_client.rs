//! Fixed persistent Go role. Events are commit observation DATA, never native
//! process/namespace authority. Drop/timeout/cancel must not terminate the service.
use serde::Deserialize;
#[cfg(test)]
use std::io::Read;
use std::{ffi::OsString, io::Write, sync::mpsc, time::Instant};

const REQUEST: &[u8] = b"{\"schema\":2,\"operation\":\"setup\"}";
const CANCEL: &[u8] = b"{\"schema\":2,\"operation\":\"cancel\"}";
const EVENT_LIMIT: usize = 256;
#[cfg(test)]
const ARGV: [&str; 2] = ["private-runtime-operation", "setup"];

#[cfg(windows)]
#[path = "private_setup_native_io.rs"]
mod native_io;
#[cfg(windows)]
pub(crate) struct SetupPackage {
    executable: crate::runtime_cache::CachedExecutable,
}
#[cfg(windows)]
impl SetupPackage {
    pub(crate) fn into_cache(self) -> crate::runtime_cache::CachedExecutable {
        self.executable
    }
}
fn frame(body: &[u8]) -> Vec<u8> {
    let mut bytes = (body.len() as u32).to_le_bytes().to_vec();
    bytes.extend_from_slice(body);
    bytes
}
pub(crate) fn environment(
    input: impl IntoIterator<Item = (OsString, OsString)>,
) -> Result<Vec<(OsString, OsString)>, String> {
    let mut output = Vec::new();
    let mut seen = std::collections::BTreeSet::new();
    let mut units = 0usize;
    for (key, value) in input {
        let text = key
            .to_str()
            .ok_or("Setup environment key is not Unicode.")?;
        let upper = text.to_ascii_uppercase();
        // Hidden Windows per-drive working directories are not selectors for
        // this role; do not forward them into the independently selected child.
        if text.len() == 3
            && text.starts_with('=')
            && text.ends_with(':')
            && text.as_bytes()[1].is_ascii_alphabetic()
        {
            continue;
        }
        if text.is_empty()
            || text.contains(['=', '\0'])
            || value.to_string_lossy().contains('\0')
            || !seen.insert(upper.clone())
        {
            return Err("Malformed setup environment.".into());
        }
        if matches!(
            upper.as_str(),
            "HOME"
                | "USERPROFILE"
                | "RDD_INSTANCE"
                | "RDD_LOG_DIR"
                | "RDD_DEVELOPER_MODE"
                | "RDD_KEEP_LOGS"
                | "LIMA_HOME"
                | "KUBECONFIG"
                | "ORBWIN_DOCKER_MANAGED_PLUGINS"
        ) || upper.starts_with("DOCKER_")
            || upper.starts_with("BUILDX_")
            || upper.starts_with("BUILDKIT_")
            || upper.starts_with("ORBWIN_BUILD")
        {
            continue;
        }
        units = units
            .checked_add(key.len() + value.len())
            .ok_or("Setup environment overflow.")?;
        if output.len() >= 512 || units > 1 << 19 {
            return Err("Setup environment exceeds bounds.".into());
        }
        output.push((key, value));
    }
    output.push(("RDD_INSTANCE".into(), "orbwin".into()));
    Ok(output)
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Event {
    schema: u32,
    state: String,
    #[serde(rename = "setupSuccess")]
    setup_success: bool,
    #[serde(rename = "runtimePublication")]
    runtime_publication: bool,
    reason: Option<String>,
    nonce: Option<String>,
    stage: Option<String>,
}
#[derive(Debug, PartialEq, Eq, Clone, Copy)]
pub(crate) enum IncompleteStage {
    ServiceReadiness,
    RuntimeObservation,
    RuntimePublication,
}
impl IncompleteStage {
    fn from_wire(value: &str) -> Option<Self> {
        match value {
            "service-readiness" => Some(Self::ServiceReadiness),
            "runtime-observation" => Some(Self::RuntimeObservation),
            "runtime-publication" => Some(Self::RuntimePublication),
            _ => None,
        }
    }
    pub(crate) fn message(self) -> &'static str {
        match self {
            Self::ServiceReadiness => "Engine startup could not be confirmed. Backend work may continue. Check status and open diagnostic logs before retrying.",
            Self::RuntimeObservation => "Engine setup could not be verified. Backend work may continue. Check status and open diagnostic logs before retrying.",
            Self::RuntimePublication => "Engine setup completion could not be confirmed. Backend work may continue. Check status and open diagnostic logs before retrying.",
        }
    }
}
#[derive(Debug, PartialEq, Eq, Clone)]
pub(crate) enum ObservationData {
    StartupAdmitted,
    DecisionRequired { nonce: String },
    SetupPaused,
    SetupIncomplete { stage: IncompleteStage },
    RuntimePublished,
}
#[derive(Default)]
struct Protocol {
    admitted: bool,
    published: bool,
    failed: bool,
    decision_seen: bool,
    pending_nonce: Option<String>,
    continued: bool,
    paused: bool,
    incomplete: Option<IncompleteStage>,
}
fn decision_nonce(value: &str) -> bool {
    value.len() == 32
        && value
            .bytes()
            .all(|c| c.is_ascii_digit() || (b'a'..=b'f').contains(&c))
}
impl Protocol {
    fn event(&mut self, line: &[u8]) -> Result<ObservationData, String> {
        let result = (|| {
            if self.failed
                || self.published
                || self.paused
                || self.incomplete.is_some()
                || line.is_empty()
                || line.len() > EVENT_LIMIT
                || line.contains(&b'\n')
                || line.contains(&b'\r')
            {
                return Err("Invalid setup event framing.");
            }
            let event: Event =
                serde_json::from_slice(line).map_err(|_| "Invalid setup event schema.")?;
            if event.schema != 2 {
                return Err("Unknown setup schema.");
            }
            match event.state.as_str() {
                "startup-admitted"
                    if !self.admitted
                        && !event.setup_success
                        && !event.runtime_publication
                        && event.reason.is_none()
                        && event.nonce.is_none()
                        && event.stage.is_none() =>
                {
                    self.admitted = true;
                    Ok(ObservationData::StartupAdmitted)
                }
                "decision-required"
                    if self.admitted
                        && !self.decision_seen
                        && !event.setup_success
                        && !event.runtime_publication
                        && event.reason.as_deref() == Some("prior-app-absent")
                        && event.stage.is_none() =>
                {
                    let nonce = event
                        .nonce
                        .filter(|n| decision_nonce(n))
                        .ok_or("Invalid setup decision.")?;
                    self.decision_seen = true;
                    self.pending_nonce = Some(nonce.clone());
                    Ok(ObservationData::DecisionRequired { nonce })
                }
                "setup-paused"
                    if self.admitted
                        && !event.setup_success
                        && !event.runtime_publication
                        && event.reason.is_none()
                        && event.nonce.is_none()
                        && event.stage.is_none() =>
                {
                    self.paused = true;
                    self.pending_nonce = None;
                    Ok(ObservationData::SetupPaused)
                }
                "setup-incomplete"
                    if self.admitted
                        && (!self.decision_seen || self.continued)
                        && !event.setup_success
                        && !event.runtime_publication
                        && event.reason.is_none()
                        && event.nonce.is_none() =>
                {
                    let stage = event
                        .stage
                        .as_deref()
                        .and_then(IncompleteStage::from_wire)
                        .ok_or("Invalid incomplete setup stage.")?;
                    self.incomplete = Some(stage);
                    self.pending_nonce = None;
                    Ok(ObservationData::SetupIncomplete { stage })
                }
                "runtime-published"
                    if self.admitted
                        && (!self.decision_seen || self.continued)
                        && event.setup_success
                        && event.runtime_publication
                        && event.reason.is_none()
                        && event.nonce.is_none()
                        && event.stage.is_none() =>
                {
                    self.published = true;
                    Ok(ObservationData::RuntimePublished)
                }
                _ => Err("Unexpected setup event transition."),
            }
        })();
        if result.is_err() {
            self.failed = true
        }
        result.map_err(str::to_owned)
    }
}

#[cfg(test)]
fn read_event(reader: &mut impl Read) -> Result<Option<Vec<u8>>, String> {
    let mut line = Vec::new();
    let mut byte = [0];
    loop {
        match reader.read(&mut byte) {
            Ok(0) if line.is_empty() => return Ok(None),
            Ok(0) => return Err("Truncated setup event.".into()),
            Ok(_) => {
                if byte[0] == b'\n' {
                    return Ok(Some(line));
                }
                if line.len() == EVENT_LIMIT {
                    return Err("Oversized setup event.".into());
                }
                line.push(byte[0]);
            }
            Err(e) if e.kind() == std::io::ErrorKind::Interrupted => continue,
            Err(_) => return Err("Setup observation read failed.".into()),
        }
    }
}

pub(crate) struct SetupObserver {
    #[cfg(windows)]
    input: Option<native_io::SetupInput>,
    #[cfg(not(windows))]
    input: Option<std::io::Sink>,
    #[cfg(test)]
    pause_test_input: Option<Vec<u8>>,
    events: mpsc::Receiver<Result<Vec<u8>, String>>,
    protocol: Protocol,
    cancel_sent: bool,
}
impl SetupObserver {
    pub(crate) fn continue_original_setup(&mut self, nonce: &str) -> Result<(), String> {
        if self.cancel_sent
            || self.protocol.failed
            || !decision_nonce(nonce)
            || self.protocol.pending_nonce.as_deref() != Some(nonce)
            || self.protocol.continued
        {
            return Err("This setup decision is no longer available.".into());
        }
        // Consume before write: failure cannot safely retry a command frame.
        self.protocol.pending_nonce = None;
        self.protocol.continued = true;
        let body = format!(
            "{{\"schema\":2,\"operation\":\"continue-original-setup\",\"nonce\":\"{nonce}\"}}"
        );
        let result = self
            .input
            .as_mut()
            .ok_or("Setup input is unavailable.")
            .and_then(|input| {
                input
                    .write_all(&frame(body.as_bytes()))
                    .and_then(|_| input.flush())
                    .map_err(|_| "Setup decision delivery is unknown.")
            });
        if let Err(e) = result {
            self.protocol.failed = true;
            self.input.take();
            return Err(e.into());
        }
        Ok(())
    }

    /// Request Go's pending-decision settlement, then keep polling for the
    /// actual positive setup-paused DATA. No kill, abandoned birth, or success.
    pub(crate) fn request_pause(&mut self) -> Result<(), String> {
        if self.cancel_sent
            || self.protocol.failed
            || self.protocol.pending_nonce.is_none()
            || self.protocol.continued
        {
            return Err("Setup is no longer waiting for this decision.".into());
        }
        self.cancel_sent = true;
        let result = self.write_pause_frame();
        if let Err(e) = result {
            self.protocol.failed = true;
            self.input.take();
            return Err(e.into());
        }
        Ok(())
    }

    fn write_pause_frame(&mut self) -> Result<(), &'static str> {
        // A test-only DATA sink permits exercising the real pause/poll methods
        // without creating a native child or pipe. Production retains the original native profile writer.
        #[cfg(test)]
        if let Some(input) = self.pause_test_input.as_mut() {
            input.extend_from_slice(&frame(CANCEL));
            return Ok(());
        }
        self.input
            .as_mut()
            .ok_or("Setup input is unavailable.")
            .and_then(|input| {
                input
                    .write_all(&frame(CANCEL))
                    .and_then(|_| input.flush())
                    .map_err(|_| "Setup pause delivery is unknown.")
            })
    }
    /// `log` is an actual owned regular private file, not a path or a pipe.
    /// This does not create it or the selected guest home before the Go fence.
    pub(crate) async fn start(
        executable: crate::runtime_cache::CachedExecutable,
        log: std::fs::File,
    ) -> Result<Self, String> {
        #[cfg(windows)]
        {
            let child = crate::native_client::PersistentSetupClient::spawn(
                SetupPackage { executable },
                log,
            )
            .await?;
            let (input, events) = native_io::attach(child).await?;
            // Existing caller cannot lose the original input during a blocking
            // initial write: the independent IO task already owns child/cache.
            let input = tokio::task::spawn_blocking(move || {
                let mut input = input;
                input
                    .write_all(&frame(REQUEST))
                    .and_then(|_| input.flush())
                    .map_err(|_| "Setup request delivery unknown.")?;
                Ok::<_, String>(input)
            })
            .await
            .map_err(|_| "Setup request worker unknown.")??;
            Ok(Self {
                input: Some(input),
                #[cfg(test)]
                pause_test_input: None,
                events,
                protocol: Protocol::default(),
                cancel_sent: false,
            })
        }
        #[cfg(not(windows))]
        {
            let _ = (executable, log);
            Err("Setup requires Windows.".into())
        }
    }
    /// Returns only stdout DATA from the verified fixed role. Timeout, EOF,
    /// process exit and cancellation never produce RuntimePublished.
    pub(crate) fn poll(&mut self, deadline: Instant) -> Result<Option<ObservationData>, String> {
        if self.protocol.failed {
            return Err("Setup observation is unknown.".into());
        }
        let result = poll_data(&mut self.protocol, &self.events, deadline);
        if result.is_err() {
            self.input.take();
        }
        result
    }
    pub(crate) fn observe(&mut self, deadline: Instant) -> Result<ObservationData, String> {
        if self.protocol.failed {
            return Err("Setup observation is unknown.".into());
        }
        let result = observe_data(&mut self.protocol, &self.events, deadline);
        if result.is_err() {
            self.input.take();
        }
        result
    }
    pub(crate) fn cancel(&mut self) -> Result<(), String> {
        if self.cancel_sent {
            return Err(
                "Setup cancellation was already requested; runtime state is unknown.".into(),
            );
        }
        self.cancel_sent = true;
        self.protocol.failed = true;
        if let Some(mut input) = self.input.take() {
            input
                .write_all(&frame(CANCEL))
                .and_then(|_| input.flush())
                .map_err(|_| "Setup cancellation delivery is unknown.".to_owned())
        } else {
            Ok(())
        }
    }
}
fn poll_data(
    protocol: &mut Protocol,
    events: &mpsc::Receiver<Result<Vec<u8>, String>>,
    deadline: Instant,
) -> Result<Option<ObservationData>, String> {
    if protocol.failed {
        return Err("Setup observation is unknown.".into());
    }
    let result = if deadline <= Instant::now() {
        Err("Setup observation timed out; runtime state is unknown.".into())
    } else {
        match events.try_recv() {
            Ok(Ok(line)) => protocol.event(&line).map(Some),
            Err(mpsc::TryRecvError::Empty) => Ok(None),
            _ => Err("Setup observation was lost; runtime state is unknown.".into()),
        }
    };
    if result.is_err() {
        protocol.failed = true
    }
    result
}
fn observe_data(
    protocol: &mut Protocol,
    events: &mpsc::Receiver<Result<Vec<u8>, String>>,
    deadline: Instant,
) -> Result<ObservationData, String> {
    if protocol.failed {
        return Err("Setup observation is unknown.".into());
    }
    let result = match deadline
        .checked_duration_since(Instant::now())
        .filter(|d| !d.is_zero())
    {
        Some(remaining) => match events.recv_timeout(remaining) {
            Ok(Ok(line)) => protocol.event(&line),
            _ => Err("Setup observation was lost; runtime state is unknown.".into()),
        },
        None => Err("Setup observation timed out; runtime state is unknown.".into()),
    };
    if result.is_err() {
        protocol.failed = true
    }
    result
}
// std::process::Child does not terminate on Drop. Closing stdin expresses only
// the Go protocol's pre-admission cancellation / post-admission observer detach.
impl Drop for SetupObserver {
    fn drop(&mut self) {
        self.input.take();
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn nonblocking_pending_keeps_ordered_publication_eligible() {
        let (sender, receiver) = mpsc::sync_channel(4);
        let mut p = Protocol::default();
        let deadline = Instant::now() + std::time::Duration::from_secs(10);
        for _ in 0..3 {
            assert_eq!(poll_data(&mut p, &receiver, deadline).unwrap(), None);
            assert!(!p.failed)
        }
        sender.send(Ok(ADMITTED.to_vec())).unwrap();
        assert_eq!(
            poll_data(&mut p, &receiver, deadline).unwrap(),
            Some(ObservationData::StartupAdmitted)
        );
        assert_eq!(poll_data(&mut p, &receiver, deadline).unwrap(), None);
        sender.send(Ok(PUBLISHED.to_vec())).unwrap();
        assert_eq!(
            poll_data(&mut p, &receiver, deadline).unwrap(),
            Some(ObservationData::RuntimePublished)
        );
    }
    #[test]
    fn nonblocking_malformed_eof_timeout_are_sticky_unknown() {
        for fault in ["malformed", "eof", "timeout", "closed", "out of order"] {
            let (sender, receiver) = mpsc::sync_channel(4);
            let mut p = Protocol::default();
            let mut deadline = Instant::now() + std::time::Duration::from_secs(10);
            match fault {
                "malformed" => sender.send(Ok(b"{}".to_vec())).unwrap(),
                "eof" => sender.send(Err("EOF".into())).unwrap(),
                "timeout" => deadline = Instant::now(),
                "out of order" => sender.send(Ok(PUBLISHED.to_vec())).unwrap(),
                _ => {
                    drop(sender);
                    assert!(poll_data(&mut p, &receiver, deadline).is_err());
                    assert!(p.failed);
                    continue;
                }
            }
            assert!(poll_data(&mut p, &receiver, deadline).is_err());
            sender.send(Ok(ADMITTED.to_vec())).unwrap();
            sender.send(Ok(PUBLISHED.to_vec())).unwrap();
            assert!(poll_data(
                &mut p,
                &receiver,
                Instant::now() + std::time::Duration::from_secs(10)
            )
            .is_err());
        }
    }
    const ADMITTED:&[u8]=br#"{"schema":2,"state":"startup-admitted","setupSuccess":false,"runtimePublication":false}"#;
    const PUBLISHED:&[u8]=br#"{"schema":2,"state":"runtime-published","setupSuccess":true,"runtimePublication":true}"#;
    #[test]
    fn wire_frames_match_fixed_go_contract() {
        assert_eq!(
            frame(REQUEST),
            b"\x20\0\0\0{\"schema\":2,\"operation\":\"setup\"}"
        );
        assert_eq!(
            frame(CANCEL),
            b"\x21\0\0\0{\"schema\":2,\"operation\":\"cancel\"}"
        );
    }
    #[test]
    fn only_ordered_actual_publication_event_completes_data() {
        let mut p = Protocol::default();
        assert_eq!(p.event(ADMITTED).unwrap(), ObservationData::StartupAdmitted);
        assert_eq!(
            p.event(PUBLISHED).unwrap(),
            ObservationData::RuntimePublished
        );
        assert!(p.event(PUBLISHED).is_err());
        assert!(Protocol::default().event(PUBLISHED).is_err());
        let mut p = Protocol::default();
        p.event(ADMITTED).unwrap();
        assert!(p.event(ADMITTED).is_err());
        assert!(p.event(PUBLISHED).is_err());
    }
    #[test]
    fn strict_events_reject_unknown_duplicate_false_and_unbounded_success() {
        for line in [br#"{"schema":2,"schema":2,"state":"startup-admitted","setupSuccess":false,"runtimePublication":false}"#.as_slice(),br#"{"schema":2,"state":"startup-admitted","setupSuccess":false,"runtimePublication":false,"extra":true}"#,br#"{"schema":2,"state":"runtime-published","setupSuccess":true,"runtimePublication":false}"#,br#"{"schema":1,"state":"startup-admitted","setupSuccess":false,"runtimePublication":false}"#,b"",b"null",b"\xff",&vec![b' ';257]]{let mut p=Protocol::default();assert!(p.event(line).is_err());assert!(p.event(ADMITTED).is_err());}
    }
    const DECISION:&[u8]=br#"{"schema":2,"state":"decision-required","setupSuccess":false,"runtimePublication":false,"reason":"prior-app-absent","nonce":"11111111111111111111111111111111"}"#;
    const PAUSED: &[u8] =
        br#"{"schema":2,"state":"setup-paused","setupSuccess":false,"runtimePublication":false}"#;
    #[test]
    fn decision_is_one_time_ordered_and_never_setup_success() {
        assert!(Protocol::default().event(DECISION).is_err());
        let mut p = Protocol::default();
        p.event(ADMITTED).unwrap();
        assert_eq!(
            p.event(DECISION).unwrap(),
            ObservationData::DecisionRequired {
                nonce: "11111111111111111111111111111111".into()
            }
        );
        assert!(p.event(PUBLISHED).is_err());
        let mut p = Protocol::default();
        p.event(ADMITTED).unwrap();
        p.event(DECISION).unwrap();
        assert_eq!(p.event(PAUSED).unwrap(), ObservationData::SetupPaused);
        assert!(p.event(PUBLISHED).is_err());
        let mut p = Protocol::default();
        p.event(ADMITTED).unwrap();
        p.event(DECISION).unwrap();
        assert!(p.event(DECISION).is_err());
        let mut p = Protocol::default();
        p.event(ADMITTED).unwrap();
        p.event(DECISION).unwrap();
        p.pending_nonce = None;
        p.continued = true;
        assert_eq!(
            p.event(PUBLISHED).unwrap(),
            ObservationData::RuntimePublished
        );
        let mut p = Protocol::default();
        p.event(ADMITTED).unwrap();
        p.event(DECISION).unwrap();
        p.continued = true;
        assert_eq!(p.event(PAUSED).unwrap(), ObservationData::SetupPaused);
        let mut p = Protocol::default();
        p.event(ADMITTED).unwrap();
        assert_eq!(p.event(PAUSED).unwrap(), ObservationData::SetupPaused);
        assert!(Protocol::default().event(PAUSED).is_err());
    }
    #[test]
    fn actual_pause_request_keeps_polling_until_confirmed_pause() {
        let (sender, receiver) = mpsc::sync_channel(4);
        let mut protocol = Protocol::default();
        protocol.event(ADMITTED).unwrap();
        protocol.event(DECISION).unwrap();
        let mut observer = SetupObserver {
            input: None,
            pause_test_input: Some(Vec::new()),
            events: receiver,
            protocol,
            cancel_sent: false,
        };
        let deadline = Instant::now() + std::time::Duration::from_secs(1);
        observer.request_pause().unwrap();
        assert_eq!(observer.pause_test_input.as_ref().unwrap(), &frame(CANCEL));
        assert!(observer.request_pause().is_err());
        assert_eq!(observer.poll(deadline).unwrap(), None);
        assert!(!observer.protocol.failed);
        sender.send(Ok(PAUSED.to_vec())).unwrap();
        assert_eq!(
            observer.poll(deadline).unwrap(),
            Some(ObservationData::SetupPaused)
        );
        assert!(!observer.protocol.published);
        sender.send(Ok(PUBLISHED.to_vec())).unwrap();
        assert!(observer.poll(deadline).is_err());
    }

    #[test]
    fn actual_pause_delivery_failure_and_terminal_cancel_stay_unknown() {
        for pause in [false, true] {
            let (sender, receiver) = mpsc::sync_channel(4);
            let mut protocol = Protocol::default();
            protocol.event(ADMITTED).unwrap();
            protocol.event(DECISION).unwrap();
            let mut observer = SetupObserver {
                input: None,
                pause_test_input: None,
                events: receiver,
                protocol,
                cancel_sent: false,
            };
            if pause {
                assert!(observer.request_pause().is_err());
            } else {
                observer.cancel().unwrap();
            }
            sender.send(Ok(PAUSED.to_vec())).unwrap();
            assert!(observer.protocol.failed);
            assert!(observer
                .poll(Instant::now() + std::time::Duration::from_secs(1))
                .is_err());
        }
    }

    #[test]
    fn decision_write_without_original_input_is_sticky_unknown() {
        let (_sender, receiver) = mpsc::sync_channel(4);
        let mut p = Protocol::default();
        p.event(ADMITTED).unwrap();
        p.event(DECISION).unwrap();
        let mut observer = SetupObserver {
            input: None,
            pause_test_input: None,
            events: receiver,
            protocol: p,
            cancel_sent: false,
        };
        assert!(observer
            .continue_original_setup("22222222222222222222222222222222")
            .is_err());
        assert!(!observer.protocol.failed);
        assert!(observer
            .continue_original_setup("11111111111111111111111111111111")
            .is_err());
        assert!(observer.protocol.failed && observer.protocol.continued);
        assert!(observer
            .poll(Instant::now() + std::time::Duration::from_secs(1))
            .is_err());
    }
    #[test]
    fn stream_bounds_and_partial_eof_are_unknown() {
        let mut bytes = ADMITTED.to_vec();
        bytes.push(b'\n');
        bytes.extend_from_slice(PUBLISHED);
        bytes.push(b'\n');
        let mut r = &bytes[..];
        assert_eq!(read_event(&mut r).unwrap().unwrap(), ADMITTED);
        assert_eq!(read_event(&mut r).unwrap().unwrap(), PUBLISHED);
        assert!(read_event(&mut r).unwrap().is_none());
        assert!(read_event(&mut &ADMITTED[..]).is_err());
        assert!(read_event(&mut &[b'x'; 257][..]).is_err());
    }
    #[test]
    fn lost_or_expired_observation_cannot_later_report_published() {
        for expired in [false, true] {
            let (sender, receiver) = mpsc::sync_channel(4);
            let mut protocol = Protocol::default();
            if !expired {
                sender.send(Err("EOF".into())).unwrap()
            }
            let first_deadline = if expired {
                Instant::now()
            } else {
                Instant::now() + std::time::Duration::from_secs(1)
            };
            assert!(observe_data(&mut protocol, &receiver, first_deadline).is_err());
            sender.send(Ok(ADMITTED.to_vec())).unwrap();
            sender.send(Ok(PUBLISHED.to_vec())).unwrap();
            assert!(observe_data(
                &mut protocol,
                &receiver,
                Instant::now() + std::time::Duration::from_secs(1)
            )
            .is_err());
        }
    }
    #[test]
    fn process_plan_cannot_inherit_engine_or_home_selectors() {
        let vars = [
            ("Path", "system"),
            ("Home", "foreign"),
            ("USERPROFILE", "foreign"),
            ("RDD_INSTANCE", "rancher"),
            ("DOCKER_HOST", "foreign"),
            ("LIMA_HOME", "foreign"),
            ("KUBECONFIG", "foreign"),
            ("BUILDKIT_HOST", "foreign"),
            ("BUILDX_CONFIG", "foreign"),
            ("ORBWIN_BUILD_SDK", "foreign"),
            ("=C:", "foreign"),
        ];
        let output = environment(vars.into_iter().map(|(a, b)| (a.into(), b.into()))).unwrap();
        assert_eq!(
            output,
            vec![
                ("Path".into(), "system".into()),
                ("RDD_INSTANCE".into(), "orbwin".into())
            ]
        );
        assert!(environment([("Path".into(), "a".into()), ("PATH".into(), "b".into())]).is_err());
        assert_eq!(ARGV, ["private-runtime-operation", "setup"]);
        // Actual native profile deliberately has no suspended/job/breakaway flag.
    }

    #[test]
    fn actual_incomplete_events_are_terminal_data_without_success() {
        for (name, stage) in [
            ("service-readiness", IncompleteStage::ServiceReadiness),
            ("runtime-observation", IncompleteStage::RuntimeObservation),
            ("runtime-publication", IncompleteStage::RuntimePublication),
        ] {
            let line = format!("{{\"schema\":2,\"state\":\"setup-incomplete\",\"setupSuccess\":false,\"runtimePublication\":false,\"stage\":\"{name}\"}}");
            let mut p = Protocol::default();
            assert!(p.event(line.as_bytes()).is_err());
            let mut p = Protocol::default();
            p.event(ADMITTED).unwrap();
            assert_eq!(
                p.event(line.as_bytes()).unwrap(),
                ObservationData::SetupIncomplete { stage }
            );
            assert_eq!(p.incomplete, Some(stage));
            assert!(!p.published && !p.paused);
            assert!(p.event(PUBLISHED).is_err());
            assert!(p.event(line.as_bytes()).is_err());
            assert!(stage.message().contains("may continue"));
            let mut p = Protocol::default();
            p.event(ADMITTED).unwrap();
            p.event(DECISION).unwrap();
            assert!(p.event(line.as_bytes()).is_err());
            let mut p = Protocol::default();
            p.event(ADMITTED).unwrap();
            p.event(DECISION).unwrap();
            p.pending_nonce = None;
            p.continued = true; // DATA state only; no native admission is supplied.
            assert_eq!(
                p.event(line.as_bytes()).unwrap(),
                ObservationData::SetupIncomplete { stage }
            );
        }
    }

    #[test]
    fn incomplete_codec_rejects_stage_leaks_aliases_and_success_bits() {
        let valid = r#"{"schema":2,"state":"setup-incomplete","setupSuccess":false,"runtimePublication":false,"stage":"service-readiness"}"#;
        for line in [
            valid.replace("service-readiness", "other"),
            valid.replace("service-readiness", "Service-Readiness"),
            valid.replace(",\"stage\":\"service-readiness\"", ""),
            valid.replace("\"setupSuccess\":false", "\"setupSuccess\":true"),
            valid.replace(
                "\"runtimePublication\":false",
                "\"runtimePublication\":true",
            ),
            valid.replace("\"stage\":", "\"stage\":\"runtime-observation\",\"stage\":"),
            valid.replace("\"stage\":", "\"path\":\"C:\\\\other\",\"stage\":"),
            valid.replace(
                "\"stage\":",
                "\"nonce\":\"11111111111111111111111111111111\",\"stage\":",
            ),
        ] {
            let mut p = Protocol::default();
            p.event(ADMITTED).unwrap();
            assert!(p.event(line.as_bytes()).is_err());
        }
        for old in [ADMITTED, PAUSED, PUBLISHED, DECISION] {
            let text = std::str::from_utf8(old).unwrap();
            let with_stage =
                text.trim_end_matches('}').to_owned() + ",\"stage\":\"service-readiness\"}";
            let mut p = Protocol::default();
            if old != ADMITTED {
                p.event(ADMITTED).unwrap();
            }
            assert!(p.event(with_stage.as_bytes()).is_err());
        }
    }
}
