# Host transition qualification

## Final observation — 2026-10-24, 4:20 PM Pacific

The user explicitly confirmed that Windows had slept since the recorded before
snapshot. Independent postchecks passed: identical daemon and Docker container
start time, exact HTTP marker, unchanged PVC UID/PV and cluster marker, Ready
node, Ubuntu source identity and Unix Docker endpoint, both foreign daemon IDs,
and both global configuration hashes. See
`artifacts/qualification/installed-host-transition-postcheck.json`.

Windows Kernel-Power 566 records `SleepButton` at 4:13:53 PM Pacific. This
corroborates a sleep request, not the completed power state. No new Kernel-Power
42/107 or Power-Troubleshooter 1 pair was found; the strict automated check failed
and its result is retained unchanged. Read-only power-request diagnostics require
elevation and were unavailable; no power setting or permission was changed.

Acceptance uses a **manual, user-confirmed sleep/wake test with verified product
postconditions**. It does not claim an instrumented S3 transition, sleep duration,
automatic wake, wake latency or absence of transient outages. The lack of the
event pair remains a measurement limitation, not a rewritten automated pass.

## Prepared protocol and evidence boundaries

Sleep must not be inferred from guest termination, process loss, a time gap or
simulated pause. The manual user observation above supplies transition evidence;
the independent postcheck supplies preservation/reachability evidence.

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
