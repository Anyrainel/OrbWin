# Durable LAN review admission

The SDK persists an exact `lan-review-<reviewID>.json` record in its opaque native
managed root before publishing any successful activation, removal, settlement or
prior-owner cleanup review. A caller reply, renderer storage and elevated helper
history do not substitute for this receipt. The record binds schema, global
installation identity, daemon, original SDK runtime, purpose, full review digest,
scope/publication, revision, lifetime and immutable prior-owner cleanup mapping.
Its original Stage identity is never rebound to a current process.

The only initial state is `issued`, sequence 1, without operation identity. Before
any ingress Stop, namespace acquisition/release, helper review, elevation or worker
dispatch, the same SDK atomically changes that exact still-issued record to
`consumed`, sequence 2, with its newly generated operation ID. Unknown append,
replace or checked-close completion dispatches nothing and exposes no provisional
operation identity. Consumed records never replay; losing the SDK before helper
history exists still leaves a possible-dispatch fence.

A new exclusive native managed owner may read and positively revoke an exact
old-runtime issued record by durably writing `revoked`, sequence 2. Only after the
native transaction and checked close succeed may reconciliation return
`outcome: review-revoked`, the exact `reviewId`, `reviewSha256`, original
`runtimeInstance`, `managedSequence: 2`, and fresh `observedAt`. This DTO has no
operation ID or lease SHA. Missing, partial, foreign, current-runtime, consumed or
unknown-origin records cannot prove nondispatch. A previously saved revoked
record can be freshly observed by a later exclusive owner. This outcome differs
from actor-only `revoked` and protected firewall `retired` sequence 5.

The actor admits one bounded managed-journal operation at a time, with five-second
IO deadlines; it does not wait synchronously for native IO. General managed-state
commands and tick IO are fenced while admission or recovery is pending. Queries
and exact operation cancellation remain responsive. Durable consume survives
caller reply loss and owns a bounded SDK continuation. Generation construction
runs off the actor. Its worker retains every late attachment until the actor
acknowledges transfer; buffered events alone do not transfer ownership. Cancellation
or SDK exit positively joins LAN, HTTP host and backend owners before release.

Each receipt is limited to 64 KiB and counts against the SDK managed root's shared
2048-entry and aggregate-byte bounds. Records are not silently evicted. The
volatile 4096-review denial-history bound does not promise 4096 durable issuances.
Admission failures remain explicit; there is no user-home fallback or native
permission repair.

Pure tests cover reply-loss/crash boundaries, consumed-before-effects for create
and removal, scope/digest/owner corruption, unknown mutation/close, exact
cross-owner revocation, unreceived generation ownership and delayed joins. These
source tests do not qualify native review persistence or administrative behavior.
