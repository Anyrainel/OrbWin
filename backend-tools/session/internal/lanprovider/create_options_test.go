package lanprovider

import (
	"orbwin.local/session/internal/lanfirewall"
	"strings"
	"testing"
	"time"
)

func createFixture() (lanfirewall.Receipt, lanfirewall.InterfaceBinding, time.Time) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	request := lanfirewall.Request{Version: 1, Operation: "create", Installation: strings.Repeat("a", 32), UserSID: "S-1-5-21-1-2-3-4", Transaction: strings.Repeat("b", 32), Nonce: strings.Repeat("c", 64), ReviewSHA256: strings.Repeat("d", 64), CallerPID: 1234, CallerImageSHA256: strings.Repeat("e", 64), CallerCreationTime: "123456789", IssuedAt: now.Add(-time.Minute).Format(time.RFC3339Nano), ExpiresAt: now.Add(time.Minute).Format(time.RFC3339Nano), Scope: lanfirewall.Scope{Protocol: "tcp", LocalAddress: "192.168.4.10", LocalPort: 58080, InterfaceGUID: "{11111111-1111-1111-1111-111111111111}", InterfaceLUID: "1234", Profile: "private", ClientNetworks: []string{"192.168.4.0/24"}, WorkerSHA256: strings.Repeat("e", 64)}}
	receipt := lanfirewall.Receipt{Version: 1, Request: request, Sequence: 1, State: "create-intent", CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano), RuleName: request.RuleName(), ScopeSHA256: request.Scope.Digest(), Worker: lanfirewall.WorkerBinding{Path: `C:\Users\fixture\OrbWin\orbwin-session.exe`, FileID: strings.Repeat("f", 32), VolumeSerial: "1234", SHA256: request.Scope.WorkerSHA256, InterfaceAlias: "Ethernet"}}
	binding := lanfirewall.InterfaceBinding{GUID: request.Scope.InterfaceGUID, LUID: "1234", Alias: "Ethernet", Address: request.Scope.LocalAddress, Profile: "private", Index: 4, PrefixLength: 24}
	return receipt, binding, now
}
func TestMICreateEncodingDoesNotConfuseWBEMFlagsOrOptionalFilters(t *testing.T) {
	receipt, binding, now := createFixture()
	wbem, e := lanfirewall.PrepareWMIDisabledCreate(receipt, binding, now)
	if e != nil {
		t.Fatal(e)
	}
	mi, e := PrepareDisabledCreate(receipt, binding, now)
	if e != nil {
		t.Fatal(e)
	}
	if wbem.Flags != 2 || mi.rttiFlags != 0 || !mi.customOptionsMustUnderstand || !mi.options.mustComply || mi.properties.Enabled != 2 || mi.options.policyStore != "PersistentStore" || mi.options.overrideBlockRules || mi.properties != wbem.Properties {
		t.Fatal("MI plan widened scope or reused WBEM flags")
	}
	receipt.Request.Scope.ClientNetworks[0] = "10.0.0.0/8"
	if mi.options.remoteAddress[0] != "192.168.4.0/24" {
		t.Fatal("MI options alias receipt memory")
	}
	receipt.State = "present"
	if _, e := PrepareDisabledCreate(receipt, binding, now); e == nil {
		t.Fatal("encoding skipped retained-intent validation")
	}
}
