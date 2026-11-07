package localnet

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

func TestExactStopReplyObservation(t *testing.T) {
	reply := []byte(`{"version":1,"state":{"sequence":7}}`)
	digest := sha256.Sum256(reply)
	want := []byte(`{"version":1,"operation":"stop-observed","replySha256":"` + hex.EncodeToString(digest[:]) + `"}`)
	if got := stopReplyObservation(reply); !bytes.Equal(got, want) || len(got) > 160 {
		t.Fatal("ACK wire")
	}
	if bytes.Equal(stopReplyObservation(append(reply, ' ')), want) {
		t.Fatal("normalized instead of exact reply")
	}
	if stopObservationMode("stop-process", true, "snapshot failed") != stopReplyRetain || stopObservationMode("stop-process", false, "legacy error") != stopReplyLegacy {
		t.Fatal("failed reply cancels or legacy changes")
	}
	if !stopObservationRequired("stop-process", true, "") || stopObservationRequired("stop-process", false, "") || stopObservationRequired("stop-process", true, "refused") || stopObservationRequired("configure", true, "") {
		t.Fatal("legacy/refusal predicate")
	}
}

func TestStopObservationFailureNeverCancels(t *testing.T) {
	reply := []byte("exact successful reply")
	for _, stage := range []string{"write", "deadline", "read", "partial", "wrong", "oversize", "positive"} {
		t.Run(stage, func(t *testing.T) {
			writes, reads, cancels := 0, 0, 0
			err := observeStopReply(reply, func() error {
				writes++
				if stage == "write" {
					return errors.New("controlled write")
				}
				return nil
			}, func() ([]byte, error) {
				reads++
				if writes != 1 {
					t.Fatal("ACK before reply write")
				}
				if stage == "deadline" || stage == "read" {
					return nil, errors.New("controlled read/deadline")
				}
				ack := stopReplyObservation(reply)
				switch stage {
				case "partial":
					return ack[:len(ack)-1], nil
				case "wrong":
					return stopReplyObservation([]byte("other reply")), nil
				case "oversize":
					return append(ack, make([]byte, 161)...), nil
				}
				return ack, nil
			})
			if err == nil {
				cancels++
			} // same actual-caller boundary: nil alone allows cancel
			if writes != 1 || stage == "write" && reads != 0 || stage != "write" && reads != 1 {
				t.Fatal("repeated/reordered transport")
			}
			if stage == "positive" {
				if err != nil || cancels != 1 {
					t.Fatal("positive ACK")
				}
			} else if err == nil || cancels != 0 {
				t.Fatal("failed ACK cancelled service")
			}
		})
	}
}
