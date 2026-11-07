package main

// Migration sources have their own read-only transport. They never inherit
// DOCKER_HOST/context/config, and cannot send a mutating API request. The source
// owns its data; closing this client has no engine lifecycle consequence.
import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/docker/go-connections/sockets"
	"github.com/moby/moby/client"
)

const desktopMigrationEndpoint = "npipe:////./pipe/dockerDesktopLinuxEngine"
const migrationMetadataLimit = 16 << 20

type migrationTransport struct{ base http.RoundTripper }

func migrationReadPath(path string) bool {
	// SDK negotiation uses an unversioned ping; every other request is versioned.
	if path == "/_ping" {
		return true
	}
	if strings.HasPrefix(path, "/v") {
		parts := strings.SplitN(path[2:], "/", 2)
		if len(parts) != 2 || parts[0] == "" || strings.Trim(parts[0], "0123456789.") != "" {
			return false
		}
		path = "/" + parts[1]
	}
	switch path {
	case "/info", "/version", "/images/json", "/volumes", "/containers/json", "/images/get":
		return true
	}
	return strings.HasPrefix(path, "/images/") && strings.HasSuffix(path, "/json") && !strings.Contains(path, "..")
}

// Buffer only bounded metadata before SDK decoding, so an oversized JSON field
// cannot allocate unbounded memory. Archive streams remain streamed and are
// bounded/validated by the existing image archive writer. Own and close the real
// response body on every metadata path, including failure.
func (t migrationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := migrationReadRequest(request); err != nil {
		return nil, err
	}
	response, err := t.base.RoundTrip(request)
	if err != nil {
		return response, err
	}
	if strings.HasSuffix(request.URL.Path, "/images/get") && response.StatusCode == http.StatusOK {
		return response, nil
	}
	return migrationMetadataResponse(response)
}

func migrationReadRequest(request *http.Request) error {
	if (request.Method != http.MethodGet && request.Method != http.MethodHead) || !migrationReadPath(request.URL.Path) {
		return errors.New("migration source permits only reviewed read-only Docker API operations")
	}
	return nil
}

func migrationMetadataResponse(response *http.Response) (*http.Response, error) {
	data, readErr := io.ReadAll(io.LimitReader(response.Body, migrationMetadataLimit+1))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		return nil, errors.Join(readErr, closeErr)
	}
	if len(data) > migrationMetadataLimit {
		return nil, errors.New("migration source metadata exceeds its 16-MiB response limit; inventory was not truncated")
	}
	response.Body = io.NopCloser(bytes.NewReader(data))
	return response, nil
}

func newDesktopMigrationClient() (*client.Client, func(), error) {
	return desktopMigrationClient(func(base http.RoundTripper) http.RoundTripper { return migrationTransport{base} }, migrationReadRequest)
}

func desktopMigrationClient(wrap func(http.RoundTripper) http.RoundTripper, hook client.RequestHook) (*client.Client, func(), error) {
	parsed, err := client.ParseHostURL(desktopMigrationEndpoint)
	if err != nil {
		return nil, nil, err
	}
	base := &http.Transport{MaxIdleConns: 2, MaxConnsPerHost: 2, IdleConnTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second, MaxResponseHeaderBytes: 64 << 10}
	if err := sockets.ConfigureTransport(base, parsed.Scheme, parsed.Host); err != nil {
		return nil, nil, err
	}
	httpClient := &http.Client{Transport: wrap(base), CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("migration source redirects are refused")
	}}
	cli, err := client.New(client.WithHost(desktopMigrationEndpoint), client.WithHTTPClient(httpClient), client.WithAPIVersionNegotiation(), client.WithHTTPRequestHook(hook))
	if err != nil {
		base.CloseIdleConnections()
		return nil, nil, err
	}
	return cli, func() { cli.Close(); base.CloseIdleConnections() }, nil
}

type migrationSourceIdentity struct {
	Kind          string                `json:"kind"`
	Daemon        string                `json:"daemon"`
	EngineVersion string                `json:"engineVersion"`
	APIVersion    string                `json:"apiVersion"`
	Product       string                `json:"product"`
	WSL           *migrationWSLIdentity `json:"wsl,omitempty"`
}
type migrationWSLIdentity struct {
	Registration string `json:"registration"`
	Name         string `json:"name"`
	Machine      string `json:"machine"`
}

func verifyDesktopMigrationSource(ctx context.Context, cli *client.Client, target, expected string) (migrationSourceIdentity, error) {
	return verifyMigrationSource(ctx, cli, target, expected, "docker-desktop", "Docker Desktop", "29.8.0")
}
func verifyRancherMigrationSource(ctx context.Context, cli *client.Client, target, expected string) (migrationSourceIdentity, error) {
	return verifyMigrationSource(ctx, cli, target, expected, "rancher-desktop", "Rancher Desktop WSL Distribution", "29.5.3")
}
func verifyMigrationSource(ctx context.Context, cli *client.Client, target, expected, kind, operatingSystem, engineVersion string) (migrationSourceIdentity, error) {
	var result migrationSourceIdentity
	info, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		return result, err
	}
	if target == "" || info.Info.ID == "" || info.Info.ID == target || (expected != "" && info.Info.ID != expected) || info.Info.OSType != "linux" || info.Info.OperatingSystem != operatingSystem {
		return result, errors.New("migration source must be the distinct reviewed source-product Linux daemon")
	}
	version, err := cli.ServerVersion(ctx, client.ServerVersionOptions{})
	if err != nil {
		return result, err
	}
	// Deliberately narrow initial qualification matrix. A different version needs
	// its own adapter qualification, never a best-effort mutation of source data.
	if version.Version != engineVersion || version.Os != "linux" || version.Arch != "amd64" {
		return result, fmt.Errorf("%s migration adapter requires qualified Linux/amd64 Engine %s; observed %s/%s %s", kind, engineVersion, version.Os, version.Arch, version.Version)
	}
	return migrationSourceIdentity{Kind: kind, Daemon: info.Info.ID, EngineVersion: version.Version, APIVersion: version.APIVersion, Product: version.Platform.Name}, nil
}
