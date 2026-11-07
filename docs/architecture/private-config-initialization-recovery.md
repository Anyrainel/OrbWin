# Interrupted private settings initialization

Agreed source contract, 2026-10-26. Rust and Go now have separate native providers
behind the existing opaque recovery API. Go's parser/provider source passes
focused tests, vet and a targeted race run; Rust's focused parser/config tests
pass. The separately reviewed candidate 4 native matrix now passes all 29 phases
across 14 fresh namespaces, as bounded below. Earlier failures remain recorded.
Legacy import is a separate protocol.

The initializer writes/flushed/closes the fixed `client.create-intent` before
creating `client` and its exclusive `controller.lock` with CREATE_NEW. It writes
only root-bound metadata and empty Docker defaults before removing that marker.
It returns no usable guard or child admission while the marker exists.

Recovery uses a separate capability, because ordinary guards require complete
metadata and settings. Its initial marker read permits sharing so a review does
not block a live creator's cleanup. Before creating anything, it recognizes only
the fixed marker or its exact byte prefix and an exact private tree:

- `client`: empty `controller.lock`, `client.init`, and/or `docker`.
- `docker`: only `config.json`.

Absent entries are allowed. Unknown entries, credentials, custom fields,
reparse points, multiple file links, foreign owners or malformed records refuse
recovery and retain the data. Full metadata has exactly seven known fields,
including the actual native client-root identity and current SID. Defaults have
exactly empty `auths`, `credsStore: "orbwin"`, and one canonical owned selector.
The historical selector is recognized without following or executing it.

Partial metadata/defaults must be provable compact producer prefixes over those
exact fields and values. Field-order matching is bounded to seven fields; only
the selector's 64 lowercase nonzero hash bytes vary. Explicit Rust and Go string
encodings are recognized. Generic JSON EOF, arbitrary whitespace/escape guesses,
and prefixes which cannot extend to a valid producer record are refused.

Review may create only the missing client directory and exclusive controller
gate with OPEN_IF. A recovery-created directory or gate causes the old creator's
CREATE_NEW to fail before it can write settings. An existing busy controller
refuses review. After claiming it, recovery captures the same marker immutably,
checks its native identity/descriptor/bytes against preflight, and repeats the
bounded inventory. A still-active marker writer causes refusal. Directory
enumeration uses dedicated LIST_DIRECTORY handles bound to the held identities.

Explicit apply rechecks all native owners, retained bytes and inventory, current
role selection, and the fixed legacy-settings denial probe. Under the same
controller and unchanged marker, it creates missing directories and completes
only recognized metadata/default records using CREATE_NEW, flush, readback and
checked writer closure. Exact recognized partial records can be deleted and
recreated. The original marker stays present throughout, even if partial; a cut
can therefore leave a partial marker with complete defaults and remain fenced.

After full metadata and current empty defaults are revalidated, all writable
handles are positively closed and legacy absence is checked again. Exact marker
deletion is last. Only then may the same retained capabilities become an ordinary
healthy guard. No credential process or authentication is replayed. Cancel and
expiry preserve records and check resource release; unknown closure retains the
controller in quarantine. Missing-marker partial trees, public legacy import,
local-replace EOF and interrupted restore-target version changes remain separate
acceptance gaps.

The preserved C6 candidate2 run obtained an actual Go review after the Rust empty
marker cut, then unexpectedly admitted DELETE access to its held client root.
It stopped before settings completion and performed no actual rename/deletion.
The root receipt is under
`artifacts/qualification/p2-fresh-init-native-v1-20261026/candidate-2/`.
Microsoft's [sharing algorithm](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-fsa/8c0e3f4f-0729-49f4-a14d-7f7add593819)
does not count metadata-only opens as participating share-access owners. Merely
omitting FILE_SHARE_DELETE was insufficient. The proposed minimal fix adds
FILE_TRAVERSE to retained directory handles, preserving dedicated listing rights;
actual no-DELETE sharing requires a new frozen native qualification. All prior
nonces/results remain unchanged, with no external cleanup or retry.

The new C6 candidate3 native run confirms that a Go-held client root now refuses
DELETE with SHARING_VIOLATION. Its first apply then refused because the Go
selector recognizer embedded the product namespace inside a raw long literal,
which the fixture's exact namespace projection did not change. The isolated
fixture selector was therefore rejected before init/default creation. The source
now uses one fixed private-directory constant (same product bytes, no caller
parameter), and the next candidate will verify that constant's projected value
before any native provider call. Candidate3 and its failed nonce remain retained;
this partial result does not qualify the complete initialization flow.

Candidate 4 then passed all 29 planned phases in 199.275 seconds:
`native-run-50e3035e-a7e8-4b04-a588-678efe32e75c`. Root verified 499 frozen files,
compiler pins and namespace-only projections before a separate native dispatch.
The matrix includes 14 actual creator/repair process-loss cuts, empty/partial/full
markers and records, repair interrupted at init/default publication, checked
review cancellation, actual marker-last deletion followed by no-review/healthy
admission, live creator preemption and busy-owner refusal. It records95 actual
sharing refusals, including 42 across held Docker/credential/runtime directories.
Go and Rust complete and reopen healthy settings from the same controller/root
capabilities. Selected roles were copied/read only; no child, engine, WSL, UI,
vault or administrator operation occurred.

Use candidate4's `root-native-receipt-v2.json`, which preserves the original ISO
timestamps and milliseconds; the first derived receipt's PowerShell date parsing
is superseded. The native matrix receipt/results themselves are unchanged. This
is bounded process-interruption and sharing evidence, not power-loss/media
durability, unknown-close faults, foreign-data/legacy import, local-replace EOF,
restore-target version succession or complete installed product acceptance.
