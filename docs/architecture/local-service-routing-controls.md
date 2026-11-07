# Exact local-service routing controls

Implemented 2026-10-25. This is the OrbWin adaptation for the P2-NAM addendum
and D08. It extends the constrained-signer design in `local-services.md`; it
does not change rdd ownership, trust stores, workload networks or global DNS.

## Public contract and precedence

Only `dev.orbwin.*` labels are read. `dev.orbstack.*` labels are ignored, so a
project prepared for another product cannot silently authorize OrbWin routes.
Names must be canonical lowercase exact ASCII names under `.orbwin.localhost`
or `.orbwin.test`. Wildcard patterns, arbitrary domains and IP literals refuse
the routing choice. Certificates remain exact, lazily issued leaves. Remote
icons are not fetched. These choices resolve D08 without weakening the signer.

- `dev.orbwin.http-port=8080` chooses the container's plain HTTP listener.
- `dev.orbwin.https-port=8443` chooses its TLS HTTP listener. Supplying both port
  labels is ambiguous and refuses the candidate. A port is decimal 1–65535,
  without whitespace, signs or leading zeroes.
- `dev.orbwin.domains` supplies comma-separated exact names, optionally with
  per-name `@port` or `@port:https`. For example:
  `app.orbwin.localhost@8080,admin.orbwin.test@8443:https`.
  Surrounding entry whitespace is accepted; name spelling is never normalized.
  A bare name uses the selected port label or the one discovered TCP listener.
  An explicit `@port` selects plain HTTP; `:https` explicitly selects TLS.

The domains label replaces automatic names. It does not implicitly add a
counterpart in the other namespace. Without a domains label, a port label keeps
the existing automatic names, including both client namespaces. Each workload
may explicitly select up to sixteen names; the existing controller limits of
sixteen active targets and 256 total names still apply. Duplicate names, including
duplicates with different ports, refuse the choice. Cross-workload collisions
remain visible and neither claimant gets a route. Malformed labels produce a
visible `routing-error` instead of falling back to an unrelated listener.

Settings accept `routes: [{name, port, https?}]`. The UI edits the equivalent
one-entry-per-line syntax. Explicit UI routes replace all label routing choices
atomically, including malformed labels. Existing name/port preferences remain
compatible and also replace label choices; mixing those legacy fields with
`routes` is refused. Empty UI fields use labels/discovery. Exact-container UI
preferences retain priority over coherent Compose-replica preferences. Disable
takes priority and removes the selection. All preferences remain guarded by the
existing settings revision transaction.

Public plain HTTP redirects to the configured public HTTPS port. The public
HTTPS endpoint always uses the constrained OrbWin signer. Upstream HTTPS is a
separate TLS connection over the same pinned namespace tunnel: normal Windows
system trust, TLS 1.2 minimum, exact selected-name verification/SNI and a bounded
handshake. There is no verification bypass or HTTP fallback. A self-signed,
untrusted, expired or wrong-name backend remains unavailable with the probe
failure visible. No private-CA import/configuration is added by this change.
Apps using upstream TLS must already present a certificate trusted by the host
and valid for every selected upstream name.

## Ownership, concurrency and failure behavior

Discovery remains pure and read-only. It publishes per-name port/protocol choices
alongside existing names/selectedPort fields. The controller validates the whole
snapshot before expanding it into independent listener groups. Plain HTTP
aliases sharing a port share a probe; TLS names are probed separately so each
certificate/name association is verified. Each group has its own retry key;
all groups share the exact target incarnation's one owned namespace helper.
Target inspection is deduplicated before bounded parallel reads. Status exposes
one row per listener group; the UI renders every group rather than treating the
first active listener as success for the entire workload.

Port, protocol, name, incarnation or helper changes retire old transports before
probing replacements. Cancellation closes tracked namespace streams, including
TLS/HTTP upgrades. Failed listeners do not suppress independently verified ones.
A helper is retained only while at least one listener remains verified. Existing
ownership journals, renewal bounds, resource limits and exact cleanup checks are
unchanged. No new executor, credential source or unbounded retry loop is added.

## Forwarded headers and selected networks

The upstream Host remains the validated public request Host, including its
explicit public port. The proxy removes caller `Forwarded`, all
`X-Forwarded-*` fields and `X-Real-IP`, then generates `X-Forwarded-Host`,
`X-Forwarded-Proto` and `X-Forwarded-For` through the standard proxy routine.
For proxied public requests the proto is `https`, including when the internal
listener uses plain HTTP. No Forwarded, X-Real-IP, X-Forwarded-Port or
X-Forwarded-Prefix value is passed through. X-Forwarded-For describes the peer
seen by the Windows route process. The opaque relay does not preserve an
end-to-end original client address; applications must not treat that address
as a proven remote-client identity or authorization decision.

Selected client networks grant access to **all** active services, including
services in another Compose project/network. The relay receives only active
exact `.orbwin.test` aliases on the reviewed user-defined bridge network IDs.
Workload networks and trust stores are not changed. Removing a network selection
removes that relay attachment through the existing reviewed transaction. This is
network-level reachability, not per-project authorization or workload isolation.
Container clients still need explicit appropriate public-root trust for HTTPS.

## Evidence and remaining acceptance gates

`go test ./localnet -count=1` and `go vet ./localnet` pass on Windows. New tests
cover malformed/duplicate/wildcard/foreign names and ports, ambiguous labels,
foreign product labels, UI precedence, independent listener failure, shared
helper/inspection, stale-port retirement, default untrusted TLS refusal,
accepted scoped test trust, wrong-name TLS refusal and spoofed forwarding fields.
The accepted scoped test trust is an in-memory test client pool; no host trust
was installed. Existing package tests cover route replacement, cancellation,
upgrade retirement, ownership journals, stale daemon/target and client networks.
Scoped Windows race tests also pass for the routing controls, controller and
verified TLS upgrade replacement (`go test -race ./localnet -run
'TestRouting|TestController|TestVerifiedTLSUpgrade' -count=1`, bundled UCRT GCC,
CGO enabled, GOMAXPROCS=2).

These are component results, not installed qualification. Required integration
gates remain: native rendered UI/keyboard states at supported sizes/scaling,
rebuilt attested SDK/desktop, labelled recreation/label removal, simultaneous
multiple HTTP/TLS listeners, HTTPS system-root/expiry chains, Windows/WSL/client
network routing, cross-project selected-network access/removal, live stale-route
and concurrent-connection retirement, three-engine coexistence, restart/installed
preservation and resource measurements. Root trust writes and machine network
changes remain deferred for an attended session.
