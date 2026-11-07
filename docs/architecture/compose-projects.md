# Compose file sets and profiles

Phase 2 first milestone, 2026-10-24. This is an original Windows convenience
over the existing Compose lifecycle, not a claim of observed OrbStack parity.
Phase 1 coverage records the single-file gap; independent research describes
Compose as a normal developer workflow but does not establish its full UI.

## Contract

Explicit interpolation env files are an ordered optional list of
at most 16 existing files, passed as separate `--env-file` arguments before the
subcommand. Compose owns parsing and precedence; later env files override earlier
ones, while launcher environment precedence remains upstream behavior. No file
contents or parsed values are added to saved input hints or resource labels. Only reviewed paths may be saved
by the existing unchecked Remember option. Blank retains ordinary Compose `.env`
behavior. Source inventory's env-file presence flag never supplies paths or
credentials: users must select original trusted files explicitly. WSL selections
must resolve as existing regular files inside the reviewed root and use the same
root/inode guard as Compose files. No file copying, broad environment changes or
fallback to another root is allowed. Missing/invalid paths refuse before client
dispatch. Cancellation/deadlines and output/secrets policy remain unchanged.

Reference: [Docker interpolation and ordered env files](https://docs.docker.com/compose/how-tos/environment-variables/variable-interpolation/).

rdd/Moby remain authoritative. The bundled, explicitly targeted Compose client
owns parsing, interpolation, merging, profiles and resource reconciliation.
OrbWin supplies a base file, ordered optional overrides, explicit project name
and optional profile names as separate argv entries, never shell text. Existing
single-file requests remain valid. All selected files must be absolute existing
files. At most 16 total files and 16 profiles are accepted. Profile names use the
existing bounded name validator; wildcard activation is intentionally unavailable.

The first file defines relative-path resolution, as documented by
[Docker](https://docs.docker.com/compose/how-tos/multiple-compose-files/merge/).
No YAML parser or engine project registry is introduced. Inputs are reviewed in
the existing operation dialog; optional lists use one item per line. Loading,
errors, output limits, cancellation, focus restoration and keyboard submission
reuse the existing dialog. No environment values, YAML or secrets are persisted.

Validation uses `config --quiet`, a 30-second read with no rendered configuration
output. It uses the existing verified-running-engine dispatch gate; offline
validation is not offered by this milestone. Compose can read local
interpolation/env/include files; errors may contain
source fragments. Trust selected files. Successful validation does not reserve
ports or guarantee a future pull/build/start. Files can change after validation.
Lifecycle actions retain their existing 600-second deadline, mutation exclusion,
bounded output and uncertain-mutation cancellation semantics. Cancellation kills
only the owned client. Down retains volumes; profile selection is not a promise
of service-scoped deletion. Review the whole project before Down.

Launcher Compose file/project/profile/env-file/remove-orphan selectors are cleared;
profile and remove-orphan controls are explicitly pinned to prevent hidden
activation/deletion from `.env`. Ordinary project `.env` interpolation remains
Compose behavior. No remove-orphans or volume-removal option is added.

Qualification must cover old request compatibility, ordered merges, selected and
unselected profiles, missing/invalid files, option injection, limits, quiet
validation without workloads, lifecycle/recreation and scoped cleanup. The engine
is never restarted for this milestone. Existing external engines and global
client defaults must remain unchanged.

## Reviewed discovery and remembered inputs

The group menu offers Validate, Up, Pull, logs and Down alongside existing
per-container actions. Discovery reads returned project/config_files/working_dir
labels from every container in the group. All snapshots must agree exactly;
file order is preserved. Only drive-absolute Windows paths are prefilled. The
adapter's canonical Win32 drive prefix is normalized; UNC/device/Linux paths,
control characters, ambiguous comma fragments, excessive file counts and explicit
environment-file labels fall back to an editable empty file set with an explanation.
Profiles cannot be recovered from the running-service list and are never inferred.
Labels may be supplied by any workload: they are untrusted hints, not authority
to run files. Every project action opens the full review form. Missing/moved
files and invalid working directories fail before client dispatch; edited files
are intentionally read anew by Compose and cannot be pinned by prior validation.

The optional directory becomes Compose's explicit `--project-directory`; blank
retains the base-file default. It is checked as an existing absolute directory.
Group deletion never becomes Compose Down automatically. Down's separate removal
confirmation describes whole-project containers/writable layers/networks and
retained volumes; selected profiles do not establish a narrower removal scope.

The unchecked Remember option stores inputs only after a successful operation.
Versioned WebView local storage holds at most 20 project-name-keyed sets and
2 million characters total: project name, ordered paths, directory and profile
names. A new successful opt-in replaces that project's prior hint. Loading
validates shape/limits; storage denial/quota failure is shown without changing
the operation's successful outcome. Forget removes only remembered hints and
does not touch files or engine objects. The Compose panel can reopen a remembered
set even after Down removes all containers. Every use remains editable and
reviewed. This storage is local to the WebView origin/profile, has no durability
or cross-installation migration guarantee, and never reconciles engine state.

Concurrent frontend windows use localStorage's whole-value replacement: last
successful write wins; no workload state depends on these hints. No autostart,
background retry, new backend supervisor or engine restart is introduced.

## Services and failure qualification

Up/Start/Stop/Restart/Pull/logs accept at most 64 explicit service names as
separate arguments after the subcommand options. Blank means Compose's enabled
services. Up retains Compose dependency/profile activation semantics; the form
describes possible dependency startup and recreation. Logs aggregate with Compose
service prefixes and accept the same service selection, bounded to 500 lines per
container plus the shared output limit. Exact service shortcuts use returned
`com.docker.compose.service` and project/file/directory labels, never display
names; one-off containers do not offer shortcuts. A service selects its replicas,
not one container ID. Invalid/unknown service names fail; no guessed fallback.

Validate always checks the whole file set. Down always removes the whole project.
Both reject nonempty service requests in Rust, even from direct IPC callers.
Their forms omit service choices and do not forward remembered selections.
Remembered inputs now optionally include service names, with backward-compatible
loading of sets created before this field existed.

The `owned_compose_journey` opt-in adapter proof covers selected/aggregate logs,
unknown services, moved files rejected before dispatch, an independently targeted
client recreating a service, occupied Windows port rejection/partial startup,
cancellation after readiness waiting begins, then explicit successful retry.
Cancellation stops only the Compose client; accepted containers and volume data
remain and are reconciled by subsequent reads. Its unique project and test volume
are removed only after the proof, including on a failed assertion path.

## Selected-source project review

The Containers page can inspect a supported Desktop/Rancher source and select an
observed Compose project. It reuses the same coherent Windows-path discovery
rules and opens the existing editable **Validate** form. No source workload is
stopped and no target project is started by this handoff. Service labels do not
infer profiles; named volumes, external env files and shared host binds receive
separate review. Successful validation can use the existing unchecked Remember
option, followed by a separately reviewed lifecycle action. Source connection
selection never implies ownership of Linux project files; WSL connection remains
explicit. See [migration](migration.md) for the contract, native evidence and
remaining combined migration gates.

## Explicit interpolation env-file qualification (2026-10-25)

Windows dispatch validates every selected env file as an existing absolute file
and passes repeated `--env-file` arguments in review order. WSL dispatch validates
bounded original/relative paths, maps each regular file inside the frozen export
root, and supplies them to Compose's initial bounded configuration resolution.
The resolved model then follows the existing filesystem adaptation/stdin workflow;
env contents are never added to labels. Reusable WSL labels retain only ordered
paths, and reopened forms copy the arrays. Old requests/hints default to no
explicit env-file selection. LocalStorage hints remain optional device-local
paths; they do not become credentials storage or migration history.

Source migration keeps its presence-only env-file metadata. It does not open or
copy source paths or values. A handoff from a source that indicates explicit env
files requires a manually selected nonempty env-file list in the review form.
The empty-list check applies to keyboard submission as well as native form
validation. The separate ordinary Compose form remains editable for intentional
configuration changes. Unknown/conflicting file labels still require manual input.

`artifacts/qualification/p2-compose-env-20261025.json` links the evidence. The
Windows native adapter quietly validates two Unicode env files, starts the owned
nonce project, checks the selected value, reverses the files and verifies the
changed value after reconciliation. An adjacent `.env` value is not selected.
Missing files refuse before dispatch, and Down removes the test workload. WSL
validates absolute and relative ordered env files as UID 1000, refuses reversed
files that resolve an invalid port and refuses a missing file. Root/signature,
registration and stale-build guards still pass. That WSL proof creates no
workloads; its four exact public fixture files and empty directory are removed.

The native desktop form separately accepts two ordered Windows env-file paths and
completes quiet validation with Remember unchecked and no container creation.
75 frontend tests, 19 operation tests, Linux helper tests and SDK/frontend/desktop
builds pass. Original/shared inode, private/project/global config hashes and three
daemon identities are unchanged; the installed Phase 1 window remains open. This
does not qualify installed env-file flows, every interpolation error/credential
case, WSL env-file lifecycle recreation, or the migration-specific required-list
UI state. Those cases and combined application migration remain acceptance work.

WSL workflows also retain the reviewed export root and device/inode signature.
Reopened group/service actions must carry that identity from their resource
labels; older labels without it direct the user to inspect the intended WSL
project. Native preparation refuses selection drift before interpreting relative
inputs. Compose files are relative to the selected working directory (the export
root when unspecified). See [WSL integration](wsl-integration.md) for compatibility
boundaries and native qualification.
