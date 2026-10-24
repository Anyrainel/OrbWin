# Backend boundary and isolation

Development baseline: `upstream.lock.json`. This source pin is not a supported
release claim. Apache-2.0 upstream code and its notices stay in the checkout;
local adaptations are reproducible patches with original headers preserved.

`rdd` owns guest and workload lifecycle. The UI observes and requests upstream
operations; window closure must never stop the service. No UI-side supervisor,
Docker protocol translation or engine storage access is introduced.

## Initial isolation contract

- OrbWin uses the fixed `RDD_INSTANCE=orbwin` identity. Never accept arbitrary
  instance arguments from the frontend. Upstream paths remain instance-derived.
- Windows Docker socket and advertised endpoint derive from the same instance
  function. Each instance gets `docker-rancher-desktop-<suffix>`; never listen on
  `docker_engine` or Docker Desktop's pipe. Unix paths retain upstream behavior.
- The bridge constructor consumes that same function. No copying/proxying Docker
  requests through an OrbWin service. Upstream owns stream semantics.
- Launches set `DOCKER_CONFIG` and `KUBECONFIG` to OrbWin-owned files and disable
  the `path-management` controller. No global context, credential or PATH edits.
- Endpoint availability is not identity proof. Isolated development workloads
  require verified owned registration, build and daemon identity. General runtime
  support still requires the complete host integration/coexistence audit.

The pipe patch changes endpoint selection only. It does not establish safe
simultaneous operation. Shared Hyper-V vsock listeners, registry registration,
WSL distro naming, mount/proxy integration, networking and host port behavior
required qualification before the isolated guest proof. The patches below and
proof preflights establish only the exercised owned case, not broad coexistence.

Candidate patch 0002 gives the OrbWin build a distinct discovery signature,
four vsock ports, WSL veth names and bridge subnet. The host, network-setup,
guest API bridge and WSL proxy must be built together. Stock Rancher guest
binaries must never be mixed into this bundle. Contract tests compare both
sides and reject reuse of the stock constants. This is product namespace
separation, not cryptographic authentication or arbitrary multi-instance support.
One OrbWin guest is supported. The proof rejects overlapping host routes before
boot, including broad routes. VPN transitions and integrated user distributions
remain unqualified. See the current runtime evidence in development progress.

Patch 0004 establishes the binary's product boundary before constructing CLI
commands: default `RDD_INSTANCE=orbwin`; reject any identity except `orbwin` or
the management-only `orbwin-proof`; force Docker/Kubernetes client files under
the selected instance's short directory. This covers dynamic `set --help`,
which upstream autostarts to discover CRD fields. Calling it without a scoped
environment during development unexpectedly created the default management
service; that exact process was stopped. Its files were retained, and WSL
distributions and selected Docker context were verified unchanged. Do not use
unscoped upstream dynamic help as a read-only inspection method.

## Failure and ownership

`rdd` retains service PID/data ownership and concurrency mechanisms. Scripts
must fail on source drift or unapplied patches and must not reset a dirty source
checkout. An occupied pipe is an error, never a reason to terminate its owner.
No automatic engine restart follows a failed read. Missing backend artifacts
must yield actionable unavailable states rather than simulated success.

The native inventory adapter is read-only. It consumes a task-generated ownership
record matching the source/build manifest and probes the daemon ID before and
after a complete four-resource inventory read. Partial errors never produce an
empty successful snapshot. Subprocesses have fixed binary/argument surfaces,
no shell, explicit endpoint/config selection and cleared competing context/TLS
environment selectors. Concurrent reads are bounded by per-command deadlines and
output caps. Closing the UI drops read clients only. This development path trusts
local build/proof artifacts; distribution signatures, installation trust and a
production backend discovery contract remain release work.

## Explicit operations adapter — phase 1 implementation

`operations.rs` accepts typed resource operations, never a renderer-supplied host
command line. Every Docker client targets the fixed product endpoint/config;
competing context/TLS/Buildx selectors are removed or fixed. Full resource IDs
are required for container/network/image-ID operations. Named-volume and image
reference inputs reject option injection. No host shell is used. Container exec
arguments are passed to Docker as separate arguments and execute in the container.

The desktop process admits at most four operation clients and one mutation at a
time. An RAII registry lease retains the slot through cancellation and cleanup.
Inventory reads remain independent and authoritative. Engine identity is checked
before and after Docker operations. Mutation interruption means outcome uncertain,
not rollback; refresh rather than automatically repeating a potentially committed
operation. Child stdout/stderr drain concurrently. IPC output is capped at 2 MiB
per command, with truncation announced; further output is drained and discarded
without terminating a mutation merely because it was verbose. UI history retains
the latest 128 KiB. Deadlines depend on operation type. Cancel kills/reaps only
the CLI client; remote builds/exec/reconciliation may continue.

Explicit management operations execute the hash-verified bundled development
`rdd` with the recorded private home and fixed identity. Registry BasePath must
identify exactly one `lima-orbwin` guest inside that home. `rdd set` remains the
owner of reconciliation and waits for its current-generation Settled condition.
Stopping the guest does not require its Docker endpoint to remain available.
Start and cluster changes require an actual post-operation Docker identity check.
Kubernetes commands use the owned probe's `k3s.yaml` explicitly, never the global
context or management API kubeconfig. Deletes carry UID preconditions through
kubectl's raw DELETE path, preventing stale selections from deleting replacements.
First-run provisioning and installed discovery remain unfinished.

`session.rs` owns only interactive Docker SDK helper clients, not the engine.
Two sessions maximum, 8 KiB per input write, a 64 KiB acknowledged output window,
and a 15-second stalled-viewer deadline bound client buffering. xterm acknowledges
only after parsing output. Input is serialized and queued with a 64 KiB frontend
bound. Closing a tab/session kills its CLI client; it does not promise to kill
the exec process inside the container. Reader and waiter lifetimes are separate
so teardown can drain output while reaping the helper. Session drop signals all
remaining clients. The SDK interactive proof passed; later reader-error handling
and final native interaction still require runtime review. The helper reuses the pinned Moby SDK for exec creation,
attach, resize and exit inspection. It verifies the daemon ID independently and
never consults default contexts or environment selectors. An initial host-ConPTY
route failed early-input tests and was removed, including its experimental
dependency patch. The retained path connects xterm to the guest TTY directly.

The Run-form mount adapter canonicalizes existing local Windows drive paths and
maps them to the observed WSL `/mnt` layout. It rejects UNC/device paths and CLI
mount-option separators; it creates no source directories and changes no WSL
configuration. This boundary handles only typed UI requests, so unknown Docker
API fields and hijacked streams never pass through it. The 2026-10-24 scoped
F-drive runtime test passed file/directory/read-only and restart preservation
after fixing canonical local-drive path dispatch. General CLI/Compose Windows
paths remain BL-005; this does not establish wider drive/distro qualification.

Before changing transport lifetimes, review cancellation, blocked copies,
half-close, active connection drain and listener cleanup separately. The initial
patch intentionally retains upstream bridge behavior; its runtime failure tests
are still required, including existing tests and coexistence under streaming load.
