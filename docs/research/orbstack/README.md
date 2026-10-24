# OrbStack behavior research

Scope corrected following user clarification: this deliverable concerns **OrbStack itself**. It contains no backend feasibility assessment, Windows adaptation, implementation plan or product requirements for another application. Baseline research date: 2026-10-21 Pacific.

## Contents

- [Behavior catalog](flows.md): what the evidence establishes about OrbStack flows, including branches and explicitly missing behavior.
- [Evidence register](evidence-register.md): official sources, screenshot observations, provenance and confidence.
- [REA assessment](rea-assessment.md): actual tool execution, available artifacts and provider limitations.
- [Observation gaps](observation-gaps.md): unanswered questions needed for deeper reconstruction.
- [Mac observation plan](mac-observation-plan.md): conditional procedure if runtime access becomes available.

## Evidence boundary

**Documented** means an official source states the behavior; **observed screenshot** means only visible pixels were inspected; **observed diagnostic** means a local tool check ran. **Statically inferred** requires actual target inspection; no such OrbStack finding exists yet. **Unknown** remains unknown rather than being filled with another product's behavior or our design choices.

REA 6.7.0 help/provider/doctor commands ran. The supplied Ghidra 12.1.4 installation passed scoped readiness checks after its path was supplied. The user-supplied DMG exists, but its contents/version/signature have not been authenticated or analyzed. No OrbStack binary investigation or live macOS observation has occurred. This package is an initial evidence baseline, not a verified reconstruction of every flow.

## Correction to earlier output

This session originally produced ten Markdown documents mixing OrbStack sources with proposed product flows, design decisions, backend analysis and qualification plans. Those mixed drafts are withdrawn from the active deliverable. They are preserved only in ignored `support/superseded-mixed-drafts/` for traceability; they must not be used as OrbStack findings. Existing temporary source checkouts are also excluded. No application code was produced.

Future research must add evidence about OrbStack before deepening a claim. Maintain stable OS flow IDs and link every nontrivial branch to evidence. Do not import backend constraints or implementation preferences into the findings.
