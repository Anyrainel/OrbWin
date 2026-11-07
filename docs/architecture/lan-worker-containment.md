# LAN worker containment and actual manager boundary

Source investigation and proposal, 2026-10-25. No listener, helper, process,
firewall operation, SDK registration, source pin, or proxy candidate is changed.
The disabled BLOCK CIM fixture remains compiled and unrun. Existing planner and
provider read qualifications do not authorize LAN activation.

## Actual available owner

The selected `upstream.lock.json` commit is
`af6511ad4e065e24e7258f575a0fc090c10cdbda` with twelve registered adaptation
patches. In the inspected source, `rdd/pkg/service/cmd/service.go:257` starts
`os.Executable()` with `service serve` using `exec.CommandContext` and
`process.SetGroup`. `pkg/util/process/process_windows.go:24` sets
CREATE_NEW_PROCESS_GROUP. This is a background process under its launching token,
not registration with Windows SCM, LocalSystem, or automatic elevation. The
controller-runtime manager manages backend controllers; it is not a privileged
Windows firewall broker. Scoped searches for SCM/CreateService/svc.Run/RunAs in
the selected rdd Go source found no such implementation. No installed service
inventory was queried, so this is a source conclusion about this integration.

The available UI-independent product owner is
`backend-tools/session/localnet.ServiceRuntime`. `services.go` opens the explicit
owned engine client and exclusive `NamespaceLedger`, then starts the runtime.
`localnet/lease_windows.go` retains a no-share CreateFile handle on
`controller.lock`. `ServiceRuntime` serializes changes through one actor and owns
helpers/routes, never workload or rdd lifecycle. Rust
`src-tauri/src/local_services.rs:259` starts `local-services-serve` with a detached
ordinary-user process and deliberately drops the child handle after readiness.
The UI can close without stopping that owner. Reuse this owner and its existing
shutdown/reconciliation discipline, not a second LAN daemon or engine manager.

Current `localnet/ipc_windows.go` bounds frames, concurrency and deadlines and
grants the current user pipe access. It does not verify peer PID, creation time,
image identity or an administrative token. Its current ACL therefore cannot be
promoted into administrative authentication. User-home ledgers and snapshot
hashes are likewise not privileged ownership receipts. The Linux trust helper
and current-user Windows trust store do not supply a Windows administrator
boundary. A narrow attended Windows helper still has to be implemented and
qualified; no existing privileged service can be assumed.

## Fixed proposed integration paths

The next isolated pure boundary belongs in new
`backend-tools/session/internal/lanlease/lease.go` and `lease_test.go`, with no
native adapters, entrypoint, goroutine, process, disk or socket operation. It
would consume copied native-observation facts and existing immutable LAN plans,
not turn caller booleans or JSON into OS authentication. Native adapter and
packaging decisions remain separate reviewed work. Existing ServiceRuntime,
lanfirewall protocol, lanprovider read bridge and SDK attestation lists remain
unchanged by this increment.

Future integration would add a LAN-owned child set to the existing ServiceRuntime
actor, under its existing exclusive installation/daemon owner. It observes the
exact authoritative publication tuple (daemon, full container ID, StartedAt,
publication generation, host/container port and protocol), never a mutable
Windows localhost destination. It reuses exact target namespace transport and
source validation. The current helper supports TCP; UDP requires its separately
bounded datagram ABI. Neither a Docker exposed-port declaration nor a refreshed
container name substitutes for an actual active published-port generation.

IP Helper GUID/LUID/address and NLM profile must independently match current
Private/Domain consent at each transition and while active. The existing
`localnet.GuardLANExposure` and peer-CIDR predicate remain the planning guard;
Public, IPv6, wildcard interfaces/ports and changed publication generations cannot
be silently substituted. The pure lease model must preserve those checks and
explicitly close on unknown observations.

To avoid a future import cycle, `internal/lanlease` must not import `localnet`:
the ServiceRuntime adapter will apply the existing LAN planner/guard, then pass
copied immutable publication/scope values. The isolated model can reuse
`lanfirewall.Scope.Validate` and `CallerIdentity`; it must not silently reinterpret
the frozen helper v1 review digest or deterministic rule-name contract. A fresh
lease/path binding is a proposed additional review version, requiring explicit
integration review.

## One manager, one bounded privileged transaction, one child

The manager remains ordinary-user. An explicitly attended, fixed-operation
Windows helper performs only the reviewed privileged setup/rule transaction and
retirement; it must not become an always-on privileged listener or start rdd,
WSL or workloads. Reuse the fixed request/scope and protected intent/sequence
contract already designed in `lanfirewall`; do not invent an HMAC/token authority
based on a user-writable secret. Native pipe/process identity is the trust input.

A fresh lease stages an exact embedded/pinned worker asset at a never-reused path
under a helper-created, administrator/SYSTEM-writable root, conceptually
`ProgramData/OrbWin/LAN/<installation>/<fresh lease>/orbwin-session.exe`.
Only read/execute access is granted to the original ordinary user. The helper
resolves the fixed root itself; no caller program path, home path, command,
argument vector or firewall expression is accepted. It holds and verifies
reparse-free directory/file descriptors, byte hash, file ID and volume identity.
The protected lease/tombstone survives worker exit and update; the path is never
reused, re-authorized, or executed by cleanup. This is a future packaging gate,
not something provided by the current user-writable SDK cache.

The original manager's native PID, creation FILETIME, token SID, image/file
identity and held live process handle bind that lease. Alternate-administrator
elevation must preserve the original manager SID separately from the helper's
elevated identity. The manager launches the exact ordinary-user child with a
default-closed control channel and restricted handle inheritance. The native
CreateProcess result retains the child process handle/PID/creation identity;
native pipe client/server PID queries must agree with these held process handles
and the protected lease. Worker receipt fields, argv parent PIDs, a generic
Toolhelp PPID snapshot, pipe names and the current user ACL are not proof.

Here “parent binding” means this mutual native manager/owned-child channel and
held creation identities. Do not infer it from an OS PPID string, or introduce
undocumented PROCESS_BASIC_INFORMATION offsets. A reopened worker with a new
process creation identity or a newly launched manager must not inherit the old
grant even when executable bytes, SID and PID happen to match. An identical SID
is an OS principal, not protection against hostile same-user process injection
or duplication of handles; that stronger threat is not claimed by this model.

The existing one-shot helper wire schema does not yet express protected asset
staging followed by native child acknowledgement within one attended action.
That bounded stage/ack sequence needs an explicit reviewed extension rather than
silently changing the frozen v1 parser or requiring an assumed privileged daemon.
Both helper and worker remain closed on denial, timeout, parent loss or any
unsettled stage. A native adapter cannot mark these bindings verified merely
because the pure model accepts supplied observations.

## Activation, shutdown and conservative retirement

Before any bind, require current source/interface/profile validation, a single
live manager-owned lease, exact worker identity, acknowledged protected native
rule ownership and full typed filter readback. An unknown or duplicate Create
never authorizes activation. A future allow/enable provider path requires its
own qualification; the current disabled BLOCK fixture cannot provide that proof.
The raw worker must enforce the reviewed peer CIDRs for every TCP accept/UDP
datagram even if some other firewall rule is broader.

For normal stop, failure or drift, first revoke further ingress, close the
listener and all accepted streams/datagrams, and join their workers. Then stop
and wait for the exact child process handle; use a manager-owned kill-on-close
job with no inherited duplicate job handle once that native lifecycle is proven.
Only a positive close/join/process-exit acknowledgement permits a containment
claim. A timeout requesting closure is not proof that sockets or the process
have closed. Parent loss must make the worker close independently, and job
rundown is a second OS mechanism, not a substitute for cooperative stream join.

After ingress is positively closed, request exact conservative rule retirement
using the protected acknowledged full keys and complete typed state. Retain
foreign/changed/unknown rules and pending intents; never delete by name, prefix,
group or matching plan. This ordering avoids leaving a healthy listener running
while cleanup is uncertain. A late persistent allow could be dormant after
verified worker death because its never-reused exact program path cannot regain
a valid lease. That is containment, not deletion, provider CAS, cleanup success,
or proof of Windows/GPO admission behavior. Failure to verify worker exit remains
an explicit unknown-containment outcome.

Recovery must use the authoritative ServiceRuntime owner plus protected helper
records. It does not revive an old child/path or adopt an uncertain create after
a name scan. Fresh activation requires a new lease/path and user-reviewed scope.
Updates preserve the old tombstone and exact rule identity until conservative
retirement; they do not rewrite the old program path or erase evidence.

## Open gates

Native caller/child identity and handle-inheritance/job semantics, protected
asset/lease storage, bounded attended helper transport/stage review, source
publication observation, peer admission, UDP transport, actual allow/enable and
exclusive ownership, conservative cleanup, cancellation/late-create recovery,
installed packaging, GPO/firewall coexistence and real LAN traffic all remain
unimplemented or unqualified as applicable. WFP remains an unadopted alternative.
The next pure model can test wrong PID creation/SID/file binding, drift, stale
observations, no open on unknown rule state, tombstone replay, and refusal to
retire before positive closure. It cannot prove the native facts it is given.

## Implemented independent pure policy

`internal/lanlease/lease.go` now implements serial actor-owned transitions:
prepared → child-bound → rule-acknowledged → ready → closing → closed →
retire-pending → retired. Invalid rule acknowledgement enters blocked; drift
enters closing. Neither blocked nor a requested close is contained or eligible
for ingress. Only positive exact child/socket/flow/process closure enters closed.
Unknown deletion remains retire-pending. Full native-key and typed-snapshot hash
equality plus the retained ownership acknowledgement are required before the
retirement transition. This remains a model of native facts, not their source.

The policy copies client scopes, binds manager/child PID + creation time + SID +
image, pipe peer PIDs and worker path/file/volume/hash/alias, and compares exact
publication and interface/profile scope. Setup must finish within the reviewed
transaction lifetime. A healthy established lease can outlive that setup expiry,
but packet admission and eligibility expire after five seconds without a fresh
matching observation, including when no next poll arrives. The native worker
must independently close existing flows on expiry; this pure predicate does not
close sockets. Rule/removal acknowledgement requires both protected provenance
and a flushed acknowledgement, not merely terminal provider success.
Initial ingress and conservative retirement also require native full-rule/filter
readback within five seconds; an old retained snapshot is ownership history,
not a fresh observation. A current read may advance only its observation time
while every retained key/scope/typed-state/sequence value remains identical.

Lease ID/path and single-use transaction/nonce reservations are never freed,
including failed setup. A restored
tombstone is denial-only and cannot restore a listener or ownership capability.
The current pure index holds at most 4096 distinct leases and refuses additional
reservations rather than evict evidence. Production protected-store indexing or
archiving needs review before treating that bound as a product capacity choice.
No existing wire version, rule-name derivation, SDK entrypoint or attestation list
was changed.

## Minimal proposed v2 helper/child adapter

This is a proposed explicit version-2 stage sequence, **not implemented IPC**.
Keep the existing v1 parser and digest semantics intact. Reuse its typed base
request/scope and native identity requirements without treating its old
review-derived rule name as a per-lease identity.

1. **Stage request:** the existing ServiceRuntime actor submits version 2,
   transaction/expected sequence, immutable base review and exact publication
   tuple through the already designed fixed helper IPC boundary. The helper
   verifies the native original caller, derives the fixed root and embedded
   dedicated worker asset itself, generates a fresh lease ID, checks absence
   under its exclusive protected-store owner, flushes the reservation, and stages
   that exact worker. No caller program/root/store/command field exists. It
   computes a separate compound lease digest over the unchanged base request,
   publication, lease ID, native parent binding and actual staged file identity.
   A qualified v2 rule identity must bind that compound digest; do not silently
   feed it into v1 as though its review semantics were unchanged.
2. **Prepared reply:** includes the lease ID/digest and exact staged native file
   binding/sequence. The manager verifies the helper's native process handle,
   bounded reply and independently opened protected prepared record. Staging
   grants no listener authority. The elevated exact-scope review includes the
   actual native program path and user/network/port scope before rule dispatch.
3. **Closed child acknowledgement:** the ordinary manager starts the dedicated
   worker suspended, assigns the exclusively held non-breakaway kill-on-close
   job before resume, then receives its default-closed hello on the fixed local
   control pipe. Native client/server PID checks and held spawn/process handles
   bind the exact child incarnation and original manager. One child-ack frame
   reports lease digest/PID/creation time; the helper independently verifies
   native token/image/file identities and stores that exact child binding. A
   frame claim alone is insufficient. It accepts one child, not arbitrary
   process IDs or a second worker after failure.
4. **Commit reply:** only after separately qualified exclusive rule creation,
   full typed scope/ownership verification and any required enable transaction,
   the helper flushes the protected acknowledgement. Its reply supplies lease
   digest, sequence, scope/native-key/typed-state digests. The manager must verify
   that durable record and fresh native source/interface/profile facts before
   the pure policy can enter ready and the dedicated worker can bind. A lost,
   duplicate, unflushed or unknown result leaves the worker closed; no retry of
   uncertain creation or adoption by later name lookup is permitted.
5. **Retirement:** the manager first closes/joins the exact owned child and
   streams/datagrams, retaining its process creation binding and tombstone.
   A distinct bounded v2 removal action names only that protected lease digest
   and expected sequence. The helper verifies the actual current caller and
   retained protected ownership/full keys/full typed state; it never executes
   the retained program path or requires the workload to remain running.
   Unknown/drift/denial retains evidence and keeps ingress closed. The removed
   acknowledgement requires settled provider success, observed exact absence
   and a durable protected acknowledgement. No CAS is claimed.

All messages are closed tagged types with unknown-field/duplicate-key/trailing
data rejection, at most 16 KiB per frame and a fixed stage count, with local-only
first-instance pipe creation and native peer identity validation. Reuse bounded
framing/parser discipline; no new shared secret, HMAC protocol or persistent
privileged daemon is proposed. The original same-SID/admin trust assumptions
remain unchanged. A prospective initial hard budget is 120 seconds for the whole
attended helper (including at most 60 seconds of review), native operations at
most five seconds clamped to remaining time, and an external 150-second process
cap. Those are safety bounds for future review, not measured completion times
or permission to abandon native drain and claim no late commit.

Proposed next source boundaries, each requiring its own review/qualification:

- `internal/lanhelper/protocol_v2.go` and tests: pure closed stage parser/digests,
  preserving v1 and rejecting replay/out-of-order/unknown states.
- `localnet/lan_runtime.go` and tests: adapter driven by the existing actor and
  owner, with a small coordinated `runtime.go` field/command addition later.
- `internal/lanhelper/*_windows.go`: narrow fixed native channel/process/protected
  store observations and one-shot helper adapter; no generic command runner.
- `lan-helper/main_windows.go`: separately attested one-shot privileged entry.
- `lan-worker/main_windows.go`: separately attested dedicated ordinary worker,
  default-closed, accepting only its fixed control ABI. Do not copy the full
  multi-command SDK into an allowed program path: another SDK entrypoint must
  not gain a chance to listen through a retained program rule.

The current selected `/orbwin/port-status` response supplies container ID, host
port, protocol and a published boolean. Its internal `PortGeneration` is not
included in that response, nor are target StartedAt/container port. It cannot
yet supply the complete immutable LAN publication tuple. A narrow authoritative
source-observation extension remains necessary; no proxy candidate is changed
by this work. UDP transport and all native listener/helper behavior remain
separate gates rather than being represented as working by the pure policy.

Source hashes and bounded conclusions are recorded in
`artifacts/qualification/p2-lan-worker-design/source-observations.json`.
Microsoft documents the native observation building blocks:
[pipe client PID](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-getnamedpipeclientprocessid),
[pipe server PID](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-getnamedpipeserverprocessid),
[process creation time from a process handle](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-getprocesstimes),
and [job process ownership/lifetime](https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects).
These APIs supply pieces of the future boundary; listing them does not qualify
their composed use or turn supplied pure-test observations into native authority.

## Source integration increment: 2026-10-25

The new `internal/lanhelper` V2 codec uses fixed tagged frames with bounded
length/token/depth, exact canonical field spelling at every typed level,
duplicate/unknown/trailing-data rejection and setup replay refusal. Frozen V1
request/digest semantics remain unchanged. Parsing does not authenticate peers.

`internal/lanworker` now contains the default-closed TCP core and separate
Windows native identity, process/job/pipe and dedicated-entry source. The worker
entry accepts no arguments and has no SDK command dispatcher or provider entry.
It derives its protected boot record and never-reused program path from its
native executable path under the fixed ProgramData root. It holds process and
file handles, hashes bounded images, observes process creation/SID/native token
session and checks pipe peer PIDs against retained handles. The manager/self and
created child must share the observed token session; no session claim was added
to V1. Protected-file descriptors reject reparse objects and write/delete sharing.
The helper must produce the exact reviewed admin/SYSTEM/full and original-user
read/execute protected descriptors; no helper installation/registration exists
in this increment. A current path hash is not proof of arbitrary preexisting
mapped image bytes before the file was held. Same-SID/admin trust is unchanged.

The ordinary child is created suspended with no inherited handles or arbitrary
arguments, a minimal child-only environment and fixed system working directory.
The exact child enters an unnamed one-process kill-on-close job before resume.
The fixed local first-instance go-winio pipe binds its native client/server PID
to held process identity. There is a 5-second hello bound and a 1-second completed
grant/refresh acknowledgement bound. Writing a grant is not a readiness proof.
The worker independently observes the qualified IP Helper/NLM scope before bind
and refresh. `ListenConfig.Control` applies `SO_EXCLUSIVEADDRUSE` before binding
the exact private IPv4 TCP tuple, refusing any unsupported option. The expression
is the SDK's `~SO_REUSEADDR`, not an unexplained numeric socket option.

The core owns admitted native checks and pending binds in its positive join
barrier. Observation expiry closes the listener AND already active streams even
when another observation or bind stalls. A late listener, partial failed bind,
partial tunnel and late dial are closed before join. Native Stop promptly starts
one shutdown worker: close the exact kill-on-close job before potentially bounded
remote stream revocations, then confirm exact native process exit and join all
manager raw flows/setup/control work. Stop, timeout, EOF or a closed frame alone
does not establish positive process exit. A native query that never returns
keeps join unknown; an external caller deadline cannot promise native drain.

The same `ServiceRuntime` actor now has an internal typed LAN start/stop lane.
It checks the exact publication before helper stage, restores protected
tombstones, accepts only a matching prepared lease, renews the existing exact
namespace, owns a dedicated child, accepts only positive flushed typed Commit
acknowledgement, and requires the completed worker acknowledgement before ready.
An uncertain Commit never retries, adopts by name or reaches rule removal.
Retirement closes/joins the exact child before fresh full rule readback and one
reviewed retirement dispatch. Unknown removal retains the receipt/generation.
The default helper remains unregistered, so these source transitions do not
expose an activation route through existing IPC/UI.

`localnet/lan_namespace.go` shares the SAME OwnedServiceBackend/managers/ledger
between fixed HTTP ownership and exact per-lease LAN ownership. Physical release
occurs only after the last owner and pending setups settle. Existing HTTP dial
and renewal parallelism remains available. At most 32 pending/active LAN TCP
streams share one exact namespace helper across all leases, with zero new exec
dispatch when full; each dedicated worker additionally permits 16 active streams.
HTTP limits are unchanged. UDP still has an explicit unavailable gate pending
combined-budget/worker wiring and qualification; its separate source API does
not constitute a working LAN datagram journey.

Unknown namespace release/revocation retains the helper reference and quota;
borrow/renew/dial and repeated Delete are refused. A separate read-only exact-ID
settlement method checks the same daemon before/after immutable full-ID NotFound,
then settles the exact acknowledged ledger record before dropping manager,
TCP-reference and UDP-counter ownership. Ledger failure or ambiguous presence
retains the reference. This method supplies no name adoption or firewall CAS.
It is not yet an exposed user recovery command. Actor sessions/lease IDs are
bounded; protected tombstones remain denial-only and cannot authorize reuse.

One `generationShutdown` operation waits LAN positive closure before host Close
once, host join and the SAME shared backend join. A bounded Runtime Close may
return cleanup-pending; `services.go` then waits the existing completion barrier,
keeping the original owner/ledger alive instead of repeating mutations or closing
it early. Forced process death remains outside graceful retention guarantees;
job containment/default closure/protected tombstones are the separate safeguards.

The current active-13 upstream still lacks the publication generation/incarnation
tuple. An additive, unregistered source patch and exact baseline record live in
`artifacts/patch-replay/lan-publication-v2/`. The SDK's bounded owned raw-dialer
source rejects the current 404 instead of guessing generations from container
lists. Closed decoding verifies target full ID, StartedAt, running state,
published host/container ports and generation. The combined observation retains
the oldest source/interface/profile timestamp; a later successful query cannot
refresh old evidence. Double source observations are stability evidence, not
reservation/atomicity with future changes.

Compile-only and fake-IO evidence is in
`artifacts/qualification/p2-lan-worker-integration-source/proof.json`: pure/race
tests, vet, Windows dedicated-worker build/full SDK test compilation, Linux
crosscompilation and C11 assertions against Windows SDK 10.0.22621.0 for the new
FILE_ID_INFO layout/signatures, process/job layouts, token output widths and
exclusive socket constant. No dedicated worker, full SDK test executable or Linux
crossbuild was launched. Normal Go test executables ran the listed pure/fake-IO
cases, which invoke neither the native adapter nor real listeners.
The proof records exact binaries, tools, headers, compiled local closure/assets
and source hashes. This does not register a helper/SDK binary or qualify runtime
coexistence, occupied-port exclusivity, cancellation/process death, protected
staging, publication restart invalidation or firewall mutation. Those native and
administrative gates remain root-controlled; no UAC/provider/engine actions were
performed in this source increment.

The native contracts also follow Microsoft's documented
[FILE_ID_INFO identity pair](https://learn.microsoft.com/en-us/windows/win32/api/winbase/ns-winbase-file_id_info),
[CreateProcessW parameters](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-createprocessw),
and [job limit semantics](https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-jobobject_basic_limit_information).
These source/API facts are distinct from a successful native composition test.

## UDP source wiring increment

The preceding TCP snapshot is preserved byte-for-byte with its 137 source guards,
compiled outputs and proof `06acde130a68e2ef7ecba081edaa237bad2c46b91ffe5263a8172b205ebf5226`
under `p2-lan-worker-integration-source/v1-preserved`. The UDP source increment
supersedes its *source wiring* gate, not its outstanding native/provider gates.

The dedicated worker now binds an exclusive exact IPv4 UDP socket in source and
uses a typed datagram-open/datagram-ready pipe channel. Both packet directions
preserve zero-length and binary datagrams without treating UDP as a byte stream.
The worker checks client CIDRs for every received packet and before each reply,
limits its pool to 16 exact address/port peers including pending setup, gives each
peer a four-packet drop-on-full queue, and retires idle peers after 15 seconds.
Queue-full drops avoid payload allocation. Observation TTL closes the active
socket and associations; blocked/late setup remains in the join barrier and its
late returned resource is closed. Native manager channels distinguish TCP and
UDP tags/peer shape, and bind the opened child creation time to the original
CreateProcess handle, in addition to SID/session/file/pipe identities.

`localnet/lan_datagram.go` connects the actor's UDP branch to the SAME owned
NamespaceManager's typed DialDatagram API. All leases share a total 32 LAN
pending/active/retained slots per helper, with an additional 16 UDP sublimit.
Positive remote revocation/exec-exit confirmation releases slots; unknown
revocation poisons shared TCP/UDP admission and retains counters/references.
Read-only exact-ID absence plus durable ledger settlement remains the only
separate uncertainty settlement path; there is no fallback Delete or name adoption.

Relevant changed tests, race checks, vet, Windows worker/actor compilation and
Linux worker/actor crosscompilation passed in
`artifacts/qualification/p2-lan-udp-worker-source/proof.json`, SHA
`eba8c081a41616c89264995337412eb552f563b7bf0ea6644e504d4a0f68f64b`.
All 136 captured compiled/test source guards matched before/after its eight
commands. Tests used only in-memory packet/socket/control fakes. Neither a real
UDP/TCP socket nor a generated worker, helper, provider or engine was invoked.
Root separately qualified the lower-level Linux helper through anonymous pipes
(`p2-udp-pipe-f3f802fafd764ab5845dbcb256ebb689`); that is not UDP socket proof.

The next concrete production work and user/owner DTO proposal are recorded in
`lan-helper-integration-next.md` and `lan-ui-owner-contract.md`. In particular,
the current internal synchronous Stage must not be exposed directly as attended
activation: the existing actor needs bounded owned asynchronous stage completion
to keep HTTP/helper/other-LAN renewal running during UAC/scope review. The
administrative helper/store/consent/provider production adapter, source-v2 patch
adoption, packaging/UI registration and actual native traffic/cleanup tests remain
implementation and qualification work, not exclusions or a permanently disabled
product scope.
