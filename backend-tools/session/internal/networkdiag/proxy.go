package networkdiag

import "context"

// ProxyConfiguration deliberately contains no URLs, hosts, bypass lists,
// credentials or raw configuration strings. Configured does not mean effective.
type ProxyConfiguration struct {
	Source           string `json:"source"`
	State            string `json:"state"`
	Mode             string `json:"mode"`
	AutoDetect       bool   `json:"autoDetect"`
	PACConfigured    bool   `json:"pacConfigured"`
	ManualConfigured bool   `json:"manualConfigured"`
	BypassConfigured bool   `json:"bypassConfigured"`
	EffectiveProxy   string `json:"effectiveProxy"`
}
type ProxySnapshot struct {
	Version        int                `json:"version"`
	CurrentUser    ProxyConfiguration `json:"currentUser"`
	WinHTTPDefault ProxyConfiguration `json:"winHttpDefault"`
}

func configuration(source string, auto, pac, manual, bypass bool) ProxyConfiguration {
	mode := "direct-configured"
	if auto || pac {
		mode = "automatic-configured"
	}
	if manual {
		if auto || pac {
			mode = "mixed-configured"
		} else {
			mode = "manual-configured"
		}
	}
	return ProxyConfiguration{Source: source, State: "observed", Mode: mode, AutoDetect: auto, PACConfigured: pac, ManualConfigured: manual, BypassConfigured: bypass, EffectiveProxy: "not-evaluated"}
}
func unknownConfiguration(source string) ProxyConfiguration {
	return ProxyConfiguration{Source: source, State: "unavailable", Mode: "unknown", EffectiveProxy: "not-evaluated"}
}

// ObserveProxy reads configuration presence only. It never performs WPAD/PAC
// discovery, fetches a PAC script, evaluates a URL, impersonates another user or
// changes proxy/session settings. Native synchronous calls require an external
// process deadline for a hard wall-clock bound.
func ObserveProxy(ctx context.Context) (ProxySnapshot, error) { return observeProxy(ctx) }
