# Private Docker settings recovery interaction

2026-10-26. Source implementation; installed/native UI qualification remains open.

`client_recovery.rs` owns one review state: vacant, preparing, ready, then
restoring or closing, then vacant. Preparing and restoring use detached workers so losing an invoke
future cannot drop the native owner midway through creation or publication.
Blocking native work and checked release run outside the async reactor. The
renderer receives a random 256-bit epoch, fixed consequence text and expiry;
it never supplies a path, config body, process identity or recovery proof.

A ready review retains the actual native `WriterRecovery`. Restore atomically
takes that owner and marks restoring before scheduling work; duplicate, expired
or stale epochs cannot consume it. Cancel takes only the matching ready review.
An old cancel or expiry cannot interfere with restoring or a newer review.
Expiry uses a weak state reference, and cancel/restore abort the timer. Thus
repeated canceled reviews cannot accumulate five-minute timer tasks or create
an owner/state reference cycle. Provider expiry and native revalidation remain
authoritative even when the UI timer is delayed. Native release errors remain
errors; a successful cancel does not clear recovery records. Closing retains
admission until checked release completes, so a new review cannot race an old
controller close. No pending write is a harmless status with no epoch.

Renderer expiry also keeps competing settings actions disabled until the native
cancel/release promise settles, reports a checked-release failure, and returns
keyboard focus to review. It does not announce closure before that release.
The backend also retains a failed-release status, including after native expiry,
so a delayed renderer cancel or another review receives the release failure.
It does not turn a failed native close into an idle review slot; restart guidance
applies to OrbWin's process, without restarting workloads.

Registry settings expose an explicit review and apply step, fixed consequence
text, cancel (including during preparation), progress, expiry and result states.
The apply control receives focus after review; completion returns focus to review.
Errors are live-announced. While a review is held, this
page disables sign-in and forgetting entries. Leaving the page cancels a ready
review; a late review reply is also closed. Applying recovery continues in the
backend if the page closes. Password input is never passed to recovery. Saved
credentials may have changed before the interruption; recovery cannot reverse
remote authentication or Windows vault effects and never retries them.

`orbwin-cli clients recover` obtains the same native review and prints its fixed
summary, then checked-closes it. `clients recover --apply-reviewed` explicitly
requests a fresh review and its apply action in one process, then checked-closes
the returned config guard. Neither command requires a running/healthy engine
identity. `--restore-original` remains available for original-baseline recovery
or its completed cleanup; it refuses a local settings transaction with no saved
original. Static PowerShell completion is opt-in and performs no engine request.

The separate local `config-replace` provider offers KEEP CURRENT consequences:
validate the actual retained current settings, discard the exact unpublished
stage, clear its local intent last, then refresh managed tool routing in a new
transaction under the same controller. It never claims to restore an original
that this transaction did not preserve. Go and Rust accept their exact three-
and four-field producer records; the four-field record binds current content
to the previous or intended digest. The separate C5 native fixture now passes
both cross-client directions after a real publication sharing conflict, eight
phases in 57.675 seconds. It verifies exact stage/intent cleanup, preserved raw
auth/unknown values, current selector refresh and healthy reopen. Its receipt is
`artifacts/qualification/p2-local-replace-cross-native-v1-20261026/candidate-1/root-native-receipt.json`.
No Docker/helper/plugin, authentication, vault, engine, WSL or UI operation
occurred in that fixture.

Verification: three owner-lifetime tests prove stale cancel/expiry cannot take the
current owner or interrupt restoring, restore is one-use, and cancel releases
exactly once and aborts its timer. TypeScript and the production frontend build
pass. The current `TabExpansion2` passes thirteen positive and five negative
cases, including both command levels and the generic apply action. These
interaction checks do not establish power-loss durability, desktop
focus/keyboard behavior or installed packaging.
