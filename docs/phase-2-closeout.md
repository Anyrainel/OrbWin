# Phase 2 source checkpoint and remaining delivery

2026-11-07. Phase 2 is **incomplete**. This checkpoint preserves the developed
source and reviewed proposals for remote review; it is not a release or installed
qualification result. The accepted [scope](phase-2-scope.md) is unchanged.

## Delivered in the active source

The selected dependency pipeline integrates the backend Winio 0.6.4 adaptation,
SDK/LAN Winio 0.6.2 close adaptation, and selected Lima lifecycle changes. Versions,
source offers, actual compiler contexts and recipe hashes remain separately bound.
Backend canonical patch set 0023 remains selected. The integration envelope records
1,227 files, including 50 ignored generated schema/non-Windows icon files. The
publication source record covers the remaining 1,177 tracked inputs. The wider checkpoint also includes the Phase 2 frontend, SDK, CLI,
workflow tests, build scripts and existing implementation records developed since
Phase 1; their historical evidence retains its original limits.

Checks completed for the integrated dependency source: 92 Node source tests,
four isolated Rust provenance tests, actual application release/test compilation
and Clippy (67 warnings), backend compiler-only build, and all three SDK/LAN
compiler-only builds. Effective module/file closure matches before and after each
Go build. Before publication, 14 files were normalized from CRLF to the Git-mandated LF
representation and an authored test trailing blank was trimmed. The 92 Node
tests and application compilation/Clippy passed again on those bytes. Pinned
upstream license/security whitespace remains unchanged.
The frontend TypeScript/Vite production build also passes; Vite reports
chunk-size and mixed import warnings. None of these checks ran the product.
The backend compiler fixture excludes the embedded distribution; SDK fixtures
exclude cold-overlay/UAC resource packaging. The full native and installed matrix
has not passed for this source epoch.

## Preserved development proposals

[Pending source proposals](../proposals/phase2/README.md) contain portable patches,
full parent/target manifests and reconstruction evidence for the initial guest
scope and mandatory authenticated guest entry/packaging. They are independent
components, not a ready-to-apply ordered series. They remain outside the active
upstream lock because runtime delivery and recovery are incomplete. The initial
reader intentionally refuses mutations without genuine authority.

Guest source review corrected idle-worker cancellation and Go deadlock retention.
Initial scope review corrected borrowed archive buffers, the 16 KiB I/O boundary,
closed two-hardlink versus generic single-link policies, public Windows code
policy, and an unreachable generic system-directory walker. These are reviewed
source corrections, not observations of successful Windows import or guest boot.
The scope still requires genuine native qualification, including generated VHD
security/sharing behavior and disk resource budgets.

The paired setup-parent association is preserved separately with the reviewed
pre-capture deadline correction. It authenticates an
actual original inherited parent/transport generation; it does not transfer a Go
writer scope, bless protocol data, or relax the early guest-mutation guard.

## Remaining implementation ownership

| Workstream | Owner | Required result |
|---|---|---|
| Private hostagent, configured driver and guest bootstrap | Dedicated “Phase 2: private guest bootstrap and driver delivery” chat | Actual original launch/purpose-channel association to the same native scope; fixed bootstrap/network/SSH authentication before provisioning; final role-key delivery only after original provision join; mandatory authenticated endpoints and complete old-route removal. |
| Older-runtime preparation, NoGuest and recovery | Dedicated “Phase 2: older-runtime preparation and recovery” chat | Useful non-destructive preparation/resume for unbound older helpers; complete current PID-role closure; genuine guest/worker settlement and durable checkpoint/completion; preserved old data plus independent-copy recovery where the original owner was lost. |
| Integration and final source publication | Original Phase 2 chat | Compose exact deltas, resolve shared source changes, regenerate one coherent canonical/recipe/source-offer epoch, and validate the combined callers. |
| Native and installed acceptance | Paused by user | Resume only after explicit user instruction; qualify initial setup, errors/cancellation, restart/upgrade/recovery, coexistence, Docker/BuildKit/Testcontainers, the expanded convenience flows and UI/accessibility. |

The pinned older helper may legitimately run without an inherited authority lock
and can leave unjoined WSL commands. PID disappearance, a lock acquisition,
namespace absence, a checkbox or CLI exit zero cannot alone certify retirement.
Manual recovery is allowed by D26, but it must be a concrete supported path with
real state verification, not a refusal described as recovery. No global WSL
shutdown, silent reset, unregister, other-engine stop or data deletion is an
accepted shortcut.

## Native execution constraint

The latest explicit instruction remains **“Keep native runs paused.”** Splitting
work and pushing source does not resume native execution. The follow-up chats
work on isolated source and pure/compiler checks. No current product, backend,
SDK, LAN helper, guest, WSL, registry or native diagnostic run is authorized by
this checkpoint. Old retained actors and failed evidence remain untouched.

The requested few-hour closeout is organized around concrete source deliverables
and one integration pass. Phase 2 can be declared complete only after its
remaining development and acceptance gates actually pass. Current implementation
pins and evidence remain in [the implementation record](phase-2-implementation.md).
