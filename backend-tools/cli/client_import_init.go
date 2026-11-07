package main

// Completed import is a root-bound admission codec, never an import provider.
// CLI cannot select/apply import or convert a pending marker into fresh defaults.
import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

var errClientBootstrapDesktop = errors.New("private client bootstrap needs an explicit empty-or-import choice in OrbWin desktop; CLI cannot apply this state")

const clientImportMarker = `{"schema":1,"kind":"legacy-import"}`
const clientImportPolicy = "legacy-transform-v3"
const clientImportNamespace = "orbwin-public-app-config-v1"

type clientBootstrapKind uint8

const (
	bootstrapUnsupported clientBootstrapKind = iota
	bootstrapChoiceRequired
	bootstrapFresh
	bootstrapImport
)

func clientBootstrapClass(body []byte) clientBootstrapKind {
	f := bytes.HasPrefix([]byte(freshClientMarker), body)
	i := bytes.HasPrefix([]byte(clientImportMarker), body)
	switch {
	case f && i:
		return bootstrapChoiceRequired
	case f:
		return bootstrapFresh
	case i:
		return bootstrapImport
	default:
		return bootstrapUnsupported
	}
}

type clientImportID struct {
	Volume uint32 `json:"volume"`
	High   uint32 `json:"high"`
	Low    uint32 `json:"low"`
}
type clientImportWitness struct {
	ID    clientImportID `json:"id"`
	SD    string         `json:"sdSha256"`
	SHA   string         `json:"bodySha256"`
	Bytes uint64         `json:"bytes"`
}
type clientImportInit struct {
	Schema          uint32              `json:"schema"`
	Product         string              `json:"product"`
	Admission       string              `json:"admission"`
	Policy          string              `json:"policy"`
	OwnerSID        string              `json:"ownerSid"`
	RootVolume      uint32              `json:"rootVolume"`
	RootHigh        uint32              `json:"rootHigh"`
	RootLow         uint32              `json:"rootLow"`
	Binding         string              `json:"binding"`
	SourceNamespace string              `json:"sourceNamespace"`
	Source          clientImportWitness `json:"source"`
	Initial         clientImportWitness `json:"initial"`
	InitialSelector string              `json:"initialSelector"`
}

func canonicalImportJSON(v any) ([]byte, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return nil, errClientConfigRecovery
	}
	p, e := checkedClientJSON(b)
	if e != nil {
		return nil, e
	}
	return json.Marshal(p)
}
func parseClientImportInit(body []byte, sid string, root clientImportID, profile string) (clientImportInit, error) {
	var v clientImportInit
	if len(body) > 16*1024 {
		return v, errClientConfigRecovery
	}
	if _, e := checkedClientJSON(body); e != nil {
		return v, errClientConfigRecovery
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if d.Decode(&v) != nil {
		return v, errClientConfigRecovery
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return v, errClientConfigRecovery
	}
	canonical, e := canonicalImportJSON(v)
	if e != nil || !bytes.Equal(canonical, body) {
		return v, errClientConfigRecovery
	}
	validWitness := func(w clientImportWitness) bool {
		return w.Bytes > 0 && w.Bytes <= clientConfigBudget && lowerDigest(w.SD) && lowerDigest(w.SHA)
	}
	if v.Schema != 2 || v.Product != "orbwin" || v.Admission != "legacy-import" || v.Policy != clientImportPolicy || v.SourceNamespace != clientImportNamespace || v.OwnerSID != sid || v.RootVolume != root.Volume || v.RootHigh != root.High || v.RootLow != root.Low || !lowerDigest(v.Binding) || v.Binding == zeroWriterDigest || !validWitness(v.Source) || !validWitness(v.Initial) || !validRecoverySelector(v.InitialSelector) || !freshSelector(v.InitialSelector, profile) {
		return v, errClientConfigRecovery
	}
	return v, nil
}
