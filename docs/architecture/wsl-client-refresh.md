# Explicit WSL private-client refresh

Recorded 2026-10-25 Pacific. This is an implemented Windows setup action, with
the development-runtime qualification below. It is separate from project Connect
and from an application or backend upgrade.

## Authority and scope

The WSL inspector offers **Refresh client tools**. Its native command,
`wsl_refresh_private_clients`, accepts no distribution, project, directory, or
renderer-supplied tool path. It derives the owned home from the existing qualified
runtime identity and verifies every bundled Linux tool against the application's
embedded tool lock. It changes only `cliPluginsExtraDirs` in
`<owned-home>/.rdorbwin/wsl-docker/config.json` to that verified public tool root.
The result contains only `changed`.

The command does not launch WSL, contact or start an engine, mount exports, rewrite
project records, synchronize registry routes, call a credential helper, or change
global configuration. Existing project/export records remain their existing
authority. The private config must already exist; initial setup still uses the
explicit Connect flow. Refresh never creates a substitute connection record.

Existing config paths must be local, regular files under real directories, with
no reparse points. Reads use a bounded stable handle and a 1 MiB limit. JSON must
be an object with unique keys at every nesting level; ambiguous, malformed, or
excessively nested input refuses before writing. The updater splices only the
plugin value, or inserts that field when absent. All unrelated bytes remain
exact, including authentication fields, unknown fields, large integers, decimal
precision, whitespace, and key spelling. Parsed credentials are not returned.

## Concurrency, commit, and recovery

Cooperating application calls serialize through the existing registry-config
lock. Windows directory handles deny ordinary parent rename while open. The
original file handle denies in-place writers but permits atomic replacement.
Before commit the updater rechecks current file identity, bytes, owner/DACL, and
the bundled tool pins. These are bounded observations, not a path compare-and-swap
against a hostile same-user process; an external replacement can race after the
last comparison. No inode/content immutability claim follows.

For an actual change, the updater exclusively creates a sibling
`.orbwin-client-refresh-*.original.json` recovery copy with the original exact
bytes, then a unique `.partial` replacement. Both receive the original owner and
exact DACL at creation, with the copied DACL protected from broader parent
inheritance. Their permissions are verified before secret bytes are written.
Failure to preserve that policy refuses without requesting elevation or changing
the parent ACL. The original config policy is compared again before replacement.

Backup and replacement bytes are synced before a same-directory rename replaces
the config. A final bounded read checks the result. A no-op does not rewrite the
config or create recovery files; the existing lock mechanism may create its lock
file. This is not qualification of power-loss persistence or every Windows
filesystem/security-policy combination.

An interrupted or failed operation may leave a backup or partial replacement.
They are retained rather than guessed away. After an uncertain commit, inspect
the current config and recovery copy before retrying; never restore automatically
over a potentially newer config. Commit errors say any remaining temporary files
were retained: a successful rename can already have consumed the partial name.
Inspect what actually exists. The original-byte backup remains the recovery
reference. Recovery copies can contain the original private authentication fields
and retain the original owner/DACL protection.

The UI has pending, error, changed, and already-current states. Browser preview is
disabled; unmounted views discard late results. Once dispatched, the local
transaction finishes and reports its outcome; there is no rollback-style Cancel
control. Standard button keyboard behavior and status/alert roles are present,
but comprehensive keyboard or assistive-technology qualification is not claimed.

## Evidence and installed-version limits

The [actual refresh receipt](../../artifacts/qualification/p2-wsl-client-refresh-4e35f0dc318c43e58e5f69094d243a21/proof.json)
records success and idempotence using frozen native SHA `24f547f9…`, embedded tool
lock SHA `37528c5f…`, and the exact ignored native test
`wsl_clients::qualification::owned_private_client_refresh`. Config SHA changed
from `29177adb…` to `77c380f6…`. It verifies the raw plugin-only edit, exactly one
original-byte backup, no temporary residue, unchanged three-engine inventories,
global/private Windows configs, project records, and all nine original WSL tree
entries. The selected tool root was the development cache
`/mnt/f/Codes/Orbwin/artifacts/runtime-tools/linux`. The installed tools were not
replaced. A separate debug UI Automation check reported pending followed by
“WSL client tools are already current.” and a reenabled button.

Eleven focused Rust tests cover config, reparse, concurrent-change, pin drift,
raw JSON preservation, no-op, and restrictive-file-DACL/broader-parent cases.
Six runner guard tests and the current full JavaScript suite pass: 180 tests,
zero failures/skips. The refresh proof does not establish a project reconnect,
WSL workflow, application restart, installed-upgrade, or complete migration flow.

One shared private WSL config selects one tool directory. Installed and development
applications with different compiled pins cannot both assume that selection is
compatible: each client verifies the selected binaries and refuses mismatches.
Another version that supplies this refresh action can explicitly select its own
bundled directory again; the existing older installed version has not been
qualified with this action. Use the intended version's verified setup; do not
rewrite pins, bypass checks, or copy binaries into another version's directory.
An attended installed-version update and its actual WSL workflow remain separate
qualification gates.
