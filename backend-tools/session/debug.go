package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

const debugImage = "alpine:3.21@sha256:bbfcd601d7cd20d35bb0b8b866ad1a8a0d107ca3e1c20870c75c5ce2c1186d30"

// Only the transient tools process is leased. Guest monotonic uptime avoids
// host clock changes; replace the heartbeat file atomically inside its tmpfs.
const debugHeartbeat = `cut -d. -f1 /proc/uptime > /tmp/orbwin-lease.next && mv /tmp/orbwin-lease.next /tmp/orbwin-lease`

// The watchdog stays a member of the helper's process group, pinning that group
// identity until it kills itself and the shell together. Never signal a bare
// parent PID that could have exited and been reused in the target PID namespace.
const debugEntrypoint = `parent=$$; group=$(cut -d' ' -f5 /proc/$$/stat); [ "$group" = "$parent" ] || exit 125
start=$(cut -d. -f1 /proc/uptime); echo "$start" > /tmp/orbwin-lease
(while :; do sleep 1; now=$(cut -d. -f1 /proc/uptime); last=$(cat /tmp/orbwin-lease) || exit
if [ "$((now-last))" -ge 15 ] || [ "$((now-start))" -ge 3600 ]; then kill -KILL -"$group"; exit; fi; done) &
exec /bin/sh`

func debugConfiguration(msg message) (client.ContainerCreateOptions, error) {
	if len(msg.Session) != 36 || strings.Trim(msg.Session, "0123456789abcdefABCDEF-") != "" {
		return client.ContainerCreateOptions{}, errors.New("debug requires a unique session ID")
	}
	pids := int64(128)
	return client.ContainerCreateOptions{
		Name: "orbwin-debug-" + msg.Session,
		Config: &container.Config{Image: maintenanceImageID, Tty: true, OpenStdin: true, StdinOnce: true, AttachStdin: true, AttachStdout: true, AttachStderr: true,
			Cmd: []string{"/bin/sh", "-c", debugEntrypoint},
			Env: []string{"TERM=xterm-256color", "PS1=orbwin-debug> ", "HOME=/tmp"}, WorkingDir: "/tmp",
			Labels: map[string]string{"dev.orbwin.transient": "debug", "dev.orbwin.session": msg.Session, "dev.orbwin.target": msg.Container}},
		HostConfig: &container.HostConfig{AutoRemove: true, ReadonlyRootfs: true, PidMode: container.PidMode("container:" + msg.Container), NetworkMode: container.NetworkMode("container:" + msg.Container),
			CapDrop: []string{"ALL"}, CapAdd: []string{"SYS_PTRACE", "DAC_READ_SEARCH"}, SecurityOpt: []string{"no-new-privileges:true"},
			Tmpfs: map[string]string{"/tmp": "rw,nosuid,nodev,size=64m,mode=1777"}, Resources: container.Resources{Memory: 128 << 20, PidsLimit: &pids, NanoCPUs: 1_000_000_000}},
	}, nil
}

func runDebug(cli *client.Client, msg message, scanner *bufio.Scanner, output io.Writer) (code int, resultErr error) {
	options, err := debugConfiguration(msg)
	if err != nil {
		return 125, err
	}
	ctx, cancel := bounded()
	target, err := cli.ContainerInspect(ctx, msg.Container, client.ContainerInspectOptions{})
	cancel()
	if err != nil {
		return 125, err
	}
	if target.Container.ID != msg.Container || target.Container.State == nil || !target.Container.State.Running || target.Container.State.Paused {
		return 125, errors.New("debug requires the exact running, unpaused target")
	}
	started := target.Container.State.StartedAt
	ctx, cancel = bounded()
	err = ensureMaintenanceImage(ctx, cli)
	cancel()
	if err != nil {
		return 125, err
	}
	// A cold image load can outlive a target restart. Recheck before creating a
	// namespace-sharing helper; the live monitor still covers later changes.
	ctx, cancel = bounded()
	current, err := cli.ContainerInspect(ctx, msg.Container, client.ContainerInspectOptions{})
	cancel()
	if err != nil {
		return 125, err
	}
	if current.Container.ID != msg.Container || current.Container.State == nil || !current.Container.State.Running || current.Container.State.Paused || current.Container.State.StartedAt != started {
		return 125, errors.New("debug target changed while preparing tools; reconnect to the intended target")
	}
	intent, err := beginTransient(cli, "debug", msg.Session, msg.Container)
	if err != nil {
		return 125, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, intent.close())
		if resultErr != nil {
			code = 125
		}
	}()
	ctx, cancel = bounded()
	created, err := cli.ContainerCreate(ctx, options)
	cancel()
	if err != nil {
		if cerrdefs.IsNotFound(err) || cerrdefs.IsConflict(err) {
			intent.settled = true
		}
		return 125, fmt.Errorf("create bundled debug tools: %w", err)
	}
	// Only this successful create's full ID is eligible for forced cleanup.
	defer func() {
		ctx, cancel := bounded()
		defer cancel()
		_, cleanupErr := cli.ContainerRemove(ctx, created.ID, client.ContainerRemoveOptions{Force: true})
		// AutoRemove may own removal already. Reconcile that exact ID rather
		// than replacing a real shell exit code with a transient conflict.
		if cerrdefs.IsConflict(cleanupErr) {
			waiting := cli.ContainerWait(ctx, created.ID, client.ContainerWaitOptions{Condition: container.WaitConditionRemoved})
			select {
			case <-waiting.Result:
				cleanupErr = nil
			case cleanupErr = <-waiting.Error:
			case <-ctx.Done():
				cleanupErr = ctx.Err()
			}
		}
		if cleanupErr != nil && !cerrdefs.IsNotFound(cleanupErr) {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove exact debug helper %s: %w", created.ID, cleanupErr))
			code = 125
		} else {
			intent.settled = true
		}
	}()
	if err = intent.created(created.ID); err != nil {
		return 125, err
	}
	ctx, cancel = bounded()
	attached, err := cli.ContainerAttach(ctx, created.ID, client.ContainerAttachOptions{Stream: true, Stdin: true, Stdout: true, Stderr: true})
	cancel()
	if err != nil {
		return 125, err
	}
	defer attached.Close()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Hour)
	defer waitCancel()
	waiting := cli.ContainerWait(waitCtx, created.ID, client.ContainerWaitOptions{Condition: container.WaitConditionNextExit})
	ctx, cancel = bounded()
	_, err = cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{})
	cancel()
	if err != nil {
		return 125, err
	}
	resize := func(cols, rows uint) error {
		ctx, cancel := bounded()
		defer cancel()
		_, err := cli.ContainerResize(ctx, created.ID, client.ContainerResizeOptions{Width: cols, Height: rows})
		return err
	}
	if err = resize(msg.Cols, msg.Rows); err != nil {
		return 125, err
	}
	exit := func() (int, error) {
		select {
		case result := <-waiting.Result:
			if result.Error != nil {
				return 125, errors.New(result.Error.Message)
			}
			return int(result.StatusCode), nil
		case err := <-waiting.Error:
			return 125, err
		case <-waitCtx.Done():
			return 125, waitCtx.Err()
		}
	}
	monitor := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		current, err := cli.ContainerInspect(ctx, msg.Container, client.ContainerInspectOptions{})
		if err != nil {
			return err
		}
		if current.Container.ID != msg.Container || current.Container.State == nil || !current.Container.State.Running || current.Container.State.Paused || current.Container.State.StartedAt != started {
			return errors.New("debug target exited, paused or restarted; reconnect to the intended target")
		}
		// An exec that cannot complete never renews the lease. The watchdog
		// expires only this helper if its Windows client disappears or stalls.
		beat, err := cli.ExecCreate(ctx, created.ID, client.ExecCreateOptions{Cmd: []string{"/bin/sh", "-c", debugHeartbeat}})
		if err != nil {
			return err
		}
		_, err = cli.ExecStart(ctx, beat.ID, client.ExecStartOptions{Detach: true})
		if err != nil {
			return err
		}
		return nil
	}
	fmt.Fprintln(output, "OrbWin debug tools. Target files: /proc/1/root. Changes there affect the target; no packages were installed in it.")
	return interact(attached.HijackedResponse, scanner, output, resize, exit, monitor)
}
