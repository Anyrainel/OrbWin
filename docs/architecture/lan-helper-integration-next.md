# Smallest remaining LAN administrative vertical slice

Source integration status, 2026-10-26. The ordinary async owner/options/review/
reconciliation and initial native authentication, detached bundle verifier,
consent, protected journal/staging sources exist. They remain unregistered and
have not launched elevation, consent, provider mutation, worker or listener.

The detached bundle uses same-build asset provenance rather than release
publisher/code-signing/update-chain trust. The Rust app must pin the exact
public key/manifest/signature/helper/SDK bytes. Build-local signing keys may
remain memory-only; retained public data reproduces the binaries and verifies
the retained signature. The helper embeds only the verifier public key, avoiding
SDK/helper digest circularity. It derives all fixed bundle paths from the held
actual SDK image and checks signature, bytes, size and native file identity.
Unsigned/unconfigured bundles refuse; ordinary product setup stays user-scoped.

`internal/lanhelper/consent_windows.go` uses explicit packed TASKDIALOG buffers,
not default-aligned Go structs. Exact SDK22621/MSVC compile-only type/offset/
enum proof is `artifacts/qualification/p2-lan-owner-source-0a3f6a01/taskdialog-abi-proof.json`.
This is ABI evidence only; no dialog or protected-file staging has run. Native
consent defaults to cancel and precedes protected provisioning. Partial journal
writes/flushes poison the writer; stage reservations flush before exclusive
private worker/boot creation. Current-user readable records are denial/info
only; authority requires native protected-source observation and fresh facts.

The existing authoritative owner is `localnet.ServiceRuntime`, its exclusive
NamespaceLedger and the same OwnedServiceBackend/NamespaceManager. The selected
rdd does not supply a privileged Windows service for this feature. The existing
trust helper is a selected-WSL-root Linux ELF; current-user Windows trust also
does not authenticate Windows administration. Reuse its fixed asset/digest,
framing and receipt discipline, not an assumed Windows service or user-home
receipt as authority. `lan-firewall-boundary.md` remains the consent contract.

## Implementable source boundary

1. Add `internal/lanhelper/owner_windows.go`, `store_windows.go` and tests for a
   one-shot helper session, exact held caller/helper identity and the protected
   ProgramData store. Reuse the existing V2 Session/Prepared/Child/Grant types
   and pure `lanlease` transition policy. Add no persistent service, second
   backend manager, shared admin secret or generic command API.
2. Add `internal/lanhelper/channel_windows.go` and `client_windows.go` plus a
   separate `lan-helper/main_windows.go`. The ordinary manager owns a different
   first-instance local helper channel, derived only from installation and the
   single-use transaction. The elevated helper connects as the verified child;
   both sides bind native pipe process identity, creation/SID/token session and
   pinned process/file hashes. Keep an alternate administrator token distinct
   from the original ordinary caller. No request-supplied program/store path,
   arbitrary arguments, endpoint or provider command is accepted.
3. Add `internal/lanhelper/consent_windows.go` for one exact-scope native review,
   defaulting to cancel. It displays interface/address/port/TCP-or-UDP/profile/
   client CIDRs/worker identity matching the reviewed digest. UAC identifies
   the fixed executable; it does not replace this scope review. Cancellation
   before provider dispatch must produce no rule. Cancellation after dispatch
   must drain/record uncertainty and cannot mint an ownership/ready capability.
4. Extract a narrow production fixed-rule adapter in NEW
   `internal/lanprovider/owned_rule_windows.go` and fault tests after root review.
   Today all Create/Delete slots live exclusively in `_test.go` fixture sources.
   The production bridge is read-only; copying the fixture into an active
   general mutator would be incorrect. Reuse reviewed MI ABI/lifetimes, exact
   PersistentStore options, required-option validation and the qualified full
   rule/seven-filter two-pass read model. Only positive terminal Create success,
   returned full keys and protected flushed acknowledgement establish ownership.
   Duplicates/unknown results cannot adopt/delete by later presence or name.
5. Connect these adapters to the existing actor's LANAdapters and build the
   dedicated helper/worker as separate, hash-attested assets. Add NEW bounded
   asset recipes/manifests before root's shared staging/notice/package changes.
   The worker must stay the dedicated no-command-dispatch binary even though
   its protected filename is `orbwin-session.exe`. Existing SDK/UI entrypoints
   must never be copied into that allowed private program path.

The helper's Stage owns protected tombstone publication BEFORE exclusive
never-overwrite worker/boot-file staging. It must implement the descriptor and
no-reparse/no-write-delete-sharing contract already enforced by the ordinary
worker. Refuse foreign/user-writable lookalike roots and preserve them. One
cooperating administrative writer/lease serializes each installation. A native
protected record, not a JSON claim, supplies the staged worker identity and the
persisted sequence. Stage returns only after flush; Commit binds the exact
ordinary child and full fresh source/interface observation. Unknown Commit
leaves the worker closed and its path permanently reserved. Removal first
requires exact positive child/flow closure, then protected ownership/full typed
state review; unknown removal is retained with no mutation replay. True provider
compare-and-delete is still not supplied by these controls.

## Concrete blockers and qualification gates

- **Publication generation:** root source-adopted patches0015/0016 now supply
  the full tuple including a required128-bit `ownerInstance` per bridge handler
  lifetime. The SDK binds it through Stage/lease/readback, so an unchanged numeric
  generation after handler restart cannot renew an old lease. Memory-transport
  source/race/vet and clean16 replay passed; active runtime remains12 and native
  stop/restart/recreate invalidation remains unqualified. See
  `artifacts/patch-replay/lan-publication-v2-owner16/source-adoption-ca070cb8-56f0-42fe-9f6d-d83a4bf9d50b/proof.json`.
- **Provider exclusive creation:** the compiled disabled BLOCK duplicate/race
  fixture remains unrun pending the root's full source/admin review. It is
  the next actual provider evidence gate, not a successful live result. Its
  deliberately inert policy is not evidence that the proposed enabled ALLOW
  template/readback works. Verify the exact provider enum/options/filter shape
  for that separate template; do not guess action flags or replace WBEM flags
  with MI semantics. Unknown late completion has to retain its protected intent.
- **Native staging/IPC/jobs/listeners:** the ordinary worker adapter is compiled
  with fake-IO/ABI tests, not runtime qualified. Root must exercise exact native
  identities/session, protected paths, occupied-port exclusivity, manager/helper/
  worker death, stalled observation, partial launch and active TCP/UDP closure.
  A successful anonymous-pipe UDP helper test is not UDP socket delivery proof.
- **Consent/package/UI:** the bounded ordinary launcher, one-shot helper,
  packed native exact-scope dialog, signed bundle verifier, protected store,
  staging and asynchronous SDK channel now exist as source. None is registered
  for product activation. Source preparation needs no unattended admin action.
  The actual narrowly reviewed
  provider fixture and final enabled policy test need attended Windows admin;
  ordinary SDK tests/builds, HTTP/UDP fake tests and source work do not.
  Product UI has explicit scope review and unknown-outcome reconciliation source;
  actual qualified recovery/cleanup remains required. These are remaining work,
  not scope exclusions or permission to ship permanently unavailable activation.

The SDK channel owns one consumed review and two fixed first-instance local
pipes, with no request-supplied executable, path or command. The signed SDK's
ordinary launcher is suspended until assigned to its one-process kill-on-close
job. Only that launcher calls the fixed sibling helper's explicit `runas` path.
The SDK duplicates the helper query/synchronization handle from its retained
original launcher before acknowledging transfer; the actual helper PID,
creation, elevated identification token, session and signed fixed image are
checked before any review is sent. A fixed empty client greeting makes the
identification token available without transmitting authority. Cancellation
closes authentication channels before stopping the ordinary launcher. The
session retains all late open results and joins the actual elevated handle when
available; a timeout never means that process has exited. Independent helper
self-watchdogs cover blocked native threads. No native role has been run for
this source increment.

Protected rule metadata source is now schema2. It adds `namespace` with exact
`containerId` and `daemonId` from the existing namespace owner; the complete
publication/owner incarnation remains inside `prepared.spec`. The full helper ID
comes only from `s.handle.ID` through the native `CommitNamespace` capability,
and is flushed before Create. It cannot come from renderer selection or a name
lookup. The strict protected reader must reject old schema, malformed tuples,
pending mutation and foreign ownership without deleting anything. Cleanup-only
restoration still requires independently observed old child exit and exact
same-daemon namespace absence after normal residual recovery; user-home ledger
absence cannot supply that proof. This restoration and explicit removal are
implemented as source below; full native qualification remains a gate before
registration.

Prior-owner review reconciliation has a separate fixed signed SDK read-only
child role, `lan-review-observation`, with exact installation/review/digest
arguments, bounded output and an independent ten-second self-cap around its
five-second read budget. One owner-local recovery read may run off the actor;
the actor remains able to revoke current unconsumed reviews. Only a positive
protected exact mapping can change `unknown` to `operation`. The reconstructed
public row is denial/cleanup information with unknown ingress, never Ready or
listener authority. A fresh exact protected-rule read may establish created
rule ownership information, while removal remains disabled until independent
native closure and opaque cleanup restoration succeed. Missing/busy/torn/foreign
records stay unknown, and recovered review IDs are permanently non-replayable
within that owner. Consumed explicit removal closes and positively joins its
LAN ingress before administrative review; shared HTTP ownership is retained.

Each native LAN lease now requests a dedicated helper from the SAME namespace
manager/backend/ledger, keyed by the final immutable prepared lease digest.
HTTP intents omit the new field, preserving their journal encoding. TCP and UDP
routes carry that exact purpose; releasing one LAN helper does not stop HTTP or
another LAN lease sharing the application network namespace. Admission is bounded
to sixteen HTTP and sixteen LAN helpers (thirty-two total), separately from the
per-helper thirty-two LAN stream budget and UDP association budget. A dedicated
helper reserves up to 64 MiB, 128 PIDs and 0.5 CPU under the existing profile;
these are configured ceilings, not observed idle cost or performance results.

The one-shot helper removal source now restores the opaque cleanup-only native
capability before showing scope consent or persisting a removal review. Its
protected original installation, SID, complete network scope and publication must
equal the displayed removal scope. Only the consumed `remove` command dispatches
retirement. Restoration cannot create a listener grant, and any unknown removal
remains retained without replay. This wiring and dedicated namespace lifecycle
still require the complete pure-test pass and native qualification before product
registration; source presence is not evidence of successful Windows removal.

Unknown removal has a separate fresh attended settlement operation. Public
`lan-settle-remove-review` issues purpose `settle-remove`, and consumption uses
`lan-settle-remove`, with new review/operation identities. The retained current
controller exposes `canSettleRemoval` only after positive host-child/flow closure
and its dedicated namespace release. The one-shot helper accepts a separate
`settle-remove` command and calls only `SettleCleanupRule`: strict protected
remove-intent4 plus current exact absence may append removed5; removed5 plus
current absence may be observed. Created3 or a present rule is refused without
Delete. The original `Retire` path cannot act as settlement and is never replayed.
A read-only `lan-removal-observation` role can recover an already flushed removed5
and fresh absence, with the same bounded child lifetime as other observations.

Removal/settlement consent shows the protected original interface alias, GUID,
LUID, address, profile and complete scope; unplugging the NIC or changing its
profile cannot relabel that dormant rule or prevent its exact cleanup review.
Activation still requires fresh current native interface eligibility both before
and after consent. Provider full typed state/closure revalidation remains the
cleanup mutation/settlement authority.

Prior-runtime cleanup now has a distinct source path. The fixed bounded SDK
`lan-cleanup-observation` role reads protected metadata2 plus journal3/4/5 and
independently observes original host-child exit and exact same-daemon namespace
absence before/after full typed rule state (created3) or exact rule absence
(remove-intent4/removed5). It preserves original SID/PID/creation/image and owner
mapping; metadata does not record a historical session. The current cleanup
helper authenticates the actual current SDK peer SID/session separately.

`lan-cleanup-review` accepts only original protected review ID/digest, refreshes
that native cleanup evidence off the actor and issues a fresh `cleanup-remove`
or `cleanup-settle` review. `lan-cleanup` consumes the fresh review/revision and
operation. Protected fresh records carry an optional typed `cleanup` mapping
and fresh lifetime, while `stage` retains the complete ORIGINAL request and
publication, including the old caller identity. New review, operation and
transaction IDs cannot stage/create/adopt the old lease. The helper reopens the
opaque native cleanup capability and compares its original mapping before
consent; only the purpose-matched remove or absence-only settlement may follow.

Positive cross-owner reconciliation may return `outcome: retired` only after
protected removed5, fresh exact native rule absence, original host-child exit and
namespace absence. `reviewId`, `reviewSha256`, `operationId` and `runtimeInstance`
come from the requested original protected review, independently correlated with
the rule's immutable original metadata; the response also contains `leaseSha256`,
`nativeSequence: 5` and `observedAt`. A missing original runtime, unknown/torn or
foreign mapping, missing closure or unconsumed review never supplies this proof.
It permits a client to retire the matching old pending operation fence without
substituting a local operation ID or adopting any recovered rule as Ready.

No helper/consent implementation should start an engine, workload, WSL distro,
global daemon/profile/context switch or arbitrary program. All native execution,
packaging registration and administrative fixture runs remain root-controlled.
