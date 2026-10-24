# Phase 1 completion work

Active from 2026-10-24. The earlier readiness report is an assessment checkpoint,
not authorization to stop before completing Phase 1. Phase 2 begins after the
Phase 1 gates pass. GitHub: https://github.com/Anyrainel/OrbWin (private).

## Windows mount and transport work

- Patch 0005 adapts the existing rdd Windows Docker bridge. Only create-request
  bind sources/local bind-volume devices are rewritten. It uses the standard Go
  HTTP proxy, preserves unknown JSON fields/large integers and unchanged bodies,
  and retains streaming/upgrade traffic. The owner/System ACL scopes the pipe.
- Unit/HTTP/race tests cover bounds, malformed input, build bodies, early upgrade
  bytes, half-close response draining and active-session shutdown. These caught
  and corrected extended UNC acceptance and a buffered-read/context cancellation
  interaction before runtime qualification.
- Clean five-patch replay and repeat preparation passed. Preparation now checks
  the final locked state instead of trying to reverse overlapping patches
  independently; it refuses incomplete/modified dirty stacks without resetting.
- `prove-guest --resume=gp-2ce2d6e3 --upgrade --suite --mounts --keep-running`
  passed on binary `82aa85bb5137ce97ec2a84173b7bd740d7faf935ffac37cf487afee03109a497`.
  This explicit upgrade preserves/archives the prior build evidence and requires
  the original daemon ID after startup; it does not force an identity pass.
- `artifacts/qualification/orbwin-mounts-3c254709/result.json`: C:/F: paths,
  spaces, Unicode, directory/file/read-only/write-visible cases, both Docker bind
  forms, local bind volumes, Compose and restart preservation passed.
- Node Testcontainers normal/concurrent/assertion/cancellation/Ryuk cleanup
  passed through the changed bridge:
  `testcontainers-b4fe3433-5fa9-4da1-88e4-8c1e2725664b/result.json`.
- The SDK interactive terminal proof passed through the changed bridge.
- Bundled-client native Buildx/Compose adapter proof passed, including data after
  restart and scoped cleanup: `build-compose-1792827959144.json`.

## Installed runtime and onboarding work

- Six Windows/Linux CLI/Compose/Buildx downloads match the exact version/hash
  pins inherited from upstream. `runtime-tools.lock.json` and
  `scripts/stage-runtime-tools.mjs` make staging repeatable.
- Native runtime discovery now distinguishes packaged resources from debug
  source artifacts. Docker Desktop's installed CLI path is no longer used by
  the desktop adapter. Plugins are pinned and configured only in the private
  client config; other keys survive atomic replacement.
- Explicit first-run setup and resume call upstream service/App operations;
  ownership intent precedes dispatch, actual readiness/registry/daemon checks
  precede atomic identity publication. A Windows file handle serializes setup
  across app processes. Cancellation preserves partial state and logs.
- The shared operation runner now also covers blocked stdin with its deadline;
  the regression test passes. It still kills only clients, not rdd/workloads.
- Patch 0006 reserves `orbwin-proof` for sequential clean provisioning tests,
  using a different guest registration and pipe. It does not change the normal
  product guest name. Tests must refuse a running normal product guest.
- Clean setup/cancel/resume runtime qualification passed, using the actual native
  setup implementation. Cancellation before management startup retained intent;
  Resume provisioned a distinct Linux daemon, and a second setup preserved its
  ID. The proof service was stopped with its disks/logs retained. Its first run
  exposed the upstream certificate routine's rejection of verbatim paths with
  mixed slashes; the CLI boundary now normalizes local-drive paths. Test data
  and failure logs remain in `artifacts/setup-proof`; the successful rerun is in
  `result.json`. This is adapter runtime evidence; native first-run UI/installer
  acceptance is still separate.

## Remaining gate work

### Continued implementation and proofs

- Pod terminals now use the pinned client-go transport through the existing
  bounded session helper. The proof guest passed real TTY input, resize, Ctrl-C,
  remote exit status, stale UID rejection and close-without-stopping-pod checks
  (`artifacts/setup-proof/pod-terminal.json`). Credential plugins, foreign or
  insecure endpoints and inherited HTTP proxies are rejected. Native pod UI
  review remains pending.
- Pod log follow has bounded output, pause, search/copy/export and scroll anchoring.
  Port forwards now persist across navigation in a bounded status panel, with
  explicit disconnect and links only after kubectl reports readiness. Native
  cluster/forward qualification remains pending.
- Transient clients and session helpers now hold Windows kill-on-close jobs;
  lifecycle/setup commands remain outside them. The test verifies terminating
  both a root client and its descendant when the job handle closes. A brief
  process-spawn/assignment interval remains; no claim is made that testing proves
  orphan absence at every instruction boundary.
- Patch 0007 adds a scoped Unix listener to the existing dockerd. Two rejected
  socket-bind implementations exposed a Docker inode replacement race and WSL's
  removal of shared bind mounts when Ubuntu exits. The direct listener passed
  the idle-distribution reproducer. It does not introduce a daemon or TCP API.
- Native `owned_wsl_project` passed on Ubuntu with the pinned Linux tools:
  non-root Docker access, Linux bind read/write, Compose health/up/down, Buildx
  build/run, Docker service restart, repeated connect, replaced-directory
  rejection, old-export disconnect and source preservation. The supported
  explicit project flow keeps a WSL shell open; shared project bind paths are
  displayed and must be reconnected after distribution exit. Transparent path
  mapping is a later convenience. See architecture/wsl-integration.md.
- Private client configuration now preserves explicit auth/helper settings and
  suppresses implicit helper discovery for anonymous pulls. Coexistence testing
  found an unrelated inherited Linux secretservice helper with missing libraries;
  the application must not depend on such a helper for public images.
- Clean seven-patch replay passed. The current guest-capable binary is
  `675c9ec413b94e50b33fb30d5bf95442f7214530b7c7b7657de81c0bd2960ebf`.
  Explicit main-guest upgrade plus Windows service/build/mount suite passed and
  preserved daemon ID `1a2a3eb3-2f04-4fd6-a2d7-2f5e3fe13cb3`. Upgrade proofs now
  stop/start the owned service to ensure the selected binary is actually loaded.
- Native UI review has exercised real BuildKit history selection/details and the
  WSL distribution/project view. It exposed stale explanatory text, now corrected.
  This is partial native review, not full interaction/accessibility acceptance.

Coexistence qualification is active. Installed Rancher Desktop 1.24.0 has started
beside OrbWin with a distinct daemon. Docker Desktop 4.91.0 fails before its
engine starts: initially stale AF_UNIX socket rename failures, then a nil-pointer
panic in `startDockerAPIProxy`. Zero-length IPC directories were preserved under
their original LocalAppData parents with `-stale-20261024` suffixes; no engine
disks, secrets database or workload data were reset or removed. Logs/evidence
are private under `artifacts/coexistence`. The product coexistence gate is not
passed by these startup observations.

The corrected Redis coexistence proof established distinct simultaneous daemons,
working TCP workloads and isolated labeled resource inventories. Its OrbWin
restart step then caught a real acceptance failure: Docker reassigned an automatic
port to 32768, already used by the Rancher workload, and the Windows request read
Rancher's sentinel instead of OrbWin's. Both owned proof resources were removed
by exact name; other workloads were untouched. Evidence:
`artifacts/coexistence/orbwin-coexist-f46626a9/result.json`. Next work must prevent
silent host-port misrouting, preserve an explicit host port across owned restarts,
and surface failed publication. This gate remains open.

Build history now reads BuildKit's retained records/details/steps rather than
inventing an app history. The pinned Buildx requires the leaf ID when inspecting
its listed default/default/id references; the adapter isolates that detail.
The proof guest passed build/list/inspect/logs/disk-usage/cleanup checks
(`artifacts/setup-proof/history.json`). Native UI validation remains pending.
Storage now has measured Docker aggregate usage and a verified owned VHDX file
length read; resource settings show Docker's observed CPU/memory separately from
the upstream requested values. These new native views still need runtime/UI QA.

Complete onboarding/runtime qualification; WSL integration; coexistence with
maintained workloads and both start orders/port conflicts; outstanding build,
storage/settings and cluster flows; core failure/data preservation and native
interaction/accessibility review; measured budgets; then packaging and installed
product checks. No gate is marked passed solely because code compiles.

## Windows publication correction — subsequent runtime evidence

Patch 0008 now sends Windows pipe, shared WSL Unix, and explicitly delegated
guest Docker-socket clients through the same adapter. Automatic ports are
selected on Windows and persisted explicitly in Moby; bounded FD reservations
avoid concurrent duplicate allocations. Start/restart verifies actual owned
gvisor publication. Native links fail closed when publication is unverified.
The private-network client entry requires per-home mutual TLS; anonymous and
foreign-certificate rejection have unit coverage. No Windows public TCP Docker
endpoint or OS trust-store change is introduced.

Existing WSL disks retain original overlay executables. The live upgrade caught
this: new provisioning with the old guest bridge could not work. The bundle now
embeds the matching guest bridge separately, stages it by content hash, verifies
the staged copy, and atomically replaces only that executable in owned guest
provisioning. Unknown/corrupt staged code is preserved and rejected. Concurrent
Windows staging passed a race test after serializing same-process publishers.
The inherited Docker socket activation also competed with the public relay;
the owned guest now masks that socket unit. No engine disk is reimported.

- Current guest-capable binary:
  `8758635eb07e9aa98a6a24df061987d28de1708434a38755edd3bf9b2e5220f7`.
- Owned upgrade, service/build/mount suite, and both public Unix socket identities
  passed; original daemon `1a2a3eb3-2f04-4fd6-a2d7-2f5e3fe13cb3` retained.
  Failed upgrade attempts remain archived. Explicit `--recover-from` selects a
  successful archived identity anchor without replacing a failed proof record.
- `artifacts/coexistence/orbwin-coexist-9dd64ae2/result.json`: simultaneous
  Rancher 1.24 and OrbWin Redis TCP/data/inventory isolation passed. Restarting
  only OrbWin retained its Windows port, both IDs, Rancher's workload start time,
  sentinel data, and global configuration hashes. This supersedes the failed
  Rancher pair result above, not the pending Docker Desktop/order matrix.
- `artifacts/qualification/orbwin-publish-42b3e117/result.json`: occupied Windows
  port rejected before container start, foreign listener preserved; six concurrent
  Windows/WSL clients plus one delegated-socket client received distinct working
  ports. Publish-all and each container restart retained ports and Redis responses.
- Native `owned_wsl_project` passed on patch 8: non-root Ubuntu read/write,
  Compose, Buildx, Docker-service restart, repeated connection, source replacement
  refusal, scoped disconnect, source-data preservation and idle-distro endpoint.
- A framework run overlapped the WSL test's explicit Docker restart and failed
  TCP readiness. It is retained as an invalid overlapping qualification run at
  `artifacts/qualification/testcontainers-464821b2-8de8-4144-a456-b0f61c3ae50e`;
  a sequential rerun is required. It must not be silently counted as a pass.

Native port links now call Windows' browser opener through a numeric loopback
port-only command. No arbitrary renderer-provided URL, scheme, command or path is
accepted. Frontend build and Rust Clippy passed; live opening remains to verify.

Further repeated framework checks reproduced a genuine restart publication gap
without the earlier overlapping lifecycle test. Failed runs `7d82b215`,
`db8ca250` and `3c3d7836` remain in ignored qualification artifacts. Redis retained
its port and answered `redis-cli ping` inside the guest, while Windows timed out
or refused the connection. Removing Lima's duplicate SSH forwarder was useful
but insufficient: the adapter could still acknowledge an old gvisor listener
before queued stop/start events removed/recreated it. Patch 8 now observes
successful expose/unexpose generations at the existing host-switch API boundary
and requires a newer actual publication before acknowledging restart. The
observation map is ephemeral, contains no container lifecycle authority, and is
updated only after successful forwarder operations. Race tests cover failed
exposure, concurrent observation and stale-generation refusal.

The latest guest-capable binary is
`29073605fa6ba7fa2352ee892e01e6ec29742c0865caff942db4b847cfaee5f9`.
Its owned upgrade/service/build/mount/Unix relay identity proof passed. A clean
eight-patch replay also passed. Testcontainers run `36906c23` passed normal
concurrent Redis/PostgreSQL/restart/log/exec cases, assertion failure and process
cancellation with Ryuk cleanup. More sequential repetitions are running.

Docker Desktop's startup precondition is now resolved for this test session.
The remaining zero-length stale IPC directory was retained as
`%LOCALAPPDATA%/Docker/run-before-order-retry-20261024` while its failed test-started
processes were stopped. No disk, secret store, registry credentials or workload
data were reset. With Rancher restored to its initial stopped state, Docker
Desktop started with daemon `a307ff0e-581d-49d3-8aee-a0636dda9137`. The maintained
Redis coexistence pair passed (`orbwin-coexist-fb674c82`), including OrbWin restart,
port/data/identity preservation and external workload start-time preservation.
Rancher subsequently started alongside it and both explicit endpoints responded.
The three-engine maintained-workload proof is still pending on the newest binary.

Native appearance QA exercised command-palette filtering/keyboard activation,
light content plus matching Windows title bar, and restoration to the original
system appearance. No permission error was observed. The terminal module now
loads lazily for both container and pod views; the production build has separate
401 KiB application and 335 KiB terminal chunks, with no Vite size warning.

On binary `29073605…`, four consecutive framework suites passed: `36906c23`,
`8fc21900`, `8c054ed5`, `4e718bad`. Each suite includes concurrent normal cases,
Redis restart/data/log/exec checks, deliberate assertion failure and client
cancellation, with Ryuk cleanup and no remaining scoped resources. The current
publication suite (`orbwin-publish-7c91bb85`) also passed Windows/WSL/delegated
clients, publish-all, explicit busy-port rejection and per-container restart.

The three-engine maintained proof passed first at `orbwin-coexist-a3b0cf38`,
then with added explicit other-engine port collisions at
`orbwin-coexist-ad3ae9dc`. It preserved each engine's identity, sentinel data,
external container start times and global configuration hashes across an owned
OrbWin stop/start. Port collisions failed before starting the OrbWin fixture,
and the external services kept answering their own sentinels. All scoped
containers/volumes were removed by exact name. The harness published
`artifacts/qualification/engine-identity.json` only after the successful `--all`
run and final build recheck. The general gated workload proof then passed as
`orbwin-qualification-2e343b64`. This closes the tested host's core coexistence
cases, not full Phase 1 or a broader OS/version compatibility claim.

Native WSL QA found the original GUI-launched WSL process immediately exited,
despite an optimistic success notice. The installed Windows Terminal launcher
now supplies the interactive PTY; the fallback reports immediate exit honestly.
Native observation confirmed an Ubuntu terminal with the `vanyr@BigPC` prompt.
The UI connected `/home/vanyr/orbwin-native-proof-20261024`; a scoped Alpine
container read the host marker and wrote a return file. Native Disconnect removed
the shared mount and both source files remained present. The fixture remains for
the follow-up status-refresh check. Distribution status now refreshes after
launch/project actions, and busy text distinguishes connecting/disconnecting.
Name-based engine grouping now says it is reserved/protected rather than falsely
claiming verified ownership from the name alone.

Current native-adapter regression runs also passed `owned_resource_operations`
(volume/network creation and in-use removal refusal, exec exit status, logs/stats,
pause/unpause, restart/stop/start and persistent sentinel),
`owned_build_compose_workflow` (Buildx/tag/inspect, Compose lifecycle/logs and data
preservation), and `owned_interactive_terminal` (SDK input/resize/Ctrl+C/exit,
backpressure and container survival). These are adapter tests, not substitutes
for rendered native UI acceptance. Frontend build, Clippy and the ordinary Rust
and Node tests pass; opt-in cluster/setup cases remain separately gated.

Native follow-up observed the maintained `nginx:1.29-alpine` fixture through
the verified localhost link in Chrome (port 57851, rendered welcome page); followed
container logs showed that browser's HTTP 200 request. The native terminal
accepted physical-key `echo proof`, reported `stty size` as 36x102 then 36x100
after the keyboard-accessible list divider changed, and displayed shell exit
code 7. The server continued running. Computer-use text injection did not reach
xterm in either screen-reader mode; disabling that mode did not solve it and was
reverted. This is a tooling observation, not evidence of general typing failure.

UI fixes from these checks route context-menu port links through the same
verified native opener, constrain lifecycle/removal controls to observed states,
provide a retry for failed full-detail reads, distinguish initial connection
from read failure, and release operation controls even when the following refresh
fails. Successful command output no longer claims a refresh is still underway.
The frontend build and 36 Node tests pass after these changes. The earlier
hand-written netcat HTTP fixture answered simple clients but reset Chrome requests;
the maintained Nginx fixture supplied the browser acceptance check.

Kubernetes completion now adds a read-only management ConfigMap catalog and
native exact-release selection. Patch 0009 resolves the installer release from
that same pinned catalog (including `+k3s3` entries) and enforces forward patch
or one-minor transitions in App admission. A version change requires readiness
and settlement at the existing generation; repeating the same version remains
available for recovery. No downgrade, skipped minor, clearing an established
version or reset is offered. The architecture note records interruption and
cancellation semantics. Enable/disable confirmations now explicitly explain
that applying restarts the guest and interrupts Docker workloads too.

All nine patches replay to their locked hashes. Controller/catalog Go tests,
38 Node tests, frontend build, Clippy and 26 ordinary Rust tests pass (eight
runtime tests remain opt-in). Binary `7e93d27234d87ab93c2d198d36899ac8699d8acc4c2670ab812e43c0bb7d8334`
passed the existing owned-guest upgrade proof (`1c561929…`), including service
and mount suites, with the original daemon identity retained. Native enablement
reached KubernetesReady/Settled at 05:15:10 Pacific. Current cluster, retained-PVC
version-upgrade and refreshed coexistence qualification remain in progress;
the old eight-patch record does not qualify the new binary.

Native version apply completed at 05:20:05 Pacific, from desired 1.34.6 to
observed server `v1.35.3+k3s1`. The scoped upgrade fixture retained its PVC UID,
PV name and a unique file written once before the upgrade. A server-admission
dry-run rejected the downgrade and left the App generation/version unchanged.
The ordinary native cluster adapter proof (`cluster-1792844143960`) also passed
apply/readiness/inspection/logs, wrong-UID deletion refusal and exact cleanup.

Log inspection exposed concurrent provisioning: Lima's direct WSL `boot.sh`
invocation retried without DNS in the original namespace, while the existing
systemd invocation successfully installed the release in the engine namespace.
Patch 0010 guards every stateful product script. The direct path waits for that
unit; only the namespace owned by network-namespace.service may mutate services.
Behavior tests execute the generated shell guard with controlled namespace/unit
observations, covering success, failure, deadline and an unknown private namespace.
The full ten-patch clean replay is `patch-replay-26d9ff07…`; controller, bridge and
host-switch race tests pass.

Binary `e3892a1f2bc994837d1f6285c2b0a85df7e76f3926bca70b80422c293225f064`
passed the owned upgrade proof `7839c0d4…`, retaining the original daemon and
the same cluster fixture's PVC/PV/sentinel. Requalification records the exact
successful prior-to-current backend proof chain rather than overwriting identity.
The direct boot log now reports systemd completion and skips duplicate scripts;
the unit is exited/success and no installer curl remains. Build metadata now
reflects the effective replaced Kubernetes modules: kubectl client 1.36.4, with
successful full version output against server 1.35.3. The original metadata
parse failure stays in the fixture's prior-failure record.

The native forward readiness/error check then found BL-015 (`socat not found`).
Do not treat the preceding listener-only readiness as HTTP acceptance. The
failed link was removed, error output remained inspectable, and the pod survived.
Signed guest-dependency installation is currently a scoped proof; bundled repair
and native forward/log/terminal acceptance remain open. Separately, storage
cleanup now cancels its current client on navigation and releases progress even
if its following refresh fails. Read-only detail/catalog reads have bounded
contention retries with cancellation; identity/transport failures are not retried.
Frontend build, 41 Node tests and Clippy pass for this checkpoint.

### Offline guest dependency checkpoint — 2026-10-24

Patch 0011 embeds signature-checked, hash-pinned Leap 16.0 socat/libwrap RPMs;
the bundle also retains their source RPMs and license texts. Build and runtime
identity bind the separate package lock. Existing-guest proof `bbbabad2…` passed
after removing the two manually installed proof packages, demonstrating automatic
offline repair. Binary `bdad9ca5a7b7554664760b3a5002bfcc3163c771496694a8a1688a2c9c1a91ce`
retains the original daemon and the upgrade fixture's PVC/PV/sentinel. Eleven
patches replay cleanly (`patch-replay-f10c65e0…`); controller/socket/host-switch
race tests and 26 ordinary Rust tests pass.

Native forwarding after manual dependency repair served the unique retained
file on Windows port 60722, survived navigation and stopped without stopping the
pod. Native followed logs pause, filter and export work; the exported three-line
file is `artifacts/qualification/native-pod-log-20261024.txt`. Wrapping now takes
precedence over shared command-output CSS. Native file dialogs are parented to
their invoking window. Fresh guest and bundled native forward/terminal acceptance
remain open. A separate service-pod failure now reproduces a publication check
on a CRI sandbox with Docker network mode `none`; investigation continues.

### Kubernetes sandbox publication correction — 2026-10-24

Patch 0012 preserves Docker's network-less sandbox contract. A `none` network
does not publish Docker bindings; CRI configures CNI after Start returns. The
adapter previously awaited publication before that step, repeatedly preventing
the service pod from becoming ready. Create preserves its request without port
allocation, and start/restart dispatch uses the inspected immutable ID without
the inapplicable publication check. Labels do not authorize this exception.
Bridge publication and occupied-port refusal remain unchanged. Regression tests
cover occupied Windows ports, retained backend failure and bridge refusal.

Twelve patches replay cleanly (`patch-replay-7a22dcbd…`); socket-bridge race tests
pass. Binary `0896047decdfa7c851870e80b5edc51439f243b8baa522379b4933c60e15c8f6`
passed upgrade/service/mount proof `821f6932…`, retaining the original daemon.
The existing service pod recovered to 2/2 ready without deletion. Retained-PVC
verification passed again; Windows/WSL/delegated publication proof
`orbwin-publish-92b1b397` passed. Node 41 tests, frontend production build and
Clippy pass. These are checkpoint results, not Phase 1 acceptance.

### Native Kubernetes sessions — 2026-10-24

On the twelve-patch binary, native pod TTY commands execute; `stty size` changes
from 36x102 to 73x296 when the window maximizes. Ctrl+C interrupts `sleep 30`
and returns the shell; `exit 7` reports code 7. The pod remains Running.
Initial missing command submission was the Windows Chinese IME composition
mode; switching input mode allowed the physical keyboard checks to complete.

Native pod forward 61898 served the exact retained PVC sentinel in Chrome and
through HTTP before and after navigating to Containers. Native service forward
54316 served the same sentinel. Explicit Stop removed each Windows listener,
reported client cancellation and retained the workload. The temporary service
`orbwin-upgrade-45ba4085/native-forward-20261024` was then deleted through native
confirmation and verified absent; the pod stayed Running. No other service was
deleted. UI request construction now selects only name/namespace/UID instead
of spreading full server metadata into the strict native command. An earlier
unintended automation target exposed that rejection before dispatch; it caused
no deletion. A regression test covers extra metadata and retained UID. Node 42
tests and production build pass.

### Setup replacement and fresh guest — 2026-10-24

Installed setup now validates the previous home/endpoint/identity/intent before
service changes. A changed bundle stops the owned management service before
starting its replacement; a live foreign or uninspectable endpoint refuses.
The previous identity remains until current-start readiness and the original
daemon agree; a concurrently changed record refuses replacement. The initial
supported transition is within the pinned upstream version. The UI offers an
explicit update with interruption confirmation rather than asking installed
users to run a development proof. Unknown upstream transitions remain blocked.

The older proof guest passed replacement and repeated resume, retaining daemon
`58d0d4b6…`. Its full 2,069,053,440-byte WSL export was inspected and hashed
(`b696f18db61f81f8e1edf2920453a4aca1a4edf547147c46f860cc9e95a96704`)
before decommissioning only that task-created fixture. Its control plane and
export remain in `artifacts/setup-proof-preserved-f4a26f81…`.
Fresh first-run cancel/setup/resume then passed in 96.66 seconds, creating a
distinct new proof daemon `9c565236…`; this is fresh provisioning, not migration.

Fresh dependency/HTTP/restart proof `fresh-guest-283e6fbe…` passed on `0896047d…`:
both bundled RPMs verify, pod/service forwards serve the unique marker, and
service forwarding works after restart with unchanged package verification.
The exact namespace was removed and the proof service stopped. Earlier failures
remain: a previous fixture's zero-byte shared socket placeholder was preserved
as `docker.sock.preserved-20261024` after checking type, size, owner and mount
status; the relay correctly refused to overwrite it. The harness now requires
current-start Lima readiness, a live API readyz and actual container exec before
trusting persisted Pod Ready conditions. No guest reset was used as recovery.

The current twelve-patch binary also passed refreshed three-engine coexistence
`orbwin-coexist-bc3992f3` and consecutive Testcontainers suites `5e0c6ddd…` and
`7176ed68…`, each including concurrency, intentional failure, cancellation and
Ryuk cleanup. Other daemon identities, sentinel data and global client configs
were preserved. Installed/native setup and remaining phase-wide gates stay open.

### Registry credentials and native private-image flows — 2026-10-24

The native Registries tab now uses a checksum-bound Windows credential helper
under OrbWin-specific encoded keys and ownership attributes. Docker performs
authentication; secrets use stdin and login output is discarded. No global
credential helper or Docker config is changed. The selected upstream runtime's
credential-helper SDK 0.9.12/wincred 1.2.3 passes scoped Windows vault tests; the
initial older-library write failure remains recorded in the architecture note.
Private per-registry helper routing preserves unrelated config and refuses
existing foreign helpers/file credentials. Setup verifies the helper before
guest mutation. Transient credential clients and descendants use the existing
kill-on-close client job; backend workloads remain independent.

Fixture `orbwin-registry-4ea39801` used an explicitly disposable public test
password and a pinned authenticated Registry 3 image. Native rejected login,
successful login, password clearing, saved-list persistence through native
process rebuilds, authenticated pull and Buildx private-base build passed.
BuildKit reported credential exchange; the resulting image executed its unique
marker. Pull verification compares the registry's manifest digest, rather than
confusing the pre-push multi-platform index ID with the pushed platform manifest.
The parented native folder picker also selected the real build context.

Native Ubuntu project connection synchronized only helper routing metadata;
the ordinary non-root user's Linux Docker CLI pulled the exact private manifest
through Windows interop without changing its global Docker config. Windows
interop is required and failures remain explicit. A paused fixture made native
login stall; Cancel cleared the password, reported uncertain storage outcome
and retained the previous saved entry. Native Forget reached the empty state;
the next private pull failed authentication while the registry stayed running.
Exact fixture container, volume, tags and vault entry were then cleaned. Global
Windows Docker/kubeconfig hashes remained unchanged. Registry state/evidence is
retained under `artifacts/qualification/orbwin-registry-4ea39801/state.json`.

Forty-two Node tests, frontend production build, thirty ordinary Rust tests and
Clippy pass. Go ordinary, race and explicit synthetic Windows vault tests pass.
This closes the registry implementation checkpoint; installed-product and
remaining phase-wide gates still apply.

### Installed packaging, files, measurements and recovery — 2026-10-24

The ordinary per-user install uses `%LOCALAPPDATA%/OrbWin` for immutable files
and sibling `OrbWinData/home` for state. Native fresh first-run cancellation and
Resume created daemon `d9c00194-ea9d-439a-8561-6a0ec580fbd5`; this is a fresh
engine, not migration of the archived development fixture. Installed three-engine
coexistence passed `orbwin-coexist-f0528dd8`. The twelve-patch rdd hash remains
`0896047d…`. The former development guest was exported and inspected before only
that task-created registration was retired; its exact persistent sentinel and
archives remain under `artifacts/main-qualification-preserved-c8a23249…`.

The separate packaging overlay stages checksum-bound backend/helpers/clients,
licenses for 772 resolved dependencies (zero missing notices), pinned guest
source RPMs, MPL source and upstream patches. NSIS current-user installation is
unsigned. A mapped backend aborts installation before file replacement; the
native refusal retained the active HTTP marker. Settings' explicit Prepare
update then stopped the owned service and verified its executable was released.
Native uninstall with **Remove window preferences** selected removed the app
but retained runtime identity and WSL disk registration. Reinstall retained the
daemon and named-volume marker. See `architecture/packaging.md` for file-lock,
partial-copy and data-lifetime constraints; final-current-bundle QA continues.

Native testing caught and corrected two packaging/startup issues: an incomplete
custom English file made maintenance labels blank (custom language files replace
all defaults), and persisted App Settled state could let Start return before a
resumed management service's guest was ready. Start now also requires current-
service Lima readiness before actual Docker identity/publication verification.
The corrected installed Start passed after uninstall/reinstall.

The Files placeholder is replaced by bounded read-only Docker archive access for
containers, including stopped containers and their mounted volumes. No guest
shell, host extraction or implicit helper container is used. Native listing and
UTF-8 preview displayed the exact retained marker from a stopped Nginx container.
Partial listings, sparse logical data, large/binary/special files, path safety,
stale reads and cancellation are bounded; see `architecture/container-files.md`.
The Go helper's ordinary/race tests and Rust argument checks pass.

Installed baseline `orbwin-baseline-744ab973` passed three 37-second restarts,
stable idle samples, cached/uncached builds and C/F/selected-Ubuntu filesystem
samples against predefined budgets, retaining identity/data/global configs and
cleaning its fixtures. The earlier intermittent 502 and cleanup-reporting failure
remain retained; the passing repeat does not establish their root cause. See
`baseline-2026-10-24.md`. Installed contained faults `orbwin-faults-39a2f549`
passed cgroup OOM/137 and tmpfs ENOSPC with named-volume data preservation.
These are scoped container-pressure tests, not host-wide resource exhaustion.

Native review at the 900x600 client minimum passed settings, light/system
appearance including the title bar, bounded logs/search/wrap, diagnostic export,
partial batch outcomes and selected storage cleanup. The two-network batch
retained the active network, removed the unused network and reported 1/2 success.
Storage removed only the two stopped test containers, retaining the active HTTP
service, marker volume and unselected volumes. Filtered log export contained
exactly one matching line; diagnostic export excluded credentials/workload files.
Raw UI observations and exports are in `artifacts/qualification/native-installed-*`
and `native-visible-1499.log`.

Guarded process faults verified exact executable/creation identity before killing
only the owned management process or hostagent. Management loss left Docker/HTTP
healthy in all ten samples, while a fresh Settings read correctly failed. Resuming
management restarted the guest on this pin and interrupted Docker; do not claim
seamless adoption. Reconnect is therefore an explicit confirmed running-state
request with interruption wording and actual readiness checks. Its new native
confirmation still requires final-current-bundle qualification.

Hostagent loss and exact owned `wsl --terminate lima-orbwin` caused rdd's own
recovery, without a new supervisor or global WSL shutdown. Native reads exposed
the unavailable/Starting state and Refresh returned connected afterward. Read-only
postchecks retained daemon `d9c00194…`, the HTTP marker and restart policy, both
external daemon identities and global config hashes. See `installed-recovery-*`
JSON records. Ubuntu's selected project export retained its exact mount signature
`2112:1374236`, and the explicit Linux client still reached the same daemon after
guest recovery. User distributions were not terminated.

### Final installed bundle regression checks

Installer `fa03b4512dd2a1c69249ee10b8f0d9fb92026018a02b3510b4e44a5f0e7d876d`
passed actual NSIS silent replacement outside the development host, following
native Prepare update and window closure. All four installed executable hashes
matched the installer payload/build records; the runtime identity was unchanged.
The final installed app started successfully with the existing HTTP fixture's
`unless-stopped` policy, returned its exact saved marker and retained daemon
`d9c00194…`. Read-only postchecks again retained both foreign daemon IDs and
global configuration hashes. This qualifies the bounded startup/publication fix.

Native detach and reattach of the task HTTP container passed on this bundle.
After detach the Info panel immediately displayed “No connected containers.”;
after reattach it displayed the container/address and HTTP returned the saved
marker. The earlier stale inspector result remains the motivation for tagging
inspect reads with their immutable inventory snapshot and cancelling stale reads.

An additional isolated-network fault did **not** pass automatic recovery:
bringing only the owned guest tap down caused its removal, so the armed up-link
restoration could not find the device. Docker stayed reachable, but HTTP did not
recover automatically. The failed `owned-link-flap.json` is retained. Native
Prepare update and subsequent Start restored the network and marker without a
reset or changing the daemon identity. This is observed explicit recovery, not
evidence of transparent host-network/VPN recovery; see BL-017.

Native confirmed reconnect subsequently passed after a second exact management
process loss. A fresh Settings read first reported the refused management API;
the confirmation warned about guest restart/workload interruption. Reconnect
returned healthy configuration and the exact HTTP marker. Postchecks retained
the original daemon, both external engines and global configurations. This is
explicit recovery with an interruption, not seamless adoption.

A second tap-loss injection first refused to run because the newly enabled
cluster added containers without the ordinary fixture label. The harness was
then narrowed to permit only API-verified system pods and the exact recorded
qualification namespace/UID; unknown workloads still refuse. The repeated
injection reproduced tap removal. Ordinary native Restart recovered HTTP and
retained the installed PVC UID, PV, marker and Ready node. This qualifies the
documented explicit recovery without relying on application-update preparation.

The installed optional cluster also enabled successfully. Its initial upstream
default was 1.34.6; kubectl 1.36.4 warned that this exceeds supported version
skew. New-App creation now explicitly selects the already-qualified 1.35.3 while
leaving Kubernetes disabled. Existing Apps are preserved. The installed explicit
1.34.6 → 1.35.3 upgrade passed and retained PVC UID `df8cf086…`, its PV, the
unique saved marker and a Ready node (`installed-cluster.json`). This is separate
from the prior archived development cluster's upgrade evidence.

Final local checks pass: 42 Node tests, 37 ordinary Rust tests (eight opt-in
qualification tests are separate), Clippy with warnings denied, Rust formatting,
frontend production build and whitespace validation. The session helper's
ordinary/race checks passed after the bounded archive parser changes. Review
covered operation ownership/cancellation, identity checks before mutation,
installer state separation/quiescence, archive byte/logical/entry limits and
stale inspector reads. These checks do not prove absence of defects.

The installed D9 engine then passed Testcontainers suite `ed404a2b…` with the
optional 1.35.3 cluster and both other engines running. Normal Redis/PostgreSQL,
two concurrent normal workers, intentional assertion failure and cancellation
all passed. Ryuk remained enabled, no task containers/helpers remained, and
global client configurations were unchanged. Raw expected-cancellation warnings
are retained; cancellation is not reported as a successful application exit.

Phase 1 remains in progress until whole-host sleep/resume and final scope/gate
reconciliation are complete.
