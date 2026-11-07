package main

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"unicode/utf16"
)

const recoveryV2Budget = 16 * 1024

// Schema 2 is flat and bounded: neither previous journal bodies nor config or
// credential bytes enter the metadata. Selector data never admits a runtime.
// Struct order plus Go JSON string escaping is the cross-language producer wire
// format for a provable interrupted recovery.next prefix.
type writerRecoveryV2 struct {
	Schema               uint32 `json:"schema"`
	Kind                 string `json:"kind"`
	Binding              string `json:"binding"`
	CurrentVolume        uint32 `json:"currentVolume"`
	CurrentHigh          uint32 `json:"currentHigh"`
	CurrentLow           uint32 `json:"currentLow"`
	CurrentSD            string `json:"currentSdSha256"`
	CurrentSHA           string `json:"currentSha256"`
	RestoreSHA           string `json:"restoreSha256"`
	IntentBody           string `json:"intentBody"`
	TargetSelector       string `json:"targetSelector"`
	PublishedBaselineSHA string `json:"publishedBaselineSha256"`
	PreviousVolume       uint32 `json:"previousVolume"`
	PreviousHigh         uint32 `json:"previousHigh"`
	PreviousLow          uint32 `json:"previousLow"`
	PreviousSD           string `json:"previousSdSha256"`
	PreviousSHA          string `json:"previousSha256"`
	PreviousRestoreSHA   string `json:"previousRestoreSha256"`
	StagePresent         bool   `json:"stagePresent"`
	StageVolume          uint32 `json:"stageVolume"`
	StageHigh            uint32 `json:"stageHigh"`
	StageLow             uint32 `json:"stageLow"`
	StageSD              string `json:"stageSdSha256"`
	StageSHA             string `json:"stageSha256"`
}

func (v writerRecoveryV2) base() writerRecoveryRecord {
	return writerRecoveryRecord{v.Schema, v.Kind, v.Binding, v.CurrentVolume, v.CurrentHigh, v.CurrentLow, v.CurrentSD, v.CurrentSHA, v.RestoreSHA, v.IntentBody}
}
func parseRecoveryAny(body []byte) (base writerRecoveryRecord, next *writerRecoveryV2, resultErr error) {
	value, e := checkedClientJSON(body)
	if e != nil {
		return base, nil, errClientConfigRecovery
	}
	fields, ok := value.(map[string]any)
	if !ok {
		return base, nil, errClientConfigRecovery
	}
	if fields["schema"] == json.Number("1") {
		if closedWriterRecord(body, 8192, 10, &base) != nil {
			return base, nil, errClientConfigRecovery
		}
	} else {
		var v writerRecoveryV2
		if closedWriterRecord(body, recoveryV2Budget, 24, &v) != nil || v.Schema != 2 || !validRecoverySelector(v.TargetSelector) {
			return base, nil, errClientConfigRecovery
		}
		if v.PublishedBaselineSHA != "" && (!lowerDigest(v.PublishedBaselineSHA) || v.PreviousSHA == "") {
			return base, nil, errClientConfigRecovery
		}
		if v.PreviousSHA == "" {
			if v.PreviousVolume != 0 || v.PreviousHigh != 0 || v.PreviousLow != 0 || v.PreviousSD != "" || v.PreviousRestoreSHA != "" {
				return base, nil, errClientConfigRecovery
			}
		} else {
			if !lowerDigest(v.PreviousSHA) || !lowerDigest(v.PreviousSD) || !lowerDigest(v.PreviousRestoreSHA) {
				return base, nil, errClientConfigRecovery
			}
		}
		if !v.StagePresent {
			if v.StageVolume != 0 || v.StageHigh != 0 || v.StageLow != 0 || v.StageSD != "" || v.StageSHA != "" {
				return base, nil, errClientConfigRecovery
			}
		} else {
			if !lowerDigest(v.StageSD) || !lowerDigest(v.StageSHA) {
				return base, nil, errClientConfigRecovery
			}
		}
		base = v.base()
		next = &v
	}
	if base.Kind != "restore-original" || !lowerDigest(base.Binding) || base.Binding == zeroWriterDigest || !lowerDigest(base.CurrentSD) || !lowerDigest(base.CurrentSHA) || !lowerDigest(base.RestoreSHA) {
		return base, nil, errClientConfigRecovery
	}
	intent, e := parseWriterIntent([]byte(base.IntentBody))
	if e != nil || intent.Binding != base.Binding {
		return base, nil, errClientConfigRecovery
	}
	return base, next, nil
}
func validRecoverySelector(selector string) bool {
	if len(utf16.Encode([]rune(selector))) > 1024 {
		return false
	}
	at := strings.Index(selector, `\`+freshPrivateDirectory+`\runtime\docker-`)
	return at >= 3 && freshSelector(selector, selector[:at])
}
func recoveryV2From(v writerRecoveryRecord, selector string) writerRecoveryV2 {
	return writerRecoveryV2{Schema: 2, Kind: v.Kind, Binding: v.Binding, CurrentVolume: v.CurrentVolume, CurrentHigh: v.CurrentHigh, CurrentLow: v.CurrentLow, CurrentSD: v.CurrentSD, CurrentSHA: v.CurrentSHA, RestoreSHA: v.RestoreSHA, IntentBody: v.IntentBody, TargetSelector: selector}
}
func recoveryV2Bytes(v writerRecoveryV2) ([]byte, error) {
	body, e := json.Marshal(v)
	if e != nil || len(body) > recoveryV2Budget {
		return nil, errClientConfigRecovery
	}
	if _, _, e = parseRecoveryAny(body); e != nil {
		return nil, e
	}
	return body, nil
}
func sameRecoveryV2Authority(a, b writerRecoveryV2) bool {
	a.RestoreSHA, b.RestoreSHA = "", ""
	a.TargetSelector, b.TargetSelector = "", ""
	return reflect.DeepEqual(a, b)
}

// A partial NEXT supplies no authority. Exact fixed-order producer bytes must
// match independently admitted old/current/stage witnesses. Only the unknown
// prior selector generation and target digest are data-pattern variables.
func recoveryNextPrefix(body []byte, expected writerRecoveryV2, profile string) bool {
	if len(body) > recoveryV2Budget || !incompleteRecoveryRecord(body) {
		return false
	}
	expected.RestoreSHA = strings.Repeat("0", 64)
	expected.TargetSelector = profile + `\` + freshPrivateDirectory + `\runtime\docker-` + strings.Repeat("0", 64) + `\docker-cli-plugins`
	template, e := json.Marshal(expected)
	if e != nil || len(template) > recoveryV2Budget {
		return false
	}
	digestNeedle := []byte(`"restoreSha256":"`)
	digestStart := bytes.Index(template, digestNeedle) + len(digestNeedle)
	selectorNeedle := []byte(`"targetSelector":`)
	selectorStart := bytes.Index(template, selectorNeedle) + len(selectorNeedle)
	selectorPrefix, _ := json.Marshal(profile + `\` + freshPrivateDirectory + `\runtime\docker-`)
	hexStart := selectorStart + len(selectorPrefix) - 1
	if len(body) >= len(template) {
		return false
	}
	nonzero := false
	for i, b := range body {
		if i >= digestStart && i < digestStart+64 || i >= hexStart && i < hexStart+64 {
			if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f') {
				return false
			}
			if i >= hexStart && i < hexStart+64 {
				nonzero = nonzero || b != '0'
			}
		} else if b != template[i] {
			return false
		}
	}
	return len(body) < hexStart+64 || nonzero
}

func incompleteRecoveryRecord(body []byte) bool {
	if len(body) > recoveryV2Budget {
		return false
	}
	prefix := bytes.TrimLeft(body, " \t\r\n")
	if len(prefix) > 0 && prefix[0] != '{' {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	var value any
	e := decoder.Decode(&value)
	return e == io.EOF || e == io.ErrUnexpectedEOF
}

// No source retirement or config effect is permitted by this transition until
// full NEXT is durable and the independently held sources were revalidated.
// Publication failures are one-use; the native callback retains/quarantines.
func durableSuccessorTransition(durable, recheck, publish func() error) error {
	if durable == nil || recheck == nil || publish == nil {
		return errClientConfigRecovery
	}
	if e := durable(); e != nil {
		return e
	}
	if e := recheck(); e != nil {
		return e
	}
	return publish()
}

// This data predicate is not a native grant. The caller must independently
// derive prior from actual retained native ID/SD/digest and strict from the
// current private descriptor. It never infers a missing baseline from prestate.
func recoveryPublishedBaseline(v writerRecoveryRecord, v2 *writerRecoveryV2, prior, strict bool, sha string) bool {
	if !strict {
		return false
	}
	if sha == v.RestoreSHA {
		return true
	}
	return v2 != nil && v2.Binding == v.Binding && v2.IntentBody == v.IntentBody && v2.PreviousSHA != "" && v2.PublishedBaselineSHA != "" && prior && sha == v2.PublishedBaselineSHA
}

func ownedManagedSelector(body []byte, profile string) (string, error) {
	v, e := checkedDockerConfigJSON(body)
	if e != nil {
		return "", e
	}
	root, ok := v.(map[string]any)
	if !ok {
		return "", errClientConfigRecovery
	}
	dirs, ok := root["cliPluginsExtraDirs"].([]any)
	if !ok || len(dirs) != 1 {
		return "", errClientConfigRecovery
	}
	selector, ok := dirs[0].(string)
	if !ok || !freshSelector(selector, profile) || privateClientRouting(body, selector) != nil {
		return "", errClientConfigRecovery
	}
	return selector, nil
}

// Cleanup completion precedes the independent ordinary local replacement.
// A failed refresh does not resurrect the old birth or claim old authority.
func completedWriterRefresh(check, cleanup, refresh func() error) error {
	if check == nil || cleanup == nil || refresh == nil {
		return errClientConfigRecovery
	}
	if e := check(); e != nil {
		return e
	}
	if e := cleanup(); e != nil {
		return e
	}
	return refresh()
}

// A byte/data plan only, never a native guard or stopped-process certificate.
// Production supplies retained-native observations and checkAuthority; tests
// exercise this same precheck/rewrite coordinator with data and failure inputs.
type recoveryDataPlan struct {
	baseline, target []byte
	needsPublish     bool
}

func prepareRecoveryDataPlan(baseline, current []byte, profile, plugins string, fromOriginal bool, record *writerRecoveryRecord, v2 *writerRecoveryV2, prior, strict bool, checkAuthority func() error) (recoveryDataPlan, error) {
	var plan recoveryDataPlan
	if !fromOriginal {
		// Current B must be strict private and owned/canonical, independently of C.
		// Routing B through C here would make version-crossing admission unreachable.
		if !strict {
			return plan, errClientConfigRecovery
		}
		if _, e := ownedManagedSelector(current, profile); e != nil {
			return plan, e
		}
	}
	if checkAuthority == nil {
		return plan, errClientConfigRecovery
	}
	if e := checkAuthority(); e != nil {
		return plan, e
	}
	if !fromOriginal {
		if record == nil || !recoveryPublishedBaseline(*record, v2, prior, strict, digestBytes(current)) {
			return plan, errClientConfigRecovery
		}
		baseline = current
	}
	target, e := recoveryTarget(baseline, plugins, profile)
	if e != nil {
		return plan, e
	}
	if record != nil && v2 != nil {
		previous, e := recoveryTarget(baseline, v2.TargetSelector, profile)
		if e != nil || digestBytes(previous) != record.RestoreSHA {
			return plan, errClientConfigRecovery
		}
	}
	plan.baseline = append([]byte(nil), baseline...)
	plan.target = target
	plan.needsPublish = fromOriginal || !bytes.Equal(target, current)
	return plan, nil
}

func recoveryTarget(baseline []byte, selector, profile string) ([]byte, error) {
	if !freshSelector(selector, profile) || !validRecoverySelector(selector) {
		return nil, errClientConfigRecovery
	}
	next, changed, e := refreshPrivateManagedField(baseline, selector, profile)
	if e != nil {
		return nil, e
	}
	if !changed {
		next = append([]byte(nil), baseline...)
	}
	if privateClientRouting(next, selector) != nil {
		return nil, errClientConfigRecovery
	}
	return next, nil
}
