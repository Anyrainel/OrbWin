// OrbWin's per-session Docker client. Uses the same Moby SDK version as the
// pinned rdd. It is not a daemon or lifecycle supervisor. No host shell, global
// context, config mutation, engine restart or Docker protocol translation.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/moby/moby/client"
)

const endpoint = "npipe:////./pipe/docker-rancher-desktop-orbwin"

func writeFull(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

type message struct {
	Mode          string `json:"mode,omitempty"`
	Namespace     string `json:"namespace,omitempty"`
	Pod           string `json:"pod,omitempty"`
	PodUID        string `json:"podUid,omitempty"`
	ContainerName string `json:"containerName,omitempty"`
	Kubeconfig    string `json:"kubeconfig,omitempty"`
	Kind          string `json:"kind"`
	Container     string `json:"container,omitempty"`
	DaemonID      string `json:"daemonId,omitempty"`
	Shell         string `json:"shell,omitempty"`
	Cols          uint   `json:"cols,omitempty"`
	Rows          uint   `json:"rows,omitempty"`
	Data          []byte `json:"data,omitempty"`
}

func decode(line []byte) (message, error) {
	var msg message
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&msg); err != nil {
		return msg, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return msg, errors.New("unexpected trailing input")
	}
	return msg, nil
}
func dimensions(cols, rows uint) error {
	if cols < 10 || cols > 400 || rows < 2 || rows > 150 {
		return errors.New("unsupported terminal dimensions")
	}
	return nil
}
func validateOpen(msg message) error {
	if msg.Mode == "kubernetes" {
		return validatePodOpen(msg)
	}
	if msg.Mode != "" {
		return errors.New("unsupported terminal mode")
	}
	if msg.Kind != "open" || msg.DaemonID == "" || len(msg.Container) != 64 {
		return errors.New("open requires the recorded engine and full container ID")
	}
	if _, err := hex.DecodeString(msg.Container); err != nil {
		return errors.New("invalid container ID")
	}
	if msg.Shell != "sh" && msg.Shell != "bash" {
		return errors.New("unsupported container shell")
	}
	return dimensions(msg.Cols, msg.Rows)
}
func bounded() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 15*time.Second)
}
func run(input io.Reader, output io.Writer) (int, error) {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 32768)
	if !scanner.Scan() {
		return 125, errors.New("missing session configuration")
	}
	msg, err := decode(scanner.Bytes())
	if err != nil {
		return 125, err
	}
	if err = validateOpen(msg); err != nil {
		return 125, err
	}
	if msg.Mode == "kubernetes" {
		return runKubernetes(msg, scanner, output)
	}
	// Deliberately do not use FromEnv: host/context/TLS variables cannot redirect us.
	cli, err := client.New(client.WithHost(endpoint), client.WithAPIVersionNegotiation())
	if err != nil {
		return 125, err
	}
	defer cli.Close()
	ctx, cancel := bounded()
	info, err := cli.Info(ctx, client.InfoOptions{})
	cancel()
	if err != nil {
		return 125, err
	}
	if info.Info.ID != msg.DaemonID || info.Info.OSType != "linux" {
		return 125, errors.New("endpoint does not match the recorded owned Linux engine")
	}
	ctx, cancel = bounded()
	created, err := cli.ExecCreate(ctx, msg.Container, client.ExecCreateOptions{TTY: true, ConsoleSize: client.ConsoleSize{Height: msg.Rows, Width: msg.Cols}, AttachStdin: true, AttachStdout: true, AttachStderr: true, Cmd: []string{"/bin/" + msg.Shell}, Env: []string{"TERM=xterm-256color"}})
	cancel()
	if err != nil {
		return 125, err
	}
	ctx, cancel = bounded()
	attached, err := cli.ExecAttach(ctx, created.ID, client.ExecAttachOptions{TTY: true, ConsoleSize: client.ConsoleSize{Height: msg.Rows, Width: msg.Cols}})
	cancel()
	if err != nil {
		return 125, err
	}
	defer attached.Close()
	fmt.Fprintln(os.Stderr, "ORBWIN_SESSION_READY")
	copied := make(chan error, 1)
	commands := make(chan error, 1)
	go func() { _, err := io.Copy(output, attached.Reader); copied <- err }()
	go func() {
		for scanner.Scan() {
			next, err := decode(scanner.Bytes())
			if err != nil {
				commands <- err
				return
			}
			switch next.Kind {
			case "input":
				if len(next.Data) > 8192 {
					commands <- errors.New("input exceeds 8 KiB")
					return
				}
				if err := attached.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
					commands <- err
					return
				}
				err := writeFull(attached.Conn, next.Data)
				_ = attached.Conn.SetWriteDeadline(time.Time{})
				if err != nil {
					commands <- err
					return
				}
			case "resize":
				if err := dimensions(next.Cols, next.Rows); err != nil {
					commands <- err
					return
				}
				ctx, cancel := bounded()
				_, err := cli.ExecResize(ctx, created.ID, client.ExecResizeOptions{Height: next.Rows, Width: next.Cols})
				cancel()
				if err != nil {
					commands <- err
					return
				}
			case "close":
				commands <- nil
				return
			default:
				commands <- errors.New("unsupported session message")
				return
			}
		}
		commands <- scanner.Err()
	}()
	select {
	case err := <-copied:
		if err != nil {
			return 125, err
		}
		ctx, cancel := bounded()
		defer cancel()
		for {
			result, err := cli.ExecInspect(ctx, created.ID, client.ExecInspectOptions{})
			if err != nil {
				return 125, err
			}
			if result.ContainerID != msg.Container {
				return 125, errors.New("exec ownership changed")
			}
			if !result.Running {
				return result.ExitCode, nil
			}
			select {
			case <-ctx.Done():
				return 125, errors.New("exec ended without a settled exit status")
			case <-time.After(20 * time.Millisecond):
			}
		}
	case err := <-commands:
		return 125, err
	case <-time.After(time.Hour):
		return 125, errors.New("terminal client reached its one-hour session limit")
	}
}
func main() {
	if len(os.Args) == 6 && os.Args[1] == "files" {
		if err := files(os.Stdout, os.Args[2], os.Args[3], os.Args[4], os.Args[5]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin files:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) > 1 {
		if len(os.Args) != 3 || os.Args[1] != "port-status" {
			fmt.Fprintln(os.Stderr, "Unsupported OrbWin client arguments")
			os.Exit(125)
		}
		if err := publicationSnapshot(os.Stdout, os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin publication:", err)
			os.Exit(125)
		}
		return
	}
	code, err := run(os.Stdin, os.Stdout)
	if err != nil {
		fmt.Fprintln(os.Stderr, "OrbWin terminal:", err)
	}
	os.Exit(code)
}
