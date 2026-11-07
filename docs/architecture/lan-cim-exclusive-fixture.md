# CIM exclusive-create qualification fixture

Status: source/compile gate only. No provider mutation, elevation, or native
fixture execution is authorized by this record. The qualified read bridge remains
unchanged. Root owns a separately reviewed opt-in execution decision.

The existing fixed-selector read proof (`17e862…`, receipt
`p2-lan-rule-keys-declaration-native-18edf0b5-e88d-4c86-8b83-cc74f24c5817`)
and seven-family typed traversal proof (`fe5e…`, receipt
`p2-lan-filter-traversal-native-b009a985-d815-4e36-8014-37210765e7c4`)
establish bounded observations, not ownership, policy-store enforcement for all
operations, or permission to mutate. Preserve their exact sources and receipts.

## Fixed inert policy and trust boundary

Only test files may name the native Create/Delete slots. The fixture accepts no
caller-supplied program, rule, query, store, or provider command. A fresh 128-bit
nonce names disabled BLOCK inbound rules, private profile, edge traversal blocked,
own test executable, TCP, loopback local/remote address, fixed high local port, and
the observed loopback interface alias. A pre-elevated administrator token is a
precondition before provider dispatch; there is no RunAs, prompt, helper install,
allow, enable, modify, COM Add, PowerShell, or fallback store.

Installed Microsoft NetSecurity CDXML and its matching provider documentation
define Enabled=False=2, Action=Block=4, Direction=Inbound=1, Profile=Private=2,
and EdgeTraversalPolicy=Block=0. These are property enums. MI operation flags are
zero; WBEM_CREATE_ONLY=2 must never be passed as MI flags. Every custom option is
mandatory, with NewOperationOptions must-understand and SetCustomOption
must-comply both true. Unsupported options fail the fixture rather than widen it.

## Ownership, races, and recovery

Before every mutation, append and flush a bounded durable intent in a fresh
administrator/SYSTEM-only receipt directory. Each actor has a separate receipt.
Only a positive terminal Create success, copied returned full keys, operation
Close, exact nonce identity, and successful durable acknowledgement flush may
mint a private live ownership capability. Positive completion/key validation is
separate from capability minting: the journal serializes candidate keys without
constructing a live capability, and Grant explicitly requires recorded=true.
Cancelled, transport-failed, late, malformed, collision, and unknown outcomes
never mint ownership, even if a later read happens to find matching state.

Actor A independently creates and acknowledges an incumbent. Actor B deliberately
submits the same name with different fixed fixture metadata. B can never adopt
A's resource. Require a definite terminal MI_RESULT_ALREADY_EXISTS=11 and exact
full typed incumbent preservation. A second fresh name has two independent
actors dispatch concurrently: require one acknowledged success and one definite
collision. Two successes or any unknown result fail and retain the inert resource
and durable intents for exact manual review; do not infer a winner by inspection.

Before retiring an acknowledged resource, record a remove intent and compare
returned full keys and every typed rule/filter field against the owner's retained
two-pass snapshot, including all seven reverse associations. Close the owned
listener first (this fixture never opens one). Drift, missing receipt, failed
flush, no live capability, duplicate, or unknown create means no Delete call.
There is no name/group/query-only delete, retry of an uncertain Create, or receipt
deserialization into authority. Unknown Delete likewise retains a pending record.

This is an exclusive cooperating fixture owner with exact drift observations.
Read-then-Delete is **not provider CAS**. Administrative actors remain trusted OS
authorities. External process termination can interrupt drain, durable recording,
and cleanup; operation/context/test deadlines cannot guarantee absence of late
commit. A retained intent is a recovery warning, not automatic ownership evidence.

## Product containment proposal, not observed implementation

Reuse the existing authoritative manager and its privileged service/ownership
boundary for a future worker lease. Do not introduce a second orchestrator or a
new authentication protocol merely for this fixture. Each lease would use an
administrator-protected, never-reused executable path/file identity and an exact
program rule; the worker must remain closed without the current authenticated
manager lease bound to its actual process identity. A unique path alone is
insufficient: an ordinary process might otherwise relaunch that path.

On retirement, first close/join sockets and stop/join the exact owned process/job,
then attempt rule retirement. Tombstone the lease/path permanently; never
re-authorize it. This could make a late/unknown persistent allowance dormant after
worker death. It does not prove rule deletion, CAS, blocked established-flow
semantics, resistance to trusted administrators, or correct admission under GPO.
Those remain separate product gates. The current deterministic review-derived
rule name must bind the fresh lease/path before any product ownership claim.

## Primary contracts

- [MI CreateInstance](https://learn.microsoft.com/en-us/windows/win32/api/mi/nf-mi-mi_session_createinstance)
  documents failure for an existing instance; actual wfascim exclusivity remains
  the purpose of this future fixture.
- [MSFT_NetFirewallRule](https://learn.microsoft.com/en-us/windows/win32/fwp/wmi/wfascimprov/msft-netfirewallrule)
  defines the property enums and provider surface.
- [New-NetFirewallRule Name](https://learn.microsoft.com/en-us/powershell/module/netsecurity/new-netfirewallrule?view=windowsserver2025-ps#-name)
  describes name uniqueness within a policy store; it does not prove atomic MI
  behavior or ownership.
- [INetFwRules.Add](https://learn.microsoft.com/en-us/windows/win32/api/netfw/nf-netfw-inetfwrules-add)
  permits replacement; it is deliberately excluded.

The exact local SDK function signatures, offsets, installed CDXML hash, source
hashes, compile-only checks, native executable hash, opt-in and bounds must be
recorded before a root-reviewed run. No result in this document is a mutation
qualification, provider-wide atomicity claim, or LAN activation approval.

## Prepared source and proposed execution boundary

Seven new `exclusive_fixture*.go` files contain the private fault model and
test-only native fixture. The source qualifier is
`artifacts/qualification/p2-lan-cim-exclusive-source/qualify-source.mjs`. It
explicitly clears all six native opt-ins, checks the frozen read-bridge hashes,
compiles exact SDK Create/Delete signatures and offsets (32/40), runs local
fault/ABI tests, vet, race detection, and Windows/Linux test cross-compilation.
The resulting source-proof records the executable hash and execution=false.
These checks do not qualify provider mutation.

The proposed future single test is
`TestMICIMExclusiveCreateDisabledBlockNative`, with the exact opt-in
`ORBWIN_QUALIFY_LAN_CIM_EXCLUSIVE=disabled-block-create-duplicate-race-v1`.
Only after root source/hash review and an explicit administrative execution
decision would the executable run with `-test.run` anchored to that exact name,
`-test.v -test.count=1 -test.timeout=75s`, minimal SystemRoot/WINDIR/TEMP/TMP plus
that opt-in, and an external 90-second hard process deadline. Other opt-ins stay
absent. The fixture requires an already elevated administrator token and never
requests one. Its cumulative context is 60 seconds, with the final 20 seconds
reserved for cleanup; each native operation receives at most five seconds,
re-clamped immediately before dispatch. Cancellation/drain can still block until
the external deadline, and a hard interruption does not guarantee cleanup.

At most five Create calls target three generated names: A/B share incumbent
name, C/D share race name, and U tests an unsupported required option. U must
produce a definite terminal NOT_SUPPORTED=7 with no instance and observed name
absence; a local setter failure or generic daemon failure does not qualify
provider option handling. At most three positively acknowledged resources can
be retired, never a collision or uncertain creation. Each full definition read
performs 31 instance operations (three rule reads and fourteen forward/reverse
pairs), with the frozen cumulative 64-KiB logical copy budget. Every created
candidate must match the exact disabled BLOCK/own program/loopback scope before
being considered a usable retained snapshot; unexpected provider normalization
is an honest failure requiring review, never a widening fallback.

Protected receipts retain own acknowledged full keys, fixed requested scope,
own executable hash, numeric operation statuses, and a deterministic hash of all
typed snapshot fields. Foreign/unknown returned keys and raw provider messages
are excluded. No crash-recovery adoption parser exists. An unknown create may
require root to inspect the exact nonce intent and provider state manually;
successful name reconciliation alone cannot authorize retirement. Receipts are
deliberately retained after a clean run as evidence rather than silently erased.
