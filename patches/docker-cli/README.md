# Docker CLI managed plugin-directory candidate

The source baseline is Docker CLI tag `v29.9.2`, annotated tag object
`0cc0fc64632a9ee9cab5884b99fee3b43f08a3e2`, commit
`a82e528901260c8df6073311379eac65f92db204`.

Patch 0001 is the original LF Rancher Desktop wrapper credential override from
wrapper commit `3ef7c31eb807e504c274f2b744f62b0a719fc97d`. Its Linux linker override
remains `wincred.exe`. The upstream wrapper's `git am` creates a different commit
(the released binary reports `c5aca45`); this build does not claim that commit or
bit identity. Patch 0002 adds an explicit managed plugin search mode only when
`ORBWIN_DOCKER_MANAGED_PLUGINS` is present. A valid absolute directory becomes the
sole plugin directory. Empty, relative, overlong or NUL/CR/LF values return no
plugin directories, without config, user or system fallback. Unset retains the
upstream extension mode. The caller must overwrite inherited environment values
and retain actual native cache leases for the selected directory and binaries.
This environment routing value is not itself a filesystem-origin capability.

`scripts/build-docker-cli.mjs --test` creates a new isolated candidate directory,
exports and verifies the pinned Git tree, applies both exact patches, and copies
`vendor.mod`/`vendor.sum` to generated `go.mod`/`go.sum` inside that frozen tree
only. The upstream checkout remains unchanged. All builds use `-mod=vendor`,
network-disabled module resolution, a private build cache, and a separately
extracted pinned Go 1.27.3 archive with full toolchain/source catalogs. Windows
and Linux amd64 are compile-only. Only the pure `TestOrbWin` plugin-directory
tests execute; candidate Docker binaries are never run by the recipe.

The candidate version is `v29.9.2-rd-orbwin.1`; linker GitCommit reports the actual
upstream base plus `orbwin.1`, and deterministic BuildTime comes from the base
commit. Full source/vendor, generated module files, compiler archive/executable,
recipe/patches, embedded module metadata and legal files are recorded. Notices
include source-root, compiled vendored-module, Rancher patch source and toolchain
license files. Missing root notices remain explicit; this inventory is not a
legal-completeness claim.

The recipe produces review candidates only. It never changes runtime-tools.lock,
stages selected tools, invokes a Docker CLI/engine or activates plugin mode. Native
cache lease integration and Windows/Linux Docker API compatibility require
separate acceptance evidence before selecting the candidates.
