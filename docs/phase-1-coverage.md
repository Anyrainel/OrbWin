# Phase 1 coverage

## Current qualification — 2026-10-24

The table below supersedes the historical implementation inventory later in this
file. Phase 1 is accepted within the stated local matrix, including manually
confirmed sleep/wake with passing preservation checks. Exact sleep depth and
wake latency remain unverified. Evidence and failures are in the
[completion record](verification/phase-1-completion.md).

| Family | Implemented and observed | Supported adaptation / qualification boundary |
| --- | --- | --- |
| Onboarding and lifecycle | Installed first-run cancellation/resume, owned identity, current-start readiness; explicit start/stop/restart and confirmed reconnect; closing UI retains workloads | rdd owns lifecycle. Native Prepare update releases application files; update/uninstall/reinstall retained engine data. |
| Containers and Compose | Real creation/actions/full inspect, grouping, selection and sequential batch outcomes; run form, Compose lifecycle/logs, resource/build/Compose preservation proofs | Common creation fields are in the form; advanced Docker options use the bundled explicitly targeted CLI or Compose. Partial batch results have no rollback. |
| Logs, terminal and stats | Native Docker/pod TTY input/resize/Ctrl+C/exit/closure, bounded followed logs/search/wrap/export; actual stats and sparklines | Export saves visible output; broader debug-shell capabilities are not implied. |
| Images and builds | Pull/tag/remove/inspect, Buildx progress/cancel and retained history; native private pull/private-base build, authentication rejection and Forget | Windows vault helper and private client configs; no global credential/default changes. |
| Files | Installed container-directory listing and UTF-8 preview, including mounted named-volume data in a stopped container | Read-only Docker archives, bounded scans/entries/previews and honest partial/error/cancel states. No implicit helper workload for standalone images/volumes. See [contract](architecture/container-files.md). |
| Volumes and storage | Create/attach/inspect/remove, in-use rejection, sentinel retention across restarts/upgrades; native disk usage and selected cleanup | Whole-engine backup/relocation has no supported standalone transaction in the inspected backend (BL-016). Unselected volumes are retained. |
| Networks and access | Native create/remove/connect/disconnect and refreshed Info; scoped localhost TCP/UDP allocation, conflicts, Windows/WSL/delegated client proofs | LAN/IPv6 and automatic names/HTTPS remain unqualified conveniences. Guest-device loss needs explicit recovery (BL-017); it does not qualify ordinary host-network/VPN transitions. |
| Kubernetes | Optional enable/disable/version controls, workloads/claims/apply/UID-safe deletion, followed logs, native pod TTY and persistent pod/service localhost forwarding; version upgrade retains PVC/PV/sentinel | Pin-specific guest dependencies are bundled. Workload cluster and backend management API are separate. Forward cancellation retains the pod. |
| WSL integration | Selected non-root Ubuntu connect/inspect/disconnect; real bind identity, Linux clients/Compose/Buildx/credentials, source replacement, idle/restart preservation and native terminal launch | A selected export needs a live user-distro session; reconnect guidance is explicit. Transparent arbitrary `/home` mapping remains later convenience. |
| Settings | Native theme/title bar, navigation/keyboard and minimum-size review; explicit lifecycle/cluster/version controls and client guidance | Requested resources are distinguished from shared WSL allocation (BL-006); unsupported proxy transactions are explained (BL-016). Tray/autostart and public auto-update remain later UI conveniences. |
| Recovery and diagnostics | Installed management/hostagent/owned-guest faults, fresh errors/retry and exact preserved markers; native bounded diagnostic export; contained cgroup OOM/tmpfs ENOSPC; manually confirmed sleep/wake retained all recorded data/endpoints | Reconnect can restart the guest and interrupt workloads. No reset or global WSL shutdown as routine recovery. Sleep depth and wake latency were not instrumented; the strict event-pair check remains a retained failure. |
| Compatibility and packaging | Twelve replayable patches; installed three-engine workloads/conflicts/restart; repeated/concurrent Testcontainers/Ryuk; C/F/Ubuntu paths; actual NSIS install/refusal/update/uninstall/reinstall and file/data retention | One explicit Windows x64 host matrix, unsigned local installer. Additional OS/client versions and public signing/distribution need qualification. See [packaging](architecture/packaging.md) and [baseline](verification/baseline-2026-10-24.md). |
| Migration | Actual standalone source/API audit completed | No source-version-specific migration transaction found. No automatic interchange assumed; custom migration remains Phase 2 (BL-016). |

These adaptations distinguish supported contracts, missing standalone capabilities
and later conveniences. They do not remove ordinary Docker or local Kubernetes
development from Phase 1. Independent OrbStack research was not edited.

Reset accounting: the Settings page explicitly explains Docker-only and full
product reset, and directs space recovery to selected-object cleanup. Separate
VM/control-plane deletion primitives do not establish either combined reset
contract. No reset success is offered. The source audit records this custom
transaction gap alongside whole-engine backup/relocation; it is not a claim
that upstream cannot delete a VM.

## Historical inventory before completion work

Updated 2026-10-24 Pacific. Phase 1 is **in progress**; no milestone or phase exit
is claimed. An unavailable UI branch records current implementation status, not
an accepted feature exclusion. The full scope in the roadmap remains required.

The [2026-10-24 reassessment](verification/phase-1-readiness-2026-10-24.md)
records historical gate decisions at that checkpoint. Subsequent implementation
and evidence are in [the active completion record](verification/phase-1-completion.md).
Run-form and general CLI/Compose Windows drive/file/directory/read-only mounts
now pass scoped C/F-drive runtime tests. Selected Ubuntu project exports also pass.
Native shell/navigation/search/settings received a smoke check, not full UI
acceptance. Table items beyond that scope remain unqualified.

| Family | Evidence / upstream boundary | Implementation today | Required completion |
| --- | --- | --- | --- |
| Onboarding, headless lifecycle, closing UI | OS-001/002; rdd service/App APIs | First-run setup/cancel/resume adapter proof, explicit owned start/stop/config reads, restart UX; actual daemon identity retained; no UI shutdown hook | Complete native first-run, interruption/reconnection and installed-product qualification |
| Containers / Compose projects | OrbStack interaction evidence incomplete; rdd mirrors + Docker/Compose | Row hover actions, multi-select batch (sequential, no rollback), context menu, structured Info tab, CPU/memory sparklines, saved collapse/sort/width, Compose group actions - implemented, unverified live (BL-008/009); run/action/inspect/confirmation dialogs; Compose up/start/stop/restart/pull/logs/down; real native-adapter service/build proof passes. Run-form Windows bind mapping added with parser tests | Qualify bind mapping, general CLI/Compose Windows paths, broader creation settings, group interaction polish, native full-journey QA and failures |
| Logs, exec, terminal, stats | Ordinary Docker APIs; OS-006 is a broader debug-shell capability | Bounded log follow/search/export, stats polling, exec; SDK/xterm TTY proof passes resize/Ctrl+C/exit/backpressure and container survival | Native rendered/session QA, further disconnect/reconnect cases and large-output review |
| Images, pull/build | rdd mirrors/requests + Docker CLI/Buildx | Pull/tag/untag/remove/full inspect and Buildx submission/progress/cancel; retained BuildKit history/details/logs and disk usage; scoped build/history proofs pass, native history selection observed | Registry credential flows, cleanup and broader native/error qualification |
| Volumes / storage | rdd volumes; OS-005 file browsing is broader convenience | Create/inspect/remove, named-volume attachments and explicit irreversible-removal wording; Compose preservation proof passes | Windows binds, broader preservation/fault tests; backup/relocation capability audit |
| Networks / access | Docker networks; OS-003/004 documented domains/HTTPS | Create/inspect/remove/connect/disconnect; Windows port allocation and verified publication; Windows/WSL/delegated socket concurrency, occupied-port rejection, publish-all and restart proofs pass. Rancher and Docker Desktop workload pairs pass | Complete three-engine/order matrix and native links; LAN/IPv6 unsupported; domains/HTTPS conveniences phase 2 |
| Kubernetes | OS-008/010; optional workload cluster distinct from rdd API | Enable/disable, explicit owned kubeconfig; deployments/pods/services/claims inspection, followed logs/apply/UID-safe delete/wait. Persistent localhost forwards survive navigation; SDK interactive pod sessions pass proof-guest input/resize/Ctrl+C/exit/closure tests | Qualify forwards/log-follow/native terminal, version controls and cluster recovery on the current networking patch |
| WSL integration / sources | OS-009 informs Linux-environment intent, Windows adaptation is original | Selected non-root Ubuntu project connect/inspect/disconnect, verified bind identity, private pinned tools/config, Linux Docker/Compose/Buildx/read-write/restart/idle proofs pass; native distribution/project view observed | Full native connect/terminal journey and broader output/cancellation qualification; maintain explicit shared-source path and idle-distro guidance |
| Settings | OS-007; rdd App spec | Tabbed settings, light theme, command palette, shortcuts, toasts, first-run steps (unverified; BL-012); fixed identity, explicit lifecycle/cluster controls, scoped client guidance; resource-scope limitation explained | Version/integration/proxy settings and resource capability audit; apply/restart/error qualification |
| Recovery / diagnostics | Source recovery behavior incompletely observed; Windows policy original | Host diagnostics, unavailable/retry/error states; bounded clients; local diagnostic snapshot export added | Export/native QA, layer-specific recovery, broader interruption/reconnect and fault tests |
| Compatibility / packaging | Rancher/Linux tool contracts, Windows target | Reproducible dev artifacts and SDK helper; Testcontainers Node 12.2.1 repeated/concurrent/failure/cancellation + Ryuk cleanup pass with explicit targeting | Installers after workflows, coexistence, Windows paths, broader workloads and phase gates; auto-update deferred |
| Migration | D25, phase 2 gap policy | No automatic migration offered or assumed | Audit selected upstream support by source/target; preserve data; custom conveniences phase 2 |

UI refresh 2026-10-23: new list/shell features are implemented but not run against
a live engine; see verification/orbstack-parity-2026-10-23.md. Phase 1 remains in
progress, no exit claimed.

No niche-feature exclusion has been approved in this implementation pass.
Evidence references are the independent research's active `flows.md`; that
directory was read but not edited. Missing OrbStack evidence does not remove
ordinary developer container or Kubernetes requirements.

Historical UI correction (2026-10-22, before the ordinary-workflow adapter): resource list selection and returned-field detail
inspection now work; Ctrl+F/search/Escape, row arrow navigation and tab arrow
navigation were exercised in the native app. Compose-label grouping exists but
has no live-project verification yet. Builds, Pods and Services have explicit
unavailable destinations. Logs/stats/terminal/files tabs explain unimplemented
adapters. These are not successful feature flows or phase scope reductions.
