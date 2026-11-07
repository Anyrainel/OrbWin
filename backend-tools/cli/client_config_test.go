package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type fakeConfigNative struct {
	calls    []string
	body     []byte
	failure  string
	original []byte
}

func (f *fakeConfigNative) step(name string) error {
	f.calls = append(f.calls, name)
	if name == f.failure {
		return errors.New("injected")
	}
	return nil
}
func (f *fakeConfigNative) check() error { return f.step("check") }
func (f *fakeConfigNative) beginWriter(_ [32]byte, _ clientWriterScope, _ []string, base []byte) error {
	f.original = append([]byte(nil), base...)
	return f.step("flush-intent-and-release-reader")
}
func (f *fakeConfigNative) readOriginal() ([]byte, error) {
	return append([]byte(nil), f.original...), f.step("read-original")
}
func (f *fakeConfigNative) readAfterWriter(string) ([]byte, error) {
	return f.body, f.step("read-after-writer")
}
func (f *fakeConfigNative) commitMergedConfig(body []byte) error {
	if e := f.step("commit-merged-config"); e != nil {
		return e
	}
	f.body = append([]byte(nil), body...)
	return nil
}
func (f *fakeConfigNative) finishIntent() error { return f.step("finish-intent") }
func (f *fakeConfigNative) abortUnstarted() ([]byte, error) {
	return f.body, f.step("verify-original-and-abort-intent")
}
func (f *fakeConfigNative) close() error { return f.step("close-owner") }
func guardedConfig(t *testing.T) (*clientConfigGuard, *fakeConfigNative) {
	t.Helper()
	body := []byte(`{"credsStore":"orbwin","cliPluginsExtraDirs":["pinned"],"unknown":{"kept":true}}`)
	f := &fakeConfigNative{body: body}
	return &clientConfigGuard{native: f, path: "private", plugins: "pinned", body: body}, f
}
func TestPrivateConfigRawAliasesAndDuplicateRefusal(t *testing.T) {
	for _, data := range []string{`{"credsStore":"orbwin","CredsStore":"foreign"}`, `{"credsStore":"orbwin","credsStore":"foreign"}`, `{"cliPluginsExtraDirs":[],"cliPluginsextraDirs":["foreign"]}`, `{"auths":{},"AUTHS":{}}`, `{"auths":{"registry":{"auth":"a","auth":"b"}}}`, `{} {}`} {
		if _, e := checkedDockerConfigJSON([]byte(data)); e == nil {
			t.Fatal(data)
		}
	}
	if _, e := checkedDockerConfigJSON([]byte(`{"unknown":123456789012345678901234567890}`)); e != nil {
		t.Fatal(e)
	}
}
func TestPrivateConfigWriterProofExactAndSingleLoan(t *testing.T) {
	g, f := guardedConfig(t)
	admission, e := g.beginWriterScope(context.Background(), writerRegistryAuth, []string{"registry"})
	if e != nil {
		t.Fatal(e)
	}
	f.body = []byte(`{"credsStore":"orbwin","cliPluginsExtraDirs":["pinned"],"auths":{"registry":{"auth":"opaque-only"}}}`)
	calls := 0
	e = g.finishWith(func(binding [32]byte) bool { calls++; return binding == admission.binding() })
	if e != nil {
		t.Fatal(e)
	}
	if calls != 1 || g.loan != nil || g.poison || string(g.body) != string(f.body) {
		t.Fatal("positive exact writer settlement lost bytes")
	}
	if e = g.finishWith(func([32]byte) bool { t.Fatal("proof reused"); return true }); e == nil {
		t.Fatal("second settlement admitted")
	}
	if e = g.close(); e != nil {
		t.Fatal(e)
	}
}
func TestPrivateConfigWrongProofNeverReadsOrClosesOwner(t *testing.T) {
	g, f := guardedConfig(t)
	_, e := g.beginWriter(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if e = g.finishWith(func([32]byte) bool { return false }); e == nil {
		t.Fatal("foreign proof accepted")
	}
	if e = g.close(); e == nil {
		t.Fatal("unknown loan owner released")
	}
	joined := strings.Join(f.calls, ",")
	if strings.Contains(joined, "read-after-writer") || strings.Contains(joined, "close-owner") {
		t.Fatal(joined)
	}
}
func TestPrivateConfigFaultsRetainIntentAndNoReplay(t *testing.T) {
	for _, stage := range []string{"flush-intent-and-release-reader", "read-after-writer", "read-original", "commit-merged-config", "finish-intent"} {
		t.Run(stage, func(t *testing.T) {
			g, f := guardedConfig(t)
			f.failure = stage
			_, e := g.beginWriter(context.Background())
			if stage == "flush-intent-and-release-reader" {
				if e == nil {
					t.Fatal("intent failure lost")
				}
			} else {
				if e != nil {
					t.Fatal(e)
				}
				if e = g.finishWith(func([32]byte) bool { return true }); e == nil {
					t.Fatal("fault lost")
				}
			}
			if _, e = g.beginWriter(context.Background()); e == nil {
				t.Fatal("replayed unknown")
			}
			if e = g.close(); e == nil {
				t.Fatal("owner close after unknown")
			}
			if strings.Contains(strings.Join(f.calls, ","), "close-owner") {
				t.Fatal(f.calls)
			}
		})
	}
}
func TestPrivateConfigPostWriterRoutingDriftNeverSeals(t *testing.T) {
	g, f := guardedConfig(t)
	_, e := g.beginWriter(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	var value map[string]any
	if json.Unmarshal(f.body, &value) != nil {
		t.Fatal("fixture")
	}
	value["credsStore"] = "foreign"
	f.body, _ = json.Marshal(value)
	if g.finishWith(func([32]byte) bool { return true }) == nil {
		t.Fatal("foreign helper accepted")
	}
	if strings.Contains(strings.Join(f.calls, ","), "finish-intent") {
		t.Fatal(f.calls)
	}
}

func TestPrivateConfigUnknownRawSchemaContextPreserved(t *testing.T) {
	body := []byte(`{"credsStore":"orbwin","cliPluginsExtraDirs":["pinned"],"unknown":1e9999,"huge":1234567890123456789012345678901234567890,"HttpHeaders":{"CreDsStoRe":"inert-header"},"other":{"credsStore":1e9999,"same":1,"same":2},"auths":{"registry":{"username":"u","unrelated":1e9999}}}`)
	if e := privateClientRouting(body, "pinned"); e != nil {
		t.Fatal(e)
	}
	bad := []byte(`{"credsStore":"orbwin","cliPluginsExtraDirs":["pinned"],"auths":{"registry":{"username":"u","UserName":"v"}}}`)
	if privateClientRouting(bad, "pinned") == nil {
		t.Fatal("AuthConfig alias admitted")
	}
}

func TestPrivateConfigKnownUnstartedRestoresOnlyUnchangedOriginal(t *testing.T) {
	for _, changed := range []bool{false, true} {
		g, f := guardedConfig(t)
		admission, e := g.beginWriter(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		if changed {
			f.body = []byte(`{"credsStore":"orbwin","cliPluginsExtraDirs":["pinned"],"changed":true}`)
		}
		e = g.abortWith(func(binding [32]byte) bool { return binding == admission.binding() })
		if changed {
			if e == nil || !g.poison || g.loan == nil {
				t.Fatal("changed original cleared abort")
			}
		} else {
			if e != nil || g.loan != nil || g.poison {
				t.Fatal(e)
			}
			if g.close() != nil {
				t.Fatal("unchanged NoStart could not release")
			}
		}
	}
}

func TestPrivateWriterAdmissionCopiesShareSingleProducerClaim(t *testing.T) {
	g, _ := guardedConfig(t)
	admission, e := g.beginWriter(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	copy := admission
	binding, ok := admission.claim()
	if !ok || binding != admission.binding() {
		t.Fatal("initial producer claim failed")
	}
	if _, ok = copy.claim(); ok {
		t.Fatal("copied admission spawned second producer")
	}
}

func TestPrivateManagedFieldUpgradePreservesRawUnknownData(t *testing.T) {
	profile := "native-profile"
	old := filepath.Join(profile, ".orbwin-private", "runtime", "docker-"+strings.Repeat("a", 64), "docker-cli-plugins")
	current := filepath.Join(profile, ".orbwin-private", "runtime", "docker-"+strings.Repeat("b", 64), "docker-cli-plugins")
	base, _ := json.Marshal(map[string]any{"credsStore": "orbwin", "cliPluginsExtraDirs": []string{old}})
	base = append(base[:len(base)-1], []byte(`,"unknown":1e9999,"nested":{"CreDsStore":"inert-data"}}`)...)
	next, changed, e := refreshPrivateManagedField(base, current, profile)
	if e != nil || !changed || !strings.Contains(string(next), "1e9999") || privateClientRouting(next, current) != nil {
		t.Fatal(string(next), changed, e)
	}
	for _, foreign := range []string{"foreign/plugins", filepath.Join(profile, ".orbwin-private", "runtime", "docker-abc", "docker-cli-plugins")} {
		body, _ := json.Marshal(map[string]any{"credsStore": "orbwin", "cliPluginsExtraDirs": []string{foreign}})
		if _, _, e = refreshPrivateManagedField(body, current, profile); e == nil {
			t.Fatal(foreign)
		}
	}
	for _, value := range []map[string]any{{"credsStore": "foreign", "cliPluginsExtraDirs": []string{old}}, {"credsStore": "orbwin", "cliPluginsExtraDirs": []string{old, current}}} {
		body, _ := json.Marshal(value)
		if _, _, e = refreshPrivateManagedField(body, current, profile); e == nil {
			t.Fatal(value)
		}
	}
}

func TestPrivateConfigUnicodeFoldAliasesAreSchemaScoped(t *testing.T) {
	for _, raw := range []string{`{"credſStore":"foreign","credsStore":"orbwin","cliPluginsExtraDirs":["pinned"]}`, `{"credsStore":"orbwin","cliPluginsExtraDirs":["pinned"],"auths":{"r":{"identitytoKen":"x"}}}`} {
		if privateClientRouting([]byte(raw), "pinned") == nil {
			t.Fatal(raw)
		}
	}
	if e := privateClientRouting([]byte(`{"credsStore":"orbwin","cliPluginsExtraDirs":["pinned"],"HttpHeaders":{"credſStore":"inert"}}`), "pinned"); e != nil {
		t.Fatal(e)
	}
}

// Shared encoding cases with Rust managed_refresh_matches_go_json_html_and_separator_escaping.
func TestPrivateManagedRefreshCrossOwnerEscapingAndRawValues(t *testing.T) {
	profile := `C:\Users\A&B<j>`
	old := filepath.Join(profile, ".orbwin-private", "runtime", "docker-"+strings.Repeat("a", 64), "docker-cli-plugins")
	next := filepath.Join(profile, ".orbwin-private", "runtime", "docker-"+strings.Repeat("b", 64), "docker-cli-plugins")
	oldJSON, _ := json.Marshal([]string{old})
	base := []byte(`{"auths":{},"cliPluginsExtraDirs":` + string(oldJSON) + `,"credsStore":"orbwin","x<&>\u2028\u2029":{"raw":"<&>"},"unknownHuge":1e9999}`)
	refreshed, changed, err := refreshPrivateManagedField(base, next, profile)
	if err != nil || !changed {
		t.Fatal(changed, err)
	}
	for _, exact := range []string{`A\u0026B\u003cj\u003e`, `x\u003c\u0026\u003e\u2028\u2029`, `{"raw":"<&>"}`, `1e9999`} {
		if !strings.Contains(string(refreshed), exact) {
			t.Fatalf("cross-owner restoration encoding differs: missing %q", exact)
		}
	}
}

func TestWriterDurabilityFaultsStopBeforeFurtherAdmission(t *testing.T) {
	expected := []writerCommitStep{writerAdmitWritable, writerFlush, writerRecheckFlushed, writerCloseWritable, writerAdmitReadOnly}
	for failure := range expected {
		seen := []writerCommitStep{}
		e := writerDurabilitySequence(func(stage writerCommitStep) error {
			seen = append(seen, stage)
			if stage == expected[failure] {
				return errors.New("injected")
			}
			return nil
		})
		if e == nil || len(seen) != failure+1 {
			t.Fatal("fault continued", failure, seen, e)
		}
		for i := range seen {
			if seen[i] != expected[i] {
				t.Fatal("commit order", seen)
			}
		}
	}
	seen := []writerCommitStep{}
	if writerDurabilitySequence(func(stage writerCommitStep) error { seen = append(seen, stage); return nil }) != nil || len(seen) != len(expected) {
		t.Fatal("positive barrier missing", seen)
	}
}

func TestRegistryWriterPreservesRawBaselineOutsideAuths(t *testing.T) {
	base := []byte(`{"credsStore":"orbwin","cliPluginsExtraDirs":["pinned"],"auths":{"old":{}},"unknown":1e9999,"unknown":123456789012345678901234567890,"HttpHeaders":{"User-Agent":"keep","x":"a","x":"b"},"nested":{"same":1,"same":2}}`)
	child := []byte(`{"credsStore":"orbwin","cliPluginsExtraDirs":["pinned"],"auths": {"new":{"auth":"opaque"}},"HttpHeaders":{"x":"b"},"psFormat":"unowned-change"}`)
	merged, e := mergePrivateWriter(base, child, "pinned", writerRegistryAuth, []string{"new"})
	if e != nil {
		t.Fatal(e)
	}
	for _, raw := range []string{`"unknown":1e9999`, `"unknown":123456789012345678901234567890`, `"HttpHeaders":{"User-Agent":"keep","x":"a","x":"b"}`, `"nested":{"same":1,"same":2}`, `"old":{}`, `"new":{"auth":"opaque"}`} {
		if !strings.Contains(string(merged), raw) {
			t.Fatal("baseline raw value or authorized auth mutation lost")
		}
	}
	if strings.Contains(string(merged), "unowned-change") {
		t.Fatal("unowned setting was adopted")
	}
	if _, e = mergePrivateWriter(base, []byte(`{"credsStore":"orbwin","cliPluginsExtraDirs":["pinned"]}`), "pinned", writerRegistryAuth, []string{"new"}); e == nil {
		t.Fatal("missing auths accepted")
	}
	if _, e = mergePrivateWriter(base, child, "pinned", writerRootUnchanged, nil); e == nil {
		t.Fatal("non-auth writer changed root")
	}
}

func TestCommandBoundWriterScope(t *testing.T) {
	for _, command := range []string{"login", "logout", "manifest", "trust"} {
		args := []string{"--config", "private", "--host", endpoint, command}
		scope, ok := clientConfigMutation(args, "private")
		if !ok || (command == "login" || command == "logout") != (scope == writerRegistryAuth) {
			t.Fatal("wrong mutation authority", command)
		}
		args[1] = "foreign"
		if _, ok = clientConfigMutation(args, "private"); ok {
			t.Fatal("foreign config authorized")
		}
	}
	if _, ok := clientConfigMutation([]string{"--config", "private", "--host", endpoint, "context"}, "private"); ok {
		t.Fatal("context silently adopted auth authority")
	}
}

func TestRootUnchangedWriterRefusesDriftBeforeIntentClear(t *testing.T) {
	g, f := guardedConfig(t)
	if _, e := g.beginWriterScope(context.Background(), writerRootUnchanged, nil); e != nil {
		t.Fatal(e)
	}
	f.body = []byte(`{"credsStore":"orbwin","cliPluginsExtraDirs":["pinned"],"changed":true}`)
	if g.finishWith(func([32]byte) bool { return true }) == nil || !g.poison || g.loan == nil {
		t.Fatal("unexpected config drift did not retain recovery state")
	}
	if strings.Contains(strings.Join(f.calls, ","), "finish-intent") {
		t.Fatal("cleared changed root")
	}
}

func TestRegistryEntryMergePreservesEveryUnrelatedRawRecord(t *testing.T) {
	base := []byte(`{"credsStore":"orbwin","cliPluginsExtraDirs":["pinned"],"auths":{"untouched":{"auth":"opaque","unknown":1e9999,"same":1,"same":2},"target":{"unknown":"replace"},"https://target":{"old":"delete"}}}`)
	child := []byte(`{"credsStore":"orbwin","cliPluginsExtraDirs":["pinned"],"auths":{"untouched":{"auth":"opaque"},"target":{"auth":"new"},"unowned":{"auth":"injected"}}}`)
	merged, e := mergePrivateWriter(base, child, "pinned", writerRegistryAuth, []string{"target", "https://target"})
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(merged), `"untouched":{"auth":"opaque","unknown":1e9999,"same":1,"same":2}`) || !strings.Contains(string(merged), `"target":{"auth":"new"}`) || strings.Contains(string(merged), "unowned") || strings.Contains(string(merged), "https://target") {
		t.Fatal("per-registry authority or raw preservation failed")
	}
}

func TestRegistryCommandTargetNormalizationAndFullFlagForms(t *testing.T) {
	cases := []struct {
		args []string
		keys []string
	}{
		{[]string{"login"}, []string{dockerHubAuthKey, dockerHubAuthKey + "access-token", dockerHubAuthKey + "refresh-token"}},
		{[]string{"login", "--username=u", "docker.io"}, []string{dockerHubAuthKey}},
		{[]string{"login", "example:5000", "-Du=user", "--password-stdin"}, []string{"example:5000"}},
		{[]string{"login", "-uuser", "--password", "secret", "https://Example:5000/v1/"}, []string{"Example:5000"}},
		{[]string{"login", "--username", "user", "--", "-server"}, []string{"-server"}},
		{[]string{"login", "-l=debug", "-u=user", "https://[::1]:5000/path"}, []string{"[::1]:5000"}},
		{[]string{"login", "-u=user", "https://2001:db8::1:5000/path"}, []string{"[2001:db8::1]:5000"}},
		{[]string{"logout"}, []string{dockerHubAuthKey, dockerHubAuthKey + "access-token", dockerHubAuthKey + "refresh-token"}},
		{[]string{"logout", "docker.io"}, []string{"docker.io", "docker.io", "http://docker.io", "https://docker.io"}},
		{[]string{"logout", "https://Example:5000/v1/", "--debug=false"}, []string{"https://Example:5000/v1/", "Example:5000", "http://Example:5000", "https://Example:5000"}},
	}
	for _, c := range cases {
		got, e := registryCommandKeys(c.args)
		if e != nil || strings.Join(got, "\x00") != strings.Join(c.keys, "\x00") {
			t.Fatal("target normalization mismatch", c.args[0], got, e)
		}
	}
}

func TestNoopWriterStillPublishesOwnerObjectBeforeIntentClear(t *testing.T) {
	g, f := guardedConfig(t)
	if _, e := g.beginWriter(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e := g.finishWith(func([32]byte) bool { return true }); e != nil {
		t.Fatal(e)
	}
	joined := strings.Join(f.calls, ",")
	if !strings.Contains(joined, "read-after-writer,read-original,commit-merged-config,finish-intent") {
		t.Fatal("no-op writer retained child object")
	}
}

func TestWriterInvocationRequiresExactActualCommandAndManagedEnvironment(t *testing.T) {
	v := &writerInvocation{docker: filepath.Join("docker", "docker.exe"), credentials: filepath.Join("helper", "docker-credential-orbwin.exe"), config: "private", plugins: "pinned", args: []string{"--config", "private", "--host", endpoint, "login", "--username=user", "registry"}}
	makeCmd := func() *exec.Cmd {
		cmd := exec.Command(v.docker, v.args...)
		cmd.Env = []string{"DOCKER_CONFIG=private", "DOCKER_HOST=" + endpoint, "PATHEXT=.EXE", "NoDefaultCurrentDirectoryInExePath=1", "ORBWIN_DOCKER_MANAGED_PLUGINS=pinned", "PATH=helper" + string(os.PathListSeparator) + "docker", "=C:=C:\\work"}
		return cmd
	}
	if e := v.validate(makeCmd()); e != nil {
		t.Fatal(e)
	}
	for _, mutate := range []func(*exec.Cmd){func(c *exec.Cmd) { c.Path = "foreign" }, func(c *exec.Cmd) { c.Args[2] = "public" }, func(c *exec.Cmd) { c.Args = append(c.Args, "extra") }, func(c *exec.Cmd) { c.Env = append(c.Env, "path=foreign") }, func(c *exec.Cmd) { c.Env = append(c.Env, "DOCKER_CONTEXT=foreign") }, func(c *exec.Cmd) { c.Env[5] = "PATH=foreign" }, func(c *exec.Cmd) { c.Env[0] = "DOCKER_CONFIG=public" }, func(c *exec.Cmd) { c.Env[2] = "PATHEXT=.BAT" }} {
		cmd := makeCmd()
		mutate(cmd)
		if v.validate(cmd) == nil {
			t.Fatal("changed invocation admitted")
		}
	}
}
func TestWriterSelectionUsesRewrittenPrivateTuple(t *testing.T) {
	original := []string{"--config", "public", "--host", endpoint, "login", "-uuser", "registry"}
	if _, ok := clientConfigMutation(original, "private"); ok {
		t.Fatal("former public tuple admitted")
	}
	actual, e := withClientConfig(original, "public", "private")
	if e != nil {
		t.Fatal(e)
	}
	scope, ok := clientConfigMutation(actual, "private")
	if !ok || scope != writerRegistryAuth {
		t.Fatal("rewritten tuple rejected")
	}
	keys, e := registryCommandKeys(actual[4:])
	if e != nil || len(keys) != 1 || keys[0] != "registry" {
		t.Fatal("actual command target lost")
	}
}
