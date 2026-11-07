# Reviewed migration volume mappings

2026-10-25. Implements a configuration handoff after separate source-volume
export/restore. It does not establish that selected destination data came from
the source archive, start workloads, or close the combined migration transaction.

## Source observation and projection

The existing fixed Desktop/Rancher source adapters now preserve each labelled
member's mount type, container destination and read-only flag, plus the physical
volume name for volume mounts. Host bind sources, guest-private volume paths,
driver options, arbitrary labels, environment and credentials remain unexposed.
Only ordinary local volumes without driver options whose observed mount source
equals the volume's full root are eligible for generated mappings. Subpath or
unknown roots remain manual review; the root comparison is internal and is not
published as a path. Bound observations to 512 mounts per member and 8192 across
the review. Older observations without mount metadata require a fresh review.

Ordinary replicas must agree on their complete mount configuration for a service.
Missing service labels, conflicting replicas, duplicate targets and unsupported
volume roots refuse draft generation. One-off tasks are excluded. Bind/tmpfs and
other mounts are retained in the source observation but are not rewritten. Source
metadata is an observation, never proof that files still exist or match labels.

## Configuration draft and ownership

The reviewer explicitly selects an existing OrbWin destination for each observed
source volume. Distinct source volumes cannot silently merge into one destination.
Generate only `services.*.volumes` and external named-volume declarations, using a
fresh alias namespace, preserved read-only flags and `volume.nocopy: true`.
Literal dollars are doubled for Compose interpolation. No images, commands,
credentials, environment or port definitions are synthesized. Drafts are bounded
to 256 mounts and 128 KiB of input JSON. Review the complete trusted Compose file
set for alias/target conflicts; the generated namespace does not establish that
arbitrary existing configuration cannot collide or change later.

The native Save operation accepts only this narrow typed schema and reserializes
it. Unknown fields, interpolation, traversal/noncanonical targets, volume
creation, driver options, duplicate destinations and image-data copying are
rejected. It verifies the reviewed target daemon and existing ordinary destination
volumes, asks for a new Windows `.json` file, then rechecks daemon and volume
metadata after the dialog. Creation is exclusive. Existing files are never
replaced; a write/sync failure leaves a named partial file with an inspection
message. Device/alternate-stream names are refused. Output is bounded to 256 KiB.
No engine mutation, Compose invocation or credential/global-client change occurs
when saving. Closing the reviewer suppresses late UI results; a native Save dialog
already requested is controlled separately by Save/Cancel.

Saving creates an editable file owned by the user, not a transaction receipt or
an applied configuration. A saved mapping is bound to the current source-review
revision and draft contents. Windows handoff appends that file last to the original
ordered overrides in the existing editable Validate form. A changed review or
destination selection invalidates that handoff synchronously; the parent owns
the selection and derives both handoffs from the same current draft.
Validation still requires a separate submission and starts no workload. Remember
is opt-in.

## Checking a restored destination

The explicit destination check is read-only. It targets only the owned pipe and
selected volume, verifies the current daemon, ordinary local-volume metadata,
restore ownership labels and a completed, untruncated owning receipt. New restores
record the destination metadata fingerprint in their existing `created` event
before helper attachment. A legacy receipt without this fingerprint cannot
establish current destination identity; its recovery behavior remains unchanged.

Match the source review's daemon/volume and a source export acknowledgement to
the completed restore's full manifest digest. Then compare the saved destination
fingerprint with current metadata, recheck receipt, volume and daemon before
reporting success. Refuse missing, incomplete, replaced, foreign or inconsistent
records; never create helpers, scan data, acquire a cleanup lease, stop workloads
or change volume labels. The native deadline is 40 seconds; engine calls share a
30-second context. Host receipt reads retain their existing size/count budgets.

Success means recorded restore provenance and observed metadata still match.
It does not reread writable contents, establish application consistency, reserve
the name or authenticate against another process with the same user's access.
Selection changes discard the UI observation. Mapping remains available for
separately reviewed manually prepared destinations; an unchecked mapping must
never be shown as a verified source restore or completed migration.

2026-10-25 qualification: the full Go suite/vet, scoped destination/receipt race
tests, 93 JavaScript tests, 57 ordinary Rust tests and frontend/desktop builds
pass. Mocked read-only engine contracts cover wrong/changed daemon, wrong source
or source volume, mismatched labels/name, legacy/incomplete/torn receipts, missing
source acknowledgement, replacement during review and cancellation. The native
operation plan validates names and camel-case input, remains a read and has no
maintenance-image dependency. Real Desktop and Rancher nonempty round trips pass
the destination check and refuse a foreign source volume, alongside the existing
content/metadata, preservation and interruption-recovery checks.

`p2-migration-destination-ui-state-20261025.json` records a separate nonempty
Desktop fixture, exported/restored through the production SDK with normal host
receipts. Native UI selected the project/destination, verified restore
`372b3ded8fa4bdf751f4a00371376574` against its source export and metadata, saved
a new mapping with the native file picker, appended it last and completed quiet
Windows Compose validation with Remember unchecked. No project container was
started on either engine. History rendered the positive complete restore link
alongside the two unlinked legacy rows, superseding the earlier pending positive
row check. The maximized desktop was 2560×1392; smaller-window qualification is
still separate. Initially the development window was partly outside the visible
desktop; its observed native Maximize control restored a usable view. Native
accessibility indices were inconsistent for some web controls; screenshots and
keyboard selection resolved the actions. No OS settings were changed.

Guarded cleanup removed the never-started source member and the two fixture
volumes, checked the exact restore token/payload labels and absence of target
users, matched the source seed bytes to the package payload hash, retained the
original fixture file and restored both prior inventories. Receipts, package and
saved mapping remain as evidence. A later destination review correctly refused
the removed name. Three daemon identities and private/project/global config
hashes were preserved with Windows elevation false; only the development UI
closed. Aggregate evidence is `p2-migration-destination-20261025.json`.

This proves nonempty data transfer through reviewed mapping and validation,
without claiming a complete application migration. Image/project coordination,
later startup/data consumption, mapping-to-receipt persistence beyond the owning
receipts, cancellation UI, replacement/adoption UI and installed qualification
remain open with the rest of Phase 2.

## Saving in an existing WSL project

The WSL reviewer can save the current draft directly into the explicitly selected,
already-connected original project. It does not reconnect the export or change
private Docker configuration. The native adapter requires the reviewed distro
registration, source directory identity and destination daemon, canonicalizes the
strict draft, and dispatches through the pinned project helper as the distro's
default user. The helper checks the source/export identity, executable pins and
target daemon, then compares ordinary destination-volume metadata before and
after saving. Native dispatch is bounded to 50 seconds; helper mapping work uses
a 30-second context including initial verification.

Only a new basename ending in `.json` is accepted, with no directory separators
or control characters. An open `os.Root` pins the directory; its device/inode
must match the reviewed source. Creation is exclusive with `O_NOFOLLOW` and mode
0600. The helper writes and syncs the file, verifies the opened and named inode,
syncs the directory and closes the file before postchecks. Concurrent attempts
cannot replace each other's file. Project rename/replacement, symlinks, missing
destinations and changed metadata refuse success. A failure after creation keeps
the named file and reports that it may remain; neither cancellation nor a killed
process promises rollback. A context cannot interrupt every filesystem syscall.

Success returns bounded JSON containing the original Linux path, directory
signature and SHA-256 of the canonical saved bytes. This is save confirmation,
not durable migration accounting. The UI binds it to draft contents, filename,
distro registration and project identity. Changed inputs invalidate handoff.
Linux validation appends the saved path last while preserving the original
ordered file list. The existing editable form still requires explicit Validate.
This Linux form does not persist the selected inputs. It does not start or stop
workloads.

## Qualification and remaining work

90 JavaScript tests and 56 ordinary Rust tests pass. Go projection/privacy,
full-suite/vet and scoped race tests pass. Tests cover replica disagreement,
one-offs, unsupported subpaths/roots, missing metadata, target collisions,
literal-dollar paths, strict schema and existing-file preservation. The initial
JavaScript escaping used replacement-string `$$` semantics incorrectly; a failing
test caught it and the implementation now uses a replacement callback.

`p2-migration-mapping-compose-20261025.json` records an actual owned-engine Compose
merge: literal `${literal}` destination, explicit external volume, preserved
read-only and nocopy, one created container that was never started. Both fixture
volumes and that container were removed; the prior inventory was restored.

Real fixed-source reviews are recorded in `p2-migration-mapping-desktop-20261025.json`
and `p2-migration-mapping-rancher-20261025.json`. Desktop had one pre-existing
Compose-labelled member with no service label, correctly unsuitable for automatic
mapping; Rancher had no project members. A direct Rancher invocation initially
lacked its pinned Linux-CLI environment and refused; the native-equivalent pinned
environment succeeded. No broader source-version compatibility is inferred.

`p2-migration-mapping-ui-state-20261025.json` records a separate isolated fixture:
one stopped/never-started Desktop container and empty source volume, plus an empty
OrbWin destination. Native UI review observed mount discovery, keyboard selection,
full-width selectable JSON, native new-file Save, append-last Windows handoff and
successful quiet validation with Remember unchecked. The initial preview was too
narrow; it was corrected to a full-width collapsible preview. Cleanup verified
exact labels/IDs and never-started state, removed only those fixtures, restored
both prior inventories and retained the original fixture file unchanged. This
qualifies configuration wiring; no archive-origin or nonempty data transfer is
claimed for this fixture.

The Linux extension passes 91 JavaScript tests and 57 ordinary Rust tests. The
Linux helper tests run under Ubuntu's normal user and cover private new files,
existing files, dangling symlinks, wrong directory identity, cancellation before
creation, retained files after postcheck failure, directory replacement,
concurrent exclusive creation and permission denial. Go suite/vet and frontend
build pass. `p2-wsl-mapping-native-20261025.json` and its records/log qualify the
actual native adapter as UID 1000: wrong-root/daemon refusal, Unicode filename,
0600 permissions, exact canonical bytes/hash, overwrite refusal, append-last
Compose validation and guarded cleanup. Both temporary project files were
removed; cleanup errors were empty. An initial assertion incorrectly expected
empty validation output; the adapter intentionally returns a bounded confirmation.
That evidence is retained separately and the corrected test passed.

`p2-wsl-mapping-ui-state-20261025.json` records a separate native UI fixture:
one never-started Desktop member, an empty source volume and an empty owned
destination, with a uniquely named base file inside the existing Linux project.
The UI qualified explicit distro selection, existing-connection inspection,
new-file save and displayed digest, append-last Linux handoff and successful
Compose validation. Changing the selected destination invalidated the saved
mapping and disabled Linux review; restoring the identical draft re-enabled it
without another write. The edited intermediate draft was never saved or applied.
The Linux form offered no Remember option. Cleanup verified exact test labels,
container identity/never-started state, source-root identity and file hashes;
only the fixtures and two new Linux files were removed. Both prior engine
inventories were restored. The retained Windows fixture copy was unchanged.
`p2-wsl-mapping-retention-20261025.json` verifies Windows was not elevated, all
three daemon identities and the private/global client and project-record hashes
were unchanged. The installed app remained running; only the development UI was
closed. This empty-volume configuration proof does not qualify nonempty migration,
active-save cancellation UI or installed behavior.

The combined durable migration plan, export/restore receipt linkage, application
consistency, image/project coordination, later target replacement, collision and
scale matrices and installed qualification remain open. Saving a
draft cannot be presented as a completed migration or an authorization to start
another application's data concurrently.
