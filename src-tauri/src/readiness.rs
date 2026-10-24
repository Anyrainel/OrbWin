//! Observe publication only; never modify ports, restart a backend, or infer
//! readiness from Docker's running state. One absolute deadline covers checks
//! and retry delays, including a blocked check. Cancellation remains explicit.
use std::{future::Future, time::Duration};
use tokio::{
    sync::oneshot,
    time::{timeout_at, Instant},
};

#[derive(Debug, PartialEq)]
pub(crate) enum Publication {
    Ready,
    Unavailable(String),
    Failed(String),
    Cancelled,
}

pub(crate) async fn publication<F, Fut>(
    mut check: F,
    cancelled: &mut oneshot::Receiver<()>,
    budget: Duration,
    interval: Duration,
) -> Publication
where
    F: FnMut() -> Fut,
    Fut: Future<Output = Result<Option<String>, String>>,
{
    let deadline = Instant::now() + budget;
    let mut last="Windows publication did not become ready before the deadline. Containers may be running; refresh and inspect the host ports.".to_string();
    loop {
        if Instant::now() >= deadline {
            return Publication::Unavailable(last);
        }
        let result = tokio::select! {
            biased;
            _=&mut *cancelled => return Publication::Cancelled,
            value=timeout_at(deadline,check()) => value,
        };
        match result {
            Ok(Ok(None)) => return Publication::Ready,
            Ok(Ok(Some(message))) => last = message,
            Ok(Err(error)) => return Publication::Failed(error),
            Err(_) => return Publication::Unavailable(last),
        }
        if Instant::now() >= deadline {
            return Publication::Unavailable(last);
        }
        tokio::select! {
            biased;
            _=&mut *cancelled => return Publication::Cancelled,
            _=tokio::time::sleep_until((Instant::now()+interval).min(deadline)) => {},
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[tokio::test]
    async fn running_docker_is_not_a_publication_pass() {
        let (_sender, mut cancelled) = oneshot::channel();
        let mut attempts = 0;
        let result = publication(
            || {
                attempts += 1;
                std::future::ready(Ok((attempts < 3).then(|| "port not published".into())))
            },
            &mut cancelled,
            Duration::from_secs(1),
            Duration::from_millis(1),
        )
        .await;
        assert_eq!(result, Publication::Ready);
        assert_eq!(attempts, 3);
    }
    #[tokio::test]
    async fn failed_checks_are_not_retried_or_converted_to_success() {
        let (_sender, mut cancelled) = oneshot::channel();
        let mut attempts = 0;
        let result = publication(
            || {
                attempts += 1;
                std::future::ready(Err("wrong engine or helper identity".into()))
            },
            &mut cancelled,
            Duration::from_secs(1),
            Duration::from_millis(1),
        )
        .await;
        assert_eq!(
            result,
            Publication::Failed("wrong engine or helper identity".into())
        );
        assert_eq!(attempts, 1);
    }
    #[tokio::test]
    async fn deadline_covers_a_stalled_check_and_retains_conflict_detail() {
        let (_sender, mut cancelled) = oneshot::channel();
        let result = publication(
            std::future::pending,
            &mut cancelled,
            Duration::from_millis(15),
            Duration::from_millis(1),
        )
        .await;
        assert!(matches!(result, Publication::Unavailable(_)));
        let result = publication(
            || std::future::ready(Ok(Some("port 60753 unavailable".into()))),
            &mut cancelled,
            Duration::from_millis(15),
            Duration::from_millis(1),
        )
        .await;
        assert_eq!(
            result,
            Publication::Unavailable("port 60753 unavailable".into())
        );
    }
    #[tokio::test]
    async fn cancellation_wins_before_a_successful_check() {
        let (sender, mut cancelled) = oneshot::channel();
        sender.send(()).unwrap();
        let result = publication(
            || std::future::ready(Ok(None)),
            &mut cancelled,
            Duration::from_secs(1),
            Duration::from_millis(1),
        )
        .await;
        assert_eq!(result, Publication::Cancelled);
    }
}
