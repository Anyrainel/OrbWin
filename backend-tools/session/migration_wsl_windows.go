//go:build windows

package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/moby/moby/client"
	"golang.org/x/sys/windows"
	"orbwin.local/session/internal/wsltrust"
)

type rancherMigrationSession struct {
	ctx          context.Context
	asset        []byte
	digest       string
	registration wsltrust.Registration
	binary       string
	mu           sync.Mutex
	startup      sync.Mutex
	machine      string
	connections  []*migrationProcessConn
	closed       bool
}

// Arguments are separate from the constant shell program. The private root
// stage contains only the pinned public CLI, never credentials or user data.
// The timer bounds abandoned Linux children even after abrupt Windows loss.
const rancherMigrationBootstrap = `set -eu
umask 077
test "$(/usr/bin/grep '^ID=' /etc/os-release)" = ID=ubuntu || exit 75
test "$(/usr/bin/grep '^VERSION_ID=' /etc/os-release)" = 'VERSION_ID="24.04"' || exit 75
machine=$(/usr/bin/cat /etc/machine-id)
test "$1" = - || test "$machine" = "$1" || exit 74
stage=$(/usr/bin/mktemp -d /tmp/orbwin-migration.XXXXXXXXXX)
trap '/usr/bin/rm -f -- "$stage/docker"; /usr/bin/rmdir -- "$stage"' EXIT
/usr/bin/head -c "$2" > "$stage/docker"
actual=$(/usr/bin/sha256sum -- "$stage/docker")
test "${actual%% *}" = "$3" || exit 73
/usr/bin/chmod 0500 -- "$stage/docker"
printf 'ORBWIN_RANCHER_STREAM_1 %s\n' "$machine"
/usr/bin/timeout --signal=TERM --kill-after=2 1830 "$stage/docker" --config "$stage" --host unix:///mnt/wsl/rancher-desktop/run/docker.sock system dial-stdio
`

func newRancherMigrationSession(ctx context.Context, registrationID, machine string) (*rancherMigrationSession, error) {
	if !wsltrust.ValidRegistrationID(registrationID) || (machine != "" && (len(machine) != 32 || strings.Trim(machine, "0123456789abcdef") != "")) {
		return nil, errors.New("review an ordinary WSL registration and machine identity")
	}
	registrations, err := wsltrust.Registrations()
	if err != nil {
		return nil, err
	}
	var selected *wsltrust.Registration
	for _, registration := range registrations {
		if registration.ID == registrationID {
			copy := registration
			selected = &copy
			break
		}
	}
	if selected == nil {
		return nil, errors.New("selected ordinary WSL 2 registration is absent or protected")
	}
	path := os.Getenv("ORBWIN_MIGRATION_DOCKER")
	file, err := openImageArchiveFile(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() < 64 || info.Size() > 128<<20 {
		return nil, errors.New("bundled Linux Docker CLI is not a bounded regular ELF")
	}
	asset, err := io.ReadAll(io.LimitReader(file, (128<<20)+1))
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(asset)
	expected := os.Getenv("ORBWIN_MIGRATION_DOCKER_SHA256")
	if !digestValid(expected) || hex.EncodeToString(digest[:]) != expected || len(asset) < 64 || string(asset[:7]) != "\x7fELF\x02\x01\x01" || asset[18] != 62 || asset[19] != 0 {
		return nil, errors.New("bundled Linux/amd64 Docker CLI checksum differs")
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, err
	}
	return &rancherMigrationSession{ctx: ctx, asset: asset, digest: expected, registration: *selected, machine: machine, binary: filepath.Join(system, "wsl.exe")}, nil
}
func (session *rancherMigrationSession) identity() migrationWSLIdentity {
	session.mu.Lock()
	defer session.mu.Unlock()
	return migrationWSLIdentity{Registration: session.registration.ID, Name: session.registration.Name, Machine: session.machine}
}
func (session *rancherMigrationSession) dial(ctx context.Context, _, _ string) (net.Conn, error) {
	session.startup.Lock()
	defer session.startup.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	session.mu.Lock()
	machine := session.machine
	closed := session.closed
	session.mu.Unlock()
	if closed {
		return nil, net.ErrClosed
	}
	registrations, err := wsltrust.Registrations()
	if err != nil {
		return nil, err
	}
	found := false
	for _, registration := range registrations {
		if registration == session.registration {
			found = true
			break
		}
	}
	if !found {
		return nil, errors.New("selected WSL registration changed; refresh migration source")
	}
	expected := machine
	if expected == "" {
		expected = "-"
	}
	command := exec.Command(session.binary, "--distribution-id", "{"+session.registration.ID+"}", "--user", "root", "--exec", "/usr/bin/env", "-i", "PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "HOME=/nonexistent", "/bin/sh", "-c", rancherMigrationBootstrap, "orbwin-migration", expected, strconv.Itoa(len(session.asset)), session.digest)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	conn, err := startMigrationProcess(session.ctx, command)
	if err != nil {
		return nil, err
	}
	session.mu.Lock()
	if session.closed {
		session.mu.Unlock()
		conn.Close()
		return nil, net.ErrClosed
	}
	session.connections = append(session.connections, conn)
	session.mu.Unlock()
	failed := true
	defer func() {
		if failed {
			conn.Close()
		}
	}()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	deadline := time.Now().Add(30 * time.Second)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	conn.SetDeadline(deadline)
	if _, err := writeAllMigration(conn, session.asset); err != nil {
		return nil, fmt.Errorf("selected WSL source did not verify Ubuntu 24.04, machine identity and bundled CLI; refresh the selected source: %w", err)
	}
	reader := bufio.NewReaderSize(conn.Conn, 512)
	line, err := reader.ReadSlice('\n')
	if err != nil {
		return nil, errors.New("selected WSL migration bootstrap did not confirm identity")
	}
	fields := strings.Fields(string(line))
	if len(fields) != 2 || fields[0] != "ORBWIN_RANCHER_STREAM_1" || len(fields[1]) != 32 || strings.Trim(fields[1], "0123456789abcdef") != "" || (machine != "" && fields[1] != machine) {
		return nil, errors.New("selected WSL machine identity differs; source connection refused")
	}
	conn.reader = reader
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	session.mu.Lock()
	session.machine = fields[1]
	session.mu.Unlock()
	conn.SetDeadline(time.Time{})
	failed = false
	return conn, nil
}
func writeAllMigration(writer io.Writer, data []byte) (int, error) {
	total := 0
	for len(data) > 0 {
		n, err := writer.Write(data)
		total += n
		data = data[n:]
		if err != nil {
			return total, err
		}
		if n == 0 {
			return total, io.ErrShortWrite
		}
	}
	return total, nil
}
func (session *rancherMigrationSession) close() {
	session.mu.Lock()
	session.closed = true
	connections := append([]*migrationProcessConn{}, session.connections...)
	session.mu.Unlock()
	for _, connection := range connections {
		connection.Close()
	}
}
func newRancherMigrationClient(ctx context.Context, registration, machine string) (*client.Client, *rancherMigrationSession, func(), error) {
	return rancherMigrationClient(ctx, registration, machine, func(base http.RoundTripper) http.RoundTripper { return migrationTransport{base} }, migrationReadRequest)
}
func rancherMigrationClient(ctx context.Context, registration, machine string, wrap func(http.RoundTripper) http.RoundTripper, hook client.RequestHook) (*client.Client, *rancherMigrationSession, func(), error) {
	session, err := newRancherMigrationSession(ctx, registration, machine)
	if err != nil {
		return nil, nil, nil, err
	}
	cli, closeClient, err := newMigrationStreamClient(session.dial, wrap, hook)
	if err != nil {
		session.close()
		return nil, nil, nil, fmt.Errorf("prepare Rancher source client: %w", err)
	}
	return cli, session, func() { closeClient(); session.close() }, nil
}
