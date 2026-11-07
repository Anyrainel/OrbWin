//go:build windows

package localnet

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Connectivity experiment only. A product endpoint must additionally restrict
// accepted peers, supervise listeners and qualify address/trust setup/recovery.
func TestOwnedWSLDirectedEndpoint(t *testing.T) {
	if os.Getenv("ORBWIN_QUALIFY_OWNED") != "1" || os.Getenv("ORBWIN_QUALIFY_DIRECT_WSL") != "1" {
		t.Skip("explicit direct-interface experiment; current environment times out without additional network policy")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "wsl.exe", "-d", "Ubuntu", "--exec", "ip", "-4", "route", "show", "default").Output()
	if err != nil {
		t.Fatal(err)
	}
	fields := strings.Fields(string(output))
	if len(fields) < 3 || fields[0] != "default" || fields[1] != "via" {
		t.Fatal("one qualified WSL default gateway required")
	}
	ip := net.ParseIP(fields[2])
	if ip == nil || ip.To4() == nil || !ip.IsPrivate() {
		t.Fatal("private IPv4 gateway required")
	}
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		t.Fatal(err)
	}
	local := false
	for _, address := range addresses {
		if prefix, ok := address.(*net.IPNet); ok && prefix.IP.Equal(ip) {
			local = true
		}
	}
	if !local {
		t.Fatal("WSL gateway is not an address owned by this Windows host")
	}
	host, authority, _, _ := hostFixture(t)
	if statuses, e := host.Reconcile(ctx, []ServiceCandidate{controllerCandidate()}, false); e != nil || statuses[0].State != "active" {
		t.Fatal(statuses, e)
	}
	configuration, err := host.controller.TLSConfig(authority)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := net.Listen("tcp4", net.JoinHostPort(ip.String(), "80"))
	if err != nil {
		t.Fatal("specific WSL-facing HTTP bind", err)
	}
	defer plain.Close()
	secure, err := net.Listen("tcp4", net.JoinHostPort(ip.String(), "443"))
	if err != nil {
		t.Fatal("specific WSL-facing HTTPS bind", err)
	}
	defer secure.Close()
	httpServer := &http.Server{Handler: host.controller.Handler(false), ReadHeaderTimeout: time.Second, IdleTimeout: time.Second}
	tlsServer := &http.Server{Handler: host.controller.Handler(true), TLSConfig: configuration, ReadHeaderTimeout: time.Second, IdleTimeout: time.Second}
	done := make(chan error, 2)
	go func() { done <- httpServer.Serve(plain) }()
	go func() { done <- tlsServer.ServeTLS(secure, "", "") }()
	defer func() { httpServer.Close(); tlsServer.Close(); <-done; <-done }()
	directory, err := filepath.Abs(filepath.Join("../../../artifacts/qualification", fmt.Sprintf("p2-wsl-directed-%d", time.Now().UnixNano())))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	publicPath := filepath.Join(directory, "public-root.pem")
	if err = os.WriteFile(publicPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: authority.RootCertificate()}), 0644); err != nil {
		t.Fatal(err)
	}
	linuxRoot := "/mnt/" + strings.ToLower(publicPath[:1]) + strings.ReplaceAll(publicPath[2:], "\\", "/")
	name := "app.orbwin.test"
	command := exec.CommandContext(ctx, "wsl.exe", "-d", "Ubuntu", "--exec", "curl", "--noproxy", "*", "--fail", "--silent", "--show-error", "--max-time", "4", "--cacert", linuxRoot, "--resolve", name+":443:"+ip.String(), "https://"+name+"/")
	output, err = command.CombinedOutput()
	if err != nil || string(output) != "selected workload\x00response" {
		t.Fatalf("WSL directed verified HTTPS %q %v", output, err)
	}
	command = exec.CommandContext(ctx, "wsl.exe", "-d", "Ubuntu", "--exec", "curl", "--noproxy", "*", "--silent", "--show-error", "--max-time", "4", "--dump-header", "-", "--output", "/dev/null", "--resolve", name+":80:"+ip.String(), "http://"+name+"/a%2Fb?q=1")
	output, err = command.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "308 Permanent Redirect") || !strings.Contains(string(output), "Location: https://"+name+"/a%2Fb?q=1") {
		t.Fatalf("WSL directed HTTP redirect %q %v", output, err)
	}
	encoded, err := json.MarshalIndent(map[string]any{"windowsOwnedWSLGateway": ip.String(), "selectedDistribution": "Ubuntu", "verifiedHTTPS": true, "HTTPRedirect": true, "systemTrustModified": false, "otherListenersModified": false, "backend": "controlled route fixture; no live engine target in this connectivity proof"}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "proof.json"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("WSL verifies HTTPS and receives HTTP redirect through the exact Windows-owned WSL gateway interface, without changing existing loopback listeners:", directory)
}
