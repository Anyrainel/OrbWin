package localnet

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// One reviewed integration pair serves all names. It is independent of selected
// workload ports and does not change within a running route-host generation.
type ServicePorts struct {
	HTTP  uint16 `json:"http"`
	HTTPS uint16 `json:"https"`
}

func DefaultServicePorts() ServicePorts { return ServicePorts{HTTP: 80, HTTPS: 443} }
func (ports ServicePorts) validate() error {
	if ports.HTTP == 0 || ports.HTTPS == 0 || ports.HTTP == ports.HTTPS {
		return errors.New("distinct nonzero HTTP and HTTPS integration ports required")
	}
	if ports.HTTP == 80 && ports.HTTPS != 443 {
		return errors.New("choose alternate HTTP and HTTPS ports together")
	}
	return nil
}

func (ports ServicePorts) requestName(host string, secure bool) (string, bool, error) {
	name := host
	var selected uint16
	if strings.Contains(host, ":") {
		var raw string
		var err error
		name, raw, err = net.SplitHostPort(host)
		if err != nil {
			return "", false, err
		}
		parsed, err := strconv.ParseUint(raw, 10, 16)
		if err != nil || parsed == 0 || strconv.FormatUint(parsed, 10) != raw {
			return "", false, errors.New("canonical service Host port required")
		}
		selected = uint16(parsed)
	}
	standard, external := uint16(80), ports.HTTP
	if secure {
		standard, external = 443, ports.HTTPS
	}
	if selected != 0 && selected != standard && selected != external {
		return "", false, errors.New("service Host port is outside the reviewed integration pair")
	}
	canonical, err := CanonicalServiceName(name)
	return canonical, selected == external && selected != standard, err
}
func (ports ServicePorts) redirect(name string, external bool, request *url.URL) string {
	host := name
	if external && ports.HTTPS != 443 {
		host = net.JoinHostPort(name, strconv.Itoa(int(ports.HTTPS)))
	}
	return (&url.URL{Scheme: "https", Host: host, Path: request.Path, RawPath: request.RawPath, RawQuery: request.RawQuery}).String()
}
