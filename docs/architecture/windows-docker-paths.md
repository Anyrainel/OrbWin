# Windows Docker path adaptation

The owned Windows Docker pipe is an rdd bridge, independent of the UI. Moby still
owns containers, volumes and persistence; no second service or lifecycle manager
is introduced. The observed native Windows create failure requires translation
at this boundary so ordinary CLI, Compose and API clients work alike.

The Docker branch of the existing bridge uses Go's HTTP reverse proxy over the
same guest vsock. Only POST container-create HostConfig bind sources and local
bind volume devices are rewritten. JSON RawMessage values retain unknown fields
and numeric precision. Unchanged JSON is forwarded byte-for-byte. Linux paths,
named volumes and mount flags retain their meaning. UNC/device and drive-relative
sources fail explicitly rather than targeting an unintended path. The existing
upstream host-path translator maps absolute drive paths to `/mnt/<drive>`.

The standard proxy owns HTTP framing, streamed responses and 101 upgrades
(exec/attach and BuildKit sessions). No payload/log transformation occurs on
other routes. Create JSON has a 4 MiB cap and a 30-second body-read deadline;
streaming routes have no blanket response deadline. Headers have a bounded read
deadline. A fixed vsock dialer ignores environment proxy/client context settings.
Connections, including hijacked sessions, are tracked and closed when the bridge
context ends; stopping the bridge cannot leave live API access behind. Listener
errors fail without stopping another engine. No retry of a rejected mutation is
introduced. Backend shutdown remains rdd's responsibility.

Qualification must cover unknown JSON fields/large integers, malformed/oversize
requests, both bind encodings, Compose/local volumes, read-only/file/directory
paths, build streams, TTY/upgrade traffic, cancellation and bridge shutdown.
