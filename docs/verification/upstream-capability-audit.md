# Selected upstream capability audit

Reviewed 2026-10-24 against Rancher source
`af6511ad4e065e24e7258f575a0fc090c10cdbda` plus the recorded OrbWin patch series.
This is an implementation audit, not evidence that a design proposal or the
Rancher Desktop 1 application exposes the same operations in standalone rdd.

| Capability | Implemented contract inspected | Phase 1 handling |
| --- | --- | --- |
| Engine lifecycle | `cmd/rdd/app.go`, `service.go`; App `running` and current-generation conditions | Explicit owned start/stop/restart; rdd remains authoritative. |
| Workload cluster | App `kubernetes.enabled/version`; K3s version catalog/admission/controllers | Optional cluster, explicit kubeconfig, qualified upgrades and persistent workload data. |
| VM resources | App `virtualMachine.cpus/memory`; admission defaults/validation and Lima template fields in `pkg/controllers/app/app/controllers` | Show requested values separately from observed Docker resources. The Windows proof observed 16 CPUs and the shared WSL memory pool despite smaller requested values. Do not offer ineffective per-engine limits or alter global WSL allocation during simultaneous-engine operation. |
| Proxy configuration | Complete `AppSpec` in `pkg/apis/app/v1alpha1/app_types.go`; controller and CLI command surfaces | No supported standalone rdd proxy-setting API was found. The supported qualification uses direct networking. Managed proxies and VPN changes need separate qualification; no proxy dialog pretends to update the backend. |
| Registry authentication | Pinned Docker CLI/Buildx/Compose and Docker credential-helper protocol | Reuse the clients and Windows vault library, with product-specific credential keys and private configs. |
| Docker object data | Docker APIs plus pinned CLI; containers, images, named volumes, networks and BuildKit history | Qualified object operations and explicit unused-object cleanup. Ordinary named-volume data preservation is required; cleanup never silently becomes a disk reset. |
| Whole-engine backup / restore | `cmd/rdd` command registration and implementation; `limavm` lifecycle/edit/delete | No supported combined rdd control-plane/engine-data backup/restore API was found. Test-only offline WSL exports preserve qualification fixtures; they are not a product backup feature or a demonstrated migration mechanism. |
| Docker-only / full product reset | `cmd/rdd/limavm.go` delete and `service.go` delete; App reconciliation | VM deletion and control-plane deletion exist as separate primitives. No combined scoped reset transaction was found that preserves the requested subset, coordinates reconciliation and republishes a fresh owned identity. The UI explains both unavailable scopes and directs space recovery to qualified selected-object cleanup. Building that custom transaction requires preservation/interruption qualification; do not claim the backend cannot delete a VM or offer raw unregister as a successful reset. |
| Engine disk relocation | Same CLI/App surface; fixed owned home and WSL registration checks | No supported standalone rdd relocation transaction was found. Do not copy a live writable disk or edit WSL registration paths. Broader storage convenience remains Phase 2. |
| Rancher / Docker Desktop migration | CLI registration, App controllers, resource APIs | No source-version-specific import/migration operation was found in the selected standalone backend. Shared ancestry does not establish data interchange. Custom transition conveniences remain Phase 2 under the roadmap. Existing engines retain independent state. |
| Application updates / locale / PATH | `ApplicationSpec` explicitly describes Electron frontend preferences; rdd generally does not implement them | OrbWin owns its UI settings. PATH remains manual and private. Installed same-pin backend replacement validates identity and retains data. Public automatic updates/signing remain later release work. |

Searches covered actual Go command/API/controller implementations for proxy,
migration/import/export, backup/restore and relocation, then inspected the
complete AppSpec and command constructors. Generic watch comments mentioning
proxy disconnection or configuration migration are not implementations of those
features. Proposal documents and the separate Electron application's interfaces
were not treated as an executable standalone contract.

The absence findings are scoped to this source pin and inspected interfaces.
Revisit them when qualifying an upstream update. Preserve the current limitations
in the UI and backend ledger; do not substitute this audit for runtime acceptance
of the operations that are supported.
