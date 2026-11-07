//go:build windows

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/moby/moby/client"
)

func TestTransientRecoveryNeverAdoptsActiveOrConflictingResources(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("ORBWIN_TRANSIENT_DIR", directory)
	id := strings.Repeat("a", 64)
	target := strings.Repeat("b", 64)
	var deleted atomic.Int32
	var returned any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			deleted.Add(1)
			w.WriteHeader(204)
			return
		}
		if returned == nil {
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]string{"message": "No such container"})
			return
		}
		json.NewEncoder(w).Encode(returned)
	}))
	defer server.Close()
	cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.56"))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	record, err := beginTransient(cli, "debug", "00000000-0000-4000-8000-000000000121", target)
	if err != nil {
		t.Fatal(err)
	}
	if err = record.created(id); err != nil {
		t.Fatal(err)
	}
	if _, err = recoverTransients(cli, directory); err != nil || deleted.Load() != 0 {
		t.Fatal("recovery touched an active lease", err)
	}
	if err = record.close(); err != nil {
		t.Fatal(err)
	}
	returned = map[string]any{"Id": id, "Name": "/" + record.intent.Name, "Config": map[string]any{"Image": debugImage, "Labels": map[string]string{"dev.orbwin.session": "foreign"}}}
	if _, err = recoverTransients(cli, directory); err == nil || deleted.Load() != 0 {
		t.Fatal("conflicting resource was adopted")
	}
	returned = map[string]any{"Id": id, "Name": "/" + record.intent.Name, "Config": map[string]any{"Image": debugImage, "Labels": map[string]string{"dev.orbwin.transient": "debug", "dev.orbwin.session": record.intent.Session, "dev.orbwin.target": target}}}
	if _, err = recoverTransients(cli, directory); err != nil || deleted.Load() != 1 {
		t.Fatal("abandoned exact helper was not recovered", err)
	}
	if _, err = os.Stat(record.file); !os.IsNotExist(err) {
		t.Fatal("settled intent retained")
	}
	// A lost create response may precede a late accepted create. Missing-by-name
	// is never rewritten into certainty; it remains available for a later retry.
	pending, err := beginTransient(cli, "files", strings.Repeat("c", 32), "project_data")
	if err != nil {
		t.Fatal(err)
	}
	pending.close()
	returned = nil
	if _, err = recoverTransients(cli, directory); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(pending.file); err != nil {
		t.Fatal("unknown accepted create intent discarded")
	}
	// Once the created full ID is recorded, absent means that exact object is gone.
	reopened, err := os.OpenFile(pending.file, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	reopened.WriteString(id + "\n")
	reopened.Close()
	if _, err = recoverTransients(cli, directory); err != nil {
		t.Fatal(err)
	}
	matches, _ := filepath.Glob(filepath.Join(directory, "*.json"))
	if len(matches) != 0 {
		t.Fatal("known removed objects retained intents")
	}
}
