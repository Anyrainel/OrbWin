//go:build windows && amd64

package lanhelper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"sync"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanlease"
	"orbwin.local/session/internal/lanprovider"
)

// Construction is verification/read-only, never elevation. The existing
// ServiceRuntime owns this adapter; no renderer may register or configure it.
type AdministrativeClient struct {
	nativeOrigin *AdministrativeClient // Actual constructor allocation, before slot admission/native work.
	mu           sync.Mutex
	manager      *NativeManager
	authority    *AuthorizedInstallation
	ctx          context.Context
	cancel       context.CancelFunc
	closed       bool
	reads        chan struct{}
	readers      sync.WaitGroup
	closeOnce    sync.Once
	current      *adminNativeSession
	sessions     sync.WaitGroup
	used         map[string]bool
	done         chan struct{}
	result       error
	cleanupPanic any
}

func (client *AdministrativeClient) exactOwner() bool {
	return client != nil && client.nativeOrigin == client
}

// The existing SDK model has one owned ServiceRuntime/administrative client per
// process. This single slot retains an unknown owner without an unbounded
// quarantine registry; only positively successful Close permits a successor.
var administrativeClientOwner struct {
	sync.Mutex
	client *AdministrativeClient
}

func claimAdministrativeClient(client *AdministrativeClient) bool {
	if !client.exactOwner() {
		return false
	}
	administrativeClientOwner.Lock()
	defer administrativeClientOwner.Unlock()
	if administrativeClientOwner.client != nil {
		return false
	}
	administrativeClientOwner.client = client
	return true
}

func releaseAdministrativeClient(client *AdministrativeClient) {
	administrativeClientOwner.Lock()
	defer administrativeClientOwner.Unlock()
	if administrativeClientOwner.client == client {
		administrativeClientOwner.client = nil
	}
}

func NewAdministrativeClient(installation, expectedManagerSHA string) (client *AdministrativeClient, err error) {
	if !recordID(installation) || !ValidSHA(expectedManagerSHA) {
		return nil, errors.New("fixed verified LAN manager namespace/image required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	owned := &AdministrativeClient{ctx: ctx, cancel: cancel, reads: make(chan struct{}, 16), done: make(chan struct{}), used: make(map[string]bool)}
	owned.nativeOrigin = owned
	if !claimAdministrativeClient(owned) {
		cancel()
		return nil, errors.New("the native administrative owner is active or requires inspection")
	}
	keep := false
	defer func() {
		if !keep {
			client, err = settleAdministrativeConstruction(owned, err, func() error { return settleFixedRoleOwners(owned.authority, owned.manager) })
		}
	}()
	manager, err := holdOrdinarySelf()
	owned.manager = manager
	if err != nil {
		return nil, err
	}
	if manager.identity.ImageSHA256 != expectedManagerSHA {
		return nil, errors.New("actual manager image differs from trusted SDK bootstrap")
	}
	authority, err := holdSignedInstallation(manager, installation, false, false)
	owned.authority = authority
	if err != nil {
		return nil, err
	}
	keep = true
	return owned, nil
}

// All partial native creators retain their stored close result. Re-observing
// that result cannot repeat a native close or turn cleared fields into success.
// The constructor statically binds authority-then-manager cleanup. Only that
// call is recovered; no stored/replaceable factory or input chooses the method.
func settleAdministrativeConstruction(owned *AdministrativeClient, failure error, closeOwners func() error) (partial *AdministrativeClient, result error) {
	if !owned.exactOwner() {
		return nil, errors.Join(failure, errNativeOwnerCopy)
	}
	partial, result = owned, failure
	closeReturned := false
	defer func() {
		if !closeReturned {
			owned.cleanupPanic = recover()
			owned.cancel()
			owned.result = errors.Join(failure, errNativeOwnerCloseUnknown)
			owned.closed = true
			owned.closeOnce.Do(func() { close(owned.done) })
			partial, result = owned, owned.result
		}
	}()
	closeErr := closeOwners()
	closeReturned = true
	owned.cancel()
	if closeErr != nil {
		owned.result = errors.Join(failure, closeErr)
		owned.closed = true
		owned.closeOnce.Do(func() { close(owned.done) })
		return owned, owned.result
	}
	releaseAdministrativeClient(owned)
	return nil, failure
}
func (client *AdministrativeClient) WorkerSHA256() string {
	return client.authority.assets.Assets[2].SHA256
}
func (client *AdministrativeClient) ManagerIdentity() lanfirewall.CallerIdentity {
	return client.manager.identity
}
func (client *AdministrativeClient) Installation() string { return client.authority.installation }

type ruleObservationOutput struct{ buffer bytes.Buffer }

func (output *ruleObservationOutput) Write(data []byte) (int, error) {
	if len(data) > FrameBudget-output.buffer.Len() {
		return 0, errors.New("read-only rule output exceeds bound")
	}
	return output.buffer.Write(data)
}

// ReadRule executes only the signed SDK's fixed read-only child role, never an
// elevated process. Each caller holds one bounded slot through actual child
// wait; a failed kernel wait cannot release the slot merely on an RPC timeout.
func (client *AdministrativeClient) ReadRule(ctx context.Context, prepared Prepared) (lanlease.RuleAcknowledgement, error) {
	var empty lanlease.RuleAcknowledgement
	if !client.exactOwner() {
		return empty, errNativeOwnerCopy
	}
	if ctx == nil || prepared.Spec.Request.Installation != client.authority.installation || !ValidSHA(prepared.LeaseSHA) {
		return empty, errors.New("exact protected LAN read owner required")
	}
	data, err := client.readObservation(ctx, []string{"lan-rule-observation", client.authority.installation, prepared.LeaseSHA})
	if err != nil {
		return empty, err
	}
	if ValidateClosedJSON(data, lanlease.RuleAcknowledgement{}) != nil || json.Unmarshal(data, &empty) != nil || empty.LeaseSHA != prepared.LeaseSHA {
		return lanlease.RuleAcknowledgement{}, errors.New("native rule observation shape/owner differs")
	}
	return empty, nil
}

// A lost Commit reply may recover only the protected flushed create result
// and fresh full typed native observation through the bounded read-only role.
// The controller accepts it while closing; this never restarts ingress.
func (client *AdministrativeClient) RecoverCreate(ctx context.Context, prepared Prepared) (lanlease.RuleAcknowledgement, error) {
	return client.ReadRule(ctx, prepared)
}

func (client *AdministrativeClient) RecoverRemoval(ctx context.Context, prepared Prepared) (lanlease.RemovalAcknowledgement, error) {
	var result lanlease.RemovalAcknowledgement
	if !client.exactOwner() {
		return result, errNativeOwnerCopy
	}
	if ctx == nil || prepared.Spec.Request.Installation != client.authority.installation || !ValidSHA(prepared.LeaseSHA) {
		return result, errors.New("exact protected removal read owner required")
	}
	data, err := client.readObservation(ctx, []string{"lan-removal-observation", client.authority.installation, prepared.LeaseSHA})
	if err != nil {
		return result, err
	}
	if ValidateClosedJSON(data, lanlease.RemovalAcknowledgement{}) != nil || json.Unmarshal(data, &result) != nil || result.LeaseSHA != prepared.LeaseSHA {
		return lanlease.RemovalAcknowledgement{}, errors.New("native removal observation shape/owner differs")
	}
	return result, nil
}

func (client *AdministrativeClient) ReadCleanup(ctx context.Context, leaseSHA string) (lanprovider.CleanupObservation, error) {
	var result lanprovider.CleanupObservation
	if !client.exactOwner() {
		return result, errNativeOwnerCopy
	}
	if ctx == nil || !ValidSHA(leaseSHA) {
		return result, errors.New("exact protected cleanup observation required")
	}
	data, err := client.readObservation(ctx, []string{"lan-cleanup-observation", client.authority.installation, leaseSHA})
	if err != nil {
		return result, err
	}
	if ValidateClosedJSON(data, lanprovider.CleanupObservation{}) != nil || json.Unmarshal(data, &result) != nil || result.LeaseSHA != leaseSHA || result.Spec.Request.Installation != client.authority.installation || result.Spec.Request.UserSID != client.manager.identity.UserSID || !result.ChildExited || !result.NamespaceAbsent {
		return lanprovider.CleanupObservation{}, errors.New("protected native cleanup observation differs")
	}
	return result, nil
}
func (client *AdministrativeClient) RecoverReview(ctx context.Context, reviewID, reviewSHA string) (ProtectedReviewObservation, error) {
	var result ProtectedReviewObservation
	if !client.exactOwner() {
		return result, errNativeOwnerCopy
	}
	if ctx == nil || !recordID(reviewID) || !ValidSHA(reviewSHA) {
		return result, errors.New("exact protected original review required")
	}
	data, err := client.readObservation(ctx, []string{"lan-review-observation", client.authority.installation, reviewID, reviewSHA})
	if err != nil {
		return result, err
	}
	if ValidateClosedJSON(data, ProtectedReviewObservation{}) != nil || json.Unmarshal(data, &result) != nil {
		return ProtectedReviewObservation{}, errors.New("protected review output shape differs")
	}
	record := result.Record
	issued, err := time.Parse(time.RFC3339Nano, record.Stage.Base.IssuedAt)
	if record.Cleanup != nil {
		issued, err = time.Parse(time.RFC3339Nano, record.Cleanup.IssuedAt)
	}
	identity := lanfirewall.CallerIdentity{PID: record.Stage.Base.CallerPID, CreationTime: record.Stage.Base.CallerCreationTime, UserSID: record.Stage.Base.UserSID, ImageSHA256: record.Stage.Base.CallerImageSHA256}
	if err != nil || record.ReviewID != reviewID || record.ReviewSHA256 != reviewSHA || record.Stage.Base.Installation != client.authority.installation || identity.UserSID != client.manager.identity.UserSID || record.Validate(identity, issued) != nil {
		return ProtectedReviewObservation{}, errors.New("protected original review owner differs")
	}
	return result, nil
}
func (client *AdministrativeClient) readObservation(ctx context.Context, args []string) ([]byte, error) {
	var empty []byte
	if !client.exactOwner() {
		return nil, errNativeOwnerCopy
	}
	if ctx == nil || !((len(args) == 3 && (args[0] == "lan-rule-observation" || args[0] == "lan-removal-observation" || args[0] == "lan-cleanup-observation") && args[1] == client.authority.installation && ValidSHA(args[2])) || (len(args) == 4 && args[0] == "lan-review-observation" && args[1] == client.authority.installation && recordID(args[2]) && ValidSHA(args[3]))) {
		return nil, errors.New("fixed read-only child scope required")
	}
	client.mu.Lock()
	if client.closed {
		client.mu.Unlock()
		return empty, errors.New("native LAN client is closed")
	}
	select {
	case client.reads <- struct{}{}:
	default:
		client.mu.Unlock()
		return empty, ErrObservationBusy
	}
	client.readers.Add(1)
	client.mu.Unlock()
	defer func() { <-client.reads; client.readers.Done() }()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	stop := context.AfterFunc(client.ctx, cancel)
	defer stop()
	command := exec.CommandContext(ctx, client.manager.path, args...)
	windowsDirectory, err := windows.GetWindowsDirectory()
	if err != nil {
		return empty, err
	}
	systemDirectory, err := windows.GetSystemDirectory()
	if err != nil {
		return empty, err
	}
	command.Dir = systemDirectory
	command.Env = []string{"SystemRoot=" + windowsDirectory, "WINDIR=" + windowsDirectory, "GOMAXPROCS=2"}
	command.SysProcAttr = &windows.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	command.WaitDelay = time.Second
	var output ruleObservationOutput
	command.Stdout = &output
	command.Stderr = io.Discard
	err = command.Run()
	if command.Process != nil && (command.ProcessState == nil || !command.ProcessState.Exited()) {
		// Pinned Go1.27.3 windows Process.wait retains its original handle on
		// Wait/GetExitCode/GetTimes errors. Only read-only wait is retried here.
		for {
			state, waitErr := command.Process.Wait()
			if waitErr == nil && state != nil && state.Exited() {
				break
			}
			timer := time.NewTimer(time.Second)
			<-timer.C
		}
	}
	if err != nil || ctx.Err() != nil {
		return empty, errors.New("bounded protected native rule observation unavailable")
	}
	return bytes.TrimSpace(output.buffer.Bytes()), nil
}
func (client *AdministrativeClient) Close(ctx context.Context) error {
	if !client.exactOwner() {
		return errNativeOwnerCopy
	}
	if ctx == nil {
		return errors.New("native client shutdown context required")
	}
	client.closeOnce.Do(func() {
		client.mu.Lock()
		client.closed = true
		client.cancel()
		client.mu.Unlock()
		go func() {
			client.sessions.Wait()
			client.readers.Wait()
			client.mu.Lock()
			failed := client.result != nil
			client.mu.Unlock()
			if !failed {
				// Session failure preserves the actual authority/manager exclusions.
				// Each native close is attempted once; uncertainty retains this slot.
				client.result = client.authority.Close()
				if client.result == nil {
					client.result = client.manager.Close()
				}
				if client.result == nil {
					releaseAdministrativeClient(client)
				}
			}
			close(client.done)
		}()
	})
	select {
	case <-client.done:
		return client.result
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Keep the fixed SDK launch environment representable without inherited shell,
// Docker/proxy variables or path/argument expansion.
func nativeChildEnvironment() ([]uint16, error) {
	root, err := windows.GetWindowsDirectory()
	if err != nil {
		return nil, err
	}
	return utf16.Encode([]rune("SystemRoot=" + root + "\x00WINDIR=" + root + "\x00GOMAXPROCS=2\x00\x00")), nil
}
