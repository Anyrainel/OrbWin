# Bounded namespace datagrams

Source implementation increment; UDP activation and the helper capability remain
unavailable pending integration and actual transport qualification. Existing TCP,
namespace creation, ledger and lease ownership are unchanged.

Each reviewed LAN peer needs an independent connected UDP socket inside the
already-owned target network namespace. The fixed helper command is
`/orbwin-services dial-udp <canonical-port> <fresh-stream-token>`; its destination
is exactly IPv4 `127.0.0.1:<port>`. No hostname, caller address, command or socket
path is accepted. The existing streamcontrol owner admits and revokes its token;
control loss terminates the command, as for TCP. NamespaceManager admission must
reuse its exact daemon, active handle, target incarnation, helper ownership,
shutdown accounting, and revocation uncertainty/poison checks.

The caller-facing interface is `ReadPacket`, `WritePacket`, `Close`,
`SetDeadline`, `SetReadDeadline`, and `SetWriteDeadline`. It is not `net.Conn`:
packet boundaries, including empty packets, are explicit. Exec stdio carries a
four-byte unsigned big-endian length followed by exactly that payload. The IPv4
payload limit is 65,507 bytes. Oversized frames, partial headers/payloads and
transport errors terminate the association, preventing resynchronization into
attacker-controlled bytes. A caller buffer that is too small consumes the whole
packet and returns an error without exposing a truncated prefix. Oversized local
writes are refused before any bytes are emitted.

One reader and one writer per association are serialized independently. The
codec has one fixed 65,507-byte receive buffer and no packet queues. Calls bound
their transport deadline even when the caller requests none: reads idle out at
30 seconds, writes at five seconds; an earlier caller deadline wins. Timeouts
terminate an incomplete framed association. The helper uses two fixed pumps,
closes both packet and stdio resources on either pump failure/cancellation, and
joins both before returning. There is no goroutine per packet. Its connected
socket reads into 65,508 bytes and refuses a truncated/oversized datagram rather
than forwarding a prefix. Slow readers/writers backpressure the corresponding
pump; kernel UDP receive-buffer overflow may drop packets. No reliable delivery,
ordering, retransmission or lossless overload claim is made.

Inherited Docker exec stdio can be blocking/nonpollable with Go 1.27.3;
forwarding os.File deadline setters would refuse the flow. The dedicated
`dial-udp` process therefore enforces stdio deadlines with process watchdogs,
without changing inherited descriptor flags, reopening proc-fd paths, or relying
on pipe-inode ownership. Each actual blocking Read/Write arms its direction's
absolute deadline; completion disarms it. No write watchdog runs while waiting
for a later packet. Generation checks under one mutex make stale callbacks
harmless; an expiry that commits termination cannot be reset back to active.
Expiration exits only this association exec with status 125, rather than claiming
a blocked stdio syscall returned a timeout. Close arms a separate two-second
termination watchdog before closing descriptors; only actual Bridge pump join
cancels it. Close returning alone is insufficient. This policy is constructed
only in the fixed UDP dial dispatch, never the persistent namespace owner/TCP.

The Windows worker remains responsible for validating the source peer against
the frozen CIDRs for **every** incoming packet, obtaining the current lease,
and limiting a lease to 16 independently reserved peers. The small peer-slot
primitive only bounds concurrent distinct associations; it does not authorize
peers, create a namespace, renew a lease, or keep durable state. Idle associations
must be closed and release their slot; a 17th peer is dropped/refused, never
queued. One host codec buffer, one helper codec buffer and two helper pump buffers
are retained per association: 262,030 bytes, at most 4,192,480 bytes across 16
peers, excluding goroutine stacks and Docker/kernel transport buffers. Each
helper connected socket requests 128 KiB send/receive buffers (Linux accounts
additional/doubled socket overhead); the existing helper also retains its 64 MiB
container memory limit. These source bounds are not live allocation measurements.
Existing manager/helper admission capacity is still required and may refuse
earlier. Worker integration must also bound pending setup and total leases.

The existing NamespaceManager additionally reserves at most 16 pending, active or
uncertain UDP execs globally per exact helper ID across all leases. Its existing
mutex and lifetime own those counters. An unknown ExecCreate or failed Attach
must fence the fresh token. Capacity additionally requires the exact created
exec ID to be observed stopped with matching ExecInspect ID and ContainerID,
under the same three-second cleanup deadline (not a fresh deadline per poll).
Unknown create identity, failed revocation or unobserved exit retains the slot
and poisons the helper; no retry creates additional uncounted execs. Exact
helper-ID physical absence also settles it. A control-lease acknowledgement
alone is not claimed as process exit or pump join.
At most 4,096 helper IDs with retained reservations are tracked; reaching that
bound refuses a fresh helper ID without eviction. Physical-absence settlement
uses the same verified daemon, exact full ID NotFound, and daemon recheck. It
never removes a container. The existing namespace owner's already-verified
absence/ledger-settlement path may call `settleDatagramAbsenceLocked` while
holding its mutex. Late association Close callbacks cannot reinsert a settled
counter or decrement an absent entry.

Setup context controls admission only. Returned associations own Close and the
existing three-second revocation acknowledgement. Failed/uncertain revocation
poisons the existing namespace handle; it must not be silently reused. Close
must interrupt blocked I/O and be joined before releasing the namespace reference.
No failed operation retries a packet, starts a workload, or changes a publication.

This increment does not wire sharedNamespaces/worker UDP ownership, register an
ABI/capability, rebuild the helper, open an actual listener, or qualify a network
path. Fake packet/stream/clock tests and compilation cannot establish socket
truncation behavior, Windows peer revocation, interoperability, loss behavior,
resource bounds under real load, or end-to-end UDP acceptance. Those are required
integration and qualification gates, not hidden successful states.
The original compile-only proof remains historical. Root subsequently ran the
exact frozen helper test binary's opt-in anonymous-pipe fixture in a new UID1000
Ubuntu temporary directory: blocked read, blocked write, Close and joined cases
all passed without the parent five-second timeout, and exact cleanup passed.
See [bounded pipe receipt](../../artifacts/qualification/p2-udp-pipe-f3f802fafd764ab5845dbcb256ebb689/proof.json).
This observes the dedicated subprocess watchdog boundary; it does not establish
actual Docker exec pipe compatibility or qualify any UDP socket traffic.
