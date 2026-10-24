# OrbWin quality standard

Recorded: 2026-10-21.
Updated: 2026-10-22 with recovery, coexistence and test-workload requirements.

The user requires exceptionally high code quality: kernel-grade engineering
discipline for important core code and a highly organized, deliberately designed
frontend. These are enforceable working practices and release gates, not a claim
that our code is Linux-kernel-reviewed or that testing can prove absence of bugs.

## Kernel-grade discipline for important core code

This applies especially to lifecycle, transport, storage, migration, networking,
credentials, privilege boundaries, update/recovery and shared-state code.

### Design before consequential implementation

Document the relevant invariants, state transitions and failure model. Specify:

- The authoritative owner of each state and resource.
- Valid transitions and behavior when an operation is repeated or interrupted.
- Process, file descriptor, socket, mount, lock and temporary-file lifetimes.
- Concurrency model, lock ordering, blocking operations and cancellation behavior.
- Trust boundaries, authentication/authorization, input/path validation and secrets.
- Crash consistency, persistence ordering, migration and rollback constraints.
- Error propagation, user-visible consequences and the smallest safe recovery.

Use a short module note or a focused code comment when that is sufficient. Do not
generate paperwork unrelated to a module's actual risk. Reuse established upstream
mechanisms rather than introducing abstractions without a concrete requirement.

### Implementation rules

- Prefer small explicit interfaces and bounded responsibilities.
- Keep backend lifecycle independent of renderer/window state.
- Do not suppress unexpected errors, create false successful states or retry
  forever. Use context-appropriate deadlines, cancellation and bounded backoff.
- Preserve streaming, EOF/half-close and backpressure semantics in transport code.
- Preserve Docker API compatibility when translating traffic, including fields
  outside our current model. Test interactive and BuildKit/session traffic.
- Keep persistent user data outside disposable UI/app artifacts. Never use
  destructive reset as routine recovery or silently discard data during migration.
- Distinguish process restart, engine restart, distro termination and global WSL
  shutdown. Recover the smallest affected scope.
- Protect local endpoints and privileged helpers; keep operations narrowly scoped.
- Retain upstream provenance and meaningful tests. Record local patches and the
  reason each exists so upstream merges remain tractable.
- Preserve other installed/running engines: owned namespaces, endpoints and data;
  explicit Docker/Kubernetes client targeting; no surprise global context changes,
  shutdowns, or shared writable persistent engine state.
- Put upstream-version differences behind a narrow adapter. Record supported
  backend versions, upgrade steps and patch provenance; exercise compatibility
  checks when updating instead of freezing a rapidly evolving upstream.

### Recovery interaction policy

Use evidence-backed OrbStack recovery flows as the starting point. Apply the same
product intent to unique Windows/WSL conditions: smooth, clean recovery when
feasible; transparent, prompt failure when reliable recovery is infeasible or
overly complex. State what failed, its workload/data impact and the next useful
action. Avoid endless spinners, silent destructive retries and exposing unnecessary
implementation detail. An automatic UI reconnect must not be confused with safely
restarting an engine during a test run. Preserve evidence needed to diagnose the
original crash before recovery overwrites it. Label proposed adaptations honestly;
do not claim to know OrbStack's undocumented internal policy.

### Verification

Use tests that exercise behavior and invariants, rather than mirroring code:

- Unit/property/fuzz tests for parsing, paths, protocol/state transitions and
  untrusted boundaries where appropriate.
- Real contract/integration tests against the pinned backend and Docker clients.
- Concurrency/race and resource-leak checks for meaningful lifecycle/transport code.
- Fault injection: cancellation, partial startup, process loss, stale endpoints,
  partial I/O, disk pressure, interrupted writes/updates and reconnect races.
- Data-preservation tests covering failed upgrades, restarts and supported recovery.
- Appropriate static analysis, formatting, linting and compilation for touched code.

Choose checks proportional to the change, preserve required upstream checks and
report actual results. Do not write redundant tests for trivial reversible edits.
Long-lived core changes require a focused review of correctness, concurrency,
resource ownership, failure paths and regression risk before phase acceptance.

### Docker-dependent test reliability

Engineering owns the initial scenario inventory. Begin with established Docker
client/runtime behavior and representative Linux service components; expand from
versioned public Docker Desktop, Rancher, WSL, Moby and test-framework reports and
our own failures. User-provided reproductions are useful, not a prerequisite.

Cover endpoint discovery from Windows and WSL; per-process engine targeting;
container/image pull and build failures; readiness/health checks; dynamic ports;
container-to-host and inter-container connections; bind mounts/volumes; streamed
logs and exec; exit status; test cancellation; repeated and concurrent suites;
cleanup after failed tests; engine/bridge loss; resource and disk pressure;
sleep/resume and network changes. Qualify representative Docker test-framework
clients, including helper/cleanup containers and socket mounts when used. Do not
confuse a test assertion failure, expected container exit/OOM, client disconnect,
UI crash, host bridge failure, engine crash and WSL/platform loss.

Record each public report's source, affected versions/environment, failure
signature and reproduction status. A forum anecdote identifies a scenario to
investigate; it does not establish a current product defect or comparative crash
rate. Use isolated disposable environments for destructive fault injection and
resource exhaustion; protect the user's existing engines and data.

## Design-system-led frontend implementation

The goal is the consistency of a deliberately designed product without requiring
a Figma document. Use versioned interaction specifications, a token system,
component contracts, prototypes and rendered visual review as design artifacts.

Before broad screen construction, define:

- Information architecture and navigation, including project/container relationships.
- Semantic colors, typography, spacing, sizing, density, elevation and motion.
- Reusable tables, lists, detail panes, forms, dialogs, terminals and notifications.
- Keyboard navigation, focus behavior, shortcuts, accessible names and announcements.
- Loading, progress, empty, error, offline/disconnected, cancellation and retry states.
- Destructive-operation consequences and data-preservation wording.
- Backend-limited adaptations and honest disabled/unavailable states.

Organize code into presentation, interaction/application logic and backend adapters.
Keep backend-derived state authoritative and handle stale requests/events explicitly.
Avoid ad hoc global state, duplicated request logic and one-off visual values.

Use development fixtures/component examples to inspect important states. Review
rendered output at supported window sizes, display scaling and themes. Include
keyboard and accessibility checks, realistic large lists/logs, and visual regression
coverage for important stable views. Inspect actual results; a passing build is
not visual QA. Use Windows conventions when they improve the intended flow.

## Phase release gate

A phase is acceptable only when:

1. It yields a working runnable product inside a documented support matrix.
2. Every in-scope acceptance criterion passes with recorded evidence.
3. There are no known unresolved product-owned correctness, security, data-loss or
   major interaction defects in the supported scope. A smaller scope must be an
   explicit product decision, not a way to hide a failed required workflow.
4. External/backend limitations have reproducers and useful UI handling; they do
   not invalidate the qualified scope. Their origin does not excuse an unsafe release.
5. Core review, relevant integration/failure tests, and frontend visual/interaction
   review are complete. Any remaining minor issue or risk is explicit and triaged.
6. Performance/resource regressions are compared with recorded budgets where relevant.
7. Builds, dependencies, local patches and test instructions are reproducible.
8. Full phase scope is accounted for separately from intermediate milestones;
   required simultaneous coexistence and Docker-dependent test workloads pass.

Do not describe these gates as proof of zero defects. State what was tested,
what remains unknown and what users can safely rely on.

## Reliability and performance evidence

Record versions, hardware/architecture, Windows edition/build, WSL configuration,
storage/project location, network conditions, workload, cache state, sample count,
measurements and failures. Compare the chosen upstream baseline and Docker Desktop
where relevant. Distinguish a renderer improvement from an engine/platform change.

Use startup/idle/build/filesystem measurements and long-running/fault-recovery
workloads appropriate to the claim. Baseline collection starts in phase 1; phase 3
is focused improvement work, not the first point at which reliability matters.
