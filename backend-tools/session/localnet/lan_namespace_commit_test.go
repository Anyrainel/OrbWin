package localnet

import (
	"context"
	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanlease"
	"testing"
)

type namespaceRecordingHelper struct {
	LANAdministrativeHelper
	namespace string
	fallback  bool
}

func (h *namespaceRecordingHelper) Commit(context.Context, lanhelper.Prepared, lanfirewall.CallerIdentity, lanlease.Observation) (lanlease.RuleAcknowledgement, error) {
	h.fallback = true
	return lanlease.RuleAcknowledgement{}, nil
}
func (h *namespaceRecordingHelper) CommitNamespace(ctx context.Context, p lanhelper.Prepared, w lanfirewall.CallerIdentity, o lanlease.Observation, id string) (lanlease.RuleAcknowledgement, error) {
	h.namespace = id
	return h.LANAdministrativeHelper.Commit(ctx, p, w, o)
}
func TestLANNativeCommitReceivesActualOwnedNamespaceNotWorkloadSelection(t *testing.T) {
	f := makeLANActor(t)
	helper := &namespaceRecordingHelper{LANAdministrativeHelper: f.helper}
	f.controller.adapters.Helper = helper
	status, err := f.controller.start(context.Background(), f.stage)
	if err != nil {
		t.Fatal(err)
	}
	session := f.controller.findSession(status.LeaseSHA)
	if helper.fallback || helper.namespace == "" || helper.namespace != session.handle.ID || helper.namespace == f.stage.Publication.ContainerID {
		t.Fatal("native cleanup metadata was not bound to actual namespace owner")
	}
	close(f.worker.joined)
	if err := f.controller.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}
