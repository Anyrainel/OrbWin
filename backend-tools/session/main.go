// OrbWin's per-session Docker client. Uses the same Moby SDK version as the
// pinned rdd. Its optional local-services process owns only named access, never
// the engine lifecycle. No host shell, global
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
	"orbwin.local/session/localnet"
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
	Session       string `json:"session,omitempty"`
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
	if msg.Mode != "" && msg.Mode != "debug" {
		return errors.New("unsupported terminal mode")
	}
	if msg.Kind != "open" || msg.DaemonID == "" || len(msg.Container) != 64 {
		return errors.New("open requires the recorded engine and full container ID")
	}
	if _, err := hex.DecodeString(msg.Container); err != nil {
		return errors.New("invalid container ID")
	}
	if (msg.Shell != "sh" && msg.Shell != "bash") || (msg.Mode == "debug" && msg.Shell != "sh") {
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
	if msg.Mode == "debug" {
		return runDebug(cli, msg, scanner, output)
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
	resize := func(cols, rows uint) error {
		ctx, cancel := bounded()
		defer cancel()
		_, err := cli.ExecResize(ctx, created.ID, client.ExecResizeOptions{Height: rows, Width: cols})
		return err
	}
	exit := func() (int, error) {
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
	}
	return interact(attached.HijackedResponse, scanner, output, resize, exit, nil)
}

func main() {
	if handled, err := runLANBootstrap(os.Args[1:], os.Stdout); handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin closed LAN bootstrap unavailable")
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "lan-network-observation" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		observation, err := localnet.ObserveNativeLAN(ctx)
		if err != nil || json.NewEncoder(os.Stdout).Encode(observation) != nil {
			fmt.Fprintln(os.Stderr, "OrbWin LAN native observation unavailable")
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "network-diagnostics" {
		if err := networkDiagnostics(context.Background(), os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin local network observation failed")
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "lan-profiles" {
		if err := lanProfiles(context.Background(), os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin LAN profiles:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "lan-interfaces" {
		if err := lanInterfaces(context.Background(), os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin LAN interfaces:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "maintenance-image-check" {
		if err := inspectMaintenanceArchive(os.Stdout, os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin maintenance archive:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "migration-wsl-sources" {
		if err := migrationWSLSources(os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin WSL migration sources:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 5 && os.Args[1] == "migration-rancher-review" {
		if err := reviewRancherMigration(os.Stdout, os.Args[2], os.Args[3], os.Args[4]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin Rancher source review:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 9 && os.Args[1] == "migration-rancher-image-export" {
		if err := exportRancherMigrationImage(os.Stdout, os.Args[2], os.Args[3], os.Args[4], os.Args[5], os.Args[6], os.Args[7], os.Args[8]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin Rancher image export:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 6 && os.Args[1] == "migration-volume-review" && os.Args[2] == "docker-desktop" {
		if err := reviewDesktopMigrationVolume(os.Stdout, os.Args[3], os.Args[4], os.Args[5]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin source volume review:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 7 && os.Args[1] == "migration-rancher-volume-review" {
		selection := migrationSourceIdentity{Kind: "rancher-desktop", Daemon: os.Args[3], WSL: &migrationWSLIdentity{Registration: os.Args[4], Machine: os.Args[5]}}
		if err := reviewMigrationVolume(os.Stdout, os.Args[2], selection, os.Args[6]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin Rancher volume review:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 9 && os.Args[1] == "migration-rancher-volume-export" {
		selection := migrationSourceIdentity{Kind: "rancher-desktop", Daemon: os.Args[3], WSL: &migrationWSLIdentity{Registration: os.Args[4], Machine: os.Args[5]}}
		if err := exportMigrationVolume(os.Stdout, os.Args[2], selection, os.Args[6], os.Args[7], os.Args[8]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin Rancher volume export (inspect its receipt and destination before retrying):", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 8 && os.Args[1] == "migration-volume-export" && os.Args[2] == "docker-desktop" {
		if err := exportDesktopMigrationVolume(os.Stdout, os.Args[3], os.Args[4], os.Args[5], os.Args[6], os.Args[7]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin source volume export (inspect its receipt and destination before retrying):", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "migration-image-history" {
		if err := migrationImageHistory(os.Stdout, os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin image export history:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "migration-volume-history" {
		if err := migrationVolumeHistory(os.Stdout, os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin source export history:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 6 && os.Args[1] == "migration-destination-review" {
		if err := reviewMigrationDestination(os.Stdout, os.Args[2], os.Args[3], os.Args[4], os.Args[5]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin restored destination review:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 4 && os.Args[1] == "migration-volume-recover" {
		if err := recoverDesktopMigrationVolume(os.Stdout, os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin source tool recovery:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 4 && os.Args[1] == "migration-review" && os.Args[2] == "docker-desktop" {
		if err := reviewDesktopMigration(os.Stdout, os.Args[3]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin migration source review:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 8 && os.Args[1] == "migration-image-export" && os.Args[2] == "docker-desktop" {
		if err := exportDesktopMigrationImage(os.Stdout, os.Args[3], os.Args[4], os.Args[5], os.Args[6], os.Args[7]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin migration image export:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 4 && os.Args[1] == "image-import-review" {
		if err := reviewImageImport(os.Stdout, os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin image import review:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "import-images" {
		if err := importImages(os.Stdin, os.Stdout, os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin image import:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "image-import-history" {
		if err := imageImportHistory(os.Stdout, os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin image import history:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 4 && os.Args[1] == "image-import-record" {
		if err := imageImportRecordOutput(os.Stdout, os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin image import record:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "volume-clones" {
		if err := volumeClones(os.Stdout, os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin clone recovery:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 4 && os.Args[1] == "discard-clone-staging" {
		if err := discardCloneStaging(os.Stdout, os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin clone recovery:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "volume-transfers" {
		if err := volumeTransfers(os.Stdout, os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin volume recovery:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 5 && os.Args[1] == "volume-size" {
		if err := volumeSize(os.Stdout, os.Args[2], os.Args[3], os.Args[4]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin volume accounting:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 4 && os.Args[1] == "discard-volume-transfer" {
		if err := discardVolumeTransfer(os.Stdout, os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin volume recovery:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 7 && os.Args[1] == "image-archive" && os.Args[2] == "export" {
		if err := exportImage(os.Stdout, os.Args[6], os.Args[3], os.Args[4], os.Args[5]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin image export:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 6 && os.Args[1] == "volume-transfer" {
		var err error
		switch os.Args[2] {
		case "export":
			err = exportVolume(os.Stdout, os.Args[5], os.Args[3], os.Args[4])
		case "restore":
			err = restoreVolume(os.Stdout, os.Args[5], os.Args[3], os.Args[4])
		case "clone":
			err = cloneVolume(os.Stdout, os.Args[5], os.Args[3], os.Args[4])
		default:
			err = errors.New("unsupported volume transfer operation")
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin volume transfer:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "save-volume-file" {
		if err := saveVolumeFile(os.Stdin, os.Stdout, os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin file save:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 8 && os.Args[1] == "files" && os.Args[6] == "copy" {
		if err := resourceFiles(os.Stdout, os.Args[2], os.Args[3], os.Args[4], os.Args[5], os.Args[6], os.Args[7]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin copy:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "recover-tools" {
		if err := recoverOwnedTools(os.Stdout, os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin tools recovery:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 7 && os.Args[1] == "files" {
		if err := resourceFiles(os.Stdout, os.Args[2], os.Args[3], os.Args[4], os.Args[5], os.Args[6]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin files:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) == 6 && os.Args[1] == "files" {
		if err := files(os.Stdout, os.Args[2], os.Args[3], os.Args[4], os.Args[5]); err != nil {
			fmt.Fprintln(os.Stderr, "OrbWin files:", err)
			os.Exit(125)
		}
		return
	}
	if len(os.Args) > 1 {
		if handled, err := runColdServiceArguments(os.Args[1:]); handled {
			if err != nil {
				fmt.Fprintln(os.Stderr, "OrbWin cold local services:", err)
				os.Exit(serviceExitCode(err))
			}
			return
		}
		if handled, err := runServiceArguments(os.Args[1:]); handled {
			if err != nil {
				fmt.Fprintln(os.Stderr, "OrbWin local services:", err)
				os.Exit(serviceExitCode(err))
			}
			return
		}
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
