# Local HTTPS private state origin

Source integration, 2026-10-26. Native ACL, file-origin, DPAPI and physical trust
store qualification are separate gates; this increment did not open or modify
existing profile state, trust stores, engines or WSL distributions.

The production SDK opens `internal/sdkstate.Root` at native Profile-derived
`.orbwin-private/services`, then transfers that opaque capability into
`OpenManagedNamespaceLedger`. The guest home, engine disks and existing service
pipe identity remain unchanged. No caller supplies a private state path. A
logical directory label used inside localnet cannot be used for filesystem IO.
The plain `OpenNamespaceLedger(temp, daemon)` constructor remains an explicit
controlled fixture seam, rather than Windows native file authority.

The native root requires private-at-CREATE current-SID/SYSTEM/Administrators
descriptors, retained unchanged native ancestry and a sole SDK owner. A root's
presence is not evidence of initialized state: its durable `state.init`, native
SID, product/global identity, original home, daemon and root identity must match.
Creation/migration fences, unknown operations and foreign or partial state block
normal startup. Legacy state is inspected only through the fixed denial probe;
it is never loaded as signing, namespace or trust ownership authority. Explicit
cold transition after positive old-owner retirement is separate work.

Every ledger transaction acquires one bounded native Guard while holding the
ledger mutex. Private record reads retain actual native file capabilities. The
same Guard must supply the prior authenticated object and expected bytes for
replace, append or delete; a digest or JSON copy cannot mint mutation ownership.
Creation, replacement, append, enumeration and deletion use a closed product
basename grammar. Image import transactions keep their guard through the daemon
operation; namespace creation still follows a positively flushed intent and
records its accepted ID through a fresh guarded transaction before start. Trust
guards span DPAPI signing-state verification and external Windows/WSL mutation.
Guard close errors join the transaction result before unlocking. Unknown native
mutation admission poisons that owner instead of allowing silent replay/reset.

The covered records are authority/signing ciphertext, desired settings,
authority rotation and its preparations/public archives, namespace/relay
intents and accepted IDs, tools-image receipts/archives, Windows/WSL trust
receipts and the LAN owner identity pin. Production paths cannot fall back to
`Mkdir0700`, `Open0600`, absolute rename or a user-home ownership receipt when a
native state operation fails.

Windows trust install verifies the reviewed DPAPI authority inside the SAME
guard as the receipt intent, exact CurrentUser ROOT operation and result flush.
The constrained intermediate signing key remains the only retained private key;
the root private key is not persisted. The fixed physical CurrentUser ROOT
adapter still uses ADD_NEW and exact DER comparison. Removal intentionally
works with an expired or missing signer using authenticated owned public DER.
It must first hold the private receipt and flush a removal intent, then recheck
origin before/after the exact store operation. A plaintext legacy receipt cannot
authorize this path. The WSL host receipt guard similarly spans its guest
mutation and acknowledgement. Startup never installs or removes trust.

Managed LAN identity schema2 hashes a fixed product domain, the unchanged
canonical global UUID32/36 identity and actual native SID, with no state-directory
or bundle hash. Its private record also pins the actual daemon. Storage movement
does not change the namespace; SID/daemon/product drift requires explicit
transition. Existing SDK arguments currently carry the registered daemon ID in
both global-identity and daemon positions; this change does not invent a new
installation GUID. No native LAN lease had been provisioned at this transition.
Old directory-derived schema1 remains fixture/history only.

Closed startup exit codes preserve refusal across the detached child boundary:
124 means explicit legacy transition is required, 123 retained state needs
recovery, and 122 native state origin is unavailable. Existing 126 means no SDK
server and 125 is generic failure. An unauthenticated old pipe peer remains a
request-not-sent generic failure; it cannot claim owned legacy state. The desktop
must read the actual retained child's exit status, rather than parse stderr.

Pure memory-capability tests cover missing/expired-signer removal while retaining
the origin guard, origin refusal before external mutation, post-intent origin
loss, settings revision CAS and unknown ownership, namespace acceptance ordering,
unopened-ciphertext rotation cancellation and LAN identity drift. They do not
qualify actual Windows descriptors, native publication/flush behavior, DPAPI,
physical CurrentUser ROOT or cold migration.

The independently reviewed disposable native store fixture now passes actual
private root creation, exclusive owner refusal, held-record write-sharing denial,
create/replace/append/delete, exact reopen and positive close:
`artifacts/qualification/p2-sdk-state-native-source-v3-20261026/native-run-e424110b-0257-46ea-a845-88e395711498/proof.private.json`.
Only the two fixed storage namespace literals differ from the frozen production
state implementation. The test uses synthetic non-secret settings in a new
Profile nonce and a separate never-created LocalAppData legacy probe. It starts
no child/listener/service, changes no production state or permissions, and uses
no CA, DPAPI, certificate store, engine, WSL or administrator action.

The earlier two frozen runs are retained failures, not retries against their
partial state. The second isolated the first failure to directory enumeration
with ERROR_NOACCESS. Windows requires eight-byte alignment for
[FILE_ID_BOTH_DIR_INFO](https://learn.microsoft.com/windows/win32/api/winbase/ns-winbase-file_id_both_dir_info).
A Go byte buffer was stack allocated without that guaranteed alignment. The
production path now uses uint64-backed 64-KiB storage and checks alignment before
the native call; an ABI test and the third native run pass. This evidence does
not establish power-loss durability, old/new publisher exclusion, actual CA/trust
transactions or cold migration acceptance.

The next independently reviewed managed-authority fixture also passes:
`artifacts/qualification/p2-sdk-managed-authority-source-v1-20261026/native-run-5aa5d0e3-f264-4341-b078-7002de6d8d09/proof.private.json`.
It freezes the complete SDK source catalog and changes only the two native store
namespace literals. Actual `Root`/`OpenManagedNamespaceLedger` creation protects a
synthetic intermediate with current-user DPAPI; native close/reopen preserves the
same CA and usable constrained signer. The persisted record contains neither the
plain bundle nor encoded intermediate key. An owned mutation of that fixture's
ciphertext makes load fail, and duplicate creation refuses without replacing it.
The damaged synthetic record remains private evidence. No root-store interface
is constructed and no certificate is trusted. There is no service, listener,
engine, WSL, administrator or child-process action by the probe. This qualifies
the tested native state/DPAPI intersection, not legacy cold import or physical
trust-store installation/removal.
