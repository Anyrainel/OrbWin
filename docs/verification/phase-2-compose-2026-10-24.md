# Phase 2 first milestone: Compose file sets and profiles

2026-10-24 Pacific. Phase 2 has started; this is one implemented convenience,
not phase completion or a replacement for the accepted Phase 1 record.

## Implemented scope

The native Compose dialog accepts a base file, up to 15 ordered overrides and
16 optional named profiles. Existing single-file callers remain compatible.
Validate performs bounded `config --quiet` checks without printing the merged
configuration. Lifecycle actions use the same selected file set and profiles.
Launcher selectors are cleared and implicit profile/orphan-removal settings are
pinned. Relative paths retain Compose's base-file semantics; no new orchestrator,
YAML parser, network/trust configuration or project registry was introduced.
See [ownership, cancellation and input contract](../architecture/compose-projects.md).

## Evidence

- `npm test`: 42 passing Node tests; frontend TypeScript/Vite production build passes.
- Ordinary Rust suite: 39 pass, 9 opt-in qualification tests ignored.
  New tests check single-file compatibility, merge order, read/mutation classification,
  validation deadline, profile/file count limits, missing paths and option injection.
- Clippy across all targets with warnings denied, Rust format check and debug
  Tauri build pass.
- Live native operation adapter: `owned_compose_profiles` passes on daemon
  `d9c00194-ea9d-439a-8561-6a0ec580fbd5` using the private configured Compose plugin (v5.6.1 in the follow-up run). The global client plugin reports v5.3.1 and is not the operation adapter plugin.
  Raw initial operation outcomes: `artifacts/qualification/compose-profiles-1792884417848799800.json`.
  The final regression run uses `compose-profiles-1792884685051264000.json`.
  Quiet validation emitted no configuration and created no project containers;
  invalid YAML schema failed as a read. Two override paths containing spaces
  applied in order (`MERGE_MARKER=last`). No optional services appeared without
  selection; selecting `dev` started `debug`, while `hidden` stayed absent.
  Restart, Stop/Start and Down/Up passed; the named-volume marker remained
  `retained` after recreation. Exact project containers/network and the test
  volume were removed. The final regression run starts with the `.env` fixture
  enabling `hidden` and orphan removal, plus conflicting launcher
  `COMPOSE_PROFILES`, `COMPOSE_FILE`, `COMPOSE_PROJECT_NAME`, `COMPOSE_ENV_FILES`
  and `COMPOSE_REMOVE_ORPHANS`. Blank and explicit `dev` selections still control
  which services start. Final cleanup enables both fixture profiles so a failed
  unexpected activation can also be removed within the exact unique project.
- Native development executable: reviewed the form at 1280 by 850 content size,
  field labels, paths with spaces, multiline overrides, profile input, pending
  state/disabled fields/cancel control, Enter submission, quiet success and Escape
  dismissal. Invalid schema produced visible output and exit-code-1 failure.
  An attempted Escape cancellation raced with successful completion; no active
  validation cancellation pass is claimed. Existing bounded-client cancellation
  tests remain applicable. The existing operation dialog supplies error/cancel handling.
  Accessibility focus metadata returned the root document; detailed restored-focus
  behavior is not established by that metadata. No new layout tokens/styles.
- Read-only postchecks retained OrbWin and both external daemon identities:
  Docker Desktop `a307ff0e-581d-49d3-8aee-a0636dda9137`, Rancher
  `741eb2cb-adf5-4676-bf28-f03f68d54f60` (its Ubuntu Unix socket, not
  `docker_engine`, which points to Docker Desktop on this host).
  Original global Docker/Kubernetes config hashes still match the Phase 1 final
  record. Validation-only UI project `orbwin-phase2-ui-validation` has no containers.
  Machine-readable postchecks are in `artifacts/qualification/phase-2-compose-postcheck.json`.

Reproduce the live adapter test only with the reviewed owned engine identity:

```powershell
$env:ORBWIN_QUALIFY_OWNED = '1'
cargo test --manifest-path src-tauri/Cargo.toml owned_compose_profiles -- --ignored --nocapture
```

## Limits and continuation

The running installed Phase 1 app/installer was not replaced. This milestone is
available in the rebuilt development executable. No engine restart, reset, prune,
global context switch or external-engine mutation was needed. Existing cancellation
and bounded output contracts apply; validation does not prove a future build or
start will succeed, and trusted project files can read interpolation/include data.

This milestone does not qualify automatic discovery of multi-file projects,
saved project sets, arbitrary WSL paths, service-scoped Down, local names/HTTPS,
migration or the complete Phase 2 acceptance matrix. The next Compose extension
should reuse the file-set contract for discovered project labels and saved inputs;
network/trust conveniences require a separate reversible setup/conflict design.

## Follow-up milestone: reviewed project reuse

Implemented ordered multi-file label discovery with exact agreement across group
members, explicit working-directory reuse and opt-in remembered input sets. Group
menus add Validate/Up/Pull and retain logs/Down and per-container actions. Every
Compose action opens the complete editable review form; conflicting, unsupported
or incomplete labels explain why paths must be supplied. Profiles are never
inferred from running containers. Remembered sets are hints, not engine state,
and Down still requires its own whole-project removal confirmation.

Checks at this milestone: 44 Node tests, 40 ordinary Rust tests (9 opt-in tests
ignored), production frontend build, Clippy with warnings denied and Rust format
check pass. Tests cover consistent ordered labels, mixed/conflicting metadata,
unsupported/env-file paths, canonical Windows drive prefixes, count limits,
corrupt/oversized storage, retained/replaced/forgotten hints, explicit working
directories and missing/moved/wrong-type paths rejected before dispatch.

Live adapter regression `owned_compose_profiles` passed with explicit directory:
`artifacts/qualification/compose-profiles-1792885631011746200.json` (148 seconds).
Named-volume marker survived Restart, Stop/Start and Down/Up; unselected hidden
service stayed absent. Exact project cleanup and test-volume deletion passed.
No engine restart or unrelated cleanup was performed. The observed labels contain
ordered canonical drive paths (`\\?\`), which discovery now normalizes narrowly.
The configured runtime plugin reports Compose **v5.6.1**; a CLI invocation without
the private config reports v5.3.1 and must not be used as adapter-version evidence.

Native development app at 1280 by 850: project menu, discovered base/two ordered
overrides/directory, empty profiles, editable inputs, explicit unchecked Remember
control, scrollable dialog and quiet successful validation were reviewed. The
Remember option was selected for the exact disposable test project's valid inputs.
After frontend reload, the remembered project still appeared in the selector
with no project containers remaining. The validation-only hint is retained in
the development origin; unit tests cover Forget without touching engine objects.
A form-control styling defect found in visual review was fixed using existing
tokens. This is a development WebView review, not installed upgrade qualification.

Read-only postchecks retained all three recorded daemon IDs and both global
Docker/Kubernetes config hashes from the previous postcheck. Exact test project
containers, networks and volumes were absent after cleanup; the machine-readable
record is `artifacts/qualification/phase-2-compose-reuse-postcheck.json`. The installed Phase 1 application was not
replaced, and optional Kubernetes was not reconfigured.

P2-CMP remains active: selected-service Compose operations/log filtering,
external recreation, deterministic partial-start/cancellation/retry/conflict
qualification and the combined installed Phase 2 gate remain. Existing client
cancellation/deadline tests pass; this follow-up does not claim a new live active
Compose cancellation pass or an engine-restart preservation pass.

## Service and failure journey follow-up

Subsequent implementation adds up to 64 explicit service names to Up/Start/Stop/
Restart/Pull/logs, remembered service choices and shortcuts derived from exact
returned service labels (never names or one-off inference). Validate/Down ignore
remembered choices in the form and reject nonempty service requests in Rust.
Logs aggregate or filter by selected services with Compose's prefixes.

`owned_compose_journey` passed on the owned daemon in 41.84 seconds. Raw evidence:
`artifacts/qualification/compose-journey-1792886516116846900.json`.
It tested app-only startup, optional worker startup, filtered/aggregate logs,
unknown-service rejection, missing/moved file pre-dispatch rejection, external
recreation via an independently targeted client, retained volume marker, occupied
Windows port failure/partial startup, explicit recovery, cancellation after the
health wait begins, accepted-container survival, then successful retry and the
unchanged volume marker. Exact project/volume cleanup passed. Engine restart
preservation and final native/installed review remain part of the combined gate.
