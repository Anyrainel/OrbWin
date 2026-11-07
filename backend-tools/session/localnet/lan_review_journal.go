package localnet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanlease"
)

const lanReviewReceiptBudget = 64 << 10

// Native SDK-issued review history is a distinct authority from elevated rule
// ownership. Only issued(seq1) can be consumed or positively revoked. Consumed
// means dispatch MAY have happened, even if the SDK died before its first effect.
type storedLANReview struct {
	Version         int                      `json:"version"`
	Installation    string                   `json:"installation"`
	Daemon          string                   `json:"daemon"`
	RuntimeInstance string                   `json:"runtimeInstance"`
	State           string                   `json:"state"`
	Sequence        uint64                   `json:"sequence"`
	OperationID     string                   `json:"operationId"`
	Review          LANReview                `json:"review"`
	Revision        uint64                   `json:"revision"`
	Stage           lanhelper.StageRequest   `json:"stage"`
	LeaseSHA        string                   `json:"leaseSha256"`
	Cleanup         *lanhelper.CleanupReview `json:"cleanup,omitempty"`
	Transaction     string                   `json:"transaction"`
	CleanupKeysSHA  string                   `json:"cleanupKeysSha256"`
	CleanupSpec     *lanlease.Spec           `json:"cleanupSpec,omitempty"`
}

func snapshotLANReview(record *lanReviewRecord, installation, daemon, runtimeID string) (storedLANReview, error) {
	snapshot := storedLANReview{Version: 1, Installation: installation, Daemon: daemon, RuntimeInstance: runtimeID, State: "issued", Sequence: 1, Review: record.review, Revision: record.revision, Stage: record.stage, LeaseSHA: record.leaseSHA, Cleanup: record.cleanup, Transaction: record.transaction, CleanupKeysSHA: record.cleanupKeysSHA, CleanupSpec: record.cleanupSpec}
	data, err := json.Marshal(snapshot)
	if err != nil || len(data) > lanReviewReceiptBudget {
		return storedLANReview{}, errors.New("LAN review receipt exceeds bound")
	}
	var copy storedLANReview
	if json.Unmarshal(data, &copy) != nil {
		return storedLANReview{}, errors.New("LAN review receipt copy failed")
	}
	return copy, nil
}
func storedReviewDigest(r storedLANReview) string {
	review := r.Review
	review.ReviewSHA256 = ""
	var data []byte
	var domain string
	if r.Cleanup == nil {
		stage := r.Stage
		stage.Base.ReviewSHA256 = ""
		data, _ = json.Marshal(struct {
			Review   LANReview
			Revision uint64
			LeaseSHA string
			Stage    lanhelper.StageRequest
		}{review, r.Revision, r.LeaseSHA, stage})
		domain = "orbwin-lan-owner-review-v1\n"
	} else {
		data, _ = json.Marshal(struct {
			Review                LANReview
			Revision              uint64
			LeaseSHA, Transaction string
			Stage                 lanhelper.StageRequest
			Cleanup               *lanhelper.CleanupReview
		}{review, r.Revision, r.LeaseSHA, r.Transaction, r.Stage, r.Cleanup})
		domain = "orbwin-lan-prior-cleanup-review-v1\n"
	}
	digest := sha256.Sum256(append([]byte(domain), data...))
	return hex.EncodeToString(digest[:])
}
func (r storedLANReview) validate(installation, daemon string) error {
	if r.Version != 1 || r.Installation != installation || r.Daemon != daemon || !validLANID(r.RuntimeInstance) || !validLANID(r.Review.ReviewID) || !fullID(r.Review.ReviewSHA256) || r.Revision > lanfirewall.MaxSequence || !r.Review.Available || r.Review.ReasonCode != "" || r.Review.ReviewSHA256 != storedReviewDigest(r) || r.Stage.Publication.Daemon != daemon {
		return errors.New("private LAN review schema/owner/digest differs")
	}
	switch r.State {
	case "issued":
		if r.Sequence != 1 || r.OperationID != "" {
			return errors.New("issued review has dispatch identity")
		}
	case "revoked":
		if r.Sequence != 2 || r.OperationID != "" {
			return errors.New("revoked review has dispatch identity")
		}
	case "consumed":
		if r.Sequence != 2 || !validLANID(r.OperationID) {
			return errors.New("consumed review lacks exact dispatch identity")
		}
	default:
		return errors.New("unsupported private LAN review state")
	}
	identity, err := deriveLANProductBinding(daemon, installation, r.Stage.Base.UserSID)
	if err != nil || identity.Namespace != r.Stage.Base.Installation {
		return errors.New("private LAN review native product/SID namespace differs")
	}
	scope, pub := r.Stage.Base.Scope, r.Stage.Publication
	selection := LANSelection{ContainerID: pub.ContainerID, StartedAt: pub.StartedAt, HostPort: pub.HostPort, ContainerPort: pub.ContainerPort, Protocol: pub.Protocol, InterfaceGUID: scope.InterfaceGUID, InterfaceLUID: scope.InterfaceLUID, LocalAddress: scope.LocalAddress, Profile: scope.Profile, ClientNetworks: append([]string(nil), scope.ClientNetworks...)}
	if !reflect.DeepEqual(selection, r.Review.Selection) || r.Review.PublicationGeneration != pub.Generation {
		return errors.New("private LAN review display scope differs")
	}
	issuedText := r.Stage.Base.IssuedAt
	if r.Cleanup != nil {
		issuedText = r.Cleanup.IssuedAt
	}
	issued, err := time.Parse(time.RFC3339Nano, issuedText)
	expires, endErr := time.Parse(time.RFC3339Nano, r.Review.ExpiresAt)
	if err != nil || endErr != nil || !expires.After(issued) || expires.Sub(issued) > time.Minute {
		return errors.New("private LAN review lifetime differs")
	}
	op := r.OperationID
	if op == "" {
		op = "00000000000000000000000000000000"
		if r.Cleanup != nil && r.Cleanup.OperationID == op {
			op = "11111111111111111111111111111111"
		}
	}
	transaction := r.Stage.Base.Transaction
	if r.Cleanup != nil {
		transaction = r.Transaction
	}
	protected := LANProtectedReview{RuntimeInstance: r.RuntimeInstance, ReviewID: r.Review.ReviewID, ReviewSHA256: r.Review.ReviewSHA256, OperationID: op, Transaction: transaction, Purpose: r.Review.Purpose, LeaseSHA256: r.LeaseSHA, Stage: r.Stage, Cleanup: r.Cleanup}
	actual := lanfirewall.CallerIdentity{PID: r.Stage.Base.CallerPID, CreationTime: r.Stage.Base.CallerCreationTime, UserSID: r.Stage.Base.UserSID, ImageSHA256: r.Stage.Base.CallerImageSHA256}
	if protected.Validate(actual, issued) != nil {
		return errors.New("private LAN review scope/correlation is malformed")
	}
	if r.Cleanup == nil {
		if r.Transaction != "" || r.CleanupKeysSHA != "" || r.CleanupSpec != nil {
			return errors.New("ordinary review contains cleanup authority")
		}
	} else {
		if r.CleanupSpec == nil || !fullID(r.CleanupKeysSHA) || !reflect.DeepEqual(r.Stage.Base, r.CleanupSpec.Request) || r.Stage.Publication != r.CleanupSpec.Publication {
			return errors.New("prior cleanup original scope differs")
		}
		oldIssued, err := time.Parse(time.RFC3339Nano, r.Stage.Base.IssuedAt)
		lease, prepareErr := lanlease.NewOwner(r.Stage.Base.Installation, actual).Prepare(*r.CleanupSpec, oldIssued)
		if err != nil || prepareErr != nil || lease.Snapshot(oldIssued).LeaseSHA != r.LeaseSHA {
			return errors.New("prior cleanup immutable lease differs")
		}
	}
	return nil
}

type lanReviewJournal interface {
	Issue(context.Context, storedLANReview) error
	Consume(context.Context, storedLANReview, string) error
	Reconcile(context.Context, string, string, string) (LANReconciliation, error)
}
type ledgerLANReviewJournal struct {
	ledger       *NamespaceLedger
	installation string
}

func (j ledgerLANReviewJournal) path(id string) string {
	return filepath.Join(j.ledger.directory, "lan-review-"+id+".json")
}
func (j ledgerLANReviewJournal) read(id, sha string) (storedLANReview, []byte, error) {
	var record storedLANReview
	if !validLANID(id) || !fullID(sha) {
		return record, nil, errors.New("exact private review identity required")
	}
	data, err := j.ledger.readState(j.path(id), lanReviewReceiptBudget)
	if err != nil {
		return record, nil, err
	}
	if lanhelper.ValidateClosedJSON(data, storedLANReview{}) != nil || strictJSON(data, &record) != nil || record.validate(j.installation, j.ledger.daemon) != nil || record.Review.ReviewID != id || record.Review.ReviewSHA256 != sha {
		return storedLANReview{}, nil, errors.New("private LAN review is partial, foreign or malformed")
	}
	return record, data, nil
}
func (j ledgerLANReviewJournal) Issue(ctx context.Context, record storedLANReview) (resultErr error) {
	if j.ledger == nil || j.ledger.stateStore == nil {
		return errors.New("native private SDK review journal required")
	}
	if err := record.validate(j.installation, j.ledger.daemon); err != nil {
		return err
	}
	if record.State != "issued" {
		return errors.New("fresh issued review required")
	}
	if err := j.ledger.lockState(ctx); err != nil {
		return err
	}
	defer j.ledger.finishState(&resultErr)
	data, _ := json.Marshal(record)
	if len(data) > lanReviewReceiptBudget {
		return errors.New("private LAN review exceeds bound")
	}
	return j.ledger.createState(j.path(record.Review.ReviewID), data)
}
func (j ledgerLANReviewJournal) Consume(ctx context.Context, want storedLANReview, operationID string) (resultErr error) {
	if j.ledger == nil || j.ledger.stateStore == nil || !validLANID(operationID) {
		return errors.New("native private consumed admission required")
	}
	if err := j.ledger.lockState(ctx); err != nil {
		return err
	}
	defer j.ledger.finishState(&resultErr)
	current, before, err := j.read(want.Review.ReviewID, want.Review.ReviewSHA256)
	if err != nil {
		return err
	}
	if current.State != "issued" || !reflect.DeepEqual(current, want) {
		return errors.New("exact still-issued private review required; no consume replay")
	}
	current.State, current.Sequence, current.OperationID = "consumed", 2, operationID
	if err = current.validate(j.installation, j.ledger.daemon); err != nil {
		return err
	}
	data, _ := json.Marshal(current)
	name, _ := j.ledger.stateName(j.path(current.Review.ReviewID))
	return j.ledger.stateStore.Replace(name, before, data)
}
func (j ledgerLANReviewJournal) Reconcile(ctx context.Context, id, sha, currentRuntime string) (out LANReconciliation, resultErr error) {
	out = LANReconciliation{ReviewID: id, ReviewSHA256: sha, Outcome: "unknown"}
	if j.ledger == nil || j.ledger.stateStore == nil || !validLANID(currentRuntime) {
		return out, errors.New("native exclusive review recovery owner required")
	}
	if err := j.ledger.lockState(ctx); err != nil {
		return out, err
	}
	defer func() {
		j.ledger.finishState(&resultErr)
		if resultErr != nil {
			out = LANReconciliation{ReviewID: id, ReviewSHA256: sha, Outcome: "unknown"}
		}
	}()
	record, before, err := j.read(id, sha)
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if record.State == "consumed" {
		out.Outcome, out.OperationID = "operation", record.OperationID
		return out, nil
	}
	if record.RuntimeInstance == currentRuntime {
		return out, errors.New("current issued review remains actor owned")
	}
	if record.State == "issued" {
		record.State, record.Sequence = "revoked", 2
		data, _ := json.Marshal(record)
		name, _ := j.ledger.stateName(j.path(id))
		if err = j.ledger.stateStore.Replace(name, before, data); err != nil {
			return out, err
		}
	}
	if record.State != "revoked" || record.Sequence != 2 || record.OperationID != "" {
		return out, errors.New("private review cannot prove nondispatch")
	}
	out.Outcome, out.RuntimeInstance, out.ManagedSequence, out.ObservedAt = "review-revoked", record.RuntimeInstance, 2, time.Now().UTC().Format(time.RFC3339Nano)
	return out, nil
}
