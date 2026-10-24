# Public UI-flow evidence and OrbWin requirements

Reviewed 2026-10-23 Pacific. Supplements the independent research record without
editing `docs/research/orbstack/`. No Mac observation, desktop automation, engine
mutation or binary analysis occurred. Published behavior and individual reports
are distinguished from runtime evidence.

## Version-bound findings

Paraphrases of the official [release notes](https://docs.orbstack.dev/release-notes),
checked on the review date:

| Version | Published UI behavior |
| --- | --- |
| 0.10.0 | Compose grouping, batch resource actions and a confirmation bypass modifier. |
| 0.16.0 | Searchable logs and Compose project log tabs. |
| 1.5.1 | Container deletion confirmation added. |
| 1.6.0 | Reading older logs no longer forces scrolling; Compose YAML access added. |
| 2.2.0 | Search retains log context; horizontal log scrolling and per-tab panel widths. |
| 2.2.2 | Optional matching-line filtering; delete shortcut confirmation and terminal-focus fix; missing-directory Compose deletion and terminal reopening fixes. |
| 2.2.3 | Build history with steps/logs and optional log wrapping. |
| 2.2.4 | Compose service log filtering and JSON highlighting; duplicate/missing log fixes. |

These establish capabilities and changes, not exact control positions, defaults,
prompt wording, cancellation results or accessibility behavior. Earlier versions
must not be treated as evidence of current defaults.

## Reports that clarify branches

| Source | Finding | Limit |
| --- | --- | --- |
| [Issue 421](https://github.com/orbstack/orbstack/issues/421), July 2023 | Maintainer explained the then-different confirmation policy for volumes versus containers/images. | Historical policy, superseded by later releases. |
| [Issue 2557](https://github.com/orbstack/orbstack/issues/2557), June–August 2026 | Reported deletion shortcut intercepted terminal editing; maintainer identified 2.2.2 as the fix. | Not locally reproduced; not proof of every keyboard path. |
| [Issue 711](https://github.com/orbstack/orbstack/issues/711), October 2023 | Incoming logs reportedly disrupted reading older output. | Later release notes describe a fix; anchor implementation remains unknown. |
| [Issue 652](https://github.com/orbstack/orbstack/issues/652), reported 0.17.3 | Terminal entry reportedly failed with another Docker context selected. | Historical report, not a current OrbStack defect or OrbWin reproduction. |

[Debug Shell documentation](https://docs.orbstack.dev/features/debug) describes
right-click entry to a separate environment supporting containers without a shell.
Ordinary `exec sh` does not establish equivalent functionality.
[Settings documentation](https://docs.orbstack.dev/settings) distinguishes
Docker-data reset from whole-product reset. Prompt and interrupted-reset behavior
remain unobserved.

## Implementation requirements — original OrbWin choices

These requirements are informed by the evidence, not claims about OrbStack's
implementation. Qualification remains mandatory.

| Priority | Required flow | Current gap and acceptance scenarios |
| --- | --- | --- |
| P1 | Logs: preserve context or explicitly filter matches; wrapping independent; incoming output must not pull the reader away from history. | Context highlighting, match count, matching-line toggle and wrapping added in this pass. Check native behavior, partial lines, bounded-history eviction, export and scroll anchoring. |
| P1 | Compose: act on selected projects/services without repeatedly entering file/name; aggregate logs with service selection. | Manual form/group labels are partial. Check moved/missing files, external recreation, partial startup and cancellation. Never guess source files or silently adopt another project. |
| P1 | Deletion: consistent explicit confirmation and consequences; terminal/text editing never invokes removal. | Check focus, selected identity, stale resources, in-use rejection and volume preservation. Windows needs no confirmation bypass shortcut. |
| P1 | Terminal: actual attach/failure/exit/disconnect states and explicit reopening against the owned container. | Check native reopen, missing shell, stopped/recreated targets and conflicting contexts. Ordinary shell must retain its honest label. |
| P1 | Builds: distinguish submission progress from retained history, selected-build steps/logs and failure. | Audit pinned backend/client history before choosing storage. Do not add a competing authoritative build database. |
| P1 | Recovery: unavailable reads differ from empty inventories; bounded retry identifies the failed layer and preserves diagnostics. | Check stale responses, independent stats failure, partial readiness and reconnect. A UI error does not establish that engine restart is safe. |
| P2 | Layout continuity: retain deliberate panel/group choices and selection when identities still match. | Define Windows persistence/reset keys; avoid storing secrets or stale workload authority. Verify keyboard/accessibility behavior. |

Priorities order work, not delivery phases or scope exclusions. Debug Shell/file
conveniences still require roadmap and backend capability accounting.

## Remaining observation targets

Public evidence is sufficient to continue implementation. It does not settle
first-run screen order/permission denial/download cancellation; partial lifecycle
and Compose retry; current prompt/default focus; image-tag deletion scope; log
search navigation/clear/follow; build details and batch outcomes; settings
apply/restart and interrupted persistence; or accessibility announcements.

On an authorized Mac, collect short named-build scenarios with before/action/after
evidence and actual engine results using the existing
[observation plan](../research/orbstack/mac-observation-plan.md). Public demos can
establish visible sequences only; no new demo was viewed in this pass. Screenshots
and release notes do not constitute runtime observation. OrbWin real-computer
verification remains deferred until the user confirms availability.

## Implementation validation

The log controls use escaped literal matching and React text rendering, retain
the existing bounded output and export the displayed text. No HTML from logs is
executed. Frontend compilation and the existing headless suite are the available
checks; native rendering, keyboard use and stream/scroll behavior remain pending.
