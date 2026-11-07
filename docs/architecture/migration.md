# Selected-source development migration

Implementation in progress, 2026-10-25. This is not Phase 2 acceptance.

Migration preserves the source engine and its data. Images, persistent volumes,
project files/Compose inputs, application consistency and target conflicts have
separate contracts. No engine-private disk is attached or copied. Registry/host
credentials, settings and Kubernetes state are not inferred from those transfers.
Selected images and volumes include their own contents, including any embedded
application configuration or secrets; exports do not sanitize those datasets.

## Combined application qualification — 2026-10-25

`scripts/qualify-migration-project.mjs` adds an opt-in, reproducible nonempty
Desktop-to-OrbWin application proof. It uses the production SDK for source
inventory, image export/review/import, volume export/restore and current
destination provenance review, and the existing project/mount models for the
ordered Compose handoff. The harness invokes the explicitly targeted pinned
Compose CLI for Validate, Up, Stop and restart; this is not installed/native UI
qualification of those actions.

The fixture is an original static Go HTTP server built with the repository's
pinned Go toolchain, added to a new nonce-labelled source image derived from an
already cached immutable Linux/amd64 image. It reads a restored text file, serves
binary bytes and creates destination-only persistent state. Its source project
member is created but never started; only dedicated test image-preparation and
volume-seeding containers run on the source. No registry pull, package download,
engine restart, global context/configuration change or Windows elevation occurs.
The destination runs with a read-only root, dropped capabilities, no-new-privileges
and one explicitly restored writable volume. The service is published only on a
dynamically selected `127.0.0.1` port.

The successful proof covers:

- A fresh unique application image export/import with verified rootfs and source
  retention, rather than relying on an already cached destination application.
- Nonempty volume transfer with text/binary data, mode metadata and hard/symbolic
  links, completed restore receipts and the current destination check.
- An external-volume mapping generated from the observed source member and
  appended last to the original Compose file, followed by quiet validation.
- Actual application startup and HTTP reads of restored text and exact binary
  bytes, plus an independently observed destination-only write.
- A real Stop/Up cycle in which the persistent startup counter advances from 1
  to 2. Recreating a constant marker on every start would not prove persistence.
- A second source export with identical complete file metadata/topology and
  payload digest after destination writes; neither destination-only file appears
  in the source. The source project member remains never started.
- Exact container/volume/image/reference ownership checks and network cleanup;
  both prior source/target container, volume and image inventories are restored.
  Receipts, source files and exported packages remain as evidence.

The harness records startup intent before Up so a failed/partial startup is
audited. Cleanup rechecks both daemon identities, acknowledged immutable IDs,
nonce labels and current adoption guards; it never prunes or removes project
volumes through Compose Down. An unexpected engine identity, adopted source
member or foreign container/image causes retention and an explicit cleanup
error. An abrupt kill of the harness itself can still retain fixtures; its proof
and the SDK's owning receipts must be inspected before any recovery.

Run from the repository with `ORBWIN_QUALIFY_OWNED=1` and
`ORBWIN_QUALIFY_MIGRATION_SOURCE=docker-desktop`, using
`node scripts/qualify-migration-project.mjs`. The current host matrix is Desktop
4.91.0 / Engine 29.8.0 to the recorded owned Engine 29.7.2-ce. This fixture does
not establish the full Rancher application journey or broader version support.

Final proof:
`artifacts/qualification/orbwin-project-3f489563d5a942c1bff441a73b4c9579/proof.json`.
An earlier static-server run also passed. Initial failed harness evidence is
retained: comparing serialized object key order falsely rejected equal rootfs
metadata, so both image proof scripts now compare objects structurally. The
cached Alpine BusyBox lacks `httpd`; failed startup diagnostics identified the
fixture dependency, and the static Go server removed it without a package
download. Every failed run restored baseline inventories with empty cleanup
errors. The image-only regression proof also passes after its comparison fix and
explicit bundled-helper environment update.

`p2-migration-application-20261025.json` aggregates proofs and preservation checks.
Three daemon IDs and private/project/global configuration hashes remain at the
baseline, with Windows elevation false. The installed app remained running.
This combined CLI/SDK proof advances P2-MIG/P2-CMP/P2-STO qualification; it does not
close their installed or UI gates. The project reviewer still needs an explicit
observed-image association (project members currently omit immutable image IDs),
and persistent image/project/mapping/receipt coordination remains open. Native
and installed startup/data-use, Rancher combined application migration, conflict/
abort/adoption matrices and the original full Phase 2 scope remain required.

## Initial Docker Desktop source adapter

The adapter connects only to `npipe:////./pipe/dockerDesktopLinuxEngine`, never
the default pipe, a Docker context, environment-selected host or remote endpoint.
The initial implementation matrix is Linux/amd64 Engine 29.8.0 reporting Docker
Desktop; the current host application is Desktop 4.91.0. This is a qualification
candidate, not a claim of support for every Desktop release. Rancher source
transport remains required and is not implemented by this adapter.

Before observing inventory, verify the registered OrbWin target through its own
endpoint and verify that source and target IDs differ. Source Info and Version
must match the adapter matrix; recheck the source ID after inventory. Recheck the
owned target before emitting the complete review. No partial inventory is emitted
on failure. Inventory is explicitly non-atomic and never authorizes a transfer.

The SDK uses a separate transport allowing only GET/HEAD for negotiation, identity,
inventory and selected-image inspection/save. It rejects mutating APIs before
they reach the pipe. Metadata responses are capped at 16 MiB before SDK decoding;
collections are capped at 4096 items and bounded fields. Oversized or incomplete
volume listings fail instead of silently truncating. HTTP redirects are refused.
The adapter owns its transport, idle connections and cancellation context. Review
has a 90-second deadline; selected-image export has the existing 30-minute budget.
Neither owns or stops any engine, workload or source resource.

Review projects only through bounded Compose membership labels. Preserve the
configuration-files label verbatim for a later explicit file review, without
splitting ambiguous commas, opening paths or running a project. Return named
volume membership, image IDs/tags and volume driver/scope eligibility. Do not emit
environment, credentials, arbitrary labels, driver options or host bind paths.
The plain-local flag is an eligibility observation, not an export implementation
or a claim of application consistency.

Selected-image export requires the reviewed source daemon, immutable image ID,
explicit reference and a new local `.tar`. It shares the existing streaming
archive writer, full archive/config/layer verification, free-space checks and
no-overwrite publication. Recheck source identity/version and selected reference
before Save and before publication. Cancellation or failure retains source data;
process loss can retain a named partial file. The export result records source
identity and intended target, but the archive itself remains a Docker archive.
An export is not an import or a completed migration. Import uses the separate
owned-engine checksum/tag-consent/readback/journal transaction. No source tag is
changed, no source helper is created and no source cleanup is needed for export.

## Durable source-image export acknowledgement

Source-image exports need their own host acknowledgement because an archive
checksum alone cannot identify which reviewed engine/image produced it. Before
Save, the fixed Desktop/Rancher adapter verifies the source tuple and flushes an
exclusive new intent under `migration-image-exports`, with intended target,
immutable source image, selected reference and new archive path. An existing
process lease owns the writing client; no engine resource is owned by this record.
The only subsequent event acknowledges a fully validated, published export with
its exact archive SHA-256, byte length and bounded unique archive config IDs.
Flush it before reporting success. Source root IDs and archive config IDs remain
distinct. No archive/environment payload is copied into the receipt.

An intent without a complete acknowledgement is uncertain about publication:
process loss or acknowledgement failure can leave a partial or a complete archive.
Retain both receipt and named files for inspection; never retry, overwrite, delete
or resume automatically. The acknowledgement records historical export, not
current file presence, import success or a current source snapshot. It grants no
source-engine cleanup or startup authority. Failed intent persistence prevents
Save; failed acknowledgement persistence prevents a successful result. This is
the export's owning record, not a second ledger for existing import transactions.

Readers must validate profile/identity, phase ordering, token/filename and exact
bounds. Incomplete/replaced/nonregular/torn receipts cannot establish origin.
Legacy exports without a new receipt remain usable archives but do not gain
source provenance from matching names or config IDs alone. Later linking to an
existing import must match target, exact archive checksum and its config-ID set;
it cannot infer current image/tag ownership from a completed receipt.

The image-history observer reads only the owning export/import journals. It
matches the intended target, exact archive checksum and complete unique config-ID
set, and shows the import's recorded phase and verified-config count. A matching
intent is not a completed import. It never opens archives or engines, acquires
job leases, changes receipts or grants retry/cleanup authority. Invalid source
receipts expose only their filename token; invalid or torn import receipts are
counted as unavailable and cannot establish links. Observation is not a snapshot
across journals; refresh can reveal later acknowledgements. Directory enumeration
is bounded to 8192 entries and 256 receipts per side, source reads to 64 KiB each,
import reads to 4 MiB each with a shared 32 MiB actual-read budget, and output to
2 MiB. Budget exhaustion fails the whole observation without partial results.

Qualification on 2026-10-25:

- `orbwin-migration-3a56671191ff43e0a9620504e8db5d80/proof.json` verifies
  Desktop receipt intent/acknowledgement against the actual archive and import,
  source preservation, target rootfs and exact fixture cleanup.
- `rancher-image-receipt-8c14fe73893d4774b8e22e02b1c30fe3/proof.json` verifies
  the Rancher source/WSL tuple, immutable cached BusyBox image, archive checksum
  and independently inspected config IDs. This read-only export does not import
  or mutate the source. Initial harness attempts lacked the required bundled CLI
  environment and assumed an absent Alpine cache; both failed before Save.
- `orbwin-project-06e47580f97f497d90baa5b05058b188/proof.json` adds receipt
  verification to the combined Desktop image/volume/Compose application proof:
  startup, restored data use, restart persistence and baseline fixture cleanup.
- Windows tests cover intent-before-save, unavailable intent storage, failed
  acknowledgement writes, cancellation, invalid export results, response loss,
  repeated writes, strict/bounded receipt parsing and foreign receipt refusal.
  Windows race checks pass. Linux UID-1000 checks cover portable validation and
  receipt parsing; writing/lease cases explicitly skip because export ownership
  is implemented by the Windows SDK client.

The aggregate is `artifacts/qualification/p2-image-export-receipt-20261025.json`.
No native UI interaction or new installed acceptance is claimed for this change.
Derived import-history links, restart-visible UI and combined project accounting
remain to be implemented; the receipt alone is not a completed migration.

### Image-history linkage qualification — 2026-10-25

`migration-image-history TARGET` now derives source export/import links directly
from the owning journals using the contract above. The native operation treats
it as host metadata and does not require a live engine. The Images and Containers
migration controls expose the same saved history, including source/image,
archive checksum, unacknowledged publication and linked import phases. Refresh,
bounded reads, cancellation, empty/error messaging, Escape and focus containment
are implemented. These observations never offer automatic retry or removal.

`orbwin-project-c313aba9c33b4530ba437fcea4664417/proof.json` qualifies the
combined Desktop application transfer with an exact completed import link. Its
real image/volume/restore receipts were intentionally written to normal app
storage through the harness's explicit `ORBWIN_QUALIFY_RECEIPT_TRANSIENT` opt-in;
the owning journals and archives remain after exact fixture cleanup. The native
development app was launched after cleanup and displayed source export
`e754c114f0756c8b3495c8a9e9c9fcf5` linked to completed import
`c20f52dc8fc9cad28657081c43f490ae`, with 1/1 configs verified. The table was
visually reviewed at the normal 1280×850 client size. Tab/Shift+Tab containment,
Escape returning focus to the trigger and keyboard reopening passed. See
`p2-image-history-ui-20261025.json`; its record contains observed accessibility
text, not a synthetic history fixture.

`orbwin-rancher-image-06dcf5dd44e819a6c64e21a87318356e/proof.json` adds a real
Rancher export/import linkage, source preservation, rootfs comparison, source
adapter cancellation and exact fixture cleanup. The final SDK independently
reads this same historical link after cleanup in
`p2-image-history-rancher-current-sdk-20261025.json`. A final Desktop image-only
regression also passes in `orbwin-migration-3ea2f4fcf35449f8a96e8f2b8b6841ef`.
No current tag/image ownership is inferred from any of these retained receipts.

Go suite/vet and focused Windows race checks pass. Portable history/parsing and
budget checks pass under ordinary WSL; Windows writer/lease tests explicitly
skip there. Tests cover target/checksum/config-set mismatch, unfinished versus
complete imports, torn-record exclusion, unchanged receipt bytes, aggregate read
limits, receipt-count limits and refusal of an oversized linked response without
partial output. 97 JavaScript tests, frontend/SDK/desktop builds and 57 ordinary
Rust tests pass. One concurrent Rust run during application qualification had
four process-test timeouts; two subsequent serial runs passed all 57 tests.
The cause of the concurrent timing failures is not established, and this is not
a resource/contention qualification pass.

Aggregate evidence is `artifacts/qualification/p2-image-history-20261025.json`.
Three daemon IDs and private/project/global config hashes are preserved without
Windows elevation. Only the additional development window was closed; the
original installed app remains running. Native empty/error/cancellation and
unverified-row UI cases, actual offline-engine UI, the complete restart-safe
project/image/volume/mapping workflow and installed qualification remain open.

## Selected-source volume export contract

The volume path uses a separate, explicit maintenance client; the image/inventory
adapter stays read-only. Before any maintenance mutation, require the distinct
reviewed source and registered target, a supported plain local volume, frozen
metadata fingerprint, no running writable Docker volume users and a new local
package destination. Recheck source identity, metadata and quiescence during the
transaction and before publication. External writers must remain stopped; Docker
does not reserve volumes against other clients.

Use only the verified bundled static file-helper asset, imported into an untagged
scratch image with a unique ownership token. No registry pull or selected workload
image execution is allowed. Its temporary container has no network or daemon
socket, a read-only root, bounded CPU/memory/PIDs, only DAC_READ_SEARCH and a
read-only bind of the API-reported volume data directory. The path must exactly
match the verified daemon data root's ordinary local-volume arrangement. A bind
mount with mountpoint creation disabled avoids silently creating a missing named
volume between preflight and helper creation. No arbitrary renderer path is used.
The helper's hold mode expires after the export deadline; normal cleanup removes
only the acknowledged, provenance-checked helper and its untagged scratch image.

Flush a per-job intent and exclusive Windows process lease before import/create;
flush each acknowledged immutable image/container ID before further use. Uncertain
acceptance retains the ownership receipt and never triggers broad cleanup or a
blind retry. Explicit recovery must recheck the source, inactive lease, exact
token/name/IDs and isolation metadata, and must refuse adopted/changed objects.
A missing unknown-ID create/import is not proof that a late result cannot appear.
Recovery cleans only tools; it never resumes export, removes source volumes,
changes source tags or removes/overwrites a published host archive.
The registered target ID is checked against the host receipt; source cleanup
does not require the owned daemon to be running. The source itself must be
reachable and match the recorded identity/version. A created but never started
helper still needs explicit receipt-based cleanup; the hold deadline applies
only after its process starts. A receipt write/flush failure blocks subsequent
cleanup mutations rather than appending contradictory phase acknowledgements.

Reuse the portable volume parser, xattr refusal, full file/metadata verification,
bounded streaming, headroom checks and no-overwrite package publication. Source
volume export does not create an OrbWin destination volume. The existing reviewed
restore transaction separately creates and validates a fresh isolated destination;
source volume, project reuse and target restore are distinct recorded steps.

## Remaining integration and qualification

### Source-to-restore provenance (implementation in progress)

The existing payload SHA-256 identifies concatenated regular-file bytes, not
the complete dataset: empty volumes and metadata-only changes can share it.
New export results and restore intents therefore also retain a SHA-256 of the
validated, canonically serialized version-1 manifest, with the domain prefix
`orbwin-volume-manifest-v1\n`. That manifest includes original engine/volume,
creation time, payload hashes and every supported entry's path, kind, ownership,
mode, timestamp and link topology. This digest identifies the semantic manifest;
it is not a claim to hash the exact package-file encoding or authenticate its
author. Existing full package validation still checks every payload byte.

The restore's existing append-only intent owns these fields and flushes them
before creating the destination. No second execution ledger or orchestrator is
introduced. New fields are all-or-none and validated when reading receipts.
Legacy receipts remain valid recovery records but carry no stronger provenance;
never infer a source linkage from their payload checksum alone. A retained
manifest digest records what was restored historically, not the current contents
of a writable destination or an authorization to attach or delete it. Later
combined migration review must independently check source/export acknowledgement,
matching source identity and manifest digest, complete restore, and current
target ownership. An unacknowledged or torn record cannot establish linkage.

The source history response now derives matching restore observations from those
existing owners. Match the target daemon, original engine/volume, manifest digest
and payload digest; retain the restore transaction, destination name and recorded
phase. Partial restores remain explicitly partial; `complete` means the historical
restore verifier acknowledged completion. Legacy, malformed, replaced/nonregular
or torn restore files are omitted from linkage, not upgraded from payload hashes.
Reads take no leases, perform no engine calls and write no receipts. In addition
to the existing source-history budget, restore scanning permits at most 8192
directory entries and 256 candidate receipts of 64 KiB each. Exceeding a budget
fails the response without publishing partial results. The response remains
bounded to 2 MiB, including all links. Current target ownership/data and mapping
remain separate checks; this is not automatic migration completion.

2026-10-25 qualification: 92 JavaScript tests, 57 ordinary Rust tests, full Go
suite/vet, scoped receipt/identity race tests and frontend/desktop builds pass.
The live Desktop and Rancher volume proofs each export nonempty binary/Unicode
data with numeric ownership, modes and hard/symbolic links; the export result,
new restore intent/result and derived history agree on source identity and full
manifest digest. Both restored-tree comparisons, source-preservation checks,
wrong-metadata/existing-destination refusals, abrupt-client-loss recovery and
exact fixture cleanup pass. Proofs are
`orbwin-migration-volume-b217e195fd4b8c71dd6acb6a64884cf0/proof.json` (Desktop)
and `orbwin-migration-volume-d2ae6a4955746e1c8fc7fa3f759e8cda/proof.json`
(Rancher), under `artifacts/qualification/`. Both report empty cleanup errors.

Native UI checked the two retained legacy source receipts: both correctly show
manifest provenance unavailable and infer no linkage. The added column initially
made recorded state too narrow; the dialog/table widths and column minima were
adjusted and visually checked at 1280×850. Escape restores focus to the history
trigger. Positive linked-row rendering, smaller-window checks and installed
qualification remain pending. Only the development UI was closed. The retention
record verifies three daemon identities and private/project/global configuration
hashes, with Windows elevation false. Aggregate evidence is
`p2-migration-provenance-20261025.json`. Project/image/receipt coordination and a
complete application migration remain open; this proof alone does not close
P2-MIG or Phase 2.

Source-volume history is a bounded local receipt observation,
available without dialing either engine. It must never resume export or infer
package publication, import completion or current workload state from a receipt.
The existing receipt remains authoritative; no second transaction ledger is added.
List only exact token filenames, reject nonregular/replaced/oversized files, bind
valid metadata to the registered target and expose no raw export/configuration
contents. Partial/unreadable receipts appear as uncertain rows for explicit
review, never successful jobs. Reads may race an active append; recovery still
acquires its existing exclusive process lease and independently verifies exact
source/tool provenance. History does not authorize cleanup. Bound directory,
receipt-count, total-byte and response work; failures emit no partial response.

The Volumes page opens an explicit resizable history dialog; opening it reads
the current saved receipts. It presents source/volume, destination, recorded phase
and export acknowledgement separately. Cleaned and uncertain receipts cannot
offer automatic tool recovery; other rows open the existing exact-transaction
review. Closing/cancelling aborts only the read, and late results cannot reopen
or overwrite a newer dialog. Tab is contained; Escape returns focus to the
trigger. No credentials or raw export-result payloads are included.

`artifacts/qualification/p2-source-history-20261025.json` records full SDK tests,
74 frontend tests, 18 operation tests, frontend/desktop builds and a native adapter
read of two existing Desktop/Rancher receipts. All four receipt/lease files remain
byte-identical. Unit cases cover torn/nonregular receipts, wrong-target metadata
redaction, the cleanup-versus-export distinction and the 256-receipt limit with no
partial response. Native UI review found and corrected the original narrow-pane
layout and transient-button focus issue. The final dialog shows both cleaned
receipts, disables recovery and passes forward/reverse Tab and Escape focus return.
Only the development window was closed; installed Phase 1 remains open. Three
daemon IDs and private/project/global config hashes match the existing baseline.
The history read performs no engine API call or host-file write. This is not
combined source-to-target accounting, an installed/offline engine outage test or
native UI coverage of torn/active receipts and recovery confirmation.

Complete the combined migration journey and source-to-target durable migration
accounting, source-volume history/recovery ergonomics and reviewed project reuse.
Rancher's explicit WSL Unix-socket adapter now has an initial Engine 29.5.3 /
Ubuntu 24.04 qualification candidate. Its active Windows application version
still needs definitive mapping; two Rancher applications are installed. Never
fall back to the ambiguous default Docker pipe. Source quiescence, capacity, conflicts, safe
abort, interruption, end-to-end validation and installed three-engine preservation
remain acceptance gates. Real user resources require their own selection; native
tests use disposable exact-owned fixtures.

## Reviewed project-file reuse

### Observed member images

Source inventory now projects each member's API-reported immutable image ID,
without substituting its mutable image-reference string or exposing environment
and arbitrary labels. Missing/legacy IDs remain unknown; malformed nonempty IDs
refuse a complete inventory. This source engine identity can differ from a Docker
archive config digest or the destination engine's image root after import.

Project review groups the IDs actually observed in ordinary members, with their
service labels and corresponding source-inventory tags. One-off tasks do not
establish ordinary-service requirements; replicas with different images remain
separate observations. A missing image summary disables its export and calls for
refresh. Unknown IDs are never inferred from service names, files or tags.

Each available image opens the existing explicitly reviewed Desktop/Rancher image
export operation, choosing an observed reference or the immutable ID. Export
rechecks source identity and reference association. It does not import the image,
rewrite Compose images or authorize startup. Export by ID alone does not promise
a tag handoff. Existing archive checksum/platform/conflict review and import
receipts still own the destination transaction. Source project snapshots are
observations, not an atomic transfer plan or proof that mutable tags/files match.

Opening image export from project review hides that dialog while retaining its
in-memory snapshot and parent-owned Windows mapping selection/save association.
Returning through the project-review trigger reopens that snapshot; Refresh or
Close invalidates the review as before. Linux connection/staged-file UI state is
not retained across that child unmount and must be explicitly inspected again.
This is device-session continuity, not durable combined migration accounting.

2026-10-25 qualification: 94 JavaScript tests and 57 ordinary Rust tests pass,
alongside the full Go suite/vet, scoped inventory race tests and frontend/desktop
builds. Coverage includes malformed nonempty IDs, legacy/missing identity,
different replica images, ordinary versus one-off membership, missing image
summaries and a frozen tag list. The combined application proof now requires its
source project to expose exactly the fixture's immutable image and a matching
source image summary before export. That updated proof passes at
`artifacts/qualification/orbwin-project-53f0638658954c3f8a323241756f4b0d/proof.json`,
including application reads, writes, restart persistence and exact cleanup.

Native UI at 1280×850 qualified the new image section, service/ID/reference
display, keyboard ID-only selection, export handoff and a completed new archive.
Reopening project review retained the selected source project without another
inspection. The 3,958,784-byte archive has SHA-256
`a3526c56404f7b2e2f7d253258c39e4b8eb23ce63ac1078fb65b8bf794385938`;
independent SDK archive review verifies Linux/amd64, one config/layer and no tag
handoff. The never-started source fixture was removed after exact identity/label/
state checks; both prior container/volume/image inventories were restored and the
original fixture file was unchanged. No source or target image was removed or
imported in this UI fixture. The installed app stayed running and only the
development UI closed. Three daemon identities and private/project/global client
configuration hashes remained at baseline with Windows elevation false.

Evidence: `p2-project-images-20261025.json`, its linked UI state and archive
review. Native UI coverage of mixed replica/missing-image states, retained Windows
mapping choices across export, Rancher project-image export, smaller windows and
installed use remains pending. Durable export/import/project/mapping/receipt
association and the complete original Phase 2 acceptance are still open.

Source inventory exposes only bounded Compose project/service/config-file/
working-directory labels, one-off status, named-volume names and presence flags
for explicit env-file labels and host binds. It never copies env-file paths or
values, host mount paths or arbitrary labels. The desktop validates every member
and groups by exact returned project label. Source labels remain untrusted hints;
inventory is not atomic and is refreshed only by explicit inspection.

Project review reuses the existing coherent Windows Compose discovery rules:
all observed members must agree on ordered config files and working directory.
Linux/UNC paths, conflicting/missing labels and explicit env-file selections
produce an empty editable file set with a useful explanation. Profiles are never
inferred, and one-off containers do not establish ordinary service membership.
Service/volume observations are shown separately. The source WSL connection is
not assumed to be the distro that owns project files. Linux projects use the
existing explicit WSL project connection and original-path workflow.

The handoff opens the existing Compose **Validate** review dialog. No file is
opened and no workload is created until the user reviews and submits it; only
quiet validation is offered from source review. Users can remember successful
inputs and use the existing separately reviewed Up later. Current OrbWin project
name collisions are displayed as observations, not reservations; rename for an
independent destination. Ports, external volume names and bind data require
explicit review before startup. Host binds can share writers with the source.
Volume export/restore and editing the destination Compose mapping remain separate
steps; image transfers do not imply project/data/settings/credential migration.
This handoff is not yet durable combined migration accounting or a reservation
against external target clients. Closing/changing source clears the selection;
late/cancelled source reads cannot replace the new review.

The initial native proof uses a real stopped Compose fixture on Docker Desktop
4.91.0 / Engine 29.8.0. Compose creates one exact-labelled container and empty
volume from two ordered Windows files with spaces/Unicode in their names; the
container is never started. Source inspection and the pure project review retain
the file order, service and separate volume name. Native desktop review hands off
to the existing editable Compose form and completes quiet validation on OrbWin.
No target containers are created. Source identity, labels and created state are
rechecked before exact non-forced fixture container/volume cleanup. The source
files remain available, without a claim of before/after byte comparison. No
remembered hint is saved for this fixture.

Evidence: `artifacts/qualification/p2-migration-project-20261025.json`, linked
native/UI results and screenshots. Full SDK tests and the source-inventory race
check pass; 65 JavaScript tests and development builds pass. Migration controls
are loaded separately with an accessible loading state; the initial production
JavaScript chunk is about 490 kB, clearing the previous Vite heuristic warning.
This is build-size evidence, not runtime resource-budget acceptance. The installed
app is untouched, three daemon IDs/global configs are preserved, and no Windows
elevation or trust writes are performed. Remaining project gates include source
drift/external target races, explicit env-file workflows, original WSL project
handoff, large groups, integrated data/config mapping, durable combined job
accounting, and installed end-to-end application migration.

The initial original-WSL project handoff now has its own implementation and
qualification record: `artifacts/qualification/p2-migration-wsl-20261025.json`.
It explicitly selects the project distro independently of source-socket access,
inspects an already-connected export, preserves coherent ordered Linux paths,
and opens the existing editable quiet-validation form. The submitted request
retains the reviewed registration and exact project root. No project connection
is replaced and no destination application is started by this handoff. See
[WSL integration](wsl-integration.md) for native evidence, the verified installed
tool-directory adaptation and remaining lifecycle/legacy-reuse gates. Combined
migration accounting, explicit env-file/data mapping and installed end-to-end
application migration remain required.

## Rancher WSL connection contract

Rancher's current source has no dedicated Windows pipe. Its initial candidate
uses only `unix:///mnt/wsl/rancher-desktop/run/docker.sock`, reached through an
explicitly selected ordinary WSL 2 registration. Read registrations from HKCU
without starting anything; refuse protected engine distributions. Actual inspect
can start the selected ordinary Ubuntu 24.04 distro and uses Linux root for the
existing socket. It never starts Rancher or changes socket/group permissions.

Each short-lived connection invokes the bundled, hash-verified upstream Linux
Docker CLI's `system dial-stdio`. The public ELF is copied to a private root-owned
temporary directory and reverified before execution; fixed shell code receives
only separate GUID/machine/digest/count arguments. Empty private CLI config and
clean environment prevent credential/context inheritance. No workload data,
credential or settings file is copied. Verify the registration immediately before
launch and bind all connections to the observed machine ID; export requires the
reviewed machine and source daemon IDs. The current engine candidate is Rancher
Linux/amd64 29.5.3 and its reported Rancher WSL operating-system identity.

Unmodified Docker wire bytes travel through bounded-backpressure process pipes.
`net.Pipe` supplies real resettable deadlines; manually owned OS pipes preserve
buffered output after child exit. One wait owner reaps the child; close and owner
cancellation close both pipes, allow bounded normal exit, then kill only that
Windows child if necessary. A Linux timeout bounds abandoned guest clients. The
shell removes only its exact staged public binary/directory on normal exit.
Metadata retains the existing pre-decode 16-MiB cap and mutation-denying request
hooks. This is a private migration RPC connection, not a published endpoint or
interactive-stdin/independent-half-close transport. The Images-page source picker
requires explicit WSL selection and freezes the verified GUID/machine for export.
Rancher volume maintenance reuses the same source-volume protocol; its current
integration and qualification status is recorded below.

For fixed-output maintenance exec, the SDK must use the same WSL dialer for
ordinary HTTP and hijacked streams. Moby v0.6.2 extracts its hijack dialer from a
concrete `http.Transport`; a custom outer RoundTripper would lose it. A concrete
transport registers the bounded/scope-wrapped ordinary HTTP path, retains the
fixed dialer, and applies the exact job request hook to both paths. Read-only
source clients still refuse all exec requests. No TCP/default endpoint fallback
or interactive-stdin capability is introduced. Volume receipts bind the source
kind, registration/name/machine and engine profile; recovery reopens only that
connection and refuses substituted registrations.

## Current evidence

`artifacts/qualification/orbwin-migration-094e6d0a14f1460fba870e9ca55bbacd/proof.json`
records a real disposable Desktop 4.91.0 / Engine 29.8.0 to OrbWin Engine 29.7.2
image transfer. The source review observed 59 images, 23 volumes and one Compose
member before fixture creation. A local scratch fixture was created with an exact
qualification label, exported through this source adapter and imported through
the reviewed target transaction. The source image ID/rootfs were preserved and
the destination rootfs diff IDs matched; archive checksum and settled import
receipt were verified. Stale source identity and an existing archive destination
were refused. Exact acknowledged fixture tags/recovery references were removed
without force or pruning; cleanup errors were empty. Global Docker and Kubernetes
configuration hashes were identical. This small fixture is not evidence of large
archives, source volume transfer, project migration or installed acceptance.

`artifacts/qualification/p2-migration-unit-20261025.jsonl` records Windows race
tests for write refusal, bounded metadata/body lifetime, source/version drift,
metadata projection, oversized/partial inventory and changed identity/tag before
archive publication. Full SDK Go tests and vet also pass.

The initial Images-page native journey also inspected the real source, selected
a separate disposable fixture and exported it to a new archive with checksum
`915178769974ef2ae90df7ded886227a71e5dbb726d25c6f3e28feb5082a8342`
(8192 bytes). `p2-migration-ui-review.png`, `p2-migration-ui-tab-close.png`,
`p2-migration-ui-confirm.png` and `p2-migration-ui-complete.png` capture review,
keyboard containment/return and completed typed native dispatch. Closing/reopening
requires a new source review. `p2-migration-ui-cleanup.json` confirms source
ID/rootfs preservation before exact fixture cleanup. The development window was
closed; installed Phase 1 was untouched. This does not qualify the combined
migration journey, all window sizes/client states or installed acceptance.

### Desktop volume transfer evidence

The initial source-volume path reuses portable format 1 and the existing owned
restore transaction. A scratch maintenance image is built from the bundled
hash-verified static helper; local-services image import retains its separate
component/executable/ledger. No source registry access is needed. The first live
attempt correctly stopped before starting a helper because Docker canonicalizes
the capability to `CAP_DAC_READ_SEARCH`. The expected profile was corrected, and
the recorded helper/image were removed through the exact recovery command.
The failed evidence remains under
`orbwin-migration-volume-ef5f8625970f280ffe2feb1a09db9a34`; recovery is recorded in
`p2-migration-volume-first-recovery.json`.

Current source/native evidence is linked by
`artifacts/qualification/p2-migration-volume-20261025.json`. A disposable source
volume with binary/Unicode files, UID/GID/modes, hardlinks and symlinks exported,
restored and matched a complete target readback. Running writable users, stale
metadata and an existing package were refused. A real Windows SDK child was
killed after its helper acknowledgement; its exclusive lease released and exact
source tool recovery settled. A controlled never-dispatched receipt with an
unavailable target also recovered without dialing/restarting that target. This
does not stand in for an actual installed/offline desktop test.

The development desktop separately inspected a disposable empty source volume,
blocked export until its metadata/quiescence review, exported a new 398-byte
package, reported tool cleanup and returned focus to the source-export trigger.
That fixture's source metadata was unchanged before exact labelled cleanup.
The development window was closed; the installed Phase 1 application was untouched.
Desktop captures precede the final additional offline-recovery routing change;
the current native helper journey, Rust checks and frontend build cover the
resulting source state. Full installed/client-state qualification remains open.

Full Go tests/vet, scoped Windows race tests, 58 JavaScript tests, 16 Rust operation
tests and development builds pass. This small-fixture evidence does not qualify
64-GiB/100,000-entry scale, guest/host disk pressure or power loss, torn-receipt
manual recovery, arbitrary external writers, changed source versions or complete
project/application migration. Target restore still needs the existing pinned
maintenance image; its fresh-install/offline provisioning remains a packaging
gate. Broader Rancher qualification, combined migration history, project reuse and the full
Phase 2 installed acceptance matrix remain required.

### Rancher image and volume evidence (2026-10-25)

`artifacts/qualification/p2-rancher-migration-20261025.json` records read-only
source inventory, image export/import with matching rootfs, source preservation,
wrong-machine/no-overwrite refusal, real owner-cancellation child reaping and a
separate native Images-page export. The UI archive is 8192 bytes, SHA-256
`7139f8e853f27ef42ea0d7201944a6e355a0bf69a34d68a3b7fe13e5f434f982`.
The disposable source image was checked against its immutable ID, qualification
label and rootfs before exact cleanup. The development window was closed and
the installed Phase 1 window remained open. Desktop image/volume regression
tests passed; all three daemon IDs and global Docker/Kubernetes config hashes
were preserved, with no staged WSL connection directories remaining.

The qualified source identifies itself as Rancher Engine 29.5.3 / API 1.54,
Linux/amd64, through ordinary Ubuntu 24.04 registration
`231810b0-fa21-48a8-b0a7-86f9a1360029`, machine
`5563914b91c14609814e3657a227cff1`. Windows uninstall metadata lists both Rancher
Desktop 2.0.0 and Rancher Desktop 1.24.0; neither entry alone proves which
application owns this socket. No source application-version claim is made.

`orbwin-migration-volume-9e13958ac14254248dbef056ab843dd7/proof.json` adds the
real Rancher source-volume transaction. The same binary/Unicode/UID/GID/mode/
hardlink/symlink fixture and full restored-tree comparison used for Desktop pass.
Running writable users, stale metadata and an existing destination are refused.
Killing only the SDK process after the helper acknowledgement releases its lease;
source-only exact receipt recovery settles. A controlled unavailable target
receipt also recovers without a target dial. Source metadata and tree are
preserved and cleanup errors are empty; no staged WSL directories remain after
this abrupt-parent-loss test. This does not prove every timing of process loss.

The stream-client contract test verifies ordinary metadata and byte-exact hijack
output through the fixed dialer, and refusal of unacknowledged exec, volume
creation and read-only-client exec before any request reaches the source. The
source receipt tests reject foreign source kinds/versions and incomplete,
protected or Desktop-mislabelled WSL tuples. Full Go tests, vet and scoped race
checks pass. The desktop now offers Rancher volume selection and freezes the
connection tuple for metadata review/export; source-choice changes clear the
volume review. Frontend build, 61 JavaScript tests, 18 Rust operation tests and
development Rust build pass. The native Volumes-page journey independently
selected an empty nonce-owned Rancher volume, blocked export before metadata
review, exported a new 406-byte package and recorded exact helper/image cleanup.
`p2-rancher-volume-ui-result.json` records source metadata preservation, package
SHA-256 `a0b06ecc2fcf3681ec894109abfafefe245de9b542fcc231e26e3701c4666a42`
and exact fixture cleanup. The saved receipt ends in `cleaned`. The development
window was closed; the installed app remained open. Summary evidence is
`artifacts/qualification/p2-rancher-volume-20261025.json`, including current
Desktop-volume and Rancher-image regressions, manifests and preservation checks.
The initial frontend chunk is 503.62 kB;
the build warning is retained and resource-budget acceptance remains open.

### Combined Rancher application harness (source only)

`scripts/qualify-migration-project.mjs` now accepts the existing explicit
`ORBWIN_QUALIFY_OWNED=1` opt-in plus source `docker-desktop` or `rancher-desktop`.
`--plan` prints the reviewed shape without filesystem/WSL/engine/SDK/native work.
Root can supply a reviewed32-hex `ORBWIN_QUALIFY_PROJECT_NONCE`; otherwise the
historical cryptographic random nonce behavior remains. An existing fixture
folder or baseline namespace is refused, never adopted.
This increment was syntax-checked and pure-tested only; it does not establish a
completed Rancher combined application proof. Historical Desktop proofs remain
unchanged. Its Desktop SDK argument ordering is preserved, while the shared
fixture now has stronger resource, identity, preservation and cleanup guards.

The Rancher source is fixed to ordinary Ubuntu registration
`231810b0-fa21-48a8-b0a7-86f9a1360029`, machine
`5563914b91c14609814e3657a227cff1`, daemon
`741eb2cb-adf5-4676-bf28-f03f68d54f60`. Literal WSL arguments use
`--distribution-id`, `--user root`, `/usr/bin/env -i`, a fixed PATH/LANG and
nonexistent HOME, checksum-pinned bundled Linux Docker, an anonymous config and
`unix:///mnt/wsl/rancher-desktop/run/docker.sock`. No distro-name/default-context
selection or shell interpolation is used. Windows artifact paths for source
`docker cp` become literal `/mnt/f/...` paths; relative/UNC/ADS/control/dot-segment
paths refuse. SDK Rancher review/image-export/volume-review/volume-export retain
production registration+machine+source/target arguments, without replacing their
own receipts or source identity checks.

The proposed Rancher cached Busybox immutable ID is
`sha256:101b4afd76732482eff9b95cae5f94bcf295e521fbec4e01b69c5421f3f3f3e5`.
Its availability/content was not inspected in this increment. Execution must
inspect that exact Linux/amd64 ID, refuse declared volumes/entrypoint, and use
`--pull=never`; no pull/tag/cache transfer is authorized by a missing image.
A nonce image fixture adds a locally compiled HTTP application. A nonce source
volume receives text/binary/hardlink/symlink data. Production SDK image export/
import and volume export/restore, owning-history link, source-tree preservation,
restored-volume HTTP reads/writes and an intentional fixture stop/up restart are
then exercised. Source project member stays created and never starts. Only nonce
fixture containers may run/stop; no engine lifecycle operation is present.

All three exact daemon IDs and the WSL machine are checked before mutations and
cleanup. Before/after snapshots use `canonicalEngineSnapshot` and
`configFingerprint` from the draft harness. Containers, images, volumes and the
complete network inspect objects are retained: Internal/Attachable/IPAM/options/
labels/peers/driver-specific/future fields are not silently discarded. Fingerprints
cover global Docker/kubeconfig/.wslconfig, OrbWin private Windows/WSL Docker config,
and the existing project-registration record. No config is restored automatically.
The baseline refuses existing nonce container/volume/image/network namespaces
before any creation/commit, preventing volume-create/tag adoption. Cleanup requires
acknowledged full container/image IDs and nonce labels, exact volume CreatedAt/
local driver/scope/options and owning transfer labels, no volume adopters, and
empty nonce-labelled project networks. No force/prune or unrelated resource
removal is used. Files/owning receipts remain as evidence.

An optional frozen native test executable is selected only with explicit
`ORBWIN_QUALIFY_APPLICATION_NATIVE` and `_SHA256`. Each call rechecks its hash and
writes a new direct-child hash-bound envelope for the root-owned ignored
`operations::qualification::migration_application_qualification::owned_migration_application_step`
adapter. Only Validate/Up/Stop are allowed. It checks the native dispatch-intent,
actual outcome, target-after identity, unchanged file checks and native-records
receipt. This is the real Rust request path, not WebView/installed/UI evidence.
Without these optional parameters, the existing targeted Compose CLI path remains
available and its proof explicitly labels that adapter.

The harness syncs intent before mutation and uses a15-minute scenario budget,
per-child180-second cap, and separate120-second cleanup/postcheck budget. A failed/
terminated/timeout client may have accepted work. It never replays a failed Up or
SDK request or claims rollback from a missing response. Partial Up cleanup may
inspect only the previously empty nonce project with exact ownership/image
checks. Unacknowledged creations/imports or adopted/changed resources are retained;
baseline mismatch/cleanup errors prevent success. Killing the harness itself can
prevent finally cleanup: retained intent/receipts and exact current resource
inspection require owner review before recovery, never a blind rerun.

Current source/runtime drift is a hard execution gate: the accepted13-patch
source lock differs from the still-qualified12-patch backend bundle. Existing
`verifyBuild` and the new Rust ordered pin binding correctly refuse that mismatch.
This harness adds no bypass or relabelling. Root must first decide an attended,
qualified matching runtime transition, freeze source/SDK/native attestations and
review the concrete nonce/fixture before actual serial execution. No backend
replacement/restart or application/native run occurred in this source increment.

Root review then corrected the direct CLI adapter's plugin provenance. Its fresh
anonymous Docker config now explicitly selects the bundled Windows plugin
directory, and the exact Compose executable is checksum-checked against the
runtime lock before and after every direct Compose call. The proof records that
hash. Pinning `docker.exe` alone did not establish which discovered Compose
plugin ran in the historical direct-CLI fixture. Those historical application
and data observations remain evidence of their recorded outcomes, with that
plugin provenance limitation; this source correction does not retroactively
qualify them. Syntax, the seven source-contract tests and plan-only invocation
pass after the correction. No engine, native bridge or fixture was executed.
