package lanprovider

import "errors"

// SDK10.0.22621.0 mi.h lines603/605: field state, not class declaration,
// permission, ownership, memory borrowing or a provider change lease. Microsoft
// OMI GetElementAt reconstructs exactly these state bits from each field while
// retaining the declaration flags. OMI corroborates the interface semantics;
// it is not evidence that Windows ships that implementation.
const instanceNullFlag uint32 = 1 << 29
const instanceNotModifiedFlag uint32 = 1 << 25
const instanceFieldStateFlags = instanceNullFlag | instanceNotModifiedFlag

type ruleInstanceDeclaration struct{ properties map[string]Property }

// Capture the current complete qualified GetClass declaration before any rule
// instance dispatch. Never infer declaration flags from a prior failed instance
// or accept the union of every documented MI flag.
func bindRuleInstanceDeclaration(class Class) (ruleInstanceDeclaration, error) {
	if err := ValidateClass(class); err != nil {
		return ruleInstanceDeclaration{}, err
	}
	properties := make(map[string]Property, len(class.Properties))
	for _, p := range class.Properties {
		properties[p.Name] = p
	}
	return ruleInstanceDeclaration{properties: properties}, nil
}

func (d ruleInstanceDeclaration) validate(index uint32, name string, kind, flags uint32, duplicate bool) (selected bool, err error) {
	p, ok := d.properties[name]
	if !ok || duplicate || kind != p.Type || (flags&^instanceFieldStateFlags) != (p.Flags&^instanceFieldStateFlags) {
		return false, sanitizedInstanceMetadataMismatch(index, name, kind, flags, duplicate)
	}
	selected = name == "InstanceID" || p.Flags&(1<<12) != 0
	if selected && flags&instanceNullFlag != 0 {
		return false, errors.New("native identity is null")
	}
	return selected, nil
}
