# Private Docker writer original and recovery boundary

Recorded: 2026-10-26. Source contract; native qualification is a separate gate.
The supported failure model is process interruption with the filesystem's
completed synchronous writes/flushes retained. Sudden power loss, media corruption
and same-user tampering are not diagnosed from a truncated record.

The selected Docker CLI saves a typed `ConfigFile`, dropping unknown root fields.
Both native private owners preserve the exact original raw bytes before releasing
the config to a child. `client/writer.original` is native parent-relative
CREATE_NEW, protected owner/SYSTEM/Administrators three-ACE full-access data.
Write, flush, bounded exact readback, positive writable-handle close and independent
read-only admission precede the loan. Raw originals are at most 1 MiB, are never
inherited, and never enter JSON/base64, logs, diagnostics or review summaries.

## Current schema 3 ordering

Historical schema 2 supplied original preservation but no before-resume journal.
Pending schema 2 is retained/refused; missing birth cannot certify its execution.
Owner-authored `config-replace` remains schema 1 and is a separate transaction.

Schema 3 `writer.prepare` is persisted before original creation. Its closed ten
fields are `schema`, `kind` (`docker-writer-prepare`), `mutation`, `binding`,
`keysSha256`, `baseSha256`, `baseVolume`, `baseHigh`, `baseLow`, `baseSdSha256`.
It binds the actual native source before any child authority is released.

Schema 3 `writer.intent` has the same source fields, kind `docker-writer`, plus
`originalVolume`, `originalHigh`, `originalLow`, `originalSdSha256`,
`originalSha256`. The intent and prepare records are bounded to 4096 bytes.
Mutation values are `registry-entries-v2` and Go's `root-unchanged-v1`; reviewed
restoration accepts both because it discards all uncertain child config output.
Volume/index values come from retained handles. Descriptor hashes cover the
self-relative SE_FILE_OBJECT OWNER|DACL descriptor returned by GetSecurityInfo.
Digests and random one-use bindings use lowercase hex. `keysSha256` hashes the
Go encoding/json compact array of sorted deduplicated command-derived keys (at
most eight keys of 2048 UTF-8 bytes, no controls, empty scope `[]`).

Only the suspended-process native producer constructs `NativeBirth` from the
actual returned root HANDLE/PID, creation FILETIME and verified session/image.
The one-use admission owns duplicates of the actual private parent and exact
intent/original capabilities. Before first ResumeThread it creates, flushes,
reads back and natively reopens `writer.birth`. The closed eight fields are
`schema` (3), `kind` (`docker-writer-birth`), `binding`, `intentSha256`,
`intentBody`, `rootPid`, `rootCreated`, `rootSession`. `intentBody` contains only
exact UTF-8 intent metadata, never raw config. Birth is bounded to 8192 bytes.
Failure or cancellation before resume retires the actual suspended owned job;
only the native checked-retirement producer may release the pending controller.

## Review and restore

Review reacquires the actual exclusive controller and private ancestor/init
provenance. It retains native identities, descriptors and bounded byte digests
for original, current config, journals and any exact `config.stage`. A complete
birth requires an actual kernel process-generation query: positive root signal,
nonexistent process, or a different creation generation permits config recovery.
An alive, inaccessible or unknown matching generation refuses it. Its session
is checked against the actual recorded generation, rather than the reviewer's
new login session. No PID is killed by review, and no engine or WSL is restarted.

Missing or EOF-truncated birth/intent is a pre-resume cut only under the schema 3
write-before-resume protocol and an independently unchanged exact native source
ID/descriptor/digest. Complete malformed or mismatching metadata is refused.
Prepare-only EOF cuts require all other journals/stages absent and the actual
strict owner config to pass managed-routing parsing. Explicit review can retire
that exact partial prepare; if current selectors need refresh, it first persists
a replacement complete prepare from retained native source metadata under the
same controller. No path/hash or renderer Boolean constructs execution proof.

A partial original is accepted only as a proper byte prefix of that unchanged
native source before a complete intent/birth exists. Explicit restore persists
a full recovery intent, retires any partial intent and that exact partial original
before changing the source, then publishes from the still-held exact source.
A post-loan original digest mismatch remains a refusal.

The backend retains an opaque, one-use review capability with a five-minute TTL.
Only a fixed secret-free summary is exposed. Consuming it rechecks every retained
capability. It restores original raw bytes with current owned selector refresh;
Go-compatible JSON escaping makes refreshed target bytes identical across owners,
including HTML characters and U+2028/U+2029. Unrelated raw values stay verbatim.
Authentication and helper/vault mutations are never resumed or replayed; callers
must refresh actual credential state because those effects may have completed.

Historical schema 1 `recovery.intent` preceded stage disposal and publication:
`schema`, `kind` (`restore-original`), `binding`, `currentVolume`, `currentHigh`,
`currentLow`, `currentSdSha256`, `currentSha256`, `restoreSha256`, `intentBody`.
It is bounded to 8192 bytes and remains readable. Retry admits either its exact
prior native current or the strictly owner-authored published target digest.
Current source writes schema 2 metadata and advances changed targets through the
durable successor protocol below; it never guesses a missing schema 1 selector.
An EOF partial recovery record
has no authority: independent source journals must validate first, and explicit
restore retires that exact partial record before durably replacing it.

An exact private stage from interrupted normal settlement or recovery may be
reviewed and discarded only under those authenticated pending records and actual
stopped/no-resume evidence. There is no directory sweep or ordinary-startup
adoption. Original/config native final checks precede cleanup. Intent is removed
before original, then prepare. A complete birth is removed last, preserving root
and embedded intent metadata across earlier cleanup cuts. Without complete birth,
recovery intent is last; incomplete birth is retired while source records remain.
Birth-only completion requires actual stopped generation and strict native
owner-config/routing checks, and exposes explicit cleanup finalization.

Cancellation, TTL, Drop and refused reviews preserve records. Checked close can
release the controller for another explicit review. Confirmed live retirement
can likewise release pending ownership without adopting uncertain child bytes,
including after normal storage settlement failure. Data and parent capabilities
use checked CloseHandle on every explicit or implicit drop. Ordinary synchronous
write/flush/validation failure closes its known handle and keeps partial bytes;
only actual unknown native close/resource ownership or native successor
publication uncertainty quarantines the owner domain and
retains the controller. A healthy or refused read-only review closes admission
and does not permanently fence the config. Writer/local review opens existing
namespaces only; fresh initialization's control-only gate creation is separate.

## Evidence and remaining qualification

Pure tests cover closed schemas, duplicate/unknown fields, legacy refusal, exact
intent/birth bytes, every EOF prefix of a birth record, producer journal failure
and cancellation before resume, managed-string escaping, and existing raw-value
merge/commit fault cases. They do not qualify Windows process/filesystem crash
behavior or the composed UI journey.

`artifacts/qualification/p2-credential-writer-native-source-v1-20261026/recovery-candidate-1`
contains a source-only recipe using actual whole modules and namespace-only
projections: live cancellation/restore, review close/reopen, failed normal
settlement with stage recovery in the same process, and creator loss before
birth with cold native reacquisition. It has not been executed by the recovery
implementation agent. Independent source/closure review and native qualification
remain separate gates. Legacy schema 2 and genuinely malformed, unbound or
externally modified records stay retained/refused; no automatic reset exists.

Root subsequently froze, independently reviewed and executed recovery candidate 1:
the three same-process cancellation/review-close/failed-settlement recovery cases
passed. Cold preparation refused before creating writer records. Inspection found
its fixture supplied an equivalent Windows Path with a different OsString
separator spelling; exact command admission correctly refused it. Candidate 2
derives argv from the actual native guard path and preserves static diagnostic
stages. It is a fresh cold-only source candidate; candidate 1 evidence and its
nonce are preserved. Partial-journal and cleanup native fault coverage remains
open regardless of these intermediate results. Root subsequently froze and ran
the corrected candidate 2 cold flow: both creator-loss and cold-review phases
passed in 1.759 seconds with the same five production-module hashes as candidate
1. This confirms four native recovery journeys across the two candidates; it
does not replace the additional producer-cutpoint plan in
`tests/registry/private-writer-recovery-fault-plan.md`.

Root independently froze and reviewed candidate 3, including all actual producer
hooks and its 182-file closure. All twelve cases and thirty finite phases passed
in `native-run-08c3ed51-b62a-4f7e-a5bb-035c60f948e0`: EOF prepare/original/intent/
birth/recovery-intent/stage, interrupted intent/original/prepare cleanup, completed
no-birth cleanup, birth-only finalization and completed birth cleanup. The plan
permitted at most three inert native births; one remained suspended before user
resume and two were actually cancelled/retired before recovery. The debug-only
seam operated on actual native producer handles and exact cleanup, with no
external record mutation or fabricated execution capability.

Those observations qualify these individual process-interruption cuts within the
frozen fixture scope. They do not qualify compound interruptions, sudden power
loss, media corruption, schema 1 config-replace/fresh initialization recovery,
legacy/schema 2 adoption, real vault/authentication effects or an upgrade across
a pinned recovery target change. Cross-version target mismatch remains a safe
refusal and needs upgrade preflight exclusion or a successor-journal protocol
before the broader upgrade acceptance criterion can close.

Root's cross-producer candidate 2 passed all six native phases in
`native-run-09805893-3a9e-4943-a67f-908f0a386afb`. Go-to-Rust and Rust-to-Go
no-child restores checked baseline digest, actual file ID/descriptor, new owner
file, cleanup and healthy reopen. Actual selected role images were held/read
only; no role or child ran. Frozen hashes matched the then-current source.

Root's admission candidate 1 passed all seven cases in 9.272 seconds in
`native-run-2a257969-8cf4-464c-89a6-3809fba1941a`, with its reviewed 144-file
closure. Actual producer/control/diagnostic/operation paths released known
stopped admission and retained actual unknown owners/generation capabilities.
The allowed ten roots remained suspended before user resume. This is admission
evidence, not writer-storage authority or complete SDK/engine/UI qualification.
Actual syscall-close failure remains a separate native qualification gap.

## Local managed-setting replacement: source implementation

The explicit review entry point also recognizes a separate, fully closed schema
1 `config-replace` intent. This producer never lends the config or starts a
child. Its authority is the actual exclusive native controller, authenticated
initialization/root chain, retained strict owner config, and exact private intent
and optional stage capabilities. It does not use schema 3 birth or writer proofs.

The Go producer records exactly `schema`, `kind`, and `baseSha256`; Rust also
records `nextSha256`. Both closed shapes are accepted. With the fourth field,
the current digest must equal either recorded digest. The three-field producer
does not preserve a target digest or original, so review explicitly offers to
keep the validated current settings; it does not claim to restore a baseline.
Null, alias, unknown, duplicate, partial and malformed fields refuse. Writer
prepare/original/birth, recovery and snapshot journals must all be absent.

Review binds the current native file ID, descriptor and bytes, and the optional
stage's native ID, descriptor and digest. Consuming review rechecks these actual
capabilities, explicitly discards only the captured unpublished stage, rechecks
the current settings and absence of stage, and clears the exact local intent
last. It then performs any current managed-selector refresh as a new ordinary
durable transaction under the same held controller. Cancel/expiry retain bytes;
recoverable errors release checked native ownership while retaining remaining
records. Actual unknown native close retains quarantine. There is no auth/vault
replay, automatic adoption, initialization repair or legacy import.

Rust parser/current-classification tests and compilation passed for this source
path; the Go counterpart has separate pure checks. Native local-replace
cutpoint/review qualification is still pending and is not covered by the earlier
schema 3 or admission receipts. Fresh `client.create-intent` recovery and
cross-version recovery-target succession remain separate unresolved work.

Root subsequently reviewed and ran local-replace cross-owner candidate 1. All
eight finite phases passed in
`native-run-766edd71-5128-46eb-a741-503795af145c`. Its 120-file frozen closure,
current Go/Rust source projections, both exact lock byte generations and all six
probe binaries were checked before a separate dispatch. Go-to-Rust and
Rust-to-Go used actual ordinary selector replacement blocked by a native
read-only observer denying delete sharing. Both produced full local intents and
stages; opposite-owner explicit KEEP CURRENT application preserved raw auth and
unknown values, changed the selector to generation B, created a new config file,
removed exact journals/stages and reopened healthy. Selected role PEs were
published/read only; no child or role executed. This receipt does not qualify
EOF local intents, fresh initialization or recovery-target version succession.

## Fresh initialization: separate source capability

Fresh initialization now has a distinct native capability behind the existing
opaque review/close/apply interface. It never constructs an incomplete ordinary
guard. Normal client admission continues to refuse `client.create-intent`.
The only marker accepted is the exact producer literal
`{"schema":1,"kind":"fresh-client"}` or a proper byte prefix, including empty.

The actual producer writes, flushes, reads back and checked-closes this marker
before creating `client`, then exclusively creates `controller.lock` before
`docker`, `client.init` and default `config.json`. Review first uses broadly
shared read-only marker/tree inspection, preserving a live creator's access. It
accepts only the native private client inventory `controller.lock`, `client.init`
and `docker`; Docker's only permitted entry is `config.json`. Controller bytes
must be empty. Unknown names, foreign descriptors, reparse points, multi-link
files and unrecognized contents refuse with bytes retained.

Only the missing client directory and exclusive controller gate may be created
during review preparation. Winning either creation collides with the original
creator's actual `FILE_CREATE` before it can publish further initialization
data; an already live exclusive controller refuses. After winning, review
reopens the client root with ordinary deny-delete sharing and binds the same
native ID/descriptor before checked-closing its preflight cap. Docker and record
caps are likewise reopened with ordinary sharing. Immutable marker ID,
descriptor and bytes and the full native tree witness must match preflight.
Dedicated temporary list handles bind enumeration to held directory IDs and
descriptors; ordinary ancestor access rights are unchanged.

Full `client.init` must contain exactly schema 1, product `orbwin`, current native
owner SID, actual client root volume/high/low ID and admission `fresh`. An
incomplete record must match a compact prefix of permutations of those exact
seven key/value encodings. Full defaults contain exactly empty `auths`,
`credsStore:"orbwin"` and one canonical native-profile private Docker-cache
plugin selector. Partial defaults match only compact producer-field permutations
and the exact native managed path pattern; only its 64 lowercase nonzero hash
bytes vary. Partial string encoding admits explicit serde UTF-8 or Go-compatible
HTML/U2028/U2029 encoding. Generic JSON EOF, unknown fields, credentials, aliases,
arbitrary escaping or noncanonical selectors do not authorize repair. Init and
default records are bounded to 8192 bytes, including escaped native profiles.

Explicit apply repeats native provenance, exact tree/witness and legacy-denial
checks. Under the same controller and still-present original marker prefix it
creates missing known entries or exactly replaces admitted metadata/default
fragments using private `CREATE_NEW`, flush/readback and checked close. Missing
entries and a partial marker alongside full defaults remain recognized retry
cuts. Before removing the exact marker last it verifies full current init,
current default-only routing and native identities/descriptors. Only then does
it transfer those same actual caps to an ordinary healthy guard. Cancel/expiry
and ordinary errors retain evidence and checked-release known resources; actual
unknown native closure retains capabilities and quarantine. Existing normal
constructor error retention is unchanged.

Source pure tests cover every init field permutation, producer prefixes,
escaping, long escaped profiles, canonical-selector negatives and forbidden user
data; focused config tests passed 21 cases. Native controller preemption,
deny-delete observations, cutpoint recovery and cross-owner initialization are
still unqualified until a separately reviewed actual fixture runs. Missing or
malformed marker with an incomplete tree, legacy import, EOF local replacement,
power loss and pinned recovery-target version succession remain separate work.

Fresh initialization candidate 2 is now frozen and source/build checked at
`artifacts/qualification/p2-fresh-init-native-v1-20261026/candidate-2`. Its
499-file signed closure contains fourteen Rust and eleven full-package Go probe
binaries for fourteen fixed fresh namespaces and twenty-nine planned phases.
Candidate 1 remains preserved as a Rust fixture compilation failure; it never
executed. Candidate 2 includes an actual zero-byte marker cut, marker/init/
defaults prefixes and full producer steps, two planned interrupted repair
phases, checked review close, exact marker-last completion and three actual
creator-thread exclusion/busy cases. Debug checkpoints are one-shot, inactive by
default, excluded from release, and gated on actual native private parent
identity/descriptor and the exact frozen proof namespace. Selected role images
are copied/read only. Raw self-relative descriptor witness encoding matches the
actual Go helper; observer handles and local descriptor allocations are checked
on successful release. The compile proof records the exact resolved Rust
compiler path/hash/version and linked Go inputs, rechecking production sources
through finish. Current focused config tests passed twenty-three cases. This
is source/build evidence only; independent full-closure review and native
dispatch remain separate, and no planned phase is reported as observed here.

Root's candidate 2 native dispatch stopped at the first case in
`native-run-3c9fffd1-4e71-467d-b988-4090b4cf8b27`. The empty marker producer
cut passed; Go review then admitted a DELETE-access directory open and correctly
failed its qualification assertion before applying initialization. Metadata-only
retained directory opens did not enforce the assumed delete-sharing fence.
Microsoft's [MS-FSA sharing algorithm](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-fsa/8c0e3f4f-0729-49f4-a14d-7f7add593819)
requires a participating data/execute/write/delete access bit on an existing
open. The prior source-level deny-delete claim was therefore unproven and, for
these handles, false; earlier exact receipts do not establish that property.

The source correction requests FILE_TRAVERSE on retained config/cache directory
and volume-root handles. It adds no ordinary directory listing; explicit list
caps keep their dedicated rights. The fixed legacy-denial volume probe remains
metadata-only because it grants no admission/path-freeze authority. Pure mask
tests check the policy, not kernel behavior. Fresh candidate 3 uses new nonces
and the same finite bounds, adding actual DELETE-sharing denial observations for
held runtime, Docker bundle and credential bundle directories as well as client
and Docker config directories. Candidate 2 and its failed namespace are retained
untouched. The correction still requires separately reviewed native evidence.

Root's candidate 3 dispatch
`native-run-8f37ab00-1560-473a-9145-b41c3aafaf3f` observed client DELETE-sharing
refusal after actual opaque review, then stopped during explicit apply before
init/config publication. This second failure was a fixture projection mismatch:
the Go defaults recognizer embedded the product namespace inside long raw
strings that the exact literal overlay did not replace. The pure helper now
uses one double-quoted private-directory constant in both builders, with
unchanged product bytes. Candidate 4 checks that projected constant against
the frozen case namespace both statically at compile and before native calls.
All candidate 4 nonces are fresh; candidate 3 and its failed namespace are
preserved. The partial client-sharing observation does not qualify the full
initialization or cache-directory matrix, which still awaits independent run.

Root independently reviewed candidate 4 and dispatched the corrected frozen
matrix. All twenty-nine phases across fourteen cases passed in 199.275 seconds
in `native-run-50e3035e-a7e8-4b04-a588-678efe32e75c`. The authoritative receipt
is `root-native-receipt-v2.json`; an initial PowerShell-derived timestamp
conversion error is preserved and superseded by its raw ISO timestamp metadata.
Observed scope includes fourteen planned creator/repair process-loss cuts,
ninety-five actual DELETE-sharing refusals (forty-two on cache directories),
checked review close, exact marker-last completion, live creator preemption and
busy-controller refusal, unchanged empty defaults and healthy reopen. Selected
role images were copied/read only, with no role execution, native child,
engine/vault/WSL/UI/admin action or old nonce cleanup/retry.

This closes that frozen fresh-initialization cutpoint matrix, including the
specific retained-directory sharing property it asserted. It does not establish
the whole phase, arbitrary interruption combinations, actual unknown-close
handling, sudden power loss, corruption, legacy import or cross-version writer
restore-target succession. The failed candidate 2/3 evidence and namespaces stay
preserved. The successor journal protocol remains a separate source/design and
native qualification task.

## Recovery target succession: current source, native proof pending

`client_config_successor.rs` supplies the called bounded metadata/data
coordinator; `client_config.rs` retains all native authority and publication.
New recovery records use a closed flat schema 2 with twenty-four required fields
in Go struct declaration order. In addition to the ten base recovery fields,
they carry `targetSelector`, `publishedBaselineSha256`, predecessor volume/high/
low/descriptor/body-digest/restore-digest fields and explicit stage-present/
volume/high/low/descriptor/body-digest fields. Empty predecessor or absent stage
requires all associated fields empty/zero. There is no recursive predecessor
journal or raw config/credential body. Exact original `intentBody` remains
unchanged metadata. Go-compatible JSON escaping and fixed field order define
producer bytes; the metadata read/encode bound is 16 KiB. Pure tests exercise
large escaping beyond 8 KiB within the conservative 15,501-byte maximum.

Both review preparation and explicit consume call the same native-bound restore
plan. It validates original/current/journal capabilities and actual stopped
generation or independent schema 3 pre-resume source before current-selector
rewrite. An exact original A can restore despite corrupt writer output. Without
original, current B must first be strict native owner data with its own canonical
managed selector and a proven owner-published digest, not merely old prestate.
Schema 2's published-baseline witness plus exact recorded current identity/
descriptor/digest carries B across an uncommitted C target to D; a planned C
digest never substitutes for an actually published baseline. Its immutable
stored target is independently reconstructed from the validated baseline and
stored selector. Selector metadata is data only; current runtime authority comes
from actual held selected-role capabilities.

Only a complete bound OLD recovery record admits `recovery.next`. Review binds
OLD's native identity/descriptor/body, current and optional stage witnesses,
original intent/binding and the actual stopped/no-resume sources. A partial NEXT
supplies no authority: it must equal an exact fixed-order producer prefix under
these independent witnesses, with only old target digest and canonical managed
generation varying. A full older NEXT requires matching authority and a target
digest reconstructed from the actual baseline. Explicit consume may dispose
only that exact captured partial/stale object while OLD remains intact.

The actual private CREATE_NEW NEXT write, flush, readback, checked writable close
and independent native reopen all precede any OLD/config/stage effect. Sources
are then rechecked again. Under the same exclusive controller and held private
parents, publication checked-closes OLD and atomically renames held NEXT over the
fixed `recovery.intent` name, checked-closes it, reopens exact new identity/
descriptor/bytes and verifies NEXT absent. This is cooperative name replacement,
not kernel destination-ID compare-and-swap against arbitrary same-user writes.
Actual rename/post-publication uncertainty or close failure consumes the attempt
and retains the real owner/caps; no publication retry converts it to success.
Ordinary/local/partial-prepare paths refuse NEXT, and fresh inventory refuses it.
NEXT without OLD or mismatched source linkage stays retained/refused.

Completed birth-last cleanup is a separate mode: full exact schema 3 birth and
embedded intent, actual stopped generation, strict current B/canonical selector,
absent writer intent/original/recovery/NEXT/stage/snapshot and optional matching
prepare. Consume rechecks actual B and all retained capabilities, removes prepare
then birth last, and only afterward begins a new ordinary selector replacement
under the same controller. It never invents original A or a lost restore hash.

Pure tests cover the actual called data/order coordinators, closed codec,
escaping/bounds, every exact NEXT prefix and failures before later effects.
Concrete native callees are implemented, but atomic replacement/process-loss,
cross-owner repeated upgrades, current/stage cuts and actual unknown-resource
qualification remain pending a separately reviewed native fixture. Historical
schema 2 writer intents remain refused; recovery schema 2 is a distinct record.

## Checked immutable snapshot reference release

The snapshot wrapper's explicit `close` and Drop both release their actual Arc
through `Arc::into_inner`. `Shared` means this reference was released while
another real reader owns the immutable graph; it is not a retirement or engine
grant. Only the atomic final winner obtains the graph and returns `Retired`
after checked native closure. No strong-count observation decides retirement.
Native fallback Drop uses the same release routine for internal transient Arc
error paths. Config/init, descendant-to-ancestor handles and the directly owned
user token close in checked order while role references remain held. The first
actual unknown close retains every remaining cap/reference and quarantines the
domain. Successful setup and worker paths can observe explicit final-close
errors before publishing success; concurrent shared readers remain supported.
Pure tests exercise sixteen concurrent actual Arc releasers, weak resurrection,
shared-reader preservation, final-error propagation and close-order faults.
This source refactor does not overhaul pre-existing cached-executable final
release or other owner-token paths, and does not claim native close-fault proof.

## EOF local replacement and attempted-effect suffixes: source only

The called recovery dispatcher gives any independent writer prepare/original/
birth/recovery/NEXT record to the writer provider before considering a partial
local intent. A no-sibling local EOF intent must be an exact bounded compact
Go three-field or Rust four-field producer prefix, bound to the actual strict
current config digest and canonical owned selector. Stage and ancestor creation/
import markers must be absent. Explicit KEEP CURRENT apply rechecks the retained
native graph, disposes only the captured intent last, then starts an independent
ordinary selector refresh under the same controller. It creates no child-stop
or no-resume authority. Pure tests cover both producer shapes, every prefix,
writer-dispatch regressions, forbidden siblings, raw-value preservation and the
called effect ordering. Native EOF-local qualification is still pending.

Ordinary replacement and reviewed writer restore now share one target-publication
callee. It arms private publication uncertainty before the actual rename attempt,
retains the stage in the owned graph on failure, checked-closes after positive
rename, and retains the reopened target before verification. Exact native ID,
descriptor and bytes, origin checks and guarded old-stage-name absence must all
succeed before clearing that state. Metadata NEXT publication has the same
guarded old-name suffix. A generic rename error does not establish that nothing
was published; it consumes the attempt and preserves the controller graph.

Actual disposition errors retain the DELETE-capable file and quarantine the
domain. After positive disposition and checked close, failed absence queries or
unexpected remaining objects also quarantine; an unexpected actual cap is
retained. Known failures before any mutation attempt remain distinct. These
source changes supersede the earlier exact-source same-process sharing-conflict
behavior; historical native receipts are not proof of the new suffix handling.
Pure order/failure tests are not evidence of an actual kernel close failure.

The source-only config test filter passes 37 tests, including the closed
qualification-event test. Its schema 2 codec
golden executes the actual Go encoder through a pure test overlay and matches
Rust's 1,924 bytes, SHA-256
`fe06a5b09f0301a93c864ed128487f2a13e156e2560429a04c5a9e570c359092`.
No private provider, native fixture or selected role runs in that golden test.

## C7A successor fixture: observed frozen matrix

`artifacts/qualification/p2-successor-cross-native-v1-20261026/candidate-4`
freezes the actual seven Rust modules and full Go package, including successor,
EOF dispatch and central mutation helpers. All 48 Rust and 16 Go probes compile;
eight fixed inert role images also compile. Root independently reviewed the
closure and separately ran candidate 4 as recorded below.
All sixteen Go build-pin sets pass an actual `cacheRole` pure preflight through
an overlay that removes the native probe TestMain. Ordinary codecs and guards
remain unchanged; only the two inert-case copies bind the audited schema-pin constant
to their fixed inert Docker image hash. Every native ID/SD/capability check stays
in the actual providers. This fixture projection does not qualify selected
Docker's argument behavior.

Candidate 1's Rust harness ownership build failure remains preserved. Root's
separate candidate-2 native run
`native-run-953b64e4-8f99-42b7-a725-35f711d8d157` stopped after six launches and
one inert birth: Go B rejected its inert Docker hash against the unchanged
selected-Docker schema-pin constant before native cache/recovery admission.
The saved pending marker was named `INERT_RETIRED`, but it established no positive
retirement or Sync/Close handshake. Candidate 3 uses `CORRUPT_PENDING`; the cold
provider must independently establish the recorded root's stopped/obsolete
generation. The failed namespace and artifacts are preserved without inspection,
retry or cleanup. Hooks 5/6 and the successor recovery gates were not exercised
by that stopped matrix.

Root's separately dispatched candidate-3 run
`native-run-901bc48b-28d5-4ead-b13f-fa5aa7e7c102` completed the original-A/corrupt-
output Go-to-Rust case, then stopped at reverse Go C review after fifteen phases
and two inert births. Saved artifacts plus source identify a fixture-only format
defect: the Rust historical hook encoded a ten-field live base with schema 2;
Go correctly required twenty-four fields for that schema. The private namespace
and processes were not inspected or retried. Candidate 4's debug-only called
encoder clones the live base, sets only the clone's schema to 1, and validates
the closed old-format output before native creation. Its exact 1,126-byte pure
golden, SHA-256
`01e766dc539eedd9d422aa70ea02b799e56b6a5ba8daa230b7ecf10c9e61e67c`,
passes Go's actual parser in all sixteen preflights; ten-field schema-2 output
remains refused. Normal schema-2 plans/codecs stay unchanged.

Root independently verified 1,302 frozen files, 49 production origins, projections,
compiler/PE pins and the historical-codec golden, then separately dispatched
`native-run-df64ba09-c8b5-43e5-bc8c-c5f4511026cd`. All twelve cases / 96 phases /
two inert births passed in 620.193 seconds. Root rechecked the frozen closure,
current origins and all receipts afterward; its receipt SHA-256 is
`3c697131459d11a01a329108202bcebafbc68cad3c1d6bfd5c079ad0c619c896`.
This establishes the six frozen scenarios in both directions: corrupt current
with original A, published B without original, exact partial/full NEXT,
post-metadata stage retirement and published B through uncommitted C to D.
It does not exercise immediate pre/post-metadata-publication cuts (hooks 5/6),
actual kernel mutation/close faults, local EOF, snapshot final-close faults,
power loss, corruption, legacy import or whole product/phase acceptance.

A test-only cfg mismatch identified during the freeze was corrected after the
hold released: the historical encoder test now has the same debug gate as its
helper. Normal release product builds exclude the hook. The requested full
release-test check currently stops in the existing build-input pin guard because
`artifacts/backend/rdd-forwarder.exe` is absent; that check is not qualified by
the debug fixture run or focused tests.

The fixed plan has twelve namespaces and 96 outer phases. Ten namespaces copy/
hold selected role PEs without executing them. Only the two original-A/corrupt-
output namespaces may run their separately built fixed inert Docker role through
the actual native writer producer. Two such births are planned, within the
admitted maximum of twelve; each has three seconds of work and twelve seconds
of retirement. Four formatting-only cache generations per namespace have a
calculated private copy bound of 5,666,990,080 bytes, below 7,680 MiB total and
640 MiB per namespace. Every outer phase is bounded to 45 seconds.

The debug/tag-only one-shot hooks cut actual NEXT writes/durability/publication,
stage retirement and published-B cleanup. Historical schema-1 encoding is an
explicit old-format injection at the actual durable record creator, not an old
binary or a native authority constructor. Normal producers retain schema 2;
Rust release excludes hooks and ordinary Go builds use no-op callees. Native
parents and fixed compiled proof namespaces guard every active checkpoint.
The runner preserves stdout/journal bytes and uses decimal-string observations;
it never parses native generation integers through JavaScript Number. Separate
root closure review and a separate native dispatch remain required for every
new matrix. Local EOF, mutation lost-result injection and snapshot native cases
belong to C7B.

## C7A2 immediate publication cuts: observed frozen matrix

`artifacts/qualification/p2-successor-publication-cuts-native-v1-20261026/candidate-1`
freezes four new namespaces for hooks 5/6 in both owner directions. Each has
eight phases: A setup, B/C/D cache-only publication, A no-child loan, B's actual
historical full-stage creation, C immediately before/after verified metadata
publication, then opposite-owner D review/apply and healthy reopen. Sixteen Rust
and six Go probes compile. Role pins stay
selected and unchanged; the private copy bound is 2,238,494,720 bytes. At C before
real review and D under real review, read-only observers record the same actual
stage ID/raw descriptor/body hashes and validate its B selector/raw values.
These observations are test correspondence, never native admission authority.

Its actual async scheduler admits at most two probes in disjoint namespaces,
preserving all eight phases' order within each case. First observed failure
closes global admission; already-owned probes may finish bounded, but no later
phase or case starts. Timeout/output-limit termination uses only Node's returned
fresh direct ChildProcess object, never a PID from metadata/output or an old
namespace. Pure tests cover ordering, overlap and a failed latch while another
owned probe still finishes. Every probe has 40/45-second inner/outer watchdogs
and 64 KiB per output stream; truncation is failure, not verbatim success.

After the cfg repair, the actual seven-source-module graph with the same fixed
runtime-pin data functions passes isolated `cargo check --release --tests`
with release debug assertions disabled. This is module test typechecking only,
not the blocked full application's build-input gate. All 37 focused debug tests,
six actual Go parser/role-pin preflights and scheduler tests pass. The actual
historical-encoder golden is re-captured against this source epoch. Root must
review all 485 frozen files, projections, runner/scheduler, compiler and PE pins
before any separate native dispatch. Root performed that independent review and
separately dispatched `native-run-5ad07805-4f2c-4c55-b838-4860895760f7`.
All four cases / 32 phases passed in 136.349 seconds, with zero writer births and
actual maximum probe overlap two. Root rechecked all 485 frozen files, 49 source
origins, phase order, receipts, actual overlap and C-to-D stage correspondence;
the resulting proof SHA-256 is
`c4a0748c7ded0f66c599ef16cb1694d9c6302f7e53b23206aa8734a857c18fba`.
This qualifies these exact immediate pre/post-publication process cuts with the
actual retained stage in both directions, not lost native results, close faults,
power loss or whole product acceptance. C4 and every earlier failure stay
immutable. The subsequent BL022 raw-contract source adoption is a new epoch;
future fixtures must include the actual new raw module/helper.

## C7B native matrix and preserved failure

The source/build native matrix separates local EOF producer cuts, real immutable
snapshot Shared/final reference release, and deliberately lost successful native
returns. The latter would execute the real operation with actual retained caps,
then inject an error before the suffix proceeds; it would not be evidence of an
actual kernel error. Any snapshot close-loss case must retain the remaining real
graph after a positively closed cap is consumed, never pretend that closed cap
remains live.

After root adopted the BL022 guard and durable tests, the actual eight-module
Rust graph and full Go package were frozen in
`artifacts/qualification/p2-private-config-c7b-native-v1-20261026/candidate-1`.
The closed 22-case /106-phase plan has zero writer births and copies/reads only
selected role images. Its private copy bound is 7,316,059,136 bytes, within
7,680 MiB total and 640 MiB per case. At most two disjoint cases may run; the
observed-failure latch stops new phase admission while already-owned probes
finish bounded. Native journals remain opaque to JS and observed integers
remain decimal strings.

The closed debug/tag events cover four actual local producer prefixes, a real
schema-3 intent prefix with prepare/original siblings, post-positive metadata/
target rename and stage-disposition lost results, and final snapshot config-
close lost result. Every active event validates the actual fixed native parent
and runs only at the actual producer/callee. Stage disposition observes real
DeletePending on the retained DELETE-cap file. The snapshot event consumes a
positively closed config cap and retains init/ancestors/token/roles; it never
pretends that closed cap survives. Normal release/ordinary Go builds have no
active injection. No path/PID/hash/offset or persisted flag can enable it.

All 52 Rust and 18 Go probe images compile. Actual-module release-test
typechecking, 40 focused Rust tests, Go ordinary/tagged/race/vet checks, eighteen
actual Go codec/role-pin preflights and actual scheduler tests pass. The signed
closure has 1,522 files and 52 current source origins; all hashes and namespace
projections were rechecked. Root independently reviewed that closure and separately
ran `native-run-27be9c34-d74d-4d44-8bf3-07736d767d09`. The run produced 104 passing
phases from 105 launches, with 21 complete passing cases and zero writer births,
in 500.187 seconds. The Go ordinary-target lost-result producer hit its inner
watchdog after its actual positive target rename and injected lost return. The
source diagnosis found a real recursive mutex acquisition: ordinary acquire held
the retained-owner registry mutex through native refresh, while its uncertainty
fence tried to acquire the same mutex. The failed namespace and all earlier
receipts remain preserved; this run does not qualify that failed journey.

Root fixed the actual registry coordinator to reserve strong ownership under a
short lock and run the complete native acquire callback outside that mutex,
preserving retained error/panic/fenced-success behavior. Focused pure, race, vet
and compile checks passed; independent source review found no additional defect.
Root then independently reviewed a NEW two-case/eight-phase regression closure
and separately ran `native-run-2e7bb74d-4147-44ca-a749-f3a10b02d432`.
Both opposite-owner ordinary lost-result journeys passed in 34.273 seconds,
with peak overlap two and zero writer births, including actual live controller
and target sharing refusal, cold opposite-owner explicit recovery and healthy
reopen. Root reverified 178 closure files, 52 origins, 106 source copies,
38 overlays and all eight receipts; root receipt SHA-256 is
`f5b4ca4ba53e3f5691381c0b9081eae1f13b829f03e329a43c245fae775ae194`.
This is observed post-positive-operation lost-return behavior, not an observed
Windows kernel error or a replacement for power-loss, full installed setup,
legacy import, or whole-phase acceptance.
