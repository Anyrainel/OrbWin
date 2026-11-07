# Fixed-operation LAN firewall boundary

Design and offline protocol work, 2026-10-25. No elevated helper/provider is
implemented or launched by this module. P2-LAN activation remains unavailable.
This contract is shared by feasible one-shot helper packaging choices; it does
not require a signed release, whole-app administrator install or persistent broker.

## Authorization and ownership

The existing trust helper is a separately attested Linux ELF executed as selected
WSL root. It does not authenticate Windows administrator operations. Current-user
Windows Root trust likewise needs no administrator boundary. Reuse their bounded
strict request parsing, component attestation, immutable intent/sequence guards,
exclusive owner and explicit unknown-outcome handling. Do not reuse a SID string,
DPAPI bundle or user-writable receipt as proof of privileged ownership.

The proposed Windows helper is one-shot and fixed-operation. Explicit attended
activation launches it through Windows elevation, then the helper itself displays
the exact interface/address/port/protocol/profile/client scope and worker program
before applying a change. UAC approval identifies the program; it does not show
or authenticate the requested port scope. A prior UI checkbox or nonce is not a
substitute for that elevated exact-scope review. Signing/admin installation remain
separate release decisions, not invented prerequisites for this Phase 2 design.

A mutually verified one-shot IPC channel binds caller/helper PID, creation time,
process token, pinned image/file identity and a random nonce to the one request.
The ordinary caller SID is obtained from the authenticated process token; it is
not taken on faith from JSON. Use first-instance pipe creation, deny remote pipe
clients, restrictive caller/admin/System ACLs and fixed bounded frames/deadlines.
The helper verifies the server process and the SDK verifies the returned helper
process handle/pipe client identity. Alternate-administrator elevation must keep
the original caller identity distinct from the elevated token. All request values
remain untrusted even after transport binding. Never accept a program/home/store
path, arbitrary command, firewall rule text, executable arguments or secret.

An admin-protected receipt root may be created on the first attended action.
Its only purpose is durable ownership: a user-writable lookalike receipt must not
authorize deleting a foreign rule. The original user may read, but not write,
the helper's registration/receipt files; administrators/System write. Validate
owner/DACL/reparse-free path traversal and fixed-root ancestry on every open.
No persistent broker or admin secret is needed. A user-home copy is display data,
not authority. Damaged/foreign records stay preserved. No broad rule-name search
or product-prefix adoption can confer ownership.

## Exact request and receipt contract

`internal/lanfirewall` is the proposed bounded pure schema/validation boundary.
Requests select only inspect/create/remove, version 1, installation/user identity,
single-use transaction/nonce, expected helper sequence, full reviewed LAN digest,
authenticated caller PID/creation-time binding, issued/expiry times and one scope.
The scope fixes protocol, private IPv4 local address/port, exact adapter GUID/LUID,
one Private/Domain profile, one to eight explicit private IPv4 client CIDRs and
the attested worker digest. No wildcard address/port/protocol/profile or caller
friendly-name/program-path field exists. The helper resolves and verifies the
actual worker path and current interface itself, and rechecks current IP Helper,
NLM category and SDK source/publication observation before any mutation.

The same client CIDRs must be enforced in the ordinary listener per TCP accept/
UDP packet; another broad allow rule must not widen admission. The request is not
authority to bind a listener. The helper never starts a workload, rdd, WSL distro,
listener or other supplied program. The existing SDK owner starts ingress only
after acknowledged native rule observation and exact source/interface validation.

The current caller image digest is distinct from the retained worker digest, so
an updated SDK can inspect/remove its old protected rule without executing or
requiring the old worker file. The helper must still authenticate the current
caller and show its own exact removal review.

Receipts retain immutable owner/review/scope identity, full last mutation request,
monotonic sequence, native-resolved worker file/path identity and interface alias,
derived exact rule name and intent/acknowledgement state. Create intent precedes
native create; present acknowledgement requires a matching normalized native
scope. Removal intent precedes exact removal; removed acknowledgement requires
observed absence. Unknown outcomes remain create/remove intent. An inspection
does not replay a mutation. It may observe a newer protected sequence than the
caller knew after a lost acknowledgement; writes require the exact current
expected sequence. Fresh request nonces and expected sequence prevent
blind replay; preserved pending intent is resolved only by read-only native
observation and explicit recovery, never by retrying Add/Remove as if absent.

One exclusive protected-store transaction owner serializes each installation's
operations. Listener removal first closes ingress and joins streams/datagrams.
If elevated cleanup is denied or uncertain, keep ingress closed and retain the
native receipt. Update/uninstall must retain the public identity needed to remove
that exact rule even after the worker executable changes or disappears. Removal
never executes the recorded worker path. A read-only native absence after an
interrupted create is insufficient if the native provider may accept the old
request late; settlement requires a provider-specific late-completion argument.

## Native provider constraints still requiring a decision

Microsoft documents that
[INetFwRule.Interfaces](https://learn.microsoft.com/en-us/windows/win32/api/netfw/nf-netfw-inetfwrule-get_interfaces)
uses friendly interface names. Those must be derived from current exact GUID/LUID
observations with uniqueness/rename rechecks, never supplied as identity by JSON.
An adapter name change invalidates the resolved filter until reviewed recovery.

`BindInterface` now provides the pure translation guard over locally obtained
IP Helper and NLM snapshots. It revalidates bounded copied adapter identities and
addresses, requires observations no older than five seconds (and never future
dated), and selects the exact reviewed GUID/LUID and private IPv4 address only
when operational, hardware Ethernet/Wi-Fi, and DAD preferred. The address must
occur on exactly one adapter. The derived alias must be nonempty and distinct
from every other observed alias under case folding, including down adapters.
Only one connected, fully observed matching Private/Domain profile is accepted.
Derived snapshot labels such as `kind`, `state` and `private` do not substitute
for underlying numeric/native facts. These in-memory snapshots are native
evidence inputs, not an authenticated JSON API.

The returned copied `InterfaceBinding` retains GUID, LUID, index, alias, address,
prefix and profile. `GuardInterfaceBinding` compares a fresh resolution against
that exact value before/after a future native operation and on restart. Rename,
index/prefix drift or category changes refuse automatic adoption. This does not
reserve an adapter, make two independent native scans atomic, establish Windows
Firewall's native name-comparison behavior, or eliminate changes after a check.
The eventual provider must qualify those semantics, re-observe after an operation,
and close ingress on drift. It must not use this guard to block exact dormant
rule cleanup after the original adapter disappears: removal instead compares the
protected retained native rule scope and never widens/recreates an interface.
No helper/provider/command is activated by this slice. Pure fault tests cover
rename/index/prefix changes, duplicate aliases/addresses/profiles, stale/future or
incomplete reads, GUID/LUID reuse, tentative/missing addresses, down/tunnel/software
interfaces, profile changes and bounds. Package tests, vet and race checks pass.

Microsoft also documents that
[INetFwRules.Add](https://learn.microsoft.com/en-us/windows/win32/api/netfw/nf-netfw-inetfwrules-add)
can overwrite an existing rule with the same identifier. Item-before-Add is not
an atomic exclusive create. Native provider selection must establish exclusive
rule creation or an explicit qualified concurrency/ownership argument; random
receipt-bound names alone are not a proof of no overwrite. No COM Add provider
is silently assumed safe by the offline contract.

The eventual rule profile is inbound allow, exactly one local address/port and
TCP/UDP protocol, exact native-resolved worker path/interface, one selected
Private/Domain profile, explicit remote CIDRs, no edge traversal, service/package/
user filters or broad exceptions. The native provider must compare every property
and refuse unsupported extra filters before adopting/removing a rule. Matching
name/group/description or a parsed receipt alone is not sufficient.

## WMI create-only provider investigation

The documented [IWbemServices::PutInstance](https://learn.microsoft.com/en-us/windows/win32/api/wbemcli/nf-wbemcli-iwbemservices-putinstance)
`WBEM_FLAG_CREATE_ONLY` contract refuses an existing instance instead of updating
it. This is a concrete candidate for avoiding COM Add's overwrite behavior, not
proof that the firewall provider implements an atomic full-scope create. The
[MS-WMI protocol](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-wmi/f8c3d183-06e8-48fa-b613-c9099b916806)
also requires dynamic providers to support Put or return not-capable. No unsupported
provider result may fall back to upsert, ordinary COM Add or a partial create.

Read-only local `Get-CimClass` observation on 2026-10-25 found provider `wfascim`,
class `root/StandardCimv2/MSFT_NetFirewallRule`, with 39 properties and five
methods. LocalAddress, RemoteAddress, Protocol, LocalPort, Program and
InterfaceAlias are absent from the class properties, and no Create method is
exposed. InstanceID and Enabled are marked ReadOnly in the observed schema.
Microsoft's installed NetSecurity `NetFirewallRule.cmdletDefinition.cdxml`
supplies the missing contract: New uses `cim:CreateInstance`, with those filters
and PolicyStore supplied as custom operation options. Plain PutInstance with
ordinary rule properties alone is therefore not a complete implementation.
Microsoft documents [custom CIM operation options](https://devblogs.microsoft.com/powershell/cim-cmdlets-some-tips-tricks/),
but neither that general mechanism nor this schema read proves this provider's
all-or-nothing behavior or exact filter semantics.

The bounded read-only reproduction is `scripts/test-lan-wmi-schema.ps1`. It reads
class metadata and installed Microsoft CDXML, never rule instances or provider
methods, with a five-second CIM operation timeout, metadata-count budgets and a
64 KiB JSON output budget. External process deadline is still required for a hard
wall-clock bound. Evidence:
`artifacts/qualification/p2-lan-wmi-schema-readonly-adca9a695bfb46ef91e92bfa6e482f35.json`
(5,657 bytes, SHA-256
`847ac1936f1116a5aa3205f8e5cd9009aaf3e7bc5ff1c02b2de484db91a7c340`).
It contains only schema/installed-contract metadata and no host/adapter/rule
instance identity. Activation and atomic-create qualification are explicitly false.

`PrepareWMIDisabledCreate` now maps a validated, fresh create-intent receipt and
exact native interface binding to a fixed disabled-rule call specification:
create-only flag 2, Enabled 2, exact single Private/Domain profile, inbound Allow,
blocked edge traversal, no block override, fixed PersistentStore, exact program,
address/port/protocol/interface and cloned peer CIDRs in their custom options.
It exposes no generic property bag, enabled-create or native dispatch hook.
Because CDXML types InterfaceAlias as WildcardPattern[], aliases containing
wildcard/escape metacharacters are refused until literal handling is qualified.
Program paths containing percent signs are also refused until environment
expansion cannot change the pinned path. Neither is silently escaped or widened.
Tests check TCP/UDP and Private/Domain scope preservation, stale/mismatched
intent/binding refusal, wildcard/path expansion refusal and ownership of copied
CIDR data. Package tests, vet and Windows race checks passed.

The next attended provider proof must establish that create-only plus all exact
custom options creates a disabled rule as one operation, concurrent duplicate
keys leave the incumbent unchanged, unsupported options fail closed, every
normalized filter/default is read back before any enable, and cancellation or
client death cannot allow late untracked completion. Effective ActiveStore/GPO
policy and explicit administrator blocks must remain authoritative. Generic
WFP transactions offer atomicity, but [filter arbitration](https://learn.microsoft.com/en-us/windows/win32/fwp/filter-arbitration)
means an independent soft permit may still be blocked by Windows Firewall;
hard-permit policy bypass is not an acceptable shortcut. No WFP replacement,
Put/Create/Enable/Delete native binding, or firewall write was implemented/run.

## Remaining integration and acceptance

`internal/lanfirewall` now implements only the bounded pure contract:
`ParseRequest(data, now)`, `ParseReceipt(data)`, `GuardCaller`,
`GuardStoredOwner` and `ValidateNext`. Request/receipt byte limits are 16/32 KiB.
All fields are explicit; unknown fields, case aliases, duplicate keys, missing
fields, extra JSON values and invalid UTF-8 are refused before typed validation.
Client CIDR lists are bounded, canonical and sorted. Request lifetimes are at
most five minutes; native operations still need a fresh source/profile read
after a slow elevation interaction. Sequence numbers retain JavaScript's exact
integer range and reserve room for both intent and acknowledgement before a
mutation starts. A parsed historical receipt validates structure, not freshness
or actual native ownership; only the future protected-store provider can supply
that authority.

Package tests, vet and scoped Windows race tests pass with the bundled Go/UCRT
toolchain. Tests cover hostile JSON, arbitrary path/operation injection, broad
scope, malformed identities, future/expired requests, PID-reuse/token/image
binding, stale/foreign protected ownership, read-only recovery of newer sequences,
intent-before-ack state transitions, unknown-outcome replay refusal and old-rule
removal by an updated caller. No firewall, process elevation, listener, protected
store or engine operation was performed by these tests.
A two-worker parser fuzz smoke run passed (12 executions, about three seconds);
that small sample is not broad fuzz qualification.

No WinAPI firewall provider/helper/main, elevated launch, rule read/write or shared
SDK/controller change is included. Packaging/path pinning, protected-root/ACL and
pipe authentication, elevated scope review, native no-overwrite/late-completion
semantics, actual receipts and IPC, listener/UDP transport, attended consent/
denial/cancel/recovery/removal and full P2-LAN qualification remain required.
