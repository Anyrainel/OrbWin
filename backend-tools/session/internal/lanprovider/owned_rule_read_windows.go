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

func ownedReadDefinition(ctx context.Context, keys observedRuleKeys, d traversalDeclarations) (out ownedDefinition, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	opened, err := openKeySession(ctx, d.rule)
	if err != nil {
		return out, err
	}
	s := &nativeTraversalSession{keys: opened.(*nativeKeySession), declarations: d}
	defer func() {
		err = errors.Join(err, s.Close())
		if err != nil {
			out = ownedDefinition{}
		}
	}()
	readRule := func() error {
		c, e := s.ReadRule(ctx, keys)
		if e != nil {
			return e
		}
		found, e := consumeOne(ctx, c)
		if e != nil || !found || s.CurrentRuleKeys() != keys {
			return errors.New("owned owned rule unavailable or identity drift")
		}
		return nil
	}
	if err = readRule(); err != nil {
		return out, err
	}
	out.keys = keys
	out.rule = s.CurrentRule()
	for pass := 0; pass < 2; pass++ {
		for family := 0; family < 7; family++ {
			c, e := s.Forward(ctx, keys, family)
			if e != nil {
				return out, e
			}
			found, e := consumeOne(ctx, c)
			if e != nil || !found {
				return out, errors.New("owned filter missing/unavailable")
			}
			value := s.CurrentFilter()
			filterKeys, e := value.filterKeys(family)
			if e != nil {
				return out, e
			}
			if pass == 0 {
				out.filters[family] = value
			} else if !sameTypedInstance(out.filters[family], value) {
				return out, errors.New("owned typed filter drift")
			}
			c, e = s.Reverse(ctx, filterKeys, family)
			if e != nil {
				return out, e
			}
			found, e = consumeOne(ctx, c)
			if e != nil || !found || s.CurrentRuleKeys() != keys {
				return out, errors.New("owned reverse association identity drift")
			}
		}
		if err = readRule(); err != nil {
			return out, err
		}
		if !sameTypedInstance(out.rule, s.CurrentRule()) {
			return out, errors.New("owned full typed rule drift")
		}
	}
	return out, ctx.Err()
}
func ownedNameAbsent(ctx context.Context, name string, d ruleInstanceDeclaration) (absent bool, err error) {
	if !ownedNamePattern.MatchString(name) {
		return false, errors.New("owned exact nonce query refused")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	opened, err := openKeySession(ctx, d)
	if err != nil {
		return false, err
	}
	s := opened.(*nativeKeySession)
	defer func() {
		err = errors.Join(err, s.Close())
		if err != nil {
			absent = false
		}
	}()
	n := s.connection
	if err = clampReadTimeout(ctx, n); err != nil {
		return false, err
	}
	table := (*instanceReadSessionTable)(unsafe.Pointer(n.session.table))
	if table.queryInstances == 0 {
		return false, errors.New("owned exact absence query unavailable")
	}
	ns, _ := windows.UTF16PtrFromString(Namespace)
	dialect, _ := windows.UTF16PtrFromString("WQL")
	query, _ := windows.UTF16PtrFromString("SELECT InstanceID, CreationClassName, PolicyRuleName, SystemCreationClassName, SystemName FROM MSFT_NetFirewallRule WHERE InstanceID = '" + name + "'")
	c := &nativeKeyCursor{owner: s, query: true}
	syscall.SyscallN(table.queryInstances, pointer(&n.session), 0, pointer(&n.options), uintptr(unsafe.Pointer(ns)), uintptr(unsafe.Pointer(dialect)), uintptr(unsafe.Pointer(query)), 0, pointer(&c.operation.handle))
	runtime.KeepAlive(ns)
	runtime.KeepAlive(dialect)
	runtime.KeepAlive(query)
	runtime.KeepAlive(n)
	runtime.KeepAlive(c)
	if c.operation.handle.table == nil {
		return false, errors.New("owned exact absence query outcome unknown")
	}
	found, e := consumeOne(ctx, c)
	if e != nil {
		return false, e
	}
	if found && s.current.instanceID != name {
		return false, errors.New("owned exact absence query selector differs")
	}
	return !found, nil
}
