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
