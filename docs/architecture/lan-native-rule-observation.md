# Proposed exact native rule and filter observation

Design only, 2026-10-25. No rule instance query, enumeration, mutation, elevation
or SDK integration has run for this proposal. The qualified `lanprovider` source,
probe executable, architecture record and proofs remain frozen.

## Established identity and the next narrow step

Root ran the exact `d471067…` read-only schema probe successfully in 170 ms:
`artifacts/qualification/p2-lan-mi-native-df0cb12a-9e9a-4026-b035-6be71b7ff30c/proof.json`.
The complete result contains 39 properties/five methods, provider `wfascim`,
activation false and atomic-create false. Actual MI property flags confirm four
string keys: CreationClassName, PolicyRuleName, SystemCreationClassName and
SystemName. InstanceID is readonly and **not** a key.

Microsoft describes [InstanceID](https://learn.microsoft.com/en-us/windows/win32/fwp/wmi/wfascimprov/msft-netfirewallrule)
as unique within a PolicyStore and PolicyRuleName as reserved for its provider.
The installed Microsoft rule CDXML maps Name/ID lookup to InstanceID. This supports
an exact identifier lookup; it does not establish a formula for the four keys,
exclusive create semantics or authority to remove a rule. No prefix/composite-key
formula will be inferred from third-party examples or invented from the rule name.

Prefer **no discovery enumeration**. The first proposed instance probe selects
one source-fixed, root-reviewed literal, tentatively `CoreNet-DNS-Out-UDP`.
Its presence/current contents are unknown: no lookup has happened. If absent,
report `not-found`; do not scan other names, inspect other apps, create a fixture,
switch stores or adopt a rule. Root can review a different exact literal in a
separate run if needed. The candidate name is only a probe selector and never
ownership evidence. No rule returned by this qualification is adopted or changed.

The future product reader will instead receive the expected InstanceID from its
protected receipt/derived reviewed rule identity, not this qualification selector.
It must retain the exact policy-store identity as part of native ownership.

## Proposed reads, in independently reviewed stages

1. **Filter metadata only.** Read the seven fixed filter and seven association
   class schemas before decoding any instances. Capture actual types/key flags/
   references, including inherited/extra fields; do not infer native types or
   defaults solely from CDXML input types. Pin a reviewed supported schema. This
   needs no instance selection and may be qualified separately from the next
   two stages.
2. **Exact identifier to full keys.** In a local NULL-destination session and
   fixed namespace, submit only this fixed WQL shape, with an explicitly required
   PersistentStore option:

   ```sql
   SELECT InstanceID, CreationClassName, PolicyRuleName,
          SystemCreationClassName, SystemName
   FROM MSFT_NetFirewallRule
   WHERE InstanceID = 'CoreNet-DNS-Out-UDP'
   ```

   There is no query-string API, LIKE, wildcard, alternate dialect, remote host,
   All or GPOSession option. The qualification literal is source-fixed; eventual
   owned IDs have a narrow ASCII grammar. Provider query support is unqualified
   until observed. Zero results is absence, one is a candidate, and a second
   candidate is an ambiguity refusal. Require exact returned InstanceID equality
   in memory even if WQL equality uses a broader case comparison. Copy each actual
   typed key, preserving empty string versus null/missing; missing/null/changed
   key metadata is unavailable, not a fabricated empty key.
3. **Exact GetInstance and filters.** Build a local in-memory keys-only MI object
   with the actual four copied key values, fixed class and namespace; never
   interpolate a CIM object path. [GetInstance](https://learn.microsoft.com/en-us/windows/win32/api/mi/nf-mi-mi_session_getinstance)
   explicitly accepts a class-and-keys instance. Verify returned class, complete
   keys and InstanceID agree with the selection. Fresh local MI object allocation,
   AddElement and local-object disposal are argument/ownership operations, not
   provider Create/Delete; no provider mutation binding is added.

The third stage traverses each fixed association using the selected rule's keys,
`role=GroupComponent`, `resultRole=PartComponent`, the exact association and result
classes below, initially `keysOnly=true`. It then reads that exact filter by its
actual observed keys. No unfiltered association or cross-rule filter search is
allowed. Reverse traversal must return the original exact rule keys. Missing,
duplicate or unexpected filter identities refuse a complete observation; absence
must not be turned into an invented Any/default filter.
The key stage permits at most three dispatched reads (one query/two exact
round-trips). After metadata qualification, the filter stage permits at most 37:
two rule reads, two passes of seven association-key plus seven exact-filter
reads, and seven reverse-link checks. No unbounded rescan loop is proposed.

| Association | Filter class | Semantic fields to inspect in RAM |
|---|---|---|
| MSFT_NetFirewallRuleFilterByAddress | MSFT_NetAddressFilter | LocalAddress, RemoteAddress |
| MSFT_NetFirewallRuleFilterByApplication | MSFT_NetApplicationFilter | AppPath, Package |
| MSFT_NetFirewallRuleFilterByInterface | MSFT_NetInterfaceFilter | InterfaceAlias |
| MSFT_NetFirewallRuleFilterByInterfaceType | MSFT_NetInterfaceTypeFilter | InterfaceType |
| MSFT_NetFirewallRuleFilterByProtocolPort | MSFT_NetProtocolPortFilter | Protocol, LocalPort, RemotePort, IcmpType, DynamicTransport |
| MSFT_NetFirewallRuleFilterBySecurity | MSFT_NetNetworkLayerSecurityFilter | Authentication, Encryption, OverrideBlockRules, LocalUsers, RemoteUsers, RemoteMachines |
| MSFT_NetFirewallRuleFilterByService | MSFT_NetServiceFilter | ServiceName |

The directions above are verified in the installed filter CDXML. The rule's
CDXML uses the reverse roles when selecting rules from a filter. Microsoft's
[address association](https://learn.microsoft.com/en-us/windows/win32/fwp/wmi/wfascimprov/msft-netfirewallrulefilterbyaddress)
likewise identifies the rule as GroupComponent and filter as PartComponent.
The offline metadata artifact records all eight installed CDXML hashes and the
exact relevant Microsoft SDK read-method declarations; it performs no queries.

The reader must also account for each reviewed filter schema's IsNegated/common
identity fields and all rule security/package/platform/dynamic-keyword fields.
Unknown additions or unsupported non-default filters are refused, not ignored.
Do not equate null, empty, Any, LocalSubnet, environment expressions, aliases,
numeric tokens, ranges or SDDL by convenient string rewriting. An existing inbox
rule can be observed without qualifying its contents as a supported LAN scope.
Two full typed reads, with original rule/key rechecks and reverse associations,
must agree; this detects some drift but is **not** an atomic provider snapshot or
a reservation. Native readers must still use fresh IP Helper/NLM bindings at a
future helper/listener boundary.

The [interface filter documentation](https://learn.microsoft.com/en-us/windows/win32/fwp/wmi/wfascimprov/msft-netinterfacefilter)
mentions LUID or alias representation. That is a useful future investigation,
not proof of an accepted LUID string encoding for this provider's custom option.
The existing verified GUID/LUID-to-unique-alias plan remains unchanged.

## Store and provider uncertainties

Set `PolicyStore=PersistentStore` as a typed custom operation option with
must-comply/must-understand behavior; if its name, transport or provider handling
is unsupported/unknown, refuse rather than silently use a default store. The
installed QueryOptions metadata supports the intended option, but does not prove
the direct MI encoding, projection behavior, store isolation or required-option
handling. Those are explicit read-only qualification gates, including a separately
reviewed unsupported-required-option negative if useful; no malformed store value
that might trigger GPO/network lookup will be generated.

Microsoft's [rule cmdlet](https://learn.microsoft.com/en-us/powershell/module/netsecurity/get-netfirewallrule?view=windowsserver2025-ps)
distinguishes PersistentStore from effective merged ActiveStore and documents
same-name merge precedence. A PersistentStore match therefore cannot establish
effective admission or bypass administrator/GPO blocks. ActiveStore comparison
is separate work with its own identity and privacy contract, not automatic
store fallback. The seven-filter relationship and full native keys must be proven
on the exact current provider before any mutation design consumes them.

## Privacy, bounds and lifecycle

The probe never emits raw rule/filter instances, object paths, keys, machine names,
application/service names, paths, addresses, SIDs/SDDL, interface aliases, localized
display strings, provider error messages or their unsalted hashes. It reads only
fixed named fields; projection may limit requested data, but is not a promise
that the provider will not internally materialize a full instance. All copied key
and filter values stay in process memory, then are discarded. The probe does not
write raw dumps; OS crash-dump behavior is outside this qualification's guarantee.

Evidence contains class/field/type identifiers, bounded counts, numeric MI result
codes, per-read operation status and booleans for exact ID/key/association/type/
two-pass agreement. It must state `owned=false`, `activationAvailable=false`,
`atomicSnapshot=false`, and `mutationAuthorized=false`. It may identify the
public source-fixed selector, not expose an automatically discovered rule list.
No automatic support-upload entry is added. Future protected owned-rule receipts
need public cleanup identity, but foreign qualification values are not receipts.

Proposed bounds: one selected rule; seven filter families; at most four processing
results per operation before cancel/drain; 64 properties per instance; 32 array
elements; 1,024 UTF-16 units per string; 16 KiB per copied field; 64 KiB total copied
values and 64 KiB sanitized output. Overflow/null/type/partial/duplicate errors
discard partial success; no truncation or retry. Metadata classes require their
own explicit count/schema bounds before instance decoding.

Keep each stage a separate externally bounded child: key round-trip uses a
five-second cumulative context, ten-second test timeout and 15-second external
deadline. The larger filter stage proposes eight seconds cumulative, 12-second
test timeout and the same 15-second external deadline, clamping each operation's
MI timeout to remaining budget. No post-deadline success is accepted. Native
calls/cancel/drain/Close can still block: copy borrowed results before advancing,
cancel once, discard further results until terminal, then close in ownership
order. Only the external process owner supplies a hard bound, as in the qualified
schema probe. No goroutine is abandoned or late mutation settlement claimed.

## Proposed implementation ownership and acceptance

Next source ownership, after root review: new instance/read/normalization/test
files under `internal/lanprovider`, plus a new isolated header-ABI/qualification
artifact. Reuse the qualified local-session/ownership code without editing its
frozen files initially; add explicit typed read-only tables verified against a
new header signature/offset probe, never call numbered forbidden slots. If safe
reuse requires changing a frozen type, stop and propose that exact extension.
No new dependency, SDK main/build-list/manifest/pin change or provider mutation
binding is part of this proposal.

Tests must cover literal/projection/option injection refusal, unknown stores,
native key flags/types/null versus empty, duplicate selection/association, borrowed
result overwrite, reverse-link mismatch, every extra filter, array/string budgets,
two-pass drift, cancellation/terminal drain/cleanup failures and secret-free
diagnostics. Root reviews the exact executable/hash/child environment before any
new metadata or instance query. Absence or unsupported direct MI behavior remains
an honest open positive gate. Even a successful read cannot authorize adoption,
prove exclusive create/delete or complete P2-LAN.

## Source-only stages 1 and 2 implementation

The new `instance_read*.go` files implement only the fixed 14 filter/association
class metadata reads and the exact rule-key round-trip. The original schema
bridge and its tests remain byte-for-byte unchanged. The fixed query requests
InstanceID and the four actual inherited native keys; a provider may return the
five projected fields or the full qualified 39-property instance. Full instances
are inspected for field names, types and key/null flags, but value outputs remain
NULL for every unselected field. Unknown counts, names, flags, types, duplicate
results, NULL identity values and cleanup failures refuse partial success.
Empty identity strings remain distinct from NULL. The five copied identity
strings are bounded to 512 UTF-16 units each and never serialized or logged.

Each public new reader requires a cumulative deadline before opening native
state. Both initial probes use context 5s / test 10s / external hard limit 15s.
Each dispatched new read clamps the MI timeout to the remaining context budget.
The fresh rule-schema read reuses the frozen original bridge, whose MI timeout
is 5s; context does not preempt any synchronous native call. Its result must
complete within the shared context, and the parent hard deadline remains
essential. Cancel/positive terminal drain/Close preserve the original ownership
contract. Process interruption is not evidence of completed in-process cleanup.

The four-key local MI instance is constructed with MI_Application_NewInstance
and MI_Instance_AddElement and destroyed after the read operation settles.
These are in-process argument allocation APIs, not provider Create/Delete. No
provider mutation function table is exposed. The PersistentStore option uses
MI_STRING, mustComply true, flags zero and NewOperationOptions custom options
must-understand true. This fixes the intended request; direct provider acceptance
and store isolation still require root's reviewed read-only native evidence.

`artifacts/qualification/p2-lan-instance-source/qualify-source.mjs` compiles C11
signature assertions against the exact Microsoft SDK 10.0.22621.0 header, extracts
named declarations and offsets, runs package unit/race tests and vet, cross
compiles Linux, and produces a Windows test executable without running any native
opt-in. In particular the verified MI_InstanceFT GetElement/GetElementAt offsets
are 88/96; SetElementAt occupies the otherwise easy-to-miss preceding slot.
The script checks the frozen original bridge and current SDK/pin guards before
and after. Its proof binds source, header, installed compiler and output hashes.

The ignored native tests are `TestMIFilterSchemasReadOnlyNative` and
`TestMIRuleKeysReadOnlyNative`, enabled separately by
`ORBWIN_QUALIFY_LAN_MI_FILTER_SCHEMA=1` and
`ORBWIN_QUALIFY_LAN_MI_RULE_KEYS=1`. A not-found exact selector is reported as
not-found with no round-trip claim. Even an observed stable round-trip reports
owned/activation/atomic snapshot/mutation authorization false. Filter metadata
reports schemaReviewed false until the actual 14-class output is independently
reviewed. The two stages have not queried the provider at source qualification
time; filter instance traversal, reverse association checks, typed filter value
decoding, atomic creation/removal and all product integration remain open.

## First root-run observations and diagnostic increment

Root ran the exact first executable SHA256
`5e9069b455c8ac5e96d7c746739655598d5a3240c8029f06a900e31d4318185f`
with each reviewed opt-in in a separate minimal four-key environment and external
15s bound. The 14-class schema observation passed in 246ms:
`artifacts/qualification/p2-lan-filter-schema-native-97f5b7b6-f2a4-45cd-83a5-58f1efa0223a/proof.json`.
The exact rule-key observation failed honestly in 713ms at instance metadata
validation:
`artifacts/qualification/p2-lan-rule-keys-native-3fe2051e-3818-454f-8fb7-3eab5e7397f0/proof.json`.
No private instance values were emitted. This failure does not establish which
metadata differs or prove a key round-trip or policy-store isolation. It remains
a failed gate, with the original source, executable and proof retained in
`artifacts/qualification/p2-lan-instance-source/v1-preserved/` before any edits.

The next source increment changes no accepted instance flags, types or selection
policy. On the same mismatch it adds only index, numeric type/flags, duplicate
boolean, expected type/key boolean and a property name only when the name exactly
matches the existing 39-property contract. Every unknown native identifier is
replaced by `unknown` before being stored in the diagnostic. It accepts no
instance value, key value or provider error message. Pure tests cover malicious
unknown names, JSON and text formatting and all known expected contracts. Root
must separately review and run the new exact executable before the failed
metadata gate can be reassessed; the agent performs no new native query.

The observed public filter class metadata is preserved in
`internal/lanprovider/testdata/qualified-filter-metadata.json` (SHA256
`d2faeca5614803dadd6c07b2c81c251fc6c2686e7790b5cdd9d098b284ad65ca`).
The new pure `validatePinnedFilterSchema` is a proposed fresh schema gate for
stage 3; it is deliberately not wired to change the stage-1 `schemaReviewed`
claim yet. The fixture is independently derived from the root-run stdout hash,
not generated from the validator. All seven filters expose 18 common fields and
four actual keys named Name, CreationClassName, SystemCreationClassName and
SystemName; these are different from the rule's four keys. Family field counts
are 20/20/19/19/23/24/19; all seven associations expose exactly the two keyed
reference ends in the reviewed direction. Only the address class has method
metadata (`QueryIsolationType`, MI_UINT32 return); it is not invoked.

The metadata gate includes common identity/health/status/IsNegated fields and all
family-specific fields, including Package, DynamicTransport and all security
filters. Protocol is a string; InterfaceType and DynamicTransport are MI_UINT32;
InterfaceAlias and port/address fields are string arrays. These are observed
types, not normalization rules or evidence that NULL/empty/default instances
mean an unrestricted filter. Exact declaration flags/default absence are pinned
as class metadata, separately from the still-unresolved instance getter flags.
Tests reject extra filters/properties, changed negation/key/protocol/security
metadata, unexpected methods and reversed reference ends. No values, forward or
reverse association traversal or admission semantics have yet been qualified.

## Declaration flags versus instance field state

Root's second exact selector run (`a19f2b1a...` executable) also failed honestly
in 528ms, in
`artifacts/qualification/p2-lan-rule-keys-diagnostic-native-18daf16c-9f7b-4a7b-b077-9b1e242f37d9/proof.json`.
Its sanitized diagnostic identified index 0, known property Caption, MI_STRING
type 13 and flags `0x22200004`, with expected type 13/non-key/no duplicate. The
exact diagnostic sources, fixture, executable, proof, ABI and test outputs were
copied and hash-verified into `p2-lan-instance-source/v2-preserved/` before edits.
Both failed native receipts remain failed evidence.

The exact local Microsoft SDK header defines this combination as PROPERTY
`0x00000004`, READONLY `0x00200000`, NOT_MODIFIED `0x02000000` and NULL
`0x20000000`. NOT_MODIFIED describes field modification state, not permission,
ownership, a provider lease or an atomic read. The [getter documentation](https://learn.microsoft.com/en-us/windows/win32/api/mi/nf-mi-mi_instance_getelementat)
lists a narrower NULL/KEY/IN/OUT set; it does not explain every observed bit.
Microsoft's [field documentation](https://learn.microsoft.com/en-us/windows/win32/api/mi/ns-mi-mi_constreferencefield)
also identifies NOT_MODIFIED as field state. These sources are recorded with the
exact header constants rather than treating the short getter list as complete.

Microsoft's public [OMI GetElementAt implementation](https://github.com/microsoft/omi/blob/360b9b62c32d35c44d4267dd24e25b69802e2ae2/Unix/base/instance.c#L1886-L1974)
at immutable commit `360b9b62c32d35c44d4267dd24e25b69802e2ae2` retains declaration
flags excluding NULL/NOT_MODIFIED, then reconstructs those two bits from the
field's existence/modification state. Its named getter delegates to this indexed
getter. This corroborates the interface semantics and observed combination;
**it does not establish the implementation of Windows' installed MI binary**.
The original source copyright and same-commit MIT license accompany the isolated
evidence copy; none is linked into the product or added as a dependency.

The next isolated source increment captures all 39 properties from the fresh
qualified GetClass result before opening the instance session. For every returned
field, type and all declaration bits must equal that exact fresh declaration
after excluding only NULL and NOT_MODIFIED (`0x22000000`) on each side. Those two
field-state bits may vary; no union of documented flags is accepted. Changes to
PROPERTY, READONLY, KEY, qualifiers, versions, IN/OUT, BORROW/ADOPT or any unknown
bit therefore fail unless the immutable declaration matches the already-qualified
fresh schema exactly. A declaration itself still must pass the original complete
schema gate; instance state cannot introduce a new accepted class flag.

NULL InstanceID or any of the four keys still fails before reading its value.
Empty non-NULL strings remain distinct copied values and must match both
round-trips exactly. NOT_MODIFIED never qualifies a foreign rule as owned or
authorizes any mutation. Named/indexed metadata still must agree exactly during
each copy. Pure tests exercise every non-state bit flip for all 39 properties,
NULL/non-NULL combinations, empty keys, duplicates, type/key/readonly/qualifier
changes, missing fresh declarations and unknown schema fields. C11 evidence now
also binds the exact flag constants from the header. This semantics change is
source-tested and compiled only until root reviews and runs the new exact probe;
the rule-key, direct-store and stage-3 traversal gates remain open.

Root subsequently reviewed and ran the exact declaration-state executable
`17e86266087ac5eb623f3a02347a213192a21602b90f037bfa5de0fa3fdb5b63`.
The bounded read **passed in 802ms**, with all 56 source/guard hashes checked:
`artifacts/qualification/p2-lan-rule-keys-declaration-native-18edf0b5-e88d-4c86-8b83-cc74f24c5817/proof.json`.
The exact fixed selector was found; all four observed keys round-tripped through
two native GetInstance reads with exact InstanceID/key equality. No private
values were emitted. Owned, activation, atomic snapshot and mutation flags all
remained false. This qualifies that exact bounded read on this provider at that
time; it does **not** prove universal honoring or isolation of PolicyStore, atomic
policy observation, foreign-rule ownership, admission semantics or stage-3
association traversal. The exact passing source/proof/executable is retained in
`p2-lan-instance-source/v3-preserved/` before stage-3 source work. The prior two
failed receipts remain part of the qualification history.

An evidence-only correction changed the MI_FLAG_IN macro lookup to require an
exact token boundary, avoiding the MI_FLAG_INDICATION prefix. The old evidence
is retained in `p2-lan-instance-source/v3-before-exact-macro-correction/`; no
source or executable changed for that correction.

## Source-only stage 3: full typed seven-filter traversal

The next increment is isolated in eight new `filter_traversal*.go` and
`typed_instance*.go` files. The exact passing v3 files and original schema bridge
remain unchanged. Before reading instances it freshly checks all 14 pinned
filter/association class declarations and the complete rule declaration. A
separate ignored schema-only probe lets root review that gate before any new
association call. No new public API accepts a caller's query, store, class,
selector, role, provider method, credential or destination.

The traversal selects only the same fixed rule, captures its actual four keys,
then reads the full typed 39-property rule. Each of two passes reads exactly one
full filter per family using fixed association/result class and GroupComponent
to PartComponent roles, then follows that filter's actual four keys through the
reverse roles and requires the original InstanceID plus all four rule keys.
No missing association is inferred to mean an unrestricted default. Ambiguous,
missing, unsupported, null-key, changed-schema or changed-identity results fail.
After each pass the full typed rule is read again and compared with the initial
rule. All seven full typed filters must agree across passes. There are at most
32 dispatched instance operations and 15 prior schema reads; no retry or broad
enumeration is added. A new read checks/clamps its timeout against the shared
context. Native result draining still requires positive terminal consumption;
the processing budget does not bound a provider's cancellation-drain calls, so
the external one-shot deadline remains essential.

Every returned field's type/declaration flags must match the freshly qualified
class, separating only the already-qualified NULL/NOT_MODIFIED state bits. The
private value representation covers all current fields: MI_BOOLEAN, MI_UINT16,
MI_UINT32, MI_DATETIME, MI_STRING, MI_UINT16A and MI_STRINGA. Unknown kinds fail
even when NULL. Booleans/discriminators, array counts/pointers, UTF-16 validity,
field counts, duplicate fields, named/indexed metadata and all identity fields
are checked before retaining values. Array order, NULL versus empty, literal
tokens, signed UTC offset and every logical timestamp/interval field are
preserved. Native interval padding is ignored; values are never normalized into
equivalent expressions or interpreted as admission policy.

Both full copies and selector/reverse identity-only copies share one cumulative
64 KiB logical payload budget, with 16 KiB per field, 1,024 UTF-16 units per
string and 32 array elements. Identity-only reads check all returned metadata
but request values only for the five identity fields. Metadata is separately
bounded by the pinned schema. The payload budget is not a Go heap/working-set
guarantee: temporary conversion buffers and fixed Go container overhead remain,
and this probe does not impose a host process memory limit. Every accepted
payload is bounded without truncation; all copied values remain private RAM
data and cannot become JSON receipts. Error context uses only fixed phase and
family numbers; no native key/value, unknown identifier, provider message,
application path, address, SDDL or data hash is emitted.

New native ABI evidence is limited to the exact named
MI_SessionFT.AssociatorInstances signature/offset and MI array/date structures.
The compile-only header check caught an initial local MI_Datetime draft before
any provider query: the exact SDK defines sizeof 36 and union offset 4, while
the enclosing MI_Value remains 40 bytes. That failed draft/proof is preserved
under `p2-lan-filter-traversal-source/before-datetime-layout-correction/`; the
corrected C and Go layout assertions agree. The code constructs/destroys local
MI key arguments, never invokes provider Create/Delete/Modify/Enable methods,
and closes operations before their owned arguments and session.

`artifacts/qualification/p2-lan-filter-traversal-source/qualify-source.mjs`
binds exact new source and compiler/header signatures, verifies frozen v3 and
original bridge sources plus SDK/pin guards, runs pure/race tests and vet,
cross compiles Linux and compiles a new Windows probe without native opt-ins.
The staged tests are `TestMIFilterTraversalSchemasReadOnlyNative`
(`ORBWIN_QUALIFY_LAN_MI_TRAVERSAL_SCHEMA=1`, context 5s/test 10s) and
`TestMIFilterTraversalReadOnlyNative`
(`ORBWIN_QUALIFY_LAN_MI_FILTER_TRAVERSAL=1`, context 8s/test 12s). Each requires
root review of the exact executable/hash and a separate external 15s deadline.
Neither new gate has been queried by the agent. Complete two-pass equality, if
later observed, still reports owned/activation/atomic/mutation flags false; it
cannot establish exclusive ownership, effective admission, atomic policy state,
universal store-option honoring or product readiness.

Root reviewed the exact stage-3 executable
`fe5e0e79b6a993ee2fd783434de7d7a9dc5ef05e93e7404b9b506ff037947c20`
and ran the two gates separately. The fresh schema gate passed in 205ms:
`artifacts/qualification/p2-lan-filter-traversal-schema-native-d3585947-c312-4da1-92ab-3d70d630a58f/proof.json`.
The bounded traversal then passed in 3,516ms:
`artifacts/qualification/p2-lan-filter-traversal-native-b009a985-d815-4e36-8014-37210765e7c4/proof.json`.
All 64 source/guard records were checked. The receipt reports seven families,
two passes, 14 forward and 14 reverse reads, three full-rule reads, full typed
stability and matching reverse identity. No private values were emitted; all
ownership/activation/atomic/mutation flags remained false. Exact sources,
executable, proof, ABI and test outputs are preserved in
`p2-lan-filter-traversal-source/v1-preserved/` before further work.

This is actual qualification of the bounded foreign-rule observation, not of
mutating or owning that rule. Effective policy, universal store honoring,
exclusive create, late mutation settlement and retirement remain separate
gates. The next provider comparison is recorded in
[lan-wfp-alternative.md](lan-wfp-alternative.md); the CIM diagnostics remain
useful whichever mutation boundary is eventually selected.
