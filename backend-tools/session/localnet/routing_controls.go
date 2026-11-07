package localnet

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func validateEndpoints(routes []ServiceEndpoint) error {
	if len(routes) == 0 || len(routes) > 16 {
		return errors.New("choose between one and sixteen exact local routes per workload")
	}
	seen := map[string]bool{}
	for _, route := range routes {
		name, err := CanonicalServiceName(route.Name)
		if err != nil || name != route.Name || seen[name] || route.Port == 0 {
			return errors.New("local routes require distinct canonical reserved names and ports 1–65535")
		}
		seen[name] = true
	}
	return nil
}

func routePort(raw string) (uint16, error) {
	port, err := strconv.ParseUint(raw, 10, 16)
	if err != nil || port == 0 || strconv.FormatUint(port, 10) != raw {
		return 0, errors.New("routing labels require a decimal port from 1 to 65535 without whitespace or leading zeroes")
	}
	return uint16(port), nil
}

// Only the OrbWin namespace is accepted. OrbStack labels cannot silently grant
// routes when a project is launched against a different engine.
func labelEndpoints(labels map[string]string, names []string, discovered uint16) ([]ServiceEndpoint, error) {
	httpPort, hasHTTP := labels["dev.orbwin.http-port"]
	httpsPort, hasHTTPS := labels["dev.orbwin.https-port"]
	domains, hasDomains := labels["dev.orbwin.domains"]
	if !hasHTTP && !hasHTTPS && !hasDomains {
		return nil, nil
	}
	if hasHTTP && hasHTTPS {
		return nil, errors.New("choose dev.orbwin.http-port or dev.orbwin.https-port; both are ambiguous")
	}
	port, secure := discovered, hasHTTPS
	var err error
	if hasHTTP {
		port, err = routePort(httpPort)
	}
	if hasHTTPS {
		port, err = routePort(httpsPort)
	}
	if err != nil {
		return nil, err
	}
	routes := []ServiceEndpoint{}
	if hasDomains {
		if len(domains) > 8192 {
			return nil, errors.New("local domain labels exceed their 8192-byte budget")
		}
		for _, entry := range strings.Split(domains, ",") {
			// Spaces around comma-separated entries are accepted; no normalization of
			// names, case, wildcard expressions or port text is performed.
			parts := strings.Split(strings.TrimSpace(entry), "@")
			if len(parts) > 2 || parts[0] == "" {
				return nil, errors.New("domain label must contain exact-name[@port[:https]] entries")
			}
			route := ServiceEndpoint{Name: parts[0], Port: port, HTTPS: secure}
			if len(parts) == 2 {
				selected := strings.Split(parts[1], ":")
				if len(selected) > 2 || len(selected) == 2 && selected[1] != "https" {
					return nil, errors.New("only :https is supported for a TLS backend")
				}
				route.Port, err = routePort(selected[0])
				if err != nil {
					return nil, err
				}
				route.HTTPS = len(selected) == 2
			}
			routes = append(routes, route)
		}
	} else {
		for _, name := range names {
			routes = append(routes, ServiceEndpoint{Name: name, Port: port, HTTPS: secure})
		}
	}
	if err := validateEndpoints(routes); err != nil {
		return nil, fmt.Errorf("review dev.orbwin routing labels: %w", err)
	}
	return routes, nil
}

// Reconciliation groups aliases by exact listener so each listener is probed and
// retried independently while all groups retain the same owned namespace helper.
// Input snapshots are validated before expansion; duplicate target IDs here are
// internal and never accepted from discovery input.
func expandServiceListeners(candidates []ServiceCandidate) []ServiceCandidate {
	expanded := []ServiceCandidate{}
	for _, candidate := range candidates {
		if candidate.State != "probe-required" || len(candidate.Routes) == 0 {
			expanded = append(expanded, candidate)
			continue
		}
		indexes := map[string]int{}
		for _, route := range candidate.Routes {
			key := fmt.Sprintf("%d/%t", route.Port, route.HTTPS)
			if route.HTTPS {
				key += "/" + route.Name
			}
			index, found := indexes[key]
			if !found {
				index = len(expanded)
				indexes[key] = index
				copy := candidate
				copy.Names = nil
				copy.Routes = nil
				copy.SelectedPort = route.Port
				expanded = append(expanded, copy)
			}
			expanded[index].Names = append(expanded[index].Names, route.Name)
			expanded[index].Routes = append(expanded[index].Routes, route)
		}
	}
	return expanded
}

func candidateHTTPS(candidate ServiceCandidate) bool {
	return len(candidate.Routes) > 0 && candidate.Routes[0].HTTPS
}
func candidateAttemptKey(candidate ServiceCandidate, stamp string) string {
	key := routeAttemptKey(candidate.ContainerID, stamp, candidate.SelectedPort)
	if candidateHTTPS(candidate) {
		key += "/https/" + candidate.Names[0]
	}
	return key
}
func routeScheme(route ServiceRoute) string {
	if route.HTTPS {
		return "https"
	}
	return "http"
}
