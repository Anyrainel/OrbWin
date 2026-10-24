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
