//go:build linux

package wsltrust

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const receiptBudget = 32768
const bundleBudget = 8 << 20

type storePaths struct {
	anchors, bundle, certs, state, osRelease, machine, updater, config string
	owner                                                              uint32
}
type Store struct {
	root   *os.Root
	paths  storePaths
	update func(context.Context, *os.File) (bool, error)
	now    func() time.Time
}
type receipt struct {
	Version        int    `json:"version"`
	Installation   string `json:"installation"`
	DistributionID string `json:"distributionId"`
	MachineID      string `json:"machineId"`
	WindowsUser    string `json:"windowsUser"`
	RootSHA256     string `json:"rootSha256"`
	Certificate    []byte `json:"certificate"`
	State          string `json:"state"`
	Sequence       uint64 `json:"sequence"`
	UpdaterWarning bool   `json:"updaterWarning"`
}

func NewSystemStore() (*Store, error) {
	if os.Geteuid() != 0 {
		return nil, errors.New("selected distro trust requires Linux root; Windows administrator access is not used")
	}
	root, err := os.OpenRoot("/")
	if err != nil {
		return nil, err
	}
	store := &Store{root: root, paths: storePaths{anchors: "usr/local/share/ca-certificates", bundle: "etc/ssl/certs/ca-certificates.crt", certs: "etc/ssl/certs", state: "var/lib/orbwin/trust", osRelease: "etc/os-release", machine: "etc/machine-id", updater: "usr/sbin/update-ca-certificates", config: "etc/ca-certificates.conf", owner: 0}, now: time.Now}
	store.update = store.systemUpdate
	return store, nil
}
func (store *Store) Close() error { return store.root.Close() }
func (store *Store) base(request Request) string {
	return "orbwin-" + request.Installation + "-" + request.RootSHA256
}
func (store *Store) anchor(request Request) string {
	return filepath.Join(store.paths.anchors, store.base(request)+".crt")
}
func (store *Store) recordPath(request Request) string {
	return filepath.Join(store.paths.state, store.base(request)+".json")
}
func (store *Store) read(path string, budget int64) ([]byte, error) {
	before, err := store.root.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Size() > budget {
		return nil, errors.New("trust input must be a bounded regular file")
	}
	file, err := store.root.Open(path)
	if err != nil {
		return nil, err
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) {
		file.Close()
		return nil, errors.New("trust input changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, budget+1))
	after, statErr := file.Stat()
	closed := file.Close()
	if err != nil || statErr != nil || closed != nil {
		return nil, errors.Join(err, statErr, closed)
	}
	if int64(len(data)) > budget || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return nil, errors.New("trust input changed while reading")
	}
	return data, nil
}
func (store *Store) protected(path string, missingOK bool) error {
	parts := strings.Split(filepath.Clean(path), string(filepath.Separator))
	current := ""
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, err := store.root.Lstat(current)
		if os.IsNotExist(err) && missingOK {
			return nil
		}
		if err != nil {
			return err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || info.Mode()&os.ModeSymlink != 0 || stat.Uid != store.paths.owner || info.Mode().Perm()&0022 != 0 {
			return fmt.Errorf("trust path has unsafe ownership, permissions or a link: %s", current)
		}
	}
	return nil
}
func (store *Store) metadata(request Request) (Status, error) {
	status := Status{DistributionID: request.DistributionID, RootSHA256: request.RootSHA256, State: "unavailable"}
	// os-release is often a relative link into /usr/lib; os.Root keeps that read
	// inside the selected filesystem. No shell evaluates distribution metadata.
	file, err := store.root.Open(store.paths.osRelease)
	if err != nil {
		return status, err
	}
	data, err := io.ReadAll(io.LimitReader(file, 16385))
	err = errors.Join(err, file.Close())
	if err != nil {
		return status, err
	}
	if len(data) > 16384 {
		return status, errors.New("os-release exceeds its budget")
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.Trim(value, "\"'")
		if key == "ID" {
			status.OSID = value
		}
		if key == "VERSION_ID" {
			status.OSVersion = value
		}
	}
	if status.OSID != "ubuntu" || status.OSVersion != "24.04" {
		return status, errors.New("automated system trust currently requires qualified Ubuntu 24.04 ca-certificates layout; use public-root export for other stores")
	}
	identity, err := store.read(store.paths.machine, 4096)
	if err != nil {
		return status, err
	}
	status.MachineID = strings.TrimSpace(string(identity))
	if !machineID.MatchString(status.MachineID) {
		return status, errors.New("stable distro machine identity unavailable")
	}
	for _, path := range []string{store.paths.anchors, store.paths.certs, store.paths.bundle, store.paths.state, store.paths.updater, store.paths.machine, store.paths.config} {
		if err := store.protected(path, path == store.paths.state); err != nil {
			return status, err
		}
	}
	config, err := store.read(store.paths.config, 1<<20)
	if err != nil {
		return status, err
	}
	if len(config) == 0 {
		return status, errors.New("empty ca-certificates configuration would trigger broad fresh cleanup; preserved")
	}
	return status, nil
}
func (store *Store) load(request Request, machine string) (*receipt, error) {
	data, err := store.read(store.recordPath(request), receiptBudget)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := store.protected(store.recordPath(request), false); err != nil {
		return nil, err
	}
	var record receipt
	if err := StrictJSON(data, &record); err != nil {
		return nil, err
	}
	if record.Version != 1 || record.Installation != request.Installation || record.DistributionID != request.DistributionID || record.WindowsUser != request.WindowsUser || record.MachineID != machine || record.RootSHA256 != request.RootSHA256 || !bytes.Equal(record.Certificate, request.RootCertificate) || record.Sequence == 0 {
		return nil, errors.New("trust receipt belongs to another installation, registration, user or machine; preserved")
	}
	switch record.State {
	case "install-intent", "installed", "remove-intent", "removed":
	default:
		return nil, errors.New("unsupported distro trust receipt state")
	}
	return &record, nil
}
func containsRoot(bundle, der []byte) (bool, error) {
	for count := 0; count < 4096; count++ {
		block, rest := pem.Decode(bundle)
		if block == nil {
			return false, nil
		}
		if block.Type == "CERTIFICATE" && bytes.Equal(block.Bytes, der) {
			return true, nil
		}
		bundle = rest
	}
	if block, _ := pem.Decode(bundle); block != nil {
		return false, errors.New("system bundle exceeds the 4096-certificate scan budget")
	}
	return false, nil
}
func (store *Store) observe(request Request) (Status, *receipt, error) {
	status, err := store.metadata(request)
	if err != nil {
		return status, nil, err
	}
	record, err := store.load(request, status.MachineID)
	if err != nil {
		return status, nil, err
	}
	if record != nil {
		status.Managed = record.State != "removed"
		status.State = record.State
		status.Sequence = record.Sequence
		status.UpdaterWarning = record.UpdaterWarning
	} else {
		status.State = "absent"
	}
	anchor, err := store.read(store.anchor(request), 16384)
	if err == nil {
		status.FilePresent = true
		want := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: request.RootCertificate})
		if !bytes.Equal(anchor, want) {
			return status, record, errors.New("certificate anchor changed or conflicts; preserved")
		}
		if err := store.protected(store.anchor(request), false); err != nil {
			return status, record, err
		}
	} else if !os.IsNotExist(err) {
		return status, record, err
	}
	bundle, err := store.read(store.paths.bundle, bundleBudget)
	if err != nil {
		return status, record, err
	}
	status.BundlePresent, err = containsRoot(bundle, request.RootCertificate)
	if err != nil {
		return status, record, err
	}
	status.Observed = true
	if !status.Managed && (status.FilePresent || status.BundlePresent) {
		status.State = "present-unmanaged"
	}
	return status, record, nil
}
func (store *Store) syncDirectory(path string) error {
	directory, err := store.root.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}
func (store *Store) save(record *receipt) error {
	if record.Sequence == math.MaxUint64 {
		return errors.New("trust receipt sequence exhausted")
	}
	copy := *record
	copy.Sequence++
	data, err := json.Marshal(copy)
	if err != nil {
		return err
	}
	if len(data) > receiptBudget {
		return errors.New("trust receipt exceeds budget")
	}
	directory, err := store.root.Open(store.paths.state)
	if err != nil {
		return err
	}
	entries, err := directory.ReadDir(4097)
	closed := directory.Close()
	if err != nil || closed != nil {
		return errors.Join(err, closed)
	}
	if len(entries) > 4096 {
		return errors.New("trust state directory exceeds its budget")
	}
	drafts := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".receipt-") {
			drafts++
		}
	}
	if drafts >= 64 {
		return errors.New("retained receipt drafts reached their budget; preserve them for review")
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	name := filepath.Join(store.paths.state, ".receipt-"+hex.EncodeToString(nonce[:])+".partial")
	file, err := store.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("trust receipt draft already exists or could not be created; retain it for review")
	}
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return err
	}
	request := Request{Installation: record.Installation, RootSHA256: record.RootSHA256}
	if err := store.root.Rename(name, store.recordPath(request)); err != nil {
		return err
	}
	if err := store.syncDirectory(store.paths.state); err != nil {
		return err
	}
	record.Sequence = copy.Sequence
	return nil
}
func (store *Store) acquire(ctx context.Context, write bool) (*os.File, error) {
	if write {
		if err := store.root.MkdirAll(store.paths.state, 0700); err != nil {
			return nil, err
		}
		if err := store.protected(store.paths.state, false); err != nil {
			return nil, err
		}
	}
	flags := os.O_RDONLY
	if write {
		flags = os.O_RDWR | os.O_CREATE
	}
	file, err := store.root.OpenFile(filepath.Join(store.paths.state, ".lock"), flags, 0600)
	if os.IsNotExist(err) && !write {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := store.protected(filepath.Join(store.paths.state, ".lock"), false); err != nil {
		file.Close()
		return nil, err
	}
	mode := unix.LOCK_SH
	if write {
		mode = unix.LOCK_EX
	}
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := unix.Flock(int(file.Fd()), mode|unix.LOCK_NB)
		if err == nil {
			return file, nil
		}
		if err != unix.EWOULDBLOCK {
			file.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			file.Close()
			return nil, ctx.Err()
		case <-deadline.C:
			file.Close()
			return nil, errors.New("distro trust operation is busy; reread before retrying")
		case <-ticker.C:
		}
	}
}

func (store *Store) Apply(ctx context.Context, request Request) (Status, error) {
	base := Status{DistributionID: request.DistributionID, RootSHA256: request.RootSHA256, State: "unavailable"}
	if err := ctx.Err(); err != nil {
		return base, err
	}
	root, err := request.Validate()
	if err != nil {
		return base, err
	}
	if request.Operation == "install" && (store.now().Before(root.NotBefore) || !store.now().Before(root.NotAfter)) {
		return base, errors.New("distro clock is outside the reviewed root validity period")
	}
	// Validate root-owned parents before creating even a lock or state directory.
	metadata, err := store.metadata(request)
	if err != nil {
		return metadata, err
	}
	if request.ExpectedMachineID != "" && request.ExpectedMachineID != metadata.MachineID {
		return metadata, errors.New("selected distro machine changed; preserve its trust state and review before applying")
	}
	lock, err := store.acquire(ctx, request.Operation != "status")
	if err != nil {
		return metadata, err
	}
	if lock != nil {
		defer lock.Close()
	}
	status, record, err := store.observe(request)
	if err != nil {
		return status, err
	}
	if request.ExpectedMachineID != "" && request.ExpectedMachineID != status.MachineID {
		return status, errors.New("selected distro machine changed during preparation")
	}
	if request.Operation == "status" {
		return status, nil
	}
	if err := ctx.Err(); err != nil {
		return status, err
	}
	if request.ExpectedSequence != status.Sequence {
		return status, errors.New("selected distro trust changed; reread status before applying")
	}
	if request.Operation == "install" {
		return store.install(ctx, request, status, record, lock)
	}
	return store.remove(ctx, request, status, record, lock)
}
