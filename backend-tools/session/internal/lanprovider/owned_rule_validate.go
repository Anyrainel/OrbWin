package lanprovider

import (
	"errors"
	"reflect"
)

// A complete fresh schema bounds every additional property. Required scope must
// match exactly; no query/name adoption, wildcard expansion or missing filter is
// accepted. The full 39+seven-family typed state is retained for later drift
// refusal, including fields that merely further restrict the requested ALLOW.
func validateOwnedDefinition(d ownedDefinition, p ownedPolicy) error {
	if !completeOwnedKeys(d.keys) || d.keys.instanceID != p.receipt.RuleName || d.rule.class != ClassName || len(d.rule.values) != 39 {
		return errors.New("LAN full typed rule identity incomplete")
	}
	keys, err := d.rule.ruleKeys()
	if err != nil || keys != d.keys {
		return errors.New("LAN typed and returned rule keys differ")
	}
	scalar := func(t typedInstance, n string, k, w uint32) bool {
		v, ok := t.values[n]
		return ok && v.kind == k && v.flags&instanceNullFlag == 0 && v.scalar == w
	}
	text := func(t typedInstance, n, w string) bool {
		v, ok := t.values[n]
		return ok && v.kind == 13 && v.flags&instanceNullFlag == 0 && v.text == w
	}
	array := func(t typedInstance, n string, w []string) bool {
		v, ok := t.values[n]
		return ok && v.kind == 29 && v.flags&instanceNullFlag == 0 && reflect.DeepEqual(v.texts, w)
	}
	for name, want := range map[string]uint32{"Enabled": 1, "Action": 2, "Direction": 1, "Profiles": uint32(p.spec.Properties.Profiles), "EdgeTraversalPolicy": 0} {
		if !scalar(d.rule, name, 3, want) {
			return errors.New("LAN required rule semantics differ")
		}
	}
	if !text(d.rule, "Description", p.spec.Properties.Description) || !text(d.rule, "ElementName", p.spec.Properties.ElementName) || !text(d.rule, "RuleGroup", p.spec.Properties.RuleGroup) || !scalar(d.rule, "LooseSourceMapping", 0, 0) || !scalar(d.rule, "LocalOnlyMapping", 0, 0) {
		return errors.New("LAN fixed identity or mapping differs")
	}
	for i, f := range d.filters {
		if len(f.values) != len(commonFilterProperties)+len(familyFilterProperties[i]) || !scalar(f, "IsNegated", 0, 0) {
			return errors.New("LAN full filter or negation unavailable")
		}
		if _, err = f.filterKeys(i); err != nil {
			return err
		}
	}
	if !array(d.filters[0], "LocalAddress", p.spec.Options.LocalAddress) || !array(d.filters[0], "RemoteAddress", p.spec.Options.RemoteAddress) || !text(d.filters[1], "AppPath", p.spec.Options.Program) || !array(d.filters[2], "InterfaceAlias", p.spec.Options.InterfaceAlias) || !array(d.filters[4], "LocalPort", p.spec.Options.LocalPort) || !text(d.filters[4], "Protocol", p.spec.Options.Protocol) || !scalar(d.filters[5], "OverrideBlockRules", 0, 0) {
		return errors.New("LAN exact native scope differs")
	}
	return nil
}
