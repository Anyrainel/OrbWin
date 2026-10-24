//! Per-session clients for the Docker SDK helper. No host console or host shell.
//! rdd/Moby own workloads. Two clients maximum; 8 KiB input writes, 64 KiB
//! acknowledged output, 15-second stalled-viewer deadline. Closing kills/reaps
//! only this SDK client; its exec process may remain in the container. The helper
//! closes its hijacked connection on stdin EOF and never restarts an engine.
use crate::engine;
use serde::{Deserialize, Serialize};
use std::{
    collections::HashMap,
    io::{Read, Write},
    process::{Child, ChildStdin, Command, Stdio},
    sync::{
        atomic::{AtomicBool, Ordering},
        Arc, Condvar, Mutex,
    },
    time::Duration,
};
use tauri::ipc::Channel;
type Slots = HashMap<String, Option<Arc<Session>>>;
#[derive(Default)]
struct Inner {
    slots: Mutex<Slots>,
    shutdown: AtomicBool,
}
#[derive(Default)]
pub struct Sessions(Arc<Inner>);
impl Drop for Sessions {
    fn drop(&mut self) {
        self.0.shutdown.store(true, Ordering::Release);
        if let Ok(slots) = self.0.slots.lock() {
            for session in slots.values().flatten() {
                session.close();
            }
        }
    }
}
struct Session {
    writer: Mutex<Option<ChildStdin>>,
    child: Mutex<Child>,
    outstanding: Mutex<usize>,
    credit: Condvar,
    closed: AtomicBool,
    finished: AtomicBool,
}
impl Session {
    fn close(&self) {
        self.closed.store(true, Ordering::Release);
        self.credit.notify_all();
        if let Ok(mut child) = self.child.lock() {
            let _ = child.kill();
        }
    }
    fn send(&self, value: serde_json::Value) -> Result<(), String> {
        if self.closed.load(Ordering::Acquire) || self.finished.load(Ordering::Acquire) {
            return Err("Terminal session has ended.".into());
        }
        let mut writer = self
            .writer
            .try_lock()
            .map_err(|_| "Terminal input is busy; wait before sending more data.")?;
        let writer = writer.as_mut().ok_or("Terminal input has closed.")?;
        serde_json::to_writer(&mut *writer, &value).map_err(|e| e.to_string())?;
        writer.write_all(b"\n").map_err(|e| e.to_string())
    }
}
#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum Shell {
    Sh,
    Bash,
}
#[derive(Clone, Serialize)]
#[serde(tag = "kind", rename_all = "camelCase")]
pub enum Event {
    Ready,
    Data { bytes: Vec<u8> },
    Exit { code: Option<u32>, message: String },
}
fn size(cols: u16, rows: u16) -> Result<(), String> {
    if !(10..=400).contains(&cols) || !(2..=150).contains(&rows) {
        return Err("Terminal dimensions are outside supported bounds.".into());
    }
    Ok(())
}
fn valid_id(id: &str) -> bool {
    id.len() == 36 && id.bytes().all(|c| c.is_ascii_hexdigit() || c == b'-')
}
struct Reservation {
    inner: Arc<Inner>,
    id: String,
    armed: bool,
}
impl Drop for Reservation {
    fn drop(&mut self) {
        if self.armed {
            if let Ok(mut slots) = self.inner.slots.lock() {
                slots.remove(&self.id);
            }
        }
    }
}
pub async fn open(
    state: &Sessions,
    id: String,
    container: String,
    shell: Shell,
    cols: u16,
    rows: u16,
    output: Channel<Event>,
) -> Result<(), String> {
    if !valid_id(&id) || container.len() != 64 || !container.bytes().all(|c| c.is_ascii_hexdigit())
    {
        return Err("Select a container with a full engine ID.".into());
    }
    size(cols, rows)?;
    let inner = state.0.clone();
    {
        let mut slots = inner
            .slots
            .lock()
            .map_err(|_| "Session registry is unavailable")?;
        if inner.shutdown.load(Ordering::Acquire) || slots.len() >= 2 || slots.contains_key(&id) {
            return Err(
                "Two terminal clients are already open, or the session ID is in use.".into(),
            );
        }
        slots.insert(id.clone(), None);
    }
    let reservation = Reservation {
        inner: inner.clone(),
        id: id.clone(),
        armed: true,
    };
    let target = engine::target()?;
    engine::check(&target).await?;
    let binary = target.session().await?;
    let initial = serde_json::json!({"kind":"open","container":container,"daemonId":target.daemon_id(),"shell":match shell { Shell::Sh => "sh", Shell::Bash => "bash" },"cols":cols,"rows":rows});
    tokio::task::spawn_blocking(move || {
        let mut reservation = reservation;
        let mut slots = inner.slots.lock().map_err(|_| "Session registry is unavailable")?;
        if inner.shutdown.load(Ordering::Acquire) { return Err("Application is closing.".into()); }
        let mut command = Command::new(binary);
        command.stdin(Stdio::piped()).stdout(Stdio::piped()).stderr(Stdio::piped());
        #[cfg(windows)]
        { use std::os::windows::process::CommandExt; command.creation_flags(0x08000000); }
        let mut child = command.spawn().map_err(|e| e.to_string())?;
        let mut writer = child.stdin.take().ok_or("Missing terminal input")?;
        let mut reader = child.stdout.take().ok_or("Missing terminal output")?;
        let mut stderr = child.stderr.take().ok_or("Missing terminal diagnostics")?;
        if let Err(error) = serde_json::to_writer(&mut writer, &initial).map_err(|e| e.to_string()).and_then(|_| writer.write_all(b"\n").map_err(|e| e.to_string())) { let _ = child.kill(); let _ = child.wait(); return Err(error); }
        let session = Arc::new(Session { writer: Mutex::new(Some(writer)), child: Mutex::new(child), outstanding: Mutex::new(0), credit: Condvar::new(), closed: AtomicBool::new(false), finished: AtomicBool::new(false) });
        slots.insert(id.clone(), Some(session.clone())); reservation.armed = false; drop(slots);
        let diagnostics = Arc::new(Mutex::new(Vec::<u8>::new())); let captured = diagnostics.clone();
        let ready_output = output.clone();
        let stderr_worker = std::thread::spawn(move || {
            let mut buffer = [0u8; 1024]; let mut ready_sent = false;
            while let Ok(n) = stderr.read(&mut buffer) {
                if n == 0 { break; }
                let mut captured = captured.lock().unwrap_or_else(|e| e.into_inner());
                let count = n.min(8192 - captured.len()); captured.extend_from_slice(&buffer[..count]);
                const READY: &[u8] = b"ORBWIN_SESSION_READY\n";
                if !ready_sent && captured.starts_with(READY) { captured.drain(..READY.len()); ready_sent = true; let _ = ready_output.send(Event::Ready); }
            }
        });
        let (read_done, read_finished) = std::sync::mpsc::channel(); let reading = session.clone(); let stream = output.clone();
        std::thread::spawn(move || {
            let mut buffer = [0u8; 4096];
            loop {
                let mut limit = buffer.len();
                if !reading.closed.load(Ordering::Acquire) {
                    let mut outstanding = reading.outstanding.lock().unwrap_or_else(|e| e.into_inner());
                    while *outstanding >= 65536 && !reading.closed.load(Ordering::Acquire) { let (next, wait) = reading.credit.wait_timeout(outstanding, Duration::from_secs(15)).unwrap_or_else(|e| e.into_inner()); outstanding = next; if wait.timed_out() { drop(outstanding); reading.close(); outstanding = reading.outstanding.lock().unwrap_or_else(|e| e.into_inner()); } }
                    if !reading.closed.load(Ordering::Acquire) { limit = buffer.len().min(65536 - *outstanding); }
                }
                match reader.read(&mut buffer[..limit]) {
                    Ok(0) => break,
                    Ok(n) => {
                        if !reading.closed.load(Ordering::Acquire) {
                            *reading.outstanding.lock().unwrap_or_else(|e| e.into_inner()) += n;
                            if stream.send(Event::Data { bytes: buffer[..n].to_vec() }).is_err() {
                                reading.close();
                            }
                        }
                    }
                    Err(error) if error.kind() == std::io::ErrorKind::Interrupted => continue,
                    Err(_) => {
                        // A broken local reader cannot service the helper. Kill
                        // only that client so its waiter can reap it and release
                        // the session slot; ordinary EOF retains remote status.
                        reading.close();
                        break;
                    }
                }
            }
            let _ = read_done.send(());
        });
        std::thread::spawn(move || {
            let status = loop { match session.child.lock().unwrap_or_else(|e| e.into_inner()).try_wait() { Ok(Some(status)) => break Some(status), Ok(None) => {}, Err(_) => break None } std::thread::sleep(Duration::from_millis(50)); };
            session.finished.store(true, Ordering::Release);
            session.writer.lock().unwrap_or_else(|e| e.into_inner()).take();
            if read_finished.recv_timeout(Duration::from_secs(20)).is_err() { session.close(); }
            let _ = stderr_worker.join();
            let code = status.and_then(|status| status.code()).map(|code| code as u32);
            let detail = String::from_utf8_lossy(&diagnostics.lock().unwrap_or_else(|e| e.into_inner())).to_string();
            let message = if session.closed.load(Ordering::Acquire) { "Terminal client disconnected. The container and its workloads were not stopped; an exec process may remain inside it." } else { "Terminal client exited." };
            let _ = output.send(Event::Exit { code, message: format!("{message} {detail}") });
            if let Ok(mut slots) = inner.slots.lock() { slots.remove(&id); }
        }); Ok(())
    }).await.map_err(|e| e.to_string())?
}
fn get(state: &Sessions, id: &str) -> Result<Arc<Session>, String> {
    state
        .0
        .slots
        .lock()
        .map_err(|_| "Session registry is unavailable")?
        .get(id)
        .and_then(Clone::clone)
        .ok_or("Terminal session is not connected.".into())
}
pub fn close(state: &Sessions, id: &str) -> Result<(), String> {
    get(state, id)?.close();
    Ok(())
}
pub fn acknowledge(state: &Sessions, id: &str, bytes: usize) -> Result<(), String> {
    let session = get(state, id)?;
    let mut outstanding = session
        .outstanding
        .lock()
        .map_err(|_| "Terminal credit is unavailable")?;
    *outstanding = outstanding.saturating_sub(bytes.min(65536));
    session.credit.notify_one();
    Ok(())
}
pub async fn input(state: &Sessions, id: &str, bytes: Vec<u8>) -> Result<(), String> {
    if bytes.len() > 8192 {
        return Err("Terminal input is limited to 8 KiB per write.".into());
    }
    let session = get(state, id)?;
    tokio::task::spawn_blocking(move || {
        session.send(serde_json::json!({"kind":"input","data":bytes}))
    })
    .await
    .map_err(|e| e.to_string())?
}
pub async fn resize(state: &Sessions, id: &str, cols: u16, rows: u16) -> Result<(), String> {
    size(cols, rows)?;
    let session = get(state, id)?;
    tokio::task::spawn_blocking(move || {
        session.send(serde_json::json!({"kind":"resize","cols":cols,"rows":rows}))
    })
    .await
    .map_err(|e| e.to_string())?
}
#[cfg(test)]
#[path = "terminal_qualification.rs"]
mod qualification;
#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn validates_dimensions_and_session_ids() {
        assert!(size(80, 24).is_ok());
        assert!(size(0, 24).is_err());
        assert!(size(80, 1000).is_err());
        assert!(!valid_id("../../outside"));
    }
}
