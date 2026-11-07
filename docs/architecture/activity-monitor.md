# Activity monitor

Updated 2026-10-25. Original Windows interaction; no claim of observed OrbStack
sampling behavior. P2-MON remains under qualification.

The Containers list opens an engine container activity dialog. A selected
container's Stats tab uses the same component. Both read the explicitly targeted
owned Docker endpoint through the existing operation adapter. Engine-wide totals
include running Kubernetes containers regardless of the inventory display filter.
These are sums of workload counters, not measurements of the guest VM's own CPU,
memory or filesystem activity. VM-wide telemetry remains an acceptance question.

Each mounted view owns at most one client read. Reads have the existing adapter
deadline, cancellation, engine identity checks and two-MiB output budget. No
mutation, inventory refresh, backend lifecycle operation or persistent settings
write is part of sampling. Closing the view cancels its client and discards its
history. Changing the selected target remounts the owner. Cancellation does not
stop a container or daemon. Intervals are 3, 5, 10 or 30 seconds, default 5;
busy ticks are skipped and hidden documents do not start periodic reads.
Cancellation pauses live sampling and keeps the read button disabled until the
native promise settles. Late successful replies after abort are discarded.
`ActivitySampler` owns admission, timers and disposal; a disposed view emits no
later state events. A sampling error affects only this view's status/history.

Retain at most 60 successful samples. Charts use sample order, not a uniform
wall-clock axis. Errors retain previous charts with a visible last-success time.
Invalid/missing samples never become zero-valued samples. An empty successful
engine read is zero running containers. A selected read must return exactly one
full identity matching the selection; missing/replaced identities refuse. Zero
reported processes in a selected sample produces a stopped/transitional warning.
Selected-container UI state transitions still require qualification.

CPU is Docker's multicore percentage and may exceed 100%. Memory sums current
byte usage, never percentages of separate container limits. Network and block I/O
are rounded cumulative CLI counters, not calculated rates. Membership changes and
restarts can lower totals. Parse only known SI/IEC units and finite bounded
values; reject duplicate identities, malformed rows and oversized observations.
Charts expose latest/peak values as accessible text. The modal supports native
keyboard controls, focus containment, Escape and return to its trigger.

Evidence so far:

- `p2-activity-tests-20261025.log`: 82 JavaScript tests passed; byte/unit, invalid,
  duplicate, replaced and zero-process observations included.
- Rust: 51 ordinary tests passed, including explicit read-only stats argument
  plans. `p2-activity-native-20261025.log` and JSON: actual native engine-wide and
  exact selected running-container reads passed without a workload mutation.
- Native development window: real 11-container totals, six charts; keyboard
  pause, interval change from 5 to 10 seconds, Escape/focus return observed.
  Paused view retained five samples across subsequent observations.
- `p2-activity-samples-20261025.json`: five sequential direct CLI reads, 1891–1958
  ms and 4114–4117 output bytes each. This measures read latency only; it does not
  establish renderer, client or VM CPU/memory sampling overhead.
- `p2-activity-postcheck-20261025.json`: all three daemon identities and private,
  project, Docker and Kubernetes configuration hashes retained, no elevation.
- `p2-activity-targets-20261025.json`: isolated networkless, unmounted sleeping
  container; running sample accepted, stopped sample refused, removed identity
  failed and same-name replacement could not attach to the old identity. Exact
  pre-test container inventory restored. This exercises the actual CLI and model;
  UI transitions and adapter cancellation still need their own qualification.
  The first fixture guard incorrectly expected a config digest in container
  `Image`; this engine reports the immutable root digest. That attempt refused
  before start. Exact ID/name/label/image/state were checked before cleanup;
  initial refusal evidence is retained separately. The corrected proof passed.
- `p2-activity-client-cost-20261025.json`: three actual Windows CLI processes
  consumed 31.25–62.5 ms CPU each, with 1845–1943 ms wall time. Peak working set
  was unavailable after exit (null). No inference about renderer or guest CPU
  overhead, or working-set size, follows from these measurements.

Polling follow-up: `p2-activity-polling-tests-20261025.log` contains 86 passing
JavaScript tests. New deterministic clock/deferred-client cases cover hidden and
busy ticks, interval replacement, cancel acknowledgement, late success/failure,
disposal, explicit manual reads while paused and next-read recovery. The native
admission cancellation test (`p2-activity-cancel-native-20261025.json` and log)
observed the adapter's precise pre-dispatch cancellation error, an empty registry
and a successful subsequent engine stats read. Its initial test expected only an
interrupted outcome; that test expectation was corrected and the initial log
retained. This does not establish cancellation during CLI collection. Revised
native UI showed real totals, pause and manual read; attempted cancel clicks
arrived after collection had completed, so that interaction remains unverified.

Remaining P2-MON gates include renderer/engine CPU and memory overhead,
stopped/recreated UI transitions, fault/cancellation and hidden-view behavior, scale, installed UI and
acceptance of the engine telemetry boundary. This implementation does not close
P2-MON or substitute container totals for a full VM telemetry requirement.
