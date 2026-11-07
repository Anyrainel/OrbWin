package lanhelper

import (
	"bytes"
	"errors"
	"testing"
)

func TestLANRuleJournalHasBoundedMutationOnlyHistoryAndNoReplay(t *testing.T) {
	var state nativeRuleWriteState
	var buffer bytes.Buffer
	flushes := 0
	for _, phase := range []string{"create-intent", "create-returned", "created", "remove-intent", "removed"} {
		if err := state.append(phase, []byte(`{"fixed":"native-provider-only"}`), buffer.Write, func() error { flushes++; return nil }); err != nil {
			t.Fatal(phase, err)
		}
	}
	if flushes != 5 || state.events != 5 {
		t.Fatal("mutation history did not flush each boundary")
	}
	if state.append("create-intent", []byte(`{}`), buffer.Write, func() error { return nil }) == nil {
		t.Fatal("terminal mutation history replayed")
	}
	if buffer.Len() >= nativeRuleRecordBudget {
		t.Fatal("unexpected record growth")
	}
}
func TestLANRuleJournalOversizeFailsBeforeWriteAndUnknownPoisons(t *testing.T) {
	for _, kind := range []string{"oversize", "partial", "flush"} {
		t.Run(kind, func(t *testing.T) {
			var state nativeRuleWriteState
			writes := 0
			write := func(data []byte) (int, error) {
				writes++
				if kind == "partial" {
					return len(data) - 1, nil
				}
				return len(data), nil
			}
			flush := func() error {
				if kind == "flush" {
					return errors.New("unknown")
				}
				return nil
			}
			data := []byte(`{}`)
			if kind == "oversize" {
				data = make([]byte, (256<<10)+1)
			}
			if state.append("create-intent", data, write, flush) == nil {
				t.Fatal("unsafe event accepted")
			}
			if kind == "oversize" {
				if writes != 0 {
					t.Fatal("oversize data wrote before refusal")
				}
				return
			}
			if state.append("create-intent", []byte(`{}`), write, flush) == nil || writes != 1 {
				t.Fatal("unknown write replayed")
			}
		})
	}
}
