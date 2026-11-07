package lanfirewall

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"orbwin.local/session/internal/lanobserve"
	"orbwin.local/session/internal/lanprofile"
)

// InterfaceBinding is copied native evidence for the friendly-name filter used
// by Windows Firewall. It is not authorization, a reservation or an activation
// capability. Callers must obtain snapshots locally, never from request JSON.
type InterfaceBinding struct {
	GUID, LUID, Alias, Address, Profile string
	Index                               uint32
	PrefixLength                        uint8
}

func freshObservation(raw string, now time.Time) bool {
	read, err := moment(raw)
	return err == nil && !now.IsZero() && !now.Before(read) && now.Sub(read) <= 5*time.Second
}

// BindInterface resolves an exact reviewed scope to one unique friendly name.
// Both native scans must be complete and fresh; no friendly-name or category
// claim supplied by the renderer is accepted. No observation can close the race
// after this function returns: the future provider must re-observe after its
// operation and close ingress on drift, including on restart.
func BindInterface(scope Scope, interfaces lanobserve.Snapshot, profiles lanprofile.Snapshot, now time.Time) (InterfaceBinding, error) {
	var empty InterfaceBinding
	if err := scope.Validate(); err != nil {
		return empty, err
	}
	if interfaces.Version != 1 || interfaces.Source != "windows-ip-helper" || profiles.Version != 1 || profiles.Source != "windows-network-list-manager" || !profiles.Complete || !freshObservation(interfaces.ReadAt, now) || !freshObservation(profiles.ReadAt, now) {
		return empty, errors.New("fresh complete native interface and profile observations required")
	}
	// Reuse the observer's identity/address bounds and normalization checks. Do
	// not trust derived Kind/State/Private fields on a copied snapshot.
	if len(interfaces.Adapters) > lanobserve.MaxAdapters {
		return empty, errors.New("interface observation exceeds adapter budget")
	}
	rows := make([]lanobserve.RawAdapter, 0, len(interfaces.Adapters))
	for _, adapter := range interfaces.Adapters {
		luid, err := strconv.ParseUint(adapter.LUID, 10, 64)
		if err != nil || luid == 0 || strconv.FormatUint(luid, 10) != adapter.LUID {
			return empty, errors.New("native interface LUID is malformed")
		}
		rows = append(rows, lanobserve.RawAdapter{GUID: adapter.GUID, LUID: luid, Index: adapter.Index, IPv4Index: adapter.IPv4Index, IPv6Index: adapter.IPv6Index, Alias: adapter.Alias, Description: adapter.Description, Type: adapter.Type, OperationalStatus: adapter.OperationalStatus, Hardware: adapter.Hardware, TunnelType: adapter.TunnelType, Addresses: adapter.Addresses})
	}
	checked, err := lanobserve.BuildSnapshot(rows, now)
	if err != nil {
		return empty, err
	}
	var binding InterfaceBinding
	addressMatches := 0
	for _, adapter := range checked.Adapters {
		for _, address := range adapter.Addresses {
			if address.Address == scope.LocalAddress {
				addressMatches++
				if adapter.GUID == scope.InterfaceGUID && adapter.LUID == scope.InterfaceLUID && adapter.Kind == "hardware-candidate" && adapter.OperationalStatus == 1 && address.DADState == 4 && boundedText(adapter.Alias, 1024) {
					binding = InterfaceBinding{GUID: adapter.GUID, LUID: adapter.LUID, Alias: adapter.Alias, Index: adapter.Index, Address: address.Address, PrefixLength: address.PrefixLength, Profile: scope.Profile}
				}
			}
		}
	}
	if binding.GUID == "" || addressMatches != 1 {
		return empty, errors.New("reviewed LAN address is missing, ambiguous or not preferred on an operational hardware interface")
	}
	for _, adapter := range checked.Adapters {
		if adapter.GUID != binding.GUID && strings.EqualFold(adapter.Alias, binding.Alias) {
			return empty, errors.New("native firewall interface friendly name is ambiguous")
		}
	}
	if len(profiles.Profiles) > lanprofile.MaxAdapters {
		return empty, errors.New("profile observation exceeds adapter budget")
	}
	guids := make([]string, 0, len(profiles.Profiles))
	matched := false
	for _, profile := range profiles.Profiles {
		guids = append(guids, profile.AdapterGUID)
		if profile.AdapterGUID == binding.GUID {
			matched = profile.State == "observed" && profile.ConnectedRecords == 1 && profile.Category == scope.Profile
		}
	}
	if err := lanprofile.ValidateAdapters(guids); err != nil {
		return empty, err
	}
	if !matched {
		return empty, errors.New("reviewed interface profile is missing, ambiguous or changed")
	}
	return binding, nil
}

// GuardInterfaceBinding must use freshly obtained native snapshots, including
// after a slow elevation prompt. Drift requires closed ingress and recovery;
// it never silently adopts a new alias, prefix, index, address or profile.
func GuardInterfaceBinding(expected InterfaceBinding, scope Scope, interfaces lanobserve.Snapshot, profiles lanprofile.Snapshot, now time.Time) error {
	actual, err := BindInterface(scope, interfaces, profiles, now)
	if err != nil {
		return err
	}
	if actual != expected {
		return errors.New("native LAN interface binding changed; close ingress and review again")
	}
	return nil
}
