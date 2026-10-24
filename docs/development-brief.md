# OrbWin development brief

Updated: 2026-10-22, America/Los_Angeles.

This is the implementation handoff after the user's six planning clarifications.
Read the decisions, roadmap, quality standard and repository instructions as the
authoritative product direction. Development is authorized, not merely another
planning or feasibility report. Start with necessary validation and proceed into
the working implementation; maintain honest milestone and phase status.

## Product and phase scope

Build OrbStack-quality Windows development experience on Rancher Desktop 2.0's
standalone `rdd` and Microsoft WSL 2. Use Tauri, React/TypeScript and a thin Rust
desktop integration layer; reuse Go/backend code and upstream capabilities. Keep
Kubernetes compatibility and meaningful local-cluster workflows. No custom WSL,
hypervisor, kernel or Moby rewrite.

Phase 1 is a full relevant Rancher-engine plus OrbStack-inspired UI product.
Prototype and qualify core journeys first, then complete the feature/flow matrix.
Do not redefine phase 1 as a minimal demo. Document niche-feature exclusions and
backend-limited adaptations. Phase 2 adds missing conveniences and migration gaps;
phase 3 measures and improves reliability/performance; phase 4 ensures/expands
compatibility. Correctness, reliability and data safety apply throughout.

The user has limited first-hand experience of Docker Desktop crashing during a
test using Linux-only components. No root cause is known. Prioritize repeated and
concurrent Docker-dependent integration/CI-style test workflows and Linux
server/service components; do not wait for an exhaustive user workload list.

## First implementation priorities

1. Inventory the current repository and available build/runtime tools. Establish
   reproducible source/dependency versions and minimal build instructions. The
   initial repository was documentation-only and not a Git repository; verify the
   current state. Use local project work, not a user-unrequested worktree.
2. Pin and reproduce a suitable Rancher 2.0 baseline. Evaluate its actual release
   and source API support, Windows packaging, lifecycle and Docker passthrough.
   Preserve source notices/licenses and an explicit upstream patch/update record.
3. Prove independent identity/storage/endpoints and simultaneous operation with
   Docker Desktop and Rancher. Inspect all shared integration surfaces, not just
   Docker contexts. Use a product-owned test instance. A source check found a
   shared Windows Docker pipe; see the decision record. Prefer narrow upstreamable
   host-integration changes and contract tests over engine changes.
4. Validate real Docker CLI/Compose/Buildx operations, test-client discovery,
   streaming logs, interactive exec, Linux service readiness, cleanup, Windows/WSL
   paths and data preservation. Collect baseline measurements before changing the
   relevant component. Diagnose failure layers separately.
5. Establish information architecture, design tokens, interaction/state contracts
   and reusable components; build and visually inspect representative real flows.
   Implement real backend operations early. Mock fixtures may exercise UI states,
   but a mock UI is not a completed integration milestone.
6. Complete the relevant phase 1 surface through working milestones. Maintain
   feature/flow coverage, backend limitations, verification results and a concrete
   next-action record. Apply the phase exit gates before claiming completion.

Order and split milestones as evidence warrants. Measurement or proofs may precede
implementation in any phase. Carry the authorized build forward; do not stop at
an outline, scaffold, happy-path demo or an early core-journey milestone.

## Coexistence and migration

Simultaneous operation is a requirement. Use distinct owned listener names,
management endpoints, service/instance identities, data, Docker/Kubernetes contexts
and appropriate WSL integration. Check global PATH/config/credentials/settings
changes and conflicts. Identical host address/port bindings cannot be silently
promised to two engines: provide clear conflict handling and test traffic routing.
Existing third-party default-discovery behavior may need explicit targeting; test
the clients important to integration suites, including helper containers.

Reuse whatever migration the selected Rancher version actually supports. Do not
assume Rancher 1.x -> 2.0, Docker Desktop -> Rancher, or Rancher -> independently
owned OrbWin state is already implemented. Compatible explicit daemon attachment
is separate from migration/ownership transfer. Preserve users' original data.
Custom migration/transition gaps belong in phase 2; record them during phase 1.

## Recovery, quality and upgrades

Start from evidence-backed OrbStack behavior, then apply its intended smoothness
to Windows/WSL-specific cases. Choose smooth, clean recovery where feasible and
transparent, fast failure when reliable recovery is infeasible or overly complex.
Do not invent source behavior, hide failures, or use destructive/global resets as
routine recovery. Closing the UI must not kill healthy workloads.

Core code needs explicit state/resource/concurrency/failure reasoning and relevant
integration, race/leak and fault-path tests. The frontend needs a deliberate design
system, coherent state handling, keyboard/accessibility coverage and actual visual
QA. Every phase ends with a working product and no known unresolved owned defects
that invalidate its gates. Testing is evidence, not proof of zero bugs.

Keep the upstream wrapper clean: isolate version-specific APIs, minimize patches,
record provenance and supported versions, and qualify frequent Rancher updates.
Breaking upstream APIs mean adaptation work. Do not scatter those assumptions
through presentation code or create a second authoritative lifecycle manager.

Engineering owns the reliability scenario catalog. Seed it from Docker/platform
knowledge, public upstream issue trackers/forums and representative test-framework
workloads. Track versions and reproduction status, then expand from actual test
evidence. Do not turn anecdotes into blanket comparative reliability claims.

## Research and host boundaries

The separate research chat owns `docs/research/orbstack/`. Read its active index
and relevant evidence; do not edit its outputs or treat ignored superseded mixed
drafts as requirements. Its current deliverable is OrbStack behavior research, not
an OrbWin implementation specification. Write original adaptations in implementation
documentation, label unknown source behavior, and continue independently while
research deepens. No requirement to repeat binary research before building.

Existing Docker Desktop/WSL environments and persistent data are valuable. Do not
reset/unregister/migrate/delete them, terminate their workloads, install a competing
default listener, or perform global WSL shutdown for convenience. Use isolated
project-owned instances; run destructive/resource-exhaustion tests in disposable
environments. Obtain missing host capability or genuinely consequential choices
from the user only when necessary after independent work is exhausted. Do not
require repeated approvals for already authorized reversible development work.

Work in this chat's project directory and keep durable progress/verification notes
for continuation. Make no unsupported completion, parity or performance claims.
