package localnet

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestReviewedIntegrationPortsRedirectExternalAndContainerClientsIndependently(t *testing.T) {
	var calls atomic.Int32
	workload := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte("selected workload"))
	}))
	defer workload.Close()
	ports := ServicePorts{HTTP: 18080, HTTPS: 18443}
	routes, err := NewRoutesWithPorts(func(ctx context.Context, _ ServiceRoute) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", workload.Listener.Addr().String())
	}, ports)
	if err != nil {
		t.Fatal(err)
	}
	defer routes.Close()
	spec := routeFixture()
	if err = routes.Replace([]ServiceRoute{spec}); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct{ host, want string }{
		{spec.Name + ":18080", "https://" + spec.Name + ":18443/a%2Fb?q=1"},
		{spec.Name + ":80", "https://" + spec.Name + "/a%2Fb?q=1"},
		{spec.Name, "https://" + spec.Name + "/a%2Fb?q=1"},
	} {
		response := httptest.NewRecorder()
		routes.Handler(false).ServeHTTP(response, routeRequest("GET", "/a%2Fb?q=1", fixture.host, ""))
		if response.Code != 308 || response.Header().Get("Location") != fixture.want {
			t.Fatalf("%s redirected to %d %q", fixture.host, response.Code, response.Header().Get("Location"))
		}
	}
	if calls.Load() != 0 {
		t.Fatal("HTTP redirect dialled a workload")
	}
	for _, fixture := range []struct {
		host, sni string
		secure    bool
		want      int
	}{
		{spec.Name + ":18443", spec.Name, true, 200},
		{spec.Name + ":443", spec.Name, true, 200},
		{spec.Name, spec.Name, true, 200},
		{spec.Name + ":18080", spec.Name, true, 404},
		{spec.Name + ":18443", "", false, 404},
		{spec.Name + ":018443", spec.Name, true, 404},
		{spec.Name + ":18444", spec.Name, true, 404},
		{spec.Name + ":18443", "other.orbwin.localhost", true, 421},
	} {
		response := httptest.NewRecorder()
		routes.Handler(fixture.secure).ServeHTTP(response, routeRequest("GET", "/", fixture.host, fixture.sni))
		if response.Code != fixture.want {
			t.Fatalf("Host %q, SNI %q secure=%v got %d want %d", fixture.host, fixture.sni, fixture.secure, response.Code, fixture.want)
		}
	}
	if calls.Load() != 3 {
		t.Fatal("rejected host/SNI reached workload", calls.Load())
	}
}
func TestServicePortsRefuseAmbiguousOrInvalidPairs(t *testing.T) {
	for _, ports := range []ServicePorts{{}, {HTTP: 80}, {HTTPS: 443}, {HTTP: 8443, HTTPS: 8443}, {HTTP: 80, HTTPS: 8443}} {
		if err := ports.validate(); err == nil {
			t.Fatal("accepted ambiguous pair", ports)
		}
	}
	if err := DefaultServicePorts().validate(); err != nil {
		t.Fatal(err)
	}
}
