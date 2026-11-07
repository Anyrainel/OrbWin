# Private runtime executable ownership

This is the current implementation contract, with remaining native qualification
called out below. It does not establish publisher signing or a sandbox against
the trusted Windows account.

## Origin and location

The desktop build hashes actual selected component bytes into the executable.
`ORBWIN_COMPONENT_INPUT_DIR` and `ORBWIN_TOOLS_INPUT_DIR` select absolute backend
and tool build-input directories; runtime
lookup remains fixed to packaged resources or the existing debug artifact path.
Debug builds without an input cannot execute that component. Release builds
require all eighteen selected inputs: rdd/forwarder/session, four LAN bundle
files, credentials/four Linux helpers, and six locked Windows/Linux client tools.
Tool bytes must also match the independently embedded runtime-tools lock. A
runtime manifest cannot replace a compiled digest.

The cache derives native KnownFolder_Profile from the actual ordinary process
token, then uses `.orbwin-private/runtime`. It leaves the existing guest home,
WSL registration and disks in place. Each ancestor must satisfy the unchanged
integrity policy; private descendants require exactly the protected current
account/SYSTEM/Administrators descriptor shared with the backend authority.
Metadata directory handles permit read/write sharing while denying delete
sharing. Executable and bundle file handles deny write/delete sharing. No ACL
repair or capability-principal exemption is applied.

Only closed role constructors can select inputs. The rdd directory is
`rdd-<image SHA256>`; its image embeds the independently compiled forwarder hash,
so a forwarder update changes the generation. The fixed adjacent
`rdd-forwarder.exe` is pinned independently by the desktop too. The session
directory is `orbwin-session-<canonical LAN manifest SHA256>` and includes the
manager plus `lan/assets.json`, `lan/assets.sig`, helper and worker. That manifest
commits all three image hashes and sizes; unchanged manager bytes cannot hide
changed siblings under an existing generation.

Windows Docker and its two plugins share a generation derived from the embedded
tool lock. Linux Docker/Compose/Buildx use a separate `linux-docker` generation.
Credential and project helpers have closed image-digest roles. A bounded six-role
table retains strong native leases for the desktop lifetime, including paths
handed to child plugin discovery. Cache reuse rechecks native identities and
descriptors; the initial hash remains valid while WRITE/DELETE sharing stays
continuously denied. Explicit rechecks still hash bytes. Guest file/service/trust
payloads require independent compiled pins before their existing in-memory hash
verification and guest transfer; maintenance already has its embedded lock.
Old public WSL plugin directories require explicit reconnect rather than a loose
matching hash followed by later execution from a mutable path.

## Publication and failure

All source files are held, bounded and hashed against independent pins before
cache creation. Source ancestry may be unsuitable for execution: only the
verified public bytes are copied. Destination traversal uses held native parent
handles and refuses reparses, hardlinks, unsafe descriptors and unknown object
types. Only native name-not-found permits creation; denied or busy is not absent.

Creation uses a private exclusive stage, bounded copy, flush, hash and one
non-replacing relative rename. The write object is closed before reopening the
same native file identity read-only. Every fixed bundle member must pass before
returning an executable. Existing mismatched or uncertain state refuses without
overwrite, repair, retry or a cleanup sweep. Interrupted stages remain evidence.
Lease clones share ownership and serialize file-pointer-based rechecks.

The backend's Windows standalone convenience must use copied upstream forwarders
for only rdd and kubectl. Creating hardlinks to the cached rdd would invalidate
its single-link identity; it is prohibited. Unrelated standalone tools survive.
That backend adaptation is in the not-yet-adopted authority successor.

## Child and detached service lifetimes

Normal operation owners retain their lease while waiting. Terminal slots retain
it with their stream/job/sole-wait owner after the start command returns. SDK RPC
workers own the exact client, lease and one of sixteen admission slots. Caller
cancellation/deadline detaches the worker; it does not release ownership. Error
or timeout requests termination of that transient client and positively waits.
An unknown wait retains ownership and its admission rather than retrying.

A detached SDK launch owns its actual child before the first fallible readiness
poll. Only a native self-lease acknowledgement for that actual child generation
permits handoff. Otherwise a sole bounded native waiter retains child and launch
lease, and a pending-start fence prevents another launch. Wait/thread/channel
failure retains unknown resources and the fence. It never kills or replays a
possibly live independent service merely because startup observation failed.

The SDK acquires its own native process, ancestry, manager and signed bundle
lease before ledger/listener publication, retaining it through runtime Close
and Wait. The acknowledgement reports actual PID/creation/session, image and
bundle hashes, with `leaseHeld` only after native verification. Rust checks the
compiled hashes, actual process creation/session, and actual image file identity
against its retained cache file. The SDK pipe client additionally authenticates
the actual connected server process and same-build user/session before writing
any request, then correlates the reply to that retained peer. JSON alone grants
no authority. The backend successor carries equivalent serving-image retention.

Process-crash atomic containment and installed update behavior require separate
native evidence; these source ownership checks do not claim those results.

## Evidence and remaining work

The frozen base cache fixture passes private native creation/copy/flush/hash,
non-replacing publication, same-file reopen and retained recheck in a fresh
Profile nonce directory. It launches no copied image and touches no production
cache. Its receipt is
`artifacts/qualification/p2-runtime-private-cache-source-v1-20261026/native-run-0032695c-88cf-437b-ae8b-515822822e64/proof.private.json`.
The expanded inert SDK boundary now also passes actual cache reuse, write-share
refusal for all five files, child PID/creation/session/image identity, same-build
named-pipe peer-before-write and wrong-path peer zero-byte refusal. Its receipt is
`artifacts/qualification/p2-sdk-self-peer-cache-source-v1-20261026/native-run-42157095-524c-417b-9591-dd47b68d62ed/proof.private.json`.
Only fixed nonce namespaces differ in the frozen production cache/SDKself
sources; all children are inert. No engines, UI, administrator, credential-store
or production-cache actions occur. This does not qualify a real SDK service,
UI exit/reconnect, host tool execution or the backend successor.
Ownership fault tests exercise the actual
worker/guard cores with inert resources, without manufacturing native authority.

The upstream forwarder compiles twice to the same image hash; neither image was
executed. The retained current recipe and source/compiler/runtime-license record
are under
`artifacts/qualification/p2-rdd-forwarder-cc9d8513-36c1-4fba-938b-6ac952565678/`.
The SDK bundle builder keeps an ephemeral qualification signing key only in
memory and passes only its public key to child compilers. Packaging verifies
the exact signed bytes, complete source/resource/recipe closure and copied
hashes before publishing its package manifest. The first integrated qualification
bundle now builds and passes that provenance check under
`artifacts/qualification/p2-private-state-sdk-bundle-20261026-a/`. Its product
executables were not installed or launched. The final installed product bundle
remains unqualified.

The separate Go CLI source now consumes the same fixed private cache roles with
independent build pins and holds capabilities through transient tree retirement.
It refuses missing private cache roles rather than executing loose installed
files. Its native atomic job runner and private config/snapshot protocol have
source, parser, race and build evidence; they still need native qualification.
Detached backend handoff and final combined deployment remain gates. A second
cache publisher or backend supervisor is unnecessary. Historical CLI functional
proofs do not qualify the new source or an installed bundle.

The retained-file process-image metadata optimization passed a fresh inert
SDK/self/peer fixture in 2.659 seconds, with four probe children positively
waited and no product cache or engine touched:
`artifacts/qualification/p2-sdk-self-peer-cache-source-v2-20261026/native-run-4a401b65-9f09-4236-9c7e-9991ff282827/proof.private.json`.
This checks the optimized origin boundary, not native client-config writers or
the new atomic transient runner.
