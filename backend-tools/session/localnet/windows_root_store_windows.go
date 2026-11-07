//go:build windows

package localnet

import (
	"bytes"
	"errors"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsRootStore struct {
	mu      sync.Mutex
	handle  windows.Handle
	userID  string
	write   bool
	missing bool
}

// OpenCurrentUserRootStore opens only this user's physical Root registry store.
// It never opens the logical collection, LocalMachine, a policy store or a
// caller-supplied path. Merely opening the store never grants a trust mutation.
func OpenCurrentUserRootStore(write bool) (RootStore, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString("ROOT")
	if err != nil {
		return nil, err
	}
	flags := uint32(windows.CERT_SYSTEM_STORE_CURRENT_USER)
	if !write {
		flags |= windows.CERT_STORE_READONLY_FLAG | windows.CERT_STORE_OPEN_EXISTING_FLAG
	}
	handle, err := windows.CertOpenStore(windows.CERT_STORE_PROV_SYSTEM_REGISTRY_W, 0, 0, flags, uintptr(unsafe.Pointer(name)))
	if !write && (errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND)) {
		return &windowsRootStore{userID: user.User.Sid.String(), missing: true}, nil
	}
	if err != nil {
		return nil, err
	}
	return &windowsRootStore{handle: handle, userID: user.User.Sid.String(), write: write}, nil
}
func (store *windowsRootStore) UserID() string { return store.userID }
func (store *windowsRootStore) Close() error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.missing = false
	if store.handle == 0 {
		return nil
	}
	handle := store.handle
	store.handle = 0
	return windows.CertCloseStore(handle, 0)
}
func (store *windowsRootStore) find(der []byte) (*windows.CertContext, error) {
	if store.handle == 0 || len(der) == 0 || len(der) > 16384 {
		return nil, errors.New("open root store and bounded DER required")
	}
	var previous *windows.CertContext
	for count := 0; ; count++ {
		// Enumeration consumes previous even on failure or end of store.
		current, err := windows.CertEnumCertificatesInStore(store.handle, previous)
		if errors.Is(err, syscall.Errno(windows.CRYPT_E_NOT_FOUND)) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if current == nil {
			return nil, errors.New("root store returned no context without an error")
		}
		if count >= 4096 {
			return nil, errors.Join(errors.New("root store enumeration exceeds 4096 certificates"), windows.CertFreeCertificateContext(current))
		}
		if current.EncodedCert != nil && current.Length == uint32(len(der)) && bytes.Equal(unsafe.Slice(current.EncodedCert, int(current.Length)), der) {
			return current, nil
		}
		previous = current
	}
}
func (store *windowsRootStore) Contains(der []byte) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.missing && len(der) > 0 && len(der) <= 16384 {
		return false, nil
	}
	context, err := store.find(der)
	if err != nil || context == nil {
		return false, err
	}
	return true, windows.CertFreeCertificateContext(context)
}
func (store *windowsRootStore) Add(der []byte) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.handle == 0 || !store.write || len(der) == 0 || len(der) > 16384 {
		return errors.New("writable current-user root store and bounded DER required")
	}
	context, err := windows.CertCreateCertificateContext(windows.X509_ASN_ENCODING, &der[0], uint32(len(der)))
	if err != nil {
		return err
	}
	err = windows.CertAddCertificateContextToStore(store.handle, context, windows.CERT_STORE_ADD_NEW, nil)
	return errors.Join(err, windows.CertFreeCertificateContext(context))
}
func (store *windowsRootStore) Remove(der []byte) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if !store.write {
		return errors.New("root store was opened read-only")
	}
	context, err := store.find(der)
	if err != nil || context == nil {
		return err
	}
	// Native deletion consumes context on success AND failure. Never free twice.
	return windows.CertDeleteCertificateFromStore(context)
}
