# Rancher adaptation record

Base: repository and exact commit in `upstream.lock.json`. Upstream Apache-2.0
headers and tests are retained. Patches are applied in order; normalized source
hashes detect unexpected drift. Preparation never resets developer work.

| Patch | Reason | Status |
| --- | --- | --- |
| 0001 instance Docker pipe | Shared default pipe prevents simultaneous engines; endpoint discovery and bridge must agree | Focused Windows tests and race checks pass; clean replay verified |
| 0002 OrbWin WSL namespace | Shared vsock ports/signature, veth cleanup names and proxy address can collide | Paired source contracts, host tests and Linux cross-compilation pass; live guest test pending |
| 0003 owned WSL distro | Lima derives global registration from VM name, not LIMA_HOME | App-controller tests pass; runtime registration test pending |
| 0004 product environment guard | Dynamic help can autostart the default instance before user intent; global client paths must not leak through | Product identity validation before CLI construction, private client paths; tests/build verification required |
| 0005 Windows Docker paths | Native CLI/Compose bind sources reach Linux Moby without drive translation | Existing bridge uses the standard HTTP reverse proxy with narrow create-body adaptation, owner/System pipe ACL and bounded create reads. Unit/HTTP/stream/upgrade/half-close/shutdown and race checks pass; clean five-patch replay verified. Runtime qualification in progress. |
| 0006 sequential provisioning proof | Clean setup must be exercised without replacing the existing product guest/data | Reserves `orbwin-proof` guest only for the already-allowed proof instance. Normal identity remains `orbwin`. The same native setup path passes cancel/provision/resume with the product guest stopped; no simultaneous proof/product support is implied. |

0002 currently assigns one product namespace, rather than supporting arbitrary
parallel instances of OrbWin: discovery 16670, network data 16657, Docker 16660,
containerd 16661; signature `dev.orbwin/wsl-network/v1`; links `veth-ow-wsl` and
`veth-ow-ns`; bridge 192.168.144.0/24. These values are development candidates.
VPN/route/other application conflicts remain a gate. 0003 uses `lima-orbwin`.
No engine changes or persistent data migrations are introduced.

Before changing the pin: stage a fresh source checkout, review each patch and
the updated host integration, update normalized hashes intentionally, rebuild all
host/guest components, run source contracts and upstream tests, then repeat
runtime/coexistence/workload/preservation qualification. A clean patch apply is
necessary but does not prove upstream compatibility. Do not reuse the old runtime
qualification record after upgrading the source or patches.

No patches have been submitted upstream. The long-term upstreamable direction
is namespace parameterization; the current product-specific patch is a bounded
development adaptation whose runtime suitability remains unverified.
