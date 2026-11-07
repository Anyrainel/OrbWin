package localnet

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAuthorityCRLsHaveVerifiedIssuerBoundsAndNoPrivateRootState(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	authority, err := NewAuthorityWithCRLPort("00000000-0000-4000-8000-000000000001", now, 18080)
	if err != nil {
		t.Fatal(err)
	}
	root, err := x509.ParseRevocationList(authority.revocation.RootCRL)
	if err != nil {
		t.Fatal(err)
	}
	if err = root.CheckSignatureFrom(authority.root); err != nil || !root.NextUpdate.Equal(authority.intermediate.NotAfter) {
		t.Fatal(err)
	}
	first, err := authority.signerCRL(now)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := x509.ParseRevocationList(first)
	if err != nil {
		t.Fatal(err)
	}
	if err = signer.CheckSignatureFrom(authority.intermediate); err != nil || !signer.NextUpdate.Equal(now.Add(24*time.Hour)) || len(signer.RevokedCertificateEntries) != 0 {
		t.Fatal(err)
	}
	first[0] ^= 1
	second, err := authority.signerCRL(now.Add(time.Hour))
	if err != nil || bytes.Equal(first, second) {
		t.Fatal("caller could modify cached CRL", err)
	}
	leaf, err := authority.Leaf("app.orbwin.localhost", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(leaf.Leaf.CRLDistributionPoints) != 1 || leaf.Leaf.CRLDistributionPoints[0] != authorityCRLURL(18080, authority.Fingerprint(), "signer") {
		t.Fatal("leaf lacks exact local revocation URI")
	}
	bundle, err := authority.PrivateBundle()
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenAuthority(bundle, authority.Fingerprint(), now)
	if err != nil || reopened.RevocationHTTPPort() != 18080 {
		t.Fatal(err)
	}
	var stored privateAuthority
	if err = json.Unmarshal(bundle, &stored); err != nil {
		t.Fatal(err)
	}
	stored.Revocation.RootCRL[len(stored.Revocation.RootCRL)-1] ^= 1
	damaged, _ := json.Marshal(stored)
	if _, err = OpenAuthority(damaged, authority.Fingerprint(), now); err == nil {
		t.Fatal("tampered root CRL loaded")
	}
	stored.Version = 1
	legacy, _ := json.Marshal(stored)
	if _, err = OpenAuthority(legacy, authority.Fingerprint(), now); err == nil {
		t.Fatal("legacy authority silently upgraded")
	}
}
func TestPublicCRLHandlerIsExactReadOnlyAndPreservesWorkloadRouting(t *testing.T) {
	authority, err := NewAuthorityWithCRLPort("00000000-0000-4000-8000-000000000002", time.Now(), 18080)
	if err != nil {
		t.Fatal(err)
	}
	nextCalls := 0
	handler := authority.PublicCRLHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { nextCalls++; w.WriteHeader(418) }))
	path := authorityCRLPath(authority.Fingerprint(), "signer")
	for _, fixture := range []struct {
		method, path, host string
		code               int
	}{
		{"GET", path, "127.0.0.1:18080", 200},
		{"HEAD", path, "127.0.0.1:18080", 200},
		{"POST", path, "127.0.0.1:18080", 405},
		{"GET", path, "app.orbwin.localhost:18080", 404},
		{"GET", path, "127.0.0.1:18081", 404},
		{"GET", path + "?extra=1", "127.0.0.1:18080", 404},
		{"GET", "/workload", "app.orbwin.localhost:18080", 418},
	} {
		request := httptest.NewRequest(fixture.method, fixture.path, nil)
		request.Host = fixture.host
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != fixture.code {
			t.Fatalf("%s %s %s: %d", fixture.method, fixture.path, fixture.host, response.Code)
		}
		if fixture.code == 200 && response.Header().Get("Content-Type") != "application/pkix-crl" {
			t.Fatal("wrong public CRL MIME type")
		}
		if fixture.method == "HEAD" && response.Body.Len() != 0 {
			t.Fatal("HEAD wrote signed bytes")
		}
	}
	if nextCalls != 1 {
		t.Fatal("authority endpoint entered workload dispatch")
	}
}

func TestSignerCRLRefreshDoesNotReturnFutureCachedMetadataAfterClockRollback(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	authority, err := NewAuthorityWithCRLPort("00000000-0000-4000-8000-000000000003", now.Add(-24*time.Hour), 18080)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = authority.signerCRL(now); err != nil {
		t.Fatal(err)
	}
	rolledBack := now.Add(-time.Hour)
	encoded, err := authority.signerCRL(rolledBack)
	if err != nil {
		t.Fatal(err)
	}
	list, err := x509.ParseRevocationList(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.Before(list.ThisUpdate) || !rolledBack.Before(list.NextUpdate) {
		t.Fatal("future/expired CRL escaped cache after clock rollback")
	}
}
