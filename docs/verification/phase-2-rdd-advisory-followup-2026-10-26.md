# Windows backend advisory source follow-up

Recorded 2026-10-26. Read-only compiler metadata, source and binary-data
inspection; no backend, guest, workload or native fixture was started. This
supplements the preserved [earlier triage](phase-2-go-advisory-triage-2026-10-25.md).

Evidence is under
`artifacts/qualification/p2-rdd-advisory-source-followup-20261026/`.
`source-observations.json` records every examined source hash, package/module,
matching source line and compiler inputs. The pinned Go1.27.3 compiler enumerated
the current selected Windows `./cmd/rdd` production closure offline with
`-mod=readonly`, `GOENV=off`, `GOWORK=off` and CGO disabled: 3,040 packages and
13,106 Go source files. Every source was rehashed after inspection, as were
go.mod/go.sum. Lexical-reference enumeration is not a reconstructed call graph.

The independently read/reviewed standard-library PCLN parser then inspected the
exact packaged rdd PE as bytes, never dispatching it. Hash:
`0896047decdfa7c851870e80b5edc51439f243b8baa522379b4933c60e15c8f6`.
It validated the host table through its moduledata pointer pair, text bounds
and `runtime.main`, avoiding embedded guest ELF tables. It found 182,225 actual
functions and 203,104 function-name entries, including stored inline names.
The parser source, compiler/PE hashes, three filter additions, output and receipt
are retained. This is function-name inspection, not arbitrary data-flow proof.

## CEL native type-provider finding

The current [GO-2026-6094](https://pkg.go.dev/vuln/GO-2026-6094) record describes
private JSON field exposure through CEL `NativeTypes`/`ParseStructTag` and lists
v0.30.0 as the fixed version. Its affected-range match remains in dependency
metadata; a generic CEL consumer is not evidence of this type-provider use.

Across the current compiled source files, the only references to either symbol
are declarations/comments inside `github.com/google/cel-go/ext` itself. No
compiled consumer references them. The exact packaged host binary's validated
function-name table contains neither affected function, including their named
closures. These independent observations support **affected implementation not
present in the examined Windows executable** for this finding. They do not
establish safety of every CEL operation, other binaries, guest components or
future feature/build selections.

## Raw DHCP client finding

[GO-2026-6237](https://pkg.go.dev/vuln/GO-2026-6237) concerns a short-payload
panic in `dhcpv4/nclient4.BroadcastRawUDPConn.ReadFrom`; the record fixes it at
`v0.0.0-20260719225207-c76316d4aa82`.

The exact current Windows production graph does not contain the `nclient4`
package. A test import recorded in earlier dependency enumeration is not a
production dependency. Its vulnerable raw implementation is also Unix-tagged.
The exact packaged binary has no function-name entries from `nclient4`.
Disposition for that Windows executable: **affected package not present**.
This does not cover DHCP servers, BusyBox `udhcpc`, other guest programs, Linux
builds or general malformed-network-input handling.

## Etcd affected implementations and final-candidate boundaries

The same parser positively finds twelve transport and 266 etcdserver-related
function names, so package-absence reasoning cannot dismiss those findings.
The primary [TLS-listener advisory](https://pkg.go.dev/vuln/GO-2026-6107) names
`NewListener`, `NewListenerWithOpts`, `NewTLSListener` and `NewTimeoutListener`.
None is in the examined PE's complete host function-name observation, including
stored inline names. All twelve transport names are client `TLSInfo` methods or
their closures; even a broader `Listener` substring search finds no match.
The [nested-transaction advisory](https://pkg.go.dev/vuln/GO-2026-5736) identifies
`EtcdServer.Txn`. No function name containing the `EtcdServer` type appears in
the examined server-package observation, covering pointer/value receivers,
method wrappers and suffixes. The 266 present names are other implementation,
initialization and generated API/protobuf names, not that affected method.

Together these exact-binary observations support **affected implementation not
present in the examined Windows executable** for each finding. The packages are
present; this is deliberately not a package-absence disposition. The read-only
interpretation, broader boundary checks, exact input hash and actual PE rehash
are preserved as `etcd-symbol-dispositions.json` and
`etcd-symbol-wide-boundary-check-v2.json` alongside the parser receipt. The first
broad check used case-insensitive matching and matched the package name; it is
preserved but superseded by the explicit case-sensitive check. This does not
establish general TLS/RBAC safety, Kine behavior, other binaries or a successor.
Current source prepares a private datastore transport, which requires its own
final build assessment; access control does not repair vulnerable accounting.

The historical source graph used go.mod hash `b87aec4f…`; this fresh graph uses
`25bf5c59…` and go.sum `e9ff6cae…`. Neither source graph alone proves correspondence
to the packaged twelve-patch binary or an eventual authority successor. Exact
binary evidence above supports only its stated package/function observations.
Rebuild, record the final command closure and scan the final candidate before
using these dispositions for release. Guest/cluster SBOM and advisory review,
remaining host/client findings and installed qualification are still required.
