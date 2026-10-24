# Kubernetes version selection

The pinned rdd version catalog owns supported versions. The UI reads its
`k3s-versions` ConfigMap through the existing owned management kubeconfig; it
never consults or changes a global Kubernetes context. Reads do not start rdd.
The installer resolves the exact release from the same embedded catalog instead
of assuming every bare Kubernetes version has a `+k3s1` release.

Version changes use the existing App spec and reconciler, including its guest
restart. The confirmation explains that Docker and Kubernetes workloads are
interrupted and data is retained. There is no UI supervisor or automatic rollback.
Cancelling the client can leave the accepted desired version applying; refresh
and inspect before another operation.

The backend admission path rejects downgrades, major changes, skipped minor
versions and clearing a previously selected version. A different version requires
the existing enabled cluster to be ready and settled at the current App generation.
This prevents rapid desired-state updates from bypassing the sequential upgrade
requirement. The same version remains repeatable for recovery; enabling/disabling
without changing the version retains the existing upstream behavior. Unsupported
versions fail before changing the App. Initial selection has no prior data-version
claim; the catalog still constrains it. These conservative product restrictions
do not claim all transitions allowed by upstream are qualified.

Persistent volumes and container data remain owned by the upstream guest. Tests
must cover exact release resolution, stale/unready admission, retained data across
a real supported upgrade, and rejected transitions without resource mutation.
Failed downloads are reported through upstream conditions and retained evidence;
the UI never resets a cluster or fabricates readiness to recover.

The pinned Lima WSL driver invokes `boot.sh` directly, while the selected guest
also invokes it through `lima-init.service` in its private network namespace.
Observed during the upgrade: systemd installed 1.35.3 successfully, but the direct
invocation kept retrying DNS and could subsequently change the same target's
dependencies. Product stateful provision scripts must run only in the systemd
namespace. The direct path waits, with a deadline, for that existing unit's result
and skips those scripts. It must propagate failure rather than mark boot complete
while the authoritative provisioning is still running. This uses the existing
unit and namespace, not another provisioner or lifecycle supervisor.
