# Read-only container files

The existing per-request Go SDK helper owns a fixed OrbWin pipe connection and
verifies the recorded Linux daemon before requesting Docker's archive endpoint.
Full container IDs and absolute Linux paths are required. No guest command,
temporary container, host extraction, writable mount or configuration change is
used. This works with stopped containers and images without a shell. Mounted
volumes are inspected at their actual container mount path. Image/volume pages
direct users to an existing container rather than silently creating one.

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
writes while viewing. No file modification controls are offered by this flow.
