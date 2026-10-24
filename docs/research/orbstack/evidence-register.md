# Evidence register

All sources accessed 2026-10-21 Pacific unless stated. OrbStack docs are rolling pages without a verified app-version binding; their copyright footer is not a version. Public screenshots establish only their visible state. No timing, shortcut, tooltip, confirmation default, or hidden behavior is inferred from them.

## OrbStack public sources

| ID | Source | Finding and classification | Confidence / limit |
|---|---|---|---|
| O01 | [Quick start](https://docs.orbstack.dev/quick-start) | Documented: Docker workflow, bundled Compose; machine creation through app/CLI. | High for documentation; exact onboarding screens unknown. |
| O02 | [Docker](https://docs.orbstack.dev/docker/) | Documented: GUI resource management, `orbstack` Docker context, optional socket link, engine configuration/logs and credential-helper difference from Docker Desktop. | High; macOS behavior, not Windows parity. |
| O03 | [Domains](https://docs.orbstack.dev/docker/domains) | Documented: names follow container or service/project identity; custom/wildcard `.local` domains, HTTP port detection and override labels; index of running services. | High; collision/failure UI unknown. |
| O04 | [HTTPS](https://docs.orbstack.dev/features/https) | Documented: local CA/proxy, first-use certificate installation prompt, CA distribution to containers with opt-out, macOS keychain protection. | High; exact trust-denial and recovery behavior unknown. |
| O05 | [Native files](https://docs.orbstack.dev/features/native-files) | Documented: folder action and native filesystem access; containers/volumes writable, images read-only. | High; update consistency and permission UI unknown. |
| O06 | [Debug Shell](https://docs.orbstack.dev/features/debug) | Documented: right-click entry and `orb debug`, distroless/read-only support, shared tooling without changing container packages, remote-context option. | High; cannot equate ordinary exec with this feature. |
| O07 | [Settings](https://docs.orbstack.dev/settings) | Documented: app/CLI settings, resource controls, proxy/network settings, Docker-only versus whole-product reset scope. | High; exact apply/restart interactions unknown. |
| O08 | [Headless use](https://docs.orbstack.dev/headless) | Documented: lifecycle/config commands, `orb top`, headless operation; GUI-free auto-update unsupported. | High; no local execution. |
| O09 | [Kubernetes](https://docs.orbstack.dev/kubernetes/) | Documented: single-node cluster, local images, service access, lifecycle CLI, context switching, localhost-default exposure. | High for documented Mac behavior; not reproduced. |
| O10 | [Machines](https://docs.orbstack.dev/machines/) | Documented: Linux machine management, distinct from Docker containers. | High at capability level; detailed machine interactions need Mac observation. |
| O11 | [Menu bar](https://docs.orbstack.dev/menu-bar) | Documented: compact app entry point. | High at capability level; menu inventory not reproduced. |
| O12 | [Official product demo page](https://orbstack.dev/) | Documented: public demo/feature presentation; performance figures belong to stated Mac workloads. | Vendor presentation, not independent measurement. |
| O13 | [Terms §2](https://docs.orbstack.dev/legal/terms) | Documented: explicit restriction on reverse engineering/decompilation/disassembly. | High; not legal advice or a clean-room conclusion. |
| O14 | [Official Kubernetes screenshot](https://docs.orbstack.dev/screenshots/k8s-hero-dark.png) | **Observed screenshot**: sidebar groups, selected Services destination, namespace-grouped middle list, right detail fields and Ports; per-row link/delete icons. | High for pixels only; app version/date and interactions unknown. Viewed locally as `support/k8s-reference.png`; no redistribution license established. |

The screenshot is a research reference; its redistribution license has not been established.

## REA evidence

| ID | Source / artifact | Finding |
|---|---|---|
| A01 | [Official repository](https://github.com/morluto/rea), [site](https://rea.tools), [npm metadata](https://registry.npmjs.org/rea-agents/latest) | Observed registry version 6.7.0, MIT. Node requirement checked against local v24.18.0. |
| A02 | [Workflow skill](https://github.com/morluto/rea/blob/dd3497e1338cda86c9ef792754d8a369bd009942/.agents/skills/reverse-engineer-anything/SKILL.md), [installation](https://github.com/morluto/rea/blob/dd3497e1338cda86c9ef792754d8a369bd009942/docs/installation.md) | Read workflow version 44 at source commit dd3497e1338cda86c9ef792754d8a369bd009942 (2026-10-21 Pacific). CLI supported without global registration; setup configuration writes need a reviewed plan. |
| A03 | [Windows Ghidra P0](https://github.com/morluto/rea/blob/dd3497e1338cda86c9ef792754d8a369bd009942/docs/windows-ghidra-p0.md) | Documented: experimental Windows x64 native PE/raw NMOS 6502 boundary; no Windows Ghidra Mach-O claim. |
| A04 | [Provider output](support/rea-providers.json), [doctor output](support/rea-doctor-ghidra.json) | Observed CLI diagnostics: Ghidra not configured; macOS native utilities unavailable on Windows; some static artifact operations available. No target analysis. |


A04 records the initial unconfigured invocation. Follow-up configured diagnostics passed with the user-supplied Ghidra installation: [configured doctor](support/rea-doctor-ghidra-configured.json). See [REA assessment](rea-assessment.md).
