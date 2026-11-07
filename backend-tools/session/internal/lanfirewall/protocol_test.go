package lanfirewall

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func firewallNow() time.Time { return time.Date(2026, 10, 25, 22, 0, 0, 0, time.UTC) }
func requestFixture() Request {
	now := firewallNow()
	return Request{Version: 1, Operation: "create", Installation: strings.Repeat("a", 32), UserSID: "S-1-5-21-1-2-3-1001", Transaction: strings.Repeat("b", 32), Nonce: strings.Repeat("c", 64), ReviewSHA256: strings.Repeat("d", 64), CallerPID: 1234, CallerImageSHA256: strings.Repeat("e", 64), CallerCreationTime: "123456789", IssuedAt: now.Add(-time.Minute).Format(time.RFC3339Nano), ExpiresAt: now.Add(time.Minute).Format(time.RFC3339Nano), Scope: Scope{Protocol: "tcp", LocalAddress: "192.168.4.10", LocalPort: 58080, InterfaceGUID: "{11111111-1111-1111-1111-111111111111}", InterfaceLUID: "1234", Profile: "private", ClientNetworks: []string{"192.168.4.0/24"}, WorkerSHA256: strings.Repeat("e", 64)}}
}
func receiptFixture() Receipt {
	request := requestFixture()
	now := firewallNow().Format(time.RFC3339Nano)
	return Receipt{Version: 1, Request: request, Sequence: 1, State: "create-intent", CreatedAt: now, UpdatedAt: now, RuleName: request.RuleName(), ScopeSHA256: request.Scope.Digest(), Worker: WorkerBinding{Path: `C:\Users\fixture\AppData\Local\OrbWin\runtime\backend\orbwin-session.exe`, FileID: strings.Repeat("f", 32), VolumeSerial: "1234", SHA256: request.Scope.WorkerSHA256, InterfaceAlias: "Ethernet"}}
}

func TestFixedFirewallRequestAndReceiptRoundTripWithoutActivation(t *testing.T) {
	request := requestFixture()
	data, _ := json.Marshal(request)
	parsed, err := ParseRequest(data, firewallNow())
	if err != nil || parsed.Digest() != request.Digest() {
		t.Fatalf("request roundtrip: %v", err)
	}
	receipt := receiptFixture()
	data, _ = json.Marshal(receipt)
	got, err := ParseReceipt(data)
	if err != nil || got.RuleName != request.RuleName() || got.State != "create-intent" {
		t.Fatalf("intent roundtrip: %v", err)
	}
	if err := ValidateNext(nil, receipt); err != nil {
		t.Fatal(err)
	}
	request.Scope.Protocol = "udp"
	if err := request.Validate(firewallNow()); err != nil {
		t.Fatal("UDP scope refused", err)
	}
	request.UserSID = "S-1-12-1-1-2-3-4"
	if err := request.Validate(firewallNow()); err != nil {
		t.Fatal("ordinary AAD SID refused", err)
	}
	if (Request{}).RuleName() != "" {
		t.Fatal("malformed identity produced a rule name")
	}
}

func TestFirewallParserRejectsAliasesDuplicatesPathsAndUnknownOperations(t *testing.T) {
	data, _ := json.Marshal(requestFixture())
	cases := [][]byte{
		append(append([]byte{}, data...), []byte(` {}`)...),
		bytes.Replace(data, []byte(`"operation":"create"`), []byte(`"operation":"create","operation":"remove"`), 1),
		bytes.Replace(data, []byte(`"operation":"create"`), []byte(`"Operation":"create"`), 1),
		bytes.Replace(data, []byte(`"protocol":"tcp"`), []byte(`"protocol":"tcp","protocol":"udp"`), 1),
		bytes.Replace(data, []byte(`"localPort":58080`), []byte(`"localPort":58080,"command":"netsh"`), 1),
		bytes.Replace(data, []byte(`"operation":"create"`), []byte(`"operation":"reset-all"`), 1),
		bytes.Replace(data, []byte(`"version":1,`), nil, 1),
		bytes.Replace(data, []byte(`"expectedSequence":0`), []byte(`"expectedSequence":0.0`), 1),
		bytes.Replace(data, []byte(`"scope":{`), []byte(`"path":"C:\\untrusted.exe","scope":{`), 1),
		bytes.Repeat([]byte{' '}, RequestBudget+1),
		[]byte{0xff},
	}
	for index, bad := range cases {
		if _, err := ParseRequest(bad, firewallNow()); err == nil {
			t.Fatalf("hostile request %d parsed", index)
		}
	}
	receipt, _ := json.Marshal(receiptFixture())
	bad := bytes.Replace(receipt, []byte(`"state":"create-intent"`), []byte(`"state":"present","state":"create-intent"`), 1)
	if _, err := ParseReceipt(bad); err == nil {
		t.Fatal("duplicate receipt state accepted")
	}
}

func TestFirewallRequestRefusesBroadStaleOrMalformedScope(t *testing.T) {
	cases := map[string]func(*Request){
		"wildcard": func(r *Request) { r.Scope.LocalAddress = "0.0.0.0" }, "public-address": func(r *Request) { r.Scope.LocalAddress = "8.8.8.8" }, "ipv6": func(r *Request) { r.Scope.LocalAddress = "fd00::1" },
		"all-ports": func(r *Request) { r.Scope.LocalPort = 0 }, "any-protocol": func(r *Request) { r.Scope.Protocol = "any" }, "public-profile": func(r *Request) { r.Scope.Profile = "public" }, "all-profiles": func(r *Request) { r.Scope.Profile = "any" },
		"broad-clients": func(r *Request) { r.Scope.ClientNetworks = []string{"0.0.0.0/0"} }, "private-start-broad": func(r *Request) { r.Scope.ClientNetworks = []string{"10.0.0.0/7"} }, "no-clients": func(r *Request) { r.Scope.ClientNetworks = nil }, "duplicate-clients": func(r *Request) { r.Scope.ClientNetworks = append(r.Scope.ClientNetworks, r.Scope.ClientNetworks[0]) },
		"alias-as-guid": func(r *Request) { r.Scope.InterfaceGUID = "Ethernet" }, "zero-guid": func(r *Request) { r.Scope.InterfaceGUID = "{00000000-0000-0000-0000-000000000000}" }, "luid-format": func(r *Request) { r.Scope.InterfaceLUID = "001" },
		"system-sid": func(r *Request) { r.UserSID = "S-1-5-18" }, "bad-sid": func(r *Request) { r.UserSID = "S-1-5-21-1-2-3-9999999999" }, "pid": func(r *Request) { r.CallerPID = 0 }, "creation": func(r *Request) { r.CallerCreationTime = "0" },
		"future": func(r *Request) { r.IssuedAt = firewallNow().Add(time.Second).Format(time.RFC3339Nano) }, "expired": func(r *Request) { r.ExpiresAt = firewallNow().Format(time.RFC3339Nano) }, "lifetime": func(r *Request) { r.ExpiresAt = firewallNow().Add(time.Hour).Format(time.RFC3339Nano) },
		"sequence-exhausted": func(r *Request) { r.ExpectedSequence = MaxSequence - 1 }, "nonce": func(r *Request) { r.Nonce = "nonce" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			request := requestFixture()
			mutate(&request)
			if err := request.Validate(firewallNow()); err == nil {
				t.Fatal("unsafe request validated")
			}
		})
	}
}

func TestFirewallCallerAndProtectedOwnerGuardsRefuseForgery(t *testing.T) {
	request := requestFixture()
	caller := CallerIdentity{UserSID: request.UserSID, PID: request.CallerPID, CreationTime: request.CallerCreationTime, ImageSHA256: request.Scope.WorkerSHA256}
	if err := GuardCaller(request, caller, firewallNow()); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"sid", "pid", "creation", "image"} {
		changed := caller
		switch field {
		case "sid":
			changed.UserSID = "S-1-5-21-1-2-3-1002"
		case "pid":
			changed.PID++
		case "creation":
			changed.CreationTime = "123456790"
		case "image":
			changed.ImageSHA256 = strings.Repeat("a", 64)
		}
		if err := GuardCaller(request, changed, firewallNow()); err == nil {
			t.Fatalf("forged %s accepted", field)
		}
	}
	receipt := receiptFixture()
	request.Operation = "inspect"
	request.ExpectedSequence = receipt.Sequence
	if err := GuardStoredOwner(request, &receipt); err != nil {
		t.Fatal(err)
	}
	staleInspect := request
	staleInspect.ExpectedSequence = 0
	if err := GuardStoredOwner(staleInspect, &receipt); err != nil {
		t.Fatal("read-only recovery could not observe a newer protected sequence", err)
	}
	for _, field := range []string{"sid", "scope", "review", "sequence"} {
		changed := request
		switch field {
		case "sid":
			changed.UserSID = "S-1-5-21-1-2-3-1002"
		case "scope":
			changed.Scope.LocalPort++
		case "review":
			changed.ReviewSHA256 = strings.Repeat("b", 64)
		case "sequence":
			changed.Operation = "remove"
			changed.ExpectedSequence++
		}
		if err := GuardStoredOwner(changed, &receipt); err == nil {
			t.Fatalf("foreign protected %s accepted", field)
		}
	}
}

func TestFirewallReceiptStatesRetainUncertaintyAndPreventReplay(t *testing.T) {
	intent := receiptFixture()
	present := intent
	present.Sequence++
	present.State = "present"
	present.NativeScopeSHA256 = present.ScopeSHA256
	if err := ValidateNext(&intent, present); err != nil {
		t.Fatal(err)
	}
	if err := ValidateNext(nil, present); err == nil {
		t.Fatal("acknowledgement without prior intent accepted")
	}
	remove := present
	remove.Sequence++
	remove.State = "remove-intent"
	remove.Request.Operation = "remove"
	remove.Request.ExpectedSequence = present.Sequence
	remove.Request.Transaction = strings.Repeat("1", 32)
	remove.Request.Nonce = strings.Repeat("2", 64)
	if err := ValidateNext(&present, remove); err != nil {
		t.Fatal(err)
	}
	// A current SDK may remove its protected old worker rule after an upgrade;
	// current caller identity is distinct from the retained worker binding.
	remove.Request.CallerImageSHA256 = strings.Repeat("9", 64)
	currentCaller := CallerIdentity{UserSID: remove.Request.UserSID, PID: remove.Request.CallerPID, CreationTime: remove.Request.CallerCreationTime, ImageSHA256: remove.Request.CallerImageSHA256}
	if err := GuardCaller(remove.Request, currentCaller, firewallNow()); err != nil {
		t.Fatal("updated caller cannot review old rule removal", err)
	}
	if err := ValidateNext(&present, remove); err != nil {
		t.Fatal("updated caller changed owned worker scope", err)
	}
	removed := remove
	removed.Sequence++
	removed.State = "removed"
	removed.NativeScopeSHA256 = ""
	if err := ValidateNext(&remove, removed); err != nil {
		t.Fatal(err)
	}
	if err := ValidateNext(&present, removed); err == nil {
		t.Fatal("native absence acknowledged without removal intent")
	}
	if err := ValidateNext(&intent, intent); err == nil {
		t.Fatal("unknown create was replayed as a new mutation")
	}
	retry := intent
	retry.Sequence++
	retry.Request.ExpectedSequence = intent.Sequence
	retry.Request.Transaction = strings.Repeat("5", 32)
	retry.Request.Nonce = strings.Repeat("6", 64)
	if err := ValidateNext(&intent, retry); err == nil {
		t.Fatal("new nonce replayed an uncertain create")
	}
	retry = remove
	retry.Sequence++
	retry.Request.ExpectedSequence = remove.Sequence
	retry.Request.Transaction = strings.Repeat("5", 32)
	retry.Request.Nonce = strings.Repeat("6", 64)
	if err := ValidateNext(&remove, retry); err == nil {
		t.Fatal("new nonce replayed an uncertain removal")
	}
	changed := present
	changed.Request.Nonce = strings.Repeat("3", 64)
	if err := ValidateNext(&intent, changed); err == nil {
		t.Fatal("acknowledgement changed accepted request")
	}
	changed = remove
	changed.Request.Nonce = present.Request.Nonce
	if err := ValidateNext(&present, changed); err == nil {
		t.Fatal("mutation nonce replay accepted")
	}
	changed = remove
	changed.ScopeSHA256 = strings.Repeat("4", 64)
	if err := ValidateNext(&present, changed); err == nil {
		t.Fatal("changed scope accepted")
	}
	changed = remove
	changed.Worker.Path = `C:\Users\fixture\other\orbwin-session.exe`
	if err := ValidateNext(&present, changed); err == nil {
		t.Fatal("changed native program binding accepted")
	}
	for _, path := range []string{`\\server\share\orbwin-session.exe`, `C:\Users\..\orbwin-session.exe`, `C:\Users\NUL\orbwin-session.exe`, `C:\Users\fixture\orbwin-session.exe:evil`, `C:\Users\fixture\arbitrary.exe`} {
		changed := intent
		changed.Worker.Path = path
		if err := changed.Validate(); err == nil {
			t.Fatalf("unsafe receipt path %q accepted", path)
		}
	}
}

func FuzzFirewallProtocol(f *testing.F) {
	request, _ := json.Marshal(requestFixture())
	receipt, _ := json.Marshal(receiptFixture())
	f.Add(request)
	f.Add(receipt)
	f.Add([]byte(`{"operation":"create","operation":"remove"}`))
	f.Add([]byte(`null`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > ReceiptBudget {
			return
		}
		if request, err := ParseRequest(data, firewallNow()); err == nil {
			encoded, _ := json.Marshal(request)
			again, err := ParseRequest(encoded, firewallNow())
			if err != nil || again.Digest() != request.Digest() || request.RuleName() == "" {
				t.Fatal("accepted request lost identity on canonical encoding")
			}
		}
		if receipt, err := ParseReceipt(data); err == nil {
			encoded, _ := json.Marshal(receipt)
			again, err := ParseReceipt(encoded)
			if err != nil || again.RuleName != receipt.RuleName || again.Sequence != receipt.Sequence {
				t.Fatal("accepted receipt lost ownership on canonical encoding")
			}
		}
	})
}
