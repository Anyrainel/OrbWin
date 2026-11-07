//go:build windows

package wsltrust

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"
	"unicode/utf16"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// Registration is a current-user WSL 2 registration, not a name-based target.
// Listing reads only HKCU; it neither invokes WSL nor starts distributions.
func ordinaryRegistration(id, name string, version uint64) bool {
	if !guid.MatchString(id) || version != 2 || name == "" || len(name) > 128 || strings.HasPrefix(name, "-") || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return false
	}
	lower := strings.ToLower(name)
	for _, prefix := range []string{"lima-orbwin", "orbwin", "rancher-desktop", "lima-rd", "docker-desktop"} {
		if strings.HasPrefix(lower, prefix) {
			return false
		}
	}
	return true
}

func CurrentWindowsUser() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}

// RegistrationPresent inspects the exact raw current-user GUID key. In
// particular, a protected name, WSL 1 version or damaged metadata is still a
// present registration; it must not be mistaken for an unregistered distro by
// looking at the filtered ordinary-distribution list. No WSL process is started.
func RegistrationPresent(id string) (bool, error) {
	if !ValidRegistrationID(id) {
		return false, errors.New("canonical registration GUID required")
	}
	root, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Lxss`, registry.ENUMERATE_SUB_KEYS)
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) || errors.Is(err, syscall.ERROR_PATH_NOT_FOUND) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	present, err := registrationPresentAt(root, id)
	return present, errors.Join(err, root.Close())
}

// The production caller supplies only the fixed HKCU Lxss root. A private key
// in Windows tests exercises missing/protected/legacy/damaged registrations
// without changing the actual WSL registration tree.
func registrationPresentAt(root registry.Key, id string) (bool, error) {
	if !ValidRegistrationID(id) {
		return false, errors.New("canonical registration GUID required")
	}
	key, err := registry.OpenKey(root, "{"+id+"}", registry.QUERY_VALUE)
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, key.Close()
}

func Registrations() (result []Registration, resultErr error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Lxss`, registry.ENUMERATE_SUB_KEYS)
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return []Registration{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { resultErr = errors.Join(resultErr, key.Close()) }()
	names, err := key.ReadSubKeyNames(129)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(names) > 128 {
		return nil, errors.New("WSL registration count exceeds its budget")
	}
	result = []Registration{}
	for _, name := range names {
		id := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(name, "{"), "}"))
		if !guid.MatchString(id) {
			continue
		}
		registration, err := registry.OpenKey(key, name, registry.QUERY_VALUE)
		if err != nil {
			return nil, errors.New("WSL registration changed or could not be read; refresh before selecting")
		}
		distribution, _, nameErr := registration.GetStringValue("DistributionName")
		version, _, versionErr := registration.GetIntegerValue("Version")
		if err := errors.Join(nameErr, versionErr, registration.Close()); err != nil {
			return nil, err
		}
		if ordinaryRegistration(id, distribution, version) {
			result = append(result, Registration{ID: id, Name: distribution})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == result[j].Name {
			return result[i].ID < result[j].ID
		}
		return result[i].Name < result[j].Name
	})
	return result, nil
}

// Host transports a separately attested Linux ELF and public request. It owns a
// copy of the bytes. Call never retries accepted or uncertain operations. The
// caller must durably record intent before an install/remove call and use a new
// status observation after transport loss. WSL cancellation does not prove the
// guest updater stopped; its inherited Linux lock remains the serialization owner.
type Host struct {
	asset         []byte
	digest        string
	user          string
	registrations func() ([]Registration, error)
	run           func(context.Context, []string, io.Reader, int) ([]byte, error)
	admission     *guestAdmission
}

func (host *Host) UserID() string { return host.user }

func NewHost(asset []byte, digest string) (*Host, error) {
	if err := ValidateHelperAsset(asset, digest); err != nil {
		return nil, err
	}
	user, err := CurrentWindowsUser()
	if err != nil {
		return nil, err
	}
	directory, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, err
	}
	binaryPath := filepath.Join(directory, "wsl.exe")
	return &Host{admission: &guestAdmission{}, asset: append([]byte(nil), asset...), digest: digest, user: user, registrations: Registrations, run: func(ctx context.Context, args []string, input io.Reader, budget int) ([]byte, error) {
		return runWSL(ctx, binaryPath, args, input, budget)
	}}, nil
}

func (host *Host) SealGuestIssuer() (*SealedGuestIssuer, error) {
	if host == nil {
		return nil, errGuestIssuer
	}
	return host.admission.seal()
}
func (host *Host) runOwned(ctx context.Context, args []string, input io.Reader, budget int) ([]byte, error) {
	if host == nil || host.run == nil {
		return nil, errGuestIssuer
	}
	done, err := host.admission.begin()
	if err != nil {
		return nil, err
	}
	defer done()
	return host.run(ctx, args, input, budget)
}

type limitedOutput struct {
	mu       sync.Mutex
	bytes    []byte
	limit    int
	exceeded bool
}

func (output *limitedOutput) Write(data []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()
	length := len(data)
	remaining := output.limit - len(output.bytes)
	if len(data) > remaining {
		output.exceeded = true
		data = data[:remaining]
	}
	output.bytes = append(output.bytes, data...)
	return length, nil
}
func runWSL(ctx context.Context, path string, args []string, input io.Reader, budget int) ([]byte, error) {
	command := exec.CommandContext(ctx, path, args...)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	command.WaitDelay = 2 * time.Second
	stdout := &limitedOutput{limit: budget}
	stderr := &limitedOutput{limit: 4096}
	command.Stdin, command.Stdout, command.Stderr = input, stdout, stderr
	err := command.Run()
	if stdout.exceeded {
		return nil, errors.Join(err, errors.New("WSL response exceeded its budget"))
	}
	// Guest diagnostics can include arbitrary local data. Keep them out of the
	// returned error and persisted receipts; the protocol supplies bounded errors.
	if err != nil {
		return stdout.bytes, fmt.Errorf("selected WSL operation did not confirm completion: %w", err)
	}
	return stdout.bytes, nil
}

func supportsDistributionID(data []byte) bool {
	// wsl.exe help can be UTF-16LE when redirected on Windows. Do not treat a
	// substring of another option as proof of support.
	if len(data) >= 2 && (bytes.HasPrefix(data, []byte{0xff, 0xfe}) || data[1] == 0) {
		if len(data)%2 != 0 {
			return false
		}
		units := make([]uint16, len(data)/2)
		for i := range units {
			units[i] = binary.LittleEndian.Uint16(data[i*2:])
		}
		data = []byte(string(utf16.Decode(units)))
	}
	for _, field := range strings.Fields(strings.TrimPrefix(string(data), "\ufeff")) {
		if field == "--distribution-id" {
			return true
		}
	}
	return false
}

// No Windows drive/automount assumptions and no shell interpolation. The fixed
// bootstrap copies exactly the attested byte count from stdin into a root-owned
// 0700 temporary directory, verifies it there, then passes remaining public JSON
// to the helper. Normal exit removes only this exact helper/directory. Abrupt
// guest loss may retain this public binary; it contains no key or request file.
const trustBootstrap = `set -eu
umask 077
stage=$(/usr/bin/mktemp -d /tmp/orbwin-trust.XXXXXXXXXX)
trap '/usr/bin/rm -f -- "$stage/helper"; /usr/bin/rmdir -- "$stage"' EXIT
/usr/bin/head -c "$1" > "$stage/helper"
actual=$(/usr/bin/sha256sum -- "$stage/helper")
test "${actual%% *}" = "$2" || exit 73
/usr/bin/chmod 0700 -- "$stage/helper"
"$stage/helper"
`

func (host *Host) Call(ctx context.Context, request Request) (*Status, error) {
	if _, err := request.Validate(); err != nil {
		return nil, err
	}
	if request.WindowsUser != host.user {
		return nil, errors.New("selected distro trust Windows user differs")
	}
	registrations, err := host.registrations()
	if err != nil {
		return nil, err
	}
	found := false
	for _, registration := range registrations {
		if registration.ID == request.DistributionID {
			found = true
			break
		}
	}
	if !found {
		return nil, errors.New("selected ordinary WSL 2 registration is no longer present; refresh before retrying")
	}
	ctx, cancel := context.WithTimeout(ctx, 65*time.Second)
	defer cancel()
	help, err := host.runOwned(ctx, []string{"--help"}, nil, 128<<10)
	if err != nil {
		// The installed Microsoft wsl.exe returns 0xffffffff for --help even
		// after emitting its complete help. Accept only that observed help exit,
		// still requiring the exact option below; all other failures propagate.
		var exit *exec.ExitError
		if !errors.As(err, &exit) || (exit.ExitCode() != -1 && int64(exit.ExitCode()) != 0xffffffff) || ctx.Err() != nil {
			return nil, err
		}
	}
	if !supportsDistributionID(help) {
		return nil, errors.New("this WSL version does not support registration-GUID addressing")
	}
	// Help can take time. Re-read immediately before launch so an engine rename
	// or registration removal during capability inspection is not overlooked.
	registrations, err = host.registrations()
	if err != nil {
		return nil, err
	}
	found = false
	for _, registration := range registrations {
		if registration.ID == request.DistributionID {
			found = true
			break
		}
	}
	if !found {
		return nil, errors.New("selected WSL registration changed during preparation")
	}
	data, err := json.Marshal(request)
	if err != nil || len(data) > RequestBudget {
		return nil, errors.New("selected distro trust request exceeds budget")
	}
	args := []string{"--distribution-id", "{" + request.DistributionID + "}", "--user", "root", "--exec", "/usr/bin/env", "-i", "PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "/bin/sh", "-c", trustBootstrap, "orbwin-trust", strconv.Itoa(len(host.asset)), host.digest}
	output, runErr := host.runOwned(ctx, args, io.MultiReader(bytes.NewReader(host.asset), bytes.NewReader(data)), ResponseBudget)
	if runErr != nil {
		return nil, runErr
	}
	if len(output) == 0 || len(output) > ResponseBudget {
		return nil, errors.New("selected distro trust response exceeds budget")
	}
	var response Response
	if err := StrictJSON(output, &response); err != nil {
		return nil, err
	}
	if response.Version != 1 || len(response.Error) > 4096 || response.Status == nil {
		return nil, errors.New("selected distro trust response version or status is invalid")
	}
	status := response.Status
	if status.DistributionID != request.DistributionID || status.RootSHA256 != request.RootSHA256 || (status.Observed && (!machineID.MatchString(status.MachineID) || status.OSID != "ubuntu" || status.OSVersion != "24.04")) {
		return nil, errors.New("selected distro trust response identity differs")
	}
	if request.ExpectedMachineID != "" && status.Observed && status.MachineID != request.ExpectedMachineID {
		return status, errors.New("selected distro machine differs from the reviewed identity")
	}
	switch status.State {
	case "unavailable", "absent", "present-unmanaged", "install-intent", "installed", "remove-intent", "removed":
	default:
		return nil, errors.New("selected distro trust response state is invalid")
	}
	if response.Error != "" {
		return status, errors.New(response.Error)
	}
	if !status.Observed {
		return status, errors.New("selected distro trust response did not observe trust")
	}
	return status, nil
}
