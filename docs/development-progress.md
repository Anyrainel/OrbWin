# OrbWin development progress

Updated 2026-10-24, America/Los_Angeles. **Phase 1 accepted in its stated local
matrix; Phase 2 implementation started.** The
[final Phase 1 assessment](verification/phase-1-final-assessment.md) supersedes
the chronological open-status entries below. Phase 2's first milestone adds
Compose override files, profiles, quiet validation, reviewed multi-file discovery
and opt-in remembered inputs/working directories; see the
[milestone record](verification/phase-2-compose-2026-10-24.md) and
[module contract](architecture/compose-projects.md). Phase 2 is not complete.

## Historical Phase 1 progress

The following entries retain their original checkpoint status.
This record separates actual implementation/results from remaining scope. It
does not replace the development brief, decisions, roadmap or quality standard.

Latest status: [2026-10-24 readiness reassessment](verification/phase-1-readiness-2026-10-24.md).
Fresh headless/build, owned guest/service/build, resource, Run-form mount,
interactive-session and Testcontainers checks pass. The canonical Windows path
classification defect found by the mount runtime test is fixed. Direct Windows
CLI binds still fail. First-run/installed discovery, coexistence, full workflow
and failure qualification, budgets and packaging remain acceptance blockers.
The chronological entries below retain their original evidence boundaries;
older pending statements and test counts are superseded by the reassessment.
At the end of the reassessment the owned service was stopped after ownership
and no-running-container checks; all five registered distributions were stopped.
The older "backend running" description below is historical, not current state.

## Working artifacts

- Native Tauri/React application: `src-tauri/target/debug/orbwin.exe`.
- Guest-capable patched Windows backend: `artifacts/backend/rdd.exe`.
- Pinned source, Go/compiler packages, four replayable patches, source drift
  checks and build/guest binary hashes. See `upstream.lock.json`,
  `build-tools.lock.json`, `patches/rdd/README.md` and the generated build manifest.
- Real, explicitly targeted read-only container/image/volume/network inventory
  in the desktop. Identity must match a successful owned-guest proof and build;
  there is no default-context fallback or automatic backend startup.
- Native WSL version/distribution diagnostics, bounded process time/output,
  error/retry/unavailable/empty/filter states and a versioned design specification.
- Safe management-only and guest proof scripts, service/build baseline,
  mount-case probes and a separately gated full qualification harness.

The UI has **no container mutations, engine lifecycle controls, streaming shell,
or integrated Kubernetes operations yet**. Visible pending views are not completed
features or scope exclusions. Full accounting is in `phase-1-coverage.md`.

## Current owned runtime on this machine

The last successful development proof left its owned backend running so the
native app can inspect real inventory. Redis/PostgreSQL test projects and their
test volumes were removed using project-scoped cleanup; the three pulled images
remain. Docker Desktop and Ubuntu remained stopped during these proofs. No
simultaneous-engine runtime claim is made.

- Proof home: `F:\Codes\Orbwin\artifacts\gp-2ce2d6e3`.
- WSL registration: `lima-orbwin`, with BasePath verified beneath that proof home.
- Docker pipe: `npipe:////./pipe/docker-rancher-desktop-orbwin`.
- Recorded daemon ID: `1a2a3eb3-2f04-4fd6-a2d7-2f5e3fe13cb3`.
- Runtime identity: `artifacts/qualification/runtime-identity.json`.
- Latest raw proof: `artifacts/gp-2ce2d6e3/evidence.json`.
- Prior successful service baseline:
  `evidence-before-f6cdbb9b-bfd3-4c2d-b465-2b3e5efcc189.json` in that home.
- Windows path failure plus explicit WSL-path success:
  `evidence-before-a1913e5d-3362-4a51-86d8-b4557411f90f.json`.
- Guest, Lima, host-switch and rdd logs are retained under the same proof home.

To stop **only this task-owned backend**, use a process-scoped environment:

```powershell
$env:USERPROFILE = 'F:\Codes\Orbwin\artifacts\gp-2ce2d6e3'
$env:HOME = $env:USERPROFILE
$env:RDD_INSTANCE = 'orbwin'
& F:\Codes\Orbwin\artifacts\backend\rdd.exe service stop --timeout=60s
```

Run that in a disposable shell, since its environment is intentionally scoped to
the proof. Do not run service delete/unregister/reset, copy a writable disk,
start a second proof guest, or apply these values to another engine. The proof
script checks registration ownership before stopping; preserve that check when
automating continuation. For another baseline on this exact unchanged build:

```powershell
node scripts/prove-guest.mjs --resume=gp-2ce2d6e3 --suite
```

It normally stops the owned service afterward. `--keep-running` retains it for
development inspection. `--mounts` includes a currently failing native Windows
path case; the script records that failure and still stops its service. It is
not an acceptance pass. Rebuilds invalidate old proof identities and require
explicit requalification; do not edit identity records to force a pass.

## Verified results and limits

| Check | Result / evidence boundary |
| --- | --- |
| Source lock and four patches | Applied/replayed into a separate clean source checkout; normalized file hashes verified |
| Windows endpoint tests | Instance/socket/bridge tests, real independent listeners and occupied-pipe rejection pass |
| Go race checks | `pkg/instance` and `pkg/socketbridge` passed before namespace follow-up; repeat appropriate changed-package race checks before acceptance |
| Upstream host/app tests | Host-switch and app-controller tests pass; CLI product identity test passes |
| Build tooling tests | 12 Node tests pass: pin/patch/binary/identity rejection, paired host/guest constants, route overlap and bounded readiness behavior |
| Native adapter tests | 6 Rust tests pass: process timeout/output/failure/decoding and identity/inventory parsing; Clippy passes with warnings denied |
| Frontend/native build | TypeScript/Vite and unsigned native debug builds pass |
| Dev server | Fixed generated-directory watching after Windows EBUSY; preview remains HTTP 200 while Rust tests run. Harmless artifact probes return 403 through both normal and `/@fs/` URLs (`npm run verify:preview`). |
| Management-only backend | Fresh project-owned home, TLS/readiness/discovery, clean stop, global Docker/Kubernetes config hashes unchanged |
| Guest bootstrap | Patched WSL rootfs imports under owned registration; dedicated pipe serves Linux Moby; Alpine smoke passes |
| Service/build baseline | Redis + PostgreSQL readiness, exec, host Redis dynamic-port PING, data after container restart, logs, Compose, Buildx build/load/run, scoped cleanup pass |
| Backend restarts | Daemon ID preserved across observed clean restarts; readiness race discovered and proof adapted; not a reliability certification |
| UI lifetime | Closing the native app left the owned daemon reachable with the same ID/version; reopening connected again. No UI shutdown hook or backend restart was used. |
| Windows mount | Native drive syntax fails; explicit `/mnt/f/...` path with spaces succeeds, recorded as BL-005 |
| UI integration | Native connected overview and actual image inventory visually inspected; browser unavailable view inspected; screenshots under `verification/screenshots/`. Native accessibility text insertion changed the filter field without establishing a corresponding React filter update; keyboard/filter automation needs a reliable input route before acceptance. |
| Simultaneous engines | Not executed. Existing Docker Desktop/Rancher state was not started or altered to manufacture a result |
| Kubernetes / broad scope | Pending, including all other phase gates in coverage/scenario records |

Observed versions: Windows build 26300.9711, WSL 2.7.11.0, kernel reported by
Docker `6.18.33.2-microsoft-standard-WSL2`, Docker CLI 29.8.0, Moby
29.7.2-ce, Compose 5.5.1, Buildx 0.37.0. This is a development observation,
not a supported Windows/WSL release matrix. The requested App resource limits
(2 CPUs/2Gi) did not match Docker's reported 16 CPUs/~49.3 GB; see BL-006.

One successful baseline measured Compose up/readiness at ~14.4 seconds and a
small Buildx build at ~1.2 seconds. Sample count one, no competing workload
control, Alpine base already cached; Redis/PostgreSQL first-run pull/init effects
are included. These values are harness evidence only, not performance claims or
budgets. No comparative Docker Desktop run was performed.

Workload image references were subsequently pinned to the observed repository
digests (Alpine, Redis and PostgreSQL) in the smoke/Compose/build fixtures, so
future test runs do not silently use a moved image tag.

## Failures and corrective work

1. Initial management proof passed `--secure-port=0`, which the upstream API
   server rejects. Corrected to upstream's default/fallback selection; proof passes.
2. Upstream `rdd set --help` dynamically started the default management service
   in the real user profile. The exact newly started process was stopped; no guest
   was created. Files remain under `%LOCALAPPDATA%/rancher-desktop-2` and `.rd2`.
   Existing distro states and selected Docker context were verified unchanged.
   Patch 0004 now defaults to OrbWin, rejects foreign identities and forces
   product-owned client config before constructing commands. Do not delete the
   retained management files without an explicit ownership review.
3. Restart proof initially trusted a persisted App condition and saw missing
   pipe/EOF. Guest logs show provisioning stops/reconfigures the guest target
   after an early daemon response. Use the upstream current-boot Lima condition
   plus bounded actual endpoint/identity checks, not a blind delay or engine retry.
4. The first resumed proof overwrote the initial successful proof JSON before
   archival was added. Later attempts archive their previous evidence; raw logs
   and later successful service baselines remain. Do not imply all initial raw
   evidence was preserved.
5. Native Windows bind sources fail; explicit WSL drive mapping works. This is
   an unresolved compatibility gate, not a justified phase 1 exclusion.
6. Vite initially watched generated Rust executables and hit Windows EBUSY.
   Generated/backend directories are now excluded from watching and private
   artifact paths are denied by the preview file server. Liveness and harmless
   file-denial checks pass while Rust tests execute.

## Next implementation sequence

1. Review runtime-owned isolation beyond the exercised case: named-pipe ACLs,
   registry/WSL ownership, host-port conflicts, other active engines, shared WSL
   resource effects and integration sockets. Complete both start orders without
   changing/stopping the user's existing environments for convenience.
2. Resolve Windows native path handling at a narrow upstream-compatible boundary;
   qualify files/directories/read-only/Compose/Unicode, other drives and WSL sources.
3. Complete real container/project actions, image/build, volume/network operations,
   logs/stats/interactive exec, diagnostics/recovery and optional Kubernetes UI.
   Expand reusable component/state contracts and rendered/keyboard coverage.
4. Qualify Testcontainers/helper discovery, streaming/session semantics,
   repeated/concurrent suites, cancellation, failures and data preservation.
5. Finish guest/tool distribution, installers, supported-version matrix,
   provenance/license inventory and the complete roadmap gates. Phase 1 scope
   remains intact; later phases have not begun.

## 2026-10-22 UI correction

The invented dashboard was replaced by grouped resource navigation, a compact
master list and adjacent tabbed inspector, grounded in official product-hero and
Kubernetes screenshots. See design-system.md for source URLs and the separation
between observed pixels and original Windows interactions. Native Windows
window decorations remain enabled. Containers is the initial destination.

Real inventory supports selection/detail fields, search, Compose-label grouping,
keyboard row movement and inspector-tab navigation. Builds and separate cluster
Pods/Services entries make the remaining integrations explicit. The backend and
its lifecycle/isolation adapter were not changed. This corrects UI structure;
it does not complete phase 1 or qualify coexistence, mutations or streaming.

Validation and screenshots: verification/ui-correction-2026-10-22.md. All prior
staged work was preserved; the independent research directory was read only.

## Active ordinary-workflow implementation — 2026-10-22

Supersedes earlier statements that the desktop exposes only read operations.
Phase 1 remains in progress, with substantial qualification and integration work.

- Added typed operations with cancellable bounded clients and actual progress:
  container run/start/stop/restart/pause/unpause/remove/exec, full inspect,
  image pull/tag/untag/remove, volume/network create/remove, network attachment,
  log follow, stats snapshots/polling, Buildx build/load and Compose lifecycle.
- Container creation includes explicit localhost port publishing, environment
  entries and named-volume attachments. Windows bind mounts are still BL-005.
- Dialogs show consequences, failures and uncertain cancellation outcomes. No
  forced container removal or implicit volume deletion accompanies removal.
  The native UI created `orbwin-ui-volume-proof` and refreshed real inventory;
  that disposable test volume remains until scoped cleanup.
- Added rdd start/stop/configuration and cluster enable/disable requests, guarded
  by actual binary hash and registry ownership checks. A real owned stop/start
  cycle passed and retained the daemon ID (`management-*.json`).
- Added cluster resource reads, logs, manifest application, readiness waits and
  UID-preconditioned deletion; Pods/Services/Deployments/storage-claim views.
  The corrected Redis fixture passed deployment/claim readiness, resource reads,
  logs, wrong-UID deletion rejection, scoped cleanup and restoration of the
  original disabled cluster state. Pod exec/forwarding remain unqualified.
- Resource adapter proof passed (including in-use removal rejection, exec exit
  status, log-follow cancellation and cleanup). Build/Compose adapter proof also
  passed, including image tagging and volume data after restart. Raw results:
  `artifacts/qualification/operations-*.json`, `build-compose-*.json`.
- Added native file pickers/output export and an xterm/Go SDK interactive adapter.
  Interactive SDK qualification passed input, resizing, Ctrl+C, exit status and
  output backpressure/disconnect with conflicting inherited Docker selectors.
  The initial host-ConPTY route
  failed early-input tests and was removed with its experimental dependency
  patch. The retained helper attaches directly to the Linux TTY through the
  pinned Moby SDK. Final native visual/keyboard acceptance remains open.

The user subsequently deferred all real-computer verification until explicit
confirmation. Headless continuation added a Windows drive-path mount adapter for
the Run form and an ignored runtime proof, fixed queued cluster reconciliation
and preserved empty command arguments. Parser tests and static checks passed;
Windows mount runtime qualification and general CLI/Compose translation remain
open. See verification/workflows-2026-10-22.md for current evidence and boundaries.

The latest source is ahead of the last visually exercised native executable.
Rebuild and repeat native visual/keyboard checks after the active changes settle.
Full coexistence, Windows mounts, selected-distro integration, first-run setup,
fault/recovery coverage, supported matrix and installer remain open. Automatic
updates are explicitly deferred by the user. Preserve prior staged work and the
independent research owner's directory.
