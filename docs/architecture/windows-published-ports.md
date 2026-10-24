# Windows published-port adaptation

The `f46626a9` coexistence proof found an acceptance-blocking defect: an automatic
guest port changed across an engine restart to a port serving Rancher on Windows.
Docker's running state and daemon identity remained correct while Windows traffic
reached the other engine. A guest-only allocator cannot see Windows listeners.

The narrow adapter will keep Moby authoritative for container identity, config,
storage and lifecycle, and keep the existing gvisor forwarder authoritative for
Windows listeners. It must not stop another engine, steal a listener, recreate a
container after a failed start, or maintain a competing container database.

The pinned Windows stack also enables Lima's generic SSH port forwarder while
the existing Rancher guest agent already publishes through gvisor. Repeated
Testcontainers restarts exposed Windows listener removal/re-add races while the
guest Redis process remained healthy. For the owned Windows instances only,
prepend Lima's supported network-port ignore rule; retain Unix socket rules and
the existing Rancher/gvisor forwarding owner. This avoids two independent host
listeners and does not replace upstream networking or add another reconciler.
Kubernetes API/service access must be requalified with that rule in effect.

Removing the duplicate forwarder was insufficient by itself. A later repeated
run caught the start adapter acknowledging the *previous* still-live publication
while the guest stop/start events were queued. The next TCP request landed in
the removal/re-add gap. Observe successful expose/unexpose operations with
ephemeral monotonic generations inside the existing host-switch API boundary.
For a new start or restart, require a newer actual publication than the one
observed before dispatch; an idempotent start of an already-running container
may retain its generation. Generations are not persistent container state and
never trigger lifecycle changes. A bounded failure remains an uncertain start.

For automatic publish requests, the Windows adapter selects an available Windows
IPv4 port and submits it as an explicit Moby binding. The stored Moby config then
retains it over restarts. Selection is a probe, not a reservation lasting until
start: a conflicting later claim must fail clearly. Before manual start/restart,
check configured bindings; after a successful Moby start response, wait a bounded
time for the owned gvisor forwarder's actual matching publication. Failure must
say that the container may be running while its host port is unavailable. Never
report the other engine's response as this engine's successful readiness.

Allocation holds a short-lived Windows FD while create/start is handed off, so
concurrent clients cannot receive the same pending candidate. The shared adapter
caps pending allocations at 256. Unstarted claims expire after 30 seconds; a
bounded start handoff extends a claim for at most three minutes. Failed create,
completed start and hostagent cancellation close the FDs. These claims contain
no persistent container state and cause no lifecycle action on expiry.

Automatic daemon restoration has no client start request to intercept. Expose a
bounded publication snapshot from Moby's running bindings and the owned forwarder
for native status/links and lifecycle reconciliation. A conflicting service is
left alone. Links for unverified publications must not be presented as connected.

Both Windows and WSL clients must cross the same adapter. The existing rdd guest
bridge can relay the owned shared Unix socket to a listener **inside the existing
private gvisor network**, served by the same Windows Docker adapter. This is not
a Windows host TCP listener. The guest bridge stays managed by upstream systemd;
it does not supervise rdd or Docker. Windows named-pipe ACLs remain owner/System.
The original vsock worker retains its original namespace. A separate systemd
worker invokes the same rdd-guest binary's Unix-client mode inside the engine
network namespace; restarting that client relay does not restart Docker.
Both the shared WSL socket and the traditional guest `/var/run/docker.sock`
(including a socket explicitly delegated to Ryuk/CI helpers) use this adapter.
Moby's backing socket moves to a root-only internal path; its data directory and
engine state are untouched. The pinned Docker unit invocation/options are retained
with that listener argument and an explicit permission hook. Socket activation is
disabled for the public path now owned by the client-relay worker.
WSL root in the same Windows user's shared utility VM is already privileged; this
does not claim an isolation boundary against a malicious root there.

Preserve unknown Docker JSON fields and number precision. Buffer only bounded
create/inspection documents; builds, logs and upgrades retain streaming and
half-close semantics. Tie both listeners and active relays to the hostagent/guest
context, including hijacked streams. Ports and private paths are fixed backend
contracts, never arbitrary renderer destinations. No environment proxy is used.

The private gvisor network is reachable from ordinary workloads through NAT.
Therefore the WSL relay listener must require mutual TLS; network placement alone
is insufficient authorization for Docker management. A per-owned-home CA and
separate server/client certificates live in a private state subdirectory, created
as a complete staged directory before atomic publication. Never pass private key
material through Lima template parameters, command arguments, tracing or support
logs. The guest receives only the fixed private directory path and reloads its
bounded certificate files for each connection. No OS trust store is modified.

Qualification must cover both client paths, automatic and explicit publishes,
publish-all, wrong-owner conflicts, unready/missing publication, restart/data
preservation, concurrent/repeated Testcontainers runs, exec/BuildKit upgrades and
maintained external workloads. A successful compile is not publication evidence.
