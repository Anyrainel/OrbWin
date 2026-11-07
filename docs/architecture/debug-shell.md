# Debug Shell ownership and failure contract

Phase 2 Windows adaptation; implementation and qualification are in progress.
Ordinary exec is retained. Debug Shell supplies the repository's already pinned
Alpine/BusyBox tools in a disposable sidecar without installing anything in the
target image. This is a bounded local Docker feature, not remote fleet debugging.

Only a full engine-returned ID on the verified OrbWin daemon may be targeted.
The target must be running and not paused. The sidecar joins that exact target's
PID and network namespaces, never the host's. No Docker socket, host path, target
volume or writable target root is mounted. Tools can inspect processes/network
and reach the target root through `/proc/1/root` where kernel permissions allow.
Target filesystem access is consequential: the UI explicitly describes it; the
tools do not install packages automatically. Target read-only mounts remain
read-only. No `--privileged`, SYS_ADMIN or host namespace is granted. Capabilities
are dropped except SYS_PTRACE and DAC_READ_SEARCH needed for target inspection.

The pinned helper image must already exist; a missing-image error directs the
user to the ordinary explicit pull flow. The helper root is read-only with a
bounded temporary /tmp. Unique session labels and an exact helper ID distinguish
cleanup from target workloads. Creation conflicts fail without adopting/deleting
an existing container. Docker owns namespaces and container lifecycle; there is
no guest or engine supervisor.

The helper's main interactive shell is attached before start, using OpenStdin,
StdinOnce and AutoRemove. Normal exit removes only that helper. The SDK client
also removes its exact helper on normal error/close paths; an AutoRemove removal
conflict is reconciled by waiting for that exact container to disappear. Actual
qualification showed that stdin-on-disconnect alone does **not** remove this TTY
helper after forced Windows client loss. That failed route is retained as evidence.

The tools therefore hold a 15-second lease in their own tmpfs, renewed every two
seconds by the SDK client only while the exact original target remains running
and unpaused with its original start time. The guest watchdog uses monotonic
/proc/uptime, and kills only the helper's pinned process group on expiry (or at
one hour). It remains a member of that group so the group ID cannot be reused;
it never signals a potentially reused bare parent PID in the target namespace.
Docker then removes the helper and remaining processes. No engine/target lifecycle
is controlled. Client loss or unavailable API can end a debugging session; it
cannot justify restarting or stopping the target. Deliberate user alteration of
helper internals can defeat its lease, as ordinary Docker authority already can.
The UI explains that lost-client cleanup can take up to 20 seconds.

The helper has a 128 MiB memory, 128-process and one-CPU ceiling and a 64 MiB
tmpfs. No host/target directory is used for tools persistence. The pinned tools
are now a verified bundled tag-free image archive, loaded into the owned engine
offline only when its immutable root is absent. Debug uses that root digest and
never pulls a registry image. The obsolete Pull debug tools prerequisite is
removed. Ordinary exec does not need this asset. See
[development data](development-data.md) for cache ownership and qualification.

The existing terminal's two-client limit, bounded input/output, resize, signal,
viewer-backpressure and cancellation handling are reused. A target monitor is a
bounded session client read, not a lifecycle orchestrator. Source/data/package
preservation, shell-less/read-only target access, terminal interactions, target
exit/replacement, forced client loss and cleanup require live tests before this
feature is accepted.

## Durable transient ownership

Before Create, the SDK client flushes an immutable intent header under the
adapter-provided persistent home and holds a Windows exclusive lease file. After
a successful Create it appends/flushes the full helper ID before Attach/Start.
Normal cleanup settles and removes the record. Process death releases the lease;
recovery skips active leases and checks the exact known ID (or recorded unique
name for an unknown Create response), pinned image and all ownership labels.
Mismatch refuses without deletion. An absent known ID can settle; an absent
unknown accepted Create remains recorded for a later retry, so a late commit is
not lost. Recovery never deletes source volumes or guesses targets from labels
without an intent. It runs before opening a new helper or explicitly through
Settings → WSL & system → Recover abandoned tools, bounded to ten seconds and
512 records. No background engine supervisor or broad label cleanup is added.

The owned-engine interruption proof left a helper stopped before Start, verified
the live lease prevented recovery, then released the dead-client lease and
recovered that exact helper. It also retained an absent unknown-create record,
then recovered its late accepted object. The fake API tests reject conflicting
metadata and exercise known-ID settlement. These prove the stated paths, not all
possible storage corruption, OS termination or malicious same-user activity.
