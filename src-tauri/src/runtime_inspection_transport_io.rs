use crate::runtime_inspection::Unavailable;
use crate::runtime_inspection_transport_model::Capture;
#[cfg(test)]
use crate::runtime_inspection_transport_model::{STDERR_LIMIT, STDOUT_LIMIT};
use tokio::io::{AsyncRead, AsyncReadExt, AsyncWrite, AsyncWriteExt};

pub(crate) async fn send_request<W: AsyncWrite + Unpin>(mut pipe: W) -> Result<(), Unavailable> {
    let request = crate::runtime_inspection::request_frame();
    pipe.write_all(&request).await.map_err(|_| Unavailable)?;
    pipe.flush().await.map_err(|_| Unavailable)?;
    // Normal owned endpoint Drop sends EOF. Child exit0 additionally requires
    // Go's actual exact-frame+EOF check; this isn't raw CloseHandle proof.
    drop(pipe);
    Ok(())
}
pub(crate) async fn drain<R: AsyncRead + Unpin>(
    mut pipe: R,
    limit: usize,
) -> Result<Capture, Unavailable> {
    let mut capture = Capture::new(limit);
    let mut buffer = [0u8; 4096];
    loop {
        let n = pipe.read(&mut buffer).await.map_err(|_| Unavailable)?;
        if n == 0 {
            return Ok(capture);
        }
        capture.push(&buffer[..n]); // Keep draining after sticky overflow.
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[tokio::test]
    async fn actual_drain_bounds_capture_and_consumes_through_eof() {
        let bytes = vec![b'x'; STDOUT_LIMIT + 8192];
        let result = drain(bytes.as_slice(), STDOUT_LIMIT).await.unwrap();
        assert_eq!(result.bytes().len(), STDOUT_LIMIT);
        assert!(result.overflow());
        assert!(drain(&b""[..], STDERR_LIMIT)
            .await
            .unwrap()
            .bytes()
            .is_empty());
    }
    #[tokio::test]
    async fn actual_writer_sends_exact_frame_once_and_eof() {
        let (writer, mut reader) = tokio::io::duplex(8);
        let task = tokio::spawn(send_request(writer));
        let mut bytes = Vec::new();
        reader.read_to_end(&mut bytes).await.unwrap();
        task.await.unwrap().unwrap();
        assert_eq!(bytes, crate::runtime_inspection::request_frame());
    }
    #[tokio::test]
    async fn receiver_loss_does_not_abort_owned_pending_reader() {
        let (mut writer, reader) = tokio::io::duplex(8);
        let (send, receive) = tokio::sync::oneshot::channel();
        let task = tokio::spawn(async move {
            let capture = drain(reader, 32).await;
            let _ = send.send(capture);
        });
        drop(receive);
        writer.write_all(b"bounded").await.unwrap();
        drop(writer);
        task.await.unwrap(); // The real drain owned its buffer until EOF.
    }
    #[tokio::test]
    async fn broken_output_is_not_request_completion() {
        let (writer, reader) = tokio::io::duplex(8);
        drop(reader);
        assert!(send_request(writer).await.is_err());
    }
    #[tokio::test]
    async fn bounded_capture_keeps_draining_backpressure_after_overflow() {
        let (mut writer, reader) = tokio::io::duplex(8);
        let task = tokio::spawn(drain(reader, 1));
        writer.write_all(&[b'x'; 8192]).await.unwrap();
        drop(writer);
        let result = task.await.unwrap().unwrap();
        assert_eq!(result.bytes(), b"x");
        assert!(result.overflow());
    }
}
