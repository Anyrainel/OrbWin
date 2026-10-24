# WSL integration boundary

Phase 1 uses WSL's existing shared `/mnt/wsl` filesystem and a Unix client
relay at `/mnt/wsl/orbwin/docker.sock`. It reaches the existing Windows Docker
adapter through mutual TLS inside rdd's private gvisor network. There is no
public Windows TCP listener, new engine, or desktop lifecycle supervisor. A separate `orbwin-proof`
path is reserved for sequential qualification. Other engines' paths and user
shell defaults remain untouched.

The systemd drop-in retains the pinned upstream `wsl-exec dockerd` invocation,
runtime and `DOCKER_OPTS`, with a root-only internal Moby socket. Both the shared
socket and `/var/run/docker.sock` are served by the relay, including when a
workload explicitly receives the latter socket. This ensures Windows port
allocation/publication checks also apply to WSL and delegated Docker clients.
The public Unix endpoints inherit upstream's local socket permission policy. Changes to upstream's
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
the oneshot still reports active. A direct dockerd listener survived that exit,
but bypassed Windows host-port adaptation. Patch 8 supersedes it with the shared
authenticated relay described in [Windows published ports](windows-published-ports.md).
The project connection's observed mount state is always rechecked.

Qualification must exercise an ordinary non-root Ubuntu user, actual Linux
Docker identity, Linux-directory read/write visibility, Windows-drive binds,
Compose/build, socket restart, disconnect and preservation of other engines'
paths/configuration. Proof experiments are not a supported integration until the
native adapter and its ownership checks pass.

Opening a user distribution validates its current registered name and refuses
known engine distributions. The preferred interactive host is the installed
Windows Terminal execution alias, launched with an explicit new window and a
fixed WSL executable; no command interpreter is used. WT's separate semicolon
action separator is refused in distribution names. Ordinary Windows argument
quoting is covered by round-trip tests against CommandLineToArgvW. The fallback
Windows shell launch detects immediate WSL exit and reports the copied-command
recovery, rather than asserting that a terminal opened. The terminal client
belongs to the user and is never put in OrbWin's kill-on-close job.

Reference: [Windows Terminal command-line interface](https://learn.microsoft.com/en-us/windows/terminal/command-line-arguments).
