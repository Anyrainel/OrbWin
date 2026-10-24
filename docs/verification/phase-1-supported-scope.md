# Phase 1 qualification scope

Recorded 2026-10-24. Phase 1 is accepted within this local qualification scope.
Sleep/wake qualification uses the user's confirmed transition and passing
postconditions; the usual Windows suspend/resume event pair was absent, so exact
sleep depth and wake latency remain unverified.
This is one observed local support matrix, not a public compatibility guarantee.

| Component | Qualified configuration |
| --- | --- |
| Host | Windows 11 Pro x64, build 26300.9711; AMD Ryzen 7 9800X3D, 16 logical CPUs |
| WSL | 2.7.11.0, kernel 6.18.33.2-2; shared WSL resource pool |
| Backend | Rancher source `af6511ad4e065e24e7258f575a0fc090c10cdbda`, twelve recorded patches; rdd SHA-256 `0896047decdfa7c851870e80b5edc51439f243b8baa522379b4933c60e15c8f6` |
| Engine | Moby Docker 29.7.2-ce, independent `lima-orbwin`, owned pipe and persistent home |
| Bundled clients | Docker 29.9.2, Buildx 0.38.1, Compose 5.6.1; exact Windows/Linux checksums in `runtime-tools.lock.json` |
| Workload cluster | Optional K3s; native upgrade 1.34.6 → 1.35.3, PVC preservation and pod/service forwarding on the same backend pin |
| Test framework | Windows Node Testcontainers 12.2.1, Ryuk enabled; repeated/concurrent/intentional-failure/cancellation suites |
| User projects | Windows C: and F:, spaces/Unicode, directory/file/read-only binds and named volumes; one explicitly selected non-root Ubuntu project |
| Coexistence | Docker Desktop 4.91.0 and Rancher Desktop 1.24, maintained dedicated endpoints/daemon identities, workload traffic, port conflicts and owned restart |
| Networking | Direct network; explicit localhost IPv4 TCP/UDP publication and scoped Windows/WSL clients |
| Installation | Per-user x64 NSIS, WebView2; application in LocalAppData/OrbWin, data in sibling OrbWinData; unsigned local qualification |

This does not qualify every Windows edition/build, ARM64, other user
distributions, managed proxies, VPN changes, LAN/IPv6 access or every Docker
framework/client version. The external Rancher application tested here is 1.24;
the selected Rancher 2.0-derived standalone backend is a separate component.
Unknown network-device loss requires explicit recovery (BL-017). No comparative
performance, seamless recovery, clean-room or absence-of-defects claim is made.

The [coverage matrix](../phase-1-coverage.md),
[completion record](phase-1-completion.md), [baseline](baseline-2026-10-24.md)
and [limitation ledger](../backend-limitations.md) carry the actual evidence and
remaining gates. Qualification fixtures are deliberately labeled and bounded;
unrecognized workloads/volumes and other engines are not cleanup targets.
