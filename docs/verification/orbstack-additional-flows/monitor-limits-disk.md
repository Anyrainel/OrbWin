# Additional OrbStack flows: activity monitor, resource limits, disk usage

Status: research record and Phase 2 scope proposal, written 2026-10-24. It does
not change accepted scope. It follows the flow template in
[the OrbStack behavior catalog](../../research/orbstack/flows.md); that folder is
owned by the research chat and is not edited here.

Evidence labels: **documented** = stated on an official OrbStack page fetched
2026-10-24; **reported** = stated by users in a public issue; **unknown** = no
evidence. Nothing here was observed on a Mac. OrbWin statements are labelled
**source** (read from this repository) or **observed** (recorded test evidence).

Sources fetched 2026-10-24:

| Key | URL |
| --- | --- |
| RN | https://docs.orbstack.dev/release-notes |
| SET | https://docs.orbstack.dev/settings |
| FAQ | https://docs.orbstack.dev/faq ("Why is there an 8 TB data file?") |
| GH29 | https://github.com/orbstack/orbstack/issues/29 |

Correction to the brief: GH29 ("Impossibly large file at
~/.orbstack/data/data.img", opened 2023-03-10, closed 2024-04-10) has **92
comments and 34 thumbs-up**, not 92 thumbs-up (GitHub API, 2026-10-24). The
release notes also put network and disk-I/O graphs in **1.11.2**, not only in
1.11.0/2.0.0.

## OSX-MON-001 — Watch engine and container activity

**Actor/intent:** developer wants to see which containers, machines or pods are
using CPU, memory, disk or network, and how that changes over time.

**Entry:** Activity Monitor view in the app (RN 1.11.0); per-container Activity
Monitor tab (RN 2.1.1); `orb top` terminal UI (RN 2.1.2).

**Documented sequence (RN):**

| Version | Documented release-note item |
| --- | --- |
| 1.11.0 | Activity Monitor for CPU, memory and disk usage; customizable views for containers, machines and Kubernetes pods |
| 1.11.2 | Network stats and graph, disk I/O graph and totals; graphs follow the selection instead of always showing totals |
| 2.0.0 | Activity Monitor graphs |
| 2.1.1 | Per-container Activity Monitor tab; fixed always scrolling to the selected item |
| 2.1.2 | Adjustable update frequency; `orb top` TUI |

**Branches (documented):** totals compared with selection-following graphs;
grouping by container, machine or pod; GUI compared with TUI.

**Unknown:** available frequencies and default; history length and whether it
persists; how CPU percent is normalized (per core or whole limit); how memory is
attributed (cache or RSS); behavior while the engine is stopped or starting; sort
and column options; `orb top` keys, exit codes and non-TTY output; what the view
costs in overhead; accessibility of graphs.

## OSX-LIM-001 — Set engine-wide memory and CPU limits

**Actor/intent:** developer stops containers from starving the host, or gives a
heavy build more headroom.

**Entry:** Settings (SET); CLI configuration keys `memory_mib` and `cpu` (SET).

**Documented sequence (SET):** the memory limit is the total that containers and
machines may use, "no more than 8 GB by default". The CPU limit is a percentage;
the page says "This is not a reservation". The CLI value is a multiple of 100%,
so `7` means 700%. Memory "is released when it's no longer used". RN 0.6.0 added
CPU limiting. RN 1.7.0 added dynamic memory: memory is auto-released when no
longer used, including for running containers and machines. RN 2.2.4 returns more
unused memory to macOS.

**Branches (documented):** GUI or CLI key; the limits apply to containers and
machines together.

**Unknown:** whether a change needs a restart and what happens to running
workloads; validation bounds and error text; behavior when the guest hits the
memory limit (OOM target, notification); how quickly memory is released; the
CPU default.

## OSX-LIM-002 — Set per-machine CPU, memory and disk limits

**Actor/intent:** developer constrains one Linux machine without changing the
engine-wide limits.

**Entry:** machine settings (location in the UI is unknown).

**Documented sequence (RN):** 2.2.0 added per-machine CPU, memory and disk
limits. 2.2.2 fixed a crash when changing machine resource limits. 2.2.3 fixed
limits being reset after an app crash. SET does not describe per-machine limits.

**Unknown:** UI, CLI and keys; how per-machine limits relate to the global limit;
whether a disk limit can shrink below current usage; restart needs; what a
workload sees when it reaches a limit.

## OSX-DSK-001 — Understand and manage disk usage

**Actor/intent:** developer sees a huge data file or low free space and wants to
know the real usage and how to reclaim it.

**Entry:** storage settings (RN 2.2.0); volume list refresh (RN 2.0.1); Finder or
the shell on `data.img` (FAQ, GH29).

**Documented sequence:**

1. `data.img` is a sparse virtual disk image with an apparent size of 8 TB. It
   "only takes as much space as you use, and automatically shrinks when you
   delete data". The size on disk is the one that matters (FAQ).
2. Storage usage is shown as a capacity bar (RN 2.2.0); the bars were improved
   in 2.2.2.
3. Volume disk usage has a refresh button (RN 2.0.1). New installs calculate
   machine disk usage instantly (RN 1.10.0).
4. Data can be moved to an external drive (RN 0.13.0).
5. To delete data: `orb delete docker` for Docker data, `orb reset` for
   everything (SET).

**Branches:** before migrating with Migration Assistant, delete or `tar` the
image, because sparse files are not handled (FAQ, documented). Exclude the data
directory from backups (FAQ, documented). Users reported backup tools
(Backblaze) and `ls` showing a file larger than the system disk; the maintainer
marked this as intended behavior (GH29, reported). RN 1.9.0 fixed the image
"growing unexpectedly" (documented).

**Unknown:** how quickly space is reclaimed and how it works (TRIM/punch-hole);
what the capacity bars break down into; free-space warnings; what happens when
the host disk fills; the external-drive flow (move compared with copy, drive
disconnect, cancellation and rollback); refresh cost and cancellation.

## OrbWin Windows adaptation

### Current baseline (source/observed)

| Area | What exists | Evidence |
| --- | --- | --- |
| Per-container stats | `StatsPanel` polls `docker stats --no-stream` every 3 s while visible; CPU/memory sparklines with 60 in-memory samples; pause/live toggle; text alternative for each sparkline; resets on target change | source: `src/components/resources/StatsPanel.tsx`; [parity table](../orbstack-parity-2026-10-23.md) |
| Engine-wide activity | None. No aggregate graphs, network or block I/O history, frequency setting or CLI TUI | source |
| Resource settings | Read-only: requested CPUs and memory compared with CPUs and memory visible to Docker | source: `Settings.tsx`; observed mismatch, BL-006 |
| Disk usage | `docker system df` table (count, active, size, reclaimable) plus Windows virtual-disk file length and path, with an explicit statement that compaction is not automatic; manual cancellable Measure action | source: `src/components/settings/DiskUsage.tsx`; [phase-1 coverage](../../phase-1-coverage.md) |
| Relocation / backup | No supported standalone transaction | BL-016, P2-D02 |

### WSL constraints (Microsoft-documented, not re-verified here)

- All WSL 2 distributions share one utility VM. `.wslconfig` `memory`,
  `processors` and `swap` are global to every distribution, including the user's
  development distributions and other engines' distributions, and take effect
  only after `wsl --shutdown`. An OrbWin-only engine limit equivalent to
  OSX-LIM-001 therefore has no direct WSL primitive. BL-006 observed 16 CPUs and
  about 49.3 GB visible despite a request for 2 CPUs and 2 GiB.
- Possible scoped mechanisms are a cgroup limit inside the engine distribution
  (for example a parent cgroup for dockerd/containerd) or per-container Docker
  limits. Both need upstream `rdd` audit and live proof; neither releases memory
  to Windows on its own.
- Memory return to Windows depends on WSL (`autoMemoryReclaim` in the
  `[experimental]` section, global). OrbWin must not toggle it silently
  (AGENTS.md coexistence rule).
- Distribution disks are VHDX files. Sparse VHDX (`sparseVhd`,
  `wsl --manage <distro> --set-sparse`) and offline compaction are WSL/Windows
  features. Compaction needs the distribution stopped and is a data-safety
  operation. A Windows-reported file length is not usage inside the guest.
  Docker Desktop and Rancher each own separate VHDX files that OrbWin must never
  touch.
- Per-machine limits (OSX-LIM-002) do not apply: OrbWin does not manage user
  distributions as machines. Per-container limits (`--memory`, `--cpus`) are the
  nearest honest analogue.

### Remaining observable gap

| Flow | Gap | Proposed honest adaptation |
| --- | --- | --- |
| OSX-MON-001 | No engine-wide view; no network or block I/O history; fixed 3 s interval; no TUI | Activity view that aggregates `docker stats` across running containers (and Compose projects), with selection-following graphs, network/block I/O and a bounded interval choice. Label data as engine-reported and not persisted. A TUI is a P2-D03 CLI decision, not a default. |
| OSX-LIM-001 | No editable limit; requested values are not enforced (BL-006) | Either (a) an engine-scoped cgroup limit proven to enforce without affecting other distributions, or (b) guided, explicit `.wslconfig` editing labelled as a global WSL change with a restart warning and a preview of affected distributions, or (c) keep it read-only and document the exclusion. |
| OSX-LIM-002 | Not applicable as machines | Per-container memory/CPU limits on Run/edit, if Docker update semantics qualify. |
| OSX-DSK-001 | No capacity bar; no host free space; no compaction or relocation; per-volume size refresh not qualified | Capacity bar: guest usage against the virtual-disk length and host volume free space, each with its source labelled. Per-volume `df -v` refresh. Compaction and relocation only through P2-D02 with backups and preflight. |

### Acceptance scenarios

| State | Scenario |
| --- | --- |
| Loading | First sample shows a measuring state, not zeros; capacity bar shows "Measuring" until both sources answer |
| Empty | No running containers: an explicit message that nothing is running, with no blank graph; engine stopped: views disabled with a reason |
| Error | Stats, `df` or WSL query failure shows the command and error text per source; one failing source does not blank the others; an unreadable sample is skipped and counted |
| Cancel | Leaving the view or pausing aborts in-flight polls; a slow `df -v` can be cancelled; a cancelled limit or `.wslconfig` edit leaves files byte-identical |
| Keyboard | Activity table is sortable and selectable by keyboard; interval chooser and pause are reachable in tab order; graphs follow keyboard selection |
| Accessibility | Every graph has a text summary (latest, peak, sample count); colour is not the only signal; values have units; capacity bar exposes a value and a label |
| Overhead | Measure monitor overhead (CPU of OrbWin and dockerd) at each interval with 0, 10 and 50 containers; polling stops when hidden |
| Limits | Requested and effective values are shown side by side; enforcement is proven by a memory/CPU stress container; Docker Desktop and Rancher distributions are unchanged; restart impact is stated before applying |
| Data safety | No compaction, relocation or limit change runs without explicit confirmation, a stopped-engine precondition and a free-space preflight; interruption leaves the original VHDX usable; other engines' disks and user distributions are never opened for write |
| Coexistence | Any global `.wslconfig` change is opt-in, previews the diff, backs up the prior file and states that it affects all WSL distributions |

## Proposed Phase 2 scope rows and decisions

1. **P2-MON / activity monitor (row):** an engine-wide and per-container
   activity view with CPU, memory, network and block I/O, an adjustable bounded
   interval, selection-following graphs and measured overhead. Builds on
   `StatsPanel`.
2. **P2-D06 / resource-limit scope (decision):** choose between an engine-scoped
   cgroup limit (proof required), explicit global `.wslconfig` editing, or
   read-only with a documented exclusion. Closing BL-006 depends on this.
   Includes whether per-container limits are offered.
3. **P2-STO addition (row extension):** capacity bar with host free space,
   qualified per-volume size refresh, and an honest sparse/VHDX explanation
   equivalent to the OrbStack FAQ (apparent compared with allocated size; backup
   tool guidance).
4. **P2-D02 addition (decision):** whether VHDX compaction and data relocation to
   another drive are offered, with preflight, stopped-engine, interruption and
   coexistence evidence; otherwise state the manual WSL procedure and its risks.
5. **P2-D03 addition (decision):** whether an `orb top`-style terminal view is
   part of the CLI convenience scope.
