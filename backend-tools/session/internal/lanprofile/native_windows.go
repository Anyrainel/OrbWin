//go:build windows

package lanprofile

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Slot layout comes from Microsoft's netlistmgr.h (dual IDispatch interfaces),
// not the abbreviated IUnknown inheritance summary in the Learn overview.
type unknownVTable struct{ QueryInterface, AddRef, Release uintptr }
type unknownObject struct{ vtable *unknownVTable }
type managerVTable struct {
	dispatch              [7]uintptr
	excludedNetworks      [2]uintptr
	GetNetworkConnections uintptr
}
type managerObject struct{ vtable *managerVTable }
type enumVTable struct {
	dispatch        [7]uintptr
	excludedNewEnum uintptr
	Next            uintptr
}
type enumObject struct{ vtable *enumVTable }
type connectionVTable struct {
	dispatch                                                                                       [7]uintptr
	GetNetwork, excludedInternet, IsConnected, excludedConnectivity, GetConnectionID, GetAdapterID uintptr
}
type connectionObject struct{ vtable *connectionVTable }
type networkVTable struct {
	dispatch                                       [7]uintptr
	excludedNames                                  [4]uintptr
	GetNetworkID                                   uintptr
	excludedDomainConnectionsTimeInternet          [4]uintptr
	IsConnected, excludedConnectivity, GetCategory uintptr
}
type networkObject struct{ vtable *networkVTable }

var createNLM = windows.NewLazySystemDLL("ole32.dll").NewProc("CoCreateInstance")
var clsidNLM = windows.GUID{Data1: 0xdcb00c01, Data2: 0x570f, Data3: 0x4a9b, Data4: [8]byte{0x8d, 0x69, 0x19, 0x9f, 0xdb, 0xa5, 0x72, 0x3b}}
var iidNLM = windows.GUID{Data1: 0xdcb00000, Data2: 0x570f, Data3: 0x4a9b, Data4: [8]byte{0x8d, 0x69, 0x19, 0x9f, 0xdb, 0xa5, 0x72, 0x3b}}

func hresult(operation string, result uintptr) error {
	if uint32(result) != 0 {
		return fmt.Errorf("%s failed (HRESULT 0x%08x)", operation, uint32(result))
	}
	return nil
}
func release(pointer unsafe.Pointer) {
	if pointer == nil {
		return
	}
	object := (*unknownObject)(pointer)
	syscall.SyscallN(object.vtable.Release, uintptr(pointer))
	runtime.KeepAlive(pointer)
}
func getter(ctx context.Context, method uintptr, object unsafe.Pointer, output unsafe.Pointer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	result, _, _ := syscall.SyscallN(method, uintptr(object), uintptr(output))
	runtime.KeepAlive(object)
	runtime.KeepAlive(output)
	if err := hresult("Network List Manager getter", result); err != nil {
		return err
	}
	return ctx.Err()
}
func guidGetter(ctx context.Context, method uintptr, object unsafe.Pointer) (string, error) {
	var value windows.GUID
	if err := getter(ctx, method, object, unsafe.Pointer(&value)); err != nil {
		return "", err
	}
	text := strings.ToLower(value.String())
	if !validGUID(text) {
		return "", errors.New("Network List Manager returned an invalid GUID")
	}
	return text, nil
}
func boolGetter(ctx context.Context, method uintptr, object unsafe.Pointer) (bool, error) {
	var value int16
	if err := getter(ctx, method, object, unsafe.Pointer(&value)); err != nil {
		return false, err
	}
	if value != 0 && value != -1 {
		return false, errors.New("Network List Manager returned an invalid VARIANT_BOOL")
	}
	return value == -1, nil
}

type nativeIterator struct{ object *enumObject }

func (iterator *nativeIterator) Close() {
	release(unsafe.Pointer(iterator.object))
	iterator.object = nil
}
func (iterator *nativeIterator) Next(ctx context.Context) (connectionReader, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	var connection *connectionObject
	var fetched uint32
	result, _, _ := syscall.SyscallN(iterator.object.vtable.Next, uintptr(unsafe.Pointer(iterator.object)), 1, uintptr(unsafe.Pointer(&connection)), uintptr(unsafe.Pointer(&fetched)))
	runtime.KeepAlive(iterator.object)
	if uint32(result) == 1 && fetched == 0 && connection == nil {
		return nil, false, ctx.Err()
	}
	if uint32(result) != 0 || fetched != 1 || connection == nil {
		release(unsafe.Pointer(connection))
		return nil, false, fmt.Errorf("Network List Manager enumeration failed (HRESULT 0x%08x, count %d)", uint32(result), fetched)
	}
	return &nativeConnection{object: connection}, true, nil
}

type nativeConnection struct{ object *connectionObject }

func (connection *nativeConnection) Close() {
	release(unsafe.Pointer(connection.object))
	connection.object = nil
}
func currentNetwork(ctx context.Context, connection *connectionObject) (network *networkObject, err error) {
	err = getter(ctx, connection.vtable.GetNetwork, unsafe.Pointer(connection), unsafe.Pointer(&network))
	if err != nil {
		release(unsafe.Pointer(network))
		return nil, err
	}
	if network == nil {
		return nil, errors.New("Network List Manager returned no network interface")
	}
	return network, nil
}
func networkIdentity(ctx context.Context, network *networkObject) (id string, category uint32, err error) {
	id, err = guidGetter(ctx, network.vtable.GetNetworkID, unsafe.Pointer(network))
	if err != nil {
		return
	}
	connected, readErr := boolGetter(ctx, network.vtable.IsConnected, unsafe.Pointer(network))
	if readErr != nil {
		err = readErr
		return
	}
	if !connected {
		err = errors.New("network disconnected during profile observation")
		return
	}
	err = getter(ctx, network.vtable.GetCategory, unsafe.Pointer(network), unsafe.Pointer(&category))
	return
}
func (connection *nativeConnection) Read(ctx context.Context) (Record, error) {
	object := connection.object
	connectionID, err := guidGetter(ctx, object.vtable.GetConnectionID, unsafe.Pointer(object))
	if err != nil {
		return Record{}, err
	}
	adapterID, err := guidGetter(ctx, object.vtable.GetAdapterID, unsafe.Pointer(object))
	if err != nil {
		return Record{}, err
	}
	connected, err := boolGetter(ctx, object.vtable.IsConnected, unsafe.Pointer(object))
	if err != nil {
		return Record{}, err
	}
	result := Record{AdapterGUID: adapterID, ConnectionGUID: connectionID, Connected: connected}
	if connected {
		first, err := currentNetwork(ctx, object)
		if err != nil {
			return Record{}, err
		}
		defer release(unsafe.Pointer(first))
		id, category, err := networkIdentity(ctx, first)
		if err != nil {
			return Record{}, err
		}
		after, err := currentNetwork(ctx, object)
		if err != nil {
			return Record{}, err
		}
		defer release(unsafe.Pointer(after))
		afterID, afterCategory, err := networkIdentity(ctx, after)
		if err != nil {
			return Record{}, err
		}
		if id != afterID || category != afterCategory {
			return Record{}, errors.New("network/profile identity changed during observation")
		}
		result.NetworkGUID = id
		result.Category = category
	}
	afterAdapter, err := guidGetter(ctx, object.vtable.GetAdapterID, unsafe.Pointer(object))
	if err != nil {
		return Record{}, err
	}
	afterConnection, err := guidGetter(ctx, object.vtable.GetConnectionID, unsafe.Pointer(object))
	if err != nil {
		return Record{}, err
	}
	afterConnected, err := boolGetter(ctx, object.vtable.IsConnected, unsafe.Pointer(object))
	if err != nil {
		return Record{}, err
	}
	if afterAdapter != adapterID || afterConnection != connectionID || afterConnected != connected {
		return Record{}, errors.New("connection/adapter identity changed during profile observation")
	}
	result.ReadComplete = true
	return result, nil
}

func observeRecords(ctx context.Context) ([]Record, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// x/sys reports S_FALSE as Errno(1), though it is successful initialization
	// requiring a matching CoUninitialize according to the COM contract.
	if err := windows.CoInitializeEx(0, 0); err != nil && !errors.Is(err, syscall.Errno(1)) {
		return nil, fmt.Errorf("initialize profile COM apartment: %w", err)
	}
	defer windows.CoUninitialize()
	if err := createNLM.Find(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var manager *managerObject
	result, _, _ := createNLM.Call(uintptr(unsafe.Pointer(&clsidNLM)), 0, windows.CLSCTX_INPROC_SERVER|windows.CLSCTX_NO_CODE_DOWNLOAD, uintptr(unsafe.Pointer(&iidNLM)), uintptr(unsafe.Pointer(&manager)))
	if err := hresult("create local Network List Manager", result); err != nil {
		release(unsafe.Pointer(manager))
		return nil, err
	}
	if manager == nil {
		return nil, errors.New("Network List Manager returned no manager interface")
	}
	defer release(unsafe.Pointer(manager))
	var enumeration *enumObject
	err := getter(ctx, manager.vtable.GetNetworkConnections, unsafe.Pointer(manager), unsafe.Pointer(&enumeration))
	if err != nil {
		release(unsafe.Pointer(enumeration))
		return nil, err
	}
	if enumeration == nil {
		return nil, errors.New("Network List Manager returned no enumeration interface")
	}
	return collectRecords(ctx, &nativeIterator{object: enumeration})
}

func Observe(ctx context.Context, guids []string) (Snapshot, error) {
	guids = append([]string{}, guids...)
	readAt := time.Now()
	unknown, err := unknownSnapshot(guids, readAt)
	if err != nil {
		return unknown, err
	}
	if err := ctx.Err(); err != nil {
		return unknown, err
	}
	if len(guids) == 0 {
		return BuildSnapshot(guids, nil, true, readAt)
	}
	records, err := observeRecords(ctx)
	if err != nil {
		return unknown, err
	}
	if err := ctx.Err(); err != nil {
		return unknown, err
	}
	return BuildSnapshot(guids, records, true, readAt)
}
