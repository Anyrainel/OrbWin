package lanobserve

import (
	"encoding/json"
	"errors"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const MaxAdapters = 128
const MaxAddresses = 512
const MaxOutput = 256 << 10

type Address struct {
	Address      string `json:"address"`
	PrefixLength uint8  `json:"prefixLength"`
	ScopeID      uint32 `json:"scopeId,omitempty"`
	DADState     uint32 `json:"dadState"`
	Private      bool   `json:"private"`
	Loopback     bool   `json:"loopback"`
	LinkLocal    bool   `json:"linkLocal"`
}
type Adapter struct {
	GUID                string    `json:"guid"`
	LUID                string    `json:"luid"`
	Index               uint32    `json:"index"`
	IPv4Index           uint32    `json:"ipv4Index"`
	IPv6Index           uint32    `json:"ipv6Index"`
	Alias               string    `json:"alias"`
	Description         string    `json:"description"`
	Type                uint32    `json:"type"`
	OperationalStatus   uint32    `json:"operationalStatus"`
	State               string    `json:"state"`
	Hardware            bool      `json:"hardware"`
	TunnelType          uint32    `json:"tunnelType"`
	Kind                string    `json:"kind"`
	Profile             string    `json:"profile"`
	ActivationAvailable bool      `json:"activationAvailable"`
	Reason              string    `json:"reason"`
	Addresses           []Address `json:"addresses"`
}
type Snapshot struct {
	Version             int       `json:"version"`
	ReadAt              string    `json:"readAt"`
	Source              string    `json:"source"`
	ProfileSource       string    `json:"profileSource"`
	ActivationAvailable bool      `json:"activationAvailable"`
	Adapters            []Adapter `json:"adapters"`
}

// RawAdapter contains copied scalar data only. Native linked-list pointers never
// escape their bounded buffer or reach renderer output.
type RawAdapter struct {
	GUID                                string
	LUID                                uint64
	Index, IPv4Index, IPv6Index         uint32
	Alias, Description                  string
	Type, OperationalStatus, TunnelType uint32
	Hardware                            bool
	Addresses                           []Address
}

var canonicalGUID = regexp.MustCompile(`^\{[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\}$`)

func validText(text string) bool {
	return len(text) <= 1024 && utf8.ValidString(text) && !strings.ContainsFunc(text, unicode.IsControl)
}

// VerifyIdentity compares two independent IP Helper reads. Drift refuses the
// whole observation; it does not adopt a reused index/name or guess continuity.
func VerifyIdentity(first, second RawAdapter) error {
	if first.GUID != second.GUID || first.LUID != second.LUID || first.Index != second.Index || first.Type != second.Type || first.OperationalStatus != second.OperationalStatus || first.Hardware != second.Hardware || first.TunnelType != second.TunnelType {
		return errors.New("Windows adapter changed during LAN observation; read again")
	}
	return nil
}

func BuildSnapshot(rows []RawAdapter, now time.Time) (Snapshot, error) {
	if now.IsZero() || len(rows) > MaxAdapters {
		return Snapshot{}, errors.New("LAN interface observation exceeds adapter/time bounds")
	}
	result := Snapshot{Version: 1, ReadAt: now.UTC().Format(time.RFC3339Nano), Source: "windows-ip-helper", ProfileSource: "unavailable", Adapters: []Adapter{}}
	guids, luids, indexes := map[string]bool{}, map[uint64]bool{}, map[uint32]bool{}
	total := 0
	for _, row := range rows {
		if !canonicalGUID.MatchString(row.GUID) || row.GUID == "{00000000-0000-0000-0000-000000000000}" || row.LUID == 0 || row.Index == 0 || guids[row.GUID] || luids[row.LUID] || indexes[row.Index] || row.IPv4Index != 0 && row.IPv4Index != row.Index || row.IPv6Index != 0 && row.IPv6Index != row.Index || !validText(row.Alias) || !validText(row.Description) || len(row.Addresses) > 32 {
			return Snapshot{}, errors.New("LAN interface observation contains malformed, duplicate or inconsistent adapter metadata")
		}
		guids[row.GUID] = true
		luids[row.LUID] = true
		indexes[row.Index] = true
		total += len(row.Addresses)
		if total > MaxAddresses {
			return Snapshot{}, errors.New("LAN interface observation exceeds total address budget")
		}
		adapter := Adapter{GUID: row.GUID, LUID: strconv.FormatUint(row.LUID, 10), Index: row.Index, IPv4Index: row.IPv4Index, IPv6Index: row.IPv6Index, Alias: row.Alias, Description: row.Description, Type: row.Type, OperationalStatus: row.OperationalStatus, Hardware: row.Hardware, TunnelType: row.TunnelType, Profile: "unknown", Kind: "unknown", Reason: "Windows network profile is unavailable; LAN activation remains unavailable.", Addresses: []Address{}}
		switch row.OperationalStatus {
		case 1:
			adapter.State = "up"
		case 2:
			adapter.State = "down"
		case 3:
			adapter.State = "testing"
		case 5:
			adapter.State = "dormant"
		case 6:
			adapter.State = "not-present"
		case 7:
			adapter.State = "lower-layer-down"
		default:
			adapter.State = "unknown"
		}
		switch {
		case row.Type == 24:
			adapter.Kind = "loopback"
			adapter.Reason = "Loopback interfaces are excluded from LAN exposure."
		case row.Type == 131 || row.TunnelType != 0:
			adapter.Kind = "tunnel"
			adapter.Reason = "Tunnel interfaces require a separate explicit support decision."
		case !row.Hardware:
			adapter.Kind = "virtual-or-software"
			adapter.Reason = "IP Helper does not identify this as hardware; physical LAN reachability is not inferred."
		case row.Type == 6 || row.Type == 71:
			adapter.Kind = "hardware-candidate"
		default:
			adapter.Reason = "Interface type is not qualified for LAN exposure."
		}
		seen := map[string]bool{}
		for _, address := range row.Addresses {
			parsed, err := netip.ParseAddr(address.Address)
			if err != nil || parsed.Zone() != "" || parsed.String() != address.Address || parsed.Is4In6() || parsed.IsUnspecified() || parsed.IsMulticast() || address.DADState > 4 || parsed.Is4() && (address.PrefixLength > 32 || address.ScopeID != 0) || parsed.Is6() && address.PrefixLength > 128 {
				return Snapshot{}, errors.New("LAN interface observation contains an invalid unicast address")
			}
			key := address.Address + "/" + strconv.Itoa(int(address.ScopeID))
			if seen[key] {
				return Snapshot{}, errors.New("LAN interface observation contains duplicate unicast addresses")
			}
			seen[key] = true
			address.Private = parsed.IsPrivate()
			address.Loopback = parsed.IsLoopback()
			address.LinkLocal = parsed.IsLinkLocalUnicast()
			adapter.Addresses = append(adapter.Addresses, address)
		}
		sort.Slice(adapter.Addresses, func(i, j int) bool { return adapter.Addresses[i].Address < adapter.Addresses[j].Address })
		result.Adapters = append(result.Adapters, adapter)
	}
	sort.Slice(result.Adapters, func(i, j int) bool { return result.Adapters[i].LUID < result.Adapters[j].LUID })
	data, err := json.Marshal(result)
	if err != nil || len(data) > MaxOutput {
		return Snapshot{}, errors.New("LAN interface observation exceeds output budget")
	}
	return result, nil
}
