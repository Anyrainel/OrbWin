# Reviewed per-port LAN exposure

Design and bounded planning implementation, 2026-10-25. P2-LAN remains required
and incomplete. Activation is deliberately unavailable until the native boundary
below is implemented and qualified; a plan is not consent or an active exposure.

## Verified current boundary

`upstream.lock.json` pins Rancher Desktop 2 commit
`af6511ad4e065e24e7258f575a0fc090c10cdbda`. The applied patch
`patches/rdd/0008-windows-port-publication.patch` and current upstream checkout
show that Windows publications are loopback only:

- `rdd/pkg/socketbridge/docker_ports_windows.go::hostBinding` accepts empty,
  wildcard or loopback Docker HostIp inputs but the Windows publication owner
  binds/checks `127.0.0.1`, not the requested LAN address.
- `rdd/pkg/hostswitch/hostswitch_windows.go` compares exact loopback forwards
  and holds an ephemeral publication generation inside the existing owner.
- The publication status API returns container ID, host port, protocol and a
  boolean. It does **not** expose a complete immutable target/StartedAt/generation
  tuple suitable for LAN consent.
- `localnet/host.go` binds the authenticated service gateway to
  `127.0.0.1:0`. Its relay's guest-side wildcard binding is required for the
  qualified guest tap path; changing that Docker field cannot authorize Windows
  LAN ingress. No Windows firewall/elevation transaction exists in the current
  session SDK or desktop adapter.

These observations come from current source, not a live LAN probe. Existing
loopback publications and service relay bindings must remain unchanged. No
firewall rule, network profile, listener or certificate trust was changed here.

## Selected ownership and transport direction

The existing persistent Go SDK/local-service owner is the natural unprivileged
home for a separate LAN-controller module and exclusive durable ledger. It
survives UI closure, owns only explicit LAN listeners/tunnels, and never starts,
stops or recreates workloads/rdd. It must acquire its own exclusive LAN lease
before observing/recovering retained LAN intents. Renderer state is a draft,
never a source of endpoint, user, executable or firewall authority.

One independently bound Windows listener is created for each reviewed address,
protocol and published host port. It is not a widening/rebinding of the existing
loopback listener. Bind conflicts, including another engine's wildcard listener,
fail promptly without stopping anything, choosing another interface/port or
falling back to `0.0.0.0`. An exposure targets one exact owned-daemon container
ID, StartedAt, published container/host port and publication generation.

Do not forward LAN bytes to a mutable Windows loopback host port: another
engine could reuse it after a stop/restart between inspection and dial. Reuse
the existing exact-target namespace helper for raw TCP transport, pinning helper
ownership and target incarnation before and after acquisition. Generic TCP ports
must remain opaque; no HTTP probe or HTTPS termination is invented for arbitrary
published services. UDP requires an explicit bounded datagram transport extension
to that helper ABI, preserving packet boundaries and exact namespace ownership.
It is not implemented by treating UDP as a TCP stream. The TCP and UDP published
port requirements remain open until both have real end-to-end evidence.

The current raw transport is insufficient to claim complete P2-LAN. Required
integration boundaries are:

1. Extend the authoritative upstream publication observation with the exact
   daemon/container/incarnation/container-port/host-port/protocol/generation
   tuple. A non-atomic inspection pair must recheck its generation; unknown
   results refuse exposure. This is a narrow observation extension, not a second
   Docker lifecycle manager or persistent container database.
2. Add native interface observation by stable Windows LUID, concrete IPv4 address
   and current network profile. Name alone is insufficient. Do not accept UI
   claims that an address/profile belongs to an interface.
3. Add the LAN-controller owner and its raw TCP/bounded UDP namespace transport,
   listener cancellation/join and repeated identity checks. Initial supported
   planning addresses are private IPv4, on explicitly selected Private or Domain
   profiles. Public-profile, wildcard and IPv6 exposure are refused by the
   current planner; IPv6 needs its own explicit support decision and evidence,
   not an implied Phase 2 completion claim.
4. Add a fixed-operation elevated Windows firewall helper. It resolves and
   attests the installed worker executable itself, verifies current-user SID,
   installation/home ownership, exact immutable receipt and fixed rule profile.
   No arbitrary command, executable path, rule text, remote destination, registry
   operation or blanket firewall permission is accepted. The helper is not a
   persistent privileged network daemon. Its invocation/ACL/update boundary is
   still to be implemented and reviewed.
5. Add typed owner-SID IPC for preview, explicit consent/apply, status and remove,
   with settings/receipt sequence guards and cancellation/unknown-result states.
   The native adapter must validate both binary/source attestation and the
   narrow request schema. No current UI button may claim activation succeeds.

## Frozen review and firewall scope

`localnet/lan_plan.go` implements the pure planning boundary. A version-1 review
contains a 128-bit installation owner token, ordinary-user SID, attested worker
digest, exact publication tuple, stable interface LUID, one private IPv4 address,
one explicit Private/Domain profile and one to eight sorted canonical RFC1918
client CIDRs. Each prefix must fit wholly inside its private block; a prefix
merely starting at a private address is insufficient. No `Any`, `LocalSubnet`,
all-profile, wildcard protocol/address/port or arbitrary client range is emitted.
TCP and UDP produce distinct per-port plans. The remote-client scope must also
be enforced by the unprivileged listener for every TCP accept/UDP packet; a
preexisting broad firewall allow rule must not widen the application's scope.

The plan digest is SHA-256 over the domain prefix `orbwin-lan-review-v1\n` and
fixed-struct JSON. Its owner token and digest derive an exact rule name. The
planner clones client CIDRs so later UI edits cannot change a frozen review.
`GuardLANExposure` refuses changed/tampered plans, missing/stale/future observations,
engine/target/restart/publication drift, interface removal/address/profile drift
and forged activation availability. Observations are at most five seconds old;
re-read them after a slow UAC interaction before any next mutation. Passing this
guard does not authorize activation: the native owner/helper must independently
obtain observations and the explicit user consent receipt.

The eventual Windows Firewall rule is inbound allow, exactly one protocol/local
address/local port, exact attested worker program path, selected profile, explicit
remote CIDRs and non-edge-traversal. There is no service-wide or executable-wide
all-port exception. A future helper must compare every filter/default/action in
the actual normalized rule, including unsupported extra filters, before adopting
or removing it. The pure `ValidateLANRemoval` compares the modeled identity and
scope; it does not read Windows Firewall, establish ownership or grant removal
authority. Unknown, foreign, conflicting or changed rules are preserved with
actionable diagnostics. An identical user-owned rule is not silently adopted.

Consent must display the concrete service/container, LAN address/port, protocol,
profile, interface and allowed client ranges, including that application data
may become reachable from other devices. Network access and firewall permission
are separate from HTTPS trust. No DNS/mDNS broadcast, public certificate issuance,
SSH/registry credential forwarding or trust-store change accompanies exposure.

## Durable transaction, restart and removal

The future LAN ledger uses bounded regular files and an exclusive process lease,
private new drafts, flushed immutable intent and acknowledged sequence events.
Its versioned receipt must retain:

- transaction/owner/user-SID, exact frozen review and plan digest;
- committed source observation and sequence, attested worker/native-helper
  component identities, concrete firewall rule specification and native rule ID;
- `consented`, `firewall-intent`, `firewall-observed`, `listener-active`,
  `removal-intent`, `listener-closed`, `firewall-removed` events with timestamps
  and exact identity/sequence guards; unknown acceptance is explicit;
- enough public ownership metadata to remove an exact owned rule after signing
  state, interface, workload or executable upgrade makes exposure unavailable.

No listener or rule mutation precedes explicit consent and flushed intent.
Before each native operation and listener activation, recheck the exact source
and interface. Create/observe the single scoped rule through the fixed helper,
then activate the independently bound listener only after its namespace target
and firewall observation agree with the frozen consent. A preexisting rule with
that name refuses creation unless an exact retained intent proves ownership.
If binding fails after rule acknowledgement, remove only that exact acknowledged
rule through the same native transaction; denied/uncertain cleanup retains the
receipt and reports a dormant rule, never a successful exposure.

Cancellation before dispatch has no mutation. Cancellation/client loss after
helper dispatch does not prove it stopped. Do not blindly redispatch, erase the
intent or widen another rule. Read actual native state under the exclusive owner;
unknown state remains unavailable until explicit reviewed recovery. UI closure
does not cancel a committed healthy LAN service. Worker/engine loss closes or
invalidates its listeners and joins active streams/datagram flows; no lifecycle
repair of the user's engines is performed.

Removal first closes ingress and joins accepted connections/flows, then requests
removal of the exact owned rule and acknowledges observed absence. A denied
firewall removal leaves ingress closed and a retained cleanup receipt. Interface
loss, Public-profile transition, publication loss and workload restart/recreate
retire ingress promptly. Reopening a changed target/incarnation/address/profile
requires fresh review. After worker process restart, unchanged retained consent
may resume only after lease acquisition and complete current-source/interface/
native-rule verification; unknown/partial journals never authorize listeners.
An engine restart's changed publication generation must not silently reuse old
consent. Application update/uninstall must close ingress and settle or explicitly
retain exact rule receipts before deleting the old worker. No broad cleanup,
firewall reset, WSL shutdown, engine stop or unrelated rule deletion is permitted.

TCP flow/datagram queues, per-peer/total connections, payload sizes, idle timeout,
worker shutdown joins and helper lease expiry need explicit finite limits and
measured qualification before activation ships. They must reuse the existing
namespace capacity boundary rather than introduce unlimited guest helpers.

## Evidence and acceptance gates

Planning tests cover TCP/UDP scope, immutable draft copies, per-port rule identity,
malformed/broad/Public/wildcard/IPv6 inputs, SID/LUID validation, publication and
interface drift, stale observations, forged activation claims, application peer
filters and foreign/altered removal scope. They make no Windows network/firewall
calls and create no engine resources or LAN listeners.
The focused `go test ./localnet -run 'TestLAN' -count=1`, full
`go test ./localnet -count=1` and `go vet ./localnet` pass on Windows with the
bundled Go toolchain. These results qualify only the side-effect-free boundary.

The complete P2-LAN gate still requires the native boundaries above, installed
helper packaging/ACL/attestation, consent/denial/cancel/unknown-result UI, actual
TCP/UDP client traffic from another machine, per-port/client/profile filtering,
occupied-port and foreign-rule refusal, workload/app/engine restart, interface
changes, three-engine coexistence, cleanup after crashes/update/uninstall and
attended firewall create/remove evidence. This record does not remove or narrow
the required flow. Localhost remains the default throughout.

## Read-only native interface observer

`internal/lanobserve` now copies Windows IP Helper observations into a bounded
version-1 snapshot; the root SDK hook is `lanInterfaces(ctx, output)`, registered
as `lan-interfaces` with no caller-supplied address/interface arguments. It reads
`GetAdaptersAddresses` into at most 4 MiB with at most five buffer attempts, checks
every linked adapter/unicast/socket pointer and structure length against the owned
buffer, and refuses malformed/cyclic lists. It then reads `GetIfEntry2Ex` by the
exact LUID before and after copying the adapter's addresses. GUID/LUID/index,
type, operational state, tunnel type and hardware-bit disagreement refuses the
whole result, without silently adopting a replacement interface. Snapshot time
records when observation began; the result is not an atomic reservation or proof
that an address remains current after the call.

The hardware flag and interface identity follow Microsoft's
[MIB_IF_ROW2 structure](https://learn.microsoft.com/en-us/windows/win32/api/netioapi/ns-netioapi-mib_if_row2).
The index is observational and may change after disable/enable; it is not used
as a persistent substitute for the GUID/LUID. Adapter/unicast fields use the
existing x/sys definitions of
[IP_ADAPTER_ADDRESSES_LH](https://learn.microsoft.com/en-us/windows/win32/api/iptypes/ns-iptypes-ip_adapter_addresses_lh).

Output contains exact GUID and decimal-string LUID (avoiding JavaScript integer
precision loss), indexes, bounded display alias/description, raw operational
state/type/tunnel/hardware metadata and unicast IP/prefix/DAD/scope metadata.
Derived address flags describe private/loopback/link-local addresses only;
they do not establish network trust. Loopback, tunnel, non-hardware/virtual and
unqualified types receive distinct reasons. Hardware Ethernet/Wi-Fi is only a
`hardware-candidate`, including when down. Friendly names never establish
hardware status or a trusted network profile. Every profile remains `unknown`,
`profileSource` is `unavailable` and activation remains false.

Limits are 128 adapters, 32 unicast addresses per adapter, 512 addresses total and
256 KiB JSON. Overflow, malformed metadata or drift returns an error with no
partial successful snapshot. The wrapper's five-second context is checked before
and between synchronous native calls; it cannot preempt a blocked IP Helper
call. The invoking process boundary must enforce the actual wall-clock deadline.
No background goroutine survives cancellation to publish a stale result.

Portable malformed/duplicate/size/classification/drift tests, Windows pointer-chain
tests, wrapper cancellation tests, vet and scoped Windows race tests pass. The
read-only native wrapper proof is
`artifacts/qualification/p2-lan-interfaces-readonly-63320347ddf1402f8e5a75ecf9f4a6cf.jsonl`
(14,793 bytes, SHA-256
`743ea0866c322f448defef31d2f0e19cea69537cd17707acdca3bb3a3d242bdf`).
It observed twelve adapters with profiles unknown and activation false. The file
contains local interface names and IP addresses and is local qualification data;
it must not enter the support-snapshot allowlist or an automatic upload. No
DNS probes, LAN listeners, firewall/trust/platform edits or engine operations
were performed. Windows network-profile observation, activation/helper receipts,
live LAN traffic and all earlier acceptance gates remain open.
