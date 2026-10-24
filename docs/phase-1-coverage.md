# Phase 1 coverage

Updated 2026-10-24 Pacific. Phase 1 is **in progress**; no milestone or phase exit
is claimed. An unavailable UI branch records current implementation status, not
an accepted feature exclusion. The full scope in the roadmap remains required.

The [2026-10-24 reassessment](verification/phase-1-readiness-2026-10-24.md)
records current gate decisions and fresh checks. Run-form Windows drive/file/
directory/read-only mounts now pass a scoped F-drive runtime test after a
canonical-path dispatch fix; general CLI/Compose Windows paths still fail.
Native shell/navigation/search/settings received a smoke check, not full UI
acceptance. Table items beyond that scope remain unqualified.

| Family | Evidence / upstream boundary | Implementation today | Required completion |
| --- | --- | --- | --- |
| Onboarding, headless lifecycle, closing UI | OS-001/002; rdd service/App APIs | Explicit owned start/stop/config reads; real stop/start preserves daemon identity; no UI shutdown hook | First-run provisioning/download/cancel, dedicated restart UX, interrupted start/reconnection qualification |
| Containers / Compose projects | OrbStack interaction evidence incomplete; rdd mirrors + Docker/Compose | Row hover actions, multi-select batch (sequential, no rollback), context menu, structured Info tab, CPU/memory sparklines, saved collapse/sort/width, Compose group actions - implemented, unverified live (BL-008/009); run/action/inspect/confirmation dialogs; Compose up/start/stop/restart/pull/logs/down; real native-adapter service/build proof passes. Run-form Windows bind mapping added with parser tests | Qualify bind mapping, general CLI/Compose Windows paths, broader creation settings, group interaction polish, native full-journey QA and failures |
| Logs, exec, terminal, stats | Ordinary Docker APIs; OS-006 is a broader debug-shell capability | Bounded log follow/search/export, stats polling, exec; SDK/xterm TTY proof passes resize/Ctrl+C/exit/backpressure and container survival | Native rendered/session QA, further disconnect/reconnect cases and large-output review |
| Images, pull/build | rdd mirrors/requests + Docker CLI/Buildx | Pull/tag/untag/remove/full inspect and Buildx submission/progress/cancel; build/Compose adapter proof passes | Cache/history management, registry credential flows and broader native/error qualification |
| Volumes / storage | rdd volumes; OS-005 file browsing is broader convenience | Create/inspect/remove, named-volume attachments and explicit irreversible-removal wording; Compose preservation proof passes | Windows binds, broader preservation/fault tests; backup/relocation capability audit |
| Networks / access | Docker networks; OS-003/004 documented domains/HTTPS | Create/inspect/remove/connect/disconnect, explicit localhost port defaults; isolated dynamic-port PING passes | Conflict handling and broader host/WSL access; domains/HTTPS conveniences phase 2 |
| Kubernetes | OS-008/010; optional workload cluster distinct from rdd API | Enable/disable, explicit owned kubeconfig; deployments/pods/services/claims inspection, logs/apply/UID-safe delete/wait; real workload/storage proof passes. List and inspector (Info/Logs last 500 lines/YAML/Events) implemented but unverified on a live cluster; port-forward lives only while its dialog is open; pod terminal is one non-interactive exec (BL-010) | Qualify newly added forward/exec, version controls, native QA and broader cluster recovery |
| WSL integration / sources | OS-009 informs Linux-environment intent, Windows adaptation is original | Read-only list parsed from `wsl --list --verbose`, roles inferred by name; `wsl_open_terminal` compiled, untested (BL-011) | Selected-distro integration, Windows/WSL paths and mounts, explicit discovery, cleanup without disturbing user distros |
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
