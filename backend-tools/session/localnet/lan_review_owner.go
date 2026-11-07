package localnet

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanlease"
)

const maxLANReviews = 16
const maxLANReviewHistory = 4096

type LANReconciliation struct {
	ReviewID        string `json:"reviewId"`
	ReviewSHA256    string `json:"reviewSha256"`
	Outcome         string `json:"outcome"`
	OperationID     string `json:"operationId,omitempty"`
	RuntimeInstance string `json:"runtimeInstance,omitempty"`
	LeaseSHA256     string `json:"leaseSha256,omitempty"`
	NativeSequence  uint64 `json:"nativeSequence,omitempty"`
	ObservedAt      string `json:"observedAt,omitempty"`
	ManagedSequence uint64 `json:"managedSequence,omitempty"`
}

// Actor-owned denial records never authorize another dispatch. Their finite
// history is not evicted: exhausted owners refuse new reviews until restart.
// Missing records, including previous-runtime records, always mean unknown.
type lanReviewRecord struct {
	review           LANReview
	revision         uint64
	stage            lanhelper.StageRequest
	leaseSHA         string
	operationID      string
	revoked          bool
	cleanup          *lanhelper.CleanupReview
	transaction      string
	cleanupKeysSHA   string
	cleanupSpec      *lanlease.Spec
	durable          bool
	receipt          *storedLANReview
	admissionPending bool
	admissionFailed  bool
}
type lanReviewOwner struct {
	records  map[string]*lanReviewRecord
	bindings map[string]string
}

func newLANReviewOwner() *lanReviewOwner {
	return &lanReviewOwner{records: map[string]*lanReviewRecord{}, bindings: map[string]string{}}
}
func lanRandomID() (string, error) {
	var bytes [16]byte
	_, err := rand.Read(bytes[:])
	return hex.EncodeToString(bytes[:]), err
}
func validLANID(id string) bool {
	bytes, err := hex.DecodeString(id)
	return err == nil && len(bytes) == 16 && hex.EncodeToString(bytes) == id
}
func canonicalLANSelection(selection LANSelection, workerSHA string) (LANSelection, error) {
	selection.ClientNetworks = append([]string(nil), selection.ClientNetworks...)
	sort.Strings(selection.ClientNetworks)
	scope := lanfirewall.Scope{Protocol: selection.Protocol, LocalAddress: selection.LocalAddress, LocalPort: selection.HostPort, InterfaceGUID: selection.InterfaceGUID, InterfaceLUID: selection.InterfaceLUID, Profile: selection.Profile, ClientNetworks: selection.ClientNetworks, WorkerSHA256: workerSHA}
	if err := scope.Validate(); err != nil {
		return LANSelection{}, err
	}
	started, err := time.Parse(time.RFC3339Nano, selection.StartedAt)
	if !fullID(selection.ContainerID) || selection.ContainerPort == 0 || err != nil || started.IsZero() {
		return LANSelection{}, errors.New("exact running publication selection required")
	}
	return selection, nil
}
func lanBinding(selection LANSelection) string {
	// Interface identity remains in the immutable review/native scope. The OS
	// socket reservation must additionally conflict across a moved/replaced NIC
	// until the previous exact child positively closes the same IP/port/protocol.
	return fmt.Sprintf("%s/%s/%d", selection.LocalAddress, selection.Protocol, selection.HostPort)
}
func (owner *lanReviewOwner) issue(selection LANSelection, revision uint64, stage lanhelper.StageRequest, purpose, leaseSHA string, now time.Time) (LANReview, error) {
	if len(owner.records) >= maxLANReviewHistory {
		return LANReview{}, errors.New("LAN denial history budget exhausted")
	}
	active := 0
	for _, record := range owner.records {
		expires, _ := time.Parse(time.RFC3339Nano, record.review.ExpiresAt)
		if record.operationID == "" && !record.revoked && now.Before(expires) {
			active++
		}
	}
	if active >= maxLANReviews {
		return LANReview{}, errors.New("LAN current review budget exhausted")
	}
	id, err := lanRandomID()
	if err != nil {
		return LANReview{}, err
	}
	review := LANReview{ReviewID: id, ExpiresAt: now.Add(time.Minute).UTC().Format(time.RFC3339Nano), Selection: selection, PublicationGeneration: stage.Publication.Generation, Purpose: purpose, Available: true}
	stage.Base.ReviewSHA256 = ""
	stage.Base.IssuedAt = now.UTC().Format(time.RFC3339Nano)
	// The renderer may consume for 60s; an accepted operation then owns up to
	// 120s of attended setup. Native request lifetime covers both, while each
	// source/interface observation remains independently fresh for only 5s.
	stage.Base.ExpiresAt = now.Add(3 * time.Minute).UTC().Format(time.RFC3339Nano)
	// Scope, generation, revision, purpose, owner nonce and lifetime are all
	// bound by the displayed review digest. The caller supplies none of these.
	data, _ := json.Marshal(struct {
		Review   LANReview
		Revision uint64
		LeaseSHA string
		Stage    lanhelper.StageRequest
	}{review, revision, leaseSHA, stage})
	digest := sha256.Sum256(append([]byte("orbwin-lan-owner-review-v1\n"), data...))
	review.ReviewSHA256 = hex.EncodeToString(digest[:])
	stage.Base.ReviewSHA256 = review.ReviewSHA256
	owner.records[id] = &lanReviewRecord{review: review, revision: revision, stage: stage, leaseSHA: leaseSHA}
	return review, nil
}
func (owner *lanReviewOwner) consume(id, sha, purpose string, revision uint64, now time.Time) (*lanReviewRecord, error) {
	record := owner.records[id]
	if record == nil || record.review.ReviewSHA256 != sha || record.revoked || record.operationID != "" || record.review.Purpose != purpose || record.revision != revision || revision > lanfirewall.MaxSequence {
		return nil, errors.New("exact current unconsumed LAN review required")
	}
	expires, _ := time.Parse(time.RFC3339Nano, record.review.ExpiresAt)
	if !now.Before(expires) {
		record.revoked = true
		return nil, errors.New("LAN review expired; observe and review again")
	}
	binding := lanBinding(record.review.Selection)
	if purpose == "activate" && owner.bindings[binding] != "" {
		return nil, errors.New("exact LAN listener binding already reserved")
	}
	op, err := lanRandomID()
	if err != nil {
		return nil, err
	}
	record.operationID = op
	if purpose == "activate" {
		owner.bindings[binding] = op
	}
	return record, nil
}
func (owner *lanReviewOwner) reconcile(id, sha string) LANReconciliation {
	result := LANReconciliation{ReviewID: id, ReviewSHA256: sha, Outcome: "unknown"}
	record := owner.records[id]
	if record == nil || record.review.ReviewSHA256 != sha {
		return result
	}
	if record.admissionPending || record.admissionFailed {
		return result
	}
	if record.operationID != "" {
		result.Outcome = "operation"
		result.OperationID = record.operationID
		return result
	}
	record.revoked = true // Fences every later activation of this review atomically.
	result.Outcome = "revoked"
	return result
}
func (owner *lanReviewOwner) releaseBinding(record *lanReviewRecord) {
	binding := lanBinding(record.review.Selection)
	if owner.bindings[binding] == record.operationID {
		delete(owner.bindings, binding)
	}
}
