//go:build windows

package localnet

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
)

// Qualification uses a reviewed alternate port pair and per-client roots. It neither
// installs trust nor occupies the product's eventual port 80/443 bindings.
func qualifyOwnedRelay(t *testing.T, ctx context.Context, cli *client.Client, manager *NamespaceManager, image ToolsImage, target, directory string) {
	t.Helper()
	session := fmt.Sprintf("%032x", time.Now().UnixNano())
	publicPorts := ServicePorts{HTTP: 18080, HTTPS: 18443}
	if err := probeRelayHostPorts(publicPorts); err != nil {
		t.Fatal("qualification port pair unavailable", err)
	}
	authority, err := NewAuthorityWithCRLPort(session, time.Now(), publicPorts.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	host, err := StartServiceHostWithPorts(manager, authority, session, publicPorts)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if e := host.Close(clean); e != nil {
			t.Error(e)
		}
	}()
	names := []string{"native-proof.orbwin.localhost", "native-proof.orbwin.test"}
	status, err := host.Reconcile(ctx, []ServiceCandidate{{ContainerID: target, ContainerName: "native-proof", Names: names, Ports: []uint16{39081}, SelectedPort: 39081, State: "probe-required"}}, false)
	if err != nil || len(status) != 1 || status[0].State != "active" {
		t.Fatalf("native route failed: %+v %v", status, err)
	}
	gateway := host.Gateway()
	p80, p443 := network.MustParsePort("80/tcp"), network.MustParsePort("443/tcp")
	pids := int64(128)
	options := client.ContainerCreateOptions{Name: "orbwin-p2-relay-" + session, Config: &container.Config{Image: image.ID, User: "65532:65532", Cmd: []string{"/orbwin-services", "relay", session, fmt.Sprint(gateway.Port), base64.StdEncoding.EncodeToString(gateway.PublicRoot)}, Labels: map[string]string{"dev.orbwin.qualification": "relay-" + session}, ExposedPorts: network.PortSet{p80: {}, p443: {}}}, HostConfig: &container.HostConfig{AutoRemove: true, ReadonlyRootfs: true, CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges:true"}, Tmpfs: map[string]string{"/run/orbwin": "rw,noexec,nosuid,nodev,size=64k,mode=0700,uid=65532,gid=65532"}, PortBindings: network.PortMap{p80: {{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: fmt.Sprint(publicPorts.HTTP)}}, p443: {{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: fmt.Sprint(publicPorts.HTTPS)}}}, Resources: container.Resources{Memory: 64 << 20, PidsLimit: &pids, NanoCPUs: 500_000_000}}}
	// Keep the exact intent and accepted ID if this qualification process is lost.
	intent, err := os.Create(filepath.Join(directory, "relay-intent.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = fmt.Fprintf(intent, "{\"name\":%q,\"image\":%q,\"session\":%q}\n", options.Name, image.ID, session)
	if err == nil {
		err = intent.Sync()
	}
	closed := intent.Close()
	if err != nil || closed != nil {
		t.Fatal(err, closed)
	}
	created, err := cli.ContainerCreate(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "relay-id.txt"), []byte(created.ID), 0600); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if _, e := cli.ContainerRemove(clean, created.ID, client.ContainerRemoveOptions{Force: true}); e != nil && !cerrdefs.IsNotFound(e) {
			t.Error(e)
		}
	}()
	if _, err = cli.ContainerStart(ctx, created.ID, client.ContainerStartOptions{}); err != nil {
		t.Fatal(err)
	}
	inspected, err := cli.ContainerInspect(ctx, created.ID, client.ContainerInspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	bindings := inspected.Container.NetworkSettings.Ports
	if len(bindings[p80]) != 1 || len(bindings[p443]) != 1 {
		t.Fatal("missing private relay bindings")
	}
	plainPort, securePort := bindings[p80][0].HostPort, bindings[p443][0].HostPort
	root, err := x509.ParseCertificate(authority.RootCertificate())
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(root)
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		_, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, e
		}
		return (&net.Dialer{}).DialContext(ctx, network, "127.0.0.1:"+port)
	}}
	defer transport.CloseIdleConnections()
	httpClient := &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(address, name string) (*http.Response, error) {
		request, e := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
		if e != nil {
			return nil, e
		}
		request.Host = name // Qualification uses ephemeral transport ports only.
		return httpClient.Do(request)
	}
	var response *http.Response
	for attempt := 0; attempt < 20; attempt++ {
		response, err = get("http://"+names[0]+":"+plainPort+"/a%2Fb?q=1", names[0])
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatal("published relay to authenticated Windows gateway", err)
	}
	response.Body.Close()
	if response.StatusCode != 308 || response.Header.Get("Location") != "https://"+names[0]+"/a%2Fb?q=1" {
		t.Fatal("wrong redirect", response.Status, response.Header)
	}
	for _, name := range names {
		response, err = get("https://"+name+":"+securePort+"/", name)
		if err != nil {
			t.Fatal("Windows verified HTTPS", err)
		}
		body, e := io.ReadAll(io.LimitReader(response.Body, 1024))
		response.Body.Close()
		if e != nil || response.StatusCode != 200 || string(body) != "orbwin-p2-loopback-1792892200" {
			t.Fatalf("wrong workload: %q %v", body, e)
		}
	}
	rootPath := filepath.Join(directory, "public-root.pem")
	if err = os.WriteFile(rootPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: root.Raw}), 0644); err != nil {
		t.Fatal(err)
	}
	linuxRoot := "/mnt/" + strings.ToLower(rootPath[:1]) + strings.ReplaceAll(rootPath[2:], "\\", "/")
	command := exec.CommandContext(ctx, "wsl.exe", "-d", "Ubuntu", "--exec", "curl", "--noproxy", "*", "--fail", "--silent", "--show-error", "--max-time", "5", "--cacert", linuxRoot, "--header", "Host: "+names[0], "--resolve", names[0]+":"+securePort+":127.0.0.1", "https://"+names[0]+":"+securePort+"/")
	output, e := command.CombinedOutput()
	if e != nil || string(output) != "orbwin-p2-loopback-1792892200" {
		t.Fatalf("WSL verified HTTPS: %q %v", output, e)
	}
	// Normal resolver, SNI and Host include the reviewed public pair; no forced
	// address, synthetic Host header, proxy or relaxed TLS verification is used.
	ordinaryTransport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}}
	defer ordinaryTransport.CloseIdleConnections()
	ordinaryClient := &http.Client{Transport: ordinaryTransport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err = ordinaryClient.Get("https://" + names[0] + ":" + securePort + "/")
	if err != nil {
		t.Fatal("Windows Go ordinary hostname verified HTTPS", err)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1024))
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || string(body) != "orbwin-p2-loopback-1792892200" {
		t.Fatalf("Windows ordinary HTTPS %q %v", body, err)
	}
	strictWindows := exec.CommandContext(ctx, "curl.exe", "--noproxy", "*", "--fail", "--silent", "--show-error", "--max-time", "8", "--cacert", rootPath, "https://"+names[0]+":"+securePort+"/")
	strictOutput, strictErr := strictWindows.CombinedOutput()
	if strictErr != nil || string(strictOutput) != "orbwin-p2-loopback-1792892200" {
		t.Fatalf("Windows Schannel with normal revocation checks %q %v", strictOutput, strictErr)
	}
	clientProof, encodeErr := json.MarshalIndent(map[string]any{"normalHostnameResolution": true, "forcedHostHeader": false, "WindowsGoTLS": true, "WindowsSchannelRevocationChecks": true, "authorityProfile": 2, "revocationHTTPPort": publicPorts.HTTP, "rootSha256": authority.Fingerprint(), "systemRootTrustInstalled": false}, "", "  ")
	if encodeErr != nil {
		t.Fatal(encodeErr)
	}
	if err = os.WriteFile(filepath.Join(directory, "client-proof.json"), clientProof, 0600); err != nil {
		t.Fatal(err)
	}
	response, err = ordinaryClient.Get("http://" + names[0] + ":" + plainPort + "/a%2Fb?q=1")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 308 || response.Header.Get("Location") != "https://"+names[0]+":"+securePort+"/a%2Fb?q=1" {
		t.Fatal("public redirect lost reviewed HTTPS port", response.Status, response.Header)
	}
	command = exec.CommandContext(ctx, "wsl.exe", "-d", "Ubuntu", "--exec", "curl", "--noproxy", "*", "--fail", "--silent", "--show-error", "--max-time", "4", "--cacert", linuxRoot, "https://"+names[0]+":"+securePort+"/")
	output, e = command.CombinedOutput()
	if e != nil || string(output) != "orbwin-p2-loopback-1792892200" {
		t.Fatalf("Ubuntu ordinary hostname HTTPS %q %v", output, e)
	}
	bridge := inspected.Container.NetworkSettings.Networks["bridge"]
	if bridge == nil || fmt.Sprint(bridge.IPAddress) == "" {
		t.Fatal("qualification relay lacks bridge address")
	}
	process, e := cli.ExecCreate(ctx, target, client.ExecCreateOptions{Cmd: []string{"/probe", "client", "https://" + names[1] + "/", net.JoinHostPort(fmt.Sprint(bridge.IPAddress), "443"), base64.StdEncoding.EncodeToString(root.Raw)}, AttachStdout: true, AttachStderr: true})
	if e != nil {
		t.Fatal(e)
	}
	attached, e := cli.ExecAttach(ctx, process.ID, client.ExecAttachOptions{})
	if e != nil {
		t.Fatal(e)
	}
	var stdout, stderr strings.Builder
	_, e = stdcopy.StdCopy(&stdout, &stderr, attached.Reader)
	attached.Close()
	state, inspectErr := cli.ExecInspect(ctx, process.ID, client.ExecInspectOptions{})
	if e != nil || inspectErr != nil || state.Running || state.ExitCode != 0 || stdout.String() != "orbwin-p2-loopback-1792892200" || stderr.Len() != 0 {
		t.Fatalf("container verified HTTPS: %q %q %+v %v %v", stdout.String(), stderr.String(), state, e, inspectErr)
	}
	proof := fmt.Sprintf("Windows HTTP redirect + both verified HTTPS names; Ubuntu verified HTTPS; container verified HTTPS; relay %s; private ports %s/%s; no system trust installed\n", created.ID, plainPort, securePort)
	if e = os.WriteFile(filepath.Join(directory, "relay-proof.txt"), []byte(proof), 0600); e != nil {
		t.Fatal(e)
	}
	t.Log(strings.TrimSpace(proof))
	if os.Getenv("ORBWIN_QUALIFY_INGRESS") == "1" {
		qualificationName := "q-" + gateway.Session + ".orbwin.test"
		selectedNames := append(append([]string{}, names...), qualificationName)
		states, e := host.Reconcile(ctx, []ServiceCandidate{{ContainerID: target, ContainerName: "native-proof", Names: selectedNames, Ports: []uint16{39081}, SelectedPort: 39081, State: "probe-required"}}, false)
		if e != nil || states[0].State != "active" {
			t.Fatal("ingress fixture route", states, e)
		}
		qualifyExistingIngress(t, ctx, fmt.Sprint(bridge.IPAddress), plainPort, securePort, gateway.Session, qualificationName, directory, httpClient)
	}
	qualifyManagedRelay(t, ctx, cli, manager.ledger, image, gateway, httpClient, names[0], directory)
	// Retire the exact manual fixture before exercising the production manager
	// on the same reviewed pair. The route host/namespace owner remains live.
	if _, err = cli.ContainerRemove(ctx, created.ID, client.ContainerRemoveOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	if err = awaitRelayPublicationRelease(ctx, publicPorts, probeRelayHostPorts); err != nil {
		t.Fatal(err)
	}
	managed, err := OpenRelayManager(ctx, cli, manager.ledger, image)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if e := managed.Close(clean); e != nil {
			t.Error(e)
		}
	}()
	managedID, err := managed.StartWithPorts(ctx, gateway, nil, publicPorts)
	if err != nil {
		t.Fatal("production alternate relay start/profile", err)
	}
	if err = managed.Renew(ctx); err != nil {
		t.Fatal("production alternate relay renewal", err)
	}
	captureWindowsPublicationScope(t, ctx, publicPorts, filepath.Join(directory, "windows-publication-scope.json"))
	response, err = ordinaryClient.Get("https://" + names[0] + ":" + securePort + "/")
	if err != nil {
		captureManagedRelayFailure(t, cli, managedID, host, directory, ordinaryTransport, "https://"+names[0]+":"+securePort+"/", err)
		t.Fatal(err)
	}
	body, err = io.ReadAll(io.LimitReader(response.Body, 1024))
	response.Body.Close()
	if err != nil || string(body) != "orbwin-p2-loopback-1792892200" {
		t.Fatalf("managed alternate workload %q %v", body, err)
	}
	if err = managed.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "managed-alternate-proof.txt"), []byte("normal Windows Go TLS + Ubuntu curl hostname/SNI/Host and public redirect; production relay profile, acknowledged renewal and exact stop; ports 18080/18443; relay "+managedID+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("normal Windows Go TLS and Ubuntu curl HTTPS + reviewed redirect passed; managed relay 18080/18443 profile, renewal and exact stop passed")
}

func captureWindowsPublicationScope(t *testing.T, ctx context.Context, ports ServicePorts, path string) {
	t.Helper()
	script := fmt.Sprintf("$ErrorActionPreference = 'Stop'; Get-NetTCPConnection -State Listen -LocalPort %d,%d | Select-Object LocalAddress,LocalPort,OwningProcess | ConvertTo-Json -Compress", ports.HTTP, ports.HTTPS)
	command := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-Command", script)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	data, err := command.Output()
	if err != nil {
		t.Fatal("Windows publication scope inspection", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	var listeners []struct {
		LocalAddress  string
		LocalPort     int
		OwningProcess int
	}
	if err := json.Unmarshal(data, &listeners); err != nil {
		t.Fatal(err)
	}
	loopbackPorts := map[int]bool{}
	for _, listener := range listeners {
		ip := net.ParseIP(listener.LocalAddress)
		if ip == nil || !ip.IsLoopback() {
			t.Fatal("relay exposed a non-loopback Windows listener", string(data))
		}
		if listener.LocalAddress == "127.0.0.1" {
			loopbackPorts[listener.LocalPort] = true
		}
	}
	if !loopbackPorts[int(ports.HTTP)] || !loopbackPorts[int(ports.HTTPS)] {
		t.Fatal("owned IPv4 publication missing", string(data))
	}
}

func captureManagedRelayFailure(t *testing.T, cli *client.Client, id string, host *ServiceHost, directory string, transport *http.Transport, address string, cause error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	report := map[string]any{"error": cause.Error(), "helperID": id, "hostDiagnostics": host.Diagnostics()}
	if inspected, e := cli.ContainerInspect(ctx, id, client.ContainerInspectOptions{}); e == nil {
		report["networkSettings"] = inspected.Container.NetworkSettings
		report["state"] = inspected.Container.State
		report["networkMode"] = inspected.Container.HostConfig.NetworkMode
	} else {
		report["inspectError"] = e.Error()
	}
	if logs, e := cli.ContainerLogs(ctx, id, client.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Tail: "8"}); e == nil {
		var stdout, stderr strings.Builder
		_, logErr := stdcopy.StdCopy(&stdout, &stderr, io.LimitReader(logs, 16384))
		logs.Close()
		report["stdout"] = stdout.String()
		report["stderr"] = stderr.String()
		if logErr != nil {
			report["logReadError"] = logErr.Error()
		}
	} else {
		report["logError"] = e.Error()
	}
	fresh := transport.Clone()
	defer fresh.CloseIdleConnections()
	client := &http.Client{Transport: fresh, Timeout: 3 * time.Second}
	response, e := client.Get(address)
	if e != nil {
		report["freshConnectionError"] = e.Error()
	} else {
		report["freshConnectionStatus"] = response.StatusCode
		response.Body.Close()
	}
	encoded, e := json.MarshalIndent(report, "", "  ")
	if e != nil {
		t.Error(e)
		return
	}
	if e = os.WriteFile(filepath.Join(directory, "managed-relay-failure.json"), encoded, 0600); e != nil {
		t.Error(e)
	}
	t.Log("captured managed relay failure and independent fresh-connection result:", directory)
}

func qualifyManagedRelay(t *testing.T, ctx context.Context, cli *client.Client, ledger *NamespaceLedger, image ToolsImage, descriptor GatewayDescriptor, httpClient *http.Client, name, directory string) {
	t.Helper()
	// Check without displacing any existing Windows listener. Start still owns
	// definitive conflict detection if another process wins the release/start race.
	var reserved []net.Listener
	conflict := false
	for _, address := range []string{"127.0.0.1:80", "127.0.0.1:443"} {
		listener, err := net.Listen("tcp4", address)
		if err != nil {
			for _, existing := range reserved {
				existing.Close()
			}
			conflict = true
			break
		}
		reserved = append(reserved, listener)
	}
	for _, listener := range reserved {
		listener.Close()
	}
	relay, err := OpenRelayManager(ctx, cli, ledger, image)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		clean, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		if e := relay.Close(clean); e != nil {
			t.Error(e)
		}
	}()
	id, err := relay.Start(ctx, descriptor, nil)
	if conflict {
		if !errors.Is(err, ErrRelayPortUnavailable) {
			t.Fatalf("known port conflict was not refused: %s %v", id, err)
		}
		records, readErr := ledger.relayRecords()
		if readErr != nil || len(records) != 0 {
			t.Fatal("known conflict committed create intent", records, readErr)
		}
		if writeErr := os.WriteFile(filepath.Join(directory, "managed-relay-conflict-proof.txt"), []byte("existing Windows listener preserved; Start refused before committing relay intent or creating a container: "+err.Error()+"\n"), 0600); writeErr != nil {
			t.Fatal(writeErr)
		}
		t.Log("managed relay safely refused existing loopback port conflict before committing create intent")
		return
	}
	if err != nil {
		t.Fatal("managed relay start", err)
	}
	if err = relay.Renew(ctx); err != nil {
		t.Fatal("managed relay renewal", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+name+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	var response *http.Response
	for attempt := 0; attempt < 20; attempt++ {
		response, err = httpClient.Do(request)
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatal("managed fixed-port HTTPS", err)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1024))
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || string(body) != "orbwin-p2-loopback-1792892200" {
		t.Fatalf("managed workload %q %v", body, err)
	}
	if err = relay.Stop(ctx); err != nil {
		t.Fatal("managed relay exact stop", err)
	}
	records, err := ledger.relayRecords()
	if err != nil || len(records) != 0 {
		t.Fatal("relay receipt not settled", records, err)
	}
	if err = os.WriteFile(filepath.Join(directory, "managed-relay-proof.txt"), []byte("verified fixed loopback 80/443 configuration + Windows HTTPS; acknowledged renewal; exact stop and receipt settlement; relay "+id+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("managed fixed loopback 80/443 relay, verified Windows HTTPS, renewal and exact receipt settlement passed")
}
