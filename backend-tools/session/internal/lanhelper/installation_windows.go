//go:build windows

package lanhelper

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sys/windows"
)

// AuthorizedInstallation retains the independent asset pins and protected files
// required by mutation. Its private fields cannot be minted from caller JSON.
type AuthorizedInstallation struct {
	nativeOrigin     *AuthorizedInstallation // Actual allocation, never first-Close adoption.
	manager          *NativeManager
	installation     string
	assets           AssetBundle
	held             []*os.File
	worker           *os.File
	self             *NativeManager // Actual ordinary launcher self check, if this role needed it.
	closeOnce        sync.Once
	closeErr         error
	closeDiagnostics []string
	cleanupPanic     any
}

func newAuthorizedInstallation(manager *NativeManager, installation string) *AuthorizedInstallation {
	authority := &AuthorizedInstallation{manager: manager, installation: installation}
	authority.nativeOrigin = authority
	return authority
}

func (manager *NativeManager) AuthorizeInstallation(installation string) (authority *AuthorizedInstallation, err error) {
	if manager == nil || manager.nativeOrigin != manager {
		return nil, errNativeOwnerCopy
	}
	return holdSignedInstallation(manager, installation, true, false)
}
func holdSignedInstallation(manager *NativeManager, installation string, helper, launcher bool) (authority *AuthorizedInstallation, err error) {
	if manager == nil || manager.nativeOrigin != manager || !recordID(installation) {
		return nil, errors.New("held native manager and installation required")
	}
	authority = newAuthorizedInstallation(manager, installation)
	owned := authority
	defer func() {
		if err != nil {
			authority, err = settleInstallationFailure(owned, err, owned.Close)
		}
	}()
	if helper {
		if err = manager.checkPipe(manager.pipe); err != nil {
			return nil, err
		}
	} else if manager.identity.PID != windows.GetCurrentProcessId() {
		if !launcher {
			return nil, errors.New("ordinary client must hold its own native manager")
		}
		self, e := holdOrdinarySelf()
		authority.self = self
		if e != nil {
			return nil, e
		}
		if self.path != manager.path || self.identity.ImageSHA256 != manager.identity.ImageSHA256 || self.identity.UserSID != manager.identity.UserSID || self.session != manager.session {
			return nil, errors.New("ordinary launcher differs from its authenticated signed SDK manager")
		}
	}
	base := filepath.Dir(manager.path)
	if filepath.Base(manager.path) != "orbwin-session.exe" || len(base) < 3 || base[1:3] != `:\` || strings.ContainsAny(base, `/%*?[]`+"`\x00") {
		return nil, errors.New("fixed local native manager bundle location required")
	}
	for current := base; ; current = filepath.Dir(current) {
		file, e := heldFile(current, true, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE)
		if e != nil {
			return nil, e
		}
		authority.held = append(authority.held, file)
		if filepath.Dir(current) == current {
			break
		}
	}
	root := filepath.Join(base, "lan")
	directory, err := heldFile(root, true, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE)
	if err != nil {
		return nil, err
	}
	authority.held = append(authority.held, directory)
	manifest, err := heldFile(filepath.Join(root, "assets.json"), false, windows.GENERIC_READ|windows.READ_CONTROL, windows.FILE_SHARE_READ)
	if err != nil {
		return nil, err
	}
	authority.held = append(authority.held, manifest)
	data, err := io.ReadAll(io.LimitReader(manifest, 4097))
	if err != nil || len(data) > 4096 {
		return nil, errors.New("bounded signed asset manifest unavailable")
	}
	signature, err := heldFile(filepath.Join(root, "assets.sig"), false, windows.GENERIC_READ|windows.READ_CONTROL, windows.FILE_SHARE_READ)
	if err != nil {
		return nil, err
	}
	authority.held = append(authority.held, signature)
	sig, err := io.ReadAll(io.LimitReader(signature, 89))
	if err != nil || len(sig) != 88 {
		return nil, errors.New("bounded detached signature unavailable")
	}
	authority.assets, err = VerifyAssetBundle(data, sig, bundleVerifierPublicKeyHex)
	if err != nil {
		return nil, err
	}
	assets := authority.assets.Assets
	managerStat, err := manager.image.Stat()
	if err != nil || uint64(managerStat.Size()) != assets[0].Size || manager.identity.ImageSHA256 != assets[0].SHA256 {
		return nil, errors.New("actual held manager is not the signed SDK asset")
	}
	// UAC may use another administrator identity, but the elevated process must
	// still be this exact fixed installed helper and share the original session.
	if helper {
		buffer := make([]uint16, 32768)
		size := uint32(len(buffer))
		if err = windows.QueryFullProcessImageName(windows.CurrentProcess(), 0, &buffer[0], &size); err != nil || size == 0 || size >= uint32(len(buffer)) || windows.UTF16ToString(buffer[:size]) != filepath.Join(root, "orbwin-lan-helper.exe") {
			return nil, errors.New("fixed installed elevated helper image required")
		}
	}
	for _, asset := range assets[1:] {
		file, e := heldFile(filepath.Join(base, filepath.FromSlash(asset.Path)), false, windows.GENERIC_READ|windows.READ_CONTROL, windows.FILE_SHARE_READ)
		if e != nil {
			return nil, e
		}
		authority.held = append(authority.held, file)
		sha, e := imageDigest(file)
		stat, statErr := file.Stat()
		if e != nil || sha != asset.SHA256 || statErr != nil || stat.Size() <= 0 || uint64(stat.Size()) != asset.Size {
			return nil, errors.New("actual held helper/worker differs from signed asset bundle")
		}
		if asset.Role == "worker" {
			authority.worker = file
		}
	}
	return authority, nil
}
func (authority *AuthorizedInstallation) Close() error {
	if authority == nil {
		return nil
	}
	return authority.closeUsing(func() error {
		var result error
		for index := len(authority.held) - 1; index >= 0; index-- {
			authority.closeDiagnostics = append(authority.closeDiagnostics, fmt.Sprintf("installation file=%#x", authority.held[index].Fd()))
			result = errors.Join(result, authority.held[index].Close())
		}
		authority.held = nil
		authority.worker = nil
		if authority.self != nil {
			result = errors.Join(result, authority.self.Close())
		}
		return result
	})
}

func (authority *AuthorizedInstallation) closeUsing(closeOwner func() error) error {
	if authority == nil || authority.nativeOrigin != authority {
		return errNativeOwnerCopy
	}
	authority.closeOnce.Do(func() {
		authority.closeErr = errNativeOwnerCloseUnknown
		authority.closeErr = closeOwner()
	})
	return authority.closeErr
}

// The fixed constructor passes owned.Close. A panic in that one cleanup call
// returns the same actual partial; unrelated work is outside this recovery.
func settleInstallationFailure(owned *AuthorizedInstallation, failure error, closeOwner func() error) (partial *AuthorizedInstallation, result error) {
	if owned != nil && owned.nativeOrigin != owned {
		return owned, errors.Join(failure, errNativeOwnerCopy)
	}
	partial, result = owned, failure
	closeReturned := false
	defer func() {
		if !closeReturned {
			owned.cleanupPanic = recover()
			partial, result = owned, errors.Join(failure, errNativeOwnerCloseUnknown)
		}
	}()
	closeErr := closeOwner()
	closeReturned = true
	if closeErr != nil {
		return owned, errors.Join(failure, closeErr)
	}
	return nil, failure
}
