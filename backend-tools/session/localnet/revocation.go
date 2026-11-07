package localnet

import (
	"bytes"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"time"
)

type authorityRevocation struct {
	HTTPPort uint16 `json:"httpPort"`
	RootCRL  []byte `json:"rootCrl"`
}

func authorityCRLPath(root, kind string) string { return "/.orbwin/ca/" + root + "/" + kind + ".crl" }
func authorityCRLURL(port uint16, root, kind string) string {
	return "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port))) + authorityCRLPath(root, kind)
}
func (authority *Authority) RevocationHTTPPort() uint16 { return authority.revocation.HTTPPort }
func (authority *Authority) validateRevocation(now time.Time) error {
	if authority.revocation.HTTPPort == 0 || len(authority.revocation.RootCRL) > 4096 {
		return errors.New("local revocation profile exceeds its bounds")
	}
	if len(authority.intermediate.CRLDistributionPoints) != 1 || authority.intermediate.CRLDistributionPoints[0] != authorityCRLURL(authority.revocation.HTTPPort, authority.Fingerprint(), "root") {
		return errors.New("local intermediate revocation endpoint differs")
	}
	list, err := x509.ParseRevocationList(authority.revocation.RootCRL)
	if err != nil {
		return err
	}
	if list.CheckSignatureFrom(authority.root) != nil || !bytes.Equal(list.RawIssuer, authority.root.RawSubject) || !bytes.Equal(list.AuthorityKeyId, authority.root.SubjectKeyId) || list.Number == nil || list.Number.Cmp(big.NewInt(1)) != 0 || len(list.RevokedCertificateEntries) != 0 || len(list.RevokedCertificates) != 0 || !list.ThisUpdate.Equal(authority.root.NotBefore) || !list.NextUpdate.Equal(authority.intermediate.NotAfter) || now.Before(list.ThisUpdate) || !now.Before(list.NextUpdate) {
		return errors.New("local root revocation metadata signature, profile or validity differs")
	}
	if len(list.Extensions) != 2 {
		return errors.New("unexpected root revocation extensions")
	}
	for _, extension := range list.Extensions {
		if extension.Critical || (extension.Id.String() != "2.5.29.35" && extension.Id.String() != "2.5.29.20") {
			return errors.New("unsupported root revocation extension")
		}
	}
	return nil
}
func (authority *Authority) signerCRL(now time.Time) ([]byte, error) {
	authority.crlMu.Lock()
	defer authority.crlMu.Unlock()
	if err := authority.validate(now); err != nil {
		return nil, err
	}
	if len(authority.crlCache) != 0 && !now.Before(authority.crlIssued) && now.Before(authority.crlRefresh) {
		return bytes.Clone(authority.crlCache), nil
	}
	expiry := now.Add(24 * time.Hour)
	if expiry.After(authority.intermediate.NotAfter) {
		expiry = authority.intermediate.NotAfter
	}
	encoded, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{Number: big.NewInt(now.UnixNano()), ThisUpdate: now.Add(-5 * time.Minute), NextUpdate: expiry}, authority.intermediate, authority.signer)
	if err != nil {
		return nil, err
	}
	authority.crlCache = encoded
	authority.crlIssued = now.Add(-5 * time.Minute).UTC()
	authority.crlRefresh = now.Add(12 * time.Hour)
	return bytes.Clone(encoded), nil
}

// PublicCRLHandler serves two exact signed DER resources; it cannot read a file,
// mutate authority state, proxy a request or expose private signing material.
func (authority *Authority) PublicCRLHandler(next http.Handler) http.Handler {
	rootPath := authorityCRLPath(authority.Fingerprint(), "root")
	signerPath := authorityCRLPath(authority.Fingerprint(), "signer")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != rootPath && r.URL.Path != signerPath {
			next.ServeHTTP(w, r)
			return
		}
		if r.URL.IsAbs() || r.URL.Host != "" || r.URL.RawQuery != "" || r.Host != net.JoinHostPort("127.0.0.1", strconv.Itoa(int(authority.revocation.HTTPPort))) {
			http.Error(w, "Unknown authority resource.", http.StatusNotFound)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "Read-only authority resource.", http.StatusMethodNotAllowed)
			return
		}
		now := time.Now()
		if err := authority.validate(now); err != nil {
			http.Error(w, "Local authority requires reviewed rotation.", http.StatusServiceUnavailable)
			return
		}
		encoded := authority.revocation.RootCRL
		if r.URL.Path == signerPath {
			var err error
			encoded, err = authority.signerCRL(now)
			if err != nil {
				http.Error(w, "Local revocation metadata is unavailable.", http.StatusServiceUnavailable)
				return
			}
		}
		w.Header().Set("Content-Type", "application/pkix-crl")
		w.Header().Set("Content-Length", fmt.Sprint(len(encoded)))
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method == http.MethodGet {
			_, _ = w.Write(encoded)
		}
	})
}
