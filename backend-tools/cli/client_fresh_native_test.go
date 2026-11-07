//go:build windows

package main

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

// These are byte-parser tests only. They do not open filesystem objects or
// exercise native admission; actual sharing, identity, and closure need a
// separately reviewed disposable qualification against the native provider.
func freshTestDirectoryEntry(name string, next uint32) []byte {
	units := utf16.Encode([]rune(name))
	size := 104 + 2*len(units)
	if int(next) > size {
		size = int(next)
	}
	body := make([]byte, size)
	binary.LittleEndian.PutUint32(body, next)
	binary.LittleEndian.PutUint32(body[60:], uint32(2*len(units)))
	for i, u := range units {
		binary.LittleEndian.PutUint16(body[104+2*i:], u)
	}
	return body
}
func TestFreshNativeDirectoryClosedInventory(t *testing.T) {
	body := append(freshTestDirectoryEntry("client.init", 128), freshTestDirectoryEntry("docker", 0)...)
	names, e := freshDirectoryBatch(body, false, map[string]bool{})
	if e != nil || len(names) != 2 || names[0] != "client.init" || names[1] != "docker" {
		t.Fatal("valid native closed inventory refused")
	}
	tests := []struct {
		name   string
		body   []byte
		docker bool
		seen   map[string]bool
	}{
		{"unknown", freshTestDirectoryEntry("writer.intent", 0), false, nil},
		{"alternate case", freshTestDirectoryEntry("Config.json", 0), true, nil},
		{"root file in docker", freshTestDirectoryEntry("client.init", 0), true, nil},
		{"duplicate across batches", freshTestDirectoryEntry("config.json", 0), true, map[string]bool{"config.json": true}},
		{"embedded nul", freshTestDirectoryEntry("config.json\x00ignored", 0), true, nil},
		{"short header", make([]byte, 103), false, nil},
		{"overlapping offset", append(freshTestDirectoryEntry("docker", 104), freshTestDirectoryEntry("client.init", 0)...), false, nil},
	}
	duplicate := append(freshTestDirectoryEntry("client.init", 128), freshTestDirectoryEntry("client.init", 0)...)
	tests = append(tests, struct {
		name   string
		body   []byte
		docker bool
		seen   map[string]bool
	}{"duplicate same batch", duplicate, false, nil})
	odd := freshTestDirectoryEntry("docker", 0)
	binary.LittleEndian.PutUint32(odd[60:], 3)
	tests = append(tests, struct {
		name   string
		body   []byte
		docker bool
		seen   map[string]bool
	}{"odd utf16 length", odd, false, nil})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seen := tt.seen
			if seen == nil {
				seen = map[string]bool{}
			}
			if _, e := freshDirectoryBatch(tt.body, tt.docker, seen); e == nil {
				t.Fatal("unsafe native inventory accepted")
			}
		})
	}
}
func TestFreshNativeDirectoryDotAndBounds(t *testing.T) {
	body := append(freshTestDirectoryEntry(".", 112), freshTestDirectoryEntry("config.json", 0)...)
	names, e := freshDirectoryBatch(body, true, map[string]bool{})
	if e != nil || len(names) != 1 || names[0] != "config.json" {
		t.Fatal("native dot entry changed closed inventory")
	}
	for _, next := range []uint32{1, 103, 113, 65528, 0xffffffff} {
		body := freshTestDirectoryEntry("docker", 0)
		binary.LittleEndian.PutUint32(body, next)
		if _, e := freshDirectoryBatch(body, false, map[string]bool{}); e == nil {
			t.Fatalf("malformed next offset %d accepted", next)
		}
	}
	body = freshTestDirectoryEntry("docker", 0)
	binary.LittleEndian.PutUint32(body[60:], 512)
	if _, e := freshDirectoryBatch(body, false, map[string]bool{}); e == nil {
		t.Fatal("oversized name accepted")
	}
}
