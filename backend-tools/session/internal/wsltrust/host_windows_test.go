//go:build windows

package wsltrust

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

func fakeHost(t *testing.T, request Request) *Host {
	t.Helper()
	asset := make([]byte, 64)
	copy(asset, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	binary.LittleEndian.PutUint16(asset[18:], 62)
	host, err := NewHost(asset, Digest(asset))
	if err != nil {
		t.Fatal(err)
	}
	host.user = request.WindowsUser
	host.registrations = func() ([]Registration, error) {
		return []Registration{{ID: request.DistributionID, Name: "Ubuntu"}}, nil
	}
	return host
}

func TestSelectedTrustExcludesEngineAndLegacyRegistrations(t *testing.T) {
	id := "00000000-0000-4000-8000-000000000002"
	for _, name := range []string{"lima-orbwin", "OrbWin-Data", "RANCHER-DESKTOP", "lima-rd-2", "docker-desktop-data", "", "--root", "Ubuntu\n"} {
		if ordinaryRegistration(id, name, 2) {
			t.Fatalf("unsafe name %q accepted", name)
		}
	}
	if ordinaryRegistration(id, "Ubuntu", 1) || ordinaryRegistration("Ubuntu", "Ubuntu", 2) || !ordinaryRegistration(id, "Ubuntu 24.04", 2) {
		t.Fatal("registration policy differs")
	}
	for _, text := range []string{"--distribution-id <ID>", "\n  --distribution-id <ID>\r\n"} {
		if !supportsDistributionID([]byte(text)) {
			t.Fatal("UTF8 capability missed")
		}
		units := append([]uint16{0xfeff}, utf16.Encode([]rune(text))...)
		encoded := make([]byte, len(units)*2)
		for i, unit := range units {
			binary.LittleEndian.PutUint16(encoded[i*2:], unit)
		}
		if !supportsDistributionID(encoded) {
			t.Fatal("UTF16 capability missed")
		}
	}
	for _, text := range []string{"--distribution-id-unsupported", "x--distribution-id", "--distribution <Name>"} {
		if supportsDistributionID([]byte(text)) {
			t.Fatal("unsupported capability accepted")
		}
	}
}

func TestSelectedTrustTransportUsesExactGUIDAndPublicFraming(t *testing.T) {
	request := fixtureRequest(t)
	request.Operation = "status"
	host := fakeHost(t, request)
	inputAsset := append([]byte(nil), host.asset...)
	calls := 0
	host.run = func(ctx context.Context, args []string, input io.Reader, budget int) ([]byte, error) {
		calls++
		if calls == 1 {
			if !reflect.DeepEqual(args, []string{"--help"}) || input != nil {
				t.Fatal("unexpected capability operation")
			}
			return []byte("--distribution-id <ID>"), nil
		}
		if !reflect.DeepEqual(args[:6], []string{"--distribution-id", "{" + request.DistributionID + "}", "--user", "root", "--exec", "/usr/bin/env"}) || args[11] != trustBootstrap {
			t.Fatalf("unexpected target/bootstrap: %v", args)
		}
		data, err := io.ReadAll(input)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data[:len(inputAsset)], inputAsset) {
			t.Fatal("helper framing differs")
		}
		var decoded Request
		if err := StrictJSON(data[len(inputAsset):], &decoded); err != nil || !reflect.DeepEqual(decoded, request) {
			t.Fatalf("public request differs: %v", err)
		}
		return json.Marshal(Response{Version: 1, Status: &Status{DistributionID: request.DistributionID, RootSHA256: request.RootSHA256, Observed: true, MachineID: strings.Repeat("a", 32), OSID: "ubuntu", OSVersion: "24.04", State: "absent"}})
	}
	status, err := host.Call(context.Background(), request)
	if err != nil || status == nil || !status.Observed || calls != 2 {
		t.Fatalf("status=%+v calls=%d err=%v", status, calls, err)
	}
}

func TestSelectedTrustRefusesStaleIdentityAndDoesNotReplayUnknownResult(t *testing.T) {
	for _, fault := range []string{"user", "removed", "removed-during-help", "capability", "unknown-result", "foreign-response", "foreign-machine", "unobserved", "response-budget", "unknown-field"} {
		t.Run(fault, func(t *testing.T) {
			request := fixtureRequest(t)
			host := fakeHost(t, request)
			calls, listings := 0, 0
			host.registrations = func() ([]Registration, error) {
				listings++
				if fault == "removed" || (fault == "removed-during-help" && listings > 1) {
					return nil, nil
				}
				return []Registration{{ID: request.DistributionID, Name: "Ubuntu"}}, nil
			}
			if fault == "user" {
				host.user = "S-1-5-21-2"
			}
			host.run = func(ctx context.Context, args []string, input io.Reader, budget int) ([]byte, error) {
				calls++
				if calls == 1 {
					if fault == "capability" {
						return []byte("--distribution <NAME>"), nil
					}
					return []byte("--distribution-id <ID>"), nil
				}
				if fault == "unknown-result" {
					return nil, context.DeadlineExceeded
				}
				if fault == "response-budget" {
					return bytes.Repeat([]byte{'x'}, ResponseBudget+1), nil
				}
				if fault == "unknown-field" {
					return []byte(`{"version":1,"command":"sh"}`), nil
				}
				status := &Status{DistributionID: request.DistributionID, RootSHA256: request.RootSHA256, Observed: true, MachineID: strings.Repeat("a", 32), OSID: "ubuntu", OSVersion: "24.04", State: "installed"}
				if fault == "foreign-response" {
					status.RootSHA256 = strings.Repeat("0", 64)
				}
				if fault == "foreign-machine" {
					status.MachineID = strings.Repeat("b", 32)
				}
				if fault == "unobserved" {
					status.Observed = false
				}
				return json.Marshal(Response{Version: 1, Status: status})
			}
			if _, err := host.Call(context.Background(), request); err == nil {
				t.Fatal("unsafe or unknown operation accepted")
			}
			if calls > 2 || ((fault == "user" || fault == "removed") && calls != 0) || ((fault == "removed-during-help" || fault == "capability") && calls != 1) {
				t.Fatal("unexpected dispatch or automatic retry")
			}
		})
	}
}

func TestWSLOutputBoundDoesNotBlockProducer(t *testing.T) {
	output := &limitedOutput{limit: 3}
	if n, err := output.Write([]byte("abcdef")); n != 6 || err != nil || string(output.bytes) != "abc" || !output.exceeded {
		t.Fatal("output bound failed")
	}
	if n, err := output.Write([]byte("more")); n != 4 || err != nil || string(output.bytes) != "abc" {
		t.Fatal("full output blocked producer")
	}
}

func TestNativeSelectedWSLTrustReadOnlyStatus(t *testing.T) {
	id := os.Getenv("ORBWIN_QUALIFY_WSL_TRUST_STATUS")
	if id == "" {
		t.Skip("explicit selected-registration status qualification only; no trust mutation")
	}
	if !guid.MatchString(id) {
		t.Fatal("exact canonical qualification registration GUID required")
	}
	asset, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "artifacts", "backend", "orbwin-trust"))
	if err != nil {
		t.Fatal(err)
	}
	host, err := NewHost(asset, Digest(asset))
	if err != nil {
		t.Fatal(err)
	}
	request := fixtureRequest(t)
	request.Operation, request.DistributionID, request.WindowsUser = "status", id, host.user
	if present, err := RegistrationPresent(id); err != nil || !present {
		t.Fatal("selected raw registration could not be confirmed", err)
	}
	request.ExpectedMachineID = ""
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	probe := func() []byte {
		// Fixed read-only evidence, including root-owned temporary directories.
		args := []string{"--distribution-id", "{" + id + "}", "--user", "root", "--exec", "/bin/sh", "-c", `/usr/bin/sha256sum /usr/sbin/update-ca-certificates /etc/ssl/certs/ca-certificates.crt && /usr/bin/find /tmp -maxdepth 1 -type d -name 'orbwin-trust.*' -print | /usr/bin/sort`}
		data, err := host.run(ctx, args, nil, 32768)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	before := probe()
	status, err := host.Call(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Observed || status.Managed || status.FilePresent || status.BundlePresent || status.State != "absent" || status.Sequence != 0 {
		t.Fatalf("unexpected ephemeral-root status: %+v", status)
	}
	// Corruption after Windows-side validation must also fail the independent
	// guest-side copy hash before execution. The private copy still cleans up.
	host.asset[len(host.asset)-1] ^= 1
	if _, err := host.Call(ctx, request); err == nil {
		t.Fatal("changed staged helper was executed")
	}
	after := probe()
	if !bytes.Equal(before, after) {
		t.Fatalf("system bundle/updater or temporary-helper cleanup differs: before=%s after=%s", before, after)
	}
	encoded, err := json.Marshal(map[string]any{"registrationId": id, "status": status, "helperSha256": host.digest, "systemBundleUpdaterAndTempDirectories": string(after), "systemTrustMutated": false, "windowsElevationRequested": false})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(encoded))
}
