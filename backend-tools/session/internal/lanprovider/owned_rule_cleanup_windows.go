//go:build windows && amd64

package lanprovider

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/client"
	"golang.org/x/sys/windows"
)

const cleanupDockerEndpoint = "npipe:////./pipe/docker-rancher-desktop-orbwin"

type nativeCleanupSource struct {
	held                        []ownedProtectedHandle
	journal                     int
	metadata                    []byte
	events                      []byte
	sid, installation, leaseSHA string
	pipe, peer                  windows.Handle
	peerPID, session            uint32
	creation                    uint64
	peerToken                   windows.Token
	declarations                traversalDeclarations
	closed                      bool
	closeErr                    error
}

func cleanupNativePeer(ctx context.Context, pipe windows.Handle) (process windows.Handle, token windows.Token, sid string, pid, session uint32, creation uint64, err error) {
	if requireOwnedElevation(ctx) != nil || windows.GetNamedPipeServerProcessId(pipe, &pid) != nil || pid == 0 {
		return 0, 0, "", 0, 0, 0, ErrCleanupPending
	}
	process, err = windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return 0, 0, "", 0, 0, 0, ErrCleanupPending
	}
	if windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token) != nil {
		return process, token, "", 0, 0, 0, ErrCleanupPending
	}
	var elevation, size, helperSession uint32
	if windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevation)), 4, &size) != nil || size != 4 || elevation != 0 || windows.GetTokenInformation(token, windows.TokenSessionId, (*byte)(unsafe.Pointer(&session)), 4, &size) != nil || size != 4 || session == 0 || windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &helperSession) != nil || helperSession != session {
		return process, token, "", 0, 0, 0, ErrCleanupPending
	}
	user, e := token.GetTokenUser()
	if e != nil || user == nil || user.User.Sid == nil {
		return process, token, "", 0, 0, 0, ErrCleanupPending
	}
	sid = user.User.Sid.String()
	var created, exited, kernel, used windows.Filetime
	if windows.GetProcessTimes(process, &created, &exited, &kernel, &used) != nil {
		return process, token, "", 0, 0, 0, ErrCleanupPending
	}
	creation = uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
	state, e := windows.WaitForSingleObject(process, 0)
	if e != nil || state != uint32(windows.WAIT_TIMEOUT) || creation == 0 {
		return process, token, "", 0, 0, 0, ErrCleanupPending
	}
	return process, token, sid, pid, session, creation, nil
}
func (s *nativeCleanupSource) recheck(ctx context.Context) error {
	if s == nil || s.closed || requireOwnedElevation(ctx) != nil {
		return ErrCleanupPending
	}
	var pid uint32
	if windows.GetNamedPipeServerProcessId(s.pipe, &pid) != nil || pid != s.peerPID {
		return ErrCleanupPending
	}
	state, e := windows.WaitForSingleObject(s.peer, 0)
	if e != nil || state != uint32(windows.WAIT_TIMEOUT) {
		return ErrCleanupPending
	}
	var c, x, k, u windows.Filetime
	if windows.GetProcessTimes(s.peer, &c, &x, &k, &u) != nil || uint64(c.HighDateTime)<<32|uint64(c.LowDateTime) != s.creation {
		return ErrCleanupPending
	}
	var elevation, size, session uint32
	user, e := s.peerToken.GetTokenUser()
	if e != nil || user == nil || user.User.Sid == nil || user.User.Sid.String() != s.sid || windows.GetTokenInformation(s.peerToken, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevation)), 4, &size) != nil || size != 4 || elevation != 0 || windows.GetTokenInformation(s.peerToken, windows.TokenSessionId, (*byte)(unsafe.Pointer(&session)), 4, &size) != nil || size != 4 || session != s.session {
		return ErrCleanupPending
	}
	for _, h := range s.held {
		if h.recheck() != nil {
			return ErrCleanupPending
		}
	}
	return nil
}

// Only the authenticated elevated helper calls this constructor with its held
// SDK server pipe. No supplied SID/path/namespace/hash is accepted as authority.
// Signed-SDK and exact removal-scope consent remain the helper's independent
// preconditions; native peer PID/token/session is rechecked here as well.
func openCleanupSource(ctx context.Context, installation, leaseSHA string, pipe uintptr) (out cleanupSource, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if ctx == nil {
		return nil, ErrCleanupPending
	}
	if _, ok := ctx.Deadline(); !ok {
		return nil, ErrCleanupPending
	}
	source := &nativeCleanupSource{pipe: windows.Handle(pipe), installation: installation, leaseSHA: leaseSHA, journal: -1}
	defer func() {
		if err != nil {
			if source.Close() != nil {
				out = source
				err = ErrCleanupPending
			} else {
				out = nil
			}
		}
	}()
	source.peer, source.peerToken, source.sid, source.peerPID, source.session, source.creation, err = cleanupNativePeer(ctx, source.pipe)
	if err != nil {
		return nil, err
	}
	base, e := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if e != nil || len(base) < 4 || base[1:3] != `:\` || strings.ContainsAny(base, `/%*?[]`+"`\x00") || filepath.Clean(base) != base {
		return nil, ErrCleanupPending
	}
	chain := []string{base}
	for p := filepath.Dir(base); ; p = filepath.Dir(p) {
		chain = append(chain, p)
		if filepath.Dir(p) == p {
			break
		}
	}
	keep := func(path string, directory bool, sd *windows.SECURITY_DESCRIPTOR, write bool) error {
		h, e := ownedProtectedOpenMode(path, directory, sd, write)
		if h.file != nil {
			source.held = append(source.held, h)
		}
		if e != nil {
			return e
		}
		return nil
	}
	for i := len(chain) - 1; i >= 0; i-- {
		if keep(chain[i], true, nil, false) != nil {
			return nil, ErrCleanupPending
		}
	}
	dirSD, e := windows.SecurityDescriptorFromString(protectedRuleSD(source.sid, true))
	if e != nil {
		return nil, ErrCleanupPending
	}
	root := filepath.Join(base, "OrbWin", "LANRules", installation)
	for _, path := range []string{filepath.Join(base, "OrbWin"), filepath.Join(base, "OrbWin", "LANRules"), root} {
		if keep(path, true, dirSD, false) != nil {
			return nil, ErrCleanupPending
		}
	}
	fileSD, e := windows.SecurityDescriptorFromString(protectedRuleSD(source.sid, false))
	if e != nil {
		return nil, ErrCleanupPending
	}
	if keep(filepath.Join(root, leaseSHA+".owner.json"), false, fileSD, false) != nil {
		return nil, ErrCleanupPending
	}
	metadataIndex := len(source.held) - 1
	if keep(filepath.Join(root, leaseSHA+".jsonl"), false, fileSD, true) != nil {
		return nil, ErrCleanupPending
	}
	source.journal = len(source.held) - 1
	source.metadata, err = protectedRuleRead(source.held[metadataIndex], ownedMetadataBudget)
	if err != nil {
		return nil, err
	}
	source.events, err = protectedRuleRead(source.held[source.journal], ownedProtectedRecordBudget)
	if err != nil {
		return nil, err
	}
	if _, err = source.Record(ctx); err != nil {
		return nil, err
	}
	return source, nil
}
func (s *nativeCleanupSource) Record(ctx context.Context) (ownedProtectedObservation, error) {
	if s.recheck(ctx) != nil {
		return ownedProtectedObservation{}, ErrCleanupPending
	}
	return parseOwnedProtectedMode(s.metadata, s.events, s.installation, s.leaseSHA, s.sid, traversalDeclarations{}, true)
}
func (s *nativeCleanupSource) schemas(ctx context.Context) error {
	if s.recheck(ctx) != nil {
		return ErrCleanupPending
	}
	d, e := readTraversalDeclarations(ctx)
	if e != nil {
		return ErrCleanupPending
	}
	if _, e = parseOwnedProtectedMode(s.metadata, s.events, s.installation, s.leaseSHA, s.sid, d, true); e != nil {
		return ErrCleanupPending
	}
	s.declarations = d
	return nil
}
func (s *nativeCleanupSource) Read(ctx context.Context, keys observedRuleKeys) (ownedDefinition, error) {
	if s.schemas(ctx) != nil {
		return ownedDefinition{}, ErrCleanupPending
	}
	return ownedReadDefinition(ctx, keys, s.declarations)
}
func (s *nativeCleanupSource) Absent(ctx context.Context, name string) (bool, error) {
	if s.schemas(ctx) != nil {
		return false, ErrCleanupPending
	}
	return ownedNameAbsent(ctx, name, s.declarations.rule)
}
func (s *nativeCleanupSource) Delete(ctx context.Context, keys observedRuleKeys, ready func() error) error {
	if s.schemas(ctx) != nil {
		return ErrCleanupPending
	}
	return ownedDispatchDelete(ctx, keys, s.declarations.rule, ready)
}
func (s *nativeCleanupSource) Closure(ctx context.Context, m ownedProtectedMetadata) error {
	if s.recheck(ctx) != nil || cleanupChildExited(ctx, m) != nil {
		return ErrCleanupPending
	}
	if observeCleanupNamespace(ctx, m) != nil || s.recheck(ctx) != nil {
		return ErrCleanupPending
	}
	return nil
}
func cleanupChildExited(ctx context.Context, m ownedProtectedMetadata) (err error) {
	if ctx == nil || ctx.Err() != nil {
		return ErrCleanupPending
	}
	original, e := strconv.ParseUint(m.Child.CreationTime, 10, 64)
	if e != nil || original == 0 || m.Child.PID == 0 {
		return ErrCleanupPending
	}
	process, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, m.Child.PID)
	if e != nil {
		if errors.Is(e, windows.ERROR_INVALID_PARAMETER) {
			return nil
		}
		return ErrCleanupPending
	}
	defer func() {
		if windows.CloseHandle(process) != nil {
			err = ErrCleanupPending
		}
	}()
	var c, x, k, u windows.Filetime
	if windows.GetProcessTimes(process, &c, &x, &k, &u) != nil {
		return ErrCleanupPending
	}
	if uint64(c.HighDateTime)<<32|uint64(c.LowDateTime) != original {
		return nil
	} // Exact PID reincarnation proves old child is gone.
	state, e := windows.WaitForSingleObject(process, 0)
	if e != nil || state != uint32(windows.WAIT_OBJECT_0) {
		return ErrCleanupPending
	}
	return nil
}

// This is a logical Docker daemon-ID +exact full-ID absence observation, not a
// cryptographic publisher/pipe-origin proof. There is no env/default/custom host,
// impersonation, WSL command or mutation API; inaccessible alternate-UAC users
// remain cleanup-pending. Native SDK pipe origin is separately retained above.
func observeCleanupNamespace(ctx context.Context, m ownedProtectedMetadata) (err error) {
	if !ownedHex(m.Namespace.ContainerID, 32) || m.Namespace.DaemonID != m.Prepared.Spec.Publication.Daemon {
		return ErrCleanupPending
	}
	bounded, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var responseFailed atomic.Bool
	cli, e := client.New(client.WithHost(cleanupDockerEndpoint), client.WithAPIVersionNegotiation(), client.WithHTTPResponseHook(cleanupResponseBound(&responseFailed)))
	if e != nil {
		return ErrCleanupPending
	}
	defer func() {
		if cli.Close() != nil || responseFailed.Load() {
			err = ErrCleanupPending
		}
	}()
	before, e := cli.Info(bounded, client.InfoOptions{})
	if e != nil || before.Info.ID != m.Namespace.DaemonID || responseFailed.Load() {
		return ErrCleanupPending
	}
	_, e = cli.ContainerInspect(bounded, m.Namespace.ContainerID, client.ContainerInspectOptions{})
	if !errdefs.IsNotFound(e) || responseFailed.Load() {
		return ErrCleanupPending
	}
	after, e := cli.Info(bounded, client.InfoOptions{})
	if e != nil || after.Info.ID != m.Namespace.DaemonID || bounded.Err() != nil || responseFailed.Load() {
		return ErrCleanupPending
	}
	return nil
}
func (s *nativeCleanupSource) Append(ctx context.Context, event OwnedRuleEvent) error {
	if s.recheck(ctx) != nil || event.LeaseSHA() != s.leaseSHA {
		return ErrCleanupPending
	}
	before, e := s.Record(ctx)
	if e != nil {
		return ErrCleanupPending
	}
	expected := map[string]string{"created": "remove-intent", "remove-intent": "removed"}[before.journalPhase]
	if event.Kind() != expected || event.sequence != before.journalSequence+1 {
		return ErrCleanupPending
	}
	data, e := event.ProtectedBytes()
	if e != nil {
		return ErrCleanupPending
	}
	data = append(data, '\n')
	h := &s.held[s.journal]
	old := h.identity
	if len(s.events)+len(data) > ownedProtectedRecordBudget {
		return ErrCleanupPending
	}
	offset, e := h.file.Seek(0, io.SeekEnd)
	if e != nil || offset != int64(len(s.events)) {
		return ErrCleanupPending
	}
	n, e := h.file.Write(data)
	if e != nil || n != len(data) || h.file.Sync() != nil {
		return ErrCleanupPending
	}
	var now windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(windows.Handle(h.file.Fd()), &now) != nil || now.VolumeSerialNumber != old.VolumeSerialNumber || now.FileIndexHigh != old.FileIndexHigh || now.FileIndexLow != old.FileIndexLow || now.NumberOfLinks != 1 || now.FileSizeHigh != 0 || now.FileSizeLow != uint32(len(s.events)+len(data)) || now.FileAttributes != old.FileAttributes {
		return ErrCleanupPending
	}
	// Only this retained exclusive writer can update the expected journal size/
	// write-time after positively flushed exact append; all other fields recheck.
	h.identity = now
	s.events = append(s.events, data...)
	if h.recheck() != nil {
		return ErrCleanupPending
	}
	_, e = s.Record(ctx)
	if e != nil {
		return ErrCleanupPending
	}
	return nil
}
func (s *nativeCleanupSource) Close() error {
	if s == nil {
		return nil
	}
	if s.closed {
		return s.closeErr
	}
	s.closed = true
	for i := len(s.held) - 1; i >= 0; i-- {
		if s.held[i].closeUnknown {
			s.closeErr = ErrCleanupPending
			continue
		}
		if s.held[i].recheck() != nil {
			s.closeErr = ErrCleanupPending
		}
		if s.held[i].file.Close() != nil {
			s.closeErr = ErrCleanupPending
		}
	}
	if s.peerToken != 0 && s.peerToken.Close() != nil {
		s.closeErr = ErrCleanupPending
	}
	if s.peer != 0 && windows.CloseHandle(s.peer) != nil {
		s.closeErr = ErrCleanupPending
	}
	return s.closeErr
}
