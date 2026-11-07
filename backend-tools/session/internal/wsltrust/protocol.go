// Package wsltrust implements the public-certificate-only Linux trust boundary.
// Its CLI accepts neither filesystem paths nor arbitrary executable arguments.
package wsltrust

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
)

const RequestBudget = 32768
const ResponseBudget = 32768

var guid = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
var installationID = regexp.MustCompile(`^(?:[a-f0-9]{32}|[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12})$`)
var fingerprintID = regexp.MustCompile(`^[a-f0-9]{64}$`)
var userSID = regexp.MustCompile(`^S-1-[0-9]+(?:-[0-9]+){1,15}$`)
var machineID = regexp.MustCompile(`^[a-f0-9]{32}$`)

type Registration struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func ValidateHelperAsset(asset []byte, digest string) error {
	if len(asset) < 64 || len(asset) > 16<<20 || !fingerprintID.MatchString(digest) || Digest(asset) != digest || !bytes.Equal(asset[:7], []byte{0x7f, 'E', 'L', 'F', 2, 1, 1}) || binary.LittleEndian.Uint16(asset[18:20]) != 62 {
		return errors.New("attested Linux amd64 trust helper required")
	}
	return nil
}

type Request struct {
	Version           int    `json:"version"`
	Operation         string `json:"operation"`
	Installation      string `json:"installation"`
	DistributionID    string `json:"distributionId"`
	WindowsUser       string `json:"windowsUser"`
	RootSHA256        string `json:"rootSha256"`
	RootCertificate   []byte `json:"rootCertificate"`
	ExpectedSequence  uint64 `json:"expectedSequence"`
	ExpectedMachineID string `json:"expectedMachineId,omitempty"`
}
type Status struct {
	DistributionID string `json:"distributionId"`
	MachineID      string `json:"machineId"`
	OSID           string `json:"osId"`
	OSVersion      string `json:"osVersion"`
	RootSHA256     string `json:"rootSha256"`
	Observed       bool   `json:"observed"`
	FilePresent    bool   `json:"filePresent"`
	BundlePresent  bool   `json:"bundlePresent"`
	Managed        bool   `json:"managed"`
	State          string `json:"state"`
	Sequence       uint64 `json:"sequence"`
	UpdaterWarning bool   `json:"updaterWarning"`
}
type Response struct {
	Version int     `json:"version"`
	Status  *Status `json:"status,omitempty"`
	Error   string  `json:"error,omitempty"`
}

func Digest(data []byte) string             { value := sha256.Sum256(data); return hex.EncodeToString(value[:]) }
func ValidRegistrationID(value string) bool { return guid.MatchString(value) }
func StrictJSON(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("unexpected trust protocol suffix")
	}
	return nil
}
func (request Request) Validate() (*x509.Certificate, error) {
	if request.Version != 1 || !installationID.MatchString(request.Installation) || !guid.MatchString(request.DistributionID) || len(request.WindowsUser) > 184 || !userSID.MatchString(request.WindowsUser) || !fingerprintID.MatchString(request.RootSHA256) {
		return nil, errors.New("trust protocol version or reviewed identity is invalid")
	}
	if request.Operation != "status" && request.Operation != "install" && request.Operation != "remove" {
		return nil, errors.New("unsupported distro trust operation")
	}
	if (request.Operation != "status" || request.ExpectedMachineID != "") && !machineID.MatchString(request.ExpectedMachineID) {
		return nil, errors.New("reviewed distro machine identity required before trust changes")
	}
	if len(request.RootCertificate) == 0 || len(request.RootCertificate) > 16384 || Digest(request.RootCertificate) != request.RootSHA256 {
		return nil, errors.New("bounded reviewed public root required")
	}
	root, err := x509.ParseCertificate(request.RootCertificate)
	if err != nil {
		return nil, err
	}
	key, ok := root.PublicKey.(*ecdsa.PublicKey)
	if !ok || key.Curve != elliptic.P256() || !root.IsCA || !root.BasicConstraintsValid || root.MaxPathLen != 1 || root.MaxPathLenZero || root.KeyUsage != x509.KeyUsageCertSign|x509.KeyUsageCRLSign || len(root.ExtKeyUsage) != 0 || len(root.UnknownExtKeyUsage) != 0 || len(root.UnhandledCriticalExtensions) != 0 || root.Subject.CommonName != "OrbWin local authority "+request.Installation || root.CheckSignatureFrom(root) != nil {
		return nil, errors.New("public root differs from the supported OrbWin authority profile")
	}
	return root, nil
}
