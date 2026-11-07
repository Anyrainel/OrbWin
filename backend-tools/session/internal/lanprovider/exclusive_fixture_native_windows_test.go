//go:build windows && amd64

package lanprovider

// All provider mutations in this increment are test-only. No production build
// can link these slots. No generic rule, option, query or command API is exported.
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
type fixtureMutationSessionTable struct {
	excluded                       [4]uintptr
	createInstance, deleteInstance uintptr
}
type fixtureNativePolicy struct {
	intent               fixtureIntent
	program, alias, port string
}
type fixtureMutationCursor struct {
	operation   nativeOperation
	argument    *instanceObject
	current     *instanceObject
	declaration ruleInstanceDeclaration
	result      fixtureCompletion
	deleting    bool
}

func (c *fixtureMutationCursor) Next() cursorStep {
	var object, details *instanceObject
	var message *uint16
	var more uint8 = 2
	var status uint32
	table := (*instanceReadOperationTable)(unsafe.Pointer(c.operation.handle.table))
	e := call("MI fixture operation result", table.getInstance, pointer(&c.operation.handle), uintptr(unsafe.Pointer(&object)), uintptr(unsafe.Pointer(&more)), uintptr(unsafe.Pointer(&status)), uintptr(unsafe.Pointer(&message)), uintptr(unsafe.Pointer(&details)))
	runtime.KeepAlive(c)
	c.current = object // Borrowed only; never Delete message/details/results.
	if e != nil || more > 1 {
		return cursorStep{err: errors.New("fixture operation transport/result unknown")}
	}
	if status != 0 {
		c.result.status = status
	}
	c.result.terminal = more == 0
	return cursorStep{hasValue: object != nil, settled: more == 0, err: nativeResult("MI fixture operation", uintptr(status))}
}
func (c *fixtureMutationCursor) CopyCurrent(ctx context.Context) error {
	if c.deleting {
		return errors.New("fixture Delete unexpectedly returned an instance")
	}
	keys, e := copyRuleKeys(ctx, c.current, false, c.declaration)
	if e != nil || !fixtureCompleteKeys(keys) {
		return errors.New("fixture Create returned incomplete/unqualified keys")
	}
	c.result.keys = keys
	return nil
}
func (c *fixtureMutationCursor) Cancel() error { return c.operation.Cancel() }
func (c *fixtureMutationCursor) Close() error {
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

func fixtureSetOption(ctx context.Context, n *nativeConnection, name string, kind uint32, texts []string, boolean bool) error {
	// Closed allowlist: every option is required; no value silently omitted.
	expected := map[string]uint32{"PolicyStore": 13, "Program": 13, "Protocol": 13, "LocalAddress": 29, "RemoteAddress": 29, "LocalPort": 29, "InterfaceAlias": 29, "OverrideBlockRules": 0, "OrbWinUnsupportedRequiredOption": 0}
	if expected[name] != kind {
		return errors.New("fixture option type not permitted")
	}
	if _, ok := expected[name]; !ok {
		return errors.New("fixture option not permitted")
	}
	key, _ := windows.UTF16PtrFromString(name)
	var v miValue
	var keep []*uint16
	switch kind {
	case 0:
		if len(texts) != 0 {
			return errors.New("fixture boolean option shape")
		}
		if boolean {
			v[0] = 1
		}
	case 13, 29:
		if len(texts) != 1 || texts[0] == "" || len(texts[0]) > 1024 || strings.ContainsRune(texts[0], 0) {
			return errors.New("fixture string option shape")
		}
		p, e := windows.UTF16PtrFromString(texts[0])
		if e != nil {
			return errors.New("fixture option text invalid")
		}
		keep = []*uint16{p}
		if kind == 13 {
			*(**uint16)(unsafe.Pointer(&v)) = p
		} else {
			*(*nativeStringArray)(unsafe.Pointer(&v)) = nativeStringArray{data: &keep[0], count: 1}
		}
	default:
		return errors.New("fixture option type unsupported")
	}
	table := (*persistentStoreOptionsTable)(unsafe.Pointer(n.options.table))
	e := callContext(ctx, "MI mandatory fixture option", table.setCustomOption, pointer(&n.options), uintptr(unsafe.Pointer(key)), uintptr(kind), uintptr(unsafe.Pointer(&v)), 1, 0)
	runtime.KeepAlive(key)
	runtime.KeepAlive(v)
	runtime.KeepAlive(keep)
	runtime.KeepAlive(n)
	return e
}
func fixtureCreateArgument(ctx context.Context, n *nativeConnection, p fixtureNativePolicy) (out *instanceObject, err error) {
	if !fixtureNamePattern.MatchString(p.intent.name) || p.program == "" || p.alias == "" || strings.ContainsAny(p.alias, "*?[]`\x00") || (p.port != "65432" && p.port != "65433") {
		return nil, errors.New("fixture policy not fixed and inert")
	}
	table := (*localInstanceApplicationTable)(unsafe.Pointer(n.app.table))
	class, _ := windows.UTF16PtrFromString(ClassName)
	err = callContext(ctx, "MI fixture local argument", table.newInstance, pointer(&n.app), uintptr(unsafe.Pointer(class)), 0, uintptr(unsafe.Pointer(&out)))
	runtime.KeepAlive(class)
	runtime.KeepAlive(n)
	if err != nil || out == nil || out.table == nil {
		return nil, errors.Join(errors.New("fixture local instance unavailable"), deleteOwnedError(out))
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
		{"InstanceID", 13, p.intent.name, 0}, {"ElementName", 13, p.intent.name, 0}, {"Description", 13, "OrbWin disabled BLOCK proof actor " + p.intent.actor, 0}, {"RuleGroup", 13, "OrbWin inert exclusive-create proof", 0},
		{"Enabled", 3, "", fixtureDisabled}, {"Profiles", 3, "", fixturePrivate}, {"Direction", 3, "", fixtureInbound}, {"Action", 3, "", fixtureBlock}, {"EdgeTraversalPolicy", 3, "", fixtureEdgeBlock}, {"LooseSourceMapping", 0, "", 0}, {"LocalOnlyMapping", 0, "", 0},
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
		err = callContext(ctx, "MI fixture local property", ft.addElement, uintptr(unsafe.Pointer(out)), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&value)), uintptr(f.kind), 0)
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
func fixturePrepareOptions(ctx context.Context, n *nativeConnection, p fixtureNativePolicy, unsupported bool) error {
	for _, option := range []struct {
		name string
		kind uint32
		text string
	}{
		{"PolicyStore", 13, persistentStore}, {"Program", 13, p.program}, {"Protocol", 13, "TCP"}, {"LocalAddress", 29, "127.0.0.1"}, {"RemoteAddress", 29, "127.0.0.1"}, {"LocalPort", 29, p.port}, {"InterfaceAlias", 29, p.alias},
	} {
		if e := fixtureSetOption(ctx, n, option.name, option.kind, []string{option.text}, false); e != nil {
			return e
		}
	}
	if e := fixtureSetOption(ctx, n, "OverrideBlockRules", 0, nil, false); e != nil {
		return e
	}
	if unsupported {
		return fixtureSetOption(ctx, n, "OrbWinUnsupportedRequiredOption", 0, nil, false)
	}
	return nil
}
func fixtureDispatchCreate(ctx context.Context, p fixtureNativePolicy, d ruleInstanceDeclaration, unsupported bool, ready func() error) (result fixtureCompletion) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
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
	if fixturePrepareOptions(ctx, n, p, unsupported) != nil || clampReadTimeout(ctx, n) != nil {
		return result
	}
	argument, e := fixtureCreateArgument(ctx, n, p)
	if e != nil {
		return result
	}
	table := (*fixtureMutationSessionTable)(unsafe.Pointer(n.session.table))
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
	ns, _ := windows.UTF16PtrFromString(Namespace)
	cursor := &fixtureMutationCursor{argument: argument, declaration: d}
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
func fixtureDispatchDelete(ctx context.Context, owner fixtureOwnership, d ruleInstanceDeclaration) (err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if !fixtureCompleteKeys(owner.keys) || owner.intent.name != owner.keys.instanceID {
		return errors.New("fixture Delete has no live returned keys")
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
	argument, e := localKeysArgument(ctx, n, owner.keys)
	if e != nil {
		return e
	}
	table := (*fixtureMutationSessionTable)(unsafe.Pointer(n.session.table))
	if table.deleteInstance == 0 {
		return errors.Join(errors.New("fixture Delete unavailable"), deleteOwnedError(argument))
	}
	ns, _ := windows.UTF16PtrFromString(Namespace)
	cursor := &fixtureMutationCursor{argument: argument, deleting: true, declaration: d}
	cursor.result.dispatched = true
	syscall.SyscallN(table.deleteInstance, pointer(&n.session), 0, pointer(&n.options), uintptr(unsafe.Pointer(ns)), uintptr(unsafe.Pointer(argument)), 0, pointer(&cursor.operation.handle))
	runtime.KeepAlive(n)
	runtime.KeepAlive(ns)
	runtime.KeepAlive(argument)
	runtime.KeepAlive(cursor)
	if cursor.operation.handle.table == nil {
		return errors.Join(errors.New("fixture Delete outcome unknown"), deleteOwnedError(argument))
	}
	found, e := consumeOne(ctx, cursor)
	if e != nil || found || !cursor.result.terminal || !cursor.result.closed || cursor.result.status != 0 {
		return errors.New("fixture Delete not positively acknowledged; pending intent retained")
	}
	return nil
}
