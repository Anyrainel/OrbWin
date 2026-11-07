# Kubernetes service access

Recorded2026-10-25. P2-D05 retains the explicit localhost TCP-forward adaptation
for the optional owned workload cluster. This is an original Windows scope choice,
not a claim that Kubernetes cluster DNS is visible to host clients or that a
cluster Ingress controller is installed.

The existing backend delegates pod/service forwards to the pinned `rdd kubectl`
client with an explicit namespace/resource and `--address=127.0.0.1`. There are at
most two concurrent clients, bounded to one hour. They survive page navigation;
closing the application disconnects them. They are not background named routes.
Stopping a forward cancels its client, not the pod, service or cluster. A failed
forward ends its active link; reconnect is explicit and does not restart workloads.

Phase1's `verification/phase-1-completion.md` records native pod/service HTTP
forwards, retained PVC marker, navigation, listener removal and fresh-guest/restart
checks. That historical evidence establishes those recorded cases, not the new
HTTPS controls or arbitrary Kubernetes service protocols.

## Protocol and trust boundary

The current implementation previously presented every connected TCP port as an
HTTP browser link. Database ports and TLS services do not establish HTTP readiness.
The forward panel now initially displays the exact numeric loopback address and
remote port with a Copy address action. The user explicitly selects HTTP or HTTPS
to reveal a browser action; neither the port number nor a service label guesses
the application protocol. The TCP listener remains the same in every selection.
Clipboard rejection is visible with an address the user can select manually.

HTTPS passes the application's TLS through unchanged. It does not terminate TLS,
mint an OrbWin certificate, alter SNI/Host, install trust or disable certificate
verification. The service certificate must cover the address the client uses and
its issuing authority must be trusted by that client. A Kubernetes-only DNS SAN
does not become valid for `127.0.0.1`; a certificate warning is not a reason to
disable validation. Applications needing their service DNS name/SNI require an
explicit compatible client configuration outside this loopback browser action.

The native opener accepts only a numeric nonzero `u16` port and an exact `http` or
`https` scheme (omission preserves HTTP for existing callers). It constructs
`SCHEME://127.0.0.1:PORT/` itself. The renderer cannot provide a hostname, userinfo,
path, query, fragment, executable or shell argument. Windows ShellExecute receives
the fixed URL directly; opening failure stays visible. No listener, DNS entry,
hosts file, trust store or global Docker/Kubernetes context is changed by selecting
a protocol or copying the address.

Readiness parsing accepts complete pinned-client loopback records only, ports
1–65535, deduplicates repeats and refuses conflicting remote mappings for one local
port. Parsing a readiness record establishes the reported listener, not an HTTP
health check or backend identity invariant; the existing owned kubectl transport
and process lifetime retain those responsibilities.

## Evidence and remaining gates

Focused Node tests cover address/protocol bounds and hostile inputs, complete
readiness records, duplicates/conflicts, and server rendering of the initial TCP
state without a web link. TypeScript checking passes. Rust tests cover default
HTTP, explicit HTTPS and refusal of arbitrary scheme/URL/userinfo/path/query/NUL
inputs without invoking Windows ShellExecute. These are source/pure tests with no
cluster or engine mutation.

The new UI/native path still needs attended checks for keyboard protocol selection,
clipboard denial, HTTP/HTTPS browser launch and opener failure. An HTTPS fixture
must include both valid certificate/address/trust and expected mismatch/untrusted
failures without bypasses. Non-web TCP, target exit/replacement and client deadline
behavior remain bounded-forward acceptance cases. Existing active user Kubernetes
workloads must not be modified or restarted merely to obtain this evidence.

Cluster naming/automatic HTTPS, Kubernetes Ingress installation, LAN exposure,
forward survival across application exit and general host access to cluster DNS
are outside this selected adaptation. Docker local-service routing and its
constrained signer remain separately owned and are not reused implicitly here.
