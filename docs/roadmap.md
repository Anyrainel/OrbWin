# OrbWin four-phase roadmap

Recorded: 2026-10-21. These phases define incremental product outcomes, not a
waterfall that postpones data safety, reliability or compatibility.

Updated: 2026-10-22 with phase 1 milestones and required simultaneous coexistence.

Each phase ends with a reproducibly built, usable product inside a stated support
matrix, documented limitations, and evidence that the phase's acceptance criteria
pass. Do not call a mock UI or an unqualified alpha integration a completed phase.
Inherited upstream failures remain relevant to release quality even when our own
code is correct.

## Phase 1 - OrbStack-inspired UI and flows over Rancher 2.0

### Outcome

A working Windows application with a Tauri frontend, backed by a pinned
Rancher-derived `rdd`, presenting coherent OrbStack-inspired development flows.

Phase 1 covers the full relevant Rancher-backed developer experience, not only a
minimum core journey. Implementing core journeys first is sequencing. Backend
limitations get useful UI adaptations and tracked later work; niche OrbStack
features irrelevant to server/service development and local Kubernetes may be
excluded with a documented rationale. Do not use an early milestone as the phase
completion claim.

### Milestones

1. **Backend and coexistence proofs.** Select and reproduce an upstream baseline;
   validate Windows lifecycle, separate owned identity/data/endpoints, simultaneous
   operation with existing engines, Docker-dependent tests, Compose, streaming and
   interactive access. Identify narrow host-integration patches. Obtain baseline
   measurements before changes when useful. Proofs may precede UI construction.
2. **Design system and first complete journeys.** Establish navigation, component
   contracts and all interaction states; deliver real end-to-end development/test
   journeys with lifecycle and failure-path checks. This is a working milestone,
   not the whole phase.
3. **Complete relevant phase 1 coverage.** Maintain a feature/flow matrix combining
   OrbStack evidence, Rancher capabilities and Windows developer needs. Complete
   container/project, image/build, volume/storage, logs/shell/stats, networking,
   settings, WSL integration, Kubernetes and diagnostic flows supported by the
   chosen backend; adapt and record blocks explicitly. Reuse upstream components
   where appropriate rather than rebuilding broad management functionality.
4. **Qualify and package.** Run the supported matrix, repeated integration-test
   workloads, coexistence order/conflict cases, interruption/data-preservation
   checks and visual/accessibility review. Produce a runnable reproducible product
   with full phase-scope accounting and measured budgets.

Milestones may be split or reordered using evidence. See
[development brief](development-brief.md) for the authorized implementation handoff.

### Work

- Run a reproducible backend prototype; record exact versions and contracts.
- Establish real Docker CLI, Compose, Buildx and IDE/API-client smoke workloads.
- Prioritize CI-style/integration test runs that launch Linux service components,
  including repeated/concurrent setup, readiness, test execution and cleanup.
- Establish simultaneous coexistence with Docker Desktop and Rancher, including
  explicit client targeting and test frameworks that discover Docker endpoints.
- Establish Windows and WSL project/mount behavior, including a non-system drive
  and integration with representative user distributions.
- Define the design system, navigation, component contracts and interaction states.
- Implement onboarding, lifecycle, inventory, project/container operations and
  other supported research-prioritized flows against the actual backend.
- Preserve Kubernetes compatibility and expose qualified upstream operations.
- Keep the upstream boundary small and updateable; test backend upgrades and API
  adaptation without mixing version-specific details throughout the frontend.
- Inventory migration operations inherited from the actual upstream version;
  record missing migration conveniences for phase 2 instead of assuming them.
- Classify unsupported flows: implement a useful Windows/backend adaptation,
  document the limitation, and retain the desired later behavior.
- Collect startup, idle resource, build and filesystem baselines; test lifecycle
  interruption, reconnection, sleep/resume and representative network changes.
- Establish local packaging and repeatable verification. Public signing and
  release distribution are explicit work, not assumed automatic benefits of reuse.

### Backend limitation ledger

Create `docs/backend-limitations.md` when implementation exposes concrete gaps.
Each entry needs: stable ID, affected flow, backend version, evidence/reproducer,
user impact, upstream issue if available, phase 1 UI adaptation, owner, target
phase, acceptance criterion and a regression test where feasible. Distinguish a
missing product feature from a backend defect, unverified capability or WSL limit.

### Exit criteria

- Install/start/use/restart the product through documented supported flows.
- Required Docker clients and supported mount/network workflows pass.
- The full relevant phase 1 feature/flow matrix is accounted for; core-journey
  completion alone does not meet this gate.
- Required simultaneous operation passes with isolated owned state/endpoints;
  ordinary host-port conflicts are clear and do not silently redirect traffic.
- No simulated success or unexplained dead-end UI for blocked operations.
- UI failure/closure does not intentionally terminate healthy backend workloads.
- Defined lifecycle and data-preservation tests pass; no unsafe global WSL reset
  is used as routine recovery.
- Baseline measurements and supported-version matrix are recorded.
- Quality-standard gates pass; remaining external limits are explicit and do not
  invalidate supported scope.

## Phase 2 - Bring missing OrbStack conveniences to OrbWin

### Outcome

A working product whose routine development experience includes the researched
conveniences absent from the selected Rancher baseline.

### Work

- Prioritize evidence-backed gaps, including automatic local names/HTTPS, file
  browsing, richer Compose flows, storage convenience and WSL integration as needed.
- Reuse OS/WSL settings and permissively licensed implementations before building
  new infrastructure. Avoid repeated manual configuration inside containers.
- Add only necessary backend extensions; retain upstream-compatible boundaries.
- Reuse verified upstream migration paths and complete missing migration/transition
  conveniences, especially from Rancher, with source-version-specific data tests.
- Resolve or explicitly retain phase 1 flow adaptations and limitation records.

### Exit criteria

- Each included convenience works end to end, including setup, conflicts, errors,
  restart, removal and recovery.
- DNS/trust changes are scoped and reversible, with qualified VPN/proxy behavior.
- No regression in phase 1 compatibility, data safety or measured resource budgets.
- The product still meets the quality standard and has a clear current scope.

## Phase 3 - Reliability and performance improvements

### Outcome

A working product with demonstrated improvements where investigations justify
them; not an assumption that an OrbStack-inspired UI changes kernel behavior.

### Work

- Diagnose failures against pinned Rancher, WSL and Docker Desktop baselines.
- Prioritize recovery correctness, state reconciliation and persistent-data safety.
- Profile startup, idle CPU/memory, transport, filesystem and representative builds.
- Evaluate promising Microsoft/upstream mechanisms such as VirtioFS where useful.
- Adapt or replace a component only with a concrete benefit and maintenance case.
- Contribute or retain narrowly scoped upstream fixes with regression coverage.

### Exit criteria

- Reliability results identify workload, environment, fault, recovery and data impact.
- Performance claims include reproducible workload, versions, sample distribution,
  host conditions and raw results; startup and cache conditions are distinguished.
- Adopted improvements pass correctness, compatibility and regression gates.
- No blanket claims of outperforming Docker Desktop or OrbStack exceed the evidence.

## Phase 4 - Ensure and expand modern-user compatibility

### Outcome

A working product with a broader validated support matrix and a credible release,
upgrade and maintenance story for its intended users.

### Work

- Extend OS/WSL/backend/client and Windows edition coverage.
- Expand development workflows, project/storage configurations, registries,
  managed-device environments, VPNs/proxies, Kubernetes and WSL distro coverage.
- Evaluate ARM64, Windows 10/Server or specialized editions only where justified.
- Evaluate larger additions such as GPU/device integration, native Windows
  containers or pre-login service modes separately; inherited support is not proof.
- Complete release signing, updates, migration, backup/restore, coexistence,
  uninstall behavior, support bundles and dependency maintenance as applicable.

### Exit criteria

- Compatibility claims map to actual qualification evidence.
- Installation, upgrade, migration and recovery preserve data under defined tests.
- Product-owned behavior satisfies the same core and frontend quality standards.
- Unsupported environments and capabilities are stated precisely.

## Work that runs throughout all phases

- Security, data preservation, compatibility smoke tests and failure-path testing.
- Source/version provenance and upstream dependency/license inventory.
- API compatibility and unknown-field preservation when translating Docker traffic.
- Design-system consistency, keyboard/accessibility review and visual QA.
- Backend-limitation tracking and regression prevention.
- OrbStack-informed recovery: smooth and clean when feasible, transparent and fast
  failure when reliable automatic recovery is infeasible or disproportionately
  complex. Preserve the distinction between observed behavior and our adaptation.
- Reliability coverage seeded by public upstream reports and expanded by our tests;
  the user need not supply exhaustive scenarios before development can proceed.
- Tests of installer/update behavior appropriate to the current release scope.

OrbStack product research and project-decision recording are parallel activities.
Research can produce qualified partial specifications and a Mac-observation plan
without waiting for the backend implementation or these planning records.
