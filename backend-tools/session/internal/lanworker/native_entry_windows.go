//go:build windows

package lanworker

import (
	"context"
	"errors"
	"net"
	"syscall"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"orbwin.local/session/internal/landatagram"
	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanobserve"
	"orbwin.local/session/internal/lanprofile"
)

// RunNative is ONLY the dedicated ordinary worker executable. It reads one
// protected fixed boot frame and authenticates the native pipe server against
// the retained manager handle. It has no administrative/provider/engine entry.
func RunNative(ctx context.Context) error {
	b, e := openNativeBoot()
	if e != nil {
		return errors.New("protected ordinary worker boot refused")
	}
	defer b.close()
	name, e := lanhelper.PipeName(b.prepared.Spec.Request.Installation, b.prepared.Spec.LeaseID)
	if e != nil {
		return e
	}
	connect, cancel := context.WithTimeout(ctx, 5*time.Second)
	control, e := winio.DialPipeContext(connect, name)
	cancel()
	if e != nil {
		return errors.New("fixed manager channel unavailable")
	}
	defer control.Close()
	fd, ok := control.(interface{ Fd() uintptr })
	if !ok || nativePipePeer(fd, false, b.manager) != nil {
		return errors.New("native manager pipe identity refused")
	}
	parentDone := make(chan struct{})
	monitorDone := make(chan struct{})
	monitorJoined := make(chan struct{})
	defer func() { close(monitorDone); <-monitorJoined }()
	go func() {
		defer close(monitorJoined)
		timer := time.NewTicker(100 * time.Millisecond)
		defer timer.Stop()
		for {
			select {
			case <-monitorDone:
				return
			case <-ctx.Done():
				close(parentDone)
				return
			case <-timer.C:
				if !b.manager.live() {
					close(parentDone)
					return
				}
			}
		}
	}()
	bindings := Bindings{Manager: b.manager.identity, Self: b.self.identity, WorkerFile: b.prepared.Spec.Worker}
	nativeCheck := func(ctx context.Context, scope lanfirewall.Scope) (Bindings, error) {
		if !b.manager.live() || !b.self.live() || nativePipePeer(fd, false, b.manager) != nil {
			return Bindings{}, errors.New("native worker/manager lifetime differs")
		}
		file, e := nativeWorkerFile(b.self.image, b.self.path, b.prepared.Spec.Worker.InterfaceAlias)
		if e != nil || file != b.prepared.Spec.Worker {
			return Bindings{}, errors.New("held worker identity differs")
		}
		interfaces, e := lanobserve.Observe(ctx)
		if e != nil {
			return Bindings{}, errors.New("native interface observation unavailable")
		}
		profiles, e := lanprofile.Observe(ctx, []string{scope.InterfaceGUID})
		if e != nil {
			return Bindings{}, errors.New("native profile observation unavailable")
		}
		bound, e := lanfirewall.BindInterface(scope, interfaces, profiles, time.Now())
		if e != nil || bound.Alias != file.InterfaceAlias {
			return Bindings{}, errors.New("reviewed native interface/profile differs")
		}
		return bindings, nil
	}
	dial := func(ctx context.Context, request lanhelper.Stream) (net.Conn, error) {
		conn, e := winio.DialPipeContext(ctx, name)
		if e != nil {
			return nil, e
		}
		fail := func() (net.Conn, error) { conn.Close(); return nil, errors.New("fixed namespace tunnel refused") }
		fd, ok := conn.(interface{ Fd() uintptr })
		if !ok || nativePipePeer(fd, false, b.manager) != nil {
			return fail()
		}
		deadline := time.Now().Add(5 * time.Second)
		if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
		if conn.SetDeadline(deadline) != nil {
			return fail()
		}
		kind, readyKind := "stream-open", "stream-ready"
		if b.prepared.Spec.Request.Scope.Protocol == "udp" {
			kind, readyKind = "datagram-open", "datagram-ready"
		}
		f := lanhelper.Frame{Version: 2, Kind: kind, Sequence: request.ID, LeaseSHA: b.prepared.LeaseSHA, Stream: &request}
		if lanhelper.Write(conn, f) != nil {
			return fail()
		}
		ack, e := lanhelper.Read(conn)
		if e != nil || ack.Kind != readyKind || ack.Sequence != request.ID || ack.LeaseSHA != f.LeaseSHA || ack.Stream.ID != request.ID || conn.SetDeadline(time.Time{}) != nil {
			return fail()
		}
		return conn, nil
	}
	config := &net.ListenConfig{Control: func(network, address string, raw syscall.RawConn) error {
		// Windows SDK winsock2.h: SO_EXCLUSIVEADDRUSE ((int)(~SO_REUSEADDR)).
		// This option must succeed BEFORE bind; default Go omits this option.
		return exclusiveBind(network, address, raw, func(fd uintptr) error {
			return windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, ^windows.SO_REUSEADDR, 1)
		})
	}}
	core, e := New(b.prepared.Spec, bindings, Dependencies{ParentDone: parentDone, NativeCheck: nativeCheck, Listen: func(ctx context.Context, address string) (net.Listener, error) {
		return config.Listen(ctx, "tcp4", address)
	}, Dial: dial,
		ListenPacket: func(ctx context.Context, address string) (net.PacketConn, error) {
			return config.ListenPacket(ctx, "udp4", address)
		},
		DialDatagram: func(ctx context.Context, request lanhelper.Stream) (landatagram.Connection, error) {
			conn, e := dial(ctx, request)
			if e != nil {
				return nil, e
			}
			return landatagram.New(conn), nil
		},
	})
	if e != nil {
		return e
	}
	defer func() { core.Stop(); _ = core.Wait(context.Background()) }()
	if core.LeaseSHA() != b.prepared.LeaseSHA {
		return errors.New("protected lease digest differs")
	}
	// Closure must interrupt the control pipe independently of a stalled native
	// observation; active streams are already closed by Core's TTL watcher.
	go func() {
		select {
		case <-core.stop:
			control.Close()
		case <-monitorDone:
		}
	}()
	hello := lanhelper.Frame{Version: 2, Kind: "worker-hello", Sequence: 1, LeaseSHA: core.LeaseSHA(), Child: &lanhelper.Child{Identity: b.self.identity}}
	if control.SetWriteDeadline(time.Now().Add(time.Second)) != nil || lanhelper.Write(control, hello) != nil {
		return errors.New("worker hello failed")
	}
	for {
		if control.SetReadDeadline(time.Now().Add(lanhelper.ObservationTTL)) != nil {
			return errors.New("worker heartbeat deadline unavailable")
		}
		frame, e := lanhelper.Read(control)
		if e != nil {
			return errors.New("worker control lost")
		}
		apply, cancel := context.WithTimeout(ctx, lanhelper.ObservationTTL)
		e = core.Apply(apply, frame)
		cancel()
		if e != nil {
			return e
		}
		if frame.Kind == "stop" {
			return nil
		}
		ack := lanhelper.Frame{Version: 2, Kind: "worker-ready", Sequence: frame.Sequence, LeaseSHA: core.LeaseSHA()}
		if control.SetWriteDeadline(time.Now().Add(time.Second)) != nil || lanhelper.Write(control, ack) != nil {
			return errors.New("worker completion acknowledgement lost")
		}
	}
}
