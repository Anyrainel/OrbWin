package lanprovider

import (
	"errors"
	"reflect"
	"unicode/utf16"
)

const maxTypedFieldBytes = 16 << 10
const maxTypedPayloadBytes = 64 << 10
const maxTypedStringUnits = 1024
const maxTypedArrayElements = 32

// All fields are private. These structures are RAM-only observations, never
// receipts, object paths, logs or authorization capabilities. Array order,
// NULL, empty values and exact field-state flags are deliberately preserved.
type typedDatetime struct {
	timestamp bool
	parts     [7]uint32
	utc       int32
}
type typedValue struct {
	kind, flags uint32
	scalar      uint32
	text        string
	shorts      []uint16
	texts       []string
	date        typedDatetime
}
type typedInstance struct {
	class  string
	values map[string]typedValue
}
type observedFilterKeys struct{ name, creationClassName, systemCreationClassName, systemName string }

type typedDeclaration struct {
	class             string
	properties        map[string]Property
	requireInstanceID bool
}
type traversalDeclarations struct {
	rule      ruleInstanceDeclaration
	typedRule typedDeclaration
	filters   [7]typedDeclaration
}

func typedRuleDeclaration(rule ruleInstanceDeclaration) typedDeclaration {
	return typedDeclaration{class: ClassName, properties: rule.properties, requireInstanceID: true}
}
func typedFilterDeclaration(value FilterSchema, index int) (typedDeclaration, error) {
	if index < 0 || index >= 7 {
		return typedDeclaration{}, errors.New("typed filter index unsupported")
	}
	if err := validatePinnedFilterSchema(value, index); err != nil {
		return typedDeclaration{}, err
	}
	d := typedDeclaration{class: filterFamilies[index].class, properties: make(map[string]Property, len(value.Class.Properties))}
	for _, p := range value.Class.Properties {
		d.properties[p.Name] = p
	}
	return d, nil
}
func (d typedDeclaration) validate(name string, kind, flags uint32, duplicate bool) error {
	p, ok := d.properties[name]
	if !ok || duplicate || kind != p.Type || (flags&^instanceFieldStateFlags) != (p.Flags&^instanceFieldStateFlags) {
		return errors.New("fixed typed instance metadata differs from fresh declaration")
	}
	if flags&instanceNullFlag != 0 && (p.Flags&4096 != 0 || (d.requireInstanceID && name == "InstanceID")) {
		return errors.New("fixed typed identity is null")
	}
	return nil
}

// The payload budget is cumulative logical copied value bytes, not a promise
// about Go heap/working-set size. Metadata and Go container overhead are fixed
// by separate class/field/result bounds. No payload is truncated to fit.
type typedCopyBudget struct{ used, field int }

func (b *typedCopyBudget) beginField() { b.field = 0 }
func (b *typedCopyBudget) remaining() int {
	a, c := maxTypedFieldBytes-b.field, maxTypedPayloadBytes-b.used
	if a < c {
		return a
	}
	return c
}
func (b *typedCopyBudget) reserve(bytes int) error {
	if bytes < 0 || bytes > b.remaining() {
		return errors.New("typed native value payload budget exceeded")
	}
	b.used += bytes
	b.field += bytes
	return nil
}
func logicalStringBytes(s string) int {
	n := 2
	for _, r := range s {
		n += utf16.RuneLen(r) * 2
	}
	return n
}
func (t typedInstance) ruleKeys() (observedRuleKeys, error) {
	if t.class != ClassName {
		return observedRuleKeys{}, errors.New("typed rule class differs")
	}
	names := [5]string{"InstanceID", "CreationClassName", "PolicyRuleName", "SystemCreationClassName", "SystemName"}
	var texts [5]string
	for i, name := range names {
		v, ok := t.values[name]
		if !ok || v.kind != 13 || v.flags&instanceNullFlag != 0 {
			return observedRuleKeys{}, errors.New("typed rule identity incomplete")
		}
		texts[i] = v.text
	}
	return observedRuleKeys{texts[0], texts[1], texts[2], texts[3], texts[4]}, nil
}
func (t typedInstance) filterKeys(index int) (observedFilterKeys, error) {
	if index < 0 || index >= 7 || t.class != filterFamilies[index].class {
		return observedFilterKeys{}, errors.New("typed filter class differs")
	}
	names := [4]string{"Name", "CreationClassName", "SystemCreationClassName", "SystemName"}
	var texts [4]string
	for i, name := range names {
		v, ok := t.values[name]
		if !ok || v.kind != 13 || v.flags&instanceNullFlag != 0 {
			return observedFilterKeys{}, errors.New("typed filter identity incomplete")
		}
		texts[i] = v.text
	}
	return observedFilterKeys{texts[0], texts[1], texts[2], texts[3]}, nil
}
func sameTypedInstance(a, b typedInstance) bool { return reflect.DeepEqual(a, b) }
