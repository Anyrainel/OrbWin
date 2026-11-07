# Buildx v0.38.1 HTTP/2 dependency floor

Recorded2026-10-25. Local build recipe/provenance only; activation, package and
runtime acceptance remain separate. No Docker API/module migration is included.

## Exact source and patch

Upstream module `github.com/docker/buildx v0.38.1`, repository
`https://github.com/docker/buildx`, source commit
`0dbe87f36ed472fe36b6796c3f70b4210a731fc5`.
Go module checksum `h1:7J7nReQndOnDTUFjun6otmQqehoi9sCOvRwFjkngYlc=`;
go.mod checksum `h1:4lEDxmaYDJ17HtXTND9A53BaDoHCJbk5WUgcmfipN7Q=`;
source module zip SHA-256
`4cd8755c2468a8a226cdb22bbbf18aee4a876d98f3d4f38e34f2a5f519f9b8ee`.
These identify the Go module archive, not a signed Git-tag verification.
All source copyright/license bytes remain intact. The reviewed archive contains
no symlinks. `source-baseline.json` records every original file hash; its hash
is fixed in the recipe/verifier. Module-cache bytes must also pass Go's archive
verification before copying, so a valid zip cannot mask an edited cache tree.
After exact patched-tree verification, the recipe also runs `go mod verify` on
the resolved build graph, including the corrected x/net dependency.

`0001-http2-dependency-floor.patch` changes only go.mod/go.sum:
`golang.org/x/net v0.59.0 → v0.60.0` and its two checksums. SHA-256
`496f3d23feb5c5e13a27883062a11b07a14c6ffbceb5242c944dde5917e52cf5`.
No other module version, crypto implementation, old Docker dependency or Go
source is changed. The resulting source identifies as **v0.38.1-orbwin.1**;
its revision field retains upstream commit identity, while the manifest/patch
describe the local change. Go1.27.3 is pinned instead of upstream Go1.26.10.

## Reproduction and failure semantics

`node scripts/build-buildx.mjs --test` validates fixed inputs, copies/patches only
`artifacts/buildx/source`, runs focused Windows utility/HTTP2 tests and
Windows/Linux vet, and builds CGO0 amd64 binaries at
`artifacts/buildx/{docker-buildx.exe,docker-buildx}`. Default invocation performs
the same fixed builds/provenance without optional tests. Neither form changes
runtime locks, active tools, engines, global defaults or installed programs.
Unknown arguments and alternate recipe/source/tool identities are refused.
All builds use readonly modules, explicit targets and bounded compiler parallelism.

Source verification rejects modified/missing/extra files and links, including
restored trees that disagree with the exact baseline plus reviewed patch.
Existing divergent binaries/manifests/legal files are preserved and refused.
An interrupted `.building` file is retained for inspection; after a completed
build its exact equality to the accepted output is verified before removing it.
No output is silently accepted because a build process exited0.

Optional tests assert observed RUN events. Go1.27's default x/net HTTP2 wraps
stdlib HTTP2; selected legacy regression tests require the **test-only**
`http2legacy` tag. Production builds use no such tag. This prevents counting a
successful no-tests invocation as regression coverage. Utility package
cobrautil has no tests; buildflags/confutil/osutil tests provide actual coverage.
No engine/native-client journey is asserted by these unit/build checks.

The earlier isolated preparation is preserved at
`artifacts/patch-replay/buildx-security-candidate/`: module-cache read-only file
attributes first refused the metadata update, and a subsequent attempt refused
overwriting the preserved read-only baseline. Both failed receipts remain.
Only that isolated copy's attributes were corrected after byte-for-byte checks;
no original cache permissions/source changed. Its initial HTTP2 probe ran no
tests due to Go1.27 tags and was explicitly excluded from passing coverage;
subsequent default-API and legacy-tag tests actually ran. Candidate versions
and hashes differ from these final-version artifacts.

## Staging schema and legal-index contract

Root integrates `scripts/buildx-provenance.mjs`; importing it performs no I/O,
build or lock read. `verifyBuildxSource(workspace,pins,{buildMissing:false})`
returns `{manifest,manifestPath,legal}` where each legal record is
`{target,directory,indexPath,index,files}`. `copyBuildxProvenance` verifies first,
then copies `buildx-manifest.json` and each target's index/files to a nonredirected
destination and returns the same proof. `buildMissing:true` can invoke only the
fixed hash-verified recipe when required outputs are absent. Altered existing
provenance is refused, not rebuilt over.

Required `localSources.buildx` fields are the fixed metadata below **plus all
`buildx-manifest.json.sourcePins` fields and `manifestSha256` of that exact
manifest**. This is the proposed lock schema; this recipe does not edit it:

```json
{
  "kind": "orbwin-buildx-v1",
  "recipe": "scripts/build-buildx.mjs",
  "repository": "https://github.com/docker/buildx",
  "tag": "v0.38.1",
  "commit": "0dbe87f36ed472fe36b6796c3f70b4210a731fc5",
  "module": "github.com/docker/buildx",
  "moduleVersion": "v0.38.1",
  "moduleSum": "h1:7J7nReQndOnDTUFjun6otmQqehoi9sCOvRwFjkngYlc=",
  "goModSum": "h1:4lEDxmaYDJ17HtXTND9A53BaDoHCJbk5WUgcmfipN7Q=",
  "sourceZipSha256": "4cd8755c2468a8a226cdb22bbbf18aee4a876d98f3d4f38e34f2a5f519f9b8ee",
  "sourceDirectory": "artifacts/buildx/source",
  "version": "v0.38.1-orbwin.1"
}
```

`sourcePins` includes `buildScript`/`buildScriptSha256`, `patch`/`patchSha256`,
`goExecutableSha256`, `goArchiveSha256`, `upstreamLockSha256`,
`provenanceScript`/`provenanceScriptSha256`, and
`sourceBaseline`/`sourceBaselineSha256`. Fixed paths are enforced. Each runtime
tool uses `localSource:{name:"buildx",target:"windowsAmd64"}` for
`docker-cli-plugins/docker-buildx.exe`, or target`linuxAmd64` for
`linux/docker-buildx`; its ordinary `sha256` must match the selected record.

The manifest contains `binaries.windowsAmd64` and `.linuxAmd64` with `path`,
`platform`, `binarySha256`, `licenseDirectory`, `licenseIndexSha256`,
`compiledModules`, and `compiledSourceHashes`. Complete patched source hashes
are in `sources`; the main-module compiled input subset is independently
recorded per platform. Old top-level binary/license fields describe Windows,
matching Compose's existing compatibility convention.

Each `licenses/<binarySHA>/index.json` has `schema:1`, `binarySha256`, `platform`,
`sourceCommit`, `patchSha256`, `embeddedModuleMetadataVerified:true`,
`modules`, `toolchain`, `source`, `modulesWithoutRootLicense:[]` and an explicit
legal-review limitation. Module inventory is checked against binary build
metadata. Every module record has `module`, `version`, `files` and
`rootLicensePresent:true`. Files record relative `path`, `bytes`, `sha256`;
module legal paths use a deterministic module/version-derived key. Go toolchain
LICENSE/PATENTS and the patch/README/baseline source records are included.
The verifier rejects missing root licenses, altered files, duplicate/escaping
paths and redirects. This is collected provenance, not a legal completeness
opinion. Root owns distribution review and qualification of the final outputs.

## Advisory boundary

The isolated candidate scan removed the five x/net IDs but retained seven other
IDs; no ignore rule or blanket clearance was introduced. The earlier exact
source-closure/PCLN evidence distinguishes legacy OpenPGP and daemon package
absence from unrelated module metadata. Two UNREVIEWED entries broadly attribute
name-generator/credential-helper symbols; raw results remain. ProtonMail signing
code, external providers, guest daemon/BuildKit versions and actual runtime
behavior remain separate review boundaries. Rescan the final-version hashes and
qualify actual Windows/WSL/Compose/DevContainers/private-auth/cancellation flows
before acceptance. No prior receipt silently qualifies a changed binary.
