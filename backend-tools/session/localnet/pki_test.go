package localnet

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func authorityFixture(t *testing.T) (*Authority, time.Time) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Second)
	authority, err := NewAuthority("00000000-0000-4000-8000-000000000001", now)
	if err != nil {
		t.Fatal(err)
	}
	return authority, now
}
func chainOptions(authority *Authority, name string, now time.Time) x509.VerifyOptions {
	roots := x509.NewCertPool()
	roots.AddCert(authority.root)
	intermediates := x509.NewCertPool()
	intermediates.AddCert(authority.intermediate)
	return x509.VerifyOptions{Roots: roots, Intermediates: intermediates, DNSName: name, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
}

func TestLocalAuthorityRoundTripAndVerifiedTLSHandshake(t *testing.T) {
	authority, now := authorityFixture(t)
	bundle, err := authority.PrivateBundle()
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]json.RawMessage
	if err = json.Unmarshal(bundle, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored) != 5 || stored["rootKey"] != nil {
		t.Fatal("root private signing state escaped into persistent bundle")
	}
	reopened, err := OpenAuthority(bundle, authority.Fingerprint(), now)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := reopened.Leaf("App.Project.OrbWin.Test.", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = leaf.Leaf.Verify(chainOptions(reopened, "app.project.orbwin.test", now)); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "verified-local-service") }))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{leaf}}
	server.StartTLS()
	defer server.Close()
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: chainOptions(reopened, "", now).Roots, ServerName: "app.project.orbwin.test", MinVersion: tls.VersionTLS12}}
	defer transport.CloseIdleConnections()
	client := http.Client{Transport: transport, Timeout: 3 * time.Second}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "verified-local-service" {
		t.Fatalf("TLS service response differed: %q %v", body, err)
	}
}

func TestExposedIntermediateKeyCannotAuthenticateUnrelatedDomainsOrSubordinateCAs(t *testing.T) {
	authority, now := authorityFixture(t)
	leaf, err := authority.Leaf("app.orbwin.localhost", now)
	if err != nil {
		t.Fatal(err)
	}
	malicious := *leaf.Leaf
	malicious.DNSNames = []string{"example.com"}
	malicious.Subject.CommonName = "example.com"
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificate(rand.Reader, &malicious, authority.intermediate, &key.PublicKey, authority.signer)
	if err != nil {
		t.Fatal(err)
	}
	forged, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = forged.Verify(chainOptions(authority, "example.com", now)); err == nil {
		t.Fatal("compromised ordinary signing key authenticated an unrelated domain")
	}
	for _, address := range []string{"203.0.113.9", "127.0.0.1", "2001:db8::9", "::1"} {
		malicious.DNSNames = nil
		malicious.IPAddresses = []net.IP{net.ParseIP(address)}
		malicious.Subject.CommonName = address
		ipDER, signErr := x509.CreateCertificate(rand.Reader, &malicious, authority.intermediate, &key.PublicKey, authority.signer)
		if signErr != nil {
			t.Fatal(signErr)
		}
		ipLeaf, parseErr := x509.ParseCertificate(ipDER)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		if _, verifyErr := ipLeaf.Verify(chainOptions(authority, address, now)); verifyErr == nil {
			t.Fatal("compromised signer authenticated an IP server outside its DNS scope", address)
		}
	}
	malicious.DNSNames = []string{"app.orbwin.test"}
	malicious.IPAddresses = nil
	malicious.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	clientDER, err := x509.CreateCertificate(rand.Reader, &malicious, authority.intermediate, &key.PublicKey, authority.signer)
	if err != nil {
		t.Fatal(err)
	}
	clientLeaf, err := x509.ParseCertificate(clientDER)
	if err != nil {
		t.Fatal(err)
	}
	clientOptions := chainOptions(authority, "", now)
	clientOptions.KeyUsages = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	if _, err = clientLeaf.Verify(clientOptions); err == nil {
		t.Fatal("compromised server signer authenticated a client identity")
	}
	subordinate := *authority.intermediate
	subordinate.Subject.CommonName = "forged subordinate"
	subordinate.SerialNumber, _ = serial()
	subordinate.NotAfter = now.Add(48 * time.Hour)
	der, err = x509.CreateCertificate(rand.Reader, &subordinate, authority.intermediate, &key.PublicKey, authority.signer)
	if err != nil {
		t.Fatal(err)
	}
	subcert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	child := *leaf.Leaf
	child.SerialNumber, _ = serial()
	der, err = x509.CreateCertificate(rand.Reader, &child, subcert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	forged, err = x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	options := chainOptions(authority, "app.orbwin.localhost", now)
	options.Intermediates.AddCert(subcert)
	if _, err = forged.Verify(options); err == nil {
		t.Fatal("ordinary signing key created an accepted subordinate CA")
	}
}

func TestLocalIssuanceAndAuthorityParsingFailClosed(t *testing.T) {
	authority, now := authorityFixture(t)
	for _, name := range []string{"example.com", "orbwin.test", "app.orbwin.test.evil", "*.orbwin.test", "127.0.0.1", "app.orbwin.test:443", "app..orbwin.test", "é.orbwin.test", "-app.orbwin.test", strings.Repeat("a", 64) + ".orbwin.test", "app/orbwin.test"} {
		if _, err := authority.Leaf(name, now); err == nil {
			t.Fatalf("issued outside canonical namespace: %q", name)
		}
	}
	bundle, err := authority.PrivateBundle()
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]byte{append(append([]byte{}, bundle...), []byte(" {}")...), append(append([]byte{}, bundle...), []byte(" !broken")...), make([]byte, authorityBudget+1)} {
		if _, err := OpenAuthority(invalid, authority.Fingerprint(), now); err == nil {
			t.Fatal("accepted malformed/oversized authority")
		}
	}
	if _, err = OpenAuthority(bundle, strings.Repeat("0", 64), now); err == nil {
		t.Fatal("adopted another root fingerprint")
	}
	if _, err = authority.Leaf("app.orbwin.test", now.AddDate(0, 0, 365)); err == nil {
		t.Fatal("issued past intermediate rotation boundary")
	}
	if _, err = OpenAuthority(bundle, authority.Fingerprint(), now.AddDate(3, 0, 0)); err == nil {
		t.Fatal("accepted expired authority")
	}
	public := authority.RootCertificate()
	public[0] ^= 1
	if _, err = x509.ParseCertificate(authority.RootCertificate()); err != nil {
		t.Fatal("public accessor exposed mutable authority certificate")
	}
}
