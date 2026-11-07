//go:build windows

package lanworker

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"orbwin.local/session/internal/landatagram"
	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanlease"
	"orbwin.local/session/internal/lanobserve"
	"orbwin.local/session/internal/lanprofile"
)

// NativeChild holds the exact created process/job and its fixed pipe. Stop is
// idempotent; only Wait is a positive child-exit AND raw-flow/goroutine join.
// It is an ordinary child, not a privileged service or administrative helper.
type NativeChild struct {
	mu                sync.Mutex
	write             sync.Mutex
	prepared          lanhelper.Prepared
	manager, child    *heldNativeProcess
	held              []*os.File
	process, job      windows.Handle
	assigned          bool
	listener          net.Listener
	control           net.Conn
	connections       map[net.Conn]bool
	channels          int
	datagrams         map[landatagram.Connection]bool
	sequence          uint64
	replay            streamReplay
	granted, stopping bool
	ctx               context.Context
	cancel            context.CancelFunc
	once              sync.Once
	workers           sync.WaitGroup
	done              chan struct{}
	hello             chan struct{}
	acks              chan lanhelper.Frame
	result            error
	dial              func(context.Context, lanhelper.Stream) (net.Conn, error)
	dialDatagram      func(context.Context, lanhelper.Stream) (landatagram.Connection, error)
}

func openStagedNative(prepared lanhelper.Prepared, manager *heldNativeProcess) ([]*os.File, error) {
	s := prepared.Spec
	var held []*os.File
	fail := func(e error) ([]*os.File, error) {
		for _, f := range held {
			f.Close()
		}
		return nil, e
	}
	root, e := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if e != nil {
		return nil, e
	}
	if s.WorkerRoot != filepath.Join(root, "OrbWin", "LAN") || !canonicalNonce(s.Request.Installation) || !canonicalNonce(s.LeaseID) || s.Worker.Path != filepath.Join(s.WorkerRoot, s.Request.Installation, s.LeaseID, "orbwin-session.exe") {
		return nil, errors.New("fixed protected staging path required")
	}
	for _, dir := range []string{root, filepath.Join(root, "OrbWin"), s.WorkerRoot, filepath.Join(s.WorkerRoot, s.Request.Installation), filepath.Dir(s.Worker.Path)} {
		f, e := holdNativeFile(dir, true)
		if e != nil {
			return fail(e)
		}
		held = append(held, f)
		if dir != root {
			if e = exactProtectedDescriptor(f, manager.identity.UserSID, true); e != nil {
				return fail(e)
			}
		}
	}
	image, e := holdNativeFile(s.Worker.Path, false)
	if e != nil {
		return fail(e)
	}
	held = append(held, image)
	if e = exactProtectedDescriptor(image, manager.identity.UserSID, false); e != nil {
		return fail(e)
	}
	got, e := nativeWorkerFile(image, s.Worker.Path, s.Worker.InterfaceAlias)
	if e != nil || got != s.Worker {
		return fail(errors.New("protected staged worker identity differs"))
	}
	record, e := holdNativeFile(filepath.Join(filepath.Dir(s.Worker.Path), "lease-v2.json"), false)
	if e != nil {
		return fail(e)
	}
	held = append(held, record)
	if e = exactProtectedDescriptor(record, manager.identity.UserSID, false); e != nil {
		return fail(e)
	}
	info, e := record.Stat()
	if e != nil || info.Size() > lanhelper.FrameBudget+4 {
		return fail(errors.New("protected staging record exceeds budget"))
	}
	r := io.NewSectionReader(record, 0, info.Size())
	frame, e := lanhelper.Read(r)
	if e != nil || frame.Kind != "prepared" || frame.Sequence != 2 || !reflect.DeepEqual(frame.Prepared, &prepared) {
		return fail(errors.New("protected staging record differs from prepared lease"))
	}
	var extra [1]byte
	if n, _ := r.Read(extra[:]); n != 0 {
		return fail(errors.New("trailing staging record"))
	}
	return held, nil
}

// LaunchNative returns a NONNIL child on every post-Create failure so the caller
// must retain its lease and Wait barrier. No helper/provider operation occurs.
// A suspended child enters a one-process kill-on-close job before any instruction.
func LaunchNative(ctx context.Context, prepared lanhelper.Prepared, dial func(context.Context, lanhelper.Stream) (net.Conn, error)) (*NativeChild, error) {
	return launchNative(ctx, prepared, dial, nil)
}
func LaunchDatagramNative(ctx context.Context, prepared lanhelper.Prepared, dial func(context.Context, lanhelper.Stream) (landatagram.Connection, error)) (*NativeChild, error) {
	return launchNative(ctx, prepared, nil, dial)
}
func launchNative(ctx context.Context, prepared lanhelper.Prepared, dial func(context.Context, lanhelper.Stream) (net.Conn, error), datagram func(context.Context, lanhelper.Stream) (landatagram.Connection, error)) (*NativeChild, error) {
	prepared.Spec.Request.Scope.ClientNetworks = append([]string(nil), prepared.Spec.Request.Scope.ClientNetworks...)
	if prepared.Spec.Request.Scope.Protocol == "tcp" && dial == nil || prepared.Spec.Request.Scope.Protocol == "udp" && datagram == nil {
		return nil, errors.New("fixed owned namespace dial required")
	}
	manager, e := holdNativeProcess(windows.GetCurrentProcessId())
	if e != nil {
		return nil, e
	}
	fail := func(e error) (*NativeChild, error) { manager.close(); return nil, e }
	if e = lanhelper.ValidateStage(lanhelper.StageRequest{Base: prepared.Spec.Request, Publication: prepared.Spec.Publication}, manager.identity, time.Now()); e != nil {
		return fail(e)
	}
	policy, e := lanlease.NewOwner(prepared.Spec.Request.Installation, manager.identity).Prepare(prepared.Spec, time.Now())
	if e != nil || prepared.Sequence != 2 || policy.Snapshot(time.Now()).LeaseSHA != prepared.LeaseSHA {
		return fail(errors.New("prepared worker lease digest differs"))
	}
	held, e := openStagedNative(prepared, manager)
	if e != nil {
		return fail(e)
	}
	cleanup := func(e error) (*NativeChild, error) {
		for _, f := range held {
			f.Close()
		}
		return fail(e)
	}
	name, e := lanhelper.PipeName(prepared.Spec.Request.Installation, prepared.Spec.LeaseID)
	if e != nil {
		return cleanup(e)
	}
	listener, e := winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;" + manager.identity.UserSID + ")", MessageMode: true, InputBufferSize: lanhelper.FrameBudget, OutputBufferSize: lanhelper.FrameBudget})
	if e != nil {
		return cleanup(e)
	}
	job, e := windows.CreateJobObject(nil, nil)
	if e != nil {
		listener.Close()
		return cleanup(e)
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS | windows.JOB_OBJECT_LIMIT_DIE_ON_UNHANDLED_EXCEPTION
	limits.BasicLimitInformation.ActiveProcessLimit = 1
	if _, e = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); e != nil {
		windows.CloseHandle(job)
		listener.Close()
		return cleanup(e)
	}
	app, e := windows.UTF16PtrFromString(prepared.Spec.Worker.Path)
	if e != nil {
		windows.CloseHandle(job)
		listener.Close()
		return cleanup(e)
	}
	command, e := windows.UTF16PtrFromString(windows.ComposeCommandLine([]string{prepared.Spec.Worker.Path}))
	if e != nil {
		windows.CloseHandle(job)
		listener.Close()
		return cleanup(e)
	}
	// No inherited handles, arbitrary arguments, Docker/proxy environment, or
	// caller-selected working directory. The worker does not execute subprocesses.
	env, e := ordinaryWorkerEnvironment()
	if e != nil {
		windows.CloseHandle(job)
		listener.Close()
		return cleanup(e)
	}
	startup := windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{}))}
	var pi windows.ProcessInformation
	systemDirectory, e := windows.GetSystemDirectory()
	if e != nil {
		windows.CloseHandle(job)
		listener.Close()
		return cleanup(e)
	}
	directory, e := windows.UTF16PtrFromString(systemDirectory)
	if e != nil {
		windows.CloseHandle(job)
		listener.Close()
		return cleanup(e)
	}
	if e = windows.CreateProcess(app, command, nil, nil, false, windows.CREATE_SUSPENDED|windows.CREATE_NO_WINDOW|windows.CREATE_UNICODE_ENVIRONMENT, &env[0], directory, &startup, &pi); e != nil {
		windows.CloseHandle(job)
		listener.Close()
		return cleanup(e)
	}
	c := &NativeChild{prepared: prepared, manager: manager, held: held, process: pi.Process, job: job, listener: listener, connections: make(map[net.Conn]bool), done: make(chan struct{}), hello: make(chan struct{}), acks: make(chan lanhelper.Frame, 1), dial: dial}
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.dialDatagram = datagram
	c.datagrams = make(map[landatagram.Connection]bool)
	defer windows.CloseHandle(pi.Thread)
	if e = windows.AssignProcessToJobObject(job, pi.Process); e != nil {
		c.Stop()
		return c, errors.New("ordinary child job assignment failed")
	}
	c.assigned = true
	c.child, e = holdNativeProcess(pi.ProcessId)
	if e != nil {
		c.Stop()
		return c, e
	}
	var created, exited, kernel, user windows.Filetime
	if e = windows.GetProcessTimes(pi.Process, &created, &exited, &kernel, &user); e != nil || !createdHandleMatches(c.child, created) {
		c.Stop()
		return c, errors.New("opened child differs from exact CreateProcess handle incarnation")
	}
	if !sameNativeSession(manager, c.child) || c.child.identity.UserSID != manager.identity.UserSID || c.child.identity.ImageSHA256 != prepared.Spec.Worker.SHA256 {
		c.Stop()
		return c, errors.New("created ordinary child identity differs")
	}
	file, e := nativeWorkerFile(c.child.image, c.child.path, prepared.Spec.Worker.InterfaceAlias)
	if e != nil || file != prepared.Spec.Worker {
		c.Stop()
		return c, errors.New("created ordinary child file identity differs")
	}
	if e = ctx.Err(); e != nil {
		c.Stop()
		return c, e
	}
	if n, e := windows.ResumeThread(pi.Thread); e != nil || n != 1 {
		c.Stop()
		return c, errors.New("created child resume contract differs")
	}
	// Listener setup is owned before any asynchronous Stop/Wait admission.
	c.workers.Add(2)
	go c.accept()
	go func() {
		defer c.workers.Done()
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-c.hello:
		case <-c.ctx.Done():
		case <-ctx.Done():
			c.Stop()
		case <-timer.C:
			c.Stop()
		}
	}()
	return c, nil
}

func createdHandleMatches(child *heldNativeProcess, created windows.Filetime) bool {
	return child != nil && child.identity.CreationTime == strconv.FormatUint(uint64(created.HighDateTime)<<32|uint64(created.LowDateTime), 10)
}
func ordinaryWorkerEnvironment() ([]uint16, error) {
	root, e := windows.GetSystemWindowsDirectory()
	if e != nil {
		return nil, e
	}
	// The fixed worker uses no temporary files, command discovery, DLL override,
	// home config or plugin loading. Unicode environment block has a double NUL.
	return utf16.Encode([]rune("SystemRoot=" + root + "\x00WINDIR=" + root + "\x00\x00")), nil
}
func (c *NativeChild) Identity() lanfirewall.CallerIdentity {
	if c.child == nil {
		return lanfirewall.CallerIdentity{}
	}
	return c.child.identity
}

// Observation derives native process/file/channel/interface facts from held
// handles. Source publication and the existing ledger's exclusive owner are
// independently rechecked by the manager actor before this becomes eligible.
func (c *NativeChild) Observation(ctx context.Context) (lanlease.Observation, error) {
	c.mu.Lock()
	if c.stopping || c.child == nil || c.control == nil {
		c.mu.Unlock()
		return lanlease.Observation{}, errors.New("ordinary child native observation unavailable")
	}
	c.workers.Add(1)
	conn := c.control
	c.mu.Unlock()
	defer c.workers.Done()
	if e := ctx.Err(); e != nil {
		return lanlease.Observation{}, e
	}
	fd, ok := conn.(interface{ Fd() uintptr })
	if !ok || !sameNativeSession(c.manager, c.child) || !c.manager.live() || !c.child.live() || nativePipePeer(fd, true, c.child) != nil {
		return lanlease.Observation{}, errors.New("native child/manager channel differs")
	}
	s := c.prepared.Spec
	file, e := nativeWorkerFile(c.child.image, c.child.path, s.Worker.InterfaceAlias)
	if e != nil || file != s.Worker {
		return lanlease.Observation{}, errors.New("held ordinary child file differs")
	}
	interfaces, e := lanobserve.Observe(ctx)
	if e != nil {
		return lanlease.Observation{}, errors.New("native LAN interface observation unavailable")
	}
	profiles, e := lanprofile.Observe(ctx, []string{s.Request.Scope.InterfaceGUID})
	if e != nil {
		return lanlease.Observation{}, errors.New("native LAN profile observation unavailable")
	}
	bound, e := lanfirewall.BindInterface(s.Request.Scope, interfaces, profiles, time.Now())
	if e != nil || bound.Alias != file.InterfaceAlias {
		return lanlease.Observation{}, errors.New("native LAN interface/profile changed")
	}
	if e = ctx.Err(); e != nil {
		return lanlease.Observation{}, e
	}
	scope := s.Request.Scope
	scope.ClientNetworks = append([]string(nil), scope.ClientNetworks...)
	read := time.Now()
	for _, raw := range []string{interfaces.ReadAt, profiles.ReadAt} {
		stamp, e := time.Parse(time.RFC3339Nano, raw)
		if e != nil {
			return lanlease.Observation{}, errors.New("native observation time unavailable")
		}
		if stamp.Before(read) {
			read = stamp
		}
	}
	return lanlease.Observation{ReadAt: read, Manager: c.manager.identity, Worker: c.child.identity, WorkerFile: file, Publication: s.Publication, Scope: scope, ManagerPipePID: c.manager.identity.PID, WorkerPipePID: c.child.identity.PID, ManagerAlive: true, WorkerAlive: true, WorkerFileHeld: true, ProtectedPath: true, PublicationActive: true, InterfacePresent: true}, nil
}
func (c *NativeChild) WaitHello(ctx context.Context) error {
	select {
	case <-c.hello:
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.stopping {
			return errors.New("ordinary worker closed during hello")
		}
		return nil
	case <-c.ctx.Done():
		return errors.New("ordinary worker closed before hello")
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (c *NativeChild) accept() {
	defer c.workers.Done()
	for {
		conn, e := c.listener.Accept()
		if e != nil {
			c.Stop()
			return
		}
		c.mu.Lock()
		if c.stopping || c.channels >= lanhelper.MaxStreams+1 {
			c.mu.Unlock()
			conn.Close()
			continue
		}
		c.connections[conn] = true
		c.channels++
		c.workers.Add(1)
		c.mu.Unlock()
		go c.receive(conn)
	}
}
func (c *NativeChild) receive(conn net.Conn) {
	defer c.workers.Done()
	defer func() { conn.Close(); c.mu.Lock(); delete(c.connections, conn); c.channels--; c.mu.Unlock() }()
	fd, ok := conn.(interface{ Fd() uintptr })
	if !ok || nativePipePeer(fd, true, c.child) != nil {
		return
	}
	if conn.SetDeadline(time.Now().Add(5*time.Second)) != nil {
		return
	}
	f, e := lanhelper.Read(conn)
	if e != nil || f.LeaseSHA != c.prepared.LeaseSHA {
		return
	}
	if f.Kind == "worker-hello" {
		c.mu.Lock()
		if c.stopping || c.control != nil || f.Sequence != 1 || f.Child.Identity != c.child.identity {
			c.mu.Unlock()
			return
		}
		c.control = conn
		close(c.hello)
		c.mu.Unlock()
		// Every grant/refresh needs a matching completed worker acknowledgement;
		// writing bytes alone does not prove the worker accepted or bound ingress.
		_ = conn.SetReadDeadline(time.Time{})
		for {
			ack, e := lanhelper.Read(conn)
			if e != nil || ack.Kind != "worker-ready" || ack.LeaseSHA != c.prepared.LeaseSHA {
				c.Stop()
				return
			}
			select {
			case c.acks <- ack:
			case <-c.ctx.Done():
				return
			default:
				c.Stop()
				return
			}
		}
	}
	c.mu.Lock()
	udp := c.prepared.Spec.Request.Scope.Protocol == "udp"
	expectedKind := "stream-open"
	if udp {
		expectedKind = "datagram-open"
	}
	allowed := !c.stopping && c.granted && f.Kind == expectedKind && c.replay.accept(f.Stream.ID)
	c.mu.Unlock()
	if !allowed || !nativeReviewedPeer(c.prepared.Spec.Request.Scope, f.Stream.Peer) {
		return
	}
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()
	if udp {
		c.receiveDatagram(ctx, conn, f)
		return
	}
	out, e := c.dial(ctx, *f.Stream)
	if e != nil || out == nil {
		if out != nil {
			out.Close()
		}
		return
	}
	c.mu.Lock()
	if c.stopping {
		c.mu.Unlock()
		out.Close()
		return
	}
	c.connections[out] = true
	c.mu.Unlock()
	defer func() { out.Close(); c.mu.Lock(); delete(c.connections, out); c.mu.Unlock() }()
	ready := lanhelper.Frame{Version: 2, Kind: "stream-ready", Sequence: f.Sequence, LeaseSHA: f.LeaseSHA, Stream: &lanhelper.Stream{ID: f.Stream.ID}}
	if lanhelper.Write(conn, ready) != nil || conn.SetDeadline(time.Time{}) != nil {
		return
	}
	copyNativeStream(conn, out)
}
func nativeReviewedPeer(scope lanfirewall.Scope, raw string) bool {
	a, e := netip.ParseAddr(raw)
	if scope.Protocol == "udp" {
		peer, parseErr := netip.ParseAddrPort(raw)
		if parseErr != nil || peer.String() != raw || peer.Port() == 0 || !peer.Addr().Is4() {
			return false
		}
		a = peer.Addr()
		e = nil
		raw = a.String()
	}
	if e != nil || a.String() != raw {
		return false
	}
	for _, s := range scope.ClientNetworks {
		p, e := netip.ParsePrefix(s)
		if e == nil && p.Contains(a) {
			return true
		}
	}
	return false
}
func copyNativeStream(a, b net.Conn) {
	done := make(chan struct{}, 2)
	copyOne := func(dst, src net.Conn) {
		_, e := io.CopyBuffer(dst, src, make([]byte, 32<<10))
		if e == nil {
			if half, ok := dst.(interface{ CloseWrite() error }); ok {
				e = half.CloseWrite()
			} else {
				e = errors.New("raw transport lacks write-half close")
			}
		}
		if e != nil {
			a.Close()
			b.Close()
		}
		done <- struct{}{}
	}
	go copyOne(a, b)
	copyOne(b, a)
	<-done
	<-done
}
func (c *NativeChild) Send(ctx context.Context, frame lanhelper.Frame) error {
	c.mu.Lock()
	if c.stopping {
		c.mu.Unlock()
		return errors.New("ordinary worker closed")
	}
	c.workers.Add(1)
	c.mu.Unlock()
	defer c.workers.Done()
	c.write.Lock()
	defer c.write.Unlock()
	c.mu.Lock()
	if c.stopping || c.control == nil {
		c.mu.Unlock()
		return errors.New("ordinary worker hello is not ready")
	}
	if frame.Kind != "worker-grant" && frame.Kind != "refresh" {
		c.mu.Unlock()
		return errors.New("fixed worker control required")
	}
	if frame.LeaseSHA != c.prepared.LeaseSHA || frame.Sequence != c.sequence+1 {
		c.mu.Unlock()
		c.Stop()
		return errors.New("worker control binding/revision differs")
	}
	conn := c.control
	c.mu.Unlock()
	deadline := time.Now().Add(time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if e := ctx.Err(); e != nil {
		c.Stop()
		return e
	}
	if e := conn.SetWriteDeadline(deadline); e != nil {
		c.Stop()
		return e
	}
	if e := lanhelper.Write(conn, frame); e != nil {
		c.Stop()
		return e
	}
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case ack := <-c.acks:
		if ack.Sequence != frame.Sequence {
			c.Stop()
			return errors.New("worker completion sequence differs")
		}
	case <-ctx.Done():
		c.Stop()
		return ctx.Err()
	case <-c.ctx.Done():
		return errors.New("worker closed before acknowledgement")
	case <-timer.C:
		c.Stop()
		return errors.New("worker acknowledgement deadline exceeded")
	}
	c.mu.Lock()
	c.sequence = frame.Sequence
	if frame.Kind == "worker-grant" {
		c.granted = true
	}
	c.mu.Unlock()
	return nil
}
func (c *NativeChild) Stop() {
	c.once.Do(func() {
		c.mu.Lock()
		c.stopping = true
		c.cancel()
		listener := c.listener
		connections := make([]net.Conn, 0, len(c.connections))
		datagrams := make([]landatagram.Connection, 0, len(c.datagrams))
		for datagram := range c.datagrams {
			datagrams = append(datagrams, datagram)
		}
		for conn := range c.connections {
			connections = append(connections, conn)
		}
		c.mu.Unlock()
		go func() {
			// Closing the exact unnamed kill-on-close job terminates only this child.
			// Before job assignment failure, terminate only the exact created handle.
			if !c.assigned {
				c.result = errors.Join(c.result, windows.TerminateProcess(c.process, 125))
			}
			if c.job != 0 {
				c.result = errors.Join(c.result, windows.CloseHandle(c.job))
			}
			if listener != nil {
				listener.Close()
			}
			// Namespace Close may perform a bounded remote revocation. Contain
			// the exact child FIRST, and keep every revocation inside this join.
			for _, conn := range connections {
				conn.Close()
			}
			for _, datagram := range datagrams {
				datagram.Close()
			}
			for {
				state, e := windows.WaitForSingleObject(c.process, 1000)
				if e == nil && state == windows.WAIT_OBJECT_0 {
					break
				}
				if e != nil {
					c.result = errors.Join(c.result, errors.New("exact child exit remains unconfirmed"))
					return
				}
			}
			c.workers.Wait()
			windows.CloseHandle(c.process)
			if c.child != nil {
				c.child.close()
			}
			c.manager.close()
			for _, f := range c.held {
				f.Close()
			}
			close(c.done)
		}()
	})
}
func (c *NativeChild) Wait(ctx context.Context) error {
	select {
	case <-c.done:
		return c.result
	default:
	}
	select {
	case <-c.done:
		return c.result
	case <-ctx.Done():
		return ctx.Err()
	}
}
