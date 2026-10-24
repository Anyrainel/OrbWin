# Phase 1 final assessment

2026-10-24 Pacific. **Accepted for the stated local Phase 1 scope.** Implementation,
installed-product qualification, manual sleep/wake preservation and exact fixture
cleanup are recorded. This is not a public release or a broader compatibility
claim. The sleep result uses the user's confirmed transition and independent
postchecks; exact power state and wake latency were not instrumented.

Source checkpoint: `ee0eb7b`. Backend remains the twelve-patch `0896047d…`
build; the [supported matrix](phase-1-supported-scope.md) specifies the exact
host, clients and coexistence scope. Final unsigned installer SHA-256:
`b13cb6f0721a2d21ee9b711cf626a7151b34c2afefa568a4aa75df3de66b7882`.
Its actual NSIS replacement and all four executable comparisons passed; its
installed application started with the retained Docker and Kubernetes data.
Raw evidence is `artifacts/qualification/installed-final-update.json` and the
read-only before-transition snapshot, not a claim based only on compilation.

| Roadmap gate | Current evidence / decision |
| --- | --- |
| Install/start/use/restart | Native installed first-run cancel/resume, application refusal while backend is mapped, explicit preparation, uninstall/reinstall retention, final source-matched replacement/start; passing recorded cases |
| Clients, mounts and workloads | Docker/Compose/Buildx, C/F/selected Ubuntu paths; installed Testcontainers `ed404a2b…` normal/concurrent/assertion/cancellation and Ryuk cleanup passed |
| Full relevant flow accounting | [Current coverage](../phase-1-coverage.md) records real adapters and useful blocked-flow adaptations; advanced options use qualified CLI/Compose; no fake reset, backup or migration success |
| Simultaneous engines | Installed `orbwin-coexist-f0528dd8`; dedicated daemon identities, maintained traffic, conflict behavior and owned restart passed with Docker Desktop/Rancher retained |
| Native developer UI | Actual resource actions, batch partial failure, private registry pull/build/forget, logs/search/wrap/export, stats, files, Docker/pod TTY, persistent pod/service forwards, WSL project/terminal, storage and diagnostics reviewed in native windows |
| Closure and cancellation | UI closure retains workloads; bounded clients/kill-on-close jobs, explicit uncertain accepted mutations, session/forward cancellation and data retention exercised |
| Fault recovery/data | Installed management/hostagent/guest loss and contained OOM/ENOSPC; confirmed reconnect and ordinary restart after guest tap removal; exact Docker and cluster markers retained |
| Host sleep/resume | User-confirmed sleep/wake; independent postchecks retain every recorded identity/marker/configuration and the Docker container start time. Kernel-Power 566 records a SleepButton request; the usual 42/107/1 event pair is absent. Manual test accepted; strict automated event check remains failed and retained. |
| Measurements and quality | Three measured starts/build/filesystem samples and idle budgets; 42 Node / 37 ordinary Rust tests, Clippy, formatting, frontend/package builds; helper race tests; focused ownership/cancellation/limits review |

Residual boundaries are explicit: direct localhost networking on one Windows x64
host, selected Ubuntu integration, shared WSL resources, unsigned local package,
manual disruptive recovery for guest tap loss, and no automatic migration,
whole-engine backup/relocation/reset transaction. A tap-down injection failed
automatic restoration and remains a failed record (BL-017); ordinary explicit
restart recovered it. These facts must not be rewritten as seamless recovery,
general VPN/host-network support or comparative reliability claims.

The final before-transition snapshot already verifies daemon `d9c00194…`, HTTP
marker, installed PVC `df8cf086…` and PV/marker, Ready node, Ubuntu source identity,
both external daemon identities and global config hashes. Docker configuration
properties and the original `desktop-linux` context match task entry. Raw JSON
formatting changed during earlier external-engine startup; byte equality is
claimed only within the individual qualification windows, not over the whole task.

At 4:20 PM Pacific, read-only postchecks passed after the user's sleep confirmation.
The missing event pair is retained as a measurement limitation, not converted to
an automated pass. See [host transition evidence](host-transition-2026-10-24.md).
Exact cleanup then deleted only the recorded namespace with a UID precondition,
its PVC, and the labeled Docker HTTP container/network/volume. Two unrecognized
anonymous volumes, cached images, historical archives and the selected Ubuntu
proof source/export were retained. The installed engine/optional cluster stay
available for Phase 2. Cleanup evidence is `installed-final-cleanup.json`.
