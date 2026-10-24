package main

import (
	"archive/tar"
	"bytes"
	"strings"
	"testing"
)

type archiveItem struct {
	name string
	kind byte
	data string
	size int64
}

func TestDockerRootArchiveUsesAbsoluteEntries(t *testing.T) {
	data := fixtureArchive(t, archiveItem{name: "/", kind: tar.TypeDir}, archiveItem{name: "/bin/", kind: tar.TypeDir}, archiveItem{name: "/bin/nested", kind: tar.TypeReg}, archiveItem{name: "/.dockerenv", kind: tar.TypeReg})
	r, err := readArchive(bytes.NewReader(data), "list")
	if err != nil || !r.Complete || len(r.Entries) != 2 || r.Entries[0].Name != "bin" {
		t.Fatalf("%+v %v", r, err)
	}
	data = fixtureArchive(t, archiveItem{name: "/", kind: tar.TypeDir}, archiveItem{name: "/../escape", kind: tar.TypeReg})
	if _, err := readArchive(bytes.NewReader(data), "list"); err == nil {
		t.Fatal("accepted root traversal")
	}
}

func TestArchiveDoesNotReadUnboundedLogicalPayload(t *testing.T) {
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	if err := w.WriteHeader(&tar.Header{Name: "data/", Typeflag: tar.TypeDir, Mode: 0755}); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteHeader(&tar.Header{Name: "data/huge", Typeflag: tar.TypeReg, Mode: 0644, Size: 1 << 50}); err != nil {
		t.Fatal(err)
	}
	// No payload is supplied: the listing must stop at the declared budget,
	// without asking the reader to skip/expand the huge entry.
	r, err := readArchive(bytes.NewReader(b.Bytes()), "list")
	if err != nil || r.Complete || len(r.Entries) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
}

func fixtureArchive(t *testing.T, items ...archiveItem) []byte {
	t.Helper()
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	for _, item := range items {
		size := int64(len(item.data))
		if item.size > 0 {
			size = item.size
		}
		if err := w.WriteHeader(&tar.Header{Name: item.name, Typeflag: item.kind, Mode: 0644, Size: size}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(item.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestArchiveListsOnlyDirectEntries(t *testing.T) {
	data := fixtureArchive(t, archiveItem{name: "data/", kind: tar.TypeDir}, archiveItem{name: "data/sub/", kind: tar.TypeDir}, archiveItem{name: "data/sub/nested", kind: tar.TypeReg, data: "nested"}, archiveItem{name: "data/space ü.txt", kind: tar.TypeReg, data: "hello"})
	r, e := readArchive(bytes.NewReader(data), "list")
	if e != nil || !r.Complete || len(r.Entries) != 2 || r.Entries[1].Name != "space ü.txt" || r.Entries[1].Size != 5 {
		t.Fatalf("%+v %v", r, e)
	}
}
func TestArchiveRejectsEscapesDuplicatesAndNonText(t *testing.T) {
	for _, name := range []string{"../outside", "/outside", "data/../../outside", "elsewhere"} {
		data := fixtureArchive(t, archiveItem{name: "data/", kind: tar.TypeDir}, archiveItem{name: name, kind: tar.TypeReg})
		if _, err := readArchive(bytes.NewReader(data), "list"); err == nil {
			t.Fatalf("accepted %q", name)
		}
	}
	for _, item := range []archiveItem{{name: "binary", kind: tar.TypeReg, data: "a\x00b"}, {name: "bad-utf8", kind: tar.TypeReg, data: "\xff"}, {name: "link", kind: tar.TypeSymlink}} {
		if _, err := readArchive(bytes.NewReader(fixtureArchive(t, item)), "read"); err == nil {
			t.Fatalf("accepted %+v", item)
		}
	}
	data := fixtureArchive(t, archiveItem{name: "data/", kind: tar.TypeDir}, archiveItem{name: "data/a", kind: tar.TypeReg}, archiveItem{name: "data/a", kind: tar.TypeReg})
	if _, err := readArchive(bytes.NewReader(data), "list"); err == nil {
		t.Fatal("accepted duplicate")
	}
}
func TestArchiveLimitsAndTextPreview(t *testing.T) {
	data := fixtureArchive(t, archiveItem{name: "data/", kind: tar.TypeDir}, archiveItem{name: "data/big", kind: tar.TypeReg, data: strings.Repeat("x", archiveBudget+1024)})
	r, err := readArchive(bytes.NewReader(data), "list")
	if err != nil || r.Complete || len(r.Entries) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err := readArchive(bytes.NewReader(data[512:]), "read"); err == nil {
		t.Fatal("accepted oversized preview")
	}
	r, err = readArchive(bytes.NewReader(fixtureArchive(t, archiveItem{name: "hello", kind: tar.TypeReg, data: "hello ü\n"})), "read")
	if err != nil || r.Text == nil || *r.Text != "hello ü\n" || !r.Complete {
		t.Fatalf("%+v %v", r, err)
	}
}
func TestFileArguments(t *testing.T) {
	id := strings.Repeat("a", 64)
	for _, source := range []string{"relative", "/a\x00b", "C:\\file", strings.Repeat("/", 2049)} {
		if fileArguments("daemon", id, source, "list") == nil {
			t.Fatal(source)
		}
	}
	if err := fileArguments("daemon", id, "/space ü/file", "read"); err != nil {
		t.Fatal(err)
	}
}
