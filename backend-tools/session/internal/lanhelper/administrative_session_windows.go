//go:build windows && amd64

package lanhelper

import (
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"sync"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanlease"
)

// One consumed review owns both channels and all late child returns. Closing
// authentication precedes stopping the ordinary launcher. Timeout is never an
// elevated-process exit acknowledgement. No method retries native mutations.
type adminNativeSession struct {
	client         *AdministrativeClient
	record         OwnerReview
	ctx            context.Context
	cancel         context.CancelFunc
	mu             sync.Mutex
	closing        bool
	listeners      []net.Listener
	connections    []net.Conn
	launcher       *ordinaryLauncher
	helper         *elevatedProcess
	control        net.Conn
	opened         chan struct{}
	done           chan struct{} // Cleanup worker completion; only a nil result proves successful native join/release.
	acceptors      sync.WaitGroup
	pulse          sync.WaitGroup
	closeOnce      sync.Once
	finishOnce     sync.Once
	write          sync.Mutex
	rpc            sync.Mutex
	sent, received uint64
	prepared       Prepared
	children       []adminChildLifetime // At most the actual ordinary launcher and elevated helper.
	result         error
	diagnostics    []string // Pre-release native values are diagnostic only, never retry authority.
}

type adminChildLifetime interface {
	Wait(context.Context) error
	release() error
}

func joinAdminChildren(children []adminChildLifetime) (error, []string) {
	if len(children) > 2 {
		return errors.New("native administrative child ownership exceeds its fixed pair"), nil
	}
	var result error
	var diagnostics []string
	for _, child := range children {
		var diagnostic string
		switch native := child.(type) {
		case *ordinaryLauncher:
			diagnostic = fmt.Sprintf("ordinary launcher process=%#x job=%#x", native.process, native.job)
		case *elevatedProcess:
			var imageHandle uintptr
			if native.image != nil {
				imageHandle = native.image.Fd()
			}
			diagnostic = fmt.Sprintf("elevated helper process=%#x image=%#x", native.process, imageHandle)
		default:
			diagnostic = fmt.Sprintf("administrative child %T", child)
		}
		if err := child.Wait(context.Background()); err != nil {
			result = errors.Join(result, fmt.Errorf("administrative child wait failed: %w", err))
			diagnostics = append(diagnostics, diagnostic)
			continue // Unknown exit cannot authorize releasing this child.
		}
		if err := child.release(); err != nil {
			result = errors.Join(result, fmt.Errorf("administrative child release failed: %w", err))
			diagnostics = append(diagnostics, diagnostic)
		} // A checked release is never retried, including after an unknown result.
	}
	return result, diagnostics
}

func (s *adminNativeSession) requestClose() {
	s.cancel()
	s.mu.Lock()
	s.closing = true
	for _, listener := range s.listeners {
		_ = listener.Close()
	}
	for _, conn := range s.connections {
		_ = conn.Close()
	}
	if s.launcher != nil {
		s.launcher.Stop()
	}
	s.mu.Unlock()
}
func (s *adminNativeSession) closeAndJoin() error {
	s.closeOnce.Do(func() {
		s.requestClose()
		go func() {
			<-s.opened
			s.acceptors.Wait()
			s.pulse.Wait()
			if s.launcher != nil {
				s.launcher.Stop()
			}
			s.result, s.diagnostics = joinAdminChildren(s.children)
			close(s.done)
		}()
	})
	<-s.done
	return s.result
}
func (s *adminNativeSession) finish() error {
	s.finishOnce.Do(func() {
		err := s.closeAndJoin()
		s.client.mu.Lock()
		if err != nil {
			s.client.result = errors.Join(s.client.result, err)
		} else if s.client.current == s {
			s.client.current = nil
		}
		s.client.mu.Unlock()
		s.client.sessions.Done()
	})
	return s.result
}
func (s *adminNativeSession) listen(name string) (<-chan net.Conn, error) {
	descriptor := "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;" + s.client.manager.identity.UserSID + ")"
	listener, err := winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: descriptor, InputBufferSize: FrameBudget + 4, OutputBufferSize: FrameBudget + 4})
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.listeners = append(s.listeners, listener)
	if s.closing {
		_ = listener.Close()
	}
	s.mu.Unlock()
	accepted := make(chan net.Conn, 1)
	s.acceptors.Add(1)
	go func() {
		defer s.acceptors.Done()
		defer close(accepted)
		conn, e := listener.Accept()
		if e != nil {
			return
		}
		s.ownConnection(conn)
		accepted <- conn
	}()
	return accepted, nil
}
func (s *adminNativeSession) ownConnection(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connections = append(s.connections, conn)
	if s.closing {
		_ = conn.Close()
	}
}
func (s *adminNativeSession) accept(channel <-chan net.Conn) (net.Conn, error) {
	select {
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	case conn := <-channel:
		if conn == nil {
			return nil, errors.New("fixed native channel unavailable")
		}
		return conn, nil
	}
}
func pipeHandle(conn net.Conn) (windows.Handle, error) {
	fd, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return 0, errors.New("native pipe handle unavailable")
	}
	return windows.Handle(fd.Fd()), nil
}
func (s *adminNativeSession) open() error {
	defer close(s.opened)
	name, _ := AdminPipeName(s.client.authority.installation, s.record.Transaction)
	admin, err := s.listen(name)
	if err != nil {
		return err
	}
	name, _ = launchPipeName(s.client.authority.installation, s.record.Transaction)
	launch, err := s.listen(name)
	if err != nil {
		return err
	}
	launcher, err := startOrdinaryLauncher(s.ctx, s.client.authority, s.record.Transaction)
	s.mu.Lock()
	s.launcher = launcher
	if launcher != nil {
		s.children = append(s.children, launcher)
	}
	if s.closing && launcher != nil {
		launcher.Stop()
	}
	s.mu.Unlock()
	if err != nil {
		return err
	}
	// This deadline only closes authority channels and the ordinary launcher.
	// Any retained elevated process is still joined independently below.
	limit := time.AfterFunc(60*time.Second, s.requestClose)
	defer limit.Stop()
	conn, err := s.accept(launch)
	if err != nil {
		return err
	}
	handle, err := pipeHandle(conn)
	if err != nil {
		return err
	}
	// Identification is possible only after the client has written its frame.
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	frame, err := readLaunch(conn)
	if err != nil {
		return err
	}
	if err = verifyLauncherPipe(handle, launcher, s.client.authority); err != nil {
		return err
	}
	if frame.Kind != "handle" {
		return errors.New("elevation launch did not return retained native process")
	}
	helper, err := launcher.duplicateHelper(frame)
	if helper != nil {
		s.mu.Lock()
		s.helper = helper
		s.children = append(s.children, helper)
		s.mu.Unlock()
	}
	if err != nil {
		return err
	}
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err = writeLaunch(conn, launchFrame{Version: 1, Kind: "received"}); err != nil {
		return err
	}
	_ = conn.Close()
	if err = launcher.Wait(s.ctx); err != nil {
		return err
	}
	control, err := s.accept(admin)
	if err != nil {
		return err
	}
	handle, err = pipeHandle(control)
	if err != nil {
		return err
	}
	// Receive only a fixed empty greeting before identifying the native peer.
	// No review is sent until the exact held elevated process is authenticated.
	_ = control.SetReadDeadline(time.Now().Add(5 * time.Second))
	greeting, err := readLaunch(control)
	if err != nil || greeting.Kind != "helper-ready" {
		return errors.New("fixed native helper greeting unavailable")
	}
	if err = verifyElevatedPipe(handle, helper, s.client.authority); err != nil {
		return err
	}
	_ = control.SetReadDeadline(time.Time{})
	s.control = control
	if _, err = s.send(AdminFrame{Kind: "review", Review: &s.record}); err != nil {
		return err
	}
	s.pulse.Add(1)
	go func() {
		defer s.pulse.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case <-ticker.C:
				if _, e := s.send(AdminFrame{Kind: "pulse"}); e != nil {
					s.requestClose()
					return
				}
			}
		}
	}()
	_, err = s.response(1, "review-accepted")
	return err
}
func (s *adminNativeSession) send(frame AdminFrame) (uint64, error) {
	s.write.Lock()
	defer s.write.Unlock()
	if err := s.ctx.Err(); err != nil {
		return 0, err
	}
	if s.sent >= 1024 {
		return 0, errors.New("one-shot native message budget exceeded")
	}
	s.sent++
	frame.Version = 2
	frame.Sequence = s.sent
	if err := s.control.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return 0, err
	}
	return frame.Sequence, WriteAdmin(s.control, frame)
}
func (s *adminNativeSession) response(sequence uint64, kind string) (AdminFrame, error) {
	frame, err := ReadAdmin(s.control)
	if err != nil {
		return AdminFrame{}, err
	}
	if frame.Sequence != s.received+1 || frame.RequestSequence != sequence {
		return AdminFrame{}, errors.New("native reply correlation differs")
	}
	s.received = frame.Sequence
	if frame.Kind != kind {
		return AdminFrame{}, errors.New("native operation acknowledgement unavailable")
	}
	return frame, nil
}
func (s *adminNativeSession) exchange(frame AdminFrame, kind string) (AdminFrame, error) {
	seq, err := s.send(frame)
	if err != nil {
		return AdminFrame{}, err
	}
	return s.response(seq, kind)
}

func (client *AdministrativeClient) PersistReview(ctx context.Context, record OwnerReview) error {
	if !client.exactOwner() {
		return errNativeOwnerCopy
	}
	if ctx == nil || record.Validate(client.manager.identity, time.Now()) != nil || record.Stage.Base.Installation != client.authority.installation {
		return errors.New("exact native owner review required")
	}
	client.mu.Lock()
	if client.closed || client.current != nil || client.used[record.ReviewID] || len(client.used) >= 4096 {
		client.mu.Unlock()
		return errors.New("native review owner busy or permanently consumed")
	}
	operation, cancel := context.WithTimeout(ctx, 120*time.Second)
	s := &adminNativeSession{client: client, record: record, ctx: operation, cancel: cancel, opened: make(chan struct{}), done: make(chan struct{})}
	client.current = s
	client.used[record.ReviewID] = true
	client.sessions.Add(1)
	client.mu.Unlock()
	// The session owns cancellation cleanup even when caller exits between
	// PersistReview and Stage/Commit. WG is released only after all actual joins.
	stopClient := context.AfterFunc(client.ctx, s.requestClose)
	go func() { <-s.ctx.Done(); s.requestClose(); _ = s.finish(); stopClient() }()
	err := s.open()
	if err != nil {
		s.requestClose()
		err = errors.Join(err, s.finish())
	}
	return err
}
func (client *AdministrativeClient) currentSession(ctx context.Context) (*adminNativeSession, error) {
	if !client.exactOwner() {
		return nil, errNativeOwnerCopy
	}
	if ctx == nil {
		return nil, errors.New("operation context required")
	}
	client.mu.Lock()
	s := client.current
	client.mu.Unlock()
	if s == nil {
		return nil, errors.New("consumed native review session required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s, nil
}
func (client *AdministrativeClient) Tombstones(ctx context.Context) ([]lanlease.Tombstone, error) {
	s, err := client.currentSession(ctx)
	if err != nil {
		return nil, err
	}
	s.rpc.Lock()
	defer s.rpc.Unlock()
	var result []lanlease.Tombstone
	for page := uint32(0); page < maxProtectedReviews/TombstonePageSize; page++ {
		frame, e := s.exchange(AdminFrame{Kind: "read-tombstones", Page: &page}, "tombstones")
		if e != nil {
			s.requestClose()
			return nil, e
		}
		if frame.Page == nil || *frame.Page != page {
			s.requestClose()
			return nil, errors.New("native history page differs")
		}
		result = append(result, (*frame.Tombstones)...)
		if !*frame.More {
			return result, nil
		}
	}
	s.requestClose()
	return nil, errors.New("native history exceeds permanent denial bound")
}
func (client *AdministrativeClient) Stage(ctx context.Context, request StageRequest) (Prepared, error) {
	s, err := client.currentSession(ctx)
	if err != nil {
		return Prepared{}, err
	}
	s.rpc.Lock()
	defer s.rpc.Unlock()
	if s.record.Purpose != "activate" || !reflect.DeepEqual(request, s.record.Stage) || s.prepared.LeaseSHA != "" {
		return Prepared{}, errors.New("exact one-shot stage required")
	}
	frame, err := s.exchange(AdminFrame{Kind: "stage"}, "prepared")
	if err != nil {
		s.requestClose()
		return Prepared{}, err
	}
	if !reflect.DeepEqual(frame.Prepared.Spec.Request, request.Base) || frame.Prepared.Spec.Publication != request.Publication {
		s.requestClose()
		return Prepared{}, errors.New("prepared owner scope differs")
	}
	owner := lanlease.NewOwner(client.authority.installation, client.manager.identity)
	lease, err := owner.Prepare(frame.Prepared.Spec, time.Now())
	if err != nil || lease.Snapshot(time.Now()).LeaseSHA != frame.Prepared.LeaseSHA {
		s.requestClose()
		return Prepared{}, errors.New("prepared lease digest or private worker scope differs")
	}
	s.prepared = *frame.Prepared
	return s.prepared, nil
}
func (client *AdministrativeClient) Commit(ctx context.Context, prepared Prepared, child lanfirewall.CallerIdentity, observation lanlease.Observation) (lanlease.RuleAcknowledgement, error) {
	return lanlease.RuleAcknowledgement{}, errors.New("actual owned namespace full ID is required for native commit")
}
func (client *AdministrativeClient) CommitNamespace(ctx context.Context, prepared Prepared, child lanfirewall.CallerIdentity, observation lanlease.Observation, namespaceID string) (lanlease.RuleAcknowledgement, error) {
	s, err := client.currentSession(ctx)
	if err != nil {
		return lanlease.RuleAcknowledgement{}, err
	}
	s.rpc.Lock()
	defer s.rpc.Unlock()
	if prepared.LeaseSHA == "" || !reflect.DeepEqual(prepared, s.prepared) || !ValidSHA(namespaceID) {
		return lanlease.RuleAcknowledgement{}, errors.New("exact prepared native worker required")
	}
	frame, err := s.exchange(AdminFrame{Kind: "commit", Child: &child, Observation: &observation, NamespaceID: namespaceID}, "committed")
	s.requestClose()
	err = errors.Join(err, s.finish())
	if err != nil {
		return lanlease.RuleAcknowledgement{}, err
	}
	if frame.Rule.LeaseSHA != prepared.LeaseSHA {
		return lanlease.RuleAcknowledgement{}, errors.New("native committed lease differs")
	}
	return *frame.Rule, nil
}
func (client *AdministrativeClient) Retire(ctx context.Context, prepared Prepared, rule lanlease.RuleAcknowledgement) (lanlease.RemovalAcknowledgement, error) {
	if rule.LeaseSHA != prepared.LeaseSHA {
		return lanlease.RemovalAcknowledgement{}, errors.New("exact explicit removal rule required")
	}
	return client.finishRemoval(ctx, prepared, "remove")
}

// Settlement is a separate attended operation. Its native provider capability
// can only observe absence and seal a previous remove-intent; it cannot Delete.
func (client *AdministrativeClient) SettleRemoval(ctx context.Context, prepared Prepared) (lanlease.RemovalAcknowledgement, error) {
	return client.finishRemoval(ctx, prepared, "settle-remove")
}

func (client *AdministrativeClient) CleanupPrior(ctx context.Context, prepared Prepared, purpose string) (lanlease.RemovalAcknowledgement, error) {
	if purpose != "cleanup-remove" && purpose != "cleanup-settle" {
		return lanlease.RemovalAcknowledgement{}, errors.New("distinct prior-owner cleanup purpose required")
	}
	return client.finishRemoval(ctx, prepared, purpose)
}

func (client *AdministrativeClient) finishRemoval(ctx context.Context, prepared Prepared, purpose string) (lanlease.RemovalAcknowledgement, error) {
	s, err := client.currentSession(ctx)
	if err != nil {
		return lanlease.RemovalAcknowledgement{}, err
	}
	s.rpc.Lock()
	defer s.rpc.Unlock()
	if s.record.Purpose != purpose || s.record.LeaseSHA256 != prepared.LeaseSHA {
		return lanlease.RemovalAcknowledgement{}, errors.New("exact explicit removal review required")
	}
	frame, err := s.exchange(AdminFrame{Kind: purpose}, "removed")
	s.requestClose()
	err = errors.Join(err, s.finish())
	if err != nil {
		return lanlease.RemovalAcknowledgement{}, err
	}
	return *frame.Removal, nil
}
