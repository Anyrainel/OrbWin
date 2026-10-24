# Native UI correction verification — 2026-10-22

## Sources and fidelity boundary

Official product page https://orbstack.dev/ links the current hero at
https://orbstack.dev/img/hero.png. Inspected those pixels and the official
https://docs.orbstack.dev/screenshots/k8s-hero-dark.png. Observed grouped
navigation, compact nested lists, list/inspector separation, subdued dark
surfaces, selected resource accent and five inspector tabs. Research images
were kept in ignored artifacts/ui-reference, not packaged or redistributed.
The separate research owner's active README, evidence register and flows were
consulted read-only. No live Mac observation or exact-version fidelity claim.

Windows adaptations: native Windows caption controls and chrome, Segoe UI,
Ctrl+F/Escape/arrow navigation, independent WSL distributions, fixed explicit
engine targeting and development-limit explanations. These interactions and
error/empty wording are original; screenshots do not prove OrbStack behavior.

## Executed checks

- npm run build: TypeScript + Vite pass after correction.
- npm run tauri build -- --debug: final native build passes.
- Opened the rebuilt native executable; final content visually inspected at
  the current approximately 1280x850 content size. Native minimize,
  maximize/restore and close controls remain present. Backend reattached.
- Native empty Containers view distinguishes an actual zero-container read
  from disconnection. Real Images returned alpine, postgres and redis.
- Actual injected key events, not accessibility value replacement: Ctrl+F
  focuses search; typing r/e/d narrowed three images to two for re, then one
  Redis result for red. Escape restored all three images.
- Clicking image row selected it and populated returned-field Info details.
  ArrowDown from Alpine selected PostgreSQL and changed the inspector.
- Clicking Logs showed its explicit unavailable explanation. Right arrow
  selected Terminal, moved the visible focus outline and showed its blocker.
- Ctrl+F and keyboard z produced No matching results and cleared the visible
  inspector rather than displaying the filtered-out selection.
- Final binary was reopened after field-label and refresh-layout refinements;
  Images selection and returned detail rendering were visually rechecked.

The host input method initially composed r. Escape dismissed composition and
Shift toggled Latin input before the successful direct-key filter checks.
Accessibility focus reporting sometimes lagged visible WebView state; visual
results were checked rather than treating stale accessibility text as proof.

## Artifacts

- screenshots/native-corrected-final.jpg: final binary, PostgreSQL selected.
- screenshots/native-corrected-images.jpg: Redis selected after keyboard filter.
- screenshots/native-corrected-keyboard-selection.jpg: original-size image view
  during keyboard verification. A resize attempt did not change window size;
  this is not compact-size evidence.
- screenshots/native-corrected-no-match.jpg: actual keyboard no-match result.

## Limits

Live Compose grouping, large lists, all display scales, minimum window size,
full keyboard-only journey, screen-reader announcements, error injection and
refresh cancellation were not exercised in this pass. Cancellation invalidates
read results and retains the in-flight guard until bounded native work settles;
it does not terminate native commands or alter workloads. No new lifecycle or
mutation adapter was added. Info is summary inventory, not full inspect output.
Builds, cluster, streaming, exec, files and settings gaps remain explicit in
backend-limitations.md and phase-1-coverage.md. Phase 1 is not complete.

No engine start/stop/reset, global context switch, WSL shutdown/unregister,
competing-engine manipulation or writable-disk change was performed. Only the
OrbWin renderer process was closed for native rebuilding. This is not new
backend reliability, data preservation or coexistence evidence.
