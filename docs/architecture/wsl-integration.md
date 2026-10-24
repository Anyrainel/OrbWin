# WSL integration boundary

Phase 1 uses WSL's existing shared `/mnt/wsl` filesystem and an additional Unix
listener on the existing dockerd at `/mnt/wsl/orbwin/docker.sock`. There is no TCP
listener, new daemon, or desktop lifecycle supervisor. A separate `orbwin-proof`
path is reserved for sequential qualification. Other engines' paths and user
shell defaults remain untouched.

The systemd drop-in retains the pinned upstream `wsl-exec dockerd` invocation,
runtime and `DOCKER_OPTS`, and adds the normal and shared Unix listener arguments.
It inherits upstream's local socket permission policy. Changes to upstream's
invocation require review when updating the pin. An inactive endpoint is an error,
never an automatic switch to another engine. WSL distributions controlled by the
same Windows user share a privileged trust boundary; this does not isolate a
malicious root in another distribution. Socket access inherits upstream modes.

Selected user distributions may use the pinned Linux tools with an explicit
endpoint. Linux projects need an explicit directory exported under the owned
shared path; a native Linux `/home/...` path has no meaning in the separate
engine distribution. Phase 1 must display/copy the actual shared bind source.
Transparent path mapping and shell-default integration remain later conveniences.
Project sharing must be opt-in, limited to an existing selected directory, verified by
mount identity, and removable without deleting its source. Repeated operations
must verify existing state rather than stack mounts. Keep a user WSL shell open
while using its project export. WSL can remove shared bind mounts when that
distribution exits; reconnecting verifies/recreates the export. No global WSL
restart or hidden distribution keepalive is used.

Rejected proof implementations: a bind of `/run/docker.sock` first raced Docker's
replacement of its socket activation inode. Ordering it after Docker fixed that
race, but a separate proof showed WSL removes the bind when Ubuntu exits while
the oneshot still reports active. The direct dockerd listener avoids that stale
success. The project connection's observed mount state is always rechecked.

Qualification must exercise an ordinary non-root Ubuntu user, actual Linux
Docker identity, Linux-directory read/write visibility, Windows-drive binds,
Compose/build, socket restart, disconnect and preservation of other engines'
paths/configuration. Proof experiments are not a supported integration until the
native adapter and its ownership checks pass.
