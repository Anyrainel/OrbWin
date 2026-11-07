package localnet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type controllerBackend struct {
	mu                                       sync.Mutex
	started, address                         string
	targetErr, acquireErr, renewErr, dialErr error
	targets, acquires, renews, releases      int
	closed                                   bool
	beforeRelease, beforeClose               func()
	targetEntered                            chan struct{}
	renewErrors                              map[string]error
}

func (backend *controllerBackend) Target(ctx context.Context, id string) (string, error) {
	backend.mu.Lock()
	backend.targets++
	stamp, err, entered := backend.started, backend.targetErr, backend.targetEntered
	backend.mu.Unlock()
	if entered != nil {
		close(entered)
		<-ctx.Done()
		return "", ctx.Err()
	}
	return stamp, err
}
func (backend *controllerBackend) Acquire(ctx context.Context, id, stamp string) (NamespaceHandle, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.acquires++
	if backend.acquireErr != nil {
		return NamespaceHandle{}, backend.acquireErr
	}
	intent := namespaceFixture()
	intent.ContainerID = id
	intent.StartedAt = stamp
	intent.Session = fmt.Sprintf("%032x", backend.acquires)
	return NamespaceHandle{Intent: intent, ID: fmt.Sprintf("%064x", 100+backend.acquires)}, nil
}
func (backend *controllerBackend) Renew(ctx context.Context, handle NamespaceHandle) error {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.renews++
	if err := backend.renewErrors[handle.Intent.ContainerID]; err != nil {
		return err
	}
	return backend.renewErr
}
func (backend *controllerBackend) Release(ctx context.Context, handle NamespaceHandle) error {
	backend.mu.Lock()
	backend.releases++
	callback := backend.beforeRelease
	backend.mu.Unlock()
	if callback != nil {
		callback()
	}
	return nil
}
func (backend *controllerBackend) Dial(ctx context.Context, route ServiceRoute) (net.Conn, error) {
	backend.mu.Lock()
	address, err := backend.address, backend.dialErr
	backend.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return (&net.Dialer{}).DialContext(ctx, "tcp", address)
}
func (backend *controllerBackend) Close(ctx context.Context) error {
	backend.mu.Lock()
	backend.closed = true
	callback := backend.beforeClose
	backend.mu.Unlock()
	if callback != nil {
		callback()
	}
	return nil
}
func controllerCandidate() ServiceCandidate {
	return ServiceCandidate{ContainerID: strings.Repeat("d", 64), ContainerName: "app", Names: []string{"app.orbwin.localhost", "app.orbwin.test"}, Ports: []uint16{8080}, SelectedPort: 8080, State: "probe-required"}
}
func controllerFixture(t *testing.T) (*ServiceController, *controllerBackend, *atomic.Int32) {
	t.Helper()
	requests := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodHead {
			t.Error("discovery probe used a mutating/body-bearing request")
		}
		w.Header().Set("Location", "http://unrelated.invalid/never-follow")
		w.WriteHeader(503)
	}))
	t.Cleanup(server.Close)
	backend := &controllerBackend{started: "2026-10-24T12:00:00Z", address: server.Listener.Addr().String()}
	controller, err := NewServiceController(backend)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = controller.Close(context.Background()) })
	return controller, backend, requests
}
func assertRouteMissing(t *testing.T, controller *ServiceController, name string) {
	t.Helper()
	response := httptest.NewRecorder()
	controller.Handler(true).ServeHTTP(response, routeRequest("GET", "/", name, name))
	if response.Code != 404 {
		t.Errorf("stale route was available during teardown: %d", response.Code)
	}
}

func TestControllerPublishesOnlyProbedRoutesAndReusesUnchangedIncarnation(t *testing.T) {
	controller, backend, requests := controllerFixture(t)
	candidate := controllerCandidate()
	status, err := controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, false)
	if err != nil || len(status) != 1 || status[0].State != "active" || len(controller.ActiveRoutes()) != 2 {
		t.Fatalf("verified HTTP listener was not routed: %+v %v", status, err)
	}
	if requests.Load() != 1 {
		t.Fatal("probe followed an application redirect or repeated requests")
	}
	status, err = controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, false)
	if err != nil || status[0].State != "active" || requests.Load() != 1 {
		t.Fatal("unchanged discovery repeated the listener probe")
	}
	backend.mu.Lock()
	acquires, renews := backend.acquires, backend.renews
	backend.mu.Unlock()
	if acquires != 1 || renews != 1 {
		t.Fatal("unchanged target replaced its helper instead of renewing")
	}
	backend.mu.Lock()
	backend.beforeRelease = func() { assertRouteMissing(t, controller, candidate.Names[0]) }
	backend.started = "2026-10-24T13:00:00Z"
	backend.mu.Unlock()
	status, err = controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, false)
	if err != nil || status[0].State != "active" || requests.Load() != 2 {
		t.Fatal("restarted target was not independently reprobed")
	}
	backend.mu.Lock()
	acquires, releases := backend.acquires, backend.releases
	backend.beforeRelease = nil
	backend.mu.Unlock()
	if acquires != 2 || releases != 1 || controller.ActiveRoutes()[0].StartedAt != "2026-10-24T13:00:00Z" {
		t.Fatal("target replacement reused a stale helper/incarnation")
	}
}
func TestControllerRenameDisableAndUnavailableRetireBeforeHelperRemoval(t *testing.T) {
	controller, backend, _ := controllerFixture(t)
	candidate := controllerCandidate()
	if _, err := controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, false); err != nil {
		t.Fatal(err)
	}
	old := candidate.Names[0]
	candidate.Names = []string{"renamed.orbwin.localhost", "renamed.orbwin.test"}
	if _, err := controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, false); err != nil {
		t.Fatal(err)
	}
	assertRouteMissing(t, controller, old)
	backend.mu.Lock()
	backend.beforeRelease = func() { assertRouteMissing(t, controller, candidate.Names[0]) }
	backend.targetErr = errors.New("target stopped")
	backend.mu.Unlock()
	status, err := controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, false)
	if err != nil || status[0].State != "unavailable" || len(controller.ActiveRoutes()) != 0 {
		t.Fatal("stopped target retained active links")
	}
	backend.mu.Lock()
	backend.beforeRelease = nil
	backend.targetErr = nil
	backend.mu.Unlock()
	if _, err := controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, false); err != nil {
		t.Fatal(err)
	}
	backend.mu.Lock()
	backend.beforeRelease = func() { assertRouteMissing(t, controller, candidate.Names[0]) }
	backend.mu.Unlock()
	candidate.State = "disabled"
	status, err = controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, false)
	if err != nil || status[0].State != "disabled" || len(controller.ActiveRoutes()) != 0 {
		t.Fatal("disabled integration retained routes")
	}
}
func TestControllerPortChangeReprobesWithoutReplacingHelper(t *testing.T) {
	controller, backend, requests := controllerFixture(t)
	candidate := controllerCandidate()
	_, _ = controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, false)
	candidate.SelectedPort = 9090
	status, err := controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, false)
	if err != nil || status[0].State != "active" || requests.Load() != 2 {
		t.Fatalf("changed port was not independently verified: %+v %v", status, err)
	}
	backend.mu.Lock()
	acquires, releases := backend.acquires, backend.releases
	backend.mu.Unlock()
	if acquires != 1 || releases != 0 || controller.ActiveRoutes()[0].Port != 9090 {
		t.Fatal("port selection recreated the namespace or retained its previous destination")
	}
}
func TestControllerBoundsAutomaticProbeRetriesAndHonorsExplicitRetry(t *testing.T) {
	controller, backend, _ := controllerFixture(t)
	clock := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	controller.now = func() time.Time { return clock }
	backend.mu.Lock()
	backend.dialErr = errors.New("listener not ready")
	backend.mu.Unlock()
	candidate := controllerCandidate()
	for index, advance := range []time.Duration{0, 0, 2 * time.Second, 4 * time.Second, 100 * time.Second} {
		clock = clock.Add(advance)
		status, err := controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, false)
		if err != nil || status[0].State != "probe-failed" || len(controller.ActiveRoutes()) != 0 {
			t.Fatalf("failed probe became active: %+v %v", status, err)
		}
		if index >= 3 && status[0].RetryAt != "" {
			t.Fatal("exhausted retry budget still promises another attempt")
		}
	}
	backend.mu.Lock()
	acquires, releases := backend.acquires, backend.releases
	backend.dialErr = nil
	backend.mu.Unlock()
	if acquires != 3 || releases != 3 {
		t.Fatalf("automatic helper/probe retries were unbounded or leaked: %d/%d", acquires, releases)
	}
	status, err := controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, true)
	if err != nil || status[0].State != "active" {
		t.Fatal("explicit retry could not recover a ready listener")
	}
}
func TestControllerRefusesInvalidSnapshotsAndPrecancelledCallsWithoutChangingRoutes(t *testing.T) {
	controller, backend, _ := controllerFixture(t)
	candidate := controllerCandidate()
	_, _ = controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, false)
	before := controller.ActiveRoutes()
	if _, err := controller.Reconcile(context.Background(), []ServiceCandidate{candidate, candidate}, false); err == nil {
		t.Fatal("duplicate source selection was accepted")
	}
	invalid := candidate
	invalid.State = "active"
	if _, err := controller.Reconcile(context.Background(), []ServiceCandidate{invalid}, false); err == nil {
		t.Fatal("caller-supplied active state bypassed verification")
	}
	invalid = candidate
	invalid.Names = []string{"evil.example"}
	if _, err := controller.Reconcile(context.Background(), []ServiceCandidate{invalid}, false); err == nil {
		t.Fatal("foreign name was accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := controller.Reconcile(ctx, []ServiceCandidate{candidate}, false); err == nil {
		t.Fatal("cancelled reconcile succeeded")
	}
	if len(controller.ActiveRoutes()) != len(before) {
		t.Fatal("invalid/cancelled request modified active routes")
	}
	backend.mu.Lock()
	acquires, releases := backend.acquires, backend.releases
	backend.mu.Unlock()
	if acquires != 1 || releases != 0 {
		t.Fatal("invalid snapshot changed helper resources")
	}
}
func TestControllerRenewalFailureRetiresRouteAndCloseOrdersTeardown(t *testing.T) {
	controller, backend, _ := controllerFixture(t)
	candidate := controllerCandidate()
	_, _ = controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, false)
	backend.mu.Lock()
	backend.renewErr = errors.New("lease failed")
	backend.beforeRelease = func() { assertRouteMissing(t, controller, candidate.Names[0]) }
	backend.mu.Unlock()
	status, err := controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, false)
	if err != nil || status[0].State != "probe-failed" || len(controller.ActiveRoutes()) != 0 {
		t.Fatal("failed lease kept an active route")
	}
	backend.mu.Lock()
	backend.renewErr = nil
	backend.beforeRelease = nil
	backend.mu.Unlock()
	_, _ = controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, true)
	backend.mu.Lock()
	backend.beforeClose = func() { assertRouteMissing(t, controller, candidate.Names[0]) }
	backend.mu.Unlock()
	if err := controller.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, false); err == nil {
		t.Fatal("closed controller was resurrected")
	}
}

func TestControllerOneFailedNamespaceDoesNotDropIndependentHealthyRoutes(t *testing.T) {
	controller, backend, requests := controllerFixture(t)
	first := controllerCandidate()
	second := controllerCandidate()
	second.ContainerID = strings.Repeat("f", 64)
	second.Names = []string{"second.orbwin.localhost", "second.orbwin.test"}
	status, err := controller.Reconcile(context.Background(), []ServiceCandidate{first, second}, false)
	if err != nil || status[0].State != "active" || status[1].State != "active" {
		t.Fatalf("initial independent routes failed: %+v %v", status, err)
	}
	backend.mu.Lock()
	backend.renewErrors = map[string]error{first.ContainerID: errors.New("first namespace unavailable")}
	backend.mu.Unlock()
	status, err = controller.Reconcile(context.Background(), []ServiceCandidate{first, second}, false)
	if err != nil || status[0].State != "probe-failed" || status[1].State != "active" {
		t.Fatalf("namespace failure spread to healthy routes: %+v %v", status, err)
	}
	active := controller.ActiveRoutes()
	if len(active) != 2 || active[0].ContainerID != second.ContainerID || active[1].ContainerID != second.ContainerID || requests.Load() != 2 {
		t.Fatal("failed renewal kept stale links or reprobed a healthy namespace")
	}
}
func TestControllerCloseCancelsAndJoinsBlockedTargetInspection(t *testing.T) {
	controller, backend, _ := controllerFixture(t)
	entered := make(chan struct{})
	backend.mu.Lock()
	backend.targetEntered = entered
	backend.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		_, err := controller.Reconcile(context.Background(), []ServiceCandidate{controllerCandidate()}, false)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("target inspection did not start")
	}
	if err := controller.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled reconcile reported success")
		}
	case <-time.After(time.Second):
		t.Fatal("close left a reconcile worker running")
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if !backend.closed || backend.acquires != 0 {
		t.Fatal("shutdown created a helper or failed to close its backend")
	}
}
func TestControllerProbeDoesNotFollowRedirectAndBoundsResponseHeaders(t *testing.T) {
	for _, oversized := range []bool{false, true} {
		t.Run(fmt.Sprint(oversized), func(t *testing.T) {
			controller, backend, _ := controllerFixture(t)
			var redirected atomic.Int32
			other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
			defer other.Close()
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if oversized {
					w.Header().Set("X-Large", strings.Repeat("x", 20<<10))
					w.WriteHeader(200)
				} else {
					http.Redirect(w, r, other.URL+"/must-not-probe", 302)
				}
			}))
			defer source.Close()
			backend.mu.Lock()
			backend.address = source.Listener.Addr().String()
			backend.mu.Unlock()
			status, err := controller.Reconcile(context.Background(), []ServiceCandidate{controllerCandidate()}, false)
			if err != nil {
				t.Fatal(err)
			}
			if oversized {
				if status[0].State != "probe-failed" || len(controller.ActiveRoutes()) != 0 {
					t.Fatal("oversized probe response was accepted")
				}
			} else if status[0].State != "active" {
				t.Fatalf("valid HTTP redirect was not recognized: %+v", status)
			}
			if redirected.Load() != 0 {
				t.Fatal("probe followed an application redirect to a different origin")
			}
		})
	}
}

func TestControllerRepeatedUnrenewableHelpersCannotChurnIndefinitely(t *testing.T) {
	controller, backend, _ := controllerFixture(t)
	clock := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	controller.now = func() time.Time { return clock }
	candidate := controllerCandidate()
	_, _ = controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, false)
	backend.mu.Lock()
	backend.renewErr = errors.New("every renewal fails")
	backend.mu.Unlock()
	for range 8 {
		clock = clock.Add(10 * time.Second)
		_, _ = controller.Reconcile(context.Background(), []ServiceCandidate{candidate}, false)
	}
	backend.mu.Lock()
	acquires := backend.acquires
	backend.mu.Unlock()
	if acquires != 3 || len(controller.ActiveRoutes()) != 0 {
		t.Fatalf("unrenewable helpers churned beyond the retry budget: %d", acquires)
	}
}
