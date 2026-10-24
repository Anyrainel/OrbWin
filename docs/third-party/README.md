# Third-party provenance

The backend is derived from Rancher Desktop 2 at the exact source lock commit.
Its Apache-2.0 license is preserved in `rancher-desktop-LICENSE.txt`, source
headers and all local patches. The checkout retains the original tests and notices.

Go module versions/checksums are inherited unchanged from upstream's root and
nested guest modules. The generated build manifest records source, patch and
binary hashes. `artifacts/backend/go-build-modules.txt` records the native binary's
Go build metadata. Rootfs, mkcert and nerdctl downloads are selected and verified
by the pinned upstream `rdd/dependencies.yaml` downloader; no unverified local
guest image override was used.

Frontend/native dependencies are locked in `package-lock.json` and
`src-tauri/Cargo.lock`. Compiler package files/hashes are in
`build-tools.lock.json`; the project-local compiler extraction retains package
license files. The OrbWin icon is an original code-native SVG in `assets/`.

Interactive terminals use MIT-licensed xterm 6.0.0 and its Fit addon; package
licenses remain in the installed dependency packages. A per-session Go helper
uses Moby client v0.6.2, the same SDK version as the pinned rdd. It attaches
directly to the Linux exec TTY; no ConPTY dependency/fork is retained.
`backend-tools/session/go.mod` and `go.sum` lock its dependency graph, and the
session build manifest binds the executable to its source hashes.
Native file pickers/export use `rfd`;
Windows registry ownership checks use `winreg`. Include these and their
transitive notices in the eventual distribution audit.

This is an initial provenance record, **not** a complete distribution license
audit or SBOM. Before redistribution, collect all applicable host/guest/tool
licenses/notices, inspect image package source obligations, and qualify the
bundled client/tool distribution separately from a source-built development app.
