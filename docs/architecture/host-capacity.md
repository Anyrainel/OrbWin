# Host capacity in Storage

2026-10-25. Implements the host-capacity part of P2-STO's capacity addition;
per-volume size refresh and installed qualification remain open.

`engine_disk_info` already verifies the bundled backend and canonical owned
VHDX path on a blocking worker. It now reads the containing directory's Windows
volume capacity through `GetDiskFreeSpaceExW`. This performs no write, engine
operation, privilege change, compaction or mount change. A capacity-read error
is reported independently, preserving the existing VHDX file-length observation.

The [Windows API contract](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getdiskfreespaceexw)
distinguishes bytes available to the current user, capacity available to that
user (which can be quota-limited), and physical free bytes on the volume. The
meter uses caller capacity minus available bytes. Physical free bytes are
reported separately and may exceed caller capacity under quotas. It is not a
Linux free-space measurement and cannot be used to infer that a particular
workload fits. The UI distinguishes Docker's shared/reclaimable usage, VHDX
file length and sparse/compressed allocation, and host-volume capacity. It also
explains that copying a live VHDX is not an application-consistent backup.

Use the canonical containing directory so redirected directories and mount
points resolve through Windows rather than assuming the user's C: drive. The
Win32 call accepts an existing directory and does not require a volume handle or
administrator rights. Paths are NUL-checked and terminated; outputs remain u64
and measurements outside JavaScript's exact integer range fail explicitly.
Zero total, available exceeding total or physical free, and OS errors refuse
the capacity observation. Physical free exceeding quota total is valid.

The existing Docker usage and native metadata requests share a view generation.
Cancel aborts the Docker client and invalidates both replies, while the read-only
native metadata call finishes independently. The generation remains owned until
both calls settle; navigation/unmount aborts and clears its ownership. No late
metadata reply may repopulate a cancelled or retired view. Refresh becomes
available after settling. Results completed before cancellation can remain and
are labelled. Host measurements are refreshed explicitly with disk usage.

Evidence: 86 JavaScript tests and 53 ordinary Rust tests passed; two new Rust
cases cover quota distinctions, invalid/oversized counters, existing-directory
reads and missing Unicode paths. `p2-host-capacity-native-20261025.json` records
the actual owned disk: file length 5,817,499,648 bytes; caller capacity
3,999,731,281,920 bytes and approximately 2.086 TB available at that observation.
`p2-host-capacity-driveinfo-20261025.json` independently observed the same total
through .NET DriveInfo afterward; free bytes changed, as expected during use.
No quota-enabled filesystem, network volume or enterprise restriction was
qualified. Ordinary API use and unit validation do not prove that whole matrix.

Native UI cancellation while metadata was pending suppressed its later capacity
result and returned the measure button after completion. That review found
outdated 'waiting' text after settling; the finalizer now replaces it with a
settled cancellation message. Final layout/read/cancel review is recorded in the
qualification aggregate. The preserved installer is unchanged and unqualified.
