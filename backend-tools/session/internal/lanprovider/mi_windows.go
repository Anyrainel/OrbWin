//go:build windows && amd64

package lanprovider

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// Windows SDK10.0.22621.0 mi.h, exact hash/compiled ABI evidence in architecture
// record. These are MI handles/function tables, NOT COM IUnknown vtables. Only
// fixed getters, timeout/Cancel and ownership cleanup slots are callable here.
type applicationTable struct {
	close, newSession uintptr
	forbidden         [3]uintptr
	newOptions        uintptr
}
type sessionTable struct {
	close     uintptr
	forbidden [11]uintptr
	getClass  uintptr
}
type operationTable struct {
	close, cancel uintptr
	forbidden     [3]uintptr
	getClass      uintptr
}
type optionsTable struct {
	delete      uintptr
	forbidden   [10]uintptr
	setInterval uintptr
}
type classTable struct{ getName, getNamespace, excludedServer, getCount, excludedElement, getAt, getQualifiers, getMethodCount, getMethodAt uintptr }
type qualifierTable struct {
	excluded [2]uintptr
	get      uintptr
}
type parameterTable struct{ getReturnType uintptr }
type instanceTable struct {
	excluded [2]uintptr
	delete   uintptr
}
type applicationHandle struct {
	reserved  uint64
	reserved2 uintptr
	table     *applicationTable
}
type sessionHandle struct {
	reserved  uint64
	reserved2 uintptr
	table     *sessionTable
}
type operationHandle struct {
	reserved  uint64
	reserved2 uintptr
	table     *operationTable
}
type optionsHandle struct {
	reserved  uint64
	reserved2 uintptr
	table     *optionsTable
}
type qualifierHandle struct {
	reserved  uint64
	reserved2 uintptr
	table     *qualifierTable
}
type parameterHandle struct {
	reserved  uint64
	reserved2 uintptr
	table     *parameterTable
}
type classObject struct {
	table    *classTable
	excluded [7]uintptr
}
type instanceObject struct{ table *instanceTable }
type miValue [5]uint64 // MI_Value is 40 bytes, eight-byte aligned on amd64.
type interval struct {
	days, hours, minutes, seconds, microseconds uint32
	padding                                     [3]uint32
}

var initializeMI = windows.NewLazySystemDLL("mi.dll").NewProc("MI_Application_InitializeV1")

func nativeResult(name string, result uintptr) error {
	if uint32(result) != 0 {
		return fmt.Errorf("%s failed (MI_Result %d)", name, uint32(result))
	}
	return nil
}

//go:uintptrescapes
func call(name string, method uintptr, args ...uintptr) error {
	if method == 0 {
		return errors.New("MI required function unavailable")
	}
	result, _, _ := syscall.SyscallN(method, args...)
	runtime.KeepAlive(args)
	return nativeResult(name, result)
}

//go:uintptrescapes
func callContext(ctx context.Context, name string, method uintptr, args ...uintptr) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return call(name, method, args...)
}
func pointer(value any) uintptr { // Used only for stack-owned native output handles.
	switch value := value.(type) {
	case *applicationHandle:
		return uintptr(unsafe.Pointer(value))
	case *sessionHandle:
		return uintptr(unsafe.Pointer(value))
	case *operationHandle:
		return uintptr(unsafe.Pointer(value))
	case *optionsHandle:
		return uintptr(unsafe.Pointer(value))
	default:
		panic("unsupported MI handle")
	}
}
func deleteOwnedError(value *instanceObject) error {
	if value == nil {
		return nil
	}
	if value.table == nil {
		return errors.New("MI returned malformed owned error instance")
	}
	err := call("MI owned error Delete", value.table.delete, uintptr(unsafe.Pointer(value)))
	runtime.KeepAlive(value)
	return err
}

// stringCopy trusts the local native ABI's readable pointer contract, but bounds
// traversal and rejects unterminated/invalid UTF16. No pointer survives a result.
func stringCopy(p *uint16, max int) (string, error) {
	if p == nil {
		return "", errors.New("MI returned null metadata string")
	}
	units := make([]uint16, 0, 32)
	for i := 0; i <= max; i++ {
		v := *(*uint16)(unsafe.Add(unsafe.Pointer(p), i*2))
		if v == 0 {
			for n := 0; n < len(units); n++ {
				if units[n] >= 0xd800 && units[n] <= 0xdbff {
					n++
					if n >= len(units) || units[n] < 0xdc00 || units[n] > 0xdfff {
						return "", errors.New("invalid native UTF16")
					}
				} else if units[n] >= 0xdc00 && units[n] <= 0xdfff {
					return "", errors.New("invalid native UTF16")
				}
			}
			return string(utf16.Decode(units)), nil
		}
		units = append(units, v)
	}
	return "", errors.New("native metadata string exceeds bound")
}

type nativeConnection struct {
	app     applicationHandle
	session sessionHandle
	options optionsHandle
	ctx     context.Context
}

func openNative(ctx context.Context) (schemaConnection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	n := &nativeConnection{ctx: ctx}
	var extended *instanceObject
	if err := initializeMI.Find(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result, _, _ := initializeMI.Call(0, 0, uintptr(unsafe.Pointer(&extended)), pointer(&n.app))
	runtime.KeepAlive(n)
	err := errors.Join(nativeResult("MI InitializeV1", result), deleteOwnedError(extended))
	if err != nil {
		return nil, errors.Join(err, n.Close())
	}
	if n.app.table == nil {
		return nil, errors.New("MI returned no application handle")
	}
	if err = ctx.Err(); err != nil {
		return nil, errors.Join(err, n.Close())
	}
	extended = nil
	// NULL protocol/destination/destinationOptions/callbacks: local only, with
	// no supplied server, credentials, proxy, transport or impersonation setting.
	err = call("MI NewSession", n.app.table.newSession, pointer(&n.app), 0, 0, 0, 0, uintptr(unsafe.Pointer(&extended)), pointer(&n.session))
	runtime.KeepAlive(n)
	err = errors.Join(err, deleteOwnedError(extended))
	if err != nil || n.session.table == nil {
		return nil, errors.Join(err, errors.New("MI local session unavailable"), n.Close())
	}
	if err = ctx.Err(); err != nil {
		return nil, errors.Join(err, n.Close())
	}
	err = call("MI NewOperationOptions", n.app.table.newOptions, pointer(&n.app), 1, pointer(&n.options))
	runtime.KeepAlive(n)
	if err != nil || n.options.table == nil {
		return nil, errors.Join(err, errors.New("MI operation options unavailable"), n.Close())
	}
	key, _ := windows.UTF16PtrFromString("__MI_OPERATIONOPTIONS_TIMEOUT")
	timeout := interval{seconds: 5}
	if err = ctx.Err(); err != nil {
		return nil, errors.Join(err, n.Close())
	}
	err = call("MI timeout", n.options.table.setInterval, pointer(&n.options), uintptr(unsafe.Pointer(key)), uintptr(unsafe.Pointer(&timeout)), 0)
	runtime.KeepAlive(key)
	runtime.KeepAlive(timeout)
	runtime.KeepAlive(n)
	if err != nil {
		return nil, errors.Join(err, n.Close())
	}
	return n, nil
}
func (n *nativeConnection) Close() error {
	var errs []error
	if n.options.table != nil {
		if n.options.table.delete != 0 {
			syscall.SyscallN(n.options.table.delete, pointer(&n.options))
		} else {
			errs = append(errs, errors.New("MI options Delete unavailable"))
		}
		n.options = optionsHandle{}
	}
	if n.session.table != nil {
		errs = append(errs, call("MI Session Close", n.session.table.close, pointer(&n.session), 0, 0))
		n.session = sessionHandle{}
	}
	if n.app.table != nil {
		errs = append(errs, call("MI Application Close", n.app.table.close, pointer(&n.app)))
		n.app = applicationHandle{}
	}
	runtime.KeepAlive(n)
	return errors.Join(errs...)
}
func (n *nativeConnection) Begin() (classOperation, error) {
	if err := n.ctx.Err(); err != nil {
		return nil, err
	}
	if n.session.table.getClass == 0 {
		return nil, errors.New("MI GetClass unavailable")
	}
	ns, _ := windows.UTF16PtrFromString(Namespace)
	class, _ := windows.UTF16PtrFromString(ClassName)
	op := &nativeOperation{}
	syscall.SyscallN(n.session.table.getClass, pointer(&n.session), 0, pointer(&n.options), uintptr(unsafe.Pointer(ns)), uintptr(unsafe.Pointer(class)), 0, pointer(&op.handle))
	runtime.KeepAlive(ns)
	runtime.KeepAlive(class)
	runtime.KeepAlive(n)
	runtime.KeepAlive(op)
	if op.handle.table == nil {
		return nil, errors.New("MI GetClass returned no operation")
	}
	return op, nil
}

type nativeOperation struct{ handle operationHandle }

func (op *nativeOperation) Cancel() error {
	err := call("MI Operation Cancel", op.handle.table.cancel, pointer(&op.handle), 0) // MI_REASON_NONE; not every cancellation is a timeout.
	runtime.KeepAlive(op)
	return err
}
func (op *nativeOperation) Close() error {
	err := call("MI Operation Close", op.handle.table.close, pointer(&op.handle))
	op.handle = operationHandle{}
	runtime.KeepAlive(op)
	return err
}
func (op *nativeOperation) Next() classResult {
	var c *classObject
	var more uint8 = 2
	var status uint32
	var message *uint16
	var details *instanceObject
	err := call("MI Operation GetClass", op.handle.table.getClass, pointer(&op.handle), uintptr(unsafe.Pointer(&c)), uintptr(unsafe.Pointer(&more)), uintptr(unsafe.Pointer(&status)), uintptr(unsafe.Pointer(&message)), uintptr(unsafe.Pointer(&details)))
	runtime.KeepAlive(op)
	// message/details/class are borrowed until next GetClass or Close. Never
	// Delete them, log arbitrary provider messages, or retain native pointers.
	if err != nil || more > 1 {
		return classResult{err: errors.Join(err, errors.New("MI class result incomplete or malformed"))}
	}
	r := classResult{more: more == 1, settled: more == 0, err: nativeResult("MI class operation", uintptr(status))}
	if c != nil {
		r.reader = nativeClass{c}
	}
	return r
}

type nativeClass struct{ object *classObject }

func (c nativeClass) Read(ctx context.Context) (out Class, err error) {
	if c.object == nil || c.object.table == nil {
		return out, errors.New("MI class missing native table")
	}
	table := c.object.table
	getString := func(method uintptr) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		var text *uint16
		err := callContext(ctx, "MI class string", method, uintptr(unsafe.Pointer(c.object)), uintptr(unsafe.Pointer(&text)))
		runtime.KeepAlive(c)
		if err != nil {
			return "", err
		}
		return stringCopy(text, 128)
	}
	if out.Name, err = getString(table.getName); err != nil {
		return out, err
	}
	if out.Namespace, err = getString(table.getNamespace); err != nil {
		return out, err
	}
	var qualifiers qualifierHandle
	if err = callContext(ctx, "MI class qualifiers", table.getQualifiers, uintptr(unsafe.Pointer(c.object)), uintptr(unsafe.Pointer(&qualifiers))); err != nil {
		return out, err
	}
	if qualifiers.table == nil {
		return out, errors.New("MI provider qualifier unavailable")
	}
	name, _ := windows.UTF16PtrFromString("provider")
	var kind, flags, index uint32
	var value miValue
	err = callContext(ctx, "MI provider qualifier", qualifiers.table.get, uintptr(unsafe.Pointer(&qualifiers)), uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&kind)), uintptr(unsafe.Pointer(&flags)), uintptr(unsafe.Pointer(&value)), uintptr(unsafe.Pointer(&index)))
	runtime.KeepAlive(name)
	runtime.KeepAlive(qualifiers)
	if err != nil || kind != 13 || flags&(1<<29) != 0 {
		return out, errors.Join(err, errors.New("MI provider qualifier type unavailable"))
	}
	out.Provider, err = stringCopy(*(**uint16)(unsafe.Pointer(&value)), 128)
	if err != nil {
		return out, err
	}
	var count uint32
	if err = callContext(ctx, "MI property count", table.getCount, uintptr(unsafe.Pointer(c.object)), uintptr(unsafe.Pointer(&count))); err != nil {
		return out, err
	}
	if count > maxProperties {
		return out, errors.New("MI property count exceeds budget")
	}
	for i := uint32(0); i < count; i++ {
		if err = ctx.Err(); err != nil {
			return out, err
		}
		var text *uint16
		var kind, flags uint32
		var exists uint8
		err = callContext(ctx, "MI property metadata", table.getAt, uintptr(unsafe.Pointer(c.object)), uintptr(i), uintptr(unsafe.Pointer(&text)), 0, uintptr(unsafe.Pointer(&exists)), uintptr(unsafe.Pointer(&kind)), 0, 0, uintptr(unsafe.Pointer(&flags)))
		if err != nil || exists > 1 {
			return out, errors.Join(err, errors.New("MI property metadata unavailable"))
		}
		propertyName, e := stringCopy(text, 128)
		if e != nil {
			return out, e
		}
		out.Properties = append(out.Properties, Property{Name: propertyName, Type: kind, Flags: flags, HasDefault: exists == 1})
	}
	if err = callContext(ctx, "MI method count", table.getMethodCount, uintptr(unsafe.Pointer(c.object)), uintptr(unsafe.Pointer(&count))); err != nil {
		return out, err
	}
	if count > maxMethods {
		return out, errors.New("MI method count exceeds budget")
	}
	for i := uint32(0); i < count; i++ {
		if err = ctx.Err(); err != nil {
			return out, err
		}
		var text *uint16
		var parameters parameterHandle
		var qualifiers qualifierHandle
		err = callContext(ctx, "MI method metadata", table.getMethodAt, uintptr(unsafe.Pointer(c.object)), uintptr(i), uintptr(unsafe.Pointer(&text)), uintptr(unsafe.Pointer(&qualifiers)), uintptr(unsafe.Pointer(&parameters)))
		if err != nil || parameters.table == nil {
			return out, errors.Join(err, errors.New("MI method parameters unavailable"))
		}
		methodName, e := stringCopy(text, 128)
		if e != nil {
			return out, e
		}
		var kind uint32
		err = callContext(ctx, "MI method return type", parameters.table.getReturnType, uintptr(unsafe.Pointer(&parameters)), uintptr(unsafe.Pointer(&kind)), uintptr(unsafe.Pointer(&qualifiers)))
		runtime.KeepAlive(parameters)
		if err != nil {
			return out, err
		}
		out.Methods = append(out.Methods, Method{Name: methodName, ReturnType: kind})
	}
	runtime.KeepAlive(c)
	return out, ctx.Err()
}

func installedContract() (string, error) {
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return "", err
	}
	file := filepath.Join(system, "WindowsPowerShell", "v1.0", "Modules", "NetSecurity", "NetFirewallRule.cmdletDefinition.cdxml")
	for node := file; ; node = filepath.Dir(node) {
		s, e := os.Lstat(node)
		if e != nil {
			return "", e
		}
		if s.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("installed contract link refused")
		}
		if node == file {
			if !s.Mode().IsRegular() || s.Size() > contractBudget {
				return "", errors.New("installed contract nonregular or oversized")
			}
		} else if !s.IsDir() {
			return "", errors.New("installed contract ancestry is not a directory")
		}
		if filepath.Dir(node) == node {
			break
		}
	}
	f, err := os.Open(file)
	if err != nil {
		return "", err
	}
	bytes, readErr := io.ReadAll(io.LimitReader(f, contractBudget+1))
	err = errors.Join(readErr, f.Close())
	if err != nil {
		return "", err
	}
	return ValidateInstalledContract(bytes)
}

// ObserveSchema requires an enclosing one-shot process with a hard deadline.
// Its synchronous native calls and cancellation drain cannot be preempted by
// context. It reads metadata only; no rule instances or provider methods.
func ObserveSchema(ctx context.Context) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{Version: 1, Source: "windows-mi-local-class-schema"}, err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	contract, err := installedContract()
	if err != nil {
		return Snapshot{Version: 1, Source: "windows-mi-local-class-schema"}, err
	}
	return observe(ctx, openNative, contract)
}
