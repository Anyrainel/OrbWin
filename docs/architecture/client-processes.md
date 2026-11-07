# Desktop client process ownership

The desktop owns transient clients, not the rdd service, guest or workload
lifecycle. `operations::run_client` owns its child, stdin writer and both output
readers. Input/output/deadline/cancellation are polled together. Interrupted
clients are killed and waited; output viewer loss also interrupts the client.
Tree-scoped Windows operations hold a non-inheritable kill-on-close job until
the root is reaped and streams have drained. Lifecycle commands intentionally
use the separate root-only path so a UI client does not own the backend service.
Terminal sessions retain their own job and child handles until session teardown.

`process::read_with_deadline` is the bounded diagnostic runner. It closes stdin,
drains stdout/stderr concurrently with a 128 KiB limit per stream and kills/waits
the root on timeout, oversized output or read failure. Dropping its future uses
Tokio kill-on-drop for the root. It does not currently promise descendant-tree
containment. Its deadline starts after synchronous process creation; it cannot
preempt a blocked Windows CreateProcess call. Error text must state the actual
configured deadline rather than always claiming ten seconds.

## Qualification and open boundaries

Four PowerShell-based Rust process tests timed out during simultaneous combined
migration qualification on 2026-10-25. Serial runs and a subsequent default
parallel run passed. Cause remains unproven; do not label this a product timeout
fix or a resource-contention pass. Keep the original PowerShell tests. Add a
native, test-only child with fixed modes to isolate pipe/status/deadline behavior
from shell startup and to retain a Windows process handle before cancellation.
Only freshly created fixture PID files are used, and fixtures have a finite
self-exit fallback. The test child is excluded from production binaries.

The added native tests passed with the retained PowerShell regressions in the
2026-10-25 default-parallel suite: 64 passed, 26 ignored after integration of
inventory controls. The acknowledged children started in 67–191 ms in that run.
These results cover timeout/cancellation root reaping, blocked stdin, output
bounds and nonzero status; they do not resolve the earlier concurrent timeout
cause or establish admission-time descendant containment.

The older Windows runner attaches its job after spawn. This leaves an execution window
before assignment; the existing child-tree test is not proof that a process
cannot create a descendant during that window. Microsoft's documented
[job-list process attribute](https://devblogs.microsoft.com/oldnewthing/20230209-00/?p=107812/)
assigns a job before the initial thread runs. The corresponding
[Rust standard-library attribute API](https://doc.rust-lang.org/std/os/windows/process/trait.CommandExt.html#tymethod.spawn_with_attributes)
is still experimental and is not available as a stable change to the pinned
toolchain. This observation does not justify a nightly switch or custom process
creation without a reviewed design. Admission-time containment and abrupt creator
loss remain qualification/design work; a passing orderly teardown test does not
close those boundaries.

## Atomic cached diagnostic integration, 2026-10-26

`engine::command` now obtains an immutable native private config generation and
passes its retained capability into `client_worker`. The worker uses
`native_client`'s creation-time job and exact inherited handle list. It replaces
inherited Docker, BuildKit, plugin and config selectors with owned values. The
newly selected Docker CLI patch closes ambient plugin discovery, including
metadata and hooks; its binaries have not yet been staged or executed.

Caller cancellation drops a signal sender, while a separate Tokio worker keeps
owning creation and cleanup. Cancellation during spawn signals the producer and
continues awaiting that same future. The worker polls both bounded output
futures and the actual root handle concurrently. Output futures have no separate
tasks and relinquish their endpoints before retirement. Interrupted clients need
positive root exit and actual whole-job retirement; a termination request alone
does not release the config generation. Unknown ownership retains the generation
and native process owner. Ordinary service/lifecycle launches remain separate.

The complete module compiles and pure selector/environment tests pass. A review
found Windows hidden drive keys such as `=C:` were rejected; the parser now
preserves only that documented key form and tests malformed and duplicate forms.
Native atomic creation/IO/cancellation qualification, streaming operations,
registry writers, terminals and setup integration remain open. This source
integration does not qualify the older runners or a deployed product.

## Credential worker and writer cutoff, 2026-10-26

`credential_writer` is a separate source worker for an admitted config loan.
It owns native creation, secret stdin, bounded discard of both output streams,
root waiting and storage settlement. It never returns child output or underlying
pipe error text. Caller loss signals the worker rather than dropping creation.
Cancellation and deadline take precedence over ready work; a final check before
settlement defines its cancellation cutoff. After storage admission the blocking
owner must finish, even if its UI request disappears. Cancellation does not roll
back authentication or a vault write already sent.

Only producer-certified NoStart can abort an unchanged loan. Observed timeout,
cancellation, IO failure or overflow retains the intent, even after confirmed
client termination. A normal nonzero CLI exit remains a failed authentication,
and is distinct from interrupted execution. File settlement additionally needs
the held Docker image and exact argv/auth-key admission, scoped raw merging and
the owner-created final file transaction. This source worker is not yet wired to
the registry adapter or qualified with a spawned writer.

The Go CLI producer now withholds writer authority after cancellation, IO-copy
failure, failed termination or a creation error with an owned partial tree.
Confirmed cleanup is distinct from permission to settle configuration. Natural
nonzero root completion can still settle verified state. Pure retirement tests
cover both cases; this does not qualify the native writer/storage intersection.

The execution-only contract review and preserved native timing failures are in
`artifacts/qualification/p2-execution-quiescence-design-20261026/review.md`.
No job-accounting observation alone is a generic pending-I/O or durability proof.

The first spawned writer probes retained a read-only master-file observation
handle through replacement. That observer changed the operation being tested:
classic Windows FileRenameInformation refuses an open destination. Candidate4
failed publication after the in-place child write; candidate6 also prevented the
child's temporary-file replacement, then failed owner publication with actual
NTSTATUS ACCESS_DENIED (0xc0000022). Both receipts and pending intents remain
preserved. These do not establish failure of a normal snapshot reader, which
holds a separate generation file. Subsequent positive tests must record the
master's native ID and close that observation before dispatch; a distinct open
master conflict must assert refusal and retained intent. No broader POSIX rename
or sharing-policy exception is introduced to make a probe pass.

The writer exposes only typed config diagnostics: closed error kinds, constant
stages and numeric native status. Child stdout/stderr and pipe error strings
remain discarded. Capturing the actual rename NTSTATUS replaced an unrelated
Win32 last-error observation, allowing this boundary to be diagnosed safely.

## Atomic SDK terminal ownership, 2026-10-26

`session` now dispatches the actual held SDK code-cache capability from
`Target::session` through `NativeSpec` and `NativeClient`. It does not construct
an executable capability from an arbitrary pathname. The native producer supplies
creation-time job membership and its exact handle inheritance list. No private
Docker CLI config snapshot is required: these terminal paths use the SDK's fixed
Docker endpoint, recorded daemon identity, or the explicitly selected kubeconfig.
The SDK still checks the target container/exec identity or current pod UID.

A detached Tokio worker owns preparation, the same creation future, all three
pipe endpoints, the original root/job/cache capabilities, and the two-session
reservation. Losing the opening request or dropping Sessions signals cancellation;
creation is cancelled and awaited, never abandoned. The registry stores only
bounded control channels and counters, with no worker/registry Arc cycle. Closing
a session does not release admission. Successful startup hands ongoing control
to the session; unknown creation or retirement retains the actual reservation
and native bundle until process exit. Only pre-dispatch failure or positive
ExecutionQuiesced permits ordinary reservation release.

Input accepts at most 8 KiB per request. Atomic nonblocking reservation of at most
two 4 KiB JSON frames keeps worst-case byte-array encoding below the SDK scanner's
32 KiB frame bound; an eight-frame queue refuses a whole write when busy. A pinned
writer preserves partial-frame progress across concurrent reader/root polling.
Output retains a 64 KiB acknowledged window and an asynchronous Notify-based
15-second stalled-viewer deadline. EOF, diagnostics, and root status are observed
independently. Root exit has a five-second pipe drain bound. Interrupted sessions
relinquish stdin for the SDK's close/EOF handling, wait up to two seconds for its
actual root, then request owned-job stop and wait up to five seconds when needed.
Every endpoint is relinquished before native retirement; the original root must
be positively exited and the exact job retired before a slot is reusable.

Source inspection: no-argument `backend-tools/session/main.go` dispatches directly
to `run(stdin, stdout)`; container/debug terminals use the fixed-host Moby client,
and pod terminals use client-go. These paths do not launch the persistent SDK
service or rdd inside the terminal job. Service/lifecycle CLI dispatch remains
separate. Stopping the SDK client is not proof that a container exec process has
stopped. Debug disconnect messages retain the documented disposable-tools expiry
caveat rather than claiming immediate guest cleanup.

The original ignored native TTY and Debug Shell qualification tests are preserved.
The source integration and pure bounded-control/credit/admission checks do not
qualify Windows TTY operation, signals, Kubernetes reconnect, debug cleanup,
cancellation at native creation, or abrupt creator loss. Those native regressions
remain open and must run separately in the authorized owned-engine environment.

## Admission after a failed atomic launch

A creation failure has three ownership outcomes: the producer observed no birth;
the actual created root/job and native resources were positively retired; or
cleanup remains unknown. The first two may release a reader generation, operation
lease or terminal reservation. The third retains them. An error message or the
absence of a successful launch is not retirement evidence.

The native error carries a sealed one-use admission-stop observation constructed
only from those producer outcomes. Each detached consumer explicitly consumes it
before releasing admission. It cannot settle writable data: writer storage still
requires its separate exact-binding producer capability. No renderer flags,
paths, digests or status strings construct either capability. Qualification of
the known-retired creation-failure branch must use actual suspended roots and
checked native teardown before asserting that both terminal slots are reusable.

Root independently reviewed/froze admission candidate 1 and observed all seven
cases pass in 9.272 seconds (native-run-2a257969-8cf4-464c-89a6-3809fba1941a).
It used whole actual native/control/worker modules and exact data-only IPC
excerpts, with no engine bootstrap. Known failures released both terminal slots,
diagnostic generation caps and the operation worker's generic lease payload;
actual retained native owners kept their slots/generations/payloads. The allowed
ten roots remained suspended before user resume. Full SDK terminal, operations
registry/UI journeys and actual syscall-close failure remain separate gates.
