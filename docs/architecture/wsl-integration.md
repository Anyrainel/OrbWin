# WSL integration boundary

## Selected project tools (implementation in progress, 2026-10-25)

An explicit Open project action may hand the last inspected project to Windows
Explorer or an existing Microsoft VS Code installation. The native adapter must
hold the existing project-record lock, match its exact source/inode signature,
return the inspected WSL 2 registration GUID with project status, and check
that exact GUID (before the first guest invocation) and the
canonical source as the distribution's ordinary user before launching a fixed
program. It must not connect an export, start an engine, install an editor or
extension, alter trust/settings, copy credentials or configure/forward an SSH
agent. Native tools retain their ordinary user environment and capabilities.
Launching WSL can start the selected distribution. The user-owned editor/file
window survives OrbWin's window. Launch acknowledgement is not proof the remote
extension opened successfully; ordinary client trust/extension prompts remain
the user's decision. Name-based OS Explorer/VS Code handoff cannot reserve a
registration against external changes after dispatch.

The initial selected address grammar for the editor authority is an ordinary
ASCII WSL name (letters/digits/dot/underscore/hyphen). Project URIs encode UTF-8
path bytes. Explorer additionally refuses Linux filename characters that cannot
be represented unambiguously in a Windows UNC path, including backslashes and
alternate-stream colons. Unsupported names/paths fail visibly and retain the
project; no generic shell association or fallback interpreter is used. Linux
preflight uses a fixed `/usr/bin/env -i` environment, `/bin/sh` and absolute
`/usr/bin/id`, `readlink` and `stat`; a distribution missing these tools fails
visibly. Program directory paths must be drive-absolute before any WSL/native
launch. The edited disconnected source invalidates the previous inspection.
Record and lock nodes must be ordinary non-reparse files, with the record
denying write/delete sharing during its bounded read. Host registry name/version
reads use fixed-size native buffers and at most 512 registrations. The existing
project's record bytes are not upgraded or rewritten by this flow.
Explorer also refuses the superscript-digit COM/LPT device names listed in
[Microsoft's filename rules](https://learn.microsoft.com/en-us/windows/win32/fileio/naming-a-file),
including when followed by an extension; these remain valid URI data for a WSL
editor but cannot be safely represented by the Explorer handoff.

Native development qualification on 2026-10-25 opened the inspected Ubuntu
project through the actual Explorer action, verified the address and four
expected entries, and observed the Explorer window surviving closure of that
OrbWin window. The fixture Explorer was then closed; the installed OrbWin
window stayed open. Receipt:
`artifacts/qualification/p2-wsl-project-explorer-native-20261025.json`.
The retention receipt `p2-wsl-tools-retention-20261025.json` records unchanged
three-daemon identities and private/project/global configuration hashes with no
Windows elevation. Five real Windows filesystem tests passed with no skips:
shared read denies writers/delete, existing writers prevent inspection,
exclusive lock releases on drop, directories refuse, and reparse nodes refuse
without changing their target. Native Code/WSL extension, installed behavior and
the remaining cancellation/failure matrix are still unqualified.

Phase 1 uses WSL's existing shared `/mnt/wsl` filesystem and a Unix client
relay at `/mnt/wsl/orbwin/docker.sock`. It reaches the existing Windows Docker
adapter through mutual TLS inside rdd's private gvisor network. There is no
public Windows TCP listener, new engine, or desktop lifecycle supervisor. A separate `orbwin-proof`
path is reserved for sequential qualification. Other engines' paths and user
shell defaults remain untouched.

The systemd drop-in retains the pinned upstream `wsl-exec dockerd` invocation,
runtime and `DOCKER_OPTS`, with a root-only internal Moby socket. Both the shared
socket and `/var/run/docker.sock` are served by the relay, including when a
workload explicitly receives the latter socket. This ensures Windows port
allocation/publication checks also apply to WSL and delegated Docker clients.
The public Unix endpoints inherit upstream's local socket permission policy. Changes to upstream's
invocation require review when updating the pin. An inactive endpoint is an error,
never an automatic switch to another engine. WSL distributions controlled by the
same Windows user share a privileged trust boundary; this does not isolate a
malicious root in another distribution. Socket access inherits upstream modes.

Selected user distributions may use the pinned Linux tools with an explicit
endpoint. Linux projects need an explicit directory exported under the owned
shared path; a native Linux `/home/...` path has no meaning in the separate
engine distribution. Phase 1 must display/copy the actual shared bind source.
Transparent path mapping and shell-default integration remain later conveniences.
Project sharing must be opt-in, limited to an existing selected directory, verified by
mount identity, and removable without deleting its source. Repeated operations
must verify existing state rather than stack mounts. Keep a user WSL shell open
while using its project export. WSL can remove shared bind mounts when that
distribution exits; reconnecting verifies/recreates the export. No global WSL
restart or hidden distribution keepalive is used.

Rejected proof implementations: a bind of `/run/docker.sock` first raced Docker's
replacement of its socket activation inode. Ordering it after Docker fixed that
race, but a separate proof showed WSL removes the bind when Ubuntu exits while
the oneshot still reports active. A direct dockerd listener survived that exit,
but bypassed Windows host-port adaptation. Patch 8 supersedes it with the shared
authenticated relay described in [Windows published ports](windows-published-ports.md).
The project connection's observed mount state is always rechecked.

Qualification must exercise an ordinary non-root Ubuntu user, actual Linux
Docker identity, Linux-directory read/write visibility, Windows-drive binds,
Compose/build, socket restart, disconnect and preservation of other engines'
paths/configuration. Proof experiments are not a supported integration until the
native adapter and its ownership checks pass.

Opening a user distribution validates its current registered name and refuses
known engine distributions. The preferred interactive host is the installed
Windows Terminal execution alias, launched with an explicit new window and a
fixed WSL executable; no command interpreter is used. WT's separate semicolon
action separator is refused in distribution names. Ordinary Windows argument
quoting is covered by round-trip tests against CommandLineToArgvW. The fallback
Windows shell launch detects immediate WSL exit and reports the copied-command
recovery, rather than asserting that a terminal opened. The terminal client
belongs to the user and is never put in OrbWin's kill-on-close job.

Reference: [Windows Terminal command-line interface](https://learn.microsoft.com/en-us/windows/terminal/command-line-arguments).

## Phase 2 original project paths (implementation contract)

The existing selected-directory export remains the mount owner. A short-lived
Linux workflow adapter will consume an adapter-built request, verify the original
directory and shared mount device/inode against the recorded signature, and run
as the selected distribution's normal default user. It must refuse a disconnected
or replaced source instead of reconnecting or mounting anything implicitly.
The registered daemon ID and pinned tool/private-config paths are supplied by
the host boundary; no global shell, context, PATH or distro configuration changes.

Compose file sets and project directories are selected using original Linux
project paths or paths relative to that selected root. Resolve and verify them
inside that root, then use the corresponding shared paths. Run the pinned Compose
`config --format=json` to resolve the ordered file set, interpolation and profiles.
Keep that result bounded in memory, never in a temporary file or diagnostic log.
Rewrite only documented filesystem fields: bind sources, local build contexts
and Dockerfiles/additional contexts, and file-backed secrets/configs. No arbitrary
application/environment strings are changed. Local sources outside the selected
directory refuse with useful paths; relative sources resolve through the selected
project directory. All bind sources must exist with the expected file/directory
type; never create a missing host source as a guessed convenience.

Dispatch the resolved configuration over stdin to the same pinned Compose client
with an explicit project/directory/profile/service action and owned endpoint.
Preserve Kubernetes/other engines and the source files. Project input metadata
must remain independently reusable because stdin Compose configuration does not
provide an original config-files label. Build streams the selected ordinary Linux
context through the pinned Docker client. Source identity and daemon checks occur
before dispatch; cancellation is client cancellation, not rollback of accepted
engine mutations. No background distro keepalive is introduced. Qualification
must separately cover non-root users, space/Unicode/link paths, relative/absolute
binds, native changes, permissions, missing/replaced/disconnected sources, Compose
profile/service choices, builds and reconnect after a real distro exit. This
contract does not itself count as observed or accepted behavior.

## Reviewed migration handoff

Migration source socket access and project ownership are independent choices.
The source-project dialog lists ordinary WSL 2 registrations separately, with no
automatic project-distro selection. Explicit inspection may start the selected
distribution, but invokes only the existing export inspection; it never connects,
disconnects, replaces a project or starts a terminal. An absent/disconnected
export points to the existing WSL project connection workflow. Native inspection
checks the selected registration/name/version before and after the observation.

The handoff offers only Compose Validate and retains the reviewed registration
and exact original export root and device/inode signature in its typed request. Immediately before workflow
preparation, the native adapter verifies that registration still names the chosen
ordinary WSL 2 distro and that the saved export still has the reviewed root and signature. The
child launches by registration ID as that distro's normal default user. Existing
Linux source/export inode and mount checks still run before any Compose command;
connection review is removed from the wire request after native enforcement.
All current frontend Compose/build entry points retain the reviewed root and
signature, including relative-path group/service actions and connected-page
builds. Raw legacy IPC requests without a connection review still explicitly use
the current selection; this compatibility path is not used by these frontend
flows. New Compose resource labels retain the signature. Older labels without
that identity remain recognizable, but their Compose shortcuts are unavailable
with a direction to inspect the intended project in WSL distributions. Ordinary
container actions remain available. Reopening a workflow never silently upgrades
old metadata to a newer selected root.

Only coherent ordered Linux labels entirely inside the selected root produce file
suggestions. Missing/conflicting/outside-root/Windows paths and explicit env-file
labels leave an empty editable Linux file set. Profiles/services are not inferred.
Files are read anew on submission. A connection change, absent registration or
disconnected/replaced mount refuses without implicit reconnect or fallback.
The existing export owns mount lifetime and no UI/backend supervisor is added.
Same-user WSL/root remains a shared platform trust boundary, not isolation against
a malicious local administrator editing registration records or mount metadata.

The subsequent Compose env-file input uses the same reviewed connection and
regular-file mapper. Explicit interpolation files must remain inside that root;
ordered paths alone may be retained in reusable labels. Native absolute/relative
validation and missing/reversed-file refusals are recorded in
`p2-compose-env-wsl-native-20261025.json`; see
[Compose qualification](compose-projects.md) for scope and remaining gates.

The first native handoff refused because the shared private WSL config pointed
to the installed Phase 1 app's public tools rather than the development path.
That evidence is retained in `p2-migration-wsl-native-first-refusal-20261025.log`.
Workflow preparation now permits one existing Windows-drive plugin directory
only after canonical-path validation and hashing every pinned Linux tool. A
different directory is not version compatibility by inference: all bytes must
match the current lock. Multiple roots, non-drive paths, missing/changed assets
and traversal aliases refuse. Large hashes run on a blocking worker; a cancelled
prepare can leave only that bounded read-only task, with no workflow spawned
before successful completion. The shared config is never rewritten by dispatch.

`artifacts/qualification/p2-migration-wsl-20261025.json` links the native adapter
and separate desktop UI evidence. The final native run validates two original
Linux files with space/Unicode names as UID 1000, refuses a changed reviewed root
and absent registration, verifies GUID-based helper launch and removal of the
review-only fields from the Linux wire request. A real stopped Rancher Compose
fixture separately supplies the UI labels. The UI requires explicit project-distro
selection, inspects the existing export and completes quiet target validation.
Source container/volume identity and stopped state are checked before exact
non-forced cleanup; no target containers are created. Only the test's exact files
and empty subdirectories are removed from the existing qualification project.

The private config and project record hashes, original/shared inode signatures,
three daemon IDs and global Docker/Kubernetes configs are preserved. The installed
Phase 1 window remains open. 68 JavaScript tests, 18 operation unit tests, three
WSL-integration unit tests, the opt-in native adapter proof and development builds
pass. Native UI captures precede the final read-only hash-worker/canonical-path
change, status-announcement attribute and model-only path checks; the final native proof and unit/build
checks cover those deltas. Different project/socket distros are covered by model
tests, not a real two-user-distro installation. Installed/offline, large-group,
source-replacement/cancellation matrices and combined application migration remain
required.

The subsequent `p2-wsl-root-retention-20261025.json` qualification covers the
shared root/signature contract. Native quiet validation succeeds for ordered
relative Unicode filenames as UID 1000. Changed roots, changed signatures at the
same path, missing migration registrations and a stale build review refuse
before workflow dispatch. This is snapshot-drift injection, not a destructive
replacement of the live project. Linux unit tests verify the emitted label;
frontend tests verify frozen inputs and refusal of missing/malformed identities.
The first fixture used the relative directory twice and correctly failed path
resolution; the corrected fixture uses file paths relative to its working
directory. Both exact fixture directories were removed. Source/shared inode,
private/project/global configuration hashes and three daemon identities remain
unchanged. This adds no installed or new native UI qualification; the disabled
older-label shortcut and complete source-replacement/cancellation matrix still
need product-level coverage.
