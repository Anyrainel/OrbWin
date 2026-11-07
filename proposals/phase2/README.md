# Phase2 pending source proposals

These components preserve dormant reviewed development source for a remote clone. They are **not selected, activated, native-qualified, or an ordered patch series**. Existing upstream/build locks are unchanged. Do not apply all patches sequentially: the first two rdd components independently target canonical23; setup-parent requires its separately declared combined1240-file canonical parent.

| Component | Exact input | Output projection | Changed paths |
|---|---|---|---:|
| native-initial-scope3/rdd.patch | canonical23 raw c75, text LF, four exclusions | Scope3 with exact c75 Into2 callers restored, same projection |39|
| guest-entry/rdd.patch | same canonical23 projection | mandatory guest Go1232, same projection |25|
| guest-entry/repo-inputs/repo.patch | declared before-repo8, byte exact | declared repo13, byte exact |9|
| setup-parent-source2/go/rdd.patch | combined Go1244 projection, LF/four exclusions | Go1248 projection, same exclusions |9|
| setup-parent-source2/desktop/repo.patch | declared two Rust files, LF | same two files with actual parent transport association |2|

Each patch has before/after complete source manifests and a delta manifest with per-path raw/projected hashes. rdd patch paths are relative to rdd/. The repository patch paths are relative to repository root and concern ONLY its declared source subset. Apply only after every required parent hash matches; later integration must resolve overlaps explicitly, recompute coherent source/build/lock closure and obtain review. No generic patch-series application or current checkout compatibility is claimed.

The four omitted historical paths are pkg/guestagent/lima-guestagent.amd64.gz, upgrade_PROGRESS.md, upgrade_SDK_TRANSITION.md and upgrade_boot_fence_snapshot.json. No archive, PE, package output, private key or ignored artifact tree is exported. Existing copyrights/licenses/tests remain in patch content and unchanged parent source; dependency/license trees must remain with the parent checkout. Full ignored evidence is represented by compact source/check/peer hashes, not copied wholesale.

The pure Python exporter checks all frozen raw source pins, projects text LF, restores the two original canonical23 Into2 callers in Scope3, constructs unified patches, independently replays every hunk, and checks exact reconstructed target bytes in fresh owned temporary source directories. export-verification.json records successful reconstruction. This is source-data verification only, not execution or semantic acceptance. The exporter itself requires the historical local frozen inputs and is retained as provenance; no product/build executable is invoked.

See each component's NOTES.md for required gates and intentional refusals. Native execution remains paused. Initial negative-state refusal, genuine private-parent association, physical older-role preparation, original guest delivery/authentication/driver joins, NoGuest/checkpoint/completion, archive rebuild and coherent recipe/offer reseal remain pending development. Publishing these proposals does not finish Phase2.

The setup-parent successor includes its finite review pass and retains the previous blocked pre-capture result. Its association remains distinct from native guest authority. See [the closeout](../../docs/phase-2-closeout.md) for active follow-up ownership.
