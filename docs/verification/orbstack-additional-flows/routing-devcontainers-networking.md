# Additional OrbStack flows: routing, Dev Containers and networking

Reviewed 2026-10-24 Pacific. This record adds OrbStack flows that are missing from
`docs/research/orbstack/` (OS-003/OS-004 cover only basic naming and HTTPS) and maps
each one to a Windows adaptation for Phase 2 scope decisions. It does not edit or
replace the research chat's catalog. Evidence labels:

- **documented**: vendor documentation or release notes fetched on 2026-10-24.
- **reported**: a public GitHub issue or discussion. Reporter claims only; not reproduced.
- **unknown**: not stated by any source we read. Not inferred from OrbStack's behavior.
- **OrbWin observed**: repository artifacts or code. **OrbWin proposal**: not implemented.

No Mac runtime was observed. Doc pages carry no version numbers, so per-version
claims come only from [release notes](https://docs.orbstack.dev/release-notes) (RN).

| Source key | URL / identity |
| --- | --- |
| DOM | https://docs.orbstack.dev/docker/domains |
| TLS | https://docs.orbstack.dev/features/https |
| NET | https://docs.orbstack.dev/docker/network |
| HOST | https://docs.orbstack.dev/docker/host-networking |
| DKR | https://docs.orbstack.dev/docker/ (context, SSH agent, credential store) |
| FAQ | https://docs.orbstack.dev/faq |
| GH-1984 | [orbstack#1984](https://github.com/orbstack/orbstack/issues/1984) "Can't request orb.local after update macOS Tahoe": 29 thumbs-up on 2026-10-24, closed 2025-08-22 by a maintainer who said "Fixed in v2.0" |
| GH-33 | [orbstack#33](https://github.com/orbstack/orbstack/issues/33) direct container IP: 39 thumbs-up, closed 2023-06-13. RN v0.12.0 lists direct-IP connection |
| GH-2645 | [discussion #2645](https://github.com/orgs/orbstack/discussions/2645) container name for Compose `dns:`: 1 upvote, open feature request |
| GH-2747 | [orbstack#2747](https://github.com/orbstack/orbstack/issues/2747) Dev Containers Compose builds broken after 2.2.4 (bundled Buildx): open |
| GH-2142 | [orbstack#2142](https://github.com/orbstack/orbstack/issues/2142) enabling "Allow access to container domain & IP" reportedly interrupts host networking: open |
| GH-2334 | [orbstack#2334](https://github.com/orbstack/orbstack/issues/2334) private VPN DNS names not resolving inside OrbStack: open |
| VSC-H, VSC-G | [Dev Containers remote host](https://code.visualstudio.com/remote/advancedcontainers/develop-remote-host), [sharing Git credentials](https://code.visualstudio.com/remote/advancedcontainers/sharing-git-credentials) |

## Domain routing controls

### OSX-NET-001 — Explicit HTTP/HTTPS port labels

**Actor/intent:** a developer's container listens on several ports, or detection picks
the wrong one. They pin the web port. **Entry:** container or Compose `labels`.
**Documented sequence:** `dev.orbstack.http-port=8080` sets the port and skips detection.
The automatic HTTPS proxy also uses it (DOM). `dev.orbstack.https-port=8443` redirects
443 to another port (TLS). RN v2.1.1: probing is skipped when `http-port` is set.
RN v1.10.1 and v1.11.3 fixed these labels being ignored or overridden.
**Branches:** with no label, the first connection probes listeners with the
`OrbStack-Server-Detection` user agent and caches the result until the container
changes (DOM). **Unknown:** invalid or non-listening label values, both labels set to
conflicting ports, label change without recreate, and how errors are shown in the UI.

### OSX-NET-002 — Custom, wildcard and per-domain-port names

**Actor/intent:** expose one container as several hostnames (app/admin/api) or as
tenant subdomains. **Entry:** `dev.orbstack.domains` label. **Documented sequence:**
the label takes a comma-separated list such as `foo.local,bar.local`. `*.foo.local`
wildcards are allowed. Default names are also wildcards (`*.container.orb.local`).
Only the `.local` TLD is supported (DOM). Per-domain ports use `name@port`, with
`:https` for TLS backends, for example `api.local@8443:https`. This also works for
the container's own domain and for wildcards (DOM; RN v2.2.4). RN v1.4.0 adds
multi-level wildcards. RN v2.2.4 shows all of a container's custom domains in the UI
and fixes domains pointing to a recreated container's old IP. RN v2.2.2 fixes
`docker compose run` containers taking over domains.
**Branches:** non-web services still use explicit ports, for example
`database.stack.orb.local:5432` (DOM). **Unknown:** precedence when two containers
claim one name, exact multi-level matching rules, and conflicts with real mDNS `.local`
hosts. The release-note fixes show that stale routes and takeover happened (reported
history), but the current rules for them are not documented.

### OSX-NET-003 — Forwarded client identity through the HTTPS proxy

**Documented:** RN v1.4.0 says "Added X-Forwaded-For to HTTPS proxy" (that spelling is
in the source). DOM and TLS do not mention forwarding headers. **Unknown:** whether
`X-Forwarded-Proto/Host` are set, whether client-supplied headers are trusted or
replaced, and what the plain-HTTP path does. Apps that build absolute URLs or
enforce CSRF by origin depend on these details.

### OSX-NET-004 — Names from inside containers, across Compose projects

**Actor/intent:** service A in one Compose project calls service B in another project
by its stable name over HTTP or HTTPS. **Documented sequence:** RN v1.4.0 makes
`orb.local` work across Compose projects. RN v1.6.4 adds HTTP port redirection when
containers use `orb.local` names. RN v1.8.0 adds a container-to-container HTTPS proxy.
RN v1.9.0 makes containers trust the certificates. TLS: containers trust the CA by
default, with the opt-out `dev.orbstack.add-ca-certificates=false`.
**Reported:** GH-2645 asks to accept a container name in Compose `dns:` instead of a
static IP. Docker requires an IP there. **Unknown:** whether redirection is a 3xx or a
transparent port rewrite, and behavior for images with nonstandard trust stores.

### OSX-NET-005 — Host and engine names (`host.docker.internal`, `docker.internal`)

**Documented:** containers reach Mac services through `host.docker.internal` (NET).
RN v0.1.9 added it "and other domains" for Docker Desktop compatibility. RN v0.4.0
added `docker.internal` so Linux machines can reach Docker. DKR uses
`host.docker.internal` for XQuartz `DISPLAY`.
**Unknown:** the complete list of the "other domains", IPv6 answers for these names,
and behavior while the host is on a VPN.

**Reported regressions relevant to all routing flows:** GH-1984 says the names stopped
resolving after a macOS 26 update even though the user allowed Local Network access.
Commenters reported the same symptom, and one reported a missing root CA on a clean
install. The maintainer closed it as fixed in v2.0. RN v2.0 says "Fixed `orb.local`
domains on macOS 26". RN v2.2.4 says "Fixed DNS failures caused by Local Network
privacy". The lesson for OrbWin is an OS-update and privacy-gate regression class:
name failure must be diagnosable separately from engine health.

### OrbWin Windows adaptation (OSX-NET-001..005)

**Exists today (OrbWin observed):**
- The `localnet` route registry has up to 256 canonical names. Each name maps to one
  reviewed TCP port and pins the target full ID and StartedAt.
- Duplicate names refuse the whole snapshot. Removing or changing a route cancels
  its in-flight connections, including upgrades.
- The proxy requires SNI to match `Host`, redirects HTTP to HTTPS, and **replaces
  client-supplied forwarding headers**.
- The constrained intermediate issues only `.orbwin.localhost`/`.orbwin.test` leaves
  and refuses wildcards, IPs and arbitrary domains (`architecture/local-services.md`).
- A container reached a Windows loopback listener through `host.docker.internal`.
  A live proof also chains relay to `host.docker.internal` to the Windows TLS gateway.
- Not wired: the permanent controller, setup UI, network aliases, OS trust, and the
  integration port pair. Qualified Windows/WSL ingress on 80/443 under three engines
  is unsolved: Traefik and Rancher own 80/443/IPv6, and `.localhost` resolves to
  `::1` first (BL-018).

| Flow | Remaining gap | Disposition (proposal) |
| --- | --- | --- |
| 001 port labels | Route selection exists in the library. No label contract or UI override. | **Core**: P2-NAM already requires "explicit override". Define `dev.orbwin.http-port`/`https-port`. Decide (P2-D08) whether to also read `dev.orbstack.*` read-only for compatibility. The label prefix must never mean OrbStack parity. |
| 002 multiple names / per-name ports | One name maps to one port, but no user-declared extra names. Wildcards are refused by the PKI. | **Core**: several exact sub-names per container, each with its own port, under the reserved suffixes. **Exclusion candidate**: arbitrary `.local`/custom TLDs. They need a resolver takeover or an unconstrained CA, which violates the constrained-intermediate boundary. **Decide**: wildcards via exact on-demand leaves for observed SNI under one route versus a wildcard leaf. Multi-level wildcards follow the same decision. |
| 003 forwarded headers | The library replaces headers. The emitted set is not specified as a user contract. | **Core, small**: document and test `X-Forwarded-For/Proto/Host` emission. Never trust inbound values. |
| 004 container-side names | `.orbwin.test` aliases are designed for selected networks only. There is no cross-project UX and no container trust. | **Core** for selected user-defined networks, including several Compose projects. **Deliberate difference**: no default CA injection into every container. P2-TLS forbids blanket image mutation, so trust is opt-in per project. Compose `dns:` by name (GH-2645) is an **exclusion candidate**: it is a Docker semantics change. |
| 005 host/engine names | `host.docker.internal` works through the upstream host gateway. It is not part of a qualified matrix, and IPv6 behavior is not recorded. | **Core (qualification only)**: add it to the P2-D01 client matrix. No new name is invented. A `docker.internal`-style engine name has no Windows need, because WSL clients use the shared socket. **Exclusion candidate**. |

**Acceptance scenarios (proposal):**
- **Three engines:** start OrbWin, Rancher and Docker Desktop in both start orders.
  Every OrbWin name reaches only OrbWin targets. The other engines' 80/443, IPv6 and
  `host.docker.internal` behavior is unchanged.
- **No default stealing:** hosts file, NRPT, DNS client, proxy, Docker context,
  `DOCKER_HOST` and kubeconfig are byte-identical before and after setup and removal.
- **Collisions:** two containers declare one name. Result: deterministic refusal shown
  on both resources, with no last-writer-wins. Another engine owns 443: the alternate
  integration pair or refusal applies, never eviction.
- **Stale routes:** recreate, rename, `compose run` and a port reused by an unrelated
  process. No request reaches the old incarnation (RN v2.2.2 and v2.2.4 show this
  class).
- **Restart:** engine restart, app restart with the UI closed, Windows reboot and
  sleep. Routes reconcile without manual repair, and in-flight streams close visibly.
- **Diagnostics:** name resolution, listener, route and target health are reported
  separately. This covers the GH-1984 class after a Windows or WSL update.
- **UI:** route list loading, empty ("no web listener detected"), error and
  conflict states. Overrides are editable and cancelable by keyboard. Link
  status is announced to screen readers, and unverified links are never shown as
  connected (BL-009).

## Dev Containers and VS Code

### OSX-NET-006 — Open a folder in a dev container

**Actor/intent:** a developer opens a repository with `devcontainer.json` in VS Code
and works inside the container. **Documented sequence:** OrbStack says it "should be
compatible with all third-party tools, such as Dev Containers" (DKR). The FAQ lists
VS Code Dev Containers as working out of the box. It creates and auto-uses an
`orbstack` Docker context and can symlink `/var/run/docker.sock` with admin approval
(DKR). Switching back is `docker context use desktop-linux` (FAQ). VS Code uses the
current context, or `DOCKER_HOST`/`DOCKER_CONTEXT` set in `containers.environment`
(VSC-H). **Reported:** GH-2747 says Compose-based dev container builds broke after the
v2.2.4 bundled Buildx update. It links an upstream VS Code issue. **Unknown:**
OrbStack-specific Dev Containers behavior, bind-mount performance claims, and port
forwarding interaction with domains. No OrbStack Dev Containers walkthrough exists.

### OSX-NET-007 — SSH agent and Git credentials inside containers

**Documented:** mount `/run/host-services/ssh-auth.sock` and set `SSH_AUTH_SOCK` to
that path. This "will not work if you're using 1Password's agent", and mounting
`$SSH_AUTH_SOCK` directly works in most other cases (DKR). RN v0.1.9 adds Docker
Desktop agent-path compatibility, and RN v1.4.2 fixes mounting `SSH_AUTH_SOCK`.
Separately, VS Code Dev Containers forwards a running local agent and reuses local Git
credential helpers. On Windows this needs the OpenSSH Authentication Agent service
(VSC-G). **Unknown:** how OrbStack's path interacts with VS Code's own forwarding,
and agent loss or rotation while a container runs.

### OrbWin Windows adaptation (OSX-NET-006..007)

**Exists today (OrbWin observed):**
- A dedicated pipe `npipe:////./pipe/docker-rancher-desktop-orbwin`. Settings states
  that no Docker context is created or switched and shows per-command `--host` and a
  session-scoped `DOCKER_HOST`.
- The WSL socket `/mnt/wsl/orbwin/docker.sock` goes through the Windows publication
  adapter. Registry credentials live in a **private** Docker config and the
  `OrbWin/registry/v1/` Credential Manager prefix.
- BL-005: a Windows-path `--mount src=F:\...` from the ordinary Docker CLI is still
  rejected. Only typed Run requests translate paths.
- No SSH-agent feature. `local-services.md` forbids copying agents or keys into guests.

**Gaps that break the journey:**
1. VS Code launched from Windows sends Windows bind sources, which hits BL-005.
2. The user's global `~/.docker/config.json` does not see OrbWin's private credential
   routes, so private-image dev containers fail to pull.
3. There is no discoverable, non-default way to select OrbWin from VS Code.
4. A WSL-hosted folder needs P2-WSL path mapping.

| Item | Disposition (proposal) |
| --- | --- |
| Engine targeting | **Core.** Offer an explicit, opt-in named context (`orbwin`) that is created but **never `use`d**, plus copyable `containers.environment` and `DOCKER_CONTEXT` snippets. Creating a context changes user Docker state, so it needs consent, a removal path and a byte-compare test for the current context. Decide under P2-D03/D04. |
| Windows-folder binds | **Core.** Closing BL-005 for ordinary CLI/Compose is a prerequisite. Dev containers are its main consumer. |
| WSL-folder dev containers | **Core within P2-WSL.** The WSL socket plus native-path mapping. Qualify the Dev Containers "Reopen in Container" path from a WSL window. |
| Private registry pulls | **Core decision (new P2-D09):** expose OrbWin credential routing to user-invoked clients, either by documented `DOCKER_CONFIG` targeting or by an opt-in per-registry helper entry in the user config. Never copy secrets or change `credsStore` silently. |
| SSH agent | **Adequately supplied by VS Code** on Windows (OpenSSH agent service) and in WSL. **Exclusion candidate** for an OrbWin agent socket. Record a 1Password-on-Windows scenario as unknown and qualify it, without asserting it works. |
| Buildx/Compose drift (GH-2747 class) | **Core regression gate:** pin-update qualification runs a Compose-based dev container build before release. |

**Acceptance scenarios:**
- A Windows folder and a WSL folder each open in a container (image and Compose
  variants), with Docker Desktop and Rancher running and their contexts unchanged.
- Rebuild, close VS Code, and reopen. Engine restart while attached produces a clear
  disconnect, and reopening works.
- A private-registry base image pulls. A missing or denied credential gives a clear
  error with no fallback to another engine's store.
- Ports forwarded by VS Code do not collide silently with OrbWin's integration pair.
- OrbWin's "connect an editor" guidance has keyboard and screen-reader access,
  copy confirmation, and an honest error when the engine is stopped.

## Networking conveniences

| ID | OrbStack flow (evidence) | Unknown |
| --- | --- | --- |
| OSX-NET-008 host networking | `--net host`: container servers appear on the Mac's `localhost`, and Mac servers are reachable from the container on `localhost` (HOST, documented). RN v0.7.0 lists 2-way localhost integration. RN v2.2.3 fixes inbound UDP. | Port collisions with Mac listeners, IPv6, privileged ports |
| OSX-NET-009 LAN exposure | Port forwards are LAN-reachable by default. Turn off "Expose ports to LAN" (`docker.expose_ports_to_lan`) to restrict them. IP-specific binds such as `192.168.1.50:80:80` are supported (NET; RN v2.1.0, v2.2.0). The engine TCP socket is "extremely dangerous" (DKR). | Firewall prompts, behavior when the setting changes while ports are live |
| OSX-NET-010 IPv6 | Disabled by default. "Enable IPv6" setting or `"ipv6": true` in engine config (NET). RN v0.8.1 disabled it by default for compatibility. | Address pools, host reachability over v6 |
| OSX-NET-011 IP ranges | The default is `192.168.x.x`. Change `bip` and `default-address-pools` on conflict (NET). RN v2.0 adds a customizable range for machines and domains. | Migration of existing networks after a change |
| OSX-NET-012 direct container IP | Reach a container by IP from the Mac (NET). Domains depend on "Allow access to container domains & IPs" (DOM). GH-33 (39 thumbs-up) requested it, and RN v0.12.0 shipped it. GH-2142 reports host network interruption when it is enabled (open). | Routing with VPNs that claim the same ranges |
| OSX-NET-013 proxy/VPN following | Containers follow macOS HTTP/HTTPS/SOCKS proxy and exclusions. `network_proxy` takes `auto`/`none`/URL, and `network.proxy.exclude` adds exclusions. The stack "follows your VPN and DNS settings" (NET, DKR). Many RN proxy fixes (v2.1.3, v2.2.0) exist. GH-2334 reports VPN DNS names not resolving (open). | PAC files, authenticated proxies, split-DNS precedence |
| OSX-NET-014 credential store switch | OrbStack uses `osxkeychain`, Docker Desktop uses `desktop`. Users may need to log in again, or fix the store in `~/.docker/config.json` when running both (DKR). RN v1.1.0 adds coexistence, and RN v2.2.0 and v2.2.2 fix migration and keychain errors. | Exact migration rewrite, rollback |

### OrbWin Windows adaptation (OSX-NET-008..014)

| ID | Exists today (OrbWin observed) | Gap and disposition (proposal) |
| --- | --- | --- |
| 008 | An owned host-network container was **unreachable** from Windows and Ubuntu loopback (`p2-host-network-loopback.json`). The direction from a container to the Windows host is not qualified. | **Decide.** Host-to-container needs a new forwarder over a shared WSL/rdd namespace. That conflicts with "no new network stack". Proposed: **exclusion candidate** for host-to-container, with an honest UI note and guidance toward names or publishing. Container-to-host `localhost` is also an **exclusion candidate**, with `host.docker.internal` as the supported path. The engine must still run `--net host` workloads with guest-local semantics, and BL docs must say so. |
| 009 | The publish parser accepts `127.0.0.1` or `0.0.0.0` but documents host access as localhost-only. Phase 1 lists LAN as unsupported. | **Likely core (opt-in only)** for CI/service testing from another device. The default stays localhost, which deliberately differs from OrbStack's LAN-by-default. Enabling it needs explicit consent. Windows Defender Firewall rules are scoped to the OrbWin listener and profile (Private only by default) and removed on disable or uninstall. Never expose the Docker API. Test while Docker Desktop holds the same port. |
| 010 | Unsupported. The Windows `.localhost` name resolves to `::1` first, and Rancher has IPv6 listeners. | **Split.** Host-side `::1` handling for names is **core**, because it is already blocking ordinary browsers. Container IPv6 networking is a **deferral candidate** (Phase 4), with the decision recorded. |
| 011 | Patch 0002 separates the WSL bridge subnet. Daemon `bip`/pools configuration through rdd is **unknown** in our audit. | **Likely core (minimal):** detect conflicts with VPN or corporate routes and show them. Allow a reviewed `default-address-pools` change only if rdd exposes a supported daemon-config path. Re-audit under BL-016. Changes never rewrite existing networks. |
| 012 | Rejected for routing: a mutable IP has a reuse race (`local-services.md`). P2-D07 lists direct-IP/L2 as not implied. | **Deliberate exclusion candidate.** Windows routes into the WSL utility VM's private gvisor network would be global and route-table invasive, and could collide with other engines. Names and publishing cover the journey. Copying an IP must state that it is guest-scoped. |
| 013 | BL-016: no proxy setting. Direct network only. Global proxy, VPN and WSL config are never altered. | **Core for P2-D01.** Read Windows/WinHTTP proxy and exclusions as **inputs** and offer an explicit engine/container proxy setting with an `auto`/`none`/URL model. Never edit `.wslconfig` (`autoProxy`/`dnsTunneling` are user-global). Qualify VPN connect/disconnect, split DNS and offline. Keep BL-017 tap loss separate. |
| 014 | Private config with an exact-prefix Credential Manager helper. The global config, `credsStore`, and other engines' entries stay untouched. | **Core (documentation/UX):** explain why `docker login` in a user's shell does not populate OrbWin and the reverse. Pair this with P2-D09 and P2-MIG; credentials are never migrated implicitly. |

**Cross-cutting acceptance scenarios:**
- All three engines run in both start orders. LAN toggle on and off. A firewall
  consent prompt is shown and denied, which leaves a clear "localhost only" state
  with no partial rule. The rule is removed on uninstall.
- Proxy `auto`/`none`/URL, and a VPN connect while containers pull and serve. The
  outcome is a clear failure or success per client class. Global proxy is never
  changed, and another engine's proxy is never changed.
- Subnet collision with a VPN route is detected before network creation, and
  existing networks are preserved.
- Every new setting has loading, saved, pending-restart, error and cancel states,
  keyboard operation, labelled controls and a reversible default.

## Proposed Phase 2 scope rows and decisions

These are proposals for the scope owner. They do not change `phase-2-scope.md`.

| ID | Proposal | Core / exclusion reasoning |
| --- | --- | --- |
| P2-NAM addendum | Explicit HTTP/HTTPS port overrides by label and UI, multiple exact sub-names per container with per-name ports, documented forwarded headers, and cross-project container access on selected networks. | Core. These are the normal ways the naming journey works for multi-port and multi-host apps. |
| P2-D08 (new) | Label namespace: own `dev.orbwin.*` only, or also read `dev.orbstack.*`. Decide on wildcards via on-demand exact leaves. | Decision. Compatibility helps users of shared Compose files. Wildcard certificates weaken the constrained-signer boundary. |
| P2-DEV (new row) | Dev Containers from Windows and WSL folders: opt-in non-default context or documented targeting, BL-005 closure, private pulls, Compose-based build gate. | Core. A priority developer journey, and the only place we currently mention it is a failure seed (R-002). |
| P2-D09 (new) | Credential visibility to user-invoked clients (`DOCKER_CONFIG` versus opt-in helper entry). | Decision. Required by P2-DEV and OSX-NET-014. No silent `credsStore` change. |
| P2-LAN (new row) | Opt-in LAN exposure with scoped firewall consent and removal. | Likely core for CI/devices. Localhost stays the default. |
| P2-D01 addendum | `host.docker.internal`, `::1`-first name resolution, proxy auto/none/URL, VPN/split-DNS, subnet-conflict detection. | Core for the supported network envelope. Container IPv6 is deferred with a record. |
| P2-D07 addendum | Record exclusions: direct container IP and L2 routes, host-to-container `--net host` bridging, arbitrary `.local`/custom TLD names, default CA injection into all containers, an OrbWin SSH-agent socket, and Compose `dns:` by name. | Each needs global routing, resolver or trust changes, a new network stack, or Docker semantics changes. VS Code, names and publishing cover the user outcome. Revisit only with a concrete need. |

Residual unknowns: any OrbStack behavior marked unknown above. Real Windows results
for every acceptance scenario are pending. None of these proposals is implemented.
