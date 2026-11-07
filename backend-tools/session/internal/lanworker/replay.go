package lanworker

// Stream setup can arrive out of order after concurrent accepts. The fixed
// window tolerates that reordering without accepting a repeated or ancient ID.
// This is replay rejection, not another resource/ownership ledger.
type streamReplay struct{ highest, seen uint64 }

func (w *streamReplay) accept(id uint64) bool {
	if id == 0 {
		return false
	}
	if id > w.highest {
		delta := id - w.highest
		if delta >= 64 {
			w.seen = 0
		} else {
			w.seen <<= delta
		}
		w.highest = id
		w.seen |= 1
		return true
	}
	delta := w.highest - id
	if delta >= 64 || w.seen&(uint64(1)<<delta) != 0 {
		return false
	}
	w.seen |= uint64(1) << delta
	return true
}
