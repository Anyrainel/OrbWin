# WFP dynamic-session alternative: ownership evidence and admission gates

Source/primary-document investigation, 2026-10-25 local date. No WFP or CIM
mutation, WFP query, elevation or native activation has been performed. The
qualified CIM observations and exact stage-3 executable remain frozen. This
record proposes no scope exclusion or accepted provider replacement.

WFP offers a stronger documented lifetime and cross-session ownership boundary
than the current persistent CIM candidate. It is not yet a defensible complete
LAN admission provider: a soft-permit filter is overridable by Windows Firewall,
and no evidence here establishes automatic approved opening under default-block
policy. Hard-permit/policy-bypass shortcuts are prohibited. Consequently neither
a persistent CIM mutation worker nor a WFP activation binding is implemented in
this increment. The concrete remaining gates and narrow next experiments are
below; a missing qualification is not silently treated as success.

## Accepted feature need and current CIM evidence

The required scope remains one reviewed TCP or UDP port, exact private IPv4
address, actual adapter GUID/LUID, one Private/Domain profile, explicit private
client CIDRs and attested worker identity. Foreign rules, explicit administrator
blocks and effective GPO policy stay authoritative. The listener independently
enforces the same source restrictions; no global defaults, another engine's
resources or network stack are replaced. See
[lan-firewall-boundary.md](lan-firewall-boundary.md) and
[lan-exposure.md](lan-exposure.md).

The current CIM bridge has actually read all 39 rule fields, seven filter
families and reverse identities in two stable typed passes. Those are useful
diagnostics, not exclusive ownership. Microsoft documents
[MI CreateInstance](https://learn.microsoft.com/en-us/windows/win32/api/mi/nf-mi-mi_session_createinstance)
as failing for an existing instance, while the
[NetSecurity Name contract](https://learn.microsoft.com/en-us/powershell/module/netsecurity/new-netfirewallrule?view=windowsserver2025-ps#-name)
permits one name per store. This supports a controlled duplicate/race fixture,
not a claim that wfascim has already passed it. MI operation flags zero and
WBEM_FLAG_CREATE_ONLY value 2 are distinct; MI value 2 means BASIC_RTTI.
[INetFwRules.Add](https://learn.microsoft.com/en-us/windows/win32/api/netfw/nf-netfw-inetfwrules-add)
can overwrite an existing identifier and is not a fallback.

The local MI DeleteInstance signature supplies instance keys, not an expected
full-state generation/CAS token. Read-before-delete cannot itself exclude an
ordinary competing delete/recreate using those keys. An unknown create outcome
cannot be adopted just because its name/bytes later match; preflight absence
does not confer ownership, and cancellation does not prove no late commit.
Administrative actors remain trusted OS authority; the concern is ordinary
cross-client races, ownership confusion and worker death, not impossible
protection against a hostile administrator.

## Why dynamic WFP is a concrete alternative

Microsoft's [object-management contract](https://learn.microsoft.com/en-us/windows/win32/fwp/object-management)
ties a session to its client process/RPC lifetime. Teardown aborts an outstanding
transaction; dynamic-session objects are deleted. Explicit transactions permit
the full bounded filter set to be committed together or aborted. An individual
Add failure does not automatically abort the whole transaction, so a candidate
must explicitly abort instead of committing a partial CIDR set. These are
documented semantics, not a measured cleanup-latency guarantee on this machine.

The [dynamic-session flag](https://learn.microsoft.com/en-us/windows/win32/api/fwpmtypes/ns-fwpmtypes-fwpm_session0)
selects automatic object removal at session end. A future privileged lease worker
would therefore keep its local engine session open for the entire reviewed
exposure, with no persistence/boot-time flags. Close, process death, BFE loss and
SDK-owner/pipe loss must all leave ingress closed. An isolated helper remaining
alive per exposure is a proposed lifetime change from the current one-shot CIM
model, not a silently accepted permanent service or required admin installation.

[DeleteById](https://learn.microsoft.com/en-us/windows/win32/api/fwpmu/nf-fwpmu-fwpmfilterdeletebyid0)
uses the ID returned by Add. In a dynamic session it refuses another session's
object and a non-dynamic object. Thus retirement can stay inside the still-owned
session rather than finding/adopting a similarly named global object. A new
worker must not replay a persisted runtime ID after restart or attempt to take
over a prior session; an uncertain old worker is first retired, and a new review
starts a new lease. Receipt names/prefixes/annotations remain diagnostics, not
authority.

| Property | Persistent CIM candidate | Dynamic WFP candidate |
| --- | --- | --- |
| Exclusive create | Documented MI fail-if-existing; wfascim duplicate/concurrency behavior unqualified | New object GUID plus transactional Add; duplicate GUID/LUID has documented error; exact behavior still needs fixture |
| Retirement identity | Four observed keys plus protected full typed state; no documented compare-delete token | Returned runtime ID used only in the same still-owned dynamic session |
| Worker death | Persistent rule may survive uncertain completion | Session teardown removes its dynamic objects; actual timing and in-flight RPC behavior need proof |
| Policy inspection | NetSecurity rule/filter diagnostics already qualified read-only | Own provider/sublayer/filter GUID, session key, returned ID, exact conditions/action/flags require new bounded observation |
| Effective admission | Normal Windows Firewall policy path; exact create/filter/default/enable semantics unqualified | Soft permit may remain blocked; admission and explicit-block coexistence are open |

[WFP error codes](https://learn.microsoft.com/en-us/windows/win32/fwp/wfp-error-codes)
include already-existing object, wrong session, aborted session and aborted
transaction. None authorizes a fallback upsert, global delete, action-right
change or retry after an uncertain mutation. New provider/sublayer GUIDs must
also be exclusively created inside the intended session, never adopted on a
name match. Returned runtime IDs are not durable identities across BFE restart;
the [filter structure](https://learn.microsoft.com/en-us/windows/win32/api/fwpmtypes/ns-fwpmtypes-fwpm_filter0)
identifies them as LUIDs.

## Exact typed scope is plausible; policy equivalence is not proven

A candidate would use the existing OS ALE_AUTH_RECV_ACCEPT_V4 layer, never a
custom callout/driver. The
[published layer-condition list](https://learn.microsoft.com/en-us/windows/win32/fwp/filtering-conditions-available-at-each-filtering-layer#fwpm_layer_ale_auth_recv_accept_v4--fwpm_layer_ale_auth_recv_accept_v4_discard--fwpm_layer_ale_auth_recv_accept_v6--fwpm_layer_ale_auth_recv_accept_v6_discard)
includes the required app/address/port/protocol/local-interface and profile
conditions. A bounded set would carry every condition on each peer-CIDR filter,
never separate broad permits whose conditions accidentally form a union.

| Required restriction | Candidate condition/value shape |
| --- | --- |
| Exact worker program | ALE_APP_ID, FWP_BYTE_BLOB_TYPE from OS app-ID conversion; still verify the pinned executable/file identity |
| One IPv4 address | IP_LOCAL_ADDRESS, exact /32 mask |
| Explicit peer CIDR | IP_REMOTE_ADDRESS, one canonical IPv4 mask per filter; one to eight filters in one transaction |
| TCP/UDP | IP_PROTOCOL, FWP_UINT8, exactly TCP or UDP |
| One local port | IP_LOCAL_PORT, FWP_UINT16 |
| Exact interface | IP_LOCAL_INTERFACE, FWP_UINT64 actual LUID; keep fresh GUID/LUID/IP Helper/NLM checks |
| One Private/Domain category | CURRENT_PROFILE_ID plus original-flow/profile-change reasoning, FWP_UINT32; exact numeric encoding still unverified |

The [condition reference](https://learn.microsoft.com/en-us/windows/win32/fwp/filtering-condition-identifiers-)
documents these value types and describes profile IDs as network categories.
This investigation does not guess their numeric mapping from NetSecurity profile
bitmasks or NLM values. Required byte order, profile encoding, IPv4 masks, app-ID
conversion, exact layer reauthorization and TCP/UDP/established-flow behavior
must be verified against primary definitions and an isolated fixture. A layer's
condition availability alone does not prove identical EdgeTraversal=Block or
effective GPO semantics. No service/package/user/dynamic-keyword exception is
silently added. Caller-supplied paths, GUIDs or arbitrary condition bags cannot
become privileged native authority.

## Admission gate and prohibited shortcuts

Microsoft's [arbitration model](https://learn.microsoft.com/en-us/windows/win32/fwp/filter-arbitration)
allows another sublayer to block a soft permit. It identifies default filter
permits as soft and discusses overridable low-priority port requests followed
by firewall validation. This is not evidence that a direct WFP Add automatically
creates a normal Windows Firewall exception. The same page contains a confusing
flag-set/cleared sentence beside its action-right table; the candidate must not
derive a bypass from that wording. CLEAR_ACTION_RIGHT, hard permits, sublayer
manipulation of another provider, exception suppression and firewall/profile
default changes are all prohibited. A blocked soft-permit result must remain
blocked, not trigger a stronger fallback.

Also, FWPM_FILTER_FLAG_DISABLED is **not an inert-create switch**: the
[filter flags contract](https://learn.microsoft.com/en-us/windows/win32/api/fwpmtypes/ns-fwpmtypes-fwpm_filter0)
states it cannot be set when adding a filter. Do not port the disabled CIM
fixture by setting it. An inert/isolated WFP test must instead have its own
reviewed non-listening program/address/port conditions and real bounded lifetime,
or use a deliberately isolated test environment. No such mutation is run here.

## Minimal next decision and qualification sequence

The useful next candidate is a closed, typed dynamic-lease boundary, not a
production replacement claim. Before native implementation, resolve the profile
encoding and determine whether the full admission requirement can be satisfied
using only overridable policy. If the supported API cannot do that, retain WFP
as an ownership/restriction mechanism and return to an explicitly qualified
normal-firewall create path; do not broaden permit strength or cut P2-LAN scope.

If that gate is defensible, prepare exact SDK-header ABI/source-only evidence and
a compiled opt-in fixture for root review. Keep action soft, use a local NULL
server and fixed authenticated caller/worker/interface scope, a dynamic session,
short explicit transaction-wait timeout and an external worker hard deadline.
No persistent provider, global install, arbitrary command or policy enumeration
belongs in the interface. At most eight fixed filters plus owned provider and
sublayer are added in one explicit transaction, with a pre-flushed exact intent.
Any failed condition/Add/readback aborts the full transaction. Do not bind ingress
until commit and same-session exact ID/GUID/action/flag/condition readback agree.

Root-controlled proofs must cover duplicate GUID refusal/incumbent preservation,
transaction partial failure/abort, same-session removal versus foreign-session
refusal, lost Add/commit acknowledgement, parent/helper death, blocked/late RPC,
BFE restart, cleanup latency and preserved unrelated filters. Close and join the
ordinary TCP/UDP listener first; policy deletion alone does not prove that every
previously authorized flow has stopped. No repeated mutation resolves an unknown
outcome. Protected receipts retain the exact lease/owner/scope/sequence and runtime
IDs for diagnostics, while live native session ownership authorizes retirement.
If retirement is uncertain, keep ingress closed and preserve an unknown-state
record; never report success from a failed/absent inspection alone.

Policy compatibility then needs positive reviewed TCP and UDP traffic through a
normal default-block profile **without** action-right bypass, and negative
explicit Windows Firewall/GPO blocks, profile/interface drift, wrong peer,
wrong port/protocol/program and previously established flows. Those results,
not the availability of WFP APIs, decide provider adoption. Root owns all actual
queries/mutations/elevation and future integration; this record leaves both
mutation candidates closed.
