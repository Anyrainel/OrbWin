# Security response for pinned OrbWin versions

Recorded 2026-10-25 for P2-D07d. This is the development response process adopted for Phase 2; public support, signing, release publication and a verified private reporting channel remain release gates. It is not a claim of continuous monitoring, a staffed security team, a completed vulnerability audit or absence of vulnerabilities.

## Accountability and intake

The OrbWin maintainer owns intake and release decisions. The relevant backend, desktop, networking/trust, storage/migration or packaging implementer owns reproduction and remediation; one named person must accept each case before it can be closed. With one maintainer, that person holds both roles and records the limitation. A scanner result never transfers responsibility to upstream.

Check primary advisory sources before every candidate, after changing a pin or vendored patch, and at least weekly while a version is supported. A credible active-exploitation or privilege/data-isolation report triggers review when received. This document creates no background job or external message. Record date, source URL, advisory revision/identifier, exact affected pin and owner.

| Component family | Primary intake source |
| --- | --- |
| Selected rdd and Rancher adaptations | [rancher-desktop-2 security policy](https://github.com/rancher-sandbox/rancher-desktop-2/security), selected upstream release notes and local patch ancestry |
| Moby runtime/API | [Moby advisories](https://github.com/moby/moby/security/advisories) |
| Compose/Buildx | [Compose security](https://github.com/docker/compose/security), [Buildx security](https://github.com/docker/buildx/security), exact release/source records |
| Tauri/Rust crates | [Tauri advisories](https://github.com/tauri-apps/tauri/security/advisories), [RustSec](https://rustsec.org/advisories/); match every transitive lock occurrence |
| Go compiler/modules | [Go security](https://go.dev/doc/security/); match helper and upstream module graphs |
| Maintenance image | [Alpine tracker](https://security.alpinelinux.org/), distribution revisions/backports |
| Additional guest RPMs | [SUSE security](https://www.suse.com/support/security/), exact package/source revisions |
| Optional Kubernetes | [Kubernetes security](https://kubernetes.io/docs/reference/issues-security/security/), selected cluster distribution's official advisories |
| Windows/WSL/WebView2 | [Microsoft Security Update Guide](https://msrc.microsoft.com/update-guide), official platform release/security records; installed versions recorded separately |

These links were opened on 2026-10-25; Microsoft's guide requires JavaScript and its advisory content was not inspected. Availability of a page does not mean every advisory was assessed. npm/Rust/Go scanner output is supplementary: record tool/database versions, dates, exit status and errors. Offline/stale databases and failed scans mean unknown, never clean.

## Pin inventory and provenance

For each candidate record source-tree identity including dirty changes, installer/binary SHA-256, manifest hashes, source URLs/commits, local patch hashes, dependency graph, licenses/notices, target/features and installed engine/kernel identities. Preserve that immutable inventory; do not replace an old assessment with today's locks. A filename/tag or marketing version alone cannot identify shipped bits.

Repository observations below do not certify an installed build:

| Boundary | Authoritative records | Observed pin/sample |
| --- | --- | --- |
| Upstream/backend | `upstream.lock.json`, `patches/rdd/`, `artifacts/backend/build-manifest.json` | Source `af6511ad4e065e24e7258f575a0fc090c10cdbda`; Go 1.27.3; twelve listed patches |
| Windows/Linux Docker clients | `runtime-tools.lock.json` | Docker CLI 29.9.2, locally patched Compose 5.6.1-orbwin.1, Buildx 0.38.1; per-platform SHA and Compose source/patch/build provenance |
| Helpers/modules | Each `backend-tools/{session,credentials,cli}/go.mod` and `go.sum`, binary manifests | Language version 1.27.0; session Moby client v0.6.2/API v1.56.1; Kubernetes modules v0.36.4 |
| Rust desktop graph | `src-tauri/Cargo.lock`, compiled binary/target/features | Tauri 2.12.5, tauri-build 2.7.4, Tokio 1.54.1, serde 1.0.229, serde_json 1.0.154, rfd 0.17.2, sha2 0.10.9; include all transitive Windows/WebView crates |
| Frontend/build graph | `package-lock.json`, bundled output | React/react-dom 19.3.1, Tauri API/CLI 2.12.4, xterm 6.0.0; Vite 6.4.5, TypeScript 5.7.3 |
| Maintenance image | `maintenance-image.lock.json`, `maintenance-sources.lock.json` | linux/amd64 root `sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30`; archive SHA `4a785a3d0998a81d8f0bf2d4d2949bc4905b54170bd20fd4d17707525de392cf` |
| Maintenance package/source graph | Full revision/source/notice lists in `maintenance-sources.lock.json` | Alpine release 3.21.9-r0, BusyBox 1.37.0-r14, musl 1.2.5-r12, libssl/libcrypto 3.3.7-r2, zlib 1.3.2-r1; sample does not replace full list |
| Additional guest packages | `guest-packages.lock.json` | libwrap 7.6-160000.2.2 and socat 1.8.0.2-160000.2.3, binary/source RPM hashes |
| Build-only compiler packages | `build-tools.lock.json`, actual build record | MSYS2 UCRT64 GCC 16.2.0-6 and full pinned list; distinguish build-host and shipped risks |
| Engine/guest/cluster/platform | Actual distribution package database, runtime status, installed candidate record | Capture per candidate. rdd commit and CLI version do not identify every kernel, daemon, containerd/k3s or platform component. External source engines have separate compatibility records. |

No complete guest SBOM or installed-candidate advisory assessment was produced by this documentation task. Keep that gap open until the actual guest payload is inventoried and assessed. Hashes identify recorded bits; they do not establish publisher trust or vulnerability absence.

## Triage and response targets

Each case records ID/date, reporter consent/contact if supplied, primary advisory/aliases, vulnerable/fixed ranges, every matching pin/hash/build graph, shipped-versus-build-only status, entry point, required privileges, attacker inputs, cross-user/host/engine/data impact, exploit evidence, confidence/unknowns, reproduction constraints, owner, disposition, next review, mitigation and validation. Use affected, not affected with evidence, fixed in candidate, or unknown. Backported fixes require exact distribution revision evidence. An image tag or disabled UI control does not prove an exposed backend is protected.

Internal expectations begin when the maintainer receives a report; these are planning targets, not a public SLA:

| Impact | Initial target | Follow-through |
| --- | --- | --- |
| Credible active exploitation, host escape, credential/key disclosure, cross-engine corruption | Same day; assign owner immediately | Within 24 hours record containment or why unavailable; reassess daily; hold affected release |
| Other plausible high-severity isolation/destructive fault | One business day | Initial applicability and mitigation/fix plan within three business days; reassess twice weekly |
| Moderate/low or applicability unknown | Three business days | Assessment/owner within ten business days; dated next review |

Record missed targets and escalate to the maintainer; do not invent an on-call team. Mitigations state workload/data consequences and reversal. Never stop engines, revoke trust, delete data or change global configuration automatically as security response.

## Patch, qualification and release

Prefer the smallest supported upstream patch/pin change. Review provenance, compatibility, license/source obligations and rebased patches. Explain changed trust boundaries, ownership, cancellation, persistence and downgrade/recovery. Use synthetic reproductions; do not test exploits against user workloads or third parties. Validate relevant unit/race/fault tests, credential/trust isolation, migration/data preservation, offline bundle integrity, installed upgrade/downgrade and three-engine coexistence. Apply the quality release gate even for urgent fixes; a mitigation may be safer than an unqualified backend replacement.

A changed lock or passing development test is not a published fix. Bind tested installer hash and actual component versions to affected/fixed ranges, user action, limitations and recovery. Acceptance-invalidating product-owned defects block Phase 2 exit. External risks require explicit applicability/mitigation and release-owner disposition; unknown high-impact exposure keeps its gate open. Coordinate upstream disclosure through the current verified policy, with human authorization before external transmission. Preserve original evidence and reopen contradicted assessments.

## Diagnostics and reporting

Export allowlisted snapshots only with explicit user review. Exclude tokens/credentials, headers, private keys, trust stores, proxy URLs, registry authentication, environment, workload names/labels, kubeconfig, bind paths and raw logs by default. Raw inspect, archives and source/config files are not secret-free diagnostics. Check minimization/redaction with synthetic secret fixtures. No automatic upload, attachment, browser submission or background telemetry is implied; sharing remains a separate explicit action.

Authenticated GitHub repository lookup by the integrating agent on 2026-10-25 verified `Anyrainel/OrbWin` exists as a **private** repository with authenticated admin/pull access. Earlier unauthenticated reads of its issues/security pages returned 404; these do not establish absence. The CLI issue URL is therefore access-restricted, not a verified public reporting destination. A private repository alone does not establish an approved confidential vulnerability-reporting policy, access/retention rules or an external reporter's access. No issue or message was sent. The public issue destination and private security-reporting policy remain pending verification. Before external release the maintainer must verify usable channels, handling/access/retention, support expectations and a public policy. Until then keep sensitive findings local and request a verified channel through existing authorized collaboration. Do not guess an email or substitute an upstream channel for an OrbWin-specific confidential report.

## Worked advisory assessment (limited sample)

### Frontend registry audit — 2026-10-25

`scripts/audit-frontend.mjs` ran npm 11.16.0 / Node 24.18.0 against exact isolated
copies of the root manifest and lockfile. Both user/global npm configs were empty,
the child environment omitted account/proxy settings, and the registry was
explicitly `https://registry.npmjs.org`. No install, fix or lifecycle script ran;
the original files and copied bytes were verified unchanged. As documented by
[npm](https://docs.npmjs.com/cli/commands/npm-audit/), this query sends package
names/versions to the registry advisory endpoint.

The dated report completed with status 0 and reported zero known vulnerabilities
for its 136-package graph, including development dependencies. Receipt/report:
`artifacts/qualification/p2-npm-audit-e51b7b6b-0873-4d55-880b-3739401799e4/`.
Lock SHA-256: `b4af71b3f5bef4b5c6f87e70cf24356da38cd2c2e6b9c01e1755ef8f3a19b756`.
The registry exposed no immutable advisory database revision. This is a
supplementary version-range scan, not an installed/bundle integrity check,
malware assessment, other-ecosystem audit or vulnerability-absence claim. Rust,
Go, guest, runtime and platform candidate-wide assessments remain open.

### Go binary advisory pass and SDK correction — 2026-10-25

`scripts/audit-go-binaries.mjs` uses pinned govulncheck v1.8.0 (tool SHA
`1e6981feb46fe2026fa4ffe83059c86ba6cf961150bd2948e1c6e87f44c76f87`),
binary/symbol mode, an isolated profile with telemetry explicitly off, and the
public Go database dated `2026-10-21T00:27:29Z`. It does not execute the scanned
binaries. Both exact binary hashes and raw scanner protocol messages are retained.
JSON mode exits zero even when findings exist; scan completion is not acceptance.
Binary symbols do not establish runtime reachability, and missing findings do not
establish absence of vulnerabilities or cover unknown main-module versions.

Initial receipt `p2-go-audit-50f679f2-555c-4e00-bab0-d75623d8d544` found SDK
HTTP/2 symbol matches against x/net 0.56.0. The local SDK module now pins x/net
0.60.0 and its required x/text 0.42.0, x/sys 0.48.0 and x/term 0.46.0. This
addresses the version ranges for
[GO-2026-6603](https://pkg.go.dev/vuln/GO-2026-6603),
[6610](https://pkg.go.dev/vuln/GO-2026-6610),
[6611](https://pkg.go.dev/vuln/GO-2026-6611),
[6612](https://pkg.go.dev/vuln/GO-2026-6612) and
[6617](https://pkg.go.dev/vuln/GO-2026-6617), and the module-only x/text finding.
All SDK package tests and vet pass. Rebuilt SDK SHA
`8f118fb18b233c70a6cf7c9d2284de7c3b02fca90f6be09395cdd9943cfdb8d5`
has zero reported findings in the repeat receipt
`artifacts/qualification/p2-go-audit-52b624b6-daaa-4874-b92b-b7fb1861804b/`.
This is a candidate-source/binary correction; installed qualification remains open.

The same repeat scan completed for 14 distinct Go binaries plus the identical CLI
alias. Raw findings remain in the unchanged upstream rdd (13 advisory IDs), each
Buildx platform binary (12 IDs), and each locally patched Compose binary (6 IDs).
Follow-up extraction found that govulncheck extracted no symbols for rdd/Buildx
and populated function traces from its advisory-metadata fallback. The earlier
11 reported function IDs per such binary are not evidence that those functions
are present or reachable. Compose reports four function IDs; preserve that
reported evidence separately from source/runtime applicability. Their
applicability, supported fixes/rebuilds and regression qualification remain open;
do not treat those components or Phase 2 as security accepted. The CLI and
credential helper each have a module/package-only
[GO-2026-5024](https://pkg.go.dev/vuln/GO-2026-5024) match for x/sys 0.20.0;
the scanner reports no matching vulnerable `NewNTUnicodeString` symbol. This is
limited binary evidence, not a general safety disposition. The rebuilt SDK, four
guest helpers and both Docker CLI binaries report zero findings. Active engine
bits, installed components and global settings were not changed by these scans.

The [upstream applicability triage](verification/phase-2-go-advisory-triage-2026-10-25.md)
covers all 19 unique IDs across rdd/Buildx/Compose with exact ranges, primary
sources, entrypoint preconditions and candidate fixes. It distinguishes rdd's
Kine/SQLite management datastore from an active etcd server, client binaries from
the actual guest daemon, and questionable Windows daemon-symbol attribution from
verified reachability. No finding is dismissed solely by a missing UI path.
Exact extraction/independent validated host-runtime name tables and production
command dependency closures are retained in
`artifacts/qualification/p2-openpgp-attribution-20261025/`. They agree that the
deprecated `golang.org/x/crypto/openpgp` implementation is absent from the exact
rdd, both Buildx and both isolated Compose-candidate command binaries. For those
five artifacts, GO-2026-5932 is assessed **not affected via that absent vulnerable
package**; its raw module finding remains. This does not assess Buildx's different
ProtonMail signing implementation, the guest engine or future pins. The same
evidence explains the spurious Windows old-Docker-daemon function attribution;
it does not clear the separately running daemon's copy/auth/plugin behavior.

Current SDK/CLI/credential extraction does have real symbol metadata (31,765 /
4,858 / 4,309 entries), so those current-binary results are not the same fallback.
The overwritten initial SDK binary was not independently extracted; its old
reported matches must not be relabelled as proven extracted symbols.

The isolated Compose and Buildx dependency-floor candidates build/test and remove
the five x/net matches in repeat scans. Reproducible adoption recipes are being
prepared; the final-version Compose candidate also passes focused native tests
and a byte-identical same-host repeat build. Active runtime pins and installed
bits still await qualification/adoption.

The development runtime/package subsequently adopted the fixed-source
Compose `v5.6.1-orbwin.2` and Buildx `v0.38.1-orbwin.1` candidates. Installed
executables and running engines were unchanged. Lock SHA is
`37528c5f2505535c07bb90f65347aca6576be9be7d038b7b091aef85adc7a0e1`.
Fixed recipes pin the bundled GOROOT, disable user GOENV, verify module-cache
integrity, reject source drift and retain both platform legal inventories.
Compose has 110/108 compiled modules; Buildx has 212/210, all with collected root
licenses. Same-host repeat builds match. Staging verifies original cache hashes
and its prior manifest before replacing only generated tools, then writes the
new manifest. Packaging reports 772 resolved notices and zero missing sources.

`artifacts/qualification/p2-client-security-27a83c1f-b48f-42e1-bcdc-bf59b76ce544/`
scans the exact four final staged binaries with actual extracted symbols (no
fallback), retains all raw findings and confirms the five HTTP/2 IDs are absent.
Buildx retains seven raw advisory IDs; Compose retains one. The separate offline
source-closure receipt verifies the final source/manifest identities and absence
of legacy OpenPGP and old Docker daemon packages in those production closures.
That supports an artifact-specific package-absence disposition for the legacy
OpenPGP match; the remaining module-only and overly broad Docker-package
attributions still require their separate triage. It does not clear ProtonMail
crypto, external providers, guest packages or other advisories. Default-client
runtime requalification and installed acceptance remain separate gates.

The complete instrumented repeat is retained at
`artifacts/qualification/p2-go-audit-569c6c8a-888d-439f-833c-ca50ede1e11a/`.
It records both raw extraction and scan hashes for 14 distinct binaries plus
the identical CLI alias, including the current WSL project helper. Windows and
Linux Docker CLI extract 37,703 and 37,545 symbols, but expose no dependency
module inventory or known main-module version. Their zero reported findings
therefore leave third-party dependency coverage unresolved. The services helper
also has zero dependency modules; unlike the vendor Docker CLI, its local source
and build manifest can establish its standard-library-only graph separately.
Current Compose has actual extracted symbols on both platforms; rdd and the
upstream Buildx binaries still use advisory metadata fallback. Extraction does
not prove runtime reachability. An earlier instrumented attempt
`p2-go-audit-db5c8d9f-78fb-4cc8-8838-9fab95145366` incorrectly required an emitted
modules array even when empty; it remains incomplete evidence and is superseded
by this repeat, which accepts an omitted empty array.

### Rust lock advisory pass — 2026-10-25

`scripts/audit-rust-lock.mjs` scanned an exact copy of the 434-dependency Cargo
lock using cargo-audit 0.22.2, fresh profile/config/cache and the explicit public
RustSec database. No fix/install ran against the project and its lock bytes stayed
unchanged. Receipt `artifacts/qualification/p2-rust-audit-222975e5-ed0e-4b82-acc0-b2177c55fccf/`
records tool SHA `4ac6ae9217aed42a5c285187619f9babed93a9cbd8733ad239e9e86866dbe1dc`,
database commit `21d909e4c8bd4e14c7aea58bf980c881a0053ab1`, updated
`2026-10-24T10:30:13+02:00`, and the lock hash used in the IDNA sample below.

It reports zero vulnerabilities, plus the unmaintained proc-macro-error 1.0.4
([RUSTSEC-2024-0370](https://rustsec.org/advisories/RUSTSEC-2024-0370.html)) and
unsound glib 0.18.5
([RUSTSEC-2024-0429](https://rustsec.org/advisories/RUSTSEC-2024-0429.html)) warnings.
Captured `cargo tree --locked --offline` inverse trees show both packages in the
all-platform GTK dependency branch and neither in the current default-feature
`x86_64-pc-windows-msvc` graph. This supports excluding those two warnings from
that Windows graph only; a future platform/feature change must reassess them.
Installed binary/lock correspondence, other ecosystems, native libraries and
platform assessment remain separate gates.

### Host backend source floor adoption

The reviewed x/net v0.60.0 correction is now registered as
`patches/rdd/0014-host-http2-floor.patch` on top of typed-status13. The guarded
source transaction and clean14 replay are recorded in `patches/rdd/README.md`.
Only host go.mod/go.sum selection and the source lock changed; nested guest
modules and running binaries remain unchanged. The earlier full12/net60 and
full13/net59 candidates do not establish final14 binary correspondence. Final
host build attribution/scan, guest dispositions, remaining advisories and
installed/runtime qualification remain open.

### IDNA version-range sample

The [2026-10-26 Windows backend follow-up](verification/phase-2-rdd-advisory-followup-2026-10-26.md)
adds independently reviewed exact-binary function-name and fresh production-source
evidence for absent CEL native-type-provider and raw DHCP-client implementations.
Its later exact-symbol assessment also finds the four affected etcd TLS listener
entrypoints and `EtcdServer.Txn` absent from that same packaged Windows PE,
including its stored inline names; their packages are present. These four
artifact-specific dispositions do not cover other binaries/guests, general
TLS/RBAC behavior or final-candidate correspondence. Earlier scan and graph
records remain preserved with their original source hashes.

Case `P2-SEC-IDNA-20261025`: [RUSTSEC-2024-0421 / CVE-2024-12224](https://rustsec.org/advisories/RUSTSEC-2024-0421.html), primary record inspected 2026-10-25. It describes inconsistent handling of certain Punycode names with possible application privilege effects and lists versions at least 1.0.0 as patched. `src-tauri/Cargo.lock` contains exactly one `idna`, version 1.1.0. Lock SHA-256: `97c0853d2529dd57fb24f8b68a67bea34c80e59ba00ddd09af01c0a1df46409a`.

Disposition: **not affected by this advisory's version range for that Cargo pin**. No upgrade is required for this specific range check. This does not prove installed binary/lock correspondence, cover other IDNA implementations in Go/JS/WebView/guest code, assess hostname-authority correctness or other advisories, or establish vulnerability absence. Bind candidate inventory/hash before using the assessment for installed acceptance. Reassess after pin/graph/advisory changes.
