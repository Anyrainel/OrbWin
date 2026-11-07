package main

import (
	"strings"
	"testing"
)

func TestSnapshotReaderClassificationDoesNotHoldMasterForLongReaders(t *testing.T) {
	prefix := []string{"--config", "master", "--host", endpoint}
	for _, command := range []string{"logs", "exec", "attach", "events", "stats", "info", "ps", "compose", "buildx"} {
		args := append(append([]string(nil), prefix...), command)
		if clientConfigWriter(args, "master") {
			t.Fatal("reader serialized behind master", command)
		}
	}
	for _, command := range []string{"login", "logout", "context", "manifest", "trust"} {
		args := append(append([]string(nil), prefix...), command)
		if !clientConfigWriter(args, "master") {
			t.Fatal("client-state writer misclassified", command)
		}
	}
}
func TestSnapshotRoutingOnlyRewritesOwnedConfigPrefix(t *testing.T) {
	args := []string{"--config", "master", "--host", endpoint, "exec", "container", "echo", "--config", "literal-payload"}
	out, e := withClientConfig(args, "master", "snapshot")
	if e != nil || out[1] != "snapshot" || out[8] != "literal-payload" || args[1] != "master" {
		t.Fatal(out, e)
	}
	for _, bad := range [][]string{{"info"}, {"--config", "foreign", "--host", endpoint, "info"}, {"--config", "master", "--host", "foreign", "info"}} {
		if _, e = withClientConfig(bad, "master", "snapshot"); e == nil {
			t.Fatal(bad)
		}
	}
}
func TestSnapshotGenerationUsesExactRawBytes(t *testing.T) {
	one := []byte(`{"unknown":9007199254740993,"credsStore":"orbwin"}`)
	same := append([]byte(nil), one...)
	changed := []byte(`{"unknown":9007199254740992,"credsStore":"orbwin"}`)
	if configGeneration(one) != configGeneration(same) || configGeneration(one) == configGeneration(changed) || !strings.HasPrefix(configGeneration(one), "cfg-") || len(configGeneration(one)) != 68 {
		t.Fatal("generation did not bind exact config data")
	}
}
