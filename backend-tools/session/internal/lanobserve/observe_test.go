package lanobserve

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func rawFixture() RawAdapter {
	return RawAdapter{GUID: "{11111111-1111-1111-1111-111111111111}", LUID: 1234, Index: 7, IPv4Index: 7, IPv6Index: 7, Alias: "Ethernet", Description: "Fixture network", Type: 6, OperationalStatus: 1, Hardware: true, Addresses: []Address{{Address: "192.168.4.10", PrefixLength: 24, DADState: 4}, {Address: "fe80::1234", PrefixLength: 64, ScopeID: 7, DADState: 4}}}
}

func TestBuildSnapshotPreservesNativeIdentityAndUnknownProfile(t *testing.T) {
	row := rawFixture()
	snapshot, err := BuildSnapshot([]RawAdapter{row}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ActivationAvailable || snapshot.ProfileSource != "unavailable" || snapshot.Version != 1 || len(snapshot.Adapters) != 1 {
		t.Fatalf("snapshot falsely qualified: %+v", snapshot)
	}
	adapter := snapshot.Adapters[0]
	if adapter.GUID != row.GUID || adapter.LUID != "1234" || adapter.Index != 7 || adapter.Kind != "hardware-candidate" || adapter.Profile != "unknown" || adapter.ActivationAvailable || adapter.State != "up" || !adapter.Addresses[0].Private || !adapter.Addresses[1].LinkLocal || adapter.Addresses[1].ScopeID != 7 {
		t.Fatalf("native observation changed meaning: %+v", adapter)
	}
	row.Addresses[0].Address = "10.0.0.1"
	if adapter.Addresses[0].Address != "192.168.4.10" {
		t.Fatal("native result retained mutable address input")
	}
}

func TestClassificationNeverUsesFriendlyNamesAsTrust(t *testing.T) {
	cases := []struct {
		kind   string
		change func(*RawAdapter)
	}{
		{"virtual-or-software", func(r *RawAdapter) { r.Hardware = false; r.Alias = "Trusted Private Physical LAN" }},
		{"loopback", func(r *RawAdapter) { r.Type = 24 }},
		{"tunnel", func(r *RawAdapter) { r.Type = 131 }},
		{"tunnel", func(r *RawAdapter) { r.TunnelType = 1 }},
		{"unknown", func(r *RawAdapter) { r.Type = 999 }},
	}
	for _, test := range cases {
		row := rawFixture()
		test.change(&row)
		got, err := BuildSnapshot([]RawAdapter{row}, time.Now())
		if err != nil || got.Adapters[0].Kind != test.kind || got.Adapters[0].Profile != "unknown" || got.Adapters[0].ActivationAvailable {
			t.Fatalf("interface kind/trust inferred incorrectly: %+v %v", got, err)
		}
	}
}

func TestMalformedNativeCopyRefusesWholeSnapshot(t *testing.T) {
	cases := map[string]func(*RawAdapter){
		"guid":              func(r *RawAdapter) { r.GUID = "interface-name" },
		"zero-guid":         func(r *RawAdapter) { r.GUID = "{00000000-0000-0000-0000-000000000000}" },
		"zero-luid":         func(r *RawAdapter) { r.LUID = 0 },
		"zero-index":        func(r *RawAdapter) { r.Index = 0 },
		"index-drift":       func(r *RawAdapter) { r.IPv4Index = 9 },
		"control-name":      func(r *RawAdapter) { r.Alias = "Ethernet\nprivate" },
		"invalid-utf8":      func(r *RawAdapter) { r.Description = string([]byte{255}) },
		"oversized-name":    func(r *RawAdapter) { r.Description = strings.Repeat("x", 1025) },
		"malformed-address": func(r *RawAdapter) { r.Addresses[0].Address = "LAN" },
		"mapped-v4":         func(r *RawAdapter) { r.Addresses[0].Address = "::ffff:192.168.4.10" },
		"multicast":         func(r *RawAdapter) { r.Addresses[0].Address = "224.0.0.1" },
		"unspecified":       func(r *RawAdapter) { r.Addresses[0].Address = "0.0.0.0" },
		"bad-v4-prefix":     func(r *RawAdapter) { r.Addresses[0].PrefixLength = 33 },
		"bad-v6-prefix":     func(r *RawAdapter) { r.Addresses[1].PrefixLength = 129 },
		"bad-dad":           func(r *RawAdapter) { r.Addresses[0].DADState = 99 },
		"v4-zone":           func(r *RawAdapter) { r.Addresses[0].ScopeID = 7 },
		"text-zone":         func(r *RawAdapter) { r.Addresses[1].Address = "fe80::1234%7" },
		"duplicate-address": func(r *RawAdapter) { r.Addresses = append(r.Addresses, r.Addresses[0]) },
		"address-budget":    func(r *RawAdapter) { r.Addresses = make([]Address, 33) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			row := rawFixture()
			row.GUID = "{22222222-2222-2222-2222-222222222222}"
			row.LUID = 222
			row.Index = 8
			row.IPv4Index = 8
			row.IPv6Index = 8
			mutate(&row)
			snapshot, err := BuildSnapshot([]RawAdapter{rawFixture(), row}, time.Now())
			if err == nil || len(snapshot.Adapters) != 0 {
				t.Fatalf("malformed copy produced a partial/success result: %+v %v", snapshot, err)
			}
		})
	}
	for _, field := range []string{"guid", "luid", "index"} {
		a := rawFixture()
		b := rawFixture()
		b.GUID = "{22222222-2222-2222-2222-222222222222}"
		b.LUID++
		b.Index++
		b.IPv4Index = b.Index
		b.IPv6Index = b.Index
		switch field {
		case "guid":
			b.GUID = a.GUID
		case "luid":
			b.LUID = a.LUID
		case "index":
			b.Index = a.Index
			b.IPv4Index = b.Index
			b.IPv6Index = b.Index
		}
		if _, err := BuildSnapshot([]RawAdapter{a, b}, time.Now()); err == nil {
			t.Fatalf("duplicate %s accepted", field)
		}
	}
	if _, err := BuildSnapshot(make([]RawAdapter, MaxAdapters+1), time.Now()); err == nil {
		t.Fatal("adapter bound ignored")
	}
	rows := []RawAdapter{}
	for index := 0; index < 17; index++ {
		row := rawFixture()
		row.GUID = fmt.Sprintf("{%08x-1111-1111-1111-111111111111}", index+1)
		row.LUID = uint64(index + 1)
		row.Index = uint32(index + 1)
		row.IPv4Index = row.Index
		row.IPv6Index = row.Index
		row.Addresses = nil
		for address := 0; address < 32; address++ {
			row.Addresses = append(row.Addresses, Address{Address: fmt.Sprintf("10.%d.0.%d", index, address+1), PrefixLength: 24, DADState: 4})
		}
		rows = append(rows, row)
	}
	if _, err := BuildSnapshot(rows, time.Now()); err == nil {
		t.Fatal("total address bound ignored")
	}
}

func TestAdapterDriftRefusesChangedIncarnation(t *testing.T) {
	first := rawFixture()
	if err := VerifyIdentity(first, first); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*RawAdapter){"guid": func(r *RawAdapter) { r.GUID = "{22222222-2222-2222-2222-222222222222}" }, "luid": func(r *RawAdapter) { r.LUID++ }, "index": func(r *RawAdapter) { r.Index++ }, "type": func(r *RawAdapter) { r.Type = 131 }, "state": func(r *RawAdapter) { r.OperationalStatus = 2 }, "hardware": func(r *RawAdapter) { r.Hardware = false }, "tunnel": func(r *RawAdapter) { r.TunnelType = 1 }}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			changed := first
			mutate(&changed)
			if err := VerifyIdentity(first, changed); err == nil {
				t.Fatal("adapter drift accepted")
			}
		})
	}
}
