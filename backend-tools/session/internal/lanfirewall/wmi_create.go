package lanfirewall

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// WMIDisabledCreate is a fixed call specification, not an executable provider or
// capability. No generic property/context bag, enabled create, upsert or fallback
// is exposed. Native support for these options still requires qualification.
type WMIDisabledCreate struct {
	Namespace  string
	ClassName  string
	Flags      uint32
	Properties WMICreateProperties
	Options    WMICreateOptions
}

type WMICreateProperties struct {
	InstanceID, ElementName, Description, RuleGroup           string
	Enabled, Profiles, Direction, Action, EdgeTraversalPolicy uint16
	LooseSourceMapping, LocalOnlyMapping                      bool
}

// These are provider operation options, NOT MSFT_NetFirewallRule properties.
// A future adapter must set every option as required/must-comply and refuse
// unsupported options. It must never issue a partial or enabled create.
type WMICreateOptions struct {
	PolicyStore                                            string
	LocalAddress, RemoteAddress, LocalPort, InterfaceAlias []string
	Protocol, Program                                      string
	OverrideBlockRules                                     bool
}

// PrepareWMIDisabledCreate accepts a validated create-intent and a binding
// derived from fresh native evidence. The receipt must separately come from the
// protected store; this function cannot authenticate that origin or consent.
// It deliberately has no native dispatch hook. WBEM_FLAG_CREATE_ONLY's generic
// contract is documented; exact wfascim option/atomicity behavior is not proven.
func PrepareWMIDisabledCreate(receipt Receipt, binding InterfaceBinding, now time.Time) (WMIDisabledCreate, error) {
	var empty WMIDisabledCreate
	if err := receipt.Validate(); err != nil {
		return empty, err
	}
	if err := receipt.Request.Validate(now); err != nil {
		return empty, err
	}
	scope := receipt.Request.Scope
	if receipt.State != "create-intent" || binding.GUID != scope.InterfaceGUID || binding.LUID != scope.InterfaceLUID || binding.Address != scope.LocalAddress || binding.Profile != scope.Profile || binding.Index == 0 || binding.PrefixLength > 32 || binding.Alias != receipt.Worker.InterfaceAlias || !boundedText(binding.Alias, 1024) {
		return empty, errors.New("disabled WMI create requires the exact retained create intent and fresh native interface binding")
	}
	// Microsoft's CDXML exposes InterfaceAlias as WildcardPattern[]. Until the
	// provider's literal encoding is qualified, do not let a native alias widen
	// into a wildcard. Program environment expansion must not change a pinned
	// path either. These are refusals, never ad-hoc escaping or name rewriting.
	if strings.ContainsAny(binding.Alias, "*?[]`") || strings.Contains(receipt.Worker.Path, "%") {
		return empty, errors.New("native alias/path requires unqualified provider pattern or environment expansion")
	}
	profile := uint16(2) // NET_FW_PROFILE2_PRIVATE / NetSecurity.Profile.Private
	if scope.Profile == "domain" {
		profile = 1
	}
	return WMIDisabledCreate{
		Namespace: `root\StandardCimv2`, ClassName: "MSFT_NetFirewallRule", Flags: 2, // WBEM_FLAG_CREATE_ONLY
		Properties: WMICreateProperties{
			InstanceID: receipt.RuleName, ElementName: "OrbWin selected LAN port", Description: "OrbWin exact-scope LAN exposure", RuleGroup: "OrbWin-LAN",
			Enabled: 2, Profiles: profile, Direction: 1, Action: 2, EdgeTraversalPolicy: 0,
		},
		Options: WMICreateOptions{
			PolicyStore: "PersistentStore", LocalAddress: []string{scope.LocalAddress}, RemoteAddress: append([]string{}, scope.ClientNetworks...),
			LocalPort: []string{strconv.Itoa(int(scope.LocalPort))}, InterfaceAlias: []string{binding.Alias}, Protocol: strings.ToUpper(scope.Protocol), Program: receipt.Worker.Path,
		},
	}, nil
}
