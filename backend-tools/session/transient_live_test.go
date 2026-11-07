//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

func TestOwnedTransientCreateInterruptionAndLateAcceptance(t *testing.T) {
	if os.Getenv("ORBWIN_QUALIFY_OWNED") != "1" {
		t.Skip("explicit verified owned daemon qualification")
	}
	daemon := "d9c00194-ea9d-439a-8561-6a0ec580fbd5"
	cli, err := client.New(client.WithHost(endpoint), client.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	info, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil || info.Info.ID != daemon || info.Info.OSType != "linux" {
		t.Fatal("owned engine mismatch", err)
	}
	directory := t.TempDir()
	t.Setenv("ORBWIN_TRANSIENT_DIR", directory)
	session := fmt.Sprintf("%032x", time.Now().UnixNano())
	target := strings.Repeat("b", 64)
	record, err := beginTransient(cli, "files", session, target)
	if err != nil {
		t.Fatal(err)
	}
	options := client.ContainerCreateOptions{Name: record.intent.Name, Config: &container.Config{Image: debugImage, Cmd: []string{"/bin/sleep", "30"}, Labels: map[string]string{"dev.orbwin.transient": "files", "dev.orbwin.session": session, "dev.orbwin.target": target}}, HostConfig: &container.HostConfig{NetworkMode: "none", ReadonlyRootfs: true, CapDrop: []string{"ALL"}}}
	created, err := cli.ContainerCreate(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.ContainerRemove(context.Background(), created.ID, client.ContainerRemoveOptions{Force: true})
	if err = record.created(created.ID); err != nil {
		t.Fatal(err)
	}
	if count, err := recoverTransients(cli, directory); err != nil || count != 0 {
		t.Fatal("active helper was adopted", count, err)
	}
	if err = record.close(); err != nil {
		t.Fatal(err)
	} // OS releases this lease when a client dies.
	recovered, err := recoverTransients(cli, directory)
	if err != nil || recovered != 1 {
		t.Fatal("stopped pre-start helper was not removed", recovered, err)
	}
	lateSession := fmt.Sprintf("%032x", time.Now().UnixNano())
	late, err := beginTransient(cli, "files", lateSession, target)
	if err != nil {
		t.Fatal(err)
	}
	if err = late.close(); err != nil {
		t.Fatal(err)
	}
	before, err := recoverTransients(cli, directory)
	if err != nil || before != 0 {
		t.Fatal(err)
	}
	if _, err = os.Stat(late.file); err != nil {
		t.Fatal("unknown create intent was discarded", err)
	}
	options.Name = late.intent.Name
	options.Config.Labels["dev.orbwin.session"] = lateSession
	accepted, err := cli.ContainerCreate(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.ContainerRemove(context.Background(), accepted.ID, client.ContainerRemoveOptions{Force: true})
	after, err := recoverTransients(cli, directory)
	if err != nil || after != 1 {
		t.Fatal("late accepted create was not recovered", after, err)
	}
	evidence := map[string]any{"daemon": daemon, "preStartHelper": created.ID, "activePreserved": true, "abandonedRemoved": recovered, "unknownCreateRetained": true, "lateHelper": accepted.ID, "lateRecovered": after}
	output := filepath.Join("..", "..", "artifacts", "qualification", fmt.Sprintf("transient-recovery-%s.json", session))
	raw, _ := json.MarshalIndent(evidence, "", "  ")
	if err = os.WriteFile(output, raw, 0600); err != nil {
		t.Fatal(err)
	}
}
