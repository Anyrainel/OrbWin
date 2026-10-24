# OrbStack macOS UI flow matrix vs OrbWin

Written 2026-10-23 Pacific. This is a status matrix, not a parity claim; no
percentage is given. OrbStack behavior is referenced only from
`docs/research/orbstack/` and `orbstack-ui-flows-2026-10-23.md` (published
documentation; no Mac observation). `docs/research/` was read, not edited.

Status keys: IU = implemented, unverified (code exists, typechecks and builds,
not run against a live engine); BB = backend-blocked; P2 = phase 2;
NA = not applicable on Windows or not replicable on this backend; NB = not built.

## Resource lists

| Flow | Status | Evidence note |
| --- | --- | --- |
| Row hover quick actions | IU | UI source only |
| Multi-select and batch actions | IU | Batch actions are published for OrbStack 0.10.0; OrbWin runs them sequentially, no rollback (BL-008) |
| Context menu | IU | Source only |
| Compose group actions | IU | Project logs/down only with exactly one Windows compose file label; group remove leaves project networks (BL-008) |
| Structured Info tab | IU | Built from inspect data; layout not compared with OrbStack |
| CPU/memory sparklines | IU | 60 samples held in memory only |
| Saved collapse/sort/width | IU | Local persistence; not tested across upgrades |
| Port links | IU, at risk | `target=_blank`, no opener plugin; may not open in the native window (BL-009) |
| Image/volume "in use" | IU | Relies on the `containers` prop now passed from App |
| Prune / disk usage | BB | No prune or `docker system df`; cleanup is list-and-remove-selected (BL-007) |
| Volume / virtual-disk sizes | BB | Unmeasured (BL-007) |
| Build history (published in 2.2.3) | BB, unknown | Needs backend audit |

## Kubernetes

| Flow | Status | Evidence note |
| --- | --- | --- |
| List and inspector (OS-010 intent) | IU | Info/Logs/YAML/Events; no live cluster run |
| Pod terminal | IU, limited | One non-interactive `clusterExec` (BL-010) |
| Port-forward / service access (OS-008) | IU, limited | Exists only while dialog open; no persistent link |
| Logs | IU, limited | Last 500 lines, no follow |
| Namespace coverage | IU, limited | Namespaces with no resources of that kind not listed |

## WSL and Linux machines

| Flow | Status | Evidence note |
| --- | --- | --- |
| Distribution list | IU | Read-only, from `wsl --list --verbose`; roles inferred by name (BL-011) |
| Open terminal | IU | `wsl_open_terminal` compiled, untested; rejects engine distros |
| Create Linux machine (OS-009) | NA / P2 | Not provided by the Rancher backend; not built |
| Per-VM resource limits | BB | BL-006 |

## Shell and settings

| Flow | Status | Evidence note |
| --- | --- | --- |
| Tabbed settings (OS-007 intent) | IU | Which settings need restart is unknown (BL-012) |
| Light theme | IU | Windows title bar does not follow a forced theme |
| Command palette, shortcuts, toasts | IU | Accessibility not audited |
| First-run steps (OS-002) | IU, limited | Provisioning not automated (BL-003) |
| Restart | IU | Implemented as stop + start |
| Reset (OS-007) | BB | No reset operation (BL-002, BL-007) |
| Host-port conflicts | Unqualified | BL-012 |

## Not replicable or deferred

| OrbStack flow | Status | Note |
| --- | --- | --- |
| `.orb.local` domains (OS-003), HTTPS trust (OS-004) | NA / P2 | Not offered by the Rancher backend |
| Finder-style file browsing (OS-005) | NA / P2 | No equivalent adapter |
| Debug Shell (OS-006) | NA / P2 | Not built |
| Migration from Docker Desktop | P2 | Custom gap per project policy; no automatic interchange assumed |
| Menu-bar widget | NB | Not implemented; a Windows tray adaptation is a possible later option only |

## Caveats

- Every IU row needs live-engine and native-window qualification before any
  claim of working behavior.
- Exact OrbStack controls, prompts and defaults are not in the evidence and are
  not asserted here.
