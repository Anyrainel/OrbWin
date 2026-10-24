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
