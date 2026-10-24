package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/docker/docker-credential-helpers/credentials"
)

type memoryVault struct {
	sync.Mutex
	entries map[string]entry
	fail    bool
	filter  string
}

func newMemoryVault() *memoryVault { return &memoryVault{entries: map[string]entry{}} }
func (v *memoryVault) get(key string) (entry, error) {
	v.Lock()
	defer v.Unlock()
	if v.fail {
		return entry{}, errors.New("DO-NOT-LOG-THIS-SECRET")
	}
	item, ok := v.entries[key]
	if !ok {
		return entry{}, missing
	}
	return item, nil
}
func (v *memoryVault) put(item entry) error {
	v.Lock()
	defer v.Unlock()
	v.entries[item.key] = item
	return nil
}
func (v *memoryVault) remove(key string) error {
	v.Lock()
	defer v.Unlock()
	delete(v.entries, key)
	return nil
}
func (v *memoryVault) list(filter string) ([]entry, error) {
	v.Lock()
	defer v.Unlock()
	v.filter = filter
	if v.fail {
		return nil, errors.New("DO-NOT-LOG-THIS-SECRET")
	}
	var result []entry
	for _, item := range v.entries {
		if strings.HasPrefix(item.key, strings.TrimSuffix(filter, "*")) {
			result = append(result, item)
		}
	}
	return result, nil
}

func TestCredentialKeysCannotEscapeTheProductNamespace(t *testing.T) {
	seen := map[string]bool{}
	for _, server := range []string{"registry.example:5000", "https://index.docker.io/v1/", "../DockerDesktop", "régistry.example", "OrbWin/registry/v1/another"} {
		key, err := keyFor(server)
		if err != nil || !strings.HasPrefix(key, prefix) || seen[key] {
			t.Fatal("Invalid or colliding namespaced key")
		}
		seen[key] = true
	}
	for _, server := range []string{"", " x", "x\n", "x\x00y", strings.Repeat("x", 2049), string([]byte{0xff})} {
		if _, err := keyFor(server); err == nil {
			t.Fatal("Accepted invalid registry key")
		}
	}
}

func TestCredentialProtocolRoundTripAndIdempotentErase(t *testing.T) {
	v := newMemoryVault()
	input := credentials.Credentials{ServerURL: "registry.example:5000", Username: "proof-user", Secret: "disposable-synthetic-secret"}
	encoded, _ := json.Marshal(input)
	var output bytes.Buffer
	if err := run([]string{"store"}, bytes.NewReader(encoded), &output, v); err != nil || output.Len() != 0 {
		t.Fatal("Store protocol failed")
	}
	if err := run([]string{"get"}, strings.NewReader(input.ServerURL), &output, v); err != nil {
		t.Fatal("Get protocol failed")
	}
	var actual credentials.Credentials
	if json.Unmarshal(output.Bytes(), &actual) != nil || actual != input {
		t.Fatal("Get returned the wrong credential")
	}
	output.Reset()
	if err := run([]string{"list"}, nil, &output, v); err != nil {
		t.Fatal("List failed")
	}
	if strings.Contains(output.String(), input.Secret) || v.filter != prefix+"*" {
		t.Fatal("List exposed a secret or enumerated outside the product namespace")
	}
	for range 2 {
		if err := (helper{v}).Delete(input.ServerURL); err != nil {
			t.Fatal("Erase was not idempotent")
		}
	}
	if _, _, err := (helper{v}).Get(input.ServerURL); !credentials.IsErrCredentialsNotFound(err) {
		t.Fatal("Missing credentials not reported with Docker's protocol error")
	}
}

func TestForeignCredentialAtAnOwnedKeyCannotBeReturnedReplacedOrDeleted(t *testing.T) {
	v := newMemoryVault()
	key, _ := keyFor("registry.example")
	foreign := entry{key: key, username: "foreign", secret: "foreign-synthetic-secret", label: "Another application"}
	v.entries[key] = foreign
	h := helper{v}
	if _, _, err := h.Get("registry.example"); err == nil {
		t.Fatal("Returned a foreign credential")
	}
	if err := h.Add(&credentials.Credentials{ServerURL: "registry.example", Username: "ours", Secret: "synthetic"}); err == nil {
		t.Fatal("Replaced a foreign credential")
	}
	if err := h.Delete("registry.example"); err == nil {
		t.Fatal("Deleted a foreign credential")
	}
	listed, err := h.List()
	if err != nil || len(listed) != 0 || v.entries[key] != foreign {
		t.Fatal("Foreign record was exposed or changed")
	}
}

func TestMalformedOversizedAndFailedInputsNeverQuoteSecrets(t *testing.T) {
	secret := "DO-NOT-LOG-THIS-SECRET"
	for _, input := range []string{`{"ServerURL":` + secret, `{"ServerURL":{"` + secret + `":true},"Username":"u","Secret":"s"}`, strings.Repeat(secret, inputLimit)} {
		var out bytes.Buffer
		err := run([]string{"store"}, strings.NewReader(input), &out, newMemoryVault())
		if err == nil || strings.Contains(err.Error(), secret) || out.Len() != 0 {
			t.Fatal("Invalid input was accepted or disclosed")
		}
	}
	v := newMemoryVault()
	v.fail = true
	if err := (helper{v}).Add(&credentials.Credentials{ServerURL: "registry.example", Username: "u", Secret: secret}); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatal("Vault error was not safely reported")
	}
	for _, size := range []int{2560, 2561} {
		err := (helper{newMemoryVault()}).Add(&credentials.Credentials{ServerURL: "registry.example", Username: "u", Secret: strings.Repeat("x", size)})
		if (err == nil) != (size == 2560) {
			t.Fatal("Windows credential size boundary is incorrect")
		}
	}
}

func TestIndependentConcurrentRegistryWritesDoNotCrossTargets(t *testing.T) {
	v := newMemoryVault()
	h := helper{v}
	var workers sync.WaitGroup
	for i := range 32 {
		workers.Go(func() {
			server := fmt.Sprintf("registry-%d.example", i)
			if h.Add(&credentials.Credentials{ServerURL: server, Username: server, Secret: "synthetic"}) != nil {
				t.Error("Store failed")
			}
		})
	}
	workers.Wait()
	listed, err := h.List()
	if err != nil || len(listed) != 32 {
		t.Fatal("Concurrent registry credentials were lost")
	}
	for server, username := range listed {
		if server != username {
			t.Fatal("Credential target crossed")
		}
	}
}
