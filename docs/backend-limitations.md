# Backend limitations

Updated 2026-10-24 Pacific. Entries distinguish source findings from observed
runtime proofs. Source-only findings do not establish runtime behavior.
Initial findings below are retained for provenance. The dated implementation
updates and `verification/phase-1-completion.md` supersede historical statements
that a subsequently implemented flow is absent.

## 2026-10-24 implementation updates

- BL-015: native Kubernetes forwarding exposed a missing guest `socat` runtime
  dependency. The local listener opens but forwarding the first request fails.
  The UI retains the concrete error and removes the dead link. Signed openSUSE
  Leap 16.0 socat/libwrap RPMs passed signature and dependency checks and were
  installed only in the owned guest for a dependency proof. Patch 0011 now
  bundles the pinned binary/source RPMs and licenses; automatic existing-guest
  repair passed after removing the manual packages. Fresh provisioning and the
  complete native forwarding journey remain in progress.

- BL-003: pinned runtime clients are bundled; native first-run cancel/setup/resume
  passed on a fresh sequential proof guest. Installed-product checks remain open.
- BL-005: general Windows Docker CLI/Compose bind translation now passes C:/F:
  directory/file, spaces/Unicode, read-only, host-visible writes, local bind
  volumes and restart checks. UNC/device/relative paths fail explicitly.
- BL-010: real interactive pod exec passed against the isolated workload cluster.
  Followed logs and persistent forwards are implemented; native qualification is
  still required before closing this entry.
- BL-011: the WSL page now has explicit selected-project integration, scoped Linux
  client guidance and disconnect. Native Ubuntu read/write/Compose/build/restart/
  replaced-source/data-preservation proofs pass. Shared project paths require a
  live user WSL session and reconnect after distribution exit. Both public Unix
  sockets now use the authenticated Windows adapter relay; a shared bind of the
  dockerd socket did not survive user-distro exit.
  Transparent `/home` path mapping remains a documented later convenience.
- BL-012: actual first-run and resource/disk/history reads now exist. Broader native
  settings, apply/restart, conflict and installed qualification remain open.
- BL-013: Docker Desktop 4.91.0 subsequently started with Rancher returned to its
  initial stopped state and its remaining zero-length stale IPC directory retained
  under a sibling backup. Its dedicated daemon passed the OrbWin coexistence pair
  proof (`orbwin-coexist-fb674c82`). Rancher then also started and both dedicated
  endpoints remained healthy. Three-engine maintained traffic, identity/data
  retention, explicit foreign-port conflicts and owned-engine restart passed
  `orbwin-coexist-ad3ae9dc` on binary `29073605…`.
- BL-014: Windows allocation, stable bindings, explicit conflict refusal and
  publication status now pass scoped tests. Repeated Testcontainers found a second
  stale-publication acknowledgement race after restart. The adapter now requires
  a fresh observed listener generation after start/restart. Four consecutive
  Testcontainers suites, concurrent Windows/WSL/delegated publication and the
  three-engine proof passed on binary `29073605…`; see the active completion
  record for exact evidence IDs. This closes these tested cases. Failed evidence
  remains retained; broader compatibility and Phase 1 gates remain separate.

## BL-015 — Kubernetes forwarding lacks the guest socat dependency

- Flow: native pod/service localhost forwarding with the pinned Rancher backend,
  openSUSE guest 0.2.11 and Kubernetes 1.35.3; observed on `e3892a1f…`.
- Reproducer: forward port 80 of the scoped Nginx pod, then request its local
  URL. kubectl first reports `Forwarding from 127.0.0.1`, then reports
  `unable to do port forwarding: socat not found` and exits on a connection.
- Impact/adaptation: the listener's readiness does not establish the complete
  stream works. The UI reports exit, retains bounded connection output and
  removes the link; the workload remains running. This is not a qualified flow.
- Dependency record: the base image lacks socat and libwrap. The official
  Leap 16.0 signed RPMs passed `rpm --checksig` and an offline dependency test.
  Manual installation in the owned guest is a proof step, not the product fix.
- Subsequent evidence: bundled automatic repair passed `bbbabad2…`; installed
  files verify cleanly and the retained PVC file survives. Native HTTP through
  the manually repaired guest passed on 60722, including navigation and explicit
  stop. Current-bundle and fresh-guest acceptance remain separate requirements.
- Owner/target: OrbWin guest packaging/provisioning, Phase 1. No upstream issue
  has been filed. Track the narrow adaptation and source/license provenance.
- Acceptance: fresh and existing guests obtain hash-pinned dependencies without
  a guest-network package download. Native pod/service forwards must carry HTTP
  traffic, survive navigation, report interruption and stop without deleting
  workloads; preserve scoped regression evidence.

## BL-013 — Existing Docker Desktop cannot reach engine readiness

- Flow: simultaneous-engine qualification on this host. Installed version 4.91.0,
  Windows 26300.9711 and WSL 2.7.11.
- Evidence: 2026-10-24 local startup logs first report inaccessible old AF_UNIX
  IPC socket files, then the backend exits with a nil-pointer panic in
  `services.(*Services).startDockerAPIProxy`. No Docker Desktop daemon responded
  on its dedicated Linux-engine pipe. Rancher's separate daemon did respond.
- Impact: required coexistence acceptance is still open; this is an observed
  external startup failure, not proof of an OrbWin coexistence pass or defect.
- Recovery attempted: retained only verified zero-length IPC directories under
  timestamped sibling names and retried startup. No reset, disk deletion, secret
  upload or global WSL shutdown occurred. Evidence stays in ignored artifacts.
- Owner/target: Phase 1 qualification. Resolve the startup precondition or obtain
  an appropriate supported test environment, then run maintained service workloads
  in both OrbWin start orders. Do not silently waive the gate.

## BL-014 — Published automatic port can redirect after an engine restart

- Flow: Windows/other-engine published TCP service access, same pinned backend.
- Reproducer: `ORBWIN_QUALIFY_OWNED=1 node scripts/qualify-coexistence.mjs --rancher`.
  Real Redis workloads on OrbWin and Rancher initially answered distinct sentinel
  values. Restarting only OrbWin preserved its daemon and data but reassigned its
  automatic port to 32768, already used by Rancher. A Windows request read the
  Rancher sentinel. See the `f46626a9` result in ignored coexistence artifacts.
- Impact: Docker's running state does not establish host publication success;
  exposed links can reach another engine. This invalidates Phase 1 acceptance.
- Required adaptation: choose available Windows ports for automatic requests,
  retain explicit bindings over restart, validate host publication, and return a
  clear conflict rather than silently redirecting. Qualify explicit conflicts,
  repeated/concurrent clients, both start orders and restart preservation.
- Owner/target: OrbWin's narrow Windows Docker/network adapter, Phase 1.

## BL-001 — Shared Windows Docker endpoint

- Flow: simultaneous engines, Docker discovery. Baseline: `af6511ad4e065e24e7258f575a0fc090c10cdbda`.
- Source reproducer: `rdd/pkg/instance/instance.go` and `pkg/socketbridge/host_windows.go`
  upstream select `docker_engine` regardless of instance suffix.
- Impact: context selection alone cannot prevent listener collision or wrong-engine routing.
- Adaptation: patch 0001 gives each instance a pipe used by both discovery and bridge.
  UI always targets the OrbWin endpoint explicitly. No global context switch.
- Owner: OrbWin host integration. Phase 1. No upstream issue filed yet.
- Exit: simultaneous real Docker/Compose/Buildx and helper-container tests pass in
  both engine start orders. Current tests cover identity and real isolated pipe allocation only.

Update: isolated Docker CLI traffic and the service/build baseline now pass on
the dedicated pipe. The simultaneous/implicit-client portion of the exit remains
unqualified.

## BL-002 — WSL host-switch guest identity and fixed ports

- Flow: simultaneous Rancher 2.0 guests. Same baseline as BL-001.
- Reproducer: `pkg/hostswitch/hostswitch_windows.go:getVMGUID` explicitly assumes
  one matching openSUSE instance. Ports 6670/6657 and signature discovery are
  shared with guest networking; `src/networking` must be adapted consistently.
- Impact: selecting a distinct pipe cannot establish the guest behind it is ours.
  WSL shared-VM behavior needs an actual scoped proof; do not boot unmodified guests.
- Adaptation: backend start is unavailable until the guest identity contract and
  host/guest ports are isolated and qualified. Diagnostics explain the reason.
- Candidate patch 0002 now separates the product discovery signature and four
  vsock ports, veth link names and WSL bridge address. The proxy uses the same
  changed address. Paired source contract checks and host tests are required;
  these are not runtime proof. The product supports one owned guest only.
- Additional source evidence: `src/networking/cmd/network/setup_linux.go`
  unconditionally cleans up `veth-rd-wsl` in the originating WSL namespace;
  sharing the name risks altering another engine's integration. The candidate
  uses a separate name and subnet, whose route conflicts still need testing.
- Pinned Lima dependency `v2.3.0-beta.1.0.20261005214905-0ae2df64800a`
  registers, starts, stops and deletes `lima-` + VM name in
  `pkg/driver/wsl2/wsl_driver_windows.go`. The stock App names its VM `rd`;
  a separate LIMA_HOME does not separate this global WSL registration. Patch
  0003 changes the App VM name to `orbwin` (`lima-orbwin`). Existing app-controller
  tests pass. No registration/boot/delete operation has been run.
- Owner: OrbWin upstream adaptation. Phase 1. No upstream issue filed yet.
- Exit: identity tied to a specific owned guest, wrong-guest rejection, collision
  tests, restart order and live simultaneous workload traffic pass.

## BL-003 — Runtime build and packaging qualification pending

- Flow: install/start, all workload operations. Same baseline.
- Evidence: this source pin requires Go 1.27, generated Linux Lima guest agent,
  overlaid WSL distro and native SQLite build. Original host inventory had no Go
  on PATH; project-local verified Go 1.27.3 now runs the focused patch tests.
- Impact: no qualified OrbWin backend bundle is available yet; do not fall back
  to a competing daemon or present an empty inventory as a successful connection.
- Adaptation: explicit unavailable state, retry and build guidance in diagnostics.
- Owner: OrbWin build. Phase 1. Exit: reproducible native bundle plus real API,
  workload, preservation and coexistence qualification. Not a phase exclusion.

Update: the native management build, all six guest binaries, rootfs overlay and
guest-capable bundle now build. Management-only and isolated guest/Linux smoke
proofs passed, as did a Redis/PostgreSQL/Compose/Buildx baseline. This entry now
covers remaining installer/release and full runtime qualification, not a missing
development executable. Raw evidence is under the task-owned artifacts directory.

## BL-004 — Persisted readiness can precede current guest provisioning

- Flow: restart/readiness. Same pinned source plus OrbWin patches.
- Observed reproducer: cleanly stop the task-owned service, resume it, wait only
  on App `ContainerEngineReady=True`, immediately run Docker info/version.
  A stored condition returned before the pipe existed. Another run briefly
  answered info, then version returned EOF while provisioning stopped the guest
  target; the guest log records the Docker socket temporarily absent.
- Impact: workload startup can race provisioning; this is not established as a
  spontaneous engine crash. Preserve raw logs and classify correctly.
- Adaptation: isolated restart proof uses upstream `ctl wait-condition` on the
  owned LimaVM `Running=True`, reason `Started`, `--since=startup`, followed by
  bounded actual endpoint/identity checks. Native inventory probes identity and
  never starts/restarts the service. UI lifecycle integration remains pending.
- Owner: OrbWin lifecycle adapter/upstream review, phase 1. No issue filed.
- Exit: repeated cold/warm starts, interrupted starts, healthy reattachment and
  failed-start behavior pass without stale readiness or unintended restarts.

## BL-005 — Native Windows bind source syntax is not translated

- Flow: ordinary Windows Docker CLI `--mount`; same source/patches and WSL 2.7.11.
- Observed: `src=F:\Codes\Orbwin\tests\workloads\bind source` is rejected by
  the Linux daemon as a non-absolute mount path. The equivalent explicit
  `/mnt/f/Codes/Orbwin/tests/workloads/bind source` read-only directory mount
  returns the expected sentinel file.
- Impact: the full Windows-path client journey does not yet pass. Non-system
  drive access exists through explicit WSL drive paths, including spaces.
- Adaptation: the Run form now accepts existing local Windows drive paths,
  canonicalizes them and maps them to WSL `/mnt/<drive>` paths. Parser tests
  passed; the separate runtime proof is deferred until host availability is
  confirmed. General CLI/Compose traffic is not translated. This does not
  qualify ordinary Windows CLI/Compose paths or hide the missing adaptation.
- Owner: OrbWin Windows integration/upstream boundary, phase 1. No issue filed.
- Exit: native Windows drive/file/directory/read-only/Compose forms pass with
  unknown Docker fields and streaming/session semantics preserved. No broad
  Docker API rewrite is selected yet.

2026-10-24 observed update: the Run adapter's live mount test initially rejected
canonical `\\?\F:\...` sources as named volumes. Local-drive classification is
fixed with a regression check. The rerun passed directory reads with spaces,
file writes visible on Windows, read-only rejection, container-restart
preservation and scoped cleanup (`bind-1792825893112476800.json`). A separately
targeted Windows Docker CLI `--mount` still fails with a non-absolute source
error. This qualifies only typed Run requests on the exercised F-drive paths;
it does not close BL-005. See the readiness reassessment for commands and limits.

## BL-006 — Requested guest resource allocation is not an observed WSL limit

- Flow: CPU/memory settings. Proof App requested 2 CPUs and 2Gi; Docker info
  reported 16 CPUs and 49,257,504,768 bytes available to the WSL environment.
- Impact: do not claim these App settings enforce independent guest allocation.
  WSL shares platform resources; exact enforcement needs upstream/WSL audit.
- Adaptation: resource editing stays unavailable, with scope to be explained
  before enabling it. Do not change global `.wslconfig` on the user's behalf.
- Owner: OrbWin settings/WSL integration, phase 1. No issue filed.
- Exit: resource scope and restart implications are documented and tested; UI
  distinguishes any global WSL setting from a product-only setting.

## BL-007 — Cleanup, disk usage and reset have no backend operation

Labels in BL-007 to BL-012: **source-only** means read from OrbWin code or
upstream source; **observed** means seen in a running system; **unverified**
means unknown. Nothing in these entries has been run against a live engine.

- Flow: prune, storage overview, volume/virtual-disk size, reset.
- Evidence: source-only. No prune or `docker system df` adapter is wired;
  volume and virtual-disk sizes are unmeasured. BL-002 records that no reset
  operation exists. Whether the Docker prune/df endpoints behave correctly on
  this engine is unverified.
- Impact: OrbStack-style reclaim-space and reset flows cannot be offered.
- UI adaptation: cleanup is list-and-remove-selected over images, volumes and
  networks; sizes are not shown rather than estimated; restart is stop + start.
- Owner: OrbWin adapter, phase 1. No upstream issue filed.
- Exit: df/prune adapter qualified with preservation tests; sizes measured on a
  live engine; a defined reset scope that cannot touch user distributions.

## BL-008 — Batch and group operations are sequential without rollback

- Flow: multi-select batch actions, Compose group actions.
- Evidence: source-only. Batch actions run one resource at a time; a failure
  mid-batch leaves earlier operations applied. Group remove leaves project
  networks behind. Project logs/down are offered only when exactly one Windows
  compose file label is present. Image/volume "in use" markers rely on the
  `containers` prop now passed from App; unverified against live data.
- Impact: partial completion is possible; leftover networks may accumulate.
- UI adaptation: per-item results, no atomicity claim, no rollback.
- Owner: OrbWin UI adapter, phase 1. No upstream issue filed.
- Exit: live multi-select and Compose group runs, including mid-batch failure
  and cancellation, with leftovers cleaned or explained.

## BL-009 — Port links may not open from the native window

- Flow: open a published port from a row or Info tab.
- Evidence: source-only. Links use `target=_blank`; the Tauri app has no opener
  plugin, so native-window behavior is unverified.
- Impact: links may do nothing or open an unexpected window.
- UI adaptation: none yet beyond showing the address.
- Owner: OrbWin Tauri integration, phase 1.
- Exit: verify in the native app; add an opener capability if links fail.

## BL-010 — Kubernetes views have bounded depth

- Flow: pod inspection, terminal, port-forward.
- Evidence: source-only. List and inspector (Info/Logs/YAML/Events) exist. Pod
  Terminal is a single non-interactive `clusterExec`. Port-forward lives only
  while its dialog is open, so no persistent service link exists. Logs are the
  last 500 lines without follow. Namespaces with no resources of the viewed
  kind are not listed. Not run against a live cluster.
- Impact: no persistent service access comparable to OS-008/010.
- UI adaptation: limits shown; nothing simulated.
- Owner: OrbWin adapter, phase 1.
- Exit: interactive exec, followed logs and persistent forwards qualified, or
  limits accepted explicitly with rationale.

## BL-011 — WSL page is a name-based read-only list

- Flow: WSL distributions page, open terminal.
- Evidence: source-only. The list is parsed from `wsl --list --verbose`; roles
  are inferred by name only. The Rust `wsl_open_terminal` command compiled but
  is untested; it rejects engine distros.
- Impact: roles may be misclassified; terminal launch is unproven.
- UI adaptation: read-only list, no mutation controls.
- Owner: OrbWin WSL integration, phase 1.
- Exit: reliable owned/engine identification; terminal launch tested; user
  distribution independence verified.

## BL-012 — Settings and first-run behavior unqualified

- Flow: settings changes, first run, port conflicts.
- Evidence: source-only/unverified. Which settings need restart is unknown; the
  restart banner reflects only rdd `Settled`. First-run provisioning is not
  automated (BL-003). Host-port conflict handling is unqualified. The forced
  light theme does not change the Windows title bar.
- Impact: a setting needing restart may apply without a prompt.
- UI adaptation: banner states only what rdd reports.
- Owner: OrbWin settings/lifecycle, phase 1.
- Exit: per-setting apply/restart behavior documented and tested; conflicts
  exercised.

## Historical UI adapter gaps — 2026-10-22 correction

At the time of the initial correction, the list/inspector UI exposed Builds, Pods and Services explicitly.
Their histories/cluster reads and setup remain unwired. Info displays only fields
returned by the existing read-only inventory, not full Docker inspect output.
Stats, Logs, Terminal and Files tabs explain missing adapters; no stream, exec,
filesystem, mutation or build operation is simulated. Compose groups derive only
from returned project labels and have not been exercised against a live project
in this correction. This is implementation debt in full phase-1 scope, rather
than evidence that rdd cannot support these operations. Reproduce by opening the
native app and selecting the relevant destination/tab. Existing BL-001 through
BL-006 and their exit criteria remain applicable.

The later ordinary-workflow implementation supersedes this historical UI-only
description: actual mutations, full inspect, builds/Compose, logs/stats,
interactive terminals and workload-cluster adapters now exist. See
verification/workflows-2026-10-22.md for successful runtime evidence and pending
native/host qualification. Files, first-run and other coverage gaps remain in
phase-1-coverage.md; they are not automatically upstream backend limitations.
