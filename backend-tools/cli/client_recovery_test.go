package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func recoveryFixture(t *testing.T) (writerIntentRecord, []byte, writerBirthRecord, []byte) {
	t.Helper()
	d := digestBytes([]byte("original"))
	i := writerIntentRecord{Schema: 3, Kind: "docker-writer", Mutation: string(writerRegistryAuth), Binding: d, KeysSHA: d, BaseSHA: d, BaseVolume: 2, BaseHigh: 3, BaseLow: 4, BaseSD: d, OriginalVolume: 2, OriginalHigh: 5, OriginalLow: 6, OriginalSD: d, OriginalSHA: d}
	ib, e := json.Marshal(i)
	if e != nil {
		t.Fatal(e)
	}
	b := writerBirthRecord{Schema: 3, Kind: "docker-writer-birth", Binding: d, IntentSHA: digestBytes(ib), IntentBody: string(ib), RootPID: 1234, RootCreated: 987654321, RootSession: 2}
	bb, e := json.Marshal(b)
	if e != nil {
		t.Fatal(e)
	}
	return i, ib, b, bb
}

func TestWriterRecoveryClosedMetadataAndExactEmbeddedIntent(t *testing.T) {
	i, ib, b, bb := recoveryFixture(t)
	if _, e := parseWriterIntent(ib); e != nil {
		t.Fatal(e)
	}
	if _, got, e := parseWriterBirth(bb); e != nil || got != i {
		t.Fatalf("birth binding: %v", e)
	}
	for name, mutate := range map[string]func() []byte{
		"legacy":    func() []byte { v := i; v.Schema = 2; out, _ := json.Marshal(v); return out },
		"unknown":   func() []byte { return append(append([]byte(nil), ib[:len(ib)-1]...), []byte(`,"extra":true}`)...) },
		"duplicate": func() []byte { return append(append([]byte(nil), ib[:len(ib)-1]...), []byte(`,"schema":3}`)...) },
		"alias":     func() []byte { return bytes.Replace(ib, []byte(`"schema"`), []byte(`"Schema"`), 1) },
		"numericOverflow": func() []byte {
			return bytes.Replace(ib, []byte(`"baseVolume":2`), []byte(`"baseVolume":4294967296`), 1)
		},
		"uppercaseDigest": func() []byte { v := i; v.Binding = strings.ToUpper(v.Binding); out, _ := json.Marshal(v); return out },
		"budget":          func() []byte { return bytes.Repeat([]byte(" "), 4097) },
	} {
		t.Run(name, func(t *testing.T) {
			if _, e := parseWriterIntent(mutate()); e == nil {
				t.Fatal("unsafe intent accepted")
			}
		})
	}
	for name, mutate := range map[string]func() writerBirthRecord{
		"intentWhitespaceChanged": func() writerBirthRecord { v := b; v.IntentBody = " " + v.IntentBody; return v },
		"bindingChanged":          func() writerBirthRecord { v := b; v.Binding = digestBytes([]byte("other")); return v },
		"emptyPID":                func() writerBirthRecord { v := b; v.RootPID = 0; return v },
		"emptyCreation":           func() writerBirthRecord { v := b; v.RootCreated = 0; return v },
	} {
		t.Run(name, func(t *testing.T) {
			out, _ := json.Marshal(mutate())
			if _, _, e := parseWriterBirth(out); e == nil {
				t.Fatal("unsafe birth accepted")
			}
		})
	}
}

func TestWriterBirthFaultOrCancellationNeverResumes(t *testing.T) {
	_, _, _, birth := recoveryFixture(t)
	for _, fault := range []string{"create", "write", "flush", "readback", "close", "open", "reopen-drift", "cancel-after-durable", ""} {
		t.Run(fault, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			storage := &memoryPrivateRecord{failure: fault}
			resumed := false
			e := writerBeforeFirstResume(ctx, func() error {
				err := durablePrivateRecord(birth, storage)
				if fault == "cancel-after-durable" {
					cancel()
				}
				return err
			}, func() error {
				if storage.writer || !storage.held || !bytes.Equal(storage.durable, birth) {
					t.Fatal("user code before durable closed birth")
				}
				resumed = true
				return nil
			})
			if resumed != (fault == "") || (e == nil) != (fault == "") {
				t.Fatalf("fault=%q resumed=%v err=%v", fault, resumed, e)
			}
		})
	}
}

func TestWriterRecoveryIncompleteBirthRequiresEOFPrefix(t *testing.T) {
	_, _, _, birth := recoveryFixture(t)
	for end := 0; end < len(birth); end++ {
		if !incompleteWriterBirth(birth[:end]) {
			t.Fatalf("real write prefix %d was not EOF incomplete", end)
		}
	}
	for _, body := range [][]byte{birth, []byte(`{"schema":3,"invalid":!}`), []byte(`{"schema":2}`), []byte(`null`), []byte(`[`), []byte(`"`), []byte(" \t["), append(append([]byte(nil), birth...), '{')} {
		if incompleteWriterBirth(body) {
			t.Fatalf("complete or corrupt record called no-resume: %q", body)
		}
	}
}

func TestWriterRecoveryPartialRecoveryIntentIsNeverMetadataAuthority(t *testing.T) {
	i, ib, _, _ := recoveryFixture(t)
	r := writerRecoveryRecord{Schema: 1, Kind: "restore-original", Binding: i.Binding, CurrentVolume: 1, CurrentHigh: 2, CurrentLow: 3, CurrentSD: i.BaseSD, CurrentSHA: i.BaseSHA, RestoreSHA: i.BaseSHA, IntentBody: string(ib)}
	body, _ := json.Marshal(r)
	for end := 0; end < len(body); end++ {
		partial := body[:end]
		if !incompleteWriterBirth(partial) {
			t.Fatalf("real recovery write prefix %d not EOF incomplete", end)
		}
		if _, e := parseWriterRecovery(partial, i.Binding, i.BaseSHA); e == nil {
			t.Fatal("partial recovery supplied authority")
		}
		// Its replacement must obtain source binding independently, rather
		// than trusting any already-written subset of the recovery record.
		if source, e := parseWriterIntent(ib); e != nil || source != i {
			t.Fatal("independent source binding unavailable")
		}
	}
	for _, corrupt := range [][]byte{[]byte(`{"schema":1,"kind":!}`), append(append([]byte(nil), body...), '{')} {
		if incompleteWriterBirth(corrupt) {
			t.Fatal("complete corruption granted replaceable partial record")
		}
	}
}

func TestWriterRecoveryRootGenerationNeverUsesBarePID(t *testing.T) {
	_, _, b, _ := recoveryFixture(t)
	for _, c := range []struct {
		name     string
		actual   writerBirthRecord
		signaled bool
		err      error
		want     bool
	}{
		{"alive", b, false, nil, false}, {"signaled", b, true, nil, true},
		{"query-denied", b, true, errors.New("denied"), false},
		{"pid-reused", func() writerBirthRecord { v := b; v.RootCreated++; return v }(), false, nil, true},
		{"session-drift", func() writerBirthRecord { v := b; v.RootSession++; return v }(), true, nil, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := writerGenerationStopped(b, c.actual, c.signaled, c.err); got != c.want {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestWriterRecoveryCapabilityOneUseExpiryCancelAndFailedRecheck(t *testing.T) {
	for _, mode := range []string{"success", "native-fault", "expired", "cancelled", "closed"} {
		t.Run(mode, func(t *testing.T) {
			calls, closes := 0, 0
			r := &clientWriterRecovery{deadline: time.Now().Add(time.Minute), restoreNative: func() (*clientConfigGuard, error) {
				calls++
				if mode == "native-fault" {
					return nil, errClientConfigRecovery
				}
				return &clientConfigGuard{}, nil
			}, closeNative: func() error { closes++; return nil }}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "expired" {
				r.deadline = time.Now().Add(-time.Second)
			}
			if mode == "cancelled" {
				cancel()
			}
			if mode == "closed" {
				if e := r.close(); e != nil {
					t.Fatal(e)
				}
			}
			_, e := r.restore(ctx)
			if (e == nil) != (mode == "success") {
				t.Fatalf("result %v", e)
			}
			if _, e = r.restore(ctx); e == nil {
				t.Fatal("review replayed")
			}
			if e = r.close(); e != nil {
				t.Fatal(e)
			}
			wantCalls := 0
			if mode == "success" || mode == "native-fault" {
				wantCalls = 1
			}
			if calls != wantCalls || (mode != "success" && closes != 1) || (mode == "success" && closes != 0) {
				t.Fatalf("native calls=%d closes=%d", calls, closes)
			}
		})
	}
}

func TestWriterPrepareAndRecoveryKeepSourceBindingAcrossCleanupCuts(t *testing.T) {
	i, ib, _, _ := recoveryFixture(t)
	p := writerPrepareRecord{Schema: 3, Kind: "docker-writer-prepare", Mutation: i.Mutation, Binding: i.Binding, KeysSHA: i.KeysSHA, BaseSHA: i.BaseSHA, BaseVolume: i.BaseVolume, BaseHigh: i.BaseHigh, BaseLow: i.BaseLow, BaseSD: i.BaseSD}
	pb, _ := json.Marshal(p)
	parsed, e := parseWriterPrepare(pb)
	if e != nil || !parsed.matches(i) {
		t.Fatalf("prepare %v", e)
	}
	drift := i
	drift.BaseLow++
	if parsed.matches(drift) {
		t.Fatal("source drift adopted")
	}
	r := writerRecoveryRecord{Schema: 1, Kind: "restore-original", Binding: i.Binding, CurrentVolume: 1, CurrentHigh: 2, CurrentLow: 3, CurrentSD: i.BaseSD, CurrentSHA: i.BaseSHA, RestoreSHA: i.BaseSHA, IntentBody: string(ib)}
	rb, _ := json.Marshal(r)
	if _, e = parseWriterRecovery(rb, i.Binding, i.BaseSHA); e != nil {
		t.Fatal(e)
	}
	// Actual intent can disappear first. The durable recovery record retains
	// precisely the source-bound schema3 intent; it contains no baseline bytes.
	var records = map[string][]byte{"writer.intent": ib, "writer.prepare": pb, "recovery.intent": rb, "writer.original": []byte("private raw baseline")}
	for _, deleted := range []string{"writer.intent", "writer.original", "writer.prepare"} {
		delete(records, deleted)
		got, e := parseWriterRecovery(records["recovery.intent"], i.Binding, i.BaseSHA)
		if e != nil {
			t.Fatalf("cut after %s: %v", deleted, e)
		}
		if bound, e := parseWriterIntent([]byte(got.IntentBody)); e != nil || bound != i {
			t.Fatal("cleanup cut lost original binding")
		}
		if bytes.Contains(records["recovery.intent"], []byte("private raw baseline")) {
			t.Fatal("baseline leaked into metadata")
		}
	}
}

type recoveryReleaseNative struct {
	fakeConfigNative
	releases int
}

func (n *recoveryReleaseNative) releaseRecoveryOwner() error { n.releases++; return nil }

func TestWriterRecoveryLiveReleaseRequiresExecutionQuiescenceNotCompletion(t *testing.T) {
	for _, fault := range []string{"cancelled", "copy-error", "root-unknown", "tree-unknown", "io-unknown", "close-unknown"} {
		t.Run(fault, func(t *testing.T) {
			bad := errors.New("uncertain")
			tree := &inertClientTree{firstWaitErr: context.Canceled}
			switch fault {
			case "copy-error":
				tree.copyErr = bad
			case "root-unknown":
				tree.errorWait = bad
			case "tree-unknown":
				tree.errorEmpty = bad
			case "io-unknown":
				tree.errorIO = bad
			case "close-unknown":
				tree.errorClose = bad
			}
			a := writerAdmission{state: &writerAdmissionState{value: [32]byte{7}, claimed: true}}
			native := &recoveryReleaseNative{}
			g := &clientConfigGuard{native: native, loan: &a, poison: true}
			unknown := false
			completion, e := retireClientTreeWithQuiescence(context.Background(), tree, a, func() { unknown = true }, a.recordQuiesced)
			if completion != nil || e == nil {
				t.Fatal("forced retirement granted writer completion")
			}
			if e = g.close(); e == nil {
				t.Fatal("pending recovery hidden")
			}
			wantRelease := fault == "cancelled" || fault == "copy-error"
			if (native.releases == 1) != wantRelease || unknown == wantRelease {
				t.Fatalf("release=%d unknown=%v", native.releases, unknown)
			}
			if !wantRelease && g.native == nil {
				t.Fatal("unknown execution released controller")
			}
		})
	}
}
