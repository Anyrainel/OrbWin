// Package localnet contains the local-service authority and route boundaries.
// It never installs trust, edits DNS or starts an engine as a library side effect.
package localnet

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"
)

var serviceSuffixes = []string{".orbwin.localhost", ".orbwin.test"}

const authorityBudget = 64 << 10

// Only an intermediate key is retained. The root signing key never becomes a
// field, serialized artifact, exported value or guest input.
type Authority struct {
	root, intermediate *x509.Certificate
	signer             *ecdsa.PrivateKey
	revocation         authorityRevocation
	crlMu              sync.Mutex
	crlCache           []byte
	crlRefresh         time.Time
	crlIssued          time.Time
}
type privateAuthority struct {
	Version         int                 `json:"version"`
	Root            []byte              `json:"root"`
	Intermediate    []byte              `json:"intermediate"`
	IntermediateKey []byte              `json:"intermediateKey"`
	Revocation      authorityRevocation `json:"revocation"`
}

func serial() (*big.Int, error) {
	value, err := rand.Int(rand.Reader, new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 128), big.NewInt(1)))
	if err != nil {
		return nil, err
	}
	return value.Add(value, big.NewInt(1)), nil
}
func fingerprint(der []byte) string { hash := sha256.Sum256(der); return hex.EncodeToString(hash[:]) }

func NewAuthority(installation string, now time.Time) (*Authority, error) {
	return NewAuthorityWithCRLPort(installation, now, 80)
}
func NewAuthorityWithCRLPort(installation string, now time.Time, httpPort uint16) (*Authority, error) {
	if httpPort == 0 {
		return nil, errors.New("reviewed local revocation HTTP port required")
	}
	if !regexp.MustCompile(`^[a-f0-9-]{32,36}$`).MatchString(installation) {
		return nil, errors.New("recorded installation identifier is required")
	}
	rootKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	rootSerial, err := serial()
	if err != nil {
		return nil, err
	}
	rootTemplate := &x509.Certificate{SerialNumber: rootSerial, Subject: pkix.Name{CommonName: "OrbWin local authority " + installation}, NotBefore: now.Add(-5 * time.Minute), NotAfter: now.AddDate(2, 0, 0), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, MaxPathLen: 1}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &rootKey.PublicKey, rootKey)
	if err != nil {
		return nil, err
	}
	root, err := x509.ParseCertificate(rootDER)
	if err != nil {
		return nil, err
	}
	signer, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	intermediateSerial, err := serial()
	if err != nil {
		return nil, err
	}
	intermediateTemplate := &x509.Certificate{SerialNumber: intermediateSerial, Subject: pkix.Name{CommonName: "OrbWin constrained local services"}, NotBefore: now.Add(-5 * time.Minute), NotAfter: now.AddDate(1, 0, 0), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, MaxPathLen: 0, MaxPathLenZero: true, PermittedDNSDomainsCritical: true, PermittedDNSDomains: append([]string(nil), serviceSuffixes...), ExcludedIPRanges: []*net.IPNet{{IP: net.IPv4zero, Mask: net.CIDRMask(0, 32)}, {IP: net.IPv6zero, Mask: net.CIDRMask(0, 128)}}, CRLDistributionPoints: []string{authorityCRLURL(httpPort, fingerprint(root.Raw), "root")}}
	intermediateDER, err := x509.CreateCertificate(rand.Reader, intermediateTemplate, root, &signer.PublicKey, rootKey)
	if err != nil {
		return nil, err
	}
	intermediate, err := x509.ParseCertificate(intermediateDER)
	if err != nil {
		return nil, err
	}
	rootCRL, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{Number: big.NewInt(1), ThisUpdate: root.NotBefore, NextUpdate: intermediate.NotAfter}, root, rootKey)
	if err != nil {
		return nil, err
	}
	authority := &Authority{root: root, intermediate: intermediate, signer: signer, revocation: authorityRevocation{HTTPPort: httpPort, RootCRL: rootCRL}}
	return authority, authority.validate(now)
}
func (authority *Authority) validate(now time.Time) error {
	if authority == nil || authority.root == nil || authority.intermediate == nil || authority.signer == nil {
		return errors.New("incomplete local authority")
	}
	root, intermediate := authority.root, authority.intermediate
	if !root.IsCA || !root.BasicConstraintsValid || root.MaxPathLen != 1 || root.CheckSignatureFrom(root) != nil {
		return errors.New("invalid local root certificate")
	}
	if !intermediate.IsCA || !intermediate.BasicConstraintsValid || intermediate.MaxPathLen != 0 || !intermediate.MaxPathLenZero || intermediate.KeyUsage != x509.KeyUsageCertSign|x509.KeyUsageCRLSign || !intermediate.PermittedDNSDomainsCritical || len(intermediate.PermittedDNSDomains) != 2 || len(intermediate.ExtKeyUsage) != 1 || intermediate.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth || len(intermediate.PermittedIPRanges) != 0 || len(intermediate.PermittedURIDomains) != 0 || len(intermediate.PermittedEmailAddresses) != 0 {
		return errors.New("local intermediate certificate profile differs")
	}
	if len(intermediate.ExcludedIPRanges) != 2 || intermediate.ExcludedIPRanges[0].String() != "0.0.0.0/0" || intermediate.ExcludedIPRanges[1].String() != "::/0" {
		return errors.New("local intermediate must exclude all IP server identities")
	}
	if err := authority.validateRevocation(now); err != nil {
		return err
	}
	for index, suffix := range serviceSuffixes {
		if intermediate.PermittedDNSDomains[index] != suffix {
			return errors.New("local certificate namespace constraints differ")
		}
	}
	expected, err := x509.MarshalPKIXPublicKey(&authority.signer.PublicKey)
	if err != nil {
		return err
	}
	actual, err := x509.MarshalPKIXPublicKey(intermediate.PublicKey)
	if err != nil || !bytes.Equal(expected, actual) || authority.signer.Curve != elliptic.P256() {
		return errors.New("local intermediate key differs")
	}
	roots := x509.NewCertPool()
	roots.AddCert(root)
	if _, err = intermediate.Verify(x509.VerifyOptions{Roots: roots, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
		return fmt.Errorf("local authority is not valid: %w", err)
	}
	return nil
}

// PrivateBundle must be OS-protected before writing it anywhere. It is not a
// backup/export format and is never suitable for a guest or diagnostic log.
func (authority *Authority) PrivateBundle() ([]byte, error) {
	if authority == nil || authority.root == nil || authority.intermediate == nil || authority.signer == nil {
		return nil, errors.New("incomplete local authority")
	}
	key, err := x509.MarshalPKCS8PrivateKey(authority.signer)
	if err != nil {
		return nil, err
	}
	defer clear(key)
	return json.Marshal(privateAuthority{Version: 2, Root: authority.root.Raw, Intermediate: authority.intermediate.Raw, IntermediateKey: key, Revocation: authority.revocation})
}
func OpenAuthority(bundle []byte, expectedRoot string, now time.Time) (*Authority, error) {
	if len(bundle) > authorityBudget {
		return nil, errors.New("local authority bundle exceeds 64 KiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(bundle))
	decoder.DisallowUnknownFields()
	var stored privateAuthority
	if err := decoder.Decode(&stored); err != nil {
		return nil, err
	}
	var suffix any
	if decoder.Decode(&suffix) != io.EOF {
		return nil, errors.New("unexpected local authority suffix")
	}
	if stored.Version != 2 || len(stored.Root) > 16384 || len(stored.Intermediate) > 16384 || len(stored.IntermediateKey) > 16384 || fingerprint(stored.Root) != expectedRoot {
		return nil, errors.New("local authority version or recorded root differs")
	}
	root, err := x509.ParseCertificate(stored.Root)
	if err != nil {
		return nil, err
	}
	intermediate, err := x509.ParseCertificate(stored.Intermediate)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParsePKCS8PrivateKey(stored.IntermediateKey)
	if err != nil {
		return nil, err
	}
	signer, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("unsupported local authority key")
	}
	authority := &Authority{root: root, intermediate: intermediate, signer: signer, revocation: stored.Revocation}
	return authority, authority.validate(now)
}
func (authority *Authority) RootCertificate() []byte {
	return append([]byte(nil), authority.root.Raw...)
}
func (authority *Authority) Fingerprint() string         { return fingerprint(authority.root.Raw) }
func (authority *Authority) SigningExpiresAt() time.Time { return authority.intermediate.NotAfter }
func CanonicalServiceName(host string) (string, error) {
	for index := range len(host) {
		if host[index] >= 128 {
			return "", errors.New("service names must use ASCII DNS labels")
		}
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if len(host) > 253 || strings.ContainsAny(host, "\x00:/\\*") {
		return "", errors.New("select a reserved local service DNS name")
	}
	suffix := ""
	for _, candidate := range serviceSuffixes {
		if strings.HasSuffix(host, candidate) {
			suffix = candidate
			break
		}
	}
	if suffix == "" || len(host) == len(suffix) {
		return "", errors.New("name is outside OrbWin's reserved service namespaces")
	}
	for _, label := range strings.Split(host, ".") {
		if !regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`).MatchString(label) {
			return "", errors.New("service names must use canonical ASCII DNS labels")
		}
	}
	return host, nil
}
func (authority *Authority) Leaf(host string, now time.Time) (tls.Certificate, error) {
	var result tls.Certificate
	host, err := CanonicalServiceName(host)
	if err != nil {
		return result, err
	}
	if err = authority.validate(now); err != nil {
		return result, err
	}
	expiry := now.Add(24 * time.Hour)
	if expiry.After(authority.intermediate.NotAfter) {
		return result, errors.New("local signing certificate needs rotation before issuing another leaf")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return result, err
	}
	number, err := serial()
	if err != nil {
		return result, err
	}
	template := &x509.Certificate{SerialNumber: number, Subject: pkix.Name{CommonName: host}, DNSNames: []string{host}, NotBefore: now.Add(-5 * time.Minute), NotAfter: expiry, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true, CRLDistributionPoints: []string{authorityCRLURL(authority.revocation.HTTPPort, authority.Fingerprint(), "signer")}}
	der, err := x509.CreateCertificate(rand.Reader, template, authority.intermediate, &key.PublicKey, authority.signer)
	if err != nil {
		return result, err
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return result, err
	}
	return tls.Certificate{Certificate: [][]byte{der, authority.intermediate.Raw}, PrivateKey: key, Leaf: leaf}, nil
}
