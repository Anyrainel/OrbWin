# Phase 2 Go advisory applicability triage — 2026-10-25

Status: evidence review, with unresolved release-security gates. No dependency,
pin, binary, engine configuration or running workload was changed by this review.
This record does not establish that the product is free of vulnerabilities, and
does not approve shipping an affected dependency.

**Later evidence correction:** the appended OpenPGP/symbol-attribution review
establishes that rdd/Buildx function fields below came from the scanner's
zero-symbol module fallback, not extracted function presence. Preserve the
original reported counts as historical output, with that revised interpretation.

## Evidence boundary and interpretation

**Interpretation correction:** the original rdd/Buildx function counts below are
scanner-reported advisory fallback, not extracted function presence. Exact
`-mode extract` and the independent host-runtime table/command-closure evidence
in the final section supersede that initial attribution. Preserve original raw
findings, but do not use their synthetic function traces as reachability proof.

The input is
`artifacts/qualification/p2-go-audit-52b624b6-daaa-4874-b92b-b7fb1861804b/proof.json`
and its `report-2.jsonl`, `report-10.jsonl`, `report-11.jsonl`,
`report-13.jsonl`, `report-14.jsonl` raw reports. The reports contain consecutive
pretty-printed JSON objects, rather than one object per physical line. Only OSV
IDs referenced by actual `finding` messages are counted below; the scanner also
emits advisory records for dependencies without matched findings.

The recorded tool is govulncheck v1.8.0, SHA-256
`1e6981feb46fe2026fa4ffe83059c86ba6cf961150bd2948e1c6e87f44c76f87`,
in binary/symbol mode, using `https://vuln.go.dev`, database timestamp
`2026-10-21T00:27:29Z`. The scan began at `2026-10-25T23:01:05.729Z`.
Its JSON-mode exit status 0 is not an absence-of-findings result. Binary scans
identify symbol mentions, omit source call stacks, and can include unreachable
code. Module/package-only findings need separate interpretation. These are
documented [govulncheck limitations](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck).

| Scanned artifact | Exact SHA-256 | Recorded build | Unique IDs / IDs with a reported function |
| --- | --- | --- | --- |
| `artifacts/backend/rdd.exe` | `0896047decdfa7c851870e80b5edc51439f243b8baa522379b4933c60e15c8f6` | rdd development build, Go 1.27.3 | 13 / 11 |
| `artifacts/runtime-tools/docker-cli-plugins/docker-buildx.exe` | `fd2e18501910dd535ea3aa7f69010b3cfdfba0f47aa27674124554386dfa26f3` | Buildx v0.38.1, Go 1.26.10 | 12 / 11 |
| `artifacts/runtime-tools/linux/docker-buildx` | `e25d5cd675092bb5d1d03632fdc46cdf3fede7b514dc5f023a105e00d3ce5ad4` | Buildx v0.38.1, Go 1.26.10 | 12 / 11 |
| `artifacts/runtime-tools/docker-cli-plugins/docker-compose.exe` | `8ae10b9ac12cf3bc5b8dc748a19561a844551f16c07354cac7f5480f420eae80` | Compose v5.6.1-orbwin.1, Go 1.27.3 | 6 / 4 |
| `artifacts/runtime-tools/linux/docker-compose` | `483c6e7ebcd41f661f16fd8a564206f72951b31fd15dadf4056eab006796d4ad` | Compose v5.6.1-orbwin.1, Go 1.27.3 | 6 / 4 |

There are **19 unique advisory IDs** across these five artifacts. A wildcard
OpenPGP function string counts as a scanner-reported function in the totals; it
is not a demonstrated concrete call. Counts do not measure severity or exposure.
This record covers neither the other Go executables in the parent scan nor the
guest Docker/containerd/k3s binaries, Windows/WSL, JavaScript, Rust, images or
installed-package contents. In particular, a module in a client executable does
not identify the engine's running version.

Pinned-source context checked for this review:

- rdd: `.upstream/rancher-desktop`, upstream commit
  `af6511ad4e065e24e7258f575a0fc090c10cdbda`, with the existing local adaptation;
  `rdd/go.mod`, `pkg/service/cmd/service.go`, `pkg/service/cmd/options/options.go`,
  `pkg/service/datastore/{doc,options,config}.go`, and Windows host-switch code.
- Compose: `.upstream/compose`, upstream commit
  `6cbc19a3fd59e305fb814dca4d854a4fb67d2941`, tag v5.6.1, existing
  `patches/compose/0001-explicit-local-dockerfile-read.patch`;
  `artifacts/compose/compose-manifest.json`, `go.mod`, `cmd/main.go`, and
  `pkg/compose/build_bake.go`. The local patch is a Dockerfile read-grant
  adaptation, not a fix for the advisories in this record.
- Buildx v0.38.1's primary [main entrypoint](https://github.com/docker/buildx/blob/v0.38.1/cmd/buildx/main.go)
  and [Docker driver](https://github.com/docker/buildx/blob/v0.38.1/driver/docker/driver.go)
  register CLI commands and connect to an engine/BuildKit through client APIs,
  including `/grpc` and `/session`; they do not establish an ordinary dockerd
  daemon entrypoint. Its [module graph](https://github.com/docker/buildx/blob/v0.38.1/go.mod)
  nevertheless includes the old Docker module and BuildKit v0.34.0. This is
  relevant context, not proof that every daemon symbol is unreachable.
- Relevant cached dependency source was read without executing it: Docker
  v28.5.2, BuildKit v0.34.0 and Kine v1.14.2. Cached source is evidence about
  those versions; it is not a source-call analysis of the exact release binary.

## Entry points and trust boundaries that remain material

**rdd is a server even with workload Kubernetes disabled.** Its management API
constructs the native Kubernetes control plane, CRD server and aggregator in
`pkg/service/cmd/service.go:createServerChain`. Options bind secure serving to
127.0.0.1; the Windows host switch also provides a private virtual-network API
path. Local/guest access boundaries, forwarding and client identity matter.
“No UI button” or “k3s off” cannot dismiss management HTTP/2/CEL findings.

**The default management datastore is Kine/SQLite.** Its Windows listener uses a
loopback TCP address; the upstream source calls `endpoint.Listen`, which creates
Kine's gRPC server and registers its own backend services. Kine imports etcd's
`embed` package for defaults, explaining a plausible route for additional etcd
code to be linked. That does not demonstrate dispatch to `etcdserver.Txn`, etcd's
watch authorization implementation, or etcd's custom TLS listener. The default
rdd datastore options do not populate TLS certificate/key files. Complete
call-path/configuration evidence is still required before any VEX-style
not-affected decision, especially for non-default upstream flags.

**Buildx and Compose have network and filesystem inputs.** Supported builds,
registry authentication, remote build sessions, contexts, export destinations
and copied archives can cross trust boundaries. Compose's ordinary plugin
entrypoint invokes its service backend; its Bake path resolves the default
Buildx plugin through Docker's plugin manager and starts that executable.
Consequently, Compose can exercise a vulnerable separate Buildx binary even
when its own embedded module graph lacks a corresponding finding. Conversely,
HTTP/2 frame parsing in a client is not sufficient evidence of the specific
`x/net/http2` server connection behavior named by an advisory.

## Shared findings: rdd, Buildx Windows/Linux, Compose Windows/Linux

For all five HTTP/2 IDs, the exact external module is `golang.org/x/net`:
rdd/Buildx **v0.59.0**, Compose **v0.58.0**. The Go database affected interval is
**[0, v0.60.0)**, fixed in **v0.60.0**. Their stdlib affected intervals are
`<go1.26.9` and `[go1.27.0-0, go1.27.2)`; the scanned toolchains are beyond those
stdlib fixes, which does **not** fix the separate `x/net` dependency.

| Advisory and primary evidence | Actual scan evidence and applicability | Narrow candidate and qualification |
| --- | --- | --- |
| **GO-2026-5932** — unsafe, unmaintained OpenPGP implementation. `golang.org/x/crypto` rdd/Buildx **v0.57.0**, Compose **v0.56.0**. Go DB range introduced 0, **no fixed release** for `openpgp` and its subpackages. [Go advisory](https://pkg.go.dev/vuln/GO-2026-5932), [Go maintainers' deprecation decision](https://go.dev/issue/44226). | rdd and both Buildx binaries report wildcard-like function names across seven OpenPGP packages; these are not precise call paths. Compose has module-only evidence. BuildKit v0.34.0's inspected signing helper uses `github.com/ProtonMail/go-crypto/openpgp`, a different module; that does not explain away other users of the deprecated package. Actual consumers and exposed signing/verification operations remain unresolved. | An `x/crypto` version bump alone cannot clear an all-versions package deprecation. Identify import owners and prefer their supported migration/removal of the legacy package. If active, qualify signature acceptance/rejection, revoked/expired keys and policy behavior without weakening verification. A replacement module is an API/security change requiring review, not a blind `replace`. |
| **GO-2026-6603 / CVE-2026-78659** — HTTP/2 trailer-map memory amplification beyond header limits. [Go advisory](https://pkg.go.dev/vuln/GO-2026-6603). | Functions are reported in all five binaries. The advisory's relevant behavior is server-side. rdd's management HTTPS server is a credible server entrypoint; confirm which implementation negotiates HTTP/2. Buildx has sessions; inspect their server handlers. Compose's reported functions are framing helpers, without a reported `ConfigureServer`/server connection path in this scan. No hostile-traffic execution has established exposure. | Prefer supported upstream dependencies containing `x/net >=v0.60.0`, or a narrowly recorded dependency patch if upstream cannot deliver promptly. Q-NET below: management API, authenticated build sessions, bounded upstream regression tests and cancellation. |
| **GO-2026-6610 / CVE-2026-78660** — HTTP/2 client accepts malformed framing headers; response smuggling requires forwarding to a permissive HTTP/1 downstream. [Go advisory](https://pkg.go.dev/vuln/GO-2026-6610). | rdd/Buildx report `GetClientConn`, `NewClientConn`, `RoundTrip`/`RoundTripOpt`; Compose is package-only. A network client alone does not establish the downstream reverse-proxy chain. Audit rdd's API/port proxy paths and Buildx registry/session HTTP forwarding for that conversion and for the actual transport used. Reachability remains conditional. | Same `x/net` floor. Q-NET must include request/response framing and proxy compatibility where that conversion exists; do not remove HTTP/2 or alter global proxy settings as an undocumented workaround. |
| **GO-2026-6611 / CVE-2026-78669** — repeated SETTINGS_INITIAL_WINDOW_SIZE can consume excessive CPU in HTTP/2 clients and servers. [Go advisory](https://pkg.go.dev/vuln/GO-2026-6611). | All five have reported functions, including Compose frame helpers. The attacker must be an HTTP/2 peer on a vulnerable transport. rdd server/client traffic and Buildx's BuildKit/session peers make this an active review priority. Compose's exact gRPC/framer path still needs source tracing; helper presence is not proof that it applies vulnerable window updates. | Same `x/net` floor. Q-NET, including bounded hostile-peer/upstream regression tests and prompt cancellation; no production-engine CPU-exhaustion experiment. |
| **GO-2026-6612 / CVE-2026-78663** — HTTP/2 server refunds flow-control credit twice after a reset and buffered-body read, permitting memory amplification. [Go advisory](https://pkg.go.dev/vuln/GO-2026-6612). | All five have reported functions. Requires the affected server body/reset accounting, not just frame serialization. Review rdd's server and Buildx session-serving implementations; Compose's server path is unproven. Server access controls reduce who can reach a listener but do not repair its accounting. | Same `x/net` floor. Q-NET, preserving stream reset/close, body handling and memory bounds. |
| **GO-2026-6617 / CVE-2026-97032** — concurrent HTTP/2 responses and SETTINGS_HEADER_TABLE_SIZE race in server HPACK state can crash the server. [Go advisory](https://pkg.go.dev/vuln/GO-2026-6617). | Functions are reported in all five. The vulnerable server/encoder state and concurrent peer activity must both be established. rdd's long-lived server deserves priority over an assumption that a CLI is harmless. Compose's framing-only matches leave implementation reachability unresolved. | Same `x/net` floor. Q-NET plus targeted upstream race/regression coverage on supported builds; lifecycle recovery must preserve existing data and independent engines. |

`x/net v0.60.0` declares Go 1.26.0, so the inspected module floor does not itself
require a higher toolchain than these scanned builds. It also still requires
`x/crypto v0.57.0`; it therefore does not resolve GO-2026-5932. Verify the whole
resolved graph and any newer advisories rather than treating this floor as a
recommended immutable final pin. [Primary v0.60.0 module file](https://github.com/golang/net/blob/v0.60.0/go.mod).

## rdd-only findings

| Advisory, exact module and affected range | Entry point / unresolved preconditions | Narrow candidate and qualification |
| --- | --- | --- |
| **GO-2026-6094 / GHSA-gcjh-h69q-9w9g**. `github.com/google/cel-go` **v0.29.2**, affected **[v0.22.0, v0.30.0)**, fixed **v0.30.0**. Native CEL JSON tag handling exposes fields marked `json:"-"` and recursively registers nested types. [Go advisory](https://pkg.go.dev/vuln/GO-2026-6094), [maintainer advisory](https://github.com/cel-expr/cel-go/security/advisories/GHSA-gcjh-h69q-9w9g). | `ext.NativeTypes` and `ext.ParseStructTag` are reported. rdd registers Kubernetes admission/CEL functionality, but this review has not found an actual registration passing confidential native Go structs through `NativeTypes(ParseStructTag("json"))` to untrusted expressions. Arbitrary CEL availability alone does not establish that specific type-provider path. Needs source-call/type audit; remains unresolved. | Prefer the compatible upstream Kubernetes/rdd dependency update to cel-go >=v0.30.0, or a focused upstream fix if needed. Q-RDD: admission/CRD expressions, native type behavior, JSON-private fields and compatibility. Do not expose extra native types merely to reproduce it. |
| **GO-2026-6237**. `github.com/insomniacslk/dhcp` **v0.0.0-20240710054256-ddd8a41251c9**, affected **<v0.0.0-20260719225207-c76316d4aa82**, fixed at that pseudo-version. Short IPv4 UDP payload can panic the raw DHCPv4 reader. [Go advisory](https://pkg.go.dev/vuln/GO-2026-6237), [upstream fix](https://github.com/insomniacslk/dhcp/commit/c76316d4aa825673b208730779843d0711021e81). | `dhcpv4/nclient4.BroadcastRawUDPConn.ReadFrom` is reported. rdd includes gvisor/Lima networking dependencies; its Windows host switch creates the virtual network. A DHCP server's presence does not establish use of this raw **client** reader. Identify which selected Windows/guest path creates it and who can send packets to it. The report does not establish a Windows raw-reader call path. | Prefer the dependency owner's supported update or narrow upstream bounds-check backport. Q-NET: upstream malformed-packet unit regression plus normal DHCP/address assignment and WSL networking; no malformed broadcast traffic on user networks. |
| **GO-2026-6107 / CVE-2026-73500 / GHSA-6vch-q96h-7gc3**. `go.etcd.io/etcd/client/pkg/v3` **v3.6.8**. Affected **<v3.5.33**, **[v3.6.0,v3.6.14)**, **[v3.7.0-alpha.0,v3.7.1)**. TLS listener creates unbounded handshake goroutines without a deadline. [Go advisory](https://pkg.go.dev/vuln/GO-2026-6107), [etcd advisory](https://github.com/etcd-io/etcd/security/advisories/GHSA-6vch-q96h-7gc3). | Reported `transport.NewListener`, `NewListenerWithOpts`, `NewTimeoutListener`, `NewTLSListener`. Default rdd Kine listener uses `net.ListenConfig`; Kine's optional TLS is gRPC credentials, not demonstrated use of this etcd listener. Non-default configurations and transitive server paths are not fully audited. No blanket not-affected conclusion. | Coherent etcd module update within 3.6 to **>=3.6.14**, if supported by pinned upstream, or that branch's exact listener fix. Q-RDD: Kine persistence/startup and transport shutdown; if an affected TLS listener is reachable, bounded incomplete-handshake regression and goroutine lifetime checks. |
| **GO-2026-5736 / CVE-2026-44283 / GHSA-x35m-3gp4-4fh5**. `go.etcd.io/etcd/server/v3` **v3.6.8**. Go DB affected **[v3.5.0,v3.5.30)** and **[v3.6.0,v3.6.11)**; same-branch fix **v3.6.11**. Nested transactions can bypass RBAC through PrevKv/lease attachment. [Go advisory](https://pkg.go.dev/vuln/GO-2026-5736), [etcd advisory](https://github.com/etcd-io/etcd/security/advisories/GHSA-x35m-3gp4-4fh5). | `etcdserver.EtcdServer.Txn` is reported. Requires dispatch to etcd's server authorization path and a relevant authenticated permission boundary. Default management storage dispatches Kine services; rdd's Kubernetes RBAC is a separate layer and does not itself prove etcd RBAC is in use. Remaining configurations/call paths unknown. | A supported coherent **3.6.14** update would cover all three recorded etcd IDs; changing only client modules would not fix server code. Q-RDD plus nested-transaction RBAC tests if such a server is reachable. Upstream also discusses a custom 3.4 fix; it is not this binary's version branch. |
| **GO-2026-6114 / CVE-2026-73499 / GHSA-xg4h-6gfc-h4m8**. `go.etcd.io/etcd/server/v3` **v3.6.8**, affected **<v3.5.33**, **[v3.6.0,v3.6.14)**, **[v3.7.0-alpha.0,v3.7.1)**. Open-ended watch requests can bypass single-key read authorization. [Go advisory](https://pkg.go.dev/vuln/GO-2026-6114), [etcd advisory](https://github.com/etcd-io/etcd/security/advisories/GHSA-xg4h-6gfc-h4m8). | `v3rpc.Server`, `NewWatchServer`, `watchServer.Watch` are reported. A restricted etcd user, affected range normalization and actual etcd watch service are needed. Kine's separate watch implementation is not automatically the same vulnerable handler. Full source-call/configuration verification remains open. | Same coherent **3.6.14** candidate; Q-RDD plus permission-bounded open-ended watch regression when relevant, including watch cancellation and shutdown. |
| **GO-2025-3521 / CVE-2025-1767 / GHSA-3wgm-2gw2-vh5m**. `k8s.io/kubernetes` **v1.36.4**, module-only. Go DB introduced 0, **no fixed version**. In-tree gitRepo volumes permit access to other local repositories. [Go advisory](https://pkg.go.dev/vuln/GO-2025-3521), [Kubernetes security announcement](https://groups.google.com/g/kubernetes-security-announce/c/19irihsKg7s). | Upstream identifies **kubelet** and clusters using gitRepo volumes. The scanned executable is the management server, not proof about a running guest kubelet; no vulnerable function/package is reported here. Workload k3s is separately supported and must receive its own version/configuration inventory. Neither workload-off state nor this module-only result closes that boundary. | Upstream says deprecated gitRepo will not receive a fix; there is no honest version-only cure in this record. Review supported volume-policy restrictions and document/init-container migration as applicable. Q-K8S below: selected guest kubelet feature/admission behavior. Do not silently disable Kubernetes or install a new policy in this triage. |
| **GO-2025-3547 / CVE-2024-7598 / GHSA-r56h-j38w-hrqq**. `k8s.io/kubernetes` **v1.36.4**, module-only. Go DB affected **>=v1.3.0**, **no fixed version**. Namespace deletion can remove NetworkPolicies before protected pods, creating a temporary network-policy bypass. [Go advisory](https://pkg.go.dev/vuln/GO-2025-3547), [Kubernetes security announcement](https://groups.google.com/g/kubernetes-security-announce/c/67D7UFqiPRc). | rdd has a real management API, but the workload/pod/NetworkPolicy enforcement/controller combination is not established by this module-only match. Optional guest Kubernetes remains relevant. Need inspect its CNI/policy support and namespace-deletion semantics rather than infer exposure from dependency metadata or infer immunity from a hidden UI. | No fixed version is listed. Upstream mitigations sequence workload deletion before namespace removal or retain policies until pods terminate; assess operational/product fit explicitly. Q-K8S: workload policy enforcement during bounded namespace teardown. A custom finalizer/controller would be a separate design, not a justified automatic patch here. |

## Buildx-only findings, Windows and Linux

The five Docker daemon IDs below all match the embedded
`github.com/docker/docker` **v28.5.2+incompatible** module. The Go DB records
all versions of that old module path (and old `github.com/moby/moby`) with no
fixed version there. The fixed v2 module ranges below are **different module
paths**, not version strings that can be pasted into the old dependency.

An attribution issue remains open: the scan reports only
`daemon.openContainerFS`, `daemon.containerExtractToDir`, and/or
`daemon.createIfNotExists` for **both** plugin/AuthZ IDs as well as archive IDs.
The cached v28.5.2 implementation of `openContainerFS` is in
`daemon/containerfs_linux.go`, yet the Windows report names it too. This review
has not independently verified those names in the exact executable or explained
the DB/scanner attribution. Keep the findings and investigate build tags,
advisory package-symbol metadata and binary extraction with an appropriate
source scan; do not call this a proven false positive or a proven Windows
daemon exploit. The raw report labels GO-2026-4883, GO-2026-4887 and
GO-2026-6253 **UNREVIEWED** in `database_specific.review_status`.

| Advisory and exact fix boundary | Shipped entrypoint / preconditions | Narrow candidate and qualification |
| --- | --- | --- |
| **GO-2026-4883 / CVE-2026-33997 / GHSA-pxq6-2prw-chj9**. Old Docker module as above; new `github.com/moby/moby/v2` **<v2.0.0-beta.8**, fixed **beta.8**. Engine product fix **29.3.1**. Plugin privilege comparison can accept a different privilege set than the user approved. [Go advisory](https://pkg.go.dev/vuln/GO-2026-4883), [Moby advisory](https://github.com/moby/moby/security/advisories/GHSA-pxq6-2prw-chj9). | Requires daemon-managed plugin installation/approval. Docker **CLI plugins** Buildx/Compose are distinct from engine-managed plugins. The scan's archive-symbol attribution does not demonstrate the daemon plugin manager in this process. Actual engine version/plugin use must be audited separately, including direct CLI access beyond UI flows. | Prefer a supported Buildx/BuildKit graph without affected old daemon code; establish whether elimination is possible before modifying imports. A v2 path migration or cherry-picked upstream fix requires build/reachability review. Q-BUILD plus Q-ENGINE if the actual guest daemon is affected. |
| **GO-2026-4887 / CVE-2026-34040 / GHSA-x744-4wpc-v9h2**. New v2 module **<beta.8**, fixed **beta.8**; Engine product fix **29.3.1**. Oversized API bodies may be omitted from an AuthZ plugin decision. [Go advisory](https://pkg.go.dev/vuln/GO-2026-4887), [Moby advisory](https://github.com/moby/moby/security/advisories/GHSA-x744-4wpc-v9h2). | Requires a daemon using body-inspecting AuthZ plugins. Buildx's client entrypoint does not prove it hosts that middleware; archive-function matches need investigation. No evidence here establishes the owned guest daemon's plugin configuration. Absence of an OrbWin AuthZ UI cannot settle direct client/server exposure. | Same supported graph/daemon review as previous row; no disabling user authorization plugins as a workaround. Q-BUILD and, where applicable, bounded AuthZ allow/deny regression against the selected engine. |
| **GO-2026-5617 / CVE-2026-42306 / GHSA-rg2x-37c3-w2rh**. New v2 module **<v2.0.0-beta.14**, fixed **beta.14**; Engine product fix **29.5.1**. Copy-in mount race can redirect a bind mount to a host path. [Go advisory](https://pkg.go.dev/vuln/GO-2026-5617), [Moby advisory](https://github.com/moby/moby/security/advisories/GHSA-rg2x-37c3-w2rh). | `openContainerFS` is reported. Upstream describes Linux daemon mount setup during copy into a container, not ordinary external API library use. Establish how any linked code is reachable in Buildx; independently inventory guest dockerd. OrbWin Files/CLI copy flows contact that daemon and remain relevant even if the client finding is later narrowed. | Prefer supported upstream graph elimination/migration; independently require a fixed supported engine if affected. Q-BUILD/Q-ENGINE, preserving confinement and existing file-copy behavior. No race experiment against user files or a shared engine. |
| **GO-2026-5668 / CVE-2026-41568 / GHSA-vp62-88p7-qqf5**. New v2 module **<beta.14**, fixed **beta.14**; Engine product fix **29.5.1**. Mountpoint symlink race permits arbitrary empty host files/directories. [Go advisory](https://pkg.go.dev/vuln/GO-2026-5668), [Moby advisory](https://github.com/moby/moby/security/advisories/GHSA-vp62-88p7-qqf5). | `openContainerFS` and `createIfNotExists` are reported. Same Linux daemon/copy-in boundary and platform attribution uncertainty as previous row; this is the mountpoint-creation race, not the later bind-mount race. A successful normal copy test cannot prove the race is repaired. | Same supported graph and engine candidate boundaries. Q-ENGINE requires upstream race/confinement regression in disposable controlled conditions, plus normal archive/copy compatibility. |
| **GO-2026-5746 / CVE-2026-41567 / GHSA-x86f-5xw2-fm2r**. New v2 module **<beta.14**, fixed **beta.14**; Engine product fix **29.5.1**. Compressed archive upload can resolve a decompressor from the container filesystem and execute it with daemon privileges. [Go advisory](https://pkg.go.dev/vuln/GO-2026-5746), [Moby advisory](https://github.com/moby/moby/security/advisories/GHSA-x86f-5xw2-fm2r). | `containerExtractToDir` is reported. The advisory concerns the daemon's `PUT /containers/{id}/archive` implementation on Unix-like systems. Buildx entrypoint and actual guest daemon must be evaluated separately. Container-controlled content is a real boundary; ordinary successful builds or use of uncompressed test archives do not establish safety. | Supported graph elimination/migration plus fixed guest daemon when affected. Q-ENGINE must check decompressor resolution ordering and compressed archive behavior using upstream regression tests; no malicious host-code execution proof on the user's machine. |
| **GO-2026-6253 / CVE-2026-17106 / GHSA-hfg8-hc9c-6c3h**. `github.com/moby/go-archive` **v0.2.0**. Go DB **<v0.3.0**, fixed **v0.3.0**. Maintainer page says affected **<v0.2.2** but patched **v0.3.0**: record the discrepancy; both include this pinned v0.2.0. [Go advisory](https://pkg.go.dev/vuln/GO-2026-6253), [maintainer advisory](https://github.com/moby/go-archive/security/advisories/GHSA-hfg8-hc9c-6c3h). | **Module-only**, no matched package/function. Archive extraction routines can follow archive-created links outside destination. The inspected BuildKit source imports compression in frontend helpers and go-archive/chrootarchive in solver unpacking; imports do not prove those extractor functions run in the Buildx client. Need trace actual local context/output extraction and distinguish remote BuildKit execution. | Prefer supported Buildx/BuildKit dependency upgrade to **>=v0.3.0**, or upstream's exact confinement patch if required. Q-BUILD: trusted/malicious-link archive regression in an isolated directory with canary preservation, context/export/archive compatibility, cancellation cleanup. No global cache prune. |

## Proposed remediation order and qualification gates

These are review proposals, not approved changes or a promise that a particular
upstream release already contains every fix. Preserve the selected upstream
backend and narrow adaptation; do not replace it or disable workload Kubernetes
to avoid findings.

1. **Prioritize the shared `x/net` floor** through supported upstream updates or
   a narrowly versioned patch, because the selected product intentionally has
   server and build-session transports. Run source-mode analysis for exact
   Windows/Linux build packages and tags to separate server/client/framer paths.
   A rebuilt stdlib-only toolchain is insufficient.
2. **Audit and resolve specific rdd paths**: CEL native type registrations;
   DHCP raw client use; etcd code linked through Kine; management API listener,
   auth and datastore configuration. Consider coherent same-branch etcd >=3.6.14
   and cel-go >=0.30.0 only after compatibility review. Do not silently remove
   features or suppress findings in lieu of this work.
3. **Resolve Buildx daemon-symbol attribution and archive consumers**. Prefer an
   upstream-supported Buildx/BuildKit graph that removes obsolete daemon code or
   includes the fixes; do not blindly replace an old Docker module with a v2
   module. Separately inventory the actual guest engine and BuildKit versions.
4. **Track no-fixed-version boundaries** explicitly: deprecated OpenPGP import
   migration; gitRepo workload policy/feature exposure; namespace/NetworkPolicy
   teardown ordering. Each needs a source/configuration-backed disposition,
   named owner and release decision in the security process. No deadline waiver
   or risk acceptance is granted here.

Qualification groups referenced above:

- **Q-NET:** preserved upstream HTTP/2 tests and focused fix regressions;
  management API authentication/streaming/watches; WSL relay/private identity;
  Docker/BuildKit session upload/download, registry pull/build, stream reset,
  cancellation and process shutdown. Run race/resource tests with bounded peers
  in isolated fixtures. Check host/guest exposure and all three engines' state
  before/after. Do not use host exhaustion as a test mechanism.
- **Q-RDD:** coherent Go graph/toolchain build and source scan; upstream tests;
  Kine/SQLite database preservation, CRD/admission/control-plane readiness and
  watch behavior; startup/cancel/reconnect and upgrade recovery. A dependency
  change must not migrate user state without explicit compatibility evidence.
- **Q-BUILD:** Windows/Linux plugin discovery and hashes; Dockerfile read-grant
  regression (positive exact file, negative sibling); plain/Compose/DevContainers
  builds, synthetic private-auth pull/build and negative-after-Forget; export,
  archive/context and cancellation cleanup; honest BuildKit cache accounting.
  Existing receipts qualify their exact earlier binaries, not a replacement.
- **Q-ENGINE:** identify the selected guest daemon's immutable version and
  upstream patch provenance independently of client modules. Any needed engine
  update uses upstream lifecycle/storage arrangements and requires data and
  coexistence/compatibility qualification. Review upstream security regressions
  in disposable infrastructure rather than exploit the user's running daemon.
- **Q-K8S:** inventory the actual optional guest k3s/kubelet/CNI versions and
  feature/admission policies; validate supported volume and namespace/policy
  behavior. Keep the management API and workload cluster distinctions explicit.

For any accepted change: preserve locks/source/license provenance, regenerate
binary hashes/SBOMs, rescan the actual Windows and Linux artifacts with a current
recorded DB/tool, review remaining module/package-only and symbol findings, then
rebuild/requalify the installed candidate. The parent scan remains immutable
historical evidence. No successful normal journey in Phase 2 constitutes a
security regression test or resolves these open advisory dispositions.

## Follow-up: exact OpenPGP package absence and scanner attribution

Additional read-only evidence is in
`artifacts/qualification/p2-openpgp-attribution-20261025/`:
`symbol-availability-proof.json`, the eight raw extraction records,
`closure-proof.json` and its command graphs, `pcln-proof.json`, the bounded
inspector source `pcln.go`, and pinned Buildx source download metadata.
No inspected executable, engine, credential provider or workload was started.
The helper executes Go's standard-library binary parsers against file bytes.
No active source, dependency, pin or package was changed by this review.
`closure-proof.json` SHA-256 is
`4de4fb4608adf5f4f8c6a754cd261c63b0242e36d0dee5158581693389461c97`;
it records each graph's own hash and source go.mod/go.sum hashes.
`evidence-hashes.json` binds the helper source and the symbol/PCLN proof files.

### The earlier function-count interpretation is corrected

govulncheck v1.8.0 `-mode extract` returns **zero `pkgSymbols`** for the exact
rdd and both Buildx artifacts in the original scan. Its
`internal/vulncheck/binary.go:106–113` then calls
`allKnownVulnerableSymbols`; lines178–206 populate package/function fields from
affected advisory metadata, including wildcard placeholders. These are
**module-level fallback findings with synthetic function fields**, not evidence
that the binary contains the named functions. The earlier 11 function-bearing
IDs per rdd/Buildx are counts of reported fields only. The metadata extractor
documents its stripped-symbol fallback in
`internal/buildinfo/additions_scan.go:49–90`.
[Pinned scanner fallback source](https://github.com/golang/vuln/blob/v1.8.0/internal/vulncheck/binary.go),
[pinned extraction source](https://github.com/golang/vuln/blob/v1.8.0/internal/buildinfo/additions_scan.go).

This explains the apparently Linux-only `daemon.openContainerFS` on Windows:
the scanner filled that name from advisory records for an embedded old Docker
module; it did not extract a Linux daemon function from the Windows executable.
The fallback also explains the OpenPGP wildcard strings and broad repeated
HTTP/2 symbol lists. Retain the affected-version findings, but do not use those
synthetic names as demonstrated presence or reachability. Other advisory
configuration/call-path reviews above remain necessary.

| Exact artifact identity | govulncheck extracted symbols | Scanner module fallback |
| --- | --- | --- |
| rdd `0896047decdfa7c851870e80b5edc51439f243b8baa522379b4933c60e15c8f6` | 0 | yes |
| Buildx Windows `fd2e18501910dd535ea3aa7f69010b3cfdfba0f47aa27674124554386dfa26f3` | 0 | yes |
| Buildx Linux `e25d5cd675092bb5d1d03632fdc46cdf3fede7b514dc5f023a105e00d3ce5ad4` | 0 | yes |
| Compose security candidate Windows `c775a6c7374c8b2adab3af70f8044fa3ca74cf5a74d93e3aeb33bdc051fe270e` | 35,349 | no |
| Compose security candidate Linux `8d2f91ec120fc95e4adfe139129f423182a191fd1a462718d7f5fc28e679ad8c` | 35,221 | no |
| Current SDK `8f118fb18b233c70a6cf7c9d2284de7c3b02fca90f6be09395cdd9943cfdb8d5` | 31,765 | no |
| Current CLI `7ccb4f47ac34eca54c8b34aad42e2a9e541523be651f17d875d39de2ab17504f` | 4,858 | no |
| Credential helper `71cfd334e450ce9f62fd0026fce74d8942217e9ca341a0040e2d88cd91afed07` | 4,309 | no |

The current SDK extraction is **not** evidence about the overwritten earlier
SDK binary. The last three rows are extraction/precision metadata only, not
new advisory scans or a new applicability review of those executables.
The proof records tool hash
`1e6981feb46fe2026fa4ffe83059c86ba6cf961150bd2948e1c6e87f44c76f87`,
raw extraction hashes, unchanged input hashes and fallback-source hash/ranges.

### Production command dependency closure

Pinned Go1.27.3 ran read-only `go list -mod=readonly -deps` with
`GOWORK=off`, `CGO_ENABLED=0`, amd64 and explicit Windows/Linux GOOS. Graphs retain
production `Imports` separately from declared `TestImports`/`XTestImports`.
No tests or all-module dependency edge was silently treated as a production
import. go.mod/go.sum hashes remained unchanged around every query.

| Command source/configuration | Production package count | Legacy `golang.org/x/crypto/openpgp` packages |
| --- | --- | --- |
| selected rdd `./cmd/rdd`, Windows | 3,036 | 0 |
| Buildx v0.38.1 `./cmd/buildx`, Windows | 1,430 | 0 |
| Buildx v0.38.1 `./cmd/buildx`, Linux | 1,423 | 0 |
| isolated Compose security candidate `./cmd`, Windows | 829 | 0 |
| isolated Compose security candidate `./cmd`, Linux | 820 | 0 |

Buildx source was obtained at exact Go module v0.38.1, checksum
`h1:7J7nReQndOnDTUFjun6otmQqehoi9sCOvRwFjkngYlc=`, origin commit
`0dbe87f36ed472fe36b6796c3f70b4210a731fc5`; no unpinned source fallback.
The reconstruction uses Go1.27.3, while the selected upstream Buildx binaries
were built with Go1.26.10; this is an explicit reconstruction limit rather than
an asserted bit-identical rebuild. Direct inspection of the exact binary runtime
name tables below supplies independent evidence.

`x/crypto` is legitimately in these module graphs for other packages: rdd uses
SSH/knownhosts through networking/Lima-related dependencies and other crypto
consumers; Compose imports BuildKit's SSH utilities and secure-system libraries;
Buildx also includes SSH/agent and other cryptographic primitives. A module
version match does not mean every package in that module is compiled in.

Buildx **does** compile OpenPGP functionality, but from the different
`github.com/ProtonMail/go-crypto` module. Concrete production owners are
`github.com/docker/buildx/policy` and
`github.com/moby/buildkit/util/{pgpsign,gitutil/gitsign}`. Policy code parses and
verifies signed Git/HTTP artifacts through those helpers; the graph also retains
their separately declared tests. This is not legacy `x/crypto/openpgp`, and this
review does not assert ProtonMail's implementation is vulnerability-free.
The rdd and Compose command graphs contain neither OpenPGP variant.
[Buildx v0.38.1 policy verification source](https://github.com/docker/buildx/blob/v0.38.1/policy/funcs.go),
[BuildKit v0.34.0 signing imports](https://github.com/moby/buildkit/blob/v0.34.0/util/pgpsign/pgpsign.go).

### Independent exact-binary runtime-name evidence

The standard-library `debug/pe`, `debug/elf` and `debug/gosym` inspector reads the
Go1.20+ PCLN runtime function/name tables. It requires one validated table with
`runtime.main`, plausible function bounds/counts and, for PE, an actual host
moduledata-shaped pointer pair to that PC header and its function-name slice.
This matters because rdd embeds guest binaries containing other valid Go tables;
a byte-string search or first magic-number match would select an unrelated
table. ELF inspection uses its explicit `.gopclntab` section. It parses the
function-name slice, including stored inline names, but does not reconstruct
call graphs or qualify code executed by external processes/plugins.
[Go runtime table layout](https://github.com/golang/go/blob/go1.27.3/src/runtime/symtab.go),
[Go standard table parser](https://github.com/golang/go/blob/go1.27.3/src/debug/gosym/pclntab.go).

| Exact binary above | Parsed functions / function-name entries | Legacy OpenPGP names | Old `github.com/docker/docker/daemon` names |
| --- | --- | --- | --- |
| rdd Windows | 182,225 / 203,104 | 0 | 0 |
| Buildx Windows | 70,603 / 80,294 | 0 | 0 |
| Buildx Linux | 70,096 / 79,898 | 0 | 0 |
| Compose candidate Windows | 32,820 / 38,700 | 0 | 0 |
| Compose candidate Linux | 32,542 / 38,516 | 0 | 0 |

**Disposition supported by this evidence:** the reviewed production command
closures and exact runtime-name tables contain no deprecated
`golang.org/x/crypto/openpgp` implementation. For these exact five artifacts,
GO-2026-5932 is a module-metadata association with evidence of package absence,
not demonstrated vulnerable OpenPGP use. Root can review an artifact-specific
“vulnerable package not present” disposition without blindly replacing crypto
dependencies. The old Docker daemon package is likewise absent from these
command closures/name tables; that resolves the Buildx attribution anomaly,
but does **not** clear the separately running guest daemon's versions or copy
API behavior. Do not extrapolate either conclusion to other binaries, optional
guest programs, a different Buildx/Compose pin or future graph/build changes.

The isolated Compose floor candidate still needs adoption/package/runtime
qualification; its x/net correction and package-absence review do not silently
close the product-wide security process or the remaining advisories.
