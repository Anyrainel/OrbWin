package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPodConfigRejectsForeignTransportAndCredentialPlugins(t *testing.T) {
	base := `apiVersion: v1
kind: Config
current-context: owned
contexts:
- name: owned
  context: {cluster: owned, user: owned}
clusters:
- name: owned
  cluster:
    server: https://127.0.0.1:6443
users:
- name: owned
  user: {token: test-token}
`
	path := filepath.Join(t.TempDir(), "config")
	for _, candidate := range []string{
		strings.Replace(base, "127.0.0.1", "example.com", 1),
		strings.Replace(base, "https:", "http:", 1),
		strings.Replace(base, "server:", "insecure-skip-tls-verify: true\n    server:", 1),
		strings.Replace(base, "server:", "proxy-url: http://127.0.0.1:8888\n    server:", 1),
		strings.Replace(base, "user: {token: test-token}", "user:\n    exec: {apiVersion: client.authentication.k8s.io/v1, command: must-never-run, interactiveMode: Never}", 1),
		strings.Repeat("x", (1<<20)+1),
	} {
		if err := os.WriteFile(path, []byte(candidate), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := podConfig(path); err == nil {
			t.Fatal("accepted unsafe kubeconfig")
		}
	}
	if err := os.WriteFile(path, []byte(base), 0600); err != nil {
		t.Fatal(err)
	}
	config, err := podConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if proxy, err := config.Proxy(nil); err != nil || proxy != nil {
		t.Fatal("inherited a proxy")
	}
}

func TestPodOpenRejectsUnboundedOrAmbiguousTargets(t *testing.T) {
	valid := message{Kind: "open", Mode: "kubernetes", Namespace: "default", Pod: "test-pod", PodUID: "returned-uid", Kubeconfig: "owned-config", Shell: "sh", Cols: 80, Rows: 24}
	if err := validateOpen(valid); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*message){
		func(m *message) { m.Namespace = "--all-namespaces" },
		func(m *message) { m.PodUID = "" },
		func(m *message) { m.ContainerName = "a/b" },
		func(m *message) { m.Shell = "powershell" },
		func(m *message) { m.Rows = 0 },
	} {
		candidate := valid
		mutate(&candidate)
		if validateOpen(candidate) == nil {
			t.Fatal("accepted invalid pod open")
		}
	}
}
