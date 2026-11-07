package lanhelper

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"

	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanlease"
)

const TombstonePageSize = 8

var ErrObservationBusy = errors.New("bounded native LAN observation capacity is busy")

// AdminFrame is a closed protocol only. Actual local native peer/image/session,
// signed bundle, scope consent and protected journal establish authority.
type AdminFrame struct {
	Version         int                              `json:"version"`
	Kind            string                           `json:"kind"`
	Sequence        uint64                           `json:"sequence"`
	RequestSequence uint64                           `json:"requestSequence,omitempty"`
	Review          *OwnerReview                     `json:"review,omitempty"`
	Prepared        *Prepared                        `json:"prepared,omitempty"`
	Child           *lanfirewall.CallerIdentity      `json:"child,omitempty"`
	Observation     *lanlease.Observation            `json:"observation,omitempty"`
	NamespaceID     string                           `json:"namespaceId,omitempty"`
	Rule            *lanlease.RuleAcknowledgement    `json:"rule,omitempty"`
	Removal         *lanlease.RemovalAcknowledgement `json:"removal,omitempty"`
	Page            *uint32                          `json:"page,omitempty"`
	Tombstones      *[]lanlease.Tombstone            `json:"tombstones,omitempty"`
	More            *bool                            `json:"more,omitempty"`
	ErrorCode       string                           `json:"errorCode,omitempty"`
}

func AdminPipeName(installation, transaction string) (string, error) {
	if !recordID(installation) || !recordID(transaction) {
		return "", errors.New("fixed admin channel identities required")
	}
	return `\\.\pipe\OrbWinLANAdmin-v2-` + installation + "-" + transaction, nil
}
func (frame AdminFrame) Validate() error {
	if frame.Version != 2 || frame.Sequence == 0 || frame.Sequence > lanfirewall.MaxSequence || frame.RequestSequence > lanfirewall.MaxSequence {
		return errors.New("admin protocol version/sequence differs")
	}
	bodies := 0
	for _, present := range []bool{frame.Review != nil, frame.Prepared != nil, frame.Child != nil, frame.Observation != nil, frame.NamespaceID != "", frame.Rule != nil, frame.Removal != nil, frame.Page != nil, frame.Tombstones != nil, frame.More != nil, frame.ErrorCode != ""} {
		if present {
			bodies++
		}
	}
	response := false
	switch frame.Kind {
	case "review":
		if bodies != 1 || frame.Review == nil || frame.Sequence != 1 {
			return errors.New("admin review shape differs")
		}
	case "review-accepted":
		response = true
		if bodies != 0 {
			return errors.New("admin accepted shape differs")
		}
	case "read-tombstones":
		if bodies != 1 || frame.Page == nil || *frame.Page >= maxProtectedReviews/TombstonePageSize {
			return errors.New("admin tombstone page request differs")
		}
	case "tombstones":
		response = true
		if bodies != 3 || frame.Page == nil || frame.Tombstones == nil || frame.More == nil || len(*frame.Tombstones) > TombstonePageSize || *frame.Page >= maxProtectedReviews/TombstonePageSize {
			return errors.New("admin tombstone page differs")
		}
	case "stage", "remove", "settle-remove", "cleanup-remove", "cleanup-settle", "pulse", "close":
		if bodies != 0 {
			return errors.New("fixed admin command has unexpected body")
		}
	case "prepared":
		response = true
		if bodies != 1 || frame.Prepared == nil || frame.Prepared.Sequence != 2 || !ValidSHA(frame.Prepared.LeaseSHA) {
			return errors.New("admin prepared reply differs")
		}
	case "commit":
		if bodies != 3 || frame.Child == nil || frame.Observation == nil || frame.Child.PID == 0 || !ValidSHA(frame.NamespaceID) {
			return errors.New("admin commit shape differs")
		}
	case "committed":
		response = true
		if bodies != 1 || frame.Rule == nil || !ValidSHA(frame.Rule.LeaseSHA) {
			return errors.New("admin rule reply differs")
		}
	case "removed":
		response = true
		if bodies != 1 || frame.Removal == nil || !ValidSHA(frame.Removal.LeaseSHA) {
			return errors.New("admin removal reply differs")
		}
	case "error":
		response = true
		if bodies != 1 || (frame.ErrorCode != "refused" && frame.ErrorCode != "unavailable" && frame.ErrorCode != "cancelled" && frame.ErrorCode != "unknown") {
			return errors.New("fixed admin error differs")
		}
	default:
		return errors.New("unsupported fixed admin message")
	}
	if response != (frame.RequestSequence != 0) {
		return errors.New("admin request/reply correlation differs")
	}
	return nil
}
func DecodeAdmin(data []byte) (AdminFrame, error) {
	var frame AdminFrame
	if ValidateClosedJSON(data, AdminFrame{}) != nil || json.Unmarshal(data, &frame) != nil {
		return frame, errors.New("admin closed JSON differs")
	}
	if err := frame.Validate(); err != nil {
		return AdminFrame{}, err
	}
	keys := []string{"version", "kind", "sequence"}
	switch frame.Kind {
	case "review":
		keys = append(keys, "review")
	case "review-accepted":
		keys = append(keys, "requestSequence")
	case "read-tombstones":
		keys = append(keys, "page")
	case "tombstones":
		keys = append(keys, "requestSequence", "page", "tombstones", "more")
	case "prepared":
		keys = append(keys, "requestSequence", "prepared")
	case "commit":
		keys = append(keys, "child", "observation", "namespaceId")
	case "committed":
		keys = append(keys, "requestSequence", "rule")
	case "removed":
		keys = append(keys, "requestSequence", "removal")
	case "error":
		keys = append(keys, "requestSequence", "errorCode")
	}
	var members map[string]json.RawMessage
	_ = json.Unmarshal(data, &members)
	if len(members) != len(keys) {
		return AdminFrame{}, errors.New("admin tag has unrelated/missing members")
	}
	for _, key := range keys {
		if _, ok := members[key]; !ok {
			return AdminFrame{}, errors.New("admin complete exact members required")
		}
	}
	return frame, nil
}
func ReadAdmin(reader io.Reader) (AdminFrame, error) {
	var header [4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return AdminFrame{}, err
	}
	size := binary.LittleEndian.Uint32(header[:])
	if size == 0 || size > FrameBudget {
		return AdminFrame{}, errors.New("admin frame budget exceeded")
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(reader, data); err != nil {
		return AdminFrame{}, err
	}
	return DecodeAdmin(data)
}
func WriteAdmin(writer io.Writer, frame AdminFrame) error {
	if err := frame.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(frame)
	if err != nil || len(data) > FrameBudget {
		return errors.New("admin encoded frame budget exceeded")
	}
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(data)))
	for _, part := range [][]byte{header[:], data} {
		for len(part) > 0 {
			n, err := writer.Write(part)
			if err != nil {
				return err
			}
			if n <= 0 || n > len(part) {
				return io.ErrShortWrite
			}
			part = part[n:]
		}
	}
	return nil
}
