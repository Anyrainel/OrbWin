package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/moby/moby/client"
)

func TestMigrationImageExportRefusesLateIdentityAndTagChanges(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("production export requires a local Windows drive")
	}
	for _, failure := range []string{"", "source-changed", "tag-changed"} {
		t.Run(failure, func(t *testing.T) {
			entries, id := imageTarTestFixture(t, false)
			archive := imageTarTestBytes(t, entries)
			saves, checks := 0, 0
			cli := migrationTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/images/get") {
					saves++
					w.Write(archive)
					return
				}
				if strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json") {
					actual := id
					if failure == "tag-changed" && saves > 0 {
						actual = "sha256:" + strings.Repeat("e", 64)
					}
					json.NewEncoder(w).Encode(map[string]string{"Id": actual, "Os": "linux"})
					return
				}
				t.Errorf("unexpected request %s", r.URL.Path)
				http.NotFound(w, r)
			})
			guard := func() error {
				checks++
				if checks > 1 && failure == "source-changed" {
					return io.ErrUnexpectedEOF
				}
				return nil
			}
			destination := filepath.Join(t.TempDir(), "reviewed.tar")
			var output bytes.Buffer
			err := exportClientImage(context.Background(), cli, &output, id, "example/probe:fixture", destination, guard)
			if failure == "" {
				if err != nil {
					t.Fatal(err)
				}
				actual, err := os.ReadFile(destination)
				if err != nil || !bytes.Equal(actual, archive) {
					t.Fatal("archive changed")
				}
			} else {
				if err == nil || output.Len() != 0 {
					t.Fatal("late source change reported success")
				}
				if _, err := os.Stat(destination); !os.IsNotExist(err) {
					t.Fatal("late source change published archive")
				}
			}
			if saves != 1 || checks != 2 {
				t.Fatalf("unexpected save/recheck counts: %d/%d", saves, checks)
			}
			partials, _ := filepath.Glob(destination + ".orbwin-partial-*")
			if len(partials) != 0 {
				t.Fatal("ordinary failure leaked partial")
			}
		})
	}
}

type migrationRoundTrip func(*http.Request) (*http.Response, error)

func (f migrationRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type migrationBody struct {
	io.Reader
	closed bool
}

func (b *migrationBody) Close() error { b.closed = true; return nil }

func TestMigrationTransportRefusesMutationsAndBoundsMetadata(t *testing.T) {
	calls := 0
	body := &migrationBody{Reader: strings.NewReader(strings.Repeat("x", migrationMetadataLimit+1))}
	transport := migrationTransport{migrationRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: body, Header: make(http.Header)}, nil
	})}
	for _, item := range []struct{ method, path string }{{"POST", "/v1.56/images/load"}, {"DELETE", "/v1.56/images/x"}, {"GET", "/v1.56/containers/x/archive"}, {"GET", "/v1.56/images/../json"}} {
		r, _ := http.NewRequest(item.method, "http://source"+item.path, nil)
		if _, err := transport.RoundTrip(r); err == nil {
			t.Fatalf("allowed %s %s", item.method, item.path)
		}
	}
	if calls != 0 {
		t.Fatal("forbidden request reached source")
	}
	r, _ := http.NewRequest("GET", "http://source/v1.56/info", nil)
	if _, err := transport.RoundTrip(r); err == nil || !body.closed {
		t.Fatal("oversized metadata accepted or source body leaked")
	}
	body = &migrationBody{Reader: strings.NewReader("archive")}
	r, _ = http.NewRequest("GET", "http://source/v1.56/images/get", nil)
	response, err := transport.RoundTrip(r)
	if err != nil || body.closed {
		t.Fatalf("archive stream consumed before caller: %v", err)
	}
	response.Body.Close()
}

func migrationTestClient(t *testing.T, handler http.HandlerFunc) *client.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	cli, err := client.New(client.WithHost(server.URL), client.WithHTTPClient(&http.Client{Transport: migrationTransport{server.Client().Transport}}), client.WithAPIVersion("1.56"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cli.Close() })
	return cli
}

func TestMigrationInventoryIdentityRedactionAndSourceDrift(t *testing.T) {
	for _, failure := range []string{"", "same-target", "source-changed", "unsupported-version", "foreign-product", "volume-warning", "oversized-inventory", "invalid-compose", "invalid-member-image"} {
		t.Run(failure, func(t *testing.T) {
			infoCalls, inventoryCalls := 0, 0
			cli := migrationTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Errorf("source mutation: %s", r.Method)
					http.Error(w, "mutation", 500)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1.56/info":
					infoCalls++
					id := "source"
					if failure == "same-target" || failure == "source-changed" && infoCalls > 1 {
						id = "target"
					}
					product := "Docker Desktop"
					if failure == "foreign-product" {
						product = "Rancher Desktop WSL Distribution"
					}
					json.NewEncoder(w).Encode(map[string]any{"ID": id, "OSType": "linux", "OperatingSystem": product})
				case "/v1.56/version":
					version := "29.8.0"
					if failure == "unsupported-version" {
						version = "29.9.0"
					}
					json.NewEncoder(w).Encode(map[string]any{"Version": version, "Os": "linux", "Arch": "amd64", "ApiVersion": "1.56", "Platform": map[string]string{"Name": "Docker Desktop 4.91.0"}})
				case "/v1.56/images/json":
					inventoryCalls++
					row := map[string]any{"Id": "sha256:" + strings.Repeat("a", 64), "RepoTags": []string{"sample:one"}, "Size": 5, "Labels": map[string]string{"secret": "do-not-emit"}}
					rows := []any{row}
					if failure == "oversized-inventory" {
						rows = make([]any, migrationInventoryItems+1)
						for i := range rows {
							rows[i] = row
						}
					}
					json.NewEncoder(w).Encode(rows)
				case "/v1.56/volumes":
					warnings := []string{}
					if failure == "volume-warning" {
						warnings = append(warnings, "partial listing")
					}
					json.NewEncoder(w).Encode(map[string]any{"Volumes": []any{map[string]any{"Name": "data", "Driver": "local", "Scope": "local", "Options": map[string]string{"password": "do-not-emit"}}}, "Warnings": warnings})
				case "/v1.56/containers/json":
					project := "review"
					memberImage := "sha256:" + strings.Repeat("a", 64)
					if failure == "invalid-member-image" {
						memberImage = "mutable:tag"
					}
					if failure == "invalid-compose" {
						project = "bad\nlabel"
					}
					json.NewEncoder(w).Encode([]any{map[string]any{"Id": strings.Repeat("b", 64), "ImageID": memberImage, "Labels": map[string]string{"com.docker.compose.project": project, "com.docker.compose.service": "web", "com.docker.compose.project.config_files": "/review/one.yml,/review/two.yml", "com.docker.compose.project.environment_file": "/do-not-emit/env", "com.docker.compose.oneoff": "True", "private": "do-not-emit"}, "Mounts": []any{map[string]string{"Type": "bind", "Source": "/do-not-emit", "Destination": "/data"}, map[string]string{"Type": "volume", "Name": "data", "Destination": "/cache"}}}})
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
					http.NotFound(w, r)
				}
			})
			result, err := inspectDesktopMigration(context.Background(), cli, "target")
			if failure != "" {
				if err == nil {
					t.Fatal("invalid source accepted")
				}
				if (failure == "same-target" || failure == "unsupported-version" || failure == "foreign-product") && inventoryCalls != 0 {
					t.Fatal("inventory read before source qualification")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			data, _ := json.Marshal(result)
			if strings.Contains(string(data), "do-not-emit") || len(result.Images) != 1 || len(result.Volumes) != 1 || result.Volumes[0].PlainLocal || len(result.ProjectMembers) != 1 || result.ProjectMembers[0].ConfigFilesLabel != "/review/one.yml,/review/two.yml" || len(result.ProjectMembers[0].Volumes) != 1 {
				t.Fatalf("unsafe/incomplete projection: %s", data)
			}
			if infoCalls != 2 {
				t.Fatal("source identity not checked after inventory")
			}
			member := result.ProjectMembers[0]
			if member.Image != "sha256:"+strings.Repeat("a", 64) {
				t.Fatal("immutable member image identity lost")
			}
			if !member.ExplicitEnvFiles || !member.BindMounts || !member.OneOff {
				t.Fatal("source project review flags lost")
			}
		})
	}
}
