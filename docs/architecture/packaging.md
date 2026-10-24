# Local Windows packaging

`npm run package:build` builds a per-user NSIS installer using the pinned Tauri
lockfile, the separate `tauri.package.conf.json` overlay and staged runtime.
The development config has no bundled resources, so a normal debug build cannot
accidentally adopt packaged discovery merely because it copied a runtime folder.
`package:prepare` verifies backend, helper-source,
helper-binary and Docker-tool checksums before copying a fixed allowlist. Old
binaries, test identities, credentials, logs and virtual disks are never package
inputs. A generated staging tree is replaced only after its fixed absolute path,
non-redirected parent and ownership marker are checked.

The installer supplies `runtime/backend`, `runtime/tools` and dependency notices
beside the application. Runtime state remains under `%LOCALAPPDATA%/OrbWinData/home`,
outside the installation. The app validates that home and WSL registration before
any lifecycle or update request. An existing development-proof registration does
not authorize an installed app to adopt its disk. The default NSIS installation
mode is current user; Windows/WSL prerequisites remain explicit. WebView2 uses the
standard Tauri bootstrapper when it is absent.
Installation and state must not contain one another; custom overlapping paths
refuse setup before state mutation. The initial proposed `OrbWin/home` state root
was superseded before any installed guest was created because NSIS uses
`%LOCALAPPDATA%/OrbWin` for application files by default.

The notice collector inventories the actual Go module metadata embedded in
Windows and built guest components and pinned client tools, resolved Windows
Rust dependencies and production npm packages. It retains license/notice files,
exact versions/checksums and upstream patches. Missing notices fail preparation.
Supplemental license texts omitted from crate archives are retained with exact
source URLs and hashes under `third-party/notices`. The MPL-covered selectors
crate's checksum-verified unmodified source archive accompanies its license.
Guest socat/libwrap source RPMs and notices remain in the bundle; the pinned base
distribution and guest dependency manifest preserve upstream provenance. An
inventory is not a claim of independent legal review or reproducible upstream
vendor binaries.

PE import inspection of the qualified rdd and Tauri application found Windows
platform DLL imports, with no separate MinGW/VC runtime DLL to copy from the
development toolchain. This inspection does not replace installed-product tests
or broaden the qualified Windows-version matrix. Build repeatability means
locked inputs and recorded manifests; byte-identical NSIS installers are not
claimed. Compare an installed application's hash with its installer payload.
Tauri logs bundle-type metadata patching during NSIS creation; in this
qualification the payload and installed hashes matched while the post-build
executable differed. The payload is the relevant installation comparison.

Local installers are unsigned during Phase 1 qualification. Public signing,
automatic update distribution and a broader compatibility matrix are later
release work. Installed first-run, restart, replacement and uninstall/state
retention require actual qualification before Phase 1 acceptance.

The English custom language file retains all strings from pinned Tauri CLI
2.12.4 (`993038c49185307dfc895ab7657d77ca0d5ed7c7`), under its Apache-2.0/MIT
terms, with the uninstall checkbox narrowed to window preferences. Custom NSIS
language files replace the defaults rather than merging a single override;
native maintenance-page inspection caught and corrected an incomplete file.

## Quiescence before file replacement

Closing the UI deliberately leaves healthy workloads running. Windows therefore
may keep the bundled rdd executable mapped after the window closes. Installation
must never ignore that lock, force-kill arbitrary processes, or schedule an
unverified replacement over a running backend. An explicit Settings action will
use the existing lifecycle operation lease and setup file lock, verify the owned
home/registration/build and any live daemon, then request `rdd service stop`.
rdd owns shutdown; no new supervisor is introduced. Cancellation stops only the
request client and must report that shutdown may already be underway. Data,
registration and identity remain unchanged. Completion also requires the backend
executable to accept an exclusive non-truncating write open, confirming Windows
released its mapping. A still-mapped file remains an error.

NSIS pre-install/pre-uninstall hooks will independently probe existing bundled
backend/helper executables without writing their contents. A locked file aborts
before copying/deleting files and directs the user to the explicit preparation
action. The installer does not stop workloads silently. After preparation, close
the UI and install/uninstall; starting the app again uses the same retained
engine identity. Same-pin backend changes still require the separately guarded
native update/setup transaction before publishing a new runtime identity.

These probes are snapshots, not an atomic lock spanning NSIS extraction. A
separate client can reopen a file afterward; Windows then refuses replacement.
An interrupted/partially copied application must be repaired by rerunning the
installer. Component/manifest checks refuse mixed runtime binaries before guest
mutation; engine disks and identity remain outside the replaced file tree.
