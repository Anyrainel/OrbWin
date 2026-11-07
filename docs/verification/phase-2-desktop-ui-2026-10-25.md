# Desktop native UI qualification, 2026-10-25 Pacific

The frozen release application `15e0c151c43f2d417a2423773267499933a28c9201bfbd9c0a9b25d977435a03`
uses current desktop/settings source, frozen built frontend and verified copied
package resources. Its new qualification identifier and profile are independent
of installed OrbWin. All 138 source hashes matched before copying the build;
the build passed. Source may subsequently evolve in the registry-fixture lane.

Evidence directory:
`artifacts/qualification/p2-desktop-ui-d67e2e75b113457d86c7e6b72831dd21/`.
Aggregate `final-ui-proof.json` SHA-256:
`420fe79ff6daad743bba8ba4c2eaf1de75ffc7c3ccbb2d979817eed425f2af5d`.

Observed outcomes:

- Manual launch displays the main window with the owned engine running. Twenty
  policy checks are absent/empty; all six browser descendants use the exact fresh
  qualification profile.
- General Settings renders the tray/sign-in description, initial reading state
  and settled unchecked sign-in checkbox. Read-only preference refresh completes.
  No startup toggle is submitted. Keyboard focus is visible on refresh and the
  checkbox; UIA focus reports only a pane.
- Normal Alt+F4 exits the qualification app and every profile browser process.
  Installed OrbWin stays open.
- Exact `--background` launch remains running with six matching profile browser
  descendants and no targetable main window in the native tool's window list.
  This establishes that observed inventory outcome, not a direct hidden-window
  visibility query or an actual sign-in launch.
- After cleanup, all three canonical engine inventories and five global/private
  configuration existence/hash records exactly equal the fresh pre-launch
  baseline. This lane has no registry raw-byte or WSL project-tree snapshot.

Native window/app inventories expose no taskbar or notification-area target.
Actual tray-menu Open and Quit therefore remain unqualified. The background app
was terminated only after exact executable hash/path, PID and creation-time
guards; no backend process was targeted. A first guard refused before mutation
because reparsing a JSON-decoded DateTime through its culture string lost UTC
kind. Its receipt is retained; direct typed UTC comparison matched the CIM and
held process creation time. Forced disposable UI cleanup does not qualify Quit.

Remaining gates include actual tray Open/Quit, tray-creation failure recovery,
native isolated-key registry transaction cases, attended real sign-in registration
and login, installer update/uninstall, and broader visual/accessibility coverage.
No Run value, startup policy, global default or engine lifecycle setting was
changed in this qualification lane.

## Subsequent registry fixture refusal

The separately compiled ignored fixture `7190e06b…` was invoked once against only
`HKCU\Software\OrbWin\Qualification\f23960666a2c4b35bd4450f08d1e29eb`.
It failed in 32 ms with Windows error 6801: transaction support for the resource
manager was not started or had shut down. No fixture step completed. The retained
receipt has `pending:create-exclusive-nonce-key-and-marker`, no parent ownership
acknowledgement, and no automatic retry or cleanup.

Read-only post-failure inspection confirms both the nonce and Qualification parent
are absent. The existing OrbWin parent was retained. No Run value, ACL, resource
manager, service or system policy was repaired or modified. Evidence is in
`p2-desktop-registry-native-faaf9ecb-0609-4c11-a586-5e45ad747655/`, including the
compile/run proofs, streams and `after-failure-key-audit.json`.

This is a native failure of the selected registry approach, not successful sign-in
qualification. The current transactional production path fails closed; its
uninstall cleanup also requires review before installation. A supported per-user
approach is being evaluated without an admin repair or a silent runtime fallback.

## Ordinary per-user replacement and native result

The transactional approach is superseded in source by ordinary fixed HKCU value
APIs, a bounded cross-session token-SID mutex, exact before-write ownership
recheck, and verified readback. There is no runtime fallback, automatic rollback
or retry. Commands are capped at 260 UTF-16 units. This serializes cooperating
OrbWin copies; external same-user writers must coordinate, and no expected-value
registry CAS or hostile same-user protection is claimed.

An initial ordinary fixture passed create, enable/noop/disable and four foreign
value cases, then refused its invalid odd-length, unterminated 8,193-byte REG_SZ.
Raw native readback had the same length but a zero last byte. The failed source,
binary and receipt remain under `p2-desktop-ordinary-registry-a24209ec-63dd-41ed-b689-350fc586da7b/`.
Root cleanup checked the exact observed value hash `11924530…`, exact owner marker,
two-value/no-child scope and recorded parent ownership before removing only this
synthetic nonce and its created empty Qualification parent. The existing OrbWin
parent was retained. This is guarded same-user cleanup, not external-writer CAS.

The corrected fixture uses a fresh nonce and a valid terminated 8,194-byte UTF-16
value; the production read cap remains 8,192. Frozen executable `091def3c…` passes
all ten stages in 429 ms, including oversized/foreign preservation, read-only
write failure, real child-process mutex contention and abandoned-lock handling.
It cleans its nonce and created parent. Read-only post-run audit confirms the
Qualification parent is absent and no fixture process remains. Run proof:
`p2-desktop-oversize-fixture-2054b1fd-ef93-496b-8638-dae20c4c0de2/run-proof.json`,
SHA-256 `8a88802695d34b804f3ab5e58833ebefd31c50f04c9ecd04f47af3960f5c344a`.
This qualifies the disposable-key adapter and synchronization cases without
admin privileges. Actual Run registration, sign-in, tray menus and installed
update/uninstall remain open. The earlier KTM installer is stale and must not be
used as qualification for this replacement.
