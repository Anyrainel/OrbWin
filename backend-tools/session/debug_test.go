package main

import (
	"bufio"
	"encoding/json"
	"github.com/moby/moby/client"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDebugRefusesTargetRestartDuringToolPreparation(t *testing.T) {
	target := strings.Repeat("a", 64)
	inspections, creates := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/images/") {
			json.NewEncoder(w).Encode(maintenanceInspection(t, "amd64").InspectResponse)
			return
		}
		if strings.Contains(r.URL.Path, "/containers/"+target+"/json") {
			inspections++
			start := "before"
			if inspections > 1 {
				start = "after"
			}
			json.NewEncoder(w).Encode(map[string]any{"Id": target, "State": map[string]any{"Running": true, "Paused": false, "StartedAt": start}})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/containers/create") {
			creates++
		}
		http.Error(w, "Unexpected request", 500)
	}))
	defer server.Close()
	cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.56"))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	_, err = runDebug(cli, message{Container: target, Session: "00000000-0000-4000-8000-000000000121"}, bufio.NewScanner(strings.NewReader("")), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "changed while preparing tools") || creates != 0 || inspections != 2 {
		t.Fatalf("stale debug target reached create: %v, creates %d, inspections %d", err, creates, inspections)
	}
}

func TestDebugBoundaryAndOwnership(t *testing.T) {
	msg := message{Mode: "debug", Kind: "open", DaemonID: "owned", Container: strings.Repeat("a", 64), Session: "00000000-0000-4000-8000-000000000121", Shell: "sh", Cols: 80, Rows: 24}
	if err := validateOpen(msg); err != nil {
		t.Fatal(err)
	}
	options, err := debugConfiguration(msg)
	if err != nil {
		t.Fatal(err)
	}
	host := options.HostConfig
	if host.Privileged || len(host.Binds) != 0 || len(host.Mounts) != 0 || string(host.PidMode) != "container:"+msg.Container || string(host.NetworkMode) != "container:"+msg.Container || !host.ReadonlyRootfs || !host.AutoRemove {
		t.Fatal("debug boundary changed")
	}
	if !options.Config.OpenStdin || !options.Config.StdinOnce || options.Config.Labels["dev.orbwin.target"] != msg.Container || options.Config.Image != maintenanceImageID {
		t.Fatal("debug ownership/EOF contract changed")
	}
	if strings.Join(host.CapAdd, ",") != "SYS_PTRACE,DAC_READ_SEARCH" || strings.Join(host.CapDrop, ",") != "ALL" {
		t.Fatal("debug privileges expanded")
	}
	msg.Session = "--foreign"
	if _, err := debugConfiguration(msg); err == nil {
		t.Fatal("accepted a foreign helper name")
	}
	msg.Shell = "bash"
	if validateOpen(msg) == nil {
		t.Fatal("debug silently selected unavailable tooling")
	}
}
