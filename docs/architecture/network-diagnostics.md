# Phase 2 network diagnostics and remaining D01 gates

Recorded 2026-10-25. This covers the bounded diagnostic slice of P2-D01; it does
not resolve proxy configuration, VPN/split-DNS transitions, subnet conflicts or
the full Windows/WSL/container/optional-cluster trust matrix.

## Existing source contract and observed limits

The pinned, patched Windows `rdd/pkg/hostswitch/hostswitch_windows.go` constructs
the `docker.internal.` zone with `host` mapped to the configured static DNS host
address. Its NAT map forwards that address to Windows `127.0.0.1`. Thus
`host.docker.internal` uses Windows IPv4 loopback, not an IPv6-only `::1` listener.
Existing local-service live evidence proves the owned relay-to-Windows gateway
direction, not arbitrary host services or VPN transitions. Container-to-host
`localhost` remains the container/guest network namespace, not a Windows alias.

The pinned gvisor-tap-vsock v0.8.9 DNS service constructs
`net.Resolver{PreferGo:false}` for non-static requests. This reuses host resolver
selection; it does not establish correct corporate split-DNS, NRPT, VPN transition
or offline behavior. Its source uses context.TODO for upstream DNS calls, so a
client timeout is not proof of cancellation inside that server. No replacement
resolver or Windows/WSL DNS setting is introduced here.

`rdd/pkg/socketbridge/docker_ports_windows.go::hostBinding` accepts empty,
wildcard-IPv4 or 127.0.0.1 publication requests and rejects IPv6. Published Windows
listeners are IPv4 loopback. Windows resolution of a `.localhost` name may yield
`::1` first; product/browser success must separately qualify fallback or an owned
IPv6 listener. Container IPv6 and host publication IPv6 are distinct scopes.
The current AppSpec/controller audit still finds no standalone engine proxy
transaction; Lima-template settings are not evidence of a Windows runtime API.

## Working read-only diagnostic

`backend-tools/session/internal/networkdiag` provides
`ObserveLoopback(context.Context) (LoopbackSnapshot, error)` and
`ObserveProxy(context.Context) (ProxySnapshot, error)`. Neither accepts an
arbitrary host, URL, port, proxy server, file path or credential.

Loopback observation owns a temporary `127.0.0.1:0` TCP listener and attempts `::1`
at the same port. Handles stay retained until all probes finish; all listeners
close and workers join before return. It generates a fresh 256-bit nonce and
requires the server to send the exact nonce first, not merely TCP connect success.
The client sends no challenge bytes that an unrelated echo service could reflect.
Listeners bind
only loopback, accept at most 32 connections and four concurrent handlers, with
one-second connection deadlines and cancellation. It sends no HTTP request or
application credentials. A local process can deny service, but cannot manufacture
a valid nonce acknowledgement from an unrelated endpoint.

Only `localhost` and `network-check.orbwin.localhost` are resolved. Answers are
bounded to 16, summarized in resolver order as IPv4/IPv6 labels, and never emitted
as addresses. Malformed, zoned or non-loopback answers withhold name dialing.
Every connect is independently checked immediately before the native connect:
only the exact retained `127.0.0.1`/`::1` listener and port are allowed, including
after DNS changes. If IPv6 bind failed, no data is sent to that family's port,
which another application may own. Other addresses in 127/8 are not treated as
our socket. The report contains no ephemeral port, nonce or raw resolver errors.

`nativeDial` uses Go's native named TCP dialing with a 300 ms fallback delay.
`ipv4OnlyDial` repeats named dialing with the IPv6 destination refused before
connect; it tests Go's fallback to the retained IPv4 socket. It does not simulate
every OS/browser connection-refusal path or qualify browser fallback. The whole
loopback observation has a six-second context bound; individual lookups/probes
have one-second bounds. An external process deadline remains necessary for a hard
wall-clock bound on synchronous native APIs or resolver implementation failures.

Windows proxy observation calls only
[WinHttpGetIEProxyConfigForCurrentUser](https://learn.microsoft.com/en-us/windows/win32/winhttp/winhttp-autoproxy-api)
and [WinHttpGetDefaultProxyConfiguration](https://learn.microsoft.com/en-us/windows/win32/api/winhttp/nf-winhttp-winhttpgetdefaultproxyconfiguration).
The former is current interactive-user configuration; the latter is WinHTTP's
registry default, not a current session's effective configuration. It reports
automatic detection/PAC/manual/bypass presence and a configured mode. It inspects
only the first WCHAR of returned strings to distinguish null/empty from present;
URLs, hostnames, bypass entries, PAC contents and credentials never become Go
strings or JSON. All returned allocations are released with GlobalFree, including
failure paths. Failed/unavailable reads remain unknown, never inferred direct.
Effective proxy is always `not-evaluated`. No WPAD discovery, PAC fetch/evaluation,
proxy session, environment dump, settings change or impersonation is performed.

## Local evidence and integration hook

Unit tests cover resolver order, address/count/zone bounds, DNS-error privacy,
exact retained-address/port refusal, nonce mismatch despite a successful connect,
refusal of an unrelated echo service without revealing the expected nonce,
cancellation/worker join/listener close and configured-versus-effective proxy
states. Package tests, vet and Windows race checks pass. Tests create only their
own temporary loopback sockets; no workload or host network configuration changes.

The dedicated no-argument entry point is:

```
cd backend-tools/session
go run ./internal/networkdiag/cmd/proof
```

This is separate from shared SDK builds. It emits at most 16 KiB JSON and uses an
eight-second parent context. Actual unelevated Windows run:
`artifacts/qualification/p2-network-diagnostic-readonly-0c0f720fc48743928173b5a7492b0876.json`,
SHA-256 `75aaa93fc6d38329c5ff55d15e0dc298f72b24d0beabb559857af15f07fc6ffe`.
Both literal families verified the owned listener. Both fixed names resolved
IPv6 first then IPv4; named dual-family and IPv4-only guarded dialing both
verified the nonce. Current-user and WinHTTP-default configurations were observed
direct-configured; effective behavior was not evaluated. This is one host's
point-in-time result, not VPN/proxy/enterprise or browser qualification.

The root-owned integration registers a fixed no-argument `network-diagnostics`
SDK command with an eight-second SDK context and an outer
`{schema:1,loopback,proxy,complete}` envelope. The Tauri command uses the same
verified SDK through `engine::session_binary()`, independently of engine running
state, and the existing ten-second subprocess deadline. It validates at most
16 KiB, exact nested schemas, fixed names/sources/versions, known enums, UTC
timestamp shape/calendar, resolution/family consistency and configured proxy
mode consistency. Unknown/duplicate/private fields and malformed output refuse
the result without displaying raw JSON or native error text.

Diagnostics now contains an explicit Run network checks button. It never runs
as a side effect of reading WSL status or saving a snapshot. Separate literal,
name/fallback and configured-proxy observations have loading, error, retry and
cancellation states. Cancel discards late completion; the notice explains that
the bounded check is finishing, and the UI holds its slot until it settles. A
Tauri-wide exclusive slot also prevents component navigation from accumulating
concurrent hidden probes. Plain buttons and live status text support keyboard
and assistive technology. Actual native UI qualification is a separate root
gate, not established by TypeScript checks.

No arbitrary renderer destination/port crosses this interface. The results are
not added to support export; changing its allowlist requires explicit privacy
review. Parser tests cover schema/size bounds, duplicate/private keys, foreign
names, inconsistent fallback/profile claims and malformed timestamps. TypeScript
checks pass; root owns Cargo integration tests, shared SDK build and attended UI
proof rather than running competing builds in this work lane.

## Remaining supported-flow work

The owned temporary TCP proof is independent of a product service publication.
Actual Windows browser access, WSL requests, containers and the optional cluster
must still qualify their exact names, protocols and trust stores. The guest
`host.docker.internal` proof should use an exact owned namespace helper and a
nonce-owned Windows IPv4 listener, with daemon/container incarnation checks and
bounded cleanup; no caller-provided remote hostname or Docker socket is needed.
Do not create/execute helpers during another agent's engine qualification lane.

Proxy auto/none/URL requires a documented engine/container transaction, scope,
credential handling, cancellation and preservation of other engines. Reading
Windows configuration does not implement that flow. VPN connect/disconnect,
split-DNS, offline operation, enterprise trust restrictions and subnet-overlap
detection remain required supported-envelope decisions and observed gates.
No user-global proxy, VPN, DNS, NRPT, hosts file, `.wslconfig`, firewall or trust
mutation is authorized by this diagnostic.
