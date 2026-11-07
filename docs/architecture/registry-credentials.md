# Registry credentials

Registry authentication uses Docker's CLI and credential-helper protocol. Reuse
the Windows Credential Manager library already selected by the pinned upstream
credential helper; do not invent encryption or use another engine's store keys.

The selected Rancher runtime dependency is Docker credential helpers 0.9.12,
whose [module pins wincred 1.2.3](https://github.com/docker/docker-credential-helpers/blob/v0.9.12/go.mod).
An initial proof with the older Go dependency's wincred 1.2.2 failed writes with
Windows status 1783. Version 1.2.3 includes
[typed pointers and explicit lifetime retention](https://github.com/danieljoos/wincred/blob/v1.2.3/sys.go)
around the Windows API call. Reuse the selected runtime version; do not patch a
private credential ABI or infer that the older proof qualifies the new dependency.

Each server becomes a deterministic encoded target under `OrbWin/registry/v1/`.
Only that prefix is enumerated, and a matching ownership attribute is required
before returning, replacing or deleting a record. Exact keys avoid approximate
registry matching across applications. Windows owns persistence and protection
for the current Windows user. Writes replace a complete credential atomically;
concurrent logins for the same server use the last successful write. Concurrent
same-user processes can access that user's vault; this is not a privilege boundary
against software already running as that user.

Secrets enter the UI as password fields, cross IPC in a redacted request type,
then enter Docker login through stdin. Never include them in arguments, output,
debug formatting, files or diagnostics. Helper input/output is bounded; parsing
and OS errors use fixed safe messages. The Windows generic credential blob limit
is 2560 bytes; larger tokens fail explicitly. Listing returns only registry names
and usernames. Cancellation may occur after authentication or credential storage;
refresh the actual credential list rather than claiming rollback.

The native private Windows Docker master requires the owned `orbwin` credential
store and rejects foreign helper routes. Current raw parsers preserve correctly
typed inline auth values during writer recovery; they do not establish that
existing private files contain no inline secrets or convert those values into
vault entries. The proposed legacy import explicitly refuses known nonempty
inline secret fields while preserving its source. Ordinary private admission and
the remediation policy remain a qualification gap tracked by
[BL-020](../backend-limitations.md#bl-020--legacy-orbwin-client-settings-need-explicit-private-import).
Registry login acquires its exclusive config owner with the held Docker and
credential-helper images. Its one-use writer admission binds the exact nine
arguments, registry key, owned config path and scoped environment. The detached
credential worker writes the secret only to stdin and discards bounded stdout
and stderr. Natural root completion, joined IO and actual job retirement precede
scoped raw config settlement through the owner-created replacement file. A zero
CLI exit is the only successful login result. Interrupted execution retains the
pending intent even after confirmed termination; cancellation cannot roll back an
already completed authentication or vault mutation.

The UI operation lease follows native execution rather than caller lifetime.
Unknown creation or retirement ownership retains that lease. Positive retirement
releases operation admission even if config settlement subsequently requires
recovery; the config owner separately retains its pending state. Listing and
forgetting invoke the held helper directly through the same atomic native client
worker, without a Docker config snapshot, so private config review does not block
inspection or removal of owned vault entries. Child diagnostics are discarded.
List stdout is bounded to 1 MiB and validated as at most 256 registry/username
pairs before publication. Forget cancellation or failure reports potentially
completed storage mutation and requires refresh.

Selected user-WSL clients reuse the helper through explicit Windows interop;
only registry/helper routing metadata is copied, never secret values. The Linux
client uses the `orbwin.exe` helper suffix and an explicit process PATH containing
the verified Windows helper directory. WSL interop must be enabled; helper failure
remains a client error. Login stages WSL routing before native Windows master
admission, releasing the legacy metadata lock first. Project connection obtains
owned server keys from the actual held Windows helper before taking that legacy
lock, rather than reading the retired Windows config path. Raw WSL metadata
transactions preserve unrelated bytes and reject foreign selected routes. Partial
metadata writes occur before secret dispatch and do not claim a successful login.

This is source integration as of 2026-10-26. Inert native writer probes qualify
only their recorded image/config/execution intersection. Actual Docker login,
Windows vault behavior through this production adapter, WSL interop and composed
cancellation/recovery flows still require dedicated end-to-end qualification.

Use disposable credentials and a scoped local authenticated registry to qualify
login, rejected authentication, authenticated pull/build, application restart,
list/forget and cancellation. Verify that credentials and config belonging to
other engines are untouched. Unit tests cover namespace encoding, foreign-entry
refusal, safe errors and size limits; an explicit opt-in test exercises only
uniquely named synthetic Windows vault entries with exact cleanup.
