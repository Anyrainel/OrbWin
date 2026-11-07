# Explicit volume size refresh

2026-10-25. P2-STO capacity addition; development implementation, not installed
acceptance or a portable backup transaction.

The volume Info inspector offers Refresh volume size after its existing inspect
read supplies creation metadata. No size scan runs when inventory is refreshed,
and no scan starts merely by opening a volume. Unsupported/missing metadata and
errors are explicit. A measured zero differs from an unmeasured/unknown result.
Display the exact engine byte count, container-reference count and timestamp.
References include stopped containers. Errors/cancellation retain a previous
successful observation with its timestamp; they do not reset it to zero.

The thin native request targets the verified owned engine through the existing
SDK operation adapter, as a read, without the maintenance image or a helper. The
SDK uses the pinned upstream Moby `DiskUsage` API with only `Volumes` and
`Verbose` enabled. Moby scans engine-wide volume usage; the API has no selected
volume scan filter. Return only the selected volume, with a 30-second SDK context
and 40-second native client deadline. Cancellation closes the SDK client; it does
not promise to interrupt scan work already running in the daemon.

The dedicated HTTP transport fixes the owned pipe and allows only GET/HEAD ping,
Info, the exact selected volume's inspect path and volume-only verbose disk
accounting. Redirects, mutation methods, other resources and a broader df query
refuse before transport dispatch. Each metadata body is buffered to at most
4 MiB before SDK decoding, and the original response is closed on both success
and failure. At most 4096 reported volumes are accepted; no truncated accounting
result is presented. The base transport has one connection and bounded headers.
Closing the client also closes idle connections. No container, mount, file,
lease, receipt, pause or engine lifecycle change is part of measurement.

Verify daemon identity/OS before and after measurement. Inspect the selected
ordinary local volume without driver options before and after. Match its reviewed
creation timestamp before scanning, retain its full metadata tuple, and reject
replacement/missing observations, duplicate selected names, negative/unavailable
usage, unsafe integer sizes and invalid reference counts. Docker provides no
immutable volume ID: same-name replacement with indistinguishable metadata and
timestamp resolution is not provably detectable. This is a read-only observation,
not authorization for a later mutation. Writers are neither paused nor locked;
the reported size is not an atomic backup size or Windows allocation measure.

The React view owns one cancellable read until native settlement and unmounts
when the selected name/creation timestamp changes. Its stdout and diagnostic
buffers are bounded separately. Aborted/retired replies cannot update state.
Explicit Cancel retains previous results; refresh rechecks current metadata.

Initial evidence: Go fault cases cover daemon replacement, selection replacement,
in-use and unused references, missing/duplicate/unknown counters, unsupported
driver options, response bounds and context cancellation. Read-only request
restrictions have a transport test. The full Go suite and vet, scoped race test,
frontend build and native operation argument checks passed.

`p2-volume-size-live-20261025.json` records an isolated networkless fixture:
131072-byte payload and sentinel, nonzero size while attached, one reference
after stopping its container, zero after removing that container, identical size
and file hashes, stale-selection refusal, exact pre-test inventory restored and
fixture volume removed. `p2-volume-size-existing-20261025.json` measures one
existing empty volume as zero bytes/zero references. The first PowerShell test
converted the timestamp to DateTime before invoking the CLI and was refused;
its diagnostic is retained. The corrected JSON-preserving caller passed.

`p2-volume-size-native-20261025.json` and log confirm actual native SDK dispatch
for the existing empty volume and stale-selection refusal, both read-only. Native
UI review at 1282×882 showed the unmeasured state, explicit refresh, successful
zero-byte result, timestamp and zero references. The attempted cancel click
arrived after the read had completed; UI cancellation is not established by that
attempt. The inspector loads its size controls separately; the main UI bundle
remained below the existing 500-kB warning threshold after this addition.

Installed qualification, scale/disk pressure, cancellation UI and same-name
replacement interaction remain separately qualified. Engine work after
request cancellation and transient scan costs must remain visible limitations;
no additional reliability or physical allocation claim is made.
