# Missing OrbStack behavior evidence

The intended research unit is a full flow: actor/intent, entry, preconditions, actions, branch conditions, transitions, side effects, output, failures, cancellation, retry, persistence and keyboard/accessibility behavior. A feature list does not establish these details.

| Priority | Question | Evidence needed |
|---|---|---|
| 1 | What actually happens during startup, partial readiness, failure and recovery? | Named-build runtime scenario or explicit official explanation with logs/sequence. |
| 1 | What do container/project start, stop, restart and removal do in every supported state? | Actual controls, confirmations, engine results, concurrent/external action cases. |
| 1 | How do Compose source/config errors and partial service failures appear? | Reproducible examples and resulting resources; cancellation/retry outcome. |
| 1 | Which data survives deletion, reset, update and interrupted operations? | Version-bound documentation or controlled before/after data checks. |
| 2 | How do domains resolve collisions, ambiguous ports and unavailable targets? | Routing/failure evidence, not inferred implementation. |
| 2 | What happens after denied HTTPS trust, CA changes or proxy failure? | Prompt/action/result traces with version and browser. |
| 2 | What are logs/shell/statistics controls and disconnect semantics? | Recorded interaction, CLI comparison and target state. |
| 2 | What are file access consistency and permission failure behaviors? | Defined target state, file operation and observed outcome. |
| 2 | What are image/build/volume workflow details? | Actual forms, defaults, progress, errors and destructive scope. |
| 3 | What are shortcuts, focus semantics and screen-reader announcements? | Listed shortcuts and direct accessibility observation. |

Without a Mac, continue with official documentation, public demonstrations and version-bound public issue evidence. Each report establishes a particular reported scenario, not a universal rule. Record reproduction status and contradictions. Static target analysis, if permissible and supported, may suggest branches but must remain statically inferred until corroborated. Do not fill gaps using another implementation or proposed behavior.

The supplied DMG is an available candidate artifact; filename alone does not authenticate its version, architecture or origin. Ghidra readiness now passes, but REA Windows Mach-O admission remains a separate limitation. No target analysis has been performed.
