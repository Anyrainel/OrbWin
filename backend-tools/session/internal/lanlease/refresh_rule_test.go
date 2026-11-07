package lanlease

import (
	"strings"
	"testing"
	"time"
)

func TestRuleRefreshRequiresExactFreshNativeStateAndOldestTime(t *testing.T) {
	for _, kind := range []string{"valid", "changed-key", "changed-scope", "old", "future", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			l, o, a, now := readyLease(t)
			o.ReadAt = now.Add(2 * time.Second)
			a.ReadAt = now.Add(time.Second)
			switch kind {
			case "changed-key":
				a.NativeKeysSHA = strings.Repeat("3", 64)
			case "changed-scope":
				a.TypedSnapshotSHA = strings.Repeat("3", 64)
			case "old":
				a.ReadAt = now.Add(-5 * time.Second)
			case "future":
				a.ReadAt = now.Add(3 * time.Second)
			case "unknown":
				a.TerminalSuccess = false
			}
			err := l.ObserveRule(l.Snapshot(now).Revision, a, o, now.Add(2*time.Second))
			if kind == "valid" {
				if err != nil || l.lastRead != a.ReadAt || !l.Snapshot(now.Add(5*time.Second)).IngressEligible || l.Snapshot(now.Add(7*time.Second)).IngressEligible {
					t.Fatal("rule timestamp did not bound eligibility", err)
				}
			} else if err == nil || l.Snapshot(now).IngressEligible {
				t.Fatal("changed rule remained eligible")
			}
		})
	}
}

func TestLateAcknowledgedRuleCanOnlyAuthorizeCleanupAfterChildJoin(t *testing.T) {
	_, l, _, o, a, now := leaseFixture(t)
	if e := l.BindChild(1, o.Worker, o, now); e != nil {
		t.Fatal(e)
	}
	if e := l.BeginClose(l.Snapshot(now).Revision); e != nil {
		t.Fatal(e)
	}
	if e := l.AcknowledgeRuleWhileClosing(l.Snapshot(now).Revision, a); e != nil {
		t.Fatal(e)
	}
	if l.Snapshot(now).IngressEligible || l.ReadyForIngress(l.Snapshot(now).Revision, o, now) == nil || l.BeginRetirement(l.Snapshot(now).Revision, a, now) == nil {
		t.Fatal("late positive completion reopened ingress or skipped child join")
	}
	if e := l.AcknowledgeClosed(l.Snapshot(now).Revision, CloseAcknowledgement{LeaseSHA: l.sha, Worker: o.Worker, IngressClosed: true, FlowsJoined: true, ProcessExited: true, NoUnknownChild: true}); e != nil {
		t.Fatal(e)
	}
	if e := l.BeginRetirement(l.Snapshot(now).Revision, a, now); e != nil {
		t.Fatal(e)
	}
}
