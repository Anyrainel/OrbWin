package localnet

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"github.com/moby/moby/api/types/container"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoutingLabelsExactNamesAndPerNamePorts(t *testing.T) {
	item := serviceSummary(1, "app", 80, 8080)
	item.Labels["dev.orbwin.domains"] = "app.orbwin.localhost@8080, admin.orbwin.test@8443:https"
	candidates, err := BuildCandidates([]container.Summary{item}, nil)
	if err != nil || len(candidates) != 1 || candidates[0].State != "probe-required" || len(candidates[0].Routes) != 2 || candidates[0].Routes[0].Port != 8080 || !candidates[0].Routes[1].HTTPS {
		t.Fatalf("per-name routes lost: %+v %v", candidates, err)
	}
	for key, value := range map[string]string{"dev.orbwin.http-port": "8080", "dev.orbwin.https-port": "8443"} {
		item.Labels = map[string]string{key: value}
		got, err := BuildCandidates([]container.Summary{item}, nil)
		if err != nil || got[0].State != "probe-required" || len(got[0].Routes) != 2 || got[0].Routes[0].HTTPS != (key == "dev.orbwin.https-port") {
			t.Fatalf("default label selection failed: %+v %v", got, err)
		}
	}
}

func TestRoutingLabelsMalformedAndUIPrecedence(t *testing.T) {
	bad := []map[string]string{
		{"dev.orbwin.http-port": "0"}, {"dev.orbwin.http-port": "65536"}, {"dev.orbwin.http-port": "080"}, {"dev.orbwin.http-port": " 80"},
		{"dev.orbwin.http-port": "80", "dev.orbwin.https-port": "443"},
		{"dev.orbwin.domains": "*.app.orbwin.test@80"}, {"dev.orbwin.domains": "evil.example@80"},
		{"dev.orbwin.domains": "app.orbwin.test@80,app.orbwin.test@81"},
		{"dev.orbwin.domains": "app.orbwin.test@8443:http"}, {"dev.orbwin.domains": "App.orbwin.test@80"},
		{"dev.orbwin.domains": "app.orbwin.test@80,"}, {"dev.orbwin.domains": ""},
	}
	for _, labels := range bad {
		item := serviceSummary(1, "app", 80)
		item.Labels = labels
		got, err := BuildCandidates([]container.Summary{item}, nil)
		if err != nil || got[0].State != "routing-error" {
			t.Fatalf("malformed labels were accepted: %v %+v %v", labels, got, err)
		}
		override := ServicePreference{Selector: ServiceSelector{ContainerID: item.ID}, Routes: []ServiceEndpoint{{Name: "reviewed.orbwin.test", Port: 9000}}}
		got, err = BuildCandidates([]container.Summary{item}, []ServicePreference{override})
		if err != nil || got[0].State != "probe-required" || got[0].Routes[0].Name != "reviewed.orbwin.test" {
			t.Fatalf("reviewed override did not replace invalid labels: %+v %v", got, err)
		}
	}
	item := serviceSummary(1, "app", 80)
	item.Labels = map[string]string{"dev.orbstack.http-port": "9000", "dev.orbstack.domains": "unrelated.orbwin.test"}
	got, err := BuildCandidates([]container.Summary{item}, nil)
	if err != nil || len(got[0].Routes) != 0 || got[0].SelectedPort != 80 {
		t.Fatalf("foreign label namespace influenced routing: %+v %v", got, err)
	}
	preference := ServicePreference{Selector: ServiceSelector{ContainerID: item.ID}, Routes: []ServiceEndpoint{{Name: "app.orbwin.test", Port: 80}, {Name: "app.orbwin.test", Port: 81}}}
	if _, err := BuildCandidates([]container.Summary{item}, []ServicePreference{preference}); err == nil {
		t.Fatal("duplicate UI routes accepted")
	}
}

func TestRoutingExactChoicesKeepSourceIdentityAndConflicts(t *testing.T) {
	first := serviceSummary(1, "first", 80)
	second := serviceSummary(2, "second", 80)
	first.Labels["dev.orbwin.domains"] = "shared.orbwin.test@8080,first.orbwin.test@9000"
	second.Labels["dev.orbwin.domains"] = "shared.orbwin.test@8000"
	got, err := BuildCandidates([]container.Summary{first, second}, nil)
	if err != nil || got[0].State != "name-conflict" || got[1].State != "name-conflict" {
		t.Fatalf("cross-workload collision was routed: %+v %v", got, err)
	}
	stale := ServicePreference{Selector: ServiceSelector{ContainerID: first.ID}, Routes: []ServiceEndpoint{{Name: "reviewed.orbwin.test", Port: 9090}}}
	got, err = BuildCandidates([]container.Summary{second}, []ServicePreference{stale})
	if err != nil || got[0].Routes[0].Name != "shared.orbwin.test" {
		t.Fatalf("stale exact-container choice selected replacement: %+v %v", got, err)
	}
	compose := composeSummary(3, "project", "app", "1")
	selector, _ := coherentCompose(compose.Labels)
	compose.ID = strings.Repeat("e", 64)
	stale.Selector = selector
	got, err = BuildCandidates([]container.Summary{compose}, []ServicePreference{stale})
	if err != nil || got[0].Routes[0].Name != "reviewed.orbwin.test" {
		t.Fatalf("coherent recreation lost reviewed routes: %+v %v", got, err)
	}
}

func TestRoutingSelectedNetworksShareOnlyCurrentExactTestNames(t *testing.T) {
	statuses := []ServiceStatus{
		{ContainerID: strings.Repeat("a", 64), Names: []string{"app.project-one.orbwin.localhost", "app.project-one.orbwin.test"}, State: "active"},
		{ContainerID: strings.Repeat("b", 64), Names: []string{"admin.project-two.orbwin.test"}, State: "active"},
		{ContainerID: strings.Repeat("c", 64), Names: []string{"stale.orbwin.test"}, State: "probe-failed"},
	}
	ids := []string{strings.Repeat("d", 64), strings.Repeat("e", 64)}
	networks, err := runtimeNetworks(ids, statuses)
	if err != nil || len(networks) != 2 || len(networks[0].Aliases) != 2 || networks[0].Aliases[0] != "admin.project-two.orbwin.test" || networks[0].Aliases[1] != "app.project-one.orbwin.test" {
		t.Fatalf("cross-project exact alias projection differs: %+v %v", networks, err)
	}
	if got, err := runtimeNetworks(nil, statuses); err != nil || len(got) != 0 {
		t.Fatalf("removed network selection retained alias projection: %+v %v", got, err)
	}
}

type routingBackend struct {
	*controllerBackend
	addresses map[uint16]string
}

func (backend *routingBackend) Dial(ctx context.Context, route ServiceRoute) (net.Conn, error) {
	address := backend.addresses[route.Port]
	if address == "" {
		return nil, errors.New("fixture listener unavailable")
	}
	return (&net.Dialer{}).DialContext(ctx, "tcp", address)
}

func TestRoutingListenersFailIndependentlyAndRetireChangedPorts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer server.Close()
	backend := &routingBackend{controllerBackend: &controllerBackend{started: "2026-10-24T12:00:00Z"}, addresses: map[uint16]string{8080: server.Listener.Addr().String()}}
	controller, err := NewServiceController(backend)
	if err != nil {
		t.Fatal(err)
	}
	defer controller.Close(context.Background())
	candidate := controllerCandidate()
	candidate.Names = []string{"app.orbwin.localhost", "admin.orbwin.test"}
	candidate.Routes = []ServiceEndpoint{{Name: candidate.Names[0], Port: 8080}, {Name: candidate.Names[1], Port: 9000}}
	status, err := controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, false)
	if err != nil || len(status) != 2 || status[0].State != "active" || status[1].State != "probe-failed" || len(controller.ActiveRoutes()) != 1 || backend.acquires != 1 || backend.targets != 1 {
		t.Fatalf("listeners did not share identity/isolate failure: %+v %v", status, err)
	}
	candidate.Routes[0].Port = 9001
	candidate.SelectedPort = 9001
	status, err = controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, false)
	if err != nil || len(controller.ActiveRoutes()) != 0 || backend.releases != 1 {
		t.Fatalf("stale port remained active: %+v %v", status, err)
	}
}

func TestRoutingTLSBackendVerifiesTrustAndExactName(t *testing.T) {
	authority, now := authorityFixture(t)
	spec := routeFixture()
	spec.HTTPS = true
	leaf, err := authority.Leaf(spec.Name, now)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{leaf}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()
	entry := newServiceRoute(spec, func(ctx context.Context, _ ServiceRoute) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}, make(chan struct{}, 32))
	defer entry.close()
	request, _ := http.NewRequest("HEAD", "https://"+spec.Name+"/", nil)
	if _, err := entry.transport.RoundTrip(request); err == nil {
		t.Fatal("untrusted TLS backend accepted")
	}
	if entry.transport.TLSClientConfig.InsecureSkipVerify || entry.transport.TLSClientConfig.ServerName != spec.Name {
		t.Fatal("TLS identity verification disabled")
	}
	roots := x509.NewCertPool()
	roots.AddCert(authority.root)
	entry.transport.TLSClientConfig.RootCAs = roots
	response, err := entry.transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	entry.transport.CloseIdleConnections()
	entry.transport.TLSClientConfig.ServerName = "different.orbwin.test"
	if _, err := entry.transport.RoundTrip(request); err == nil {
		t.Fatal("wrong TLS backend name accepted")
	}
}

func TestRoutingForwardedHeadersCannotSupplyAdditionalIdentity(t *testing.T) {
	received := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received <- r.Header.Clone(); w.WriteHeader(204) }))
	defer server.Close()
	routes, _ := NewRoutes(func(ctx context.Context, _ ServiceRoute) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	})
	defer routes.Close()
	spec := routeFixture()
	if err := routes.Replace([]ServiceRoute{spec}); err != nil {
		t.Fatal(err)
	}
	request := routeRequest("GET", "/", spec.Name, spec.Name)
	for _, key := range []string{"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Forwarded-Port", "X-Forwarded-Prefix", "X-Real-IP"} {
		request.Header.Set(key, "attacker")
	}
	response := httptest.NewRecorder()
	routes.Handler(true).ServeHTTP(response, request)
	if response.Code != 204 {
		t.Fatal(response.Code)
	}
	headers := <-received
	for key, values := range headers {
		if strings.Contains(strings.Join(values, ","), "attacker") {
			t.Fatalf("spoofed header %s survived", key)
		}
	}
	if headers.Get("X-Forwarded-Host") != spec.Name || headers.Get("X-Forwarded-Proto") != "https" || headers.Get("Forwarded") != "" || headers.Get("X-Real-IP") != "" {
		t.Fatalf("forwarding contract differs: %v", headers)
	}
}
