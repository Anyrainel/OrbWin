//! Actual terminal controls and two-slot native admission. No engine bootstrap.
use std::{
    collections::HashMap,
    sync::{
        atomic::{AtomicBool, Ordering},
        Arc, Mutex,
    },
};
use tokio::sync::{mpsc, Notify};
type Slots = HashMap<String, Arc<Session>>;
#[derive(Default)]
pub(super) struct Inner {
    pub(super) slots: Mutex<Slots>,
    pub(super) shutdown: AtomicBool,
}
#[derive(Default)]
pub struct Sessions(pub(super) Arc<Inner>);
impl Drop for Sessions {
    fn drop(&mut self) {
        self.0.shutdown.store(true, Ordering::Release);
        if let Ok(slots) = self.0.slots.lock() {
            for session in slots.values() {
                session.close();
            }
        }
    }
}
pub(super) struct Session {
    pub(super) controls: mpsc::Sender<Vec<u8>>,
    pub(super) outstanding: Mutex<usize>,
    pub(super) credit: Notify,
    pub(super) cancellation: Notify,
    pub(super) closed: AtomicBool,
    pub(super) finished: AtomicBool,
}
impl Session {
    pub(super) fn control() -> (Arc<Self>, mpsc::Receiver<Vec<u8>>) {
        let (controls, receiver) = mpsc::channel(8);
        (
            Arc::new(Self {
                controls,
                outstanding: Mutex::new(0),
                credit: Notify::new(),
                cancellation: Notify::new(),
                closed: AtomicBool::new(false),
                finished: AtomicBool::new(false),
            }),
            receiver,
        )
    }
    pub(super) fn close(&self) {
        self.closed.store(true, Ordering::Release);
        self.cancellation.notify_one();
        self.credit.notify_one();
    }
    pub(super) fn send(&self, value: serde_json::Value) -> Result<(), String> {
        self.send_frames(vec![value])
    }
    pub(super) fn send_frames(&self, values: Vec<serde_json::Value>) -> Result<(), String> {
        if self.closed.load(Ordering::Acquire) || self.finished.load(Ordering::Acquire) {
            return Err("Terminal session has ended.".into());
        }
        let frames = values
            .into_iter()
            .map(|value| {
                let mut bytes = serde_json::to_vec(&value).map_err(|e| e.to_string())?;
                bytes.push(b'\n');
                Ok::<_, String>(bytes)
            })
            .collect::<Result<Vec<_>, _>>()?;
        let permits = self.controls.try_reserve_many(frames.len()).map_err(|_| {
            "Terminal input is busy or closed; wait before sending more data.".to_string()
        })?;
        for (permit, bytes) in permits.zip(frames) {
            permit.send(bytes);
        }
        Ok(())
    }
}
pub(super) struct Reservation {
    pub(super) inner: Arc<Inner>,
    pub(super) id: String,
    pub(super) armed: bool,
}
pub(super) fn reserve(
    inner: Arc<Inner>,
    id: String,
    session: Arc<Session>,
) -> Result<Reservation, String> {
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
        slots.insert(id.clone(), session);
    }
    Ok(Reservation {
        inner,
        id,
        armed: true,
    })
}
pub(super) struct Opening(pub(super) Arc<Session>, pub(super) bool);
impl Drop for Opening {
    fn drop(&mut self) {
        if self.1 {
            self.0.close();
        }
    }
}
pub(super) async fn await_creation<T>(
    creation: impl std::future::Future<Output = T>,
    session: &Session,
    cancel: impl FnOnce(),
) -> T {
    tokio::pin!(creation);
    tokio::select! {
        biased;
        _ = session.cancellation.notified() => { cancel(); creation.await }
        result = &mut creation => result,
    }
}
#[cfg(windows)]
pub(super) async fn spawn_reserved(
    reservation: &mut Reservation,
    session: &Session,
    spec: crate::native_client::NativeSpec,
) -> Result<crate::native_client::NativeClient, String> {
    let cancellation = spec.cancel_handle();
    reservation.armed = false;
    let spawned = await_creation(
        crate::native_client::NativeClient::spawn(spec),
        session,
        || cancellation.cancel(),
    )
    .await;
    match spawned {
        Ok(child) => Ok(child),
        Err(mut error) => {
            if let Some(stopped) = error.take_admission_stopped() {
                stopped.consume();
                reservation.armed = true;
            }
            Err(error.message)
        }
    }
}
impl Drop for Reservation {
    fn drop(&mut self) {
        if self.armed {
            if let Ok(mut slots) = self.inner.slots.lock() {
                slots.remove(&self.id);
            }
        } else {
            // Unknown native cleanup: retain admission even if the last UI
            // owner drops. NativeClient independently retains its exact bundle.
            std::mem::forget(self.inner.clone());
        }
    }
}
