# Rancher adaptation record

Base: repository and exact commit in `upstream.lock.json`. Upstream Apache-2.0
headers and tests are retained. Patches are applied in order; normalized source
hashes detect unexpected drift. Preparation never resets developer work.

| Patch | Reason | Status |
| --- | --- | --- |
| 0001 instance Docker pipe | Shared default pipe prevents simultaneous engines; endpoint discovery and bridge must agree | Focused Windows tests and race checks pass; clean replay verified |
| 0002 OrbWin WSL namespace | Shared vsock ports/signature, veth cleanup names and proxy address can collide | Paired source contracts, host tests and Linux cross-compilation pass; live guest test pending |
| 0003 owned WSL distro | Lima derives global registration from VM name, not LIMA_HOME | App-controller tests pass; runtime registration test pending |
| 0004 product environment guard | Dynamic help can autostart the default instance before user intent; global client paths must not leak through | Product identity validation before CLI construction, private client paths; tests/build verification required |
| 0005 Windows Docker paths | Native CLI/Compose bind sources reach Linux Moby without drive translation | Existing bridge uses the standard HTTP reverse proxy with narrow create-body adaptation, owner/System pipe ACL and bounded create reads. Unit/HTTP/stream/upgrade/half-close/shutdown and race checks pass; clean five-patch replay verified. Runtime qualification in progress. |
| 0006 sequential provisioning proof | Clean setup must be exercised without replacing the existing product guest/data | Reserves `orbwin-proof` guest only for the already-allowed proof instance. Normal identity remains `orbwin`. The same native setup path passes cancel/provision/resume with the product guest stopped; no simultaneous proof/product support is implied. |
| 0007 WSL shared socket | Selected user distributions need explicit private clients and shared sources | Non-root Ubuntu connect/read/write/Compose/build/restart/idle and source-replacement proofs pass; both public socket paths subsequently use patch 0008's authenticated adapter relay. |
| 0008 Windows port publication | Automatic ports could redirect to another engine; stale forwarding could be acknowledged after restart | Fixed Windows allocations, conflict preflight, observed publication generations and mTLS WSL relay. Four consecutive Testcontainers suites and maintained all-three-engine conflict/restart proof pass on binary `29073605…`. See architecture and completion records for preserved failure evidence and scope. |
| 0009 Kubernetes version selection | Installer assumed `+k3s1`; version changes lacked data-preserving transition guards | Exact embedded catalog release resolution and existing App admission restrictions; controller/catalog and transition tests pass. Native 1.34.6→1.35.3 apply, retained PVC/PV/sentinel and downgrade-admission rejection pass. |
| 0010 single provisioning owner | Lima's direct WSL boot raced the guest's systemd provisioning and retried without engine-namespace DNS | Stateful product scripts run only in the verified existing network namespace; the direct path waits for the existing unit's success/failure with a deadline. Guard behavior tests and owned upgrade/PVC-preservation proof pass on `e3892a1f…`. |
| 0013 typed service status | Generic API failures cannot authorize cold service startup; PID-only status can delete diagnostic evidence | Adopted source supplies a bounded read-only process-generation observation, duplicate-receipt refusal and stable file checks. CLI source rechecks supported running identity after one explicit cold service start. Clean13-patch replay and focused source tests pass; active backend binary remains0896047d..., so supported-generation/live cold lifecycle is not yet qualified. |
| 0014 host HTTP/2 floor | Five published HTTP/2 advisories affect the selected host x/net version | Host module selection advances from v0.59.0 to v0.60.0; guest modules are unchanged. Combined typed-status/dependency source tests and clean14 replay pass. Running binaries are unchanged; guest remediation, final binary attribution/scan and runtime qualification remain separate. |
| 0015 LAN publication observation | LAN leases need the exact running container incarnation and observed listener generation | Adds a bounded exact-ID v2 observation through the existing Docker adapter, with double inspection and publication checks. Memory-transport tests, race and vet pass; existing v1/Docker routes are covered. No runtime upgrade or LAN reachability qualification. |
| 0016 publication owner lifetime | Numeric listener counters can repeat after the serving bridge restarts | Adds a 128-bit public owner identity for each serving HTTP bridge lifetime; missing entropy refuses v2 observations. Eight focused tests plus 28 subcases, race, vet, clean16 replay and ordered Rust attestation pass. The SDK binds this identity into lease renewal; actual restart invalidation remains unqualified. |
| 0017 private engine proxy | Engine pulls/builds need an owned auto/none/URL policy and authenticated headless consumers | Source integrates private management/datastore authority, same-image service credential refresh, bounded Auto child, exact guest transaction/helper rollout and current controller/host acknowledgements. Independent review corrected pending restart convergence. Focused tests/race/vet, clean17 replay and ordered Rust attestation pass. No runtime replacement; legacy authority migration, actual-profile permission blockers, live proxy/PAC and installed qualification remain open. |

0002 currently assigns one product namespace, rather than supporting arbitrary
parallel instances of OrbWin: discovery 16670, network data 16657, Docker 16660,
containerd 16661; signature `dev.orbwin/wsl-network/v1`; links `veth-ow-wsl` and
`veth-ow-ns`; bridge 192.168.144.0/24. These values are development candidates.
VPN/route/other application conflicts remain a gate. 0003 uses `lima-orbwin`.
No engine changes or persistent data migrations are introduced.

Before changing the pin: stage a fresh source checkout, review each patch and
the updated host integration, update normalized hashes intentionally, rebuild all
host/guest components, run source contracts and upstream tests, then repeat
runtime/coexistence/workload/preservation qualification. A clean patch apply is
necessary but does not prove upstream compatibility. Do not reuse the old runtime
qualification record after upgrading the source or patches.

No patches have been submitted upstream. The long-term upstreamable direction
is namespace parameterization; the current product-specific patch is a bounded
development adaptation whose runtime suitability remains unverified.

## Patch0013 source adoption

The unregistered8e509... candidate remains preserved under
`artifacts/patch-replay/service-status-candidate`. The reviewed expanded candidate
and source adoption receipt are under `artifacts/patch-replay/cli-typed-status-integration`.
Patch0013 is now registered in the13-patch source lock. Its original CRLF context
applied to the active working tree but failed clean cached replay against the LF
Git index. The failed replay and original registered bytes are retained; only
patch serialization was normalized to LF. The normalized source hashes and active
Go source bytes did not change. Registered patch SHA:
`d7dea90dbab064fae8eed79fc736ad83fa2e0acb0d344048c86b825cfdae24ac`.

The corrected [clean13-patch receipt](../../artifacts/patch-replay-fabfa21c-ead9-4a11-bc6d-419422efbf8a/result.json)
passes from the pinned commit using the normal source preparation recipe. This is
accepted source evidence, not runtime adoption: backend `rdd.exe`, its manifest,
the existing service/home, and all engines were not replaced or restarted.
Missing/legacy/foreign or other-session generation evidence remains unknown and
cannot authorize cold startup. Diagnostic receipts/events do not own lifecycle;
existing rdd Start/Stop remains the sole authority. After an attended supported
backend upgrade, actual generation/start/exit/readiness and coexistence gates must
be qualified before claiming the cold CLI journey complete.

## Patch0014 source adoption

The reviewed source transaction is retained under
`artifacts/patch-replay/rdd-host-http2-typed13-adoption-4cc05416-cc92-4dd9-84dc-36d102d657ce/source-adoption-3faa7ca2-9680-4f62-9c91-d04fffcf2a67/`.
It backed up exact original bytes, explicitly normalized only host `rdd/go.mod`
and `rdd/go.sum` to LF, applied the exact registered Git patch and committed the
normalized lock last. The receipt is complete with no unknown pending write.
Patch SHA-256 is
`30f79ab6cc71b09f5c05e438f241b2172e488026ce0d514b5270f90a5819b67b`.
The [clean14 replay](../../artifacts/patch-replay-d2605252-8a23-419c-8dee-0afdc195ed71/result.json)
passes against the unchanged pinned upstream commit. Rust's ordered patch
attestation now includes this patch as well. The existing runtime binaries,
manifest, CLI, guest modules, private home and engines were not changed by this
source adoption. Earlier full12/net60 and full13/net59 builds remain historical
artifacts, not a final14 build or combined runtime qualification.

## Patch0018 source consolidation — 2026-11-06

`0018-private-runtime-authority.patch` consolidates the reviewed private runtime,
configuration and SDK ownership source milestones over the exact seventeen-patch
baseline. It adds 174 paths and modifies seventeen; no binary fixture, mode,
gitlink or deletion changes are included. Generated guest-agent gzip and three
local development records remain outside canonical source. The full lock now
records eighteen ordered patches and all 382 final changed-file LF hashes.

Root and independent review compared all 914 canonical files with the adopted
source, regenerated exact patch bytes from actual Git objects and checked the
complete isolated replay tree. Patch SHA256:
`9fe5c2cf3bd33dfe1401411ea3a21d404ca3a281154ce8ff35663ad5858ee514`.
The source-publication receipt is
`artifacts/qualification/p2-root-canonical-0018-adoption-20261106/adoption.json`.
Original source/lock evidence is preserved; the selected upstream working tree,
runtime binaries, engines and user data were unchanged. A coherent rebuild and
production/native/installed qualification remain required. Whole SDK retirement
continues to refuse missing support; patch registration does not enable it.

## Patch0019 source consolidation — 2026-11-07

`0019-runtime-custody-and-inspection-foundations.patch` records the reviewed
constructor/inspection/SDK and datastore ownership source convergence, including
the unrelated-name reaper correction and dormant candidate statement verifier.
It adds271 files and modifies46 over the exact0018 tree. The complete canonical
source has1185 files after the same four historical generated/local exclusions;
the lock records all654 final changed paths against the pinned upstream commit.

The isolated19-patch replay and independent full-source/hunk/object review match.
Patch SHA256: `a8ad56782c330b24a467c584fd3c52a2618d7095e8af755a18740d833afdb293`.
The [adoption record](../../artifacts/qualification/p2-root-canonical-0019-adoption-20261107-1/adoption.json)
retains exact before bytes. Registration changes the source lock, the canonical
build count and one ordered Rust attestation tuple; it preserves the separately
adopted candidate-staging facade. Ten affected Node contracts and the actual
release caller compilation pass. Historical component binaries were compile
inputs only. No selected upstream checkout, installed backend or engine changed.

This consolidation is not Phase2 acceptance. The later fresh-setup I/O/factory
settlement successor is a separate patch. Persistent launch/cache acquisition,
actual publication/reopen, modern handoff and installed-flow qualification remain
required; no missing native retirement or admission evidence is synthesized.


## Patch0020 setup source consolidation — 2026-11-07

`0020-fresh-setup-early-entry-and-io-custody.patch` registers the reviewed
early setup route and actual protocol/worker/factory settlement successor. It
adds12 and modifies3 paths over0019:1197 canonical files,666 complete patched
paths. The isolated20-patch replay and independent full closure match.
Patch SHA256: `8f4699e11d2e614199e0fedd1d77768e181f2acffec463f2c15f9f7c0a38cc88`.

The [adoption record](../../artifacts/qualification/p2-root-canonical-0020-adoption-20261107-1/adoption.json)
preserves before bytes. This is source registration only; existing installed
backend binaries and engines remain unchanged. Persistent launch security, cache
process verification, real guest-session authentication and native/installed
qualification remain acceptance gates. Native runs are currently user-paused.


## Patch0021 candidate read custody — 2026-11-07

`0021-modern-candidate-native-read-custody.patch` adds4 and modifies4 paths
over0020, preserving all15 setup paths byte for byte. The canonical source has
1201 files and670 final patched paths. Root isolated replay and independent full
hunk/tree/lock closure match: patch SHA256
`95b3af0bef53c3dffe155dc51fa033fe13ff2fe0d820cc4b084f9820abd6b739`.
[Root adoption](../../artifacts/qualification/p2-root-canonical-0021-adoption-20261107-1/adoption.json)
changes only source registration, canonical count and the ordered Rust tuple.
Ten affected Node contracts pass. Rust cache custody changes remain separate.

This is a dormant native image-read foundation. It grants no launch, writer,
retirement, checkpoint, cold recovery, publication or installed qualification.
No backend binary, installed engine or user workload changed. Native runs remain
user-paused; combined caller compilation and native acceptance are distinct.
