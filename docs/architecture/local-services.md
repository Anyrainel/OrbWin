# Local service names and HTTPS

Phase 2 design/implementation record. This is an original Windows adaptation,
not an observed OrbStack implementation. No DNS or system root trust has been
applied to this host. The user restored native access on 2026-10-24. Disposable
live relay qualification now supplements the component tests; permanent product
setup remains incomplete.

## Network observations and ownership

Windows and selected Ubuntu resolve `probe.orbwin.localhost` to loopback on this
host. Ubuntu cannot reach a Windows loopback-only Vite listener through that
name. A disposable static HTTP process in an owned Docker host-network container
also could not be reached through either Windows or Ubuntu loopback, despite
remaining running. The backend has distinct networking scopes. The exact probe
was stopped/removed without changing routes, resolvers, other engines or trust;
see `p2-host-network-loopback.json`. Alpine's pinned minimal image lacks `httpd`;
the failed prerequisite route is retained separately, and the actual probe used
our static fixture binary.

Therefore one successful Windows `.localhost` link cannot establish the WSL or
container matrix. The remaining routing proof must establish a reversible,
explicitly bound reachable endpoint for each client class before selecting the
final integration. Broad DNS takeover, implicit global proxy changes and a new
network stack are outside the design. Container network aliases and selected WSL
OS/client mechanisms are candidates, not qualified behavior. Optional Kubernetes
retains its existing persistent-forward adaptation pending explicit D05 accounting.

Only the owned verified daemon can supply routes. Derive identities from full
container IDs and exact Compose labels; reconcile create/recreate/rename/stop,
and never forward a stale route to an unrelated process that reused a host port.
Multiple or non-web listeners require an explicit reviewed route selection.
Reserved integration listeners fail cleanly when occupied; never stop or steal
another engine's listener. UI closure must not stop a healthy integration or
workload, and disabling integration must remove only owned listeners/resources.
The specialized route service must not become a competing rdd lifecycle manager.

## Selected routing direction after the published-port proof

The ordinary Docker-publishing path **does** bridge both Windows and Ubuntu
loopback: `p2-published-client-loopback.json` records the same static marker through
`probe.orbwin.localhost:58433` in both clients. Host-network mode bypassed that
publishing path and was therefore the wrong primitive. A container request through
`host.docker.internal:1420` reached the owned Windows Vite listener (initially its
expected Host-header denial). Reuse this verified direction rather than adding
NRPT, resolver replacement, hosts-file edits or a new WSL network supervisor.

The selected implementation direction is a small owned relay container with
ordinary published HTTP/HTTPS ports. It forwards opaque TCP to loopback-only
Windows listeners through the upstream host gateway. HTTPS terminates in the
Windows route process, so even leaf private keys need not enter a guest. That
process owns CA protection, route selection and source identity checks, while
rdd remains engine lifecycle authority. Ports are reserved through the existing
upstream publication adapter; conflicts must refuse cleanly. All service lifetime,
expiry and inactive-ownership recovery still need implementation/qualification.

Windows/WSL links use `.orbwin.localhost`. Selected container-client networks can
use `.orbwin.test` aliases on the relay, avoiding clients that special-case
`.localhost` to their own container loopback. Network joins/aliases apply only to
the owned relay after reviewed selection, never by changing a workload's network
or every image's DNS/trust configuration. TLS chain/client-store compatibility,
expiry, network removal and optional-cluster accounting remain explicit gates.

For upstream HTTP connections, a direct mutable container-IP route has a reuse
race even if it was inspected immediately before dialing. Use a bounded diagnostic
helper sharing **only** the exact target's network namespace, with no target PID,
filesystem, credentials or Docker socket. Its trusted tools can dial target
loopback through a non-TTY Docker exec stream. The namespace remains tied to that
target incarnation; another container reusing an IP cannot replace its loopback.
Reconcile target full ID/start time and retire old helpers/connections on restart.
This reuses the qualified Debug Shell namespace pattern with fewer privileges.
Streaming/half-close/backpressure, target exit, client/controller loss, renewal,
helper/resource caps and concurrent requests must be proved before acceptance.
No running service or completed HTTPS integration is claimed by this design note.

## Certificate authority boundary

Local HTTPS is an explicit opt-in setup with reviewable certificate/trust scope.
Windows current-user trust, selected WSL stores and container client stores are
different transactions. Denied/partial trust must stay visible; never disable
verification or blanket-edit every image. Public certificate distribution does
not grant authority to copy credentials, SSH agents or private keys into guests.

Use a generated root public certificate and a separate constrained intermediate.
The root private key is used only during initial creation and is not included in
the persistent authority bundle. The persisted signing key belongs to the
intermediate, constrained critically to `.orbwin.localhost` and `.orbwin.test`,
server authentication only, with no subordinate CA depth. A compromised ordinary
signing key must not authenticate an unrelated public/internal hostname in a
standards-validating client. Protect the private bundle with the Windows user's
OS protection before persistence; no guest receives the root/intermediate key.
The Windows route process retains the constrained signer and issues short-lived
leaves for active names. Individual routes cache only their selected leaf; this
is not a separate-process isolation boundary between signing and HTTP serving.

Bind installation/trust ownership to the exact generated root fingerprint and
registered persistent home. Fixed certificate profile and bounded parsing must
refuse modified/unsupported material. Leaf issuance accepts only canonical ASCII
service names inside the two reserved namespaces, never arbitrary caller domains,
IPs or wildcards. Root/intermediate rotation needs an explicit reversible trust
transaction with expiry diagnostics; no silent replacement of user certificates.
Disable/uninstall/update retention and removal must distinguish public trust,
protected signing state and active leaf certificates. PKI unit tests are evidence
for certificate constraints, not evidence that Windows/WSL/container trust setup
or browser behavior is already qualified.

## Route and namespace implementation contract

`backend-tools/session/localnet` now contains a route registry, TLS selector,
verified namespace dialer and scratch-image importer. They are source modules;
the persistent controller, ownership ledger, setup UI and installed service are
not yet wired. The library has no engine/DNS/trust setup side effects.

A complete route snapshot is bounded to 256 canonical names and validated before
publication. Each route records the full target ID, StartedAt, full helper ID and
one reviewed TCP port. Duplicate names refuse the snapshot. Unchanged snapshots
retain connections; changing/removing a route cancels requests, closes backend
connections (including HTTP upgrades), and rejects late connections. Backend
connections use a fixed HTTP destination over the verified namespace tunnel,
never an IP/host-port fallback. TLS requires matching SNI and HTTP Host. Unknown
names, absolute-form proxy requests and CONNECT are refused. Selected plain HTTP
links redirect to HTTPS, preserving the escaped path/query; client-supplied
forwarding headers are replaced. Leaf certificates are cached by active route
and authority fingerprint and renewed with an hour remaining.

Before dialing, the adapter checks current target ID/start/running state and the
exact helper ID/image/name/labels, executable, namespace, non-root user, mount,
capability, security and resource profile. The controller must also check target
identity before creation and after start, retire routes on observed target change,
and persist immutable create intent and accepted ID before starting a helper.
Read-only verification itself neither adopts nor removes resources. Separate
Docker API checks cannot form an atomic transaction with target restart; the
namespace binding prevents a mutable IP from selecting an unrelated workload,
while post-start checks and reconciliation must bound stale-incarnation service.

Initial budgets are 32 total backend connections and four per helper, shared
across aliases. Each namespace helper has 64 MiB memory, 128 PIDs, half a CPU and
a 64 KiB non-executable tmpfs for its 45-second monotonic lease. These are initial
bounds, not measured capacity guarantees; real concurrency/memory/expiry proof
and tuning remain required. Idle detached tunnel execs expire after two minutes.
The permanent controller must disclose that idle policy and join/close requests
when integration is disabled, engine identity changes or a helper cannot renew.

Headless Windows unit tests cover selected forwarding, hostile host/SNI and
forwarding headers, escaped redirects, atomic-invalid snapshot refusal,
replacement/late-dial/in-flight cancellation, shared capacity and shutdown.
A real loopback test uses the generated CA with certificate verification enabled,
upgrades HTTP to a binary stream and observes closure on target replacement.
Fake-daemon tests reject changed target/helper ownership and isolation before any
exec mutation. A separate ownership check accepts exact stopped/start-failed
helpers for reviewed cleanup, so target loss does not require a running helper
to establish ownership. All Go package tests and vet pass; Linux test binaries
cross-compile. Source hashes and the current localnet unit result are recorded in
`artifacts/qualification/p2-localnet-source-20261024.json` and its linked JSONL.
The race detector was not run because no C compiler was discovered on this host.
This is not live Docker transport, OS trust, WSL/container TLS, native UI or
installed qualification. Those runs remain deferred under the desktop restriction.

## Namespace ownership and recovery implementation

The namespace manager and Windows ledger are now implemented in `localnet`.
They are controller components, not a launched permanent service. Startup first
checks the registered Linux daemon and the verified imported tools image, then
opens an exclusive Windows handle in the supplied persistent-home subdirectory.
A second live controller cannot take that lease. Daemon identity is rechecked
before create, renewal, dial and cleanup operations. Endpoint changes refuse
mutations; they never authorize operations against another engine.

Each namespace has an immutable versioned JSONL intent, flushed before Docker
Create, and one accepted full-ID record, flushed before Start. The directory is
bounded to 512 records and records to 8 KiB. Partial, foreign, renamed and
unsupported records block new ownership transactions and remain available for
review. After exclusive ownership is obtained, recovery can inspect the exact
recorded ID, or the exact intended unique name when Create acceptance is unknown.
Name discovery must pass the full metadata/isolation check and flush its accepted
ID before cleanup. A missing unknown create is retained because the original
daemon request can finish late. A known-ID absence or confirmed exact owned
removal permits settlement; there is no broad label search, prune or source-data
deletion. Pending uncertainty is exposed through a typed status method.

The manager bounds active targets to sixteen, reuses a healthy helper for an
unchanged target incarnation, and verifies the target again after helper Start.
Failed start/post-start verification cleans only that accepted exact helper.
Uncertain creates are not retried with the same name. Renewal/dial setup can run
in parallel while create/recovery/removal are serialized; operations have bounded
contexts. Renewal checks the guest exec exit result, rather than treating its
start as a healthy heartbeat. Close cancels and joins in-flight renewal/dial
setup, cleans exact active helpers and releases the process lease. Cleanup
failures retain records. The caller must first close the route registry so
established streams are retired before namespace teardown.

Windows headless tests exercise second-owner refusal, damaged/foreign journals,
accepted-ID ordering before Start, failed start, target restart, changed daemon,
foreign helper, failed renewal, cancellation/join on Close, unknown late Create
acceptance and exact cleanup. A deliberately exited child test process bypasses
Close/defer after flushing its accepted ID; the next process obtains the released
Windows lease and reads the preserved record. This proves that process-loss case,
not power-loss durability or actual Docker cleanup. Fake-daemon tests do not
replace live engine/resource/lease-expiry qualification.

The permanent process, relay ownership/image-import ledger, scheduled discovery,
protected authority persistence, reversible trust transactions and setup/
status UI still need wiring and qualification. None of those are implied by a
passing namespace-manager test.

## Discovery and reconciliation implementation contract

Discovery reads only the registered owned daemon, caps its inventory at 512
containers, and excludes OrbWin helpers and Compose one-off jobs. Container names
form standalone proposals; coherent Compose project/service/replica labels form
stable service proposals. A single running replica can have a service/project
alias; multiple running replicas require distinct replica names or an explicit
selection, not an invented load balancer. Stopped former replicas do not compete
with their running replacement. Name conflicts remain visible for every involved
candidate. DNS-label normalization must preserve distinctions through a stable
hash whenever a source label is changed or shortened.

User preferences use exact standalone IDs or coherent Compose project/service/
replica selectors, so reviewed Compose choices can survive recreation without
guessing membership from names. Preferences can disable a proposal or select a
reserved name/TCP port. Port declarations are hints: one TCP candidate requires
an HTTP probe before publication; multiple/no TCP candidates require selection.
UDP and a published port number alone do not establish an HTTP service. No
discovery call creates a helper, probes a workload or installs a route.

`DiscoverServices`/`BuildCandidates` implement those proposals and versionable
typed preferences. Invalid selectors/overrides and duplicate full IDs refuse the
snapshot. Paused, restarting, stopped, disabled, unsupported-network, ambiguous
port and name-conflict states remain distinct. Unit tests cover coherent Compose
recreation, stopped former replicas, multiple/duplicate running replicas,
incomplete labels, exact container overrides, normalization, alias collisions,
excluded helpers/one-offs and owned-daemon-only bounded read APIs.

Host-network and container-shared-network targets are excluded from this private
namespace adapter: their loopback can belong to another workload. That refusal
is also enforced by target inspection before helper creation/dial, rather than
relying on discovery alone. Existing published links remain available. This does
not establish process ownership of every socket inside a namespace; another
container deliberately joined to the selected namespace is part of that shared
boundary. No claim of per-process network isolation is made.

`ServiceController` now connects reviewed candidates, the namespace manager,
HTTP probing and the route registry. It is a tested component; no permanent
process or timer has been started. It validates a complete selection before
mutations, inspects current target incarnations, retires stale names/ports/
incarnations before helper removal, and renews independent helpers with at most
four concurrent jobs. A failed namespace does not remove independently verified
routes. Startup/probe/unrenewable-helper failures have at most three automatic
attempts per selected incarnation/port, with one/three-second retry delays and
visible retry timestamps. Exhausted attempts require explicit retry or a new
target incarnation/port. A successful probe does not replenish repeated heartbeat
failures; a successful renewal is required. Failed probes release unused helpers.

Probes send HEAD `/` only through the same owned namespace dialer and shared
connection budgets as routing. They have a three-second overall timeout,
two-second response-header timeout and 16 KiB response-header budget, and do not
follow redirects. A valid HTTP error/redirect establishes protocol support, not
application health. Active status is assigned only after verification; it cannot
be supplied as an input discovery state. Invalid or pre-cancelled snapshots do
not change the active graph. Close cancels/join reconciliation, retires streams,
then closes the namespace backend. Inventory failure has an explicit invalidation
method; the permanent scheduler still needs to call it and expose degraded status.

Headless tests cover unchanged reuse, rename, changed port/incarnation boundaries,
disabled/unavailable targets, partial renewal failure, bounded retries, repeated
unrenewable helpers, cancellation/join, oversized probe headers and no redirect
following. A combined Windows test runs the controller through the real namespace
manager/Moby SDK against a controlled TCP Docker API fixture: journal before
create, ID before start, fixed non-TTY exec, framed HTTP probe, renewal and exact
removal. That fixture uses the SDK's TCP endpoint mode for hijacked exec streams;
an HTTP-scheme mock supports ordinary API calls but cannot stand in for this
transport. This contract test executes no Linux helper and is not a live Docker,
Windows named-pipe, client trust or installed product qualification.

## Authority persistence transaction

Authority creation is an explicit setup action under the same exclusive local
service owner. Persist only one bounded versioned record: installation identity,
public root certificate/fingerprint and a current-user DPAPI-protected signing
bundle. Never write a plaintext key or private temporary file. Flush a unique
encrypted draft, then publish the fixed authority path exclusively; an existing
record must refuse creation even if it is damaged or expired. Trust installation
must happen only after committed state is loaded and validated against the
reviewed fingerprint. A crash must not silently generate a replacement for a
root that could already have been trusted.

Loading requires the registered installation and expected root fingerprint,
strict bounded parsing, exact public-certificate/profile checks, DPAPI integrity
and agreement with the protected bundle. Foreign, modified, expired or incomplete
state produces a visible refusal, never regeneration or verification bypass.
Public certificate review/export is separate from loading signing authority and
from installing trust. Disable retains signing state until an explicit removal/
rotation transaction; no implicit root-store mutation is part of file access.
File-flush and process-crash tests do not establish power-loss durability.

The Windows implementation now stores that record at `authority-v1.json` under
the exclusive owner. It flushes only encrypted draft contents and publishes with
the same-directory exclusive NTFS hard-link primitive. Load requires the reviewed
root SHA-256, rejects replaced/non-regular/linked/oversized state paths and checks
the public record against the decrypted signing bundle. Public review/export
returns only the root PEM and explicitly named root validity dates; signing expiry
is a separate value. Public review does not establish signing-key integrity or
OS trust, and any trust action must first complete Load successfully.

Headless Windows tests pass save/reload/leaf issuance, no private key/bundle in
persisted plaintext, exact public-only PEM export, duplicate setup refusal,
foreign installation/fingerprint, ciphertext/public-record tampering, unsupported/
partial/oversized state, expired signer refusal, directory/link refusal and
committed-root survival across deliberate process exit. Refusals preserve the
original bytes. State is retained after expiry; rotation/removal and root-store
transactions still need implementation and native/client qualification.

## Tools image import ownership

The same exclusive owner now records a flushed token/asset/daemon intent before
importing its verified static scratch image, followed by a flushed accepted image
ID. Subsequent reuse verifies the exact immutable ID, architecture and component/
asset/token labels. A lost import response leaves a receipt; retry performs only
an exact three-label image lookup. Zero/multiple matches or mismatched metadata
remain uncertain without another import. No registry pull, mutable tag replacement,
image removal or pruning is part of this operation.

Explicit repair can archive a missing known-image receipt or an uncertain receipt
with the reviewed token, preserving evidence before another import is authorized.
An existing known image cannot have its receipt retired. Unknown imports may still
finish late; their archived token remains available for later audit. Conflicting
archives refuse and preserve both files. Image receipts use a separate suffix
from namespace intents so namespace recovery never treats them as containers.

The namespace manager can now take the already-held owner after CA/image setup,
requiring a committed image receipt and matching metadata. This avoids releasing
the process lease between setup and serving. A failed handoff leaves the caller
in ownership for diagnostics; a successful manager closes that owner on shutdown.
Headless tests cover import ordering, exact reuse, lost/missing/late acceptance,
ambiguous/foreign matches, missing-image explicit repair, archive conflict,
foreign-daemon refusal, partial receipt refusal, zero-owner refusal and lease
continuity across the handoff. Live import/setup remains deferred.

## Gateway identity requirement before permanent process wiring

Code review identified a Windows-side reuse gap in the current plain relay
prototype: after the route process dies, its ephemeral loopback port could be
reassigned before the guest lease expires. The target-namespace binding does not
address that separate hop. No product relay has been launched and no such traffic
has been observed; this is a design finding that must be closed before wiring it.

Use an additional TLS tunnel from relay to a process-specific Windows gateway.
The relay receives only a public, private-purpose root certificate, verifies it
with a dedicated root pool and exact instance hostname, and sends an HTTP/HTTPS
mode prefix only after the standard verified handshake. Client HTTP/TLS bytes
remain opaque inside that tunnel; application HTTPS and private signing keys stay
on Windows. A newly reused Windows port cannot receive application bytes without
the old process's ephemeral gateway key. Do not use OS trust changes, arbitrary
system roots or disabled verification for this hop. The gateway certificate is
separate from the persisted application authority and its public trust workflow.
The gateway implementation below replaces that prototype. Rotation/expiry policy,
full resource proofs and permanent-process activation remain qualification gates.

## Authenticated gateway and route-host implementation

`internal/gateway` now implements a fresh process identity, a dedicated public-root
client pool, exact instance SNI, required private ALPN and a bounded version/mode
handshake. It never loads system roots or disables verification. The ephemeral
root signer is not retained; the server holds a separate leaf key in Windows
memory, and only the public root enters relay configuration. Identity objects do
not expose serializable key fields. Validity is one year and the parsed expiry is
exposed; the permanent process must handle approaching expiry/rotation alongside
the application signer rather than serving indefinitely past it.

The static Linux relay now uses that transport for both public listeners. Its
ABI is 2: `relay <session> <gateway-port> <base64-public-root>`. The old pair of
plain upstream ports is no longer accepted. It validates the bounded canonical
root input before opening listeners. The tools manifest covers every gateway
source dependency; package staging expects the new ABI. Application bytes remain
opaque. TCP/TLS copy preserves independent half-close, joins cancellation/error
workers and uses the existing disclosed two-minute idle bound.

The Windows gateway mux accepts only loopback TCP, bounds total connections to
128 and each protocol queue to 32, and routes verified mode streams to private
HTTP or nested application-TLS listeners. Shutdown closes raw transports first,
then joins TLS setup/queue workers, avoiding serialized close-notify delays from
stalled peers. Concurrent Close callers wait for completed cleanup. It does not
parse application HTTP or manage engine/workload lifecycle.

`localnet.ServiceHost` connects that mux to the actual route controller and
application authority, with no administrative HTTP endpoint. It owns the supplied
namespace backend after startup acceptance, closes it on startup failure, and
supervises all three server loops. Gateway/server failure stops ingress, retires
route streams and cleans only owned helpers. HTTP diagnostics stay in a bounded
in-memory log. It is not tied to a renderer context. Permanent discovery/renewal,
management IPC, the actual relay container and native UI are still to wire.

Windows headless tests pass verified mode negotiation, different-key reuse of the
same loopback port, wrong root/SNI/ALPN, expiry, malformed/fragmented framing,
coalesced binary payload, TCP/TLS half-close, idle/cancel joining, nested verified
application HTTPS, protocol separation, queued/unhandshaken shutdown and wildcard
bind refusal. Route-host tests pass redirect/HTTPS proxying, startup/fatal failure
cleanup and cancellation. A combined fixture loads a DPAPI-protected application
authority, crosses the authenticated gateway and verified application TLS, then
uses the real namespace manager/Moby SDK's framed exec stream to a controlled
Docker API fixture, and finally settles the exact helper. These are headless
contract tests, not a Linux relay execution, Windows/WSL host-gateway proof,
browser/OS trust result or installed service qualification.

## Namespace-stream cancellation and current live evidence

Closing an exec attachment does not by itself prove immediate guest-process
termination. The current idle guard bounds a detached dial to two minutes, while
Windows-side connection capacity can be released earlier. Repeated cancellation
therefore needs stronger guest ownership/accounting before activation; the idle
guard alone is not a proof that guest process/memory counts stay within budget.
The implemented per-stream cancellation/revocation retains admission until its
acknowledgement, or poisons that exact helper on uncertainty. Never signal a target process, use a bare
reusable PID, or restart the workload/engine to collect these helper processes.
The idle timeout remains a fallback, not the cancellation admission mechanism.

The selected fix is a private helper-local Unix control socket and one random
128-bit nonce per exec. A dial must register before opening target TCP. Revocation
either waits for the registered control connection to close or records an unseen
nonce before acknowledging, so a delayed start cannot connect later. Retain a
bounded set of 4096 seen/revoked nonces for the helper lifetime; exhaustion refuses
new admission and is reported through renewal for exact helper replacement.
No reusable PID is signalled. Loss/revocation of the control channel exits only
the diagnostic dial process. Windows closes data first, then keeps its admission
slot until acknowledgement; uncertain cancellation marks that helper unavailable
for new dials and renewal, allowing only its exact owned cleanup/recovery.

ABI 3 readiness and renewal require an attached command acknowledgement and
completed exec, so a detached successful start is not mistaken for control health.
Windows tests exercise failed acknowledgements, poisoned-helper refusal and lost
attachment cleanup without replay. Ubuntu executes the stream-control, gateway
and service-tools tests successfully.

Live proof `artifacts/qualification/orbwin-p2-stream-1792901702210273600/`
contains flushed image receipts, relay intent/accepted ID, public test root and
stream/relay results. It verifies 32 sequential cancellations and eight batches
of four simultaneous idle streams. Each check found zero remaining exec IDs, a
running non-OOM helper and an unchanged target start time. Both diagnostic and
relay fixtures use the 64 MiB/128 PID limits; this small proof does not establish
long-run memory bounds or maximum relay throughput.

The actual static Linux relay crosses `host.docker.internal` into the pinned
Windows TLS gateway, then the application TLS endpoint and target namespace.
Windows receives the escaped-path-preserving HTTP redirect and verifies HTTPS for
both reserved name families. Ubuntu curl and a separate process in the owned
target fixture verify application HTTPS using only the explicit public test root.
The container client uses the exact fixture relay address; this is not automatic
DNS/alias qualification. Ephemeral host ports avoid claiming product 80/443
availability. Test clients send the ordinary service Host header while connecting
through those ephemeral ports; production rejects arbitrary Host ports.

The proof removes exact fixture containers and its fixture image, and preserves
the tools image/import receipt for audit. Earlier attempts retain their records,
including a qualification Host-header error (ephemeral transport port incorrectly
included in the service Host); the final proof corrects the client fixture rather
than relaxing production hostname/port validation. System/browser/WSL root stores,
network aliases, permanent relay recovery and installed acceptance remain pending.

## Managed relay ownership contract

One exclusive local-services owner controls the relay, namespace helpers and
authority. A flushed immutable relay intent precedes Docker create; its accepted
full container ID must be flushed before start. Unknown acceptance is resolved
only through the exact unique name, image, labels and full security/configuration
profile. Missing unknown creates stay pending; known absence can settle. A
partial or foreign record prevents setup and remains available for review.

The relay has no workload filesystem/PID namespace, daemon socket or private key.
It runs the trusted image as UID/GID 65532, read-only root, no capabilities,
no-new-privileges, 64 KiB private tmpfs, 64 MiB memory, 128 PIDs and half a CPU.
Only the reviewed IPv4 loopback HTTP/HTTPS pair may be published by product setup
(80/443 by default, with explicit alternate integration ports). A conflict
fails the exact start; it never evicts another listener. Only explicitly selected
local user-defined bridge networks receive reserved `.orbwin.test` aliases.
The default bridge remains the host-gateway path. The owned relay binds the
reviewed ports on the guest interfaces so rdd's tap-address forwarding can reach
it; rdd's Windows listeners remain loopback, with any WSL-created IPv6 listener
also checked for loopback scope. This is not a rewrite of user workload bindings.
Guest-loopback-only relay bindings proved unreliable in repeated Windows tests;
BL-018 preserves the failures and the remaining broader publishing audit. No
workload network is changed.
Network IDs and alias sets are immutable within a relay generation; a changed
selection retires the owned generation before replacement and exposes that
connection interruption. No implicit disconnect/reconnect of a workload is allowed.

Renewal requires an attached exact acknowledgement and completed exec. Controller
loss expires the guest's monotonic lease. Shutdown retires routes/ingress before
exact relay/helper cleanup; unresolved cleanup retains its receipt. The common
ledger owner must outlive relay cleanup, including route-host fatal failure.
Permanent runtime wiring must enforce that order; a closed renderer is never the
runtime owner. These are implementation requirements, not completed acceptance.

### Newly observed ingress and address-family conflicts

Live inspection on 2026-10-24 finds the owned optional cluster's bundled Traefik
3.6.10 serving 80/443, with rdd's Windows IPv4 forwards already bound. Rancher's
separate host-switch and WSL relay also have IPv6 listeners. Native Windows DNS
returns `::1` before `127.0.0.1` for a reserved `.localhost` name, and ordinary
curl selects `::1`. Thus the earlier explicit-IPv4 proofs do not qualify ordinary
browser name resolution under simultaneous engines. A Windows-only alternate
loopback IP is free, but the shared WSL network's wildcard 80/443 prevents simply
repeating that bind in Ubuntu. No listener, ingress controller or cluster was
stopped to obtain these observations.

The standalone relay now preflights its fixed ports before committing an intent;
a known conflict creates neither a receipt nor a container. A live proof covers
that refusal. Recovery contract tests cover ordered intent/ID flush, failed start,
unknown/late acceptance without replay, foreign profile refusal, retained removal
failure, and exact successful settlement. The relay manager borrows the common
owner. The composed backend now settles the relay before namespace cleanup releases
that owner, including fatal gateway failure and caller cancellation. Its bounded
background cleanup survives a caller deadline; completion is published only when
the cleanup worker has finished. Permanent runtime wiring remains pending.

Evaluate reuse of the already-running owned Traefik for its occupied listeners:
HTTP routes only a unique qualification Host, and a TCP route passes that exact
SNI's TLS to the relay, leaving Windows as certificate/key owner. Create only a
unique disposable namespace, selector-free Service/EndpointSlice and two CRDs;
record returned UIDs and delete only that UID-pinned namespace. Never install or
reconfigure an ingress controller, change unrelated rules, or route through
another engine. This is a qualification experiment, not an accepted production
dependency. Traefik's documented [TLS passthrough](https://doc.traefik.io/traefik/v3.6/reference/routing-configuration/kubernetes/crd/tcp/ingressroutetcp/)
and [exact SNI routing](https://doc.traefik.io/traefik/v3.6/reference/routing-configuration/tcp/routing/rules-and-priority/)
support this route shape. Browser/OS name selection still needs an explicit
per-client solution; `.localhost` must not silently reach another engine.

The Windows-only ingress experiment succeeds using the node's published relay
ports, with exact SNI passthrough and unchanged Traefik deployment. Direct pod-to-
Docker-bridge access times out. Ubuntu's `127.0.0.1:443` reaches a different TLS
server even when Windows reaches the owned ingress; it is not a qualified shared
Windows/WSL endpoint. A listener bound to Windows' verified WSL-facing address
also times out from Ubuntu without further network/firewall setup. These failed
routes remain evidence, not accepted production dependencies. Disposable namespaces
were UID-checked and removed; no ingress controller, DNS, trust or firewall policy
was changed.

The supported routing design must therefore include a reviewed **integration port
pair**. Prefer 80/443 when available; allow a saved alternate pair when occupied.
Every selected service still receives its predictable name and a generated Open
link, independent of its application/container/published ports. Users do not have
to track individual workload ports. This extends conflict handling without
discarding standard-port support. Alternate pairs must pass Host/SNI validation,
redirect to the reviewed HTTPS port, and work through ordinary client hostname
resolution. Container clients can continue using relay-local 80/443. Changing the
pair is an explicit integration generation change, with old routes/listeners
retired before replacement; no implicit fallback after setup.

Windows curl with a per-command CA file reports Schannel's unknown revocation
status for the earlier version 1 local chain. That was not a passing Windows default-client
trust result. Keep hostname/chain verification enabled and qualify OS/browser
trust separately; do not soften that failure merely to obtain a green proof.

### Version 2 authority revocation distribution contract

Before product trust setup, qualify a versioned CA profile that supplies signed
local CRLs to clients enforcing revocation checks. The intermediate gains CRL
signing usage while retaining the critical DNS/server-auth/path-length constraints.
Its distribution point references a root-signed empty CRL, minted before the root
private key is discarded and valid until the intermediate expires. Leaves reference
a short-lived intermediate-signed CRL refreshed by the Windows authority. These
lists report no known revoked certificates; compromise/removal recovery remains
explicit root rotation and removal from reviewed client stores. Individual leaf
revocation is not an implemented feature or a claimed guarantee.

CRLs contain only public signed metadata. Store the initial root CRL and its
reviewed HTTP integration port inside the OS-protected versioned bundle. Their
URLs use literal loopback and that port, with the exact root fingerprint in the
path. Serve only GET/HEAD for the two exact public DER resources, before workload
routing; never perform filesystem dispatch or administrative actions. Verify CRL
issuer/signature, times, profile and namespace on load; bound size and refresh
concurrency. Root keys must still never become a field or persisted value.

The bound HTTP port is immutable for that authority; changing it requires an
explicit reviewed authority/trust rotation. Older bundles must be refused and
preserved rather than silently rewritten. Container clients requiring online CRL
retrieval need a separately qualified distribution story because their literal
loopback is a different host; ordinary verified-chain clients and Windows/WSL
revocation-enforcing clients must be reported separately. This remains a design
contract until the native client actually verifies it without relaxed checks.

Profile 2 is now implemented and the native Schannel client verifies it without
relaxing revocation checks (`orbwin-p2-stream-1792906202883657300/client-proof.json`).
The root CRL and bound HTTP port are serialized public metadata in the protected
bundle; the root signing key remains absent. Signer CRLs are cached under a mutex,
expire within 24 hours and refresh after 12 hours; a backward clock move cannot
return a cache entry whose ThisUpdate is in the future. Tampered/legacy bundles,
wrong issuer/signature/validity and unexpected CRL routes/methods are refused.
The intermediate also excludes every IPv4/IPv6 identity. Compromised-signer tests
reject public/loopback IP certificates, client-auth certificates, unrelated DNS
names and subordinate CAs. These tests do not claim all client policies are covered.

The current live journey additionally verifies ordinary Windows Go and Ubuntu
curl hostname/SNI/Host handling on 18080/18443, correct external redirects, and
container-client standard-port behavior. It uses the production relay manager,
including exact profile checks and settled cleanup, after retiring the manual
fixture. Windows default 80/443 conflicts are refused before create intent or
container mutation. The actual root is absent from both Windows trusted-root
stores; per-command/client roots were used. Browser/OS installation and removal,
container DNS aliases and persistent client configuration remain acceptance gates.

ABI 5 relay readiness waits for both listeners and performs the authenticated
private gateway handshake before acknowledgement. Lease existence alone did not
prove traffic readiness. A bounded wait after exact removal observes asynchronous
Windows publication release before reusing a pair; failure retains the known-ID
receipt. First gateway setup errors are logged once per relay listener with a
512-byte message bound, while transport bytes remain unlogged. Root/client/process
loss still requires permanent runtime scheduling and installed fault qualification.

Windows race-detector tests pass for localnet, gateway and streamcontrol with the
workspace's existing GCC 16.2.0 toolchain. Real pre-start recovery proves accepted-
ID and unknown-acceptance stopped helpers are removed without starting them;
`orbwin-p2-relay-recovery-1792906918902489700/proof.json` records exact IDs and
the explicit lease replacement used. It does not replace the broader process-loss,
resource-pressure, expiry/rotation, UI-independent lifetime or installed gates.

## Permanent process configuration and generation ownership

The independent process owns one exclusive ledger for its whole lifetime. An
enabled host generation borrows that owner; stopping a generation must not open
a lease gap before settings, authority or the replacement generation is ready.
Existing standalone namespace-manager constructors keep their transfer-of-owner
contract; the process uses an explicit borrowed-owner constructor.

Persist only bounded non-secret preferences, reviewed ports/network IDs, enabled
intent and the expected root fingerprint. The fixed settings file has a version,
daemon identity and monotonic revision. Updates compare the caller's revision,
validate the whole replacement, flush a same-directory draft and atomically
replace the prior file. Windows replacement requests write-through; interrupted
drafts do not override a valid committed record. Damaged, linked, foreign or
unknown-format records are preserved and refused. An enabled intent requires a
root fingerprint; it never authorizes generating or trusting a replacement root.

The process serializes configuration transitions and reports desired settings
separately from observed running/degraded state. Renderer loss never cancels the
process. Daemon/inventory loss retires routes and surfaces the bounded error;
the process never starts/restarts rdd or workloads. IPC must be current-user-only,
bounded, versioned and typed; no arbitrary command, endpoint, file or URL dispatch.

### Process and settings milestone, 2026-10-24 Pacific

The Go SDK executable now provides the permanent process and a short-lived IPC
client. The Windows pipe has an explicit current-user SID DACL and rejects remote
clients through go-winio. Frames, connections, request deadlines and response
sizes are bounded. Only a missing pipe permits the thin Rust adapter to launch
the verified executable; other IPC failures do not authorize another process.
The renderer owns neither its process lifetime nor the namespace ledger. Binary,
source and guest-tool identities are checked before dispatching UI operations.
Another component build is refused; update/restart handling remains unfinished.

The actor commits desired settings independently of renderer cancellation, owns
one ledger across host generations, and bounds automatic retries. Disabled setup
does not generate a CA or allocate a relay. Enabling validates the existing
protected authority and its bound HTTP port before committing enabled intent.
Disabling remains available when signing state is unavailable. A successful empty
inventory has its own read timestamp; disabled ticks and failed discovery retain
the last successful read rather than relabeling it as an unread inventory.

The Network settings panel now exposes discovery, saved ports, explicit authority
creation/public export, enable/disable/retry and per-service name/HTTP-port/disable
choices. Choices use the returned full container or coherent Compose selector;
Compose choices survive recreation while standalone choices remain exact-ID.
Background reads do not clear mutation errors or replace newer state. These are
source/build milestones: rendered/native interaction, network-selection controls,
trust installation/removal/rotation and the installed journey are still open.

`scripts/qualify-local-service-process.ps1` exercises a unique disposable home,
actual hidden processes and the private pipe. The first updated run is
`artifacts/qualification/service-process-639285069935476000/proof.json`: 40,452
bytes of stdin JSON, 360 explicit disabled preferences, verified component
identities, clean stop/restart, retained revision/ports/preferences, and no CA
generation. It does not enable routes or prove live daemon reachability. This
script does not change trust stores or request elevation. The installed Phase 1
home remains separate. Scope gates still require enabled-process traffic,
process-loss/recovery/resource tests, current-user ACL adversarial checks and
installed update/uninstall/data-preservation evidence.

After adding enable-time authority/HTTP-port validation, the rebuilt executable
repeats this proof in `service-process-639285071388127213/proof.json` with binary
SHA-256 `3e537853e17498dc3852ffc12287ba7cedc48c27cf889dc1136dcd4336a87ea4`.
Full Go tests/vet, scoped Windows race checks, frontend build and Rust check pass.

### Enabled permanent-process and selected-network proof

`TestOwnedRuntimeProcessTrafficAndCrashRecovery` is an opt-in Windows integration
test against the fixed verified OrbWin daemon. It imports a uniquely labeled
static fixture, creates a unique user-defined bridge and private state home,
explicitly disables unrelated inventory selections and chooses one exact target.
It starts the real hidden SDK process, creates a protected test authority without
installing trust, enables the reviewed alternate pair and verifies ordinary-DNS
Windows HTTPS with a per-client root. A separate read-only disposable container
resolves the relay's `.orbwin.test` alias and verifies HTTPS with the public root
supplied to that invocation. Neither client disables certificate verification.

The test kills only the exact disposable Windows service process, starts it
again, verifies unchanged enabled revision/root and both HTTPS clients, and checks
that the old relay/namespace helper IDs are absent and exactly two replacements
exist. Disabling, stopping and reopening the owner leaves no helper containers,
namespace/relay receipts or occupied publication ports. Target start time remains
unchanged. Exact fixture containers, network and imported images are removed.
The disposable authority record remains in the evidence folder, OS-protected;
it is not installed into any trust store.

The expanded live pass is
`artifacts/qualification/orbwin-p2-runtime-1792910593975080200/process-proof.json`.
This proves process-loss recovery and selected-network aliases for these Windows
Go/Linux Go clients; it does not prove installed/browser trust, WSL trust stores,
resource-pressure behavior, login start or update/uninstall. The preceding
Windows-only process pass is `orbwin-p2-runtime-1792910476175334800`.
After the expanded pass, all three daemon IDs and global Docker/Kubernetes config
hashes match the recorded baseline. No engine restart or Windows elevation was
used. The source UI now includes a reviewed exact-ID container-client network
selector, with the cross-network routing scope and relay interruption explained;
rendered/native interaction qualification remains open.

### Interrupted authority setup and uncertain settings acknowledgement

An authority can be published before its settings fingerprint is committed.
Setup now exposes the saved public certificate after an incomplete attempt and
offers a distinct `adopt-authority` operation. The confirmation captures the
reviewed fingerprint and settings revision. The actor requires disabled intent,
an unbound authority selection, that revision, the exact fingerprint, valid
current-user protected signing state and the authority's original HTTP port.
It commits only the fingerprint selection; it neither generates/replaces the
saved key nor installs trust. Wrong fingerprint, incompatible port, stale
revision and an already-bound selection are refused. A contract test models the
publication/commit gap across runtime reconstruction and verifies that protected
authority bytes stay identical. This is not a claim of native crash injection at
every filesystem instruction or complete rotation support.

After a settings write error, the actor rereads the committed record before
assuming intent stayed unchanged. If a later committed revision is observable,
it publishes/applies that revision while retaining the error; it does not replay
the write. The contract test models an already-committed replacement with an
unacknowledged actor revision. Broader disk-error, unreadable-reload, pressure and
partial trust/rotation tests remain required before acceptance.

The rebuilt binary passes the expanded real-process journey again in
`orbwin-p2-runtime-1792911075778212800/process-proof.json` (binary SHA-256
`d9aa832770c1608245d2627f308285f62a1ef0c919259780cb37b6f1f15c8e90`).
Full Go tests/vet, Windows localnet/gateway/streamcontrol race tests, frontend
build, all 46 repository JavaScript tests and Rust check pass. The actual
process proof includes exact process loss/recovery, both verified clients,
retired old helpers and settled cleanup. No proof service process remains.

### Windows trust transaction contract

Trust setup targets only the physical CurrentUser Root store. Use the system
registry provider, not the logical Root collection that also exposes machine and
policy siblings. Microsoft documents these distinct scopes in
[CertOpenStore](https://learn.microsoft.com/en-us/windows/win32/api/wincrypt/nf-wincrypt-certopenstore).
Every request must review the exact installation/root and current settings
revision. Installation validates the protected authority; removal must remain
possible using its recorded public root when the signer has expired or its
private state is unavailable. No automatic install, machine-store write, policy
override, unrelated-root cleanup or trust-store change on ordinary startup.

Before adding a root absent from the selected store, flush an ownership intent
bound to the daemon, installation, Windows user SID, fixed store scope and exact
DER/SHA-256. A preexisting root without that intent is reported as unmanaged;
do not claim ownership or remove it. Persist installed observation only after
rechecking exact presence. Denial and lost acknowledgement retain pending intent
for explicit retry; they do not report successful setup or authorize background
trust changes. Removal flushes intent before deleting the exact recorded DER,
then settles only observed absence. A completed removal ends ownership; a later
external re-addition is unmanaged. Settings/authority/key state and other roots
remain unchanged throughout. Mutation is actor-serialized and records use bounded
strict JSON, same-directory flushed drafts and atomic replacement. Unknown,
foreign, partial or changed records are preserved and refused.

The adapter must use add-new semantics, never replace a matching root, and obey
native context ownership (delete consumes its certificate context even on error).
See [CertAddCertificateContextToStore](https://learn.microsoft.com/en-us/windows/win32/api/wincrypt/nf-wincrypt-certaddcertificatecontexttostore)
and [CertDeleteCertificateFromStore](https://learn.microsoft.com/en-us/windows/win32/api/wincrypt/nf-wincrypt-certdeletecertificatefromstore).
Bound enumeration and compare full DER, not a display name or SHA-1 thumbprint.
Store presence is distinct from effective client trust: browser policies/custom
stores still require explicit qualification. Native API calls are synchronous;
caller loss cannot safely undo an accepted mutation, so an uncertain result must
be resolved by an explicit reread rather than replay. Current work may test the
native adapter against an in-memory certificate store only. Real Root-store
installation/removal and any resulting consent prompt remain attended gates.

### Trust and settings recovery source milestone, 2026-10-25 Pacific

The transaction, physical-store adapter and typed IPC/Rust/React controls are now
implemented. Ordinary process/UI startup never opens a Windows certificate store;
trust reads and writes are explicit actions. The confirmation freezes the root
fingerprint and settings revision. Installation revalidates protected signing
state and selected root before opening a writable store. Removal uses the owned
receipt/public certificate and stays available if private authority state is lost.
The UI describes last observed physical-store presence rather than asserting
effective browser trust. There is no machine-store fallback or auto-elevation.

Contract tests cover preexisting/unrelated preservation, denied addition/removal,
unknown accepted additions/removals, failed observations, partial/foreign-user
receipts, receipt replacement before/after commit, explicit recovery without
replaying an accepted mutation, and removal after private state loss. Native
adapter tests open only `CERT_STORE_PROV_MEMORY`: same-subject different roots,
add-new refusal, 32 exact add/remove cycles, read-only/closed behavior and the
[diagnostic context-leak check](https://learn.microsoft.com/en-us/windows/win32/api/wincrypt/nf-wincrypt-certclosestore)
pass. No actual Root-store installation/removal was invoked.

Settings now retire only local ingress when an unsuccessful write cannot be
resolved to a readable, non-regressed record. Retry cannot recreate ingress
until a valid record is restored; the last known desired revision/root is kept
visible. Corrupt, missing and regressed-file tests pass across repeated runs,
including restoration without workload restart. These tests exposed equal
Windows wall-clock timestamps across different transitions. Snapshot ordering now
uses a random process instance plus monotonic sequence; the frontend rejects
older sequences and late retired-process replies independently of clock changes.
Three frontend regression tests cover these ordering boundaries and malformed
metadata. Wall time remains display metadata only.

Evidence: `artifacts/qualification/p2-windows-trust-source-20261025.json` and its
raw unit JSONL record 28 passing test events and no failures. Full Go tests/vet,
scoped Windows race tests, 49 JavaScript tests, frontend build and Rust check pass.
The current binary repeats the enabled process/verified Windows and container
HTTPS/crash-recovery/settled-cleanup journey in
`orbwin-p2-runtime-1792911942516742800/process-proof.json`. Its generated public
root is absent from both CurrentUser and LocalMachine Root readbacks. All three
daemon IDs and the global Docker/Kubernetes configuration hashes still match the
baseline. This does not qualify the new trust controls against the real Root
store, browser/enterprise policy, native dialog behavior, rotation or installed
update/uninstall; those remain explicit gates, alongside selected WSL trust and
the broader Phase 2 requirements.

### Authority rotation contract

Rotation is explicit and starts only from disabled, settled local ingress with
the reviewed settings revision/old fingerprint. The prior public authority must
be readable and match that fingerprint; an expired or unusable private signer
does not prevent replacement. Require the old root to be absent from the physical
CurrentUser store and any owned Windows trust transaction to be settled as removed.
Do not remove roots automatically. Other clients' old trust must be removed by
their reviewed setup/removal paths; rotation never claims global revocation.

Generate a new protected authority for the reviewed port pair. Flush an exclusive
new protected stage and an exact old-state recovery copy, then publish a bounded
immutable journal containing their digests, public fingerprints/certificates and
the complete old/target non-secret settings. Preserve the old public certificate
for manual client cleanup. No root store is mutated. Atomically replace the
authority file before committing target settings. Resume accepts only the exact
old or new authority bytes and exact old or target settings; unexpected state is
preserved and refused. Process startup must expose a pending journal and suppress
ingress until explicit resume. Never generate another root to resolve an
uncertain accepted rotation.

Once the new file/settings are verified committed, delete only the recorded
protected recovery/stage copies by exact digest and settle the journal last.
This retires the old signer from product operation; it is not a secure-erasure
claim. Keep the old public certificate and residual error evidence. Before any
switch, explicit cancellation may discard only these exact prepared artifacts;
after either committed switch it must refuse rollback and require resume.
New trust remains a separate fingerprint review. Rotation during active access,
stale review, mismatched/tampered files, unknown settings, conflicting recovery
files, trust still present and interrupted switch/cleanup require tests.

The rotation journal, protected staging/recovery copies and explicit begin/resume/
cancel operations are now implemented and wired through private IPC, Rust and the
settings panel. Startup exposes retained or unreadable rotation state and keeps
ingress off; conflicting configuration/authority/trust-install actions refuse.
The UI captures revision/fingerprints/ports for review, disables cancellation once
a switch is observed, preserves current port choices after a successful rotation
and provides the old public certificate during pending recovery. Native system
trust is only observed read-only for this operation. New trust still requires its
separate install action. Rotation presently requires a readable matching old
public authority envelope; arbitrary destruction of all public/recovery metadata
is not silently repaired by minting another authority.

Contract tests cover expired old signing state, retained preferences/networks,
old Windows trust still present or unsettled, stale/incorrect review, active
integration, invalid port pairs, prepared-file/active-file/settings/journal
tampering, cancellation before switch, no rollback after switch, and reopening
the owner after an unknown accepted authority replacement. A real Windows file
handle denying deletion causes cleanup to fail after target settings commit;
resuming settles the same target revision without replaying the switch. Pending
runtime reconstruction creates no helpers or trust writes until explicit resume.
A related shutdown test verifies queued/new mutations cannot run once stopping
begins; an already-accepted operation can still have an uncertain result requiring
reread. Atomic settings and relay readers gained locked internal entry points so
rotation can serialize the whole transaction without recursive locking.

`artifacts/qualification/p2-authority-rotation-source-20261025.json` binds this
milestone to binary/source digests and the raw unit log (20 passing test events,
zero failures). The expanded real-process pass is
`orbwin-p2-runtime-1792913827449999600/process-proof.json`. After exact process
loss/recovery it disables ingress, rotates the root and port pair from
18080/18443 to 28080/28443, and re-enables the same workload selection. Windows Go,
Windows Schannel curl, Ubuntu curl and the selected-network Linux Go container
all verify the new chain with per-client roots. The old-root-only client rejects
that chain. Curl config is disabled per invocation and no insecure/relaxed
revocation flags are used. Actual Windows IPv4/IPv6 listener captures before and
after rotation contain only loopback addresses, with both IPv4 ports present.
The target incarnation stays unchanged; final helper/receipt/port cleanup passes.
Both generated roots remain absent from CurrentUser and LocalMachine Root stores.
Three daemon identities and global Docker/Kubernetes config hashes match baseline;
no qualification service process or workload is left running.

Full Go tests/vet, scoped Windows race tests, 49 JavaScript tests, frontend build
and Rust check pass. These are source/contract/live-adapter results. Native UI
interaction, actual trust-store consent/removal/browser behavior, selected WSL
trust transactions, installed upgrade/uninstall and the rest of Phase 2 remain
acceptance gates. Public-only archive retention is not proof that every external
client has removed its old root.

### Selected WSL trust contract

Keep this separate from Windows trust. Enumerate only the current Windows user's
registered ordinary WSL 2 distributions; never modify the OrbWin guest or another
engine's distributions. Explicit actions may start the selected distro and run a
narrow Linux helper as Linux root, with no Windows elevation or shell/profile
changes. Require GUID-addressed invocation (`wsl --distribution-id`), verified in
this host's installed help and with a read-only Ubuntu invocation, rather than
falling back to a mutable distribution name. Unsupported WSL versions must refuse
the feature rather than target by name. A registration replacement has a new GUID
and cannot silently inherit an old trust operation.

Initial native qualification targets Ubuntu 24.04 and its installed standard
`update-ca-certificates`; other trust-store layouts need separate adapters and
evidence. The helper receives only a reviewed public root, installation, Windows
user identity and registration GUID, never signing keys or arbitrary paths or
commands. Its root-owned receipt also binds the distro machine identity. Only
that exact certificate file/receipt may be changed. Preexisting/foreign/changed
state is preserved; denied/partial/unknown updates report observed CA-file and
main-bundle state without claiming success. Serialize helper operations with a
Linux lock, persist intent before file/update changes, and use explicit retry for
unconfirmed operations. Preserve receipts and public metadata needed to remove
trust after the signing state expires or disappears.

Use the distro's updater to refresh its main certificate bundle. Do not replace
it with a custom trust engine or use `--fresh` to purge unrelated symlinks. Remove
only generated links whose exact target belongs to the owned certificate. Hooks
and application-specific stores are not implied by main-bundle presence: the
installed updater can report hook errors while returning success. Surface that
distinction and never promise all Java/NSS/custom stores changed. Consent must
explain the selected distro-wide root effect and OS updater/hooks. No automatic
changes to every distro, image, workload, global shell or default Docker context.

The [Debian updater interface](https://manpages.debian.org/unstable/ca-certificates/update-ca-certificates.8.en.html)
and the actual Ubuntu 24.04 script support alternate cert/config/output/hook
directories. Native tests must redirect every such path, use empty private hooks,
and compare system bundle/updater hashes before and after. Real selected-distro
system trust installation/removal remains an attended gate. GUID invocation does
not expand the existing security boundary: malicious root in another distro owned
by this Windows user is not isolated from that user's Windows data.

The user requested unattended progress without repeated administrator approvals.
Continue source/build and non-elevated qualification; defer installer/WSL platform
changes and certificate trust prompts to an attended batch. Do not report these
deferred gates as passed or create automatic elevation/trust acceptance.

#### Selected-distro helper and host transport foundation (2026-10-25)

`internal/wsltrust` and `trust-helper` now implement the Linux side of this
contract. Changes require a reviewed machine ID as well as the registration GUID,
Windows SID, public-root fingerprint and exact guest receipt sequence. Check the
machine before creating even the state directory, and again under the operation
lock. An identity failure is an error with available public observations; it is
never proof of absence. Receipt writes use flushed exclusive random drafts,
rename and directory sync; unknown drafts remain for review. The updater inherits
the operation lock and has a bounded execution deadline. Client/process loss can
leave an accepted updater running, so a subsequent caller must observe state and
obtain that lock; a Windows timeout never authorizes automatic mutation replay.
Removing an exact absent root also persists a removal tombstone when no guest
receipt exists yet. This settles host intent whose first install was delayed
before acquiring the guest lock: a subsequently arriving sequence-zero install
then refuses. A present unmanaged certificate is still preserved. Never delete
these tombstones as routine cleanup or infer that a missing response means the
guest invocation cannot arrive later.

The Windows transport lists only current-user registry registrations, without
invoking WSL. It excludes WSL 1 and the existing protected engine-name prefixes,
and rereads the registration list immediately before GUID-addressed launch.
Capability detection handles both UTF-8 and UTF-16 help, including the observed
`0xffffffff` help exit from this installed Microsoft `wsl.exe`; it still requires
the exact `--distribution-id` option. There is no fallback to a name or default
distro. A hidden fixed-system-directory executable receives separate arguments.

The host owns an independently hashed copy of the bounded Linux amd64 ELF. Its
fixed bootstrap streams exactly those bytes into a private root-owned temporary
directory, hashes that guest copy before execution, and leaves only the remaining
public JSON on stdin. This avoids assuming `/mnt/c` or a particular automount
layout. Normal exit removes only the exact temporary helper and directory; abrupt
guest/process loss may retain a public binary, with no private key or request
file. Output is bounded, unknown protocol fields are rejected, current-user and
response identities are checked, and transport failure does not replay a call.

The native Windows status proof uses the production Linux helper, then corrupts
the host copy to verify refusal before execution. System bundle/updater hashes
and temporary-helper directory listings are identical before and after. The
separate unprivileged Ubuntu test runs its actual updater against wholly private
config, certificate, output and hook paths, including installation, OpenSSL
verification and removal while preserving another fixture root. Fake-store tests
cover preexisting certificates, changed files/owners, unknown updater acceptance,
hook warnings, exact-link cleanup, identity/sequence changes and lock contention.
Those initial proofs covered the foundation only. The next source milestone
integrates host ownership, IPC, UI and rotation guards as described below;
installed and actual system-trust qualification remain open.
No real system trust mutation has been performed.

#### Host ownership, service IPC and reviewed controls (2026-10-25)

`localnet/wsl_trust.go` now flushes a Windows-owned public receipt before guest
mutation, binding daemon, installation, Windows SID, GUID, machine ID, root
bytes/fingerprint and last confirmed guest sequence. The service actor and ledger
mutex serialize changes, including rotation; snapshot reads remain independent.
Both the host and guest receipt sequences must match the explicit review. A
guest receipt without matching active host intent cannot become host ownership.
Preexisting roots and roots re-added after completed removal remain unmanaged.
Status never settles intent or replays an updater. Explicit installation can
acknowledge already observed guest completion without dispatching it again.
Removal uses retained public bytes even when signing state is lost or expired.

Host receipts use bounded, validated regular files, flushed private drafts and
the existing Windows write-through replacement. An uncertain replacement is
reread without dispatching a guest mutation; failed guest transport retains
intent. Budgets are 128 retained receipts, 64 retained drafts and 4096 directory
entries. Unknown or damaged records are preserved and refuse the relevant
operation. Rotation validates all retained selected-WSL receipts and requires
completed removal without starting any distro. Pending rotation refuses new
trust installation, including direct ledger calls.

The production service accepts optional attested trust-helper arguments alongside
its service asset, retaining the older six-argument launch for existing isolated
qualification journeys. The desktop now supplies and verifies the trust helper's
ABI, eight source hashes and binary digest; snapshots bind that digest too. Typed
SID-only IPC adds distribution listing, status, install and removal. The service
uses a 145-second WSL-operation budget for two separately bounded 65-second helper
calls; its CLI allows five seconds for response delivery and the desktop ten
more. Other request budgets remain unchanged. These are bounds, not latency or
availability promises. Caller loss never proves a guest invocation stopped.

The UI lists registrations only on request, separately from the explicit action
that may start the selected distro. Changes require an observed identity and
exactly representable sequences. Confirmation freezes settings revision, root,
registration, machine and both receipt sequences, explains distro-wide Linux-root
scope and updater hooks, and distinguishes main-bundle state from custom stores.
Pending/unknown/partial/unmanaged and rotation states retain useful reads and
guard changes. A missing distribution cannot be targeted by a replacement name.

The isolated browser fixture at `tests/ui/wsl-trust/` has no backend calls. At
1280x720 it verifies rendered review fields, pending/preexisting/unknown/warning/
empty/rotation/unsafe-sequence controls, Escape, destructive Cancel-first focus,
Enter-on-Cancel and focus return. It exposed Tab leaving the shared confirmation
dialog's last button for the page body; explicit forward/reverse focus wrapping
now passes. This does not qualify native Windows scaling, other view sizes or the
complete network settings panel.

`TestOwnedServiceProcessSelectedWSLTrustReadOnly` qualifies the rebuilt SDK process,
real pipe, actor and native selected-GUID helper using a unique disposable home
and a fresh public root. Proof `orbwin-p2-wsl-trust-1792919423553389300/proof.json`
records matched component digests, observed absent guest trust, absent Windows
Root trust, no host trust receipt, unchanged system updater/bundle and global
Docker/Kubernetes config hashes, and clean owned-process shutdown. It creates no
engine objects. Controlled clients cover denial, unknown accepted results,
stale/foreign/regressed/tampered state and four host replacement boundaries.
Go tests/vet, scoped Windows race tests, frontend build, 49 JavaScript tests and
Rust check pass. Evidence: `p2-wsl-trust-integration-20261025.json` and linked logs.

That source milestone left missing-registration receipt recovery open; the next
section records its implementation and qualification. Actual system-store
consent/install/remove, custom/browser clients, native Windows UI and installed
update/uninstall preservation remain open. An unreachable distro is never proof
that its trust disappeared. These gates still prevent selected WSL trust or
Phase 2 from being accepted as complete.

#### Missing-registration receipt recovery (2026-10-25)

Host-only explicit retirement is now implemented, distinct from successful
certificate removal. Preserve the public receipt and use `registration-retiring` then
`registration-retired` states, guarded by reviewed owner/guest sequences and
machine/root identities. Read the raw current-user GUID key, including WSL 1 or
protected/renamed registrations; absence from the filtered ordinary-distro list
is insufficient. An access/read failure is unknown and must refuse retirement.
Persist retirement intent before the final absence check, retain uncertainty
after a failed acknowledgement, and recheck raw presence after the final write.
Retired records must be rechecked at every rotation begin/resume; a restored GUID
or registry error blocks rotation without starting any distro. Pending retirement
can still use explicit ordinary-distro trust removal if the registration returns.
Completed retirement never adopts restored trust. Explain that unregistered disks
or exports may still contain the root: retirement is not proof of trust removal
and does not delete registrations, disks, public records or certificate bytes.

The actor/typed IPC adds `wsl-trust-receipts` and
`retire-wsl-trust-receipt`. Receipt listing works without signing state and
returns bounded, stable-order public metadata plus raw registration presence:
present, absent or unknown. It never completes an intent. Native registry errors
remain unknown; damaged receipt files fail the read visibly. The UI has a
separate receipt panel, frozen retirement review, exact-number guards and
Cancel-first confirmation. A pending retirement can still finish reviewed trust
removal if an ordinary registration returns. A completed retirement never
authorizes removal/adoption of restored certificate bytes; returned raw GUIDs
require manual review and keep rotation blocked.

Tests cover present/filtered-out/legacy/damaged registrations, registry failures,
owner/machine/sequence changes, cancellation, two registration-return boundaries,
four replacement-acknowledgement boundaries, failed compensating writes and
rotation begin/resume rechecks. The native registry boundary uses a unique newly
created disposable HKCU test key and deletes only that owned key; it never changes
the actual Lxss tree. Actor/IPC tests prove a filtered-out registration is still
present and cannot be retired, and metadata operations do not call a guest.

The real SDK process proof
`orbwin-p2-wsl-retirement-1792921388335358000/proof.json` starts from an explicit
controlled precondition: the ledger API persists intent before a denied simulated
guest dispatch, using a cryptographically generated GUID confirmed never
registered. The production SDK then lists/retire-checks that intent through the
real current-user registry and named pipe and rotates its private test authority.
It retains public history, keeps guest sequence zero distinct from certificate
removal, verifies old/new Windows-root absence and cleanly stops. It neither
unregisters a real distro nor claims that an actual distro certificate was removed.
The rebuilt read-only helper journey also passes at
`orbwin-p2-wsl-trust-1792921389156663300/proof.json`, including metadata listing
before authority creation. System bundle/updater and global client config hashes
remain unchanged, and all three recorded engine identities match.

The isolated browser fixture now includes both application stylesheets and
checks nine recovery states, disabled-action reasons, exact submitted receipt
identity/sequences, cancellation and focus wrapping/return at 1280x720. It has no
backend connection. Native Windows sizing/scaling remains a separate gate.
Evidence: `p2-wsl-trust-retirement-20261025.json`, linked logs, UI JSON and screenshot.
Full Go tests/vet, scoped Windows race tests, frontend build, 49 JavaScript tests
and Rust check pass. Real system trust installation/removal and installed
preservation remain unqualified; no actual certificate store or WSL registration
was changed by this work.
