package localnet

import (
	"context"
	"errors"
	"io"
	"net"
	"orbwin.local/session/internal/landatagram"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanlease"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type referencePacketConnection struct{ closeErr error }

func (c *referencePacketConnection) ReadPacket([]byte) (int, error)    { return 0, io.EOF }
func (c *referencePacketConnection) WritePacket(p []byte) (int, error) { return len(p), nil }
func (c *referencePacketConnection) Close() error                      { return c.closeErr }
func (c *referencePacketConnection) SetDeadline(time.Time) error       { return nil }
func (c *referencePacketConnection) SetReadDeadline(time.Time) error   { return nil }
func (c *referencePacketConnection) SetWriteDeadline(time.Time) error  { return nil }

type referenceDatagramBackend struct {
	*namespaceReferenceFake
	datagrams atomic.Int32
	closeErr  error
}

func (b *referenceDatagramBackend) DialDatagram(context.Context, ServiceRoute) (DatagramConnection, error) {
	b.datagrams.Add(1)
	return &referencePacketConnection{closeErr: b.closeErr}, nil
}
func TestLANUDPAndTCPShareCombinedCapacityAcrossLeasesWithUDPSubcap(t *testing.T) {
	b := &referenceDatagramBackend{namespaceReferenceFake: &namespaceReferenceFake{dialFactory: func() (net.Conn, error) { return &referenceMemoryConn{}, nil }}}
	s := newSharedNamespaces(b, nil, nil)
	a, c := strings.Repeat("c", 64), strings.Repeat("d", 64)
	h := referenceBorrow(t, s, a)
	referenceBorrow(t, s, c)
	referenceBorrow(t, s, httpNamespaceOwner)
	r := ServiceRoute{ContainerID: h.Intent.ContainerID, StartedAt: h.Intent.StartedAt, HelperID: h.ID, Port: 8080}
	var udp []landatagram.Connection
	var tcp []net.Conn
	for i := 0; i < 16; i++ {
		conn, e := s.dialDatagramOwner(context.Background(), a, r)
		if e != nil {
			t.Fatal(e)
		}
		udp = append(udp, conn)
	}
	if _, e := s.dialDatagramOwner(context.Background(), c, r); e == nil || b.datagrams.Load() != 16 {
		t.Fatal("UDP per-helper subcap bypassed through second lease")
	}
	for i := 0; i < 16; i++ {
		conn, e := s.dialOwner(context.Background(), c, r)
		if e != nil {
			t.Fatal(e)
		}
		tcp = append(tcp, conn)
	}
	if _, e := s.dialOwner(context.Background(), a, r); e == nil || b.dials.Load() != 16 {
		t.Fatal("combined UDP/TCP capacity bypassed")
	}
	http, e := s.Dial(context.Background(), r)
	if e != nil {
		t.Fatal("changed existing HTTP quota")
	}
	http.Close()
	udp[0].Close()
	replacement, e := s.dialDatagramOwner(context.Background(), c, r)
	if e != nil {
		t.Fatal("positive datagram revocation did not release quota")
	}
	udp = append(udp, replacement)
	for _, conn := range udp {
		_ = conn.Close()
	}
	for _, conn := range tcp {
		_ = conn.Close()
	}
	if s.release(context.Background(), a, h) != nil || s.release(context.Background(), c, h) != nil || s.Release(context.Background(), h) != nil || s.Close(context.Background()) != nil || b.releases.Load() != 1 {
		t.Fatal("typed UDP references were not retired exactly once")
	}
}
func TestLANUDPUnknownRevocationRetainsCombinedSlotAndNoDeleteFallback(t *testing.T) {
	b := &referenceDatagramBackend{namespaceReferenceFake: &namespaceReferenceFake{}, closeErr: errors.New("exact exec exit unconfirmed")}
	settled := 0
	s := newSharedNamespaces(b, func(context.Context, NamespaceHandle) error { settled++; return nil }, nil)
	owner := strings.Repeat("c", 64)
	h := referenceBorrow(t, s, owner)
	referenceBorrow(t, s, httpNamespaceOwner)
	r := ServiceRoute{ContainerID: h.Intent.ContainerID, StartedAt: h.Intent.StartedAt, HelperID: h.ID, Port: 8080}
	conn, e := s.dialDatagramOwner(context.Background(), owner, r)
	if e != nil {
		t.Fatal(e)
	}
	if conn.Close() == nil {
		t.Fatal("unknown exec revocation became success")
	}
	if _, e = s.dialOwner(context.Background(), owner, r); e == nil {
		t.Fatal("poisoned UDP helper admitted TCP")
	}
	if s.release(context.Background(), owner, h) != nil || s.Release(context.Background(), h) == nil || b.releases.Load() != 0 {
		t.Fatal("unknown UDP used delete fallback")
	}
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if s.Close(short) == nil || b.closes.Load() != 0 {
		t.Fatal("lost retained common owner")
	}
	if s.settleAbsent(context.Background(), httpNamespaceOwner, h) != nil {
		t.Fatal("positive exact absent/settled callback could not retire helper")
	}
	bounded, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if s.Wait(bounded) != nil || settled != 1 || b.releases.Load() != 0 {
		t.Fatal("settlement retried physical deletion")
	}
}
func TestLANActorSelectsTypedUDPLaunchAndStillJoinsBeforeRetirement(t *testing.T) {
	f := makeLANActor(t)
	f.stage.Base.Scope.Protocol = "udp"
	f.stage.Publication.Protocol = "udp"
	prepared := f.helper.prepared
	prepared.Spec.Request = f.stage.Base
	prepared.Spec.Publication = f.stage.Publication
	lease, e := lanlease.NewOwner(f.stage.Base.Installation, f.observation.Manager).Prepare(prepared.Spec, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	prepared.LeaseSHA = lease.Snapshot(time.Now()).LeaseSHA
	f.helper.prepared = prepared
	f.helper.rule.LeaseSHA = prepared.LeaseSHA
	f.helper.rule.ReviewedScopeSHA = f.stage.Base.Scope.Digest()
	f.observation.Scope = f.stage.Base.Scope
	f.observation.Publication = f.stage.Publication
	var udp atomic.Int32
	f.controller.adapters.LaunchDatagram = func(context.Context, lanhelper.Prepared, func(context.Context, lanhelper.Stream) (landatagram.Connection, error)) (LANWorker, error) {
		udp.Add(1)
		return f.worker, nil
	}
	f.controller.adapters.Launch = func(context.Context, lanhelper.Prepared, func(context.Context, lanhelper.Stream) (net.Conn, error)) (LANWorker, error) {
		t.Fatal("UDP used TCP fallback")
		return nil, nil
	}
	status, e := f.controller.start(context.Background(), f.stage)
	if e != nil || status.Phase != "ready" || udp.Load() != 1 {
		t.Fatalf("typed UDP launch %v %+v", e, status)
	}
	close(f.worker.joined)
	bounded, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if f.controller.Close(bounded) != nil || f.helper.retires.Load() != 0 || f.base.releases.Load() != 1 {
		t.Fatal("UDP teardown did not join ingress without administrative prompts")
	}
	if f.controller.retire(bounded, status.LeaseSHA, nil) != nil || f.helper.retires.Load() != 1 {
		t.Fatal("explicit UDP retirement did not remove the exact dormant rule")
	}
}
