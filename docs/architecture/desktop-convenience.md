# Windows tray and explicit sign-in launch

Source contract, 2026-10-25 Pacific. D03 includes a Windows tray with Open OrbWin
and Quit UI, and explicit per-user “Run OrbWin in the tray at sign-in”. This is
an original Windows daily-flow adaptation. Native and installed acceptance remain
open until reviewed; no sign-in value has been changed by this implementation.

`rdd` remains the backend lifetime and desired-state authority. Tray creation,
opening, quitting, and sign-in registration do not call backend lifecycle APIs,
change its running state, reconnect WSL, or mutate Docker/Kubernetes defaults.
Normal window close retains the existing exit behavior; it is not converted to
close-to-tray. Quit UI exits this application, leaving healthy workloads owned
by the independent backend.

Manual launch shows the window. The exact `--background` launch mode hides the
initial window only after a tray was successfully created. Missing icons, tray
creation failure, malformed arguments, and hide failure retain a visible window
with an observable diagnostic. Open OrbWin shows, restores and focuses the main
window. The tray is owned by the Tauri application and is removed on UI exit.

The only persistent setting is the current user's fixed `OrbWin` value under
`Software\Microsoft\Windows\CurrentVersion\Run`. Its sole accepted owned content
is a REG_SZ containing the quoted native current executable and the fixed
`--background` argument. Renderer input is a boolean, never a path, argument,
registry key or value name. A foreign type, malformed/oversized bytes or any
different command refuses overwrite and removal; values are never printed.
Enabling an absent value, disabling that exact owned value and repeating either
operation are the only transitions. No HKLM, PATH, profile or startup-folder
edits are involved. The native registry read is bounded.

Changes use ordinary HKCU value APIs as the sole implementation. An earlier
KTM-only candidate failed with Windows6801 before any successful fixture stage;
its frozen source/binary/receipt are retained. There is no runtime fallback or
transaction-manager repair. Cooperating OrbWin copies and maintenance mode share
`Global\OrbWin.Startup.v1.<current-token-user-SID>` across Windows sessions.
CreateMutexExW requests only SYNCHRONIZE and MUTEX_MODIFY_STATE; a new object's
DACL grants those rights to the current SID and SYSTEM. Existing objects are not
reconfigured. An unavailable/incompatible lock fails closed, without an unlocked
path. Wait is bounded to5000ms; abandonment releases ownership and returns an
interrupted-operation error before registry mutation. RAII releases and closes
on the acquiring blocking thread.

Under that lock, the native adapter reads exact bounded raw type/bytes. Before
its single write/delete it rereads and requires the original observation; after
writing it verifies the exact intended postcondition. Reads and no-ops use the
same lock. A failed write/readback remains failed or uncertain: no replay,
automatic rollback, or restoring stale original bytes. Ordinary writes are
immediate, not a multi-operation transaction or power-loss durability proof.
The registry Run value remains the sole preference authority.

This serializes cooperating versions using this mutex, not arbitrary external
same-user writers. External same-SID tools are trusted/coordinated; a write can
occur between check and mutation or after readback. There is no compare-and-swap,
immutable key identity or hostile same-SID guarantee. Older KTM-only copies do
not participate: simultaneous preference edits across those versions are
unsupported. No new service or persistent lock file is introduced.

The prepared quoted command must fit the documented260UTF16-unit Run command
limit (terminal NUL excluded), including non-BMP surrogate pairs. Foreign-value
reads retain their8192-byte bound. The UI serializes requests and reports observed
state after success. There is no post-dispatch Cancel: it waits for the bounded
operation result and does not claim rollback. Closing a view discards late
presentation only. Windows Startup Apps policy may suppress a registered entry;
registration does not prove actual sign-in launch.

The setting launches this copy of the app. Moving it or replacing the entry with
another command requires explicit user reconciliation; this implementation does
not adopt a different executable as its own. No unattended sign-in, Explorer
restart recovery, single-instance activation or installed upgrade behavior is
claimed by source tests.

API basis is pinned upstream Tauri 2.12.5 local source: `src/tray/mod.rs` documents
builder ownership, menu callbacks and successful build; `src/menu/normal.rs`
documents menu items; `src/webview/webview_window.rs` supplies show/hide/restore/
focus. Pinned winreg0.55 supplies ordinary key/value APIs; existing windows-sys
supplies token SID, protected named mutex, and bounded wait APIs. The narrow
Security_Authorization feature enables SID/SDDL conversion; the winreg
transactions feature was removed. See Microsoft's [Run contract](https://learn.microsoft.com/en-us/windows/win32/setupapi/run-and-runonce-registry-keys),
[CreateMutexExW](https://learn.microsoft.com/en-us/windows/win32/api/synchapi/nf-synchapi-createmutexexw),
and [kernel namespaces](https://learn.microsoft.com/en-us/windows/win32/termserv/kernel-object-namespaces).
No custom tray or startup service is introduced.

Review gates: pure ownership/quoting/transition/failure tests, frontend pending/
preview/keyboard states, native tray failure and background/manual visibility,
actual Open and Quit UI with engine/config retention, denied/foreign registry
values using isolated fixture keys, and attended installed registration/removal.
Actual uninstall cleanup and stable-path update retention remain native installer
qualification gates; the source integration below does not silently delete
sign-in values on unrelated UI exit.
No real HKCU Run mutation or unattended login is part of source qualification.

## Installer ownership

The fixed `--uninstall-owned-startup` mode runs before constructing Tauri. It
creates no tray, WebView or backend client. It accepts no other arguments and
uses the same current-executable command and guarded ordinary registry adapter as Settings.
Direct invocation has no privilege increase: it is equivalent to removing this
copy's exact owned sign-in preference. Other argument combinations remain normal
unknown-argument launches with a visible window. Exit 0 means removed or absent;
10 means a foreign value was preserved; 21 means failure or an unknown outcome.
There is no fallback registry operation or automatic retry.

The pinned Tauri CLI 2.12.4 NSIS template otherwise deletes the product-name Run
value unconditionally. The local template preserves the exact upstream source
and licenses in `installer-template.upstream.nsi` and the associated provenance
and license files. Its only three adaptations are:

1. A normal version-upgrade handoff to the prior NSIS uninstaller also passes
   standard `/UPDATE`, preserving the stable-path entry even through that prior
   uninstaller. Existing updater mode already retains the entry. This uses
   upstream update semantics: shortcuts and preferences are retained; uninstall
   metadata is removed and rewritten by the incoming installation as before.
2. Explicit uninstall invokes the closed cleanup mode after app-close
   confirmation and before deleting the native executable. Status 0 proceeds,
   10 proceeds while preserving the foreign value, and any failed launch or
   other status aborts before deleting application files. Existing quiescence
   probes remain unchanged; they never stop the backend.
3. The later blanket Run-value deletion is removed. NSIS neither coerces a value
   through `ReadRegStr` nor performs a competing `DeleteRegValue` operation.

Explicit same-version uninstall retains its cleanup behavior. A moved install
path is unsupported automatic reconciliation: the old command is foreign to the
new copy and is visibly refused, never rewritten or removed silently. An
interrupted installation or accepted cleanup followed by a later uninstall error
is not rolled back automatically. Prior already-installed uninstallers retain
their historical behavior until replaced; source adaptation is not an installed
upgrade or uninstall proof. Exact template-diff/provenance and hook-order tests
are source evidence; NSIS compilation, normal upgrade, cancellation, foreign
value preservation and actual removal still require attended qualification.

Package preparation now preflights the exact CLI version, template, upstream
source, provenance and both license hashes before changing generated outputs.
The collector distributes those five files under
`licenses/dependencies/installer-template` with a distinct upstream inventory
entry. Redirected, dangling, hardlinked, changed or escaping targets refuse;
identical read-only copies are reusable without changing source/cache attributes.
This source/legal integration does not execute an installer or a maintenance mode.

Full native test compilation initially exposed a String/error-type inference
error in the setter's final read; that initial failure is retained. The narrow
String fix passed native debug compilation and all 99 serial ordinary Rust tests
(30 explicit live fixtures ignored). TypeScript/Vite and 213 JavaScript tests
passed, and two package-preparation runs resolved all 772 notices. The new D03
ordinary Rust tests use only a fake immediate-write registry store; neither setup nor the
maintenance entry runs under the test harness. Real tray behavior, HKCU Run,
sign-in, uninstall and stable-path upgrade remain attended acceptance gates.

## Observed disposable registry qualification

Root ran the corrected frozen091def3c... native fixture once under fresh nonce
`a73cd9812c864f3aaff7ab229e460d35`. The actual [fixture receipt](../../artifacts/qualification/p2-desktop-registry-a73cd9812c864f3aaff7ab229e460d35/proof.json)
reports all10 stages successful and cleaned, including valid8194-byte oversize
preservation/refusal, read-only write failure, second-process contention and
abandonment handling. Root observed429ms elapsed, the created Qualification
parent absent afterward, and no fixture child process remaining. This qualifies
the ordinary adapter against the disposable key, not real Run or installed/login
behavior. Tray Open/Quit, actual sign-in and installed uninstall/upgrade remain
separate gates.

The prior5a8e08... fixture's odd8193-byte unterminated REG_SZ did not round-trip
exactly. Root's bounded native audit observed8192 bytes0x41 then one0x00, rawSHA
`119245301c87dc810f2f4e2d342cbd7750ad3feb24377d158213c50675638c75`.
Root removed only that exact audited synthetic value and its exact nonce marker,
then the empty fixture leaf/created Qualification parent, preserving the existing
OrbWin parent. Failed source/binary/receipts and audit remain retained. The new
valid terminated fixture does not relax the production8192-byte read limit.

Subsequent source review corrected write-access admission for no-op startup
requests and uninstall cleanup. Under the existing SID lock, the adapter first
queries the bounded value with `KEY_QUERY_VALUE` only. An already matching
setting, absent disabled entry or preserved foreign uninstall entry does not
request `KEY_SET_VALUE`. A needed change then opens the write-capable store and
repeats the existing ownership check, immediate prewrite read and final readback.
Read denial remains an error; a failed write admission is not treated as removal.
This avoids unnecessarily blocking uninstall on a readable but write-protected
Run key when there is nothing owned to delete. Seven focused ordinary tests pass
with two live fixtures ignored. No real Run key or DACL was changed, and this
new admission ordering has not received an installed/native fixture run.
