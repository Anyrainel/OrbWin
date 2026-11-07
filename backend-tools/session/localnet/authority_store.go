package localnet

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const authorityStateBudget = 128 << 10
const authorityStateName = "authority-v1.json"

type authorityState struct {
	Version          int    `json:"version"`
	Installation     string `json:"installation"`
	RootSHA256       string `json:"rootSha256"`
	RootCertificate  []byte `json:"rootCertificate"`
	ProtectedSigning []byte `json:"protectedSigning"`
}
type AuthorityInfo struct {
	RootSHA256           string    `json:"rootSha256"`
	RootNotBefore        time.Time `json:"rootNotBefore"`
	RootNotAfter         time.Time `json:"rootNotAfter"`
	PublicCertificatePEM []byte    `json:"publicCertificatePem"`
}

// Creation never replaces an existing record, including expired/damaged state.
// If encrypted-draft cleanup fails after publication, a non-nil authority and
// error report the committed state; retrying Create still cannot overwrite it.
func (ledger *NamespaceLedger) CreateAuthority(installation string, now time.Time) (created *Authority, resultErr error) {
	return ledger.CreateAuthorityWithCRLPort(installation, now, 80)
}
func (ledger *NamespaceLedger) CreateAuthorityWithCRLPort(installation string, now time.Time, httpPort uint16) (created *Authority, resultErr error) {
	if err := ledger.lockState(nil); err != nil {
		return nil, err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() {
		return nil, errors.New("local-services owner is closed")
	}
	path := filepath.Join(ledger.directory, authorityStateName)
	if _, err := ledger.readState(path, authorityStateBudget); err == nil {
		return nil, errors.New("local authority state already exists; load/review it instead of replacing it")
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	authority, err := NewAuthorityWithCRLPort(installation, now, httpPort)
	if err != nil {
		return nil, err
	}
	if err := ledger.checkState(); err != nil {
		return nil, err
	}
	data, err := encodeProtectedAuthority(authority, installation)
	if err != nil {
		return nil, err
	}
	if len(data) > authorityStateBudget {
		return nil, errors.New("protected local authority state exceeds its budget")
	}
	if ledger.stateStore != nil {
		if err := ledger.createState(path, data); err != nil {
			return nil, err
		}
		return authority, nil
	}
	temporary, err := os.CreateTemp(ledger.directory, ".authority-*.partial")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := os.Remove(temporary.Name()); err != nil && !os.IsNotExist(err) {
			resultErr = errors.Join(resultErr, fmt.Errorf("review encrypted authority draft cleanup: %w", err))
		}
	}()
	if _, err = temporary.Write(data); err == nil {
		err = temporary.Sync()
	}
	err = errors.Join(err, temporary.Close())
	if err != nil {
		return nil, err
	}
	// A same-directory exclusive hard link is the existing Windows/NTFS
	// publication primitive used for new file copies; never rename over a root.
	if err = os.Link(temporary.Name(), path); err != nil {
		return nil, fmt.Errorf("exclusive authority publication failed; existing state was preserved: %w", err)
	}
	return authority, nil
}

func encodeProtectedAuthority(authority *Authority, installation string) ([]byte, error) {
	bundle, err := authority.PrivateBundle()
	if err != nil {
		return nil, err
	}
	defer clear(bundle)
	protected, err := ProtectPrivateBundle(bundle, installation, authority.Fingerprint())
	if err != nil {
		return nil, err
	}
	return json.Marshal(authorityState{Version: 1, Installation: installation, RootSHA256: authority.Fingerprint(), RootCertificate: authority.RootCertificate(), ProtectedSigning: protected})
}

func validateAuthorityPublic(state authorityState, installation string) (*x509.Certificate, error) {
	if state.Version != 1 || state.Installation != installation || len(state.RootCertificate) == 0 || len(state.RootCertificate) > 16384 || len(state.ProtectedSigning) == 0 || len(state.ProtectedSigning) > authorityBudget+4096+len(protectedKeyHeader) {
		return nil, errors.New("local authority state version, installation or size differs")
	}
	return validateAuthorityCertificate(state.RootCertificate, installation, state.RootSHA256)
}

func validateAuthorityCertificate(der []byte, installation, rootSHA256 string) (*x509.Certificate, error) {
	if len(der) == 0 || len(der) > 16384 {
		return nil, errors.New("bounded public authority certificate required")
	}
	if _, err := protectionEntropy(installation, rootSHA256); err != nil {
		return nil, err
	}
	if fingerprint(der) != rootSHA256 {
		return nil, errors.New("stored public root does not match its fingerprint")
	}
	root, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, errors.New("stored local root certificate is invalid")
	}
	key, ok := root.PublicKey.(*ecdsa.PublicKey)
	if !ok || key.Curve != elliptic.P256() || !root.IsCA || !root.BasicConstraintsValid || root.MaxPathLen != 1 || root.MaxPathLenZero || root.KeyUsage != x509.KeyUsageCertSign|x509.KeyUsageCRLSign || len(root.ExtKeyUsage) != 0 || len(root.UnknownExtKeyUsage) != 0 || len(root.UnhandledCriticalExtensions) != 0 || root.Subject.CommonName != "OrbWin local authority "+installation || root.CheckSignatureFrom(root) != nil {
		return nil, errors.New("stored public root differs from the supported local certificate profile")
	}
	return root, nil
}
func (ledger *NamespaceLedger) readAuthority(installation string) (authorityState, *x509.Certificate, error) {
	var state authorityState
	path := filepath.Join(ledger.directory, authorityStateName)
	data, err := ledger.readState(path, authorityStateBudget)
	if err != nil {
		return state, nil, err
	}
	if len(data) > authorityStateBudget {
		return state, nil, errors.New("local authority state exceeds 128 KiB")
	}
	if err := strictJSON(data, &state); err != nil {
		return state, nil, errors.New("local authority state is incomplete or unsupported; retain it for review")
	}
	root, err := validateAuthorityPublic(state, installation)
	return state, root, err
}

// Info is public-certificate review/export, not proof of signing-key integrity,
// current trust or validity at the current time. A trust transaction must Load
// successfully with this reviewed fingerprint before installing the certificate.
func (ledger *NamespaceLedger) AuthorityInfo(installation string) (out AuthorityInfo, resultErr error) {
	if err := ledger.lockState(nil); err != nil {
		return AuthorityInfo{}, err
	}
	defer ledger.finishState(&resultErr)
	if !ledger.owned() {
		return AuthorityInfo{}, errors.New("local-services owner is closed")
	}
	state, root, err := ledger.readAuthority(installation)
	if err != nil {
		return AuthorityInfo{}, err
	}
	return AuthorityInfo{RootSHA256: state.RootSHA256, RootNotBefore: root.NotBefore, RootNotAfter: root.NotAfter, PublicCertificatePEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: state.RootCertificate})}, nil
}
func (ledger *NamespaceLedger) LoadAuthority(installation, expectedRoot string, now time.Time) (out *Authority, resultErr error) {
	if err := ledger.lockState(nil); err != nil {
		return nil, err
	}
	defer ledger.finishState(&resultErr)
	return ledger.loadAuthorityLocked(installation, expectedRoot, now)
}

func (ledger *NamespaceLedger) loadAuthorityLocked(installation, expectedRoot string, now time.Time) (*Authority, error) {
	if !ledger.owned() {
		return nil, errors.New("local-services owner is closed")
	}
	if _, err := protectionEntropy(installation, expectedRoot); err != nil {
		return nil, err
	}
	state, _, err := ledger.readAuthority(installation)
	if err != nil {
		return nil, err
	}
	if state.RootSHA256 != expectedRoot {
		return nil, errors.New("stored authority differs from the reviewed root; no signing state loaded")
	}
	if err := ledger.checkState(); err != nil {
		return nil, err
	}
	return openAuthorityState(state, installation, expectedRoot, now)
}

func openAuthorityState(state authorityState, installation, expectedRoot string, now time.Time) (*Authority, error) {
	bundle, err := UnprotectPrivateBundle(state.ProtectedSigning, installation, expectedRoot)
	if err != nil {
		return nil, errors.New("Windows could not verify/decrypt local signing state for this installation/user")
	}
	defer clear(bundle)
	authority, err := OpenAuthority(bundle, expectedRoot, now)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(authority.RootCertificate(), state.RootCertificate) {
		return nil, errors.New("public certificate and protected signing state differ")
	}
	return authority, nil
}
