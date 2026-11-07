package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// Stateful storage model: writes are volatile until flush; an unknown close
// leaves write authority outstanding. Existing files cannot be overwritten.
type memoryPrivateRecord struct {
	failure              string
	exists, writer, held bool
	volatile, durable    []byte
	reads                int
}

func (m *memoryPrivateRecord) createNew() error {
	if m.exists || m.failure == "create" {
		return errors.New("create refused")
	}
	m.exists, m.writer = true, true
	return nil
}
func (m *memoryPrivateRecord) writeAll(data []byte) error {
	if !m.writer {
		return errors.New("no writer")
	}
	m.volatile = append([]byte(nil), data...)
	if m.failure == "write" {
		m.volatile = m.volatile[:len(m.volatile)/2]
		return errors.New("partial write")
	}
	return nil
}
func (m *memoryPrivateRecord) flush() error {
	if m.failure == "flush" {
		return errors.New("flush unknown")
	}
	m.durable = append([]byte(nil), m.volatile...)
	return nil
}
func (m *memoryPrivateRecord) readWritten() ([]byte, error) {
	m.reads++
	if m.failure == "readback" {
		return []byte("different"), nil
	}
	return append([]byte(nil), m.volatile...), nil
}
func (m *memoryPrivateRecord) closeWriter() error {
	if m.failure == "close" {
		return errors.New("writer close unknown")
	}
	m.writer = false
	return nil
}
func (m *memoryPrivateRecord) openReadOnly() error {
	if m.writer || m.failure == "open" {
		return errors.New("held admission refused")
	}
	m.held = true
	return nil
}
func (m *memoryPrivateRecord) readHeld() ([]byte, error) {
	m.reads++
	if m.failure == "reopen-drift" {
		return []byte("different"), nil
	}
	if !m.held {
		return nil, errors.New("no held object")
	}
	return append([]byte(nil), m.durable...), nil
}
func TestPrivateOriginalExactDurableBytesSurviveTypedSave(t *testing.T) {
	original := []byte("{\n  \"unknown\": 123456789012345678901234567890, \"plugins\": {\"future\": [1,  2]}\n}\n")
	storage := &memoryPrivateRecord{}
	if e := durablePrivateRecord(original, storage); e != nil {
		t.Fatal(e)
	}
	// The selected typed serializer can replace the target with a lossy object.
	target := []byte(`{}`)
	storage.volatile = nil // abrupt owner loss; only flushed storage survives.
	if storage.writer || !storage.held || !bytes.Equal(storage.durable, original) || bytes.Equal(target, original) {
		t.Fatal("exact durable baseline unavailable after typed save")
	}
	existing := append([]byte(nil), storage.durable...)
	if durablePrivateRecord(target, storage) == nil || !bytes.Equal(storage.durable, existing) {
		t.Fatal("existing original overwritten")
	}
}
func TestPrivateOriginalFaultsNeverAdmitAnUnsafeLoan(t *testing.T) {
	original := []byte(`{"unknown":[1,  2],"large":123456789012345678901234567890}`)
	for _, failure := range []string{"create", "write", "flush", "readback", "close", "open", "reopen-drift"} {
		t.Run(failure, func(t *testing.T) {
			storage := &memoryPrivateRecord{failure: failure}
			if durablePrivateRecord(original, storage) == nil {
				t.Fatal("unsafe original admitted")
			}
			if failure == "create" {
				if storage.exists {
					t.Fatal("failed CREATE_NEW produced evidence")
				}
				return
			}
			if !storage.exists {
				t.Fatal("failed creation evidence removed")
			}
			if failure == "close" && (!storage.writer || storage.held) {
				t.Fatal("unknown writer close released authority")
			}
			if failure == "write" && (len(storage.durable) != 0 || storage.held) {
				t.Fatal("partial write admitted")
			}
			if failure == "flush" && (len(storage.durable) != 0 || storage.held) {
				t.Fatal("unflushed write admitted")
			}
			if failure == "close" || failure == "open" || failure == "reopen-drift" {
				if !bytes.Equal(storage.durable, original) {
					t.Fatal("flushed original evidence lost")
				}
			}
		})
	}
	storage := &memoryPrivateRecord{}
	if durablePrivateRecord(make([]byte, clientConfigBudget+1), storage) == nil || storage.exists {
		t.Fatal("oversize original created")
	}
}
func TestPrivateOriginalDriftBlocksNaturalAndNoStartCleanup(t *testing.T) {
	for _, unstarted := range []bool{false, true} {
		g, native := guardedConfig(t)
		if _, e := g.beginWriter(t.Context()); e != nil {
			t.Fatal(e)
		}
		native.original = []byte(`{"credsStore":"orbwin","cliPluginsExtraDirs":["pinned"],"unknown":"drift"}`)
		var e error
		if unstarted {
			e = g.abortWith(func([32]byte) bool { return true })
		} else {
			e = g.finishWith(func([32]byte) bool { return true })
		}
		if e == nil || !g.poison || g.loan == nil {
			t.Fatal("drift cleared pending loan")
		}
		calls := strings.Join(native.calls, ",")
		if strings.Contains(calls, "finish-intent") || strings.Contains(calls, "commit-merged-config") || strings.Contains(calls, "verify-original-and-abort-intent") {
			t.Fatal(calls)
		}
	}
}
func TestPrivateOrphanOriginalBlocksStartupWithoutIntent(t *testing.T) {
	for _, failure := range []string{"", "probe-unknown"} {
		files := map[string][]byte{"writer.original": []byte("exact original")}
		e := refusePendingPrivateRecords(func(name string) (bool, error) {
			if failure == "probe-unknown" {
				return false, errors.New("sharing or query unknown")
			}
			_, present := files[name]
			return present, nil
		})
		if e == nil || string(files["writer.original"]) != "exact original" {
			t.Fatal("orphan adopted or deleted")
		}
	}
	if e := refusePendingPrivateRecords(func(string) (bool, error) { return false, nil }); e != nil {
		t.Fatal(e)
	}
}
func TestPrivateCleanupFailureLeavesEvidenceAndStopsAdmission(t *testing.T) {
	// Both natural finish and verified NoStart share this native cleanup gate.
	for _, failure := range []string{"final-check", "intent-delete", "original-delete", ""} {
		files := map[string]bool{"writer.intent": true, "writer.original": true}
		verified := false
		e := finishPrivateMutation(func() error {
			if failure == "final-check" {
				return errors.New("final native identity unverified")
			}
			verified = true
			return nil
		}, func() error {
			if !verified {
				t.Fatal("cleanup before final native verification")
			}
			if failure == "intent-delete" {
				return errors.New("intent close/delete unknown")
			}
			delete(files, "writer.intent")
			return nil
		}, func() error {
			if files["writer.intent"] {
				t.Fatal("original removed before intent")
			}
			if failure == "original-delete" {
				return errors.New("original delete unknown")
			}
			delete(files, "writer.original")
			return nil
		})
		if failure == "" {
			if e != nil || len(files) != 0 {
				t.Fatal("clean settlement failed")
			}
			continue
		}
		if e == nil || !files["writer.original"] {
			t.Fatal("failed cleanup lost baseline")
		}
		if failure != "original-delete" && !files["writer.intent"] {
			t.Fatal("earlier failure cleared intent")
		}
		if refusePendingPrivateRecords(func(name string) (bool, error) { return files[name], nil }) == nil {
			t.Fatal("failed cleanup permitted startup")
		}
	}
}

func TestPrivateWriterKeyDigestBytesMatchRustMetadataEncoding(t *testing.T) {
	data, e := writerKeysJSON([]string{"a<&>", "é\u2028\u2029"})
	if e != nil || string(data) != `["a\u003c\u0026\u003e","é\u2028\u2029"]` {
		t.Fatal("metadata encoding differs")
	}
	data, e = writerKeysJSON(nil)
	if e != nil || string(data) != "[]" {
		t.Fatal("empty key scope differs")
	}
}
