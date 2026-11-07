//go:build windows && amd64

package lanprovider

// Adapted from the gated inert MI fixture, preserving its operation ownership
// and borrowed-value rules. Product registration remains disabled.
import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

// SDK10.0.22621.0 MI_SessionFT; exact signatures/offsets must be compile-proven.
type ownedMutationSessionTable struct {
	excluded                       [4]uintptr
	createInstance, deleteInstance uintptr
}
type ownedMutationCursor struct {
	operation   nativeOperation
	argument    *instanceObject
	current     *instanceObject
	declaration ruleInstanceDeclaration
	result      ownedCompletion
	deleting    bool
}

func (c *ownedMutationCursor) Next() cursorStep {
	var object, details *instanceObject
	var message *uint16
	var more uint8 = 2
	var status uint32
	table := (*instanceReadOperationTable)(unsafe.Pointer(c.operation.handle.table))
	e := call("MI owned operation result", table.getInstance, pointer(&c.operation.handle), uintptr(unsafe.Pointer(&object)), uintptr(unsafe.Pointer(&more)), uintptr(unsafe.Pointer(&status)), uintptr(unsafe.Pointer(&message)), uintptr(unsafe.Pointer(&details)))
	runtime.KeepAlive(c)
	c.current = object // Borrowed only; never Delete message/details/results.
	if e != nil || more > 1 {
		return cursorStep{err: errors.New("owned operation transport/result unknown")}
	}
	if status != 0 {
		c.result.status = status
	}
	c.result.terminal = more == 0
	return cursorStep{hasValue: object != nil, settled: more == 0, err: nativeResult("MI owned operation", uintptr(status))}
}
func (c *ownedMutationCursor) CopyCurrent(ctx context.Context) error {
	if c.deleting {
		return errors.New("owned Delete unexpectedly returned an instance")
	}
	keys, e := copyRuleKeys(ctx, c.current, false, c.declaration)
	if e != nil || !completeOwnedKeys(keys) {
		return errors.New("owned Create returned incomplete/unqualified keys")
	}
	c.result.keys = keys
	return nil
}
func (c *ownedMutationCursor) Cancel() error { return c.operation.Cancel() }
func (c *ownedMutationCursor) Close() error {
	c.current = nil
	e := c.operation.Close()
	c.result.closed = e == nil
	e = errors.Join(e, deleteOwnedError(c.argument))
	c.argument = nil
	if e != nil {
		c.result.closed = false
	}
	return e
}

func ownedSetOption(ctx context.Context, n *nativeConnection, name string, kind uint32, texts []string, boolean bool) error {
	// Closed allowlist: every option is required; no value silently omitted.
	expected := map[string]uint32{"PolicyStore": 13, "Program": 13, "Protocol": 13, "LocalAddress": 29, "RemoteAddress": 29, "LocalPort": 29, "InterfaceAlias": 29, "OverrideBlockRules": 0}
	if expected[name] != kind {
		return errors.New("owned option type not permitted")
	}
	if _, ok := expected[name]; !ok {
		return errors.New("owned option not permitted")
	}
	key, _ := windows.UTF16PtrFromString(name)
	var v miValue
	var keep []*uint16
	switch kind {
	case 0:
		if len(texts) != 0 {
			return errors.New("owned boolean option shape")
		}
		if boolean {
			v[0] = 1
		}
	case 13, 29:
		if len(texts) < 1 || len(texts) > 8 || (kind == 13 && len(texts) != 1) {
			return errors.New("owned string option shape")
		}
		for _, text := range texts {
			if text == "" || len(text) > 1024 || strings.ContainsRune(text, 0) {
				return errors.New("owned option text invalid")
			}
			p, e := windows.UTF16PtrFromString(text)
			if e != nil {
				return errors.New("owned option text invalid")
			}
			keep = append(keep, p)
		}
		if kind == 13 {
			*(**uint16)(unsafe.Pointer(&v)) = keep[0]
		} else {
			*(*nativeStringArray)(unsafe.Pointer(&v)) = nativeStringArray{data: &keep[0], count: uint32(len(keep))}
		}
	default:
		return errors.New("owned option type unsupported")
	}
	table := (*persistentStoreOptionsTable)(unsafe.Pointer(n.options.table))
	e := callContext(ctx, "MI mandatory owned option", table.setCustomOption, pointer(&n.options), uintptr(unsafe.Pointer(key)), uintptr(kind), uintptr(unsafe.Pointer(&v)), 1, 0)
	runtime.KeepAlive(key)
	runtime.KeepAlive(v)
	runtime.KeepAlive(keep)
	runtime.KeepAlive(n)
	return e
}
func ownedCreateArgument(ctx context.Context, n *nativeConnection, p ownedPolicy) (out *instanceObject, err error) {
	table := (*localInstanceApplicationTable)(unsafe.Pointer(n.app.table))
	class, _ := windows.UTF16PtrFromString(ClassName)
	err = callContext(ctx, "MI owned local argument", table.newInstance, pointer(&n.app), uintptr(unsafe.Pointer(class)), 0, uintptr(unsafe.Pointer(&out)))
	runtime.KeepAlive(class)
	runtime.KeepAlive(n)
	if err != nil || out == nil || out.table == nil {
		return nil, errors.Join(errors.New("owned local instance unavailable"), deleteOwnedError(out))
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, deleteOwnedError(out))
			out = nil
		}
	}()
	fields := []struct {
		name   string
		kind   uint32
		text   string
		scalar uint16
	}{
		{"InstanceID", 13, p.spec.Properties.InstanceID, 0}, {"ElementName", 13, p.spec.Properties.ElementName, 0}, {"Description", 13, p.spec.Properties.Description, 0}, {"RuleGroup", 13, p.spec.Properties.RuleGroup, 0},
		{"Enabled", 3, "", 1}, {"Profiles", 3, "", p.spec.Properties.Profiles}, {"Direction", 3, "", 1}, {"Action", 3, "", 2}, {"EdgeTraversalPolicy", 3, "", 0}, {"LooseSourceMapping", 0, "", 0}, {"LocalOnlyMapping", 0, "", 0},
	}
	for _, f := range fields {
		name, _ := windows.UTF16PtrFromString(f.name)
		var value miValue
		var text *uint16
		if f.kind == 13 {
			text, _ = windows.UTF16PtrFromString(f.text)
			*(**uint16)(unsafe.Pointer(&value)) = text
		} else {
			value[0] = uint64(f.scalar)
		}
		ft := (*instanceReadTable)(unsafe.Pointer(out.table))
		err = callContext(ctx, "MI owned local property", ft.addElement, uintptr(unsafe.Pointer(out)), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&value)), uintptr(f.kind), 0)
		runtime.KeepAlive(name)
		runtime.KeepAlive(value)
		runtime.KeepAlive(text)
		runtime.KeepAlive(out)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
func ownedPrepareOptions(ctx context.Context, n *nativeConnection, p ownedPolicy) error {
	for _, o := range []struct {
		name  string
		kind  uint32
		texts []string
	}{
		{"PolicyStore", 13, []string{persistentStore}}, {"Program", 13, []string{p.spec.Options.Program}}, {"Protocol", 13, []string{p.spec.Options.Protocol}},
		{"LocalAddress", 29, p.spec.Options.LocalAddress}, {"RemoteAddress", 29, p.spec.Options.RemoteAddress}, {"LocalPort", 29, p.spec.Options.LocalPort}, {"InterfaceAlias", 29, p.spec.Options.InterfaceAlias},
	} {
		if e := ownedSetOption(ctx, n, o.name, o.kind, o.texts, false); e != nil {
			return e
		}
	}
	return ownedSetOption(ctx, n, "OverrideBlockRules", 0, nil, false)
}
func ownedDispatchCreate(ctx context.Context, p ownedPolicy, d ruleInstanceDeclaration, ready func() error) (result ownedCompletion) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if requireOwnedElevation(ctx) != nil {
		return result
	}
	connection, e := openNative(ctx)
	if e != nil {
		return result
	}
	n := connection.(*nativeConnection)
	defer func() {
		if n.Close() != nil {
			result.closed = false
		}
	}()
	if ownedPrepareOptions(ctx, n, p) != nil || clampReadTimeout(ctx, n) != nil {
		return result
	}
	argument, e := ownedCreateArgument(ctx, n, p)
	if e != nil {
		return result
	}
	table := (*ownedMutationSessionTable)(unsafe.Pointer(n.session.table))
	if table.createInstance == 0 {
		_ = deleteOwnedError(argument)
		return result
	}
	if ready != nil {
		if e = ready(); e != nil {
			_ = deleteOwnedError(argument)
			return result
		}
	}
	if ctx.Err() != nil {
		_ = deleteOwnedError(argument)
		return result
	}
	// Preparation/barrier time must not extend a dispatched operation beyond
	// the remaining cumulative budget. Re-clamp immediately before dispatch.
	if clampReadTimeout(ctx, n) != nil {
		_ = deleteOwnedError(argument)
		return result
	}
	if requireOwnedElevation(ctx) != nil {
		_ = deleteOwnedError(argument)
		return result
	}
	ns, _ := windows.UTF16PtrFromString(Namespace)
	cursor := &ownedMutationCursor{argument: argument, declaration: d}
	cursor.result.dispatched = true
	syscall.SyscallN(table.createInstance, pointer(&n.session), 0, pointer(&n.options), uintptr(unsafe.Pointer(ns)), uintptr(unsafe.Pointer(argument)), 0, pointer(&cursor.operation.handle))
	runtime.KeepAlive(n)
	runtime.KeepAlive(ns)
	runtime.KeepAlive(argument)
	runtime.KeepAlive(cursor)
	if cursor.operation.handle.table == nil {
		_ = deleteOwnedError(argument)
		return cursor.result
	}
	found, e := consumeOne(ctx, cursor)
	cursor.result.returnedFullKeys = e == nil && found && cursor.result.terminal && cursor.result.status == 0
	if !cursor.result.returnedFullKeys {
		cursor.result.keys = observedRuleKeys{}
	}
	return cursor.result
}
func ownedDispatchDelete(ctx context.Context, keys observedRuleKeys, d ruleInstanceDeclaration, ready func() error) (err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err = requireOwnedElevation(ctx); err != nil {
		return err
	}
	if !completeOwnedKeys(keys) {
		return errors.New("owned Delete has no live returned keys")
	}
	connection, e := openNative(ctx)
	if e != nil {
		return e
	}
	n := connection.(*nativeConnection)
	// This only accepts the live opaque capability at its caller. No generic keys
	// deletion or name/query fallback is provided.
	defer func() { err = errors.Join(err, n.Close()) }()
	if e = setPersistentStore(ctx, n); e != nil {
		return e
	}
	if e = clampReadTimeout(ctx, n); e != nil {
		return e
	}
	argument, e := localKeysArgument(ctx, n, keys)
	if e != nil {
		return e
	}
	table := (*ownedMutationSessionTable)(unsafe.Pointer(n.session.table))
	if table.deleteInstance == 0 {
		return errors.Join(errors.New("owned Delete unavailable"), deleteOwnedError(argument))
	}
	if e = ready(); e != nil {
		return errors.Join(e, deleteOwnedError(argument))
	}
	if e = clampReadTimeout(ctx, n); e != nil {
		return errors.Join(e, deleteOwnedError(argument))
	}
	if e = requireOwnedElevation(ctx); e != nil {
		return errors.Join(e, deleteOwnedError(argument))
	}
	ns, _ := windows.UTF16PtrFromString(Namespace)
	cursor := &ownedMutationCursor{argument: argument, deleting: true, declaration: d}
	cursor.result.dispatched = true
	syscall.SyscallN(table.deleteInstance, pointer(&n.session), 0, pointer(&n.options), uintptr(unsafe.Pointer(ns)), uintptr(unsafe.Pointer(argument)), 0, pointer(&cursor.operation.handle))
	runtime.KeepAlive(n)
	runtime.KeepAlive(ns)
	runtime.KeepAlive(argument)
	runtime.KeepAlive(cursor)
	if cursor.operation.handle.table == nil {
		return errors.Join(errors.New("owned Delete outcome unknown"), deleteOwnedError(argument))
	}
	found, e := consumeOne(ctx, cursor)
	if e != nil || found || !cursor.result.terminal || !cursor.result.closed || cursor.result.status != 0 {
		return errors.New("owned Delete not positively acknowledged; pending intent retained")
	}
	return nil
}
