//go:build windows

package localnet

import (
	"bytes"
	"errors"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DPAPI uses this Windows user's protection scope. UI_FORBIDDEN ensures setup
// never opens an unexpected prompt or falls back to unprotected/machine keys.
// Entropy binds adoption to the recorded installation/root, not a new secret.
func ProtectPrivateBundle(bundle []byte, installation, root string) ([]byte, error) {
	if len(bundle) == 0 || len(bundle) > authorityBudget {
		return nil, errors.New("invalid local signing bundle size")
	}
	entropy, err := protectionEntropy(installation, root)
	if err != nil {
		return nil, err
	}
	input := windows.DataBlob{Size: uint32(len(bundle)), Data: &bundle[0]}
	extra := windows.DataBlob{Size: uint32(len(entropy)), Data: &entropy[0]}
	var output windows.DataBlob
	err = windows.CryptProtectData(&input, nil, &extra, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output)
	runtime.KeepAlive(bundle)
	runtime.KeepAlive(entropy)
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(output.Data))))
	if output.Data == nil || output.Size == 0 || output.Size > authorityBudget+4096 {
		return nil, errors.New("OS-protected local key has an invalid size")
	}
	return append([]byte(protectedKeyHeader), unsafe.Slice(output.Data, int(output.Size))...), nil
}
func UnprotectPrivateBundle(ciphertext []byte, installation, root string) ([]byte, error) {
	if !bytes.HasPrefix(ciphertext, []byte(protectedKeyHeader)) || len(ciphertext) <= len(protectedKeyHeader) || len(ciphertext) > authorityBudget+4096+len(protectedKeyHeader) {
		return nil, errors.New("invalid protected local signing state")
	}
	entropy, err := protectionEntropy(installation, root)
	if err != nil {
		return nil, err
	}
	payload := ciphertext[len(protectedKeyHeader):]
	input := windows.DataBlob{Size: uint32(len(payload)), Data: &payload[0]}
	extra := windows.DataBlob{Size: uint32(len(entropy)), Data: &entropy[0]}
	var output windows.DataBlob
	err = windows.CryptUnprotectData(&input, nil, &extra, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output)
	runtime.KeepAlive(payload)
	runtime.KeepAlive(entropy)
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(output.Data))))
	if output.Data == nil || output.Size == 0 || output.Size > authorityBudget {
		return nil, errors.New("unprotected local signing state has an invalid size")
	}
	original := unsafe.Slice(output.Data, int(output.Size))
	result := append([]byte(nil), original...)
	clear(original)
	return result, nil
}
