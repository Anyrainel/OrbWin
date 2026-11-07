package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func configModel(t *testing.T, text string) map[string]any {
	t.Helper()
	var model map[string]any
	if err := json.Unmarshal([]byte(text), &model); err != nil {
		t.Fatal(err)
	}
	return model
}
func TestComposeMappingChangesOnlyFilesystemFields(t *testing.T) {
	model := configModel(t, `{"services":{"web":{"image":"fixture","environment":{"SOURCE":"/home/user/project/data"},"command":["printf","/home/user/project/data"],"labels":{"retained":"/home/user/project/data"},"volumes":[{"type":"bind","source":"/home/user/project/data","target":"/project","read_only":true},{"type":"volume","source":"persistent","target":"/persistent"}],"build":{"context":"/home/user/project","dockerfile":"docker/Dockerfile","additional_contexts":{"local":"/home/user/project/assets","base":"docker-image://alpine:3.21","service":"service:builder"}}}},"secrets":{"key":{"file":"/home/user/project/key"}},"configs":{"app":{"file":"/home/user/project/config"}},"volumes":{"persistent":{}}}`)
	calls := map[string]pathKind{}
	mapper := func(value string, kind pathKind) (string, error) {
		calls[value] = kind
		if !strings.HasPrefix(value, "/home/user/project") && !strings.HasPrefix(value, "/mnt/wsl/orbwin/projects/proof") {
			return "", errors.New("outside project")
		}
		return strings.Replace(value, "/home/user/project", "/mnt/wsl/orbwin/projects/proof", 1), nil
	}
	if err := rewriteComposePaths(model, mapper); err != nil {
		t.Fatal(err)
	}
	service := model["services"].(map[string]any)["web"].(map[string]any)
	for _, field := range []string{"environment", "labels"} {
		object := service[field].(map[string]any)
		for _, value := range object {
			if value != "/home/user/project/data" {
				t.Fatalf("application %s string was changed", field)
			}
		}
	}
	if service["command"].([]any)[1] != "/home/user/project/data" {
		t.Fatal("application command was rewritten")
	}
	attachment := service["volumes"].([]any)[0].(map[string]any)
	if attachment["source"] != "/mnt/wsl/orbwin/projects/proof/data" || attachment["read_only"] != true || attachment["bind"].(map[string]any)["create_host_path"] != false {
		t.Fatal("bind scope/options changed incorrectly")
	}
	build := service["build"].(map[string]any)
	if build["dockerfile"] != "/mnt/wsl/orbwin/projects/proof/docker/Dockerfile" || calls["/mnt/wsl/orbwin/projects/proof/docker/Dockerfile"] != pathFile {
		t.Fatal("Dockerfile was not resolved relative to its build context")
	}
	if build["additional_contexts"].(map[string]any)["base"] != "docker-image://alpine:3.21" {
		t.Fatal("remote context changed")
	}
	if calls["/home/user/project/key"] != pathFile || calls["/home/user/project/data"] != pathBind {
		t.Fatal("file/bind type checks were lost")
	}
}
func TestComposeMappingRefusesUnreviewedOrEscapingFilesystemFeatures(t *testing.T) {
	for _, text := range []string{
		`{"services":{"app":{"env_file":["/home/user/.secret"]}}}`,
		`{"services":{"app":{"volumes":["/home/user/data:/data"]}}}`,
		`{"services":{"app":{"devices":["/dev/kvm"]}}}`,
		`{"services":{"app":{"develop":{"watch":[]}}}}`,
		`{"services":{"app":{"use_api_socket":true}}}`,
		`{"services":{"app":{}},"volumes":{"data":{"driver_opts":{"device":"/home/user/outside"}}}}`,
		`{"services":{"app":{"build":{"context":"/home/user/project","dockerfile":"../../../etc/shadow"}}}}`,
		`{"services":{"app":{"volumes":[{"type":"bind","source":"/home/user/outside","target":"/data"}]}}}`,
	} {
		model := configModel(t, text)
		mapper := func(value string, _ pathKind) (string, error) {
			if !strings.HasPrefix(value, "/home/user/project/") && value != "/home/user/project" {
				return "", errors.New("outside selected root")
			}
			return value, nil
		}
		if err := rewriteComposePaths(model, mapper); err == nil {
			t.Fatalf("accepted unsupported/escaping workflow: %s", text)
		}
	}
}
