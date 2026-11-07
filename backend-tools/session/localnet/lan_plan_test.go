package localnet

import (
	"net/netip"
	"strings"
	"testing"
	"time"
)

func lanReviewFixture() LANExposureReview {
	return LANExposureReview{Version: 1, Owner: strings.Repeat("a", 32), UserSID: "S-1-5-21-1-2-3-1001", ProgramSHA256: strings.Repeat("b", 64), Publication: LANPublication{Daemon: "owned-daemon", ContainerID: strings.Repeat("c", 64), StartedAt: "2026-10-25T12:00:00Z", Generation: 7, HostPort: 58080, ContainerPort: 8080, Protocol: "tcp"}, InterfaceLUID: "1123", LocalAddress: "192.168.4.10", Profile: "private", ClientNetworks: []string{"192.168.4.0/24"}}
}
func lanObservedFixture(review LANExposureReview, now time.Time) LANExposureObservation {
	return LANExposureObservation{Publication: review.Publication, PublicationActive: true, InterfaceLUID: review.InterfaceLUID, LocalAddress: review.LocalAddress, Profile: review.Profile, InterfacePresent: true, ReadAt: now}
}

func TestLANPlanFreezesPerPortScopeAndNeverActivates(t *testing.T) {
	review := lanReviewFixture()
	plan, err := PlanLANExposure(review)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ActivationAvailable || plan.RuleName == "" {
		t.Fatal("plan claims activation or lacks exact ownership")
	}
	review.ClientNetworks[0] = "10.0.0.0/8"
	if plan.Review.ClientNetworks[0] != "192.168.4.0/24" {
		t.Fatal("mutable draft altered frozen plan")
	}
	for _, protocol := range []string{"tcp", "udp"} {
		draft := lanReviewFixture()
		draft.Publication.Protocol = protocol
		got, err := PlanLANExposure(draft)
		if err != nil || got.SHA256 == "" {
			t.Fatalf("protocol %s: %v", protocol, err)
		}
	}
	second := lanReviewFixture()
	second.Publication.HostPort++
	other, _ := PlanLANExposure(second)
	if other.RuleName == plan.RuleName {
		t.Fatal("different port shares firewall ownership")
	}
}

func TestLANPlanRejectsBroadMalformedAndPublicScope(t *testing.T) {
	cases := map[string]func(*LANExposureReview){
		"wildcard":            func(r *LANExposureReview) { r.LocalAddress = "0.0.0.0" },
		"loopback":            func(r *LANExposureReview) { r.LocalAddress = "127.0.0.1" },
		"public":              func(r *LANExposureReview) { r.LocalAddress = "8.8.8.8" },
		"ipv6":                func(r *LANExposureReview) { r.LocalAddress = "fd00::1" },
		"public-profile":      func(r *LANExposureReview) { r.Profile = "public" },
		"all-profiles":        func(r *LANExposureReview) { r.Profile = "any" },
		"all-clients":         func(r *LANExposureReview) { r.ClientNetworks = []string{"0.0.0.0/0"} },
		"private-start-broad": func(r *LANExposureReview) { r.ClientNetworks = []string{"10.0.0.0/7"} },
		"noncanonical-cidr":   func(r *LANExposureReview) { r.ClientNetworks = []string{"192.168.4.1/24"} },
		"duplicate-cidr":      func(r *LANExposureReview) { r.ClientNetworks = append(r.ClientNetworks, r.ClientNetworks[0]) },
		"no-clients":          func(r *LANExposureReview) { r.ClientNetworks = nil },
		"too-many-clients":    func(r *LANExposureReview) { r.ClientNetworks = make([]string, 9) },
		"no-generation":       func(r *LANExposureReview) { r.Publication.Generation = 0 },
		"zero-port":           func(r *LANExposureReview) { r.Publication.HostPort = 0 },
		"wild-protocol":       func(r *LANExposureReview) { r.Publication.Protocol = "any" },
		"invalid-incarnation": func(r *LANExposureReview) { r.Publication.StartedAt = "now" },
		"short-container":     func(r *LANExposureReview) { r.Publication.ContainerID = "abc" },
		"missing-luid":        func(r *LANExposureReview) { r.InterfaceLUID = "0" },
		"noncanonical-luid":   func(r *LANExposureReview) { r.InterfaceLUID = "001" },
		"system-sid":          func(r *LANExposureReview) { r.UserSID = "S-1-5-18" },
		"oversized-sid":       func(r *LANExposureReview) { r.UserSID = "S-1-5-21-1-2-3-9999999999" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			review := lanReviewFixture()
			mutate(&review)
			if _, err := PlanLANExposure(review); err == nil {
				t.Fatal("unsafe exposure plan accepted")
			}
		})
	}
}

func TestLANGuardRefusesRestartNetworkAndObservationDrift(t *testing.T) {
	now := time.Date(2026, 10, 25, 13, 0, 0, 0, time.UTC)
	review := lanReviewFixture()
	plan, _ := PlanLANExposure(review)
	if err := GuardLANExposure(plan, lanObservedFixture(review, now), now); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*LANExposureObservation){
		"other-engine":       func(o *LANExposureObservation) { o.Publication.Daemon = "foreign" },
		"recreated":          func(o *LANExposureObservation) { o.Publication.ContainerID = strings.Repeat("d", 64) },
		"restarted":          func(o *LANExposureObservation) { o.Publication.StartedAt = "2026-10-25T13:00:00Z" },
		"publication-reused": func(o *LANExposureObservation) { o.Publication.Generation++ },
		"stopped":            func(o *LANExposureObservation) { o.PublicationActive = false },
		"interface-lost":     func(o *LANExposureObservation) { o.InterfacePresent = false },
		"interface-reused":   func(o *LANExposureObservation) { o.InterfaceLUID = "1124" },
		"address-changed":    func(o *LANExposureObservation) { o.LocalAddress = "192.168.4.11" },
		"public-network":     func(o *LANExposureObservation) { o.Profile = "public" },
		"expired":            func(o *LANExposureObservation) { o.ReadAt = now.Add(-6 * time.Second) },
		"future":             func(o *LANExposureObservation) { o.ReadAt = now.Add(time.Second) },
		"missing":            func(o *LANExposureObservation) { o.ReadAt = time.Time{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			observed := lanObservedFixture(review, now)
			mutate(&observed)
			if err := GuardLANExposure(plan, observed, now); err == nil {
				t.Fatal("stale LAN consent accepted")
			}
		})
	}
	plan.ActivationAvailable = true
	if err := GuardLANExposure(plan, lanObservedFixture(review, now), now); err == nil {
		t.Fatal("caller forged activation availability")
	}
}

func TestLANPeerFilterAndRemovalNeverAdoptForeignScope(t *testing.T) {
	plan, _ := PlanLANExposure(lanReviewFixture())
	if !plan.AllowsPeer(netip.MustParseAddr("192.168.4.21")) {
		t.Fatal("reviewed client refused")
	}
	for _, address := range []string{"192.168.5.21", "8.8.8.8", "127.0.0.1", "::ffff:192.168.4.21", "fd00::1"} {
		if plan.AllowsPeer(netip.MustParseAddr(address)) {
			t.Fatalf("unreviewed client %s admitted", address)
		}
	}
	review := plan.Review
	rule := LANFirewallRule{Name: plan.RuleName, Owner: review.Owner, ProgramSHA256: review.ProgramSHA256, LocalAddress: review.LocalAddress, LocalPort: review.Publication.HostPort, Protocol: review.Publication.Protocol, Profile: review.Profile, ClientNetworks: append([]string{}, review.ClientNetworks...)}
	if err := ValidateLANRemoval(plan, rule); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*LANFirewallRule){"name": func(r *LANFirewallRule) { r.Name = "foreign" }, "owner": func(r *LANFirewallRule) { r.Owner = strings.Repeat("d", 32) }, "program": func(r *LANFirewallRule) { r.ProgramSHA256 = strings.Repeat("e", 64) }, "port": func(r *LANFirewallRule) { r.LocalPort++ }, "address": func(r *LANFirewallRule) { r.LocalAddress = "0.0.0.0" }, "profile": func(r *LANFirewallRule) { r.Profile = "any" }, "protocol": func(r *LANFirewallRule) { r.Protocol = "udp" }, "clients": func(r *LANFirewallRule) { r.ClientNetworks = []string{"0.0.0.0/0"} }}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			changed := rule
			mutate(&changed)
			if err := ValidateLANRemoval(plan, changed); err == nil {
				t.Fatal("foreign/altered firewall rule accepted for removal")
			}
		})
	}
	plan.Review.ClientNetworks[0] = "10.0.0.0/8"
	if plan.AllowsPeer(netip.MustParseAddr("10.0.0.1")) {
		t.Fatal("edited unacknowledged plan admitted peer")
	}
}
