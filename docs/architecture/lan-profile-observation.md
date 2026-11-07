# Read-only LAN network-profile observation

Design, 2026-10-25. This component observes Windows network categories. It does
not authorize a listener, firewall rule, trusted profile change or LAN exposure.
The existing interface observer remains independently unchanged until explicit
root integration. Profiles join only by exact canonical adapter GUID.

## Verified API and ABI contract

Microsoft documents connection enumeration through
[INetworkListManager](https://learn.microsoft.com/en-us/windows/win32/api/netlistmgr/nn-netlistmgr-inetworklistmanager),
adapter identity through
[INetworkConnection::GetAdapterId](https://learn.microsoft.com/en-us/windows/win32/api/netlistmgr/nf-netlistmgr-inetworkconnection-getadapterid),
and Public, Private and DomainAuthenticated categories through
[NLM_NETWORK_CATEGORY](https://learn.microsoft.com/en-us/windows/win32/api/netlistmgr/ne-netlistmgr-nlm_network_category).
Those categories do not establish which firewall ports are open. We additionally
do not treat Private/DomainAuthenticated as user authorization to expose a port.

The Learn interface overview's IUnknown inheritance summary is insufficient for
ABI layout. Microsoft's actual
[netlistmgr.h SDK header](https://raw.githubusercontent.com/microsoft/win32metadata/main/generation/WinSDK/RecompiledIdlHeaders/um/netlistmgr.h)
defines INetworkListManager, INetworkConnection, INetwork and
IEnumNetworkConnections as dual interfaces deriving from IDispatch. All have
the seven-slot IUnknown/IDispatch prefix. The bundled MinGW SDK header agrees.
The implementation must use that exact layout rather than a three-slot prefix.
Only fixed getters and IUnknown Release are callable; no setter, simulation,
name/description/BSTR, event subscription or caller-selected CLSID is exposed.

COM initialization follows
[CoInitializeEx](https://learn.microsoft.com/en-us/windows/win32/api/combaseapi/nf-combaseapi-coinitializeex):
lock the goroutine to one OS thread, initialize MTA, and balance both S_OK and
S_FALSE with CoUninitialize on that same thread. A changed apartment model is
an unavailable observation; do not adopt another thread's apartment or uninitialize
a failed initialization. Every acquired manager/enumerator/connection/network
reference is released once before uninitialization, including failure, cancellation
and enumeration overflow. No finalizers or background goroutines own COM state.
See [IUnknown::Release](https://learn.microsoft.com/en-us/windows/win32/api/unknwn/nf-unknwn-iunknown-release).

Create only the fixed local NetworkListManager coclass/interface. The current
host registers its in-process implementation as System32 netprofm.dll. Request
only in-process activation with code downloading disabled; unsupported activation
is unknown, not permission to start a remote/elevated object. This is a local
read-only metadata operation, not a registry, network-profile or service-config
change.

## Observation, cancellation and ambiguity

The proposed package API is `lanprofile.Observe(ctx, adapterGUIDs)`, where
adapter GUIDs come from a separate verified IP Helper snapshot. Validate at most
128 distinct canonical nonzero GUIDs before COM access. Enumerate at most 256
connections, one at a time through
[IEnumNetworkConnections::Next](https://learn.microsoft.com/en-us/windows/win32/api/netlistmgr/nf-netlistmgr-ienumnetworkconnections-next).
Enforce S_OK/one-element or S_FALSE/zero-element agreement; malformed enumeration
or a read error discards all partial category assignments.

For each connection read its connection and adapter IDs and connected state.
For a connected connection, acquire its network, read the network ID, connected
state and category, then recheck category/network/connection/adapter identity and
connected state before releasing references. Disconnection or disagreement during
those calls refuses the observation. This bounds detectable drift; COM calls are
not an atomic network reservation. The future activation transaction must obtain
fresh observations immediately before each native/listener boundary.

Map one unique, fully read, connected connection to its exact requested adapter
GUID. Multiple connected connections, unsupported category values or incomplete
data produce unknown for that adapter. Only disconnected records produce unknown
disconnected state; a missing adapter match is unknown missing state. Historical
disconnected records do not override a uniquely observed current connection.
Malformed/duplicate identities, COM failures, cancellation and inventory overflow
produce an unavailable all-unknown result with an error; partial Private results
must never survive an incomplete scan. No name, address, interface index, LUID
guess or network display name substitutes for the exact GUID join.

Iteration and output sizes are bounded. Check cancellation before/between every
native call and iteration; releasing references and balancing COM initialization
remain mandatory after cancellation. Synchronous COM/native calls cannot be
preempted by Go context. The enclosing short-lived SDK process must enforce its
wall-clock deadline; abandoning an unjoined COM goroutine is not cancellation.

Output includes only requested adapter GUID, observed category/state and bounded
counts/reasons. It omits network names, connection/network IDs, credentials, MACs,
client data and IPs. The profile metadata is nevertheless private local diagnostic
data and must remain outside automatic support upload allowlists. All activation
flags remain false even for a successfully observed Private category.

## Integration and remaining gates

Root integration should invoke IP Helper observation, profile observation for
that exact GUID set, and a second IP Helper identity/address check. Do not merge
an old profile map into a fresh interface list or promote unknown to Private.
Preserve a useful interface read when profile COM access is unavailable, showing
the unknown profile and reason. This component will not edit the stable observer,
SDK main switch, build/attestation lists or desktop schema itself.

The implementation exports `Observe(ctx context.Context, guids []string)
(Snapshot, error)`. Missing/disconnected/ambiguous/unsupported categories produce
per-adapter unknown values with a complete enumeration and nil error. COM/read/
cancel/overflow/malformed-record errors return an all-unknown incomplete snapshot
for the validated requested set plus a nonnil error. Invalid requests return an
empty/error result. Callers must not discard the error and reuse an older Private
result. The package is independently implemented; the proposed `lan-profiles`
SDK command can obtain the bounded IP Helper GUID set and print only this profile
snapshot without altering the existing `lan-interfaces` response.

Package tests and vet pass. Windows race tests pass with the bundled UCRT GCC,
CGO enabled and GOMAXPROCS=2. Tests exercise exact joins, all category values,
ambiguity/missing/disconnection, unsupported/incomplete categories, malformed and
duplicate identities, iteration budgets, partial-result rejection, cancellation,
and exact release of connections/enumerators after success/read failure/
enumeration failure/overflow. An ABI boundary test verifies the SDK's read-only
vtable offsets so the documentation inheritance discrepancy cannot silently
invoke another method. A COM changed-mode test proves caller STA remains intact
after refused MTA initialization.

Native read-only proof:
`artifacts/qualification/p2-lan-profiles-readonly-4422e330485248f3b47d4f3b3e14e7f5.jsonl`
(6,044 bytes, SHA-256
`b8af6d7f6ad384d6ff8d8badcb2567ef59c6fcb69d0f5ab3e6e54b0521dab347`).
It observed twelve requested adapter GUIDs: two unique connected Private
categories and ten unknown/missing profiles. Activation remained false throughout.
The test deliberately initializes MTA before calling the observer, exercising
S_FALSE; successful subsequent STA initialization on a fresh apartment proves
the observer balanced its nested initialization. The proof contains no interface
names or IPs, but its adapter GUID/category metadata remains private qualification
data excluded from support uploads. No profile setters, firewall/trust/network
changes, listeners, elevation or engine operations ran.

Profile reads alone do not complete P2-LAN; durable listener/firewall ownership,
explicit consent, source-generation observation, TCP/UDP transport and attended
real LAN proofs remain required by `lan-exposure.md`.
