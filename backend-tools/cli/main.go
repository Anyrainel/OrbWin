// OrbWin CLI is a client, never a supervisor. rdd owns desired state and
// convergence; cancellation terminates only the current client. An interrupted
// set may already be accepted and must be observed before another request.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

const endpoint = "npipe:////./pipe/docker-rancher-desktop-orbwin"

var sourceCommit, dockerSHA, version string // populated from reviewed locks by build-cli.mjs
type identity struct {
	Endpoint     string `json:"endpoint"`
	Home         string `json:"home"`
	DaemonID     string `json:"daemonId"`
	SourceCommit string `json:"sourceCommit"`
	BinarySHA    string `json:"binarySha256"`
	Verified     bool   `json:"identityVerified"`
}
type target struct {
	identity
	runtime, config                                 string
	authority, dockerDirectory, credentialDirectory string
	buildxConfig                                    string
}

func readJSON(path string, value any) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, 1<<20+1))
	if e != nil {
		return e
	}
	if len(b) > 1<<20 {
		return errors.New("metadata exceeds 1 MiB")
	}
	return json.Unmarshal(b, value)
}
func digest(path string, limit int64) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(f, limit+1))
	if e != nil {
		return "", e
	}
	if n > limit {
		return "", errors.New("binary exceeds size limit")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func loadTarget() (target, error) {
	var t target
	exe, e := os.Executable()
	if e != nil {
		return t, e
	}
	t.runtime = filepath.Join(filepath.Dir(exe), "runtime")
	profile, local, e := nativeLocations()
	if e != nil {
		return t, e
	}
	t.authority = filepath.Join(profile, ".orbwin-private", "authority")
	state := filepath.Join(local, "OrbWinData")
	if e = readJSON(filepath.Join(state, "runtime.json"), &t.identity); e != nil {
		return t, errors.New("owned engine identity unavailable; prepare OrbWin in the desktop application")
	}
	allowed := filepath.Join(state, "home")
	if !strings.EqualFold(filepath.Clean(t.Home), allowed) || !t.Verified || t.Endpoint != endpoint || t.DaemonID == "" || len(t.DaemonID) > 256 || sourceCommit == "" || t.SourceCommit != sourceCommit || !canonicalPin(rddSHA) || t.BinarySHA != rddSHA {
		return t, errors.New("owned engine identity differs from this CLI; no request sent")
	}
	t.Home = allowed
	t.config = filepath.Join(profile, ".orbwin-private", "client", "docker")
	return t, nil
}
func (t target) docker() (string, error) { return existingCache("docker") }
func (t target) rdd() (string, error) {
	p, e := existingCache("rdd")
	if e != nil {
		return "", e
	}
	if e = verifyRegistration(t.Home); e != nil {
		return "", e
	}
	return p, nil
}
func cleanEnv(t target, management bool) []string {
	blocked := map[string]bool{}
	for _, k := range []string{"DOCKER_HOST", "DOCKER_CONFIG", "DOCKER_CONTEXT", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH", "DOCKER_API_VERSION", "BUILDKIT_HOST", "BUILDX_BUILDER", "BUILDX_CONFIG", "RDD_INSTANCE", "RDD_LOG_DIR", "RDD_DEVELOPER_MODE", "RDD_KEEP_LOGS", "LIMA_HOME", "KUBECONFIG", "COMPOSE_FILE", "COMPOSE_PROFILES", "COMPOSE_PROJECT_NAME", "COMPOSE_ENV_FILES", "COMPOSE_REMOVE_ORPHANS", "ORBWIN_DOCKER_MANAGED_PLUGINS"} {
		blocked[k] = true
	}
	if management {
		blocked["HOME"] = true
		blocked["USERPROFILE"] = true
	}
	blocked["NODEFAULTCURRENTDIRECTORYINEXEPATH"] = true
	blocked["PATHEXT"] = true
	env := []string{}
	for _, entry := range os.Environ() {
		k, _, _ := strings.Cut(entry, "=")
		if !blocked[strings.ToUpper(k)] {
			env = append(env, entry)
		}
	}
	if t.dockerDirectory != "" {
		env = append(env, "ORBWIN_DOCKER_MANAGED_PLUGINS="+filepath.Join(t.dockerDirectory, "docker-cli-plugins"))
	}
	// Windows Go exec checks CWD before PATH unless this variable is present.
	// Keep suffix lookup on the actual retained .exe helper, not an inherited
	// PATHEXT which could skip it and select a later ambient helper.
	env = append(env, "NoDefaultCurrentDirectoryInExePath=1", "PATHEXT=.EXE")
	env = append(env, "DOCKER_HOST="+endpoint, "DOCKER_CONFIG="+t.config, "BUILDX_BUILDER=default", "BUILDX_CONFIG="+clientBuildxConfig(t), "COMPOSE_REMOVE_ORPHANS=false")
	if management {
		env = append(env, "HOME="+t.Home, "USERPROFILE="+t.Home, "RDD_INSTANCE=orbwin", "RDD_DEVELOPER_MODE=1", "RDD_KEEP_LOGS=1", "KUBECONFIG="+filepath.Join(t.Home, ".rdorbwin", "kube-client.config"))
	}
	// Credential helpers and CLI plugins are installation resources. This is a
	// child-only PATH; the user's persistent PATH is never written.
	pathSet := false
	for i, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, "PATH") && t.credentialDirectory != "" && t.dockerDirectory != "" {
			env[i] = key + "=" + t.credentialDirectory + string(os.PathListSeparator) + t.dockerDirectory + string(os.PathListSeparator) + value
			pathSet = true
		}
	}
	if !pathSet && t.credentialDirectory != "" && t.dockerDirectory != "" {
		env = append(env, "PATH="+t.credentialDirectory+string(os.PathListSeparator)+t.dockerDirectory)
	}
	return env
}

type limitedBuffer struct {
	b     []byte
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(b.b)+len(p) > b.limit {
		return 0, errors.New("client output exceeds limit")
	}
	b.b = append(b.b, p...)
	return len(p), nil
}
func run(ctx context.Context, t target, bin string, args []string, management, stream bool) (result []byte, runErr error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, errors.New("client cancelled before dispatch")
	}
	var configGuard *clientConfigGuard
	var admission writerAdmission
	if !management && t.authority != "" {
		original := t.config
		if _, e := withClientConfig(args, original, original); e != nil {
			return nil, e
		}
		if clientConfigWriter(args, original) {
			var e error
			configGuard, e = acquirePrivateClientConfig(ctx)
			if e != nil {
				return nil, e
			}
			defer func() { runErr = errors.Join(runErr, configGuard.close()) }()
			t.config = configGuard.path
			t.buildxConfig = filepath.Join(configGuard.path, "buildx")
		} else {
			snapshot, e := acquirePrivateClientSnapshot(ctx)
			if e != nil {
				return nil, e
			}
			t.config = snapshot.path
			t.buildxConfig = snapshot.buildx
		}
		var e error
		args, e = withClientConfig(args, original, t.config)
		if e != nil {
			return nil, e
		}
	}
	if t.authority != "" {
		credentials, e := existingCache("credentials")
		if e != nil {
			return nil, e
		}
		docker, e := existingCache("docker")
		if e != nil {
			return nil, e
		}
		t.credentialDirectory = filepath.Dir(credentials)
		t.dockerDirectory = filepath.Dir(docker)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = cleanEnv(t, management)
	cmd.WaitDelay = 2 * time.Second
	b := &limitedBuffer{limit: 2 << 20}
	if stream {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stdout = b
		cmd.Stderr = io.Discard
	}
	var e error
	noDispatch := false
	if configGuard != nil {
		// All fallible selector/cache preparation precedes the durable loan.
		// Bind the actual rewritten argv immediately before its native producer.
		admission, e = configGuard.beginWriterForCommand(ctx, args)
		if e != nil {
			return nil, e
		}
		proof, clientErr := runClientWriter(ctx, cmd, admission)
		if proof != nil {
			e = errors.Join(clientErr, configGuard.finishWriter(proof))
		} else {
			var startErr *writerStartError
			if errors.As(clientErr, &startErr) {
				if unstarted := startErr.takeNeverStarted(); unstarted != nil {
					abortErr := configGuard.abortUnstarted(unstarted)
					noDispatch = abortErr == nil
					e = errors.Join(clientErr, abortErr)
				} else {
					e = errors.Join(clientErr, errClientConfigRecovery)
				}
			} else {
				e = errors.Join(clientErr, errClientConfigRecovery)
			}
		}
	} else {
		e = runClientCommand(ctx, cmd, management && exactDetachedStart(args))
	}
	if e != nil {
		if ctx.Err() != nil {
			if noDispatch {
				return nil, fmt.Errorf("client cancelled before this Docker process started; no request from it was sent: %w", errors.Join(ctx.Err(), e))
			}
			return nil, fmt.Errorf("client interrupted; a dispatched mutation may still converge; inspect before retry: %w", errors.Join(ctx.Err(), e))
		}
		return nil, fmt.Errorf("targeted client failed: %w", e)
	}
	return b.b, nil
}
func (t target) dockerArgs(args ...string) []string {
	return append([]string{"--config", t.config, "--host", endpoint}, args...)
}
func (t target) check(ctx context.Context) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	p, e := t.docker()
	if e != nil {
		return nil, e
	}
	raw, e := run(ctx, t, p, t.dockerArgs("info", "--format", "{{json .}}"), false, false)
	if e != nil {
		return nil, e
	}
	var info map[string]any
	if e = json.Unmarshal(raw, &info); e != nil {
		return nil, e
	}
	if info["ID"] != t.DaemonID || info["OSType"] != "linux" {
		return nil, errors.New("owned Linux daemon identity mismatch; request refused")
	}
	return info, nil
}
func safeInfo(info map[string]any) map[string]any {
	out := map[string]any{}
	for _, k := range []string{"ID", "ServerVersion", "OSType", "Architecture", "NCPU", "MemTotal", "Containers", "ContainersRunning", "ContainersPaused", "ContainersStopped", "Images"} {
		out[k] = info[k]
	}
	return out
}
func dockerGuard(args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return errors.New("supply a Docker command, without global options")
	}
	// Target flags can appear after subcommands and plugins have their own parsers.
	// Refuse them before verified run/create/exec payload boundaries, and throughout
	// other commands whose positional boundary is not adapted here.
	commandName := args[0]
	commandArgs := args[1:]
	if commandName == "container" && len(commandArgs) > 0 {
		commandName = commandArgs[0]
		commandArgs = commandArgs[1:]
	}
	payload := len(args)
	if flags, known := dockerCommandFlags[commandName]; known {
		if relative, ok := commandPayloadIndex(commandArgs, flags); ok {
			payload = len(args) - len(commandArgs) + relative
		}
	}
	for i, a := range args {
		// Docker run/create/exec stop flag parsing after IMAGE/CONTAINER.
		// Their payload can contain /bin/sh -c or literal --host arguments.
		if i >= payload {
			break
		}
		if flags, known := dockerCommandFlags[commandName]; known && shortTargetCluster(a, flags) {
			return errors.New("Docker target overrides inside short-option clusters are not accepted")
		}
		// The selected CLI also has a hidden --cloud[=NAME] resolver.
		// Its current host-conflict check refuses our fixed --host, but keep
		// external context provisioning outside this owned-engine adapter.
		for _, flag := range []string{"--host", "--context", "--cloud", "--config", "--tls", "--tlscacert", "--tlscert", "--tlskey", "--tlsverify", "--builder"} {
			if a == flag || strings.HasPrefix(a, flag+"=") {
				return errors.New("Docker target/security overrides are not accepted")
			}
		}
		// run/create own -c as --cpu-shares, shadowing the inherited global
		// context shorthand. After IMAGE it is also an ordinary command argument.
		if strings.HasPrefix(a, "-H") || (strings.HasPrefix(a, "-c") && commandName != "run" && commandName != "create") {
			return errors.New("Docker target overrides are not accepted")
		}
	}
	if args[0] == "context" {
		return errors.New("context management uses separately reviewed explicit clients")
	}
	if args[0] == "buildx" && (len(args) < 2 || (args[1] != "build" && args[1] != "version")) {
		if len(args) < 2 || args[1] != "inspect" {
			return errors.New("only default-target Buildx build/version/inspect are accepted; builder management is separate")
		}
		for i := 2; i < len(args); i++ {
			if args[i] == "default" {
				continue
			}
			if args[i] == "--format" && i+1 < len(args) {
				i++
				continue
			}
			if strings.HasPrefix(args[i], "--format=") {
				continue
			}
			return errors.New("Buildx inspect accepts only the default builder and formatting; bootstrap/foreign selectors are refused")
		}
	}
	return nil
}
func shortTargetCluster(arg string, flags map[string]bool) bool {
	if !strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--") {
		return false
	}
	for i := 1; i < len(arg); i++ {
		short := "-" + arg[i:i+1]
		value, local := flags[short]
		if arg[i] == 'H' || (arg[i] == 'c' && !local) {
			return true
		}
		if local && value {
			return false
		}
	}
	return false
}
func commandPayloadIndex(args []string, flags map[string]bool) (int, bool) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			if i+1 < len(args) {
				return i + 2, true
			}
			return 0, false
		}
		if !strings.HasPrefix(a, "-") {
			return i + 1, true
		}
		key, _, equals := strings.Cut(a, "=")
		if strings.HasPrefix(a, "--") {
			requiresValue, known := flags[key]
			if !known {
				return 0, false
			}
			if !requiresValue {
				continue
			}
			if !equals {
				i++
				if i >= len(args) {
					return 0, false
				}
			}
			continue
		}
		for short := 1; short < len(a); short++ {
			requiresValue, known := flags["-"+a[short:short+1]]
			if !known {
				return 0, false
			}
			if requiresValue {
				if short == len(a)-1 {
					i++
					if i >= len(args) {
						return 0, false
					}
				}
				break
			}
		}
	}
	return 0, false
}
func configSet(args []string) (string, error) {
	if len(args) != 1 {
		return "", errors.New("use config set running=true|false or kubernetes.enabled=true|false")
	}
	k, v, ok := strings.Cut(args[0], "=")
	if !ok || (k != "running" && k != "kubernetes.enabled") || (v != "true" && v != "false") {
		return "", errors.New("unsupported setting; this CLI only changes running or kubernetes.enabled")
	}
	return args[0], nil
}
func (t target) settings(ctx context.Context) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	p, e := t.rdd()
	if e != nil {
		return nil, e
	}
	b, e := run(ctx, t, p, []string{"kubectl", "--kubeconfig", filepath.Join(t.authority, "config.yaml"), "get", "apps/app", "--output=json", "--request-timeout=10s"}, true, false)
	if e != nil {
		return nil, e
	}
	var app map[string]any
	if e = json.Unmarshal(b, &app); e != nil {
		return nil, e
	}
	spec, ok := app["spec"].(map[string]any)
	if !ok {
		return nil, errors.New("management settings have no spec")
	}
	return spec, nil
}
func (t target) set(ctx context.Context, value string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	p, e := t.rdd()
	if e != nil {
		return e
	}
	var serviceArgs []string
	if e = readJSON(filepath.Join(t.authority, "args.json"), &serviceArgs); e != nil {
		return errors.New("owned management service configuration unavailable; prepare OrbWin before starting it")
	}
	if e = validateServiceArgs(serviceArgs); e != nil {
		return e
	}
	probe := func(ctx context.Context) (managementObservation, error) { return t.serviceObservation(ctx, p) }
	return desiredState(ctx, value, t.settings, func(ctx context.Context, args []string) error { _, e := run(ctx, t, p, args, true, true); return e }, func(ctx context.Context) error { _, e := t.check(ctx); return e }, func(ctx context.Context) error {
		status, e := probe(ctx)
		if e != nil {
			return e
		}
		if status.State != "stopped" {
			return errColdStartUnqualified
		}
		return errManagementUnavailable
	}, func(ctx context.Context) error {
		status, e := probe(ctx)
		if e != nil {
			return e
		}
		if status.State != "running" {
			return errColdStartUnqualified
		}
		return nil
	})
}
func (t target) serviceObservation(ctx context.Context, p string) (managementObservation, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	directory := t.authority
	actual, e := filepath.EvalSymlinks(directory)
	if e != nil || !strings.EqualFold(actual, directory) {
		return managementObservation{}, errColdStartUnqualified
	}
	executable, e := filepath.EvalSymlinks(p)
	if e != nil {
		return managementObservation{}, errColdStartUnqualified
	}
	return decodeManagementObservation(ctx, actual, executable, func(ctx context.Context) ([]byte, error) {
		return run(ctx, t, p, []string{"service", "status", "--output=json"}, true, false)
	})
}
func (t target) managementObservation(ctx context.Context) (managementObservation, error) {
	p, e := t.rdd()
	if e != nil {
		return managementObservation{}, e
	}
	return t.serviceObservation(ctx, p)

}

const help = `OrbWin CLI (explicit owned engine; no PATH/profile/context edits)
  status                   Sanitized engine status (no auto-start)
  start | stop             Request rdd desired state; stop interrupts workloads
  logs                     Print the owned backend log directory; do not read logs
  snapshot NEW.json        Save a bounded, allowlisted diagnostic snapshot
  docker COMMAND ...       Run bundled Docker against the verified owned endpoint
  config get [KEY]         Read running / kubernetes.enabled (no auto-start)
  config set KEY=BOOL       Set one supported backend setting
  top                      One stats sample through the targeted Docker client
  clients recover          Review an interrupted private Docker settings write
  clients recover --apply-reviewed
                           Apply the current recovery review; never replay auth
  clients recover --restore-original
                           Restore a saved original or finish its cleanup
  feedback                 Issue guidance; no upload or browser launch
  update                   Explain the release-feed/Prepare update boundary
PowerShell completion is opt-in: dot-source completions/OrbWin.ps1 for this session.
Cancellation stops this client, not rdd; inspect status before retrying a mutation.
`

func execute(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Print(help)
		return nil
	}
	if args[0] == "feedback" && len(args) == 1 {
		fmt.Println("OrbWin's repository is private: https://github.com/Anyrainel/OrbWin . Public/private reporting policy and issue-channel availability are pending. Use a project support channel you can access. Create a snapshot with `orbwin-cli snapshot NEW.json`, review it, then attach it yourself. Do not include credentials or raw logs without review. Nothing was opened or uploaded.")
		return nil
	}
	if args[0] == "update" && len(args) == 1 {
		fmt.Println("No trusted OrbWin release feed is configured. Automatic update availability is deferred to Phase 4. Use Prepare update in the desktop app for a reviewed local candidate.")
		return nil
	}
	// Config recovery must remain available when the engine identity is absent
	// or the engine is stopped. The native owner obtains its own fixed locations.
	if args[0] == "clients" {
		if len(args) < 2 || args[1] != "recover" || len(args) > 3 || (len(args) == 3 && args[2] != "--restore-original" && args[2] != "--apply-reviewed") {
			return errors.New("use clients recover [--apply-reviewed | --restore-original]")
		}
		review, err := preparePrivateWriterRecovery(ctx)
		if err != nil {
			if errors.Is(err, errClientConfigNoRecovery) {
				fmt.Println("No interrupted private Docker settings write found.")
				return nil
			}
			return err
		}
		if len(args) == 2 {
			fmt.Println(review.summary())
			fmt.Println("Run clients recover --apply-reviewed to review again and apply recovery. This does not replay a login or undo credential-store changes.")
			return review.close()
		}
		if args[2] == "--restore-original" && !review.acceptsRestoreOriginal() {
			return errors.Join(errors.New("this recovery has no saved original; use clients recover --apply-reviewed for its reviewed action"), review.close())
		}
		fmt.Println(review.summary())
		guard, err := review.restore(ctx)
		if err != nil {
			return errors.Join(err, review.close())
		}
		if err = guard.close(); err != nil {
			return err
		}
		fmt.Println("Private Docker settings recovery completed. Refresh saved registry credentials before signing in again.")
		return nil
	}
	t, e := loadTarget()
	if e != nil {
		return e
	}
	emit := func(v any) error { return json.NewEncoder(os.Stdout).Encode(v) }
	switch args[0] {
	case "status":
		if len(args) != 1 {
			break
		}
		report, e := managementStatus(ctx, t.check, t.managementObservation)
		if report != nil {
			if outErr := emit(report); outErr != nil {
				return outErr
			}
		}
		return e
	case "logs":
		if len(args) != 1 {
			break
		}
		return emit(map[string]string{"directory": filepath.Join(t.Home, "AppData", "Local", "rancher-desktop-orbwin-logs"), "note": "Log contents may contain private data. Review before sharing."})
	case "snapshot":
		if len(args) != 2 || !strings.EqualFold(filepath.Ext(args[1]), ".json") {
			break
		}
		info, e := t.check(ctx)
		status := "available"
		if e != nil {
			status = "unavailable"
			info = nil
		}
		v := map[string]any{"schema": 1, "product": "OrbWin", "version": version, "sourceCommit": sourceCommit, "createdAt": time.Now().UTC().Format(time.RFC3339), "endpoint": endpoint, "status": status, "engine": safeInfo(info)}
		b, e := json.MarshalIndent(v, "", "  ")
		if e != nil {
			return e
		}
		f, e := os.OpenFile(args[1], os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		_, e = f.Write(append(b, '\n'))
		if e == nil {
			e = f.Sync()
		}
		e = errors.Join(e, f.Close())
		if e != nil {
			return fmt.Errorf("snapshot incomplete; retain/review the new file: %w", e)
		}
		return nil
	case "start", "stop":
		if len(args) != 1 {
			break
		}
		return t.set(ctx, fmt.Sprintf("running=%t", args[0] == "start"))
	case "docker", "top":
		a := args[1:]
		if args[0] == "top" {
			if len(a) != 0 {
				break
			}
			a = []string{"stats", "--no-stream"}
		}
		if e = dockerGuard(a); e != nil {
			return e
		}
		if _, e = t.check(ctx); e != nil {
			return e
		}
		p, e := t.docker()
		if e != nil {
			return e
		}
		_, e = run(ctx, t, p, t.dockerArgs(a...), false, true)
		return e
	case "config":
		if len(args) < 2 {
			break
		}
		if args[1] == "set" {
			v, e := configSet(args[2:])
			if e != nil {
				return e
			}
			return t.set(ctx, v)
		}
		if args[1] == "get" && len(args) <= 3 {
			spec, e := classifiedSettings(ctx, t.settings, t.managementObservation)
			if e != nil {
				return e
			}
			k8s, _ := spec["kubernetes"].(map[string]any)
			values := map[string]any{"running": spec["running"], "kubernetes.enabled": k8s["enabled"]}
			if len(args) == 3 {
				v, ok := values[args[2]]
				if !ok {
					return errors.New("unsupported config key")
				}
				return emit(v)
			}
			return emit(values)
		}
	}
	return errors.New("invalid command; use --help")
}
func invocationArgs(executable string, args []string) []string {
	if strings.EqualFold(filepath.Base(executable), "orbwin-docker.exe") || strings.EqualFold(filepath.Base(executable), "orbwin-docker") {
		return append([]string{"docker"}, args...)
	}
	return args
}
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if e := execute(ctx, invocationArgs(os.Args[0], os.Args[1:])); e != nil {
		fmt.Fprintln(os.Stderr, "OrbWin:", e)
		os.Exit(1)
	}
}
