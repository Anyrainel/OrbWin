//go:build linux

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const socket = "unix:///mnt/wsl/orbwin/docker.sock"
const configLimit = 16 << 20

func identity(path string) (string, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !stat.IsDir() {
		return "", errors.New("project root is not a directory")
	}
	value, ok := stat.Sys().(*syscall.Stat_t)
	if !ok {
		return "", errors.New("project inode metadata unavailable")
	}
	return fmt.Sprintf("%d:%d", value.Dev, value.Ino), nil
}
func (request workflowRequest) verifyProject() error {
	if !strings.HasPrefix(request.Source, "/home/") || filepath.Clean(request.Source) != request.Source || !regexp.MustCompile(`^/mnt/wsl/orbwin/projects/[a-f0-9]{24}$`).MatchString(request.Shared) || request.Daemon == "" || !regexp.MustCompile(`^[0-9]+:[0-9]+$`).MatchString(request.Signature) {
		return errors.New("invalid recorded project boundary")
	}
	original, err := identity(request.Source)
	if err != nil {
		return err
	}
	shared, err := identity(request.Shared)
	if err != nil {
		return err
	}
	if original != request.Signature || shared != request.Signature {
		return errors.New("project source/export changed or disconnected; refresh and explicitly reconnect")
	}
	table, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return err
	}
	defer table.Close()
	scanner := bufio.NewScanner(io.LimitReader(table, 8<<20))
	found := false
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) > 4 && fields[4] == request.Shared {
			found = true
			break
		}
	}
	if err = scanner.Err(); err != nil {
		return err
	}
	if !found {
		return errors.New("selected project export is not mounted; reconnect explicitly")
	}
	return nil
}
func (request workflowRequest) mappedPath(raw string, kind pathKind) (string, error) {
	base := request.Source
	if request.Directory != "" {
		base = request.Directory
		if !filepath.IsAbs(base) {
			base = filepath.Join(request.Source, base)
		}
	}
	if !filepath.IsAbs(raw) {
		raw = filepath.Join(base, raw)
	}
	raw = filepath.Clean(raw)
	if raw == request.Shared || strings.HasPrefix(raw, request.Shared+"/") {
		raw = request.Source + strings.TrimPrefix(raw, request.Shared)
	}
	if raw != request.Source && !strings.HasPrefix(raw, request.Source+"/") {
		return "", fmt.Errorf("path %q is outside the selected project; select a common project root", raw)
	}
	resolved, err := filepath.EvalSymlinks(raw)
	if err != nil {
		return "", fmt.Errorf("project path must already exist: %w", err)
	}
	if resolved != request.Source && !strings.HasPrefix(resolved, request.Source+"/") {
		return "", fmt.Errorf("project link resolves outside the selected root: %q", raw)
	}
	stat, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if (kind == pathDirectory && !stat.IsDir()) || (kind == pathFile && !stat.Mode().IsRegular()) || (kind == pathBind && !stat.IsDir() && !stat.Mode().IsRegular()) {
		return "", errors.New("project source has unsupported type")
	}
	return request.Shared + strings.TrimPrefix(resolved, request.Source), nil
}
func (request workflowRequest) command(ctx context.Context, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, request.Docker, append([]string{"--config", request.Config, "--host", socket}, args...)...)
	var environment []string
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		switch key {
		case "DOCKER_CONTEXT", "DOCKER_HOST", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH", "BUILDKIT_HOST", "BUILDX_BUILDER", "BUILDX_CONFIG", "COMPOSE_FILE", "COMPOSE_PROFILES", "COMPOSE_PROJECT_NAME", "COMPOSE_ENV_FILES", "COMPOSE_REMOVE_ORPHANS", "PATH", "PWD":
			continue
		}
		environment = append(environment, item)
	}
	command.Env = append(environment, "DOCKER_HOST="+socket, "BUILDX_BUILDER=default", "BUILDX_CONFIG="+filepath.Join(request.Config, "buildx"), "COMPOSE_PROFILES=", "COMPOSE_REMOVE_ORPHANS=false", "PATH="+request.HelperDirectory+":"+os.Getenv("PATH"))
	return command
}

type limitBuffer struct {
	bytes.Buffer
	left int
}

func (buffer *limitBuffer) Write(data []byte) (int, error) {
	if len(data) > buffer.left {
		return 0, errors.New("resolved project configuration exceeded its output budget")
	}
	buffer.left -= len(data)
	return buffer.Buffer.Write(data)
}
func (request workflowRequest) verifyDaemon(ctx context.Context) error {
	command := request.command(ctx, "info", "--format={{.ID}}")
	out, diagnostic := &limitBuffer{left: 8192}, &limitBuffer{left: 8192}
	command.Stdout = out
	command.Stderr = diagnostic
	if err := command.Run(); err != nil {
		return fmt.Errorf("owned WSL endpoint unavailable: %w: %s", err, diagnostic.String())
	}
	if strings.TrimSpace(out.String()) != request.Daemon {
		return errors.New("WSL endpoint identity differs; no project mutation dispatched")
	}
	return nil
}

func (request workflowRequest) verifyNamespace(ctx context.Context) error {
	for _, kind := range []string{"container", "volume", "network"} {
		args := []string{kind, "ls", "--quiet", "--filter", "label=com.docker.compose.project=" + request.Project}
		if kind == "container" {
			args = append(args, "--all", "--no-trunc")
		}
		list := request.command(ctx, args...)
		output, diagnostic := &limitBuffer{left: 65536}, &limitBuffer{left: 8192}
		list.Stdout = output
		list.Stderr = diagnostic
		if err := list.Run(); err != nil {
			return fmt.Errorf("inspect Compose project namespace: %w", err)
		}
		ids := strings.Fields(output.String())
		if len(ids) > 512 {
			return errors.New("project has more than 512 resources of one kind; narrow the supported workflow")
		}
		if len(ids) == 0 {
			continue
		}
		identifier := regexp.MustCompile(`^[a-f0-9]{12,64}$`)
		if kind == "volume" {
			identifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,255}$`)
		}
		for _, id := range ids {
			if !identifier.MatchString(id) {
				return errors.New("invalid returned Compose resource identity")
			}
		}
		format := "{{json .Labels}}"
		if kind == "container" {
			format = "{{json .Config.Labels}}"
		}
		args = append([]string{kind, "inspect", "--format", format}, ids...)
		inspect := request.command(ctx, args...)
		output = &limitBuffer{left: 16 << 20}
		inspect.Stdout = output
		inspect.Stderr = diagnostic
		if err := inspect.Run(); err != nil {
			return fmt.Errorf("verify Compose resource ownership: %w", err)
		}
		decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
		for range ids {
			var labels map[string]string
			if err := decoder.Decode(&labels); err != nil {
				return errors.New("invalid Compose resource ownership response")
			}
			if labels["dev.orbwin.wsl.project-source"] != request.Source || labels["dev.orbwin.wsl.distribution"] != request.Distribution {
				return errors.New("Compose project name already belongs to another or unadopted project; select a different explicit project name")
			}
		}
		if decoder.Decode(&struct{}{}) != io.EOF {
			return errors.New("unexpected Compose resource ownership suffix")
		}
	}
	return nil
}

func (request workflowRequest) labelModel(model map[string]any) error {
	inputs, _ := json.Marshal(map[string]any{"distribution": request.Distribution, "source": request.Source, "signature": request.Signature, "file": request.File, "overrides": request.Overrides, "envFiles": request.EnvFiles, "directory": request.Directory, "project": request.Project, "profiles": request.Profiles})
	if len(inputs) > 32768 {
		return errors.New("reusable Compose path metadata exceeds 32 KiB")
	}
	for _, kind := range []string{"services", "volumes", "networks"} {
		entries, exists := model[kind]
		if !exists {
			continue
		}
		objects, ok := entries.(map[string]any)
		if !ok {
			return errors.New("unsupported Compose resource model")
		}
		for key, value := range objects {
			var object map[string]any
			if value == nil {
				object = map[string]any{}
				objects[key] = object
			} else {
				var ok bool
				object, ok = value.(map[string]any)
				if !ok {
					return errors.New("unsupported Compose resource definition")
				}
			}
			if object["external"] == true {
				continue
			}
			labels, ok := object["labels"].(map[string]any)
			if !ok {
				if value, exists := object["labels"]; exists && value != nil {
					return errors.New("resolved Compose labels have unsupported syntax")
				}
				labels = map[string]any{}
				object["labels"] = labels
			}
			labels["dev.orbwin.wsl.project-source"] = request.Source
			labels["dev.orbwin.wsl.distribution"] = request.Distribution
			if kind == "services" {
				labels["dev.orbwin.wsl.compose-inputs"] = string(inputs)
			}
		}
	}
	return nil
}
func workflow(ctx context.Context, request workflowRequest) error {
	if request.MappingReview != nil && request.Kind != "compose" {
		return errors.New("mapping review requires Compose")
	}
	if request.ReviewedDaemon != "" && request.ReviewedDaemon != request.Daemon {
		return errors.New("reviewed destination engine changed; no workflow dispatched")
	}

	if request.Kind == "mapping" {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	if err := request.verifyProject(); err != nil {
		return err
	}
	for _, tool := range []string{request.Docker, request.Config, request.HelperDirectory} {
		if !filepath.IsAbs(tool) || strings.ContainsAny(tool, "\x00\r\n") {
			return errors.New("invalid host-supplied pinned tool/config path")
		}
	}
	if err := request.verifyDaemon(ctx); err != nil {
		return err
	}
	if request.Kind == "mapping" {
		if request.ReviewedDaemon == "" || request.ReviewedDaemon != request.Daemon {
			return errors.New("reviewed destination engine changed; no mapping file was created")
		}
		return saveProjectMapping(ctx, request, os.Stdout)
	}
	if request.Directory != "" && !filepath.IsAbs(request.Directory) {
		request.Directory = filepath.Join(request.Source, request.Directory)
	}
	directory := request.Source
	if request.Directory != "" {
		directory = request.Directory
	}
	mappedDirectory, err := request.mappedPath(directory, pathDirectory)
	if err != nil {
		return err
	}
	var command *exec.Cmd
	switch request.Kind {
	case "build":
		if request.Tag == "" || strings.HasPrefix(request.Tag, "-") || len(request.Tag) > 256 || strings.ContainsAny(request.Tag, "\x00\r\n") {
			return errors.New("invalid project build tag")
		}
		command = request.command(ctx, "build", "--tag", request.Tag, mappedDirectory)
	case "compose":
		valid := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)
		if !valid.MatchString(request.Project) || len(request.Overrides) > 15 || len(request.EnvFiles) > 16 || len(request.Profiles) > 32 || len(request.Services) > 64 {
			return errors.New("invalid bounded Compose inputs")
		}
		for _, value := range append(append([]string{}, request.Profiles...), request.Services...) {
			if !valid.MatchString(value) {
				return errors.New("invalid profile/service selection")
			}
		}
		if (request.Action == "down" || request.Action == "validate") && len(request.Services) > 0 {
			return errors.New("Down/Validate cannot be scoped to services")
		}
		switch request.Action {
		case "validate", "up", "pull", "logs", "down", "start", "stop", "restart":
		default:
			return errors.New("unsupported Compose workflow")
		}
		if err := request.verifyMapping(ctx); err != nil {
			return err
		}
		configArgs := []string{"compose", "--project-directory", mappedDirectory, "--project-name", request.Project}
		files := append([]string{request.File}, request.Overrides...)
		for _, file := range files {
			mapped, err := request.mappedPath(file, pathFile)
			if err != nil {
				return err
			}
			configArgs = append(configArgs, "--file", mapped)
		}
		for _, profile := range request.Profiles {
			configArgs = append(configArgs, "--profile", profile)
		}
		for _, file := range request.EnvFiles {
			mapped, err := request.mappedPath(file, pathFile)
			if err != nil {
				return err
			}
			configArgs = append(configArgs, "--env-file", mapped)
		}
		configArgs = append(configArgs, "config", "--format=json")
		resolve := request.command(ctx, configArgs...)
		resolve.Dir = mappedDirectory
		resolve.Env = append(resolve.Env, "PWD="+mappedDirectory)
		resolved, diagnostic := &limitBuffer{left: configLimit}, &limitBuffer{left: 32768}
		resolve.Stdout = resolved
		resolve.Stderr = diagnostic
		if err = resolve.Run(); err != nil {
			return fmt.Errorf("Compose configuration refused: %w: %s", err, diagnostic.String())
		}
		var model map[string]any
		decoder := json.NewDecoder(bytes.NewReader(resolved.Bytes()))
		decoder.UseNumber()
		if err = decoder.Decode(&model); err != nil {
			return err
		}
		if decoder.Decode(&struct{}{}) != io.EOF {
			return errors.New("unexpected resolved Compose suffix")
		}
		if err = rewriteComposePaths(model, request.mappedPath); err != nil {
			return err
		}
		if err := request.verifyMapping(ctx); err != nil {
			return err
		}
		if request.Action == "validate" {
			fmt.Fprintln(os.Stdout, "Compose configuration and selected project paths validated.")
			return nil
		}
		if err = request.verifyNamespace(ctx); err != nil {
			return err
		}
		if err = request.labelModel(model); err != nil {
			return err
		}
		frame, err := json.Marshal(model)
		if err != nil {
			return err
		}
		if len(frame) > configLimit {
			return errors.New("adapted Compose model exceeds 16 MiB")
		}
		args := []string{"compose", "--project-directory", mappedDirectory, "--project-name", request.Project, "--file", "-"}
		for _, profile := range request.Profiles {
			args = append(args, "--profile", profile)
		}
		switch request.Action {
		case "up":
			args = append(args, "up", "--detach", "--wait", "--wait-timeout=120")
		case "logs":
			args = append(args, "logs", "--no-color", "--tail=500", "--timestamps")
		case "down":
			args = append(args, "down", "--timeout=10")
		default:
			args = append(args, request.Action)
		}
		args = append(args, request.Services...)
		command = request.command(ctx, args...)
		// A sealed anonymous memfd provides stdin across exec without persisting the
		// resolved configuration or secrets on either Windows or the WSL filesystem.
		fd, err := unix.MemfdCreate("orbwin-compose", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
		if err != nil {
			return err
		}
		input := os.NewFile(uintptr(fd), "orbwin-compose")
		defer input.Close()
		if _, err = input.Write(frame); err != nil {
			return err
		}
		if _, err = input.Seek(0, 0); err != nil {
			return err
		}
		if _, err = unix.FcntlInt(input.Fd(), unix.F_ADD_SEALS, unix.F_SEAL_WRITE|unix.F_SEAL_GROW|unix.F_SEAL_SHRINK|unix.F_SEAL_SEAL); err != nil {
			return err
		}
		if err = unix.Dup2(fd, 0); err != nil {
			return err
		}
	default:
		return errors.New("unsupported project workflow kind")
	}
	if err = request.verifyProject(); err != nil {
		return err
	}
	if err = request.verifyDaemon(ctx); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = request.verifyMapping(ctx); err != nil {
		return err
	}
	command.Dir = mappedDirectory
	command.Env = append(command.Env, "PWD="+mappedDirectory)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err = os.Chdir(mappedDirectory); err != nil {
		return err
	}
	// Replace this short-lived adapter with the ordinary pinned Docker client.
	// Accepted engine mutations retain the existing Docker cancellation semantics.
	return syscall.Exec(command.Path, command.Args, command.Env)
}
func main() {
	decoder := json.NewDecoder(io.LimitReader(os.Stdin, (1<<20)+1))
	decoder.DisallowUnknownFields()
	var request workflowRequest
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	defer cancel()
	err := decoder.Decode(&request)
	if err == nil && decoder.Decode(&struct{}{}) != io.EOF {
		err = errors.New("unexpected workflow input")
	}
	if err == nil {
		err = workflow(ctx, request)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "OrbWin WSL project:", err)
		os.Exit(125)
	}
}
