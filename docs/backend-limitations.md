# Backend limitations

Updated 2026-10-26 Pacific. Entries distinguish source findings from observed
runtime proofs. Source-only findings do not establish runtime behavior.
Initial findings below are retained for provenance. The dated implementation
updates and `verification/phase-1-completion.md` supersede historical statements
that a subsequently implemented flow is absent.

## 2026-10-24 implementation updates

- Current completion evidence supersedes older open-status prose below:
  installed first-run/cancel/resume, NSIS refusal/replacement/uninstall/reinstall,
  measured baseline, native logs/TTY/forwarding/storage/diagnostic export and
  explicit reconnect now pass their recorded cases. BL-015's guest dependency
  is bundled on the twelve-patch backend. Files uses bounded read-only Docker
  archives. Current full accounting is in `phase-1-coverage.md`; historical
  source-only entries are not claims that these adapters remain absent.
- New-App creation selects Kubernetes 1.35.3 while disabled: the upstream 1.34.6
  default exceeds bundled kubectl 1.36's supported skew. Existing clusters are
  never silently upgraded. Installed explicit upgrade retained its PVC marker.
- BL-006 / BL-016: the selected-source capability audit now distinguishes WSL
  resource scope, absent standalone proxy settings and missing whole-engine
  backup/relocation/migration transactions. Settings explain shared resources
  and direct-network qualification; these are not simulated configuration flows.
  See `verification/upstream-capability-audit.md`.
- BL-003 / BL-012: ordinary per-user installation and native first-run
  cancellation/resume created a fresh installed engine, distinct from the
  archived development fixture. Its three-engine coexistence proof passed.
  Installer replacement/uninstall retention and the complete baseline remain
  open; an intermittent Windows API failure during restart is under investigation.

- BL-015: native Kubernetes forwarding exposed a missing guest `socat` runtime
  dependency. The local listener opens but forwarding the first request fails.
  The UI retains the concrete error and removes the dead link. Signed openSUSE
  Leap 16.0 socat/libwrap RPMs passed signature and dependency checks and were
  installed only in the owned guest for a dependency proof. Patch 0011 now
  bundles the pinned binary/source RPMs and licenses; automatic existing-guest
  repair passed after removing the manual packages. Fresh provisioning and the
  complete native forwarding journey remain in progress.

- BL-003: pinned runtime clients are bundled; native first-run cancel/setup/resume
  passed on a fresh sequential proof guest. Installed-product checks remain open.
- BL-005: general Windows Docker CLI/Compose bind translation now passes C:/F:
  directory/file, spaces/Unicode, read-only, host-visible writes, local bind
  volumes and restart checks. UNC/device/relative paths fail explicitly.
- BL-010: real interactive pod exec passed against the isolated workload cluster.
  Followed logs and persistent forwards are implemented; native qualification is
  still required before closing this entry.
- BL-011: the WSL page now has explicit selected-project integration, scoped Linux
  client guidance and disconnect. Native Ubuntu read/write/Compose/build/restart/
  replaced-source/data-preservation proofs pass. Shared project paths require a
  live user WSL session and reconnect after distribution exit. Both public Unix
  sockets now use the authenticated Windows adapter relay; a shared bind of the
  dockerd socket did not survive user-distro exit.
  Transparent `/home` path mapping remains a documented later convenience.
- BL-012: actual first-run and resource/disk/history reads now exist. Broader native
  settings, apply/restart, conflict and installed qualification remain open.
- BL-013: Docker Desktop 4.91.0 subsequently started with Rancher returned to its
  initial stopped state and its remaining zero-length stale IPC directory retained
  under a sibling backup. Its dedicated daemon passed the OrbWin coexistence pair
  proof (`orbwin-coexist-fb674c82`). Rancher then also started and both dedicated
  endpoints remained healthy. Three-engine maintained traffic, identity/data
  retention, explicit foreign-port conflicts and owned-engine restart passed
  `orbwin-coexist-ad3ae9dc` on binary `29073605…`.
- BL-014: Windows allocation, stable bindings, explicit conflict refusal and
  publication status now pass scoped tests. Repeated Testcontainers found a second
  stale-publication acknowledgement race after restart. The adapter now requires
  a fresh observed listener generation after start/restart. Four consecutive
  Testcontainers suites, concurrent Windows/WSL/delegated publication and the
  three-engine proof passed on binary `29073605…`; see the active completion
  record for exact evidence IDs. This closes these tested cases. Failed evidence
  remains retained; broader compatibility and Phase 1 gates remain separate.

## BL-025 — Passive registry inspection identity check is unqualified

- Flow: the dormant Phase 2 native inspector independently reopens registry
  edges and currently requires `CompareObjectHandles` to return true.
- Observed 2026-11-06: a separate read-only probe opened the current user's
  `Software` key twice from one retained parent with KEY_READ / OPEN_LINK.
  Comparing one handle with itself returned true; comparing the independent
  opens returned false. All three opens and reverse closes returned zero.
  The measured parent observed original child exit zero; all 33 packet inputs
  matched afterward. [Root outcome](../artifacts/qualification/p2-root-registry-key-comparison-native-20261106-1/OUTCOME.json),
  SHA `6f7ea433ba76428e1fa869eecc633eead7cfd58d4bf3c0b0e95ace0c3d99eda9`.
- Impact: the current positive cross-open check cannot be activated on an
  assumption that it proves logical registry-key equality. This probe does not
  qualify the actual HKU/SID chain, establish universal behavior, or authorize
  treating false as identity proof. No registry mutation or product activation
  occurred. Existing setup remains separate from the dormant inspector.
- Exit: review a documented identity/edge contract and qualify the actual
  provider, including symbolic-link refusal, concurrency and unknown outcomes.

## BL-024 — Runtime publication ancestor admission is not yet qualified

- Flow: the new backend-owned runtime publication storage and mapped-self
  build-origin capture, currently implemented in the isolated backend candidate.
- Evidence: reviewed runtime primitive candidate 2 launched two producer probes;
  both refused native ancestor security/identity validation before the fixture
  creation boundary. Both processes closed and the scheduler admitted no later
  probes. No runtime constructor or publication case passed. Saved run:
  `../artifacts/qualification/p2-runtime-storage-build-origin-native-v1-20261026/candidate-2/native-run-986164da-687c-4d14-89d3-b51cecd8817a`.
  Post-run verification passed all 9,023 frozen files, 888 current origins,
  3,628 dependencies and 5,310 reverse source projections.
- Diagnosis at candidate 2: source policy rejected untrusted mutation grants on public
  ancestors. Ordinary metadata reads show an existing capability-SID FullControl
  grant on AppData and Local. The saved error does not identify the failing
  ancestor, so this is a candidate explanation, not a proven native cause.
  The import owner's then-current Rust integrity policy rejected those same grants
  on its LocalAppData chain. This predicts an import admission refusal on this
  host; it is source evidence, not an executed import result. Exact read-only
  ancestor descriptors and both source-mask comparisons are retained in
  `../artifacts/qualification/p2-runtime-storage-build-origin-native-v1-20261026/known-ancestor-policy-v1.json`.
- Source correction: independently reviewed Go and adopted Rust routing now
  retain the complete native parent/child graph. Existing old-strict routes
  remain accepted; otherwise an actual strict protected boundary is required.
  No SID exemption, unsafe destination acceptance or host ACL repair was added.
  Policy facts and descriptor hashes bind the same observed SD buffer. Active
  Rust tests pass 56 configuration and three coordinator cases, with release
  test compilation; selected Go Runtime/SDK pure, race and vet checks pass.
- Further observation: fresh candidate 3 passed both ancestor walks, then its
  first two producers failed while attempting their new public-directory create.
  Private fixture caches had already been created. Both processes closed; no
  constructor phase ran. The saved generic error omits NTSTATUS/output facts,
  so this failure needs better diagnostic evidence. Run
  `../artifacts/qualification/p2-runtime-storage-build-origin-native-v1-20261026/candidate-3/native-run-ffe72ab9-0386-4ac8-85ae-e66b4fa8c6a7`
  is retained. Post-run verification passed all 10,895 frozen files and 899
  current origins. No failed namespace/process was inspected or retried.
- Narrow diagnostic: a separate reviewed one-shot probe captured native status
  `0xc000050b` (`STATUS_REPARSE_POINT_ENCOUNTERED`) and no returned handle while
  attempting one fresh Local directory create. Both native ancestry graphs and
  all checked closes passed; the process closed normally. This completes a DATA
  diagnostic only. The actual reparse source is unknown, and candidate 3 did not
  record a status, so a shared cause is not established. No guard was weakened.
  Run `../artifacts/qualification/p2-runtime-local-create-diagnostic-native-v1-20261026/candidate-2/native-run-b937f28f-5926-4050-9d51-be9df94e8659`;
  root receipt `../artifacts/qualification/p2-root-local-create-diagnostic-review-20261026/result.json`.
  All 4,553 frozen files, 897 origins, 3,628 dependencies and 896 exact source
  copies passed post-run verification before subsequent source adoption.
  Microsoft's [OBJECT_ATTRIBUTES contract](https://learn.microsoft.com/en-us/windows/win32/api/ntdef/ns-ntdef-_object_attributes)
  documents the no-reparse refusal; it does not identify this host's cause.
- Read-only follow-up: nine metadata records across the held known-folder
  chains and a temporary reopen reported normal native volume names and no
  reparse tag. The single protected empty-name reopen succeeded and matched
  Local's actual directory ID/descriptor and requested read-only rights.
  All checked closes and the post-run 4,580-file/902-origin audit passed. Run
  `../artifacts/qualification/p2-runtime-handle-metadata-diagnostic-native-v1-20261026/candidate-1/native-run-ce7c9514-f71a-41ef-9687-a00483abccf3`.
  A separate own-process package query returned `APPMODEL_ERROR_NO_PACKAGE`
  from both APIs; package virtualization is not supported by that new probe's
  identity. It does not attest the earlier processes or identify the cause.
  Run `../artifacts/qualification/p2-self-package-identity-diagnostic-native-v1-20261026/candidate-1/native-run-345eac11-0aa8-4924-8059-eb0f3e09357a`.
  These probes created no namespaces, inspected no failed namespace/process,
  changed no policy and qualified no product storage constructor.
- 2026-11-06 bounded directory diagnostic: strict LocalAppData CREATE refused
  with STATUS_REPARSE_POINT_ENCOUNTERED; removing DONT_REPARSE for CREATE alone
  created an unverified object and the strict child-edge OPEN still refused.
  A separate fresh C diagnostic omitted that flag only for its own CREATE and
  original-created-slot edge; its bounded sequence completed, but the reported
  child NT name was under Codex package LocalCache rather than directly under
  the selected LocalAppData parent. Both current-process package APIs reported
  no-package in that same sample. A no-package result therefore cannot exclude
  filesystem redirection here; it does not attest earlier processes. Keep the
  production flags and actual storage acceptance gate. Native normalized names
  remain observations, not new physical-parent/durability or ownership grants.
  Root [C outcome](../artifacts/qualification/p2-root-native-create-localappdata-c-20261106-1/OUTCOME.json)
  SHA faf5f82a84b334bfcce889d2e6ff61b071e4370b33be834211cd0718b7da9b31;
  its close-accounting addendum distinguishes journal rows from cached replays.
  All targets/captures are preserved. No old target was reopened or retried, no
  admin/policy change was used, and no actual product constructor is qualified.
- 2026-11-06 strict D diagnostic: the explicitly measured Windows PowerShell
  5.1 / CLR 4 parent launched the reviewed native launcher, which successfully
  applied desktop-app policy ENABLE and launched Node. The original leaf still
  refused its single strict CREATE with `0xc000050b`, zero returned handle and
  unchanged IOSB sentinels. The policy was accepted; its proposed routing fix was
  not demonstrated. Original owners observed terminal exits (launcher 1, Node 0,
  leaf 1); all 60 pinned inputs matched afterward. The 18 leaf close journal rows
  include cached replays and are not 18 distinct native closes. No target was
  reopened, retried or cleaned; refusal does not prove absence of disk effects.
  [D outcome](../artifacts/qualification/p2-root-native-create-localappdata-d-20261106-1/OUTCOME.json)
  SHA `760c9c12fc1749632e1703a1b23c8d0693f294e140775f47cdd5b02b87a4720d`.
  Historical C's outer CLR was not measured: its requested shell label is not
  runtime evidence, and C versus D does not isolate the policy as one variable.
- 2026-11-06 fresh E comparison: through the reviewed Explorer COM broker,
  one new strict CREATE and the original-handle edge/recheck/flush sequence
  completed (launcher, Node and leaf exit0 in saved original-owner receipts).
  Four held-name observations retained the literal selected LocalAppData/child
  relationship, with zero recorded tags. The strict flags stayed enabled.
  All69 pinned inputs matched afterward. [E outcome](../artifacts/qualification/p2-root-native-create-localappdata-e-20261106-1/OUTCOME.json),
  SHA `70e058ea85e7f08829974f983d121bc2b82667e8ccb84564931ef4b7eb074d83`.
  The broker provides no original outer WinPS process handle; its receipts are
  DATA, not whole-family authority. This is one diagnostic graph, not a general
  devirtualization guarantee, crash-durability proof or product-constructor pass.
  A separate source review found that generic storage open still needs retained
  HANDLE/IOSB/frame handling for pending and partial replies. That correction
  must precede new product storage-constructor qualification. Earlier failed
  targets remain preserved; no target was reopened, retried or cleaned.
- 2026-11-07 projected constructor qualification: after retained-output and
  selector corrections, one fresh C7 PREPARE and one separately admitted PROBE
  completed through the encoded Explorer broker route. The actual selector,
  root/Serving factories, registered storage capture and mapped-self origin
  returned successfully with checked release; all66 PROBE inputs matched
  afterward. [PROBE outcome](../artifacts/qualification/p2-root-native-constructor-probe-20261107-1/OUTCOME.json),
  SHA `320f2b406c4140238510a8dbbb5d003bfc8b05bbef3ffe7afa2542ee8d3aa6bf`.
  This exercises source-projected private/local role names under the actual
  token/profile. It does not qualify the shipped namespace, newer combined
  source, publication/reopen, whole outer-family termination or crash durability.
  The possible scoped setup.lock creation was admitted as mutation; no admin
  elevation or old failed-target query, retry or cleanup was used.
- Impact: the new publisher remains unwired and unqualified. Existing setup
  locking remains in place. No host ACL repair or blanket SID trust is implied.
- Owner: OrbWin lifecycle/storage adapter, phase 2.
- Exit: resolve the ancestry invariants and supported host policy, then review a
  fresh fixture and qualify actual storage, provenance and publication. Preserve
  the failed namespaces and receipts without retry or external cleanup.

## BL-023 — Go private-config acquisition can self-deadlock while fencing a failure

- Flow: CLI admission automatically refreshing its private managed selector.
- Evidence: C7B's Go ordinary-refresh phase reached the injected lost return
  after an actual successful target rename, then hit its 40-second watchdog
  (exit 124). It emitted only `INJECTED_LOST_RETURN 15`. The other 104 launched
  phases passed. Root and an independent source review confirm that acquisition
  held the retention-registry mutex across native refresh; the mutation failure
  path tried to lock that same mutex in `successorFence` before returning.
  Saved run: `../artifacts/qualification/p2-private-config-c7b-native-v1-20261026/candidate-1/native-run-27be9c34-d74d-4d44-8bf3-07736d767d09`.
- Impact: a native failure reaching that fencing path can prevent the CLI from
  returning. The observed case injected uncertainty after success; it does not
  establish an actual Windows rename failure or data loss. The failed namespace
  and evidence are preserved without inspection, cleanup or retry.
- Source fix: reserve strong retention under a short registry lock, release it
  before all native work, and reacquire only for failure/panic fencing. The
  actual filesystem controller still owns serialization. Reentrant failure,
  panic, concurrent fencing and bounded reservation tests pass, along with the
  focused Go suite, race checks, vet and Windows test compilation. Evidence:
  `../artifacts/qualification/p2-go-config-registry-deadlock-source-v1-20261026/source-review-and-checks-v2.json`.
- Targeted native regression: two fresh cases/eight phases passed in 34.273 s,
  including prompt return, actual controller/target retention and opposite-owner
  recovery in both directions. Evidence:
  `../artifacts/qualification/p2-private-config-ordinary-loss-regression-native-v1-20261026/candidate-1/native-run-2e7bb74d-4147-44ca-a749-f3a10b02d432/root-native-receipt.json`.
- Exit: complete installed and full current-source matrix acceptance. The
  original failed case remains failed and its namespace remains untouched.

## BL-022 — Desktop and CLI private-config parsers accept different input bounds

- Flow: the desktop and CLI reading the same private Docker configuration.
- Evidence: 2026-10-26 source review and synthetic pure tests of the unchanged
  Rust `parse_config` and Go `privateClientRouting` helpers. Both accept the
  depth-64 sample; Rust accepts the depth-65 and over-node-budget samples that
  Go refuses. Go accepts an unknown key with an unpaired JSON Unicode escape
  that Rust refuses. Exact results, source extraction and input hashes are in
  `../artifacts/patch-replay/legacy-client-import-rust-transform-candidate-20261026/ordinary-divergence-evidence.json`.
- Impact: data accepted by one parser can be refused by the other. This is pure
  parser evidence, not observed installed/native behavior, data loss or a claim
  that malformed encoding is supported user data. Preserve refused input;
  never reset or discard it to make admission succeed.
- Source update: a shared ordinary bound/Unicode contract has now been reviewed
  and adopted in both entry paths: 1 MiB, depth 64 and the existing Go limit of
  65,537 counted values. Metadata parsers/codecs and ordinary credential/context/
  extension policy remain unchanged. Admitted raw unknown values stay preserved;
  unsupported bytes are refused without replacement. Active-package checks pass:
  176 Go pass events including subtests, race/vet/Windows test compilation, 39
  Rust tests and an isolated actual-eight-module release-test typecheck. Results:
  `../artifacts/qualification/p2-root-writer-continuation-20261026/ordinary-active-verification.json`.
  Native/installed refusal behavior remains separate qualification.
  The import-only typed-field and credential policy is not applied to ordinary
  writer rollback data.
- Exit: adopt a consistent supported contract in both ordinary owners and import,
  verify boundary and cross-language preservation/refusal behavior, and exercise
  both native owners against the resulting supported private configurations.

## BL-021 — SDK retirement needs positive member completion before exclusion release

- Flow: replacement of the standalone SDK/backend during application upgrade.
- Observed evidence: the bounded own-image V7 fixture on 2026-10-26 observed
  zero active members in its retained Windows job, then an immediate wait on
  its captured descendant process returned WAIT_TIMEOUT (258). A subsequent
  bounded wait returned signaled (0), with exit code 0 and the same captured
  process generation. The fixture waited before releasing its exclusions and
  passed. Exact evidence is retained in
  `../artifacts/qualification/p2-sdk-resources-retirement-native-v7-20261026/`.
- Source gap: that additional wait belongs to the test callback. The unpublished
  production physical-resource holder still relies on job emptiness and does
  not establish the same positive process-completion predicate. The stricter
  fixture pass does not qualify that production release path or mint an upgrade
  grant. V6's separate failed final check remains failed; V7 does not establish
  the cause of that earlier failure.
- Limits: this observation establishes an ordering gap, not continued data I/O,
  observed data corruption, or a defect in every other retirement family. No
  installed engine, selected runtime image, service, WSL or UI was changed.
- Exit: define and implement ownership of the complete native process cohort,
  including the end of further births; establish positive completion while
  retaining the actual resource exclusions; qualify failure and cancellation
  without releasing uncertain ownership. Real SDK/engine/guest retirement and
  publication issuers, installed upgrade and cold reopen remain separate gates.

## BL-020 — Legacy OrbWin client settings need explicit private import

- Flow: supported application upgrade, setup, registry/client configuration.
- Evidence: 2026-10-26 source review. Fresh private-client admission deliberately
  refuses the existing token-local `OrbWinData/home/.rdorbwin/docker` directory.
  At that audit no explicit import provider/review existed. This is historical OrbWin
  application state, not the user's global Docker configuration or another
  engine's settings. The source audit also found setup writing this public
  master before private admission, and Build History requiring its obsolete
  validator. Root source integration now removes those writes/prerequisites and
  uses held private snapshots plus the atomic worker for setup info; compilation
  and current installed acceptance are separate gates.
- Credential-policy gap: the initial registry-credentials description claimed
  inline-secret refusal, but existing private raw config/routing parsers accept
  typed auth values during data-preserving writer recovery. Documentation now
  records that limit. This is not an established import policy or evidence of
  vault conversion. Resolve ordinary admission and remediation policy and
  preserve rollback data explicitly; do not silently delete existing auth bytes.
- Import type compatibility: a pure audit of the pinned Docker
  `ConfigFile.LoadFromReader` rejected 84 of 185 synthetic targets admitted by
  isolated transform V2, for known-field type mismatches. The raw parser is not
  the complete Docker schema. Transform V3 now includes the import-only typed
  gate: all 382 Go/Rust synthetic classifications match and all 120 admitted
  outputs pass the pinned Docker loader. Unknown raw data remains preserved;
  unsupported types refuse without changing the source. The initial audit is in
  `../artifacts/qualification/p2-legacy-import-docker-decoder-audit-20261026/`.
  Docker's default loader can warn and continue with partial settings; this is
  not evidence that every command fails or that a helper/endpoint changed.
- Source integration: the actual fixed-source native import owner, completed-init
  CLI codec, shared bootstrap choice/review coordinator, command registration and
  explicit settings-choice UI are adopted after root and independent review.
  Frozen source proof: `688060f2…`; exact adoption record:
  `../artifacts/qualification/p2-root-import-owner-adoption-source-v1-20261026/adoption.json`.
  Active CLI pure/race/vet/test compilation, the frontend production build and
  all 257 JavaScript tests pass. Native import fixtures and installed/visual
  acceptance remain open; this source milestone does not close BL-020.
- Impact/adaptation: public settings are preserved; unsupported or unqualified
  states retain a clear refusal. Do not remove the legacy denial or use
  empty defaults to bypass the upgrade. The import contract excludes global
  settings and credential-helper/vault execution. Default import policy refuses
  nonempty inline secrets and foreign helper routes while retaining the source.
- Owner: native client/config integration, Phase 2. Product integration debt;
  no upstream defect or external issue is asserted.
- Exit: reviewed fixed-source import/recovery with exact source retention,
  private managed routing and clear credential policy; meaningful native cuts,
  concurrency/cancel/refusal and installed fresh/upgrade preservation. Verify
  upstream public context recreation never overwrites private state. Existing
  initialization and successor fixtures do not qualify this separate protocol.
- Reproducer/design: `p2-legacy-client-import-design-20261026/report.md` in
  `artifacts/qualification`; root setup plan/checks are under
  `p2-root-writer-continuation-20261026`.

## BL-019 — arm64 execution is unavailable on the qualified x86_64 guest

- Flow: cross-platform container execution and P2-D07c / platform badges.
- Observed 2026-10-25 on the owned Linux x86_64 daemon, Docker 29.7.2-ce:
  a dependency-free static Go amd64 scratch image executes successfully; the
  corresponding arm64 image builds and inspects as linux/arm64, but execution
  exits 255 with `exec /probe: exec format error`. Both probes use no network,
  a read-only root, dropped capabilities and no-new-privileges. This proves a
  current execution limitation, not the cause or absence of every binfmt handler.
- Reproducer: build/probe sources in `tests/workloads/platform-probe/`; run
  `scripts/qualify-platforms.mjs` with `ORBWIN_QUALIFY_OWNED=1`. It verifies the
  qualified endpoint and builds unique labelled fixtures without installing
  packages or changing binfmt. Raw proof:
  `artifacts/qualification/orbwin-platform-2537dc8fe5444aa7/proof.json`.
  The nonzero overall result intentionally preserves the failed arm64 gate;
  exact fixture containers/images were removed and the daemon ID rechecked.
- Buildx separately produced amd64 and arm64 **local filesystem outputs**, each
  matching its precompiled probe hash. This establishes cross-platform assembly
  without a foreign-architecture RUN step. It does not establish emulated RUN,
  multi-platform image-store load or registry publication.
- Supported adaptation / P2-D07c disposition: native linux/amd64 execution only
  for this qualified guest. Do not display an emulated-platform success badge or
  imply that selecting arm64 makes it runnable. The ordinary Docker error remains
  visible. Cross-compilation and platform-specific filesystem output are useful
  Buildx capabilities within the measured envelope.
- Owner: OrbWin backend/platform integration. Runtime emulation requires a
  separately reviewed upstream-supported dependency/provisioning path followed
  by execution, resource, restart/update and coexistence qualification. Do not
  install a privileged binfmt helper merely to make this probe pass. Revisit when
  the bundled guest/upstream changes or arm64 runtime support is explicitly added.

## BL-018 — Guest-loopback relay publication is not a qualified Windows path

- **Observed, Phase 2 relay fixture:** repeated normal Windows TLS connections
  returned EOF with guest `HostIp=127.0.0.1` despite a running non-OOM relay and
  successful authenticated relay-to-Windows gateway readiness. Independent fresh
  transports failed too; this was not only a stale client pool. Exact diagnostics
  are `artifacts/qualification/orbwin-p2-stream-1792907291043510700/managed-relay-failure.json`
  and `orbwin-p2-stream-1792907301218377900/managed-relay-failure.json`.
- The rdd publication source forwards Windows loopback to the guest tap address.
  The already-successful relay prototype accepted guest-interface traffic through
  wildcard guest bindings. The managed relay now uses that same guest binding,
  while the qualified Windows forwarder remains loopback. Native qualification
  records actual Windows listener addresses rather than inferring their scope
  from the Docker guest `HostIp` field. WSL can additionally create `::1` relay
  listeners; both families must be accounted for.
- This is a relay-specific observation. The broader user-container guest-loopback
  Docker publishing contract still needs isolated reproduction and IPv4/IPv6/WSL
  accounting under P2-D06 before Phase 2 acceptance. Do not declare all explicit
  `127.0.0.1` Docker bindings reliable based on a logical Windows mapping string.
  No user workload bindings or backend forwarding code were rewritten for this
  owned-relay adaptation. Earlier successful published-port artifacts remain
  evidence for their exact paths, not a general guest-address guarantee.

## BL-017 — Guest tap loss requires explicit recovery

- Flow/version: network interruption on the twelve-patch backend `0896047d…`.
- Evidence/reproducer: `artifacts/qualification/owned-link-flap.json`. The test
  verified the owned WSL registration, private namespace and tap MAC, armed an
  independent restoration, then brought only that tap down. The tap disappeared;
  Docker API access survived but all four HTTP probes failed and restoration
  reported the missing device. This injects guest-device loss, not a demonstrated
  ordinary Ethernet, Wi-Fi or VPN transition.
- Impact/adaptation: Docker running does not establish application reachability.
  Use the explicit Settings restart when workload networking is broken. In the
  installed test, native Prepare update then Start restored the exact HTTP marker
  and daemon identity. A second API-verified system/qualification-pod-only
  injection also passed ordinary native Restart, preserving the Docker marker
  and installed cluster PVC UID/PV/marker with a Ready node. Recovery can
  interrupt workloads; restart policies apply.
  Do not reset data, stop other engines or use global WSL shutdown.
- Owner/target: upstream networking adapter; investigate automatic device recovery
  in Phase 2. No upstream issue is claimed. Host sleep qualification is separate.
- Acceptance/regression: preserve this failed injection, qualify explicit recovery
  and require exact identity/data/foreign-engine checks for any later automatic fix.

## BL-016 — Standalone settings and whole-engine convenience boundaries

- Flow/version: proxy settings, whole-engine backup/restore, disk relocation and
  migration from other engines; Rancher source
  `af6511ad4e065e24e7258f575a0fc090c10cdbda` and OrbWin's recorded patches.
- Evidence/reproducer: inspect the complete AppSpec, registered CLI constructors
  and corresponding controllers as recorded in
  `verification/upstream-capability-audit.md`. No supported transactions for
  these capabilities were found in those executable interfaces. Electron
  preferences and proposal documents are not standalone implementations.
- Impact/adaptation: Settings states direct-network qualification and the absence
  of an application proxy setting. OrbWin does not alter global proxy/VPN/WSL
  settings. Object operations and selected-project integration remain available.
  No live disk copy, implicit migration or product backup success is offered.
  Test-only offline fixture exports do not establish a product recovery contract.
- Owner/target: OrbWin upstream adapter and Phase 2 convenience work. No upstream
  issue has been filed; this is a scoped capability finding, not a reproduced
  upstream defect.
- Acceptance/regression: revisit the source/API audit on each upstream update;
  add such controls only with a documented transaction, cancellation/recovery
  semantics and real preservation tests. Direct networking does not qualify
  managed proxies or VPN transitions. BL-006 separately covers resource scope.

## BL-015 — Kubernetes forwarding lacks the guest socat dependency

- Flow: native pod/service localhost forwarding with the pinned Rancher backend,
  openSUSE guest 0.2.11 and Kubernetes 1.35.3; observed on `e3892a1f…`.
- Reproducer: forward port 80 of the scoped Nginx pod, then request its local
  URL. kubectl first reports `Forwarding from 127.0.0.1`, then reports
  `unable to do port forwarding: socat not found` and exits on a connection.
- Impact/adaptation: the listener's readiness does not establish the complete
  stream works. The UI reports exit, retains bounded connection output and
  removes the link; the workload remains running. This is not a qualified flow.
- Dependency record: the base image lacks socat and libwrap. The official
  Leap 16.0 signed RPMs passed `rpm --checksig` and an offline dependency test.
  Manual installation in the owned guest is a proof step, not the product fix.
- Subsequent evidence: bundled automatic repair passed `bbbabad2…`; installed
  files verify cleanly and the retained PVC file survives. Native HTTP through
  the manually repaired guest passed on 60722, including navigation and explicit
  stop. Current-bundle and fresh-guest acceptance remain separate requirements.
- Owner/target: OrbWin guest packaging/provisioning, Phase 1. No upstream issue
  has been filed. Track the narrow adaptation and source/license provenance.
- Acceptance: fresh and existing guests obtain hash-pinned dependencies without
  a guest-network package download. Native pod/service forwards must carry HTTP
  traffic, survive navigation, report interruption and stop without deleting
  workloads; preserve scoped regression evidence.

## BL-013 — Existing Docker Desktop cannot reach engine readiness

- Flow: simultaneous-engine qualification on this host. Installed version 4.91.0,
  Windows 26300.9711 and WSL 2.7.11.
- Evidence: 2026-10-24 local startup logs first report inaccessible old AF_UNIX
  IPC socket files, then the backend exits with a nil-pointer panic in
  `services.(*Services).startDockerAPIProxy`. No Docker Desktop daemon responded
  on its dedicated Linux-engine pipe. Rancher's separate daemon did respond.
- Impact: required coexistence acceptance is still open; this is an observed
  external startup failure, not proof of an OrbWin coexistence pass or defect.
- Recovery attempted: retained only verified zero-length IPC directories under
  timestamped sibling names and retried startup. No reset, disk deletion, secret
  upload or global WSL shutdown occurred. Evidence stays in ignored artifacts.
- Owner/target: Phase 1 qualification. Resolve the startup precondition or obtain
  an appropriate supported test environment, then run maintained service workloads
  in both OrbWin start orders. Do not silently waive the gate.

## BL-014 — Published automatic port can redirect after an engine restart

- Flow: Windows/other-engine published TCP service access, same pinned backend.
- Reproducer: `ORBWIN_QUALIFY_OWNED=1 node scripts/qualify-coexistence.mjs --rancher`.
  Real Redis workloads on OrbWin and Rancher initially answered distinct sentinel
  values. Restarting only OrbWin preserved its daemon and data but reassigned its
  automatic port to 32768, already used by Rancher. A Windows request read the
  Rancher sentinel. See the `f46626a9` result in ignored coexistence artifacts.
- Impact: Docker's running state does not establish host publication success;
  exposed links can reach another engine. This invalidates Phase 1 acceptance.
- Required adaptation: choose available Windows ports for automatic requests,
  retain explicit bindings over restart, validate host publication, and return a
  clear conflict rather than silently redirecting. Qualify explicit conflicts,
  repeated/concurrent clients, both start orders and restart preservation.
- Owner/target: OrbWin's narrow Windows Docker/network adapter, Phase 1.

## BL-001 — Shared Windows Docker endpoint

- Flow: simultaneous engines, Docker discovery. Baseline: `af6511ad4e065e24e7258f575a0fc090c10cdbda`.
- Source reproducer: `rdd/pkg/instance/instance.go` and `pkg/socketbridge/host_windows.go`
  upstream select `docker_engine` regardless of instance suffix.
- Impact: context selection alone cannot prevent listener collision or wrong-engine routing.
- Adaptation: patch 0001 gives each instance a pipe used by both discovery and bridge.
  UI always targets the OrbWin endpoint explicitly. No global context switch.
- Owner: OrbWin host integration. Phase 1. No upstream issue filed yet.
- Exit: simultaneous real Docker/Compose/Buildx and helper-container tests pass in
  both engine start orders. Current tests cover identity and real isolated pipe allocation only.

Update: isolated Docker CLI traffic and the service/build baseline now pass on
the dedicated pipe. The simultaneous/implicit-client portion of the exit remains
unqualified.

## BL-002 — WSL host-switch guest identity and fixed ports

- Flow: simultaneous Rancher 2.0 guests. Same baseline as BL-001.
- Reproducer: `pkg/hostswitch/hostswitch_windows.go:getVMGUID` explicitly assumes
  one matching openSUSE instance. Ports 6670/6657 and signature discovery are
  shared with guest networking; `src/networking` must be adapted consistently.
- Impact: selecting a distinct pipe cannot establish the guest behind it is ours.
  WSL shared-VM behavior needs an actual scoped proof; do not boot unmodified guests.
- Adaptation: backend start is unavailable until the guest identity contract and
  host/guest ports are isolated and qualified. Diagnostics explain the reason.
- Candidate patch 0002 now separates the product discovery signature and four
  vsock ports, veth link names and WSL bridge address. The proxy uses the same
  changed address. Paired source contract checks and host tests are required;
  these are not runtime proof. The product supports one owned guest only.
- Additional source evidence: `src/networking/cmd/network/setup_linux.go`
  unconditionally cleans up `veth-rd-wsl` in the originating WSL namespace;
  sharing the name risks altering another engine's integration. The candidate
  uses a separate name and subnet, whose route conflicts still need testing.
- Pinned Lima dependency `v2.3.0-beta.1.0.20261005214905-0ae2df64800a`
  registers, starts, stops and deletes `lima-` + VM name in
  `pkg/driver/wsl2/wsl_driver_windows.go`. The stock App names its VM `rd`;
  a separate LIMA_HOME does not separate this global WSL registration. Patch
  0003 changes the App VM name to `orbwin` (`lima-orbwin`). Existing app-controller
  tests pass. No registration/boot/delete operation has been run.
- Owner: OrbWin upstream adaptation. Phase 1. No upstream issue filed yet.
- Exit: identity tied to a specific owned guest, wrong-guest rejection, collision
  tests, restart order and live simultaneous workload traffic pass.

## BL-003 — Runtime build and packaging qualification pending

- Flow: install/start, all workload operations. Same baseline.
- Evidence: this source pin requires Go 1.27, generated Linux Lima guest agent,
  overlaid WSL distro and native SQLite build. Original host inventory had no Go
  on PATH; project-local verified Go 1.27.3 now runs the focused patch tests.
- Impact: no qualified OrbWin backend bundle is available yet; do not fall back
  to a competing daemon or present an empty inventory as a successful connection.
- Adaptation: explicit unavailable state, retry and build guidance in diagnostics.
- Owner: OrbWin build. Phase 1. Exit: reproducible native bundle plus real API,
  workload, preservation and coexistence qualification. Not a phase exclusion.

Update: the native management build, all six guest binaries, rootfs overlay and
guest-capable bundle now build. Management-only and isolated guest/Linux smoke
proofs passed, as did a Redis/PostgreSQL/Compose/Buildx baseline. This entry now
covers remaining installer/release and full runtime qualification, not a missing
development executable. Raw evidence is under the task-owned artifacts directory.

## BL-004 — Persisted readiness can precede current guest provisioning

- Flow: restart/readiness. Same pinned source plus OrbWin patches.
- Observed reproducer: cleanly stop the task-owned service, resume it, wait only
  on App `ContainerEngineReady=True`, immediately run Docker info/version.
  A stored condition returned before the pipe existed. Another run briefly
  answered info, then version returned EOF while provisioning stopped the guest
  target; the guest log records the Docker socket temporarily absent.
- Impact: workload startup can race provisioning; this is not established as a
  spontaneous engine crash. Preserve raw logs and classify correctly.
- Adaptation: isolated restart proof uses upstream `ctl wait-condition` on the
  owned LimaVM `Running=True`, reason `Started`, `--since=startup`, followed by
  bounded actual endpoint/identity checks. Native inventory probes identity and
  never starts/restarts the service. UI lifecycle integration remains pending.
- Owner: OrbWin lifecycle adapter/upstream review, phase 1. No issue filed.
- Exit: repeated cold/warm starts, interrupted starts, healthy reattachment and
  failed-start behavior pass without stale readiness or unintended restarts.

## BL-005 — Native Windows bind source syntax is not translated

- Flow: ordinary Windows Docker CLI `--mount`; same source/patches and WSL 2.7.11.
- Observed: `src=F:\Codes\Orbwin\tests\workloads\bind source` is rejected by
  the Linux daemon as a non-absolute mount path. The equivalent explicit
  `/mnt/f/Codes/Orbwin/tests/workloads/bind source` read-only directory mount
  returns the expected sentinel file.
- Impact: the full Windows-path client journey does not yet pass. Non-system
  drive access exists through explicit WSL drive paths, including spaces.
- Adaptation: the Run form now accepts existing local Windows drive paths,
  canonicalizes them and maps them to WSL `/mnt/<drive>` paths. Parser tests
  passed; the separate runtime proof is deferred until host availability is
  confirmed. General CLI/Compose traffic is not translated. This does not
  qualify ordinary Windows CLI/Compose paths or hide the missing adaptation.
- Owner: OrbWin Windows integration/upstream boundary, phase 1. No issue filed.
- Exit: native Windows drive/file/directory/read-only/Compose forms pass with
  unknown Docker fields and streaming/session semantics preserved. No broad
  Docker API rewrite is selected yet.

2026-10-24 observed update: the Run adapter's live mount test initially rejected
canonical `\\?\F:\...` sources as named volumes. Local-drive classification is
fixed with a regression check. The rerun passed directory reads with spaces,
file writes visible on Windows, read-only rejection, container-restart
preservation and scoped cleanup (`bind-1792825893112476800.json`). A separately
targeted Windows Docker CLI `--mount` still fails with a non-absolute source
error. This qualifies only typed Run requests on the exercised F-drive paths;
it does not close BL-005. See the readiness reassessment for commands and limits.

## BL-006 — Requested guest resource allocation is not an observed WSL limit

- Flow: CPU/memory settings. Proof App requested 2 CPUs and 2Gi; Docker info
  reported 16 CPUs and 49,257,504,768 bytes available to the WSL environment.
- Impact: do not claim these App settings enforce independent guest allocation.
  WSL shares platform resources; exact enforcement needs upstream/WSL audit.
- Adaptation: resource editing stays unavailable, with scope to be explained
  before enabling it. Do not change global `.wslconfig` on the user's behalf.
- Owner: OrbWin settings/WSL integration, phase 1. No issue filed.
- Exit: resource scope and restart implications are documented and tested; UI
  distinguishes any global WSL setting from a product-only setting.

## BL-007 — Cleanup, disk usage and reset have no backend operation

2026-10-26 accounting update: the source-only baseline below is historical.
The current adapter provides engine disk usage, selected cleanup, host volume
capacity and explicit volume-size measurement; see
[volume-size](architecture/volume-size.md) and
[development-data](architecture/development-data.md). Their recorded native
evidence does not establish current installed combined acceptance. Whole-engine
reset remains unavailable. VHD file length is not physical allocated size;
automatic compaction and relocation are not offered. The D02 manual-maintenance
disposition is recorded in development-data. Preserve the baseline for provenance.

Labels in BL-007 to BL-012: **source-only** means read from OrbWin code or
upstream source; **observed** means seen in a running system; **unverified**
means unknown. Nothing in these entries has been run against a live engine.

- Flow: prune, storage overview, volume/virtual-disk size, reset.
- Evidence: source-only. No prune or `docker system df` adapter is wired;
  volume and virtual-disk sizes are unmeasured. BL-002 records that no reset
  operation exists. Whether the Docker prune/df endpoints behave correctly on
  this engine is unverified.
- Impact: OrbStack-style reclaim-space and reset flows cannot be offered.
- UI adaptation: cleanup is list-and-remove-selected over images, volumes and
  networks; sizes are not shown rather than estimated; restart is stop + start.
- Owner: OrbWin adapter, phase 1. No upstream issue filed.
- Exit: df/prune adapter qualified with preservation tests; sizes measured on a
  live engine; a defined reset scope that cannot touch user distributions.

## BL-008 — Batch and group operations are sequential without rollback

- Flow: multi-select batch actions, Compose group actions.
- Evidence: source-only. Batch actions run one resource at a time; a failure
  mid-batch leaves earlier operations applied. Group remove leaves project
  networks behind. Project logs/down are offered only when exactly one Windows
  compose file label is present. Image/volume "in use" markers rely on the
  `containers` prop now passed from App; unverified against live data.
- Impact: partial completion is possible; leftover networks may accumulate.
- UI adaptation: per-item results, no atomicity claim, no rollback.
- Owner: OrbWin UI adapter, phase 1. No upstream issue filed.
- Exit: live multi-select and Compose group runs, including mid-batch failure
  and cancellation, with leftovers cleaned or explained.

## BL-009 — Port links may not open from the native window

- Flow: open a published port from a row or Info tab.
- Evidence: source-only. Links use `target=_blank`; the Tauri app has no opener
  plugin, so native-window behavior is unverified.
- Impact: links may do nothing or open an unexpected window.
- UI adaptation: none yet beyond showing the address.
- Owner: OrbWin Tauri integration, phase 1.
- Exit: verify in the native app; add an opener capability if links fail.

## BL-010 — Kubernetes views have bounded depth

- Flow: pod inspection, terminal, port-forward.
- Evidence: source-only. List and inspector (Info/Logs/YAML/Events) exist. Pod
  Terminal is a single non-interactive `clusterExec`. Port-forward lives only
  while its dialog is open, so no persistent service link exists. Logs are the
  last 500 lines without follow. Namespaces with no resources of the viewed
  kind are not listed. Not run against a live cluster.
- Impact: no persistent service access comparable to OS-008/010.
- UI adaptation: limits shown; nothing simulated.
- Owner: OrbWin adapter, phase 1.
- Exit: interactive exec, followed logs and persistent forwards qualified, or
  limits accepted explicitly with rationale.

## BL-011 — WSL page is a name-based read-only list

- Flow: WSL distributions page, open terminal.
- Evidence: source-only. The list is parsed from `wsl --list --verbose`; roles
  are inferred by name only. The Rust `wsl_open_terminal` command compiled but
  is untested; it rejects engine distros.
- Impact: roles may be misclassified; terminal launch is unproven.
- UI adaptation: read-only list, no mutation controls.
- Owner: OrbWin WSL integration, phase 1.
- Exit: reliable owned/engine identification; terminal launch tested; user
  distribution independence verified.

## BL-012 — Settings and first-run behavior unqualified

- Flow: settings changes, first run, port conflicts.
- Evidence: source-only/unverified. Which settings need restart is unknown; the
  restart banner reflects only rdd `Settled`. First-run provisioning is not
  automated (BL-003). Host-port conflict handling is unqualified. The forced
  light theme does not change the Windows title bar.
- Impact: a setting needing restart may apply without a prompt.
- UI adaptation: banner states only what rdd reports.
- Owner: OrbWin settings/lifecycle, phase 1.
- Exit: per-setting apply/restart behavior documented and tested; conflicts
  exercised.

## Historical UI adapter gaps — 2026-10-22 correction

At the time of the initial correction, the list/inspector UI exposed Builds, Pods and Services explicitly.
Their histories/cluster reads and setup remain unwired. Info displays only fields
returned by the existing read-only inventory, not full Docker inspect output.
Stats, Logs, Terminal and Files tabs explain missing adapters; no stream, exec,
filesystem, mutation or build operation is simulated. Compose groups derive only
from returned project labels and have not been exercised against a live project
in this correction. This is implementation debt in full phase-1 scope, rather
than evidence that rdd cannot support these operations. Reproduce by opening the
native app and selecting the relevant destination/tab. Existing BL-001 through
BL-006 and their exit criteria remain applicable.

The later ordinary-workflow implementation supersedes this historical UI-only
description: actual mutations, full inspect, builds/Compose, logs/stats,
interactive terminals and workload-cluster adapters now exist. See
verification/workflows-2026-10-22.md for successful runtime evidence and pending
native/host qualification. Files, first-run and other coverage gaps remain in
phase-1-coverage.md; they are not automatically upstream backend limitations.
