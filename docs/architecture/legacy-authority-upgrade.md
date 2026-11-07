# Legacy management authority upgrade

Status: candidate source implementation and review, 2026-10-26. The preflight gate
protects setup/resume before a known candidate stops the existing service. The
transaction below still requires production provider integration and fault/native qualification;
it is part of the supported upgrade work, not a permanent legacy exclusion.

## Verified source constraints

`installation::setup_at` retains `setup-intent.json` and `runtime.json`, checks
the same owned home/registration, compares the Docker daemon ID at the product
pipe, then stops the old service when the verified candidate digest differs.
Previously an existing `args.json` skipped Create. The selected candidate's
`preparePrivateAuthority` now requires `managementconfig.OpenInitialized`, which
verifies a private `authority.init` bound to the exact saved arguments. Existing
legacy instances do not acquire that marker simply by retaining their arguments.
Stopping first therefore made an otherwise healthy engine unavailable before
discovering an incompatible authority layout.

The legacy/selected service root is the fixed `home/AppData/Local/rancher-desktop-orbwin`;
`instance.ShortDir` is `home/.rdorbwin` and `instance.LimaHome` is its `lima`
child. The latter contains independent guest/hostagent state and disks and must
not be moved with the service root. The management datastore is `db/state.db`
under the service root; SQLite WAL/SHM sidecars can contain necessary state.
The persisted App, LimaVM, management settings/secrets and Kubernetes version selections
belong to that datastore. Recreating a default App is not a migration.
The separate local-services owner uses `home/.rdorbwin/local-services`, including
its authority and relay receipts. That ledger, Docker/CLI configuration, guest
disks and other state outside the management root require separate accounting. Migration
must coordinate its existing owner to close/retire affected listeners and retain
LAN/firewall cleanup records; it cannot discard those records or claim an
uncertain native rule was removed. Its own HTTPS CA is a separate authority and
is not rotated merely because management TLS changes.

The unadopted successor selects management authority independently at native
`KnownFolder_Profile/.orbwin-private/authority`; it does not move the guest home,
Lima disks or WSL registration. The SDK similarly owns its separate native
`services` sibling. Its old-home HTTPS state needs an explicit cold transition
that preserves ciphertext/DER/settings while quarantining historical side-effect
receipts. Moving plaintext receipts into a private folder does not grant trust
removal or firewall authority.

The new service generates fresh volatile admin/user bearer tokens per process.
Its protected TLS defaults are `tls/apiserver.crt`, `tls/apiserver.key`, and
`tls/sa.key`; `authority.init` is an arguments binding, not proof that historical
credentials were safe. The kine transport uses a separate private mTLS
generation. Legacy `config.yaml`, mirrored management kubeconfigs and legacy TLS
or token-store material must not be imported as trusted new authority.

`StopWithWait` can fall back to killing the management process. Its source
explicitly notes that hostagents may then survive. The new service's normal
unwatched startup kills an orphaned hostagent before restarting it. Neither a
successful CLI exit nor an absent management pipe proves that every old reader,
hostagent, TCP listener, named-pipe publisher or SQLite writer has retired.

## Read-only candidate preflight

Setup invokes the exact verified bundled candidate with a fixed command and the
same isolated home/instance environment as the subsequent lifecycle operation.
The preflight reports only its closed versioned classification. It does not
start a service, bind a listener, initialize files, obtain a service-owner write
lease, repair ACLs, or read credential contents. In particular, it must not call
`OpenInitialized` (which opens/creates the owner lock) or `options.NewOptions`
(which probes a loopback listening port on Windows).

Only a compatible current private authority, or a proven fresh Create path,
permits setup to continue. Legacy, partial, inaccessible, unsupported or malformed
results stop setup before any service Stop/Create/Start. The message retains the
existing engine and calls for the explicit authority transition. This observation
does not reserve future startup success: the service repeats its authoritative
checks, and an intervening change remains a normal failure requiring inspection.

The existing Prepare Update journey verifies and stops the selected installed
engine for application-file replacement. It cannot prove compatibility with an
unknown future installer. It remains unchanged. An incoming installer/candidate
must be staged independently of the running executable and preflighted before
its own replacement/Stop boundary. Installer sequencing and any candidate
selection UX require a separate reviewed change; no update-feed claim follows
from the setup gate.

## Complete data-preserving transition ordering

1. **Prepare without interruption.** The backend owns one explicit migration
   operation under the existing setup mutation lease plus a cross-process
   migration lease; the renderer only observes it. Verify old and incoming
   binary/patch manifests, matching intent/runtime identity, canonical fixed
   home, registry BasePath, product instance and actual Docker daemon identity.
   A live publication must match the owned old process generation and endpoint;
   a PID, stale config file, absent pipe or cached UI snapshot is insufficient.
   Offline resume requires the persisted migration journal and exact old object
   identities, not adoption of arbitrary files. Validate bounded saved arguments
   against the supported defaults; custom key/datastore/config paths require a
   separately specified migration route and are never followed implicitly.
2. **Create the recovery envelope.** Retain verified old binaries outside the
   replaceable application files, with recorded digests and an explicit old
   launch recipe. Create a fresh protected fixed-role migration journal and
   backup destination by handle beneath the owned trusted parent. Record source
   directory/file IDs, manifest hashes, desired running state, instance and
   registration identity, and the current stage. The journal contains no bearer
   token/private-key bytes. Check space for a complete management backup before
   requesting interruption. Existing ambiguous journals/backups are inspected,
   never overwritten. Do not create `authority.init` on the legacy root or alter
   its inherited ACLs to make it pass the fresh-authority policy.
3. **Interrupt through the old owner.** After explicit workload-interruption
   confirmation, ask the verified old owner to close ingress and stop the owned
   guest/hostagent through its existing lifecycle. Preserve the intended running
   state for restoration; do not change the selected cluster version or disable
   an existing cluster. Observe exact old HA/process generations and owned
   publication retirement, then request the old management service shutdown and
   join its process and SQLite writer. No global WSL shutdown, unregister,
   unrelated engine stop or ambient PID kill is permitted. Unknown retirement
   leaves the journal in `retirement-unknown` and prevents data copying/root
   replacement. An explicit recovery action must resolve the same generations.
4. **Capture a cold complete management checkpoint.** With all writers retired,
   retain the old service root as an exact backup object through same-volume
   handle-relative rename to a fresh transaction-specific sibling. Record and
   flush the rename receipt and verify predecessor/successor file IDs. Preserve
   the whole legacy root as rollback evidence, including SQLite main/WAL/SHM,
   arguments and old public/private configuration; never expose it through logs
   or generic export. A coherent cold main/WAL set is the source of a separate
   candidate working copy. Do not copy `state.db` alone or assume deleting WAL
   is a checkpoint. Candidate SQLite recovery/integrity checks run only on that
   copy, without starting controllers; retain the untouched original checkpoint.
   The exact cold-copy/SQLite API and Windows flush/rename semantics need their
   own tests. No guest VHD or Lima directory is copied, renamed or restored.
5. **Create new authority, retain application state.** At the native Profile
   authority location, use the existing private CREATE_NEW initializer and
   explicit validated arguments. Copy only the supported management datastore
   checkpoint into protected new files and validate its expected schema and
   application inventory before controllers run. Import of arbitrary paths,
   executable hooks or new credentials from legacy arguments/state is refused.
   Preserve object identities/specs, settings, cluster version, secrets and
   resource versions; do not issue `ctl create` for an existing App. Generate
   fresh management TLS/signing material in the private root, fresh process
   admin/user tokens and a separate new kine mTLS generation. Re-publish all
   current management consumers through the existing protected publisher and
   inherited serving anchor. The old root remains historical evidence, never a
   new reader's credential fallback.
6. **Validate before publishing the new runtime identity.** Start the candidate
   through the sole existing owner. Require private management publication,
   old credential rejection, successful new authentication, no plaintext kine
   access, current-start Lima readiness, unchanged WSL registration/BasePath,
   the same Linux Docker daemon ID, expected Docker inventory and actual fresh
   Windows publication. Restore the recorded desired running state through the
   normal owner; restore an originally stopped engine as stopped. Check existing
   cluster version and representative persistent objects, volumes and settings.
   Only after these observations atomically publish the new `runtime.json` and
   mark the journal committed. Retain backup and old binaries according to an
   explicit product retention policy, not immediate automatic cleanup.
7. **Recover at the last proven boundary.** Before root rename, failures leave
   old state in place and can offer explicit old-engine resume after identity
   checks. After rename but before candidate writes, the exact original root can
   be restored only after proving all candidate resources retired. After any
   candidate datastore write, never overlay the old checkpoint onto newer state
   or silently downgrade. Preserve both roots; report the last stage and offer
   a reviewed explicit rollback decision covering lost post-checkpoint control
   changes. Such rollback first retires candidate management/HA/publications,
   verifies the backup and old executable identities, restores the old root by
   an exact rename transaction, then resumes only the old recorded owner and
   verifies daemon/registration/data. Changed guest/on-disk formats or unknown
   retirement block automatic rollback; forward recovery is preferred. A lost
   reply inspects the durable journal and original transaction, never reruns a
   Stop/rename/import based on file absence.

Cancellation before interruption is a clean refusal. After interruption it
stops the requesting client only; owner work/journal remain inspectable and
Resume follows the recorded stage. Each native operation needs a bounded owner
deadline and an explicit unknown outcome when termination/flush/rename cannot
be established. A process exit is not evidence that prior durable writes failed.

## Qualification needed for completion

Pure contract tests cover closed classifications, malformed/duplicate/trailing
JSON and unsupported schemas. Cancellation before dispatch and actual refusal
ordering also require an injected runner or end-to-end qualification.
Migration fault injection must cover interruption after every durable stage,
disk full, denied handles, held files, reused PID/generation, owner/HA loss,
rename-success/reply-loss, SQLite WAL presence, partial private initialization,
new-publication failure and candidate writes before rollback. Native Windows
qualification must compare real user fixtures before/after: Docker daemon ID,
containers/images/volumes, bind mounts, settings, cluster version/PVCs, other
engines and independent user distributions. Source/compile success does not
qualify the migration or promise uninterrupted workloads.

## Reviewed candidate startup fence

The focused independent re-review in
`artifacts/qualification/p2-upgrade-import-recovery-independent-review-20261026/fix-review.json`
resolves the discovered incomplete-import startup gap and bare-PID reuse refusal
in the reviewed source. A private sibling migration fence is flushed before the
new authority directory exists. Normal startup requires the exact final committed
journal with no recovery branch and bound native fence/root/imported DB IDs,
rechecked under the migration and service-owner locks. `authority.init` is
withheld until full import and candidate-start admission. The first candidate
requires an inherited exact native FILE_OBJECT permit, verified live parent,
incoming image/generation and full imported data; a path or Boolean cannot bypass
the fence. Process comparisons include creation/image identity so PID reuse alone
does not reject a different generation. Scoped fault-contract tests and vet pass.
Actual native inheritance, SQLite import, launch/observer/runtime publication and
SDK sidecar commit remain separate unqualified gates. This successor is not the
selected runtime or an adopted patch yet.

## Legacy SDK controller denial primitive

A disposable Windows fixture on 2026-10-26 verified a proposed persistent
denial fence for the supported older SDK's exact `GENERIC_READ|GENERIC_WRITE`,
share-zero, `OPEN_ALWAYS`, `FILE_ATTRIBUTE_NORMAL` controller opener. Setting
READONLY through the exclusively held original FILE_OBJECT, flushing and
reading back its attributes, then positively closing it caused that old opener
to fail with ERROR_ACCESS_DENIED (5). A new read-only/share-zero owner reopened
the same native file ID; a second reader failed with sharing violation (32).
After closing the successor, the old write opener still failed, and a second
successor read-only reopen retained the same ID. The fixture requested no admin
access and touched no product state. It remains preserved under
`artifacts/qualification/p2-legacy-controller-denial-native-v1-20261026/` with
independent source/binary boundary review and `native-result.json`.

This is a denial primitive, not authority or a complete upgrade proof. The
candidate must bind the actual retired generations, private descriptors, durable
admission and sealed receipts, sole writable-handle retirement, exact read-only
owner and publication lifetime before ordinary cold reopen can be enabled.
Fresh absence and explicit loss-reviewed rollback require separate handling.
