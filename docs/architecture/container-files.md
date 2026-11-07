# File access and guarded volume edits

The existing per-request Go SDK helper owns a fixed OrbWin pipe connection and
verifies the recorded Linux daemon before requesting Docker's archive endpoint.
Full container IDs and absolute Linux paths are required. No guest command,
temporary container, host extraction, writable mount or configuration change is
used for container reads. This works with stopped containers and images without
a shell. Mounted volumes are inspected at their actual container mount path.
Standalone image/volume reads use the separate tools flow below.

The helper closes every HTTP/archive reader and client. A 15-second deadline,
8 MiB archive byte budget, 256 direct-entry limit and 256 KiB text limit bound
work. A separate 8 MiB logical payload budget also bounds sparse-file skipping;
a network deadline cannot interrupt CPU-only sparse expansion. Reaching a
listing limit returns an explicitly partial result. A narrower
directory path is the next action; a large file earlier in an archive can consume
the budget before later entries appear. UTF-8 regular files can be previewed;
binary, oversized, link and special-file previews refuse. Directory entry names
must remain within the requested archive root; duplicate names refuse. Symlink
targets are displayed as text, never extracted or executed. Docker owns container
path resolution; users already have that engine's ordinary Docker authority.

The native operation is a cancellable read under the existing four-client lease.
Cancellation or renderer loss kills only that helper client and closes the stream.
Navigation discards stale results and aborts the previous client. Files are live
reads, not atomic snapshots, backups, exports, or a guarantee against application
writes while viewing. Guarded editing is a distinct explicit transaction below.

## Phase 2 standalone sources

The pinned Moby API exposes read-only image mounts. A local owned-engine proof
mounted the existing pinned Alpine image and read its release file without ever
starting that selected image. Standalone image/volume reads use that upstream
mechanism in a separate pinned tools container, with no network, host paths,
Docker socket or writable selected-data mount. Existing container reads retain
their helper-free archive path.

A uniquely labeled helper has a read-only root, dropped capabilities, resource
limits and a 30-second main-process expiry. The read has a 15-second deadline;
normal cleanup removes only its returned full helper ID. Forced client loss
after helper startup is bounded by guest expiry plus Docker AutoRemove. The
create/start interruption window is covered by flushed ownership intents and an
exclusive Windows process lease. Next-helper or Settings recovery only considers
inactive recorded exact IDs/names with matching metadata. Uncertain missing
creates remain recorded. This does not authorize deleting user workloads/volumes.

Volume existence/metadata are checked before and after attachment; image IDs
are immutable digests. No source volume is intentionally created. External
removal/recreation during attachment is a conflict, not empty-data success.
Docker volumes have no immutable exposed ID and a mount may race external
removal; any unexpectedly replaced volume is retained and reported, never deleted.
Ordinary local named volumes without driver options are the current support
candidate. Snapshots remain live bounded reads, not backups or transactions.
Mounted-source path prefixes are checked for links to avoid resolving an image's
absolute symlink through the tools root. Links are displayed but traversal refuses;
original container-path semantics remain. Concurrent external mutation is not
prevented by read-only access.

Write/edit/export/restore operations require separate ownership and interruption
contracts. Standalone preview alone does not complete P2-FIL or P2-STO.

## Copy, native tools and guarded text save

Copy streams one regular file up to 4 GiB through an exclusive random host partial
file, verifies length/SHA-256, flushes and publishes through an exclusive NTFS
hard link. Existing/racing destinations refuse. Free-space preflight precedes
streaming. No untrusted archive extraction occurs on Windows. Normal outcomes
clean owned partials; process death can retain the disclosed named partial.
Cancellation attempts bounded tools recovery and reports destination uncertainty.

Native Explorer/Notepad actions select an existing local drive file through the
OS dialog and invoke fixed executables with argument arrays. No shell association
or guest command runs. Notepad refuses non-UTF-8, NUL-containing or over-256-KiB
content. Saving a local copy does not upload changes to the engine.

Text previews include SHA-256. Container paths map to editing only through the
deepest writable-volume mount and verified link-free ancestry. Bind/read-only
and writable-layer paths explain the refusal. Direct volume edits use the same
guarded transaction. Active/paused/restarting writable users must be stopped;
external clients must keep the volume quiesced. This is not a global volume lock.

The verified static Linux helper uses directory descriptors and O_NOFOLLOW,
checks content version/inode/metadata, refuses xattrs/nonregular/multiply-linked
targets, and preserves numeric ownership/mode. Flushed prepared receipts precede
renameat2 RENAME_EXCHANGE. The displaced original is always retained as a backup
or prepared draft. Post-exchange writer conflicts report both retained versions.
Committed receipts append without truncating prepared state. Restore previous
text is another guarded save. Cancellation preserves the draft and reports that
an edit/backup may already exist. Subprocess-loss tests cover before/after exchange;
they are not power-loss durability evidence.

Only edit tools have a writable disposable root to stage the trusted static
helper through Docker's archive API; selected images/root layers stay untouched.
Portable volume transactions have a separate [contract](development-data.md).
