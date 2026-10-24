# Phase 1 qualification scenarios

Current status and fresh checks: [2026-10-24 reassessment](phase-1-readiness-2026-10-24.md).
The table below is the historical initial scenario inventory, not current test
status. Results are recorded in that reassessment and development progress.
All guest tests require isolated identity and state. Never obtain a result by
starting/stopping Docker Desktop, terminating user distributions or changing a
global client context. Keep raw versions, environment, timing and failure layer.

| Area | Required proof | Current implementation/evidence |
| --- | --- | --- |
| Source | Pin + patch replay, notices, upgrade drift rejection | Source lock, patch preparation, focused Windows tests |
| Endpoint | Separate listener/discovery, occupied listener, explicit clients | Patch tests; no runtime Docker traffic yet |
| Guest identity | Correct guest, reject foreign signature, separate shared WSL resources | BL-002; shared vsock ports and veth need adaptation |
| Management | TLS/auth/readiness, independently running service, no UI lifetime coupling | Native desktop has no shutdown handler; runtime proof pending |
| Service tests | Redis/Postgres readiness, exec, restart, persistence, cleanup | Opt-in workload harness; gated on qualified daemon ID |
| Clients | Docker CLI, Compose, Buildx build/session, Testcontainers + cleanup helper | Harness version/readiness stage; builds/helpers pending |
| Projects | Windows system/non-system drives, spaces/Unicode, WSL sources, file/directory/read-only mounts | Pending |
| Streams | Follow logs, EOF, interactive TTY exec, detach, cancellation/backpressure | Upstream bridge tests preserved; end-to-end pending |
| Parallelism | Repeated/concurrent service suites, dynamic ports and no wrong-engine traffic | Harness unique Compose project; concurrent runtime pending |
| Networking | Same-port conflict, inbound/outbound, DNS, host access, VPN/proxy transitions | Pending, no guarantee of identical host-port sharing |
| Coexistence | Docker Desktop/Rancher/OrbWin both start orders, workloads maintained in all | Pending; guest launches gated |
| Fault layers | Assertion failure, process exit/OOM, client disconnect, UI loss, bridge loss, engine loss, WSL loss | Classify separately; destructive faults only in disposable host |
| Data | Named-volume persistence, restart/interruption, failed update and recovery | Service restart case prepared; no fault/upgrades tested |
| UI | Loading/empty/error/unavailable/retry, keyboard/focus/a11y, minimum window/scaling | Browser unavailable-state QA begun; runtime/native QA pending |
| Cluster | Enable/disable, context targeting, workloads, logs, services, storage, restart | Pending; keep management API distinct from workload cluster |
| Distribution | Reproducible Windows bundle/install/uninstall, no removal of user data | Desktop builds underway, no release qualification |

The workload harness's test password belongs only to disposable PostgreSQL; its
ports bind loopback. `down --volumes` is scoped to the generated test project.
No global prune or unrelated container removal is allowed. Interrupted harness
cleanup is a remaining case: retain the project ID and diagnose before removal.
Do not copy a sample identity record to bypass qualification.

Public issue-derived scenarios must be added with exact source, affected versions,
failure signature, and reproduction status. This table is initially seeded from
engineering knowledge and the current source audit; it is not comparative crash
rate evidence or a completed survey of public defects.
