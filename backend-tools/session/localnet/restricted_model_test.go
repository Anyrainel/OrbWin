package localnet

import "testing"

func TestExpectedGenerationBeforeDispatch(t *testing.T) {
	for _, text := range []string{"", "0", "01", "-1", "18446744073709551616", "99999999999999999999"} {
		if compareExpectedServer(&ExpectedServer{1, text}, 1, text) == nil {
			t.Fatal("invalid creation accepted", text)
		}
	}
	e := &ExpectedServer{7, "18446744073709551615"}
	if compareExpectedServer(e, 7, e.CreationTime) != nil {
		t.Fatal("uint64 maximum refused")
	}
	for _, actual := range []ExpectedServer{{8, e.CreationTime}, {7, "2"}} {
		if compareExpectedServer(e, actual.PID, actual.CreationTime) == nil {
			t.Fatal("replacement admitted")
		}
	}
	if compareExpectedServer(nil, 7, "1") == nil {
		t.Fatal("missing restriction")
	}
}

func TestExactDisabledInventory(t *testing.T) {
	base := map[string][]byte{"settings-v1.json": []byte("disabled record"), "lan-identity.json": []byte("actual passive binding")}
	want, err := restrictedInventory(base)
	if err != nil {
		t.Fatal(err)
	}
	reordered := map[string][]byte{"lan-identity.json": base["lan-identity.json"], "settings-v1.json": base["settings-v1.json"]}
	got, _ := restrictedInventory(reordered)
	if got != want {
		t.Fatal("map order changed digest")
	}
	changed := map[string][]byte{"settings-v1.json": []byte("disabled record "), "lan-identity.json": base["lan-identity.json"]}
	got, _ = restrictedInventory(changed)
	if got == want {
		t.Fatal("raw byte change hidden")
	}
	delete(changed, "lan-identity.json")
	got, _ = restrictedInventory(changed)
	if got == want {
		t.Fatal("presence hidden")
	}
	for _, role := range []string{"authority-v1.json", "namespace-00000000000000000000000000000001.jsonl", "wsl-trust-foreign.json", "state.op"} {
		if _, err = restrictedInventory(map[string][]byte{"settings-v1.json": []byte("x"), role: []byte("x")}); err == nil {
			t.Fatal("effect record admitted", role)
		}
	}
	if _, err = restrictedInventory(map[string][]byte{"lan-identity.json": []byte("x")}); err == nil {
		t.Fatal("no disabled settings")
	}
}

func TestPermanentRestriction(t *testing.T) {
	for _, op := range []string{"create-authority", "retry", "preview", "lan-options", "lan-activate", "trust-install", "wsl-trust-install"} {
		if restrictedOperation(op, false) {
			t.Fatal("effect admitted", op)
		}
	}
	if restrictedOperation("configure", true) || !restrictedOperation("configure", false) || !restrictedOperation("snapshot", false) || !restrictedOperation("stop-process", false) {
		t.Fatal("closed operation set")
	}
}
