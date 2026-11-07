//go:build linux

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestComposeLabelsRetainReviewedSourceIdentityAndOrderedRelativeInputs(t *testing.T) {
	request := workflowRequest{Source: "/home/user/project 例", Signature: "1:2", Distribution: "Ubuntu", File: "base.yaml", Overrides: []string{"override.yaml", "last.yaml"}, Project: "sample", Profiles: []string{"worker"}}
	request.EnvFiles = []string{"first.env", "last 例.env"}
	service := map[string]any{"image": "example:test"}
	model := map[string]any{"services": map[string]any{"web": service}}
	if err := request.labelModel(model); err != nil {
		t.Fatal(err)
	}
	labels := service["labels"].(map[string]any)
	encoded := labels["dev.orbwin.wsl.compose-inputs"].(string)
	var decoded struct {
		Source, Signature, File string
		Overrides               []string
		EnvFiles                []string
	}
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		t.Fatal(err)
	}
	request.Signature = "3:4"
	request.EnvFiles[0] = "later.env"
	if len(decoded.EnvFiles) != 2 || decoded.EnvFiles[0] != "first.env" || decoded.EnvFiles[1] != "last 例.env" {
		t.Fatalf("lost reviewed env-file order: %s", encoded)
	}
	if decoded.Source != "/home/user/project 例" || decoded.Signature != "1:2" || decoded.File != "base.yaml" || len(decoded.Overrides) != 2 || decoded.Overrides[0] != "override.yaml" || decoded.Overrides[1] != "last.yaml" {
		t.Fatalf("lost reviewed identity/merge order: %s", encoded)
	}
}

func TestOriginalProjectPathMappingContainsLinksAndKeepsFilesTyped(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "project ü"), 0750); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "project ü", "file.txt")
	if err := os.WriteFile(file, []byte("retained"), 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escaped")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("project ü", filepath.Join(root, "inside")); err != nil {
		t.Fatal(err)
	}
	request := workflowRequest{Source: root, Shared: "/mnt/wsl/orbwin/projects/000000000000000000000001", Directory: filepath.Join(root, "project ü")}
	for _, source := range []string{file, "file.txt", request.Shared + "/project ü/file.txt"} {
		mapped, err := request.mappedPath(source, pathFile)
		if err != nil || mapped != request.Shared+"/project ü/file.txt" {
			t.Fatalf("wrong original file mapping %q: %q %v", source, mapped, err)
		}
	}
	for _, source := range []string{outside, filepath.Join(root, "escaped"), filepath.Join(root, "missing"), filepath.Join(root, "project ü")} {
		if _, err := request.mappedPath(source, pathFile); err == nil {
			t.Fatalf("accepted unavailable/escaping/non-file path %q", source)
		}
	}
	mapped, err := request.mappedPath(filepath.Join(root, "inside"), pathDirectory)
	if err != nil || mapped != request.Shared+"/project ü" {
		t.Fatalf("internal link mapping failed: %q %v", mapped, err)
	}
	if _, err := request.mappedPath(file, pathDirectory); err == nil {
		t.Fatal("file accepted as build context directory")
	}
}

func TestProjectOwnershipLabelsPreserveUserLabelsAndExternalResources(t *testing.T) {
	request := workflowRequest{Source: "/home/user/project", Distribution: "Ubuntu", File: "compose.yaml", Project: "proof"}
	model := configModel(t, `{"services":{"app":{"labels":{"user":"retained"}}},"volumes":{"data":{},"outside":{"external":true}},"networks":{"default":{}}}`)
	if err := request.labelModel(model); err != nil {
		t.Fatal(err)
	}
	service := model["services"].(map[string]any)["app"].(map[string]any)
	labels := service["labels"].(map[string]any)
	if labels["user"] != "retained" || labels["dev.orbwin.wsl.project-source"] != request.Source {
		t.Fatal("ownership labels dropped user data")
	}
	outside := model["volumes"].(map[string]any)["outside"].(map[string]any)
	if _, exists := outside["labels"]; exists {
		t.Fatal("external volume was relabeled")
	}
}
