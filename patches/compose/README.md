# Compose 5.6.1 local Dockerfile read grant

## Dependency-floor revision (2026-10-25)

The current recipe builds `v5.6.1-orbwin.2`. In addition to the exact-file
permission patch below, `0002-http2-dependency-floor.patch` updates x/net from
0.58.0 to 0.60.0 and its selected x/crypto and x/text requirements to 0.57.0 and
0.42.0. The readonly resolved graph also selects x/term 0.46.0 and x/tools 0.49.0.
The patch changes only go.mod/go.sum. Its SHA-256 is
`2794b78f3fc23ead824a4f46693500785a866c0a446442cb413e236f3fa04c5f`.
The recipe admits exactly the original patch state or the two complete reviewed
patches, rejects unrelated checkout changes, and includes both patches in each
binary's source/legal record. This revision addresses the five HTTP/2 advisory
matches documented in `docs/security-response.md`; the isolated candidate's
symbol scan and focused checks are retained under
`artifacts/patch-replay/compose-security-candidate/`. Final-version binary hashes,
reproduction and runtime adoption require separate evidence. The historical
version-1 receipts below remain evidence for those exact bytes.

Recorded 2026-10-25. Isolated candidate; runtime locks and installed tools are
unchanged. This is a narrow Apache-2.0 upstream adaptation, not a new build engine.

## Provenance and observed failure

Repository: `https://github.com/docker/compose.git`.
Tag: `v5.6.1`, annotated tag object
`02f05e3787e5b2e83220923907bb014e6a36b359`, peeled source commit
`6cbc19a3fd59e305fb814dca4d854a4fb67d2941`.
The tag/commit were checked against GitHub and the isolated shallow clone at
`.upstream/compose`. This records object identity; no signature verification is
claimed. The existing official Windows binary reports module v5.6.1 / Go1.26.10.
The candidate uses the repository-pinned Go1.27.3 and identifies itself as
`v5.6.1-orbwin.1`. Upstream copyright, license and tests remain intact.

Real Dev Containers0.469.0 Compose builds failed before executing the build:
the extension selects a generated `Dockerfile-with-features` in its temporary
directory outside the configured build context. Compose5.6.1's Bake command grants
read access to local contexts/additional contexts but omits that explicit file.
Buildx0.38.1 then requests an additional exact `fs.read` grant and fails in the
noninteractive invocation. Failure receipts are:

- `artifacts/cli/orbwin-devcontainer-b9c00a920c20440788a41b2e47f226af/proof.json`.
- `artifacts/cli/orbwin-devcontainer-bac3d9531a244ce6916bbe1a190ebdd2/proof.json`:
  setting child-only `COMPOSE_BAKE=false` did not change the Bake path.

Both failed fixtures were cleaned with unchanged inventories and no cache growth.
The pinned [buildWithBake/localBuildPaths/bakeArgs source](https://github.com/docker/compose/blob/6cbc19a3fd59e305fb814dca4d854a4fb67d2941/pkg/compose/build_bake.go)
explains this: it checks BuildKit/plugin availability, does not consult
`COMPOSE_BAKE`, and builds read grants from contexts. Compose's build command
exposes no `--allow` option. A broad filesystem-entitlement bypass is not acceptable.

A separate path experiment confined **only the Dev Containers child process's**
TEMP/TMP to its new fixture build context. It passed Compose build/up/exec,
build-marker and Windows directory/file write checks, then reopened the same
container. Receipt:
`artifacts/cli/orbwin-devcontainer-550ae6750831417db7ab94de6309f7c9/proof.json`.
Owned inventories and Desktop/Rancher inventories/container states were restored
or unchanged; all three IDs/config hashes stayed unchanged. The exact built image
and temporary aliases were removed. Build cache remained:20→22 records,
49.7→53.57MB. It was not pruned. This is a harness path workaround; the ordinary
default installed Compose journey remains a separate qualification gate.

## Patch contract

`0001-explicit-local-dockerfile-read.patch` adds one exact regular file to the
existing per-service local read grants when all of these hold:

- The context is local, nonempty, and not a git/URL context.
- A Dockerfile path is explicit and the build does not use inline Dockerfile text.
- Resolution of that path relative to the context succeeds, including symlinks.
- The resolved path names an existing regular file.

It grants the resolved file itself, never its directory, a parent, a wildcard or
an arbitrary temporary directory. A later symlink replacement cannot broaden the
already generated path grant; Bake must validate its actual inputs against that
fixed grant. This preserves upstream path-based consent: it does not lock source
files or claim resistance to arbitrary concurrent filesystem replacement. The
test replaces the originally selected symlink after grant generation; it does not
establish a file-handle-based identity boundary for the resolved target. Existing
context/additional-context permissions and upstream build
ownership are preserved. Missing/nonregular paths get no new permission and fail
through upstream validation/build handling. No Dockerfile is copied, rewritten,
executed or deleted by this helper. No global environment, Docker context, guest
lifecycle, credentials or endpoint is changed.

The focused tests exercise relative/absolute paths, Windows native/forward-slash
drive syntax, missing/default/inline/directory paths, git/URL contexts, preserved
additional-context handling, symlinks outside the context and fixed exact Bake
arguments after replacing a selected link. Symlink cases passed natively on this
Windows host without elevation and as UID1000 in Ubuntu; no skip was needed.

## Reproduction and evidence

`node scripts/build-compose.mjs --test` verifies the exact tag/commit, applies only
the reviewed patch in the isolated checkout, rejects unrelated changes, uses
`-mod=readonly` with the pinned Go toolchain, and writes only:

- `artifacts/compose/docker-compose.exe`.
- `artifacts/compose/docker-compose` (Linux amd64).
- `artifacts/compose/compose-manifest.json` with source/patch/binary hashes and
  separate compiled-source and license inventories for both platforms. Original
  top-level binary/license fields continue to describe Windows for compatibility.

It never changes `runtime-tools.lock.json`, staged runtime tools or installed
binaries. Module fetching required `go.uber.org/goleak v1.3.0` and
`github.com/creack/pty v1.1.24` from the existing upstream go.sum; other dependencies
were available in the module cache. No dependency versions/go.mod/go.sum changed.
The Windows candidate SHA-256 is
`8ae10b9ac12cf3bc5b8dc748a19561a844551f16c07354cac7f5480f420eae80`.
The Linux candidate SHA-256 is
`483c6e7ebcd41f661f16fd8a564206f72951b31fd15dadf4056eab006796d4ad`.
Patch SHA-256 is
`4471013594081363973c1ad072aa53809cdf951f3f4efd1998eda5c793648766`.

`node scripts/qualify-cli-compose-patch.mjs` retains test output and a proof under
`artifacts/compose/tests-c4597ec1-dd03-4cc1-a521-69062d3e513b/`:

- Focused native Windows tests pass while compiling every production GoFile.
- A Linux test binary compiled from the full package runs the selected new and
  existing Bake tests successfully in Ubuntu as UID1000.
- Both standalone candidates report `5.6.1-orbwin.1`. Linux execution selects
  Ubuntu GUID `{231810b0-fa21-48a8-b0a7-86f9a1360029}` and verifies UID1000 before
  running the candidate; it does not depend on the default WSL distribution.
- The ordinary native Windows package test build has a pre-existing compile
  failure: untagged `down_test.go` and `service_containers_test.go` refer to
  `newStartTestService`/`serviceContainer` in `start_test.go`, which is excluded by
  `!windows`. The failure is retained; tests were not removed or retagged.

These are focused source tests, not the full upstream suite or installed release
acceptance. Installed coexistence/data/cancellation and broader Compose regression
remain phase gates.

## Candidate live proof and exact-file boundary

`artifacts/cli/orbwin-devcontainer-fbfa2aa3f30e468284a78533a27c420f/proof.json`
records a successful real Dev Containers0.469.0 candidate build/up, build marker,
exec, host directory/file writes and reopen of the same container with ordinary
TEMP/TMP and no filesystem bypass. Exact fixtures were removed. All three engine
IDs, container IDs/states, image/volume/network inventories and config bytes stayed
unchanged. BuildKit cache grew28→31 records,53.61→53.64MB; it remains and was not
pruned.

Dev Containers probes `docker compose version --short` before considering its
`--docker-compose-path` fallback. This proof uses a harness-only adapter that
rejects only that exact probe, forwarding every other argument literally to the
frozen OrbWin alias. Adapter source/test/binary hashes and actual candidate
commands are retained in the receipt. This validates the patched standalone
candidate; installed/default plugin discovery still needs a separate proof.

Pinned [Buildx flag handling](https://github.com/docker/buildx/blob/v0.38.1/commands/bake.go)
uses a string-array entitlement flag, without CSV splitting.
[Entitlement matching](https://github.com/docker/buildx/blob/v0.38.1/bake/entitlements.go)
preserves the suffix after `fs.read=`, canonicalizes paths and compares exact files
or directory prefixes. Brackets are not globs; only the whole `*` is special.
The live receipt proves an exact-file grant with brackets/commas in directory
components succeeds while a similarly named sibling file is denied. A separate
receipt `orbwin-devcontainer-a748b96d7f804e3a9d3541c62a3f7de5` accepted permission
for a combined bracket/comma Dockerfile basename, but BuildKit later failed file
lookup after transfer. Its cause and broader filename support remain unresolved.
These checks do not claim inode or content immutability.

## Build attestation and proposed local-source adoption

Both candidates use the same reviewed patch, source commit and Go1.27.3 compiler,
CGO disabled, GOAMD64=v1, local toolchain, readonly modules, trimpath and disabled
VCS stamping. The recipe clears inherited Go flags/experiments/workspaces. The
manifest pins the build script, patch, Go executable/archive and upstream lock;
each target records188 compiled main-module source hashes. Windows has110 compiled
modules and Linux108, checked against each binary's embedded module metadata.
Each binary's `licenses/<binary-sha>/index.json` hashes collected module legal
files, upstream Apache license/notices, Go LICENSE/PATENTS and the source patch and
this provenance record. This is source legal-file accounting, not legal approval
or a completeness guarantee. `node scripts/qualify-cli-compose-repro.mjs` compares
repeat-build bytes and source/license manifest identities on this host; its
receipt is separate from functional qualification.

Proposed integration for the root-owned staging/lock/notices changes:

- Pin the local source commit/tag object, patch/script/toolchain hashes and both
  artifact hashes; keep the original source and patch available for redistribution.
- Map Windows to `docker-cli-plugins/docker-compose.exe`, Linux to
  `linux/docker-compose`. Verify the complete source attestation and actual
  artifact SHA before copying. Refuse drift or missing candidates rather than
  silently substituting the official unpatched download.
- Copy the corresponding indexed legal files and verify every listed hash;
  merge both platform inventories into collected notices with module versions and
  upstream/patch provenance. Retain the Apache source notices.
- Run the actual installed/default plugin-discovery journey after adoption.

This proposal does not modify runtime locks, staging or installed binaries.
