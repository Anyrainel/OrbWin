package localnet

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"orbwin.local/session/internal/sdkstate"
	"reflect"
	"sort"
	"strings"
	"time"
)

// ColdStateReview is data inspection only. It never grants trust/removal,
// Windows/WSL mutation, signing readiness or permission to recreate a CA.
type ColdStateReview struct {
	Authority, Validity, Rotation string
	// Preservation does not establish usable signing or completed rotation.
	// Only a separate native recovery/signing admission may permit activation.
	Activation        string
	RootSHA256        string
	SettingsRevision  uint64
	ServicesEnabled   bool
	HistoricalRecords int
	DataSHA256        [32]byte
}
type ColdStateValidator struct {
	review    ColdStateReview
	validated bool
	now       func() time.Time
}

func NewColdStateValidator() *ColdStateValidator { return &ColdStateValidator{now: time.Now} }
func (v *ColdStateValidator) Review() (ColdStateReview, error) {
	if v == nil || !v.validated {
		return ColdStateReview{}, errors.New("cold SDK data not validated")
	}
	return v.review, nil
}
func (v *ColdStateValidator) ValidateColdSDK(snapshot *sdkstate.ColdSnapshot) error {
	if v == nil || snapshot == nil || v.validated {
		return errors.New("one actual cold snapshot required")
	}
	files := map[string][]byte{}
	for _, name := range snapshot.Names() {
		data, err := snapshot.Read(name)
		if err != nil {
			return err
		}
		files[name] = data
	}
	defer func() {
		for _, data := range files {
			clear(data)
		}
	}()
	now := time.Now()
	if v.now != nil {
		now = v.now()
	}
	review, err := validateColdStateRecords(snapshot.Scope(), files, now)
	if err != nil {
		return err
	}
	v.review = review
	v.validated = true
	return nil
}

func validateColdStateRecords(scope sdkstate.TransitionScope, files map[string][]byte, now time.Time) (ColdStateReview, error) {
	out := ColdStateReview{Authority: "none", Validity: "absent", Rotation: "none", Activation: "native-signing-admission-required"}
	var authority authorityState
	haveAuthority := false
	if data, ok := files[authorityStateName]; ok {
		if len(data) == 0 || len(data) > authorityStateBudget || coldJSON(data, &authority) != nil || authority.Version != 1 || authority.Installation != scope.Installation || len(authority.ProtectedSigning) > authorityBudget+4096+len(protectedKeyHeader) {
			return out, errors.New("cold SDK authority schema/owner/size differs; original retained")
		}
		// validateAuthorityPublic requires a signer blob; cold preservation must
		// also retain an authentic public-only/expired authority without replacing
		// it. No DPAPI call or readiness claim occurs at this boundary.
		root, err := validateAuthorityCertificate(authority.RootCertificate, scope.Installation, authority.RootSHA256)
		if err != nil {
			return out, err
		}
		haveAuthority = true
		out.Authority = "opaque-key"
		if len(authority.ProtectedSigning) == 0 {
			out.Authority = "public-only"
		}
		out.RootSHA256 = authority.RootSHA256
		out.Validity = "current"
		if now.Before(root.NotBefore) {
			out.Validity = "not-yet-valid"
		} else if !now.Before(root.NotAfter) {
			out.Validity = "expired"
		}
	}
	settings := SettingsSnapshot{Settings: DefaultSettings()}
	if data, ok := files[settingsName]; ok {
		var record settingsRecord
		if len(data) == 0 || len(data) > settingsBudget || coldJSON(data, &record) != nil || record.Version != 1 || record.Daemon != scope.Daemon || record.Revision == 0 || record.Settings.validate() != nil {
			return out, errors.New("cold SDK settings schema/daemon/revision differs; original retained")
		}
		settings = record.SettingsSnapshot
	}
	if settings.Settings.RootSHA256 != "" && (!haveAuthority || settings.Settings.RootSHA256 != authority.RootSHA256) {
		return out, errors.New("cold SDK active settings and authority disagree; no default/rotation applied")
	}
	out.SettingsRevision = settings.Revision
	out.ServicesEnabled = settings.Settings.Enabled
	if data, ok := files[rotationStateName]; ok {
		// All rotation bytes are historical-only, even a syntactically valid
		// record. Validation may explain its state; it cannot replay old actions.
		out.Rotation = "retained-recovery-required"
		out.Activation = "rotation-recovery-required"
		var rotation authorityRotation
		if len(data) > 0 && len(data) <= rotationBudget && coldJSON(data, &rotation) == nil && rotation.validate(scope.Daemon, scope.Installation) == nil {
			oldPair := haveAuthority && rotationDigest(files[authorityStateName]) == rotation.OldStateSHA256 && reflect.DeepEqual(settings, rotation.Old)
			newPair := haveAuthority && rotationDigest(files[authorityStateName]) == rotation.NewStateSHA256 && reflect.DeepEqual(settings, rotation.Target)
			if !oldPair && !newPair {
				return out, errors.New("cold SDK pending rotation has no coherent active pair; bytes retained without replay")
			}
			// Even a coherent pair does not prove whether a historical mutation
			// was dispatched or completed. Keep both states and require recovery.
			out.Rotation = "retained-coherent-recovery-required"
		}
	}
	for name := range files {
		if name != authorityStateName && name != settingsName {
			out.HistoricalRecords++
		}
	}
	// Hash exact opaque bytes, including DPAPI/DER and every quarantined record;
	// never put their bodies in diagnostics or an authorization receipt.
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	h := sha256.New()
	for _, name := range names {
		hash := sha256.Sum256(files[name])
		_ = json.NewEncoder(h).Encode(struct {
			Name, SHA256 string
			Size         int
		}{name, hex.EncodeToString(hash[:]), len(files[name])})
	}
	copy(out.DataSHA256[:], h.Sum(nil))
	return out, nil
}

func coldJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := coldJSONValue(decoder, 0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("cold SDK JSON suffix")
	}
	return strictJSON(data, target)
}
func coldJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 64 {
		return errors.New("cold SDK JSON nesting")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			name, ok := key.(string)
			if err != nil || !ok || seen[strings.ToLower(name)] {
				return errors.New("duplicate cold SDK JSON member")
			}
			seen[strings.ToLower(name)] = true
			if err = coldJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("cold SDK JSON object")
		}
	case '[':
		for decoder.More() {
			if err := coldJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("cold SDK JSON array")
		}
	default:
		return errors.New("cold SDK JSON delimiter")
	}
	return nil
}

var _ sdkstate.ColdValidator = (*ColdStateValidator)(nil)
