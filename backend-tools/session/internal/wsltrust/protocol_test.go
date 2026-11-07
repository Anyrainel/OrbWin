package wsltrust

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"strings"
	"testing"
	"time"
)

func fixtureRequest(t *testing.T) Request {
	t.Helper()
	installation := "00000000-0000-4000-8000-000000000001"
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	profile := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "OrbWin local authority " + installation}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().AddDate(1, 0, 0), IsCA: true, BasicConstraintsValid: true, MaxPathLen: 1, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	der, err := x509.CreateCertificate(rand.Reader, profile, profile, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	return Request{Version: 1, Operation: "install", Installation: installation, DistributionID: "00000000-0000-4000-8000-000000000002", WindowsUser: "S-1-5-21-1", RootSHA256: Digest(der), RootCertificate: der, ExpectedMachineID: strings.Repeat("a", 32)}
}
func TestPublicOnlyTrustBoundaryRejectsPathsCommandsAndForeignRoot(t *testing.T) {
	request := fixtureRequest(t)
	if _, err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, fault := range []string{"version", "command", "installation", "registration", "user", "fingerprint", "certificate", "unreviewed-machine"} {
		t.Run(fault, func(t *testing.T) {
			bad := request
			switch fault {
			case "version":
				bad.Version = 2
			case "command":
				bad.Operation = "sh"
			case "installation":
				bad.Installation = "../root"
			case "registration":
				bad.DistributionID = "Ubuntu"
			case "user":
				bad.WindowsUser = "root"
			case "fingerprint":
				bad.RootSHA256 = strings.Repeat("0", 64)
			case "certificate":
				bad.RootCertificate = []byte("private key")
				bad.RootSHA256 = Digest(bad.RootCertificate)
			case "unreviewed-machine":
				bad.ExpectedMachineID = ""
			}
			if _, err := bad.Validate(); err == nil {
				t.Fatal("invalid public trust boundary accepted")
			}
		})
	}
	for _, raw := range []string{`{"version":1,"rootDir":"/tmp"}`, `{"version":1} {"operation":"install"}`} {
		var decoded Request
		if err := StrictJSON([]byte(raw), &decoded); err == nil {
			t.Fatal("untyped paths or concatenated requests accepted")
		}
	}
}
