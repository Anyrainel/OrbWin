//go:build windows && amd64

package lanprovider

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Typed read-only overlays of SDK10.0.22621.0 mi.h. No provider mutation
// entrypoints are named or called. NewInstance/AddElement affect only a local
// keys argument, not the provider. Compiled signature/offset evidence accompanies
// this increment; these tables must never be inferred from COM slot numbers.
type instanceReadSessionTable struct {
	excluded0      [2]uintptr
	getInstance    uintptr
	excluded1      [5]uintptr
	queryInstances uintptr
}
type instanceReadOperationTable struct {
	excluded    [3]uintptr
	getInstance uintptr
}
type instanceReadTable struct {
	excluded0                                                                     [4]uintptr
	getClassName, excludedSetNamespace, getNamespace, getElementCount, addElement uintptr
	excluded1                                                                     [2]uintptr
	getElement, getElementAt                                                      uintptr
}
type localInstanceApplicationTable struct {
	excluded    [3]uintptr
	newInstance uintptr
}
type persistentStoreOptionsTable struct {
	excluded        [3]uintptr
	setCustomOption uintptr
}

func clampReadTimeout(ctx context.Context, n *nativeConnection) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return errors.New("native read requires cumulative deadline")
	}
	remaining := time.Until(deadline)
	if remaining > 5*time.Second {
		remaining = 5 * time.Second
	}
	if remaining < time.Microsecond {
		return context.DeadlineExceeded
	}
	value := interval{seconds: uint32(remaining / time.Second), microseconds: uint32((remaining % time.Second) / time.Microsecond)}
	key, _ := windows.UTF16PtrFromString("__MI_OPERATIONOPTIONS_TIMEOUT")
	err := callContext(ctx, "MI bounded timeout", n.options.table.setInterval, pointer(&n.options), uintptr(unsafe.Pointer(key)), uintptr(unsafe.Pointer(&value)), 0)
	runtime.KeepAlive(n)
	runtime.KeepAlive(key)
	runtime.KeepAlive(value)
	return err
}

// This is a typed mandatory MI custom option, not WBEM context/string options.
// Provider acceptance/honoring is an explicit native qualification gate. Failure
// never falls back to ActiveStore or a default policy store.
func setPersistentStore(ctx context.Context, n *nativeConnection) error {
	key, _ := windows.UTF16PtrFromString("PolicyStore")
	store, _ := windows.UTF16PtrFromString(persistentStore)
	var value miValue
	*(**uint16)(unsafe.Pointer(&value)) = store
	table := (*persistentStoreOptionsTable)(unsafe.Pointer(n.options.table))
	err := callContext(ctx, "MI required PersistentStore", table.setCustomOption, pointer(&n.options), uintptr(unsafe.Pointer(key)), 13, uintptr(unsafe.Pointer(&value)), 1, 0)
	runtime.KeepAlive(key)
	runtime.KeepAlive(store)
	runtime.KeepAlive(value)
	runtime.KeepAlive(n)
	return err
}

type filterSchemaCursor struct {
	operation *nativeOperation
	current   classReader
	index     int
	copied    FilterSchema
}

func (c *filterSchemaCursor) Next() cursorStep {
	r := c.operation.Next()
	c.current = r.reader
	return cursorStep{hasValue: r.reader != nil, settled: r.settled, err: r.err}
}
func (c *filterSchemaCursor) Cancel() error { return c.operation.Cancel() }
func (c *filterSchemaCursor) Close() error  { c.current = nil; return c.operation.Close() }
func (c *filterSchemaCursor) CopyCurrent(ctx context.Context) error {
	native, ok := c.current.(nativeClass)
	if !ok {
		return errors.New("native class reader unavailable")
	}
	base, err := native.Read(ctx)
	if err != nil {
		return err
	}
	out := FilterSchema{Class: base}
	for i, p := range base.Properties {
		var name, reference *uint16
		var kind, flags uint32
		var exists uint8
		err = callContext(ctx, "MI reference metadata", native.object.table.getAt, uintptr(unsafe.Pointer(native.object)), uintptr(i), uintptr(unsafe.Pointer(&name)), 0, uintptr(unsafe.Pointer(&exists)), uintptr(unsafe.Pointer(&kind)), uintptr(unsafe.Pointer(&reference)), 0, uintptr(unsafe.Pointer(&flags)))
		runtime.KeepAlive(native)
		if err != nil || exists > 1 {
			return errors.Join(err, errors.New("native reference metadata unavailable"))
		}
		observed, e := stringCopy(name, 128)
		if e != nil {
			return e
		}
		if observed != p.Name || kind != p.Type || flags != p.Flags || exists == 1 != p.HasDefault {
			return errors.New("native schema changed during reference read")
		}
		if kind == 14 {
			target, e := stringCopy(reference, 128)
			if e != nil {
				return e
			}
			out.References = append(out.References, ReferenceMetadata{Name: p.Name, Class: target})
		} else if reference != nil {
			return errors.New("native non-reference has reference class")
		}
	}
	if err = validateFilterSchema(out, c.index); err != nil {
		return err
	}
	c.copied = out
	return ctx.Err()
}

// ObserveFilterSchemas is a bounded fixed 14-class metadata observation only.
// Caller must enforce an external process deadline; cancellation drains can block.
func ObserveFilterSchemas(ctx context.Context) (out FilterSchemaSnapshot, err error) {
	out = FilterSchemaSnapshot{Version: 1, ReadAt: time.Now().UTC().Format(time.RFC3339Nano), Source: "windows-mi-fixed-filter-schemas"}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	if _, ok := ctx.Deadline(); !ok {
		return out, errors.New("native read requires cumulative deadline")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	connection, err := openNative(ctx)
	if err != nil {
		return out, err
	}
	n := connection.(*nativeConnection)
	defer func() {
		err = errors.Join(err, n.Close())
		if err != nil {
			out.Complete = false
			out.Classes = nil
		}
	}()
	for i := 0; i < 14; i++ {
		if err = clampReadTimeout(ctx, n); err != nil {
			return out, err
		}
		family := filterFamilies[i%7]
		name := family.class
		if i >= 7 {
			name = family.association
		}
		ns, _ := windows.UTF16PtrFromString(Namespace)
		class, _ := windows.UTF16PtrFromString(name)
		op := &nativeOperation{}
		if n.session.table.getClass == 0 {
			return out, errors.New("native class getter unavailable")
		}
		syscall.SyscallN(n.session.table.getClass, pointer(&n.session), 0, pointer(&n.options), uintptr(unsafe.Pointer(ns)), uintptr(unsafe.Pointer(class)), 0, pointer(&op.handle))
		runtime.KeepAlive(ns)
		runtime.KeepAlive(class)
		runtime.KeepAlive(n)
		runtime.KeepAlive(op)
		if op.handle.table == nil {
			return out, errors.New("native class returned no operation")
		}
		cursor := &filterSchemaCursor{operation: op, index: i}
		found, e := consumeOne(ctx, cursor)
		if e != nil || !found {
			return out, errors.Join(e, errors.New("fixed filter schema absent or unavailable"))
		}
		out.Classes = append(out.Classes, cursor.copied)
	}
	data, e := json.Marshal(out)
	if e != nil || len(data) > MaxOutput {
		return out, errors.New("filter metadata output exceeds budget")
	}
	out.Complete = true
	return out, ctx.Err()
}

type nativeKeySession struct {
	connection  *nativeConnection
	current     observedRuleKeys
	declaration ruleInstanceDeclaration
}

func (s *nativeKeySession) CurrentKeys() observedRuleKeys { return s.current }
func (s *nativeKeySession) Close() error                  { s.current = observedRuleKeys{}; return s.connection.Close() }
func openKeySession(ctx context.Context, declaration ruleInstanceDeclaration) (keySession, error) {
	connection, e := openNative(ctx)
	if e != nil {
		return nil, e
	}
	n := connection.(*nativeConnection)
	if e = setPersistentStore(ctx, n); e != nil {
		return nil, errors.Join(e, n.Close())
	}
	return &nativeKeySession{connection: n, declaration: declaration}, nil
}

type nativeKeyCursor struct {
	operation     nativeOperation
	owner         *nativeKeySession
	current       *instanceObject
	query         bool
	localArgument *instanceObject
}

func (c *nativeKeyCursor) Next() cursorStep {
	var object *instanceObject
	var more uint8 = 2
	var status uint32
	var message *uint16
	var details *instanceObject
	table := (*instanceReadOperationTable)(unsafe.Pointer(c.operation.handle.table))
	err := call("MI instance result", table.getInstance, pointer(&c.operation.handle), uintptr(unsafe.Pointer(&object)), uintptr(unsafe.Pointer(&more)), uintptr(unsafe.Pointer(&status)), uintptr(unsafe.Pointer(&message)), uintptr(unsafe.Pointer(&details)))
	runtime.KeepAlive(c)
	c.current = object // Borrowed until NEXT/Close; never Delete.
	if err != nil || more > 1 {
		return cursorStep{err: errors.Join(err, errors.New("native instance result malformed"))}
	}
	return cursorStep{hasValue: object != nil, settled: more == 0, err: nativeResult("MI instance operation", uintptr(status))}
}
func (c *nativeKeyCursor) Cancel() error { return c.operation.Cancel() }
func (c *nativeKeyCursor) Close() error {
	c.current = nil
	err := c.operation.Close()
	err = errors.Join(err, deleteOwnedError(c.localArgument))
	c.localArgument = nil
	return err
}
func (c *nativeKeyCursor) CopyCurrent(ctx context.Context) error {
	keys, e := copyRuleKeys(ctx, c.current, c.query, c.owner.declaration)
	if e == nil {
		c.owner.current = keys
	}
	return e
}
func (s *nativeKeySession) Query(ctx context.Context) (readCursor, error) {
	n := s.connection
	if err := clampReadTimeout(ctx, n); err != nil {
		return nil, err
	}
	table := (*instanceReadSessionTable)(unsafe.Pointer(n.session.table))
	if table.queryInstances == 0 {
		return nil, errors.New("native query unavailable")
	}
	ns, _ := windows.UTF16PtrFromString(Namespace)
	dialect, _ := windows.UTF16PtrFromString("WQL")
	query, _ := windows.UTF16PtrFromString(exactRuleQuery)
	cursor := &nativeKeyCursor{owner: s, query: true}
	syscall.SyscallN(table.queryInstances, pointer(&n.session), 0, pointer(&n.options), uintptr(unsafe.Pointer(ns)), uintptr(unsafe.Pointer(dialect)), uintptr(unsafe.Pointer(query)), 0, pointer(&cursor.operation.handle))
	runtime.KeepAlive(ns)
	runtime.KeepAlive(dialect)
	runtime.KeepAlive(query)
	runtime.KeepAlive(n)
	runtime.KeepAlive(cursor)
	if cursor.operation.handle.table == nil {
		return nil, errors.New("native exact query returned no operation")
	}
	return cursor, nil
}
func (s *nativeKeySession) Read(ctx context.Context, keys observedRuleKeys) (readCursor, error) {
	n := s.connection
	if err := clampReadTimeout(ctx, n); err != nil {
		return nil, err
	}
	table := (*instanceReadSessionTable)(unsafe.Pointer(n.session.table))
	if table.getInstance == 0 {
		return nil, errors.New("native instance getter unavailable")
	}
	argument, e := localKeysArgument(ctx, n, keys)
	if e != nil {
		return nil, e
	}
	ns, _ := windows.UTF16PtrFromString(Namespace)
	cursor := &nativeKeyCursor{owner: s, localArgument: argument}
	syscall.SyscallN(table.getInstance, pointer(&n.session), 0, pointer(&n.options), uintptr(unsafe.Pointer(ns)), uintptr(unsafe.Pointer(argument)), 0, pointer(&cursor.operation.handle))
	runtime.KeepAlive(ns)
	runtime.KeepAlive(argument)
	runtime.KeepAlive(n)
	runtime.KeepAlive(cursor)
	if cursor.operation.handle.table == nil {
		return nil, errors.Join(errors.New("native GetInstance returned no operation"), deleteOwnedError(argument))
	}
	return cursor, nil
}
func localKeysArgument(ctx context.Context, n *nativeConnection, keys observedRuleKeys) (out *instanceObject, err error) {
	table := (*localInstanceApplicationTable)(unsafe.Pointer(n.app.table))
	class, _ := windows.UTF16PtrFromString(ClassName)
	err = callContext(ctx, "MI local keys instance", table.newInstance, pointer(&n.app), uintptr(unsafe.Pointer(class)), 0, uintptr(unsafe.Pointer(&out)))
	runtime.KeepAlive(class)
	runtime.KeepAlive(n)
	if err != nil || out == nil || out.table == nil {
		return nil, errors.Join(err, errors.New("native local keys allocation unavailable"), deleteOwnedError(out))
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, deleteOwnedError(out))
			out = nil
		}
	}()
	names := [4]string{"CreationClassName", "PolicyRuleName", "SystemCreationClassName", "SystemName"}
	values := [4]string{keys.creationClassName, keys.policyRuleName, keys.systemCreationClassName, keys.systemName}
	for i, name := range names {
		field, _ := windows.UTF16PtrFromString(name)
		text, e := windows.UTF16PtrFromString(values[i])
		if e != nil {
			return out, errors.New("native key contains embedded null")
		}
		var value miValue
		*(**uint16)(unsafe.Pointer(&value)) = text
		fields := (*instanceReadTable)(unsafe.Pointer(out.table))
		err = callContext(ctx, "MI local key element", fields.addElement, uintptr(unsafe.Pointer(out)), uintptr(unsafe.Pointer(field)), uintptr(unsafe.Pointer(&value)), 13, 4096)
		runtime.KeepAlive(out)
		runtime.KeepAlive(field)
		runtime.KeepAlive(text)
		runtime.KeepAlive(value)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

func copyRuleKeys(ctx context.Context, object *instanceObject, projection bool, declaration ruleInstanceDeclaration) (out observedRuleKeys, err error) {
	if object == nil || object.table == nil {
		return out, errors.New("native instance missing table")
	}
	table := (*instanceReadTable)(unsafe.Pointer(object.table))
	var class, namespace *uint16
	if err = callContext(ctx, "MI instance class", table.getClassName, uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(&class))); err != nil {
		return out, err
	}
	name, e := stringCopy(class, 128)
	if e != nil || name != ClassName {
		return out, errors.New("native instance class differs")
	}
	if err = callContext(ctx, "MI instance namespace", table.getNamespace, uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(&namespace))); err != nil {
		return out, err
	}
	ns, e := stringCopy(namespace, 128)
	if e != nil || !strings.EqualFold(strings.ReplaceAll(ns, `\`, "/"), Namespace) {
		return out, errors.New("native instance namespace differs")
	}
	var count uint32
	if err = callContext(ctx, "MI instance field count", table.getElementCount, uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(&count))); err != nil {
		return out, err
	}
	if count != 39 && (!projection || count != 5) {
		return out, errors.New("native instance field count differs from qualified schema")
	}
	seen := map[string]bool{}
	selected := map[string]string{}
	for i := uint32(0); i < count; i++ {
		var field *uint16
		var kind, flags uint32
		// NULL value output: observe all field metadata without copying private
		// application/address/service/SDDL values from unselected properties.
		err = callContext(ctx, "MI instance field metadata", table.getElementAt, uintptr(unsafe.Pointer(object)), uintptr(i), uintptr(unsafe.Pointer(&field)), 0, uintptr(unsafe.Pointer(&kind)), uintptr(unsafe.Pointer(&flags)))
		if err != nil {
			return out, err
		}
		name, e := stringCopy(field, 128)
		if e != nil {
			return out, e
		}
		selectedField, e := declaration.validate(i, name, kind, flags, seen[name])
		if e != nil {
			return out, e
		}
		seen[name] = true
		if !selectedField {
			if count == 5 {
				return out, errors.New("native query projection differs")
			}
			continue
		}
		var value miValue
		var actualKind, actualFlags, index uint32
		err = callContext(ctx, "MI selected identity", table.getElement, uintptr(unsafe.Pointer(object)), uintptr(unsafe.Pointer(field)), uintptr(unsafe.Pointer(&value)), uintptr(unsafe.Pointer(&actualKind)), uintptr(unsafe.Pointer(&actualFlags)), uintptr(unsafe.Pointer(&index)))
		if err != nil {
			return out, err
		}
		if actualKind != 13 || actualFlags != flags || index != i {
			return out, errors.New("native identity metadata changed")
		}
		text, e := stringCopy(*(**uint16)(unsafe.Pointer(&value)), 512)
		if e != nil {
			return out, e
		}
		selected[name] = text
	}
	runtime.KeepAlive(object)
	if len(selected) != 5 {
		return out, errors.New("native identity fields incomplete")
	}
	return observedRuleKeys{selected["InstanceID"], selected["CreationClassName"], selected["PolicyRuleName"], selected["SystemCreationClassName"], selected["SystemName"]}, ctx.Err()
}

// ObserveRuleKeyRoundTrip performs a fresh exact schema check followed by one
// fixed PersistentStore query and two keys-only GetInstance reads. Keys remain
// private RAM data; equality is observation, never ownership or an atomic lease.
func ObserveRuleKeyRoundTrip(ctx context.Context) (out RuleKeySnapshot, err error) {
	if err = ctx.Err(); err != nil {
		return out, err
	}
	if _, ok := ctx.Deadline(); !ok {
		return out, errors.New("native read requires cumulative deadline")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	schema, e := ObserveSchema(ctx)
	if e != nil || !schema.Complete || schema.Class == nil {
		return out, errors.Join(e, errors.New("fresh native rule schema unavailable"))
	}
	declaration, e := bindRuleInstanceDeclaration(*schema.Class)
	if e != nil {
		return out, e
	}
	out, err = keyRoundTrip(ctx, func(ctx context.Context) (keySession, error) { return openKeySession(ctx, declaration) })
	out.RuleSchemaRechecked = true
	return out, err
}
