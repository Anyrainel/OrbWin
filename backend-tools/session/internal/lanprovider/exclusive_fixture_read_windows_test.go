//go:build windows && amd64

package lanprovider

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

func fixtureReadDefinition(ctx context.Context, keys observedRuleKeys, d traversalDeclarations) (out fixtureDefinition, err error) {
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
			out = fixtureDefinition{}
		}
	}()
	readRule := func() error {
		c, e := s.ReadRule(ctx, keys)
		if e != nil {
			return e
		}
		found, e := consumeOne(ctx, c)
		if e != nil || !found || s.CurrentRuleKeys() != keys {
			return errors.New("fixture owned rule unavailable or identity drift")
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
				return out, errors.New("fixture filter missing/unavailable")
			}
			value := s.CurrentFilter()
			filterKeys, e := value.filterKeys(family)
			if e != nil {
				return out, e
			}
			if pass == 0 {
				out.filters[family] = value
			} else if !sameTypedInstance(out.filters[family], value) {
				return out, errors.New("fixture typed filter drift")
			}
			c, e = s.Reverse(ctx, filterKeys, family)
			if e != nil {
				return out, e
			}
			found, e = consumeOne(ctx, c)
			if e != nil || !found || s.CurrentRuleKeys() != keys {
				return out, errors.New("fixture reverse association identity drift")
			}
		}
		if err = readRule(); err != nil {
			return out, err
		}
		if !sameTypedInstance(out.rule, s.CurrentRule()) {
			return out, errors.New("fixture full typed rule drift")
		}
	}
	return out, ctx.Err()
}
func fixtureValidateDefinition(value fixtureDefinition, p fixtureNativePolicy) error {
	if value.keys.instanceID != p.intent.name {
		return errors.New("fixture returned nonce differs")
	}
	scalar := func(instance typedInstance, name string, kind, want uint32) bool {
		v, ok := instance.values[name]
		return ok && v.kind == kind && v.flags&instanceNullFlag == 0 && v.scalar == want
	}
	text := func(instance typedInstance, name, want string) bool {
		v, ok := instance.values[name]
		return ok && v.kind == 13 && v.flags&instanceNullFlag == 0 && v.text == want
	}
	array := func(instance typedInstance, name, want string) bool {
		v, ok := instance.values[name]
		return ok && v.kind == 29 && v.flags&instanceNullFlag == 0 && len(v.texts) == 1 && v.texts[0] == want
	}
	for name, want := range map[string]uint32{"Enabled": uint32(fixtureDisabled), "Action": uint32(fixtureBlock), "Direction": uint32(fixtureInbound), "Profiles": uint32(fixturePrivate), "EdgeTraversalPolicy": uint32(fixtureEdgeBlock)} {
		if !scalar(value.rule, name, 3, want) {
			return errors.New("fixture inert rule properties not honored")
		}
	}
	if !text(value.rule, "Description", "OrbWin disabled BLOCK proof actor "+p.intent.actor) || !scalar(value.rule, "LooseSourceMapping", 0, 0) || !scalar(value.rule, "LocalOnlyMapping", 0, 0) {
		return errors.New("fixture actor/mapping properties differ")
	}
	for _, filter := range value.filters {
		if !scalar(filter, "IsNegated", 0, 0) {
			return errors.New("fixture filter negation unavailable/unexpected")
		}
	}
	if !array(value.filters[0], "LocalAddress", "127.0.0.1") || !array(value.filters[0], "RemoteAddress", "127.0.0.1") || !text(value.filters[1], "AppPath", p.program) || !array(value.filters[2], "InterfaceAlias", p.alias) || !array(value.filters[4], "LocalPort", p.port) || !text(value.filters[4], "Protocol", "TCP") || !scalar(value.filters[5], "OverrideBlockRules", 0, 0) {
		return errors.New("fixture mandatory fixed scope not observed exactly; no widening fallback")
	}
	return nil
}
func fixtureNameAbsent(ctx context.Context, name string, d ruleInstanceDeclaration) (absent bool, err error) {
	if !fixtureNamePattern.MatchString(name) {
		return false, errors.New("fixture exact nonce query refused")
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
		return false, errors.New("fixture exact absence query unavailable")
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
		return false, errors.New("fixture exact absence query outcome unknown")
	}
	found, e := consumeOne(ctx, c)
	if e != nil {
		return false, e
	}
	if found && s.current.instanceID != name {
		return false, errors.New("fixture exact absence query selector differs")
	}
	return !found, nil
}
func fixtureValidateLoopbackAlias(alias string) error {
	if alias == "" || len(alias) > 1024 || strings.ContainsAny(alias, "*?[]`\x00") {
		return errors.New("fixture loopback interface alias unavailable/wildcard")
	}
	return nil
}
