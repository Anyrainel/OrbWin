# Read-only native LAN firewall schema bridge

Source-only increment, 2026-10-25. `internal/lanprovider` is separate from the
attested SDK entrypoints and existing observers/protocol. It has no Create,
Delete, Add, Modify, Enable, Invoke, instance enumeration, arbitrary query,
command execution, elevation, protected-store or installation API. LAN exposure
and atomic-create qualification remain false.

## Native contract and ABI evidence

The exact locally installed Microsoft SDK is `10.0.22621.0`. Its
`um/mi.h` SHA-256 is
`c0c43bbda10b59109484ec0d5569c2cd330bf1e81ce73f4182cf8fd6434492b3`.
The version-specific C probe at
`artifacts/qualification/p2-lan-mi-abi-source/layout.c` includes that header and
emits constant `sizeof`/`offsetof` values into assembly/object files using the
installed MSVC x64 compiler. It is compiled only, never linked or executed.
Its C11 type-compatibility assertions check argument order, widths and pointer
shapes for InitializeV1/NewSession/session and operation GetClass, Class
GetElementAt/GetMethodAt and ParameterSet GetMethodReturnType against the actual
header, not just table offsets.
The probe is an observed header/compiler correspondence, not a pinned compiler
distribution or an MI provider test. The evidence proof records their hashes.

Windows amd64 handle sizes are 24 bytes, with the function-table pointer at byte
16, for application/session/operation/options/qualifier/parameter handles.
`MI_Class` is 64 bytes with its table pointer at byte zero. `MI_Value` is 40
bytes aligned to eight; `MI_Interval` is 32 bytes, seconds at 12 and microseconds
at 16. `MI_Boolean` is one byte, `MI_Char` two, and `MI_Type` four. The initial
export uses `MI_MAIN_CALL`; table members use `MI_CALL`. Windows amd64 uses the
same native calling convention for these forms. No x86/arm64 ABI is promised.

Relevant table byte offsets are:

| Table | Fixed members used |
|---|---|
| Application | Close 0, NewSession 8, NewOperationOptions 40 |
| Session | Close 0, GetClass 96 |
| Operation | Close 0, Cancel 8, GetClass 40 |
| Class | GetClassName 0, GetNameSpace 8, GetElementCount 24, GetElementAt 40, GetClassQualifierSet 48, GetMethodCount 56, GetMethodAt 64 |
| QualifierSet | GetQualifier 16 |
| ParameterSet | GetMethodReturnType 0 |
| OperationOptions | Delete 0, SetInterval 88 |
| Instance | Delete 16, only for owned initialization/session error instances |

These are MI function tables, not COM IUnknown/IDispatch tables. Native argument
widths and borrowed-result lifetimes follow the exact header and Microsoft's
[GetClass](https://learn.microsoft.com/en-us/windows/win32/api/mi/nf-mi-mi_session_getclass)
and [GetClass-result](https://learn.microsoft.com/en-us/windows/win32/api/mi/nf-mi-mi_operation_getclass)
contracts. The syscall wrappers carry `go:uintptrescapes` so Go output buffers
cannot move or die while their integer-form addresses cross a Go wrapper.

## Fixed observation and ownership

`ObserveSchema(ctx)` is for an externally bounded one-shot process only. It loads
System32 `mi.dll` and fixed `MI_Application_InitializeV1`, then creates a local
session with **NULL protocol, destination, destination options and callbacks**.
It supplies no remote host, credentials, proxy, impersonation or caller-selected
class. The only session operation is callback-free `GetClass` with flags zero,
namespace `root/StandardCimv2` and class `MSFT_NetFirewallRule`. A five-second MI
operation timeout is set using the exact inline-header SetTimeout translation
to `SetInterval("__MI_OPERATIONOPTIONS_TIMEOUT", ...)`. Microsoft documents
[NULL destination/protocol](https://learn.microsoft.com/en-us/windows/win32/api/mi/nf-mi-mi_application_newsession)
as a local WMIDCOM session.

Before opening MI, Go reads only the fixed installed Microsoft NetSecurity
CDXML under `GetSystemDirectory()`. It refuses links/nonregular or oversized
files; XML directives, foreign namespaces, excessive nesting/tokens, duplicate
or changed create metadata are refused. The reader checks the exact 36 captured
New parameter names/types and `cim:CreateInstance`, retaining the actual file
SHA. It does not execute the file or claim to validate all CDXML semantics.
Lstat/ancestry checks followed by Open are a metadata observation, not an atomic
trusted descriptor or protected-store ownership proof. This path is not reused
as privileged receipt authority.

The native result copies the class name/namespace, `provider=wfascim`, the exact
39 property names/types/key/readonly classifications, and five method names/
UInt32 return types. Unknown flags, missing/extra/duplicate/case-aliased metadata
fail closed. Raw MI flags and default-presence observations remain distinct from
the earlier cmdlet CIM flag strings. Metadata flags are not mutation authority.
The method names are inspected; none of those methods is invoked.

The native operation owns borrowed class/error-detail pointers until its next
result or Close. All needed fields are copied before advancing; borrowed classes,
qualifiers, parameter sets and result errors are never independently deleted.
Owned extended-error instances from initialization/session creation are deleted.
The one thread owns operation, options, session and application lifetimes; normal
cleanup closes the completed operation before deleting options and closing the
session/application. Every error discards partial successful schema output.

Class parsing is bounded to 64 properties, 16 methods and 128 UTF-16 units per
metadata string, with invalid/unterminated UTF-16 refused. The expected contract
requires exactly 39 properties/five methods. Output is capped at 64 KiB. Only one
class result is accepted, and more than four processing results is an error.
After cancellation/schema/read/budget failure, cancel is requested once and
remaining results are discarded, without growing buffers or parsing more class
data, until positive terminal `moreResults=false` permits Close.

**There is no in-process hard cancellation guarantee.** Synchronous GetClass,
cancel, draining and Close may block. Microsoft explicitly requires final result
consumption even after Cancel; premature Close can hang. A provider that never
settles can therefore keep the cleanup owner blocked. The external process owner
must terminate that one-shot process on its hard deadline. No detached goroutine,
successful partial snapshot, early Close or cancellation-equals-settlement claim
is used. This read-only lifecycle is not a proof about late firewall mutations.

## Typed create plan and unresolved provider semantics

`PrepareDisabledCreate` reuses the unchanged `lanfirewall` receipt/interface
validation and produces an opaque copied MI-specific plan, without dispatch or
generic property/option bags. It retains disabled/private-or-domain/inbound/allow
scope, exact program/address/port/interface/client CIDRs, PersistentStore and
block-override false. Required custom options are marked must-comply and the
future operation-options configuration must understand them. This slice does not
set them on a native handle or submit them to a provider.

The existing WMI plan's flag 2 remains **WBEM_FLAG_CREATE_ONLY**. It is not an MI
RTTI flag. The new MI plan uses RTTI flags zero: [MI CreateInstance](https://learn.microsoft.com/en-us/windows/win32/api/mi/nf-mi-mi_session_createinstance)
documents refusal of an existing instance. This is API-level evidence only;
wfascim concurrency, required custom-option handling and all-or-nothing filter
creation remain unqualified. Ordinary COM Add/upsert is never a fallback.

The captured schema marks InstanceID readonly but not a key; CreationClassName,
PolicyRuleName, SystemCreationClassName and SystemName are keys. Therefore
fail-if-existing by actual CIM keys does **not yet prove** exclusive creation by
the planned InstanceID or safe deletion by that ID alone. Native proof must
establish the exact InstanceID/full-key mapping and preserve the incumbent under
concurrent duplicates. Exact normalized rule and associated-filter readback,
effective-policy/GPO authority, protected receipt ownership, mutually authenticated
one-shot IPC/elevated exact-scope review, late-completion settlement, enable/remove
and dormant cleanup after adapter/worker loss remain separate required work.
The existing IP Helper GUID/LUID and NLM binding guards remain authoritative;
this schema reader neither observes interfaces nor substitutes aliases for them.

## Tests and reviewed native probe gate

Local tests cover changed schema/types/key/readonly/unknown flags, missing or
broadened custom-option metadata, borrowed-result overwrite, partial-read errors,
duplicate/result overflow, cancellation draining, cleanup failures and exact
resource closure order. ABI/UTF-16 tests make no MI provider calls. The copied
test fixture is the prior metadata-only qualified CIM/CDXML observation, not
evidence that the new native bridge already works.

`TestMISchemaReadOnlyNative` is ignored unless the child-only environment sets
`ORBWIN_QUALIFY_LAN_MI_SCHEMA=1`. The source qualification prepares the exact
Windows test executable, SHA and proposed command for root review. Root must
enforce an external hard wall-clock deadline in addition to the five-second
context and test timeout. **No real MI provider query has been run for this
increment.** A failed/unsupported native schema is a refusal, not a fake pass.
No SDK main, module dependency, build/attestation list, observer, firewall protocol,
trust helper or product activation changed.
