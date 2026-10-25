# OrbStack-informed Windows interaction specification

Updated 2026-10-22 Pacific. Supersedes the initial dashboard specification.

## Evidence and boundaries

Observed pixels: official [product hero](https://orbstack.dev/img/hero.png),
linked from [the product page](https://orbstack.dev/), inspected 2026-10-22;
and [Kubernetes screenshot](https://docs.orbstack.dev/screenshots/k8s-hero-dark.png),
also catalogued as O14 by the independent research. Local research copies are
ignored under artifacts/ui-reference; third-party screenshots are not app assets.
No screenshot version binding or live Mac interaction has been established.

The hero shows grouped Docker navigation (Containers, Volumes, Images, Builds,
Networks), Kubernetes Pods/Services, Linux Machines, compact Compose group rows
with nested containers, and adjacent Info/Stats/Logs/Terminal/Files tabs. O14
shows namespace service lists beside field/value details. Both use subdued dark
surfaces, narrow separators, magenta resource icons and a saturated selected
resource row. These are visible findings, not evidence of hidden interactions.

## Current layout and tokens

Open Containers directly. Remove the invented overview, workspace branding block,
marketing cards, breadcrumb, and oversized page headings. Use a 190px grouped
navigation, 350px list and flexible inspector; at <=1050px use 170px/300px.
Keep native Tauri decorations enabled: Windows title bar, minimize, maximize /
restore and close. Never draw macOS traffic lights. Segoe UI, 13px body, 11px
metadata, 16px list title, 32px navigation rows and 51px resource rows. Colors:
content #272327, sidebar #302732, selected resource #b91acd, text #e8e5e9,
secondary #aaa3ad, focus #81baff. Surfaces stay flat with small grouped corners.

## Interaction contracts — original Windows adaptations

Resource buttons select a current snapshot item for the adjacent inspector.
Arrow keys/Home/End move selection within visible rows; normal Tab/Enter/Space
remain available. Ctrl+F focuses search, Escape clears it. Search covers returned
fields. Compose grouping uses returned project labels only; no invented projects.
Refresh removes detail selection if the old snapshot object is no longer present.
Tabs use roving focus and arrow navigation. Search, navigation, counts and
selection all reflect actual reads. No fabricated workloads or statistics.

Read failures differ from successful empty inventories. Checking, unavailable,
empty, no-match, selected and blocked-tab states are distinct. Errors are live
announced. Refresh cancellation discards the pending result; it does not cancel
native subprocesses or mutate the engine. Native reads remain bounded by the
existing adapter. No UI action implicitly starts/stops a daemon or changes global
contexts. Backend-limited actions show a useful explanation, not a success toast.

WSL distributions replaces Machines as an explicit Windows adaptation. Settings
and Diagnostics replace unsupported General destinations; Pods/Services and Builds
stay visible with honest integration gaps. Cluster setup, mutations, streaming,
filesystem access and resource editing remain phase-1 work, not scope exclusions.
Observed screenshots do not establish OrbStack empty/error text, keyboard
shortcuts, cancellation or confirmation semantics; those choices are original.

## Additional interaction evidence — 2026-10-23

The [public UI-flow review](verification/orbstack-ui-flows-2026-10-23.md)
adds version-bound release-note and issue evidence, with implementation requirements
for logs, Compose, deletion, terminals and builds. Treat those requirements as
original OrbWin adaptations. They supplement the screenshot baseline without
claiming exact current OrbStack prompts, defaults or runtime parity.

## Logo

The official OrbWin mark is a magenta sphere cut by a window cross on a plum
tile, with the top-right pane lit. Source of truth is `assets/orbwin.svg`
(full-color tile) and `assets/orbwin-mono.svg` (single-color, `currentColor`,
for tray or monochrome uses). `python scripts/generate-icons.py` regenerates the
SVGs and the Tauri icons in `src-tauri/icons`. Geometry: gap 14, pane corner
radius 12, orb radius 88 in a 256 grid. Do not recolor the lit pane or add
macOS-style glossy effects. No tray icon or in-app logo is wired yet; the
design system keeps the sidebar free of a branding block.
