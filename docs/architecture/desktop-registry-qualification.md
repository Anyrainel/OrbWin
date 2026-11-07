# Desktop sign-in registry qualification

## Current ordinary-HKCU candidate

The ignored exact test is
`desktop::startup::qualification::owned_startup_registry_guarded_operations`.
Its fresh nonce is `a73cd9812c864f3aaff7ab229e460d35`, and its only registry
leaf is `HKCU\Software\OrbWin\Qualification\a73cd9812c864f3aaff7ab229e460d35`.
Execution requires `ORBWIN_QUALIFY_DESKTOP_REGISTRY=1` and
`ORBWIN_QUALIFY_DESKTOP_REGISTRY_NONCE=a73cd9812c864f3aaff7ab229e460d35`.
The private child test `desktop::startup::qualification::startup_mutex_child`
also requires `ORBWIN_QUALIFY_DESKTOP_REGISTRY_CHILD=1` and is invoked only by
the main fixture. Both are ignored by ordinary tests.

The fixture reuses production bounded raw reads, ordinary RegistryStore, pure
ownership/transition policy, and the named-mutex implementation. Only its private
test constructor substitutes the fixed disposable key. Production commands still
accept a boolean and operate only on the fixed real Run value. The fixture does
not open Run, construct Tauri, or contact a backend. The full contract and limits
are in [desktop convenience](desktop-convenience.md).

Before writes it exclusively creates
`artifacts/qualification/p2-desktop-registry-a73cd9812c864f3aaff7ab229e460d35`.
Its synced receipt records intent and each parent/nonce creation disposition.
An existing nonce is refused, even if empty; it receives no owner marker. Only
the exact QualificationOwner marker and known synthetic OrbWin field are allowed.
Native failure may leave a partially created key with recorded pending state;
there is no adoption, automatic retry, or automatic failure cleanup.

Cases cover absent → enabled → no-op → disabled → no-op; exact preservation of
foreign REG_SZ, owned-looking REG_EXPAND_SZ, REG_DWORD, odd-byte malformed REG_SZ,
and8194-byte valid NUL-terminated UTF16 oversize. Production reads stay8192bytes; the fixture-only bounded
reader accommodates the synthetic oversize case. A read-only handle verifies
write refusal without changing ACLs. Mock tests separately exercise changed-before-
write and uncertain post-write/readback outcomes with no replay or rollback.

A child process acquires a fixed nonce-scoped Global mutex through the production
lock code. Parent acquisition times out while it is held. The parent opens a
handle to that same object before terminating/reaping its child, then requires
abandonment refusal and a subsequent successful acquire, proving the refusal
released ownership. This safely kills only the fixture's exact child. There is
no claim of cross-session live execution: the Global namespace provides the
source contract; this fixture executes two processes in one session.

Successful cleanup checks owner marker, known absent field, no extra values or
subkeys, then removes only that leaf. Parents are removed only when this run
created them and they are still exactly empty; existing parents are preserved.
Cleanup is nonrecursive and may partially succeed before an error. All uncertain
outcomes retain their evidence for review; no ACL edits, resource-manager repair,
or arbitrary key paths are available. Same-SID concurrent writers are trusted/
coordinated; these checks are not a hostile-writer CAS/ABA guarantee.

An external runner must verify frozen executable/source hashes, invoke only the
exact opted-in test, capture stdout/stderr/exit, and impose a bounded deadline.
Timeout means unknown: retain keys/receipt and do not rerun or recursively delete.
The fixture's child has a30second lifetime and bounded3second readiness wait;
ordinary lock waits are5000ms. This source is not evidence of actual invocation.

## Preserved failed KTM candidate

The prior exact test was
`desktop::startup::qualification::owned_startup_registry_transactions`, nonce
`f23960666a2c4b35bd4450f08d1e29eb`. Its frozen executable SHA is
`7190e06b4724c332ed542fad08e804229ea8b908aea3add21b71a3372096e3bf` and its
[compile-only receipt](../../artifacts/qualification/p2-desktop-registry-native-faaf9ecb-0609-4c11-a586-5e45ad747655/proof.json)
remains unchanged. Initial compile diagnostics are retained separately.

Root's actual run failed in32ms with Windows6801 (ERROR_RM_NOT_ACTIVE), at the
creation stage with no successful step and parents null. The [actual receipt](../../artifacts/qualification/p2-desktop-registry-f23960666a2c4b35bd4450f08d1e29eb/proof.json)
does not identify the exact native call: CreateTransaction, transacted opens/
creates, marker/guard, and commit shared the stage. Root's subsequent read-only
audit found the nonce and Qualification parent absent, and an existing OrbWin
parent with one foreign child. That audit is not full historical foreign-child
metadata equivalence. No cleanup or repair was attempted. Prior source bytes are
retained under `artifacts/patch-replay/desktop-startup-without-ktm/previous`.

The replacement deliberately removes KTM as an implementation dependency rather
than silently falling back at runtime. Actual sign-in, real Run edits, tray
Open/Quit, stable-path upgrade, and explicit uninstall remain separate product
qualification gates.

## Preserved first ordinary-HKCU fixture failure

The frozen9b2c47fa... executable used nonce
`5a8e08c774db4dde9a965b70176ad568`. Root's actual run passed creation,
owned enable/no-op/disable, foreign string/type/DWORD, and malformed-string
preservation. It failed at the oversized case's exact raw-byte guard, before
production oversize refusal was reached. Its8193-byte repeated-byte REG_SZ was
odd-sized and unterminated; pinned winreg0.55 passes that slice and byte count
directly to RegSetValueExW without normalizing. Microsoft's
[API contract](https://learn.microsoft.com/en-us/windows/win32/api/winreg/nf-winreg-regsetvalueexw)
requires terminated string data with the terminator included in cbData. Exact
native normalization must come from root's retained raw audit, not inference.
The old nonce, frozen binary, failed receipt and source remain untouched here.

The new candidate uses4096 UTF16 A characters plus one NUL:8194bytes, even-sized,
valid terminated text, still above the unchanged8192-byte production cap. A pure
fixture contract test checks that shape. The fresh nonce above prevents reusing
or adopting the retained previous fixture. Production policy, registry read cap,
lock code, and ordinary adapter are unchanged by this fixture-only correction.
