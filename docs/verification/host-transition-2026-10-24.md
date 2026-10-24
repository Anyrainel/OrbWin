# Host transition qualification

Physical Windows sleep/resume is pending. Do not infer it from guest termination,
process loss, a time gap or simulated pause. It is the remaining roadmap host
transition gate, separate from the passing scoped faults below.

The prepared read-only before/after harness is
`artifacts/installed-host-transition.mjs`, launched through ordinary Explorer to
avoid the development host's profile virtualization. It never suspends Windows.
It records the installed daemon ID, unique HTTP marker, PVC UID/PV and marker,
Ready node, selected Ubuntu project device/inode, both foreign daemon IDs and
global Docker/kubeconfig hashes. The actual markers are created once, not
rewritten to make a post-resume check pass.

After an approved real sleep and wake, it requires new Windows System log
Kernel-Power 42 and resume 107 / Power-Troubleshooter 1 events after the before
snapshot. It then observes recovery for at most 180 seconds and compares the
same identities/data/configurations, including the selected Ubuntu client's
explicit Unix endpoint reaching the recorded daemon. Actual container start time is recorded;
automatic container restart is distinguished from unchanged process execution.
The result is `artifacts/qualification/installed-host-transition.json`.

Neither test machinery nor recovery stops another engine, suspends a user distro,
changes a power plan, toggles hibernation, disables host Ethernet or resets WSL.
Whole-PC sleep needs a deliberate user-coordinated moment because other apps and
engines are affected. The test requires a real transition; declining it does not
turn the gate into a pass.

## Already observed scoped transitions

- Exact owned management-process loss preserved Docker/HTTP; explicit confirmed
  native reconnect recovered management but restarted the guest on this pin.
- Exact owned hostagent loss and owned-guest termination exercised rdd recovery;
  identity/data/foreign engines/configuration postchecks passed.
- Native task-container network detach/reattach changed actual connectivity and
  refreshed the inspector correctly after the stale-read fix.
- Two private guest-tap-down injections removed the tap, rather than allowing
  automatic link restoration. The failed records are retained. Ordinary native
  Restart recovered the second case and preserved the installed Docker marker
  and Kubernetes PVC UID/PV/marker. This is explicit disruptive recovery (BL-017),
  not evidence of transparent Ethernet/Wi-Fi/VPN recovery.

Managed proxies, VPN changes and additional host-network arrangements remain
outside the stated direct-network matrix. These distinctions must survive the
final acceptance report and Phase 2 handoff.
