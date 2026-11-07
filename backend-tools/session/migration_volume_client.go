package main

import (
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/moby/moby/client"
)

// This maintenance scope never exposes a client/endpoint to the renderer. Only
// acknowledged job-owned resources gain mutation permissions. Request hooks also
// cover SDK hijack requests, which bypass the ordinary HTTP round tripper.
type migrationVolumeScope struct {
	mu                                     sync.Mutex
	token, asset, helper, image, container string
	importing, creating                    bool
	execs                                  map[string]bool
}

func migrationAPIPath(path string) string {
	if strings.HasPrefix(path, "/v") {
		parts := strings.SplitN(path[2:], "/", 2)
		if len(parts) == 2 && parts[0] != "" && strings.Trim(parts[0], "0123456789.") == "" {
			return "/" + parts[1]
		}
	}
	return path
}
func fullContainerID(id string) bool {
	return len(id) == 64 && strings.Trim(id, "0123456789abcdef") == ""
}
func (scope *migrationVolumeScope) acceptContainer(id string) error {
	if !fullContainerID(id) {
		return errors.New("maintenance helper returned an invalid ID")
	}
	scope.mu.Lock()
	defer scope.mu.Unlock()
	scope.container = id
	return nil
}
func (scope *migrationVolumeScope) acceptExec(id string) error {
	if !fullContainerID(id) {
		return errors.New("maintenance exec returned an invalid ID")
	}
	scope.mu.Lock()
	defer scope.mu.Unlock()
	if scope.execs == nil {
		scope.execs = map[string]bool{}
	}
	if len(scope.execs) >= 4 {
		return errors.New("maintenance exec budget exceeded")
	}
	scope.execs[id] = true
	return nil
}
func (scope *migrationVolumeScope) allow(request *http.Request) error {
	scope.mu.Lock()
	defer scope.mu.Unlock()
	path := migrationAPIPath(request.URL.Path)
	read := request.Method == http.MethodGet || request.Method == http.MethodHead
	if read && migrationReadPath(request.URL.Path) {
		return nil
	}
	if read && strings.HasPrefix(path, "/volumes/") && resourceFileArguments("volume", strings.TrimPrefix(path, "/volumes/")) == nil {
		return nil
	}
	if read && strings.HasPrefix(path, "/containers/") && strings.HasSuffix(path, "/json") {
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/containers/"), "/json")
		if fullContainerID(id) || id == scope.helper {
			return nil
		}
	}
	if scope.container != "" {
		prefix := "/containers/" + scope.container
		if read && path == prefix+"/archive" {
			return nil
		}
		if request.Method == http.MethodPost && (path == prefix+"/start" || path == prefix+"/exec") {
			return nil
		}
		if request.Method == http.MethodDelete && path == prefix && request.URL.Query().Get("v") == "" {
			return nil
		}
	}
	for id := range scope.execs {
		if (read && path == "/exec/"+id+"/json") || (request.Method == http.MethodPost && path == "/exec/"+id+"/start") {
			return nil
		}
	}
	if scope.importing && request.Method == http.MethodPost && path == "/images/create" {
		query := request.URL.Query()
		if query.Get("fromSrc") == "-" && query.Get("repo") == "" && query.Get("tag") == "" {
			changes := query["changes"]
			expected := []string{"LABEL dev.orbwin.component=file-maintenance-tools", "LABEL dev.orbwin.source-sha256=" + scope.asset, "LABEL dev.orbwin.import-token=" + scope.token}
			if len(changes) == len(expected) {
				match := true
				for i := range changes {
					if changes[i] != expected[i] {
						match = false
					}
				}
				if match {
					return nil
				}
			}
		}
	}
	if scope.creating && request.Method == http.MethodPost && path == "/containers/create" && request.URL.Query().Get("name") == scope.helper {
		return nil
	}
	if scope.image != "" && request.Method == http.MethodDelete && path == "/images/"+scope.image && request.URL.Query().Get("force") == "" && request.URL.Query().Get("noprune") == "1" {
		return nil
	}
	return errors.New("source maintenance request is outside this job's exact helper scope")
}

type migrationVolumeTransport struct {
	base  http.RoundTripper
	scope *migrationVolumeScope
}

func (transport migrationVolumeTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := transport.scope.allow(request); err != nil {
		return nil, err
	}
	response, err := transport.base.RoundTrip(request)
	if err != nil {
		return response, err
	}
	if strings.HasSuffix(request.URL.Path, "/archive") && response.StatusCode == http.StatusOK {
		return response, nil
	}
	return migrationMetadataResponse(response)
}
func newDesktopVolumeMigrationClient(scope *migrationVolumeScope) (*client.Client, func(), error) {
	return desktopMigrationClient(func(base http.RoundTripper) http.RoundTripper { return migrationVolumeTransport{base, scope} }, scope.allow)
}
