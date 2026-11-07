# OrbStack reference capture checklist and Scaleway feasibility

Prepared 2026-10-24 Pacific. Status: planned; no cloud Mac allocated or tested.
This is an OrbWin qualification plan outside the independent research chat's
owned `docs/research/orbstack/` directory. It supplements that chat's
[Mac observation plan](../research/orbstack/mac-observation-plan.md),
[flow catalog](../research/orbstack/flows.md) and
[observation gaps](../research/orbstack/observation-gaps.md).

## Scope and completion rule

Capture scope = the baseline checklist below, plus relevant newly discovered
flows and branches, reconciled into one versioned register. REA exploration is
not confined to validating our initial assumptions. Give each discovery a new
scenario ID, evidence pointer and relevance decision. Deduplicate discoveries
against the register; preserve contradictions and failed/partial observations.
Do not label a one-day collection complete if included rows remain uncaptured.
The [machine-readable register](mac-reference-scenario-register.json) starts with
48 planned scenarios and unexecuted gates. Copy it into each run bundle and add
discovery rows there; its existence is not evidence that any case was observed.

Capture each meaningful visual state and transition, not every redundant frame.
For CLI-only or invisible effects, keep commands/results, logs and data checks;
screenshots cannot establish those effects. REA findings that suggest behavior
become hypotheses until ordinary runtime observations corroborate them.
No claim of exhaustive reconstruction, hidden-state coverage or pixel parity.

The reference runs OrbStack on macOS; OrbWin comparisons run on Windows. Use
equivalent fixture data and platform-appropriate interactions. Windows/WSL
adaptations are recorded as differences rather than invented OrbStack behavior.

## Capture record required for every scenario

- ID, revision, family, priority, source/question, expected coverage (not an
  assumed expected OrbStack result), exact prerequisites and owned fixture IDs.
- App/build/channel, macOS/architecture, clients/tools/REA/provider versions,
  theme, viewport/content dimensions, display scaling, network and trust state.
- Steps, timestamps, selected resource/PID/window and actual action/results.
- Before, input/form/menu, pending, confirmation, result/error and recovery
  screenshots where those states exist; short recording for transitions that a
  still image misses. Record focus/keyboard/accessibility observations separately.
- Actual resource state, logs/exit status and sentinel/checksum before/after.
- Artifact paths, REA Evidence IDs where used, limitations and cleanup outcome.
- Status: planned, captured, partial, blocked, or not-applicable-with-rationale.
  Only captured cases support observed claims. An unavailable screenshot state
  remains a gap; do not fake a spinner, error or successful cancellation.

Repeat shared branch checks where relevant: successful empty versus read failure;
invalid input; missing/stale/recreated target; partial operation; cancel/retry;
navigation/closure/reopen; external CLI changes; supported window sizes/themes;
keyboard focus and accessible names. Skip a branch only with a recorded reason.

## Baseline scenario register

These are scenarios to observe, not assertions that OrbStack has every listed
control. An absent feature or unavailable branch is itself a recorded finding.
P0 covers core Phase 1/2 journeys; P1 fills important branches; P2 is conditional
or outside a one-day cloud machine's physical capabilities. P1 remains required
where included; priority orders work and does not silently remove scope.

| ID | Priority | Journey / starting fixture | States and results to collect |
| --- | --- | --- | --- |
| MAC-001 | P0 | First launch in disposable user/app state | Welcome/setup, permissions, download/provision progress, cancel/resume if offered, actual ready state. Preserve initial evidence before changing defaults. |
| MAC-002 | P0 | Normal startup with retained data | Launch/reopen, partial readiness, resource appearance, CLI readiness, persistent markers. |
| MAC-003 | P0 | Close UI while a fixture runs | Closure/reopen, backend and HTTP continuity, resource identity/start time. |
| MAC-004 | P0 | Explicit owned engine stop/start/restart | Prompt, progress, client failure, recovery and data; distinguish each lifecycle scope. |
| MAC-005 | P0 | Resource navigation and selection | Containers/images/volumes/networks/builds/cluster/machines, menus, tabs, search/no-match, selection, keyboard and minimum-size layout. |
| MAC-006 | P0 | Standalone running/stopped containers | Create entry if any; name/ports/mounts/command controls; start/stop/restart/pause/resume availability and actual target state. |
| MAC-007 | P0 | Fast-exit/unhealthy fixture | Status and exit/health details, errors and offered next actions; do not misclassify application exit as engine loss. |
| MAC-008 | P0 | Single and multiple container selection | Actions/context menu/shortcut confirmations; mixed states, per-item partial results and retained volumes. |
| MAC-009 | P0 | Two Compose stacks with distinct names | Group hierarchy, project versus service selection, service names, returned metadata, source-file entry points. |
| MAC-010 | P0 | Compose lifecycle | Up/start/stop/restart/down controls, aggregate output, readiness/partial failures and remaining resources. |
| MAC-011 | P0 | Compose ordered overrides/profiles | How configuration is discovered/reused, working directory, active profiles, services, reviewed source and repeat actions. |
| MAC-012 | P1 | Moved/missing/invalid Compose files | Actual menus and error/deletion behavior; recovery when source files are restored or relocated. |
| MAC-013 | P0 | Compose partial startup and cancellation | Missing image/unavailable port, failure attribution, cancel/retry and exact containers/networks/volume results. |
| MAC-014 | P0 | Noisy UTF-8 logs | Follow/history, search/highlight/context/filter, wrap, service selection, copy/export and bounded-history behavior. |
| MAC-015 | P0 | Ordinary container terminal | Open/reopen, input/resize/Ctrl+C/exit/disconnect, focus and editing shortcuts; missing shell and stopped target. |
| MAC-016 | P0 | Debug Shell against shell-less/read-only targets | Entry, initial tools, target identity/privileges, install-tool flow if available, cancellation, target exit and package/data preservation. |
| MAC-017 | P1 | Live resource statistics | Idle/load display, units, history, paused/stopped target and read failure; avoid latency conclusions from remote desktop. |
| MAC-018 | P0 | Pull/tag/remove images | Progress/cancel, absent reference, authentication failure, multiple tags, in-use deletion and confirmations. |
| MAC-019 | P0 | Successful/failed/cached builds | Submit entry, progress, cancel, retained history/steps/logs, local image use and cleanup. |
| MAC-020 | P1 | Registry/private-base build fixture | Login entry if present, failed auth, retry/forget; only disposable credentials, no secret screenshots. |
| MAC-021 | P0 | Container native files | Folder action, Finder/editor entry, running/stopped container, directory/text/binary/large/Unicode files, path continuity. |
| MAC-022 | P0 | Edit/add/delete supported container files | Permission/read-only refusal, editor save/reload, external replacement/concurrent writer and actual filesystem results. |
| MAC-023 | P0 | Standalone image files | Read-only browsing, image/tag replacement, permissions and denied write; note image-store limitations. |
| MAC-024 | P0 | Standalone volume files | Attached/unattached volume entry, native editing, owner/mode/symlinks, actual data and helper/resource side effects. |
| MAC-025 | P0 | Volume management | Create/attach/inspect/remove, in-use refusal, persistence across container recreation; exact delete consequences. |
| MAC-026 | P1 | Storage usage/cleanup/export/restore entry points | Inventory actual offered capabilities; selected cleanup/refusal, consistency warnings, cancel and preservation of unselected data. Absence is not backup success. |
| MAC-027 | P0 | Automatic container/Compose domains | Default names, open/copy/index entry, browser and CLI reachability; creation/recreation/stop/removal and stale links. |
| MAC-028 | P1 | Multiple ports/custom domains/wildcards | Detection and explicit override, ambiguous listeners, name collisions, non-HTTP access and unavailable target. |
| MAC-029 | P0 | First HTTPS access before trust | Setup prompt, denial/cancel, retry, certificate properties and actually trusted response. Keep clean initial trust state. |
| MAC-030 | P0 | HTTPS from different clients | Browser, host CLI, supported containers and Linux machine; opt-out, read-only/missing CA store, restart and trust removal. |
| MAC-031 | P1 | Native networking controls | Published-port conflict, host access, exposure scope and any configurable network boundary; compare routing with actual responses. |
| MAC-032 | P1 | Remote-safe proxy/VPN/DNS transition | Observe configuration/status and bounded failure/recovery only with independent access restoration. If unsafe or unsupported, record blocked; never sever the only remote route. |
| MAC-033 | P0 | Optional Kubernetes enable/disable | Setup/version/readiness, cancellation if available, disabled state and preserved data according to actual behavior. |
| MAC-034 | P0 | Kubernetes resource journeys | Workload/service/namespace/claim views, inspect/YAML/events, logs/TTY and delete confirmations; collect errors too. |
| MAC-035 | P0 | Kubernetes service access and local images | Service links/domains/Ingress/ports, namespace collisions, local-image workflow, persistent marker and unavailable service. Do not assume an Ingress controller. |
| MAC-036 | P0 | Disposable Linux machine | Create distro/name controls, progress, shell/open/stop/restart/delete, default user and persistence. Windows equivalent uses supported user WSL integration. |
| MAC-037 | P0 | Linux-machine files and developer tools | Two-way host sharing, native paths, editor/terminal/SSH entry, missing path/permission error and independence from Docker engine lifecycle. |
| MAC-038 | P1 | CLI/SSH/agent integration | Discover actual commands, defaults/context selection, native executable invocation and credential/agent behavior; no user private keys or implicit forwarding. |
| MAC-039 | P0 | Settings/apply/error | Available categories, draft/validation/apply/restart, cancellation, retained values and requested versus enforced resource controls. |
| MAC-040 | P1 | Menu-bar/tray equivalent and login startup | Quick actions/status, opt-in startup, quit versus stop, shortcuts and stale/offline presentation; record Windows adaptation separately. |
| MAC-041 | P0 | Docker Desktop migration, disposable source only | Source/version selection, discovery, data selection/space estimate, progress, cancel, conflicts, verification and source retained. Conditional on source installation/license/available time. No Rancher migration assumption. |
| MAC-042 | P1 | Migration retry/failure/accounting | Interrupted transfer or unavailable source, safe retry and target cleanup; separate images/volumes/settings/credentials/cluster claims. |
| MAC-043 | P1 | Docker-only and whole-app reset | First capture scope/prompt/cancel. Execution only against an explicitly disposable reference setup after other captures and export; inspect actual retained/deleted data. |
| MAC-044 | P1 | Diagnostics/unavailable engine | Failure versus empty inventory, log/support export, error copy/retry and useful recovery guidance; bounded disposable faults only. |
| MAC-045 | P1 | Restart/reopen persistence | Saved settings/layout/project choices, outstanding sessions/forwards, fixture data and actual engine identity after supported restart. |
| MAC-046 | P1 | Cross-cutting appearance/accessibility | Dark/light, representative small/large windows, long names/large lists, focus return, keyboard navigation and permission dialogs. No pixel-equality requirement across OSes. |
| MAC-047 | P2 | Update/uninstall/reinstall | Entry/prompt/cancel, data retention contract and supported source/target versions. Destructive execution last, after verified export and restored remote access plan. |
| MAC-048 | P2 | Physical sleep/wake/Wi-Fi/power/device changes | Not certified by cloud rental, screenshots or a remote disconnect. Needs suitable physical hardware and instrumentation; keep distinct from ordinary app restart. |

## Fixtures and session preparation

Prepare before allocation: pinned multi-architecture images/digests, two Compose
projects, ordered override files/profiles, healthy/unhealthy/fast-exit/shell-free
and read-only containers, noisy logs, bound/named volumes, checksum sentinels,
spaces/Unicode/binary/symlink fixtures, cluster manifests and a disposable Linux
machine. Use per-session labels and names. Confirm licensing for any Pro-only
feature and any migration source before relying on it. Never reuse real user data.
Do not require an amd64-only historical DMG on an Apple silicon host: record the
actual installed architecture and verify official download/build provenance.

Maintain original host/application settings and inventories. Record any context,
socket, trust or PATH change that normal first-run setup actually makes. Do not
silently correct OrbStack defaults before observing them. Use a separate before-
state for trust/setup/migration branches. Do not create a Mac version of OrbWin
or treat Windows-on-ARM virtualization as an equivalent qualification host.

Suggested one-day sequence (budget, not a promise): first hour capability gates;
then environment/fixtures and core capture; interleave question-driven REA
discovery with runtime corroboration; reserve several hours for added branches,
repeated export and gap reconciliation. Export incrementally from the start.
Preserve unknown/blocked rows if time expires; request an extension only when
the remaining useful scope and actual cost are concrete.

## Scaleway capability assessment

Primary documentation checked on 2026-10-24:
[M4 bare-metal product](https://www.scaleway.com/en/mac-mini-m4/),
[SSH quickstart](https://www.scaleway.com/en/docs/apple-silicon/quickstart/),
[remote desktop](https://www.scaleway.com/en/docs/apple-silicon/how-to/access-remote-desktop-mac-mini/),
[FAQ/restrictions](https://www.scaleway.com/en/docs/apple-silicon/faq/),
[REA native observation](https://github.com/morluto/rea/blob/main/docs/native-investigation.md).
Documentation supports candidacy, not an end-to-end compatibility certification.

| Capability | Evidence / present conclusion | Gate before relying on it |
| --- | --- | --- |
| Physical Apple hardware and graphical/terminal access | Scaleway advertises bare-metal M4 with VNC and CLI access. | Verify selected hardware/OS, remote login/control and working shell. |
| OrbStack runtime and nested Linux environment | Plausible on native macOS/Apple hardware; no provider-specific OrbStack certification found. | Install official ARM64 build and run a real container, HTTP publication, mounted sentinel and optional Linux machine/cluster. |
| REA installation/providers | REA capability varies by host, release and provider. Local Windows checks do not qualify the rented Mac. | Record package/hash/version and scoped doctor; run supported source-owned fixture proofs first. |
| REA screenshots and accessibility actions | REA documents selected-window capture on macOS 14+ with Screen Recording/Accessibility consent. No Scaleway confirmation that the needed consent is grantable remotely was found. | In a live GUI session, grant only required consent through authorized human/setup interaction; capture one known window and perform one harmless action, then test OrbStack. Preserve denial evidence. |
| Privileged/system-wide tracing | Scaleway forbids host SIP disablement and restricts recovery/boot policy. REA reports denied debugging for hardened targets without required entitlement. | This is not an unrestricted tracing host. Inspect target signature/provider requirements where authorized; record unsupported tracing rather than modifying vendor protections. |
| File export | SSH is documented; local Windows ssh/scp clients exist. Actual end-to-end transfer not tested. | Transfer a harmless file both directions and verify SHA-256; then test evidence archive extraction on Windows. |
| Physical host transitions | No physical access; no ordinary laptop Wi-Fi/sleep/power equivalence established. | Mark MAC-048 unqualified here. Do not interrupt the only remote access path. |

This assessment cannot certify **everything** without an allocated reachable
machine. No credentials/host are currently available to run those gates. No
machine was rented, account created or charge incurred by preparing this plan.

Before paying, obtain provider confirmation if the user needs assurance beyond
the public documentation. Prepared questions (not sent): Does the selected M4
plan allow OrbStack and its Linux VM? Can an admin in VNC grant Screen Recording,
Accessibility and Developer Tools consent to the selected REA host/helper? Do MDM
profiles block those grants or local certificate trust changes? Can SSH/SFTP/SCP
export arbitrary owned research artifacts? Which exact macOS release is offered?
What is the remedy if the documented remote-access capabilities cannot be used?
Do not send this inquiry or any user information without explicit authorization.

Scaleway's suggested guest-VM workaround for SIP is not proof that OrbStack or
the desired tracing works inside that guest; it can introduce nested-virtualization
and capture limitations. Do not choose it as an assumed workaround.

## First-hour go/no-go gates

1. G0: validate host/build/hardware and SSH + interactive graphical session.
2. G1: prove a harmless two-way file transfer/checksum, plus archive download.
3. G2: prove OrbStack starts and actual fixture Docker traffic/data work.
4. G3: prove REA selected-window screenshot, accessibility read and supported
   harmless action on a source-owned fixture, then the selected OrbStack window.
5. G4: check tooling, license and target boundaries for the requested independent
   REA investigation. Trace/decompile only within separately established rights;
   normal UI observation and binary tracing are different capabilities.
6. Save gate results to the local machine before the full campaign. Partial or
   failed gates do not count as success. Manual VNC screenshots may be a useful
   explicitly labeled fallback but do not satisfy a promised REA capture result.

No blanket SIP/MDM/entitlement bypass or automatic OS-consent grant. OrbStack's
[published terms](https://docs.orbstack.dev/legal/terms) restrict reverse engineering;
renting hardware does not grant additional software rights. Resolve the applicable
authorization for binary investigation rather than implying it from tool access.

## Evidence delivery and comparison

Use this bundle layout, versioned by run/build:

```text
reference-run/
  environment.json
  gate-results.json
  scenario-register.json
  summary.md
  gaps.md
  discoveries/          # REA questions/findings, source IDs and evidence levels
  scenarios/MAC-001/    # steps.json, observations.md, screenshots/, recordings/, logs/
  fixtures/            # definitions/digests and owned-resource inventories
  checksums.sha256
```

Each run record includes observed time, outcome, error/unknown and artifact paths.
Export raw REA records as supported by the pinned version alongside summaries.
Keep proprietary/internal investigation material separate from implementation-
facing behavior specifications. Do not bundle secrets, license tokens, private
keys, original app binaries or third-party packages for redistribution.

Download an archive incrementally using the proved channel; verify every manifest
checksum and extract/open representative documents/images locally. Export final
coverage/discovery/gap registers, checksums and cleanup record before deletion.
Set the provider's deletion deadline only with enough verified export margin;
never rely on a promise that files will be sent later from a deleted machine.

For side-by-side review, pair each captured Mac scenario with the corresponding
Windows journey, equivalent data and comparable content dimensions/themes. Keep
OS differences visible. Mark each result: equivalent outcome, intentional Windows
adaptation, missing capability, interaction defect, or unknown. Preserve the raw
images; composites are navigation aids, not original evidence. A screenshot match
does not replace the Windows compatibility/preservation suite or establish a
cross-hardware performance advantage.
