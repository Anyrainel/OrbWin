# Guest network loss: current evidence and recovery boundary

Recorded 2026-10-26. This is a source investigation for P2-D06/BL-017, not an
implemented or natively qualified automatic repair. No network, engine, WSL or
service action was performed for this investigation.

The preserved `artifacts/qualification/owned-link-flap.json` is a failed guest
device-loss injection. The Docker daemon ID remained constant, four HTTP probes
failed, and the independent restoration could no longer find `eth0`. It does
not establish an ordinary VPN transition or identify the complete failure path.
The later explicit restart preservation evidence remains the current supported
recovery, including its workload interruption warning.

## Source observations

The inspected tree is `.upstream/rancher-desktop` at the selected upstream
commit. These exact source files were read and hashed:

| File below `rdd/` | SHA256 |
| --- | --- |
| `src/networking/cmd/vm/switch_linux.go` | `55d237dfa371f8628002632f2d8ed6f88ac913a834aedc989d51046a78e11be1` |
| `src/networking/cmd/network/setup_linux.go` | `4f344108aa8ec8995c36e5c41c6710930aad9657af315217e854e09d55ae2dbd` |
| `overlay/root/usr/local/lib/systemd/system/network-setup.service` | `ff8ac898b5b479664afc0a7c0176fa1239fd88fab3501ae968b0fa7409e1f008` |
| `overlay/root/usr/local/lib/systemd/system/docker.service.d/network-namespace.conf` | `adf1d98ecb274bf8d67ef8e516cf52678e17c4c13b709bdacca29a6a11b5e39c` |

The setup source matches the selected lock's patched-file hash. The other three
have no worktree difference from that upstream HEAD. This does not independently
bind a running guest ELF to these sources; the currently packaged twelve-patch
backend and selected seventeen-patch source are distinct provenance records.

`vm-switch` receives one already-open vsock file descriptor. Its main loop calls
`run` repeatedly with the same `connFile`. Each `run` creates a tap and starts
transmit, receive and DHCP workers; its deferred teardown closes both `connFile`
and the tap when the first worker error is received. The next loop therefore
reuses the same closed Go file object. No new vsock dial occurs in this loop.
The workers are not all positively joined before the next iteration; their
shared error channel has capacity one, and transmit/receive error sends do not
select cancellation. The outer signal check cannot interrupt a blocked `run`.
These are source-level lifetime defects, not proof that each occurred during
the retained injection.

`network-setup` opens the vsock connection before entering the network namespace,
then passes an extra descriptor to its `vm-switch` child and waits for it. This
ordering is intentional because vsock depends on the network namespace. Simply
dialing a new vsock from the existing child's private namespace is not an
equivalent repair. The parent code also retains its original connection/file
references while waiting; closing only the child's duplicate is not proof that
the host has observed EOF. The Windows host switch accepts data connections
serially, after the previous `AcceptStdio` returns.

Adding `Restart=on-failure` to `network-setup.service` is not a transparent fix.
Docker, containerd, k3s, BuildKit, openresty and the guest agent require that
service. Stopping or failing that unit can stop dependent services. Its current
unit has no restart policy, and the namespace itself is owned by a separate
unit. A source change must account for this dependency graph before claiming
workloads survive network repair.

## Implementation and qualification requirements

The smallest candidate repair should reuse the existing network setup owner,
retain the same namespace and preserve daemon/container processes. It needs:

- One owner for each connection generation, tap, DHCP child and all copy workers.
  On failure, cancel, close the exact endpoints, positively join every worker
  and child, and check closure before admitting another generation. No retry of
  a closed file object or accumulation of blocked error senders.
- A fresh connection created from the correct original namespace, with explicit
  ownership transfer to the admitted switch generation. Parent duplicates must
  have a documented checked lifetime so old EOF cannot be accidentally withheld.
- Bounded recovery attempts and honest unavailable status on exhaustion. Service
  readiness or Docker API access must not stand in for a working data path.
  Never silently restart Docker/Kubernetes or the distribution to make the test
  pass. Explicit disruptive recovery remains available.
- Pure cancellation/teardown/error-order tests, followed by an actual contained
  guest-device-loss test against a source-bound candidate ELF. Compare namespace,
  daemon/container/PVC identities and payloads, foreign engine/distro state,
  connection behavior and resource counts before and after. Preserve failed
  evidence and distinguish recovered existing connections from new connections.

BL-018 is a separate related acceptance gap. The guest tracker forwards through
the tap address, and Docker event handling adds broad guest DNAT rules for
explicit loopback bindings while logging rule errors. Neither those source rules
nor the successful wildcard-bound managed relay proves the ordinary user
container `127.0.0.1` publishing contract. Qualify exact TCP/UDP and address-family
paths and listener scope without silently rewriting user bindings.

## Isolated candidate and review

The source-only V1 candidate is preserved under
`artifacts/patch-replay/tap-generation-recovery-candidate-20261026`. It keeps the
existing setup owner and namespace, bounds connection generations, dials from
the original namespace, and gives each switch a direct PID namespace whose
positive init reap is required before another generation. Nine portable lifetime
tests and race checks passed; the Linux programs were cross-compiled only.
Neither the selected backend nor a running guest was changed.

Independent review found three issues requiring a separate V2 candidate:

- DHCP deconfiguration flushes IPv4 while leaving the TAP up. V1 latches READY,
  so it can publish stale readiness or miss an established lease loss. Private
  status must invalidate pending readiness and retire a generation after loss
  of an established lease; initial pre-lease deconfiguration remains allowed.
- The parent-loss watcher starts after synchronous TAP/netlink setup. Establish
  it before setup, bound startup, and ensure PID namespace init exits within a
  finite interval if canceled setup cannot join. An unknown parent wait still
  prevents another generation.
- Netlink receive must reject truncated datagrams and incomplete trailing
  frames, rather than relying on a parser which can ignore short fragments.

The separately frozen V2 implements these fixes and passes portable race checks
and cross-compilation. Root review then identified a renewal-hook failure case:
the hook can flush an address and fail while udhcpc remains alive. The frozen
source-only V3 adds private configuration-begin/success/failure observations and
a five-second pending-hook bound, polled independently every 250ms. Successful
renewal stays in its generation; explicit failure, missing completion or a late
success after the observed deadline cannot keep stale readiness. Both package
race checks pass in 1.220 seconds; Linux programs and preserved Linux tests were
compiled only. The actual hook, kernel, timing and descriptor paths remain
unexecuted. A second independent review found that a normal renewal between the
first R and initial readiness publication unnecessarily retired that generation.
The separately frozen V4 distinguishes pending configuration, waits on the same
owner within the original readiness deadline, and avoids polling a closed Ready
channel continuously. Actual-coordinator tests verify one start and no extra
retirement for that successful renewal, finite unfinished-renewal timeout, and
no false readiness from an ended generation. Both portable race checks pass in
1.285/1.223 seconds; vet and Linux cross-compilation also pass. See the preserved
V3 and successor V4 candidate records under `artifacts/patch-replay/`.

These are source changes, not qualified behavior. Systemd status and logs are
also insufficient product health propagation: SDK/API/UI unavailable/recovering
states remain an integration gate. Actual pinned-kernel descendant retirement,
lease loss, startup parent loss, TAP exclusivity, connection behavior and workload
preservation require separately reviewed native qualification before adoption.


## Phase 2 investigation disposition — 2026-11-06

P2-D06 retains the explicit manual recovery already provided by EngineLifecycle:
confirmed stop followed by start, a Docker/Kubernetes interruption warning,
persistent-disk retention and an explanation that cancellation between steps
leaves the engine stopped. Source review confirms this interaction; the new
coherent package still needs its actual restart/data-preservation acceptance.
The scope permits truthful disruptive recovery. The automatic repair proposal
above remains unadopted Phase 3 reliability work, with its concrete safety and
qualification requirements intact. BL-017 and failed device-loss evidence remain
open limitations; this disposition neither proves automatic repair impossible
nor establishes ordinary VPN transition behavior.
