# OrbWin engineering instructions

## Read the relevant project record

- `docs/project-decisions.md` records accepted decisions, superseded proposals,
  and unresolved implementation choices.
- `docs/roadmap.md` defines deliverables and exit criteria for each phase.
- `docs/quality-standard.md` defines required engineering and design discipline.
- Load only the research and module documentation relevant to the current task.

These instructions supplement the user's instructions. Current repository state
and verified upstream behavior take precedence over historical research claims.

## Architecture and scope

- Start with Rancher Desktop 2.0's standalone `rdd` backend. Grow with upstream;
  do not replace working backend components without evidence of a concrete need.
- Reuse Microsoft WSL 2 and the selected upstream's Windows integration. Do not
  build a WSL replacement, custom hypervisor, kernel, or chipset drivers.
- Kubernetes compatibility is a positive capability. Preserve it; distinguish
  the backend's management API from an optional workload cluster.
- Use Tauri and React/TypeScript for the frontend. Keep Rust desktop integration
  thin and reuse Go backend code. Do not rewrite existing code solely to enforce
  a language preference.
- The engine guest distribution and virtual-disk arrangement are implementation
  details. Preserve upstream choices initially. User development distributions
  must remain independent.
- Keep backend lifecycle independent of the UI. Do not add a second competing
  source of truth or orchestrator around `rdd`.
- Phase 1 is the full relevant Rancher-backed, OrbStack-inspired developer UI.
  Proofs and core journeys are milestones, not permission to cut the phase scope.
- Docker-dependent integration/CI-style tests using Linux server/service
  components are priority workloads; local Kubernetes remains a positive feature.
- Simultaneous coexistence with Docker Desktop and Rancher is required. Isolate
  owned state/endpoints and target clients explicitly; never silently steal global
  defaults, stop another engine, or share writable engine disks. Validate early.
- Reuse verified upstream migration capabilities; custom migration gaps belong in
  phase 2. Do not infer automatic data interchange from shared Rancher ancestry.
- Keep version-specific Rancher adaptation narrow, document patches and qualify
  frequent upstream updates. Breaking APIs are expected maintenance work.

## Quality and evidence

- Every phase must produce a working product within an explicit supported scope.
  Reliability, compatibility, security, and data-safety testing begin in phase 1.
- Important core code requires explicit invariants, ownership, concurrency,
  cancellation, failure semantics, and recovery reasoning. Read the quality
  standard before changing lifecycle, storage, transport, security, or networking.
- Preserve upstream tests and provenance when adapting code. Add meaningful
  tests for changed behavior and failure modes, not implementation-mirroring tests.
- Frontend work must follow the design system and interaction specifications;
  include loading, empty, error, cancellation, keyboard, and accessibility states.
- Record backend-blocked flows in `docs/backend-limitations.md` when implementing
  phase 1. Provide an honest, useful UI adaptation and a reproducible dependency
  record; do not create a fake successful flow or silently hide limitations.
- Do not claim reliability, performance, clean-room status, feature parity, or
  absence of defects without appropriate evidence. Clearly distinguish goals,
  documented behavior, observed behavior, proposals, and unknowns.
- A phase cannot ship with known defects that invalidate its acceptance criteria.
  Record residual risks and external limitations explicitly.
- Start recovery from OrbStack evidence and product principles: smooth and clean
  where feasible, transparent and fast failure otherwise. Never invent observed
  OrbStack behavior to fill an evidence gap.
- Own reliability planning: use Docker knowledge, public upstream bug evidence and
  actual test results. Do not ask the user for an exhaustive failure-mode list.
- Plan proofs, measurements and milestones autonomously before implementation
  where they reduce risk. Continue to the complete authorized phase outcome.

## Parallel work and research

- The OrbStack product-research chat owns `docs/research/orbstack/`. Do not edit
  its in-progress artifacts without coordination. It does not depend on completion
  of the planning documents.
- Implementation-facing requirements describe observable behavior and original
  Windows adaptations. Preserve evidence provenance and keep any proprietary
  analysis artifacts separate; do not treat a separate chat as a legal clean-room
  guarantee.
- Record tooling, Mac-access, and backend limitations precisely. Continue useful
  independent research when a particular observation or analysis route is unavailable.
