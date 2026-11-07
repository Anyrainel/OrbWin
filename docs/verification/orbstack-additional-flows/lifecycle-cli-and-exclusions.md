# OrbStack additional flows: lifecycle, CLI and exclusion decisions

2026-10-24 Pacific. Input to [Phase 2 scope](../../phase-2-scope.md) decisions
P2-D03 (desktop and CLI convenience), P2-D04 (WSL/editor/SSH) and P2-D07
(remaining inventory). This is a supplement written outside the research chat.
It does not edit or supersede [the research catalog](../../research/orbstack/flows.md)
(OS-001...OS-010), which already covers start/stop, `orb restart docker`,
`orb logs docker`, settings/`orb config` and reset scope at summary level.

Evidence labels: **documented** = vendor page or release note fetched on
2026-10-24; **reported** = public issue tracker or third-party write-up;
**unknown** = not established. Nothing here was observed on a Mac. Release-note
version numbers are the vendor's; the dates and wording were not reproduced.
"OrbWin today" cites repository state on 2026-10-24, not new qualification.

Sources: [release notes](https://docs.orbstack.dev/release-notes) (RN),
[headless](https://docs.orbstack.dev/headless) (HL),
[settings](https://docs.orbstack.dev/settings) (ST),
[install](https://docs.orbstack.dev/install) (IN),
[downgrade](https://docs.orbstack.dev/downgrade) (DG),
[Docker](https://docs.orbstack.dev/docker/) (DK),
[machines](https://docs.orbstack.dev/machines/) (MA),
[SSH](https://docs.orbstack.dev/machines/ssh) (SSH),
[Docker Desktop comparison](https://docs.orbstack.dev/compare/docker-desktop) (CMP),
[issues by reactions](https://github.com/orbstack/orbstack/issues?q=is%3Aissue+sort%3Areactions-%2B1-desc) (GH).

## A. Updates, product health and CLI

### OSX-LIFE-001 — Update channel and in-app update

**Actor/intent:** developer stays current or opts into earlier builds. **Entry:**
Settings. **Documented:** 1.0.0 made Stable the default channel; Canary is an
opt-in in Settings (RN). **Branches:** headless installs do not auto-update
(HL; see LIFE-003). **Unknown:** check frequency, download/verify/apply sequence,
whether running containers are interrupted, deferral, failure/rollback, channel
switch-back behavior.

**OrbWin today:** no update check, channel or download. Settings offers
*Prepare for application update…*, which quiesces the owned `rdd` through its
own lifecycle and verifies the executable mapping is released; the user then
runs a new per-user NSIS installer, and the next start performs the guarded
bundled-backend replacement ([packaging](../../architecture/packaging.md),
[installation](../../architecture/installation.md)). D30 states automatic updates
are not required yet; roadmap Phase 4 owns signed release updates.
**Gap:** discovering that an update exists; any channel concept. **Acceptance
(if included):** an update check that is read-only and never stops workloads;
the apply path reuses Prepare update + installer + identity-preserving resume;
offline/failed check is a visible, non-blocking state; retained daemon ID and
volume sentinel after update; Docker Desktop and Rancher untouched.

### OSX-LIFE-002 — Scripted update check (`orb update --check`)

**Documented:** 1.10.1 added `orb update --check` "for automated update checks"
(RN). **Unknown:** output format, exit codes, network behavior, whether it can
also apply. **OrbWin today:** none (no OrbWin CLI; see LIFE-008). **Gap/acceptance:**
only meaningful after LIFE-001 has a release feed; a check command must have a
documented exit-code contract and perform no mutation.

### OSX-LIFE-003 — Headless update via Homebrew

**Documented:** auto-update does not work without the GUI; update manually with
`brew upgrade --greedy orbstack`, because Homebrew otherwise leaves self-updating
casks alone (HL). **Unknown:** behavior when the engine is running during the
upgrade. **OrbWin today:** no package-manager distribution; the installer
refuses to overwrite a mapped backend and directs the user to Prepare update.
**Gap:** winget/Scoop manifests are release work (Phase 4). **Acceptance:** a
package-manager upgrade must hit the same NSIS lock probe and never kill `rdd`.

### OSX-LIFE-004 — Downgrade

**Documented:** downgrading is "not officially supported"; it may break
configuration or prevent startup; the page lists older installers (v2.2.2 back to
v1.0.1) and notes v1.5.1 as the last release for older Intel CPUs; bugs caused by
downgrading should not be reported (DG). **Unknown:** any data migration or
version marker check. **OrbWin today:** "No automatic downgrade or disk rollback
occurs" (installation.md); identity/manifest checks refuse mismatched runtime
binaries before guest mutation. **Gap:** behavior of an older installer over
newer engine state is not qualified. **Acceptance:** installing an older build
either works or refuses before guest mutation with a clear message; engine disks
are never rewritten by the refusal.

### OSX-LIFE-005 — In-app feedback and bug reports

**Documented:** 0.15.0 "Send feedback from the app" (RN). **Unknown:** channel,
included data, consent, attachments of logs. **OrbWin today:** command palette
*Save diagnostic snapshot…* writes a user-chosen JSON file (product version,
backend commit, connection status, host diagnostics; `src-tauri/src/main.rs`);
registry tokens are excluded. No submission channel. **Gap:** a project issue
destination and guidance on what to attach. **Acceptance:** never uploads
automatically; the user sees and chooses the file; snapshot contains no
credentials or registry tokens; works when the engine is down.

### OSX-LIFE-006 — Security bulletins

**Documented:** release notes carry security fixes inline, e.g. 1.4.0 Docker
Engine/runc/BuildKit fixes; 2.1.2 a fix for CVE-2026-43284 and a statement of
non-impact for CVE-2026-31431 (RN). **Reported:** no separate advisory page found
by search. **OrbWin today:** exact pins in `upstream.lock.json`,
`runtime-tools.lock.json`, guest package lock and notice inventory make affected
components identifiable. **Gap:** no published advisory practice or response
target. **Acceptance:** for a disclosed CVE in Moby/runc/BuildKit/containerd/Lima
or the WSL kernel boundary, the record states affected/not affected per pin and
the qualified update path. Kernel CVEs are Microsoft-owned (WSL).

### OSX-LIFE-007 — Enterprise managed settings (MDM)

**Documented:** 1.10.1 "MDM support for OrbStack and Docker engine settings" (RN).
ST does not describe keys. **Reported:** a third-party MDM vendor lists OrbStack.
**Unknown:** key set, lock semantics, UI presentation of managed values.
**OrbWin today:** none. **Disposition:** defer to Phase 4 managed-environment
breadth (P2-D01 note); a Windows adaptation would read Group Policy/registry
policy, not invent a parallel config store.

### OSX-LIFE-008 — CLI surface (`orb` / `orbctl`)

**Documented:** `orb`/`orb start`, `orb stop`, `orb config`, `orb restart docker`,
`orb create`, `orb list`, `orb top`, and `orb --help`/`orbctl --help` for more (HL);
`orb logs` for Docker and machine logs since 0.6.0 (RN); `orb docker migrate` (IN);
`orb delete docker`, `orb reset` (OS-007). **Unknown:** the full `--help` output
(not executed), exit codes, JSON output. **OrbWin today:** no product CLI.
Packaged tools are `rdd.exe`, `orbwin-session.exe`, `docker-credential-orbwin.exe`
and pinned Docker/Compose/Buildx under `runtime/tools`; Settings shows explicit
`docker --host <endpoint>` / `DOCKER_HOST` guidance. `rdd` itself remains the
lifecycle owner. **Gap:** an ergonomic, explicitly targeted entry point (P2-D03).
**Acceptance:** a thin CLI that calls the same operations as the UI (start, stop,
status, logs path, diagnostic snapshot, a targeted `docker` wrapper); it must not
become a second orchestrator, must work with the UI closed, must not edit PATH,
Docker context or kubeconfig unless the user opts in, and must have documented
exit codes and cancellation (Ctrl+C stops the client, reports backend work may
continue).

### OSX-LIFE-009 — `orb config` parity with Settings

**Documented:** "All settings can be changed from both the command line
(`orb config`) and the app"; some hidden settings are CLI-only, e.g.
`ssh.expose_port`, `docker.node_name` (ST). **Unknown:** validation and restart
behavior per key. **OrbWin today:** Settings only; requested-resource and proxy
limits explained (BL-006/BL-016). **Gap/acceptance:** if LIFE-008 ships, a
`config get/set` that reads/writes through the same `rdd` settings path as the UI,
rejects unknown keys and reports restart requirements; no CLI-only hidden keys
without a documented reason.

### OSX-LIFE-010 — Logs (`orb logs`)

**Documented:** `orb logs` shows Docker and machine logs (RN 0.6.0);
`orb logs docker` (OS-001). **OrbWin today:** in-app container/Compose/pod logs
are qualified; backend and setup logs are retained under the owned
`%LOCALAPPDATA%/OrbWinData/home`; diagnostics snapshot does not bundle them.
**Gap:** a discoverable path to engine logs from UI/CLI. **Acceptance:** open or
print engine/management log location; bounded output; no log deletion.

### OSX-LIFE-011 — Shell completions

**Documented:** 1.4.0 completions for `docker` and `kubectl`; 1.11.3 for `orb` and
`orbctl` (RN). **Unknown:** shells covered and how they are installed.
**OrbWin today:** none; bundled clients are not on PATH. **Gap/acceptance:** if
an OrbWin CLI exists, provide PowerShell completion that the user installs
explicitly (printed script or documented profile line); never edit `$PROFILE`
silently. Docker/kubectl already ship their own `completion` subcommands.

### OSX-LIFE-012 — Docker context switch-back

**Documented:** OrbStack creates an `orbstack` context used automatically (DK);
to return, stop OrbStack, start Docker Desktop, run `docker context use
desktop-linux` (IN). Credential store may need correcting (DK). **Reported:**
Docker Desktop may reset the current context on its own start/stop (third-party
write-ups). **OrbWin today:** by design OrbWin "does not create or switch a
Docker context and does not change DOCKER_HOST globally" (Settings; D24, BL-001);
scoped clients ignore inherited contexts (`process.rs`). There is therefore no
switch-back step. **Gap:** users of tools that rely on implicit discovery still
need guidance (BL-001 residual). **Acceptance:** with all three engines running,
`docker context show` and the global Docker config are byte-identical before and
after OrbWin install, start, stop, update and uninstall; any future opt-in
context is named `orbwin`, never made current without consent, and removable.

### OSX-LIFE-013 — Opting out of admin prompts (`setup.use_admin`)

**Documented:** 0.16.1 added an option to stop requesting admin privileges (RN);
`orb config set setup.use_admin false` (HL); admin access is used to point
`/var/run/docker.sock` at OrbStack (IN). **OrbWin today:** per-user NSIS install,
no elevation for socket takeover, WSL prerequisites explicit
([installation](../../architecture/installation.md)). **Gap:** none for current
scope. **Acceptance:** keep a regression check that install/setup/start never
request elevation; any future privileged step (e.g. P2-TLS trust, P2-NAM
resolver) must be separately consented and declinable with a working fallback.

## B. Deliberate exclusion and decision records

Dispositions: **core** (required for accepted scope), **adapt** (Windows
adaptation), **defer** (later phase with trigger), **exclude** (outside product).

| ID | OrbStack evidence | User-visible effect for OrbWin users | Disposition and reasoning |
| --- | --- | --- | --- |
| OSX-DEC-001 multi-arch emulation | documented: Rosetta for amd64 on Apple Silicon, `--platform`, `DOCKER_DEFAULT_PLATFORM`, 32/64-bit ARM and x86 images (DK); 32-bit x86 emulation 0.5.0, AVX emulation added 2.0.2 and removed 2.0.3 (RN) | On Windows x64, `linux/amd64` is native. `linux/arm64` images (and multi-platform Buildx output) need QEMU binfmt in the guest. Whether the pinned `rdd` guest registers binfmt is **unknown** (no repo evidence found). Docker documents installing it with `tonistiigi/binfmt`. | **adapt (P2 evaluation)**: audit the guest; if absent, decide between documented user-run binfmt install and a bundled, pinned option. Qualify `docker run --platform linux/arm64` and `buildx --platform linux/amd64,linux/arm64`. Rosetta/AVX are Apple-specific: exclude. |
| OSX-DEC-002 Docker checkpoint/restore | documented: CRIU for machines 1.5.0; Docker checkpoint/restore 2.2.0 (RN) | `docker checkpoint` is an experimental Docker feature; availability on OrbWin unknown. | **defer**: niche for service/CI workloads; revisit if upstream Moby/guest ship CRIU. Do not patch the kernel (Microsoft-owned). |
| OSX-DEC-003 Linux machines (create, isolated, cloud-init, clone/import/export, per-machine limits, SSH) | documented: cloud-init 1.3.0; clone/import/export 1.10.0; isolated machines 2.1.0 (+ selective mounts, host-network block 2.1.1/2.1.2) (RN); per-machine CPU/memory/disk limits, built-in SSH as host `orb`, port 32222, agent forwarding (MA, SSH). `machine.orb.local` naming not found in fetched pages (**unknown**). Reported: #11 Custom VM Image (open, 105 thumbs-up), #342 bridged networking (open, 54) (GH). | Windows users already have WSL distributions with `wsl --import/--export`, cloud-init support in recent WSL, `.wslconfig` limits and native SSH/VS Code Remote. | **exclude as an OrbWin machine manager** (AGENTS.md: user distros independent, no second distro manager; P2-D04). **adapt** only the integration: selected-distro project workflow (P2-WSL). Bridged/L2 networking excluded by D20. |
| OSX-DEC-004 USB, sound, GPU | documented: USB passthrough 2.2.0; webcams/audio/etc. 2.2.2 (RN). reported: #1818 GPU in containers (open) (GH) | WSL offers usbipd-win and WSL GPU paths at platform level; OrbWin adds none. | **exclude from Phase 2** (P2-D07, P15 Phase 4 evaluation). If later pursued, reuse WSL/Microsoft mechanisms and qualify `--gpus` honestly; never ship a custom driver. |
| OSX-DEC-005 Podman | reported: #88 Podman support (open, 37) (GH) | Podman users keep Podman Desktop/WSL. | **exclude**: D09 selects the Docker/Moby ecosystem; a second engine would be a competing source of truth. |
| OSX-DEC-006 AI-agent sandboxing | reported: #2295 `docker sandbox` support (open, 28) (GH); isolated machines described as useful for agent sandboxing (RN 2.1.0) | `docker sandbox` behavior on OrbWin is unknown; ordinary containers remain available. | **defer**: track the upstream Docker feature; adopt only if it is standard Docker CLI behavior that works against the explicitly targeted endpoint. |
| OSX-DEC-007 Supabase one-click stacks | documented: 2.2.3 start/stop Supabase CLI stacks from UI (RN) | Supabase CLI already works through Docker if pointed at the OrbWin endpoint (**unknown**, not tested). | **defer**: vendor-specific; covered generically by Compose project grouping (P2-CMP). Add to the client smoke matrix only on demand. |
| OSX-DEC-008 Swarm node name | documented: 1.3.0; CLI-only `docker.node_name`, default `orbstack` (RN, ST) | Swarm init works under standard Moby (**unknown** on OrbWin). | **defer**: no setting; record as an unqualified Docker capability, not a removed flag (D16). |
| OSX-DEC-009 containerd image store | documented: 2.2.0 option; experimental; app image-file browsing unavailable while enabled (RN, ST) | Multi-platform tags in one local store; affects image files browsing (P2-FIL). | **defer** with trigger: pair with DEC-001 if multi-arch local images become core. Preserve the `rdd` default storage; switching stores is a data-visible change needing migration reasoning. |
| OSX-DEC-010 Buildx Kubernetes driver | documented: 2.2.0 (RN) | Standard Buildx can create a `kubernetes` builder against any kubeconfig, including OrbWin's explicit one (**unknown**, untested). | **defer**: document as standard Buildx usage; no product UI. |
| OSX-DEC-011 Docker Desktop-only features | documented: Extensions, Scout, restrict registry access are absent from OrbStack (CMP) | Same absence in OrbWin. | **exclude** Extensions (Docker-proprietary SDK). Scout: exclude from product; standalone scanners work via standard endpoint. Registry restriction: defer to Phase 4 managed environments with LIFE-007. |

## Proposed Phase 2 scope rows and decisions

| Proposed ID | Proposal | Disposition | Acceptance evidence |
| --- | --- | --- | --- |
| P2-D03a / CLI entry | Thin `orbwin` CLI over existing operations (status, start, stop, logs location, snapshot, targeted docker wrapper, `config get/set`); covers LIFE-008/009/010 | core candidate under P2-D03 | UI closed; three engines running; no PATH/context/kubeconfig mutation; exit codes; Ctrl+C semantics; parity tests with UI operations |
| P2-D03b / completions | Opt-in PowerShell completion for the CLI (LIFE-011) | include only with P2-D03a | No silent profile edits; removal documented |
| P2-D03c / update awareness | Read-only "update available" check reusing Prepare update + installer (LIFE-001/002) | defer to Phase 4 unless a release feed exists; keep Prepare update | Non-mutating check; offline state; identity retained |
| P2-D03d / feedback | Issue destination + snapshot guidance; optional engine-log inclusion chosen by the user (LIFE-005/010) | core (small) | No automatic upload; secret-free snapshot |
| P2-D07a / context neutrality | Formal regression for LIFE-012/013: global Docker context/config and elevation unchanged across lifecycle | core | Byte-identical global config with Docker Desktop and Rancher present |
| P2-D07b / downgrade refusal | Qualify older-installer-over-newer-state (LIFE-004) | core (qualification only) | Refusal before guest mutation, disks untouched |
| P2-D07c / arm64 images | Guest binfmt audit and decision (DEC-001, DEC-009) | evaluate in Phase 2 | `--platform linux/arm64` run and multi-platform Buildx on the supported matrix, or a documented limitation in backend-limitations |
| P2-D07d / security response | Per-pin advisory practice (LIFE-006) | adopt process in Phase 2; publication in Phase 4 | Sample CVE assessment against the lock files |
| P2-D07e / exclusions | Record DEC-003/004/005/011 as exclusions and DEC-002/006/007/008/010 as deferred with triggers | decision | Linked from phase-2-scope; revisited on concrete demand |

Residual unknowns: the full `orb --help` surface, MDM keys, feedback payload and
update mechanics were not observed; OrbWin behaviors marked unknown (binfmt,
Swarm, checkpoint, Buildx Kubernetes driver, Supabase CLI) need test runs before
any support claim.
