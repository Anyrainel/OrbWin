package main

import (
	"net"
	"net/url"
	"strings"
)

const dockerHubAuthKey = "https://index.docker.io/v1/"

// Mirrors the selected CLI login/logout constructors + pflag v1.0.10 value
// consumption (including interspersed flags, --, clusters, attached and = forms).
// No credentials are stored here: only the final option emptiness matters for
// deciding whether default-Hub device flow can write its two OAuth records.
func registryCommandKeys(args []string) ([]string, error) {
	if len(args) == 0 || (args[0] != "login" && args[0] != "logout") {
		return nil, errClientConfig
	}
	login := args[0] == "login"
	flags := map[string]bool{"--help": false, "-h": false, "--debug": false, "-D": false, "--log-level": true, "-l": true, "--version": false, "-v": false}
	if login {
		for _, k := range []string{"--username", "-u", "--password", "-p"} {
			flags[k] = true
		}
		flags["--password-stdin"] = false
	}
	var positional []string
	user, password := false, false
	set := func(key, value string) {
		if key == "--username" || key == "-u" {
			user = value != ""
		}
		if key == "--password" || key == "-p" {
			password = value != ""
		}
	}
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if arg == "-" || !strings.HasPrefix(arg, "-") {
			positional = append(positional, arg)
			continue
		}
		if strings.HasPrefix(arg, "--") {
			key, value, eq := strings.Cut(arg, "=")
			takes, ok := flags[key]
			if !ok {
				return nil, errClientConfig
			}
			if takes && !eq {
				i++
				if i >= len(args) {
					return nil, errClientConfig
				}
				value = args[i]
			}
			set(key, value)
			continue
		}
		for j := 1; j < len(arg); j++ {
			key := "-" + arg[j:j+1]
			takes, ok := flags[key]
			if !ok {
				return nil, errClientConfig
			}
			var value string
			if j+1 < len(arg) && arg[j+1] == '=' {
				value = arg[j+2:]
				j = len(arg)
			} else if takes {
				if j+1 < len(arg) {
					value = arg[j+1:]
					j = len(arg)
				} else {
					i++
					if i >= len(args) {
						return nil, errClientConfig
					}
					value = args[i]
				}
			}
			set(key, value)
		}
	}
	if len(positional) > 1 {
		return nil, errClientConfig
	}
	server := ""
	if len(positional) == 1 {
		server = positional[0]
	}
	if login {
		if server == "" || server == "docker.io" {
			server = dockerHubAuthKey
		}
		if server == dockerHubAuthKey {
			if !user && !password {
				return []string{server, server + "access-token", server + "refresh-token"}, nil
			}
			return []string{server}, nil
		}
		return []string{dockerRegistryHostname(server)}, nil
	}
	if server == "" {
		return []string{dockerHubAuthKey, dockerHubAuthKey + "access-token", dockerHubAuthKey + "refresh-token"}, nil
	}
	host := dockerRegistryHostname(server)
	return []string{server, host, "http://" + host, "https://" + host}, nil
}

// Adapted from the selected CLI cli/config/credentials/file_store.go
// ConvertToHostname + hostFromURLFallback (Apache-2.0). Keep exact case, port,
// URL and legacy IPv6 behavior; this is a key adapter, not URL validation.
func dockerRegistryHostname(value string) string {
	if strings.Contains(value, "://") {
		parsed, e := url.Parse(value)
		if e == nil && parsed.Hostname() != "" {
			if parsed.Port() == "" {
				return parsed.Hostname()
			}
			return net.JoinHostPort(parsed.Hostname(), parsed.Port())
		}
		_, rest, _ := strings.Cut(value, "://")
		host, _, _ := strings.Cut(rest, "/")
		if host != "" {
			if strings.Count(host, ":") > 1 && !strings.HasPrefix(host, "[") {
				p := strings.LastIndex(host, ":")
				addr, port := host[:p], host[p+1:]
				numeric := port != ""
				for _, r := range port {
					if r < '0' || r > '9' {
						numeric = false
					}
				}
				if addr != "" && numeric {
					return net.JoinHostPort(addr, port)
				}
			}
			return host
		}
	}
	host, _, _ := strings.Cut(value, "/")
	return host
}
