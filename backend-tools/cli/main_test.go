package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTargetOverridesRefused(t *testing.T) {
	for _, args := range [][]string{{"--host=x", "ps"}, {"ps", "-Hforeign"}, {"compose", "-cforeign", "up"}, {"compose", "--context=x", "up"}, {"run", "--", "--config=x"}, {"buildx", "create", "remote"}, {"context", "use", "default"}, {"build", "--builder", "remote"}, {"ps", "--tlsverify"}} {
		if dockerGuard(args) == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	for _, args := range [][]string{{"ps", "-a"}, {"buildx", "build", "."}, {"buildx", "version"}, {"compose", "--file", "my compose.yaml", "config"}, {"run", "--rm", "alpine", "printf", "a b"}} {
		if e := dockerGuard(args); e != nil {
			t.Fatal(e)
		}
	}
}
func TestDockerAliasDispatch(t *testing.T) {
	for _, name := range []string{"orbwin-docker.exe", "ORBWIN-DOCKER.EXE", filepath.Join("space dir", "orbwin-docker.exe")} {
		if !reflect.DeepEqual(invocationArgs(name, []string{"info"}), []string{"docker", "info"}) {
			t.Fatal(name)
		}
	}
	if !reflect.DeepEqual(invocationArgs("orbwin-cli.exe", []string{"status"}), []string{"status"}) {
		t.Fatal("normal CLI changed")
	}
}
func TestCloudResolverIsNotAnOwnedEngineTarget(t *testing.T) {
	for _, args := range [][]string{{"ps", "--cloud"}, {"login", "--cloud=team"}, {"compose", "--cloud=", "up"}, {"run", "--cloud", "image"}} {
		if dockerGuard(args) == nil {
			t.Fatalf("accepted resolver selection %v", args)
		}
	}
	for _, args := range [][]string{{"run", "image", "--cloud=literal"}, {"exec", "container", "program", "--cloud"}, {"container", "create", "image", "--cloud"}} {
		if err := dockerGuard(args); err != nil {
			t.Fatalf("inspected workload payload %v: %v", args, err)
		}
	}
}
func TestBuildxInspectBoundary(t *testing.T) {
	for _, args := range [][]string{{"buildx", "inspect"}, {"buildx", "inspect", "default"}, {"buildx", "inspect", "--format", "{{json .}}", "default"}} {
		if e := dockerGuard(args); e != nil {
			t.Fatal(args, e)
		}
	}
	for _, args := range [][]string{{"buildx", "inspect", "remote"}, {"buildx", "inspect", "--bootstrap"}, {"buildx", "inspect", "--builder=remote"}, {"buildx", "ls"}, {"buildx", "use", "default"}} {
		if dockerGuard(args) == nil {
			t.Fatal(args)
		}
	}
}
func TestDockerExecAndRunPayloadAreNotContextFlags(t *testing.T) {
	for _, args := range [][]string{{"run", "--entrypoint", "/bin/sh", "image", "-c", "echo okay"}, {"run", "-c", "100", "image"}, {"exec", "-i", "-u", "root", "container", "/bin/sh", "-c", "echo okay"}, {"container", "exec", "-it", "--env", "VALUE=1", "container", "printf", "--host=literal"}} {
		if e := dockerGuard(args); e != nil {
			t.Fatal(args, e)
		}
	}
	for _, args := range [][]string{{"exec", "-u", "root", "-cforeign", "container", "sh"}, {"exec", "--context=foreign", "container", "sh"}, {"run", "--host=foreign", "image"}, {"ps", "-cforeign"}} {
		if dockerGuard(args) == nil {
			t.Fatal(args)
		}
	}
}
func TestPinnedDockerArgumentBoundaries(t *testing.T) {
	good := [][]string{
		{"run", "-it", "--mount", "type=bind,source=C:\\fixture,target=/workspace", "--entrypoint", "/bin/sh", "--label", "key=value", "image", "-c", "echo okay", "--context=literal", "--host=literal"},
		{"create", "--env", "VALUE=image-like", "--name", "fixture", "image", "--config=literal"},
		{"container", "run", "-ituroot", "--cpu-shares=100", "image", "--tlsverify"},
		{"exec", "--env", "VALUE=image-like", "--user", "root", "container", "sh", "-c", "echo okay", "--builder=literal"},
		{"exec", "-itu", "root", "--", "container", "--host=literal"},
		{"compose", "--project-name", "fixture", "--file", "compose.yaml", "config"},
		{"buildx", "build", "--build-arg", "MESSAGE=image-like", "--load", "."},
	}
	for _, args := range good {
		if e := dockerGuard(args); e != nil {
			t.Fatal(args, e)
		}
	}
	bad := [][]string{
		{"run", "--env", "VALUE=image-like", "--context=foreign", "image"},
		{"run", "--mount", "type=bind,source=fixture,target=/workspace", "--host", "foreign", "image"},
		{"create", "--label", "k=image-like", "--config=foreign", "image"},
		{"exec", "--env", "VALUE=container-like", "--context=foreign", "container", "sh"},
		{"exec", "-u", "root", "--tlsverify", "container", "sh"},
		{"run", "--unknown-future-option", "image-like", "--host=foreign", "image"},
		{"compose", "--file", "compose.yaml", "--context=foreign", "up"},
		{"buildx", "build", "--build-arg", "VALUE=image-like", "--builder=foreign", "."},
	}
	for _, args := range bad {
		if dockerGuard(args) == nil {
			t.Fatal(args)
		}
	}
}
func TestShortTargetClusterRefusalDoesNotInspectOptionValuesOrPayload(t *testing.T) {
	for _, args := range [][]string{{"run", "-itHtcp://foreign", "image"}, {"exec", "-itcFOREIGN", "container", "sh"}, {"container", "create", "-itHforeign", "image"}, {"exec", "-ZitHforeign", "container", "sh"}} {
		if dockerGuard(args) == nil {
			t.Fatal(args)
		}
	}
	for _, args := range [][]string{{"run", "-itc100", "image"}, {"run", "-iteVALUE=Hc", "image"}, {"run", "-it", "image", "-itHliteral"}, {"exec", "-ituroot", "container", "sh", "-itcLITERAL"}, {"compose", "-pfixtureH", "config"}, {"buildx", "build", "-fDockerfileH", "."}} {
		if e := dockerGuard(args); e != nil {
			t.Fatal(args, e)
		}
	}
}
func TestEnvironmentDoesNotInheritTargetOrManagementOverrides(t *testing.T) {
	for _, k := range []string{"DOCKER_CONTEXT", "DOCKER_HOST", "DOCKER_API_VERSION", "RDD_INSTANCE", "RDD_LOG_DIR", "LIMA_HOME", "KUBECONFIG", "COMPOSE_FILE"} {
		t.Setenv(k, "foreign-secret")
	}
	env := cleanEnv(target{identity: identity{Home: "owned"}, config: "private"}, true)
	for _, e := range env {
		if strings.Contains(e, "foreign-secret") {
			t.Fatal(e)
		}
	}
	if !strings.Contains(strings.Join(env, "\n"), "RDD_INSTANCE=orbwin") {
		t.Fatal(env)
	}
}
func TestManagedHelperEnvironmentCannotSkipThePinnedExecutable(t *testing.T) {
	t.Setenv("NoDefaultCurrentDirectoryInExePath", "foreign")
	t.Setenv("PATHEXT", ".CMD")
	t.Setenv("PATH", "")
	// Exercise a genuinely absent PATH, not only an empty inherited value.
	if err := os.Unsetenv("PATH"); err != nil {
		t.Fatal(err)
	}
	env := cleanEnv(target{config: "private", credentialDirectory: "held-credentials", dockerDirectory: "held-docker"}, false)
	got := map[string]string{}
	for _, value := range env {
		key, data, _ := strings.Cut(value, "=")
		got[strings.ToUpper(key)] = data
	}
	if got["NODEFAULTCURRENTDIRECTORYINEXEPATH"] != "1" || got["PATHEXT"] != ".EXE" {
		t.Fatal("managed helper lookup inherited directory or suffix selection")
	}
	if got["PATH"] != "held-credentials"+string(os.PathListSeparator)+"held-docker" {
		t.Fatal("missing parent PATH lost the retained helper path")
	}
}
func TestSnapshotAllowlistDropsPrivateEngineFields(t *testing.T) {
	info := map[string]any{"ID": "daemon", "ServerVersion": "1", "HTTPProxy": "password", "RegistryConfig": map[string]string{"secret": "value"}, "Name": "private-host", "Labels": []string{"secret"}}
	b, _ := json.Marshal(safeInfo(info))
	for _, s := range []string{"password", "RegistryConfig", "secret", "private-host"} {
		if bytes.Contains(b, []byte(s)) {
			t.Fatal(string(b))
		}
	}
}
func TestConfigBoundary(t *testing.T) {
	for _, v := range []string{"application.addPath=front", "running=true --help", "kubernetes.version=latest", "running=TRUE", "running", "running=true=bad"} {
		if _, e := configSet([]string{v}); e == nil {
			t.Fatal(v)
		}
	}
	v, e := configSet([]string{"kubernetes.enabled=false"})
	if e != nil || v != "kubernetes.enabled=false" {
		t.Fatal(v, e)
	}
}
func TestReadAndOutputBounds(t *testing.T) {
	p := filepath.Join(t.TempDir(), "large")
	if e := os.WriteFile(p, bytes.Repeat([]byte(" "), (1<<20)+1), 0600); e != nil {
		t.Fatal(e)
	}
	var v any
	if readJSON(p, &v) == nil {
		t.Fatal("accepted oversized JSON")
	}
	b := limitedBuffer{limit: 3}
	if _, e := b.Write([]byte("1234")); e == nil || len(b.b) != 0 {
		t.Fatal("output bound")
	}
}
func TestDockerArgumentsPreserveLiteralValues(t *testing.T) {
	v := target{config: "space path"}.dockerArgs("run", "image", "$(secret)", "a b")
	want := []string{"--config", "space path", "--host", endpoint, "run", "image", "$(secret)", "a b"}
	if !reflect.DeepEqual(v, want) {
		t.Fatal(v)
	}
}
func TestClientCancellationAndFailure(t *testing.T) {
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	t.Setenv("ORBWIN_CLI_TEST_CHILD", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, e = run(ctx, target{}, exe, []string{"-test.run=TestChildProcess"}, false, false)
	if e == nil || ctx.Err() == nil {
		t.Fatal(e)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	_, e = run(ctx2, target{}, exe, []string{"-test.run=TestChildFailure"}, false, false)
	if e == nil {
		t.Fatal("accepted failed client")
	}
}
func TestChildProcess(t *testing.T) {
	if os.Getenv("ORBWIN_CLI_TEST_CHILD") == "1" {
		time.Sleep(5 * time.Second)
		os.Exit(0)
	}
}
func TestChildFailure(t *testing.T) {
	if os.Getenv("ORBWIN_CLI_TEST_CHILD") == "1" {
		os.Exit(23)
	}
}
