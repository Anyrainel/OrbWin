package lanworker

import "testing"

func TestStreamReplayAcceptsBoundedReorderingWithoutReplays(t *testing.T) {
	var w streamReplay
	for _, id := range []uint64{2, 1, 4, 3, 65, 64} {
		if !w.accept(id) {
			t.Fatalf("concurrent stream %d refused", id)
		}
	}
	for _, id := range []uint64{0, 1, 2, 64, 65} {
		if w.accept(id) {
			t.Fatalf("old/repeated stream %d accepted", id)
		}
	}
}
