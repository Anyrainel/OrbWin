package localnet

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

type ServiceSelector struct {
	ContainerID string `json:"containerId,omitempty"`
	Project     string `json:"project,omitempty"`
	Service     string `json:"service,omitempty"`
	Replica     uint16 `json:"replica,omitempty"`
}
type ServicePreference struct {
	Selector ServiceSelector   `json:"selector"`
	Disabled bool              `json:"disabled"`
	Name     string            `json:"name,omitempty"`
	Port     uint16            `json:"port,omitempty"`
	Routes   []ServiceEndpoint `json:"routes,omitempty"`
}

// Endpoints are exact names, never wildcard certificate or arbitrary URL inputs.
type ServiceEndpoint struct {
	Name  string `json:"name"`
	Port  uint16 `json:"port"`
	HTTPS bool   `json:"https,omitempty"`
}
type ServiceCandidate struct {
	ContainerID   string            `json:"containerId"`
	ContainerName string            `json:"containerName"`
	Selector      ServiceSelector   `json:"selector"`
	Names         []string          `json:"names"`
	Ports         []uint16          `json:"ports"`
	SelectedPort  uint16            `json:"selectedPort,omitempty"`
	Routes        []ServiceEndpoint `json:"routes,omitempty"`
	State         string            `json:"state"`
	Reason        string            `json:"reason,omitempty"`
}

var plainDNSLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func selectorText(value string) bool {
	return value != "" && len(value) <= 128 && !strings.ContainsAny(value, "\x00\r\n")
}
func (selector ServiceSelector) key() (string, error) {
	if selector.ContainerID != "" {
		if !fullID(selector.ContainerID) || selector.Project != "" || selector.Service != "" || selector.Replica != 0 {
			return "", errors.New("select an exact container or one Compose replica")
		}
		return "container:" + selector.ContainerID, nil
	}
	if !selectorText(selector.Project) || !selectorText(selector.Service) || selector.Replica == 0 {
		return "", errors.New("coherent Compose project/service/replica required")
	}
	return "compose:" + selector.Project + "\x00" + selector.Service + "\x00" + strconv.Itoa(int(selector.Replica)), nil
}

// A changed source spelling receives a hash suffix, so underscores, case,
// punctuation and truncation cannot silently select another source's label.
func serviceDNSLabel(source string) string {
	if plainDNSLabel.MatchString(source) {
		return source
	}
	var text strings.Builder
	for _, character := range strings.ToLower(source) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			text.WriteRune(character)
		} else {
			text.WriteByte('-')
		}
	}
	value := strings.Trim(text.String(), "-")
	if len(value) > 48 {
		value = strings.TrimRight(value[:48], "-")
	}
	if value == "" {
		value = "service"
	}
	digest := sha256.Sum256([]byte(source))
	return value + "-" + hex.EncodeToString(digest[:6])
}
func counterpartNames(name string) []string {
	for _, suffix := range serviceSuffixes {
		if strings.HasSuffix(name, suffix) {
			base := strings.TrimSuffix(name, suffix)
			return []string{base + serviceSuffixes[0], base + serviceSuffixes[1]}
		}
	}
	return nil
}
func containerDisplayName(names []string) string {
	selected := ""
	for _, raw := range names {
		name := strings.TrimPrefix(raw, "/")
		if name == "" || strings.Contains(name, "/") {
			continue
		}
		if selected != "" && name != selected {
			return ""
		}
		selected = name
	}
	return selected
}
func coherentCompose(labels map[string]string) (ServiceSelector, bool) {
	selector := ServiceSelector{Project: labels["com.docker.compose.project"], Service: labels["com.docker.compose.service"]}
	raw := labels["com.docker.compose.container-number"]
	number, err := strconv.ParseUint(raw, 10, 16)
	if err != nil || number == 0 || strconv.FormatUint(number, 10) != raw {
		return ServiceSelector{}, false
	}
	selector.Replica = uint16(number)
	_, err = selector.key()
	return selector, err == nil
}
func preferencesBySelector(preferences []ServicePreference) (map[string]ServicePreference, error) {
	if len(preferences) > 512 {
		return nil, errors.New("service preferences exceed 512 entries")
	}
	result := make(map[string]ServicePreference)
	for _, preference := range preferences {
		key, err := preference.Selector.key()
		if err != nil {
			return nil, err
		}
		if _, exists := result[key]; exists {
			return nil, errors.New("duplicate local service preference selector")
		}
		if preference.Name != "" {
			name, err := CanonicalServiceName(preference.Name)
			if err != nil || name != preference.Name {
				return nil, errors.New("service override requires a canonical reserved name")
			}
		}
		if len(preference.Routes) > 0 {
			if preference.Name != "" || preference.Port != 0 {
				return nil, errors.New("choose exact routes or legacy name/port overrides, not both")
			}
			if err := validateEndpoints(preference.Routes); err != nil {
				return nil, err
			}
		}
		result[key] = preference
	}
	return result, nil
}

// BuildCandidates is a pure proposal pass. Even probe-required is not a live
// route or a claim that the selected TCP port speaks HTTP.
func BuildCandidates(items []container.Summary, preferences []ServicePreference) ([]ServiceCandidate, error) {
	if len(items) > 512 {
		return nil, errors.New("local service inventory exceeds 512 containers")
	}
	choices, err := preferencesBySelector(preferences)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int)
	for _, item := range items {
		if item.State != "running" || item.Labels["dev.orbwin.services"] != "" || item.Labels["dev.orbwin.transient"] != "" || strings.EqualFold(item.Labels["com.docker.compose.oneoff"], "true") {
			continue
		}
		if selector, ok := coherentCompose(item.Labels); ok {
			counts[selector.Project+"\x00"+selector.Service]++
		}
	}
	result := make([]ServiceCandidate, 0, len(items))
	seen := make(map[string]bool)
	for _, item := range items {
		if item.Labels["dev.orbwin.services"] != "" || item.Labels["dev.orbwin.transient"] != "" || strings.EqualFold(item.Labels["com.docker.compose.oneoff"], "true") {
			continue
		}
		if !fullID(item.ID) || seen[item.ID] {
			return nil, errors.New("service inventory contains an invalid or duplicate full container ID")
		}
		seen[item.ID] = true
		name := containerDisplayName(item.Names)
		candidate := ServiceCandidate{ContainerID: item.ID, ContainerName: name, Selector: ServiceSelector{ContainerID: item.ID}, Names: []string{}, Ports: []uint16{}, State: "needs-port", Reason: "Choose the container's HTTP TCP port."}
		if selector, ok := coherentCompose(item.Labels); ok {
			candidate.Selector = selector
			project, service := serviceDNSLabel(selector.Project), serviceDNSLabel(selector.Service)
			replica := serviceDNSLabel(selector.Service + "-" + strconv.Itoa(int(selector.Replica)))
			candidate.Names = counterpartNames(replica + "." + project + serviceSuffixes[0])
			if counts[selector.Project+"\x00"+selector.Service] == 1 {
				candidate.Names = append(candidate.Names, counterpartNames(service+"."+project+serviceSuffixes[0])...)
			}
		} else if item.Labels["com.docker.compose.project"] != "" || item.Labels["com.docker.compose.service"] != "" || item.Labels["com.docker.compose.container-number"] != "" {
			candidate.State = "metadata-conflict"
			candidate.Reason = "Compose metadata is incomplete. Review an explicit container-level name and port."
		} else if name != "" {
			candidate.Names = counterpartNames(serviceDNSLabel(name) + serviceSuffixes[0])
		}
		ports := make(map[uint16]bool)
		for _, port := range item.Ports {
			if port.Type == "tcp" && port.PrivatePort != 0 {
				ports[port.PrivatePort] = true
			}
		}
		for port := range ports {
			candidate.Ports = append(candidate.Ports, port)
		}
		sort.Slice(candidate.Ports, func(i, j int) bool { return candidate.Ports[i] < candidate.Ports[j] })
		if len(candidate.Ports) == 1 {
			candidate.SelectedPort = candidate.Ports[0]
		}
		labelRoutes, labelErr := labelEndpoints(item.Labels, candidate.Names, candidate.SelectedPort)
		key, _ := candidate.Selector.key()
		preference, configured := choices[key]
		// Explicit container-level overrides take priority over a Compose default.
		if exact, ok := choices["container:"+item.ID]; ok {
			preference, configured = exact, true
		}
		if configured {
			// A reviewed UI override replaces label routing atomically. Empty UI
			// fields retain labels; disabling always removes routes.
			if len(preference.Routes) > 0 {
				labelRoutes = append([]ServiceEndpoint{}, preference.Routes...)
				labelErr = nil
			} else if preference.Name != "" || preference.Port != 0 {
				labelRoutes = nil
				labelErr = nil
			}
			if preference.Name != "" {
				candidate.Names = counterpartNames(preference.Name)
			}
			if preference.Port != 0 {
				candidate.SelectedPort = preference.Port
			}
			if preference.Name != "" && candidate.State == "metadata-conflict" {
				candidate.State = "needs-port"
			}
		}
		if len(labelRoutes) > 0 {
			candidate.Routes = labelRoutes
			candidate.Names = nil
			for _, route := range labelRoutes {
				candidate.Names = append(candidate.Names, route.Name)
			}
			candidate.SelectedPort = labelRoutes[0].Port
			if candidate.State == "metadata-conflict" {
				candidate.State = "needs-port"
			}
		}
		if len(candidate.Names) == 0 {
			candidate.State = "needs-name"
			candidate.Reason = "Review an unambiguous reserved local service name."
		}
		if candidate.State != "metadata-conflict" && len(candidate.Names) > 0 && candidate.SelectedPort != 0 {
			candidate.State = "probe-required"
			candidate.Reason = "Verify this TCP listener speaks HTTP before publishing its routes."
		}
		if labelErr != nil {
			candidate.State = "routing-error"
			candidate.Reason = labelErr.Error()
			candidate.Routes = nil
		}
		if configured && preference.Disabled {
			candidate.State = "disabled"
			candidate.Reason = "Local routing is disabled for this selection."
		} else if item.State == "paused" {
			candidate.State = "paused"
			candidate.Reason = "Resume the selected workload to restore its local HTTP route."
		} else if item.State == "restarting" {
			candidate.State = "restarting"
			candidate.Reason = "The workload is restarting; its local route is unavailable."
		} else if item.State != "running" {
			candidate.State = "stopped"
			candidate.Reason = "Start the selected workload to discover its HTTP service."
		} else if item.HostConfig.NetworkMode == "host" || strings.HasPrefix(item.HostConfig.NetworkMode, "container:") {
			candidate.State = "unsupported-network"
			candidate.Reason = "This adapter requires a private network namespace. Use existing published access."
		}
		result = append(result, candidate)
	}
	owners := make(map[string][]int)
	for index, candidate := range result {
		if candidate.State == "routing-error" || candidate.State == "disabled" || candidate.State == "stopped" || candidate.State == "paused" || candidate.State == "restarting" || candidate.State == "unsupported-network" {
			continue
		}
		for _, name := range candidate.Names {
			owners[name] = append(owners[name], index)
		}
	}
	for _, indices := range owners {
		if len(indices) > 1 {
			for _, index := range indices {
				result[index].State = "name-conflict"
				result[index].Reason = "Multiple running workloads propose this name. Choose distinct names or disable a selection."
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ContainerName == result[j].ContainerName {
			return result[i].ContainerID < result[j].ContainerID
		}
		return result[i].ContainerName < result[j].ContainerName
	})
	return result, nil
}

func DiscoverServices(ctx context.Context, cli *client.Client, daemon string, preferences []ServicePreference) ([]ServiceCandidate, error) {
	if _, err := preferencesBySelector(preferences); err != nil {
		return nil, err
	}
	if cli == nil || daemon == "" {
		return nil, errors.New("recorded owned daemon required for service discovery")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	info, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		return nil, err
	}
	if info.Info.ID != daemon || info.Info.OSType != "linux" {
		return nil, errors.New("service discovery endpoint differs from the registered owned Linux daemon")
	}
	listed, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true, Limit: 513})
	if err != nil {
		return nil, fmt.Errorf("read local service inventory: %w", err)
	}
	return BuildCandidates(listed.Items, preferences)
}
