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

The private Docker config assigns the OrbWin helper per selected registry,
preserving unrelated keys and explicit user helper choices. Product clients add
only the verified bundled helper directory to their process PATH. No global PATH,
Docker config, login or Credential Manager entry for another engine changes.
Selected user-WSL clients can reuse this helper through explicit Windows interop;
only registry/helper routing metadata is copied, never secret values. The Linux
client uses the `orbwin.exe` helper suffix and an explicit process PATH containing
the verified Windows helper directory. WSL interop must be enabled; helper failure
remains a client error and never falls back to another engine's credential store.
Project connection synchronizes existing product routes; login stages the route
in both private configs before sending credentials. A shared exclusive profile
file handle serializes those writers across OrbWin processes. Partial metadata
writes are safe to retry; no secret or successful login is claimed at that stage.
Explicit edits made concurrently by an external Docker client are outside this
lock; do not edit private config files while product settings are changing.

Use disposable credentials and a scoped local authenticated registry to qualify
login, rejected authentication, authenticated pull/build, application restart,
list/forget and cancellation. Verify that credentials and config belonging to
other engines are untouched. Unit tests cover namespace encoding, foreign-entry
refusal, safe errors and size limits; an explicit opt-in test exercises only
uniquely named synthetic Windows vault entries with exact cleanup.
