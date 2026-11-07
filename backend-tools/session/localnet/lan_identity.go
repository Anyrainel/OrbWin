package localnet

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"orbwin.local/session/internal/lanhelper"
	"os"
	"path/filepath"
	"strings"
)

type lanNamespaceBinding struct {
	Version            int    `json:"version"`
	Directory          string `json:"directory"`
	Daemon             string `json:"daemon"`
	GlobalInstallation string `json:"globalInstallation"`
	UserSID            string `json:"userSid"`
	Namespace          string `json:"namespace"`
}

type lanProductBinding struct {
	Version            int    `json:"version"`
	Daemon             string `json:"daemon"`
	GlobalInstallation string `json:"globalInstallation"`
	UserSID            string `json:"userSid"`
	Namespace          string `json:"namespace"`
}

func deriveLANProductBinding(daemon, globalInstallation, sid string) (lanProductBinding, error) {
	if err := validateLANOwnerFacts(daemon, globalInstallation, sid); err != nil {
		return lanProductBinding{}, err
	}
	identity := struct{ Installation, UserSID string }{strings.ReplaceAll(globalInstallation, "-", ""), sid}
	data, _ := json.Marshal(identity)
	sum := sha256.Sum256(append([]byte("orbwin-lan-owner-namespace-v2\n"), data...))
	return lanProductBinding{Version: 2, Daemon: daemon, GlobalInstallation: globalInstallation, UserSID: sid, Namespace: hex.EncodeToString(sum[:16])}, nil
}

func (ledger *NamespaceLedger) managedLANInstallationID(globalInstallation, sid string) (string, error) {
	want, err := deriveLANProductBinding(ledger.daemon, globalInstallation, sid)
	if err != nil {
		return "", err
	}
	path := filepath.Join(ledger.directory, "lan-identity.json")
	data, err := ledger.readState(path, 2048)
	if err == nil {
		var got lanProductBinding
		if lanhelper.ValidateClosedJSON(data, lanProductBinding{}) != nil || strictJSON(data, &got) != nil || got != want {
			return "", errors.New("managed LAN product/SID/daemon binding differs; retain it for explicit transition")
		}
		return got.Namespace, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	data, _ = json.Marshal(want)
	if err = ledger.createState(path, data); err != nil {
		return "", err
	}
	got, err := ledger.readState(path, 2048)
	if err != nil || !bytes.Equal(got, data) {
		return "", errors.Join(errors.New("managed LAN product identity confirmation failed"), err)
	}
	return want.Namespace, nil
}

func (runtime *ServiceRuntime) newRuntimeLANController(shared *sharedNamespaces) *lanController {
	installation := runtime.lanInstallation
	if installation == "" && runtime.opts.LAN != nil {
		installation = runtime.opts.LAN.Installation
	}
	controller := newLANController(installation, shared, runtime.opts.LAN)
	if runtime.lanEvents != nil {
		controller.onChange = func() {
			select {
			case runtime.lanEvents <- lanOperationEvent{resourceObservation: true}:
			default:
			}
		}
	}
	return controller
}

// The immutable query result originated in native-SID/owned-ledger derivation,
// never the renderer. Only the SAME actor binds a previously empty controller.
func (runtime *ServiceRuntime) bindLANInstallation(installation string) error {
	if !validLANID(installation) || (runtime.lanInstallation != "" && runtime.lanInstallation != installation) {
		return errors.New("LAN owner namespace differs from retained native identity")
	}
	if runtime.generation != nil && runtime.generation.lan != nil {
		controller := runtime.generation.lan
		controller.mu.Lock()
		defer controller.mu.Unlock()
		if controller.installation != "" && controller.installation != installation {
			return errors.New("existing LAN generation belongs to a different native namespace")
		}
		controller.installation = installation
	}
	runtime.lanInstallation = installation
	return nil
}
func validateLANOwnerFacts(daemon, globalInstallation, sid string) error {
	globalHex := strings.ReplaceAll(globalInstallation, "-", "")
	if daemon == "" || len(daemon) > 256 || strings.ContainsAny(daemon+sid, "\x00\r\n") || len(sid) < 8 || len(sid) > 184 || !validLANID(globalHex) || (len(globalInstallation) != 32 && !(len(globalInstallation) == 36 && globalInstallation[8] == '-' && globalInstallation[13] == '-' && globalInstallation[18] == '-' && globalInstallation[23] == '-')) {
		return errors.New("trusted daemon, global product identity and native SID required")
	}
	return nil
}

func deriveLANBinding(directory, daemon, globalInstallation, sid string) (lanNamespaceBinding, error) {
	var empty lanNamespaceBinding
	if !filepath.IsAbs(directory) {
		return empty, errors.New("trusted owned directory required")
	}
	if err := validateLANOwnerFacts(daemon, globalInstallation, sid); err != nil {
		return empty, err
	}
	binding := lanNamespaceBinding{Version: 1, Directory: strings.ToLower(strings.ReplaceAll(filepath.Clean(directory), "\\", "/")), Daemon: daemon, GlobalInstallation: globalInstallation, UserSID: sid}
	data, _ := json.Marshal(binding)
	sum := sha256.Sum256(append([]byte("orbwin-lan-owner-namespace-v1\n"), data...))
	binding.Namespace = hex.EncodeToString(sum[:16])
	return binding, nil
}

// LANInstallationID is a denial/info pin under the SAME exclusive ledger. It
// never changes the existing UUID36 installation used by CA/WSL receipts. Every
// read re-derives identity from trusted owner facts and actual native caller SID;
// the user-home file itself confers no protected/native authority.
func (ledger *NamespaceLedger) LANInstallationID(globalInstallation, sid string) (out string, resultErr error) {
	if err := ledger.lockState(nil); err != nil {
		return "", err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() {
		return "", errors.New("exclusive ledger owner required for LAN namespace")
	}
	if ledger.stateStore != nil {
		return ledger.managedLANInstallationID(globalInstallation, sid)
	}
	want, err := deriveLANBinding(ledger.directory, ledger.daemon, globalInstallation, sid)
	if err != nil {
		return "", err
	}
	path := filepath.Join(ledger.directory, "lan-identity.json")
	read := func() (string, error) {
		info, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 2048 {
			return "", errors.New("LAN namespace record kind/budget differs")
		}
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, 2049))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil || len(data) > 2048 {
			return "", errors.New("LAN namespace record read unknown")
		}
		var got lanNamespaceBinding
		if lanhelper.ValidateClosedJSON(data, lanNamespaceBinding{}) != nil || strictJSON(data, &got) != nil || got != want {
			return "", errors.New("LAN namespace owner facts changed; preserve prior record for review")
		}
		return got.Namespace, nil
	}
	if id, err := read(); err == nil {
		return id, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	data, _ := json.Marshal(want)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if os.IsExist(err) {
		return read()
	}
	if err != nil {
		return "", err
	}
	n, writeErr := file.Write(data)
	flushErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || n != len(data) || flushErr != nil || closeErr != nil {
		return "", errors.New("LAN namespace write/flush unknown; reread before new review")
	}
	if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, data) {
		return "", errors.New("LAN namespace confirmation differs")
	}
	return want.Namespace, nil
}
