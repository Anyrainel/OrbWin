//go:build windows

package main

import (
	"context"
	"orbwin.local/session/internal/sdkself"
	"orbwin.local/session/internal/sdkstate"
	"orbwin.local/session/localnet"
	"testing"
	"time"
)

func TestColdJoinedStageFlagsCannotPublishNativeSource(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	e := &coldServiceEntry{retirementPrepared: true, closing: true, prepared: &localnet.ColdServicePreparation{}, closedSelf: &sdkself.JoinedLeaseClose{}}
	if e.publishJoinedSource(ctx) == nil || e.joinedSource != nil {
		t.Fatal("stage flags manufactured native source")
	}
	e.joinedSource = &sdkstate.ColdJoinedSource{}
	if e.checkJoinedSource(ctx) == nil {
		t.Fatal("unbacked participant reached retire")
	}
}
