package lanfirewall

import (
	"strings"
	"testing"
	"time"
)

func TestWMIDisabledCreatePreservesExactReviewedScope(t *testing.T) {
	for _, protocol := range []string{"tcp", "udp"} {
		for _, profile := range []string{"private", "domain"} {
			receipt := receiptFixture()
			receipt.Request.Scope.Protocol = protocol
			receipt.Request.Scope.Profile = profile
			receipt.ScopeSHA256 = receipt.Request.Scope.Digest()
			scope, interfaces, profiles := bindingFixture(t)
			scope.Protocol, scope.Profile = protocol, profile
			profiles.Profiles[0].Category = profile
			binding, err := BindInterface(scope, interfaces, profiles, firewallNow())
			if err != nil {
				t.Fatal(err)
			}
			call, err := PrepareWMIDisabledCreate(receipt, binding, firewallNow())
			if err != nil {
				t.Fatal(err)
			}
			wantProfile := uint16(2)
			if profile == "domain" {
				wantProfile = 1
			}
			if call.Flags != 2 || call.Namespace != `root\StandardCimv2` || call.ClassName != "MSFT_NetFirewallRule" || call.Properties.Enabled != 2 || call.Properties.Profiles != wantProfile || call.Properties.InstanceID != receipt.RuleName || call.Properties.Direction != 1 || call.Properties.Action != 2 || call.Properties.EdgeTraversalPolicy != 0 || call.Options.OverrideBlockRules {
				t.Fatalf("unsafe create policy: %+v", call)
			}
			if call.Options.PolicyStore != "PersistentStore" || call.Options.Protocol != strings.ToUpper(protocol) || call.Options.LocalAddress[0] != scope.LocalAddress || call.Options.LocalPort[0] != "58080" || call.Options.Program != receipt.Worker.Path || call.Options.InterfaceAlias[0] != binding.Alias || len(call.Options.RemoteAddress) != 1 || call.Options.RemoteAddress[0] != scope.ClientNetworks[0] {
				t.Fatalf("scope lost in provider option translation: %+v", call)
			}
			call.Options.RemoteAddress[0] = "10.0.0.0/8"
			if receipt.Request.Scope.ClientNetworks[0] != "192.168.4.0/24" {
				t.Fatal("call aliases retained receipt memory")
			}
		}
	}
}

func TestWMIDisabledCreateRefusesUnreviewedAndPatternScopes(t *testing.T) {
	_, interfaces, profiles := bindingFixture(t)
	receipt := receiptFixture()
	binding, err := BindInterface(receipt.Request.Scope, interfaces, profiles, firewallNow())
	if err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"Ether*", "Ether?", "Ether[net]", "Ether`net"} {
		next := receipt
		next.Worker.InterfaceAlias = alias
		changed := binding
		changed.Alias = alias
		if _, err := PrepareWMIDisabledCreate(next, changed, firewallNow()); err == nil {
			t.Fatalf("pattern accepted: %q", alias)
		}
	}
	for _, change := range []func(*Receipt, *InterfaceBinding){
		func(r *Receipt, _ *InterfaceBinding) {
			r.State = "present"
			r.Sequence = 2
			r.NativeScopeSHA256 = r.ScopeSHA256
		},
		func(r *Receipt, _ *InterfaceBinding) { r.Worker.Path = `C:\%TEMP%\orbwin-session.exe` },
		func(_ *Receipt, b *InterfaceBinding) { b.GUID = "{44444444-4444-4444-4444-444444444444}" },
		func(_ *Receipt, b *InterfaceBinding) { b.LUID = "5678" },
		func(_ *Receipt, b *InterfaceBinding) { b.Address = "192.168.4.11" },
		func(_ *Receipt, b *InterfaceBinding) { b.Profile = "domain" },
		func(_ *Receipt, b *InterfaceBinding) { b.Alias = "Renamed" },
		func(_ *Receipt, b *InterfaceBinding) { b.Index = 0 },
	} {
		next, changed := receipt, binding
		change(&next, &changed)
		if _, err := PrepareWMIDisabledCreate(next, changed, firewallNow()); err == nil {
			t.Fatal("changed ownership/binding accepted")
		}
	}
	if _, err := PrepareWMIDisabledCreate(receipt, binding, firewallNow().Add(2*time.Minute)); err == nil {
		t.Fatal("expired review accepted after elevation delay")
	}
}
