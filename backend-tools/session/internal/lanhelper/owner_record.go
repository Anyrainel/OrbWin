package lanhelper

import (
	"encoding/hex"
	"errors"
	"time"

	"orbwin.local/session/internal/lanfirewall"
)

// OwnerReview is an immutable correlation record, not native authentication.
// The elevated helper may persist it only after binding the actual pipe peer.
type OwnerReview struct {
	RuntimeInstance string         `json:"runtimeInstance"`
	ReviewID        string         `json:"reviewId"`
	ReviewSHA256    string         `json:"reviewSha256"`
	OperationID     string         `json:"operationId"`
	Transaction     string         `json:"transaction"`
	Purpose         string         `json:"purpose"`
	LeaseSHA256     string         `json:"leaseSha256,omitempty"`
	Stage           StageRequest   `json:"stage"`
	Cleanup         *CleanupReview `json:"cleanup,omitempty"`
}

// CleanupReview binds a fresh attended operation to immutable original native
// ownership. Stage remains the ORIGINAL request/publication, including its old
// caller identity. Only the current native pipe peer supplies current identity.
type CleanupReview struct {
	RuntimeInstance string `json:"runtimeInstance"`
	ReviewID        string `json:"reviewId"`
	ReviewSHA256    string `json:"reviewSha256"`
	OperationID     string `json:"operationId"`
	IssuedAt        string `json:"issuedAt"`
	ExpiresAt       string `json:"expiresAt"`
}

// ProtectedReviewObservation is denial/correlation information returned only
// by the fixed signed SDK read-only child. It confers no listener/mutation
// authority and cannot restore Ready; a helper reopens native protected state.
type ProtectedReviewObservation struct {
	Record   OwnerReview `json:"record"`
	Prepared *Prepared   `json:"prepared,omitempty"`
}

func recordID(id string) bool {
	b, e := hex.DecodeString(id)
	return e == nil && len(b) == 16 && hex.EncodeToString(b) == id
}
func (record OwnerReview) Validate(actual lanfirewall.CallerIdentity, now time.Time) error {
	if record.Cleanup != nil {
		return record.validateCleanup(actual, now)
	}
	if !recordID(record.RuntimeInstance) || !recordID(record.ReviewID) || !recordID(record.OperationID) || record.Transaction != record.Stage.Base.Transaction || record.ReviewSHA256 != record.Stage.Base.ReviewSHA256 {
		return errors.New("exact protected owner review correlation required")
	}
	if record.Purpose == "activate" {
		if record.LeaseSHA256 != "" {
			return errors.New("activation review cannot select an existing lease")
		}
	} else if record.Purpose == "remove" || record.Purpose == "settle-remove" {
		if !ValidSHA(record.LeaseSHA256) {
			return errors.New("removal review requires exact retained lease digest")
		}
	} else {
		return errors.New("fixed owner review purpose required")
	}
	return ValidateStage(record.Stage, actual, now)
}

func (record OwnerReview) validateCleanup(actual lanfirewall.CallerIdentity, now time.Time) error {
	c := record.Cleanup
	if (record.Purpose != "cleanup-remove" && record.Purpose != "cleanup-settle") || !ValidSHA(record.LeaseSHA256) || !ValidSHA(record.ReviewSHA256) || !recordID(record.RuntimeInstance) || !recordID(record.ReviewID) || !recordID(record.OperationID) || !recordID(record.Transaction) || !recordID(c.RuntimeInstance) || !recordID(c.ReviewID) || !ValidSHA(c.ReviewSHA256) || !recordID(c.OperationID) || record.ReviewID == c.ReviewID || record.OperationID == c.OperationID || record.Transaction == record.Stage.Base.Transaction || c.ReviewSHA256 != record.Stage.Base.ReviewSHA256 || actual.UserSID != record.Stage.Base.UserSID || actual.PID == 0 || actual.CreationTime == "" || !ValidSHA(actual.ImageSHA256) {
		return errors.New("fresh native cleanup correlation required")
	}
	issued, err := time.Parse(time.RFC3339Nano, c.IssuedAt)
	expires, endErr := time.Parse(time.RFC3339Nano, c.ExpiresAt)
	if err != nil || endErr != nil || issued.IsZero() || now.Before(issued) || !now.Before(expires) || !expires.After(issued) || expires.Sub(issued) > 3*time.Minute {
		return errors.New("fresh attended cleanup review lifetime required")
	}
	old := record.Stage.Base
	oldIssued, err := time.Parse(time.RFC3339Nano, old.IssuedAt)
	oldIdentity := lanfirewall.CallerIdentity{PID: old.CallerPID, CreationTime: old.CallerCreationTime, UserSID: old.UserSID, ImageSHA256: old.CallerImageSHA256}
	if err != nil || ValidateStage(record.Stage, oldIdentity, oldIssued) != nil {
		return errors.New("original protected cleanup request is malformed")
	}
	return nil
}
