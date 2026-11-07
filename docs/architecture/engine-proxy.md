# Engine-scoped proxy transaction candidate

Recorded 2026-10-25. P2-D01 requires actual auto/none/URL behavior; read-only proxy
presence diagnostics do not implement it. The isolated source candidate is
`artifacts/patch-replay/engine-proxy-candidate/pkg/networkproxy`. No active upstream,
SDK, engine, guest, Windows settings or workload configuration was changed.

## Verified source and ownership boundary

The selected rdd App ContainerEngineSpec has only Name, not proxy settings.
Windows host-switch `newVirtualNetworkConfig` already owns guest-facing virtual
listeners, DNS and NAT in the headless host-agent lifetime. Dockerd is started
through `wsl-exec` inside the owned network namespace and receives service-specific
environment files. The App's Lima template contains provisioning/xtrace scripts;
putting credentials there is inappropriate, and changing a creation template is
not a live transaction for an existing guest.

The shipped Docker/containerd `allowed-images.conf` drop-ins set HTTPS_PROXY to
the local OpenResty policy proxy when its configuration exists. Overriding that
proxy could bypass image policy. The candidate refuses any such policy, foreign
daemon proxy config or foreign service proxy environment until explicit chaining
is implemented and qualified. The native observation of those conditions remains
an integration gate; planner booleans do not prove policy absence.

[Docker daemon proxies](https://docs.docker.com/engine/daemon/proxy/) apply to
daemon image access and require restart. The documented
[reload option list](https://docs.docker.com/reference/cli/dockerd/#configuration-reload-behavior)
does not establish proxy reload. The selected default `docker` Buildx driver uses
[BuildKit embedded in Docker](https://docs.docker.com/build/builders/drivers/docker/).
The shipped standalone buildkitd.service is conditional on the containerd engine,
so configuring it would not configure that default Moby builder.

## Candidate package behavior

Config is strict version 1 with mode auto, none or url, an explicit URL string
(empty outside URL mode), and at most 128 validated NO_PROXY entries, all within
16 KiB. Unknown/duplicate/missing/null fields, invalid UTF-8, broad invalid syntax,
ambiguous ports and unsupported schemes refuse. Canonical HTTP(S) proxy endpoints
are accepted; credential-bearing URLs are explicitly unsupported and refused.
No accepted credentials are silently removed. Vault-backed authentication needs a
separate exact-endpoint credential reference; it is not implemented or promised.
Private URL/bypass values are excluded from errors and default String output.

`Policy.Decide` evaluates each actual HTTP(S) destination. None returns DIRECT
without reading inherited proxy environment. URL uses the validated explicit
proxy. Explicit canonical bypass entries use the same x/net httpproxy parser
already selected upstream, including loopback bypass. No HTTP_PROXY, HTTPS_PROXY,
ALL_PROXY or NO_PROXY process environment is read. Auto requires an injected
AutoResolver and fails construction if absent; the candidate does not pretend
native Windows Auto is implemented. Each call passes a copied destination URL,
accepts at most eight distinct ordered proxy/DIRECT entries, refuses unsupported
credential-bearing/ambiguous results, and refuses direct downgrade on errors.

The future WinHTTP provider must run in the original interactive owner's context,
not an alternate elevated identity. It must resolve per destination, honor native
ordered choices, free all native results, bound cancellation/callback ownership,
and never retry PAC authentication with automatic logon enabled. Microsoft's
[WINHTTP_AUTOPROXY_OPTIONS](https://learn.microsoft.com/en-us/windows/win32/api/winhttp/ns-winhttp-winhttp_autoproxy_options)
documents that false fAutoLogonIfChallenged refuses authenticated PAC downloads
instead of sending domain credentials. Opting into Auto authorizes its bounded
PAC/WPAD requests; Diagnostics presence observation never fetches/evaluates PAC.
Native PAC failures, integrated-auth requirements or unsupported entries must be
visible refusals. A one-time Windows proxy snapshot is not an Auto implementation.

`RoundTripHTTP` performs one ordinary HTTP request through the explicit selected
route with no ambient proxy, normal TLS verification for an HTTPS upstream proxy,
bounded connection/header establishment and nonpersistent per-request transport
ownership. Incoming Proxy-Authorization/Proxy-Connection are removed. It never
replays a possibly accepted application request against another route. It returns
generic failures without URL/path/credential text. A future rdd HTTP handler must
also perform standard hop-by-hop/header handling and stream admission; this is a
transport, not an activated complete forward-proxy server.

`DialTunnel` establishes an opaque HTTPS CONNECT stream through only the explicitly
authorized ordered routes. Retry occurs before an application stream is returned;
DIRECT is used only when explicitly present. Proxy TLS uses system verification,
the exact hostname and TLS 1.2 minimum, with no insecure fallback. CONNECT headers
are bounded (4 KiB line / 16 KiB total); buffered bytes following the 200 response
remain part of the returned stream. Cancellation closes the retained connection;
write half-close preserves the remaining response. Guest target TLS remains
end-to-end: this component does not terminate/decrypt destination TLS or install
corporate roots. The future server must reuse existing qualified duplex stream
ownership rather than inventing another lifecycle.

## Native Auto resolver source, opt-in and qualification boundary

The candidate now includes Windows `NativeAutoResolver` source and a non-Windows
refusal stub. `NewNativeAutoResolver(nativeOwnerSID)` validates the canonical
ordinary-user SID spelling and probes required WinHTTP exports using System DLL
loading. It does not query user configuration, fetch PAC, perform WPAD or open a
proxy session. SID input must come from the registered native owner, not JSON.

`Capture(ctx)` separately locks the calling OS thread, rejects any impersonation
token, verifies the process token's exact owner SID and nonzero interactive
session, and reads current-user configuration. It is one-shot; refresh creates
a separately reviewed provider/revision. Native UTF-16 inputs are copied with
length/encoding bounds; all returned config allocations are GlobalFree'd. The
configuration is private with no exported/JSON fields. PAC query tokens, proxy
endpoints and bypass names never enter native errors or guest data. PAC URL
userinfo is refused, and normal HTTPS validation is not disabled.

Static/manual configuration is resolved locally per destination with exact
scheme mappings. `https=proxy:80` means an HTTP proxy for HTTPS destinations,
not TLS to that proxy; an explicit `https://` proxy endpoint preserves TLS.
Supported native bypass syntax is exact DNS/IP, leading `*.domain`, `*` and
`<local>`; unsupported patterns fail visibly rather than being silently ignored.
Mixed automatic/manual configuration is currently explicitly refused because
its failover policy has not been qualified. This is a remaining supported-envelope
gate, not removal of Auto from Phase 2.

For PAC/WPAD, `Resolve` verifies owner/thread identity again before initiating
[WinHttpGetProxyForUrlEx](https://learn.microsoft.com/en-us/windows/win32/api/winhttp/nf-winhttp-winhttpgetproxyforurlex).
Microsoft documents Ex as asynchronous, cancellable, out-of-process and returning
the full proxy list; the classic synchronous API does not supply those same
guarantees. A fresh native session explicitly uses NO_PROXY for its ambient
session configuration; PAC discovery/evaluation uses only captured user options.
fAutoLogonIfChallenged stays zero, with no authenticated retry. The actual target
URL is passed per call, preserving its authority. The destination is never
requested by this resolver; PAC itself may perform DNS and script network work
only after the user opts into Auto. Existing HTTP/CONNECT transports continue to
validate proxy TLS and bind Host/CONNECT authority to the decision destination.

The callback is registered before creating each resolver, with request-error,
completion and handle-created/closing notifications. A monotonically allocated
integer context token references a registry bounded to eight pending operations.
Options/URL/PAC buffers are explicitly pinned until both session and resolver
final callbacks, successful close acknowledgements and completed Go native
cleanup. Session and resolver notifications match their exact handles; a session
notification cannot complete a decision. Callback code copies only the fixed numeric error structure and
signals bounded channels; it does not retain native result pointers or URLs.
[GetProxyResult](https://learn.microsoft.com/en-us/windows/win32/api/winhttp/nf-winhttp-winhttpgetproxyresult)
results are bounded to eight copied typed entries and released with
WinHttpFreeProxyResult. Direct entries are explicit, HTTP/HTTPS proxy schemes are
preserved, credentials/unsupported SOCKS/invalid ports/duplicates refuse, and no
implicit direct fallback is added. Inapplicable hostname fields on DIRECT are
not dereferenced.

Each decision uses an eight-second context. Closing the asynchronous resolver
cancels it; retirement waits up to two additional seconds for HANDLE_CLOSING.
Microsoft's [close-handle contract](https://learn.microsoft.com/en-us/windows/win32/api/winhttp/nf-winhttp-winhttpclosehandle)
requires retaining context through that last callback, since callbacks can occur
after CloseHandle returns. A failed close or missing retirement acknowledgement keeps the slot
and pinned state retained and reports unknown, instead of freeing callback data
or admitting unlimited abandoned operations. Provider Close cancels active calls
and reports success only after all operation slots retire. Configuration read,
Open/SetCallback/SetOption/Close and other synchronous WinHTTP calls cannot be
preempted by a Go context; a supervisor process deadline remains required for a
hard wall-clock bound. Hosting/isolating those residual blocking calls must be
reviewed before active rdd integration; the candidate is not proof that arbitrary
native stalls cannot affect host-agent teardown.

Tests run without native PAC traffic: exports are present on this Windows host;
constructor does not capture, uncaptured Resolve refuses, canceled Capture
refuses, and an explicitly distinct fixture SID fails token identity before any
actual-user proxy configuration query. Additional pure tests exercise bounded
UTF-16, ordered native choice conversion, per-protocol manual/bypass semantics,
mixed/credential refusal, malformed numeric callback data and exactly-once
registry/pin retirement with late duplicate notifications. Those callback tests
are state/lifetime tests, not evidence from a native PAC callback. Unit, vet and
Windows race checks pass. Actual local PAC success/error/ordered-choice,
cancellation/handle-closing timing, PAC HTTPS certificates/auth refusal, caches
and owner-context transitions remain native qualification gates. No user PAC,
WPAD, domain authentication, proxy settings, engine or guest was accessed/changed
by this qualification.

The additional source-only local PAC subprocess harness and corrected cleanup
invariants are recorded in
`artifacts/patch-replay/engine-proxy-candidate/LOCAL-PAC-QUALIFICATION.md`.
Its tagged tests compiled and three separately reviewed synthetic native PAC runs
completed. Two loopback-target cases fetched the PAC and retired but returned
only DIRECT, failing the fixture expectation. A documentation-address target
(decision only, never contacted) returned the ordered HTTP proxy then DIRECT and
passed, including native Close/zero pending operations and child exit/reaping.
The passing evidence directory is
`artifacts/qualification/p2-local-pac-9a3cb3f3c1a2450196a8460625f26953/`.
This is one local PAC/target scope, not arbitrary URL/TLS/auth/cancellation
qualification. Implicit platform loopback bypass is an inference, not a verified
WinHTTP cause. The harness does not enforce an OS-wide external egress sandbox.
A separately reviewed delayed-PAC cancellation passed after the server observed
the original GET: zero routes returned, both successful closes/final callbacks
observed, zero pending, fixture handlers joined and child exited/reaped. Evidence:
`artifacts/qualification/p2-local-pac-cancel-58926fa6366740d68401e00cdfa6f6b0/`.
This qualifies only that controlled cancellation point, not arbitrary PAC stalls.

Non-network evidence is
`artifacts/qualification/p2-native-proxy-nonnetwork-e9d655fba117410bba3cf51348a7a201.jsonl`,
SHA-256 `2df5a4471085c635833d2ab06b5f3d20a6aa3cd42b969b912ff5ffd6b8bd7c71`.
It records the five native-source tests above, including actual export
availability and owner-token refusal, not an executed native PAC request.

## Fixed guest plan and recovery invariants

The proposed virtual-only proxy endpoint is `http://gateway.docker.internal:3129`,
owned by rdd's existing virtual network. That static gateway name already exists
in the selected source. Port availability, guest DNS, policy and lifetime still
require live qualification. No Windows/LAN listener is proposed. Upstream proxy
URLs, PAC contents, credentials and corporate bypass entries stay Windows-side.

Only `/etc/systemd/system/docker.service.d/90-orbwin-network-proxy.conf` is planned.
The enabled form sets both cases of HTTP_PROXY/HTTPS_PROXY to the credential-free
gateway, a fixed local NO_PROXY set and empty ALL_PROXY variants. None clears the
owned service proxy variables; it does not mutate global environment. URL-to-Auto
changes preserve the same guest endpoint and do not require a needless daemon
restart solely for routing selection. First installation/None transitions require
the reviewed component restart; healthy workloads may be interrupted and restart
policies apply. No WSL shutdown, other engine or user distribution is involved.

`PrepareGuestChange` requires exact registration GUID, machine ID and daemon ID,
no conflicting policy/proxy, and either absence or a bounded root-owned mode-0600
regular file with retained complete hash and one of the two exact generated forms.
A marker alone never adopts a foreign file. File device/inode and contents are
retained; `GuardGuestCAS` refuses identity, metadata, inode or hash changes before
mutation. This is a pure comparison contract, not atomic kernel compare-and-swap.
The future fixed guest helper must hold its exclusive cooperative transaction
lock through no-follow handle validation, secure temporary write, fsync, rename
and parent fsync. Unexpected foreign root edits refuse recovery; the helper must
not claim a preflight closes all races with independent privileged actors.

An intent is durably recorded before write. Restart is a separate recorded
operation, not inferred from cancellation or file presence. `RecoveryState`
distinguishes not-written, write-present/restart-outcome-unknown and applied only
after an identity-bound native restart receipt for the exact content. Unknown
restart outcome never triggers automatic replay or rollback. Review/explicit
recovery must observe the fixed file and actual component state first. Receipt
authentication, protected storage and actual native operations are not supplied
by this pure package.

| Fault or boundary | Required outcome before integration |
| --- | --- |
| Invalid mode/URL, userinfo, bad bypass, unknown/duplicate fields | Refuse without printing private values; no mutation |
| Auto provider absent/error/timeout/unsupported auth or result | Visible failure; no implicit DIRECT or global fallback |
| PAC returns several destinations/proxies | Resolve separately per target; retain explicit route order |
| HTTP application request may have been accepted | Never replay it on another route |
| CONNECT proxy refuses, malformed/oversize header, untrusted TLS | Close that attempt; try only explicitly listed establishment alternatives |
| Caller/controller cancellation or headless shutdown | Close/join owned streams and native callbacks; keep unknown guest intent |
| Allowed-images policy/foreign daemon or service proxy | Refuse bypass/overwrite; do not disable policy |
| Guest, daemon, file inode/owner/mode/content drift | Refuse before write; preserve observed foreign state |
| Crash/cancel before durable write | Re-observe as not-written; no restart claim |
| Crash/cancel after write or during restart | Keep unknown outcome; no blind restart/rollback |
| Restart succeeds but acknowledgement is lost | Reconcile native identity/config receipt before marking applied |
| Other engine, existing container or explicit Compose configuration | Preserve configuration and process lifetime |
| TLS interception/private CA/authenticated proxy | No trust downgrade or unsolicited credential use; explicit scoped support gate |

## Integration recipe and remaining scope

Copy candidate `pkg/networkproxy` into upstream `rdd/pkg/networkproxy` unchanged;
it is not under SDK `internal/` and has no cross-module import problem. Register a
narrow AppSpec proxy desired-state/revision and condition, validated in upstream
admission and reconciled by rdd. Add the headless host-switch virtual listener,
native owner-context resolver and safe HTTP handler; prove they stop with their
owner and never bind a Windows/LAN wildcard. Add the fixed guest observer/writer
and durable transaction receipt before wiring explicit component restart. No
copy, patch registration, generated API change or live operation is done here.
Preparation order is material: validate/build the immutable Windows policy (Auto
must have its native provider), retain the headless virtual listener, and prove
readiness before the guest can reference it. Record desired/applied revisions
separately. Keep existing stream policy/handles alive while new requests adopt a
committed revision; do not make a half-written desired configuration effective.
Guest write/restart/acknowledgement then follows the durable intent protocol.
The isolated planner cannot establish listener readiness or native Auto support.

The isolated candidate now also contains a working supplied-listener forward
server and local transport tests. Exact runOnce/hostagent/AppReconciler hooks,
ownership and remaining activation gates are in
`artifacts/patch-replay/engine-proxy-candidate/HOSTSWITCH-INTEGRATION.md`.
It binds nothing, takes one immutable policy, delegates CONNECT duplex to the
existing upstream Pipe adapter, and closes/joins accepted streams on owner exit.
This is source preparation; active gonet/controller/guest integration has not run.

Future container/build defaults belong in OrbWin's private Docker config keyed
by the exact owned daemon endpoint, not a `default` entry. GUI creation/build
must merge only defaults absent from explicit user configuration. Existing
containers, existing Compose env and user Docker configs are preserved. Docker
SDK/Testcontainers may ignore CLI config, so an opt-in environment path must be
qualified rather than claiming transparent proxying of every TCP connection.
[Docker documents](https://docs.docker.com/engine/cli/proxy/) that these defaults
affect new containers/builds and not daemon pulls, and that container env is
inspectable plaintext. Only the credential-free owned endpoint belongs there.
RUN proxy arguments are distinct from registry/image resolution and can leak if
explicitly referenced by a Dockerfile. Containerd/K3s/standalone BuildKit and
nondefault Buildx drivers need separately qualified service/worker transactions.
GUI/control-plane/local service requests remain explicitly direct.

Candidate package tests, vet and Windows race checks pass using local owned
HTTP/TLS/TCP fixtures. Tests cover exact bypass semantics, per-destination Auto
callback choices, no inherited env/failure downgrade, credential refusal, HTTP
no-replay, CONNECT ordered failover/buffer/half-close/cancel/header bounds, TLS
verification refusal, policy conflicts and guest identity/ownership/CAS/recovery
faults. No native PAC/provider, active host-switch server, guest file transaction,
daemon restart or full proxy/VPN managed-network journey is claimed complete.

## Integrated source checkpoint, 2026-10-26

The earlier isolated-candidate and future-wiring descriptions above are
historical. Patch `0017-private-engine-proxy.patch` now wires the App reconciler,
fixed guest transactions, Lima launch observation, host-agent and host-switch
owners into the selected source. Its 165-file catalog, generated schemas,
focused tests/race/vet, independent review and guarded source adoption are in
`artifacts/patch-replay/proxy-integrated17-review-7cbb3277-3520-4497-9f0f-8d2439f19e29/`.
The adoption receipt is
`active-source-adoption-d0b73c3f-38d1-493a-b568-c51b0cd03fb7/proof.json`;
the ordered clean replay is
`artifacts/patch-replay-a74bd1dd-5fd4-487c-bd46-bf3f515aac65/`.

The source adoption did not replace the selected backend or SDK executables.
Native private-origin authentication and cold upgrade, the combined current
build, actual None/URL/Auto/refresh transitions, guest restart and workload/data
preservation, proxy pulls/builders and supported network/coexistence matrix still
need qualification. These are deployment and acceptance gates, rather than
absence of the controller/guest/host source integration.
