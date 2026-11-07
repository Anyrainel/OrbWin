package main

// Durable intent belongs to an SDK client, not a UI process or engine lifecycle.
// Only exact persisted intent names/IDs with matching metadata may be recovered.
import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/client"
)

type transientIntent struct {
	Version int    `json:"version"`
	Kind    string `json:"kind"`
	Session string `json:"session"`
	Target  string `json:"target"`
	Name    string `json:"name"`
}
type transient struct {
	intent     transientIntent
	file, lock string
	lease      *os.File
	settled    bool
}

func (record transientIntent) validate() error {
	if record.Version != 1 || (record.Kind != "debug" && record.Kind != "files" && record.Kind != "edit") || !regexp.MustCompile(`^[a-fA-F0-9-]{32,36}$`).MatchString(record.Session) || record.Name != "orbwin-"+record.Kind+"-"+record.Session || record.Target == "" || len(record.Target) > 256 || strings.ContainsAny(record.Target, "\x00\r\n") {
		return errors.New("invalid transient ownership record")
	}
	return nil
}
func transientDirectory() (string, error) {
	directory := os.Getenv("ORBWIN_TRANSIENT_DIR")
	if !filepath.IsAbs(directory) {
		return "", errors.New("OrbWin did not provide its persistent transient ownership directory")
	}
	return directory, nil
}
func recoverOwnedTools(output io.Writer, daemon string) error {
	if daemon == "" || len(daemon) > 256 || strings.ContainsAny(daemon, "\x00\r\n") {
		return errors.New("recorded owned daemon required")
	}
	cli, err := client.New(client.WithHost(endpoint), client.WithAPIVersionNegotiation())
	if err != nil {
		return err
	}
	defer cli.Close()
	ctx, cancel := bounded()
	defer cancel()
	info, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		return err
	}
	if info.Info.ID != daemon || info.Info.OSType != "linux" {
		return errors.New("tools recovery endpoint does not match the recorded owned Linux daemon")
	}
	directory, err := transientDirectory()
	if err != nil {
		return err
	}
	removed, err := recoverTransients(cli, directory)
	if err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(map[string]any{"removed": removed, "message": "Only inactive recorded helper resources were considered. Active leases and uncertain missing-by-name creates are retained."})
}
func beginTransient(cli *client.Client, kind, session, target string) (*transient, error) {
	directory, err := transientDirectory()
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	if _, err = recoverTransients(cli, directory); err != nil {
		return nil, err
	}
	record := transientIntent{Version: 1, Kind: kind, Session: session, Target: target, Name: "orbwin-" + kind + "-" + session}
	if err = record.validate(); err != nil {
		return nil, err
	}
	base := filepath.Join(directory, kind+"-"+session)
	lease, err := transientLease(base+".lock", true)
	if err != nil {
		return nil, err
	}
	entry := &transient{intent: record, file: base + ".json", lock: base + ".lock", lease: lease}
	file, err := os.OpenFile(entry.file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		lease.Close()
		return nil, err
	}
	if err = json.NewEncoder(file).Encode(record); err == nil {
		err = file.Sync()
	}
	closed := file.Close()
	if err == nil {
		err = closed
	}
	if err != nil {
		lease.Close()
		return nil, err
	}
	return entry, nil
}
func (entry *transient) created(id string) error {
	raw, err := hex.DecodeString(id)
	if err != nil || len(raw) != 32 {
		return errors.New("invalid created helper ID")
	}
	file, err := os.OpenFile(entry.file, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err = fmt.Fprintln(file, id); err != nil {
		return err
	}
	return file.Sync()
}
func (entry *transient) close() error {
	err := entry.lease.Close()
	if entry.settled {
		return errors.Join(err, removeIntent(entry.file, entry.lock))
	}
	return err
}
func removeIntent(file, lock string) error {
	var failures []error
	for _, name := range []string{file, lock} {
		if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
func recoverTransients(cli *client.Client, directory string) (int, error) {
	recoveryCtx, recoveryCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer recoveryCancel()
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	removed, processed := 0, 0
	for _, entry := range entries {
		if err := recoveryCtx.Err(); err != nil {
			return removed, err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		processed++
		if processed > 512 {
			return removed, errors.New("transient ownership directory exceeds 512 records; review diagnostics before more helpers are created")
		}
		file := filepath.Join(directory, entry.Name())
		lock := strings.TrimSuffix(file, ".json") + ".lock"
		lease, err := transientLease(lock, false)
		if err != nil {
			if transientBusy(err) {
				continue
			}
			return removed, err
		} // An active SDK client retains exclusive ownership.
		cleanupErr := func() error {
			data, err := os.Open(file)
			if err != nil {
				return err
			}
			scanner := bufio.NewScanner(io.LimitReader(data, 8192))
			scanner.Buffer(make([]byte, 1024), 8192)
			if !scanner.Scan() {
				data.Close()
				return errors.New("incomplete transient ownership header")
			}
			var record transientIntent
			err = json.Unmarshal(scanner.Bytes(), &record)
			if err == nil {
				err = record.validate()
			}
			if err != nil {
				data.Close()
				return err
			}
			if entry.Name() != record.Kind+"-"+record.Session+".json" {
				data.Close()
				return errors.New("transient filename does not match its owner")
			}
			knownID := ""
			if scanner.Scan() {
				candidate := scanner.Text()
				raw, decodeErr := hex.DecodeString(candidate)
				if decodeErr == nil && len(raw) == 32 {
					knownID = candidate
				}
			}
			data.Close()
			query := knownID
			if query == "" {
				query = record.Name
			}
			ctx, cancel := context.WithTimeout(recoveryCtx, 5*time.Second)
			defer cancel()
			inspected, err := cli.ContainerInspect(ctx, query, client.ContainerInspectOptions{})
			if cerrdefs.IsNotFound(err) {
				if knownID != "" {
					lease.Close()
					return removeIntent(file, lock)
				}
				return nil
			} // Unknown accepted create may still finish; retain its intent.
			if err != nil {
				return err
			}
			helper := inspected.Container
			if helper.Config == nil || helper.ID == record.Target || helper.Name != "/"+record.Name || (helper.Config.Image != debugImage && helper.Config.Image != maintenanceImageID) || helper.Config.Labels["dev.orbwin.transient"] != record.Kind || helper.Config.Labels["dev.orbwin.session"] != record.Session || helper.Config.Labels["dev.orbwin.target"] != record.Target {
				return errors.New("transient metadata conflicts with its durable intent; no object was removed")
			}
			_, err = cli.ContainerRemove(ctx, helper.ID, client.ContainerRemoveOptions{Force: true})
			if err != nil && !cerrdefs.IsNotFound(err) {
				return err
			}
			removed++
			lease.Close()
			return removeIntent(file, lock)
		}()
		lease.Close()
		if cleanupErr != nil {
			return removed, fmt.Errorf("recover %s: %w", entry.Name(), cleanupErr)
		}
	}
	return removed, nil
}
