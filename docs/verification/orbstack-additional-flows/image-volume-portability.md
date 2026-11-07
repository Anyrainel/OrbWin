# Additional OrbStack flows: image/volume portability and resource-list conveniences

Status: research record and Phase 2 scope proposal, written 2026-10-24. It does
not change accepted scope. It follows the flow template in
[the OrbStack behavior catalog](../../research/orbstack/flows.md); that folder is
owned by the research chat and is not edited here.

Evidence labels: **documented** = stated on an official OrbStack page fetched
2026-10-24; **reported** = stated by users in a public issue; **unknown** = no
evidence. Nothing here was observed on a Mac. OrbWin statements are labelled
**source** (read from this repository) or **observed** (recorded test evidence).

| Key | URL |
| --- | --- |
| RN | https://docs.orbstack.dev/release-notes |
| FS | https://docs.orbstack.dev/docker/file-sharing (section "Volume clone, export, and import") |
| IMG | https://docs.orbstack.dev/docker/images |
| NF | https://docs.orbstack.dev/features/native-files |

Corrections to the brief, from RN and FS as fetched:

- Zstandard appears in FS only, not in RN. RN 1.11.0 says "Volume import and
  export" without naming a surface; RN 1.11.2 adds the CLI commands and the
  image `.tar` UI. Copy-on-write appears in FS only; RN 2.0.2 says "Added volume
  cloning".
- Sorting: RN 1.6.4 "Sort images and volumes by size"; RN 1.11.0 "More volume
  and image sorting options"; RN 2.1.1 "Sorting options for
  containers/volumes/images". RN does not say containers can be sorted by size.
- Icons: RN 2.0.2 `dev.orbstack.icon` for custom **container** icon URLs; RN
  2.1.0 custom **volume** icons via the same label; RN 2.1.1
  `dev.orbstack.compose-icon` for Compose projects; RN 2.2.2 fixes "missing
  icons on some containers" and "creating containers with custom commands".
- "Hiding shared mount" is RN 0.9.0 "option to hide 'OrbStack' volume from
  Finder": a macOS Finder volume, not a Docker volume.
- IMG and NF do not describe image import/export, badges, sorting or icons.
  NF documents read-only image files and editable volume files only.

## OSX-PORT-001 — Export and import an image as a `.tar` file

**Actor/intent:** developer moves an image to another machine or keeps an
offline copy without a registry. **Entry:** OrbStack UI (RN 1.11.2). **Documented
sequence:** RN 1.11.2 says only "UI: Import and export container images as
`.tar` files". No documentation page describes the dialog or format.

**Branches:** none documented. RN 1.4.2 (before this UI) fixes images with
extended attributes failing to import and incorrect import error messages, so an
image import path existed earlier; its surface is unknown. **Unknown:** whether
the tar is `docker save` format (OCI or Docker archive), multi-tag or
multi-platform selection, compression, progress, cancellation, overwrite
behavior, free-space checks, tag conflicts on import, and what happens to a
partially written file. Ordinary `docker save`/`docker load` against the
OrbStack engine is ordinary Docker behavior, not OrbStack evidence.

### OrbWin Windows adaptation

**Source today:** no image save/load request exists in `src/backend.ts`,
`src-tauri/src/*.rs` or `backend-tools/session/*.go`. Image actions are pull,
tag, remove, run and info (platform shown as `Os/Architecture` in
`src/components/resources/InfoPanel.tsx`). Users can run `docker save`/`load`
against the isolated OrbWin endpoint themselves; there is no product flow.

**Gap:** a native export (selected image + tags to a new Windows file) and import
(file to the owned engine, explicit tag-conflict handling). P2-D02 in
[development-data.md](../../architecture/development-data.md) already says images
need "separate reviewed transfer/reuse flows"; this is that flow. Docker's
archive format already carries a manifest and layer digests, so a custom
container format is not needed; validate the archive with Docker's own load
rather than extracting it on Windows.

| Scenario | Required behavior |
| --- | --- |
| Multi-GB image | Streamed, bounded memory, progress in bytes; host free-space preflight and incremental checks as `freeSpaceWriter` does for volumes |
| Existing destination file | Refuse; publish via exclusive partial file and no-replace link, never silent overwrite |
| Cancel / process killed mid-export | Partial file removed or reported; engine images unchanged |
| Truncated or corrupt tar on import | Docker load error surfaced verbatim; no tags changed; no partial tag reported as success |
| Import tag already exists with different ID | Refuse or require explicit retag; never silently retarget a tag used by containers |
| Multi-platform or foreign-arch image | State which platform(s) were exported; import shows the resulting platform |
| Image in use by running container | Export allowed (images are immutable); import never replaces it implicitly |
| Paths | Local drive paths with spaces/Unicode; UNC/device/relative paths refuse as for volumes |
| UI states | Loading/progress, empty (no images), error with retry, Escape cancels dialog, keyboard focus order, announced completion with digest |

## OSX-PORT-002 — Export and import a volume archive (zstd)

**Actor/intent:** developer backs up a volume, moves it to another Mac, or
shares a database dump. **Entry:** `orb docker volume export <vol> <file>` and
`orb docker volume import <file>` (FS; RN 1.11.2); an unspecified surface in RN
1.11.0. **Documented sequence:** FS: export to `my-volume.tar.zst`, import it
later; "uses Zstandard compression to produce a small file while being fast to
import and export."

**Branches:** none documented. **Unknown:** import target name selection and
collision behavior (the example passes no destination name), whether a running
or paused writer is refused, consistency guarantees, manifest/checksum,
preserved ownership/xattrs, free-space checks, cancellation, partial-file
cleanup, cross-version compatibility, and whether non-local drivers work. "Back
up" in FS is not evidence of an application-consistent database backup.

### OrbWin Windows adaptation

**Source/observed today:** export and restore are implemented
(`backend-tools/session/volume_transfer.go`, `volume_package.go`,
`volume_recovery.go`; UI in `src/components/resources/actions.ts`,
`Operations.tsx` "Restore data…", `VolumeTransfers.tsx`). Contract and evidence
are in [development-data.md](../../architecture/development-data.md): uncompressed
`ORBWIN-VOLUME-1`, per-file and aggregate SHA-256, 64 GiB / 100,000-entry limit,
quiesced-writer refusal including paused containers, read-only helper mount,
xattr/ACL refusal, exclusive partial plus no-replace publication, restore only
into a new name with an ownership receipt, and explicit pending-discard
recovery. Adapter qualification artifacts pass; native UI and installed gates
remain pending.

**Gap against OSX-PORT-002:** (1) no compression, so packages are about the
logical data size and export spools about twice that on the host; (2) no
`orbwin` CLI equivalent; (3) no OrbStack or Docker Desktop archive
interchange. Compression is a format-version decision (version 2), not a
change to version 1. Interchange is a P2-MIG question and must not be inferred.

| Scenario | Required behavior (status) |
| --- | --- |
| Large volume near 64 GiB / many small files | Refuse clearly over limits; bounded memory (implemented; large-scale timing unmeasured) |
| Host free space | Preflight plus incremental checks with headroom (implemented; ENOSPC fault test still to qualify) |
| Guest free space on restore | Checked, no reservation claimed (implemented) |
| Running/paused writer | Refuse with workload identities; no implicit stop (observed) |
| Stopped referencing container | Export allowed; restore destination refuses any external reference (observed) |
| Interrupted export | Source untouched; partial removed or reported (implemented; kill-test pending) |
| Interrupted/cancelled restore | Pending volume retained with receipt; exact discard only (observed) |
| Corrupt or edited package | Checksum refusal before any destination write (observed) |
| Name collision | Restore refuses an existing name; export refuses an existing file (observed) |
| Compression (if v2 adopted) | Checksum over uncompressed payload; decompression bounded against zip-bomb ratios; v1 still readable |
| UI states | Progress, cancel, error text, keyboard and screen-reader review of dialogs and `VolumeTransfers` (pending native review) |

## OSX-PORT-003 — Clone a volume

**Actor/intent:** developer duplicates a volume, for example to experiment on a
copy of database state. **Entry:** `orb docker volume clone source-vol dest-vol`
(FS; RN 2.0.2). **Documented sequence:** FS says clones copy a volume's contents
into a new one and "are copy-on-write and use very little additional space until
files get modified."

**Branches:** none documented. **Unknown:** UI entry, existing destination
behavior, in-use source handling, label/driver copying, and the filesystem
mechanism behind copy-on-write.

### OrbWin Windows adaptation

**Source today:** no clone request exists. A clone can be composed from the
existing primitives (quiesce check, read-only source helper, new owned
destination with receipt, verification) without a Windows host spool, by
copying inside the guest between two mounts.

**Copy-on-write caveat:** the engine's volumes live on the upstream Rancher WSL
data disk, which is expected to be ext4. ext4 has no reflink support, so an
OrbWin clone is a **full copy** that uses the source's size in guest space and
takes time proportional to data size. Do not claim copy-on-write, instant clone
or space sharing unless the actual guest filesystem is verified to support
reflinks and the copy tool is shown to use them. The UI must state space cost
before starting.

| Scenario | Required behavior |
| --- | --- |
| Destination exists | Refuse; never merge into or overwrite an existing volume |
| Insufficient guest space | Preflight against source logical size plus overhead; refuse before create |
| Source in use by running/paused writer | Refuse with identities, as for export |
| Interrupted clone | Pending destination retained with receipt; reuse the restore recovery/discard path |
| Verification | Manifest comparison between source and destination before `complete` |
| Unsupported entries | Same xattr/ACL/special-object refusals as export |
| UI states | Progress, cancel, space estimate, error, keyboard and accessible names |

## OSX-UI-001 — Create containers and networks from the GUI

**Actor/intent:** developer creates a container or network without the CLI.
**Entry:** UI. **Documented sequence:** RN 2.0 "Redesigned UI: OrbStack is now a
full-fledged container IDE!" and "Create containers and networks". RN 2.2.2
fixes creating containers with custom commands, so a command field exists
(documented by inference from a fix note; field set otherwise unknown).

**Unknown:** form fields (ports, env, mounts, restart policy, network driver,
subnet), validation, name collisions, pull-on-create, and error display.

**Source today:** OrbWin has Run (image, name, command, ports, environment,
volumes; `kind: "run"` in `src/backend.ts`), Create volume and Create network
(name only) in `Operations.tsx`. **Gap:** network driver/subnet/internal options
and container restart policy/network selection are absent. These are useful but
not portability; treat as P2 resource-management polish, with tests for name
collision, invalid subnet overlap, Docker error passthrough, and form keyboard
and error states.

## OSX-UI-002 — Sort, badges, paused state, icons and hidden mount

| Item | OrbStack evidence | OrbWin today (source) | Gap / note |
| --- | --- | --- | --- |
| Sort by size | Documented images/volumes (RN 1.6.4); sorting options for all three (RN 2.1.1) | `SortMode = "status" \| "name"` in `model.ts`; size shown in row subtitle | Add size sort for images/volumes; volume sizes depend on `docker system df`-style measurement, which is costly; show "unknown" rather than 0 |
| Emulated-image badge | Documented (RN 1.6.0, "e.g. amd64") | Platform shown in image info only | Badge when image platform differs from engine native architecture; on x64 Windows hosts, foreign images are arm64 etc. Whether emulation (binfmt/QEMU) is installed in the upstream guest must be verified, not assumed |
| Paused indicator | Documented gray dot (RN 1.5.0) | `isRunning` is `State == "running"` only, so a paused container shows the stopped dot; actions already distinguish paused | Distinct paused indicator with text label, not colour alone |
| Custom icons | Documented labels for container icon URL, volume icon, Compose icon (RN 2.0.2, 2.1.0, 2.1.1) | Fixed lucide icons per kind | Label-driven icons fetch remote URLs: privacy/security decision needed (no fetch by default, or bounded HTTPS fetch with cache and size/type limits); alt text from resource name |
| Hide mount from Finder | Documented option (RN 0.9.0) | No Explorer integration exists | Not applicable until a P2-FIL Explorer path exists; then hiding must be an explicit reversible setting |

## Relationship to P2-STO, P2-FIL and P2-MIG

P2-STO already owns volume export/restore (P2-D02). OSX-PORT-002 is the same
transaction and must not be counted again; it adds only compression and a CLI
as candidate deltas. OSX-PORT-003 (clone) reuses P2-STO's ownership, receipt
and verification invariants and should be accounted as a P2-STO sub-item, not a
separate storage transaction. OSX-PORT-001 (images) is not covered by P2-STO's
volume work; P2-D02 says images need their own flow. P2-MIG may later compose
image and volume transfers, but interchange with OrbStack/Docker Desktop archives
is unverified. P2-FIL owns browsing/editing and any Explorer mount; it does not
gain anything from this record except the hide-mount note.

## Proposed Phase 2 scope rows and decisions

| Proposed ID | Outcome | Dependency / note |
| --- | --- | --- |
| P2-STO-IMG | Native image export to a new `.tar` and import with explicit tag-conflict handling, using Docker's archive format | New flow; feeds P2-MIG image transfer |
| P2-STO-CLN | Volume clone into a new owned volume with verification and stated full-copy space cost | Reuses P2-STO receipts; no copy-on-write claim on ext4 |
| P2-STO-ZST | Decide whether package format v2 adds zstd compression | Decision only; v1 stays readable |
| P2-UI-SRT | Size sort for images/volumes, paused indicator, emulated-platform badge | Badge requires verifying guest emulation support |
| P2-UI-NET | Network create options (driver, subnet, internal) and container restart policy/network in Run | Resource-management polish |

Decisions to record in `docs/project-decisions.md` (not edited here):

1. Image transfer uses Docker save/load archives; no custom image container format.
2. Clone is a full verified copy unless reflink support on the actual guest disk
   is measured; UI wording follows the measurement.
3. Label-driven remote icons are off unless a fetch policy is accepted.
4. OrbStack `.tar.zst` volume archive import is out of scope until its format is
   documented or verified with a real archive; it is not implied by OSX-PORT-002.
