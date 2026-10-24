# Phase 1 readiness reassessment — 2026-10-24

Historical checkpoint: subsequent fixes and qualification are recorded in
[the completion record](phase-1-completion.md) and
[current coverage](../phase-1-coverage.md). The decision below describes this
earlier checkpoint, not the later working tree.

**Decision: not ready for Phase 1 acceptance or a Phase 2 implementation handoff.**
The current product is a working development build with successful scoped engine
proofs. Required implementation and qualification remain open. Passing the
checks below does not waive the roadmap or narrow the phase scope.

## Scope and environment

Assessed the current working tree against `docs/roadmap.md`,
`docs/quality-standard.md`, `docs/phase-1-coverage.md`, the limitation ledger,
current source, and fresh native/runtime checks. Earlier progress entries are
historical; this report supersedes their test counts and mount-test status.
Independent `docs/research/orbstack/` artifacts were not edited.

- Windows 11 Pro x64, build 26300.9711; WSL 2.7.11.0, kernel
  6.18.33.2-2. This single development host is not a qualified release matrix.
- Node 24.18.0; Rust 1.90.0; project-local Go 1.27.3.
- Rancher source `af6511ad4e065e24e7258f575a0fc090c10cdbda`, four pinned
  patches; backend SHA-256
  `edc0900cda42838219f7cb3be9e8032123a55cd88b004df7d5ae5ab64972e973`.
- Owned registration `lima-orbwin`, recorded home `artifacts/gp-2ce2d6e3`,
  endpoint `npipe:////./pipe/docker-rancher-desktop-orbwin`, daemon ID
  `1a2a3eb3-2f04-4fd6-a2d7-2f5e3fe13cb3`.
- Docker CLI 29.8.0 / Moby 29.7.2-ce, Compose 5.5.1, Buildx 0.37.0;
  workload images remain digest-pinned in the test fixtures.
- Ubuntu, Docker Desktop and Rancher registrations were stopped at entry.
  This pass resumed only the verified OrbWin guest. It does not establish
  simultaneous coexistence or either engine start order.

## Fresh checks

| Check | Result and boundary |
| --- | --- |
| `npm test` | 33 passed. Model, identity, patch, targeting and readiness checks. |
| `npm run build` | TypeScript and Vite production build passed. |
| `cargo test --manifest-path src-tauri/Cargo.toml` | 19 passed, six opt-in runtime tests ignored by default; selected runtime checks were executed separately below. |
| Rust Clippy / formatting | Clippy with `--all-targets -- -D warnings` passed. Initial formatting check found two layout issues; corrected and rechecked successfully. |
| `npm run upstream:test` | Pinned source and patch hashes verified; instance, socketbridge, hostswitch and App-controller packages passed (cached package results). This is not a fresh race run or the entire upstream suite. |
| Session helper `go test ./...` | Passed using project-local Go (cached). |
| Go race checks | Session helper and upstream instance/socketbridge passed with project-local MinGW and `CGO_ENABLED=1`; instance result was cached. Initial helper invocation without CGO was rejected by Go, then rerun with the required compiler configuration. This is focused race coverage, not a full concurrency qualification. |
| Native debug build | `npm run tauri -- build --debug --no-bundle` passed; an executable is not an installer. |
| Preview boundary | Running Vite served OrbWin; generated harmless artifacts were denied with HTTP 403 via normal and `/@fs/` URLs. |
| Resumed owned guest + service/build baseline | Passed with the same daemon identity: Alpine, Redis/PostgreSQL readiness, localhost dynamic-port Redis PING, exec/logs, volume data after container restart, Compose and Buildx build/load/run, project-scoped cleanup. Global Docker/Kubernetes config hashes unchanged and existing registrations preserved. |
| Native resource operations | Passed: volume/network/container operations, in-use removal rejection, network attachment, exec exit status, logs/stats, pause/unpause, restart/start/stop, persistence and scoped cleanup. |
| Native Run mount adapter | Failed initially on canonical `\\?\F:\...` input. Fixed classification, extended regression coverage, reran successfully: directory with spaces, writable file visible on Windows, read-only rejection, data retained across container restart, scoped cleanup. |
| Direct Windows Docker CLI bind | **Failed**, reproducibly: `invalid mount path: 'F:/Codes/Orbwin/tests/workloads/bind source' mount path must be absolute`. The Run adapter fix does not translate CLI/Compose traffic. |
| Interactive SDK adapter | Passed: input, resize, Ctrl+C, exit status 7, bounded stalled-viewer/backpressure handling and container survival/cleanup. This is not full native xterm UI acceptance. |
| Windows Node Testcontainers 12.2.1 | Passed: one normal suite, two concurrent suites, intentional assertion failure, cancellation, real Ryuk cleanup; no remaining recorded resources or fallback cleanup. Global client config hashes unchanged. |
| Native UI smoke | Rebuilt app rendered; stopped/startup explanation, manual reconnect to actual inventory, WSL listing, settings and disabled reset explanations inspected. Ctrl+1/Ctrl+3, Ctrl+comma, Ctrl+K/Escape, search no-match and Escape-clear exercised. Scope excludes destructive native actions, full keyboard/a11y, scaling, all dialogs and cluster/session flows. |

Local raw evidence is retained under ignored `artifacts/`; these paths are not
committed evidence bundles and require this qualification workspace to inspect:

- `artifacts/gp-2ce2d6e3/evidence.json`, run
  `a382f02c-6fb8-4c52-a159-131be8d126ca`, with logs in that home.
- Mount failure: `artifacts/qualification/bind-1792825860623375000.json`;
  corrected pass: `bind-1792825893112476800.json`.
- Resource operations: `artifacts/qualification/operations-1792825914904.json`.
- Interactive session: `artifacts/qualification/terminal-1792825976491.json`.
- Framework: `artifacts/qualification/testcontainers-c3b0018e-3ec2-446a-8d18-c184ffe7993a/result.json`.
- Preview: `artifacts/preview-verification.json`.

After testing, the native app and task-started preview were closed. Docker
answered with the same daemon ID after UI closure. The owned backend service
was then stopped after verifying its registry BasePath and that it had no
running containers. All five WSL registrations were stopped again, matching the
entry state. No existing project resources were removed. The guest-proof JSON's
`serviceLeftRunning` describes the end of that proof, before this cleanup.

## Roadmap exit gates

| Required gate | Assessment | Evidence still needed / implementation gap |
| --- | --- | --- |
| Install/start/use/restart | **Fail** | First-run download/provisioning remains a visible development limitation; bundle configuration has `active: false`. `engine.rs` resolves runtime assets relative to the compile-time source checkout and expects Docker Desktop's installed CLI path. Installed discovery, bundled tools and install/uninstall are unfinished. |
| Required clients, mounts and networking | **Fail** | CLI/Compose native Windows bind paths fail. Run-form F-drive mapping now passes; C-drive/Unicode runtime cases, representative user-WSL sources, integration/discovery, host-port conflicts and broader network cases remain unqualified. |
| Full relevant feature/flow matrix | **Open** | Builds provide submission rather than qualified retained history/cache management; WSL integration is a read-only list; settings/version/resource semantics and cluster flows remain incomplete/unqualified. BL-007 through BL-012 need their individual exit proofs. Explicit unavailable screens account for work but do not complete it. |
| Simultaneous engines and both start orders | **Open** | `runtime-identity.json` still has `coexistenceQualified: false`; no full qualification identity exists. Neither a dedicated pipe nor a successful isolated workload is a coexistence result. |
| Honest blocked-operation UI | **Partial pass** | Examined setup/reset/settings branches explain limits. Published-port opening, partial batch/Compose outcomes, stream/disconnect and all failure branches remain unqualified. Product-owned missing adapters must not be classified as upstream impossibilities. |
| Backend independent of UI | **Partial pass** | Closing the native app in this pass left Docker reachable with the same daemon ID. Repeat closure/reopen with supported active workloads and session teardown as part of final native acceptance. |
| Lifecycle and data preservation | **Partial pass** | Clean service resume, container restart persistence and scoped cleanup passed. Interrupted startup/update, bridge/engine loss, recovery, disk/resource pressure, sleep/resume and network transitions remain open. No global WSL reset was used. |
| Baselines, budgets and supported versions | **Open** | Existing single-sample measurements are not regression budgets. Record startup/idle/build/filesystem distributions, host/cache conditions and explicit supported versions; qualify against those budgets. |
| Quality-standard release gates | **Open** | Broader core failure/concurrency review, appropriate fresh race checks, full native interaction/a11y/scaling review, license/tool distribution audit and reproducible installed-product verification remain required. |

The lack of full qualification is not proof that every untested behavior is
broken. First-run provisioning, installed discovery and direct Windows binds
are concrete missing/failing product requirements. They independently prevent
acceptance even if all other unknowns later pass.

## Fix review and next work

The change to `mounts::windows_source` accepts only the verbatim **local drive**
prefix before the existing drive-letter test. UNC/device sources still fail;
canonicalization, existence/type checks and mount-option validation are retained.
It creates no files or directories, changes no WSL settings, adds no transport,
and retains Moby/rdd ownership. The existing filesystem regression now checks a
canonical source through both dispatch classification and bind conversion.
The real mount test initially reproduced the bug and passes after the fix.

Complete Phase 1 in this order, without silently reassigning its requirements
to Phase 2:

1. Resolve ordinary Windows CLI/Compose binds and selected-distro integration at
   a narrow upstream-compatible boundary; qualify paths and stream preservation.
2. Prove coexistence with maintained workloads in the other engines, both start
   orders and explicit port conflicts, retaining global client defaults.
3. Complete first-run/installed runtime discovery and the remaining required
   feature matrix; qualify native end-to-end and failure interactions.
4. Run lifecycle/data/fault, security/concurrency and measured-budget gates;
   finish tool/license packaging and clean install/use/restart/uninstall proofs.
5. Reassess against every gate, then finalize Phase 1 and begin Phase 2.

At assessment entry the repository had an unborn `master` branch, staged and
unstaged implementation, and no Git remote. A source checkpoint can preserve
this work, but must not be labelled a Phase 1 release. Push requires the user's
repository URL. The requested push-then-Phase-2 sequence remains pending until
its prerequisites are satisfied.
