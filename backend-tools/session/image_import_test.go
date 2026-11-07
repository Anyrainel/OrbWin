package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moby/moby/client"
)

func TestImageImportReviewRequiresExactTagAndConfigConsent(t *testing.T) {
	a := "sha256:" + strings.Repeat("a", 64)
	b := "sha256:" + strings.Repeat("b", 64)
	index := imageArchiveIndex{Images: []imageArchiveImage{{ID: a, Tags: []string{"example/probe:test"}}, {ID: b, Tags: []string{"docker.io/example/probe:test"}}}}
	plan, err := imageImportTagPlan(index)
	if err != nil || len(plan) != 1 || len(plan[0].ConfigIDs) != 2 {
		t.Fatalf("ambiguous tag was lost: %+v %v", plan, err)
	}
	choice := imageImportChoice{Reference: plan[0].Reference, ConfigID: a, ExpectedID: b, Apply: true, Replace: true}
	if _, err := imageImportChoices(index, []imageImportChoice{choice}); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*imageImportChoice){func(c *imageImportChoice) { c.Replace = false }, func(c *imageImportChoice) { c.ConfigID = "" }, func(c *imageImportChoice) { c.Reference = "example/probe:test" }, func(c *imageImportChoice) { c.ConfigID = "sha256:" + strings.Repeat("c", 64) }, func(c *imageImportChoice) { c.ExpectedID = "not an image" }} {
		copy := choice
		change(&copy)
		if _, err := imageImportChoices(index, []imageImportChoice{copy}); err == nil {
			t.Fatalf("unreviewed choice accepted: %+v", copy)
		}
	}
	choice.Apply = false
	choice.Replace = false
	choice.ConfigID = ""
	if _, err := imageImportChoices(index, []imageImportChoice{choice}); err != nil {
		t.Fatal("explicit skip should not select an arbitrary platform:", err)
	}
	if _, err := imageImportChoices(index, nil); err == nil {
		t.Fatal("omitted archived tag accepted")
	}
	for _, ref := range []string{"name", "name@sha256:" + strings.Repeat("a", 64), "orbwin-imports/owned/image:loaded"} {
		if _, err := imageImportReference(ref); err == nil {
			t.Fatalf("unsupported reference accepted: %s", ref)
		}
	}
}
func TestImageImportNormalizationOmitsAlternateTagInstructions(t *testing.T) {
	entries, id := imageTarTestFixture(t, true)
	entries = append(entries, imageTarTestEntry{"index.json", []byte(`{"unreviewed":"tag"}`), tar.TypeReg}, imageTarTestEntry{"repositories", []byte(`{"other":"tag"}`), tar.TypeReg})
	data := imageTarTestBytes(t, entries)
	index, err := inspectImageArchive(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	stage := imageImportStageName(strings.Repeat("1", 32), 0)
	var normalized bytes.Buffer
	if err := writeImageImportArchive(context.Background(), &normalized, bytes.NewReader(data), index, []string{stage}); err != nil {
		t.Fatal(err)
	}
	copy, err := inspectImageArchive(bytes.NewReader(normalized.Bytes()), int64(normalized.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(copy.Images) != 1 || copy.Images[0].ID != id || len(copy.Images[0].Tags) != 1 || copy.Images[0].Tags[0] != stage {
		t.Fatalf("normalization changed identity or leaked original tag: %+v", copy.Images)
	}
	if _, ok := copy.Entries["index.json"]; ok {
		t.Fatal("OCI tag instructions were forwarded")
	}
	if _, ok := copy.Entries["repositories"]; ok {
		t.Fatal("legacy tag instructions were forwarded")
	}
	changed := append([]byte{}, data...)
	entry := index.Entries["config.json"]
	changed[entry.Offset]++
	if err := writeImageImportArchive(context.Background(), io.Discard, bytes.NewReader(changed), index, []string{stage}); err == nil {
		t.Fatal("source payload change was not detected while streaming")
	}
	var config struct {
		RootFS struct {
			DiffIDs []string `json:"diff_ids"`
		} `json:"rootfs"`
	}
	if err := json.Unmarshal(entries[1].data, &config); err != nil {
		t.Fatal(err)
	}
	if err := verifyImageImportStream(bytes.NewReader(normalized.Bytes()), id, config.RootFS.DiffIDs); err != nil {
		t.Fatal(err)
	}
	if err := verifyImageImportStream(bytes.NewReader(normalized.Bytes()), "sha256:"+strings.Repeat("0", 64), config.RootFS.DiffIDs); err == nil {
		t.Fatal("wrong staged config was accepted")
	}
	if err := verifyImageImportStream(bytes.NewReader(normalized.Bytes()), id, []string{"sha256:" + strings.Repeat("0", 64)}); err == nil {
		t.Fatal("wrong staged rootfs was accepted")
	}
}

type imageImportTransport func(*http.Request) (*http.Response, error)

func (transport imageImportTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}
func TestImageImportSpaceRefusesAChangedDaemonBeforeCreatingTools(t *testing.T) {
	var mutations atomic.Int32
	cli, err := client.New(client.WithHost("http://localhost"), client.WithAPIVersion("1.56"), client.WithHTTPClient(&http.Client{Transport: imageImportTransport(func(request *http.Request) (*http.Response, error) {
		if request.Method != "GET" {
			mutations.Add(1)
			return nil, errors.New("unexpected mutation")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"ID":"foreign","OSType":"linux","DockerRootDir":"/var/lib/docker"}`)), Header: http.Header{"Content-Type": []string{"application/json"}}, Request: request}, nil
	})}))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	if err := imageImportGuestSpace(context.Background(), cli, imageArchiveIndex{Bytes: 1024}, "owned"); err == nil || mutations.Load() != 0 {
		t.Fatalf("changed daemon reached helper creation: %v, mutations=%d", err, mutations.Load())
	}
}
func TestImageImportLoadStopsItsProducerOnEarlyFailure(t *testing.T) {
	entries, _ := imageTarTestFixture(t, false)
	data := imageTarTestBytes(t, entries)
	index, err := inspectImageArchive(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"success", "early reject", "stream error", "transport failure"} {
		t.Run(scenario, func(t *testing.T) {
			cli, err := client.New(client.WithHost("http://localhost"), client.WithAPIVersion("1.56"), client.WithHTTPClient(&http.Client{Transport: imageImportTransport(func(request *http.Request) (*http.Response, error) {
				if scenario == "transport failure" {
					return nil, errors.New("injected transport failure")
				}
				status := 200
				body := `{"stream":"Loaded isolated fixture\n"}`
				if scenario == "early reject" {
					status = 400
					body = `{"message":"rejected before consuming archive"}`
				} else if scenario == "stream error" {
					body = `{"errorDetail":{"message":"injected disk full"}}`
				} else {
					if _, err := io.Copy(io.Discard, request.Body); err != nil {
						return nil, err
					}
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": []string{"application/json"}}, Request: request}, nil
			})}))
			if err != nil {
				t.Fatal(err)
			}
			defer cli.Close()
			done := make(chan error, 1)
			go func() {
				done <- loadIsolatedImageArchive(context.Background(), cli, bytes.NewReader(data), index, []string{imageImportStageName(strings.Repeat("1", 32), 0)})
			}()
			select {
			case err := <-done:
				if (err == nil) != (scenario == "success") {
					t.Fatalf("unexpected load outcome: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("load returned without releasing its blocked normalization producer")
			}
		})
	}
}
func TestImageImportReceiptRetainsPreviousContentBeforeUserTags(t *testing.T) {
	t.Setenv("ORBWIN_TRANSIENT_DIR", filepath.Join(t.TempDir(), "transients"))
	entries, id := imageTarTestFixture(t, false)
	data := imageTarTestBytes(t, entries)
	index, err := inspectImageArchive(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	old := "sha256:" + strings.Repeat("a", 64)
	loaded := "sha256:" + strings.Repeat("b", 64)
	request := imageImportRequest{Source: `C:\fixture.tar`, SHA256: index.SHA256, Tags: []imageImportChoice{{Reference: "docker.io/example/probe:fixture", ConfigID: id, ExpectedID: old, Apply: true, Replace: true}}}
	job, err := beginImageImport("owned-daemon", request, index)
	if err != nil {
		t.Fatal(err)
	}
	if err := job.append(imageImportEvent{Phase: "tagging"}); err == nil {
		t.Fatal("user tag writes preceded staged verification")
	}
	if err := job.append(imageImportEvent{Phase: "loading"}); err != nil {
		t.Fatal(err)
	}
	if err := job.append(imageImportEvent{Phase: "verified", ConfigID: id, ImageID: loaded}); err != nil {
		t.Fatal(err)
	}
	if err := job.append(imageImportEvent{Phase: "tagging"}); err == nil {
		t.Fatal("replacement preceded previous-content retention")
	}
	ref := request.Tags[0].Reference
	for _, event := range []imageImportEvent{{Phase: "previous-intent", Reference: ref, ImageID: old}, {Phase: "previous-retained", Reference: ref, ImageID: old}, {Phase: "tagging"}, {Phase: "tag-intent", Reference: ref, ConfigID: id, ImageID: loaded}} {
		if err := job.append(event); err != nil {
			t.Fatal(err)
		}
	}
	if err := job.append(imageImportEvent{Phase: "complete"}); err == nil {
		t.Fatal("unacknowledged tag was marked complete")
	}
	if status, err := imageImportRecordStatus("owned-daemon", job.record.Transaction); err != nil || !status.Active || status.PendingReference != ref {
		t.Fatalf("active uncertainty was lost: %+v %v", status, err)
	}
	for _, event := range []imageImportEvent{{Phase: "tagged", Reference: ref, ConfigID: id, ImageID: loaded}, {Phase: "complete"}} {
		if err := job.append(event); err != nil {
			t.Fatal(err)
		}
	}
	path := job.file.Name()
	token := job.record.Transaction
	if err := job.close(); err != nil {
		t.Fatal(err)
	}
	status, err := imageImportRecordStatus("owned-daemon", token)
	if err != nil || status.Active || status.Phase != "complete" || status.Previous[ref] != old || status.PreviousReferences[ref] == "" {
		t.Fatalf("durable result differs: %+v %v", status, err)
	}
	if _, err := imageImportRecordStatus("foreign", token); err == nil {
		t.Fatal("foreign receipt was adopted")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString(`{"phase":`)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	status, err = readImageImportRecord(path)
	if err != nil || !status.Truncated {
		t.Fatalf("truncated suffix was not preserved: %+v %v", status, err)
	}
	if err := applyImageImportEvent(&status, imageImportEvent{Phase: "complete"}); err == nil {
		t.Fatal("truncated receipt was replayable")
	}
}
