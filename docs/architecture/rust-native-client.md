# Rust atomic transient client

`native_client` accepts only a retained `CachedExecutable`, explicit arguments,
a complete environment and optional absolute working directory. Its constructor
consumes the non-Clone configuration `WriterAdmission`; a second specification
cannot borrow the same writer nonce. There is no serialized specification,
ambient environment merge, shell command construction, detached service or native
application launch in this class. Interactive TTY ownership remains in the SDK.

The producer creates a noninheritable kill-on-close job and three private named
pipe pairs. Every name has a fresh 256-bit system RNG nonce. The server is an
overlapped first-instance local byte pipe with remote clients rejected, a
protected DACL containing only the actual ordinary caller SID, SYSTEM and
Administrators, and a native owner/ACE recheck. The synchronous child endpoints
alone are inheritable. Both ends must report this desktop's actual process ID
before any child exists. Parent endpoints become Tokio `NamedPipeServer`s.

Creation uses `CreateProcessW` with exact application name, quoted UTF-16 argv,
Unicode environment, `CREATE_NO_WINDOW`, `CREATE_SUSPENDED`, `STARTUPINFOEXW`,
`JOB_LIST` and an exact three-handle `HANDLE_LIST`. There is no assign-after-spawn
fallback or bare PID reopening. The returned process/thread handles are retained.
While suspended, `ClientJob::from_atomic` independently checks membership and job
limits, and the held cache verifies the returned native PID, creation time,
current session and actual image. Child endpoints close with checked results;
the initial suspension must be exactly one before Resume.

`wait` observes the returned process handle with nonblocking native polls and a
Tokio timer. `kill` requests termination of the exact retained job. `try_wait`
and `id` never confer tree-retirement authority. After root exit,
`into_root_exited` transfers job, original process, cache lease and admission
permit together into `NativeRetirement`. The sole caller worker must positively
join every task that took a stdio endpoint before invoking `retire`. Dropping
stdin after writing is required to deliver EOF. Only whole-job retirement,
checked explicit handle cleanup and a final cache recheck return
`RetiredClientTree`. Parent Tokio pipe Drop uses Tokio's normal close contract;
its underlying Close result is unobservable and is not writer authority.

Unexpected Drop, panic during retirement, ambiguous Tokio registration or failed
checked Close retains the actual bundle/cache and latches admission denial.
There is no cleanup thread or uncertain Close retry. Admission is bounded to 32
owners; retaining an unknown owner keeps its slot occupied and blocks new births.
The process may be requested to stop, but that request does not prove Wait.

`NativeSpawnError` separately reports the diagnostic `no_child_created` fact and
may carry a sealed non-Clone `NeverStartedClient`. Only the actual producer can
mint it after no successful CreateProcess and known cleanup with no remaining
creation continuation. Any successful creation is privately latched before PI
handling, so a later failure cannot mint it. An opaque cancellation handle lets
the worker request cancellation while continuing to await spawn. Arbitrarily
dropping that future retains unknown ownership instead. The config guard must
consume the exact writer-bound grant and independently verify its original file
ID, private descriptor and unchanged base bytes before aborting the writer intent.
Neither a Boolean diagnostic nor absence can substitute for the grant.

Pure tests cover UTF-16 quoting/environment framing and rejection, native ABI,
positive versus unknown owner release, unexpected Drop and panic. The spawn
future has a compile-only Send assertion. Native creation, pipes, parent-loss
behavior, image verification and cancellation require separate frozen Windows
qualification; none were executed for this source record.
