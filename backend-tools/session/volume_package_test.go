package main

import (
	"archive/tar"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

func portableFixture(t *testing.T) (volumeManifest, []byte) {
	t.Helper()
	var source, payload bytes.Buffer
	writer := tar.NewWriter(&source)
	modified := time.Unix(1700000000, 123456789).UTC()
	for _, header := range []*tar.Header{
		{Name: "source/", Typeflag: tar.TypeDir, Mode: 0750},
		{Name: "source/project/", Typeflag: tar.TypeDir, Mode: 0700},
		{Name: "source/project/例.txt", Typeflag: tar.TypeReg, Mode: 0640, Uid: 1234, Gid: 2345, ModTime: modified, Size: 6},
		{Name: "source/linked", Typeflag: tar.TypeSymlink, Mode: 0777, Linkname: "project/例.txt"},
		{Name: "source/hard", Typeflag: tar.TypeLink, Mode: 0640, Linkname: "source/project/例.txt"},
	} {
		header.Format = tar.FormatPAX
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Size > 0 {
			_, _ = writer.Write([]byte{0, 1, 2, 3, 4, 255})
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	manifest, err := collectVolumeArchive(&source, &payload, "verified-engine", "project_data")
	if err != nil {
		t.Fatal(err)
	}
	return manifest, payload.Bytes()
}
func TestPortableVolumeRoundTripPreservesBytesLinksAndNumericMetadata(t *testing.T) {
	manifest, payload := portableFixture(t)
	var packed bytes.Buffer
	if err := writeVolumePackage(&packed, manifest, bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	accepted, start, err := readVolumePackage(bytes.NewReader(packed.Bytes()), int64(packed.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var restored bytes.Buffer
	if err := volumeRestoreArchive(&restored, bytes.NewReader(packed.Bytes()), start, accepted); err != nil {
		t.Fatal(err)
	}
	reader := tar.NewReader(&restored)
	found := map[string]*tar.Header{}
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		found[header.Name] = header
		if header.Typeflag == tar.TypeReg {
			data, _ := io.ReadAll(reader)
			if !bytes.Equal(data, payload) || header.Mode != 0640 || header.Uid != 1234 || header.Gid != 2345 || header.ModTime.Nanosecond() != 123456789 {
				t.Fatalf("file data/metadata changed: %+v %v", header, data)
			}
		}
	}
	if found["source/linked"].Linkname != "project/例.txt" || found["source/hard"].Linkname != "source/project/例.txt" || found["source/project"].Mode != 0700 {
		t.Fatal("links/directory metadata changed")
	}
}

func TestManifestIdentityBindsMetadataAndSourceBeyondPayload(t *testing.T) {
	original, payload := portableFixture(t)
	want, err := volumeManifestIdentity(original)
	if err != nil || !digestValid(want) {
		t.Fatalf("identity: %s %v", want, err)
	}
	var packed bytes.Buffer
	if err := writeVolumePackage(&packed, original, bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	decoded, _, err := readVolumePackage(bytes.NewReader(packed.Bytes()), int64(packed.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := volumeManifestIdentity(decoded); err != nil || got != want {
		t.Fatalf("round trip changed identity: %s %v", got, err)
	}
	for _, change := range []func(*volumeManifest){
		func(m *volumeManifest) { m.Engine = "other-engine" },
		func(m *volumeManifest) { m.Volume = "other_volume" },
		func(m *volumeManifest) { m.Created = m.Created.Add(time.Second) },
		func(m *volumeManifest) { m.Entries[0].Mode = 0700 },
		func(m *volumeManifest) { m.Entries[0].UID++ },
		func(m *volumeManifest) { m.Entries[0].Modified = m.Entries[0].Modified.Add(time.Second) },
	} {
		changed := original
		changed.Entries = append([]volumeEntry(nil), original.Entries...)
		change(&changed)
		got, err := volumeManifestIdentity(changed)
		if err != nil || got == want || changed.SHA256 != original.SHA256 {
			t.Fatalf("metadata/source not distinguished: %s %v", got, err)
		}
	}
	bad := original
	bad.Engine = "engine\ncontrol"
	if _, err := volumeManifestIdentity(bad); err == nil {
		t.Fatal("control-bearing provenance accepted")
	}
}

func TestEmptyVolumesCannotLinkByEmptyPayloadAlone(t *testing.T) {
	var archive bytes.Buffer
	writer := tar.NewWriter(&archive)
	if err := writer.WriteHeader(&tar.Header{Name: "source/", Typeflag: tar.TypeDir, Mode: 0750}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	first, err := collectVolumeArchive(&archive, io.Discard, "source", "empty_a")
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.Volume = "empty_b"
	left, err := volumeManifestIdentity(first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := volumeManifestIdentity(second)
	if err != nil || left == right || first.SHA256 != second.SHA256 {
		t.Fatalf("empty payload falsely identifies volumes: %s %s %v", left, right, err)
	}
}

func TestVolumeVerificationComparesInodeGroupsRegardlessOfTarRepresentative(t *testing.T) {
	original, _ := portableFixture(t)
	reversed := original
	reversed.Entries = append([]volumeEntry(nil), original.Entries...)
	file, link := reversed.Entries[2], reversed.Entries[4]
	file.Path, link.Path = link.Path, file.Path
	link.Link = file.Path
	reversed.Entries[2], reversed.Entries[4] = file, link
	if !equivalentVolumeTrees(original, reversed) {
		t.Fatal("equivalent hard-linked content rejected")
	}
	reversed.Entries[2].Mode = 0600
	if equivalentVolumeTrees(original, reversed) {
		t.Fatal("changed inode metadata was accepted")
	}
}
func TestPortableVolumeRejectsMalformedAndChangingPayload(t *testing.T) {
	manifest, payload := portableFixture(t)
	var packed bytes.Buffer
	if err := writeVolumePackage(&packed, manifest, bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	original := packed.Bytes()
	for _, data := range [][]byte{original[:len(original)-1], append(append([]byte{}, original...), 0)} {
		if _, _, err := readVolumePackage(bytes.NewReader(data), int64(len(data))); err == nil {
			t.Fatal("accepted truncated/trailing package")
		}
	}
	changed := append([]byte{}, original...)
	changed[len(changed)-1] ^= 1
	if _, _, err := readVolumePackage(bytes.NewReader(changed), int64(len(changed))); err == nil {
		t.Fatal("accepted corrupt payload")
	}
	_, start, err := readVolumePackage(bytes.NewReader(original), int64(len(original)))
	if err != nil {
		t.Fatal(err)
	}
	if err = volumeRestoreArchive(io.Discard, bytes.NewReader(changed), start, manifest); err == nil {
		t.Fatal("concurrent change was not detected during restore")
	}
	// Excessive manifest lengths refuse before allocation or payload reads.
	malformed := append([]byte{}, original...)
	binary.LittleEndian.PutUint64(malformed[len(volumeMagic):], 1<<40)
	if _, _, err = readVolumePackage(bytes.NewReader(malformed), int64(len(malformed))); err == nil {
		t.Fatal("accepted excessive manifest allocation")
	}
}
func TestPortableVolumeRefusesEscapesLinkAncestryAndUnsupportedMetadata(t *testing.T) {
	manifest, _ := portableFixture(t)
	for _, mutate := range []func(*volumeManifest){
		func(m *volumeManifest) { m.Entries[2].Path = "../outside" },
		func(m *volumeManifest) { m.Entries[2].Path = "/absolute" },
		func(m *volumeManifest) { m.Entries[2].Offset++ },
		func(m *volumeManifest) { m.Entries[3].Link = "../../outside" },
		func(m *volumeManifest) { m.Entries[3].Link = "/outside" },
		func(m *volumeManifest) { m.Entries[3].Path = "project" },
		func(m *volumeManifest) { m.Entries[4].Link = "linked" },
		func(m *volumeManifest) { m.Entries[2].Path = "linked/child" },
		func(m *volumeManifest) { m.Entries[2].Path = "project/missing/child" },
		func(m *volumeManifest) { m.Entries[2].Kind = "device" },
	} {
		clone := manifest
		clone.Entries = append([]volumeEntry(nil), manifest.Entries...)
		mutate(&clone)
		if clone.validate() == nil {
			data, _ := json.Marshal(clone)
			t.Fatalf("accepted unsafe manifest: %s", data)
		}
	}
	for _, header := range []*tar.Header{
		{Name: "source/pipe", Typeflag: tar.TypeFifo, Mode: 0600},
		{Name: "source/file", Typeflag: tar.TypeReg, Mode: 0600, Xattrs: map[string]string{"user.secret": "retained"}},
		{Name: "source/file", Typeflag: tar.TypeReg, Mode: 0600, PAXRecords: map[string]string{"SCHILY.acl.access": "unsupported"}},
	} {
		var data bytes.Buffer
		writer := tar.NewWriter(&data)
		_ = writer.WriteHeader(&tar.Header{Name: "source", Typeflag: tar.TypeDir, Mode: 0755})
		header.Format = tar.FormatPAX
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		_ = writer.Close()
		if _, err := collectVolumeArchive(&data, io.Discard, "engine", "data"); err == nil {
			t.Fatalf("accepted unsupported metadata: %+v", header)
		}
	}
	// A package is not accepted solely because its manifest is valid JSON.
	bad := strings.NewReader("{}")
	if _, _, err := readVolumePackage(bad, 2); err == nil {
		t.Fatal("accepted non-package")
	}
}
