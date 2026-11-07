package lanhelper

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanlease"
	"time"
)

const maxProtectedReviews = 4096
const protectedJournalBudget = 64 << 10
const protectedJournalEvents = 16

type protectedEntry struct {
	Version     int            `json:"version"`
	Sequence    int            `json:"sequence"`
	Previous    string         `json:"previous"`
	Kind        string         `json:"kind"`
	Review      *OwnerReview   `json:"review,omitempty"`
	Prepared    *Prepared      `json:"prepared,omitempty"`
	StageIntent *stagingIntent `json:"stageIntent,omitempty"`
}
type stagingIntent struct {
	Tombstone lanlease.Tombstone `json:"tombstone"`
}
type protectedJournalState struct {
	sequence, bytes int
	chain           string
	failed          bool
}

func (state *protectedJournalState) append(entry protectedEntry, write func([]byte) (int, error), flush func() error) error {
	if state.failed || state.sequence >= protectedJournalEvents {
		return errors.New("protected journal is poisoned or exhausted")
	}
	entry.Version = 1
	entry.Sequence = state.sequence + 1
	entry.Previous = state.chain
	data, err := json.Marshal(entry)
	if err != nil || len(data) > FrameBudget || state.bytes+len(data)+1 > protectedJournalBudget {
		return errors.New("protected journal byte budget exceeded")
	}
	data = append(data, '\n')
	state.failed = true
	n, err := write(data)
	if err != nil || n != len(data) {
		return errors.New("protected intent write incomplete; retain unknown record")
	}
	if err = flush(); err != nil {
		return errors.New("protected intent flush unknown; never dispatch")
	}
	sum := sha256.Sum256(data)
	state.chain = hex.EncodeToString(sum[:])
	state.bytes += len(data)
	state.sequence++
	state.failed = false
	return nil
}

// ParsedHistory is observation-only. Only a separately checked native protected
// file read can establish the origin; user-home copies confer no authority.
type ParsedHistory struct {
	Review      OwnerReview
	Prepared    *Prepared
	StageIntent *stagingIntent
}

func ParseProtectedHistory(data []byte) (ParsedHistory, error) {
	var result ParsedHistory
	if len(data) == 0 || len(data) > protectedJournalBudget || data[len(data)-1] != '\n' {
		return result, errors.New("protected history incomplete or over budget")
	}
	lines := bytes.Split(data[:len(data)-1], []byte{'\n'})
	if len(lines) > protectedJournalEvents {
		return result, errors.New("protected history event budget exceeded")
	}
	chain := ""
	for index, line := range lines {
		var entry protectedEntry
		if ValidateClosedJSON(line, protectedEntry{}) != nil || json.Unmarshal(line, &entry) != nil || entry.Version != 1 || entry.Sequence != index+1 || entry.Previous != chain {
			return ParsedHistory{}, errors.New("protected history schema/sequence/chain differs")
		}
		switch entry.Kind {
		case "owner-review":
			if index != 0 || entry.Review == nil || entry.Prepared != nil || entry.StageIntent != nil {
				return ParsedHistory{}, errors.New("protected history owner mapping differs")
			}
			result.Review = *entry.Review
			request := result.Review.Stage.Base
			issued, err := time.Parse(time.RFC3339Nano, request.IssuedAt)
			if result.Review.Cleanup != nil {
				issued, err = time.Parse(time.RFC3339Nano, result.Review.Cleanup.IssuedAt)
			}
			identity := lanfirewall.CallerIdentity{PID: request.CallerPID, UserSID: request.UserSID, CreationTime: request.CallerCreationTime, ImageSHA256: request.CallerImageSHA256}
			if err != nil || result.Review.Validate(identity, issued) != nil {
				return ParsedHistory{}, errors.New("protected history owner record is malformed")
			}
		case "stage-intent":
			request := result.Review.Stage.Base
			if index != 1 || entry.StageIntent == nil || entry.Review != nil || entry.Prepared != nil || result.Review.Purpose != "activate" || entry.StageIntent.Tombstone.Transaction != request.Transaction || entry.StageIntent.Tombstone.Nonce != request.Nonce {
				return ParsedHistory{}, errors.New("protected stage reservation differs")
			}
			if lanlease.NewOwner(request.Installation, lanfirewall.CallerIdentity{}).RestoreTombstone(entry.StageIntent.Tombstone) != nil {
				return ParsedHistory{}, errors.New("protected stage reservation path differs")
			}
			result.StageIntent = entry.StageIntent
		case "prepared":
			if index != 2 || entry.Review != nil || entry.StageIntent != nil || entry.Prepared == nil || result.StageIntent == nil || entry.Prepared.Spec.LeaseID != result.StageIntent.Tombstone.LeaseID || entry.Prepared.Spec.Worker.Path != result.StageIntent.Tombstone.WorkerPath || entry.Prepared.Sequence != 2 || entry.Prepared.Spec.Request.Digest() != result.Review.Stage.Base.Digest() || entry.Prepared.Spec.Publication != result.Review.Stage.Publication || !ValidSHA(entry.Prepared.LeaseSHA) {
				return ParsedHistory{}, errors.New("protected history prepared mapping differs")
			}
			result.Prepared = entry.Prepared
			request := result.Review.Stage.Base
			issued, _ := time.Parse(time.RFC3339Nano, request.IssuedAt)
			identity := lanfirewall.CallerIdentity{PID: request.CallerPID, UserSID: request.UserSID, CreationTime: request.CallerCreationTime, ImageSHA256: request.CallerImageSHA256}
			lease, err := lanlease.NewOwner(request.Installation, identity).Prepare(entry.Prepared.Spec, issued)
			if err != nil || lease.Snapshot(issued).LeaseSHA != entry.Prepared.LeaseSHA {
				return ParsedHistory{}, errors.New("protected history lease digest/binding differs")
			}
		default:
			return ParsedHistory{}, errors.New("unsupported protected history event")
		}
		sum := sha256.Sum256(append(append([]byte(nil), line...), '\n'))
		chain = hex.EncodeToString(sum[:])
	}
	return result, nil
}
