# Ordinary-workflow implementation and qualification

2026-10-22 Pacific. This is incremental evidence, not phase 1 approval. The
coverage matrix and roadmap remain authoritative; no scope reduction is implied.

## Actual results

- Native resource operations proof passed: create/inspect/remove volume/network,
  run/inspect/exec/logs/stats, network attachment, pause/unpause/restart/stop/start,
  rejection of in-use removal, nonzero exec status, cancellation of active log
  follow, and scoped cleanup. The extended port/environment/volume proof passed
  in 51.54 seconds (`operations-1792725416839.json`): default publication was
  localhost, requested environment was present and volume data survived lifecycle
  changes. This predates the new Windows bind adapter.
- Build/Compose native adapter proof passed: default explicitly targeted builder,
  build/load, tag/full inspect, Compose up/readiness/restart/logs/stop/start/down,
  retained Redis data after restart and exact test-resource cleanup.
- Management proof passed a real owned stop/start and preserved daemon identity.
  Configuration reads were subsequently changed from `rdd ctl` to explicit
  management-kubeconfig `rdd kubectl`, avoiding upstream ctl's auto-start behavior;
  qualify that change in the next cycle.
- Interactive SDK helper proof passed, including immediate input, tty size,
  resize, Ctrl+C, exit code 7, 64 KiB unacknowledged output bound, automatic
  stalled-viewer disconnection and continued container operation. The latest
  repeat also supplies deliberately wrong inherited Docker host/context/TLS
  selectors. The helper independently checks the expected daemon before exec.
- Kubernetes workload proof passed: optional cluster enable, apply, deployment
  Available and claim Bound conditions, deployment/service/claim reads, pod logs,
  wrong-UID delete rejection and continued service existence, UID-preconditioned
  cleanup/deletion waits and restoration of the original disabled cluster state.
  Newly added forwarding and pod exec still need their own runtime proof.
- Testcontainers Node 12.2.1 on Node 24.18.0 passed three normal suites (including
  two concurrent processes), intentional assertion exit 23 and process
  cancellation (SIGTERM). Redis/PostgreSQL readiness/exec, published Redis PING,
  restart data, expected exit 7 and logs passed. Ryuk remained enabled; all
  recorded workload/helper resources disappeared without fallback cleanup.
  Docker and Kubernetes global config hashes were unchanged.
- Native UI created `orbwin-ui-volume-proof`, showed actual command output and
  refreshed inventory. This is limited UI evidence; the final executable needs
  renewed visual/keyboard review after the later additions.

Raw evidence is retained under `artifacts/qualification`: `operations-*.json`,
`build-compose-*.json`, `management-*.json`, `terminal-*.json`, `cluster-*.json`.
The successful framework run is
`testcontainers-09b6760d-4815-4c3c-945a-5718be47551c/result.json`.
Inspect result fields, not just filename existence: failed attempts remain too.

## Failures diagnosed without changing the phase claim

The first Kubernetes fixture used an Alpine image without `httpd`; its process
exited and the deployment wait correctly failed. Pod events, prior logs and claim
state distinguish that test error from backend failure. The corrected pinned
Redis fixture passed. Test cleanup restored the previous cluster setting.

The initial host-ConPTY terminal route repeatedly lost early test input. Removing
creation flags did not resolve it; no definitive Windows root cause is claimed.
That route, the portable-pty dependency and its experimental patch were removed.
The retained per-session helper uses the same Moby SDK version as rdd and attaches
directly to the Linux TTY. Stable xterm/Fit versions are pinned. No second engine
supervisor, host shell or custom Docker proxy is involved.

Testcontainers' standard CLI `DOCKER_HOST` form was parsed through WHATWG URL,
which removed the `/./` pipe segment. Its configuration strategy failed and its
default named-pipe strategy selected another daemon. The identity assertion
rejected this before GenericContainer.start could create any helper or workload.
An opaque URI also failed because Docker modem processes environment defaults
before applying socket options. The working Windows Node-specific form is:

```text
DOCKER_HOST=npipe:////.\pipe\docker-rancher-desktop-orbwin
TESTCONTAINERS_HOST_OVERRIDE=127.0.0.1
TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock
```

This is per-process guidance for the tested client version, not a global setting
or a universal URI for all Docker clients. The CLI retains its canonical form.
Both resolve to the same owned pipe. Keep the identity guard in test harnesses;
do not assume an explicit environment variable forbids framework fallback.

## Outstanding gates

Full native workflow/accessibility review; first-run setup; Windows bind paths;
WSL distro integration; settings/cache/credential coverage; simultaneous engines
and start orders; interrupted lifecycle/data preservation and broader fault tests;
measured support matrix; and installable packaging. Automatic updates are deferred.
The runtime still depends on development proof artifacts and an installed Docker
CLI. These are explicit unfinished integration/distribution work.

## Headless continuation while the computer is in use

The user requires confirmation before further real-computer verification.
Desktop interaction and live Docker/WSL/lifecycle/coexistence tests are deferred.
No engine was stopped or restarted during this continuation.

- Container Run accepts existing local Windows drive sources and maps their
  canonical paths into the observed WSL `/mnt` layout. Files, directories,
  spaces and read-only syntax have parser coverage; shares/device paths and
  option injection fail explicitly. No host source directory is created.
  This is a UI adapter, not a general Docker API or Compose translation layer.
- Added the ignored `owned_windows_bind_mounts` runtime proof for directory
  reads, read-only files, writable host files and restart persistence. It has
  not run. Its only attempted writes target a newly created disposable file.
- Fixed dropped cluster refresh events during in-flight reads and preserved
  empty arguments in command forms. A terminal reader failure now disconnects
  its helper rather than abandoning the client; runtime requalification remains.
- Headless checks passed: 14 Node tests, production frontend build, Go helper
  unit tests, 17 Rust tests with six live proofs ignored, and Clippy including
  test targets with warnings denied. Native visual review remains deferred.
