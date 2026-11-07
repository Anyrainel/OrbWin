package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanlease"
	"orbwin.local/session/internal/lanprovider"
	"strings"
	"testing"
)

func TestClosedLANBootstrapRejectsArityAndMalformedIDsWithoutDispatch(t *testing.T) {
	calls := 0
	launch := func(context.Context, []string) error { calls++; return nil }
	observe := func(context.Context, string, string) (lanlease.RuleAcknowledgement, error) {
		calls++
		return lanlease.RuleAcknowledgement{}, nil
	}
	for _, args := range [][]string{{"lan-elevation-launcher"}, {"lan-elevation-launcher", strings.Repeat("a", 32), strings.Repeat("b", 32), "extra"}, {"lan-elevation-launcher", strings.Repeat("A", 32), strings.Repeat("b", 32)}, {"lan-rule-observation", strings.Repeat("a", 32), "short"}} {
		handled, err := runLANBootstrapWith(args, &bytes.Buffer{}, launch, observe, nil, nil)
		if !handled || err == nil || calls != 0 {
			t.Fatal("closed command fell through or dispatched")
		}
	}
}

func TestClosedLANCleanupObserverKeepsOriginalNativeProofReadOnlyAndBounded(t *testing.T) {
	calls := 0
	read := func(ctx context.Context, installation, lease string) (lanprovider.CleanupObservation, error) {
		calls++
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("missing native read deadline")
		}
		return lanprovider.CleanupObservation{RuntimeInstance: strings.Repeat("1", 32), ReviewID: strings.Repeat("2", 32), ReviewSHA256: strings.Repeat("3", 64), OperationID: strings.Repeat("4", 32), LeaseSHA: lease, Phase: "removed", NativeSequence: 5, ChildExited: true, NamespaceAbsent: true, RuleAbsent: true}, nil
	}
	for _, args := range [][]string{{"lan-cleanup-observation"}, {"lan-cleanup-observation", strings.Repeat("A", 32), strings.Repeat("b", 64)}, {"lan-cleanup-observation", strings.Repeat("a", 32), strings.Repeat("b", 64), "extra"}} {
		var output bytes.Buffer
		if handled, err := runLANCleanupBootstrapWith(args, &output, read); !handled || err == nil || calls != 0 || output.Len() != 0 {
			t.Fatal("malformed cleanup read dispatched")
		}
	}
	var output bytes.Buffer
	args := []string{"lan-cleanup-observation", strings.Repeat("a", 32), strings.Repeat("b", 64)}
	if handled, err := runLANCleanupBootstrapWith(args, &output, read); !handled || err != nil || calls != 1 {
		t.Fatal("closed native cleanup read failed", err)
	}
	var proof lanprovider.CleanupObservation
	if json.Unmarshal(output.Bytes(), &proof) != nil || proof.RuntimeInstance != strings.Repeat("1", 32) || proof.NativeSequence != 5 {
		t.Fatal("original native proof changed")
	}
	output.Reset()
	fail := func(context.Context, string, string) (lanprovider.CleanupObservation, error) {
		return proof, errors.New("private native detail")
	}
	if handled, err := runLANCleanupBootstrapWith(args, &output, fail); !handled || err == nil || strings.Contains(err.Error(), "private") || output.Len() != 0 {
		t.Fatal("failed native read emitted authority/details")
	}
}

func TestClosedLANRemovalObserverNeverLaunchesOrReplaysMutation(t *testing.T) {
	installation, leaseSHA := strings.Repeat("a", 32), strings.Repeat("b", 64)
	calls := 0
	read := func(ctx context.Context, i, sha string) (lanlease.RemovalAcknowledgement, error) {
		calls++
		if _, ok := ctx.Deadline(); !ok || i != installation || sha != leaseSHA {
			t.Fatal("unbounded or changed removal observation")
		}
		return lanlease.RemovalAcknowledgement{LeaseSHA: sha, Sequence: 5, TerminalSuccess: true, OperationClosed: true, ObservedAbsent: true, ProtectedReceipt: true, AcknowledgementFlushed: true}, nil
	}
	for _, args := range [][]string{{"lan-removal-observation"}, {"lan-removal-observation", installation, strings.Repeat("B", 64)}, {"lan-removal-observation", installation, leaseSHA, "extra"}} {
		var output bytes.Buffer
		if handled, err := runLANBootstrapWith(args, &output, nil, nil, nil, read); !handled || err == nil || calls != 0 || output.Len() != 0 {
			t.Fatal("malformed closed removal read dispatched or fell through")
		}
	}
	var output bytes.Buffer
	if handled, err := runLANBootstrapWith([]string{"lan-removal-observation", installation, leaseSHA}, &output, nil, nil, nil, read); !handled || err != nil || calls != 1 {
		t.Fatal("positive removal observation failed", err)
	}
	var ack lanlease.RemovalAcknowledgement
	if json.Unmarshal(output.Bytes(), &ack) != nil || ack.LeaseSHA != leaseSHA || ack.Sequence != 5 || !ack.ObservedAbsent {
		t.Fatal("positive removal observation changed")
	}
	output.Reset()
	fail := func(context.Context, string, string) (lanlease.RemovalAcknowledgement, error) {
		return ack, errors.New("private failure")
	}
	if handled, err := runLANBootstrapWith([]string{"lan-removal-observation", installation, leaseSHA}, &output, nil, nil, nil, fail); !handled || err == nil || strings.Contains(err.Error(), "private") || output.Len() != 0 {
		t.Fatal("failed removal observation emitted positive result/details")
	}
}
func TestClosedLANBootstrapBoundsObservationAndNeverFallsThroughFailure(t *testing.T) {
	var out bytes.Buffer
	observed := false
	handled, err := runLANBootstrapWith([]string{"lan-rule-observation", strings.Repeat("a", 32), strings.Repeat("b", 64)}, &out, func(context.Context, []string) error { t.Fatal("wrong branch"); return nil }, func(ctx context.Context, a, b string) (lanlease.RuleAcknowledgement, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("missing observation deadline")
		}
		observed = true
		return lanlease.RuleAcknowledgement{}, errors.New("private detail")
	}, nil, nil)
	if !handled || err == nil || !observed || out.Len() != 0 || strings.Contains(err.Error(), "private detail") {
		t.Fatal("failed observer emitted success/private error")
	}
}
func TestClosedLANReviewObservationRejectsMalformedAndEmitsOnlyBoundedSuccess(t *testing.T) {
	calls := 0
	read := func(ctx context.Context, installation, review, sha string) (lanhelper.ProtectedReviewObservation, error) {
		calls++
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("missing read deadline")
		}
		return lanhelper.ProtectedReviewObservation{}, errors.New("private failure")
	}
	for _, args := range [][]string{{"lan-review-observation"}, {"lan-review-observation", strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("C", 64)}, {"lan-review-observation", strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 64), "extra"}} {
		var output bytes.Buffer
		handled, err := runLANBootstrapWith(args, &output, nil, nil, read, nil)
		if !handled || err == nil || calls != 0 || output.Len() != 0 {
			t.Fatal("malformed review dispatched or fell through")
		}
	}
	var output bytes.Buffer
	handled, err := runLANBootstrapWith([]string{"lan-review-observation", strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 64)}, &output, nil, nil, read, nil)
	if !handled || err == nil || calls != 1 || output.Len() != 0 || strings.Contains(err.Error(), "private") {
		t.Fatal("failed protected review leaked output/details")
	}
}
