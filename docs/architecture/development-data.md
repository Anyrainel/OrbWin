# Portable development data and edits

Phase 2 implementation contract, 2026-10-24 Pacific. Owned-engine volume transfer
is implemented and under qualification. Project packaging, selected-source
migration and the combined installed gate remain open.

## Scope decision P2-D02

The Phase 2 storage outcome is portable project/named-volume export and restore,
plus explicit selected-source migration built on those transactions. Existing
engine usage, selected cleanup, attach and preservation remain. Volumes, images,
Compose projects, credentials/settings and Kubernetes state are different kinds
of state; success for one cannot imply the others were transferred.

Whole-engine offline disk backup/restore, relocation and Docker-only/full-product
reset remain separately recorded lifecycle transactions for the Phase 4 release
and upgrade work. Portable application data is needed for the accepted Phase 2
daily development/migration journey; copying engine-private disks and a new reset
orchestrator are not needed to satisfy it and have no verified standalone
upstream transaction. This disposition must remain visible in final scope
accounting. Do not expose a fake successful reset or describe a volume archive
as a whole-engine or transactionally consistent database backup.

### Compaction and host allocation disposition (2026-10-26)

P2-D02's compaction addendum selects explanatory storage help and manual host
maintenance guidance, with no integrated compact action in Phase 2. Existing
portable transfer, clone and project migration remain core Phase 2 outcomes.
The storage view distinguishes Docker logical usage, Windows volume free space
and virtual-disk file length; it does not measure physical allocated file bytes
or promise that deleting resources shrinks the host file. Relocation retains the
separate Phase 4 lifecycle disposition above.

For elective manual maintenance, first retain verified portable data exports and
record their coverage: they exclude engine metadata, cluster state and settings.
Identify the exact owned distribution and every associated disk; confirm installed
WSL/upstream version support and a workload interruption window. Do not compact
an active writable disk or treat a guessed disk filename as ownership evidence.
Microsoft's [compaction prerequisites](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/compact-vdisk)
require a dynamically expanding disk detached or attached read-only. Its
[WSL disk guidance](https://learn.microsoft.com/en-us/windows/wsl/disk-space)
includes procedures that interrupt WSL globally, affecting foreign engines and
user distributions. These OS procedures are not a qualified OrbWin multi-disk
transaction; no generic shutdown/unregister/copy recipe is provided here.

Future integrated compaction requires exact owned-disk identity, stopped/detached
proof, host capacity preflight, interruption recovery and foreign-data preservation
evidence. Current storage help and this disposition close the scope decision,
not current installed UI or data-preservation acceptance.

## Current portable format and adapter

The development implementation replaces the cached-Alpine prerequisite with a bundled, verified offline
maintenance-image archive. This retains the existing pinned Alpine tools rather
than rewriting working helper commands. The asset is Linux/amd64 only, bound to
the existing immutable OCI root, config and rootfs digest. Its Docker manifest
contains no tags and its top-level OCI index has no naming annotations. Staging
and runtime checks reject unexpected identities, tags or changed bytes. Loading
targets only the already verified owned daemon; no registry API, pull, tag write,
engine start, workload image execution or source-engine mutation is allowed.

The image is immutable shared engine cache, not a temporary per-job resource.
Concurrent clients may load the same bytes; no cache lease, separate lifecycle
supervisor or destructive rollback is needed. Only an exact NotFound inspection
permits load, and successful stream consumption plus exact post-inspection is
required before helper creation. Cancellation/SDK loss may retain cached bytes;
the next request inspects/reuses them. Existing helper leases still own container
cleanup and accept the old pinned repository reference and the equivalent exact
root digest. An uncertain load never authorizes image removal or a blind retry.
Ordinary unused-image cleanup may remove the cache; the next use reloads the
bundled bytes. The bundle/notice inventory and actual fresh-install acceptance
must be qualified before this is described as shipped offline support.

### Offline maintenance asset evidence (2026-10-25)

`maintenance-image.lock.json` records the existing Alpine OCI root/config/rootfs
pins and a canonical tag-free 3,731,968-byte archive, SHA-256
`4a785a3d0998a81d8f0bf2d4d2949bc4905b54170bd20fd4d17707525de392cf`.
`scripts/stage-maintenance-image.mjs` consumes an explicit absolute Docker Save
archive, bounds source reads, checks blob hashes, normalizes tar metadata/order,
removes top-level image names and invokes the SDK's full graph validator before
publishing the exact pinned asset. It never contacts an engine. A source with
reordered entries, changed timestamps and deliberately added names produces the
same bytes; a corrupted blob refuses without replacing the staged asset.

Native dispatch verifies the bundled bytes against the compiled pin on a blocking
worker before its existing cancellation gate. The SDK permits a load only after
an exact NotFound image inspection. It freezes at most 8 MiB of verified archive
bytes so bundle replacement cannot change a subsequent Load. It pins the expected
compressed layer and Docker manifest before the general image reader inflates
anything, then verifies the full OCI/config/layer graph. Load output is capped,
drained and closed; errors or failed post-inspection refuse helper creation.
Permission, transport, malformed identity/platform/rootfs and cancellation failures
never trigger fallback loads. Helper container leases/recovery remain unchanged
apart from recognizing the equivalent exact immutable image reference.

Debug preparation rechecks the original target's ID/running/paused/start-time
after image preparation, before creating a namespace-sharing helper. A mocked
restart during that interval refuses without a create. The live monitor still
handles later target changes. Files, portable transfers, clone, volume editing,
image-storage preflight and Debug Shell share this cache. Source-migration scratch
helpers retain their separate per-job image receipts and never load this asset
into another engine. The Debug Shell Pull prerequisite and stale file-browser
instructions are removed.

Evidence is linked by `artifacts/qualification/p2-maintenance-20261025.json`.
The actual owned-engine Load/readback test injects only the initial inspection's
NotFound; it does not delete a baseline image or use an empty engine. Existing
tags are byte-for-byte equivalent. Unit/fault checks cover unavailable/wrong
assets, transport/cancellation failures, bounded/malformed/error Load responses,
missing readback and concurrent bundle replacement. Full SDK tests, vet, scoped
race checks, 75 frontend tests, 19 operation tests and development builds pass.
Native portable round trip, metadata/attribute audit/recovery, standalone file
read/edit/copy, volume clone, reviewed image import and Debug Shell TTY/namespace/
target-preservation checks pass. The portable proof predates the final private
archive-copy/preflight changes, and the other cached-helper proofs predate only
unused cold-load preflight refinements; current fault tests and real Load/readback
cover those deltas. No claim of a complete installed or empty-engine test is made.

The three daemon identities, private/project/global config hashes, two original
volumes and 14-image inventory are retained, with no transient helpers remaining.
Installed Phase 1 stays open; no Windows elevation or engine lifecycle request was
used. Initial package preparation verified/staged this asset but correctly refused
with one unresolved notice entry for the complete Alpine image. Its embedded APK
database records 15 packages from ten origins and contains no license files.
Exact-revision APKBUILD recipes for all ten origins have been retrieved and hashed;
package notices and source accounting were then completed as described below.
No package manifest/installer was produced by that failed stage.
Fresh-install/offline product qualification remains open.

### Maintenance image source distribution (2026-10-25)

The notice/source packaging gate now passes. Collection derives the package
database from the exact checksum-pinned image rather than a separate version
guess. For all 15 installed packages it records ten aports origin revisions,
checks recipe-directory files against their Git blob IDs and all recipe inputs
against their SHA-512 lists. APKBUILD is parsed as data and never executed.
BusyBox's explicit static pkg-config dependency also leads to retaining utmps and
skalibs source/build contexts from that recipe's aports revision. Those contexts
are recorded as such; historical binary build-tool versions and an identical
binary rebuild have not been demonstrated.

`scripts/collect-maintenance-sources.py [--offline] [--cache <artifact-directory>]`
acquires/verifies the bounded source cache. It refuses unverified downloads,
nonliteral source URL expressions, escaping paths and changed cached files.
`scripts/bundle-maintenance-sources.py [--cache <artifact-directory>]` rechecks
the collected hashes, preserves complete upstream archives plus aports files,
collects original notices/headers and supplementary standard license texts, and
produces the frozen `maintenance-sources.lock.json`. Existing differing pins,
notices or archives refuse replacement for explicit review. Python 3.13.1 was
used for this collection; ordinary packaging needs the staged artifact, not a
Python runtime or a network connection.

The resulting `maintenance-sources.tar.gz` has 190 source files, 17 notice files
and a source inventory: 208 members, 24,226,031 compressed bytes, SHA-256
`dd470cff7a02302e22764ddeba41b8bef8c16f7017964520c773143d9b739096`.
Every member, size and checksum was independently read back. Original source
archives stay byte-for-byte intact; outer tar metadata is normalized. Repeating
collection with network disabled and rebuilding the archive reproduced its pin.
MIT template placeholders are retained as standard text, never substituted with
invented copyright holders. Component-specific notices and complete sources
remain available. This is source/notice inventory evidence, not a legal review
or a claim of binary reproducibility.

`scripts/maintenance-notices.mjs` binds package revisions/versions, build inputs,
notice coverage and the whole source archive to the image pin before publishing
any package copies. Checked bytes are frozen before copying. Wrong-image,
incomplete-input, changed-file, unsafe-path and mismatched-stage cases refuse.
The collector stages `SOURCES.tar.gz` and `SOURCE-INVENTORY.json` alongside the
notices; all license/source resources now appear in the package checksum manifest.
Four Python parser/cache/offline tests and 78 JavaScript tests pass. Package
preparation resolves all 773 dependency entries with zero missing notice entries.
All 1,102 staged runtime resources, including 1,081 license/source files, match
their manifest hashes (817,586,731 bytes inspected).

Evidence: `artifacts/qualification/p2-maintenance-distribution-20261025.json`.
The optimized application and NSIS candidate build pass. The 488,999,994-byte
installer is preserved under
`artifacts/qualification/p2-candidate-0784bea91f61-20261025/`, SHA-256
`0784bea91f61a24eec612c1158f58dbb50a4a5c6294b56e7530226c713687a0f`.
Its generated installer script references the source archive. This is an
uninstalled, unqualified Phase 2 candidate at the existing 0.1.0 version, not a
release or completed acceptance. The installed Phase 1 app remains in place;
three-engine identity and private/project/global config checks remain unchanged.
No elevation, trust write or engine lifecycle action was used. Installed upgrade,
fresh-cache/offline, resource budgets and the complete Phase 2 scope/decision
matrix still require qualification.

Version 1 is an uncompressed `ORBWIN-VOLUME-1` package: bounded JSON manifest,
then concatenated regular-file payloads with individual and aggregate SHA-256.
The manifest carries source daemon/volume, export time, relative paths, entry
types, byte offsets, modes, numeric UID/GID and modification times. The format
does not carry engine disks, volume driver settings, application credentials as
separate configuration, or ACL/xattr guarantees. Application file contents may
themselves contain credentials and require appropriate handling.

Support is ordinary local volumes, up to 64 GiB of logical file bytes and 100,000
entries. UTF-8 relative names, directories/regular files, contained relative
symlinks and hard links to regular files are accepted. Absolute/escaping links,
link ancestry, traversal/duplicate entries, special/sparse objects and extended
attributes/ACL records refuse. Linux atime/ctime and textual user/group lookup
names are not preserved; numeric ownership and modification time are verified.
No archive path is extracted on Windows. An exclusive host payload spool plus
final package need roughly twice the data size; incremental available-space
checks maintain headroom. Publication is an exclusive NTFS hard link after flush.

Restore validates the entire manifest/checksum before creating a new destination.
It creates a random ownership token and flushed per-transfer receipt before
VolumeCreate, checks returned ownership, metadata and all container references,
then mounts the selected destination in a bounded tools container. Guest free
space includes estimated metadata/small-file overhead; no space reservation is
claimed. A validated tar stream has only fixed `source/`-prefixed paths, with
directories/files before links. CopyToContainer targets the disposable tools
root so Moby restores the selected volume root metadata too. The selected data
mount is the only writable user-data mount. No imported executable is run.

Archive-header checks alone cannot establish absence of extended attributes.
The Moby archive writer examined in `go-archive@v0.3.3/archive.go` only reads
`security.capability` into tar headers. Export now runs the verified static guest
helper over the read-only selected mount before and after collection; it uses
OpenRoot-confined parent descriptors and llistxattr without following final links
to refuse any attribute/ACL-bearing entry. Restore repeats that audit before
acceptance. Tools roots are writable disposable layers only to stage this trusted
binary through Docker's archive API; export's selected source mount remains
read-only. DAC_READ_SEARCH is the only added capability, for inaccessible owned
files; no socket/network/host or target-PID mount is provided. Other read tools
and Debug Shell retain their separate documented boundaries.

A second archive read verifies every entry/content/inode group/metadata before
the receipt can append `complete`. Tar's choice of hard-link representative is
irrelevant to verification. Root timestamp mismatches in the first live attempt
were detected and refused; the fixed-prefix route passed the round trip rather
than weakening verification. Failed records remain in qualification artifacts.

Receipts append flushed phases `intent`, `created`, `importing`, `verifying`,
`complete` or `discarded`; a separate exclusive Windows lease identifies an
active client and is released on process loss. Volume recovery reads are explicit.
They show uncertain absent creates and replaced names without deleting either.
Discard rechecks inactive lease, receipt/token/source hash, ordinary-local-volume
metadata and absence of **any** external container reference, including stopped
or read-only users. It never deletes a completed destination or source package.
Next-tool/explicit recovery removes only separately recorded inactive tools.
An interrupted/truncated ownership record requires inspection rather than a
guessed successful recovery. Other clients must not replace/attach the selected
volume during a transaction; Docker exposes no immutable volume ID or global lock.

Observed adapter evidence: `portable-volume-1792890969343008100.json` passed
bytes/metadata/link/collision/corrupt-package/source-preservation checks (41.72 s).
`portable-volume-1792891158178345000.json` additionally passed import cancellation,
pending record discovery, exact pending discard, repeat-discard refusal and
completed-destination protection (49.05 s). Native review, broader space/failure
qualification, migration and installed upgrade evidence remain separate gates.
`portable-volume-1792891298775272700.json` additionally passed paused-writer and
read-only/stopped external-reference refusal (70.61 s). A larger unboxed Rust
qualification future overflowed its default Windows test stack before dispatch;
the fixture was inspected/cleaned exactly and the harness now heap-bounds adapter
futures. This was a test-harness failure, not an engine-crash observation.
The combined actual-engine attribute audit and recovery regression subsequently
passed in `portable-volume-1792891532714324300.json` (99.43 s). Linux helper tests
also passed user-xattr/special-object refusal and the original edit crash cases.
Shared-helper edit/copy regression `standalone-files-1792891676946434600.json`
passed (60.98 s). Native file-editor save/restore with exact whitespace and retained
original passed in `native-files-2026-10-24.json`; direct native local-copy launch
and storage UI/installed gates remain pending while desktop access is unavailable.

## Export invariant

Source state is authoritative and never modified by export. Support begins with
ordinary local named volumes on explicitly verified engines and quiesced writers.
Preflight inspects volume metadata and every referencing container, including
paused containers; active writers refuse with useful workload identities. No
implicit stop is offered. A selected read-only helper mounts only that volume.
Recheck metadata/writers before publishing the export; external clients must
remain quiesced during the operation. Docker volume names have no immutable
exposed IDs, so coordinate replacement with other clients rather than claiming
a global lock.

A versioned package contains source engine/volume metadata, format/capabilities,
payload length and SHA-256. A complete manifest and verified payload are required
for import. Write an exclusive partial artifact on the selected host filesystem;
publish only after successful completion, checksum and flush. Never overwrite
an existing export. Free-space preflight and bounded streaming are required;
ENOSPC/cancellation retains source state and reports any recoverable partial file.
Do not extract untrusted archives onto Windows. Secrets in application data may
be present; exporting/sharing that file is the user's explicit action.

## Restore invariant

Validate format/version/checksum and safe entry semantics before any destination
write. Restore into a newly created uniquely owned volume, never an existing
user volume. Verify owner labels after creation before attachment/writes; target
name collisions refuse. The pending volume is isolated from ordinary workloads
until validation succeeds. Preserve supported file bytes, names, modes, numeric
ownership, timestamps and supported link semantics; unsupported metadata must
be reported, never silently called faithful. Reject traversal, unsupported special
objects and escaping hard links. Define extended-attribute/ACL support using actual
Moby archive evidence before accepting the package format.

On failure or interruption, retain source/export. A pending target may be
removed only with its exact transaction ownership and no external attachments;
otherwise retain it as an explicit incomplete restore with a recovery action.
Do not attach an unvalidated volume to a project or switch global Docker defaults.
Record transfer phase and enough durable intent to reconcile a late accepted
create or interrupted import. Successful completion rechecks content/metadata
against the manifest. Retry creates a fresh isolated destination or resumes only
when its durable state and source checksum still match.

## Volume clone transaction

P2-STO-CLN is included as a P2-D02 convenience over the existing portable-volume
transaction, without a new archive format, storage driver or copy-on-write claim.
Clone an ordinary local named volume on the verified owned engine into a new
volume name. Preflight source/target names and collisions before capturing data;
the existing restore path checks the target again before creation and verifies
its exclusive ownership before any write. Never stop a writer implicitly.

Capture a checksummed snapshot using the existing quiesced export. The snapshot
point is the export's successful source metadata/writer recheck and publication;
target content is verified against that immutable snapshot, not against a later
mutable source. Keep external writers quiesced during capture. Docker volume
names have no immutable exposed identity, so external replacement still needs
coordination. The existing 64-GiB/100,000-entry and metadata/link restrictions
apply. The cost is a full guest copy plus metadata and temporary uncompressed
host staging of roughly twice payload size during capture. Existing host/guest
space checks remain authoritative preflights, not reservations.

An exclusive job lease and flushed public job intent precede staging. Use a fixed
private directory derived from a random transaction token, with no renderer path
input. Journal intent, captured snapshot, restoring and complete in order.
Completion requires the existing restore's content/metadata verification; retain
its separate ownership receipt and transaction ID. A killed client can leave a
saved snapshot and an isolated pending target, never a falsely completed clone.
The existing restore recovery owns target inspection/discard; clone recovery
must not delete targets or infer that an absent target cannot appear after an
unknown create. Restoring a saved snapshot uses an explicit fresh volume name.

Normal completion can remove only its known unchanged snapshot file after the
complete record is durable. Cleanup failure leaves a verified clone plus an
explicit staging-retention warning. Interrupted staging is reviewed separately:
listing is read-only, saved packages must validate before reuse, and explicit
discard needs an inactive job lease, a coherent record and a canonical owned
directory. Enumerate and preflight all entries before deleting only known regular
snapshot/partial files; refuse links, subdirectories or unknown names. Never
recursively delete a computed path. Retain public job receipts after cleanup.
UI actions need loading/empty/error/cancel states and must distinguish a saved
snapshot, an unconfirmed copy and a verified completed clone.

The 2026-10-25 source implementation and native SDK journey cover active/paused
writer refusal, collisions, verified Unicode/binary/link/ownership copying and
import cancellation with separate snapshot and pending-target cleanup. A native
development-window fixture also completed the clone through the actual UI,
reported no retained staging and passed an independent read-only payload check.
See `artifacts/qualification/p2-volume-clone-20261025.json`. These results do not
establish installed update/uninstall, power-loss or disk-pressure acceptance.

## File edits

Native editing must operate on an explicitly selected file/version. Reject stale
content and unsupported link/special/read-only destinations. Quiescence and
ownership checks precede mutations. Stage new bytes before commit, preserve a
recoverable original/version, and publish atomically where the guest filesystem
supports it. Concurrent application writers are not made safe by a UI checkbox
or a Docker archive upload. Define and qualify external-writer detection,
replacement and backup behavior before offering Save. Explorer/editor opening
uses OS mechanisms on staged copies or selected WSL project paths; never silently
map a Windows editor onto a live database or engine-private storage directory.

## Migration composition

### Image archive transaction / P2-D11

Use ordinary uncompressed Docker save/load tar archives, separate from the
portable volume format. Bind export to the reviewed immutable image ID on the
verified owned daemon. A selected tag must still resolve to that ID before and
after collection; the saved archive must independently match the reviewed ID.
Classic-store config IDs and containerd-store OCI index/manifest IDs differ:
verify the latter's present descriptor graph and all exported configs' membership
instead of treating a config digest as interchangeable with an index digest.
Missing foreign-platform descriptors cannot prove membership. Stream to an exclusive
same-directory random partial file with incremental free-space checks; flush,
validate the archive and publish by an exclusive hard link. Existing destinations
are never replaced. Ordinary failure removes only this client's partial file;
process loss can leave that named partial for explicit inspection. No image,
tag, container, source layer or global client setting is changed by export.

The initial format envelope is a plain tar up to 64 GiB, 100,000 outer entries,
128 manifest images and 4,096 layers per image. Manifest/config metadata have
separate memory bounds. The archive is never extracted on Windows. Reject
escaping/ambiguous/duplicate outer paths, links, special/sparse entries, truncated
payloads, nonzero suffixes and unsupported manifests. Verify content-addressed
SHA-256 blob names, config image IDs and each config's ordered rootfs diff IDs
against uncompressed layer bytes (plain or gzip, with an aggregate 64-GiB
expanded budget). Report platform data from configs, never infer emulation.
OCI metadata included by Docker save is preserved in an export, but it does not
make an arbitrary OCI-only archive part of the supported import format.

Import is implemented in source and under qualification. Its review must
bind the full archive checksum, config IDs, original tag set, observed destination
tag IDs and exact replacement consent. A generic `docker load` against the
original archive before that review would silently replace tags and is forbidden.
The load path emits only validated configs/layers and an isolated Docker
manifest, excluding alternate OCI/repositories tag instructions. Revalidate the
review immediately before dispatch. Docker has no compare-and-swap tag API:
other clients must not write selected tags during this transaction; do not imply
an engine-wide lock. Persist intent and previous IDs before any reference write,
retain previous content, and report a partial/unknown outcome on cancellation or
load/verification failure. Never prune images to recover. A successful archive
export alone does not prove import or complete the migration journey.

The import implementation first loads all validated configs/layers under
random per-transaction `orbwin-imports/` recovery references. No original archive
reference is passed to Load. Verify every staged image by streaming Save back and
checking its config digest and ordered uncompressed layer digests against the
reviewed input. Only then apply explicitly reviewed original tags. Normalization
does not preserve an OCI index digest, attestations or registry signatures; a
tag shared by several platform configs needs an explicit config selection, never
last-entry-wins. Images can be imported without restoring their original tags;
their recovery references remain usable and visible in ordinary image inventory.

On Windows, hold a read-only file handle denying concurrent write/delete for the
entire inspect/load operation; bind the archive checksum and revalidate per-entry
bytes while streaming. The helper owns the normalization pipe/goroutine, closes
both ends on error and waits for its writer before releasing the source handle.
No host extraction or normalized full-size staging copy is needed. A pinned,
networkless, capped read-only df helper measures the verified daemon's reported
Docker storage root before Load, requiring archive bytes plus expanded unique
layers and headroom. It mounts that guest root read-only, executes only trusted
bundled tools and uses the existing durable transient recovery; it never runs
imported code or mounts the daemon socket. This checks an observed shortfall,
not a reservation or a guarantee covering a differently configured storage
filesystem. Guest image space remains shared; load errors retain
the original archive and explicit unconfirmed recovery references.

Flush an intent (owned daemon, source checksum, original tag observations and
all recovery names) under an exclusive process lease before Load. Journal loading,
each verified config/actual image ID, previous-content intent/retention, tagging,
each tag intent/acknowledgement and
completion. A replacement requires its exact observed previous ID and explicit
consent; skipped tags are untouched. Before any original tag write, create and
verify the recorded recovery reference for every replaced image. A previous ID
alone is insufficient protection against collection after its last tag moves.
Recheck all applied tags before loading and
again immediately before each tag API call. This guards stale reviews but cannot
replace external-client coordination around Docker's non-CAS tag API. Preserve
previous content and the recovery references; never automatically undo a partial
tag set, remove images or replay an interrupted job. History is durable evidence,
not a claim that later clients left those references unchanged. Removing a
recovery reference uses the existing explicit per-tag image removal flow.

The 2026-10-25 native SDK journey exercises missing-consent/stale-review refusal,
skip, retained previous content before replacement, absent-tag application,
verified readback and settled receipts. Native desktop evidence covers the
review, cancellation without replacement, focus containment/return, skip import
and historical receipt inspection. See
`artifacts/qualification/p2-image-import-20261025.json`. Multi-image scale,
disk-pressure/power-loss, actual mid-write client cancellation and the installed
combined gate remain unqualified. The declared limits are implementation bounds,
not scale/performance measurements.

Source endpoints and versions are selected and verified explicitly. Prototype
only with disposable exact-owned fixtures on the existing Desktop/Rancher
engines; actual user data requires its own selection/authorization. Export reads
the source without changing its defaults, packages or runtime. Target collisions
refuse; validation and a preservation postcheck are mandatory. Images and project
files require separate reviewed transfer/reuse flows. Credentials and cluster
state are not implicit transfer inputs. A failed or cancelled migration retains
the source and a clear target disposition; no shared writable disks or automatic
data interchange from Rancher ancestry is assumed.
