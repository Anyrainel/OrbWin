# Private writer recovery fault qualification plan

Recorded 2026-10-26. Source plan; no execution authorized by this file.

The actual schema 3 private owner and process producer must create every journal,
native identity, descriptor and execution observation. A fixture must not edit a
pending record afterward, create a fake process proof, infer quiescence from a
PID/path/digest or alter production admission policy. Whole modules are projected
with namespace literals only; fresh fixed nonces isolate every case.

The proposed debug-only fault seam has a closed enum, no environment or IPC
control and no caller path/body constructor. A prefix cut runs inside the actual
native CREATE_NEW record writer: write a proper prefix, flush, bounded readback
through that same native handle, then exit the creator before the producer can
advance. A cleanup cut exits only after exact native ID/SD delete, checked close
and observed absence. Release builds exclude the seam. Independent root review
must approve its call graph, source closure, fixed exit marker and all bounds.

| Case | Actual interruption | Review authority | Finite phases |
| --- | --- | --- | --- |
| EOF prepare | partial prepare before original | actual strict owner config, init, other markers absent | 2 |
| EOF original | partial original after durable prepare | unchanged exact native base, proper prefix copy | 2 |
| EOF intent | partial intent after durable original | schema 3 prepare, unchanged exact native base | 2 |
| EOF birth | suspended actual root, partial birth before first resume | schema 3 source protocol and unchanged native base | 2 |
| EOF recovery intent | partial recovery record before source/stage change | independently validated source journals and stopped/no-resume | 3 |
| EOF config stage | partial owner stage under full recovery intent | exact stage cap and original/source journals | 3 |
| After intent cleanup | intent absent, original retained | recovery journal or complete birth metadata | 3 |
| After original cleanup | original absent after intent | strict owner current target and recovery journal | 3 |
| After prepare cleanup | only recovery journal on no-birth flow | exact owner target and embedded source metadata | 3 |
| Birth-only cleanup | full birth after recovery intent removed | actual stopped generation, strict owner current target | 2 |
| Completed cleanup | last record removed before in-memory health update | ordinary native provenance/config validation | 3 |

Phase zero creates its own baseline through the actual private owner, then arms
the selected cut. Cold phases run in separate processes so exclusive-controller
reacquisition is real. A preparatory no-birth loan exits without Rust Drop; the
OS releases its actual handles. Recovery phases consume the actual retained
opaque review capability. Each phase has a finite watchdog; no old nonce is
retried, reset, swept or externally cleaned.

EOF birth creates one suspended root that has not executed user code; the actual
creation-time kill-on-close job owns creator-loss cleanup. Birth-only cleanup
instead uses one finite inert root, positively cancelled/waited/retired before
review. No real Docker/helper, vault, registry, engine, WSL, admin or UI is used.
Record/config bytes and role-copy totals need exact budgets in the frozen recipe.

This models process interruption after completed synchronous operations. It
cannot establish sudden-power-loss durability or diagnose arbitrary corruption.
Missing/malformed state outside the protocol remains retained/refused.

The separate stopped-slot follow-up must cover nonwriter native creation that
actually created a root, then positively terminated/joined/closed it on failure.
The producer should expose a sealed one-use generic retirement observation for
read/session admission release, distinct from writer-bound storage proof. Unknown
native cleanup still retains the slot. That follow-up requires its own frozen
source candidate after these cutpoint changes stabilize.
