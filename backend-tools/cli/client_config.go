package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode"
)

var errClientConfig = errors.New("native-private Docker configuration unavailable; prepare or explicitly review legacy client settings in OrbWin desktop")
var errClientConfigRecovery = errors.New("private client configuration needs recovery; no replay or implicit reset")

const clientConfigBudget = 1 << 20

var dockerConfigFields = []string{"auths", "HttpHeaders", "psFormat", "imagesFormat", "networksFormat", "pluginsFormat", "volumesFormat", "statsFormat", "detachKeys", "credsStore", "credHelpers", "serviceInspectFormat", "servicesFormat", "tasksFormat", "secretFormat", "configFormat", "nodesFormat", "pruneFilters", "proxies", "currentContext", "cliPluginsExtraDirs", "plugins", "aliases", "features"}

func checkedClientJSON(data []byte) (any, error) { return clientJSON(data, false) }
func checkedDockerConfigJSON(data []byte) (any, error) {
	if len(data) > clientConfigBudget || !validUnicodeJSONStrings(data) {
		return nil, errClientConfig
	}
	return clientJSON(data, true)
}
func clientJSON(data []byte, config bool) (any, error) {
	if len(data) > clientConfigBudget {
		return nil, errClientConfig
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	nodes := 0
	var read func(int, int) (any, error)
	read = func(kind int, depth int) (any, error) {
		if depth > 64 || nodes > 65536 {
			return nil, errClientConfig
		}
		nodes++
		token, e := decoder.Token()
		if e != nil {
			return nil, errClientConfig
		}
		switch delim := token.(type) {
		case json.Delim:
			switch delim {
			case '{':
				result := map[string]any{}
				for decoder.More() {
					key, e := decoder.Token()
					if e != nil {
						return nil, errClientConfig
					}
					name, ok := key.(string)
					if !ok {
						return nil, errClientConfig
					}
					schema := []string(nil)
					if kind == 1 {
						schema = dockerConfigFields
					} else if kind == 3 {
						schema = []string{"username", "password", "auth", "serveraddress", "identitytoken", "registrytoken"}
					}
					known := false
					for _, canonical := range schema {
						if strings.EqualFold(canonical, name) {
							known = true
							if canonical != name {
								return nil, errClientConfig
							}
						}
					}
					if _, exists := result[name]; exists && (kind == 5 || kind == 2 || kind == 4 || known) {
						return nil, errClientConfig
					}
					child := 0
					if kind == 5 {
						child = 5
					} else if kind == 1 && name == "auths" {
						child = 2
					} else if kind == 1 && name == "credHelpers" {
						child = 4
					} else if kind == 2 {
						child = 3
					}
					value, e := read(child, depth+1)
					if e != nil {
						return nil, e
					}
					result[name] = value
				}
				end, e := decoder.Token()
				if e != nil || end != json.Delim('}') {
					return nil, errClientConfig
				}
				return result, nil
			case '[':
				var result []any
				for decoder.More() {
					value, e := read(0, depth+1)
					if e != nil {
						return nil, e
					}
					result = append(result, value)
				}
				end, e := decoder.Token()
				if e != nil || end != json.Delim(']') {
					return nil, errClientConfig
				}
				return result, nil
			}
			return nil, errClientConfig
		default:
			return token, nil
		}
	}
	kind := 5
	if config {
		kind = 1
	}
	value, e := read(kind, 0)
	if e != nil {
		return nil, e
	}
	if _, e = decoder.Token(); e != io.EOF {
		return nil, errClientConfig
	}
	return value, nil
}
func privateClientRouting(data []byte, plugins string) error {
	value, e := checkedDockerConfigJSON(data)
	if e != nil {
		return e
	}
	root, ok := value.(map[string]any)
	if !ok || root["credsStore"] != "orbwin" {
		return errClientConfig
	}
	dirs, ok := root["cliPluginsExtraDirs"].([]any)
	if !ok || len(dirs) != 1 || dirs[0] != plugins {
		return errClientConfig
	}
	if helpers, present := root["credHelpers"]; present {
		m, ok := helpers.(map[string]any)
		if !ok {
			return errClientConfig
		}
		for _, suffix := range m {
			if suffix != "orbwin" {
				return errClientConfig
			}
		}
	}
	if auths, present := root["auths"]; present && auths != nil {
		hosts, ok := auths.(map[string]any)
		if !ok {
			return errClientConfig
		}
		for _, entry := range hosts {
			if entry == nil {
				continue
			}
			auth, ok := entry.(map[string]any)
			if !ok {
				return errClientConfig
			}
			for _, key := range []string{"username", "password", "auth", "serveraddress", "identitytoken", "registrytoken"} {
				if value, exists := auth[key]; exists && value != nil {
					if _, ok = value.(string); !ok {
						return errClientConfig
					}
				}
			}
		}
	}
	return nil
}

// Caller must already hold the authenticated private root/controller and init
// provenance. This data-only helper never adopts a legacy/public configuration.
func refreshPrivateManagedField(data []byte, plugins, profile string) ([]byte, bool, error) {
	if privateClientRouting(data, plugins) == nil {
		return nil, false, nil
	}
	value, e := checkedDockerConfigJSON(data)
	if e != nil {
		return nil, false, e
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, false, errClientConfig
	}
	dirs, ok := root["cliPluginsExtraDirs"].([]any)
	if !ok || len(dirs) != 1 {
		return nil, false, errClientConfig
	}
	old, ok := dirs[0].(string)
	if !ok {
		return nil, false, errClientConfig
	}
	prefix := filepath.Join(profile, ".orbwin-private", "runtime", "docker-")
	suffix := string(os.PathSeparator) + "docker-cli-plugins"
	key, ok := strings.CutPrefix(old, prefix)
	if !ok {
		return nil, false, errClientConfig
	}
	key, ok = strings.CutSuffix(key, suffix)
	if !ok || !canonicalPin(key) || privateClientRouting(data, old) != nil {
		return nil, false, errClientConfig
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if token, e := decoder.Token(); e != nil || token != json.Delim('{') {
		return nil, false, errClientConfig
	}
	out := []byte{'{'}
	first := true
	for decoder.More() {
		key, e := decoder.Token()
		if e != nil {
			return nil, false, errClientConfig
		}
		name, ok := key.(string)
		if !ok {
			return nil, false, errClientConfig
		}
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil {
			return nil, false, errClientConfig
		}
		if !first {
			out = append(out, ',')
		}
		first = false
		encoded, _ := json.Marshal(name)
		out = append(out, encoded...)
		out = append(out, ':')
		if name == "cliPluginsExtraDirs" {
			raw, _ = json.Marshal([]string{plugins})
		}
		out = append(out, raw...)
	}
	if _, e = decoder.Token(); e != nil {
		return nil, false, errClientConfig
	}
	out = append(out, '}')
	if privateClientRouting(out, plugins) != nil {
		return nil, false, errClientConfig
	}
	return out, true, nil
}

// Preserve raw data values: Docker's typed ConfigFile.Save drops unknown roots
// and rewrites unrelated known values. Only command-bound auth entries belong
// to this loan; every other raw registry record is retained.
func mergePrivateWriter(base, incoming []byte, plugins string, scope clientWriterScope, keys []string) ([]byte, error) {
	if privateClientRouting(base, plugins) != nil || privateClientRouting(incoming, plugins) != nil {
		return nil, errClientConfigRecovery
	}
	if scope == writerRootUnchanged {
		if !bytes.Equal(base, incoming) {
			return nil, errClientConfigRecovery
		}
		return append([]byte(nil), base...), nil
	}
	if scope != writerRegistryAuth {
		return nil, errClientConfigRecovery
	}
	if bytes.Equal(base, incoming) {
		return append([]byte(nil), base...), nil
	}
	var auth json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(incoming))
	_, _ = decoder.Token() // Fully validated above; bounded JSON object.
	for decoder.More() {
		key, e := decoder.Token()
		if e != nil {
			return nil, errClientConfigRecovery
		}
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil {
			return nil, errClientConfigRecovery
		}
		if key == "auths" {
			auth = raw
		}
	}
	decoder = json.NewDecoder(bytes.NewReader(base))
	_, _ = decoder.Token()
	out := []byte{'{'}
	first, emitted := true, false
	for decoder.More() {
		key, e := decoder.Token()
		if e != nil {
			return nil, errClientConfigRecovery
		}
		var raw json.RawMessage
		if decoder.Decode(&raw) != nil {
			return nil, errClientConfigRecovery
		}
		if key == "auths" {
			if auth == nil {
				return nil, errClientConfigRecovery
			}
			var mergeErr error
			raw, mergeErr = mergeAuthEntries(raw, auth, keys)
			if mergeErr != nil {
				return nil, mergeErr
			}
			emitted = true
		}
		if !first {
			out = append(out, ',')
		}
		first = false
		encoded, e := json.Marshal(key)
		if e != nil {
			return nil, errClientConfigRecovery
		}
		out = append(out, encoded...)
		out = append(out, ':')
		out = append(out, raw...)
	}
	if !emitted && auth != nil {
		if !first {
			out = append(out, ',')
		}
		out = append(out, []byte(`"auths":`)...)
		merged, e := mergeAuthEntries(nil, auth, keys)
		if e != nil {
			return nil, e
		}
		out = append(out, merged...)
	}
	out = append(out, '}')
	if privateClientRouting(out, plugins) != nil {
		return nil, errClientConfigRecovery
	}
	return out, nil
}
func mergeAuthEntries(base, incoming []byte, keys []string) ([]byte, error) {
	allowed := func(key string) bool {
		for _, k := range keys {
			if k == key {
				return true
			}
		}
		return false
	}
	out := []byte{'{'}
	first := true
	emit := func(key string, raw []byte) {
		if !first {
			out = append(out, ',')
		}
		first = false
		encoded, _ := json.Marshal(key)
		out = append(out, encoded...)
		out = append(out, ':')
		out = append(out, raw...)
	}
	for side, data := range [][]byte{base, incoming} {
		if len(data) == 0 || bytes.Equal(data, []byte("null")) {
			continue
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		if token, e := decoder.Token(); e != nil || token != json.Delim('{') {
			return nil, errClientConfigRecovery
		}
		for decoder.More() {
			token, e := decoder.Token()
			if e != nil {
				return nil, errClientConfigRecovery
			}
			key, ok := token.(string)
			if !ok {
				return nil, errClientConfigRecovery
			}
			var raw json.RawMessage
			if decoder.Decode(&raw) != nil {
				return nil, errClientConfigRecovery
			}
			if (side == 1) == allowed(key) {
				emit(key, raw)
			}
		}
	}
	return append(out, '}'), nil
}

type writerAdmissionState struct {
	sync.Mutex
	value        [32]byte
	claimed      bool
	invocation   *writerInvocation
	birth        func(uintptr) error
	birthClaimed bool
	quiesced     bool
}

func (a writerAdmission) recordQuiesced(proof *executionQuiescedClient) {
	if a.state == nil || proof == nil {
		return
	}
	a.state.Lock()
	defer a.state.Unlock()
	if a.state.claimed && proof.consume(a.state.value) {
		a.state.quiesced = true
	}
}
func (a writerAdmission) executionQuiesced() bool {
	if a.state == nil {
		return false
	}
	a.state.Lock()
	defer a.state.Unlock()
	return a.state.quiesced
}

// Only the native producer passes its actual suspended CreateProcess handle.
// A claimed invocation alone cannot authorize ResumeThread.
func (a writerAdmission) recordBirth(handle uintptr) error {
	if a.state == nil || handle == 0 {
		return errClientConfigRecovery
	}
	a.state.Lock()
	defer a.state.Unlock()
	if !a.state.claimed || a.state.invocation == nil || a.state.birthClaimed || a.state.birth == nil {
		return errClientConfigRecovery
	}
	a.state.birthClaimed = true
	return a.state.birth(handle)
}

type writerAdmission struct{ state *writerAdmissionState }

// Invocation is fixed before the durable loan; validation rechecks the actual
// retained cache capabilities at the producer boundary, never a caller hash.
type writerInvocation struct {
	docker, credentials, config, plugins string
	args                                 []string
}

func (a writerAdmission) validateInvocation(cmd *exec.Cmd) error {
	if a.state == nil || cmd == nil {
		return errClientConfigRecovery
	}
	a.state.Lock()
	defer a.state.Unlock()
	expected := a.state.invocation
	if !a.state.claimed || expected == nil {
		return errClientConfigRecovery
	}
	docker, e := existingCache("docker")
	if e != nil || docker != expected.docker {
		return errClientConfigRecovery
	}
	credentials, e := existingCache("credentials")
	if e != nil || credentials != expected.credentials {
		return errClientConfigRecovery
	}
	return expected.validate(cmd)
}
func (v *writerInvocation) validate(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Path != v.docker || !slices.Equal(cmd.Args, append([]string{v.docker}, v.args...)) {
		return errClientConfigRecovery
	}
	env := map[string]string{}
	for _, entry := range cmd.Env {
		key, value, ok := strings.Cut(entry, "=")
		if strings.HasPrefix(entry, "=") && len(entry) >= 4 && entry[2] == ':' && entry[3] == '=' && ((entry[1] >= 'A' && entry[1] <= 'Z') || (entry[1] >= 'a' && entry[1] <= 'z')) {
			key, value, ok = entry[:3], entry[4:], true
		}
		key = strings.ToUpper(key)
		if !ok || key == "" || strings.ContainsRune(entry, 0) {
			return errClientConfigRecovery
		}
		if _, exists := env[key]; exists {
			return errClientConfigRecovery
		}
		env[key] = value
	}
	for key, value := range map[string]string{"DOCKER_CONFIG": v.config, "DOCKER_HOST": endpoint, "PATHEXT": ".EXE", "NODEFAULTCURRENTDIRECTORYINEXEPATH": "1", "ORBWIN_DOCKER_MANAGED_PLUGINS": v.plugins} {
		if env[key] != value {
			return errClientConfigRecovery
		}
	}
	for _, key := range []string{"DOCKER_CONTEXT", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH", "DOCKER_API_VERSION", "DOCKER_AUTH_CONFIG"} {
		if _, exists := env[key]; exists {
			return errClientConfigRecovery
		}
	}
	prefix := filepath.Dir(v.credentials) + string(os.PathListSeparator) + filepath.Dir(v.docker)
	if env["PATH"] != prefix && !strings.HasPrefix(env["PATH"], prefix+string(os.PathListSeparator)) {
		return errClientConfigRecovery
	}
	return nil
}

func (a writerAdmission) binding() [32]byte {
	if a.state == nil {
		return [32]byte{}
	}
	a.state.Lock()
	defer a.state.Unlock()
	return a.state.value
}
func (a writerAdmission) claim() ([32]byte, bool) {
	if a.state == nil {
		return [32]byte{}, false
	}
	a.state.Lock()
	defer a.state.Unlock()
	if a.state.claimed || a.state.value == [32]byte{} {
		return [32]byte{}, false
	}
	a.state.claimed = true
	return a.state.value, true
}

type clientConfigNative interface {
	check() error
	beginWriter([32]byte, clientWriterScope, []string, []byte) error
	readOriginal() ([]byte, error)
	readAfterWriter(string) ([]byte, error)
	commitMergedConfig([]byte) error
	finishIntent() error
	abortUnstarted() ([]byte, error)
	close() error
}
type clientConfigGuard struct {
	native        clientConfigNative
	path, plugins string
	body          []byte
	loan          *writerAdmission
	poison        bool
	scope         clientWriterScope
	keys          []string
}

func (g *clientConfigGuard) beginWriter(ctx context.Context) (writerAdmission, error) {
	return g.beginWriterScope(ctx, writerRootUnchanged, nil)
}
func (g *clientConfigGuard) beginWriterForCommand(ctx context.Context, args []string) (writerAdmission, error) {
	if g == nil {
		return writerAdmission{}, errClientConfigRecovery
	}
	scope, ok := clientConfigMutation(args, g.path)
	if !ok {
		return writerAdmission{}, errClientConfig
	}
	var keys []string
	if scope == writerRegistryAuth {
		var e error
		keys, e = registryCommandKeys(args[4:])
		if e != nil {
			return writerAdmission{}, e
		}
	}
	docker, e := existingCache("docker")
	if e != nil {
		return writerAdmission{}, e
	}
	credentials, e := existingCache("credentials")
	if e != nil {
		return writerAdmission{}, e
	}
	admission, e := g.beginWriterScope(ctx, scope, keys)
	if e == nil {
		admission.state.invocation = &writerInvocation{docker: docker, credentials: credentials, config: g.path, plugins: g.plugins, args: append([]string(nil), args...)}
	}
	return admission, e
}
func (g *clientConfigGuard) beginWriterScope(ctx context.Context, scope clientWriterScope, keys []string) (writerAdmission, error) {
	var admission writerAdmission
	if ctx == nil || ctx.Err() != nil || g == nil || g.native == nil || g.poison || g.loan != nil {
		return admission, errClientConfigRecovery
	}
	if g.native.check() != nil || privateClientRouting(g.body, g.plugins) != nil {
		return admission, errClientConfig
	}
	if len(keys) > 8 {
		return admission, errClientConfigRecovery
	}
	for _, key := range keys {
		if key == "" || len(key) > 2048 || strings.ContainsFunc(key, unicode.IsControl) {
			return admission, errClientConfigRecovery
		}
	}
	keys = append([]string{}, keys...)
	slices.Sort(keys)
	keys = slices.Compact(keys)
	var binding [32]byte
	if _, e := rand.Read(binding[:]); e != nil || binding == [32]byte{} {
		return admission, errClientConfig
	}
	admission.state = &writerAdmissionState{value: binding}
	g.poison = true
	g.loan = &admission
	g.scope = scope
	g.keys = append([]string{}, keys...)
	if e := g.native.beginWriter(binding, scope, g.keys, g.body); e != nil {
		return admission, errClientConfigRecovery
	}
	if native, ok := g.native.(interface{ recordWriterBirth(uintptr, [32]byte) error }); ok {
		admission.state.birth = func(h uintptr) error { return native.recordWriterBirth(h, binding) }
	}
	g.poison = false
	return admission, nil
}

// The wrapper accepts ONLY the runner's unexported native retirement token.
// Generic core is private and exists for fault tests; no JSON/bool constructor.
func (g *clientConfigGuard) finishWriter(proof *retiredClientTree) error {
	if proof == nil {
		return errClientConfigRecovery
	}
	return g.finishWith(proof.consume)
}
func (g *clientConfigGuard) finishWith(consume func([32]byte) bool) error {
	if g == nil || g.native == nil || g.poison || g.loan == nil {
		return errClientConfigRecovery
	}
	g.poison = true
	if !consume(g.loan.binding()) || g.native.check() != nil {
		return errClientConfigRecovery
	}
	body, e := g.native.readAfterWriter(g.plugins)
	if e != nil || privateClientRouting(body, g.plugins) != nil {
		return errClientConfigRecovery
	}
	original, e := g.native.readOriginal()
	if e != nil || !bytes.Equal(original, g.body) {
		return errClientConfigRecovery
	}
	merged, e := mergePrivateWriter(original, body, g.plugins, g.scope, g.keys)
	if e != nil {
		return errClientConfigRecovery
	}
	// Every successful writer settlement creates a new owner-authored object,
	// even for byte-identical output. Old child references cannot address it.
	if e = g.native.commitMergedConfig(merged); e != nil {
		return errClientConfigRecovery
	}
	if e = g.native.finishIntent(); e != nil {
		return errClientConfigRecovery
	}
	g.body = merged
	g.loan = nil
	g.poison = false
	return nil
}
func (g *clientConfigGuard) abortUnstarted(proof *neverStartedClient) error {
	if proof == nil {
		return errClientConfigRecovery
	}
	return g.abortWith(proof.consume)
}
func (g *clientConfigGuard) abortWith(consume func([32]byte) bool) error {
	if g == nil || g.native == nil || g.poison || g.loan == nil {
		return errClientConfigRecovery
	}
	g.poison = true
	if !consume(g.loan.binding()) {
		return errClientConfigRecovery
	}
	original, e := g.native.readOriginal()
	if e != nil || !bytes.Equal(original, g.body) {
		return errClientConfigRecovery
	}
	body, e := g.native.abortUnstarted()
	if e != nil || !bytes.Equal(body, g.body) || privateClientRouting(body, g.plugins) != nil {
		return errClientConfigRecovery
	}
	g.loan = nil
	g.poison = false
	return nil
}
func (g *clientConfigGuard) close() error {
	if g == nil {
		return nil
	}
	if g.poison || g.loan != nil {
		if g.native != nil && g.loan != nil && g.loan.executionQuiesced() {
			if native, ok := g.native.(interface{ releaseRecoveryOwner() error }); ok && native.releaseRecoveryOwner() == nil {
				g.native = nil
				g.loan = nil
				g.poison = true
			}
		}
		return errClientConfigRecovery
	}
	if g.native == nil {
		return errClientConfig
	}
	if e := g.native.close(); e != nil {
		g.poison = true
		return errClientConfigRecovery
	}
	g.native = nil
	return nil
}

// Clearing writer.intent is outside this barrier and cannot occur on any fault.
type writerCommitStep uint8

const (
	writerAdmitWritable writerCommitStep = iota
	writerFlush
	writerRecheckFlushed
	writerCloseWritable
	writerAdmitReadOnly
)

func writerDurabilitySequence(step func(writerCommitStep) error) error {
	for _, stage := range []writerCommitStep{writerAdmitWritable, writerFlush, writerRecheckFlushed, writerCloseWritable, writerAdmitReadOnly} {
		if e := step(stage); e != nil {
			return e
		}
	}
	return nil
}

// A durable private data record is never loaned until its writable handle has
// positively closed and a separate read-only capability verifies the exact bytes.
// Failures stop here; the native owner retains all acquired resources/evidence.
// The small interface also admits real stateful fault models without Windows IO.
type privateRecordIO interface {
	createNew() error
	writeAll([]byte) error
	flush() error
	readWritten() ([]byte, error)
	closeWriter() error
	openReadOnly() error
	readHeld() ([]byte, error)
}

func durablePrivateRecord(data []byte, record privateRecordIO) error {
	if len(data) > clientConfigBudget {
		return errClientConfigRecovery
	}
	if e := record.createNew(); e != nil {
		return e
	}
	if e := record.writeAll(data); e != nil {
		return e
	}
	if e := record.flush(); e != nil {
		return e
	}
	written, e := record.readWritten()
	if e != nil || !bytes.Equal(written, data) {
		return errClientConfigRecovery
	}
	if e := record.closeWriter(); e != nil {
		return e
	}
	if e := record.openReadOnly(); e != nil {
		return e
	}
	held, e := record.readHeld()
	if e != nil || !bytes.Equal(held, data) {
		return errClientConfigRecovery
	}
	return nil
}

// The exact-byte original can survive without an intent after a crash during
// birth or final cleanup. Presence is never permission to import/delete/resume.
func refusePendingPrivateRecords(probe func(string) (bool, error)) error {
	for _, name := range []string{"writer.prepare", "writer.intent", "writer.original", "writer.birth", "recovery.intent", "recovery.next", "snapshot.intent"} {
		present, e := probe(name)
		if e != nil || present {
			return errClientConfigRecovery
		}
	}
	return nil
}
func finishPrivateMutation(finalCheck, clearIntent, clearOriginal func() error) error {
	if e := finalCheck(); e != nil {
		return e
	}
	if e := clearIntent(); e != nil {
		return e
	}
	return clearOriginal()
}

func writerKeysJSON(keys []string) ([]byte, error) { return json.Marshal(append([]string{}, keys...)) }
