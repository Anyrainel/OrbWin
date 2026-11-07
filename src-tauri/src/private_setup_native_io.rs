//! Managed DATA IO. No Drop/EOF is a raw native close/retirement certificate.
use std::{
    io::{self, Write},
    sync::mpsc,
    time::Duration,
};
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt},
    net::windows::named_pipe::NamedPipeServer,
};
struct Command {
    bytes: Vec<u8>,
    reply: mpsc::SyncSender<Result<usize, String>>,
}
pub(crate) struct SetupInput {
    sender: tokio::sync::mpsc::Sender<Command>,
}
impl Write for SetupInput {
    fn write(&mut self, bytes: &[u8]) -> io::Result<usize> {
        if bytes.is_empty() || bytes.len() > 132 {
            return Err(io::Error::other("Setup frame bounds."));
        }
        let (tx, rx) = mpsc::sync_channel(1);
        self.sender
            .try_send(Command {
                bytes: bytes.to_vec(),
                reply: tx,
            })
            .map_err(|_| io::Error::other("Setup writer unavailable."))?;
        match rx.recv_timeout(Duration::from_secs(30)) {
            Ok(Ok(n)) => Ok(n),
            _ => Err(io::Error::other("Setup frame delivery unknown.")),
        }
    }
    fn flush(&mut self) -> io::Result<()> {
        Ok(())
    }
}
// Sole writer owns every request buffer until managed API write/buffer handoff.
// No caller timeout aborts its task or closes under an active write.
fn input_worker(
    mut pipe: NamedPipeServer,
) -> (
    SetupInput,
    tokio::sync::oneshot::Sender<()>,
    tokio::task::JoinHandle<()>,
) {
    let (tx, mut rx) = tokio::sync::mpsc::channel::<Command>(2);
    let (close, mut closing) = tokio::sync::oneshot::channel();
    let work = tokio::spawn(async move {
        loop {
            let command = tokio::select! {biased;_=&mut closing=>break,c=rx.recv()=>match c{Some(c)=>c,None=>break}};
            let result = async {
                pipe.write_all(&command.bytes).await?;
                pipe.flush().await
            }
            .await;
            let failed = result.is_err();
            let answer = result
                .map(|_| command.bytes.len())
                .map_err(|_| "Setup frame delivery unknown.".into());
            let _ = command.reply.send(answer);
            if failed {
                break;
            }
        }
        // Managed Drop after API write future completion; mio retains pending buffers, never concurrent with active write.
        drop(pipe);
    });
    (SetupInput { sender: tx }, close, work)
}
async fn event(pipe: &mut NamedPipeServer) -> Result<Option<Vec<u8>>, String> {
    let mut bytes = Vec::with_capacity(257);
    loop {
        let mut b = [0u8; 1];
        match pipe.read(&mut b).await {
            Ok(0) if bytes.is_empty() => return Ok(None),
            Ok(0) => return Err("Partial setup event.".into()),
            Ok(1) if b[0] == b'\n' => return Ok(Some(bytes)),
            Ok(1) => {
                bytes.push(b[0]);
                if bytes.len() > 256 {
                    return Err("Oversized setup event.".into());
                }
            }
            _ => return Err("Setup observation read unknown.".into()),
        }
    }
}
pub(crate) async fn attach(
    mut child: crate::native_client::PersistentSetupClient,
) -> Result<(SetupInput, mpsc::Receiver<Result<Vec<u8>, String>>), String> {
    let input = child.take_input().ok_or("Setup input unavailable.")?;
    let mut output = child.take_output().ok_or("Setup output unavailable.")?;
    let (input, close_input, writer) = input_worker(input);
    let (send, events) = mpsc::sync_channel(4);
    tokio::spawn(async move {
        let mut settlement = crate::persistent_setup_model::IoSettlement::default();
        loop {
            let line = match event(&mut output).await {
                Ok(Some(line)) => Ok(line),
                Ok(None) => Err("Setup observation ended; runtime state unknown.".into()),
                Err(e) => Err(e),
            };
            let terminal = line.is_err();
            if send.try_send(line).is_err() || terminal {
                break;
            }
        }
        // Output EOF/error closes original input independently of UI, BEFORE wait.
        settlement.end_output();
        let _ = close_input.send(());
        let joined = writer.await.is_ok();
        settlement.writer_join(joined);
        drop(output);
        if !joined {
            retain(child).await
        }
        loop {
            match child.try_wait() {
                Ok(Some(_)) => {
                    settlement.terminal();
                    break;
                }
                Ok(None) => tokio::time::sleep(Duration::from_millis(20)).await,
                Err(_) => retain(child).await,
            }
        }
        if !settlement.may_release() {
            retain(child).await
        }
        if child.retire().is_err() { /* Native guard keeps exact owner on unknown. */ }
    });
    Ok((input, events))
}
async fn retain(child: crate::native_client::PersistentSetupClient) -> ! {
    loop {
        std::hint::black_box(&child);
        tokio::time::sleep(Duration::from_secs(3600)).await
    }
}
