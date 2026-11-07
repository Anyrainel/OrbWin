//go:build windows && amd64

package lanprovider

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"runtime"
	"syscall"
	"unsafe"
)

// Exact Microsoft SDK MI_SessionFT.AssociatorInstances, read-only association
// traversal. No provider Reference/Create/Modify/Delete/Invoke slot is named.
type associationReadSessionTable struct {
	excluded            [9]uintptr
	associatorInstances uintptr
}

func readTraversalDeclarations(ctx context.Context) (out traversalDeclarations, err error) {
	if err = ctx.Err(); err != nil {
		return out, err
	}
	if _, ok := ctx.Deadline(); !ok {
		return out, errors.New("traversal requires cumulative deadline")
	}
	filters, err := ObserveFilterSchemas(ctx)
	if err != nil || !filters.Complete || len(filters.Classes) != 14 {
		return out, errors.Join(err, errors.New("fresh fixed filter schemas unavailable"))
	}
	for i, value := range filters.Classes {
		if err = validatePinnedFilterSchema(value, i); err != nil {
			return out, err
		}
		if i < 7 {
			out.filters[i], err = typedFilterDeclaration(value, i)
			if err != nil {
				return out, err
			}
		}
	}
	rule, err := ObserveSchema(ctx)
	if err != nil || !rule.Complete || rule.Class == nil {
		return out, errors.Join(err, errors.New("fresh fixed rule declaration unavailable"))
	}
	out.rule, err = bindRuleInstanceDeclaration(*rule.Class)
	if err != nil {
		return out, err
	}
	out.typedRule = typedRuleDeclaration(out.rule)
	return out, ctx.Err()
}

type nativeTraversalSession struct {
	keys         *nativeKeySession
	declarations traversalDeclarations
	budget       typedCopyBudget
	rule, filter typedInstance
}

func (s *nativeTraversalSession) Close() error {
	s.rule = typedInstance{}
	s.filter = typedInstance{}
	return s.keys.Close()
}
func (s *nativeTraversalSession) CurrentRuleKeys() observedRuleKeys { return s.keys.current }
func (s *nativeTraversalSession) CurrentRule() typedInstance        { return s.rule }
func (s *nativeTraversalSession) CurrentFilter() typedInstance      { return s.filter }

type traversalCursor struct {
	base *nativeKeyCursor
	copy func(context.Context, *instanceObject) error
}

func requireNativeTraversalCursor(ctx context.Context, cursor readCursor) (*nativeKeyCursor, error) {
	if base, ok := cursor.(*nativeKeyCursor); ok {
		return base, nil
	}
	// Frozen private factories currently guarantee this concrete type. If that
	// contract ever changes, cancel/drain the acquired cursor rather than leak
	// it or attempt session Close with a still-live operation.
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, cleanup := consumeOne(cancelled, cursor)
	return nil, errors.Join(errors.New("native traversal cursor contract differs"), cleanup)
}

func (c *traversalCursor) Next() cursorStep                      { return c.base.Next() }
func (c *traversalCursor) Cancel() error                         { return c.base.Cancel() }
func (c *traversalCursor) Close() error                          { return c.base.Close() }
func (c *traversalCursor) CopyCurrent(ctx context.Context) error { return c.copy(ctx, c.base.current) }

func (s *nativeTraversalSession) identityCursor(cursor readCursor) (readCursor, error) {
	base, err := requireNativeTraversalCursor(s.keys.connection.ctx, cursor)
	if err != nil {
		return nil, err
	}
	return &traversalCursor{base: base, copy: func(ctx context.Context, _ *instanceObject) error {
		keys, err := copyTypedRuleIdentity(ctx, base.current, s.declarations.typedRule, &s.budget, base.query)
		if err != nil {
			return err
		}
		s.keys.current = keys
		return nil
	}}, nil
}
func (s *nativeTraversalSession) Select(ctx context.Context) (readCursor, error) {
	cursor, err := s.keys.Query(ctx)
	if err != nil {
		return nil, err
	}
	return s.identityCursor(cursor)
}
func (s *nativeTraversalSession) ReadRule(ctx context.Context, keys observedRuleKeys) (readCursor, error) {
	cursor, err := s.keys.Read(ctx, keys)
	if err != nil {
		return nil, err
	}
	base, err := requireNativeTraversalCursor(ctx, cursor)
	if err != nil {
		return nil, err
	}
	return &traversalCursor{base: base, copy: func(ctx context.Context, object *instanceObject) error {
		value, e := copyTypedInstance(ctx, object, s.declarations.typedRule, &s.budget)
		if e != nil {
			return e
		}
		actual, e := value.ruleKeys()
		if e != nil {
			return e
		}
		s.rule = value
		s.keys.current = actual
		return nil
	}}, nil
}
func (s *nativeTraversalSession) Forward(ctx context.Context, keys observedRuleKeys, index int) (readCursor, error) {
	if index < 0 || index >= 7 {
		return nil, errors.New("fixed forward family unsupported")
	}
	n := s.keys.connection
	if err := clampReadTimeout(ctx, n); err != nil {
		return nil, err
	}
	argument, err := localKeysArgument(ctx, n, keys)
	if err != nil {
		return nil, err
	}
	base, err := s.associate(ctx, index, argument, false)
	if err != nil {
		return nil, err
	}
	return &traversalCursor{base: base, copy: func(ctx context.Context, object *instanceObject) error {
		value, e := copyTypedInstance(ctx, object, s.declarations.filters[index], &s.budget)
		if e != nil {
			return e
		}
		if _, e = value.filterKeys(index); e != nil {
			return e
		}
		s.filter = value
		return nil
	}}, nil
}
func (s *nativeTraversalSession) Reverse(ctx context.Context, keys observedFilterKeys, index int) (readCursor, error) {
	if index < 0 || index >= 7 {
		return nil, errors.New("fixed reverse family unsupported")
	}
	n := s.keys.connection
	if err := clampReadTimeout(ctx, n); err != nil {
		return nil, err
	}
	argument, err := localFilterKeysArgument(ctx, n, index, keys)
	if err != nil {
		return nil, err
	}
	base, err := s.associate(ctx, index, argument, true)
	if err != nil {
		return nil, err
	}
	return s.identityCursor(base)
}
func (s *nativeTraversalSession) associate(ctx context.Context, index int, argument *instanceObject, reverse bool) (out *nativeKeyCursor, err error) {
	n := s.keys.connection
	// Argument is locally owned. Once dispatched it belongs to the cursor until
	// positive terminal consumption + operation Close, even after cancellation.
	if err = clampReadTimeout(ctx, n); err != nil {
		return nil, errors.Join(err, deleteOwnedError(argument))
	}
	table := (*associationReadSessionTable)(unsafe.Pointer(n.session.table))
	if table.associatorInstances == 0 {
		return nil, errors.Join(errors.New("native association getter unavailable"), deleteOwnedError(argument))
	}
	class, role, resultRole := filterFamilies[index].class, "GroupComponent", "PartComponent"
	if reverse {
		class, role, resultRole = ClassName, "PartComponent", "GroupComponent"
	}
	ns, _ := windows.UTF16PtrFromString(Namespace)
	association, _ := windows.UTF16PtrFromString(filterFamilies[index].association)
	resultClass, _ := windows.UTF16PtrFromString(class)
	sourceRole, _ := windows.UTF16PtrFromString(role)
	targetRole, _ := windows.UTF16PtrFromString(resultRole)
	out = &nativeKeyCursor{owner: s.keys, localArgument: argument}
	// flags0, keysOnly FALSE, callbacks NULL; fixed classes/roles only. Full
	// result allows identity and exact metadata checks, never outputting values.
	syscall.SyscallN(table.associatorInstances, pointer(&n.session), 0, pointer(&n.options), uintptr(unsafe.Pointer(ns)), uintptr(unsafe.Pointer(argument)), uintptr(unsafe.Pointer(association)), uintptr(unsafe.Pointer(resultClass)), uintptr(unsafe.Pointer(sourceRole)), uintptr(unsafe.Pointer(targetRole)), 0, 0, pointer(&out.operation.handle))
	runtime.KeepAlive(n)
	runtime.KeepAlive(argument)
	runtime.KeepAlive(ns)
	runtime.KeepAlive(association)
	runtime.KeepAlive(resultClass)
	runtime.KeepAlive(sourceRole)
	runtime.KeepAlive(targetRole)
	runtime.KeepAlive(out)
	if out.operation.handle.table == nil {
		return nil, errors.Join(errors.New("native association returned no operation"), deleteOwnedError(argument))
	}
	return out, nil
}

func localFilterKeysArgument(ctx context.Context, n *nativeConnection, index int, keys observedFilterKeys) (out *instanceObject, err error) {
	if index < 0 || index >= 7 {
		return nil, errors.New("fixed filter keys family unsupported")
	}
	table := (*localInstanceApplicationTable)(unsafe.Pointer(n.app.table))
	class, _ := windows.UTF16PtrFromString(filterFamilies[index].class)
	err = callContext(ctx, "MI local filter keys", table.newInstance, pointer(&n.app), uintptr(unsafe.Pointer(class)), 0, uintptr(unsafe.Pointer(&out)))
	runtime.KeepAlive(class)
	runtime.KeepAlive(n)
	if err != nil || out == nil || out.table == nil {
		return nil, errors.Join(err, errors.New("local filter keys unavailable"), deleteOwnedError(out))
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, deleteOwnedError(out))
			out = nil
		}
	}()
	names := [4]string{"Name", "CreationClassName", "SystemCreationClassName", "SystemName"}
	values := [4]string{keys.name, keys.creationClassName, keys.systemCreationClassName, keys.systemName}
	for i, name := range names {
		field, _ := windows.UTF16PtrFromString(name)
		text, e := windows.UTF16PtrFromString(values[i])
		if e != nil {
			return out, errors.New("local filter key contains embedded null")
		}
		var value miValue
		*(**uint16)(unsafe.Pointer(&value)) = text
		fields := (*instanceReadTable)(unsafe.Pointer(out.table))
		err = callContext(ctx, "MI local filter key element", fields.addElement, uintptr(unsafe.Pointer(out)), uintptr(unsafe.Pointer(field)), uintptr(unsafe.Pointer(&value)), 13, 4096)
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

// ObserveFilterTraversal requires an external one-shot hard deadline. This
// compares observations; it is not an atomic policy snapshot or ownership proof.
func ObserveFilterTraversal(ctx context.Context) (out FilterTraversalSnapshot, err error) {
	if err = ctx.Err(); err != nil {
		return out, err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	declarations, err := readTraversalDeclarations(ctx)
	if err != nil {
		return out, err
	}
	out, err = observeFilterTraversal(ctx, func(ctx context.Context) (filterTraversalSession, error) {
		opened, e := openKeySession(ctx, declarations.rule)
		if e != nil {
			return nil, e
		}
		return &nativeTraversalSession{keys: opened.(*nativeKeySession), declarations: declarations}, nil
	})
	out.SchemaRechecked = true
	return out, err
}
