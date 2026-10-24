# OrbStack behavior catalog

This catalog records OrbStack behavior only. Source IDs resolve to URLs in [the evidence register](evidence-register.md). All entries are **documented**, except OS-010's explicitly observed screenshot. Confidence is high that the sources say what is summarized; runtime behavior in the supplied 2.2.4-named artifact remains unverified. No undocumented defaults, keyboard shortcuts, confirmations, cancellation semantics or retry behavior are asserted.

## OS-001 — Start and stop from the command line

**Actor/intent:** developer wants to use Docker without opening the main window. **Entry:** `orb` / `orb start`; `orb stop` ends operation. **Documented sequence:** start OrbStack, then use normal Docker commands. Engine restart is exposed as `orb restart docker`; engine logs as `orb logs docker` (O08/O02).

**Branches:** headless use is documented; headless auto-update is not supported. GUI installation/update is a separate path. **Unknown:** already-starting behavior, partial startup, failed downloads, stop ordering, timeout, interrupt/retry semantics, restart persistence and exact exit codes. Keyboard behavior is ordinary CLI invocation only; app shortcuts unknown. Evidence is documentation, not executed commands.

## OS-002 — First use and Docker context

**Actor/intent:** developer installs OrbStack and targets its engine. **Entry:** downloaded app or Homebrew. **Documented sequence:** open installed app and use Docker; Compose tooling is included (O01). OrbStack creates an `orbstack` Docker context and documents automatic use from the terminal. An optional privileged socket symlink improves third-party compatibility (O02).

**Branches:** switching from Docker Desktop can require registry login/credential-store correction because helpers differ. **Unknown:** exact setup screens/order, existing context conflicts and environment overrides, rejected administrator prompt, rollback/cancellation, first-run failures, persistence across upgrades and all UI accessibility behavior. “Ready out of the box” is vendor documentation, not observed onboarding.

## OS-003 — Access a container or Compose service by domain

**Actor/intent:** developer opens a service without remembering host port numbers. **Entry:** container domain, Compose service domain, or running-service index. **Documented sequence:** container name maps to `container-name.orb.local`; Compose uses `service.project.orb.local`. Web requests route without an explicit port. First access probes listeners for HTTP/HTTPS and remembers the result until the container changes (O03).

**Branches:** explicit HTTP-port label bypasses detection; custom/wildcard `.local` names and per-domain ports are supported. Non-web services still use explicit ports. **Unknown:** ambiguous detection selection, collision precedence, failed probes/cache invalidation details, stopped-container response, DNS outage, retry/cancellation, exact GUI editing/link behavior. The source provides routing rules, not a complete error state machine.

## OS-004 — First HTTPS access and trust

**Actor/intent:** developer accesses a local service using HTTPS. **Entry:** HTTPS container domain or HTTPS index. **Documented sequence:** local CA/reverse proxy supplies certificates; first use requests certificate installation. Root CA distribution to containers is documented with an opt-out label. Private-key protection is described using the macOS keychain (O04).

**Branches:** container trust opt-out; explicit HTTPS-port routing; older Firefox trust behavior. **Unknown:** exact prompt/default action, denied installation, locked keychain, expired/rotated CA, browser-specific failures, cancellation/retry, trust removal and persistence after reinstall. No claim that a failed trust step falls back automatically.

## OS-005 — Inspect and modify files

**Actor/intent:** developer examines resource files through native tools. **Entry:** `~/OrbStack`, Finder sidebar or resource folder button. **Documented sequence:** open resource path using Finder/editor/CLI. Container and volume contents can be modified; image contents are read-only. Machine files are also exposed (O05).

**Branches:** read-only image versus writable container/volume. **Unknown:** stopped/deleted-resource access, permissions, concurrent writers, symlinks, filesystem consistency, editor save failures, unmount/disconnect, cancellation, recovery and exact UI/keyboard actions. Documented access is not evidence of transactional data safety.

## OS-006 — Debug a minimal container

**Actor/intent:** developer needs tools absent from the container. **Entry:** container context menu → Debug Shell, or `orb debug <container>`. **Documented sequence:** open debug shell; use included tools or install additional commands. Tool packages are shared across debug sessions and do not modify container packages, including read-only targets (O06).

**Branches:** local target versus specified remote Docker context; missing command can offer package installation. **Unknown:** stopped target, namespace/permission constraints, download failure, package prompt cancellation, reconnect/session lifetime, signal handling and cleanup. This capability is broader than ordinary container exec; its underlying implementation has not been inspected.

## OS-007 — Settings and destructive reset scope

**Actor/intent:** developer configures or resets OrbStack. **Entry:** Settings or `orb config`. **Documented behavior:** settings are available through app and CLI. Resource/network/proxy/engine settings are documented. Docker-only deletion uses `orb delete docker`; whole-product reset uses `orb reset` and includes Docker data and Linux machines (O07).

**Branches:** Docker-only versus all-data scope; some advanced settings are CLI-only. **Unknown:** dialog wording/defaults, draft/apply behavior, restart requirements per setting, validation errors, partial reset, cancellation, retry, backup/undo and persistence after failure. No confirmation or recovery mechanism is assumed.

## OS-008 — Kubernetes lifecycle and service access

**Actor/intent:** developer uses the local cluster. **Entry:** app Kubernetes views or `orb start/stop/restart/delete k8s`. **Documented behavior:** single-node cluster, local image availability, included kubectl, context switching and integrated service access. LAN exposure is optional; default service exposure described as localhost-only (O09).

**Branches:** local-image pull policy; service types and custom cluster alternatives. **Unknown:** first enable sequence, version selection/default, failure/partial readiness, reset confirmations, cancellation/retry, context conflict handling and persistence. Network claims are documented, not reproduced.

## OS-009 — Create and enter a Linux machine

**Actor/intent:** developer wants a Linux environment. **Entry:** choose distribution in app or `orb create ubuntu <name>`; double-click to start or use CLI. **Documented behavior:** machine terminal/CLI use, shared files, SSH and editor integration (O01/O10).

**Unknown:** exact wizard/options/defaults, naming conflicts, image download verification/failure, creation cancellation, cleanup, distro-specific differences, stop/delete confirmation, isolation transitions and recovery. Detailed machine branch logic has not been researched sufficiently to assert it.

## OS-010 — Navigate Kubernetes service details

**Actor/intent:** developer inspects a service. **Observed screenshot O14:** grouped sidebar with Services selected; middle service list grouped under a namespace; selected row; adjacent detail showing Type, Age, Cluster IP, Domain and Ports. Link/delete glyphs appear on rows.

**Unknown:** what click/double-click does, menu contents, deletion scope/confirmation, selection persistence, search/filter mechanics, empty/loading/error states, keyboard order and accessibility. No executable behavior follows from the screenshot alone.

## Areas without adequate flow evidence

Container create/actions/removal, image pull/build/removal, volume creation/deletion, log follow/search/export, ordinary shell, statistics interaction, update/migration/recovery and menu-bar action inventory remain insufficiently characterized. Earlier OW-prefixed flows were proposed behavior for another product and are withdrawn as evidence about these areas. See [observation gaps](observation-gaps.md).
