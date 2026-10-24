# OrbWin

OrbWin is a Windows container-development product in early implementation. It
targets OrbStack's interaction quality and convenience using Rancher Desktop
2.0's backend and Microsoft WSL 2. Reliability relative to Docker Desktop and
any performance improvements must be demonstrated, not assumed.

The native Tauri/React development application and a patched, guest-capable
Rancher `rdd` build run locally. Management startup, an owned WSL guest, Linux
container execution and a Redis/PostgreSQL/Compose/Buildx baseline have passed.
Typed resource operations, Buildx/Compose, interactive exec, Kubernetes workloads
and Windows Node Testcontainers have also passed scoped runtime proofs.
**Phase 1 is accepted within its stated local qualification scope.** Installed first-run, native developer
flows, simultaneous-engine operation, selected Ubuntu integration, packaging,
fault recovery and measured budgets have recorded evidence. The manually
confirmed sleep/wake check preserved Docker/Kubernetes data and all recorded
endpoints; exact sleep depth and wake latency were not instrumented. See the
[final assessment](docs/verification/phase-1-final-assessment.md) for evidence
and remaining external limits.
The [2026-10-24 readiness reassessment](docs/verification/phase-1-readiness-2026-10-24.md)
records fresh passing workload checks, a fixed Run-form mount defect, and the
failed/open phase gates at that checkpoint. Implementation continued afterward.

Implementation has continued beyond that assessment: see the
[active Phase 1 completion record](docs/verification/phase-1-completion.md).
The unsigned per-user NSIS installer includes the pinned backend, guest,
Windows/Linux clients, helpers and third-party notices. Actual install,
replacement, uninstall and reinstall retained the separate engine data.
See [packaging](docs/architecture/packaging.md) and
[current scope](docs/phase-1-coverage.md) for boundaries and evidence.

Build the local installer after the backend/helper/tool steps below:

```powershell
npm run package:prepare
npm run package:build
```

The output is `src-tauri/target/release/bundle/nsis/OrbWin_0.1.0_x64-setup.exe`.
Before replacing an installed build, use Settings → Docker engine → Prepare for
application update, then close OrbWin. This stops owned workloads and retains
their data in `%LOCALAPPDATA%/OrbWinData`; application files are separate.

## Local development (Windows x64)

Node 24/npm, Git, the MSVC/Windows SDK build tools and WebView2 are required.
`rust-toolchain.toml` pins Rust 1.90.0; Rustup installs that toolchain without
changing the user's default. Go and MinGW compiler packages are project-local.

```powershell
npm ci
npm run tools:bootstrap
npm run upstream:prepare
npm test
npm run upstream:test
npm run session:build
npm run credentials:build
npm run tools:stage-runtime
npm run build
cargo test --manifest-path src-tauri/Cargo.toml
npm run tauri -- build --debug --no-bundle
```

Run `src-tauri/target/debug/orbwin.exe` for the native UI or `npm run dev` for a
browser preview. Browser preview cannot call native backend commands. The native
development adapter targets only a previously verified, product-owned engine.
Reads never start a backend or fall back to Docker Desktop; explicit Start/Stop
controls forward lifecycle requests to the owned rdd instance.

`npm run backend:build` builds the native management binary and all guest
components. `npm run backend:bundle` also stages checksum-verified upstream
guest dependencies and overlays the WSL rootfs. Bundling needs XZ Utils (tested
with 5.8.1 at `C:/msys64/usr/bin/xz.exe`; set `ORBWIN_XZ` for another installation).
The build writes source/patch/binary provenance to `artifacts/backend/build-manifest.json`.

`npm run backend:proof` runs the management-only proof in a fresh task-owned home.
`npm run backend:guest-proof` refuses any existing `lima-orbwin` registration,
creates an isolated guest, runs a Linux smoke container, stops its owned service
and retains the guest and evidence. It must not be used to replace an existing
registration. Recorded proof homes can be resumed explicitly; see
[development progress](docs/development-progress.md) for this machine's current
owned instance, results, outstanding gates and safe continuation instructions.

The full qualification harness is opt-in and refuses an absent/stale runtime
identity or an engine without coexistence qualification. No global prune,
context switching, WSL shutdown or existing-environment migration is used.

## Project record

- [Decisions and pending choices](docs/project-decisions.md)
- [Four-phase delivery roadmap](docs/roadmap.md)
- [Core and frontend quality standard](docs/quality-standard.md)
- [Development brief and starting priorities](docs/development-brief.md)
- [Development progress and verification](docs/development-progress.md)
- [Phase 1 coverage](docs/phase-1-coverage.md)
- [Backend limitations](docs/backend-limitations.md)

OrbStack product and user-flow research is proceeding independently in a separate
OrbWin chat. Its artifacts belong in `docs/research/orbstack/`; these planning
documents are not prerequisites for that research.

The frontend approach is **design-system-led implementation**: interaction
specifications, reusable components, visual rules, accessibility, and visual
regression review. The core follows **kernel-grade engineering discipline** as
defined in the quality standard.

Priority workloads include Docker-dependent integration/CI-style tests using Linux
service components and local Kubernetes development. Phase 1 delivers the full
relevant Rancher-backed UI through milestones, with required simultaneous
coexistence alongside existing engines. Missing migration conveniences follow in
phase 2; upstream migration support must be verified rather than assumed.
