package main

import (
	"encoding/json"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"strings"
	"testing"
)

func TestMigrationMountRootsAndPrivatePaths(t *testing.T) {
	roots := map[string]string{"data": "/private-engine-volume/data"}
	for _, test := range []struct {
		kind      mount.Type
		source    string
		supported bool
	}{
		{mount.TypeVolume, "/private-engine-volume/data", true},
		{mount.TypeVolume, "/private-engine-volume/data/subpath", false},
		{mount.TypeVolume, "", false},
		{mount.TypeBind, "/private-host-secret", false},
	} {
		got := projectMigrationMount(container.MountPoint{Type: test.kind, Name: "data", Source: test.source, Destination: "/project/$literal", RW: false}, roots)
		if got.MappingSupported != test.supported || got.Target != "/project/$literal" || !got.ReadOnly {
			t.Fatalf("incorrect mapping projection: %+v", got)
		}
		raw, _ := json.Marshal(got)
		if strings.Contains(string(raw), "private") {
			t.Fatal("private host/engine mount source escaped projection")
		}
	}
}
