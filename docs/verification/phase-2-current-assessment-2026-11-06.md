# Phase 2 current delivery assessment

Reviewed: 2026-11-06–07 Pacific. This is a delivery audit, not Phase 2 acceptance.
The accepted scope remains [phase-2-scope](../phase-2-scope.md), with recorded
[exclusions and adaptations](../phase-2-exclusions.md). No scope is removed here.

Latest checked checkpoint: **Phase2 is incomplete.** Reviewed source corrections
are ahead of the last coherent product build; the installed runtime has not been
requalified by this review. The implementation owner retains integration/build
ownership, while this chat supplies isolated reviewed changes and concrete gate
evidence. Passing component tests are kept separate from package, installation
and end-to-end qualification.

Current user steering, confirmed in the implementation chat: **keep native runs
paused**. Source work, reviews, pure stdlib models and compilation continue.
Product/app/engine/cache runs, native filesystem/API diagnostics, UI use and live
target/process queries remain paused. Historical positive preparation and
constructor results precede that instruction; they are not new authorization.
Phase 2 acceptance still requires the deferred runtime matrix.

The latest independently reviewed backend source closure is patch0022
(`0aa0e73d...8d5e408`): all 1210 canonical source files, fifteen changed paths,
679 lock entries and the complete 22-patch replay match. All fifteen accepted
setup paths and prior READ changes remain unchanged. Root has registered that
source (`817d2d98...0a38973`). Its combined SDK successor now passes the source
checks below; current package and runtime acceptance remain separate evidence.

Candidate0023 also passes complete source closure (`0994143b...308b06b9`), but is
not registered at this checkpoint: its new transport calls must be paired with
the patched-library build profile, resource offer and application provenance
validator. The actual27-file JS integration and paired Rust validator pass
bounded reviews (`04d40ce4...8b312c52`, `af845e93...25f63d3e`); active composition
and its checks are separate. The initial profile covers root-rdd0.6.4. SDK's
declared0.6.2 and actual selected module remain independently accounted; this
does not establish a shared patched library across every caller.

A current-source scope check (`a7177328...a7c3344`) corrects two earlier delivery
rows: LAN constructors and UI requests are wired, and the WSL refresh wrapper
reads the current tool-lock digest. Their runtime acceptance is still separate.
The old `qualify-local-service-process.ps1` continues to launch the artifact SDK
directly, which does not satisfy the current SDK's private-cache self-admission.
Preserve it as historical evidence; a current product-entry qualification fixture
is required. These findings do not justify rewriting the existing feature flows
or treating all remaining Phase 2 work as blocked only by the native pause.

The minimal current-SDK fixture plan (`f45e6f3c...1f9405f0`) reuses the real
product target/session-cache path. Its implementation intake confirms two missing
admission interfaces: SDK startup loads fixed-profile saved state and constructs
NativeLAN before the later ACK/Snapshot, and mutation/stop RPCs do not require
the originally captured server generation. Pipe absence, filesystem absence and
an opt-in environment value do not close those races. The independent
actual-ServeLaunch/sole-waiter observation seam is now delivered
(`7ef994f0...2cff21a`), with six pure helper tests, full isolated caller compilation
and two bounded source reviews passing. A positive restart fixture remains
blocked on a real exclusive-fresh admission and peer-generation
contract. No alternate home, fabricated target or always-refusing placeholder is
counted as completed qualification.

The first restrictive Go SDK source packet now supplies those proposed wire
interfaces, but review blocks adoption (`e016e723...68dbf3fb4`): its body-panic
handler runs after the resource cleanup defers, so a runtime panic can release
the root/client/self owners before the intended retention path. The implementation
owner has now supplied Source2: bounded semantic review (`d7f60f14...9d970678`)
and independent closure review (`ea810f59...98b32608`) pass. The actual producer
now detects a body panic before any resource cleanup; the added pure test covers
that defer order. Its wire contract is byte-identical. The Source1 finding remains
historical evidence, and no native failure was exercised. Rust continuation is
being implemented in a new isolated current-source copy with these exact reviewed
dependencies; active adoption and native acceptance remain separate.

The second Rust continuation passes ten pure helper tests and full isolated
caller compilation, but semantic review blocks adoption (`68017639...3c88502f`):
its test `.expect` can end a standalone test process while the original service
and lease remain in a background waiter. A small successor keeps that same test
host alive until the existing owners settle; it adds no second process waiter or
restart permission. The separate Go stop-reply liveness race now has a paired
reply-observed ACK correction in Go Source3 (bounded reviews `ff43bd02...0576ea`
and `ac61836a...1119f3b`). Rust's final source must use that exact SDK dependency.
Neither the compiled selector nor these source corrections are native acceptance.

The final Rust Source3 is now delivered (`c5090bb4...687672e`): independent source
and host-semantics reviews pass, and full fresh-target caller compilation passes
76.808s on all796 unchanged inputs. It reuses the exact previously tested lifetime
module/ten-test evidence; no test executable was run in this epoch. Only three
Rust files may be adopted, together with Root's separate paired Go3 slice. The
historical canonical21 compilation context must not overwrite active22. Root has
adopted only the guarded slice into current22 (`3c989863...2e1a186`). Thirty-two
selected after-pins independently match (`7fa77b24...8e33ac8`), including the SDK
changes and preserved22/security/ordinary inputs. Root's actual604-file composite
checks pass (`09ecffae...8f43704`): ten pure tests, fifteen Node contracts,
release/test compilation and Clippy. The 67 warnings and use of historical
compile-only resources are explicit. This does not qualify a current runnable
package, SDK process, native selector or installed workflow; those gates remain
paused/open. Other backend reader, provenance and lifecycle source work continues.

A separate guest-guard design review (`f9a20f01...a46bbb60`) finds a protocol
contradiction between reusing the other empty/reclaimed history slot and refusing
every empty slot in an established profile. The early paired startup gate and
supported survivor-recovery source paths also remain unimplemented. These are
concrete design/source dependencies, rather than work blocked only by the native
pause. Required supported crash and older-installer journeys must remain safe;
universal containment of arbitrary old binaries or reconstruction of all lost
native handles is not automatically part of Phase 2. Any change to the accepted
survivor policy requires an explicit owner disposition. The five existing Legacy
cache roles likewise need evidence of a changed-path or acceptance failure before
a broad rewrite is treated as a release prerequisite.

The1229-file open-only negative-reader component now passes source/lineage and
bounded semantic review (`1e49abe1...7df3dd5`, `9a9ee528...92d9eb0`). It deliberately
refuses every required mutable entry because the genuine pending producer,
same-process continuation and inherited schema2 endpoint are absent. It must not
be activated alone or counted as a working Phase2 journey. A useful scoped
non-destructive recovery path also still requires source implementation.

Selected Lima lifecycle Source3 passes its declared tracked-worker review
(`c1db50ab...a699f9fd`). An earlier synchronous wslpath failure branch still uses
unchecked recursive cleanup outside that component's claim; the precise inherited
limitation has been sent to the owner for the complete startup/cancellation
journey. Component review does not qualify that whole cleanup path or native use.

The current installer-source inventory (`f03482bc...952d87c9`) traces its exact
maintenance branch to owned sign-in-entry cleanup, with no direct `rdd Delete`
call. Other installer paths invoke prior uninstallers, ordinary app startup and
WebView2; this is not a no-side-effects claim. Historical installer invocation
and downgrade behavior remain unqualified. An old backend's exposed Delete
command alone does not prove that a supported old installer invokes it.

The 2026-11-07 same-build incoming-image component now passes its finite source
review (`ae03079b...3d47038b`): the actual old self/image graph is borrowed behind
explicit parent Close barriers, with retained query outputs and fixed lock order.
It adds no launcher, receiver, transfer or different-build issuer. The six-file
cleanup prerequisite review (`a3655bfd...5382f2d`) found one P2 behavior regression:
a purely noncanonical historical generation name now causes startup refusal
instead of being preserved and counted below the existing retention bound.
The implementation owner has the exact path and minimum correction; no native
failure or unsafe deletion was observed by this review. Other reviewed cleanup
ownership/stop-first boundaries passed, while legacy acquisitions and complete
service/checkpoint retirement remain separate work. These components' executed
tests remain pure DATA tests; their Windows owner tests are compile-only.
Their combined 1181-file backend source now passes exact composition review
(`3e189dc8...7f9ad9c`): eighteen disjoint component paths replay precisely onto
the 1169-file parent, including preservation of the newer Q1 service correction.
The saved checks record sixteen model tests in normal/race modes, vet of three
packages and three test compilations. That source epoch still contains the name
regression above; the four-file successor now passes independent review
(`cfdeb77c...bbc4fbdc`). It restores the original preserve/count behavior before
any child allocation or native effect and still rejects bad parent/context.
The implementation owner has since composed the name correction, the six-file
datastore acquisition component and the four-file Go candidate verifier into an
isolated 1189-file source epoch. The acquisition component now also passes this
chat's finite review (`fcc053b0...f5f0370`). The owner records nine acquisition
and eight verifier model tests in normal/race modes, vet and three package test
compilations. This is neither a coherent desktop/product build nor an installed
qualification.

This chat additionally implemented the accepted four-file Rust candidate staging
slice in an isolated copy (`09018e1b...6a4cb163`). It selects four fixed files
through independent desktop pins, refuses incomplete existing slots before any
copy, uses exclusive creation for a fresh slot, and retains a separate bounded
memo. Ordinary cache preparation stays unchanged. Six production-helper DATA
tests and a full `cargo check --tests --locked --offline` passed; the 577 source
files and 576-file active baseline remained unchanged during verification.
Independent source review passed. The packet is delivered for owner integration;
the owner has now adopted its exact four dormant source files. A two-read
observation verifies those four after-pins, the unchanged installation module and
three original backup preimages (`8ee3de0f...5ee28b6`). No actual cache/target,
product process, key or native qualification was exercised by this chat.
The inherited synchronous native cache/SD/RAII contracts remain explicit
dependencies, including incomplete native preparation. This staging result does
not issue Go launch, writer, checkpoint or head authority. Initial candidate
native acquisition custody is a hard activation gate: the first staging memo did not
retain a partial graph lost inside older helpers, and the older rename helper's
signed-negative test accepted positive pending status. A candidate-only source
successor now passes eleven pure helper tests and isolated full caller compilation
(`9455b602...734ee1f`). It publishes the actual owned graph before provenance or
native entry, retains uncertain original outputs and checks writer close before
the first readonly timestamp baseline. Independent ACL and ownership reviews
passed their bounded source scopes. The implementation owner has adopted its
three exact source files (`d428e962...b71aac7e`); the historical 774-file context must not overwrite current
canonical0020 or setup changes. Native retention, fault and close qualification
remain paused. The separately authorized ordinary Rdd2 adapter has now been
implemented and delivered (`e280244b...c7efb758`): sixteen pure helper tests and
full isolated caller compilation pass on all 778 frozen source inputs. Both
ordinary entrypoints publish the real owner before provenance; the remaining
PID-based Rdd caller now supplies its original process handle. Reusable original
process/image slots preserve the existing identity and liveness predicates.
Independent review passes. The revision also captures complete FILE_ID_INFO
outputs, closing the documented ReFS 64-bit identity gap without an NTFS-only
scope restriction. It keeps source acquisition capacity separate from the
actual final-cache READ21 bound. Root has merged only the three guarded cache
files and two semantic blocks into current canonical21/security6. Five selected
source after-pins independently match (`98a5bddf...4cdb7538`); Root's actual
composition then passes sixteen pure tests, release/tests compilation and Clippy
(`2c2ecfd8...dd27654`). Clippy success does not mean warning-free code. The other
five legacy owners are not migrated here.
Source adoption does not close actual runtime or Phase 2 acceptance gates.

The different-build producer design's thirteen pinned source files match
(`bc8fd0e4...5d6e27`). The existing SDK producer creates a new signing key per
build; it does not establish a stable old-product trust anchor for future rdd
builds. Stable purpose-authorized signer custody, the outgoing compiled verifier
and the actual receiver/head/cold compatibility profile remain required before
different-build activation. No key material was read or generated by this audit.
The constructor finalizer's optimization-mode guard defect is statically fixed
in two stable observed successor files (`f73e5123...ea276e`); that observation
does not qualify the complete parent-final packet or a native constructor run.
The separately frozen parent-final source now passes its bounded lineage review
(`c0732dac...2b4d46b`). Its raw command transport failed before broker entry;
the new encoded-transport source delta passes its own review
(`e82f0306...596bf91c`). Root subsequently reports normal original launcher,
Node and fixture exits for the fresh preparation run. This audit has not queried
its targets. An independent check of the fourteen expressly allowed saved files
now confirms consistent inner-owner completion, fixture protocol and recorded
input hashes (`22adf9bd...9c15b8d`); it does not rehash the original targets or
prove the outer process family terminated. Preparation of the isolated namespace
is distinct from the constructor probe, default product
storage/publication/reopen and the installed acceptance matrix.
The owner also records a historical successful source-projected constructor
probe (`320f2b40...8d3aa6bf`): the actual root/Serving/storage factories and mapped
self origin ran and checked release completed. This chat has only read its
coordination summary, not independently checked its saved run outputs or queried
its targets. It does not qualify the current default product namespace,
publication/reopen or later source epochs.

Full source/patch/lock/replay closure now passes independent review for canonical
0019 (`61579021...bd255ba5`) and candidate 0020 (`53939a68...d5361ee4`). The
checks cover the complete canonical source sets, ordered patch bytes and full
patched-file locks. Registration and Rust provenance-tuple updates belong to the
implementation owner and must preserve the staging changes. These are source
integration gates, not runtime or installed product acceptance.

The latest Serving/build-origin source review found a concrete failed-domain
admission gap and blocked that frozen epoch. Its four-file successor now passes
primary and three independent source/lineage reviews (`63a61114...2b98e7d`):
original Serving membership and sticky failure are checked at actual origin
admission, frame pre/post and completed provenance checks. Concurrent failure is
handled by retaining and fencing the original native output; no atomic global
entry barrier is claimed. The new actual-domain regression is compile-only;
the seven executed tests remain pure DATA models. Root integration, I/O and SDK
consumer composition and native constructor/publication/reopen remain separate
gates. The original rejected epoch is preserved and is not retrospectively
accepted.

The sibling I/O1136 review finds no additional concrete output-custody or
own-ambiguous-rename suffix defect in its bounded source range (`8b7dd966...f3c0eb`).
It identified IO-CANCEL-01/P2: the inherited preparation entry check precedes a
mutex wait and predecessor work, so an already visible cancellation can still
be followed by stage creation/write/flush. Outer caller checks prevent later
admission but do not prevent those effects. The implementation owner has the
exact path and owns correction; no committed-data overwrite or corruption is
asserted. The four-file1139 successor now passes primary and three independent
source reviews (`3d657634...b3953e5`): cancellation is rechecked under the original
mutex, and final admission immediately before FILE_CREATE consumes prepared only
after a fresh context observation. Later cancellation preserves synchronous
settle/retain semantics. Its four recorded tests exercise the production helper
with deterministic Go synchronization; the Windows method is compile-only. Real
staging/cancellation and current Root composition remain separate qualification.

The separate SDK observed-image consumer review is complete but not passed
(`18ac2b26...b7d21b6`): four inherited P2 abnormal-close/replay gaps can leave a
nil success cache after incomplete cleanup. They affect never-resumed retirement,
legacy peer, helper and SDK parent-image paths. No native panic was observed;
the candidate's only static call is error cleanup and the legacy constructor has
no production issuer. Exact source counterexamples and minimum repairs are with
the implementation owner. No additional definite output-slot/borrow/lock or
legacy-walk defect was found in the finite review. Eleven recorded tests remain
pure models and actual Windows guards are compile-only. The map-only prospective
SDK/I/O union is not a current combined build or product qualification.

The dormant Rust inspection transport now passes primary and three independent
source/lineage reviews (`64d7ab7a...21feb20`). Its fixed gate remains None before
cache preparation or native work, with no UI caller; the existing general native
client policy is preserved. The12 recorded tests cover codec/model/memory-IO and
the desktop check is compile-only, not actual Windows worker/pipe/job behavior.
An independent14-path observation confirms actual active adoption of inspection8,
preserved UI4/schema2 codec and the advanced lazy-Lima input (`f5d9ae7c...208cbc7`).
Its source gate is still disabled; this is selected source evidence, not a full
current build or installed-runtime check.
The SDK terminal-cache correction now passes primary and three independent
source reviews (`106dfdd6...77f3579`). All four reported gaps are resolved in
that successor, along with the connected outer-owner path; abnormal cleanup now
returns stable failure and only full normal completion publishes success. The15
recorded tests remain pure state/finalizer tests. Actual Windows callbacks and
retirement remain unqualified, and the older SDK source is not retrospectively
accepted. Root integration of this successor and I/O cancellation is separate.

Retained-selector factory3 also passes bounded source review (`134c4739...06e1316`),
preserving the old capture body and independent file ownership. Its initial author
invocation ran two selected package tests despite the compile-only assignment;
package initialization was not instrumented, so it supplies no native-absence or
provider qualification. The forthcoming source-projected constructor fixture is
still unbuilt/unrun. Review identified a writable-close timestamp transition and
a pre-create cancellation gap. Corrected Source3 now passes primary and three
independent source reviews (`8f021dd6...aaccdae6`), preserving strict native
checks. Its build recipe remains unsealed and needs the new sealed input/module/
artifact-preparation proof and generated-testmain correction before Root builds.
The fixture qualifies projected constructor internals only; it cannot establish
the default production namespace or current installed-product acceptance.

SDK producer6 now passes bounded source review (`92b2ab9b...bf4f2023`); original
acquisition cells and failure retention are corrected, while underlying read/hash
and actual SDK behavior still need qualification. The actual-leaf PREPARE
controller review (`46fac13b...13b0d50`) leaves one P2 guard issue in its future
Python finalizer: assert-only seals disappear under optimized Python. Root has
the required new-tool correction; this does not establish an executed native
failure. The reviewed PS source epoch remains unsealed and grants no dispatch.

An earlier external source-only gate is complete: the sealed2 strict-D
PowerShell parent/bootstrap passed primary and two independent reviews with
unchanged input hashes (`phase2-review-sealed2-parent-source-review-20261106-1`,
receipt `580e0fe5...716a2b0`). The accepted boundary is the documented trusted
WinPS5/.NET typed contract; it supplies no raw close, storage or descendant-family
acceptance. The implementation owner still owns the full immutable packet and
actual dispatch. The separate modern different-build contract is frozen design
only (`49780242...3acdd4`), with implementation and qualification outstanding.
A record-only criterion/dependency map is retained at
`artifacts/qualification/phase2-review-remaining-acceptance-map-20261106-1/`.
Detailed earlier findings below remain evidence for their recorded epochs;
later source adoption must be checked in the coordination records before
reopening a source defect.

The latest isolated WSL-refresh routing successor passes13 pure JS tests and a
complete Rust-caller compile, with all561 frozen source files checked afterward.
The implementation owner has reviewed it and included it in an isolated combined
desktop candidate whose Rust release and frontend typechecks pass; active
adoption and actual refresh qualification remain open. The standalone directory
CREATE diagnostic has now actually passed the strict F-drive artifact case;
the strict LocalAppData counterpart reproduced a known creation refusal. Its
single-flag B counterpart created the directory, then failed the strict relative
OPEN check; the implementation owner classified it as created-unverified. A new
C-only source candidate adds bounded self-package and already-held-object DATA
observations and restricts its relative OPEN exception to its own created slot.
Its source, pure tests, build and static checks pass. The owner ran C once: its
internal checks completed, but the child physically normalized below Codex's
LocalCache while the held parent normalized to ordinary LocalAppData. Both
self-package APIs returned no-package. This demonstrates a routing mismatch in
that C process's view, without proving its cause or describing earlier actors.
Flushing the selected LocalAppData parent does not prove flushing the child's
physical LocalCache parent. Production roles remain unqualified, and strict
storage policy is retained. A separate strict-D source successor restores every
CREATE/relative OPEN safeguard and adds negative literal-name/stability checks;
18 pure tests and 72 subcases, vet and static closure checks pass. Its saved
strict-D captures now report a CREATE refusal with attributes `0x1040`, status
`0xc000050b`, zero output handle and unchanged IOSB sentinels. The separately
owned launcher record reports successful ENABLE policy setup, Node exit 0 and
diagnostic exit 1; this does not identify the refusal's cause. A six-capture/D9
source consistency peer passes (`f27da24d...7330a2a`), with no target inspection
or rerun. The implementation owner's formal outcome (`760c9c12...7a4720d`)
records known-negative, natural outer completion and all 60 inputs unchanged
afterward. Production storage/reopen qualification remains open.
The bounded official-source follow-up found no stronger documented reset for
this already affected context (`bd0a1405...8a86e20`); it neither proves all routes
absent nor justifies weaker flags or a relocated production state root.
The owner's subsequent E record (`70e058ea...074d83`) reports a single successful
strict diagnostic through its reviewed Explorer-origin route: literal parent/
child names stayed directly related, and the existing edge/flush checks passed.
The broker has no original outer PowerShell handle/terminal receipt. This is a
bounded diagnostic success, with production constructor, later-process reopening,
whole-family and product qualification still open. A newly identified generic
pending-I/O frame-lifetime defect is being corrected before constructor tests;
the earlier fresh-directory-specific fix did not cover every storage helper.
An actual development
StrictMode dialog focus regression was found, corrected in an isolated one-file
candidate, and verified in keyboard/browser fixtures plus a full frontend typecheck.
These are remaining integration/qualification gates, not a completed Phase2 claim.
Details and receipts appear below.

The three reviewed desktop source components now have a verified 572-file,
21-path convergence candidate: full Rust-caller compilation and frontend
typechecking pass (`c6701bf7...10ef32`), with inherited warnings preserved.
A separate four-file volume-editor fix retains drafts across inventory failure
and refuses new disconnected saves. Its original failure was reproduced, and
version-scoped IAB mock checks covered retained drafts, pending/late replies,
explicit reconnect, confirmations and focus. The final v2 source and its
25-component frozen desktop composition typecheck; receipts are `9360ffd7...393adc`,
`40efe0a5...11f2d0` and browser evidence `cc4ac1eb...d30368`. A later read-only
observation confirms that all 25 selected active source files match the reviewed
desktop21/UI4 bytes and preserve the newer lazy-Lima build input (`4912d867...fd926a`).
This confirms selected source adoption. The isolated compiler/mock-UI results do
not qualify real volume writes, the current Schema2 integration, a new coherent
package or the installed application.

The working tree contains substantial feature implementation. A first coherent
backend/SDK/tools/desktop release build now passes, while later reviewed source
corrections are ahead of that build. Source, older development proofs, the built
bundle and the installed product remain different evidence levels. Installed
runtime was unchanged at its latest recorded checkpoint; this review has not
queried its current health. Neither old feature proofs nor recent component
tests establish that the current combined product works.

## Current evidence boundary

The coordinating review session reports **266 passing JavaScript tests** and a
passing **frontend production build** on 2026-11-06. These establish those checks
for the reviewed working tree; they do not qualify the Rust application, current
backend bundle, installation, privileged actions or workload preservation.
The implementation owner later reports **292 passing JavaScript tests**, including
the seven new current-package WSL wrapper cases. This review independently ran
the preceding 285-test epoch; it has not independently rerun the 292-test epoch.

The implementation/checkpoint records cited below are historical, primarily
2026-10-24 through 2026-10-26. Their test counts and binary identities apply only
to their recorded versions. This audit inspected current source presence and
the documented evidence; it did not rerun the native proofs or attest the
installed executable. Historical UI pauses are not assumed to be current
permission constraints. Runtime, trust, firewall and installation actions need
coordination around the exact intended operation and owned resources.

## Corrections made during this review

- The volume-clone coordinator rejected the real portable export/restore ACKs
  because its strict schemas omitted manifest and source fields. Both phases now
  accept the complete contract and require matching engine, source volume,
  payload, manifest and restored dataset. A real export producer using a local
  archive endpoint, wrong-source cases, the focused clone race tests and
  `go vet .` pass. No engine was contacted.
- Package preflight required an original absolute Go overlay after SDK files
  were copied into the assembly, so it refused valid relocated artifacts.
  Original build validation still checks that overlay. Assembly validation now
  independently checks signed assets, source/recipe hashes, generated payload
  identity and manager embedding without build-only paths. Relocation and tamper
  regressions pass; the full JavaScript suite passes **273 tests** after this
  change and the concurrent credential-output correction. A subsequent check
  copied the nine relevant files from the real freshly signed SDK bundle to a
  new directory without the overlay. Original strict verification, relocated
  packaged verification and original re-verification returned identical pins.
  No binary ran; receipt:
  `artifacts/qualification/phase2-review-relocated-sdk-20261106-1/review-result.json`.
- LAN administrative sessions discarded child-release errors after successful
  protocol replies. Those errors now reach mutation and shutdown callers;
  uncertain native owners remain retained and are not retried. Runtime terminal
  cleanup now joins the shared administrative helper before releasing its
  ledger. Cleanup-result reporting is separate from the positive release
  barrier, so failed cleanup cannot release an ordinary caller's SDK root.
  Focused fake lifecycle/protocol tests, Windows race selectors and package vet
  pass. This does not enable LAN or qualify actual native wait/provider faults.

These source corrections do not close current native or installed gates. They
must be included before freezing the SDK source/catalog and build-recipe hashes.

During coordination, canonical patch 0018 and the complete source lock were
published and replayed into a fresh checkout. The signed SDK, guest-capable
backend, credential helper, six client tools, CLI and a fresh assembly were built.
The full Windows desktop release compile passed, and the implementation session
verified its layout against the assembled inputs. No installer or application
was launched. The review session independently reran
the expanded JavaScript suite: **285 passed, zero failed**. These are build and
source milestones; installation, startup/upgrade completion, LAN activation and
combined native workflow acceptance remain open.

The same coherent application's Rust test executable subsequently ran its
default suite serially: **191 passed, zero failed, 33 ignored**, with the one
native-profile cache fixture deliberately filtered. The run took 7.079 seconds;
stderr was empty and executable bytes remained unchanged. Ignored product
qualification and application main did not run. Receipt:
`artifacts/qualification/phase2-review-rust-defaults-20261106-1/result.json`.

A production LAN-registration candidate is being developed separately from that
completed build epoch. Cross-review rejected its first version before adoption:
fixed-role callers could drop partially constructed authority, and copied owners
or a panicking close body could escape the intended once-only close result. The
successor must preserve the actual partial graph and reject copied native owners.
The second version now passes independent caller-chain review, 24 top-level
tests and five subcases, focused race, vet and complete session compilation.
It adds actual partial-owner retention, exact-self checks and cached-unknown
cleanup results including constructor cleanup panics. V1 and active sources
remain unchanged. Receipt:
`artifacts/qualification/phase2-review-lan-registration-candidate-20261106-2/validation.json`.
Passing candidate tests is not adoption or native activation.

The review also delivered a frozen eight-file authenticated management transport
candidate. Its pure/source selectors pass 44 top-level tests and 163 subcases,
race, vet and compile-only checks. It uses the actual held credentials/listener
owners, normal TLS verification and repeated native peer correlation, retaining
unknown cleanup in the original owner graph. No positive native constructor,
socket or TLS connection ran. The result is authenticated observation data;
HOME, supported old build, graceful Stop and complete retirement remain separate
gates. Receipt:
`artifacts/qualification/phase2-review-management-transport-candidate-20261106-1/validation.json`.

Two additional source designs are coordinated with the implementation owner:
fresh ordinary-old-start denial after the durable fresh intent, and legacy
actual-port selection from the same held credential publication. The latter
corrects a pinned-source producer mismatch: Create saves requested secure-port
arguments, but Complete may bind a different port and publish that actual
endpoint. The actual-port successor is now implemented and frozen in
`artifacts/qualification/phase2-review-legacy-actual-port-candidate-20261106-1`:
nine modified files, 49 top-level tests/203 subcases, race, vet, gofmt and
compile-only checks pass. Real HostPort/generator/kubeconfig/serializer tests
use an inert address and do not execute native fallback. Two independently
applied patches reproduce all 935 final source hashes. Root adoption remains
separate. Fresh denial is now also frozen in
`artifacts/qualification/phase2-review-fresh-legacy-denial-candidate-20261106-1`:
four new/three modified files, six exact inert tests/race, two-package vet and
two test compilations pass, with three source reviews. Its patch reproduces
all 953 final source hashes from the exact949 parent. Author results are observed
normalized transcripts, explicitly without saved raw logs; source/PE hashes were
independently rechecked. Publication/ordinary handoff binding and adoption remain
the integration owner's next work.
Neither lane has been exercised natively by this review. Exact file ownership
avoids overlapping the active fresh service, publication and recovery work.

A further current-source mismatch affects ordinary setup and Kubernetes access:
the canonical backend now selects its native private management directory, while
desktop setup, management requests and pod terminals still construct the old
`home/AppData/Local/rancher-desktop-orbwin` paths. The existing backend
`service paths -o json` command can provide its selected paths without starting
the service or guest. Normal Cobra construction does briefly probe an ephemeral
loopback port, so this command is not a zero-native-effects primitive. A narrow
desktop adapter and affected-callsite correction are now frozen and independently
source-reviewed in `artifacts/qualification/phase2-review-management-paths-source-20261106-1`.
Six actual-module parser/environment/argv tests and offline mini-harness Clippy
pass. A separate complete Tauri source successor, including the four setup
replacements, also passes `cargo check --release --tests --locked --offline`:
`artifacts/qualification/phase2-review-management-paths-integration-20261106-1/FROZEN-RESULT.json`.
The adapter and ordinary operation/pod-terminal call sites are now adopted after
exact baseline hash checks. The installation owner subsequently adopted the
four setup replacements too; all five active files match the checked candidate.
The SDK and epoch1 build remain unchanged. These checks execute no tests,
backend query or product process and do not qualify dispatch.

Authenticated-upgrade transport design exposed another deterministic contract
mismatch: pinned client-go's default certificate generator emits leaf+CA PEM,
but the new legacy credential parser and global private-authority material
validator accept only one certificate. Fresh creation and upgrade import thus
reject that real default generator path before serving. No host credentials
were read to establish this. The implementation owner is correcting the global
material contract separately. The isolated legacy-reader successor in
`artifacts/qualification/phase2-review-upgrade-credential-chain-candidate-20261106-1`
passes the real generator → actual CreateKubeConfig → serializer → parser
regression (YAML/JSON), 19 top-level/50 subcases, vet and compilation. It retains
anchor and peer-leaf identity separately. The implementation owner subsequently
adopted both the global two-file correction and this three-file reader correction
into the 932-file integration source; neither is in the historical canonical18
application build. The management-transport design is approved for isolated
source implementation and supplies no HOME/Stop/retirement authority. Native
TLS, startup and migration acceptance remain open.

The approved eight-file management-transport source candidate is now frozen at
`artifacts/qualification/phase2-review-management-transport-candidate-20261106-1`.
Its 44 top-level tests/163 subcases, race, vet and compilation pass; two
independent source reviews pass. It combines held native owners with fixed
authenticated reads and exact connection/peer checks, but no positive native
constructor or actual connection has run. Its result is captured DATA, not
HOME, Stop or retirement authority. Adoption and native qualification remain
with the implementation owner.

The bounded LocalAppData A/B diagnostic completed once after source review and
offline vet/build. Original and empty-name-reopened directory handles matched
identity, raw descriptor and access. Both fresh directory creates returned
`STATUS_REPARSE_POINT_ENCOUNTERED` without an output handle; neither directory
was created, all native closes succeeded and the process exited normally. This
sample does not support the proposed parent-file-object workaround and does not
identify the historical cause. It concerns the fixture's directory preparation;
the product publisher's existing-boundary/file operations remain unqualified.
Receipt: `artifacts/qualification/phase2-review-local-create-reopen-20261106-1/native-run/result.json`.

## Bounded frontend observation

The existing `tests/ui/wsl-trust/` fixture was rendered in the background browser
at a 1280-by-720 viewport on 2026-11-06. Its callback records requests locally;
it imports no backend API and changes no system trust. Selecting Ubuntu exposes
the reviewed registration and operation controls. The confirmation fits the
viewport; Shift+Tab wraps to Cancel, and Escape closes it, restores the trigger
focus and leaves "No action submitted." An unmanaged pre-existing root disables
install/removal; an unconfirmed outcome disables another trust change and asks
for a status read. These observations cover that fixture only, not native WSL
or certificate-store acceptance. The temporary Vite process was stopped afterward.

## Core outcome matrix

| Outcome | Current source / historical evidence | Next acceptance gate | Independent parallel work |
| --- | --- | --- | --- |
| P2-CMP: reusable Compose projects | Ordered files, profiles, reviewed retained inputs, service actions and filtered logs are implemented. The recorded adapter journey covers partial startup, conflicts, cancellation/retry and volume retention. [Record](../phase-2-implementation.md#parallel-integration--2026-10-25), [contract](../architecture/compose-projects.md). | Current native/installed project reuse, external recreation and engine restart against the assembled bundle. | Prepare a fixed project matrix and expected resource/volume preservation checks. |
| P2-NAM / P2-TLS: names and HTTPS | Route/signing/trust components and disposable relay proofs exist; permanent setup and client coverage remain incomplete. [Local-services contract, line 1](../architecture/local-services.md), [private state](../architecture/local-https-private-state.md). | Wire the permanent owner; qualify Windows, selected WSL and supported container clients, trust consent/removal, conflicts, restart and route retirement. | Review client cases and routing UI independently; coordinate shared SDK ownership. |
| P2-FIL: files and native tools | Image/volume browsing, binary copy and guarded volume text editing exist. Historical SDK/native proofs cover bounded files, stale/writer refusal and save/restore. [Record, line 381](../phase-2-implementation.md), [files contract](../architecture/container-files.md). | Current editor/Explorer handoff, interrupted transfers and saves, helper cleanup and installed interaction. | Audit file UI states and prepare bounded interruption cases without touching engines. |
| P2-DBG: minimal-container debugging | A separate pinned diagnostic container, target identity checks and shared terminal transport exist; historical scratch/read-only tests cover TTY/signals and cleanup. [Record, line 372](../phase-2-implementation.md), [contract](../architecture/debug-shell.md). | Current resource-cap regression, target recreation/client loss and native terminal acceptance. | Review resource/identity assertions and terminal interaction cases. |
| P2-WSL / D04: project and tool integration | Original-project adapters, root/registration identity retention, Explorer/editor actions and explicit private-client refresh exist. Historical Windows/Ubuntu and refresh proofs are recorded. [WSL contract](../architecture/wsl-integration.md), [record, line 340](../phase-2-implementation.md). | Current installed client pins, non-root project reopen/replacement/exit behavior and actual VS Code WSL handoff. | Check client-pin drift and prepare one consistent project/editor scenario. |
| P2-STO: portable persistent data | Verified volume export/isolated restore, pending recovery and clone compose the portable-data transaction. Historical fidelity, writer/collision refusal and cancellation cases passed. [Record, line 401](../phase-2-implementation.md), [data contract](../architecture/development-data.md). | Current installed export/restore/clone, free-space/interruption cases and project-level composition. Whole-engine lifecycle work keeps its existing Phase 4 disposition. | Audit destination ownership and preservation assertions; prepare data fixtures. |
| P2-MIG: supported source transition | Image/volume histories, source inventory, Windows/WSL project review, mapping and draft reopen are implemented. Historical Validate and draft/UI proofs are narrower than a complete application transfer. [Migration contract](../architecture/migration.md), [draft qualification, line 284](../architecture/migration-draft-qualification.md). | A supported-version source-to-target project/data journey through actual Up/use/reopen, with collisions, cancellation and unchanged source/global defaults. | Build the exact source/target acceptance inventory; do not duplicate the active import/runtime owners. |
| P2-QUAL: coherent runnable product | The first current coherent backend/SDK/tools/CLI/assembly and complete desktop release build passed; installed correspondence remains unqualified. [Build receipt](../../artifacts/qualification/p2-coherent-app-build-20261106-1/result.json), [build contract](../architecture/coherent-build-inputs.md). | Adopt the reviewed LAN successor in a fresh build epoch; complete installation/upgrade/recovery, three-engine and Phase 1 regressions, visual/keyboard review, resource budgets and security assessment. | Maintain this matrix and review build inputs while the runtime owner closes its production path. |

## Additional outcomes and decisions

| Outcome | Current source / historical evidence | Remaining gate / useful parallel work |
| --- | --- | --- |
| P2-MON: activity | Bounded adjustable container-counter sampling and native development UI proofs exist. [Contract, line 6](../architecture/activity-monitor.md). | Qualify sampling costs, stopped/recreated UI and in-flight cancellation; explicitly resolve the engine-counter versus VM-telemetry boundary. Independent measurement planning is useful. |
| Capacity / volume size / image archives / clone | Host-capacity and opt-in volume measurement, reviewed image import/export and clone exist with historical native proofs. [Capacity](../architecture/host-capacity.md), [volume size](../architecture/volume-size.md), [archive record, line 448](../phase-2-implementation.md). | Current installed interaction, space/interruption and tag-conflict recovery. Preserve the existing format and consistency limits; prepare one shared portable-data matrix. |
| P2-UI-SRT / P2-UI-NET / D10 | Sorting, paused/platform labels, bridge/subnet/internal choices, restart policies and per-container limits are implemented; historical adapter tests include enforcement. [Contract](../architecture/inventory-run-network.md). | Current keyboard/visual states and enforcement regression on the assembled bundle. Native amd64 and unsupported foreign-platform labels must follow BL-019. UI review can proceed independently. |
| P2-DEV / D09: Dev Containers and credentials | Historical real default-plugin/private-Compose runs cover authenticated build, workspace writes, same-container reopen and post-Forget refusal. [Record, line 1619](../phase-2-implementation.md). | Repeat the supported Windows/WSL/editor journeys using the new private-config owners and packaged pins. Historical successful credentials tests cannot qualify the new admission/import protocol. |
| P2-LAN | Read-only discovery, scope-review UI and launcher/helper/worker source exist. Activation remains unavailable pending protected native helper registration. [Owner contract, line 3](../architecture/lan-ui-owner-contract.md), [gates, line 84](../architecture/lan-helper-integration-next.md). | Native provider/IPC/listener proof, actual consent, product activation, traffic, removal/restart/coexistence. This is an implementation-and-evidence gap, not an exclusion. A separate owner can close it after SDK file coordination. |
| D01–D11 dispositions and desktop/CLI | Explicit CLI/completion, tray/sign-in source and named exclusions/adaptations are recorded. Proxy/guest recovery candidates and new upgrade authorities still have production/native gates. [CLI](../architecture/cli.md), [desktop](../architecture/desktop-convenience.md), [proxy, line 244](../architecture/engine-proxy.md), [recovery, line 129](../architecture/guest-network-recovery.md). | Reconcile every decision with current behavior. Keep Kubernetes's explicit application-lifetime localhost-forward adaptation and existing storage/architecture decisions; qualify their retained supported paths. |
| D06 / BL-017 investigation | Scope explicitly permits truthful disruptive manual recovery. Settings already offers a confirmed stop/start and workload warning; retained tap-loss and historical data/PVC recovery evidence exist. The automatic generation-repair successor remains unadopted. | Recommended disposition: close the Phase 2 investigation with the explicit manual-restart adaptation, retaining BL-017 and its failed evidence. Keep automatic repair with Phase 3 reliability work. This neither proves automatic repair impossible nor qualifies the new installed package's ordinary restart. |
| D07b / older application and installer | Both package versions remain 0.1.0; installer SemVer and file-in-use probes do not prove old-code refusal of newer state. The legacy cold-capture source has a durable original-name PID-directory denial before checkpoint, while retaining the original root and copying a backup. It prevents ordinary old serve from reaching controllers, but is not a whole-actor retirement proof. | Wire and qualify that migrated-state guard. Fresh private initialization currently lacks its equivalent ordinary-old-start fence; the fresh owner must account for it. Qualify both migrated and fresh state with exact old installer/app identities before guest effects. Extra new identity fields alone cannot protect an old reader that ignores them. |

## Five dependencies that control delivery

The next native acceptance run can reuse existing exact ignored Rust tests after
real product setup/upgrade publishes the final identity. Use a reviewed release
test executable with the final `runtime/{backend,tools}` beside it; running the
copy under `target/release/deps` alone lacks that packaged layout. Run individual
tests in separate processes: resource operations, Compose profiles/env/journey,
standalone files, portable volume/clone, then interactive/debug terminals, WSL
and optional-cluster cases. Run the disruptive management-cycle case last; its
existing guard refuses interruption while containers are running. This sequence
is not complete LAN/trust/upgrade/installed acceptance and has not been executed.

Qualification inputs need current-package binding. The old WSL client refresh
wrapper pins a superseded tool-lock hash; historical wrappers also select old
artifact/identity paths. Old local-service process fixtures directly run the SDK
from `artifacts/backend`, whereas current `sdkself.Hold` requires its actual
signed private-cache entry. Use reviewed successors or the real product entry;
preserve those historical proofs and self-admission checks. Conversely, the
fixed historical daemon ID is an in-place preservation guard, not a demonstrated
mismatch: compare the new publication before changing any identity assertion.
The `orbwin-proof` first-run fixture cannot qualify the product private authority
because it deliberately skips that product-only preflight.

1. **A coherent bundle unlocks meaningful whole-product verification.** The
   first coherent full application build now passes. Preserve that receipt as
   its own source epoch; the isolated LAN successor requires a matching rebuild
   after adoption. Neither build alone establishes startup or installed behavior.
2. **Private configuration and management-authority transitions govern setup and
   upgrade.** Existing legacy state is deliberately refused by
   [installation preflight, line 44](../../src-tauri/src/installation_preflight.rs).
   [BL-020, line 220](../backend-limitations.md) and the
   [authority transition, line 3](../architecture/legacy-authority-upgrade.md) require
   actual import/transition/recovery, with preserved source data. A refusal is
   useful failure handling, but does not complete the supported upgrade journey.
3. **Runtime publication and retirement must reach a production completion
   predicate.** [BL-021, line 195](../backend-limitations.md) records the process-wait
   gap; [BL-024, line 74](../backend-limitations.md) records unqualified storage
   admission. Close their specific causes and production wiring before the
   installed replacement gate; avoid counting stricter fixture-only behavior
   as a completed production path.
4. **LAN and permanent local-service/proxy wiring are real feature gates.** Their
   component tests do not supply product activation, client traffic or reversible
   trust/firewall outcomes. Assign these separately from the runtime critical
   path, with explicit SDK/module ownership and coordinated native operations.
5. **One current package must pass the combined journeys.** Reuse useful old
   fixtures, but bind results to current source/component identities. The final
   record must cover setup/use/conflict/error/cancel/restart/remove/recovery,
   coexistence, data preservation, interaction, resource and security gates.
   Keep historical failures and limits explicit; neither missing evidence nor a
   passing subset changes the accepted Phase 2 scope.

## Additional source milestone: per-import WSL2 dependency and portable recipe

The pinned Lima driver's uninitialized guest import now has a reviewed
three-file source adaptation that explicitly requests WSL2 without changing
global defaults or converting an existing registration. Its frozen source and
explicit-file-list tests are separate from native driver execution: the full
driver test closure initializes a native registry reader and was not executed
in this lane. The original module ZIP/h1 and all701 original files, the patch,
and the final703-file projection are bound independently.

The isolated production build-input successor is also frozen. It prepares a
current-output version-specific local replacement and modfile, routes only the
root-rdd module consumers, verifies inputs before/after, and emits a relative
six-file source offer that can be verified after relocation. Same-version
ordinary and modified module notices retain their canonical identity with
distinct notice directories. Real pinned filesystem inputs plus static Go
module selection passed;39 targeted Node checks and four syntax checks passed.
The complete ten-file patch reproduces all168 snapshot hashes in a fresh replay;
active source is unchanged. [Delivery receipt](../../artifacts/qualification/phase2-review-lima-build-route-candidate-20261106-1/delivery.json).

This closes the external source/recipe candidate and its adopted build-script
milestone, not Phase 2 acceptance. The implementation owner still must combine
it with the final backend source epoch, compile coherent binaries, verify actual
module metadata and collected notices,
package the source offer, and qualify new guest import and three-engine
coexistence. The running legacy HOME issuer design remains under owner review;
its retained-process/native-cache premises and stopped-engine transition cannot
be replaced by supplied identity fields or a positive fixture alone.

## Additional review: upgrade evidence and controller effects

The legacy HOME investigation now has separate source-backed ABI and producer
records. The selected extended x/sys fields permit local layout arithmetic,
but no exact Windows build/layout has been natively qualified; there is no
positive remote-memory profile to enable. Old serve's current-directory output
may help bind its cached service directory and explicit args/config/TLS paths.
It does not prove the independently cached Lima directory. The old host-switch
also scans HCS guest GUIDs for its product signature rather than receiving the
selected registration identity. That is a missing source relation, not an
observed wrong-guest event. [ABI proposal](../../artifacts/qualification/phase2-review-legacy-home-abi-proposal-20261106-1/PROPOSAL.md),
[producer review](../../artifacts/qualification/phase2-review-legacy-home-producer-review-20261106-1/PROPOSAL.md).

Only two reusable pure parser files have been implemented in the isolated HOME
successor: bounded UTF16 selector consumption/clearing and the actual old
hostagent argv shape. Three tests/23 subcases, race, vet and compile-only pass;
no native owner, remote reader or HOME/Stop/retirement grant is present.
[Subset delivery](../../artifacts/qualification/phase2-review-legacy-home-observation-candidate-20261106-1/delivery.json).
The current cold-tree/PID-denial implementation itself requires an existing
successful retirement owner, so it cannot be reused as its own initial
stopped-engine proof. A separate actual stopped-origin admission is still needed.

Draft reviews of missing-App continuation and managed hostagent retirement
identified further concrete effects to close before native dispatch. Empty
App/VM lists plus atomic App creation do not prevent an existing controller
from acting on a concurrently created same-name VM. Existing deletion and
template paths also need actual owner/UID checks and API mutation preconditions.
An isolated finite controller/finalizer candidate is in progress; active source
and the other implementation owners' files remain unchanged by this lane.
Retirement review separately found ignored stop errors followed by deletion,
unretained registry/VHD capabilities and unchecked event-handle settlement.
These are reported source/draft gaps, not a claim that the final successor has
already adopted or qualified their fixes.

Subsequent root checkpoint v8 adopted the exact ten Lima build-input files after
checking the168-file source candidate and three active parent hashes. The full
current Node suite passed330 tests. This is now an adopted source milestone;
canonical18/active945 and the previously built product remain their own older
epochs. The final0019 coherent product build/collector/native gates remain open.
[Root adoption receipt](../../artifacts/qualification/p2-root-lima-build-route-adoption-20261106-1/result.json).

Root clarified that App/base source repairs already belong to its import owner.
External implementation was stopped after one unfinished change in a separate
throwaway candidate; shared source was never changed and no App/base test or
native initializer was run. Its source is abandoned and is not a delivery.
Read-only ownership/CAS and initialization findings are forwarded to the actual
owner. The full App test closure queries the current Windows user's identity;
the full Base test closure imports envtest's cache-directory initializer.
Selectors alone cannot make those closures memory-only.
[Initialization review](../../artifacts/qualification/phase2-review-app-base-init-closure-20261106-build-gate-1/REVIEW.md).

The standalone BL-024 directory-create diagnostic now has a source-reviewed,
offline-built AMD64 executable and independent final static review. Its actual
3,137,536-byte PE is bound to the v3 nine-file source and the selected809-file
dependency closure. The actual Go build metadata reports Go1.27.3, CGO0 and pinned
x/sys0.49.0. This executable has not run.
[Build receipt](../../artifacts/qualification/phase2-review-native-create-diagnostic-candidate-20261106-1/reviewed-build/result.json),
[Root static review](../../artifacts/qualification/p2-root-native-create-final-build-review-20261106-1/review.json).

At Root's request, an additive artifact-a-only dispatch and clean-startup launcher
have been prepared for exact-byte review. Their source cannot grant native-resume
authorization. Parent startup explicitly clears the Node environment; the original
Node object is retained before launch, and an unknown launch/wait cannot authorize
a retry. The default UNSEALED template and diagnostic source/PE are unchanged.
The Go dependency closure does initialize Winsock before main, so this fixture
must not be described as making zero network API calls; no socket/listen/connect
or engine/WSL operation is added by that initialization path. No launcher, real
controller or native diagnostic case has been executed. Phase2 remains incomplete
pending actual native qualification and the coherent current product gates.
[Concrete v2 review manifest](../../artifacts/qualification/phase2-review-native-create-diagnostic-candidate-20261106-1/artifact-a-dispatch-review/review-manifest-v2.json).
The preserved v1 launcher is blocked after source review found that a diagnostic
writer exception could skip its unknown-owner keeper. The additive v2 places
keeper entry in finally. No launcher or native case ran before this correction;
v2 still requires exact-byte review and separate native resume.

Root subsequently passed the v2 source review and prepared the one-case raw
capture recipe; no native diagnostic has run. Independent current-package WSL
refresh review also found that JavaScript used environment-selected LocalAppData
and Profile while the Rust mutator used native product routing. An isolated
five-file successor now obtains bounded native folder DATA before private reads,
rechecks it in the actual fixture, and reuses the same admitted Target for both
refresh transactions. Product transaction code after Target selection is shared
unchanged. This closes the redundant Target-reselection gap without treating
path DATA as authority.
[Frozen source](../../artifacts/qualification/phase2-review-wsl-refresh-root-candidate-20261106-2/source-freeze.json).

Thirteen pure JS tests, syntax/static binding checks and the actual full Rust
caller compile pass. All561 source files,24 compile inputs and561 before files
were verified after compilation. No native query, ignored Rust fixture, actual
refresh wrapper, WSL or engine operation was executed. Root adoption, actual
current package qualification and supported profile/path behavior remain open;
raw-vs-canonical folder aliases can conservatively refuse. The older coherent
assembly used for typechecking is not a current complete-product claim.
[JS validation](../../artifacts/qualification/phase2-review-wsl-refresh-root-candidate-20261106-2/js-validation-v3/result.json),
[Caller compilation](../../artifacts/qualification/phase2-review-wsl-refresh-root-candidate-20261106-2/fullcaller-result.json),
[Independent source review](../../artifacts/qualification/phase2-review-wsl-refresh-root-candidate-20261106-2/SOURCE-PEER-FINAL.json).

The implementation owner audited the actual human authorization messages and
corrected an unnecessary persistent native pause: a prior restriction had already
been superseded by permission to use the computer and continue. Internal
source-review flags and interrupted tools do not create a new human approval
requirement. Native work remains coordinated through that owner, with concrete
source/data-safety gates and no independent overlapping provider runs.
[Authorization-source audit](../../artifacts/qualification/p2-root-native-authorization-source-audit-20261106-1/review.json).

The artifact-a diagnostic then ran through the pinned direct Windows PowerShell
invocation. The actual Go report records strict OBJ_DONT_REPARSE FILE_CREATE
success, verified directory identity/security, required flushes,427 journal calls
and53 positive checked closes. Earlier script-policy and nested-command failures
were preserved; they did not dispatch the Go case. No policy setting or elevation
was changed. This proves that exact artifact-parent construction on this host;
it does not qualify LocalAppData, the production producer or the whole engine.
The optional Node console snapshot did not reach the outer tool. An additive
report writer now persists the same controller DATA once with wx/write/flush/close,
with10 inert I/O failure scenarios and independent review. Its exact LocalAppData-A
packet is source-reviewed and ready for the owner's separate dispatch.
[Actual artifact outcome](../../artifacts/qualification/p2-root-native-create-artifact-a-dispatch-20261106-3/OUTCOME.json),
[Report writer review](../../artifacts/qualification/phase2-review-native-create-diagnostic-candidate-20261106-1/peer-report-v2-validation/receipt.json),
[LocalAppData packet](../../artifacts/qualification/phase2-review-native-create-diagnostic-candidate-20261106-1/localappdata-a-dispatch-review/review-manifest.json).

The dialog review has actual browser evidence beyond source inspection. Under
the same development StrictMode wrapper used by the frontend, keyboard opening
and Escape repeatedly lost opener focus. ConfirmDialog opened a native dialog
without closing that same element in its effect cleanup. The isolated correction
balances those effects before restoring focus. Repeated setup, ordinary/destructive
confirmation, disabled actions, mixed control focus loops, busy/error states and
light/dark inspected layouts pass the selected mock-callback fixture checks.
No extra decision callback was generated by cleanup. All102 frontend inputs pass
the full project TypeScript check with one overlay. This is a development-browser
regression proof, not a production/native-WebView or screen-reader qualification.
The one-file patch and complete evidence are ready for composite source adoption;
agent-created browser tabs and both owned Vite sessions are closed.
[UI delivery](../../artifacts/qualification/phase2-review-confirm-dialog-focus-candidate-20261106-1/delivery.json),
[Observed fixture result](../../artifacts/qualification/phase2-review-confirm-dialog-focus-candidate-20261106-1/UI-RESULT.json).

The exact LocalAppData-A case has now run. Original-child spawn/exit/close were
observed with no abort; Go exited1 after CREATE returned0xc000050b, no handle and
unchanged raw IOSB sentinels. Sixteen CloseHandle calls succeeded and two
CoTaskMemFree calls returned normally under their void contract. The new persisted
controller observation closes the earlier console-only evidence gap. No absence
of filesystem side effects or production root cause is inferred. The name and
all captures remain preserved, without reopening or cleanup. An additive B packet
uses the same PE and parent selection with a fresh ledger component; only the
target CREATE DONT_REPARSE flag differs. Its54 inputs and exact source deltas pass
independent review; execution remains with the implementation owner. This is a
controlled diagnostic comparison, not an automatic fallback or production-policy
adoption.
[Actual LocalAppData-A outcome](../../artifacts/qualification/p2-root-native-create-localappdata-a-20261106-1/OUTCOME.json),
[B packet review](../../artifacts/qualification/phase2-review-native-create-diagnostic-candidate-20261106-1/localappdata-b-dispatch-review/peer-scope-source-review.json).
