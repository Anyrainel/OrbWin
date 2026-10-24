# Installation and first-run ownership

Application binaries and tool manifests are bundled read-only resources. New
runtime state defaults to `%LOCALAPPDATA%/OrbWinData/home`, outside the application
installation and source checkout. Uninstall retains this state. Reads never
create/start a service. Setup is a single explicit, cancellable operation using
the same operation registry as other mutations.

Setup verifies bundled backend/client hashes and WSL availability, refuses a
foreign existing `lima-orbwin` registration, and writes an ownership intent before
calling the upstream service/App creation commands. rdd owns import, downloads,
reconciliation and persistent disks. Cancellation terminates only its CLI client;
the UI reports that backend work may continue and offers Resume. Setup never
unregisters or deletes a partially provisioned guest as automatic rollback.

An identity is published atomically only after current-start Lima readiness,
registry BasePath ownership and an actual Linux Docker daemon response agree.
Interrupted setup retains intent and logs; retries reuse the same owned home and
upstream resources. The stored daemon ID must remain unchanged on later startup.
The app never copies a writable engine disk or assumes Rancher data interchange.

Docker, Compose, Buildx and Linux clients use the selected upstream's exact
version/checksum pins. Client plugins are configured only in the private OrbWin
Docker config, preserving other keys. Global PATH, Docker context, kubeconfig,
WSL configuration and other engines are not changed.

A named qualification-only instance may exercise clean provisioning using a
separate registration/home/pipe. Because the product currently owns one network
namespace, such a proof must reject a running product guest and cannot run in
parallel with it. This is test infrastructure, not multi-instance product support.

## Bundled backend replacement

An installed upgrade must be an explicit setup/resume action with a workload
interruption confirmation. It verifies the new immutable resources before any
lifecycle request, validates the existing intent and identity against the exact
owned home/endpoint/registration, and retains the recorded daemon ID. If the
bundle differs, stop the owned management service before starting the new binary;
never let an old service answer readiness for a new manifest. Cancellation at any
step retains intent, the old identity, disks and logs; resume repeats validation.
Publish the new identity only after current-start readiness and the same actual
daemon ID. An unknown or mismatched record must refuse before service changes.
The existing exclusive setup lock and operation mutation lease serialize clients;
rdd remains the only reconciler. No automatic downgrade or disk rollback occurs.

An ordinary Start also waits for Lima's `Running=True/Started` condition after
the current management service's startup before verifying the Docker identity
and Windows publication. A service resumed after file replacement may retain
an already-true App Settled condition with no new generation for `running=true`;
that condition alone is insufficient. The extra observation has a 180-second
backend timeout and shares the existing operation cancellation/output path.
It never restarts an engine or modifies desired state on its own.

Management-process loss can leave the existing guest/hostagent and Docker
workloads healthy. On the selected pin, resuming management then restarted the
guest: do not claim uninterrupted adoption. Settings and the command palette
expose Reconnect backend using the same explicit `running=true` operation, with
a workload-interruption confirmation. It waits for current-start readiness and
verifies actual identity/publication. No cached renderer status authorizes
readiness, and no destructive reset or second lifecycle owner is introduced.

Creating a new App explicitly selects Kubernetes 1.35.3 while leaving the
optional cluster disabled. The pinned upstream default 1.34.6 exceeds the
bundled kubectl 1.36 client's supported one-minor skew; 1.35.3 is in the backend
catalog and has passed native upgrade/PVC-preservation qualification. This is a
creation-only default. Existing App versions are preserved and require explicit
version selection/confirmation for upgrades; resuming setup never upgrades them.

Docker readiness can precede Windows publication when an existing container
resumes under its restart policy. Post-start/configuration publication checks
therefore wait up to 30 seconds, observing the actual adapter's ownership and
fresh-generation status once per second. One absolute deadline covers the checks
and delays. Missing publication retains the concrete port error at the deadline;
identity/helper errors fail immediately. Cancellation interrupts the observer
and reports an uncertain accepted backend change. Nothing reallocates a port,
restarts a process or converts Docker's running state into publication success.
