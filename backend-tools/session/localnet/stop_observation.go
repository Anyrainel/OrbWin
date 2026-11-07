package localnet

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

const stopObservationBudget = 160

func stopReplyObservation(reply []byte) []byte {
	digest := sha256.Sum256(reply)
	return []byte(`{"version":1,"operation":"stop-observed","replySha256":"` + hex.EncodeToString(digest[:]) + `"}`)
}

type stopReplyDisposition uint8

const (
	stopReplyLegacy stopReplyDisposition = iota
	stopReplyRetain
	stopReplyObserve
)

func stopObservationMode(operation string, hasExpectedServer bool, responseError string) stopReplyDisposition {
	if operation != "stop-process" || !hasExpectedServer {
		return stopReplyLegacy
	}
	if responseError != "" {
		return stopReplyRetain
	}
	return stopReplyObserve
}
func stopObservationRequired(operation string, hasExpectedServer bool, responseError string) bool {
	return stopObservationMode(operation, hasExpectedServer, responseError) == stopReplyObserve
}

// This is managed transport DATA sequencing, not an owner or native permission
// callback. The actual server calls runtime.cancel ONLY after nil return.
func observeStopReply(reply []byte, writeReply func() error, readACK func() ([]byte, error)) error {
	if err := writeReply(); err != nil {
		return err
	}
	ack, err := readACK()
	if err != nil {
		return err
	}
	if len(ack) > stopObservationBudget || !bytes.Equal(ack, stopReplyObservation(reply)) {
		return errors.New("local-services stop reply was not observed; service retained")
	}
	return nil
}
