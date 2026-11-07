//go:build windows && amd64

package lanhelper

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// RunElevationLauncher is ONLY the SDK's fixed early private role. It can run
// just one verified sibling helper; no Docker/engine/shell/generic child API.
// The parent owns its ordinary one-process kill-on-close job and can terminate
// and join this launcher even while the Windows UAC broker call is stalled.
func RunElevationLauncher(ctx context.Context, args []string) error {
	if ctx == nil || len(args) != 2 || !recordID(args[0]) || !recordID(args[1]) {
		return errors.New("fixed elevation launcher identities required")
	}
	deadline := time.Now().Add(70 * time.Second)
	stopCap := startNativeProcessCap(deadline)
	defer stopCap()
	ctx, cancel := context.WithTimeout(ctx, 65*time.Second)
	defer cancel()
	name, _ := launchPipeName(args[0], args[1])
	connect, stop := context.WithTimeout(ctx, 5*time.Second)
	conn, err := winio.DialPipeAccessImpLevel(connect, name, windows.GENERIC_READ|windows.GENERIC_WRITE, winio.PipeImpLevelIdentification)
	stop()
	if err != nil {
		return errors.New("fixed launcher owner channel unavailable")
	}
	defer conn.Close()
	closeOnCancel := context.AfterFunc(ctx, func() { conn.Close() })
	defer closeOnCancel()
	fd, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return errors.New("native launcher channel handle unavailable")
	}
	manager, err := authenticateManagerPipe(windows.Handle(fd.Fd()), false)
	var authority *AuthorizedInstallation
	defer func() { finishFixedRoleOwners(authority, manager) }()
	if err != nil {
		return err
	}
	authority, err = holdSignedInstallation(manager, args[0], false, true)
	if err != nil {
		return err
	}
	child, launchErr := launchElevatedHelper(ctx, authority, args[1])
	if child != nil {
		defer child.closeLauncherSource()
	}
	if launchErr != nil || child == nil || child.pid == 0 || child.creation == "" {
		_ = writeLaunch(conn, launchFrame{Version: 1, Kind: "error", ErrorCode: "declined"})
		return errors.New("explicit native helper launch did not complete")
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if manager.checkPipe(manager.pipe) != nil {
		return errors.New("launcher owner exited")
	}
	if err = conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	// Send the SOURCE handle only. The SDK owns the original CreateProcess
	// launcher handle and duplicates FROM this exact child itself; no unknown
	// duplicated target handle is leaked if this frame/acknowledgement is lost.
	frame := launchFrame{Version: 1, Kind: "handle", SourceHandle: strconv.FormatUint(uint64(child.process), 10), PID: child.pid, Creation: child.creation}
	if err = writeLaunch(conn, frame); err != nil {
		return err
	}
	ack, err := readLaunch(conn)
	if err != nil || ack.Kind != "received" || manager.checkPipe(manager.pipe) != nil {
		return errors.New("SDK native handle handoff acknowledgement unknown")
	}
	return nil
}
