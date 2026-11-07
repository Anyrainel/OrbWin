package lanhelper

import (
	"errors"
	"sync"
)

// One helper conversation consumes commands before external dispatch. Native
// IO never holds this mutex; pulses remain process-liveness observations while
// the main thread waits on scope consent or a bounded provider call.
type adminConversation struct {
	mu             sync.Mutex
	phase, purpose string
	incoming       uint64
	frames         int
}

func (session *adminConversation) accept(frame AdminFrame) error {
	if err := frame.Validate(); err != nil {
		return err
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.frames >= 1024 {
		return errors.New("admin one-shot frame budget exhausted")
	}
	session.frames++
	if frame.RequestSequence != 0 || frame.Sequence <= session.incoming || session.phase == "closed" {
		return errors.New("admin request replay/role/state differs")
	}
	if frame.Kind == "pulse" {
		if session.phase == "" {
			return errors.New("admin pulse before review")
		}
		session.incoming = frame.Sequence
		return nil
	}
	if frame.Kind == "close" {
		session.incoming = frame.Sequence
		session.phase = "closed"
		return nil
	}
	switch session.phase {
	case "":
		if frame.Kind != "review" || frame.Review == nil {
			return errors.New("admin review must be first")
		}
		session.phase = "reviewing"
		session.purpose = frame.Review.Purpose
	case "accepted":
		if frame.Kind == "read-tombstones" && session.purpose == "activate" {
			break
		}
		if frame.Kind == "stage" && session.purpose == "activate" {
			session.phase = "stage-dispatched"
			break
		}
		if frame.Kind == "remove" && session.purpose == "remove" {
			session.phase = "remove-dispatched"
			break
		}
		if frame.Kind == "settle-remove" && session.purpose == "settle-remove" {
			session.phase = "settle-remove-dispatched"
			break
		}
		if (frame.Kind == "cleanup-remove" || frame.Kind == "cleanup-settle") && frame.Kind == session.purpose {
			session.phase = frame.Kind + "-dispatched"
			break
		}
		return errors.New("admin command differs from consumed exact review")
	case "prepared":
		if frame.Kind != "commit" {
			return errors.New("one fixed commit expected")
		}
		session.phase = "commit-dispatched"
	default:
		return errors.New("admin mutation already dispatched or review not completed")
	}
	session.incoming = frame.Sequence
	return nil
}
func (session *adminConversation) reviewed() error {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.phase != "reviewing" {
		return errors.New("admin review completion differs")
	}
	session.phase = "accepted"
	return nil
}
func (session *adminConversation) prepared() error {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.phase != "stage-dispatched" {
		return errors.New("admin stage completion differs")
	}
	session.phase = "prepared"
	return nil
}
func (session *adminConversation) close() {
	session.mu.Lock()
	session.phase = "closed"
	session.mu.Unlock()
}
