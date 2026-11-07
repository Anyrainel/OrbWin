package localnet

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"
)

type NamespaceBackend interface {
	Target(context.Context, string) (string, error)
	Acquire(context.Context, string, string) (NamespaceHandle, error)
	Renew(context.Context, NamespaceHandle) error
	Release(context.Context, NamespaceHandle) error
	Dial(context.Context, ServiceRoute) (net.Conn, error)
	Close(context.Context) error
}
type ServiceStatus struct {
	ContainerID string   `json:"containerId"`
	Names       []string `json:"names"`
	Port        uint16   `json:"port,omitempty"`
	State       string   `json:"state"`
	Reason      string   `json:"reason,omitempty"`
	RetryAt     string   `json:"retryAt,omitempty"`
}
type routeAttempt struct {
	failures int
	next     time.Time
	reason   string
}

// The permanent process supplies complete reviewed discovery snapshots. This
// component never starts/stops workloads or rdd. Reconcile retires stale routes
// before helper removal, renews independent helpers concurrently, probes only
// selected HTTP ports, then publishes verified routes. Startup failures have at
// most three automatic attempts per incarnation/port; explicit retry resets them.
// Close cancels an in-flight reconcile and retires streams before backend Close.
type ServiceController struct {
	operation sync.Mutex
	backend   NamespaceBackend
	routes    *Routes
	handles   map[string]NamespaceHandle
	snapshot  []ServiceRoute
	attempts  map[string]routeAttempt
	ctx       context.Context
	cancel    context.CancelFunc
	closed    bool
	now       func() time.Time
}

func NewServiceController(backend NamespaceBackend) (*ServiceController, error) {
	return NewServiceControllerWithPorts(backend, DefaultServicePorts())
}
func NewServiceControllerWithPorts(backend NamespaceBackend, ports ServicePorts) (*ServiceController, error) {
	if backend == nil {
		return nil, errors.New("owned namespace backend required")
	}
	routes, err := NewRoutesWithPorts(backend.Dial, ports)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &ServiceController{backend: backend, routes: routes, handles: make(map[string]NamespaceHandle), attempts: make(map[string]routeAttempt), ctx: ctx, cancel: cancel, now: time.Now}, nil
}
func (controller *ServiceController) Handler(secure bool) http.Handler {
	return controller.routes.Handler(secure)
}
func (controller *ServiceController) TLSConfig(authority *Authority) (*tls.Config, error) {
	return controller.routes.TLSConfig(authority)
}
func routeTargetKey(id, started string) string { return id + "\x00" + started }
func routeAttemptKey(id, started string, port uint16) string {
	return routeTargetKey(id, started) + "\x00" + strconv.Itoa(int(port))
}

func validateServiceSelections(candidates []ServiceCandidate) error {
	if len(candidates) > 512 {
		return errors.New("service selection exceeds 512 candidates")
	}
	ids, names, targets := make(map[string]bool), make(map[string]bool), make(map[string]bool)
	for _, candidate := range candidates {
		if !fullID(candidate.ContainerID) || ids[candidate.ContainerID] {
			return errors.New("service selection requires distinct full target IDs")
		}
		ids[candidate.ContainerID] = true
		switch candidate.State {
		case "routing-error", "probe-required", "needs-port", "needs-name", "metadata-conflict", "disabled", "stopped", "paused", "restarting", "unsupported-network", "name-conflict":
		default:
			return errors.New("unsupported discovery state; active status is assigned only after verification")
		}
		if candidate.State != "probe-required" {
			continue
		}
		if len(candidate.Routes) > 0 {
			if err := validateEndpoints(candidate.Routes); err != nil {
				return err
			}
			if candidate.SelectedPort != candidate.Routes[0].Port {
				return errors.New("selected port differs from the first exact route")
			}
			if len(candidate.Names) != len(candidate.Routes) {
				return errors.New("route names differ from candidate names")
			}
			for index, route := range candidate.Routes {
				if route.Name != candidate.Names[index] {
					return errors.New("route names differ from candidate names")
				}
			}
		}
		if candidate.SelectedPort == 0 || len(candidate.Names) == 0 {
			return errors.New("service selection requires a reviewed name and TCP port")
		}
		targets[candidate.ContainerID] = true
		for _, name := range candidate.Names {
			canonical, err := CanonicalServiceName(name)
			if err != nil || canonical != name || names[name] {
				return errors.New("selected service names are invalid or conflicting")
			}
			names[name] = true
		}
	}
	if len(names) > 256 || len(targets) > 16 {
		return errors.New("review a selection of at most sixteen targets and 256 names")
	}
	return nil
}
func (controller *ServiceController) Reconcile(ctx context.Context, candidates []ServiceCandidate, retry bool) ([]ServiceStatus, error) {
	if err := validateServiceSelections(candidates); err != nil {
		return nil, err
	}
	candidates = expandServiceListeners(candidates)
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	stop := context.AfterFunc(controller.ctx, cancel)
	defer stop()
	controller.operation.Lock()
	defer controller.operation.Unlock()
	if controller.closed {
		return nil, errors.New("local service controller is closed")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	statuses := make([]ServiceStatus, len(candidates))
	started := make(map[string]string)
	errorsByID := make(map[string]error)
	var inspection sync.WaitGroup
	type targetResult struct {
		id, started string
		err         error
	}
	results := make(chan targetResult, 16)
	slots := make(chan struct{}, 4)
	inspected := make(map[string]bool)
	for index, candidate := range candidates {
		statuses[index] = ServiceStatus{ContainerID: candidate.ContainerID, Names: append([]string{}, candidate.Names...), Port: candidate.SelectedPort, State: candidate.State, Reason: candidate.Reason}
		if candidate.State != "probe-required" {
			continue
		}
		if inspected[candidate.ContainerID] {
			continue
		}
		inspected[candidate.ContainerID] = true
		inspection.Add(1)
		go func(id string) {
			defer inspection.Done()
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				results <- targetResult{id: id, err: ctx.Err()}
				return
			}
			defer func() { <-slots }()
			stamp, err := controller.backend.Target(ctx, id)
			results <- targetResult{id: id, started: stamp, err: err}
		}(candidate.ContainerID)
	}
	inspection.Wait()
	close(results)
	for result := range results {
		if result.err != nil {
			errorsByID[result.id] = result.err
		} else {
			stamp, err := time.Parse(time.RFC3339Nano, result.started)
			if err != nil || stamp.IsZero() {
				errorsByID[result.id] = errors.New("target returned an invalid incarnation")
			} else {
				started[result.id] = result.started
			}
		}
	}
	if ctx.Err() != nil {
		return statuses, ctx.Err()
	}
	wanted := make(map[string]bool)
	selected := make(map[string]ServiceRoute)
	attemptKeys := make(map[string]bool)
	for _, candidate := range candidates {
		stamp, ok := started[candidate.ContainerID]
		if !ok {
			continue
		}
		wanted[routeTargetKey(candidate.ContainerID, stamp)] = true
		attemptKeys[candidateAttemptKey(candidate, stamp)] = true
		for _, name := range candidate.Names {
			selected[name] = ServiceRoute{Name: name, ContainerID: candidate.ContainerID, StartedAt: stamp, Port: candidate.SelectedPort, HTTPS: candidateHTTPS(candidate)}
		}
	}
	for key := range controller.attempts {
		if retry || !attemptKeys[key] {
			delete(controller.attempts, key)
		}
	}
	// Retire removed/renamed/restarted/changed-port routes before cleanup or
	// probing. Matching routes retain their existing transport and upgrades.
	keep := make([]ServiceRoute, 0, len(controller.snapshot))
	for _, route := range controller.snapshot {
		if choice, ok := selected[route.Name]; ok && choice.ContainerID == route.ContainerID && choice.StartedAt == route.StartedAt && choice.Port == route.Port && choice.HTTPS == route.HTTPS {
			keep = append(keep, route)
		}
	}
	if err := controller.routes.Replace(keep); err != nil {
		return statuses, err
	}
	controller.snapshot = keep
	var failures []error
	for key, handle := range controller.handles {
		if !wanted[key] {
			if err := controller.backend.Release(ctx, handle); err != nil {
				failures = append(failures, err)
			} else {
				delete(controller.handles, key)
			}
		}
	}

	// A slow/unavailable namespace must not serialize all healthy renewals.
	type renewalResult struct {
		key string
		err error
	}
	renewals := make(chan renewalResult, 16)
	var renewing sync.WaitGroup
	for key, handle := range controller.handles {
		if !wanted[key] {
			continue
		}
		renewing.Add(1)
		go func(key string, handle NamespaceHandle) {
			defer renewing.Done()
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				renewals <- renewalResult{key, ctx.Err()}
				return
			}
			defer func() { <-slots }()
			renewals <- renewalResult{key, controller.backend.Renew(ctx, handle)}
		}(key, handle)
	}
	renewing.Wait()
	close(renewals)
	renewed := make(map[string]bool)
	for result := range renewals {
		if result.err != nil {
			handle := controller.handles[result.key]
			errorsByID[handle.Intent.ContainerID] = result.err
		} else {
			renewed[result.key] = true
		}
	}
	renewedKeep := keep[:0]
	for _, route := range keep {
		if errorsByID[route.ContainerID] == nil {
			renewedKeep = append(renewedKeep, route)
		}
	}
	keep = renewedKeep
	if err := controller.routes.Replace(keep); err != nil {
		return statuses, err
	}
	controller.snapshot = keep
	ready := make(map[string]bool)
	next := make([]ServiceRoute, 0, len(keep))
	for index, candidate := range candidates {
		if candidate.State != "probe-required" {
			continue
		}
		if err := errorsByID[candidate.ContainerID]; err != nil {
			if stamp, known := started[candidate.ContainerID]; known {
				controller.failAttempt(&statuses[index], candidateAttemptKey(candidate, stamp), err)
			} else {
				statuses[index].State = "unavailable"
				statuses[index].Reason = err.Error()
			}
			continue
		}
		stamp := started[candidate.ContainerID]
		key := routeTargetKey(candidate.ContainerID, stamp)
		attemptKey := candidateAttemptKey(candidate, stamp)
		if renewed[key] {
			delete(controller.attempts, attemptKey)
		}
		attempt := controller.attempts[attemptKey]
		if attempt.reason != "" && (attempt.failures >= 3 || controller.now().Before(attempt.next)) {
			statuses[index].State = "probe-failed"
			statuses[index].Reason = attempt.reason
			if attempt.failures < 3 {
				statuses[index].RetryAt = attempt.next.UTC().Format(time.RFC3339Nano)
			}
			continue
		}
		handle, exists := controller.handles[key]
		if !exists {
			if ctx.Err() != nil {
				statuses[index].State = "cancelled"
				statuses[index].Reason = "Service reconciliation was cancelled."
				continue
			}
			if len(controller.handles) >= 16 {
				statuses[index].State = "capacity-unavailable"
				statuses[index].Reason = "Retained helper cleanup must complete before another target is enabled."
				continue
			}
			var err error
			handle, err = controller.backend.Acquire(ctx, candidate.ContainerID, stamp)
			if err != nil {
				controller.failAttempt(&statuses[index], attemptKey, err)
				continue
			}
			if handle.Intent.ContainerID != candidate.ContainerID || handle.Intent.StartedAt != stamp || !fullID(handle.ID) {
				failures = append(failures, errors.New("namespace backend returned an unexpected target/helper identity"))
				statuses[index].State = "unavailable"
				statuses[index].Reason = "Namespace helper identity differs from the selected target."
				continue
			}
			controller.handles[key] = handle
		}
		route := ServiceRoute{Name: candidate.Names[0], ContainerID: candidate.ContainerID, StartedAt: stamp, HelperID: handle.ID, Port: candidate.SelectedPort, HTTPS: candidateHTTPS(candidate)}
		wasReady := false
		for _, old := range keep {
			if old.ContainerID == route.ContainerID && old.StartedAt == route.StartedAt && old.HelperID == route.HelperID && old.Port == route.Port && old.HTTPS == route.HTTPS && (!route.HTTPS || old.Name == route.Name) {
				wasReady = true
				break
			}
		}
		if !wasReady {
			if ctx.Err() != nil {
				statuses[index].State = "cancelled"
				statuses[index].Reason = "Service reconciliation was cancelled."
				continue
			}
			if err := controller.probeHTTP(ctx, route); err != nil {
				controller.failAttempt(&statuses[index], attemptKey, err)
				continue
			}
		}
		// A successful probe cannot reset repeated renewal failures: require a
		// healthy heartbeat from this helper before replenishing its retry budget.
		if attempt.failures > 0 {
			attempt.reason = ""
			attempt.next = time.Time{}
			controller.attempts[attemptKey] = attempt
		}
		ready[key] = true
		for _, name := range candidate.Names {
			copy := route
			copy.Name = name
			next = append(next, copy)
		}
		statuses[index].State = "active"
		statuses[index].Reason = "HTTP route is active; application health is reported by the workload."
	}
	if err := controller.routes.Replace(next); err != nil {
		failures = append(failures, err)
	} else {
		controller.snapshot = next
	}
	// Failed probes and renewal failures cannot retain idle helper processes.
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cleanupCancel()
	for key, handle := range controller.handles {
		if !ready[key] {
			if err := controller.backend.Release(cleanupCtx, handle); err != nil {
				failures = append(failures, err)
			} else {
				delete(controller.handles, key)
			}
		}
	}
	if ctx.Err() != nil {
		failures = append(failures, ctx.Err())
	}
	return statuses, errors.Join(failures...)
}
func (controller *ServiceController) failAttempt(status *ServiceStatus, key string, err error) {
	attempt := controller.attempts[key]
	attempt.failures++
	attempt.reason = err.Error()
	status.State = "probe-failed"
	status.Reason = attempt.reason
	if attempt.failures < 3 {
		delay := time.Second
		if attempt.failures == 2 {
			delay = 3 * time.Second
		}
		attempt.next = controller.now().Add(delay)
		status.RetryAt = attempt.next.UTC().Format(time.RFC3339Nano)
	}
	controller.attempts[key] = attempt
}
func (controller *ServiceController) probeHTTP(ctx context.Context, route ServiceRoute) error {
	probe := newServiceRoute(route, controller.routes.dial, controller.routes.slots)
	defer probe.close()
	probe.transport.DisableKeepAlives = true
	probe.transport.MaxResponseHeaderBytes = 16 << 10
	probe.transport.ResponseHeaderTimeout = 2 * time.Second
	client := &http.Client{Transport: probe.transport, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, routeScheme(route)+"://"+route.Name+"/", nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("HTTP listener probe failed: %w", err)
	}
	response.Body.Close()
	return nil
}

// Call when inventory/daemon status is unavailable. Existing links must not
// remain presented as reachable. Retained helpers will expire without renewal;
// the next successful reconcile can verify or recover them.
func (controller *ServiceController) Invalidate() error {
	controller.operation.Lock()
	defer controller.operation.Unlock()
	controller.snapshot = nil
	return controller.routes.Replace(nil)
}
func (controller *ServiceController) ActiveRoutes() []ServiceRoute {
	controller.operation.Lock()
	defer controller.operation.Unlock()
	snapshot := append([]ServiceRoute{}, controller.snapshot...)
	sort.Slice(snapshot, func(i, j int) bool { return snapshot[i].Name < snapshot[j].Name })
	return snapshot
}
func (controller *ServiceController) Close(ctx context.Context) error {
	controller.cancel()
	controller.operation.Lock()
	defer controller.operation.Unlock()
	if controller.closed {
		return nil
	}
	controller.closed = true
	controller.snapshot = nil
	return errors.Join(controller.routes.Close(), controller.backend.Close(ctx))
}
