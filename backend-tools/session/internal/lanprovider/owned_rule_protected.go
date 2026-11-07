package lanprovider

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanlease"
)

const ownedMetadataBudget = 16 << 10
const ownedProtectedRecordBudget = 8 << 20

type ownedProtectedMetadata struct {
	Version         int    `json:"version"`
	ReviewID        string `json:"reviewId"`
	ReviewSHA256    string `json:"reviewSha256"`
	OperationID     string `json:"operationId"`
	RuntimeInstance string `json:"runtimeInstance"`
	Prepared        struct {
		Spec     lanlease.Spec `json:"spec"`
		LeaseSHA string        `json:"leaseSha256"`
		Sequence uint64        `json:"sequence"`
	} `json:"prepared"`
	Child     lanfirewall.CallerIdentity `json:"child"`
	Namespace struct {
		ContainerID string `json:"containerId"`
		DaemonID    string `json:"daemonId"`
	} `json:"namespace"`
}
type ownedProtectedEvent struct {
	Version  int
	Phase    string
	Sequence uint64
	At       time.Time
	LeaseSHA string
	Receipt  lanfirewall.Receipt
	Native   ownedWireDefinition
}
type ownedProtectedObservation struct {
	policy          ownedPolicy
	definition      ownedDefinition
	sequence        uint64
	metadata        ownedProtectedMetadata
	journalPhase    string
	journalSequence uint64
}

func ownedHex(s string, n int) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == n && hex.EncodeToString(b) == s && s != strings.Repeat("0", n*2)
}

// Strict storage shape, distinct from LAN wire JSON: native typed array nulls
// are retained. Every struct field is required, case-sensitive and unique.
// Duplicate fields, unknown keys, trailing data and excessive token/depth/array
// budgets are refused before typed decoding. Only held protected files may call
// this parser; parsed bytes alone never establish provenance or authority.
func decodeOwnedProtected(data []byte, out any) error {
	if len(data) == 0 || len(data) > OwnedRuleEventBudget || !utf8.Valid(data) {
		return errors.New("protected LAN encoding budget")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	tokens := 0
	var visit func(reflect.Type, int) error
	visit = func(t reflect.Type, depth int) error {
		tokens++
		if tokens > 16384 || depth > 24 {
			return errors.New("protected LAN shape budget")
		}
		token, err := d.Token()
		if err != nil {
			return errors.New("protected LAN JSON incomplete")
		}
		if t == reflect.TypeOf(time.Time{}) {
			if _, ok := token.(string); !ok {
				return errors.New("protected LAN time shape")
			}
			return nil
		}
		if token == nil {
			if t.Kind() == reflect.Slice {
				return nil
			}
			return errors.New("protected LAN null shape")
		}
		switch t.Kind() {
		case reflect.Struct:
			if token != json.Delim('{') {
				return errors.New("protected LAN object shape")
			}
			fields := map[string]reflect.Type{}
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				name := strings.Split(f.Tag.Get("json"), ",")[0]
				if name == "" {
					name = f.Name
				}
				fields[name] = f.Type
			}
			seen := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				name, ok := key.(string)
				ft, exists := fields[name]
				if e != nil || !ok || !exists || seen[name] {
					return errors.New("protected LAN unknown/duplicate field")
				}
				seen[name] = true
				if e = visit(ft, depth+1); e != nil {
					return e
				}
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') || len(seen) != len(fields) {
				return errors.New("protected LAN missing field")
			}
		case reflect.Map:
			if token != json.Delim('{') {
				return errors.New("protected LAN map shape")
			}
			seen := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				name, ok := key.(string)
				if e != nil || !ok || seen[name] || len(seen) >= 64 || !schemaIdentifier.MatchString(name) {
					return errors.New("protected LAN map budget/identity")
				}
				seen[name] = true
				if e = visit(t.Elem(), depth+1); e != nil {
					return e
				}
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') {
				return errors.New("protected LAN map incomplete")
			}
		case reflect.Array, reflect.Slice:
			if token != json.Delim('[') {
				return errors.New("protected LAN array shape")
			}
			count := 0
			limit := 128
			if t.Kind() == reflect.Array {
				limit = t.Len()
			}
			for d.More() {
				count++
				if count > limit {
					return errors.New("protected LAN array budget")
				}
				if e := visit(t.Elem(), depth+1); e != nil {
					return e
				}
			}
			end, e := d.Token()
			if e != nil || end != json.Delim(']') || (t.Kind() == reflect.Array && count != t.Len()) {
				return errors.New("protected LAN array incomplete")
			}
		case reflect.String:
			if _, ok := token.(string); !ok {
				return errors.New("protected LAN string shape")
			}
		case reflect.Bool:
			if _, ok := token.(bool); !ok {
				return errors.New("protected LAN boolean shape")
			}
		default:
			if _, ok := token.(json.Number); !ok {
				return errors.New("protected LAN number shape")
			}
		}
		return nil
	}
	t := reflect.TypeOf(out)
	if t == nil || t.Kind() != reflect.Pointer {
		return errors.New("protected LAN decode target")
	}
	if err := visit(t.Elem(), 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("protected LAN trailing data")
	}
	if err := json.Unmarshal(data, out); err != nil {
		return errors.New("protected LAN typed value invalid")
	}
	return nil
}
func ownedInstanceFromWire(w ownedWireInstance, d typedDeclaration) (typedInstance, error) {
	out := typedInstance{class: w.Class, values: map[string]typedValue{}}
	if w.Class != d.class || len(w.Values) != len(d.properties) {
		return out, errors.New("protected LAN typed declaration differs")
	}
	budget := typedCopyBudget{}
	for n, v := range w.Values {
		if err := d.validate(n, v.Kind, v.Flags, false); err != nil {
			return out, err
		}
		if len(v.Shorts) > maxTypedArrayElements || len(v.Texts) > maxTypedArrayElements {
			return out, errors.New("protected LAN typed array budget")
		}
		budget.beginField()
		size := logicalStringBytes(v.Text) + len(v.Shorts)*2
		if len(v.Text) > maxTypedStringUnits*3 {
			return out, errors.New("protected LAN typed string budget")
		}
		for _, s := range v.Texts {
			if len(s) > maxTypedStringUnits*3 {
				return out, errors.New("protected LAN typed string budget")
			}
			size += logicalStringBytes(s)
		}
		if err := budget.reserve(size); err != nil {
			return out, err
		}
		out.values[n] = typedValue{kind: v.Kind, flags: v.Flags, scalar: v.Scalar, text: v.Text, shorts: v.Shorts, texts: v.Texts, date: typedDatetime{timestamp: v.Timestamp, parts: v.Parts, utc: v.UTC}}
	}
	return out, nil
}
func parseOwnedProtected(metaData, eventData []byte, installation, leaseSHA, sid string, declarations traversalDeclarations) (ownedProtectedObservation, error) {
	return parseOwnedProtectedMode(metaData, eventData, installation, leaseSHA, sid, declarations, false)
}
func parseOwnedProtectedMode(metaData, eventData []byte, installation, leaseSHA, sid string, declarations traversalDeclarations, cleanup bool) (ownedProtectedObservation, error) {
	var out ownedProtectedObservation
	bad := errors.New("protected native LAN record is incomplete or inconsistent")
	if len(metaData) > ownedMetadataBudget || len(eventData) == 0 || len(eventData) > ownedProtectedRecordBudget || !ownedHex(installation, 16) || !ownedHex(leaseSHA, 32) || eventData[len(eventData)-1] != '\n' {
		return out, bad
	}
	var meta ownedProtectedMetadata
	if decodeOwnedProtected(metaData, &meta) != nil {
		return out, bad
	}
	if meta.Version != 2 || !ownedHex(meta.ReviewID, 16) || !ownedHex(meta.ReviewSHA256, 32) || !ownedHex(meta.OperationID, 16) || !ownedHex(meta.RuntimeInstance, 16) || meta.Prepared.Sequence != 2 || meta.Prepared.LeaseSHA != leaseSHA {
		return out, bad
	}
	spec := meta.Prepared.Spec
	r := spec.Request
	if !ownedHex(meta.Namespace.ContainerID, 32) || meta.Namespace.ContainerID == spec.Publication.ContainerID || meta.Namespace.DaemonID != spec.Publication.Daemon {
		return out, bad
	}
	if r.Installation != installation || r.UserSID != sid || meta.ReviewSHA256 != r.ReviewSHA256 {
		return out, bad
	}
	issued, err := time.Parse(time.RFC3339Nano, r.IssuedAt)
	if err != nil {
		return out, bad
	}
	manager := lanfirewall.CallerIdentity{UserSID: sid, PID: r.CallerPID, CreationTime: r.CallerCreationTime, ImageSHA256: r.CallerImageSHA256}
	// Pure policy revalidation at original issue time does not authorize restart.
	lease, err := lanlease.NewOwner(installation, manager).Prepare(spec, issued)
	if err != nil || lease.Snapshot(issued).LeaseSHA != leaseSHA {
		return out, bad
	}
	childTime, e := strconv.ParseUint(meta.Child.CreationTime, 10, 64)
	parentTime, _ := strconv.ParseUint(r.CallerCreationTime, 10, 64)
	if e != nil || childTime == 0 || strconv.FormatUint(childTime, 10) != meta.Child.CreationTime || childTime < parentTime || meta.Child.PID == 0 || meta.Child.PID == r.CallerPID || meta.Child.UserSID != sid || meta.Child.ImageSHA256 != spec.Worker.SHA256 {
		return out, bad
	}
	lines := bytes.Split(eventData[:len(eventData)-1], []byte{'\n'})
	if len(lines) != 3 && (!cleanup || (len(lines) != 4 && len(lines) != 5)) {
		return out, bad
	} // removal pending/settled is not positive created ownership.
	var events [5]ownedProtectedEvent
	for i, line := range lines {
		if decodeOwnedProtected(line, &events[i]) != nil {
			return out, bad
		}
		v := events[i]
		if v.Version != 1 || v.Phase != []string{"create-intent", "create-returned", "created", "remove-intent", "removed"}[i] || v.Sequence != uint64(i+1) || v.LeaseSHA != leaseSHA || v.At.IsZero() || v.At.After(time.Now().UTC()) || i > 0 && v.At.Before(events[i-1].At) {
			return out, bad
		}
		if v.Receipt.Validate() != nil || v.Receipt.State != "create-intent" || v.Receipt.Sequence != 1 || !reflect.DeepEqual(v.Receipt.Request, r) || v.Receipt.Worker != spec.Worker {
			return out, bad
		}
		updated, _ := time.Parse(time.RFC3339Nano, v.Receipt.UpdatedAt)
		if v.At.Before(updated) {
			return out, bad
		}
		if i > 0 && !reflect.DeepEqual(v.Receipt, events[0].Receipt) {
			return out, bad
		}
		if i > 2 && !reflect.DeepEqual(v.Native, events[2].Native) {
			return out, bad
		}
	}
	receipt := events[2].Receipt
	if !reflect.DeepEqual(events[0].Native, ownedDefinitionWire(ownedDefinition{})) {
		return out, bad
	}
	if events[1].Native.Keys != events[2].Native.Keys {
		return out, bad
	}
	wire := events[2].Native
	keys := observedRuleKeys{wire.Keys[0], wire.Keys[1], wire.Keys[2], wire.Keys[3], wire.Keys[4]}
	if !reflect.DeepEqual(events[1].Native, ownedDefinitionWire(ownedDefinition{keys: keys})) {
		return out, bad
	}
	if declarations.typedRule.class == "" {
		declarations, err = ownedProtectedDeclarations(wire)
		if err != nil {
			return out, bad
		}
	}
	rule, err := ownedInstanceFromWire(wire.Rule, declarations.typedRule)
	if err != nil {
		return out, bad
	}
	definition := ownedDefinition{keys: keys, rule: rule}
	for i, f := range wire.Filters {
		definition.filters[i], err = ownedInstanceFromWire(f, declarations.filters[i])
		if err != nil {
			return out, bad
		}
	}
	profile := uint16(2)
	if r.Scope.Profile == "domain" {
		profile = 1
	}
	p := ownedPolicy{receipt: receipt, leaseSHA: leaseSHA, spec: lanfirewall.WMIDisabledCreate{Properties: lanfirewall.WMICreateProperties{InstanceID: receipt.RuleName, ElementName: "OrbWin selected LAN port", Description: "OrbWin exact-scope LAN exposure", RuleGroup: "OrbWin-LAN", Enabled: 1, Profiles: profile, Direction: 1, Action: 2}, Options: lanfirewall.WMICreateOptions{PolicyStore: persistentStore, LocalAddress: []string{r.Scope.LocalAddress}, RemoteAddress: append([]string(nil), r.Scope.ClientNetworks...), LocalPort: []string{strconv.Itoa(int(r.Scope.LocalPort))}, InterfaceAlias: []string{spec.Worker.InterfaceAlias}, Protocol: strings.ToUpper(r.Scope.Protocol), Program: spec.Worker.Path}}}
	if validateOwnedDefinition(definition, p) != nil {
		return out, bad
	}
	return ownedProtectedObservation{policy: p, definition: definition, sequence: 3, metadata: meta, journalPhase: events[len(lines)-1].Phase, journalSequence: uint64(len(lines))}, nil
}

func acknowledgeProtectedRule(observed ownedProtectedObservation, current ownedDefinition, readAt time.Time) (lanlease.RuleAcknowledgement, error) {
	if readAt.IsZero() || readAt.After(time.Now().UTC()) || !reflect.DeepEqual(current, observed.definition) {
		return lanlease.RuleAcknowledgement{}, errors.New("protected LAN exact native typed state changed or unavailable")
	}
	ack := newOwnedAck(observed.policy, current, observed.sequence)
	ack.ReadAt = readAt
	return ack, nil
}

func acknowledgeProtectedRemoval(observed ownedProtectedObservation, absent bool) (lanlease.RemovalAcknowledgement, error) {
	if !absent || observed.journalPhase != "removed" || observed.journalSequence != 5 {
		return lanlease.RemovalAcknowledgement{}, ErrCleanupPending
	}
	return lanlease.RemovalAcknowledgement{LeaseSHA: observed.policy.leaseSHA, NativeKeysSHA: ownedDigest(ownedKeysWire(observed.definition.keys)), Sequence: 5, TerminalSuccess: true, OperationClosed: true, ObservedAbsent: true, ProtectedReceipt: true, AcknowledgementFlushed: true}, nil
}

// Pure syntax/known-schema check before any MI call. This does not replace the
// fresh native GetClass gate; its declaration is checked again before readback.
func ownedProtectedDeclarations(w ownedWireDefinition) (traversalDeclarations, error) {
	var out traversalDeclarations
	c := Class{Name: ClassName, Namespace: Namespace, Provider: "wfascim"}
	for n, v := range w.Rule.Values {
		c.Properties = append(c.Properties, Property{Name: n, Type: v.Kind, Flags: v.Flags &^ instanceFieldStateFlags})
	}
	for _, n := range []string{"Enable", "Disable", "Rename", "CloneObject", "EnumerateFull"} {
		c.Methods = append(c.Methods, Method{Name: n, ReturnType: 5})
	}
	var err error
	out.rule, err = bindRuleInstanceDeclaration(c)
	if err != nil {
		return out, err
	}
	out.typedRule = typedRuleDeclaration(out.rule)
	for i := range out.filters {
		d := typedDeclaration{class: filterFamilies[i].class, properties: map[string]Property{}}
		for n, p := range commonFilterProperties {
			flags := filterReadOnlyMetadataFlags
			if p.key {
				flags = filterKeyMetadataFlags
			}
			d.properties[n] = Property{Name: n, Type: p.kind, Flags: flags}
		}
		for n, k := range familyFilterProperties[i] {
			d.properties[n] = Property{Name: n, Type: k, Flags: filterMutableMetadataFlags}
		}
		out.filters[i] = d
	}
	return out, nil
}
