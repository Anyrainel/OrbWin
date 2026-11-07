//go:build windows && amd64

package lanprovider

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"unsafe"
)

// Compiled-only SDK layout checks bind these two array layouts and MI_Datetime
// before any opt-in traversal. No native pointer survives the borrowed result.
type nativeShortArray struct {
	data  *uint16
	count uint32
}
type nativeStringArray struct {
	data  **uint16
	count uint32
}
type nativeDatetimeValue struct {
	isTimestamp uint32
	parts       [8]uint32
}

func copyNativeTypedString(value *uint16, budget *typedCopyBudget) (string, error) {
	remaining := budget.remaining()
	if remaining < 2 {
		return "", errors.New("typed native string budget exceeded")
	}
	units := (remaining - 2) / 2
	if units > maxTypedStringUnits {
		units = maxTypedStringUnits
	}
	s, err := stringCopy(value, units)
	if err != nil {
		return "", err
	}
	if err = budget.reserve(logicalStringBytes(s)); err != nil {
		return "", err
	}
	return s, nil
}

func decodeNativeTypedValue(kind, flags uint32, value *miValue, budget *typedCopyBudget) (out typedValue, err error) {
	out = typedValue{kind: kind, flags: flags}
	defer func() {
		if err != nil {
			out = typedValue{}
		}
	}()
	switch kind {
	case 0, 3, 5, 12, 13, 19, 29:
	default:
		return out, errors.New("native typed value kind is unsupported")
	}
	if flags&instanceNullFlag != 0 {
		return out, nil
	}
	switch kind {
	case 0:
		v := *(*uint8)(unsafe.Pointer(value))
		if v > 1 {
			return out, errors.New("native typed boolean is invalid")
		}
		err = budget.reserve(1)
		out.scalar = uint32(v)
	case 3:
		err = budget.reserve(2)
		out.scalar = uint32(*(*uint16)(unsafe.Pointer(value)))
	case 5:
		err = budget.reserve(4)
		out.scalar = *(*uint32)(unsafe.Pointer(value))
	case 12:
		v := (*nativeDatetimeValue)(unsafe.Pointer(value))
		if v.isTimestamp > 1 {
			return out, errors.New("native datetime discriminator is invalid")
		}
		err = budget.reserve(36)
		out.date.timestamp = v.isTimestamp == 1
		if out.date.timestamp {
			copy(out.date.parts[:], v.parts[:7])
			out.date.utc = int32(v.parts[7])
		} else {
			copy(out.date.parts[:5], v.parts[:5])
		} // Ignore interval padding, not logical date fields.
	case 13:
		out.text, err = copyNativeTypedString(*(**uint16)(unsafe.Pointer(value)), budget)
	case 19:
		a := (*nativeShortArray)(unsafe.Pointer(value))
		if a.count > maxTypedArrayElements || (a.count > 0 && a.data == nil) {
			return out, errors.New("native typed short array exceeds bounds or is malformed")
		}
		if err = budget.reserve(16 + int(a.count)*2); err != nil {
			return out, err
		}
		out.shorts = append([]uint16{}, unsafe.Slice(a.data, int(a.count))...)
	case 29:
		a := (*nativeStringArray)(unsafe.Pointer(value))
		if a.count > maxTypedArrayElements || (a.count > 0 && a.data == nil) {
			return out, errors.New("native typed string array exceeds bounds or is malformed")
		}
		if err = budget.reserve(16); err != nil {
			return out, err
		}
		out.texts = make([]string, 0, int(a.count))
		for _, text := range unsafe.Slice(a.data, int(a.count)) {
			s, e := copyNativeTypedString(text, budget)
			if e != nil {
				return out, e
			}
			out.texts = append(out.texts, s)
		}
	default:
		return out, errors.New("native typed value kind is unsupported")
	}
	return out, err
}

func copyTypedInstance(ctx context.Context, object *instanceObject, declaration typedDeclaration, budget *typedCopyBudget) (out typedInstance, err error) {
	return copyTypedDeclaredInstance(ctx, object, declaration, budget, false, false)
}

// Identity-only reads still check every returned metadata field but never ask
// for unselected private values. A WQL projection may return five fields or all
// 39; a reverse full-instance association must return all 39.
func copyTypedRuleIdentity(ctx context.Context, object *instanceObject, declaration typedDeclaration, budget *typedCopyBudget, projection bool) (observedRuleKeys, error) {
	value, err := copyTypedDeclaredInstance(ctx, object, declaration, budget, true, projection)
	if err != nil {
		return observedRuleKeys{}, err
	}
	return value.ruleKeys()
}

func copyTypedDeclaredInstance(ctx context.Context, object *instanceObject, declaration typedDeclaration, budget *typedCopyBudget, identityOnly, projection bool) (out typedInstance, err error) {
	if object == nil || object.table == nil || len(declaration.properties) == 0 || len(declaration.properties) > 64 {
		return out, errors.New("typed native instance/declaration unavailable")
	}
	table := (*instanceReadTable)(unsafe.Pointer(object.table))
	var class, namespace *uint16
	if err = callContext(ctx, "MI typed instance class", table.getClassName, uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(&class))); err != nil {
		return out, err
	}
	name, e := stringCopy(class, 128)
	if e != nil || name != declaration.class {
		return out, errors.New("typed native instance class differs")
	}
	if err = callContext(ctx, "MI typed instance namespace", table.getNamespace, uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(&namespace))); err != nil {
		return out, err
	}
	ns, e := stringCopy(namespace, 128)
	if e != nil || !strings.EqualFold(strings.ReplaceAll(ns, `\`, "/"), Namespace) {
		return out, errors.New("typed native instance namespace differs")
	}
	var count uint32
	if err = callContext(ctx, "MI typed instance count", table.getElementCount, uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(&count))); err != nil {
		return out, err
	}
	if count != uint32(len(declaration.properties)) && !(identityOnly && projection && count == 5) {
		return out, errors.New("typed native instance field count differs")
	}
	out = typedInstance{class: declaration.class, values: make(map[string]typedValue, int(count))}
	seen := make(map[string]bool, int(count))
	for i := uint32(0); i < count; i++ {
		var field *uint16
		var kind, flags uint32
		err = callContext(ctx, "MI typed metadata", table.getElementAt, uintptr(unsafe.Pointer(object)), uintptr(i), uintptr(unsafe.Pointer(&field)), 0, uintptr(unsafe.Pointer(&kind)), uintptr(unsafe.Pointer(&flags)))
		if err != nil {
			return typedInstance{}, err
		}
		name, e := stringCopy(field, 128)
		if e != nil {
			return typedInstance{}, e
		}
		if err = declaration.validate(name, kind, flags, seen[name]); err != nil {
			return typedInstance{}, err
		}
		seen[name] = true
		if identityOnly && name != "InstanceID" && declaration.properties[name].Flags&4096 == 0 {
			if count == 5 {
				return typedInstance{}, errors.New("typed rule identity projection differs")
			}
			continue
		}
		budget.beginField()
		var value miValue
		if flags&instanceNullFlag == 0 {
			var actualKind, actualFlags, index uint32
			err = callContext(ctx, "MI typed value", table.getElement, uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(field)), uintptr(unsafe.Pointer(&value)), uintptr(unsafe.Pointer(&actualKind)), uintptr(unsafe.Pointer(&actualFlags)), uintptr(unsafe.Pointer(&index)))
			if err != nil {
				return typedInstance{}, err
			}
			if actualKind != kind || actualFlags != flags || index != i {
				return typedInstance{}, errors.New("typed native metadata changed during copy")
			}
		}
		copied, e := decodeNativeTypedValue(kind, flags, &value, budget)
		if e != nil {
			return typedInstance{}, e
		}
		out.values[name] = copied
	}
	runtime.KeepAlive(object)
	return out, ctx.Err()
}
