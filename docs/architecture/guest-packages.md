# Offline guest packages

Kubernetes with the selected Moby runtime invokes socat for pod port forwarding.
The pinned openSUSE rootfs lacks socat and its libwrap dependency. OrbWin reuses
signed Leap 16.0 RPMs rather than replacing Kubernetes streaming or implementing
another network proxy.

`guest-packages.lock.json` records exact binary and corresponding source RPM
URLs and hashes. The existing upstream guest dependency downloader verifies them
in a separate cache. The Windows guest-capable binary embeds the two binary RPMs;
the product bundle retains the source RPMs and license texts. Build identity also
binds the package lock. No package download occurs inside the engine guest.

The existing content-addressed staging path publishes immutable RPMs under the
owned backend home. The guest verifies their hashes and RPM signatures before
installation. Within the authoritative provision script, the existing target is
stopped, both packages are installed in one offline RPM transaction if their
versions differ, and installed files are verified. RPM refuses downgrades and
file/dependency conflicts; no force, reset or repository refresh is used.
The existing EXIT trap restarts the engine target even when this step fails.
The error remains visible through failed provisioning. A retry is idempotent;
no engine disk is replaced and no user development distribution is modified.

Acceptance requires a fresh guest, an existing guest with missing packages,
idempotent restart, concrete HTTP traffic through pod/service forwards and
retained workload data. A manually repaired guest alone does not meet that gate.
