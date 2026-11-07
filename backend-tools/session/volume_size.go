package main

// Read-only volume accounting. Moby owns the scan; client cancellation closes
// its request, but cannot promise to stop filesystem work already in the daemon.
// No helper mount, container, receipt, pause, or volume write is permitted.
import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/docker/go-connections/sockets"
	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
)

const volumeSizeMetadataLimit = 4 << 20

type volumeSizeTransport struct {
	base http.RoundTripper
	name string
}

func (t volumeSizeTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	path := request.URL.Path
	if strings.HasPrefix(path, "/v") {
		parts := strings.SplitN(path[2:], "/", 2)
		if len(parts) != 2 || parts[0] == "" || strings.Trim(parts[0], "0123456789.") != "" {
			return nil, errors.New("invalid accounting API path")
		}
		path = "/" + parts[1]
	}
	if (request.Method != http.MethodGet && request.Method != http.MethodHead) || (path != "/_ping" && path != "/info" && path != "/system/df" && path != "/volumes/"+t.name) {
		return nil, errors.New("volume accounting permits only its fixed read-only API requests")
	}
	if path == "/system/df" && request.URL.RawQuery != "type=volume&verbose=1" {
		return nil, errors.New("volume accounting requires a volume-only scan")
	}
	response, err := t.base.RoundTrip(request)
	if err != nil {
		return response, err
	}
	data, readErr := io.ReadAll(io.LimitReader(response.Body, volumeSizeMetadataLimit+1))
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		return nil, errors.Join(readErr, closeErr)
	}
	if len(data) > volumeSizeMetadataLimit {
		return nil, errors.New("volume accounting metadata exceeds 4 MiB; no partial size was accepted")
	}
	response.Body = io.NopCloser(bytes.NewReader(data))
	return response, nil
}

func volumeSizeClient(name string) (*client.Client, func(), error) {
	parsed, err := client.ParseHostURL(endpoint)
	if err != nil {
		return nil, nil, err
	}
	base := &http.Transport{MaxIdleConns: 1, MaxConnsPerHost: 1, IdleConnTimeout: 5 * time.Second, ResponseHeaderTimeout: 20 * time.Second, MaxResponseHeaderBytes: 64 << 10}
	if err = sockets.ConfigureTransport(base, parsed.Scheme, parsed.Host); err != nil {
		return nil, nil, err
	}
	cli, err := client.New(client.WithHost(endpoint), client.WithHTTPClient(&http.Client{Transport: volumeSizeTransport{base, name}, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("volume accounting redirects are refused")
	}}), client.WithAPIVersionNegotiation())
	if err != nil {
		base.CloseIdleConnections()
		return nil, nil, err
	}
	return cli, func() { cli.Close(); base.CloseIdleConnections() }, nil
}

type volumeSizeResult struct {
	Name       string `json:"name"`
	CreatedAt  string `json:"createdAt"`
	Daemon     string `json:"daemon"`
	Bytes      int64  `json:"bytes"`
	References int64  `json:"references"`
	MeasuredAt string `json:"measuredAt"`
}

func inspectAccountingVolume(ctx context.Context, cli *client.Client, name string) (volume.Volume, error) {
	result, err := cli.VolumeInspect(ctx, name, client.VolumeInspectOptions{})
	if err != nil {
		return volume.Volume{}, err
	}
	value := result.Volume
	if value.Driver != "local" || value.Scope != "local" || len(value.Options) != 0 || value.ClusterVolume != nil {
		return volume.Volume{}, errors.New("size refresh supports ordinary local volumes without driver options")
	}
	return value, nil
}

func readVolumeSize(ctx context.Context, cli *client.Client, daemon, name, createdAt string) (volumeSizeResult, error) {
	var result volumeSizeResult
	verify := func() error {
		info, err := cli.Info(ctx, client.InfoOptions{})
		if err != nil {
			return err
		}
		if daemon == "" || info.Info.ID != daemon || info.Info.OSType != "linux" {
			return errors.New("volume accounting requires the recorded owned Linux daemon")
		}
		return nil
	}
	if err := verify(); err != nil {
		return result, err
	}
	before, err := inspectAccountingVolume(ctx, cli, name)
	if err != nil {
		return result, err
	}
	if createdAt == "" || before.CreatedAt != createdAt {
		return result, errors.New("volume selection changed; refresh its details before measuring")
	}
	usage, err := cli.DiskUsage(ctx, client.DiskUsageOptions{Volumes: true, Verbose: true})
	if err != nil {
		return result, err
	}
	if len(usage.Volumes.Items) > 4096 {
		return result, errors.New("volume accounting exceeds 4096 volumes; no partial size was accepted")
	}
	found := false
	for _, item := range usage.Volumes.Items {
		if item.Name != name {
			continue
		}
		if found {
			return result, errors.New("volume accounting returned duplicate selected names")
		}
		found = true
		if (item.Driver != "" && item.Driver != before.Driver) || (item.CreatedAt != "" && item.CreatedAt != before.CreatedAt) {
			return result, errors.New("accounting observation refers to changed volume metadata")
		}
		if item.UsageData == nil || item.UsageData.Size < 0 || item.UsageData.Size > 9_007_199_254_740_991 || item.UsageData.RefCount < 0 || item.UsageData.RefCount > 1_000_000 {
			return result, errors.New("the engine did not provide supported size and reference counters for this volume")
		}
		result.Bytes, result.References = item.UsageData.Size, item.UsageData.RefCount
	}
	if !found {
		return result, errors.New("volume disappeared from the accounting observation")
	}
	after, err := inspectAccountingVolume(ctx, cli, name)
	if err != nil {
		return result, err
	}
	if !volumeSame(before, after) {
		return result, errors.New("volume metadata changed while measuring; no size was accepted")
	}
	if err = verify(); err != nil {
		return result, err
	}
	result.Name, result.CreatedAt, result.Daemon, result.MeasuredAt = name, createdAt, daemon, time.Now().UTC().Format(time.RFC3339Nano)
	return result, nil
}

func volumeSize(output io.Writer, daemon, name, createdAt string) error {
	if err := resourceFileArguments("volume", name); err != nil {
		return err
	}
	if createdAt == "" || len(createdAt) > 128 {
		return errors.New("select a current volume creation identity")
	}
	cli, closeClient, err := volumeSizeClient(name)
	if err != nil {
		return err
	}
	defer closeClient()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := readVolumeSize(ctx, cli, daemon, name, createdAt)
	if err != nil {
		return fmt.Errorf("volume size: %w", err)
	}
	return json.NewEncoder(output).Encode(result)
}
