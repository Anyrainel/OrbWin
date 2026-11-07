package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/volume"
	"github.com/moby/moby/client"
)

func TestVolumeSizeReadOnlySnapshotAndFaults(t *testing.T) {
	for _, mode := range []string{"unused", "in-use", "wrong-daemon", "changed-daemon", "stale-selection", "replaced", "missing", "unknown-size", "unknown-refs", "nil-usage", "driver-options", "duplicate", "oversized", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			inspections, infos, scans := 0, 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("unexpected mutation: %s", r.Method)
					http.Error(w, "mutation", 500)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/info"):
					infos++
					daemon := "owned"
					if mode == "wrong-daemon" || (mode == "changed-daemon" && infos > 1) {
						daemon = "foreign"
					}
					json.NewEncoder(w).Encode(map[string]any{"ID": daemon, "OSType": "linux"})
				case strings.HasSuffix(r.URL.Path, "/volumes/data"):
					inspections++
					created := "2026-10-25T00:00:00Z"
					if mode == "replaced" && inspections > 1 {
						created = "2026-10-25T00:00:01Z"
					}
					value := volume.Volume{Name: "data", Driver: "local", Scope: "local", CreatedAt: created, Mountpoint: "/volumes/data"}
					if mode == "driver-options" {
						value.Options = map[string]string{"device": "remote"}
					}
					json.NewEncoder(w).Encode(value)
				case strings.HasSuffix(r.URL.Path, "/system/df"):
					scans++
					if r.URL.RawQuery != "type=volume&verbose=1" {
						t.Errorf("broad scan: %s", r.URL.RawQuery)
					}
					if mode == "cancelled" {
						<-r.Context().Done()
						return
					}
					if mode == "oversized" {
						io.WriteString(w, strings.Repeat(" ", volumeSizeMetadataLimit+1))
						return
					}
					size, refs := int64(12345), int64(0)
					if mode == "in-use" {
						refs = 2
					}
					if mode == "unknown-size" {
						size = -1
					}
					if mode == "unknown-refs" {
						refs = -1
					}
					items := []volume.Volume{{Name: "data", UsageData: &volume.UsageData{Size: size, RefCount: refs}}}
					if mode == "nil-usage" {
						items[0].UsageData = nil
					}
					if mode == "missing" {
						items = nil
					}
					if mode == "duplicate" {
						items = append(items, items[0])
					}
					json.NewEncoder(w).Encode(map[string]any{"VolumeUsage": map[string]any{"Items": items}})
				default:
					t.Errorf("unexpected request: %s", r.URL.Path)
					http.Error(w, "unexpected", 500)
				}
			}))
			defer server.Close()
			cli, err := client.New(client.WithHost(server.URL), client.WithHTTPClient(&http.Client{Transport: volumeSizeTransport{http.DefaultTransport, "data"}}), client.WithAPIVersion("1.56"))
			if err != nil {
				t.Fatal(err)
			}
			defer cli.Close()
			created := "2026-10-25T00:00:00Z"
			if mode == "stale-selection" {
				created = "older"
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if mode == "cancelled" {
				ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
				defer cancel()
			}
			result, err := readVolumeSize(ctx, cli, "owned", "data", created)
			if mode == "unused" || mode == "in-use" {
				if err != nil || result.Bytes != 12345 || result.Name != "data" || result.CreatedAt != created || result.MeasuredAt == "" || (mode == "in-use" && result.References != 2) {
					t.Fatalf("bad size: %+v %v", result, err)
				}
			} else if err == nil {
				t.Fatalf("fault accepted: %+v", result)
			}
			if (mode == "wrong-daemon" || mode == "stale-selection") && scans != 0 {
				t.Fatal("stale selection reached accounting scan")
			}
		})
	}
}

type sizeRoundTripFunc func(*http.Request) (*http.Response, error)

func (f sizeRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestVolumeSizeTransportRefusesUnreviewedRequests(t *testing.T) {
	calls := 0
	transport := volumeSizeTransport{sizeRoundTripFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("unexpected") }), "data"}
	for _, path := range []string{"/v1.56/volumes/other", "/v1.56/system/df", "/v1.56/system/df?type=image&verbose=1", "/vbad/info", "/v1.56/containers/json"} {
		request, _ := http.NewRequest(http.MethodGet, "http://owned"+path, nil)
		if _, err := transport.RoundTrip(request); err == nil {
			t.Fatal("request accepted")
		}
	}
	request, _ := http.NewRequest(http.MethodPost, "http://owned/v1.56/volumes/data", nil)
	transport.RoundTrip(request)
	if calls != 0 {
		t.Fatal("unreviewed request reached transport")
	}
}
