package lanprovider

import (
	"orbwin.local/session/internal/lanfirewall"
	"time"
)

// DisabledCreate is an opaque, copied MI-specific encoding plan. It has no
// dispatch/handle or generic property/option API and grants no authority.
type DisabledCreate struct {
	namespace, className        string
	rttiFlags                   uint32
	customOptionsMustUnderstand bool
	properties                  lanfirewall.WMICreateProperties
	options                     requiredCreateOptions
}
type requiredCreateOptions struct {
	policyStore                                            string
	localAddress, remoteAddress, localPort, interfaceAlias []string
	protocol, program                                      string
	overrideBlockRules                                     bool
	mustComply                                             bool
}

func PrepareDisabledCreate(receipt lanfirewall.Receipt, binding lanfirewall.InterfaceBinding, now time.Time) (DisabledCreate, error) {
	plan, err := lanfirewall.PrepareWMIDisabledCreate(receipt, binding, now)
	if err != nil {
		return DisabledCreate{}, err
	}
	// The existing plan's Flags=2 is WBEM_FLAG_CREATE_ONLY. MI CreateInstance
	// has separate fail-if-existing semantics; its RTTI flags are zero here.
	// Neither documented API contract proves wfascim atomic/exclusive creation.
	return DisabledCreate{namespace: Namespace, className: ClassName, rttiFlags: 0, customOptionsMustUnderstand: true, properties: plan.Properties, options: requiredCreateOptions{
		policyStore: plan.Options.PolicyStore, localAddress: append([]string(nil), plan.Options.LocalAddress...), remoteAddress: append([]string(nil), plan.Options.RemoteAddress...), localPort: append([]string(nil), plan.Options.LocalPort...), interfaceAlias: append([]string(nil), plan.Options.InterfaceAlias...), protocol: plan.Options.Protocol, program: plan.Options.Program, overrideBlockRules: false, mustComply: true,
	}}, nil
}
