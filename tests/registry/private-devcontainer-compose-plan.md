# D09 Dev Containers Compose/private FROM proposal

## Dedicated actual journey passed — 2026-10-25

Fresh receipt `orbwin-private-client-d96d1b87971b4271a0618730e18a2c76/state.json`
has `success:true`, `cleaned:true`, no errors and exact three-engine baseline
equality. Positive actual alias/default Compose build/up passed, including the
869-byte generated Dockerfile outside context, marker/workspace roundtrip and
same-ID reopen. Registry log observations advanced 2/0 to 6/3 success/refusal.
After native Forget, the real Dev Containers client refused its private source
pull before reaching Compose build: one exact failed source-pull event, five
auth diagnostics, no build/up/generated-override event. Manifest observations
advanced 6/3 to 6/8, with no new authenticated success. No negative container or
output image appeared, and the registry was still running. This is a qualified
early Dev Containers authentication refusal, not a negative Compose build; the
separate plain Compose receipt covers that build path.

Both earlier failed attempts remain failed evidence. BuildKit records/history
advanced 40/30 to 41/31 in this passing run; cache/ref/activity and contexts remain
accounted without pruning. Runtime read-grant argv was not observed; generated
file/patch/plugin identity and successful original build provide the previously
documented source-backed inference. This remains CLI automation, not an
interactive editor/installed journey.

The passing run used a frozen, hash-checked existing native credential test binary
(`46bb6ca01a041026968618650446d67c1f16f4ee3c284940f04f8d6a7f750119`)
through explicit `ORBWIN_PRIVATE_NATIVE_TEST` and expected-SHA opt-ins. Its fixed
ignored-test selection and 480s bound are unchanged. The main harness reloads the
child-owned state before recording its outcome, avoiding stale overwrite. This
allows independent source edits to continue without compiling them into this
registry proof. The receipt distinguishes that frozen binary from current Rust
sources; no newer application-source qualification is inferred. The optional
path must be a regular canonical file under qualification artifacts, exact
expected basename/hash and at most 64MiB, and is rehashed before/after use.

## Second actual dedicated attempt and revised negative contract — 2026-10-25

Receipt `artifacts/qualification/orbwin-private-client-1a2ae6073b254bfa99f05bd96eb5a0d9/state.json`
preserves another failed overall attempt (`success:false`, `cleaned:true`). The
positive actual default-plugin build/up, generated Dockerfile, marker/write and
same-ID reopen checks passed; its accepted fixture base was removed before
Forget. Positive manifest observations were 2/0 → 6/3 success/refusal. Negative
observations were 6/3 → 6/8, with no new manifest success. The negative stage
failed the then-mandatory Compose build-event assertion at line268 before
generated-source verification. Raw child output was deliberately not retained,
so this receipt cannot retrospectively prove which native command failed or be
relabeled a passed journey. Root reported complete cleanup and equal snapshots.

The revised source contract distinguishes authentication refusal during the
original Dev Containers private-source pull/inspection from a refusal during a
Compose build. The pinned CLI source performs `inspect --type image <source>`
and may fall back to `pull <source>` before it generates its Compose build
override. Therefore an absent generated file/build command after Forget is not
itself a product failure, provided the early authentication boundary is proven.
The independent plain-Compose negative build receipt remains a separate proof.

The next negative receipt requires a nonzero actual Dev Containers exit plus
JSON events for the **exact current alias and exact nonce source image** pull
and an exact matching `Command failed:` pull command, an authentication-refusal
diagnostic, a new nonce/known-digest manifest401 observation with no new200, no
output image/container and a still-running registry. A Compose up event without
a build is refused as an unexpected phase. For an early source refusal it
records `phase:prebuild-private-source-auth-refusal` and
`composeBuildObserved:false`; it does not claim default-plugin use, a generated
Dockerfile, a negative build, or build success. If a Compose build event is
observed instead, the exact original-alias command and generated-file validation
remain mandatory, and the phase is `compose-build-auth-refusal`.

Minimized JSON/start/alias/source-pull/source-inspect/source-failure/failed-pull/auth/Compose
build/up/generated-override counts and child exit status are saved before
negative assertions, including unparsed-line counts. No arbitrary event text,
credential-bearing output or unrelated command paths are retained. Missing or
changed real CLI event formats fail closed for root review; fabricated events
are not added to compensate. Positive build/up and generated-source requirements
are unchanged. Both prior failed receipts remain immutable. Ten dedicated pure
tests and20combined observer/extension tests pass; revised runtime behavior is
not yet qualified.

## First actual dedicated attempt — 2026-10-25

Receipt `artifacts/qualification/orbwin-private-client-ae9d3ef2485b4495b7db40389101c581/state.json`
records actual default-plugin private FROM build/up, generated outside-context
Dockerfile (869 bytes), marker/workspace writes and same-ID reopen success.
Selected manifest log observations advanced from 2/0 success/refusal to 6/3
before later operations. Native Forget completed, but the negative callback
stopped at its base-reference-absence precondition before issuing a build.
Dev Containers had materialized the private base reference during its positive
flow; the harness had only removed its generated output. This attempt is not a
passing negative authentication journey. Exact outer cleanup passed and restored
all three inventories/states and client configuration (`cleaned:true`).

The positive cleanup now also removes that independently known fixture base
before Forget, after checking exact committed ID, non-baseline status, nonce,
sole expected tag, finite command, base rootfs, platform and no volumes/entrypoint.
It uses non-force removal and refuses altered/shared references. A focused test
rejects foreign IDs, extra tags, baseline IDs and changed content. Cache/history
remain retained. The failed attempt is preserved; a new run is required to
qualify the negative flow. The preparation narrative below describes the source
contract and does not supersede these actual observations.

Source-only preparation,2026-10-25. Root currently owns an unrelated published-service engine fixture. Do not execute engine mutations or alter its receipts. The completed plain Compose stage/receipts remain intact.

## Ownership and admission

Proposed new files: `scripts/private-devcontainer-compose-stage.mjs`, its dedicated pure tests, and this plan. Reuse exported pinned-tool/plugin verification, cache/metadata observation, sanitized registry-request parsing and synthetic native credential owner. No second helper store, production changes, plugin installation, global settings or real credentials. Add main-harness opt-in wiring only after root reviews the separate module. Existing plain Compose flow is not rewritten.

Use current Dev Containers0.469.0 CLI with its exact recorded hash and current OrbWin alias. No `--docker-compose-path`, discovery-rejection adapter, empty config, TEMP/TMP override, broad filesystem entitlement or `COMPOSE_BAKE=false`. Child-only product DOCKER_CONFIG/helper PATH matches the qualified image-only journey; default plugin discovery is independently checked against actual selected Compose/Buildx bytes. Normal inherited temporary directory is retained and hashed/path-scoped in evidence, not changed to the build context.

## Positive journey

Inside actual native Login's authenticated interval, after the existing client/plain Compose stages, create a new nonce workspace subdirectory (not the parent directory holding synthetic htpasswd). Bind only this subdirectory to `/workspace`. Compose service uses a new output tag and exact nonce name/labels; context has a tiny Dockerfile with the independently verified nonce private FROM, no remote syntax frontend/install/features, a unique build marker and finite BusyBox timeout CMD. Build pulls and no-cache are explicit. Prove private base/output reference absence; nonce layer content can remain cached. Bound CPU/RAM/PIDs, drop capabilities, no-new-privileges, network none, no published ports/default network/restart. Ephemeral writable container root may be needed by the real Dev Containers setup; do not call it readonly. No credential directory is mounted.

Actual Dev Containers `up --build-no-cache --update-remote-user-uid-default never --skip-post-create` must build through ordinary alias/default-plugin discovery. Capture a sanitized registry baseline immediately before and manifest200 observation delta immediately after this original up; a later companion build must not substitute for the original flow's authentication proof. Verify returned full container ID, actual image/nonce/base-rootfs marker, finite nonempty CMD and inspected entrypoint execution semantics, limits and exact bind. Dev Containers may inject a shell wrapper with an unreachable fallback loop; acceptance requires its exec path actually preserves the verified finite command rather than silently accepting an empty/replaced command. Verify build marker, seed read, container-to-host write, host-to-container write, and `--expect-existing-container` reopen returning the same immutable ID.

Parse only the CLI's generated build override event in RAM to identify the actual `Dockerfile-with-features` outside the build context. Require a bounded regular file under the ordinary Dev Containers temp subtree, validate source context relationship, and hash its bytes. It must include the original private FROM/nonce marker and expected generated base-stage structure, with no unreviewed external feature/install steps. Keep the actual generated Dockerfile path/hash as a fixture receipt, not an invented path or manually generated replacement. Do not delete the shared Dev Containers temp root.

## Exact-file grant evidence proposal

The adopted patch adds the resolved selected regular Dockerfile to Bake's read grants. The original successful default-plugin up plus actual generated-path/hash and verified patch/binary receipt establishes a source-backed inference of its use. Root reviewed and selected this approach, together with the separate earlier positive/sibling-denial entitlement receipt. Runtime grant argv is not claimed observed by this journey. There is no companion rebuild, adapter or grant override.

Read-only pinned source inspection found a hidden Compose `--verbose` flag that controls its trace logger and the Bake argument debug line. No `COMPOSE_LOG_LEVEL` implementation was found in the pinned Compose command source, and ordinary Dev Containers default-plugin invocation does not pass `--verbose`. Do not invent that environment control or modify invocation selection to obtain it. The extension uses Dev Containers' supported `--log-level trace` for its own original build-override event; this is not claimed to enable Compose's trace logger.

## Negative and preservation

After actual native Forget, use a fresh nonce Dev Containers workspace/output reference (same independently verified private base) and request an up with forced build. Require authentication refusal at the exact observed private-source pull/inspection or actual Compose build phase, plus new nonce/known-digest manifest401 observations with no new200 while registry remains running, and no accepted container/output image. Early source refusal explicitly does not exercise a negative Compose build. Existing cached base layers or builder auth sessions cannot silently excuse negative success; unexpected success is an acceptance failure with guarded cleanup.

Take separate before/after cache/history and metadata observations for this extension without overwriting plain Compose receipts. Preserve selection/default-instance metadata and account retained refs/activity/history/cache; no prune/default-builder change. Journal every intended container/image before dispatch and every accepted full ID. Rebuild may leave the original running container's image dangling while moving its output tag; retain each verified nonce full image ID and remove only after all fixture containers are stopped/removed. Refuse unrelated tags, altered nonce/marker/rootfs/command/mounts or baseline IDs.

The specialized cleanup must run before the old generic fixture guard, which does not understand Compose-generated images/containers. Source contexts/generated files and cache/ref receipts remain explicitly listed; credentials/private config restore through the existing native owner. A timeout/crash may require exact manual recovery. Root chooses and reviews combined client/server deadlines before execution; finite workload CMD and service bounds do not constrain BuildKit worker resources. This plan contains no claim that the new journey already passed.

## Prepared module and approved dedicated wiring

`scripts/private-devcontainer-compose-stage.mjs` exports `runPrivateDevcontainerCompose(ctx, mode)` (async) and `cleanupPrivateDevcontainerCompose(ctx, final=true)` with the existing `{root,state,execute,intent,save}` context and optional synchronous `withCleanupBudget`. Root approved minimal main wiring after timing review. `ORBWIN_QUALIFY_DEVCONTAINER_COMPOSE_ONLY=1` selects a dedicated run and rejects combining old WSL/plain Compose opt-ins. Existing fixture preparation/native credential owner remain; no Rust, production/package or finished receipt changed.

Authenticated order is exact: current alias/three-engine identity check; nonce push; independent authenticated manifest/config digest/nonce/rootfs verification; remove only the verified fixture source alias; early branch to new original Dev Containers Compose up/build; capture its manifest200 delta before marker/write/reopen; verify generated file/source/actual alias commands; verify marker/write/reopen; specialized cleanup; cache/metadata accounting. It never repeats the old Windows forced pull, image-only Dev Containers, WSL or plain Compose journeys. Native owner then actually Forgets and verifies saved metadata. Forgotten callback checks identity and early-branches directly to fresh negative Dev Containers up with forced-build requested; requires exact source-refusal/build phase evidence, its own manifest401 delta with no new200 and absence of container/output, registry still running. Existing native credential/profile restoration completes before the outer Node specialized fixture cleanup, then old generic nonce cleanup and full snapshots run.

The module records its own plugin/cache/metadata baseline and actual Compose patch SHA, current Dev Containers CLI hash, source plans, generated file path/hash/size, sanitized registry samples, immutable accepted IDs and metadata/cache/history residue. It verifies actual original-flow alias/default-plugin Compose build/up command events (a version probe or standalone fallback cannot satisfy that check), generated feature stages with no added RUN/install/feature/external FROM, exact nonce base/rootfs marker, finite300s CMD, inspected exec-preserving entrypoint, one exact workspace bind and limits. A writable ephemeral root is explicit; no credentials/parent auth directory are mounted. Seven dedicated pure tests pass, with17combined tests including the shared observer, and main/module syntax passes.

Initial Dev Containers up is bounded120s, each exec20s with its own5s BusyBox command timeout, and reopen45s. Other client reads/stops are bounded30s and registry log reads2s with at most five observations/four150ms waits. Root must review combined native/client/server timeouts before execution because multiple prior journeys plus these calls can exceed a parent's budget. Creator loss is bounded by the verified300s workload command but native cleanup is not guaranteed after process termination. No engine call, private build, credential/config mutation or UI run was performed by this preparation.

## Cumulative deadline and partial-result policy

Each dedicated child records a cumulative callback clock before publication/identity work. Positive total180s reserves20s for cleanup (ordinary work160s); negative total90s reserves20s (work70s). Every actual execute timeout, including cache/plugin/log observations, is clamped to the remaining current-phase budget. Both independent HTTP fetches are clamped too. A child command returning exit0 after that deadline is rejected, and the callback checks again before completion; it cannot claim success merely because a late process returned zero. Safe deadline mode/phase/code is retained, including failures before extension initialization.

Normal positive specialized cleanup and catch-path partial cleanup can enter the remaining20s reserve. Its stop/inspect/remove commands remain individually clamped to the absolute callback end; this is a best-effort reserve, not permission to skip ownership checks. On any source/ownership/error/timeout condition, original failure remains recorded and a refused/expired cleanup gets a separate sanitized code. Accepted container/image intents and full IDs remain for the fresh outer cleanup. A paused, retagged, replaced or unexpectedly mounted fixture is preserved/refused. Source/reference removal and build mutation may have committed even when a client times out.

Native client-tree240s per child and outer Cargo480s remain unchanged. Thus admitted callback work/reserve is at most270s across both children, subject to ordinary scheduler/filesystem overhead; individual child command timeouts are not added afresh for every command. The outer480s cap also contains actual native credential operations, each with an existing120s production timeout. Up to eight List/Login/Forget calls plus inspect can theoretically exceed that outer budget even without both callbacks. Consequently480s is an interruption backstop, not a cleanup-completion guarantee; killed native ownership can lose its RAM snapshots and leave synthetic routes/credential state for exact manual recovery. No native operation timeout or production semantics were shortened or silently dropped, and no larger timeout was introduced. The separate registry600s and workload300s self-exits limit fixture activity, not credential/config recovery. Host-side preparation/final snapshots have their existing per-command bounds; no single whole-Node-run wall-clock guarantee is asserted.

Root-reviewed execution shape (not executed by this preparation):

```powershell
$env:ORBWIN_QUALIFY_OWNED='1'
$env:ORBWIN_QUALIFY_PRIVATE_CLIENTS='1'
$env:ORBWIN_QUALIFY_DOCKER_WRAPPER=(Resolve-Path artifacts/package/orbwin-docker.exe).Path
$env:ORBWIN_QUALIFY_WSL_PRIVATE=''
$env:ORBWIN_QUALIFY_COMPOSE_PRIVATE=''
$env:ORBWIN_QUALIFY_DEVCONTAINER_COMPOSE_ONLY='1'
node scripts/qualify-private-clients.mjs
```

These values belong to the qualification process only. Root must confirm its current lane and staged alias/hash before running. Existing successful receipts continue to cover skipped journeys; this new receipt must state its narrower dedicated scope rather than imply they were replayed.
