package lanfirewall

import (
	"testing"
	"time"

	"orbwin.local/session/internal/lanobserve"
	"orbwin.local/session/internal/lanprofile"
)

func bindingFixture(t *testing.T) (Scope, lanobserve.Snapshot, lanprofile.Snapshot) {
	t.Helper()
	scope := requestFixture().Scope
	interfaces, err := lanobserve.BuildSnapshot([]lanobserve.RawAdapter{{GUID: scope.InterfaceGUID, LUID: 1234, Index: 7, IPv4Index: 7, Alias: "Ethernet", Type: 6, OperationalStatus: 1, Hardware: true, Addresses: []lanobserve.Address{{Address: scope.LocalAddress, PrefixLength: 24, DADState: 4}}}}, firewallNow())
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := lanprofile.BuildSnapshot([]string{scope.InterfaceGUID}, []lanprofile.Record{{AdapterGUID: scope.InterfaceGUID, ConnectionGUID: "{22222222-2222-2222-2222-222222222222}", NetworkGUID: "{33333333-3333-3333-3333-333333333333}", Connected: true, ReadComplete: true, Category: 1}}, true, firewallNow())
	if err != nil {
		t.Fatal(err)
	}
	return scope, interfaces, profiles
}

func TestNativeInterfaceBindingAndRecheck(t *testing.T) {
	scope, interfaces, profiles := bindingFixture(t)
	binding, err := BindInterface(scope, interfaces, profiles, firewallNow())
	if err != nil || binding.Alias != "Ethernet" || binding.Index != 7 || binding.PrefixLength != 24 {
		t.Fatalf("binding: %+v %v", binding, err)
	}
	if err := GuardInterfaceBinding(binding, scope, interfaces, profiles, firewallNow().Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	// Renderer-derived fields do not establish hardware/address/profile facts.
	interfaces.Adapters[0].Kind = "loopback"
	interfaces.Adapters[0].State = "down"
	interfaces.Adapters[0].Addresses[0].Private = false
	if err := GuardInterfaceBinding(binding, scope, interfaces, profiles, firewallNow()); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*lanobserve.Adapter){
		func(a *lanobserve.Adapter) { a.Alias = "Renamed Ethernet" },
		func(a *lanobserve.Adapter) { a.Index = 8; a.IPv4Index = 8 },
		func(a *lanobserve.Adapter) { a.Addresses[0].PrefixLength = 25 },
	} {
		_, next, _ := bindingFixture(t)
		change(&next.Adapters[0])
		if err := GuardInterfaceBinding(binding, scope, next, profiles, firewallNow()); err == nil {
			t.Fatal("binding drift accepted")
		}
	}
}

func TestNativeInterfaceBindingRefusesAmbiguousStaleAndChangedEvidence(t *testing.T) {
	cases := map[string]func(*Scope, *lanobserve.Snapshot, *lanprofile.Snapshot){
		"stale interfaces": func(_ *Scope, i *lanobserve.Snapshot, _ *lanprofile.Snapshot) {
			i.ReadAt = firewallNow().Add(-5*time.Second - time.Nanosecond).Format(time.RFC3339Nano)
		},
		"future profiles": func(_ *Scope, _ *lanobserve.Snapshot, p *lanprofile.Snapshot) {
			p.ReadAt = firewallNow().Add(time.Nanosecond).Format(time.RFC3339Nano)
		},
		"stale profiles": func(_ *Scope, _ *lanobserve.Snapshot, p *lanprofile.Snapshot) {
			p.ReadAt = firewallNow().Add(-6 * time.Second).Format(time.RFC3339Nano)
		},
		"incomplete profiles": func(_ *Scope, _ *lanobserve.Snapshot, p *lanprofile.Snapshot) { p.Complete = false },
		"foreign source":      func(_ *Scope, i *lanobserve.Snapshot, _ *lanprofile.Snapshot) { i.Source = "renderer" },
		"bad LUID":            func(_ *Scope, i *lanobserve.Snapshot, _ *lanprofile.Snapshot) { i.Adapters[0].LUID = "01234" },
		"reused LUID": func(_ *Scope, i *lanobserve.Snapshot, _ *lanprofile.Snapshot) {
			i.Adapters[0].GUID = "{44444444-4444-4444-4444-444444444444}"
		},
		"down":    func(_ *Scope, i *lanobserve.Snapshot, _ *lanprofile.Snapshot) { i.Adapters[0].OperationalStatus = 2 },
		"virtual": func(_ *Scope, i *lanobserve.Snapshot, _ *lanprofile.Snapshot) { i.Adapters[0].Hardware = false },
		"tunnel":  func(_ *Scope, i *lanobserve.Snapshot, _ *lanprofile.Snapshot) { i.Adapters[0].TunnelType = 1 },
		"tentative": func(_ *Scope, i *lanobserve.Snapshot, _ *lanprofile.Snapshot) {
			i.Adapters[0].Addresses[0].DADState = 1
		},
		"missing IP":        func(_ *Scope, i *lanobserve.Snapshot, _ *lanprofile.Snapshot) { i.Adapters[0].Addresses = nil },
		"empty alias":       func(_ *Scope, i *lanobserve.Snapshot, _ *lanprofile.Snapshot) { i.Adapters[0].Alias = "" },
		"public":            func(_ *Scope, _ *lanobserve.Snapshot, p *lanprofile.Snapshot) { p.Profiles[0].Category = "public" },
		"disconnected":      func(_ *Scope, _ *lanobserve.Snapshot, p *lanprofile.Snapshot) { p.Profiles[0].State = "disconnected" },
		"ambiguous profile": func(_ *Scope, _ *lanobserve.Snapshot, p *lanprofile.Snapshot) { p.Profiles[0].ConnectedRecords = 2 },
		"missing profile":   func(_ *Scope, _ *lanobserve.Snapshot, p *lanprofile.Snapshot) { p.Profiles = nil },
		"duplicate profile": func(_ *Scope, _ *lanobserve.Snapshot, p *lanprofile.Snapshot) {
			p.Profiles = append(p.Profiles, p.Profiles[0])
		},
		"duplicate alias": func(_ *Scope, i *lanobserve.Snapshot, _ *lanprofile.Snapshot) {
			a := i.Adapters[0]
			a.GUID = "{44444444-4444-4444-4444-444444444444}"
			a.LUID = "5678"
			a.Index = 9
			a.IPv4Index = 9
			a.Alias = "ETHERNET"
			a.Addresses = nil
			i.Adapters = append(i.Adapters, a)
		},
		"duplicate address": func(_ *Scope, i *lanobserve.Snapshot, _ *lanprofile.Snapshot) {
			a := i.Adapters[0]
			a.GUID = "{44444444-4444-4444-4444-444444444444}"
			a.LUID = "5678"
			a.Index = 9
			a.IPv4Index = 9
			a.Alias = "Other"
			i.Adapters = append(i.Adapters, a)
		},
		"adapter budget": func(_ *Scope, i *lanobserve.Snapshot, _ *lanprofile.Snapshot) {
			i.Adapters = make([]lanobserve.Adapter, lanobserve.MaxAdapters+1)
		},
		"profile budget": func(_ *Scope, _ *lanobserve.Snapshot, p *lanprofile.Snapshot) {
			p.Profiles = make([]lanprofile.Profile, lanprofile.MaxAdapters+1)
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			scope, interfaces, profiles := bindingFixture(t)
			change(&scope, &interfaces, &profiles)
			if binding, err := BindInterface(scope, interfaces, profiles, firewallNow()); err == nil || binding != (InterfaceBinding{}) {
				t.Fatalf("unsafe evidence accepted: %+v %v", binding, err)
			}
		})
	}
}
