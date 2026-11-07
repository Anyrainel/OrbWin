# Go CLI transient process ownership

Transient clients use a native Windows creation adapter. The exact detached
service Start retains its independent lifetime. The adapter treats `exec.Cmd`
as path/argv/environment/stdio configuration; it never calls `Cmd.Start`,
`Cmd.Wait`, reopens a PID or inspects private `os.Process` fields.

One CLI writer admission owns a noninheritable kill-on-close job. Creation uses
`STARTUPINFOEXW`, `PROC_THREAD_ATTRIBUTE_JOB_LIST` and an explicit three-handle
stdio list. The job therefore contains the child at creation, before the
suspended initial thread can execute. There is no assign-after-create fallback,
breakaway flag or process-wide job. The actual returned process and thread
handles remain owned; initial suspension must be exactly one before Resume.

Streamed `*os.File` stdio uses inheritable duplicates without taking ownership of
the caller's files. Nil stdio uses owned NUL handles. Other readers/writers use
64-KiB anonymous kernel pipes and 32-KiB copy buffers. Only child endpoints are
inherited. Copy workers start after atomic creation and before Resume; output
writes are serialized to preserve shared-writer safety. Copy errors request job
termination and wake root observation. No caller Reader/Writer can supply a
positive join merely because its kernel pipe closed.

One owner waits the returned root handle. On root completion, cancellation or IO
failure it terminates the owned job, positively observes root signal and job
ActiveProcesses zero, joins all IO, then checks every resource close. Retirement
has a two-second deadline after the first wait. Uncooperative arbitrary caller IO,
failed query, deadline or unknown Close latches denial and retains strong owner
references, including file wrappers; future dispatch is refused. Unknown closes
are never retried. Before a successful creation, partial IO is closed and joined;
there is no fictitious root-retirement token for a failed start.

`runClientWriter` binds an opaque writer admission nonce to a private single-use
`retiredClientTree`. Only complete positive retirement mints it. Nonzero child
exit or a joined copy error remains an operation error but may accompany a valid
retirement token, allowing config settlement independently of command success.
The config guard consumes that token against the exact admitted nonce. There is
no boolean/JSON retirement authority. A missing token keeps the guard denied.

A distinct sealed `neverStartedClient` can accompany a typed `writerStartError`
when the actual producer proves no CreateProcess success and known precreation
cleanup. It is removed from the error once and consumed against the exact nonce
once. The guard independently verifies original native file identity, descriptor
and unchanged base bytes before aborting the writer intent. The admission's
shared claim state is consumed before context or creation checks; copying its Go
interface/value cannot admit another writer or mint another no-start grant.
Unknown cleanup and any successfully created process receive no such grant.

The closed transient adapter rejects token/parent-process/custom-inheritance,
creation flags, extra files, relative executables and embedded environment NUL.
It accepts normal caller stdio and hidden-window configuration. Environment
encoding is bounded to 1 MiB. The CLI admits one native tree at a time; the
closed setup owns fewer than sixteen native handles, with no unbounded retries.

Pure fault tests cover cancellation rewait, root/tree/IO/Close uncertainty,
nonzero exit and copy errors, nonce mismatch/replay, exclusive admission and IO
join semantics. Go ABI assertions and MinGW C header static assertions qualify
the source layouts/constants. No actual process/job/pipe execution was performed
for this source record; native Windows lifecycle behavior remains a separate
qualification gate.
