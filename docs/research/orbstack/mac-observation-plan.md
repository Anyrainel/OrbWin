# Conditional OrbStack runtime observation plan

Planned only; no Mac is currently available. This is an evidence collection procedure, not a prerequisite for continuing public-source research or a claim of completed observation.

Record macOS build/architecture, OrbStack version/build/channel, Docker/Compose/kubectl versions, relevant settings, network conditions and fixture identity. Use an authorized disposable environment and legitimate app access. Respect published terms; binary investigation is a separate question from observing ordinary usage.

For each scenario capture: flow ID, exact starting state, action/input, branch, transition, visible output, backend-visible result, cancellation/retry, persisted state, timestamp and evidence artifact. Distinguish screenshot, operator recording, CLI result and REA evidence. Redact unrelated personal material and secrets.

Use disposable fixtures: running/stopped/fast-exit/shell-free containers, noisy UTF-8 logs, two Compose stacks, missing config/image, port collision, named-volume sentinel, read-only image files, Kubernetes workload and disposable Linux machine where authorized. Pin fixture images and preserve before/after inventories.

Run successful and relevant failed/cancelled cases for OS-001–010 and the gaps table. Pay particular attention to partially completed operations, app/backend loss, missing source, denied certificate trust, resource-in-use deletion and reopen persistence. Record exact confirmations and keyboard focus; do not assume shortcuts or defaults. Destructive cases apply only to explicitly disposable resources.

Optional REA UI observation requires actual supported tools, scoped readiness, exact app PID/window and appropriate macOS permissions. Observe before running a bounded scenario; preserve Evidence IDs, export records and clean up owned sessions. Tool availability is not a claim that every control or branch can be inspected.

Completion means evidence for each covered branch, or an explicit unknown. A single happy-path recording does not reconstruct the whole application. Reconcile findings with the public sources, retain discrepancies and bind conclusions to the tested build.
