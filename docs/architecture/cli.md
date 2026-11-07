# Thin Windows CLI

The source now also provides `clients recover [--apply-reviewed | --restore-original]` for an
interrupted private Docker settings write, without requiring engine startup.
See the [recovery interaction contract](client-recovery-interaction.md) for its
one-use native review, consequences and remaining qualification gates.

Recorded 2026-10-25. This implements P2-D03a/b/d and dispositions D03c/e.
It is a development implementation pending packaging and lifecycle qualification.

## Ownership and supported commands

`orbwin-cli.exe` lives beside `orbwin.exe` and uses the same `runtime/backend`
and `runtime/tools` directories. It reads `%LOCALAPPDATA%/OrbWinData/runtime.json`
and accepts only the qualified OrbWin endpoint, matching compiled upstream commit,
and canonical product home. It never selects the current Docker context.
`rdd` remains the only backend lifecycle authority. The CLI has no service loop,
background supervisor, engine store, renderer dependency, or startup registration.

Commands:

- `status`: bounded verified Docker info, projected to identity, version, platform,
  capacity and inventory counts. If unavailable, a read-only typed probe reports
  management state/reason and engine unavailable. Unknown emits useful JSON and
  exits nonzero; supported stopped/running observations can be reported without
  starting anything. Management stopped never means engine/workloads stopped.
- `start` / `stop`: validate qualified backend SHA and owned WSL registration,
  validate persisted service arguments and inspect existing management policy,
  then forward `rdd set running=...`. If `start` cannot read the management API,
  only a strictly validated supported-generation machine observation confirming
  the owned service stopped permits one existing rdd service-start request with
  path management explicitly disabled. A fresh typed running/identityVerified
  observation is then mandatory before rereading manual policy and desired state.
  A successful warm settings read does not require that new machine interface.
  Start additionally waits for the current startup and verifies the Docker ID.
  Stop affects owned workloads and is always an explicit command.
- `logs`: report the backend log directory, without reading or uploading logs.
- `snapshot NEW.json`: exclusively create an allowlisted JSON snapshot. No raw
  logs, environment, image/container names, labels, registry data, proxy strings,
  credentials, host paths, management tokens or kubeconfig enter it. An offline
  engine produces `status: unavailable` and null projected fields, without raw
  client errors. A write/sync failure retains the new possibly partial file.
- `docker COMMAND ...`: verify the bundled CLI checksum and owned daemon ID,
  then execute literal arguments with fixed `--host` and private `--config`.
  Inherited targeting/TLS/API-version/remote BuildKit and hidden Compose selection
  environment variables are removed from this child only. Target/security override
  flags are refused before the verified Docker run/create/exec command payload,
  and throughout other command arguments. The payload boundary uses option-arity
  tables from checksum-pinned Docker 29.9.2; a value resembling an image/container
  does not establish the boundary. `-c` is Docker run/create CPU shares, while shell
  `-c` and literal target-looking container arguments remain ordinary payload.
  Unknown flags fail closed for boundary recognition. Run/create/exec short clusters
  are checked for target shorthands before that boundary while respecting local
  value options and run/create CPU shares. Pinned Docker independently rejects
  `run -itH... --help` / `exec -itc... --help` before any engine request; the explicit
  wrapper defense preserves that refusal and tests literal payload/value cases.
  `context` management and Buildx
  builder management are refused: they can select a different endpoint or remote
  builder. `buildx build`, `buildx version` and read-only `buildx inspect [default]`
  use the fixed default builder. Inspect accepts formatting but refuses bootstrap
  and foreign selectors. Ordinary
  targeted `build`, `compose`, `exec`, `run` and other Docker commands are available.
  Commands retain Docker's data impact; this is not a read-only wrapper.
- `config get [KEY]` / `config set KEY=BOOL`: only `running` and
  `kubernetes.enabled` are exposed. No arbitrary upstream setting passthrough,
  PATH editing, kubeconfig or context change is provided. Reads use existing
  management kubeconfig through `rdd kubectl`, avoiding `ctl` auto-start. A failed
  read preserves the owning API error and independently distinguishes confirmed
  stopped management. Running, unknown and unsupported evidence do not become an
  offline sentinel, and reads never start anything.
- `top`: one Docker stats sample, suitable for a terminal or repeated explicit
  invocation. It reuses Docker's metrics and formatting. The GUI activity monitor
  owns the adjustable live history; this CLI does not add a second sampler or
  pretend a single sample is engine/VM overhead measurement (D03e).
- `feedback`: private repository/reporting-policy status plus snapshot/review instructions. No browser
  launch, network request, attachment or automatic upload (D03d).
  The repository is verified private via the connected account; public issue URLs
  returned 404. A usable public/private reporting policy remains required before
  the feedback destination can be called complete.
- `update`: explains that no trusted release feed exists; retains desktop Prepare
  update for reviewed local candidates. A live update-available check is deferred
  to Phase 4 until a release feed/signing policy exists (D03c).

The executable name avoids colliding with the installed GUI executable. No PATH,
profile, Docker context or kubeconfig edit is required: invoke its absolute path
or use `& 'C:\path\OrbWin\orbwin-cli.exe' status` in PowerShell.

Session completion is enabled explicitly by dot-sourcing `completions/OrbWin.ps1`.
It uses static suggestions and literal AST string values without launching the CLI,
reading engine state or evaluating argument expressions. Completion respects the
cursor when editing before existing arguments, handles quoted words and absolute
executable paths, and stops suggesting keys once the command's supported argument
count is complete. Real PowerShell7 `TabExpansion2` qualification covers ten
positive and five negative cases in `scripts/qualify-cli-completion.ps1`, with a
source-hashed receipt under `artifacts/cli/completion-*/proof.json`.
Windows PowerShell5.1 on this host exposes native completer registration, but its
script execution policy blocked the script test. That host remains unqualified;
no execution-policy change or bypass was made. No profile or persistent PATH edit
is part of completion setup. Tray/login-start behavior belongs to desktop
integration and is not implemented by this thin CLI; cold lifecycle remains a
separate required live gate despite the mock-tested request path.

## Invariants and failure behavior

No command changes persistent global client configuration. Management mutations
require an existing App with `application.addPath=manual`; foreign policy fails
closed. Every lifecycle call also checks one `lima-orbwin` registry registration
strictly inside the recorded home. Binary hashes are bounded streaming reads.
No CLI option can supply a foreign runtime, state directory, pipe or daemon ID.
Bundled resource layout is resolved from the CLI executable, not working directory.

Read-only observations have 15-second client deadlines and a 2 MiB output bound.
Management mutations have a ten-minute overall deadline and upstream bounded
waits. Docker passthrough has no arbitrary runtime deadline; Ctrl+C cancels its
client. Cancellation or a failed wait does not undo an accepted desired state,
stop rdd, stop unrelated workloads, reset a distro, or imply a mutation was not
sent. Inspect status/settings before retrying. Child output pipe draining is bounded
by Go's `WaitDelay`. The process owns no resource beyond its child and snapshot file.

Qualified cold management-service startup uses the exact upstream request already used by
`installation.rs`: `rdd service start --controllers=*,-path-management --timeout=90s`.
Before any lifecycle request, the CLI requires an existing `args.json` with one
unprivileged secure port, the exact disabled-path-controller list, optional bounded
verbosity, and no unknown/duplicate flags. It never implicitly creates a service
or rewrites its defaults. Pinned upstream `serviceStartAction` treats provided
controller flags as a transient override; `service.Start` appends them after saved
defaults. The path controller therefore cannot act while the API starts and the
App policy is re-read. Startup may already reconcile persisted owned desired state;
that remains rdd's responsibility. Failed service startup, failed policy re-read,
foreign policy, cancellation and post-set readiness/identity failures stay errors
and never cause a rollback, engine restart or destructive retry. Offline `stop`
and configuration changes fail without starting the management service.

A settings error alone never authorizes a management-service start. The fallback
requires schema1/generation `rdd-service-identity-v1`, the exact OrbWin instance,
canonical product-home service directory and qualified executable, a positive
bounded PID, and `state: stopped` / `reason: processExited`. Unknown, running,
permission denied, legacy unsupported flags, malformed/duplicate/unknown fields,
foreign identities and cancellation all fail closed. API authentication, malformed
settings and identity errors remain failures when the service is running/unknown;
there is no stderr-string classification or fallback retry. Successful warm
settings reads preserve compatibility with the current legacy backend.

The current pinned/installed backend does not expose this typed status interface.
Cold automatic CLI startup is therefore explicitly unsupported with that backend:
use the desktop application's existing startup path. The isolated unregistered
candidate at `artifacts/patch-replay/service-status-candidate/` has full package
tests/build and disposable-home status proofs, but it has not been adopted into
the active backend or qualified with a real supported service generation. Its
diagnostic receipt/marker never replaces rdd lifecycle authority. Do not interpret
this CLI guard as proof that cold CLI startup currently works or as permission to
upgrade/restart the active user Kubernetes cluster.

Cancellation after service-start dispatch ends the client before a settings
re-read or desired-state request. Cancellation after the accepted startup wait
ends before a Docker probe. A failed `set` reports acceptance as unconfirmed for
running and Kubernetes changes: process failure cannot prove the backend rejected
the request. Behavioral tests exercise these boundaries and preserve the original
error for callers. The CLI never retries a mutation or rolls back on these errors.

This gated cold path is implemented and mock-tested, **not live-qualified**. The GUI's
own start/stop adapter is not evidence that this new CLI passed lifecycle tests.

### Bounded future cold-lifecycle qualification

This plan is not permission to stop the current engine. Active user Kubernetes
workloads currently block execution; do not disrupt them for qualification. Root
must assign a suitable lifecycle window and the sole mutation lane before running
the plan. No WSL shutdown, forced process kill, reset, prune, elevation or global
configuration change belongs to the proof.
The typed-status backend must first be adopted through the attended upgrade path
and have run a supported generation; the current legacy backend is not eligible.

1. Freeze the new CLI/alias/source manifest and backend/tool hashes. Verify the
   owned daemon ID and both foreign engine IDs, App policy, controller arguments,
   WSL registration identity, current running/Kubernetes settings and all three
   engine container IDs/states, image/volume/network inventories. Hash global
   Docker/kubeconfig, product config and registration records. Refuse a lifecycle
   window containing active user workloads unless their interruption is explicitly
   accepted; do not infer consent from the Phase2 scope or registry fixture tests.
2. In the granted window, create only a nonce-labelled cached-image fixture and
   exact named volume with a content marker. No pull, credential access or
   unrelated resource adoption. Record its identity and data checksum. Confirm
   Windows/WSL clients have no in-flight operations before requesting `stop`.
3. Exercise the built CLI's warm start/idempotence and explicit stop. Observe
   backend desired/actual state with bounded upstream reads; an accepted request
   alone is not stopped-engine evidence. Before testing a cold management process,
   confirm the owned engine is stopped. Use the existing qualified rdd service
   mechanism, never an alternative process supervisor. Upstream service stop has
   an internal hard-kill fallback on timeout, so any such failure ends the test;
   retain evidence and do not add an independent force-kill/retry.
4. With the management API actually unavailable, verify `status` and `config get`
   fail without auto-start and a new snapshot reports unavailable. Invoke the
   CLI's `start` once. Verify manual PATH policy, restored management/owned Docker
   readiness, original daemon identity and retained fixture data. An uncertain
   start is a failed proof requiring observation, never automatic rollback.
5. Restore only the original supported desired settings through the qualified
   upstream path. Verify original Kubernetes capability/state and fixture data,
   then clean only exact guarded fixtures. Compare all foreign inventories/states
   and global config bytes; account for any owned system workload recreation
   explicitly. Retain command receipts/timings and any failure rather than claiming
   byte-identical engine internals across a restart. Release the lane only after
   restoration and inventory checks, or report unresolved state to root.

Cancellation after acceptance remains a separate fault-injection gate; use mock
coverage until a dedicated safe window permits it. This plan does not qualify
login start, tray actions or unattended reboot behavior.

The pinned source contract is
[rdd service command](https://github.com/rancher-sandbox/rancher-desktop-2/blob/af6511ad4e065e24e7258f575a0fc090c10cdbda/rdd/cmd/rdd/service.go)
and [service process owner](https://github.com/rancher-sandbox/rancher-desktop-2/blob/af6511ad4e065e24e7258f575a0fc090c10cdbda/rdd/pkg/service/cmd/service.go).

The Docker wrapper deliberately uses OrbWin's private client config and credential
helper. Selecting the wrapper is an explicit opt-in to allowing launched Docker,
Compose and supported Buildx clients to request saved OrbWin registry credentials.
The same binary is built as `orbwin-docker.exe`; its executable basename dispatches
directly into Docker mode. VS Code's executable-only `dev.containers.dockerPath`
may select the absolute alias path without arguments, global environment edits,
context changes or credential copies. Clients launched through this alias have
the same private credential access as the explicit wrapper. Ordinary unrelated
Docker/VS Code clients retain their existing credential visibility. Raw output of a
user-requested Docker command can contain private workload data and is not a support
snapshot. The CLI does not claim credential-free arbitrary Docker output.

## Completion

Dot-source `completions/OrbWin.ps1` to register completion for the current
PowerShell session. The script provides literal command/config/Docker verb choices;
it does not run the CLI, inspect engine/container names, read credentials, execute
completion input, write the profile, or alter PATH. A user may deliberately place
the dot-source command in their own profile; OrbWin does not do that implicitly.

## Build, checks and current evidence

`node scripts/build-cli.mjs` uses the pinned Go compiler and produces only
`artifacts/cli/orbwin-cli.exe`, the identical `orbwin-docker.exe` alias, and
`cli-manifest.json` containing both hashes. It compiles upstream commit,
Docker checksum and application version from current locks, records source and
binary hashes, and refuses source changes during the build. Package integration
must copy both executables beside the GUI and the manifest/completion into the installation;
it must not register PATH or completion automatically.

Run `go test ./...` and `go vet ./...` in `backend-tools/cli` using pinned Go.
Tests cover target-override rejection, literal argument preservation, inherited
environment removal, snapshot allowlist, supported config keys, read/output bounds,
client cancellation and nonzero exit handling. `scripts/qualify-cli-completion.ps1`
checks five actual PowerShell completion cases without profile edits.

`node scripts/qualify-cli.mjs` stages the CLI in a unique artifacts directory with
a junction to the existing installed runtime. It only reads status, management
settings and Docker info, creates a new snapshot, proves snapshot overwrite and
unsafe-command refusal, and compares global Docker/kubeconfig plus private Docker
config hashes. It does not mutate engine lifecycle, configurations or workloads.
The development run on 2026-10-25 produced
`artifacts/cli/qualification-021b80e9-4990-4c02-acb5-d1b9fcffacea/proof.json`:
the expected owned daemon answered, projected settings were boolean, five completion
cases passed, the executable-only Docker alias selected the same daemon, and all
three client configuration hashes remained identical. Buildx version/default
inspect also passed without bootstrap. Inspect reports endpoint name `default`,
not its pipe; a separately explicit Docker context inspection proved that this
default resolves to the owned pipe. This matches Buildx v0.38.1's
[default node-group resolution](https://github.com/docker/buildx/blob/v0.38.1/store/storeutil/storeutil.go)
and [context-to-client resolution](https://github.com/docker/buildx/blob/v0.38.1/util/dockerutil/api.go).

The locally installed Dev Containers 0.469.0 compiled CLI
`dist/spec-node/devContainersSpecCLI.js` has SHA-256
`e2051ce3598a26b11d29048a6dd3252ae8d3b056b413c0d53ba1fd3a56ec1b74`.
Inspection found literal `buildx version` and `buildx build` invocations and no
literal inspect/ls/create/use or `--builder` invocation. This source inspection
supports the selected wrapper commands, but does not replace a real Dev Containers
build/open/Compose/private-registry journey. Future extension updates must be
rechecked. Custom builder management remains an explicit unsupported wrapper path.

## Actual Dev Containers CLI journeys

`scripts/qualify-cli-devcontainers.mjs` requires explicit sole-mutation-lane consent
and only uses new nonce fixtures. It pins all three daemon IDs, uses cached Alpine
Linux amd64 with `--pull=never`, and names every fixture with a cryptographic nonce.
The Windows fixture is outside the repository. Workspace Git-root detection is
disabled and the configuration explicitly binds only the fixture directory and a
separate fixture file. No root/user-home/repository parent is mounted. The WSL
variant verifies Ubuntu machine ID, UID 1000 and both recorded source/shared root
inode signatures, then creates one nonce child in the already registered proof
project. It reuses the existing bind export and does not register a project, alter
mounts, start/stop an engine or modify WSL platform state.

The first actual invocation exposed a wrapper bug: a blanket context-short-flag
filter rejected `/bin/sh -c` after the image. The failed proof is retained at
`artifacts/cli/orbwin-devcontainer-598bde11572346cb95583a84d563d372/proof.json`.
It created no container; its exact cached-image alias was removed and all inventories
were restored. The pinned option-boundary adapter and adversarial tests fix that
observed failure, rather than disabling context protection.

The fixed wrapper SHA-256
`5eff66fa5ebaa4785168e27ad5817338bdd784af465a560a90789d92bf76185c` passed:

- Windows: `artifacts/cli/orbwin-devcontainer-b8bdc07b01ce4a59aedb2a06e176f696/proof.json`.
- WSL registered project: `artifacts/cli/orbwin-devcontainer-185fd6b1aa324bf8a040b46e5732bfcc/proof.json`.

Both used the real extension CLI for `up`, `exec` and reopen with
`--expect-existing-container`. Container writes through the directory and separate
file mounts were read back on the actual Windows/WSL host, and reopen retained the
same container ID. Cleanup rechecks daemon identity and exact token labels/image
reference before removing only nonce containers and the temporary alias. Owned
container/image/volume/network inventories exactly matched their before snapshots;
all three daemon IDs and global Docker/kubeconfig/private Docker bytes stayed
unchanged. WSL registration bytes also stayed unchanged. New host fixture files and
per-command stdout/stderr are retained with receipts; no credentials were used.
The mutation lane was released after both guarded cleanups.

This establishes a real basic Dev Containers CLI bind/write/reopen journey. It
does not establish interactive VS Code editor UI, non-root UID adjustment, Compose
builds, private pulls, arbitrary features/extensions, lifecycle or installed package
qualification. Later feedback wording, checksum-schema guard and short-cluster
defense have separate binary identities; this is not a claim that every later
binary ran these exact live journeys.

The later cached-image Compose proof found a real pinned-client compatibility gap:
Compose5.6.1 Bake omits a read grant for the Dev Containers generated Dockerfile
outside the build context. `COMPOSE_BAKE=false` did not change that. A child-only
TEMP/TMP-inside-context experiment passed build/up/exec/write/reopen and exact
cleanup; it retains about3.87MB/two BuildKit cache records and does not close the
ordinary default flow. The isolated exact-file Compose patch, provenance, tests,
receipts and remaining installed qualification gate are recorded in
[patches/compose/README.md](../../patches/compose/README.md). No broad filesystem
grant, runtime pin or installed tool was changed by the experiment.

The isolated patched Compose Windows candidate subsequently passed real Dev
Containers build/up/exec, host directory/file writes and reopen with ordinary
TEMP/TMP and exact cleanup. Receipt:
`artifacts/cli/orbwin-devcontainer-fbfa2aa3f30e468284a78533a27c420f/proof.json`.
It preserves all three engines' inventories/container states and config bytes;
BuildKit cache remained28→31 records,53.61→53.64MB. A harness-only exact version
probe rejection selected Dev Containers' standalone `--docker-compose-path`
fallback; actual installed/default plugin discovery remains unqualified. The
isolated recipe now builds both Windows/Linux amd64 candidates with per-platform
compiled-source, binary and legal-file manifests; neither is installed by it.

Required remaining evidence: attended cold management-service startup, start/stop and
Kubernetes toggle with preserved data/three-engine coexistence, installed resource
layout, default plugin Compose/interactive/credential passthrough, cancellation after
accepted mutation, missing/offline/wrong-identity runtime, and installed keyboard/
terminal behavior. No engine lifecycle mutation or Windows elevation was performed
by this qualification. Passing read-only qualification does not close these gates.

## Typed management source adoption

Root adopted five reviewed CLI source files and registered upstream patch0013
through the guarded source-only recipe. The [adoption receipt](../../artifacts/patch-replay/cli-typed-status-integration/source-adoption-aa800032-f231-43be-80be-48721f39214e/proof.json)
retains exact before-byte backups and pending-stage accounting. Source lock SHA is
`75f32067af2aa14b2b35516b1930a5dd12479fa8893f01e57568181d2c004d69`.
The [candidate review](../../artifacts/patch-replay/cli-typed-status-integration/adoption-review.json)
records34 top-level CLI tests/vet, six focused upstream tests/package vet, both
isolated builds and ten actual JSON command-fixture cases with retained bytes.
Actual fixtures return before process inspection and address only new disposable
homes. They do not qualify current-service lifecycle.

The registered patch's LF serialization correction and clean13-patch replay are
recorded in the [adaptation record](../../patches/rdd/README.md). Read-only JSON
status never invokes the historical PID-cleanup path. Duplicate, unknown or
case-aliased receipt keys at root/nested identity levels refuse before a process
probe. File identity/size/mode/mtime are checked before and after bounded reads,
then PID/receipt bytes are reread after process observation. These are diagnostic
checks, not content/inode immutability or hostile same-user protection. The
upstream Windows diagnostic marker retains its Local session namespace: another
session without accessible marker evidence is unknown, never guessed stopped.

Warm legacy successful Docker/settings reads remain working. The currently
qualified backend binary remains
`0896047decdfa7c851870e80b5edc51439f243b8baa522379b4933c60e15c8f6` and has not
been replaced; it does not implement this new typed command. Consequently cold
startup on that backend remains explicitly unsupported/manual-desktop, rather
than bypassing admission. Even after source adoption, first cold startup cannot
be qualified from absent legacy receipt/PID alone. A supported generation must
have actually run, followed by attended generation/exit/start/readiness and
three-engine/data-preservation checks. No current engine was stopped for these
source checks.

The adopted CLI subsequently passed all34 top-level tests and vet. Focused tests
in the real active upstream packages passed seven top-level cases, including only
the test process's own diagnostic identity/marker (no engine/service observation),
and package vet passed. Tests used an isolated disposable home and existing
offline module caches. The normal clean13-patch source replay passed; neither
backend binary nor runtime identity/home was rebuilt or changed.

The first normal CLI build produced a different hash from the isolated candidate
because Go stamped the surrounding checkout's VCS revision (product repository
versus upstream repository). Exact source/module byte hashes matched. The approved
one-flag recipe fix, `-buildvcs=false`, removes that incidental checkout metadata;
manifest sourceCommit, exact source/module hashes and the pinned Docker hash retain
provenance. Active and isolated builds with identical flags now match byte-for-byte,
as does the executable-only Docker alias:
`f624bc96a97d5e808453e35d01dac50f531382729e94f323934d40dcf240f38b`.
CLI manifest SHA is
`db9af987ec56c90d5c57d21b74817b8e46b1cb9a3db069a99f9c26cf7b0b2745`.
The original isolated6ebeb4... candidate, first active3f98... binaries, failed
replay and interim receipts remain retained. The [comparison receipt](../../artifacts/qualification/p2-cli-typed-adoption-c2891d70-a4aa-4543-bee2-8db85ca56d83/reproducibility.json)
and [adopted-source proof](../../artifacts/qualification/p2-cli-typed-adoption-c2891d70-a4aa-4543-bee2-8db85ca56d83/proof.json)
bind the final hashes. These are build/source proofs, not a new installed or live
cold-start qualification for this CLI binary.
