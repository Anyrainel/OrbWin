# Phase 2 scope and completeness evaluation

2026-10-24 Pacific. Evaluation baseline v1, after the user confirmed the scope:
add the missing OrbStack-inspired development conveniences to OrbWin, including
local names/HTTPS, richer Compose/files/storage/WSL workflows and migration,
while preserving Phase 1 compatibility and data safety.

## Finding

The earlier seven-family task list was directionally correct but not sufficient
to establish completeness. It omitted the already-recorded Debug Shell gap and
left important user outcomes hidden inside broad labels. In particular, file
preview is not editor integration, a browser HTTPS link does not establish trust
from WSL/containers, and additional Compose flags do not complete project reuse.
Conversely, listing backup, reset and relocation together does not establish that
every whole-engine transaction is a required Phase 2 deliverable.

This document decomposes the accepted goal into observable outcomes, test gates
and explicit decisions. It is an implementation-facing Windows adaptation, not
an assertion of exact OrbStack parity or an approved dependency selection. No
claim that all relevant behaviors have been discovered or implemented is made.
Unresolved scope rows must receive evidence and a recorded disposition before
Phase 2 acceptance; difficulty alone is not permission to remove a core outcome.

## Evidence and present state

- [Roadmap](roadmap.md), decisions D05/D12/D15/D20/D24/D25/D28/D29, and the
  [quality standard](quality-standard.md) define product intent and constraints.
- [Accepted Phase 1 accounting](phase-1-coverage.md) and its
  [final assessment](verification/phase-1-final-assessment.md) supersede historical
  open-status entries. Existing operations must not be reclassified as absent.
- Independent research OS-003/004/005/006/007/008/009 and the
  [public flow review](verification/orbstack-ui-flows-2026-10-23.md) establish
  relevant documented conveniences. Research remains owned by its separate chat.
- Official pages rechecked on 2026-10-24: [domains](https://docs.orbstack.dev/docker/domains),
  [HTTPS](https://docs.orbstack.dev/features/https),
  [native files](https://docs.orbstack.dev/features/native-files),
  [Debug Shell](https://docs.orbstack.dev/features/debug),
  [settings](https://docs.orbstack.dev/settings),
  [file sharing](https://docs.orbstack.dev/machines/file-sharing),
  [CLI](https://docs.orbstack.dev/headless), and
  [Kubernetes](https://docs.orbstack.dev/kubernetes).
  These are vendor-documented behaviors, not locally observed Mac behavior.
- The [additional-flows review](verification/orbstack-additional-flows/) (2026-10-24)
  adds rows and decisions found in OrbStack's public release notes, settings,
  FAQ and issue tracker; see the section below the decision table.
- The [first Compose milestone](verification/phase-2-compose-2026-10-24.md) adds
  ordered overrides, optional profiles and quiet validation. It is tested locally;
  it does not complete the entire Compose journey or replace the installed package.

## Core work and acceptance outcomes

The following rows are the implementation baseline derived from the accepted
goal and already-recorded gaps. Exact mechanisms and supported matrices require
design/qualification. The implementation chat owns feature code and evidence.

| ID / workstream | Observable completed outcome | Current baseline | Required acceptance evidence |
| --- | --- | --- | --- |
| P2-CMP / Compose projects | Open an existing project and reuse its reviewed file set, working directory, project name and supported profile/service choices without repeated manual entry. Manage useful project/service actions and aggregate/filter logs. | First file-set/profile/validation milestone tested; discovery and retained project inputs incomplete. | Ordered multi-file labels, ambiguous/moved/missing files, env/interpolation, profiles, selected-service identity, external recreation, partial startup, cancellation, retry, remove scope and volume retention. Never guess file order or infer membership from names. |
| P2-NAM / local service names | Reach supported container and Compose services through predictable local names and useful discovered links; web access avoids manually tracking host-port numbers. | Qualified localhost publishing only. | Discovery/reconciliation after create/recreate/rename/stop, route selection and explicit override, multiple listeners, non-web behavior, collisions with other engines, occupied ports, stale routes, disabled/removed integration and UI-independent lifetime. Resolve client coverage jointly with P2-TLS. |
| P2-TLS / local HTTPS and trust | Supported local service names have usable HTTPS without configuring each application manually; setup, denial, removal and failure are understandable and reversible. | No local CA/proxy/trust feature. | Key ownership/protection, hostname restrictions, expiry/rotation, consent/privilege scope, denied/partial trust, cleanup, browser/client matrix, restart and uninstall/update retention. Explicitly test Windows, selected WSL and supported container clients; record unsupported trust stores instead of disabling verification. No blanket mutation of every image. |
| P2-FIL / files and native tools | Inspect image files read-only; browse/copy and safely edit supported container/volume data, with a useful Windows Explorer/editor path where feasible. Access selected WSL project files through OS mechanisms. | Bounded container directory/text reads, including mounted-volume data, only. | Standalone image/volume access, running/stopped targets, large/binary files, paths/symlinks, ownership/permissions, read-only refusal, concurrent writers/replacement, interrupted transfer/save, no silent overwrite and cleanup of any helper resources. A preview alone does not close this row. Do not promise transactional application/database backups. |
| P2-DBG / minimal-container debugging | Launch useful diagnostic tools for minimal/distroless/read-only containers without installing packages in the target image/container. | Ordinary container exec requires a usable target executable; broader Debug Shell is absent. | Explicit target identity, namespace/privilege boundary, tool provenance, TTY/signals/resize, target exit/recreation, cancellation and helper cleanup, preserved target packages/data. Choose a bounded supported local scope; remote debugging is a separate decision. |
| P2-WSL / ordinary project integration | Select a normal Windows or user-WSL project, run its Docker/Compose/build workflow with useful native paths, and reopen/reconnect it without manually maintaining internal export paths. | Qualified C/F-drive paths and explicit selected-Ubuntu exports; user distro must remain live. | Non-root users, spaces/Unicode, native Linux paths, file/directory/read-only binds, file changes and permissions, distro idle/exit/restart, source replacement, disconnect and data preservation. No hidden global shell/socket takeover, distro destruction or silent keepalive policy. Explain residual WSL constraints. |
| P2-STO / persistent development data | Make persistent data easy to find, attach, copy/export and restore within a clearly defined supported scope, with explicit workload-consistency limits. | Volume management, usage, selected cleanup and preservation exist. No supported product backup/restore transaction. | Volume/project mapping, unused/in-use data, free-space checks, manifest/version/checksum, faithful restore, quiescence requirements, partial writes and interrupted import/export, exact destination ownership, conflicts and source retention. Define the relationship with file operations and migration; do not count the same copy feature as three completed transactions. |
| P2-MIG / migration and transition | Move supported development projects and persistent data from explicitly supported Rancher/Docker Desktop versions without harming the source or silently changing global defaults. | Source/API audit only; no migration transaction. | Version/source/target matrix, preflight and inventory, explicit selection, projects/images/volumes considered separately, consistency/space requirements, secrets/settings policy, resumability or safe abort, target collisions, validation and source preservation. Kubernetes state/credentials are separately accounted, never implied by image or volume transfer. |
| P2-QUAL / product integration | Ship the combined conveniences as a coherent runnable product in an explicit supported matrix. | Phase 1 accepted locally; first Phase 2 milestone tested in development build. | Setup/use/conflict/error/cancel/restart/remove/recovery for every included flow; keyboard/accessibility/visual checks; lifecycle independent of UI; three-engine coexistence; Docker/Compose/Buildx/Testcontainers/WSL/optional-cluster regressions; data preservation; resource-budget comparison; reproducible installed upgrade and cleanup. |

## Decisions that the short task list concealed

These are required evaluation tasks, not automatic feature promises or silent
deferrals. Record the chosen support, original Windows adaptation or justified
exclusion and its effect on the user journey. Apply project decisions before
adding infrastructure. Use isolated proofs where risky.

| ID | Decision to resolve | Why it matters / boundary |
| --- | --- | --- |
| P2-D01 / network and trust client matrix | Which names/protocols work from Windows, WSL, containers and the optional cluster? How do proxy/VPN/split-DNS, offline use and enterprise trust restrictions affect them? | HTTPS needs a client/trust story; browser-only success cannot imply container or WSL success. Reuse OS/upstream settings where possible. Standalone proxy transactions are currently absent (BL-016). General managed-environment breadth expands in Phase 4; the Phase 2 supported network envelope must still be real and explicit. |
| P2-D02 / storage transaction scope | Which volume/project export/restore, whole-engine backup/restore, relocation and Docker-only/product-reset operations are included now? | These have different consistency and recovery contracts. The backend has deletion primitives, not the combined transactions. Safe development-data transfer is core; whole-engine operations need an explicit outcome-based decision, including the Phase 4 release overlap. No reset as routine recovery. |
| P2-D03 / desktop and CLI convenience | Are tray/status/quick actions, opt-in login start, an ergonomic explicitly targeted CLI entry and native editor/terminal launch needed to complete the supported daily journey? | Headless backend lifetime is already preserved. A Windows tray is an adaptation, not a literal macOS menu bar. Starting at login must be opt-in; no global context/PATH changes without reviewed scope. A frontend closing must never stop healthy workloads. |
| P2-D04 / WSL/editor/SSH integration | What editor-open, CLI invocation and SSH/agent conveniences are needed beyond project paths, and which are already supplied adequately by Windows/WSL? | Avoid duplicating machine management, silently accessing agents/credentials or building a second distro manager. Standard WSL/native tool integration can satisfy the outcome without copying macOS mechanisms. |
| P2-D05 / optional Kubernetes convenience | Should local naming/HTTPS extend to supported Kubernetes services/Ingress in Phase 2, or is the qualified persistent-forward adaptation retained? | OrbStack documents integrated service naming/access; preserving Kubernetes as a positive feature requires explicit accounting. Do not infer host access from cluster DNS or install an Ingress controller implicitly. |
| P2-D06 / BL-017 recovery | Can guest-device loss safely recover without a disruptive restart, and what is the smallest useful fix? | The ledger explicitly targets Phase 2 investigation. Retain failed tap-loss evidence and truthful explicit recovery if automatic repair is infeasible. This is not ordinary VPN transition evidence or authority to restart during tests. Broader comparative reliability improvements remain Phase 3. |
| P2-D07 / remaining feature inventory | Cross-check the current feature catalog and relevant ordinary developer journeys; record missing flows and deliberate exclusions. | This evaluation found Debug Shell missing from the earlier list. Sound/USB, Apple-specific acceleration, custom hypervisor/network stack, direct-IP/L2 parity, arbitrary VM management and remote fleet management are not implied requirements of the accepted Windows server/service-development scope. Expanding them needs a concrete need and architectural fit. |

2026-10-26 disposition accounting: [development-data](architecture/development-data.md)
retains portable transfer, clone and project migration in Phase 2; whole-engine
backup/reset/relocation remain Phase 4 lifecycle work. The compaction addendum
selects explicit manual-maintenance guidance and truthful allocation labels,
without an integrated compact action. [Kubernetes access](architecture/kubernetes-access.md)
retains explicit localhost TCP forwards for the application lifetime: navigation
survives, app exit disconnects. These decisions do not close installed/current
source acceptance, new forwarding protocol/TLS checks or core project handoff.

## Additional flows found by the 2026-10-24 public-documentation review

Evaluation baseline v2. A second review of OrbStack's official documentation,
release notes and public GitHub issues found flows that the rows above did not
name. Each is written up in
[orbstack-additional-flows](verification/orbstack-additional-flows/) with every
claim labelled documented, reported or unknown. These are vendor-documented
behaviors, not observed Mac behavior, and they do not prove that no other flows
exist. Rows below are proposals for the same evidence-and-disposition process as
the core rows: none is implemented, and none is an approved dependency.

Sources: [monitor, limits, disk](verification/orbstack-additional-flows/monitor-limits-disk.md),
[image/volume portability](verification/orbstack-additional-flows/image-volume-portability.md),
[routing, Dev Containers, networking](verification/orbstack-additional-flows/routing-devcontainers-networking.md),
[lifecycle, CLI, exclusions](verification/orbstack-additional-flows/lifecycle-cli-and-exclusions.md).

### Additional rows

| ID / workstream | Observable completed outcome | Baseline | Required acceptance evidence |
| --- | --- | --- | --- |
| P2-MON / activity monitor | Engine-wide and per-container CPU, memory, network and block I/O with a bounded, adjustable interval. | Per-container CPU/memory sparklines only (`StatsPanel`). | Measured sampling overhead, stopped/recreated targets, stats failure independent of inventory, bounded retention, keyboard and accessible text equivalents. |
| P2-STO addition / capacity | Capacity bar with host free space, qualified per-volume size refresh, and an honest explanation of sparse VHDX versus allocated size. | Disk-usage table and virtual-disk length only. | Free-space source, refresh cancellation, in-use volumes, backup-tool guidance. |
| P2-STO-IMG / image archive | Export an image to a new `.tar` and import with explicit tag-conflict handling, using Docker's archive format. | None. | Large archives, free-space check, interrupted transfer, tag collisions, cancel, no silent overwrite. Feeds P2-MIG image transfer. |
| P2-STO-CLN / volume clone | Clone a volume into a new owned volume with verification and a stated space cost. | Volume export/restore exists (uncompressed, SHA-256). | In-use source, quiescence, partial copy, collisions. A full copy unless reflink support on the real guest disk is measured; no copy-on-write claim. Sub-item of the P2-D02 transaction, not a separate one. |
| P2-UI-SRT / list polish | Size sort for images and volumes, distinct paused indicator, emulated-platform badge. | Sort by status or name only; paused shows as stopped. | Badge requires verifying guest emulation support (see P2-D07c). |
| P2-UI-NET / creation options | Network create options (driver, subnet, internal) and restart policy/network in the run form. | Basic create/remove. | Validation, conflicts, cancel, accessibility. |
| P2-NAM addendum / routing controls | Explicit HTTP/HTTPS port override by label and UI, several exact names per container with per-name ports, documented forwarded headers, cross-project container access on selected networks. | Qualified localhost publishing only. | Collisions, stale routes, multiple listeners, removal; ties to P2-TLS constrained-signer boundary. |
| P2-DEV / Dev Containers | Open a Windows or WSL folder in a container through VS Code Dev Containers, including private pulls and Compose-based builds. | Not accounted beyond failure seed R-002. | Closure of BL-005 (Windows-path binds from the ordinary CLI), credential visibility (P2-D09), opt-in non-default context or documented targeting, Compose-build gate, no silent global context change. |
| P2-LAN / LAN exposure | Opt-in exposure of published ports to the LAN with scoped Windows Firewall consent and removal. | Localhost only; LAN unsupported. | Consent, per-port scope, removal, restart, three-engine coexistence. Localhost stays the default. |

### Additional decisions

| ID | Decision to resolve | Why it matters / boundary |
| --- | --- | --- |
| P2-D01 addendum | Record support for `host.docker.internal`, `::1`-first name handling, proxy auto/none/URL, VPN/split-DNS and subnet-conflict detection. Container IPv6 deferral. | These form the supported network envelope. Reuse OS and upstream settings. |
| P2-D02 addendum | Whether VHDX compaction and data relocation to another drive are offered. | Needs preflight, stopped-engine, interruption and coexistence evidence; otherwise state the manual WSL procedure and its risks. |
| P2-D03a | Thin `orbwin` CLI (status, start, stop, logs location, snapshot, targeted docker wrapper, `config get/set`). | Must not mutate PATH, Docker context or kubeconfig; UI-independent. |
| P2-D03b | Opt-in PowerShell completion for that CLI. | Only with P2-D03a; no silent profile edits. |
| P2-D03c | Read-only "update available" check. | Defer to Phase 4 unless a release feed exists; keep Prepare update. |
| P2-D03d | Feedback guidance: issue destination and snapshot. | No automatic upload; secret-free snapshot. |
| P2-D03e | Whether an `orb top`-style terminal view is part of the CLI convenience. | Small; depends on P2-MON. |
| P2-D07a | Docker context and elevation neutrality regression across lifecycle. | Byte-identical global config with Docker Desktop and Rancher present. |
| P2-D07b | Older installer over newer state: refuse before guest mutation. | Qualification only. |
| P2-D07c | Guest binfmt audit for arm64 images and multi-platform Buildx. | Run `--platform linux/arm64` or record the limitation in `backend-limitations.md`. Unknown today. |
| P2-D07d | Security response per pinned version. | Adopt process in Phase 2; publication in Phase 4. |
| P2-D07e | Recorded exclusions and deferrals. | Excluded: Linux machine management, USB/GPU/sound, Podman, Docker Extensions and Scout, direct container IP and L2 routes, host-to-container `--net host` bridging, arbitrary `.local` TLDs, default CA injection into all containers, an OrbWin SSH-agent socket, Compose `dns:` by container name. Deferred with triggers: checkpoint/restore, AI-agent sandboxing, Supabase stacks, Swarm node name, containerd image store, Buildx Kubernetes driver. |
| P2-D08 | Label namespace: `dev.orbwin.*` only, or also read `dev.orbstack.*`; wildcards via on-demand exact leaves. | Wildcard certificates weaken the constrained-signer boundary. Remote icon fetching stays off unless a fetch policy is accepted. |
| P2-D09 | Credential visibility to user-invoked clients (`DOCKER_CONFIG` versus opt-in helper entry). | OrbWin's private Docker config hides its registry credentials from VS Code and user CLIs. No silent `credsStore` change. |
| P2-D10 | Resource-limit scope: engine-scoped cgroup limit (proof needed), explicit global `.wslconfig` editing, or read-only with documented exclusion; per-container limits. | Closing BL-006 depends on it. Per-machine limits have no OrbWin equivalent. WSL limits are global and need a shutdown. |
| P2-D11 | Image transfer uses Docker save/load archives; OrbStack `.tar.zst` volume archives and zstd compression are out until the format is verified. | Avoids a custom image container format and an unverified import path. |

## Completion method

1. Keep the roadmap goal fixed. For each relevant evidence-backed convenience,
   identify the user journey, present implementation and remaining observable gap.
2. Resolve P2-D01 through P2-D07 into versioned supported scope. No unresolved
   decision or residual limitation disappears merely because a feature title was
   checked off. Narrow adapters and OS/upstream reuse remain the default.
3. Give every included row a feature contract with setup, happy path, conflicts,
   failures, cancellation, restart, removal, recovery, ownership and data impact.
4. Implement vertical journeys and collect real outcomes. Research documentation,
   implementation, unit tests, live adapter tests, native review and installed
   qualification are distinct evidence levels.
5. Before phase exit, reconcile every row and limitation against acceptance
   artifacts, inspect an installed candidate, run the preservation/coexistence
   matrix and compare resource budgets. Unfinished work cannot be relabeled as
   complete Phase 2; justified retained adaptations must remain explicit.

There is no defensible percentage complete yet. The first Compose milestone is
done locally, its broader journey is active, and the remaining core rows and
decision tasks are pending. This evaluation increases confidence in coverage;
it does not prove absence of undiscovered requirements or product defects.

## Parallel ownership

The user requested a separate implementation chat on 2026-10-24. That chat owns
feature code/tests and feature-specific architecture/qualification records and
starts with completing the Compose journey. This chat owns this scope evaluation
and its roadmap link. Both use the existing local checkout; avoid concurrent edits
to the same files. Independent OrbStack research remains read-only here. Runtime
qualification must use the reviewed installed identity and preserve other engines.
