# Phase 2 implementation record

2026-10-24 Pacific. The full Phase 2 outcome in [scope](phase-2-scope.md) is
authorized. Milestones sequence work; they are not a stopping criterion. Phase 2
is active and incomplete. This file is implementation-owned; the originating
scope-evaluation chat owns the scope document and roadmap link. On 2026-10-25
the user authorized parallel agents, including the additional scope flows.
Use explicit non-overlapping file ownership and coordinated runtime qualification.
Preserve independent research and unrelated checkout changes.

## Desktop access

**Native diagnostics and product runs are paused by the user's latest instruction,
"Keep native runs paused" (2026-11-07).** Continue source work, independent review,
pure models and compilation. Do not start native Windows/API/filesystem diagnostic
fixtures, product/backend/engine tests or UI automation until the user restores
permission. No native qualification is currently active.

Historically, the user restored computer access on 2026-10-24, and reviewed
projected constructor fixtures subsequently ran. Their recorded evidence remains
limited to their original scope. The current pause supersedes those permissions;
no timer or successful source/build check ends it. Native and installed phase
acceptance remains incomplete.

## Current work

### Source continuation — 2026-11-07

The active backend source patch set is 0023. Independent replay checks 1,210
canonical files and 679 complete patched-file hashes. The selected Lima lifecycle
changes and separate Winio profiles for the backend (0.6.4) and SDK/LAN tools
(0.6.2) are now integrated with matching recipe and source-offer hashes. Their
versions remain distinct. The integrated source/compilation envelope covers 1,227 files. The publication
record retains 1,177 tracked inputs, excluding 50 ignored generated schema and
non-Windows icon files; 14 CRLF inputs were normalized to the Git-mandated LF.
The 92 source tests and release/test compilation/Clippy pass again after that
normalization. See [the source closeout](phase-2-closeout.md) for current work
ownership and the remaining release gates.

The combined source passes 92 Node tests, four isolated Rust provenance tests,
release/test compilation and Clippy (67 warnings). Actual compiler-only builds of
the backend and all three SDK/LAN roles also pass; each checks the same effective
module and source-file closure before and after compilation. The backend fixture
excludes the embedded guest distribution, and the SDK fixtures exclude cold-payload
and UAC resource packaging. These checks do not establish installed or native
qualification. No product, backend, SDK, LAN or guest executable ran.

The Go dependency metadata query now requests every field used by the validator.
This avoids the previous full-output parser limit while preserving all requested
fields across 3,044 packages. Tests also carry their original module data inside
the repository, removing qualification-packet directory assumptions. Failed
attempts and preimages are preserved in the qualification artifacts.

The active source includes the reviewed ordinary-backend process custody,
persistent setup security changes and paired Go/Rust SDK continuation.
Source review found a stop sequencing race: the server could exit before the
client's final peer check. A narrow acknowledgement on the authenticated
connection now passes source review, pure tests, race checks and compilation
for requests bound to an expected server; the actual original-child waiter
still determines process termination. Root adopted the exact SDK source slice
while preserving the current runtime, patch lock and security changes. The
combined 604-file source record passes ten pure ownership tests, fifteen source
and package contract tests, release/test compilation and Clippy (67 warnings).
Historical package resources were used only for compilation; no SDK process,
native selector or installed product ran.

Parallel source work has frozen an OPEN-only startup reader and a dormant
generation-1 pending-record producer. They have pure and compile checks, but no
active callsite or inherited authority continuation; the reader intentionally
refuses mutations while those are absent. Useful recovery remains unimplemented.
Selected Lima and SDK build profiles are integrated as described above. Guest
role authentication remains a dormant source proposal: reusable streams and the
known-exit cancellation fix pass pure and compiler checks; a narrow keeper fix
addresses Go's possible deadlock exit. Genuine private delivery, native authority,
product wiring and recovery remain required.
Native qualification and the expanded installed flows remain outstanding under
the user's pause. Current pins and review handoffs are in the coordination ledger
and `artifacts/qualification/p2-root-continuation-20261106/checkpoint-v30.json`.

### Coordinated continuation — 2026-11-06

Phase 2 remains incomplete. Active file ownership and the shortest remaining
delivery path are recorded in `artifacts/coordination/phase2-status.md`. An
independent review lane owns volume-clone acknowledgement interoperability and
relocatable cold-payload package verification. Root owns final canonical backend
patch consolidation and the coherent build. Source and compiler work needs no
administrator approval.

Import candidate 5 compiled all nineteen actual probe binaries (eight Go and
eleven Rust). Root checked the signed 12,449-file frozen closure, 12,934 original
compiler inputs and their snapshots, production origins and all nineteen PE
hashes/import tables. Native import has not run. The first matching-rebuild
recipe incorrectly assumed nine Go probes; its successor corrected the table.
That successor stopped before compiler execution because Node on Windows rejects
a drive-letter path passed directly to dynamic import. Original closure checks
still passed. A fresh file-URL successor then refused original-input verification
because a recorded Go cache file was missing; it started no compiler. Both
attempts are preserved. Matching full probe bytes remains a fixture-dispatch gate, not a
prerequisite for consolidating the production patch.

A fresh own-PE zero-limit probe reached the actual Windows job limit: setting and
querying ActiveProcessLimit zero succeeded. Assignment of one suspended own child
failed, and the captured child did not signal within its two-second wait although
the target job reported zero active processes. The run stopped and retained its
unknown state without external inspection, cleanup or retry. This does not prove
a permanent birth/assignment seal; whole-product retirement remains refused.

The isolated retained-retirement V2 source corrects a review-found ownership bug:
normal receiver admission transfers member custody to the map and removes its
partial slot. Map-only members must therefore be accepted; any optional aliases
must still name the exact validated member. Root and independent review passed;
the source is adopted and all 918 active files match. Focused retained tests,
the SDK management race suite and management vet pass. A broader three-package
wrapper did not complete successfully and remains recorded separately.
Existing ordinary lease cleanup and full engine/guest closure
are outside that source milestone's claims.

Fresh-output credential-license generation now creates the actual explicit output
tree and preserves selected backend files. Three filesystem regressions were
added; all 269 JavaScript tests passed at that source epoch. Frozen source V2 is
`p2-coherent-bundle-recipes-source-v2-20261106`. The fresh credential helper now
builds in `artifacts/coherent-credentials-20261106-1`, with testsNotRun explicitly
recorded and no helper execution. The independent lane also corrected clone
acknowledgement interoperability and relocated cold-payload package verification;
its 273 JavaScript tests passed at that epoch, and package changes passed an
independent source review.

Canonical patch0018 is now registered after root and independent full-source/tree
review: 174 additions, seventeen modifications, 914 canonical rdd files and all
382 final patched-file hashes. The isolated eighteen-patch replay matched the
exact final tree. Root source-publication receipt:
`p2-root-canonical-0018-adoption-20261106/adoption.json`. A fresh real working
checkout passed the normal eighteen-patch replay and all 914 source checks.
The selected checkout and installed runtime are unchanged.

The same-epoch SDK, guest-capable backend, forwarding shim, credentials, six
client tools and CLI now build successfully. Every client-tool binary matches
its prior locked digest; only source/recipe provenance selectors changed. The
fresh assembly successor copies the eleven forwarder license files beside their
manifest and records their digests; the first assembly and its failed application
preflight remain preserved. All 285 JavaScript tests pass. The frontend and full
Windows release application compile passed; actual release-layout verification
also passed. Result:
`artifacts/qualification/p2-coherent-app-build-20261106-1/result.json`.
The executable is under `artifacts/coherent-app-20261106-1/target/release/`.
It was not run; no installer, engine, service, WSL or elevation action occurred.
This is a complete-build milestone, not phase acceptance. Permanent production
setup/upgrade and LAN wiring, whole-retirement support, current native/workload/
UI/coexistence/data-preservation qualification remain open. All 225 Rust tests
compile in release mode. The one existing metadata-only backend attestation test
also ran successfully: it accepts the complete current ordered patch set and
refuses omitted, reordered, duplicated or changed entries. The application itself
was not launched. Test result:
`artifacts/qualification/p2-coherent-app-tests-20261106-1/result.json`.

### Recovery and upgrade qualification — 2026-10-26

Phase 2 remains incomplete. Fresh private-client initialization passed 29 bounded
native phases across 14 fresh namespaces. Go successor and local EOF recovery
source is adopted; Rust parity, checked final snapshot release and publication
uncertainty fences pass focused source tests and independent review. Setup now
admits held private configuration before service changes and no longer creates
the obsolete public client master. These source changes still need current
installed acceptance.

The cross-language successor matrix's first native candidate stopped after six
phases: the Go consumer refused the fixed inert Docker cache before recovery.
Source inspection identifies its unchanged selected-Docker argument-schema pin
as incompatible with the fixture's inert-image pin. The production guard remains
intact. The next fresh fixture declared the data-pin projection explicitly and
passed all sixteen actual pure cache-role checks before dispatch. Its reverse
case then stopped before Go review because the Rust historical-format test hook
emitted ten fields with a schema-2 label; Go correctly requires twenty-four
fields for that version. After correcting that debug-only encoder and checking
its actual output in both parsers, fresh candidate 4 passed all 96 native phases
across twelve cases in 620.193 seconds. Run
`p2-successor-cross-native-v1-20261026/candidate-4/native-run-df64ba09-c8b5-43e5-bc8c-c5f4511026cd`
covers six scenarios in both Go/Rust directions, with two fixed inert writer
births. Selected Docker/Compose/Buildx/helper images were copied/read only.
The separate immediate-metadata matrix also passed all 32 phases across four
cases in 136.349 seconds, with two disjoint cases actually overlapping. Run
`p2-successor-publication-cuts-native-v1-20261026/candidate-1/native-run-5ad07805-4f2c-4c55-b838-4860895760f7`
covers hooks 5/6 before rename and after metadata publication, retaining the
actual stage through opposite-owner review and recovery. It launched no writer
children. Local EOF, snapshot uncertainty, legacy import and installed acceptance
remain separate gates. Failed prior candidates and their
namespaces/receipts remain preserved; none were retried or externally cleaned.

The SDK physical-resource V7 fixture passed with a stronger test-only wait before
releasing exclusions. It observed job emptiness before a captured process handle
signaled completion; the unpublished production holder lacks that additional
predicate. [BL-021](backend-limitations.md#bl-021--sdk-retirement-needs-positive-member-completion-before-exclusion-release)
records the exact observation and remaining ownership work. This is no upgrade
grant or evidence of data corruption. Earlier failed fixtures remain failed.

The SDK self-lease directory helper now passes its separate native sharing proof:
four exact relative-directory roles have traverse access without directory-list
access and refuse DELETE-right opens; the metadata-only control admits that open.
All seven fixture handle closes succeeded. Run
`p2-sdkself-directory-sharing-native-v1-20261026/native-run-b66c04e0-b1f2-404a-a636-daf6a7091242`
created only six fresh empty directories, retained without external cleanup.
This proves the directory-sharing primitive, not the full SDK lease, peer,
volume-root traversal or upgrade retirement.

The legacy public-to-private client transform V3 passed root source review and
signature/provenance checks. All 382 synthetic Go/Rust cases match; all 120
admitted targets pass the actual pinned Docker configuration reader. Its typed
field gate refuses 84 previously admitted wrong-type examples. This qualifies
that reader contract, not complete Docker command behavior. The fixed-source
native import owner, recovery, CLI admission codec and explicit review UI are now
adopted in source after full root and independent review. Native and installed
acceptance remain open under
[BL-020](backend-limitations.md#bl-020--legacy-orbwin-client-settings-need-explicit-private-import).
No legacy user settings were read or changed by these source tests.
Synthetic pure comparison also found different ordinary parser nesting/node and
invalid-key encoding acceptance; [BL-022](backend-limitations.md#bl-022--desktop-and-cli-private-config-parsers-accept-different-input-bounds)
records that compatibility gate. Both active ordinary entry paths now share the
reviewed size/depth/count/Unicode contract. Durable boundary and raw-preservation
tests pass: 176 Go pass events including subtests, Go race/vet/Windows test
compilation, and 39 Rust tests. An isolated compile-only harness also typechecks
the eight actual modules' release tests. The full application release check still
requires the unfinished real backend input bundle; no placeholder was staged.
Ordinary credential/context/extension policy and metadata codecs are unchanged.
Native and installed refusal/preservation acceptance remain open.

A separate bootstrap handoff primitive passed in 132 ms: a read-reduced duplicate
of the same native file object retained exclusive sharing across the writable
handle's checked close. An actual controller was held before independent
read-only reopen, which preserved file ID and bytes; all four checked closes
succeeded. Run
`p2-bootstrap-file-object-native-v1-20261026/native-run-48a71a25-9c02-4d6a-994c-65f98844054b`
used two synthetic temporary files. This clears that narrow design question;
actual import/source security, journal recovery and installed acceptance remain.

C7B launched 105 of its planned 106 phases: 104 passed across 21 complete cases.
The final Go ordinary-refresh fault case reached the injected lost return after
successful target rename, then hit its 40-second watchdog. Root and independent
source review identified a registry mutex held across refresh that its failure
fence tried to acquire again. [BL-023](backend-limitations.md#bl-023--go-private-config-acquisition-can-self-deadlock-while-fencing-a-failure)
records the observation and source fix. Strong retention is now reserved under a
short lock; native work runs outside that lock. The 181-event focused Go suite,
race/vet/Windows test compilation and reentrant/panic/budget tests pass. A fresh
two-case/eight-phase native regression passed in 34.273 seconds. Both directions
returned promptly after the injected lost return, retained the actual controller
and target handles, and recovered through the other owner. The failed namespace
remains untouched and its result remains failed. Installed and full current-source
matrix acceptance remain open.
Runtime publication now requires a durable, immutable storage-admission record
before rename can begin. It binds the captured predecessor or actual absence,
parent/controller, immutable stage and actual observed candidate. Committed
runtime bytes must match that admitted stage; mismatches refuse before checked
release. The focused Runtime/Upgrade tests, race checks and vet pass. Independent
review also corrected traversal permissions that would reject the standard C:
root and retained partial native open outputs before classifying errors. This is
source-level work. The concrete publisher now binds actual observed-candidate,
storage, native journal custody and mapped incoming-self build provenance. Its
pre-interruption cancellation revokes the same recorder before releasing handles,
so a later interruption cannot race cancellation. Runtime/Upgrade pure, race and
vet checks pass. Single setup-owner entrypoint/callsite integration, process-cut
recovery and native/installed acceptance remain open.

The adopted import UI offers explicit empty setup, current historical-source
import and interrupted-import continuation through the shared native review and
Apply slot. The production frontend build and all 257 JavaScript tests pass;
active CLI pure/race/vet/test compilation pass. The active Rust-module harness
passes 49 configuration and 3 coordinator tests plus release test typechecking.
An omitted test-only cut module was added as an exact frozen-source supplement.
The complete app build still requires the coherent real backend bundle.
Native import, visual and installed acceptance remain open.

Runtime primitive candidate 2 passed source, signature and reproducible-build
review, then its first two producer probes refused ancestor security/identity
validation before fixture creation. Both processes closed; no later probe was
admitted. Post-run frozen-source verification passed. The failure is preserved
and tracked in BL-024; it does not establish native storage or publisher success.
Import fixture construction and SDK lifecycle work continue independently.

The subsequent routing correction retains actual complete native edges and
requires a strict protected destination when intermediate public permissions
are broader; it preserves old strict routes. Independent Go/Rust review passed.
Rust's two-file change is adopted: 56 active configuration tests, three
coordinator tests and release test compilation pass. Go Runtime/SDK pure,
race and vet checks pass. The SDK source now has actual master custody and
same-control birth/stopped/closed exchanges with checked child compaction;
whole retirement and OS late-birth admission remain withheld.

Fresh runtime candidate 3 passed its ancestor checks but both first producers
failed at the public test-directory creation attempt, after creating their
private test caches. Both processes closed and no constructor phase ran.
The failure and all namespaces are preserved; diagnostics/source investigation
continues without retry or cleanup. A subsequent one-shot diagnostic captured
`STATUS_REPARSE_POINT_ENCOUNTERED` (`0xc000050b`) with no returned handle. Its
actual ancestry checks, checked closes and post-run source audit passed; this is
diagnostic evidence, not storage acceptance or proof of candidate 3's cause.
No no-reparse guard or host ACL was changed.

The SDK initial-cohort handoff's seven-file overlay is now adopted after full
root and independent review. All 900 resulting source files match the reviewed
overlay. Focused Runtime/SDK pure tests, race and vet pass. Actual candidate-table
custody and whole SDK retirement still require native qualification; the whole
retirement gate remains refused. The actual joined-source participants are also
adopted in source: eleven SDK paths and eight backend paths after full root and
independent cross-review. The actual original dispatcher requires the joined
holder, sealed factory, empty mailbox and checked extra-self closure, using the
authority's separately retained self lease. The original backend framer owns
seal/drain/leaf ACKs, rechecks that participant before final retire bytes, and
refuses missing support before consuming shutdown. Failed file-close witnesses
retain the pre-close raw value for diagnostics without retrying it. All 500 SDK
and 902 backend files match their reviewed source snapshots; focused active
pure/race/vet checks pass. This remains source qualification: coherent updated
mapped images, OS birth admission and every-member signal before release are
separate mandatory gates, and whole retirement still refuses.

The import fixture's candidate 2 build stopped at a qualification-only adapter
type mismatch: IO_STATUS_BLOCK's status is pointer-width, while its diagnostic
argument was declared i32. Go preflights/compilation passed; no native import or
selected role ran. Candidate 2 and its failed proof are preserved. A fresh
successor will retain the full pointer-width diagnostic value. Compiler/library
discovery is recorded separately from a required matching-PE rebuild before any
native approval.

Import candidate 3's named pure checks and all eleven debug probe builds passed,
including the upper-bit diagnostic tests. Its input-evidence collector then
refused a compiler-generated NATVIS file already deleted after linking. The
failed proof and eleven final PEs are preserved; no native import ran. A bounded
compiler-only experiment confirmed documented save-temps retention, including
one genuine generated visualizer. A fresh source recipe can retain those inputs
in its own workspace without adding a linker wrapper or dropping evidence.

Import candidate 4 has passed root and independent compiler-retention source
review, source/projection preflight and scheduler checks. Its Go and eleven Rust
debug binaries built, but evidence collection refused a library path containing
Cargo's closing backtick. No native fixture ran. The saved compiler logs will
validate the parser correction before another build. The runner atomically consumes
its reviewed namespace before any native child, preventing a second dispatch.
The C3 failed source/log/PE evidence was independently rehashed before this build.

A separate backend storage-preflight command passed isolated pure/race/vet and
compilation checks, but review blocked its first candidate: its reused constructor
can release the actual setup lock on an acquisition failure before the outer
selector closes. A subsequent unknown selector close cannot retain an already
released lock. The corrected successor is now adopted in the backend source:
actual partial storage ownership is published before native capture, and error
settlement closes the selector before storage/controller. Terminal data is
accepted by the owner after a positive held-event write, avoiding a fast-client
response race. Root and independent review, plus active three-package runtime/SDK/
selection pure tests, race checks and vet pass. All 914 backend files match the
reviewed successor. The original candidate is preserved. The installed runtime
and Rust routing are unchanged; native setup/upgrade and durable recovery remain
unqualified. See
`artifacts/qualification/p2-root-operation-preflight-review-20261026/review.json`.

An independently rebuilt own-PE shutdown mechanism probe stopped at its
existing-job preflight before image capture, jobs or child attempts. Its saved
output does not distinguish a membership-query error from existing membership;
the zero-limit mechanism was not reached. The run is preserved without retry or
process inspection. A fresh source proposal will record that result explicitly
and keep any ambient job unchanged. Separately, a compiler-only experiment found
ordinary MSVC timestamps prevent matching Rust rebuilds; an explicit deterministic
metadata flag produced matching toy PEs. This does not replace the import
fixture's mandatory full matching rebuild.

Read-only follow-up storage diagnostics completed: actual known-directory
metadata and a same-directory protected reopen succeeded, but did not identify
the create refusal. Both package-identity APIs reported no package for a separate
fresh self probe. No guard, process policy or host ACL was changed; BL-024 stays
open.

Full evidence and remaining gates are indexed by
`artifacts/qualification/p2-root-writer-continuation-20261026/checkpoint-v31.json`
and the subsequent C7 receipts. No native UI, real engine, service, WSL,
trust/firewall, elevation or selected runtime replacement occurred here.

### Parallel integration — 2026-10-25

Non-overlapping work streams implemented inventory size sorting and platform
inspection; bridge network/subnet/internal options and restart policies;
per-container CPU/RAM controls with reviewed existing limits; exact per-name
local-service routing, verified HTTPS upstreams and forwarded-header handling;
and the thin Windows CLI, executable-only Docker alias and session completion.
See `architecture/inventory-run-network.md`,
`architecture/local-service-routing-controls.md` and `architecture/cli.md` for
contracts and remaining acceptance checks. These are implementation milestones,
not closure of the associated live/native/installed gates.

Real Dev Containers 0.469.0 Windows and registered-WSL folder journeys passed
open, directory/file write-through and reopen with the same container. Exact
cleanup restored baseline inventories, three daemon identities and client config
hashes. Receipts: `artifacts/cli/orbwin-devcontainer-b8bdc07b01ce4a59aedb2a06e176f696/proof.json`
and `artifacts/cli/orbwin-devcontainer-185fd6b1aa324bf8a040b46e5732bfcc/proof.json`.
The first failed Windows run exposed argument-boundary rejection of a shell
payload `-c`; the retained failure and subsequent parser/tests are documented
in the CLI record. A Compose build/up/write/reopen journey passed only after
confining the fixture process TEMP/TMP inside its explicit build context:
`artifacts/cli/orbwin-devcontainer-550ae6750831417db7ab94de6309f7c9/proof.json`.
The ordinary default path and COMPOSE_BAKE=false both failed with pinned
Compose 5.6.1's outside-context generated Dockerfile filesystem entitlement
refusal. The workaround does not close the normal Compose gate. Exact fixture
cleanup restored inventories/configuration; BuildKit cache grew from 20 to 22
records (49.7 to 53.57 MB), retained without global pruning. A narrow upstream
adaptation subsequently passed the ordinary temporary-path journey described
below. Private pulls remain a separate gate.

The adopted Compose `v5.6.1-orbwin.1` patch grants only the resolved existing
explicit local Dockerfile as a BuildKit read entitlement. It does not grant its
parent or a sibling. The actual generated-Dockerfile build/up/write/reopen proof
is `artifacts/cli/orbwin-devcontainer-fbfa2aa3f30e468284a78533a27c420f/proof.json`.
This fixture deliberately forces Dev Containers' standalone Compose fallback;
default Docker plugin discovery remains unqualified. Exact cleanup preserved
all three engines and client configuration. BuildKit cache increased from 28 to
31 records (53.61 to 53.64 MB), retained without pruning. Exact-file positive and
sibling-denial checks, repeat builds and legal/source provenance are recorded in
`architecture/cli.md` and the Compose patch record. Windows and Linux candidate
hashes are pinned in `runtime-tools.lock.json`. Root successfully staged both
local candidates with `--replace-prior-compose-cache`; replacement required the
exact previous official cache hash. Installed tools and existing WSL project
configuration have not been upgraded. The earlier installer below is stale for
this adoption and subsequent WSL changes.

Packaging now stages both CLI aliases, their manifest and PowerShell completion
beside the GUI, with source-inventory, lock, version and binary-hash checks.
Installer quiescence checks include running CLI aliases. No automatic PATH,
profile or VS Code setting mutation is added. Docker settings explain explicit
targeting, credential visibility and the application-wide scope of VS Code's
Docker Path setting. Installed layout and interaction remain unqualified.

The read-only `lan-interfaces` SDK command is registered and source-attested.
Native IP Helper observation returned 12 adapters; network profiles stay unknown
and activation is unavailable. The detailed proof includes local IPs/names and
is not a support-upload allowlist input. LAN profile discovery, privileged
firewall helper and attended exposure/removal are still required; see
`architecture/lan-exposure.md`. No firewall/trust/elevation change was made.
The subsequent independent read-only NLM profile component is registered as
`lan-profiles`, preserving the interface command's JSON contract. It joins exact
adapter GUIDs, refuses partial results after failed/incomplete COM scans and
keeps activation false. Actual native SDK output reports 12 profiles, two
observed categories and ten unknown/missing. This is an observation, not consent
or proof of LAN reachability; see `architecture/lan-profile-observation.md`.
The post-candidate SDK containing this command has SHA-256
`047fab5a7f13465715db0a69ccd45dae3823f767ec56dfd575994ee1c488c8a9`.

Integrated verification: 105 JavaScript tests, TypeScript build, all Go session
packages and 64 Rust tests pass (26 intentionally ignored live tests). The
checksum-verified package stage passes with 773 dependency notices resolved.
The installer candidate's SDK SHA-256 is `89a1ea2013e46c568f16d99ada119ef0362512fc87ca97dd28dfd966f500157d`;
CLI aliases are `9628cbfacbab88702a34aa66fc54c32a3fe799ec50ae65420ab4cee4cb7ddf21`.
The resulting installer candidate is retained at
`artifacts/qualification/p2-parallel-candidate-859f274c2181-20261025/OrbWin_0.1.0_x64-setup.exe`
(492,340,970 bytes; SHA-256
`859f274c218180f90f4e4969ab2faad66f2bc50f26332a76019871a701bcc773`).
It has not been installed or accepted. Native development UI review at 1280×850
confirmed the client guidance and accessible text; receipt
`artifacts/qualification/p2-client-guidance-native-20261025.json`. No client
settings/commands were entered, and the original installed UI stayed open.
Native development review also confirmed Run's new CPU/RAM/restart/network
fields, scroll-to-focus forward/backward wrapping and Escape focus return;
the create-network bridge/subnet/routing fields fit the window. No form was
submitted (`artifacts/qualification/p2-new-form-native-20261025.json`).
The subsequent serialized production Rust adapter qualification passed network
bridge/internal/subnet choices, duplicate-name and overlapping-subnet refusals,
four stored restart policies, actual cgroup CPU throttling, partial live CPU/RAM
updates and bounded fixture OOM termination. Every mutation used production
Request/run dispatch. Exact nonce/full-ID cleanup completed and the final
three-engine inventories, running/paused/restart states and all client/global
WSL config hashes matched. Receipt:
`artifacts/qualification/orbwin-inventory-05c5fd735bab4b8d87598ed1eb3930fb/proof.json`.
This proves stored policy values and explicit manual stopping, not policy
behavior across engine restart; native enforcement interactions and installed
acceptance remain required.

The selected WSL project now exposes explicit Explorer and VS Code actions,
bound to the last inspected distribution registration GUID, canonical source and
inode signature. Record locks, non-reparse nodes, denied write/delete sharing,
bounded registry/record reads and clean guest preflight protect the handoff.
Editing the source invalidates inspection. A native development Explorer action
opened the exact existing Ubuntu project, displayed the four expected entries
and survived closing the development OrbWin window. Only that new Explorer
window was then closed; the installed app remained open. Receipt:
`artifacts/qualification/p2-wsl-project-explorer-native-20261025.json`.
Five actual Windows filesystem sharing/lock/reparse tests pass without skips,
alongside the focused URI/path and changed-record tests. Three daemon identities
and private/project/global configuration hashes remained unchanged without
elevation (`p2-wsl-tools-retention-20261025.json`). The native VS Code WSL extension
journey has not been run; installed and remaining D04 acceptance are open.
See `architecture/wsl-integration.md` for the handoff and its post-dispatch
registration-race limitation.
The native process fixture and the retained PowerShell regressions pass; the
previous concurrent timeout cause and pre-job-assignment execution window remain
unresolved (`architecture/client-processes.md`).

Security response and explicit exclusions are recorded in `security-response.md`
and `phase-2-exclusions.md`. Authenticated read-only GitHub metadata verified the
repository is private; a public issue/private vulnerability reporting policy
remains unverified. No issue, message or upload was sent. A sample dependency
assessment does not establish candidate-wide advisory coverage.

- **P2-CMP:** ordered files/profiles/quiet validation, consistent-label discovery,
  explicit directory, reviewed/remembered inputs, exact service choices/shortcuts
  and aggregate/service-filtered logs implemented. Adapter service/failure proof
  `compose-journey-1792886516116846900.json` passes (41.84 s), including external
  recreation, port conflict, partial startup, readiness cancellation/retry and
  volume retention. Profiles/directory regression also passes. Final native/service
  review, engine restart and combined installed gate remain.
- **P2-DBG:** separate pinned Alpine/BusyBox tools share only exact target PID/net
  namespaces. No target packages, host mounts/socket or privileged mode. Static
  scratch/read-only HTTP fixture proves shell-less access, network, read-only
  target files, TTY resize/Ctrl+C, exit status, client-loss expiry and target-exit
  cleanup. Ordinary terminal regression passes after shared transport extraction.
  Failed records retain the source-hash refusal, AutoRemove removal race and
  failed stdin-only cleanup route. Current code adds 128 MiB/128-PID/one-CPU caps;
  final resource-cap regression/native review remain.
- **P2-FIL:** image/local-volume browsing, regular binary copies up to 4 GiB,
  guarded UTF-8 volume editing with retained originals, and verified container
  mount mapping are implemented. `standalone-files-1792890518101876800.json`
  passes (62.31 s), including Unicode Windows copy paths, writer/stale refusals,
  ownership/mode preservation and stopped-container mapping. Native explicit
  local-copy selection for Explorer/Notepad is implemented; native review,
  broader interrupted-I/O qualification and selected-WSL paths remain.
  Native fixture save and Restore previous text passed, including exact leading
  spaces/newline and retained original, in `native-files-2026-10-24.json`; the
  fixture was ownership-checked and removed through the explicit owned CLI.
  OS text picker open/cancel passed, but Notepad launch was not qualified because
  the automation route could not confirm the picker input focus. New direct
  completed-copy Explorer/Notepad actions are implemented but await native access.
- **Transient ownership gate:** flushed intents and exclusive Windows leases
  reconcile exact inactive helper IDs/names. Tests include active leases,
  conflicting ownership, pre-start abandonment and late accepted create. Missing
  uncertain intents remain. Settings recovery and next-helper reconciliation
  never remove user volumes. See `architecture/debug-shell.md` and live
  `transient-recovery-000000000000000018e19e24af5379d0.json`.
- **P2-STO:** owned-engine portable-volume export/isolated restore and explicit
  pending recovery implemented. `portable-volume-1792891298775272700.json` passes
  (70.61 s): byte/metadata/link fidelity, corrupt inputs, names/destination
  collisions, active/paused writers, import cancellation, recorded exact pending
  discard and refusal for completed/read-only-attached/stopped-attached targets.
  Earlier root-timestamp mismatch was detected and fixed through the upstream
  extraction path; failed evidence remains. A read-only guest attribute audit
  now prevents Moby's user-xattr/ACL omissions from silently losing metadata.
  `portable-volume-1792891532714324300.json` passed that live boundary plus the
  combined cancellation/recovery cases (99.43 s). Shared-helper file regression
  `standalone-files-1792891676946434600.json` passes (60.98 s). Broader space/interruption cases,
  native review, project/migration composition and installed gate remain.
- **P2-STO-CLN:** full verified local-volume clone now composes quiesced export
  and isolated restore. Flushed job receipts, inactive-client leases, retained
  snapshots and separate pending-destination recovery prevent an interrupted
  copy from becoming an assumed success. Staging cleanup validates all entries
  before removing known files and never deletes a volume. Native SDK proof
  `volume-clone-1792923486489195100.json` passes active/paused writer refusal,
  existing-target refusal, Unicode/binary/link/numeric-owner/mode preservation,
  cancellation during import, snapshot disposal and independent pending-target
  recovery. The actual development desktop also cloned an isolated fixture via
  the volume action and keyboard review/submission; the destination payload was
  independently read, successful staging cleanup was visible, and both fixtures
  were ownership-checked and removed through the explicitly targeted CLI.
  Evidence: `artifacts/qualification/p2-volume-clone-20261025.json` and linked
  native screenshots/records. Three engine identities and global client hashes
  remain unchanged. This is a full copy with the existing portable format's
  metadata/size restrictions; it does not qualify crash/power-loss, disk-pressure,
  broad native accessibility, migration or the combined installed release gates.
- **P2-WSL/MIG/NAM/TLS and D01-D07:** remain to implement/evaluate. Reuse
  existing selected-WSL integration and upstream Docker mechanisms. Storage and
  migration need source/target ownership, consistency, rollback, collision and
  interruption contracts before writes. Names/trust need reversible explicit
  setup and actual client/VPN/port/lifecycle qualification.
- **P2-D07c platform audit:** the non-elevated scratch probe executes amd64 but
  arm64 fails with `exec format error` on the qualified x86_64 guest. Independent
  multi-platform Buildx local outputs match both precompiled binaries. See
  `orbwin-platform-2537dc8fe5444aa7/proof.json` and BL-019 for exact support and
  the distinction between cross-compilation and emulated runtime/build RUN.
  All unique fixture containers/images were ownership-checked and removed. No
  handler, package, guest setting or engine lifecycle change was made. This
  resolves the current support observation without claiming arm64 execution or
  closing the remaining P2-UI-SRT/native/installed gates.
- **P2-UI-SRT paused state:** paused containers now retain their own visible
  text/amber indicator rather than inheriting stopped-row dimming. Status sort
  places running rows/groups before paused rows/groups, then other states; this
  never treats a paused target as executable or changes lifecycle authorization.
  Frontend build and 50 JavaScript tests pass. Native development-window evidence
  `artifacts/qualification/p2-paused-ui-20261025.json` shows a real paused fixture,
  then removes the label after explicit resume and inventory refresh. The exact
  fixture was removed through the owned CLI. Size sorting, measured volume-size
  refresh, other platform UI and the installed combined gate remain open.
- **P2-STO-IMG export foundation:** bounded plain Docker tar inspection and
  verified new-file export are implemented through the SDK, typed native adapter
  and actual image action. Config/rootfs diff IDs and content-addressed blobs are
  checked without Windows extraction. The first live attempt exposed the
  containerd store's OCI index identity, distinct from a config digest; it
  correctly failed publication. The adapter now proves exported configs belong
  to the reviewed OCI root instead of weakening identity checks. Native proof
  `image-export-1792926090733148300/proof.json` passes selected-tag preservation,
  independent archive checksum, Unicode paths, existing-file/stale-ID refusal,
  cancellation with a retained named partial, source-layer preservation and
  exact unique-alias cleanup. The actual development desktop also exported an
  untagged reviewed image; its independently read hash matches the UI result.
  Summary: `artifacts/qualification/p2-image-export-20261025.json`, linked logs
  and screenshot. Full Go tests/vet, scoped Windows race tests, frontend build,
  50 JavaScript tests and Rust build/native adapter proof pass. The 64-GiB/count
  bounds are implementation limits, not measured scale guarantees. Large/disk-
  pressure cases, reviewed import/tag-conflict handling, source migration and
  the installed combined gate remain open. No generic unreviewed load is wired.
- **P2-STO-IMG reviewed import:** source implementation now validates and locks
  the archive, measures the verified daemon's storage-root free space through
  capped/read-only pinned tools, and loads only normalized configs/layers under
  random recovery references. It verifies staged content by streaming Save back
  before applying original tags. Existing references default to Skip; replacement
  requires its exact observed ID and explicit consent, with a verified recovery
  reference retaining previous content before any original tag write. Durable
  process leases/receipts retain partial outcomes; history and exact-record reads
  never replay or roll back changes and are available without a live engine.
  Native typed proof `image-import-1792928924892258400/proof.json` passes missing
  consent, stale review, skip, replacement/previous-content preservation, absent
  tag application, readback and settled receipt cases (31.57 s for the complete
  test). The desktop checks conflict defaults, blocked consent, cancel/keyboard
  focus return, fresh review after cancel, a real skip import and historical
  receipt inspection. This exposed and fixed the shared destructive-dialog label
  and initial-focus issues; bounded history now includes time/transaction identity.
  Source/desktop test aliases were exact-ID checked and removed through the owned
  CLI. Evidence: `artifacts/qualification/p2-image-import-20261025.json` and linked
  logs/screenshots. Full Go tests/vet, scoped Windows race tests, frontend build,
  53 JavaScript tests and native Rust qualification pass. The latest typed proof
  also covers the final storage-probe daemon guard; the desktop capture precedes
  that guard addition. Real mid-write client loss, multi-image/large-scale and
  disk-pressure/power-loss cases, supported source migration and installed gates
  remain open. Normalization preserves configs/layers, not OCI index digests,
  attestations/signatures or a multi-platform tag's original index identity.
  The pinned maintenance image is an explicit dependency shared with files/debug;
  fresh-install/offline tool provisioning remains part of the packaging gate.
  Offline history routing is implemented in source but has not been qualified
  against an actually stopped owned engine in this milestone.
- **P2-WSL current adapter:** typed original-project Compose/build requests and a
  pinned short-lived Linux adapter are implemented. The existing selected mount
  remains authoritative; normal-user path/type/inode checks, bounded in-memory
  Compose resolution, contained filesystem-field mapping, source-owned project
  namespace checks and reusable workload metadata are covered by initial unit
  tests. Resolved configuration uses sealed anonymous stdin memory, not a saved
  secrets-bearing file. Existing global contexts/shells remain independent.
  Initial live proof correctly refused to replace an already selected Ubuntu
  project before any fixture creation. Inspect that selection and retain it;
  do not disconnect a user's project to force qualification. Native runs remain
  prohibited by the current desktop restriction.
- **P2-NAM/TLS:** headless host-network probe confirmed distinct Windows/Ubuntu/
  engine loopback scopes (`p2-host-network-loopback.json`). Ordinary Docker port
  publishing subsequently reached the same fixture from Windows and Ubuntu
  (`p2-published-client-loopback.json`), supporting the relay direction recorded
  in `architecture/local-services.md`. Full client routing is still unqualified.
  A pure PKI module now creates a root public cert
  and constrained server-auth intermediate, retains no root private key in the
  serialized bundle, limits names to reserved namespaces and protects signing
  bundles with current-user Windows DPAPI bound to installation/root fingerprint.
  Headless tests pass verified TLS, malicious unrelated-domain/subordinate-CA
  chain refusal, parsing/expiry limits and DPAPI tamper/context refusal. No root
  trust, DNS or network integration was installed. This is a tested building
  block, not completed local HTTPS or a client-matrix claim.
  The static service asset implements fixed gateway relay and target-loopback
  dialing with monotonic leases. The Windows-side exec stream parser preserves
  binary frames, half-close and resumable read deadlines. Scratch-image import
  accepts only a verified bundled asset and exact inspected ownership; malformed,
  oversized, conflicting or failed import results refuse without retry/prune.
  Go unit tests and vet pass. These modules are not yet a wired route controller;
  live image/namespace transport, cancellation/resource bounds and installed
  service qualification remain required. While the desktop restriction is active,
  defer live engine/native/trust runs and continue source work/headless unit checks.
  The route registry/TLS selector and read-only namespace verification are now
  implemented with atomic snapshot replacement, explicit target incarnation,
  alias-shared limits, connection retirement and restricted host/SNI handling.
  Headless verified-TLS binary-upgrade replacement, cancellation, capacity and
  fake-daemon isolation tests pass. Persistent controller/setup UI remain unwired;
  this source-level progress does not complete the local service journey.
  Namespace ownership/recovery components now add an exclusive Windows process
  lease, flushed versioned intent/accepted-ID ordering, exact inactive recovery,
  retained/visible unknown creates, bounded lifecycle methods, daemon rechecks
  and joined renewal/dial cancellation. Headless tests pass failed start/restart,
  late create acceptance, foreign identity/isolation, partial journal refusal,
  exact cleanup and real child-process loss with lease release/record survival.
  The permanent process, relay/import ownership, discovery and setup/trust UI
  remain to implement; no native/live/installed acceptance is claimed.
  Discovery and reconciliation components now connect exact/coherent source
  identity, versionable preferences, HTTP-only probing, namespace ownership and
  route publication. Headless tests cover recreation/replicas/names/ports,
  partial failure, bounded automatic attempts, cancellation and teardown order.
  The real manager/Moby SDK passes a combined controlled TCP API/exec-framing
  journey. Host/shared-network target refusal is enforced at inspection as well
  as discovery. Permanent scheduling, relay/import ownership, authority/trust
  persistence, UI and native/live/installed qualification remain pending.
  Protected authority persistence now passes Windows headless creation/load,
  public-only export, mismatch/tamper/expiry/partial-state refusal and process-loss
  survival tests. Tools-image receipts now precede import, recover lost responses
  without replay, verify reuse, retain explicit-repair provenance and hand the
  same exclusive owner into the namespace manager. Tests/vet pass. Code review
  found a separate Windows relay-port reuse gap; a pinned internal TLS gateway
  must replace the raw relay prototype before permanent service activation.
  Trust installation/rotation/removal, gateway, process/UI and qualification remain.
  The authenticated gateway and Windows route-host components are now wired and
  pass headless TLS/mode/binary/half-close/shutdown tests. Linux relay source uses
  the pinned gateway and ABI 2; the combined DPAPI-authority/gateway/application
  TLS/real manager/Moby exec-fixture path passes. Actual Linux relay execution,
  Windows/WSL host-gateway traffic, permanent management/scheduling and trust/UI/
  installed gates remain. Before activation, close the namespace exec cancellation
  accounting gap: a closed attachment/idle timeout alone does not establish that
  repeated cancelled guest dials remain inside process/memory admission budgets.
  ABI 3 now adds helper-local nonce admission/revocation and acknowledged readiness/
  renewal. Windows tests cover failed cancellation poisoning and lost attachment
  cleanup without start replay; Ubuntu executes the control/gateway/tunnel tests.
  Live proof `orbwin-p2-stream-1792901702210273600` passes 64 idle revocations
  (32 sequential and eight batches of four), zero remaining exec IDs after each
  check, healthy non-OOM helper, and unchanged target incarnation. The same proof
  crosses the actual Linux relay and pinned Windows gateway: Windows redirect and
  verified application HTTPS for both reserved name families; Ubuntu and an owned
  container client verify HTTPS with an explicitly supplied test root. Temporary
  published ports and explicit fixture addresses do not establish production
  80/443 conflict behavior, automatic network aliases, installed/client-store trust
  or browser acceptance. No system trust was installed. Three daemon identities
  remain unchanged. Permanent relay ownership/process/UI and remaining gates are
  still required.
  The production relay configuration/ownership verifier and durable receipt
  components are now implemented and unit tested. They restrict publications to
  a reviewed loopback port pair, selected exact bridge IDs/reserved aliases, trusted command and
  security/resource profile; foreign profiles cannot authorize cleanup. Receipts
  flush intent before acceptance, refuse partial/foreign/replayed state and
  preserve unknown creates. These components are not wired to an active product
  relay yet.
  Relay recovery and the composed backend owner now pass ordered flush, unknown/
  late create, failed-start/removal, foreign-profile refusal, caller cancellation
  and fatal-host shutdown tests. Cleanup settles the relay before the namespace
  manager releases the common owner. Real pre-start recovery passes with both
  accepted-ID and unrecorded-ID receipts (`orbwin-p2-relay-recovery-1792906918902489700`);
  this uses explicit lease replacement, not a new process-kill claim.
  Current live journey `orbwin-p2-stream-1792906202883657300` passes normal Windows
  Go TLS, Windows Schannel revocation checks and Ubuntu curl with ordinary hostname/
  SNI/Host on the reviewed 18080/18443 pair, container verified HTTPS, 64 stream
  revocations, blocked default ports, and the production relay profile/readiness/
  renewal/exact-stop path. CRL profile 2 retains the root-key discard boundary,
  supplies signed local revocation metadata and excludes all IP/client identities
  from the ordinary signer. Service ABI 5 authenticates the private gateway in
  the relay readiness command. The Windows/WSL/Traefik default-port experiments
  expose real address-family/network conflicts; none is silently counted as
  supported. No DNS, firewall or trusted-root changes were applied. Windows race
  detector checks pass for localnet/gateway/streamcontrol using the existing
  workspace MinGW compiler; all package tests, vet and Linux component tests pass.
  The independent Go process/config/scheduler/private IPC and thin Rust/React
  setup are now implemented at source/build level. A private-pipe process
  restart proof preserves a reviewed 40,452-byte settings request (360 disabled
  preferences) in an isolated home without CA generation. Network settings add
  discovery, authority review, enable/disable/retry and exact-selector routing
  choices. Successful empty discovery is distinguished from unread state, and
  background polling preserves action failures and newer snapshots. Protected
  authority/HTTP-port validation precedes committing enabled intent. Windows
  localnet/gateway/streamcontrol race tests, full Go tests/vet, frontend build
  and Rust check pass for this milestone; enabled permanent-process/native UI,
  client network selection, trust transactions and installed qualification remain.
  See the local-services architecture record and
  `service-process-639285069935476000/proof.json` for precise evidence boundaries.
  The actual permanent-process journey now passes Windows verified HTTPS and a
  disposable container's selected-network `.test` DNS/verified HTTPS before and
  after killing/restarting the exact service process. Saved revision/root and
  workload incarnation survive; old helper IDs retire; disable leaves zero
  helpers/receipts and releases the ports. Evidence:
  `orbwin-p2-runtime-1792910593975080200/process-proof.json`. The UI adds reviewed
  exact-ID client-network choices. Per-client roots were supplied; no trust store
  was changed. Native setup/installed trust, resource and upgrade gates remain.
  Interrupted authority setup now has an explicit reviewed-fingerprint adoption
  operation; it validates protected state/port/revision and preserves key bytes.
  Uncertain settings acknowledgement rereads committed intent without replaying
  the write. Contract tests model these boundaries; broader filesystem faults
  remain unqualified. Latest expanded native process pass after these changes:
  `orbwin-p2-runtime-1792911075778212800/process-proof.json`. Go tests/vet, scoped
  Windows race tests, frontend build, 46 JavaScript tests and Rust check pass.
  The 2026-10-25 trust source milestone adds actor-serialized CurrentUser physical
  Root install/status/removal, durable exact-certificate/user ownership receipts,
  denial/unknown-acceptance recovery and UI consent controls. Tests use fake stores
  and the native in-memory provider only; real Root mutations remain attended.
  Unreadable/regressed settings now retire local ingress until restored. Snapshot
  ordering uses instance/sequence after tests exposed equal Windows timestamps.
  Evidence: `p2-windows-trust-source-20261025.json`, its unit JSONL, and the rebuilt
  process pass `orbwin-p2-runtime-1792911942516742800/process-proof.json`.
  Go tests/vet, scoped Windows race tests, frontend build, 49 JavaScript tests and
  Rust check pass. Real trust/browser/rotation, native UI, WSL and installed
  qualification plus the other original scope gates remain unfinished.
  Authority/port rotation is now journaled and exposed through review/resume/
  cancel controls. Tests cover interrupted file/settings boundaries, real Windows
  cleanup sharing failure, tamper/stale/trust/active-access refusal, cancellation,
  pending-runtime restart and shutdown queue rejection. The expanded live process
  journey `orbwin-p2-runtime-1792913827449999600/process-proof.json` rotates to
  28080/28443 and verifies the new root from Windows Go, Schannel curl, Ubuntu curl
  and a selected-network container, while rejecting an old-root-only client.
  Listener captures before/after are loopback-only; workloads and global engine
  identities/configs are preserved, and owned fixtures settle. No roots are
  installed or removed. Source/unit evidence:
  `p2-authority-rotation-source-20261025.json` and its raw JSONL. Native UI,
  attended actual trust and installed qualification remain open alongside the
  original Compose/files/WSL/storage/migration/decision/resource gates.
  Repeated publication handoff checks exposed a real guest-loopback binding gap
  after that first pass. The owned relay now accepts the guest tap-address path;
  this leaves Windows publication loopback-only. Latest repeated full passes are
  `orbwin-p2-stream-1792907617263940300` and `orbwin-p2-stream-1792907628147151200`,
  including captured IPv4/IPv6 Windows listener scope. BL-018 preserves the failed
  runs and requires separate user-container publishing reproduction under D06;
  do not generalize this helper adaptation into a broad compatibility claim.

## Environment and qualification discipline

The 2026-10-25 selected-WSL-trust foundation adds the public-only Linux helper,
machine/registration/user/sequence-bound guest receipts and exact certificate
ownership, plus Windows current-user WSL 2 registration discovery and a hidden
GUID-addressed, independently verified streamed helper transport. Its unit/failure
tests and real Ubuntu updater tests in private directories pass. A native Windows
read-only status call reaches the production helper, rejects a corrupted staged
copy, and leaves system bundle/updater hashes and helper temporary directories
unchanged. Evidence: `p2-wsl-trust-foundation-20261025.json` and its linked raw logs.
The new helper participates in build/package/provenance manifests and SDK source
attestation. The subsequent source milestone integrates Windows-owned public
receipts, both receipt-sequence guards, actor/typed IPC, reviewed desktop controls,
attested trust assets and WSL-aware authority rotation. Controlled tests cover
ownership-before-dispatch, unowned/foreign/stale/regressed/tampered state, lost
transport and host file acknowledgements, removal after signing-state loss and
rotation refusal without starting a guest. A browser-only fixture verifies the
review and blocked/recovery states and exposed a shared confirmation-dialog focus
wrap defect, now fixed and rechecked. The native read-only SDK/pipe/actor/helper
journey `orbwin-p2-wsl-trust-1792919423553389300/proof.json` passes with unchanged
system certificate files/global configs and clean process shutdown. Evidence:
`p2-wsl-trust-integration-20261025.json`, raw logs and the fixture screenshot.
Full Go tests/vet, scoped Windows race tests, frontend build, 49 JavaScript tests
and Rust check pass. The subsequent missing-registration recovery milestone adds
host-only public receipt listing and reviewed retirement, raw GUID presence checks
distinct from filtered distro discovery, intent/final/post-write observations and
fresh rotation begin/resume guards. Controlled and native private-registry tests
cover uncertainty and returned registrations. Native SDK proof
`orbwin-p2-wsl-retirement-1792921388335358000/proof.json` retires a controlled
pre-dispatch intent for a never-registered GUID and rotates a private test
authority without changing system trust or registrations. The updated native
read-only journey is `orbwin-p2-wsl-trust-1792921389156663300/proof.json`; the
browser-only fixture checks nine recovery states and exact frozen submission.
Evidence: `p2-wsl-trust-retirement-20261025.json` and linked logs/screenshot.
Selected WSL trust still needs attended actual trust consent/install/remove,
browser/custom clients, native Windows UI and installed update/uninstall
qualification. Returned retired GUIDs require manual review and cannot silently
regain ownership. No real system trust installation or removal has occurred.
These source milestones close none of the remaining Phase 2 families or final
acceptance gates.

The current scope addendum also requires activity monitoring, capacity/image
archives/volume clone, list/run/network polish, routing labels/multiple exact
names, Dev Containers and opt-in LAN exposure, plus D08-D11 and the expanded
D01-D07 decisions. Local TLS milestones do not close those rows. Carry the current
`phase-2-scope.md` addendum through the final audit; do not use the original
seven-family list as a substitute for the current accepted scope.

Owned daemon `d9c00194-ea9d-439a-8561-6a0ec580fbd5`, fixed product pipe and
registered persistent home remain authoritative. Docker Desktop and Rancher
identities/global configs must be retained. Last reuse postcheck is
`artifacts/qualification/phase-2-compose-reuse-postcheck.json`. The optional cluster
was not reconfigured. The installed Phase 1 app has not been replaced.

Only unique disposable exact-owned fixtures may be cleaned; never global prune,
WSL shutdown/reset/unregister or migration of actual user data. Debug fixture
binary is generated/ignored at `tests/workloads/debug-target/target`; build with
pinned Go 1.27.3 for linux/amd64, CGO disabled. `npm run session:build` rebuilds
the Windows SDK helper and source-hash manifest. Root/localnet/gateway/streamcontrol
production Go files are enumerated by both the build script and Rust build-time
source attestation; guest helpers retain separate explicit manifests. New module
directories must join both enumerations before qualification. Session source
hash refusals are expected if code changed after the helper was built.

Do not report percentages or Phase 2 acceptance until every included scope row,
decision and release gate is accounted for with installed preservation,
coexistence, compatibility and resource evidence.

### Selected-source migration: initial Desktop image adapter (2026-10-25)

Added a fixed, read-only Docker Desktop Linux pipe adapter, bounded/redacted
image/volume/Compose-membership inventory and selected-image export using the
existing verified Docker archive writer. Source and owned target identities must
differ; the initial adapter candidate checks Linux/amd64 Engine 29.8.0. No source
mutation API is permitted by its transport. The native scratch-fixture transfer
from Desktop 4.91.0 to OrbWin completed with matching rootfs, retained source,
stale-source/no-overwrite refusals, exact fixture cleanup and unchanged global
configs. Evidence and contracts: [migration architecture](architecture/migration.md),
`artifacts/qualification/p2-migration-desktop-20261025.json` and linked proof.
Full Go tests/vet and scoped Windows race tests pass. The Images-page source
review/selection and actual fixture export also passed in the development native
window, with reverse Tab containment, focus return and a fresh review on reopen.
Frontend build, 56 JavaScript tests and Rust operation tests pass. Source volume
transactions, Rancher transport, reviewed project reuse, combined migration job
accounting/history and installed end-to-end migration remain required.
No actual user resource was selected for transfer and no elevation/trust change
was performed.

### Selected-source volumes and exact source-tool recovery (2026-10-25)

Desktop source-volume review/export now verifies the source/target distinction,
ordinary local layout, metadata fingerprint, writable-user quiescence and new
destination. It uses a separately scoped maintenance client and a bundled static
scratch helper with a read-only source bind, avoiding source-volume auto-creation
and a registry dependency. Flushed job/phase receipts and an exclusive process
lease protect exact tool cleanup/recovery; source data and host packages are
never recovery deletion targets. Source cleanup can run while the owned target
is unavailable, using its host-recorded identity. Full contracts and residuals
are in [migration architecture](architecture/migration.md).

The current native fixture exported binary/Unicode/permission/link data from
Desktop, restored it into a fresh OrbWin volume and matched target readback. Real
SDK process loss after helper acknowledgement recovered through the exact saved
receipt. Running writable users, stale metadata and destination reuse refused;
cleanup errors were empty. The first canonical-capability mismatch failure is
retained with its successful exact recovery evidence. The Volumes-page native
review/export journey also passed on an empty disposable source, including the
blocked unchecked state and focus return. Actual user data was not transferred.
Evidence: `artifacts/qualification/p2-migration-volume-20261025.json` and links.

Full Go tests/vet, scoped Windows race checks, frontend build, 58 JavaScript tests,
16 Rust operation tests and development Rust build pass. Final offline recovery
routing is source-implemented with a controlled unavailable-target adapter check;
actual installed/offline desktop and scale/fault qualification remain open.
Rancher source transport, project reuse, integrated migration accounting/history,
target maintenance-image provisioning and installed combined acceptance are still
required. No elevation, system trust writes, global context changes or engine
shutdowns were performed. Existing source workloads were not stopped.

### Rancher WSL migration and source-volume recovery (2026-10-25)

The selected-source migration path now reaches Rancher's fixed Unix socket
through an explicitly chosen ordinary Ubuntu 24.04 WSL registration. It verifies
the bundled Linux CLI, registration/machine identity and Rancher Engine 29.5.3 /
API 1.54 profile. Linux root provides existing socket access; Windows elevation,
socket permission changes and default Docker context changes are unnecessary.
The active Windows Rancher application version still needs definitive mapping:
both 2.0.0 and 1.24.0 uninstall entries exist and do not prove socket ownership.

Image inventory/export is read-only. The process transport preserves wire bytes,
backpressure, resettable deadlines and bounded child reaping. Fixed-output
maintenance exec uses the same private WSL dialer as ordinary HTTP, with a scoped
request hook on both paths. Only acknowledged job-owned source helpers/images
and exec IDs receive mutation permission; source volumes cannot be created or
deleted by that production client. No interactive-stdin/half-close support is
claimed. Rancher source-volume receipts additionally freeze the WSL tuple;
recovery refuses missing/replaced/renamed registrations rather than substituting
a connection. Desktop and Rancher share the volume transaction/receipt protocol.

Native fixtures verified image rootfs transfer, no-overwrite/wrong-machine
refusal, source preservation and owner cancellation. Rancher volume export,
fresh OrbWin restore and complete metadata/tree readback passed for binary and
Unicode data, UID/GID/modes, hardlinks and symlinks. Exact recovery after a real
SDK process kill and controlled unavailable-target recovery passed; no helpers,
scratch source images or WSL connection stages remained. Desktop volume and
Rancher image regressions passed after the shared transport changes.

The Images and Volumes native desktop journeys independently exported disposable
Rancher fixtures. Volume export stayed blocked until metadata/quiescence review,
and its saved receipt reached `cleaned`. Source fixtures were verified and removed
through exact non-UI cleanup. All three daemon identities, the original owned
volumes, and global Docker/Kubernetes configuration hashes were retained. The
installed Phase 1 window remained open and unchanged. This work ran without
Windows elevation; actual trust-store writes remain for an attended batch.

Evidence: `artifacts/qualification/p2-rancher-migration-20261025.json`,
`p2-rancher-volume-20261025.json` and [migration architecture](architecture/migration.md).
Full Go tests/vet and scoped race checks, 61 JavaScript tests, 18 Rust operation
tests and development builds pass. The 503.62-kB initial frontend chunk retains
its build warning pending resource-budget work. This does not qualify large
archives/volumes, disk pressure, every process-loss timing, the installed/offline
desktop matrix or complete project/application migration. Reviewed project reuse,
combined migration history/recovery ergonomics, fresh/offline target maintenance
provisioning, source-version qualification and every remaining Phase 2 row and
decision are still required; the full phase goal remains active.

### Source project review into existing Compose validation (2026-10-25)

Source Compose inventory now has validated member identities and presence flags
for explicit env files, binds and one-off containers, without copying env paths/
contents or arbitrary labels. The Containers page offers source project review
using the existing Desktop/Rancher picker and coherent ordered Windows-file
discovery. It shows ordinary service/volume observations separately, refuses to
guess conflicting/unsupported paths or profiles, warns about current target
project-name collisions, and opens the existing editable Compose Validate form.
The reviewed source and file hints are retained independently of later inventory
changes. Linux source paths are not translated through the socket connection's
WSL distro; the original project connection remains explicit.

A real stopped Desktop Compose fixture with two ordered space/Unicode filenames
and an empty volume passed source review, native UI handoff and quiet validation
on OrbWin. No target containers were created; the exact source fixture stayed in
created state until guarded non-forced cleanup. Files remain available and no
remembered hint was saved. Source/target/global configuration preservation and
installed Phase 1 window retention passed. Evidence:
`artifacts/qualification/p2-migration-project-20261025.json` and the updated
[migration contract](architecture/migration.md). Full SDK tests and the source
inventory race check, 65 JavaScript tests and development builds pass.

Migration controls now load in a separate frontend chunk with a loading state;
the initial JavaScript chunk is about 490 kB and no longer triggers Vite's 500-kB
warning. Runtime budget acceptance is still open. This handoff does not complete
integrated project/application migration: original WSL project handoff, explicit
env-file cases, source drift/target collisions, durable combined history and
data/config mapping need further work, alongside all remaining Phase 2 scope and
installed qualification gates. No Windows elevation or trust changes were used.

### Original WSL project migration handoff (2026-10-25)

Project review now offers an explicit ordinary WSL project-distro choice,
independent of Rancher's socket connection. Inspection uses the existing export
and never connects or replaces a root. The editable Linux Compose validation form
retains coherent ordered paths inside that root and carries the reviewed root
and registration into native preparation. Changed roots and absent registrations
refuse; helper launch is by registration ID as the ordinary default user. Source
and shared-mount identity checks remain owned by the existing project helper.

The first native attempt retained a useful path mismatch refusal: the shared
private config referenced the installed app's public Linux tools. Preparation now
accepts that existing directory only when its canonical drive path and every
Linux asset match the current pins. Hashing uses a blocking worker. Neither the
private config nor the export record is rewritten. Native UI/adapter tests verify
the root handoff, quiet Unicode/ordered-file validation, stale-root/missing-GUID
refusals, exact stopped-source fixture cleanup, no target container creation and
unchanged private/global configuration and three-engine identities. The selected
export inode remains `2112:1374236`; the installed app is untouched.

Evidence: `artifacts/qualification/p2-migration-wsl-20261025.json` and
[WSL integration](architecture/wsl-integration.md). 68 JavaScript tests, 18 Rust
operation tests, three WSL integration unit tests, final opt-in native adapter
proof and builds pass. Native UI captures precede the final hash-worker/path
check, status announcement and model-only path checks; final native/unit/build results cover
those deltas. No Windows elevation, trust writes, reconnect, engine shutdown or
user distro termination was performed. This does not close installed/offline,
multi-user-distro, cancellation/source-replacement or full application-migration
gates. Next work includes retaining reviewed roots for legacy relative-path
shortcuts and durable combined migration accounting/data mapping; the complete
Phase 2 scope and installed acceptance objective remain active.

### WSL workflow root and identity retention (2026-10-25)

Connected-page Compose/build and reopened WSL group/service actions now retain
the reviewed root and its device/inode signature. Migration uses the same review
with its explicit registration ID. Native preparation rejects a different saved
root or signature before launching a workflow; the existing Linux helper still
checks actual source/shared-mount identity. New reusable labels retain the
signature. Older metadata remains identifiable but requires explicit project
inspection before Compose reuse, with ordinary container actions preserved.
Legacy direct IPC without a review keeps current-selection compatibility.

71 frontend tests, 18 operation tests, three WSL integration tests, Linux project
helper tests, SDK tests and the frontend build pass. The native adapter validates
ordered relative Unicode inputs as UID 1000 and refuses root/signature drift,
missing registration and stale build review. A fixture path error was corrected
after its first native refusal; no product path rule was relaxed. Both exact
fixture directories were removed. Three engine IDs, private/project/global config
hashes and the original/shared inode are unchanged. No Windows elevation was used.

Evidence: `artifacts/qualification/p2-wsl-root-retention-20261025.json` and
[WSL integration](architecture/wsl-integration.md). This qualifies the adapter
boundary, not installed behavior, destructive source replacement, cancellation
races or the disabled older-label UI state. Durable combined migration accounting,
data/config mapping, the remaining scope/decision rows and installed acceptance
remain required.

### Discoverable source-volume recovery history (2026-10-25)

Saved Desktop/Rancher source-volume receipts are now discoverable from Volumes,
without requiring a copied transaction ID. The bounded local read exposes
historical source/volume/destination, phase and export acknowledgement separately;
it never implies package existence, completed import or current inactivity.
Partial/foreign/unreadable records appear as uncertain identities. Exact recovery
still uses the existing exclusive lease and provenance checks. Cleaned/uncertain
rows disable automatic recovery. No second ledger or engine supervisor was added.

Full Go tests, 74 frontend tests, 18 operation tests and builds pass. The native
adapter and desktop dialog read two real saved receipts; all four receipt/lease
files remain byte-identical. Native layout and focus defects found during review
were corrected; final Tab/Shift+Tab containment and Escape focus return pass.
Installed Phase 1 remains open, and engine/config preservation checks pass without
Windows elevation. Evidence: `artifacts/qualification/p2-source-history-20261025.json`.
Installed/offline outage, active/torn receipt UI, combined migration mapping and
the full Phase 2 acceptance/decision gates remain open.

### Explicit Compose interpolation env files (2026-10-25)

Windows and WSL Compose forms now accept up to 16 ordered env-file paths. Native
dispatch keeps each as a separate client argument; WSL uses the existing root/
inode review and contained regular-file mapper. Remembered inputs and WSL reuse
labels retain only paths. Existing requests/hints remain compatible. Source
project reviews still disclose only the presence of explicit env files and now
require manual env-file selection before validation; no source credentials or
env contents are copied into the form.

Windows native lifecycle qualification verifies both file orders and an adjacent
unselected `.env`, rejects a missing file before dispatch and removes its owned
project. WSL quietly validates ordered Unicode absolute/relative files and rejects
reversed/missing selections with root guards intact. Exact WSL fixture cleanup,
75 frontend tests, 19 operation tests, Linux helper tests and builds pass. A native
desktop form separately completes quiet validation with two env files and Remember
unchecked. Three-engine identity/config preservation passes without elevation.

Evidence: `artifacts/qualification/p2-compose-env-20261025.json` and
[Compose architecture](architecture/compose-projects.md). Installed, WSL env-file
lifecycle, migration required-list UI, cancellation/error credential cases and
combined migration accounting/mapping remain open with the rest of Phase 2.

### Bundled offline maintenance image (2026-10-25)

File/storage/debug flows now use a verified tag-free archive of the existing
pinned Alpine tools. A missing cache image loads only into the verified owned
daemon, without a registry pull or tag write, followed by exact readback before
helper creation. Concurrent loads share immutable cache; cancellation may retain
cache bytes but never rolls back user images. Native verification and a bounded
SDK private copy prevent bundle replacement from changing the loaded bytes.
Pinned compressed inputs precede general archive decompression. Existing helper
leases and source-migration scratch-image receipts retain their separate owners.
Debug also rechecks target start identity after tools preparation. The obsolete
Pull prerequisite is removed from terminal/file guidance.

Canonical staging, corrupted-asset refusal, actual Load/readback with an injected
initial cache miss, fault/cancellation tests, vet and race checks pass. Native
portable-volume, standalone-file, volume-clone, image-import and Debug Shell
regressions pass; 75 frontend tests, 19 operation tests and builds pass. Some cached
helper regressions precede cold-load-only refinements; current fault/Load checks
cover those changes. Existing data/configs, three-engine identities, 14 images,
two original volumes and the installed app are retained; transient helpers are
empty. No Windows elevation was used.

Evidence: `artifacts/qualification/p2-maintenance-20261025.json` and
[development-data contract](architecture/development-data.md). This is not an
empty-engine or installed qualification. Initial package preparation refused
the one unresolved full-Alpine notice entry, rather than producing an installer.
The embedded package inventory and all ten exact-revision APKBUILD recipes are
recorded; completing notices/build inputs/source accounting is the next packaging
action. The complete migration/scope/decision and installed acceptance gates stay
active.

### Source distribution and first bundled candidate (2026-10-25)

The Alpine notice/source gate is resolved against the pinned image package
manifest. Collection verifies all ten installed-package origins and records the
utmps/skalibs static build contexts at BusyBox's recipe revision, without running
upstream recipes. A frozen 24,226,031-byte source archive contains 190 source
files, 17 notices and its inventory. Independent member/checksum readback and an
offline cache rebuild pass. Source revisions/versions, checksums, package notice
coverage and the full archive are checked before package copies are published;
changed or incomplete inputs fail closed. All license/source resources are now
included in the generated package hash manifest.

Four Python tests and 78 JavaScript tests pass. Package preparation resolves 773
dependencies with zero missing notice entries, and all 1,102 staged resource
hashes pass. The optimized app and NSIS installer build succeed. The candidate
is preserved with its compiler script, manifests and checksum at
`artifacts/qualification/p2-candidate-0784bea91f61-20261025/`; installer SHA-256
`0784bea91f61a24eec612c1158f58dbb50a4a5c6294b56e7530226c713687a0f`.
It is an **uninstalled, unqualified candidate**, still at version 0.1.0. No
installed upgrade, fresh-empty-engine qualification or Phase 2 acceptance is
implied. The existing app, three daemon identities and private/global configs
are retained without elevation or trust writes.

Evidence: `artifacts/qualification/p2-maintenance-distribution-20261025.json` and
[source distribution contract](architecture/development-data.md). Source/notice
inventory is not a legal review or an identical upstream binary rebuild. The
remaining Phase 2 flows, decisions, resource and installed matrices stay active;
this candidate does not narrow the original objective.

## Activity monitor expansion — 2026-10-25

Added an engine container activity dialog and shared six-chart container Stats
view, with bounded adjustable sampling, explicit cumulative I/O units, exact
selection identity checks and previous-success timestamps on errors. Histories
remain view-local and independent of inventory. The engine dialog includes
Kubernetes containers and explicitly excludes VM overhead from its totals.

82 JavaScript tests, 51 ordinary Rust tests, frontend/desktop builds and an
opt-in native engine/selected-container stats proof passed. Real UI metrics,
keyboard pause, interval change and Escape/focus return were observed without
elevation. Five direct samples measured 1891–1958 ms read latency; CPU/memory
overhead and the remaining fault/target/installed matrices are still pending.
An isolated CLI/model proof subsequently covered stopped and same-name recreated
containers and restored the exact prior container inventory. Three client reads
measured 31.25–62.5 ms Windows CLI CPU each; working-set measurement was unavailable.
See [activity monitor contract](architecture/activity-monitor.md). P2-MON remains
open; no Phase 2 acceptance or replacement of the preserved installer is implied.

Activity polling now retains the native read slot until cancellation settles,
pauses automatic sampling on Cancel, and suppresses late replies after disposal.
86 JavaScript tests include deterministic hidden/busy/interval/cancel/fault cases.
Actual native cancellation at admission and a subsequent successful read passed;
cancellation during CLI collection and its UI interaction remain unverified.

## Host data volume capacity — 2026-10-25

Storage now displays a quota-aware Windows data-volume capacity meter and
physical free bytes separately from Docker usage and VHDX file length. Native
read errors preserve the file-length observation. Cancel/navigation invalidate
both Docker and native metadata replies; a cancelled native read finishes without
repopulating the view. The UI includes sparse-disk and backup consistency guidance.

53 ordinary Rust tests, 86 JavaScript tests, native owned-disk measurement and
frontend/desktop builds passed. An independent later .NET observation matched the
host total capacity. Native UI showed the meter and actual values; cancelling a
pending metadata read suppressed its later result. See the
[capacity contract](architecture/host-capacity.md) for evidence and boundaries.
Per-volume size refresh, quota-enabled filesystem qualification and installed
qualification remain open. This closes no broader storage or Phase 2 gate.

## Explicit per-volume accounting — 2026-10-25

Volume Info now offers an explicit engine-reported size refresh, timestamp and
container-reference count. A dedicated read-only SDK path uses upstream Moby's
volume-only disk usage and rechecks selected metadata/daemon identity. No helper,
mount, maintenance image or automatic inventory-time scan is involved. Unsupported
driver options, stale selections and unknown counters produce errors; writers
remain live and cancellation does not promise to stop work in the daemon.

The full Go suite/vet and scoped race/fault tests pass. Live isolated-volume
qualification covered nonzero data, attached/stopped/unused reference counts,
stale refusal, unchanged file hashes and exact fixture cleanup. Native adapter
and UI measured an existing empty volume successfully; native stale refusal also
passed. 87 JavaScript tests and 54 ordinary Rust tests pass. See
[volume-size contract](architecture/volume-size.md). Installed, scale and remaining
cancellation/replacement interaction checks remain open; original Phase 2 scope
is retained.

## Migration volume configuration handoff — 2026-10-25

Source review now carries bounded service mount destinations and read-only
intent without emitting host/guest-private source paths. Supported full-volume
mounts can be explicitly assigned to existing OrbWin destinations and saved as a
strict external-volume Compose override. Native saving refuses overwrite and
rechecks the target engine and selected metadata. Windows handoff appends the
saved file last in the existing editable validation form; it never starts Up.

90 JavaScript tests, 56 ordinary Rust tests, Go tests/vet/scoped race and an actual
Compose merge passed. Native UI qualified discover/select/preview/Save/append/
quiet Validate using isolated empty fixtures, with Remember unchecked. Fixtures
were removed after exact ownership and never-started checks; original inventories
and the source fixture file were retained. See the
[mapping contract](architecture/migration-mapping.md). Combined durable migration
accounting and receipt linkage, nonempty integrated transfer, Linux staging and
installed qualification remain open. No complete migration or phase acceptance
is implied.

## Connected WSL mapping save — 2026-10-25

The migration project reviewer can now save its strict volume mapping into the
existing connected Linux project as the normal distro user and append that path
last in the editable Linux Validate form. Creation is exclusive, root-identity
bound and mode 0600; cancellation or postcheck failure may retain a named file
and never triggers automatic deletion. The helper rechecks destination volume
metadata and engine identity. Source workloads and global configuration are
retained. Saved mappings now bind to current draft contents as well as review
identity, fixing a stale Windows mapping handoff after destination edits.

91 JavaScript tests, 57 ordinary Rust tests, Linux filesystem/refusal/concurrency
tests, Go suite/vet and frontend build passed. Actual native adapter qualification
under UID 1000 verified save/hash/permissions, stale-root/daemon refusal,
overwrite refusal and Compose validation. Both unique test files were removed
after identity/hash checks; cleanup errors were empty. A separate native UI proof
qualified explicit distro selection, existing-connection inspection, save/digest,
stale-draft invalidation and append-last Linux Validate. Exact cleanup restored
the source and target fixture inventories and removed only the two new Linux
files. Three daemon identities and private/global client/project-record hashes
were retained, with Windows elevation false. See the
[mapping contract](architecture/migration-mapping.md). Combined migration
accounting, nonempty integrated transfer, active-save cancellation interaction
and installed qualification remain open. No phase acceptance is implied.

## Source-to-restore provenance — 2026-10-25

Export results and the existing durable restore intents now retain the full
validated manifest digest and original engine/volume, alongside the payload
checksum. Payload-only matching could conflate empty volumes or metadata changes.
Source history derives matching restore transaction/name/phase observations from
the owning receipts, without creating another execution ledger, dialing engines
or mutating data. Legacy receipts retain recovery behavior and show provenance
unavailable. Historical completion does not verify current writable contents.

92 JavaScript tests, 57 ordinary Rust tests, Go suite/vet/scoped race checks and
frontend/desktop builds pass. Both nonempty Desktop and Rancher export/restore
proofs qualify the digest/source linkage, restored data/metadata, source retention,
interruption recovery and exact cleanup. Native UI qualified honest legacy rows,
readable columns and Escape focus return; positive linked rows and smaller-window
interaction remain pending. Three daemon identities and private/global config
hashes were preserved without Windows elevation. See the
[migration contract](architecture/migration.md). Project/image/mapping coordination,
current destination ownership review and installed complete-journey qualification
remain open with the rest of the full Phase 2 scope.

## Current restored destination review — 2026-10-25

Mapping now offers an explicit read-only restore/destination check. New restore
receipts acknowledge the destination metadata fingerprint before attachment;
review requires a complete receipt, source export/full-manifest linkage, matching
source/target identity and current ownership labels/metadata, with postchecks.
Legacy receipts keep recovery behavior and cannot establish the new identity
claim. This observation does not scan current contents, reserve a name or grant
startup/cleanup authority. Selection changes discard the UI result.

93 JavaScript tests, 57 ordinary Rust tests, Go suite/vet/scoped race checks and
frontend/desktop builds pass. Real nonempty Desktop and Rancher proofs pass the
current destination check and reject a foreign source volume. A separate native
UI journey carried a real nonempty export/restore through destination review,
native mapping Save, append-last Windows Compose Validate and positive historical
link rendering, with Remember unchecked and no project startup. Exact fixture
cleanup restored both inventories; the package/mapping/receipts remain evidence.
A check after removal refused the destination. Three daemon IDs and private/
project/global config hashes were preserved, with Windows elevation false.

See the [mapping contract](architecture/migration-mapping.md). Full project/image
coordination, application startup/data use, retained mapping/receipt association,
additional cancellation/replacement/sizing UI and installed complete-journey
qualification remain open. This is progress toward the original full scope,
not a Phase 2 acceptance record.

## Combined migrated application proof — 2026-10-25

The new opt-in `scripts/qualify-migration-project.mjs` carries a fresh application
image and nonempty source volume through production SDK transfer transactions,
source-to-restore/current-destination review, generated Compose mapping and actual
destination startup. The service serves restored text and exact binary bytes
through an ephemeral loopback port. Persistent state survives Stop/Up with a
startup counter advancing from 1 to 2. A later source export matches its original
complete data/metadata and contains no destination-only writes; the source project
member remains never started. This is a quiesced file-data fixture, not a database
backup consistency claim.

The final application proof and a prior static-server run pass. Initial fixture
failures exposed object-key-order comparison and a missing BusyBox HTTP applet;
structural rootfs comparison and an original static Go server corrected the
harness. Failed and successful runs all restored baseline inventories with empty
cleanup errors. The separate image-only transfer regression also passes.
Three daemon identities and private/project/global configuration hashes were
preserved without elevation, and the installed app remained running. See the
[combined migration qualification](architecture/migration.md).

Compose startup here uses the explicitly targeted CLI. Native/installed startup
and data-use, observed project-image association, retained image/project/mapping/
receipt coordination, Rancher combined application migration and the remaining
scope/matrix gates are still open. This evidence does not mark Phase 2 complete.

## Project member image association — 2026-10-25

Source project review now carries API-reported immutable member image IDs and
groups them by ordinary service. Different replica images stay separate;
one-offs do not imply ordinary-service requirements. Legacy or missing identity
remains unknown, malformed IDs refuse inventory and absent image summaries cannot
offer export. Users can select an observed tag or immutable ID and open the
existing guarded image export operation directly from project review. This does
not infer current Compose contents or import/start anything automatically.

Project image export now hides the reviewer while retaining its in-memory source
snapshot and parent-owned Windows mapping choices; returning reopens the snapshot.
Refresh/Close still invalidate it. Linux child connection/staging state requires
reinspection after that child unmount. Durable combined accounting remains open.

94 JavaScript tests, 57 ordinary Rust tests, Go suite/vet/scoped race checks and
frontend/desktop builds pass. The combined application regression now asserts
exact project-to-image association and passes through transferred application
startup, restored-data use and restart persistence. Native UI qualified service/
image display, keyboard immutable-ID selection, export to a new verified archive
and return to the retained selected project. Exact cleanup restored source/target
container/volume/image inventories; the original fixture file and all three
daemon/private/project/global configuration identities were preserved without
elevation. See [observed member images](architecture/migration.md).

Retained Windows mapping-choice interaction, ambiguous/missing-image UI cases,
Rancher project-image UI, durable receipt coordination and installed qualification
remain open with the other full Phase 2 requirements.

## Durable source-image export acknowledgement — 2026-10-25

Desktop and Rancher image exports now flush a source/target/image/reference/path
intent before Save and a separate verified archive checksum/size/config-ID
acknowledgement before returning success. Export receipts survive loss of the UI
response. An interrupted or failed acknowledgement leaves publication uncertain;
the product retains receipt and archive/partial files without retry or deletion.
No engine resource or cleanup authority is granted by an image export receipt.
Source image root identities remain distinct from archive config identities.

Go tests, vet, scoped Windows race checks, 57 ordinary Rust tests and desktop/SDK
builds pass. Portable receipt validation/parsing also passes under ordinary WSL;
Windows-only lease/writer tests explicitly skip there. Actual Desktop transfer,
read-only Rancher export and the combined Desktop application migration all
verify the new source acknowledgement against actual archive bytes/configs.
The application fixture still starts, uses restored data and preserves writes
across Stop/Up; fixture cleanup restores baseline inventories. The three daemon
identities and private/project/global config hashes remain unchanged without
Windows elevation. See [the receipt contract and evidence](architecture/migration.md).

Derived import links, restart-visible history, complete migration coordination
and installed qualification remain open. This increment does not close Phase 2.

## Source-image to import history — 2026-10-25

Image history now derives links from owning export/import journals by target,
exact archive checksum and complete config-ID set. It displays recorded import
phases and verified-config counts, including unfinished imports; it never infers
current image/tag ownership. Reads are bounded and do not call an engine or
change receipts. Invalid source rows expose only the transaction token, and
unreadable/torn import receipts cannot establish links. The native Images and
Containers controls offer the same saved history with refresh and cancellation.

Actual Desktop and Rancher image transfers link to completed import receipts.
The combined Desktop application migration still qualifies startup, restored
data use, restart persistence, source retention and baseline fixture cleanup.
Its owning receipts intentionally remain in app storage. A subsequently launched
native development app displayed this completed link at 1280×850, with keyboard
focus containment, Escape focus return and keyboard reopening. No new installed
acceptance is claimed. Evidence and exact receipt IDs are in
[the migration record](architecture/migration.md).

97 JavaScript tests, Go suite/vet/focused race checks, portable WSL reader tests,
57 serial Rust tests and frontend/SDK/desktop builds pass. A concurrent Rust run
had four process-test timeouts; subsequent serial runs passed, with cause still
unproven. The three engine IDs and private/project/global hashes remain unchanged
without elevation; the original installed app remains open. Native error/empty/
cancel/unverified-row and actual offline UI checks, complete restart-safe migration
coordination, Rancher combined application migration and full installed/matrix
acceptance remain open with the original Phase 2 scope.

## Synthetic private-client qualification — 2026-10-25

The actual native RegistryList/Login/Forget production adapter, staged Docker
alias and Dev Containers 0.469.0 passed against a new nonce-labelled, bounded
loopback-only registry. Wrong-password authentication failed; correct login
staged only helper routing and saved the synthetic credential in the Windows
store. An uncached alias pull matched an independently fetched registry manifest
and config digest. Dev Containers then forced another private pull and completed
up/exec/workspace write-through. After native Forget, a fresh alias pull failed
authentication while the registry remained running. Exact credential metadata,
private configuration and nonce/full-ID resource cleanup restored all three
engines' inventories and container states. Receipt:
`artifacts/qualification/orbwin-private-client-673bf627f21145ac8b8e02283e4da844/state.json`
and its `native-credentials.json` (`success:true`, `cleaned:true`, no errors).
No unrelated vault values were read or raw configuration snapshots persisted.

The pinned registry dependency was acquired separately with an isolated anonymous
Docker config and helper-free child PATH. Exact digest/platform/config/rootfs and
reported image size are recorded in
`artifacts/qualification/p2-registry-acquire-df5b70a8-5a75-4b6e-9b2b-28f41ab42f47/proof.json`.
Its new cached image is intentionally retained; actual wire bytes were not
measured. The client harness itself performed no dependency acquisition. Global
Docker/kube/WSL, project and private config hashes remained unchanged with no
Windows elevation (`p2-private-clients-retention-20261025.json`). Ordinary-user
WSL private pulls, Compose private-base builds, native editor and installed
qualification remain separate D09 gates. This HTTP loopback fixture does not
qualify enterprise TLS/proxy registry behavior.

The subsequent CLI cancellation fix stops additional observations after a
cancelled service-start/readiness call and reports acceptance as unconfirmed
after failed state changes; three behavioral tests and Go tests/vet pass.
PowerShell 7 session completion passed ten positive and five negative real
TabExpansion2 cases, including cursor-in-middle and quoted command words.
PowerShell 5.1 policy blocked execution and was left unchanged. Root rebuilt the
CLI (SHA-256 `a3acc2919dcfe508a1e9fc0a0d153ad5a695aa85a7cf762ea39458836fcd3b02`)
and restaged the checksum-verified package with the patched Windows/Linux Compose
candidates and 775 resolved dependency notices. The earlier installer does not
contain these later changes. Serial Rust verification passes 72 tests, with
28 explicit live/fixture tests ignored; the private-client test was separately
run as described above. No new installed acceptance is claimed.

The ordinary-user WSL extension subsequently passed with current CLI
`a3acc2919dcfe508a1e9fc0a0d153ad5a695aa85a7cf762ea39458836fcd3b02`:
`artifacts/qualification/orbwin-private-client-41bd5c64621849ef9aae8ac4bd2299fd/state.json`.
Ubuntu was selected by its existing registration GUID and explicit UID1000 user,
with the fixed owned Unix endpoint and a child-only clean environment/helper
PATH/private config. The uncached pull matched the independent manifest,
config/rootfs/platform and nonce; after Forget, Windows and ordinary WSL pulls
both failed authentication while the registry remained running. Ordinary-user
global Docker config, both private configs and all previous engine/global
baselines were preserved, with `success:true`, `cleaned:true`, and no errors.
This closes that specific WSL interoperability proof, not the Compose private
base, native editor or installed gates.

The generic form submit guard now refuses Enter after a terminal result. A local
validation failure can explicitly return to review; any request dispatched to
the backend stays terminal because failure/cancellation may follow acceptance.
Cancel remains pending with stopping feedback until acknowledgement. Native
terminal-Enter, stopping and focus behavior still require interaction checks.

Kubernetes port forwards now present an exact TCP address and an explicit
HTTP/HTTPS choice, rather than assuming every service is HTTP. The native
opener constructs only bounded numeric-loopback HTTP(S) URLs. TLS passes through
unchanged; no certificate/trust bypass or automatic cluster-name signer is
introduced. Strict anchored readiness parsing refuses malformed/conflicting
port records. See `architecture/kubernetes-access.md` for the existing forwarding
evidence, supported adaptation and remaining native/installed gates. Integrated
JavaScript tests now pass 111 cases; the new native opener test also passes.

The integrated installer was successfully rebuilt and preserved at
`artifacts/qualification/p2-parallel-candidate-3972d31086bd-20261025/OrbWin_0.1.0_x64-setup.exe`
(492,740,939 bytes; SHA-256
`3972d31086bd0879355199bb0368dfd6a4853fdbd27a73b332fb63ad9de263fd`).
Its SDK is `047fab5a7f13465715db0a69ccd45dae3823f767ec56dfd575994ee1c488c8a9`
and CLI is `a3acc2919dcfe508a1e9fc0a0d153ad5a695aa85a7cf762ea39458836fcd3b02`.
It includes patched Compose, WSL project tools, terminal form guard and explicit
forward protocols. It is uninstalled; later isolated LAN/CLI investigations do
not establish behavior of this candidate. An earlier packaging attempt correctly
failed on the staged alias being busy during the client proof; after that proof
completed, the package stage and full build succeeded.

Native development review submitted an invalid leading-dash image, observed
backend preflight refusal, changed the image while retaining an invalid CPU limit
and pressed Enter again. The distinct original error remained after settling,
with no new pending state or Run button. This qualifies that terminal-error
replay guard, not success-terminal or in-flight cancellation. No Docker mutation
was intentionally dispatched. The development app was closed and the installed
app remained open; engine/config identities were preserved without elevation.
Receipts: `p2-terminal-form-enter-native-20261025.json` and
`p2-terminal-form-retention-20261025.json` under `artifacts/qualification/`.

## Local network diagnostics — 2026-10-25

The Diagnostics page now explicitly runs fixed local-name and loopback checks
through the source/binary-verified SDK without requiring engine state. The helper
owns temporary IPv4/IPv6 listeners, verifies server-first random nonces, accepts
no hostname/port input and closes/joins its work. Output reports address-family
order and guarded IPv4 fallback rather than local addresses, ports or nonces.
Windows proxy observation reports configuration presence/mode, never URLs, PAC
contents, credentials or bypass entries; effective proxy behavior is not
evaluated. Native calls have a process wall-clock cap. Result parsing is bounded
to 16 KiB with strict sources/schema/enums/known fixed names; the Tauri slot and
UI prevent overlapping hidden probes. Cancel discards completion while the
bounded check settles. No result is automatically added to support export.

Actual SDK and native development UI checks passed: both fixed names resolved
IPv6 first, both literal families and guarded IPv4 fallback verified the owned
listeners, and both proxy sources reported direct configuration. The UI showed
loading and the rendered results. A cancellation click arrived after completion;
no native cancellation pass is claimed. Receipts:
`artifacts/qualification/p2-network-diagnostic-sdk-20261025.json` and
`artifacts/qualification/p2-network-diagnostic-native-20261025.json`.
Focused strict-parser tests, Go unit/vet/race checks and TypeScript pass.
Browser/workload/VPN/split-DNS/proxy-transaction/installed gates remain open; this
probe does not establish them. The preserved installer above predates this
source/SDK addition. See `architecture/network-diagnostics.md`.

The first opted-in private Compose attempt stopped in preflight before recording
a Compose stage or issuing a build. Windows/WSL positive private pulls repeated
successfully; native credential/config cleanup and all engine inventory checks
passed (`cleaned:true`). Failure receipt:
`artifacts/qualification/orbwin-private-client-7d2052e0632a4b04abc356afe0b7fdca/state.json`.
Default discovered plugin versions and hashes were independently verified;
the harness preflight failure remains under investigation. No Compose private
build success is claimed from that attempt.

Read-only reproduction found a harness path-normalization defect for Docker's
extended Windows plugin path; the corrected preflight passes and the failed
receipt remains. The next attempt's actual default-plugin Compose build exited
zero, but the required positive registry-request counter did not advance, so the
fixture correctly stopped before service startup and negative-build testing.
Receipt: `orbwin-private-client-51d1c085e0404f4e8d03b05c4c64fde1/state.json`.
Exact engine/config/credential cleanup passed. BuildKit du/history increased
31/25 to 34/26; cache/history and ref/activity residue are retained and recorded.
This is a successful build command, not a qualified private Compose journey;
request-observation instrumentation remains under investigation.

After integrating local network diagnostics, the ordinary verification passes
119 JavaScript tests, TypeScript checking and 76 serial Rust tests (28 explicit
live/fixture tests ignored). The new SDK SHA-256 is
`c56065178dd849d4825aa30d8a034cf756443336c78bbafb5f43db5b01754c86`.
The SDK build ran its Go package suite, including network diagnostics. Native
diagnostic retention rechecked the three engine IDs and private/project/global
hashes without elevation (`p2-network-diagnostic-retention-20261025.json`).

## Default-plugin private Compose and browser loopback — 2026-10-25

The corrected registry observer recognizes Distribution 3 combined access logs,
structured logs and known manifest-digest/query forms. Its counts are selected
log observations, not unique HTTP requests. The subsequent full run passed with
the current CLI/alias SHA-256
`7ccb4f47ac34eca54c8b34aad42e2a9e541523be651f17d875d39de2ab17504f`:
`artifacts/qualification/orbwin-private-client-92442bb9b2184d98a887b9ef2395d7db/state.json`.
Actual default plugin discovery selected pinned patched Compose and Buildx.
Forced private FROM build observed authenticated manifest access; the bounded
service returned its expected marker with verified limits. After native Forget,
Windows/ordinary WSL pulls and the forced Compose build refused authentication.
Exact resource/credential/config cleanup restored all three engine inventories
and states (`success:true`, `cleaned:true`, no errors). BuildKit cache grew from
34 to 38 records and history 26 to 28 (one completed and one expected failed
build); cache/ref/activity residue is explicitly retained without pruning.
This qualifies plain Windows Compose private-base use. Dev Containers Compose,
WSL Compose, interactive editor and installed gates remain distinct.

The CLI now requires a strict typed confirmed-stopped observation before any
service-start fallback after a settings-read failure. Warm legacy settings reads
remain usable. The current backend does not yet implement that machine interface,
so cold CLI startup remains unsupported with explicit desktop guidance. The
isolated upstream candidate has full package tests/vet/build and disposable-home
JSON evidence, but is unregistered/uninstalled and has not addressed a running
service. No generic error, authentication failure or malformed settings response
authorizes automatic startup. Current CLI read-only qualification passed at
`artifacts/cli/qualification-1ea1de67-4859-43b5-99b7-66851dc24297/proof.json`.

Actual Windows Chrome reached the owned IPv4-only published fixture through both
`localhost` and `network-check.orbwin.localhost`, displaying the exact nonce in
both documents. The finite readonly container logged only the two expected
requests; the temporary browser tab was closed and exact fixture cleanup restored
all three engines' inventories/states and client/global hashes. Receipt:
`artifacts/qualification/orbwin-browser-96f8a7d917e54cedaad1709b176eefec/proof.json`.
The installed Chrome executable reports 155.0.8059.85; the browser's running
version was not independently read. OS probes before and after reported IPv6
first, but Chrome's internal attempt order/timing was not captured. This is a
real browser outcome against the Windows IPv4 publisher, not proof of browser
fallback internals or the whole D01 matrix. No elevation or network settings
changes were used (`p2-browser-loopback-retention-20261025.json`).

## Migration accounting integration and advisory scans — 2026-10-25

The derived project receipt panel now reads image and volume histories together
and requires both response targets to match the reviewed destination. It matches
observed immutable source identities and existing provenance links, without
creating migration job state or selecting a latest transaction. Integration passes
135 JavaScript tests, TypeScript and all SDK package tests. Actual rebuilt SDK
reads returned one image-export and five source-volume receipts bound to the
requested target; local network diagnostics also completed. Receipt:
`artifacts/qualification/p2-migration-accounting-sdk-20261025.json`.
Durable draft/mapping reopen and installed/native panel qualification remain open.

Exact isolated frontend and Rust lock scans completed. npm reports zero known
advisories; RustSec reports zero vulnerabilities and two informational warnings,
both absent from the current Windows dependency graph. The Go binary scan found
SDK networking dependency matches; the SDK was updated narrowly to x/net 0.60.0
and its required dependencies, rebuilt, tested and vetted. Repeat binary scanning
reports zero matches for SDK SHA
`8f118fb18b233c70a6cf7c9d2284de7c3b02fca90f6be09395cdd9943cfdb8d5`.
Package staging passes with 772 dependency notices and none missing. The current
installer remains an older uninstalled milestone; it does not contain these bits.

Upstream rdd, Buildx and patched Compose still have advisory matches requiring
applicability/fix qualification. JSON scanner status zero is not acceptance, and
module/symbol matches are distinct from runtime exploitability. The full security
gate, installed correspondence, guest and platform assessment remain open. Exact
tool/database/binary hashes, findings, limitations and primary advisory links are
in `security-response.md`; scans used isolated configuration and changed no
running engines or global settings.

The first dedicated actual Dev Containers Compose attempt passed its default
plugin private-base build/up, generated outside-context Dockerfile verification,
workspace roundtrip and same-ID reopen. After native Forget it stopped at a
harness precondition before negative build: the real client had materialized
the private base reference, which the original positive cleanup had not removed.
Receipt `orbwin-private-client-ae9d3ef2485b4495b7db40389101c581/state.json`
is retained as a failed overall attempt, with exact cleanup and all three
baseline snapshots restored. A guarded nonce-base removal and hostile ownership
test now address that harness gap; negative authentication still requires a new
run. Neither an overall pass nor an installed/editor journey is claimed.

The isolated native Auto proxy candidate now has a real local PAC ordered-result
and cancellation milestone. Its decision-only documentation-address target
returned HTTP proxy then DIRECT; a separate request-observed cancellation returned
no routes, observed both successful native closes and final callbacks, retired
all pending operations, joined its server and reaped its child. Exact proofs:
`p2-local-pac-9a3cb3f3c1a2450196a8460625f26953/` and
`p2-local-pac-cancel-58926fa6366740d68401e00cdfa6f6b0/` under qualification.
Two earlier loopback-target attempts returned DIRECT and remain failed
ordered-result proofs. No user proxy/PAC configuration, WPAD or engine state was
changed; no proxy/target connection was made. OS-wide egress was not enforced or
claimed. Hosting, configuration delivery, guest transaction, other native cases
and production activation remain open; the candidate is still unregistered.

The subsequent dedicated Dev Containers run passed:
`orbwin-private-client-d96d1b87971b4271a0618730e18a2c76/state.json`, success/cleaned
true, no errors, exact three-engine baseline equality. Positive actual default
Compose private-base build/up, generated outside-context file, workspace writes
and same-ID reopen passed. After native Forget, Dev Containers itself refused
the exact private source pull before reaching Compose build; manifest refusal
observations increased without new authenticated success, with no output/container.
This is early client authentication refusal, not a negative Compose build; the
earlier plain Compose proof separately covers that build path. The two failed
dedicated attempts remain recorded. Cache/history advanced 40/30 to 41/31 and
are retained without pruning. A hash-checked frozen native credential-test binary
was used while unrelated Rust changes continued; its receipt makes no claim
about those newer sources. WSL Compose/editor/installed gates remain separate.

Windows migration draft reopening is implemented through the existing remembered
Compose-input owner, with opt-in provenance hints and the actual native-written
mapping-byte SHA. Fresh source/target/member/image/mount review and strict native
mapping-byte/semantic checks precede separately confirmed Validate/Up. No transfer
state or new ledger is persisted. Integrated checks pass 146 JavaScript tests,
TypeScript and 78 serial Rust tests (28 live/fixture tests ignored). WSL draft
continuity is being added separately. Its frozen Linux pure mapping tests passed
as the explicitly selected ordinary Ubuntu UID1000 inside a new private temp
directory, which was cleaned: `p2-wsl-mapping-pure-x4YP4J/proof.json`. Those tests
did not invoke Docker/Compose, project workflows or engine lifecycle operations.

The WSL input continuity increment is now integrated: explicit tagged records
reuse the input store and connection authority, and repeated guest mapping-byte
hash/semantic checks precede the existing Compose boundary. Root rebuilt the
source-attested project helper to SHA
`71e6c1db70d044d0e826a84c7e10b5b7227d1987dbd113ed8a1bb5500ddde726`
and staged it with SDK 8f118f…; 772 notices remain fully resolved. Verification
passes 154 JavaScript tests, 80 serial Rust tests (28 explicit fixture tests
ignored), all Go package tests, TypeScript, production frontend and native debug
builds. Existing frontend bundle-size/chunk warnings remain performance-review
work, not build failures.

Actual current native review read the explicitly selected Desktop source, selected
its observed project using the keyboard, showed both-history loading and then
zero matching image/volume exports with the historical/incomplete-evidence limits.
No project completion was inferred. Escape dismissed the source review; the
temporary development app was closed, preserving the original installed window.
Receipt `p2-migration-accounting-native-20261025.json` excludes raw project names
and asset IDs. The corresponding retention check preserves all three engine IDs
and known configuration hashes without elevation. This read-only milestone does
not qualify native saved-draft restart/reopen/Validate/Up or asset/data transfer.

The isolated proxy increment now contains actual App types, generated CRD and
deepcopy, existing-admission integration, proxy-only revision/digest planning and
separate status-owner helpers. Package tests, generated-schema validation, Windows
race and vet pass in `artifacts/patch-replay/engine-proxy-schema-candidate/`.
Its exact baseline records the current commit and 12 registered patches. It has
no active Reconcile/hostagent/guest integration or registered runtime patch;
the bounded generation owner and authenticated delivery remain implementation
work. No real proxy, PAC/WPAD, host network or engine configuration was changed.

Security checks now retain complete frontend/Rust lock scans and exact Go binary
extraction/scan correspondence; see `docs/security-response.md` for findings and
coverage gaps. The final-version Compose 5.6.1-orbwin.2 candidate incorporates
the reviewed HTTP/2 dependency floor, passes focused Windows and Linux tests and
matches its same-host repeat build byte for byte. Its binaries, source patches,
compiler and 110/108 compiled-module legal inventories are attested. Buildx
adoption is being prepared independently. Active runtime pins and installed bits
remain the prior versions pending combined staging and runtime requalification.

The new bounded Windows migration-draft runner uses a frozen current native test
adapter to exercise production inspection/Validate with actual input-store bytes.
Two preflight attempts are retained: `orbwin-draft-e2a234774c984a6d82d00005108317e5`
failed on expected absent `.wslconfig`; `orbwin-draft-d696452aa20347f49cc589fe2bdb98fe`
failed because its pinned Alpine was absent in Desktop. Neither created engine
fixtures. The second raw baseline mismatch was independently reduced to varying
mount enumeration order; canonicalizing that order yields equal baselines. The
runner is being corrected to preserve explicit absence and unordered inspection
semantics and to use an already-cached common immutable fixture image. These
failed harness preparations are not product qualification passes. The independent
retention receipt preserves the three IDs/configuration hashes without elevation.

The first fully created Windows fixture found a real product defect:
`inspect_migration_mapping` reused the save-new path check and refused every
existing mapping. The existing-file inspection path now resolves only the parent,
preserves the final reparse check, and retains bounded SHA/semantic inspection and
Windows reader sharing. Save still refuses existing destinations. The regression
traverses that actual inspection seam, including final symlink, relative/UNC/ADS
refusal; all four mapping tests pass. The failed `7dc70648276f4d539c973f34aa9af5f7`
fixture was cleaned with equal baselines.

The corrected current-native run
`orbwin-draft-3c1cd8735a0c48ce87ec25f814d311ed/proof.json` passes all seven stages:
three production Validate calls (initial, input-store roundtrip, restored mapping),
changed-byte refusal before Compose dispatch, missing-source refusal and
pre-read cancellation. All fixtures were removed; complete canonical three-engine,
config and original WSL baselines match, with a separate non-elevated retention
receipt. This uses the reviewed cached Redis immutable image without pulling,
tagging or starting a container. The receipt binds the fixed production source
and current frozen native binary. Actual Save-dialog and WebView/app restart,
Up/data use, and WSL draft execution remain distinct open gates. Integrated checks
now pass 168 JavaScript tests and 80 serial Rust tests (29 explicit fixture tests
ignored), including the new packaging provenance and harness guard tests.

The development runtime and package now adopt Compose `v5.6.1-orbwin.2` and
Buildx `v0.38.1-orbwin.1`; installed binaries and engines remain unchanged. Both
platform candidates pass exact source/compiler/module/legal verification and
same-host repeat builds. The runtime lock hash is
`37528c5f2505535c07bb90f65347aca6576be9be7d038b7b091aef85adc7a0e1`.
Final exact-binary scans retain actual extraction/scan hashes, remove the five
HTTP/2 advisory matches and preserve the remaining findings/coverage limits in
`docs/security-response.md`. Packaging still resolves all 772 dependency notices.
Read-only pin-drift and legal-copy checks preserve the staged cache. An initial
Compose guard still selected the first local source after Buildx became local;
it was corrected to select Compose explicitly and its repeat passes.

The real default-client Dev Containers private-Compose proof passes again with
these exact Windows plugins:
`orbwin-private-client-6592a5c878a14ca9b6879019ba0f3d2b/state.json`.
Authenticated build/up, generated Dockerfile outside context, workspace writes,
same-container reopen and post-Forget early source-pull refusal all pass; cleanup
restores equal three-engine inventories/configs without pruning. Source/native
attestation is attached to the receipt. Earlier `dfa8612e5df942a9b0797ac9e3c83c4e`
failed at an obsolete harness version check and was cleaned. The pin check now
admits the explicitly reviewed Compose revision and normalizes only an optional
leading `v` in plugin version metadata, with a regression that still refuses a
different version.

WSL draft-runner source is prepared and independently reviewed, including exact
original mapping inode checks before edits and conservative partial-create
retention. Its live run remains gated: shared OrbWin WSL private configuration
points to the older installed plugin set. Current development pins correctly
refuse it, and reconnect would also rewrite project records. A separate explicit
client-only refresh is being implemented rather than changing exports or
credentials during a migration fixture. No WSL fixture files or project records
were changed for this preparation.

The isolated proxy now also has bounded policy generations/stream leases and a
strict authenticated management-client constructor with local TLS contract
tests. New v2/v3 race/vet/unit receipts preserve the schema candidate's earlier
evidence. Windows ownership/ACL reading and actual hostagent/controller/guest
integration remain implementation gates; these source increments do not activate
proxy settings. A separate rdd host HTTP/2 floor source patch changes only x/net
0.59.0 to 0.60.0; existing pure suites, vet and eight actual HTTP/2 tests pass.
Preserving exact accepted embedded guest assets is under review for an isolated
native host build, with unchanged guest dependency findings explicitly retained.

## Private WSL client refresh and current parallel work — 2026-10-25

The explicit client-only refresh is implemented and qualified without elevation.
`p2-wsl-client-refresh-4e35f0dc318c43e58e5f69094d243a21/proof.json`
binds the current native adapter, runtime lock and all source hashes. Only the
private WSL plugin-directory value changed; every unrelated raw config byte,
owner/DACL, original WSL entry, project record, global configuration and canonical
three-engine inventory was preserved. The exact original is retained in a private
backup. A second refresh is a no-op. The private WSL config now has SHA-256
`77c380f608131ab1a9ea7710961205ca9171b65cf6d42f612371b478051a4f50`.
Earlier retention receipts with the previous hash describe historical state.

The current development UI exposed the refresh region and disabled its button
during dispatch, then reported “WSL client tools are already current.” The
no-op left the config and sole original backup unchanged. The temporary window
was closed while the installed window remained open; receipt
`p2-wsl-client-refresh-ui-20261025.json` records the exact application/source hashes.
This single viewport check does not close the keyboard, scaling, accessibility
or installed-upgrade matrix. See `architecture/wsl-client-refresh.md` for ownership,
uncertain-commit recovery and the shared-config cross-version limitation.
Current integrated checks pass 180 JavaScript tests and 91 serial Rust tests
(30 explicit native fixtures ignored), plus TypeScript/frontend and native debug
builds. The current installer remains stale.

The first WSL draft attempt `orbwin-draft-f1c9e7906b7e42c9bcf685e27c0b0034`
stopped during its initial read-only snapshot when an engine network disappeared
between list and inspect. Its only intent created the anonymous artifact config;
no guest or engine fixture was created. Its missing initial baseline also caused
a secondary comparison diagnostic, which is now explicitly labelled unqualified
rather than comparing a final snapshot to an absent baseline. The original failed
receipt is preserved. The fresh nonce
`orbwin-draft-b7251f975a6f46fb92285ab0faaf59e1/proof.json` passes all seven
ordered stages, including three actual production WSL Validate calls, changed
mapping refusal in the guest before configuration resolution, restored validation,
stale-source refusal before native dispatch and pre-read cancellation. Its 155
recorded subprocess steps completed in about 142 seconds. Exact fixture cleanup
restores all three canonical engine baselines, configs, project record and nine
original WSL entries (1,050 file bytes). `run-source-attestation-v2.json` binds
the runner/models and verifies all 44 frozen-native source/manifest entries;
the earlier sidecar's array-shaped count is retained and explicitly corrected.
Actual Save-helper/dialog, WebView/app restart and Up/data use remain separate
acceptance gates; serialized input reloading does not close those gates.

The isolated proxy candidate's metadata-only observation identified an inherited
read-data allow entry on its management config; no foreign-token access or
cross-user exploit was demonstrated. The proposed fix creates the private
instance anchor and config with protected security descriptors at creation,
rather than relying on Unix mode bits on Windows. Fresh-fixture implementation
is proceeding separately; live credentials, DACLs and APIs remain untouched.
The separate LAN provider work implements only bounded local schema reads, with
compiled Windows SDK ABI evidence. Actual provider queries, exclusive rule-key
semantics and any firewall mutation remain separate gates.

The subsequent exact read-only LAN MI probe passes in 170 ms without elevation:
`p2-lan-mi-native-df0cb12a-9e9a-4026-b035-6be71b7ff30c/proof.json`.
It uses only fixed local GetClass with a five-second context, ten-second test and
external fifteen-second process deadline. The observed wfascim schema has the
expected 39 properties/five methods, with activation and atomic-create flags
false; all 43 frozen source/guard files match. No rule instance, associated filter
or mutation method was queried. InstanceID is readonly but not a CIM key; safe
exclusive creation/full-key removal remains unproven and disabled.

A fresh per-user installer build is retained at
`artifacts/qualification/p2-current-client-refresh-installer-20261025/`:
495,071,180 bytes, SHA-256
`68ab996166f737db2d66814d572aaf9b0bb88783fe6c6db7891b2af2594ec190`.
It includes the current client pins and refresh UI/command, resolves 772 notices,
and passes the frontend/native release build. It has not been installed or
accepted. Proxy/LAN candidate source remains outside this packaged runtime.
The already-recorded frontend bundle/chunk warnings remain performance work.

## Migration native UI and dependency integration — 2026-10-25 Pacific

Windows migration now has actual native Save, explicit remember, normal app
close/reopen and reopened Validate evidence in the isolated qualification profile.
The GUI refused changed mapping bytes, passed after exact restoration, and
refused the removed original source. Fixture cleanup completed after retaining
failed preflights, restoring the qualification-selected private Windows plugin
path to its exact original bytes, and separately acknowledging an unattributed empty
Docker Desktop bridge ID change. No network was repaired or workload started.
See `architecture/migration-draft-qualification.md` for exact receipts and the
limitation on full original network-identity preservation. WSL native Save/app
restart were open at this record; subsequent evidence follows below. Up/data use
and installed qualification remain open.

Reviewed CLI and credential-helper modules now use x/sys 0.48.0. Existing recipes
reproduce the isolated, symbol-audited candidates: CLI/alias `230b4d99…` and helper
`28fc26a1…`. Active-source CLI/helper tests, vet and module verification pass;
the real vault test remains explicitly skipped. Their manifests and dependency
licenses were regenerated. A separate recipe fix avoids inheriting module-cache
ReadOnly attributes in generated licenses; four guard tests and two actual repeat
builds pass without elevation or changing cache permissions.

Migration presentation now uses bounded friendly saved-project labels, expandable
exact provenance, concise Validate descriptions with full review details, and an
explicit Validate primary action. Its four focused rendering/spec tests passed;
the agent's full JavaScript suite passed 192 tests before the later recipe and
snapshot regressions were added. The integrated Rust suite passes 91 tests with
30 native fixtures ignored. Actual rendered review of the updated frontend is
still pending. The new retained installer at
`artifacts/qualification/p2-xsys-migration-ux-installer-20261025/` has SHA-256
`161852d0a89ef6a219dac5e42cb52b877ebae97560024954d280778470ecf7db`
(495,076,545 bytes); TypeScript/Vite, Rust release and NSIS builds passed, with
771 dependency notices and zero unresolved notices. It is not installed or
accepted. Isolated LAN/proxy work remains outside this runtime.

The actual read-only LAN MI filter-class probe observes all fourteen fixed public
schemas. An initial exact rule read refused instance flags; a diagnostic-only
retry identified declaration flags plus documented NULL/NOT_MODIFIED field state.
The constrained implementation compares every immutable property flag to a fresh
qualified class declaration and still refuses null identity, type/key/unknown-bit
drift. Exact executable `17e86266…` passes the fixed selector and two four-key
GetInstance rounds in the retained `p2-lan-rule-keys-declaration-native-18edf0b5-e88d-4c86-8b83-cc74f24c5817`
receipt. Private values stay in RAM. This does not establish mutation ownership,
an atomic snapshot or activation; seven-filter traversal remains under source
implementation. Proxy protected-file and held-directory proofs likewise do not
close live selected-instance authentication, controller/hostagent/guest wiring or
the supported proxy matrix.

## Parallel integration evidence — 2026-10-25 Pacific, later run

WSL migration now has actual native Save, explicit remember, normal app close/
process exit/reopen and successful reopened Validate in the same isolated profile.
Frozen app `acf2d114…` also provides limited rendered review of the updated saved
project labels, provenance disclosure and Validate action. Exact cleanup removed
the stopped source, two empty volumes and new guest files. The final read-only
review `p2-migration-ux-ui-49eb8d862bf44b50bb5f7593ba6d718a/wsl-final-ui-cleanup-proof.json`
(`1b9716b1…`) establishes complete original baseline equality except the reviewed,
unattributed Desktop empty default-bridge ID change. The original cleanup failure
receipt remains intact: its final direct comparison omitted that exception.
No network repair, workload start, Up or application-data use was performed.

Read-only LAN traversal now passes all seven filter families in both directions
over two passes, with three full typed rule reads (3,516 ms), using exact binary
`fe5e0e79…`. The preceding fourteen-schema probe passes in 205 ms. Receipts are
`p2-lan-filter-traversal-native-b009a985-d815-4e36-8014-37210765e7c4/`
and `p2-lan-filter-traversal-schema-native-d3585947-c312-4da1-92ab-3d70d630a58f/`.
These are actual read-only observations; ownership, activation, atomic snapshot
and rule mutation remain unqualified. A disabled BLOCK create/duplicate fixture
is under source preparation only. WFP remains an evaluated alternative, not an
adopted replacement for scoped firewall consent.

The integrated JavaScript suite passes 204 tests before the desktop additions.
New tray, explicit per-user sign-in and exact owned uninstall cleanup compile;
the latest serial nonignored Rust suite passes 99 tests with 30 native fixtures
ignored. Initial compile failure and corrected retry are retained. These desktop
results are source/build evidence only: no Run value was changed, and actual tray,
sign-in and installer acceptance remain open. See `architecture/desktop-convenience.md`.
The existing retained installer predates these additions and is not current
desktop qualification. Isolated proxy hostagent integration remains outside the
active runtime pending review and remaining lifecycle/guest/native proofs.

Desktop integration subsequently passes 213 JavaScript tests and two package
preparations with 772 notices and no unresolved licenses. The isolated release
build `15e0c151…` now has actual manual UI, read-only sign-in preference, visible
keyboard focus, normal window-close and background window-inventory evidence.
All three canonical engine inventories and five configuration records match
exactly after cleanup. Actual tray Open/Quit remain open because the native tool
does not expose a notification-area target; background test cleanup is explicitly
forced disposal of only that verified UI process, not Quit acceptance. No startup
toggle was submitted. See `verification/phase-2-desktop-ui-2026-10-25.md`.

The subsequent isolated native registry fixture fails with OS error 6801 at its
first creation step. Both disposable nonce and Qualification parent are absent
on read-only audit; existing parent content is retained. This prevents claiming
working sign-in mutation and requires reviewing uninstall cleanup before any
installation. No transaction-resource-manager repair, elevation, ACL edit or
silent fallback was attempted. The selected per-user mechanism remains active
implementation work, with failed evidence preserved.

That API blocker is now resolved in source: ordinary fixed HKCU APIs replace KTM,
with bounded cross-session SID locking, immediate ownership recheck/readback,
no replay/rollback, and the documented 260-unit UTF-16 command limit. Cooperating
copies serialize; trusted external same-user writers must coordinate. The first
ordinary fixture's malformed oversize input refused and was exactly audited/
cleaned; a valid terminated oversize fixture then passes all ten native stages in
429 ms, including child-process contention and abandonment, with exact disposable
key cleanup. Run proof `p2-desktop-oversize-fixture-2054b1fd-ef93-496b-8638-dae20c4c0de2/run-proof.json`
is `8a888026…`. No real Run entry or admin setting changed. Real sign-in and
installed qualification remain open; prior transactional installers are stale.

## Typed status integration and migration adapter bridge — 2026-10-25 Pacific

The reviewed typed service-status patch is now registered as patch 0013. Active
CLI and explicitly targeted Docker alias build identically to the isolated
comparison (`f624bc96a97d5e808453e35d01dac50f531382729e94f323934d40dcf240f38b`).
All 34 CLI top-level tests, vet, seven focused upstream tests, package vet and a
clean 13-patch replay pass. See `architecture/cli.md` and the frozen proof in
`artifacts/qualification/p2-cli-typed-adoption-c2891d70-a4aa-4543-bee2-8db85ca56d83/`.
The running backend remains the qualified 12-patch `0896047d…` build. Source
adoption is not a runtime upgrade or cold-start acceptance.

Review found the Rust backend attestation still embedded only twelve patches.
It now includes 0013 and requires the complete ordered source lock, embedded
patch bytes and bundled manifest to agree. The regression rejects omitted,
reordered, duplicated and altered patches. New app builds therefore refuse the
old backend bundle; JavaScript qualification's existing `verifyBuild` also
refuses that mismatch. No guard was relaxed, backend replaced, identity record
rewritten or engine restarted. A matching backend and subsequent qualified
transition are required before live tests of the new integrated source.

The new ignored `migration_application_qualification.rs` bridge admits only a
reviewed nonce fixture's hash-bound base/mapping files and one actual Rust
Compose Validate, Up or Stop operation. It fixes the project, directory and
target, journals dispatch intent, retains the native operation outcome, rechecks
files/daemon and refuses arbitrary requests or Down/Pull. The external fixture
owner retains responsibility for application-data checks and exact cleanup;
missing/failed response after Up is an unknown outcome, never permission to
repeat Up. This prepares actual adapter evidence beyond the existing direct
Compose CLI proof. The bridge has not been executed. Full serial nonignored Rust
tests pass: 99 passed, 33 ignored. Neither bridge compilation nor these tests
qualify the UI, installed product or migration application startup.

After the read-only startup-admission correction described in
`architecture/desktop-convenience.md`, the complete serial ordinary Rust suite
again passes 99 tests with 33 live fixtures ignored. The source hashes remain
stable across that run. Log and receipt:
`artifacts/qualification/p2-runtime-adapter-review-5190d4b4d183461e8e73d46527bb6927/`.
The receipt also verifies the active backend is still `0896047d…`; it records no
live application-bridge execution or installed acceptance.

A matching 13-patch, `with_distro` host candidate now compiles in
`artifacts/qualification/p2-rdd-typed13-full-8170c10b-a409-4530-a903-dc9fb169b905/`:
`rdd.exe` SHA-256
`ee5481002dba9b47684639e23ba8f3c4491f207d2864ac338e1b3369fac1adb2`.
All 27 preparation/build checks pass. Existing guest assets are reused with
independently verified complete byte-range correspondence to the accepted
backend; this is not a guest rebuild or a full production recipe rerun. The
candidate has never executed or replaced the active backend. Its host x/net
remains 0.59.0, so the separate HTTP/2 security-floor candidate and remaining
guest/advisory dispositions are still required integration work.

The session build and Rust runtime source catalogs now include the LAN worker,
lease, helper, firewall and datagram packages. The service-helper catalog also
includes the UDP command and its framing/watchdog implementation. Offline Go
dependency enumeration confirms coverage of every compiled local source for
both session targets (Windows: 13 packages/117 files; Linux: 12 packages/112
files) and the Linux service helper (9 files). The build/runtime catalog sets
agree, JavaScript syntax checking and `cargo check` pass. These are source and
compilation checks only: no bundled helper was rebuilt, no manifest was replaced,
and the dedicated LAN worker remains unregistered. Existing binaries do not
qualify these changes.

Packaging now checks the complete expected session/service-helper source sets,
not just entries present in an older manifest. Four temporary-file regression
tests pass for omitted LAN sources, newly added files, changed bytes, unexpected
paths and Go-ignored/test files; build and staging syntax checks pass. Session
build completion also rechecks the source set, detecting additions during build.
No helper build or installer staging was performed by these checks.

Patch0014 is now adopted into the active source lock through the reviewed
source-only transaction under
`artifacts/patch-replay/rdd-host-http2-typed13-adoption-4cc05416-cc92-4dd9-84dc-36d102d657ce/source-adoption-3faa7ca2-9680-4f62-9c91-d04fffcf2a67/`.
Only host module selection advances to x/net v0.60.0; nested guest modules are
unchanged. Complete14-patch replay passes from the pinned commit in
`artifacts/patch-replay-d2605252-8a23-419c-8dee-0afdc195ed71/`. Rust's ordered
patch byte catalog also includes0014. No runtime binary/manifest/identity, CLI,
private home or engine transition accompanied the source change. A final14
build, remaining advisory dispositions and installed/runtime qualification are
still required.

The corrected UDP helper's blocking inherited-stdio watchdog is now observed
in an ordinary UID1000 Ubuntu subprocess fixture:
`artifacts/qualification/p2-udp-pipe-f3f802fafd764ab5845dbcb256ebb689/`.
The exact selected test and its read/write/close/joined cases pass. The close
case terminates through its two-second guard; read/write deadlines terminate
only the dedicated child. Temporary files are removed and identities rechecked.
This uses anonymous pipes, with no network sockets, Docker, systemd, UI or
elevated action. It does not qualify actual guest UDP traffic, LAN listeners,
firewall rules or installed helper integration.

The LAN renderer now persists a denial-only review reference before activation,
keeps recovery controls independent of a stalled submission, and reconciles only
the exact runtime/review/digest and immutable operation mapping. A contradictory
revoked reply cannot erase a known consumed operation. Lost replies, stale ready
observations, changed publication/interface identities and malformed recovery
storage have focused regressions. Eight LAN model tests, four source-catalog
tests, TypeScript compilation and two current Rust LAN request-boundary tests
pass. Rendered interaction and native LAN acceptance remain open.

Setup now asks the known incoming backend for a bounded read-only authority
classification before any service lifecycle dispatch. Legacy or ambiguous
authority is refused before stopping the current service. Two current Rust
closed-protocol tests pass. This gate does not perform the required legacy
data-preserving migration; its transaction proposal is recorded in
`architecture/legacy-authority-upgrade.md` and remains implementation work.

Two reviewed fresh Windows temporary-directory fixtures now pass for the
private datastore generation and inherited serving-anchor reopen:
`artifacts/qualification/p2-private-generation-source-fc6c8fab-8393-4997-92e0-8d140c8fc433/native-run-0e835d12-07c1-4246-8f6a-8a3e4399d81d/proof.json`.
They exercise private creation, retained immutable material, refusal to release
a live generation, positive retirement and reopening the same authority while
an inherited handle remains alive. Source and binary hashes remain unchanged;
known fixture cleanup reports no error. No product state, actual datastore,
socket, engine, UI or elevation was involved. Full proxy/kine restart and
installed-product qualification remain open.

The additive LAN publication endpoint and its restart fence are now source
patches 0015/0016. Exact running-container inspection and actual listener epochs
are observed twice; ambiguous bindings, incomplete state and changed tuples
refuse. A separate public 128-bit owner identity changes with each serving
HTTP bridge, preventing repeated numeric counters from making an old lease
current after restart. Eight focused tests with 28 subcases, race and vet pass
using only in-memory transports. Existing v1 and ordinary Docker routes are
covered. Complete 16-patch replay and Rust's ordered attestation regression pass.
The source-only adoption receipts are under
`artifacts/patch-replay/lan-publication-v2-reviewed15/` and
`artifacts/patch-replay/lan-publication-v2-owner16/`; clean replay is
`artifacts/patch-replay-82575f4b-35df-4668-93c1-118ba7aba1f6/`.
The running backend/manifest remain `0896047d…`/`5ed5dea8…`; a coherent rebuilt
backend and actual stop/restart/recreate, LAN traffic and coexistence checks
remain required.

The reviewed private service-generation fixture also passes:
`artifacts/qualification/p2-service-generation-source-408b56fa-ab8e-4610-85fe-b2a0bcb1cd89/native-run-2a79468a-4fcd-482f-9c07-6ea4897ee450/proof.json`.
It reads/hashes the current test process image, publishes a fixed synthetic
config and generation receipt in a new private temporary authority, and refuses
cancelled or incoherent snapshots. No real service, socket, engine or elevation
is involved. It does not qualify a live backend restart or token refresh.

The build-side LAN asset-manifest utility now implements the fixed three-role
canonical JSON and domain-separated Ed25519 signature contract. It checks exact
paths/order, bounded sizes and hashes, canonical signature encoding and actual
asset bytes. It has no private-key file access, implicit/default key, or native
installation action. Three focused tests pass; the complete JavaScript suite
passes 235 tests after the LAN renderer, source-catalog and manifest changes.
The frontend production build also passes; Vite retains its mixed static/dynamic
`VolumeSize` import warning and main-chunk size warning. This is compilation,
not rendered interaction review or a measured resource-budget result.
A public-only synthetic Node signature vector is retained for Go verifier
interoperability. This does not establish a production signing-key policy,
provision a bundle or enable privileged LAN activation.

The closed inert Auto child fixture was reviewed and executed once:
`artifacts/qualification/p2-bounded-auto-process-source-20261026/native-run-fbd30bbe-ba9b-44cc-b287-7128d330e3ed/proof.private.json`.
It fails during startup in 60 ms without external timeout; its generic refusal
does not yet identify which image-policy/launch/handshake boundary failed.
The overlay contains no native WinHTTP/PAC implementation and performs no
engine, UI, ACL or elevation action. This is retained failure evidence, not
native Auto qualification; stage-specific diagnosis remains required.

The integrated private engine proxy is now source patch0017, after independent
core review and the observed-pending restart correction. Its 165-file catalog,
generated schema/deepcopy provenance, dependency/license closure and focused
tests/race/vet are retained under
`artifacts/patch-replay/proxy-integrated17-review-7cbb3277-3520-4497-9f0f-8d2439f19e29/`.
The guarded source adoption receipt is
`active-source-adoption-d0b73c3f-38d1-493a-b568-c51b0cd03fb7/proof.json`
within that directory. It preserves raw modified predecessors and records LF
normalization before exact apply, verifies every patched source and publishes
the source lock last. Clean17 replay passes in
`artifacts/patch-replay-a74bd1dd-5fd4-487c-bd46-bf3f515aac65/`; Rust's complete
ordered patch attestation regression also passes. Backend and SDK binaries and
manifests remain unchanged. Source adoption is not live proxy acceptance.

The Network settings pane now has original Windows engine-proxy controls instead
of only the historical limitation notice. The thin Rust adapter sends one fixed
App desired-state JSON patch through the existing owned API, testing UID,
resourceVersion and generation before the write. It passes private values only
through stdin, writes no status, starts no management service and never retries
an uncertain mutation. The UI requires explicit Docker-interruption review,
preserves drafts, fences reads across submissions, and displays Ready only for
a fresh current controller/guest/host acknowledgement (including the explicit
untouched-None case). Three native-adapter/model tests, four frontend model tests
and TypeScript compilation pass. Rendered/native interaction, actual proxy pulls
and builders, cancellation and installed acceptance remain unqualified.

The stage-specific inert Auto fixture now identifies the startup refusal as
the F: volume-root directory integrity policy, before any child start:
`artifacts/qualification/p2-bounded-auto-process-source-v2-20261026/native-run-4b833291-642c-49e4-8614-b05711b71a6c/proof.private.json`.
No WinHTTP/PAC code, engine, UI or elevation ran. A separate read-only native
KnownFolder_Profile ancestry observation passes the unchanged policy for all
three actual components:
`artifacts/qualification/p2-profile-root-integrity-source-v1-20261026/native-run-93168551-14a4-4f36-be25-8cf0611b84cc/proof.private.json`.
This establishes a usable observed ancestry without an AppData/capability ACE
exemption; it does not itself qualify a service or migration.

The frozen disposable executable-cache fixture also passes on that native
Profile ancestry:
`artifacts/qualification/p2-runtime-private-cache-source-v1-20261026/native-run-0032695c-88cf-437b-ae8b-515822822e64/proof.private.json`.
It creates only its fresh private nonce folder, copies its independently pinned
test image, flushes, hashes, publishes without replacement, reopens the same
native file identity and rechecks held handles and descriptors. It launches no
copied image, requests no elevation, touches no production cache or engine and
retains the nonce evidence. ABI and injected copy-failure tests accompany the
frozen source. This qualifies that base fixture, not the subsequent bundle or
detached-service integration.

The desktop source now embeds actual build-input hashes for rdd, the session
manager and its four fixed LAN bundle files. Runtime manifests cannot substitute
their own component digest for that compiled origin. Debug source-only builds
may omit inputs but refuse execution of unpinned components; release builds
require every input. `ORBWIN_COMPONENT_INPUT_DIR` may select an absolute isolated
build-input directory; it does not redirect runtime lookup. The private cache
uses image-digest generations for rdd and canonical-manifest-digest generations
for the complete session bundle. It never replaces an existing file or repairs
an existing ACL. Terminal/session owners retain cache leases through client
retirement; setup/update checks distinguish immutable cached code from packaged
resources being replaced.

Detached SDK startup is being connected to a positive native self-lease
acknowledgement: actual PID/creation/session and image/file identity, the pinned
manager and bundle digests, and held native ancestry/image/bundle handles. The
Rust boundary rejects missing, malformed, wrong-build or nonpositive handoffs
and compares a newly spawned service to its retained actual child handle. The
Go named-pipe client must authenticate its actual peer before sending requests.
These source checks do not yet qualify UI exit/reconnect or a live detached
service; all production binaries remain unchanged.

The inert expanded SDK cache/self/peer handoff now passes natively:
`artifacts/qualification/p2-sdk-self-peer-cache-source-v1-20261026/native-run-42157095-524c-417b-9591-dd47b68d62ed/proof.private.json`.
The frozen actual cache and SDKself code changes only the two fixed namespace
literals in each implementation. It tests private complete-bundle publication,
global cache reuse, continuous write-sharing refusal for all five files, actual
child PID/creation/session/file identity, native same-build pipe-peer admission
before request bytes, and wrong-path peer rejection with zero observed bytes.
All inert children positively exit within the bounded recipe. No production
cache, engine, UI, administrator, DPAPI or certificate-store action occurs.
Real service restart, UI reconnect and LAN activation remain unqualified.

The execution-origin closure now also independently pins credential and Linux
helper payloads, and caches Windows/Linux Docker toolsets, plugins, credentials
and the directly executed project helper. The selected eighteen build inputs
and embedded tool lock determine origin. Old public WSL plugin paths require
explicit reconnect. Strong bounded cache leases cover child/plugin paths without
repeated passive-poll byte hashing; native object/descriptor checks remain.

LAN source now includes dedicated helper ownership per native LAN lease, current
removal/absence-only settlement, and prior-owner cleanup with fresh review/op
IDs. Recovered rows never become Ready. Positive cross-owner retirement requires
the exact protected original runtime/review/digest/operation/lease mapping,
removed sequence five, fresh absence and old worker/namespace closure. The UI
distinguishes removal, settlement and retained cleanup and keeps unknowns fenced.
Source race/vet/cross-build records are retained under
`artifacts/qualification/p2-lan-prior-owner-cleanup-source-20261026/`;
native elevation/provider/traffic qualification remains pending.

Independent migration review found a crash-time startup gap before import
completion and a bare-PID reuse false refusal. The source successor is adding a
durable normal-startup fence and full process-generation comparisons; it is not
adopted or runtime-qualified. A separate source audit found missing native
Windows origin for local HTTPS/state/trust receipts (without claiming a
plaintext-key leak). The SDK is being moved to an opaque native private store
with an explicit legacy-state transition; guest home/disks and ROOT store entries
are retained. No implicit CA reset, receipt adoption or store change is allowed.

The opaque SDK private-state source and its production callsite integration are
now compile-ready. State transactions retain native record capabilities through
external trust decisions; legacy state and incomplete transactions refuse normal
startup. The desktop maps the closed SDK exit codes to distinct migration,
recovery and ownership errors. LAN confirmation now retains its issuing runtime
instance and refuses a replacement SDK even if the state revision is unchanged.
Current verification passes 247 JavaScript tests, the frontend build, and 117
Rust tests (33 attended qualification tests remain ignored).

The first complete signed qualification SDK bundle built successfully under
`artifacts/qualification/p2-private-state-sdk-bundle-20261026-a/`. The recipe
ran its Go tests, built the Linux helpers and Windows manager/helper/worker,
compiled the dialog resource, and verified the exact signed bytes plus complete
source/resource/recipe provenance. Its signing private key existed only in the
builder's memory. No resulting product executable was installed or launched;
selected runtime binaries remain unchanged. This is build evidence, not native
private-state, trust-store, LAN or migration acceptance.

An additional recovery gap remains explicit: a renderer pending record can
survive SDK death before helper persistence of the consumed review. Absence of a
protected native record cannot clear that fence. A positive durable issued-review
and dispatch-admission protocol is under review; unknown operations remain
fenced until their outcome can be established.

The separate thin Go CLI also needs the retained executable-origin integration:
its current hash-then-close loose Docker/rdd paths and public helper/plugin PATH
are outside the desktop's new fixed cache contract. The successor backend cannot
be treated as CLI-qualified until independent build pins, shared cache consumption
and client-tree lifetime have been connected. This is an explicit remaining
P2-D03a/P2-QUAL gate, not an exception to the executable-origin requirement.

Independent SDK-state review reproduced an aggregate retained-memory admission
bypass and an owner-close path that could release exclusion after an origin
precheck failure. Both fixes now pass the independent pure acceptance cases in
`artifacts/qualification/p2-sdkstate-independent-review-20261026/acceptance.json`.
The native store fixture has compiled into a fresh frozen qualification namespace;
it has not yet run. Its source and execution boundary are under review. The
managed cross-owner review-revocation DTO/model also passes negative tests;
backend durable issuance/consumption integration is in progress.

The third fresh native store fixture passes in 0.184 seconds:
`artifacts/qualification/p2-sdk-state-native-source-v3-20261026/native-run-e424110b-0257-46ea-a845-88e395711498/proof.private.json`.
The first two failures remain evidence. Bounded diagnostics isolated the failure
to native directory enumeration; explicit eight-byte backing fixes the ABI
contract, with the same synthetic Root/Guard test now passing actual private
creation, sole-owner exclusion, write-sharing denial, mutation, reopen and close.
The corresponding two enumeration buffers in the unadopted management successor
are corrected as well. No production state, CA, DPAPI, trust store, engine, WSL,
elevation, child or listener was involved in this fixture. Publisher exclusion,
cold import and trust-store acceptance remain open.

The managed native-state/current-user-DPAPI intersection now passes separately in
0.270 seconds:
`artifacts/qualification/p2-sdk-managed-authority-source-v1-20261026/native-run-5aa5d0e3-f264-4341-b078-7002de6d8d09/proof.private.json`.
Actual managed creation, positive close/reopen, unchanged synthetic CA identity,
usable constrained signer, absence of plaintext signing material, ciphertext
tamper refusal and no implicit CA replacement are exercised. No root-store
adapter, trust change, production state, engine, WSL, administrator, service,
listener or probe child is involved.

Durable LAN review admission is source-complete in
`artifacts/qualification/p2-lan-durable-review-source-20261026/review.json`:
private issuance precedes every successful review response; consumed admission
precedes stop/helper/native effects; a new exclusive owner can positively revoke
only an exact still-issued private receipt. Unknown/consumed records cannot be
revoked or replayed. The actor does not block on the native journal, late
generation ownership is joined, and race/fault/vet/build checks pass. The UI
accepts the distinct fresh `review-revoked` proof and retains known operation or
lease records. Native LAN/provider/elevation/traffic acceptance remains open.

The Go CLI source now consumes fixed private executable cache roles and uses
independent build pins. Its atomic transient runner uses native JOB_LIST and
HANDLE_LIST at process creation, actual returned handles and one-use retirement
proofs; source/race/ABI/build evidence is separate from native qualification.
Detached backend handoff and client configuration still need integration. A
native private client-config guard and immutable concurrent reader generations
are being connected; long log/terminal readers must not hold an exclusive
configuration lock. Explicit user extensions, known inactive snapshot eviction,
legacy config review and installed writer/recovery acceptance are separate gates.

The 2026-10-26 continuation selected the independently verified Windows and Linux
Docker CLI managed-plugin candidates in `runtime-tools.lock.json`. The Windows
argument-schema binding was independently requalified against all compiled
sources before updating its binary pin; the sole compiled source change is the
narrow plugin-discovery adapter. No candidate Docker binary was staged or run.
Read-only engine commands now connect the native config reader snapshot and the
atomic cached-client worker in source. Cancellation keeps creation and cleanup
owned independently of the caller. The complete Rust test target compiles, both
environment regressions pass, and the full JavaScript suite passes 254 tests.
Native atomic runner/config qualification and the remaining operation, registry,
terminal and setup callers are still required. No Phase 2 completion is claimed.

The later private client filesystem fixture passes in 3.740 seconds:
`artifacts/qualification/p2-client-config-native-source-v7-20261026/native-run-8df6ec3f-463d-41b6-815e-d6a24f03983e/proof.private.json`.
It exercises fresh master admission, raw settings, actual cancelled native
creation/one-use NoStart abort, inherited data-file reopening, concurrent
immutable readers, exact inactive-generation eviction beyond 32 changes,
32-live Busy/recovery and tampered-original abort refusal. It opens no credential
store and starts no child. The original failed nonces remain preserved. Two
concrete corrections were needed: actual restricting SIDs replace an
ever-filtered UAC-token test, and file Data uses GetSecurityInfo(SE_FILE_OBJECT)
consistently. The observed kernel-object query omitted inherited ACE flags;
the file query passed the unchanged exact three-ACE policy.

Actual atomic-client fixtures now pass birth admission, private pipe/stdio,
EOF/status, bounded stdout/stderr, timeout and caller-loss stages. They expose a
separate retirement-contract issue: empty job accounting/list precedes native
descendant process-object signaling by about 1.6–1.8 ms in the recorded cases.
The exact job also includes Windows conhost.exe; later receipts explicitly stop
claiming every associated process is an inert probe. Retained native handles
verify exact membership, generation and bounded final exit. The immediate-signal
criterion remains a preserved failure while the execution/storage authority
contract is reviewed; no generic kernel-I/O retirement claim is qualified.

Rust and Go writer source now owner-flushes, positively closes and independently
reopens incoming data before intent settlement. Registry auth changes are merged
into the baseline raw root rather than losing unrelated settings through Docker's
typed ConfigFile serializer. Positive spawned-writer qualification remains open.
Managed Windows child environments skip implicit current-directory executable
lookup and pin PATHEXT to .EXE, retaining the exact helper directory first even
without an inherited PATH. Go environment regressions and the Rust selector
regression pass; global user environment is unchanged.

The selected Docker CLI Windows candidate separately executes its client-only
`--version` in 95 ms, with an isolated config/home, disabled plugin discovery and
a nonexistent dedicated daemon pipe:
`artifacts/qualification/p2-docker-cli-native-version-7ecd815e-58ad-4fbb-a74b-2a45b3a76a59/proof.json`.
It reports `29.9.2-rd-orbwin.1`; config bytes remain unchanged. It was not staged,
installed or used against a live engine. This is startup/version evidence only.

The continuation adds a separate credential writer worker in source: secret input
and both discarded output streams remain owned through native creation and
settlement, while cancellation retains the pending intent. It compiles, but is
not yet wired into the production registry adapter or positively qualified with
a spawned writer. The parallel writer review also confirmed a second raw-data
gap: replacing the whole auth map loses unknown fields in unrelated registry
entries. Per-target entry preservation and exact image/argv admission are being
implemented before integration.

The Go CLI now refuses to mint configuration settlement authority after an
interrupted writer, IO-copy error, failed termination or partial creation error.
Known cleanup and natural command completion have separate outcomes; natural
nonzero completion remains eligible for verified file settlement. The focused
retirement/one-use tests pass (0.350 s). Explicit `--cloud` target refusal and
workload payload regressions pass (0.344 s). The pinned Docker source already
rejects cloud resolution with the fixed host, so this is explicit adapter policy,
not evidence of a prior observed foreign-engine connection. No product service,
engine, trust store or firewall was changed.

The independently reviewed reader v13 probe passes in 2.933 seconds:
`artifacts/qualification/p2-rust-native-client-native-source-v13-20261026/native-run-fb4d3443-a3aa-465c-8895-e7490d873d76/proof.private.json`.
It exercises actual atomic cached creation, IO/EOF, output caps, timeout, caller
loss and root-first descendant execution settlement. It also positively waits
both retained descendant/conhost handles, with the known descendant signaling
1.440 ms after the separate execution observation. No writer/config/vault or
all-historical-process/kernel-I/O durability claim follows from this reader test.

Per-registry raw preservation, exact image/argv/helper-environment admission and
unconditional owner-created final publication are now implemented in Rust and
Go. The Go package tests/vet and twelve Rust config tests pass; two credential
output-discard tests and the managed environment regression pass. The new
SDK-to-backend build input verifies signed assets, source/resource provenance,
all five file pins and public key, and rechecks before manifest publication.
Its focused provenance tests pass; the full JavaScript suite now passes 255.
No successor backend/SDK binaries were built or staged by that wiring change.

Positive writer qualification remains open. Disposable candidates4 and6 retain
their failed receipts: an observation handle held the master config open during
classic replacement. Actual candidate6 NTSTATUS is ACCESS_DENIED; its child
temporary file and owner stage remain preserved. The corrected test separates
normal closed-observer publication from deliberate open-master conflict. No
production sharing/ACL exception or automatic retry was added.

## 2026-10-26 continuation: writer and recovery boundaries

The corrected actual writer fixtures subsequently passed all four finite cases
in candidates7 and8 under
`artifacts/qualification/p2-credential-writer-native-source-v1-20261026/`.
Candidate8 receipt is
`native-run-5f913c79-ac5e-4595-b190-e89e0f67b66b/proof.private.json`.
Actual owners published new private config identities for normal and unchanged
output, preserved unknown raw baseline values, refused unauthorized routing,
and preserved pending evidence on interruption/open-master conflict. No actual
registry, credential vault, Docker engine or UI was exercised. Candidate9 freezes
an intermediate durable-original protocol and compiles, but has not run; current
schema3 recovery requires its own fresh qualification.

Registry login/list/forget and ordinary Docker streaming now use retained native
workers in source. Terminal clients also use atomic native creation with bounded
input/output and retained admission through execution retirement. These are
source integrations with focused pure tests, not installed/native acceptance.

The new [desktop/CLI recovery interaction](architecture/client-recovery-interaction.md)
retains the real native review behind a one-use epoch. Its two lifetime tests,
TypeScript, frontend production build, and twelve positive/five negative native
PowerShell completion cases pass. Parallel Go/Rust schema3 work adds before-first-
resume birth and pre-original prepare records; independent review found additional
healthy-config, staged-settlement and interrupted-cleanup cases being closed.
At that source milestone, recovery-native execution had not yet established the
protocol. The subsequent bounded qualification results are recorded below.

Standard-user isolated primitives qualified actual process/image FILE_OBJECT
binding through class44 and nonempty PID-directory old-start denial. Class89
was actually denied even for the disposable owned child; no elevation fallback
was added. Combined legacy upgrade still needs the real SDK child launcher,
listener/job ownership handoff and positive serving ACK, plus the enabled-guest
closure path. No actual product process or WSL service was stopped.

Phase2 remains incomplete. Coherent successor source/binary packaging, actual
upgrade and installed combined-flow qualification, all included additional flows,
coexistence/data preservation and measured resource budgets remain required.

### Schema3 native recovery milestone

The fresh recovery-candidate-1 actual run passed cancellation/restore,
review-close/review-again/restore, and actual open-master publication conflict
followed by same-process review/restore. Its cold preparation case refused
before creating a writer journal: the fixture supplied slash-equivalent argv
while production admission requires the exact native guard path. The failed
receipt and namespace remain preserved. No production admission exception was
added.

Recovery-candidate-2 changes only that fixture argv and fixed safe diagnostic
labels. Its two actual processes passed in1.759 seconds: the first persisted
actual prepare/original/intent then exited without Rust Drop; the second
reacquired the native controller, explicitly reviewed/restored exact raw baseline
bytes under a new owner file identity, cleared the exact reviewed journals,
and reopened normally. No Docker/helper child ran in this cold case. Both
candidates compile the same five production module hashes; root separately
verified complete source closure, namespace-only substitutions, signatures and
hard-pinned binaries before each distinct execution call.

Receipts are `recovery-candidate-1/root-native-receipt.json` and
`recovery-candidate-2/root-native-receipt.json` under the writer qualification
directory above. These establish four bounded native recovery journeys, not
all partial-journal/cleanup cutpoints, Go-native recovery, actual selected client
behavior, credential-vault effects, power-loss durability or a desktop UI journey.
Those gates remain open. Independent source review also corrected object-prefix
EOF classification and Go-compatible managed-selector JSON escaping, preventing
cross-owner restoration hash drift for HTML/line-separator characters while
preserving unowned raw values. Focused Go race tests and vet pass.

The Go-native cold recovery counterpart also passed in
`p2-go-client-recovery-native-source-v1-20261026/candidate-2/root-native-receipt.json`:
actual Rust publishers establish the private code/config namespace, an actual Go
owner persists its writer journal then exits, and a separate actual Go recovery
owner restores exact baseline bytes with a new native config identity and healthy
reopen. Four actual selected role PEs are held/read/copied only (143318528 bytes);
no Docker, helper or plugin is executed and no child, engine, vault, WSL or UI
operation occurs. Its first fixture correctly refused empty build pins because
the test-binary linker namespace was wrong; that failed nonce remains untouched.
The correction changes only the fixture build inputs and verifies their actual
initialized symbols and values before native admission.

The producer-cutpoint milestone now passes all twelve declared native scenarios
and thirty phases (21.236 seconds), recorded in
`p2-credential-writer-native-source-v1-20261026/recovery-candidate-3/root-native-receipt.json`.
Actual debug-only producer hooks interrupt private CREATE_NEW writes after proper
prefix/flush/readback, or exact ID/SD disposal after checked close and absence.
They are closed one-shot enums, unarmed during normal operation and excluded
from release builds; fixtures never rewrite pending records externally or create
fake native proofs. Root independently verified182 frozen files and all native
binary/source pins before the separate execution call.

Observed cases include EOF prepare/original/intent/birth/recovery intent/stage,
cleanup after intent/original/prepare, final recovery cleanup, birth-only cleanup
and final birth cleanup. Each recovers exact raw baseline including1e9999, or
verifies an already completed healthy state. The three bounded inert native
births include one suspended root before first user resume and two cancelled,
positively retired roots. No real client/helper, vault, engine, WSL, UI or admin
operation is exercised. Arbitrary compound interruptions, power loss/corruption,
schema1 refresh/fresh-init/legacy import and version-crossing recovery target
policy remain separate acceptance gates. A new runtime selector must not silently
change a durable pending restoration hash; upgrade preflight or a concrete
successor transaction still has to close that case.

### Cross-client recovery and failed-launch admission

Root independently reviewed the complete current-source closure and executed
`p2-go-rust-cross-recovery-native-v1-20261026/candidate-2`. All six phases passed
in `native-run-09805893-3a9e-4943-a67f-908f0a386afb`: an actual Go writer journal
was recovered by Rust, and an actual Rust writer journal was recovered by Go.
Each direction preserved identical raw baseline bytes (including `1e9999`),
verified native identity and owner/DACL witnesses before restoration, published
a new config file identity, cleared the exact journals and reopened normally.
The four selected role PEs were held/read/copied only, never executed. This is
before-birth schema3 interoperability, not a version-crossing upgrade or real
credential-store workload.

The separately reviewed `admission-candidate-1` under the writer qualification
directory passed all seven cases in9.272 seconds, recorded in
`native-run-2a257969-8cf4-464c-89a6-3809fba1941a`. Actual native creation failed
before first resume; positive checked retirement returned the opaque one-use
admission token. The actual terminal-control callee, diagnostic worker and
streaming-operation worker then released their terminal slot, immutable config
generation and operation admission. Deliberately retained unknown native owners
returned no token and retained those resources. Ten suspended roots and zero
user-code births were permitted. The frozen closure contains seven whole actual
modules, the real Tauri Channel and exact data-only DTO/decoder source excerpts;
it does not qualify the complete operations registry, SDK TTY or desktop journey.

The independent Go runner fixture also passed all three native cases in
`p2-go-cli-atomic-native-source-v1-20261026/native-run-b32e4c28-668e-4371-98ba-04cc38691f94`.
Root reviewed the full30-file closure and independently rebuilt the identical
PE before a separate execution. Whole actual CLI sources, with no overlays,
performed normal bounded I/O, cancellation only after actual child READY, and
an actual created-root refusal before first resume. The actual runner positively
joined the root, observed its job empty, joined I/O and checked resource closure.
Its children were the same inert frozen PE; nil writer admission deliberately
granted no config-writer authority. This does not qualify real Docker/plugin
workloads, installed dispatch or arbitrary stalled caller I/O.

### Local settings transaction recovery

Go and Rust now expose a separate explicit KEEP CURRENT recovery for full
schema1 local `config-replace` intents. It captures strict native current
settings and the exact unpublished stage, discards only that stage, clears the
intent last, then refreshes managed tool routing in a fresh transaction while
retaining the same controller. It grants no Docker-writer or process authority.
The generic CLI `--apply-reviewed` action supports this consequence;
`--restore-original` refuses a local transaction without a saved original. The
desktop review also states the actual consequence before application.

Root independently reviewed the120-file C5 closure and separately ran all eight
phases successfully in57.675 seconds:
`p2-local-replace-cross-native-v1-20261026/candidate-1/native-run-766edd71-5128-46eb-a741-503795af145c`.
Each fresh namespace has two actual cache generations distinguished only by
valid lock-file formatting, with identical selected role pins. An actual
read-only baseline observer causes a real publication sharing failure during
the ordinary A-to-B selector refresh. After creator exit, the opposite client
reviews/applies recovery, preserves raw auth/unknown fields including `1e9999`,
publishes B's selector with a new native file identity, clears exact journals
and reopens normally. No journal was externally rewritten and no new fault seam
or child was used. The actual selected roles were copied/read only, never run.
This does not cover EOF local intents, fresh initialization, legacy import,
compound faults, power loss or the pending restore-target version-crossing gap.
Focused Go race/vet, Rust19 tests/check-tests, TypeScript/production frontend
build and13-positive/five-negative PowerShell completion checks also pass.

### Upgrade ownership investigation

Root independently rebuilt and separately ran four preserved, disposable
SDK-control/job primitive versions. Actual pending read cancellation completed
with `ERROR_OPERATION_ABORTED`; real pending-buffer ownership survived caller
drop and GC; creation-time job membership, mapped-image/token/session checks
and the first control echo passed. The complete control/job handoff remains
unqualified. Version1 timed out awaiting candidate acceptance. Version2 refused
its singleton job predicate. Version3 recorded actual successful membership and
exact kill-on-close limits but two job processes after SDK resume, so its
singleton-forever predicate refused. The serving candidate was not among those
two PIDs; the additional process identity is unverified. Version4 retains
birth-time singleton isolation, checks later SDK membership and exact job
limits, and positively checks that the serving candidate stays outside that
job. Its separately reviewed/rebuilt native run passed all primitive checks in
`p2-sdk-control-job-native-v4-20261026/native-run-d890523c-0e47-464d-b68d-3d72c2339e11`:
same-control transfer, actual token identity, parent job-copy closure while the
SDK remained alive, candidate last-copy close and both captured-root joins.
This does not prove positive empty-descendant observation after the job handle
has closed. Product resource release still needs root, active-job-zero and I/O
completion observations while retaining the actual job, followed by checked
close. None of these fixtures touched a product engine, service or WSL
distribution.

Source review also found that retiring the old Windows SDK process does not
prove engine-side helper retirement: failed container creation or image import
can outlive a failed client call, and selected historical cleanup loses some
failure evidence. The upgrade gate now refuses without actual engine-helper
ownership/retirement authority, including on the old version without user-WSL
helpers. Current source retains failed/pending namespace, relay and import
records and cached cleanup errors. The actual engine-owner provider, any issued
Linux guest closure and final SDK supervisor transfer/ACK remain required.

A separate controller-object primitive passed after root reviewed every source,
verified all pins and independently rebuilt an identical PE:
`p2-sdk-controller-object-native-v1-20261026/native-run-c84d8ea0-6466-4c5a-9c31-12b2c82e91ef`.
A new metadata-only open had the same native file ID but a different kernel
object. An actual remote duplicate and roundtrip retained the same kernel object,
per-handle granted access and file mode. After checked parent-copy closure, a
new RW/share-zero open remained denied until the child checked its copy closed
and exited zero; the subsequent open retained the original file identity. All
fixture cleanup checks passed. The fresh 32-byte artifact and receipt remain
preserved. This qualifies that standard-user primitive only, not the production
RPC, controller grant, complete SDK handoff or guest retirement.

The independent [guest network recovery investigation](architecture/guest-network-recovery.md)
resolves the P2-D06 investigation with explicit, confirmed stop/start recovery
and its workload-interruption warning. The existing EngineLifecycle interaction
provides this adaptation, which the accepted scope permits; current-package
restart and preservation acceptance remains required. BL-017 and failed tap-loss
evidence remain explicit. The observed source loop reuses a closed vsock object
and does not join every worker before another iteration; a naive systemd restart
can stop Docker/Kubernetes dependents. Automatic connection-generation repair
is unadopted Phase 3 reliability work, with the investigation's preservation
requirements retained. This does not prove automatic repair impossible or cover
ordinary VPN transitions. BL-018 guest-loopback publishing remains separate.

### Fresh initialization: native directory-sharing finding

Separate Rust and Go initialization-recovery providers now recognize only the
actual fresh marker, root-bound metadata and empty defaults, including exact
producer prefixes. Focused tests, race checks and source review pass. The C6
candidate1 compile failure is preserved. Root reviewed all499 frozen files of
candidate2, its namespace-only projections, compiler pins and bounded recipe,
then separately dispatched the native matrix.

The first actual empty-marker producer cut passed (exit117). Its Go consumer
obtained the opaque review, but a DELETE-access open of the held client directory
unexpectedly succeeded. The matrix stopped after two phases; no rename/delete or
settings completion occurred. Exact result:
`p2-fresh-init-native-v1-20261026/candidate-2/native-run-3c9fffd1-4e71-467d-b988-4090b4cf8b27`.
The failed namespace/nonce and all artifacts are retained, without retry or
cleanup. No engine, WSL, selected role, UI or administrator action occurred.

The source diagnosis is backed by Microsoft's
[share-access algorithm](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-fsa/8c0e3f4f-0729-49f4-a14d-7f7add593819):
metadata-only directory opens do not participate in these sharing checks merely
because FILE_SHARE_DELETE is omitted. Minimal FILE_TRAVERSE access is being added
to retained directory authority, with separate audits of cache and upgrade
directory caps. It adds no directory-listing permission. A new frozen candidate
with fresh nonces must establish actual exclusion; pure access-mask tests cannot
replace that observation. At that milestone initialization recovery remained
natively unqualified; the later candidate result follows.

The following candidate3 run passed actual client-root sharing exclusion, then
refused before init/default creation because a raw product-path literal in Go's
new recognizer escaped the fixture's exact namespace projection. That failure
and nonce remain retained. A fixed compile-time constant preserves product bytes
and is checked against each projected case before native APIs in candidate4.

Candidate 4's separately reviewed native run passed all 29 phases across 14 fresh
namespaces in 199.275 seconds:
`p2-fresh-init-native-v1-20261026/candidate-4/native-run-50e3035e-a7e8-4b04-a588-678efe32e75c`.
It includes 14 actual creator/repair process-loss cuts, checked cancel, marker-last
healthy/no-review, live preemption/busy refusal and 95 actual DELETE-sharing
refusals, including 42 cache-directory refusals. No selected role, child, engine,
WSL, UI or administrator operation ran. The corrected timestamp derivation is
`candidate-4/root-native-receipt-v2.json`; its native matrix/results are unchanged.
This qualifies the recorded initialization cuts and directory-sharing scope,
not power loss, unknown-close faults, legacy import, local EOF replacement,
cross-version restoration or full installed/WSL/UI acceptance.

### Cross-version client recovery and remaining scope accounting

Root reviewed and adopted ten Go source files from frozen successor candidate V4
on 2026-10-26. Its 117-file closure, signature and current source origins were
independently checked before adoption; pre-adoption originals remain in
`p2-root-writer-continuation-20261026/go-successor-v4-pre-adoption`.
The schema 2 bounded journal and durable `recovery.next` replacement preserve
the original or actually published baseline across selector changes. Completed
birth-last cleanup precedes a separate ordinary managed-settings refresh under
the same controller. Atomic rename is a cooperative fixed-name protocol, not a
kernel destination-ID compare-and-swap. Unknown close/rename retains the owner.
Active focused pure tests passed in 0.511 seconds, race tests in 2.340 seconds,
and vet passed. Matching Rust integration and cross-language native cutpoint
qualification remain open; no runtime bundle was staged from this adoption.

The D02/D05 accounting review is retained in
`p2-remaining-decision-accounting-20261026/report.md`; root adoption receipts
record the subsequent source changes. Core portable data transfer, clone and
project migration remain required. Storage help now distinguishes logical usage,
host free space, file length and unmeasured physical allocation, with an explicit
manual-maintenance compaction disposition. Whole-engine reset/backup/relocation
retain their existing Phase 4 lifecycle accounting. Kubernetes forwards retain
their application-lifetime localhost adaptation and all remaining native
protocol/TLS/target/deadline checks. The frontend production build passed after
the help change; native visual/keyboard and installed acceptance remain open.

The same review found an additional integration defect: setup still manufactured
the old public OrbWin Docker config before admitting the private owner. Source
now admits a real private snapshot before service changes, uses its retained
clones through the atomic cached worker for both fixed-endpoint Docker info
reads, and removes the obsolete public-config Build History prerequisite.
The public master writer/validator was removed; upstream management context
paths remain separate. Setup cancellation drops the request while the detached
worker retains the actual snapshot through positive native release. Rust
compilation/focused tests and independent review of this integration are still
pending alongside the Rust successor refactor; no setup was executed.

BL-020 records the separate required legacy OrbWin settings import and the
credential-policy mismatch found during audit. Existing public data is retained
and refused before service changes until explicit import review exists. This is
not permission to import global Docker configuration, invoke foreign helpers,
silently discard inline secrets or infer vault conversion from raw config
preservation. The import design and exact source hashes are retained under
`p2-legacy-client-import-design-20261026`.

### Physical SDK resource retirement: native fast-path proof

Root independently verified the frozen 150-file management fixture closure,
reran tagged vet and compilation, and obtained an identical PE before separate
native dispatch. Run
`p2-sdk-resources-retirement-native-v1-20261026/native-run-e2668589-25de-4236-bb42-3781a35d15ae`
passed in 0.676 seconds on PE
`d7eccb050dd2500ca6dda6e760031bcdab27d0e7698af7b6b1613015ea186e8e`.
The actual product physical holder observed its own inert child generation exit
zero, retained/queryable job active count zero, raw IO join, checked job close
and controller exclusions released last. All thirteen checked cleanup operations
succeeded. Actual directory DELETE exclusion and mutable-file DELETE exclusion
with legitimate read/write sharing also passed. The random private 128-byte
fixture namespace remains retained.

This used an ordinary token and no selected role, engine/service, WSL, UI or
administrator operation. It does not qualify the descendant termination branch,
SQLite/WAL shutdown, exact SDKself image-cache roles, logical protocol issuers,
engine/issued-guest retirement or full upgrade grants. A separately frozen
own-descendant candidate is source/build work; V1 is not retried or expanded.

The own-descendant V2 run
`native-run-dc416809-bc26-4601-ab3b-6d2676d6ebdf` refused before descendant resume
because its exact two-member assumption failed; it did not record the numeric
count. V3 run `native-run-c44dcf15-1681-4daf-ad92-3bb8982cade4` added raw accounting
and bounded PID-list observations. Native queries agreed: three members before
descendant resume, then four after resume, stable through the two-second bound.
The extra members' identities are unknown in that evidence. Neither run reached
product retirement or termination. Both actual graphs were quarantined until
fixture process exit; their nonces and files remain preserved without cleanup or
retry. These are fixture-scope failures, not evidence of a running-engine defect.
The next separately reviewed diagnostic must identify every bounded own-job
member and explicitly record parent/root generations; no name or guessed PID
may justify a termination target.

### EOF local replacement: source integration

Root reviewed and adopted seven Go files from frozen local EOF candidate V2,
after independent verification of its 129-file closure/signature/current origins.
V1 remains preserved: its unconditional EOF dispatch would have intercepted
existing schema3 writer-intent recovery. V2's actual loader classifier prioritizes
independent prepare/original/birth/recovery/NEXT evidence, preserving that route.
Only no-sibling exact current-producer prefixes reach the separate KEEP CURRENT
provider. It requires actual strict current/native identity/descriptor/body,
canonical owned selector, controller/root and all competing-record/stage absence;
intent retirement precedes a new ordinary refresh under the same controller.
Native errors/uncertainty preserve admission fences. Generic EOF, metadata,
filename or a hash alone supplies no authority, and no power-loss claim follows.
Active pure tests passed in 0.477 seconds, race tests in 2.129 seconds, and vet
passed. Rust port and separate native cutpoint qualification remain open.


### Current source integration — 2026-11-06

The complete coherent desktop release build and layout verification passed in
`artifacts/qualification/p2-coherent-app-build-20261106-1/result.json`. All 225
Rust tests compiled. The root ran the exact metadata-only backend patch-lock
attestation unit (one passed); a separate parallel review ran the default suite
against that epoch's test executable: 191 passed, zero failed, 33 ignored, one
filtered. The latter included bounded owned fixture children and temporary data,
but no application main, engine, WSL, trust, registry or installer operations.
These are distinct results; neither is 225 executed tests or installed acceptance.

The source has since advanced beyond that immutable application build. The five
Rust backend-path adapter files now match the candidate checked by an actual
`cargo check --release --tests --locked --offline`. Setup's four obsolete
args/config directory uses were replaced after independent review. The backend
path response supplies routing data; it does not grant filesystem authority or
service lifecycle ownership. Source receipt:
`p2-root-management-path-installation-adoption-20261106-1`.

The reviewed LAN V2 source is also integrated: seventeen changed files, with all
499 active SDK catalog files matching the frozen candidate. The correction
rejects copied owners, caches unknown cleanup before effects and retains actual
partial owners through fixed callers. Its existing pure/race/vet/compile checks
and two source reviews support integration; signed SDK rebuild and actual
registration, elevation, rule recovery/removal and client traffic remain open.
Receipt: `p2-root-lan-v2-adoption-20261106-1`.

Fresh setup still needs the actual upstream service and publication composed
under one native selector/root/storage/setup-lock owner. Legacy transition still
needs real authenticated old-service/state and complete retirement participants.
Held arguments, credentials and listener observations are prerequisites rather
than completed transition authority. Those changes remain isolated pending
consolidation. Next coherent package and the combined workflow, coexistence,
data-preservation and interaction acceptance gates remain required. Current
checkpoint: `p2-root-continuation-20261106/checkpoint-v3.json`.


### Actual default certificate producer correction — 2026-11-06

Review found a concrete setup/import compatibility defect: pinned client-go's
default generator returns serving leaf plus CA PEM, while both private authority
validation and the legacy credential reader expected one certificate. Root
reproduced the authority rejection with that actual in-memory generator before
changing the code. The reviewed fix preserves historical single certificates,
bounds and validates the linked default two-certificate chain, preserves the
serving-leaf DER fingerprint, and retains key match, loopback hostname and
separate service-account key checks. The credential reader retains the anchor
separately from that leaf fingerprint. Its new regression runs the actual default
generator through CreateKubeConfig and clientcmd.Write for YAML and JSON.

Independent reviews passed, and the exact five-file combined source change is
adopted into the isolated integration tree (now 932 files). The authority change
passed focused pure/race/vet/test compilation; the combined authority/publication
and credential tests passed in 2.217 seconds. No real credential, native authority
writer, TLS connection or engine was exercised. Source adoption receipt:
`p2-root-authority-and-credential-chain-adoption-20261106-1`.

The current Node suite passes all 292 tests. Canonical18, its locks and the first
complete app remain unchanged; they predate these fixes. The fresh setup owner
and authenticated legacy-management observation are separate isolated production
lanes. Setup/service publication, complete migration retirement and the combined
native/installed acceptance still remain. Checkpoint:
`p2-root-continuation-20261106/checkpoint-v4.json`.

### Persistent setup integration continuation — 2026-11-06

The isolated backend integration tree now has 945 files after the independently
reviewed FreshOwner V3 adoption. It captures the actual native selector, storage,
setup controller and durable fresh intent before authority creation; ordinary
startup refuses an incomplete fresh transaction. Its adoption does not enable
the new product setup command. The later V5 service/publication/recovery work is
still a separate mutable candidate, with finite frozen component receipts.

The persistent Rust setup observer and private host diagnostic sink are adopted
as unused source. A review found and corrected premature executable-lease release
after child birth when identity verification or observation-thread creation fails.
Original child/image owners now remain retained until actual positive child exit;
failure closes observation pipes without terminating the service. Seven focused
pure tests pass, and actual Windows observer/cache modules pass offline typecheck
and lint checks. These checks do not execute process or log providers. Rust handles
cannot survive the UI process's own exit; the Go serving process must retain its
own required image/forwarder custody. The log sink uses the native current-user
profile and fixed private host directory, accepts no caller path, preserves prior
logs, and creates neither the engine namespace nor guest home. Installation is
not wired to either helper yet. Adoption receipts are
`p2-root-private-setup-client-adoption-20261106-1` and
`p2-root-private-setup-log-adoption-20261106-1`.

The fresh runtime observer's raw transport correction passed independent source
review: failed Dial outputs are retained, borrowed unknown state prevents cleanup,
owned credential buffers are cleared on terminal failure, and cancellation joins
I/O and its deadline watcher before the sole cached raw close. Legacy authenticated
observation and actual-bound-port selection also passed a finite source review;
their fifteen-file cumulative delta still requires explicit integration with V5.
Neither authenticated data nor endpoint ownership grants HOME or retirement
authority. A producer gap in the pinned Lima driver is being addressed separately:
owned new guest imports must explicitly request WSL 2 without changing global
WSL defaults or relaxing the observer's actual Version2 check.

Remaining integration work includes legacy-start denial binding, actual service
and forwarder custody, interrupted setup at every durable cut, later upgrade
supersession, the desktop setup handoff, complete legacy retirement, and the next
coherent package and native workflow qualification. No engine, WSL, trust,
firewall, installer or admin action ran in this continuation. The latest source
checkpoint at this entry is `p2-root-continuation-20261106/checkpoint-v5.json`;
the two Rust adoptions above are later explicit source advances beyond it.

An additive validation correction is recorded in
`p2-root-service-init-evidence-correction-20261106-1/receipt.json`: executed
service-package tests import Lima's Windows driver registration, whose initializer
attempts a read-only HKLM GuestCommunicationServices lookup before selected tests
run. Earlier labels saying no native queries were therefore too broad. The
registry query's result was not instrumented; this is not evidence of a socket,
WSL or engine action. Original receipts remain preserved. Further service-package
checks are compile-only pending explicit native-query review. A source-only
`go list -deps` check confirms the frozen V4 management package does not import
that driver or the service package; it executes no package initialization.

The isolated desktop handoff now passes the complete actual Tauri release/test
typecheck (`cargo check --release --tests --locked --offline`, 36.47 seconds),
with two frozen caller/observer files over 444 verified unchanged source inputs.
The engine, runtime, IPC and private-client consumers are the real application
modules in this check. No test executable or product role ran. Historical
assembly/frontend resources are compiler inputs only; this is not a new coherent
package. Receipt: `p2-root-private-setup-fullcaller-check-20261106-1/FROZEN-RESULT.json`.
The handoff remains isolated until the actual Go entry/recovery path is ready.

A parallel availability audit identified remaining desktop routing gaps, including
environment-selected LocalAppData disagreeing with the backend's native known
folder and automatic debug proof-identity selection. A separate native-read
routing successor is underway; metadata or action availability will not substitute
for the Go owner's mutation checks. The earlier partial-control and missing-App
recovery cuts, later upgrades, complete legacy retirement and installed workflow
qualification remain open. Latest checkpoint for these source advances:
`p2-root-continuation-20261106/checkpoint-v7.json`.

### Continuation checkpoint: native routing and portable Lima

Native LocalAppData routing and explicit proof identity selection are adopted.
The SDK/CLI own-token KnownFolder rights corrections are adopted, with compile-only
validation; new binaries have not been built. The actual Tauri caller including
the frozen native-routing/setup overlay typechecks successfully (37.97s). This
is compiler evidence; active setup and native qualification remain outstanding.

The reviewed ten-file Lima build route is adopted. Root-rdd builds use an
output-owned, version-limited local replacement with original701-file input and
reviewed703-file patched projection, explicit module flags and portable source/
notices records. The complete Node suite passes330 tests, zero failures.
Receipt: p2-root-lima-build-route-adoption-20261106-1/result.json. No product build
or WSL import was executed. Canonical18/active945 Go remain unchanged.

Parallel implementation continues on durable pre-POST App admission and explicit
continuation, actual managed-child/native namespace retirement, and interrupted
authority/publication recovery. Force-path refusal is an interim safety fix;
it cannot replace healthy stop/restart/update acceptance. Guest pre-controller
review requested a final cancellation-boundary successor. Legacy HOME, bridge
binding, safe old retirement and the stopped cold-owner route remain open.
Native/current-package acceptance and final0019/coherent build remain required.
Checkpoint-v8.json links current adoptions and distinguishes isolated V5 source.

### Source recovery/lifecycle checkpoints and native platform gate

The adopted Lima build helper compiled the actual Linux Lima guest successfully
in20.095s. Its embedded Main metadata has the pinned original module/version and
exact output-owned local replacement expected by the notice collector. The guest
was not run; full backend/collector/package validation remains outstanding.

Finite isolated V5 source reviews now pass for final guest-preflight cancellation,
orphan publication stage, original WithHandle child birthV2, initial protected
material tuple before TLS leaves, and missing-control recovery after intent plus
complete old-start denial. Storage.Close now refuses its retained management
child; the denial-only regression passed and independent review agreed. These
components do not establish complete partial-root/repeated-cut recovery, actual
healthy lifecycle retirement, schema2 UI continuation, upgrade or native success.

BL024 remains a critical platform gate: past native CREATE refusal is not resolved
by compiler evidence. Root prepared a standalone closed-case diagnostic plan
(p2-root-native-create-diagnostic-design-20261106-1); source preparation may
continue, but no native run is authorized. Exact source/recipe/closure/origins/PE
review, immutable approval, explicit native resume and separate root dispatch
remain required. Failed namespaces/process evidence remain untouched. Additive
checkpoint-v9.json links the current finite reviews and outstanding gates.

### Continued setup and platform source review

The isolated schema2 desktop candidate passed the actual full Tauri Rust 1.90
release/test compiler check (tests and product not executed). Root reverified
all 545 snapshot inputs and eight candidate overlays after compilation; frozen
result SHA349ae981a8a4c14bdcd53c5ce0052cdfd9dff4873f4d3028fec98a9e54720b8d.
Independent source review then found a concrete pause defect: request_pause set
cancel_sent while poll rejected that state, preventing positive SetupPaused
observation. The original candidate remains preserved; a narrow successor with
an actual inert method-interaction regression is in progress. Compiler success
does not establish this behavior as correct.

The 23-file Go App successor passed scoped root source review, receipt
p2-root-fresh-app-component-review-20261106-2/review.json SHA
6cbbebddecd52372fcc6f8c23a19f1322bb1fb9a271f052df2c175b3271ec5e4.
It corrects Created/storage lock order, adds the original RequestInfo-based
pending write gate, separates preparation/consent from irreversible Begin, and
publishes a checked complete staged marker before POST. The unchanged eleven
controller files retain their earlier review. Actual service gate placement,
CRD-only preparation, never-started-manager cleanup and checked API/kine joins
remain separate integration requirements. No active setup switch occurred.

The standalone native CREATE diagnostic v3 source and exact offline CGO0 build
recipe passed root review (p2-root-native-create-source-review-20261106-2,
SHA1d68e7ea06fe1540dcab4c4f7651b53393fd0000c8fdda7687ba7ffef8eedae1).
Public ancestor descriptors are explicitly diagnostic observations; newly
created destinations retain strict private descriptors. AppContainer denial
and the final provider deadline check are explicit. Final PE, used closure and
Node controller review remain required. Native resume is not authorized by
these source/compiler receipts, and BL024 remains open.

The pause observer successor is now frozen (desktop receipt9035d2b8...879fe2):
production retains concrete ChildStdin, successful pause permits continued polling,
and terminal cancellation/delivery failure remains sticky. Thirteen inert actual
observer tests and Clippy passed. Root's full caller successor compiler check
passed42.50s; all545snapshot inputs/eight overlays remained unchanged. Frozen
result p2-root-app-decision-fullcaller-check-20261106-3/FROZEN-RESULT.json SHA
b892950d09e1f07455ede3a29fdb248a94b9fd3d9f53f8f1e658d1ff49fd68f3.
This remains isolated pending complete Go service integration and native gates.

Five qualification wrappers were confirmed to target the historical daemon,
not a general current package. Their native/debug paths can inherit the proof
selector while direct JS checks still use historical engine IDs. The finite
routing audit (p2-import-historical-wrapper-routing-audit-20261106-1, receipt
91cfe605d8432c5a8aa375f4ba702be37bcdc27986e2d1cfdc6ca1b89e97af01)
records explicit product-vs-reviewed-proof successor requirements. These scripts
were not run, edited, or counted as current-package acceptance.

The diagnostic final PE was compiled but not executed: SHA967a3c4b5b5561e917f994853aa680d19568b5b24ac65d70efe4d83a1addf355,
3137536bytes, AMD64 console, Go1.27.3 CGO0 internal linker. Root rehashed all809
selected source files from116packages and reviewed exact buildinfo/static
inspection (final build review96a5190e...a4cf5). The pinned original-object Node
controller/template source review passed with explicit runtime/fatal-window
limits (8d398813...ef176). Exact sealed one-case dispatch/clean startup and
explicit native resume remain separate gates. No native success is implied.

A subsequent selected-closure startup audit found a required scope correction:
x/sys/windows imports net; Go1.27.3 net initialization calls poll.InitWSA and
WSAStartup. The diagnostic therefore includes trusted runtime Winsock DLL/process
initialization before main. This path adds no socket/listener/connect operation;
zero-network-API or exhaustive initializer-absence claims are withdrawn. The
source/PE is unchanged, and no native case ran. Addendum
p2-root-native-create-init-scope-addendum-20261106-1/review.json SHA
a09286eb53b29fa0da715bf4be20c4f7f23346347d9196ca1dab39aaf7784c79
must accompany the eventual sealed dispatch manifest.

### Shared setup integration and continuation routing

Both App integrations are now present in the shared V5 candidate: management12
receipt65a6735a...0102 and controller11 receipt271e858e...b34f7. Root independently
rehashed23 retained sources and before/after receipts; ten controller files and
all twelve management files match approved inputs. Removing only the three-line
Lima template-owner guard reproduces the entire retained beforefile; the original
observer guest-directory-through-close tail is byte-identical. Root review
p2-root-app-shared-integration-review-20261106-1/review.json SHA
3398b10a332593798d5ff38a5887660c2dd7edb74029edc475eb61f5c83b9bcc.
Actual service gate placement, deferred-manager sequence and lifecycle joins
remain unqualified; this is not a complete V5/source adoption.

The typed availability plan is frozen (df3cfced...085b) and identifies the
existing status/client reader's ability to create client state. A passive native
inspector must use FILE_OPEN-only observation and keep file, service connection
and desktop-client readiness distinct. Actual inspect/upgrade routes remain
absent; this design is not a successful flow. Historical qualification wrappers
retain their separate target scope. Isolated current-package WSL-root and fixed
setup-log access candidates are being prepared without active-source changes.

A source-peer review blocked the original diagnostic PowerShell launcher:
fallible error reporting could escape before its unknown-owner keeper. The
original is preserved and must not run. The additive v2 puts keeper entry in
finally; exact26-input source review passed (f3fb2de7...92b2d). The same single
artifact-a action is awaiting explicit user native resume. Raw outer-capture
recipe cf4fabfb...a5bb is prepared only. No launcher, diagnostic, engine or WSL
qualification ran during this continuation.


### Native authorization correction and observed directory results

The earlier notes requiring a new native resume were incorrect. The actual user
resumed computer use at 2026-10-25T04:06:12.677Z ("You can use the computer now");
no later explicit human pause was found. Root's source audit ac2fba39...b0d
records this correction. Existing authorization permits scoped reviewed native
work; source/data-preservation gates still apply. No elevation or execution-policy
change was used.

The reviewed diagnostic actually ran three distinct fresh cases. Artifact-A on
F: completed its bounded creation/verification sequence (OUTCOME 3d6e2d9a...79fb).
Strict LocalAppData-A refused CREATE with STATUS_REPARSE_POINT_ENCOUNTERED
(OUTCOME df099617...16ad). LocalAppData-B omitted DONT_REPARSE for CREATE only:
creation succeeded, but its subsequent strict parent-relative child OPEN refused
with the same status; outcome is created-unverified, not a successful product
flow (OUTCOME f6b57612...a724). Original child termination and capture settlement
were observed. Each target and all evidence remain preserved; none was retried
or reopened. This does not establish root cause or justify production flag changes.
A new closed-name diagnostic C is authorized for source preparation only.

The fixed log-access9, WSL routing5, and ConfirmDialog focus1 components have a
scoped root review bbcd96b6...130f; a composite with desktop8 is being compiled
in isolation. Native WebView, Explorer navigation, installed/current-package
flows and accessibility acceptance remain open.

The narrow Windows lazy-home Lima adapter is adopted in seven build-route source
files after before-byte verification, receipt 0fa1e4de...481f. The portable source
offer covers six replacements/all705 reconstructed files; prior WSL2 import
replacement bytes are unchanged. Candidate pure/recipe tests and package compile
checks passed; no provider/product run or setup CLI enable follows from adoption.

The actual service schema2 checkpoint freezes18 owned files and1081 compiler
context files (1468c9f2...d626). Root verified the complete retained closure. Final
protocol delivery at process exit and manager-readiness observation bounds remain
review questions. Independent shutdown review found two real custody/bookkeeping
defects; corrected successor review is pending. Durable pre-intent seed recovery,
parent-crash recovery, actual read-only availability classification, supported
legacy/modern upgrade transitions, coherent packaging and installed/coexistence/
workload acceptance still prevent Phase2 completion.


The desktop composite18/569 passed the actual release Rust caller typecheck,
frontend typecheck and15 inert Node tests (0edf6fdf...148a). Root independently
reconstructed the four additive cache hunks and verified all569 files plus the
exact18-path component union (7b85febc...436d). ConfirmDialog alone is now adopted
with its backwards-compatible optional props and unique title association; active
frontend typecheck passed (03b0e723...55ee). Other setup handoff changes remain
isolated pending actual Go/native gates.

Root accepted the V4 two-defect lifetime corrections (cbdd95c8...6479) and the
V3 reduced-rights delta (872f9f15...fe91), with native behavior and survivor
recovery explicitly open. Service review confirmed final output delivery and
readiness-bound defects; a distinct successor is being prepared.

The inspection wire contract is now actual isolated Go source with four passing
behavior tests and vet (b33bec54...0a0a). It rejects noncanonical/extra requests,
contradictory states, duplicate/aliased fields and action promotion; all five new
actions remain unsupported and connection/client observations remain unperformed.
Eight wire examples were emitted by the actual tests. The native FILE_OPEN-only
producer and CLI are absent, so this is not a working availability flow.

Additive root checkpointv11 records the current authorization, actual negative
LocalAppData observations, adopted sources, parallel tasks and remaining exit
gates. Phase2 is incomplete.

Root continuation: strict D final leaf build completed once, no execution: source review0f38e908d1d5a5887b72cbe98739971a11216588f8c7b0abc6c476238252dc15; PE2a36e3545335d1f562b1ecb22e1fd8b5c2f9583f6930b8e2515e0b488281df69; final build/static809source reviewce0bdc28092617975dca5baf1e73cec75ad93171d3ba137f73f1d0c0077e5751. First review-script assertion expected linkflags in buildinfo; original preserved, successor checks linked manifest bytes directly. No diagnostic recompile or native effect from correction.
Refined launcher contract fb177bd8e18ecc49f4e959fc85c998830591826782bf1e3f868261e4a4e95360 approved for SOURCE implementation: PS5 original launcher -> pinnedNode with ENABLE1+HANDLE_LIST -> strictD. Launcher Node env only fixed SystemRoot/WINDIR=C:\WINDOWS, no PATH. EnteredCreate ambiguity retains original graph. New D capture fsync+onceclose; actualNode0 transport envelope separately classified against Go0/knownnegative1. retirement_review owns new isolated launcher AND D Node/controller/PS proposal; external authors must not edit those overlapping files. No final launcher build/native dispatch yet.
Source convergence: cached selector guard2 adopted only sharedV5 receipt6fb9ed82c8407f38cdfb2ad8dae7f52c4dfc48bb676fd45ab38c98b98385249f. Service3 incomplete DATA and SQOS2 reviewed; four sharedV5 files already match, receipt761ba4cb32ed9601ee287c4049753facb3cb0882e1136bd0362b4a7336ace6e2. No active945/setupCLI/wholeV5 acceptance.
Denial caller root5 frozen350f55c407e88553fc0db7f1756e632915d5ff89c00d500f5f43e74dd755e486; four focused tests PASS1.622s. Independent draft13a3425 found earlier selector read-handle flush self-block and ignored malformed legacy reserved history names. Author fixed in frozen successor source880 pin9d753698050e05e05380b045b351b5247223111860df88c1e537bffc60f07fce, validation running. Final independent successor review still required. Current4slots root+denial+inspector+launcher; reviewer resumes when author settles. No denial adoption/native acceptance.
C close-accounting correction: prior status wording 39positiveCloseHandle+2void rows describes ledger replay rows, not39distinct native close effects. Frozen CLOSE-ACCOUNTING-ADDENDUM e5d3269ac2598260a59c87a94b37b1318a5f6442b4b5836b102b262c48866685 governs; observed redirection despite no-package remains unresolved, strict production flags unchanged.


## 2026-11-06 continuation: strict D negative and coherent source context

The one authorized D diagnostic completed; saved original-owner receipts report
launcher 1 / Node 0 / leaf 1, known-negative. Strict CREATE returned c000050b
with no handle and unchanged IOSB sentinels. Desktop policy ENABLE was accepted
but did not establish a routing fix. All 60 dispatch inputs matched afterward.
Root outcome: artifacts/qualification/p2-root-native-create-localappdata-d-20261106-1/OUTCOME.json,
SHA 760c9c12fc1749632e1703a1b23c8d0693f294e140775f47cdd5b02b87a4720d.
No failed target reopen/retry/cleanup, admin request or policy relaxation occurred.
C outer runtime was unmeasured; do not label it verified WinPS5 or infer a
single-variable C/D comparison. Leaf close journal rows include cached replays.

Shared V5 now exactly matches the compiled 1,119-file context, manifest
7555cdfff63f7b1f3002223aa32386894349be6851ab04402c9f68b927d8c121.
Adopted source components include denial completion/history/prefix source4,
actual admission callers, dormant native inspection and its low32 ABI fixes.
The coherent checks passed 49 management cases normal/race, focused model,
protocol and route checks, vet and management/service/main compile-only checks.
These are source checks; no service/main test executable or product native case
ran. Active backend/canonical patches and disabled setup activation remain separate.
Seeded/interrupted inspection, modern supersession, whole-actor retirement,
coherent backend/SDK/desktop/package integration and installed expanded-flow
qualification remain open. Phase 2 is incomplete.

Next work is parallel: independent saved-D review, a fresh strict E source
candidate (not a retry of D), and modern supersession implementation planning.
A different launch origin requires its own reviewed ownership contract; no
native E dispatch or desktop-origin result is implied by this source plan.

### Read-only registry comparison and remaining inspector work

The once-only fixed current-user Software comparison completed with original
child exit0: self-handle comparison true, independent-open comparison false,
three open statuses0 and three reverse close statuses0. All33 final packet pins
matched afterward. Root outcome SHA
6f7ea433ba76428e1fa869eecc633eead7cfd58d4bf3c0b0e95ace0c3d99eda9,
`artifacts/qualification/p2-root-registry-key-comparison-native-20261106-1/OUTCOME.json`.
This demonstrates a concrete refusal in the dormant inspector's positive
cross-open assumption on this host. It neither qualifies actual HKU/SID routing
nor makes false into identity proof. BL-025 tracks the block.

The proposed successor retains original key observations, strict link/name/SD
checks and owned one-shot notifications, refusing signalled/uncertain outcomes.
Notifications are change detectors: they cannot establish unique logical key
identity or an atomic snapshot. Any resulting passive classification must remain
explicitly observational, with all actions unsupported. Source implementation
and protocol interpretation require review before adoption or native activation.

Modern supersession's next source component uses the existing outgoing backend
owner to launch one fixed incoming no-writer role with actual inherited custody
and a real old-service process handle. This avoids introducing another resident
lifecycle manager. The old-service terminal barrier, complete cold checkpoint,
early older-reader refusal and authoritative head ordering remain release gates;
the new source plan supplies none of those as runtime evidence.

### Strict E diagnostic and next qualification gate

The fresh E diagnostic completed through the reviewed Explorer COM route with
the strict flags intact. The recorded original internal launcher/Node/leaf exits
were all zero. One CREATE returned success/FILE_CREATED, its IOSB low32 status
was zero, and the original-handle edge/recheck/flush sequence completed. Four
held-name observations retained the literal LocalAppData/child relationship.
All69 final-packet inputs matched afterward. Root outcome SHA
70e058ea85e7f08829974f983d121bc2b82667e8ccb84564931ef4b7eb074d83,
`artifacts/qualification/p2-root-native-create-localappdata-e-20261106-1/OUTCOME.json`.
The broker returned no original outer WinPS handle, so no whole-family claim
follows. This is one successful diagnostic graph, not product storage or crash
durability qualification. All earlier failed targets remain preserved.

Before new constructor tests, source review found additional storage acquisition
gaps: generic NtCreate outputs and some Fresh/Win32/token outputs were local
until return, allowing pending-zero or panic paths to lose custody. Generic and
Fresh frame corrections have isolated source/test evidence; the remaining direct
volume/token captures are being corrected separately. Do not treat the earlier
Source4 directory-lease correction as coverage of every acquisition path.

The reviewed five-file modern self-process lease foundation is now in shared V5
only (adoption f074461e3df7149be7252d34b2eb05a5c622307757beb3338c1808e6af6544f7).
Its whole1123 source context matches its recorded compiler checks. No receiver,
inherited launch, writer admission or active backend change is included.
Independent source components now also cover registry bounded observations,
joint Go/Rust/TS schema2 labeling, seeded/interrupted classification and an early
same-image inspection CLI. Their composition, actual transport, native tests and
installed workflows remain separate required work.

### Coherent source integration and remaining native gates

Root composed the inspection/CLI/classifier, retained storage frames, Serving
domain correction and modern self-lease foundation in an isolated 1156-file Go
tree. Fifty selected DATA tests passed normally and with race detection; vet and
management/service/main compilation passed. A second isolated 1162-file tree adds
the SDK caller migration and five connected cleanup-cache panic corrections.
Its 15 pure SDK tests passed normally and with race detection, and vet plus the
same three compile-only checks passed. Neither tree is the active packaged
backend. The SDK successor remains under independent source review.

The active desktop now includes the reviewed convergence changes, volume editor
draft retention, and the three-file bounded-observation schema2 codec/model
update. Five Rust codec and five model tests pass against these active files;
TypeScript passes. The new inspection transport remains dormant and under
independent review, with no renderer invocation or native activation. These
source and mock results do not establish installed product behavior.

Before the next native constructor case, the actual retained selector factories
and a fresh fixture recipe must be reviewed. Constructor capture may create
setup.lock and therefore requires mutation custody even without stage/write/
publication testing. Storage cancellation before stage admission is being
corrected separately. Full modern handoff, coherent package delivery and all
expanded-flow/coexistence/workload acceptance remain outstanding.

### 2026-11-07 continuation: corrected transport and source review

The active inspection transport has since been adopted in a dormant state; the
full active Rust check passed offline. There is still no renderer invocation or
installed native inspection qualification. Root's newer isolated 1181-file Go
composition adds SDK image output custody, a same-build incoming image derivative
and datastore cleanup custody. Focused pure tests/race, vet and compile-only
checks passed. External review then found Q2A-REAPER-NAME-01: an unrelated
noncanonical directory name can abort startup. A narrow successor is being tested;
the frozen 1181 tree is not accepted or adopted into the active backend.

The constructor fixture and wrapper compiled successfully. The first dispatch
attempt stopped in PowerShell command parsing before broker execution. This is a
transport failure, not a native constructor result. The failed attempt is retained
in `p2-root-native-constructor-prepare-20261106-1`; no target was queried or cleaned.
A new controller uses verified encoded bytes and a fresh wrapper/runtime packet.
The harmless transport proof establishes exact bytes and PS5 runtime only; outer
exit status does not establish broker or target completion. Actual constructor
prepare and probe remain separate native gates, with original-owner observations
required. The existing successful diagnostic does not close product storage.

Datastore acquisition and candidate signed-statement work continue in separate
source lanes. The latter uses explicit build-time signer/verifier inputs under the
new decision; no actual candidate launch, inherited transfer or head publication
has been implemented by that decision. Full modern handoff, package integration,
expanded user flows, coexistence and workload qualification remain required.

The new encoded-transport PREPARE completed positively on 2026-11-07. Evidence:
`artifacts/qualification/p2-root-native-constructor-prepare-20261107-1/OUTCOME.json`
SHA7a143113eedad0a906a24caddf33ce570bd42789c73d1a3627aaeb05230a2ae9.
The original inner parent, native launcher, Node controller and leaf all reported
terminal success and their checked release/capture completion. All60 pinned inputs
matched afterward. The leaf used actual native identity, strict opens and checked
writes/readback in the source-projected C7 namespace. This is one fixture prepare
result; ConstructorsChecked is false. The separate mapped-cache constructor probe
is next, then publication/reopen and remaining failure/recovery work. The broker
still supplies no original outer process handle, so whole-family terminal status
is unobserved. No old target or other engine was queried, cleaned or stopped.

The separately admitted constructor PROBE also completed positively on2026-11-07:
`artifacts/qualification/p2-root-native-constructor-probe-20261107-1/OUTCOME.json`
SHA320f2b406c4140238510a8dbbb5d003bfc8b05bbef3ffe7afa2542ee8d3aa6bf.
Actual selector/root/Serving factories, registered native storage capture and
mapped-self origin/source-commit validation returned successfully, followed by
checked release and original inner process terminal/capture completion. All66
pinned inputs matched afterward. The possible scoped setup.lock creation was
admitted as mutation. This qualifies that source-projected constructor path only;
publication/reopen, changed1181 source, shipped namespace and installed flows
remain separate. Rejected probe source packets with normalized manifest bytes
were preserved and never built or dispatched. No admin elevation was required.

### Current combined source and candidate trust modules

Root's isolated1189-file backend composition includes the accepted unrelated-name
reaper correction, the datastore acquisition successor and the four-file Go
candidate verifier. Seventeen affected pure tests pass normally and with race
detection; vet and management/service/main compilation pass. All four complete
source inputs and the composed tree match their recorded hashes afterward.
Acquisition review is still pending, and this is not the packaged backend.

The explicit candidate statement/signing modules are now present in the actual
scripts directory; their nine pure tests pass against those files. Existing LAN
signing and build entrypoints are unchanged. Actual operator key provisioning,
signing, candidate staging, native issuance and activation remain unimplemented
or unqualified. Candidate signatures authenticate the fixed bundle data; they
cannot replace original native ownership or fresh/upgrade admission.


### Source continuation with native qualification paused — 2026-11-07

The user's latest instruction pauses native diagnostics and product runs. Source
implementation, independent source review, pure models and compilation continue.
The active persistent setup Source2 integration passes eight standalone model
tests and actual release/tests compilation (Root receipt `e986fe5a871f293e9d7a30c218973a8fd97d6f2ff591aef37d4edb2841788c92`).
That result does not qualify Windows process, token, cache or pipe behavior.

Canonical patch0020 now registers the accepted early setup/I/O/factory settlement
source after independent full replay closure. It preserves all prior19 patches
and the separately adopted Rust staging/setup source; no backend binary rebuild
or product run occurred. Root adoption: `9256265760107cc6da52e4c40a4ceb57c6e5096fbe5543dcde0ad25ada925963`.
The native candidate READ successor remains separate coherent1205 source and
proposed0021, not silently included in0020.

Remaining startup source gates include owned token/security outputs, ordinary
Rdd cache/process-verification ownership, genuine original hostagent birth
lineage, and authentication binding the guest bridge to the actual managed
session. The current upstream public static handshake is insufficient evidence
of that binding; distribution/VM discovery alone is a locator. Full simultaneous
engine coexistence remains required. Historical projected constructor success
does not close publication/reopen or installed expanded-flow acceptance.


### Further source integration — 2026-11-07

Canonical0021 registers only the accepted candidate-read eight-path successor,
preserving the0020 setup delta. Independent full source/hunk/tree/lock closure
217b6fb7 and ten affected Node tests pass. No image was launched or staged by
this registration; installed backend resources remain unchanged.

The Rust candidate acquisition custody successor is separately adopted as exactly
three source files (Root adoptiond428e962). All eleven standalone production
helper tests pass against the active files. It retains original output/storage
frames before native entry and preserves the current canonical attestation.
Actual Windows acquisition and ordinary Rdd process verification remain separate
qualification/implementation gates. Combined release caller compilation will
follow the corrected persistent setup security/error-delivery successor.

Independent security review found a concrete setup error-delivery defect: a
prelaunch security-unknown result reached synchronous retention and prevented
the request from completing. Source4 corrects that route and passes independent
review; a separate narrow successor corrects returned checked-close failures and
reports their actual bounded reason. These successors remain isolated until
reviewed composition. They change error delivery while preserving the same
retained state; they do not establish native close or retirement evidence.


### Checked source composition — 2026-11-07

The corrected persistent security/error-delivery Source6 is now adopted only as
its six-file net change from Source2, preserving candidate cache custody and
canonical0021. Root source receipt `c7bfbddd8484e4e776bb3b18b66946ca6dc9866486273a7ebe8b0d067cd28717`
records twelve setup plus six policy regressions, release/tests compilation and
Clippy exit0 against the actual combined source. All586 declared composition
rows,21 source patches and historical compile resource/tool inputs postmatch.
Clippy emitted86 warnings, including deliberate stable boxed native frames and
dormant candidate code; this is not a warning-free or native-qualified result.

The separately run eleven candidate custody tests still match the adopted cache
code. Both setup liveness defects now return errors while retaining the same
uncertain state; checked-close failure preserves the actual bounded cleanup
reason. Last-resort destructor/fatal-runtime retention and actual Windows
close/token/cache/pipe faults remain qualification limits. Ordinary Rdd2 ownership,
real guest-session authentication, selected driver joins/checkpoint and modern
receiver/recovery source work continue. Native and installed flows remain paused.
