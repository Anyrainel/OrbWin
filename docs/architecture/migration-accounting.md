# Project transfer receipt accounting

Implemented in source2026-10-25. This advances P2-MIG's combined review without
adding a transaction ledger, job orchestrator or engine/data mutation. Existing
image-export/import and source-volume/restore owners retain all durable truth.
This is not complete durable project migration or Phase2 acceptance.

## Identity and ownership

An explicitly reviewed source project supplies source kind, source daemon,
observed ordinary member image IDs/volume names, source project label and intended
target daemon. The new project panel rereads both existing owning histories on an
explicit request. It never starts work, opens a project file, retries a transfer,
chooses an archive, applies a mapping or resumes an interrupted job.

Both SDK history envelopes now echo their already validated request/receipt-filter
target. This adds response metadata only, not a receipt format or an engine call.
The combined reader requires both envelope targets to equal the project snapshot's
target exactly. A replacement target between the source review and either history
read refuses publication. Legacy standalone history views may still read an older
envelope without target; this stricter combined view cannot. Root must rebuild the
SDK/package before the new panel can work with the current application resources.

Source-image associations use exact source kind/daemon and the immutable image ID
observed on ordinary project members. Archive config IDs, rootfs digests, tag names
and project names are not substitutes. The owning export acknowledgement already
links imports through exact target/archive/config-set provenance; this view does
not reimplement that transaction. Assets may be shared across projects, so a match
does not establish that a transfer was originally requested for this project.

Source-volume associations use exact source kind/daemon and observed ordinary
member volume names. One-off-only mounts are excluded. Existing owning history
links retain complete manifest/payload/source/target provenance; the panel filters
those links by the currently selected destination mapping only. It does not infer
links from equal payload bytes, names, cleanup phase or timestamps. Several
complete/partial historical restores may match; all remain evidence, without
choosing a latest or preferred destination transaction.

## Read lifetime and failure behavior

Reads are sequential to respect existing backend admission. Each response has the
existing2MiB parser/UI cap and SDK receipt-count/read budgets. No partial view is
published if either read, parse, target check or project association fails. A new
read clears the prior view. Cancellation/closing/unmount aborts the current read;
late responses cannot reopen the panel or publish stale results. Changing the
project review revision creates a new component lifetime. Mapping changes derive
a new historical projection from the same acknowledged read, without writing it.

Every matching export is accounted for in counts. Details show at most eight
export/link observations per group, with an explicit total and an instruction to
inspect owning history for the rest. Export ordering is transaction order, not
newest-first and never an implicit selection. Uncertain receipts and unavailable
import counts stay unassociated; absence in a bounded window does not prove that
no transfer occurred. Existing restore scanning can omit legacy/torn records;
zero visible uncertain counts is not a completeness guarantee.

The initial state requests a read; loading, cancel, empty, error and incomplete
evidence states are explicit. Receipt IDs/phases are historical. A complete import
does not establish current tags, and a complete restore does not establish current
writable contents, unreplaced volume ownership or application consistency. Existing
destination ownership checks and separately reviewed Compose validation/startup
remain mandatory before use. No aggregate project-complete state is invented.

## Evidence and next gaps

Focused source tests cover target replacement/missing legacy envelopes, both-or-
error reads, cancellation before/between reads, exact source IDs versus shared tags
and archive config IDs, several historical transfers, selected-volume links,
partial restores, unknown evidence, one-off exclusion and the initial rendered UI.
Existing SDK receipt tests additionally verify envelope target and unchanged
receipt bytes; empty history retains offline/no-directory-creation behavior.
No current engine, source workload, project files or user dataset is modified.

## Windows input drafts and reopening

The existing opt-in Compose-input owner (`orbwin.compose.inputs.v1` in the
application's WebView storage) now accepts bounded migration provenance alongside
reviewed Windows file paths, ordered overrides/env-file paths, profiles and
services. The unchecked "remember" choice is available after Validate; successful
validation saves hints only when selected. Reopening a picker entry opens another
editable confirmation form, never an automatic Up. Different source kind/daemon/
project and intended target identities have separate bookmark keys, including
when their destination project names match an ordinary remembered project.

The stored projection contains immutable observed source member/image IDs,
ordinary volume mount tuples, source/target/project identities, and optional
mapping selections/path/SHA. It contains no YAML, environment values, credentials,
transfer phases, aggregate completion state or receipt copies. Unknown source
image identities and incomplete mappings permit Validate-only drafts. Up refuses
these incomplete inputs. Existing owning image/volume histories must still be
reread from their panel for transfer accounting; the input draft never supplies
transfer status.

Before every migrated Validate or Up, the existing source review command rereads
the selected source and intended target. Rancher review explicitly addresses the
saved development-distribution registration and machine; this read can start that
selected user distribution, as in the original source review. Exact source/target,
ordinary member IDs, source image IDs and ordinary volume mount tuples must still
match. Recreated source members require fresh review, rather than being adopted
silently. Original configuration labels are hints: base/override/env files are
read anew and trusted by the user; their content is not attested as equal to the
original source configuration. Validate is not a reservation or a promise that a
later Up will succeed.

Mapping save returns the SHA-256 of the actual serialized disk bytes written by
Rust, not the frontend preview string. The saved mapping must occur exactly once
as the final override. Before dispatch, a read-only Rust check opens the final
file without following a final Windows reparse point, requires a regular file
within 256 KiB, holds a Windows handle denying write/delete while reading, hashes
the bounded bytes, parses the strict existing mapping schema and compares its
semantics with the reconstructed selected source-mount/destination mapping. Missing,
changed, malformed, oversized, substituted or semantically mismatched files fail
closed without rewriting them. The target record must also match the freshly
reviewed target. Cancellation before or between checks prevents Compose dispatch.
This is point-in-time verification: the handle closes before upstream Compose
reads files, and no cross-process file reservation, inode/content immutability,
atomic source-to-target transaction or protection from a later external writer is
claimed. Coordinate external file and data writers.

The existing owner remains bounded to twenty entries and 2 MB; each migration
projection is additionally bounded to 256 KiB and 256 members/images/mounts.
Invalid/unknown metadata is rejected, corruption is surfaced in the picker, and
save/forget refuse to replace corrupt stored bytes. A failed storage write leaves
the previous value intact. This device-local input convenience is not a durable
asset receipt, transactional database or backup. Existing localStorage last-writer
behavior remains; concurrent app windows must coordinate input edits.

Focused pure tests cover restart-style reload, same-name separation, hostile
metadata, corruption/quota retention, mapping order, source/target/member/image/
mount drift, incomplete images, cancellation and failed checks. Rust tests cover
actual serialized-byte identity, changed/missing/oversized/hostile files, semantic
mismatch and Windows write/delete denial during inspection. Installed/native
keyboard/loading/error/reopen proof remains pending integration. Combined
cross-restart handoff identity beyond these input hints, current data-use checks
and complete migration acceptance remain open. Neither these drafts nor the
derived accounting panel claim project migration completion.

## WSL draft continuity

The same input owner additionally accepts explicitly tagged `kind: "wsl"` records.
The Windows/legacy representation remains unchanged. WSL records include the
selected project distribution name, registration GUID, canonical `/home` source
root and reviewed device:inode signature; these are hints, never authority to
connect/reconnect an export. The source engine's optional Rancher registration/
machine identity is separate from this project distribution. Bookmark identity
also includes the project registration/root/signature, so identically named
projects from different connections do not overwrite each other. Save remains
explicitly opt-in after successful Validate. Neither reopen nor saving starts Up.

Every reopened submission reads `wsl_migration_project` again, requires an exact
returned registration and still-connected root/signature, then rereads the
original source engine/project and intended target using the existing migration
review. A missing/renamed registration, changed root, disconnect, source/target
replacement or observed project-member/image/mount drift refuses. No saved
`shared_source`, guest socket, tool path or config path is accepted as authority.
The existing `.rdorbwin/wsl-projects` record remains the sole connection owner;
existing native preparation validates it and derives the actual shared mount,
private client configuration and verified helper launch. Invoking these explicit
reads can start the selected development distribution, never the engine.

WSL mappings saved by the existing helper retain that save result's SHA, and must
remain the unique final override. A bounded optional `mappingReview` travels only
through the existing typed WslCompose request: a root-relative JSON basename,
expected saved-byte SHA and Rust-canonical reconstructed narrow mapping. The
native adapter requires the reviewed registration and target, permits Validate/Up
only, and rejects malformed hashes/filenames/schema. It verifies the current
owned daemon before preparing the existing guest helper.

The guest helper verifies source/shared mount device:inode and mount presence as
before. Its new read-only check opens a pinned `os.Root` for the recorded source,
opens the final basename using `O_NOFOLLOW | O_NONBLOCK`, requires a regular file
within 256 KiB, reads bounded actual disk bytes, verifies SHA plus strict semantic
mapping equality, and compares file metadata/name identity around that read.
A symlink, FIFO, directory, missing/oversized file, changed bytes/semantics, observed
replacement or cancellation refuses. The check runs before configuration
resolution, after resolution before Validate can succeed, and again before Up's
existing final project/daemon verification and exec boundary. A review target is
not replaced by whichever daemon happens to be available.

This is a repeated point-in-time check, not exclusion of Linux writers or a
cross-process atomic snapshot. No prevention of ABA changes, external file
writers after checks, inode/content immutability or application-consistent data
transfer is claimed. Existing secret-containing resolved configuration stays in
the short-lived sealed memfd; the draft stores only input/provenance hints and
never stores resolved YAML, environment values, credentials or transfer status.
Owning receipt accounting must be reread independently.

Focused TypeScript tests cover explicit tags and Windows compatibility,
connection/source-engine independence, root/path/mapping-order confinement,
registration/root/source/target drift, partial image checks and cancellation.
Two Rust pure tests cover typed bounded request transport and reviewed-target
refusal while preserving ordinary WSL requests. Cross-compiled Linux tests were
run by the root agent using an exact inspected Ubuntu registration, UID 1000,
clean environment and fresh mode-0700 temporary parent; the three
`TestMappingReview*` tests passed and cleanup was verified. Receipt:
`artifacts/qualification/p2-wsl-mapping-pure-x4YP4J/proof.json`; test binary SHA
`eb2354a14ac31a4a331aeeaad20c3d44aa18bdd1584563a708cd27285e815f81`.
These tests do not invoke an engine, project workflow, Compose or Docker. Actual
installed WSL draft-save/restart/reopen/Validate/Up and concurrent-writer journeys
remain unqualified pending an explicitly isolated runtime fixture. No complete
project/application migration claim follows from these source and pure checks.
