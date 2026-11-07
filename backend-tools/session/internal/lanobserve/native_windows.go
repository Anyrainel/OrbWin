//go:build windows

package lanobserve

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const maxNativeBuffer = 4 << 20

type nativeBuffer []byte

func (buffer nativeBuffer) contains(pointer unsafe.Pointer, size uintptr) bool {
	if len(buffer) == 0 || pointer == nil || size > uintptr(len(buffer)) {
		return false
	}
	start := uintptr(unsafe.Pointer(&buffer[0]))
	at := uintptr(pointer)
	return at >= start && at-start <= uintptr(len(buffer))-size
}
func (buffer nativeBuffer) ascii(pointer *byte, limit int) (string, error) {
	if !buffer.contains(unsafe.Pointer(pointer), 1) {
		return "", errors.New("adapter name pointer is outside the native buffer")
	}
	result := make([]byte, 0, limit)
	for index := 0; index < limit; index++ {
		at := unsafe.Add(unsafe.Pointer(pointer), index)
		if !buffer.contains(at, 1) {
			break
		}
		value := *(*byte)(at)
		if value == 0 {
			return string(result), nil
		}
		if value > 127 {
			break
		}
		result = append(result, value)
	}
	return "", errors.New("adapter name is not bounded ASCII")
}

func readNativeRow(luid uint64) (RawAdapter, error) {
	row := windows.MibIfRow2{InterfaceLuid: luid}
	if err := windows.GetIfEntry2Ex(windows.MibIfEntryNormalWithoutStatistics, &row); err != nil {
		return RawAdapter{}, fmt.Errorf("read exact Windows interface: %w", err)
	}
	return RawAdapter{GUID: strings.ToLower(row.InterfaceGuid.String()), LUID: row.InterfaceLuid, Index: row.InterfaceIndex, Type: row.Type, OperationalStatus: row.OperStatus, TunnelType: row.TunnelType, Hardware: row.InterfaceAndOperStatusFlags&1 != 0, Alias: windows.UTF16ToString(row.Alias[:]), Description: windows.UTF16ToString(row.Description[:])}, nil
}

func decodeNativeAdapters(ctx context.Context, buffer nativeBuffer, inspect func(uint64) (RawAdapter, error)) ([]RawAdapter, error) {
	if len(buffer) == 0 {
		return nil, nil
	}
	result := []RawAdapter{}
	seenAdapters := map[*windows.IpAdapterAddresses]bool{}
	for adapter := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buffer[0])); adapter != nil; adapter = adapter.Next {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(result) >= MaxAdapters || seenAdapters[adapter] || uintptr(unsafe.Pointer(adapter))%unsafe.Alignof(*adapter) != 0 || !buffer.contains(unsafe.Pointer(adapter), unsafe.Sizeof(*adapter)) {
			return nil, errors.New("malformed or over-budget native adapter chain")
		}
		seenAdapters[adapter] = true
		if uintptr(adapter.Length) < unsafe.Sizeof(*adapter) || !buffer.contains(unsafe.Pointer(adapter), uintptr(adapter.Length)) {
			return nil, errors.New("unsupported native adapter structure length")
		}
		name, err := buffer.ascii(adapter.AdapterName, 64)
		if err != nil {
			return nil, err
		}
		guid, err := windows.GUIDFromString(name)
		if err != nil {
			return nil, errors.New("native adapter name is not an exact GUID")
		}
		row, err := inspect(adapter.Luid)
		if err != nil {
			return nil, err
		}
		if row.LUID != adapter.Luid || row.GUID != strings.ToLower(guid.String()) || row.Type != adapter.IfType || row.OperationalStatus != adapter.OperStatus || row.TunnelType != adapter.TunnelType || adapter.IfIndex != 0 && row.Index != adapter.IfIndex || adapter.Ipv6IfIndex != 0 && row.Index != adapter.Ipv6IfIndex {
			return nil, errors.New("Windows adapter identity/state drifted between IP Helper reads")
		}
		row.IPv4Index = adapter.IfIndex
		row.IPv6Index = adapter.Ipv6IfIndex
		row.Addresses = nil // Only the bounded adapter snapshot supplies addresses.
		seenAddresses := map[*windows.IpAdapterUnicastAddress]bool{}
		for unicast := adapter.FirstUnicastAddress; unicast != nil; unicast = unicast.Next {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if len(row.Addresses) >= 32 || seenAddresses[unicast] || uintptr(unsafe.Pointer(unicast))%unsafe.Alignof(*unicast) != 0 || !buffer.contains(unsafe.Pointer(unicast), unsafe.Sizeof(*unicast)) {
				return nil, errors.New("malformed or over-budget native unicast chain")
			}
			seenAddresses[unicast] = true
			if uintptr(unicast.Length) < unsafe.Sizeof(*unicast) || !buffer.contains(unsafe.Pointer(unicast), uintptr(unicast.Length)) || unicast.Address.SockaddrLength < 2 || uintptr(unsafe.Pointer(unicast.Address.Sockaddr))%unsafe.Alignof(windows.RawSockaddrInet4{}) != 0 || !buffer.contains(unsafe.Pointer(unicast.Address.Sockaddr), uintptr(unicast.Address.SockaddrLength)) {
				return nil, errors.New("invalid native unicast structure/socket bounds")
			}
			address := Address{PrefixLength: unicast.OnLinkPrefixLength, DADState: uint32(unicast.DadState)}
			switch unicast.Address.Sockaddr.Addr.Family {
			case windows.AF_INET:
				if uintptr(unicast.Address.SockaddrLength) < unsafe.Sizeof(windows.RawSockaddrInet4{}) {
					return nil, errors.New("short native IPv4 socket")
				}
				socket := (*windows.RawSockaddrInet4)(unsafe.Pointer(unicast.Address.Sockaddr))
				address.Address = netip.AddrFrom4(socket.Addr).String()
			case windows.AF_INET6:
				if uintptr(unicast.Address.SockaddrLength) < unsafe.Sizeof(windows.RawSockaddrInet6{}) || uintptr(unsafe.Pointer(unicast.Address.Sockaddr))%unsafe.Alignof(windows.RawSockaddrInet6{}) != 0 {
					return nil, errors.New("short native IPv6 socket")
				}
				socket := (*windows.RawSockaddrInet6)(unsafe.Pointer(unicast.Address.Sockaddr))
				address.Address = netip.AddrFrom16(socket.Addr).String()
				address.ScopeID = socket.Scope_id
			default:
				return nil, errors.New("unsupported native unicast address family")
			}
			row.Addresses = append(row.Addresses, address)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		after, err := inspect(row.LUID)
		if err != nil {
			return nil, err
		}
		if err := VerifyIdentity(row, after); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, nil
}

func Observe(ctx context.Context) (Snapshot, error) {
	readAt := time.Now()
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	var size uint32 = 16 << 10
	for attempt := 0; attempt < 5; attempt++ {
		if size == 0 || size > maxNativeBuffer {
			return Snapshot{}, errors.New("Windows adapter buffer exceeds 4 MiB")
		}
		buffer := make(nativeBuffer, size)
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, windows.GAA_FLAG_SKIP_ANYCAST|windows.GAA_FLAG_SKIP_MULTICAST|windows.GAA_FLAG_SKIP_DNS_SERVER, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buffer[0])), &size)
		if ctx.Err() != nil {
			return Snapshot{}, ctx.Err()
		}
		if errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			continue
		}
		if errors.Is(err, windows.ERROR_NO_DATA) {
			return BuildSnapshot(nil, readAt)
		}
		if err != nil {
			return Snapshot{}, fmt.Errorf("read Windows adapters: %w", err)
		}
		if size > uint32(len(buffer)) || size == 0 {
			return Snapshot{}, errors.New("Windows adapter read returned an invalid buffer length")
		}
		buffer = buffer[:size]
		rows, err := decodeNativeAdapters(ctx, buffer, readNativeRow)
		runtime.KeepAlive(buffer)
		if err != nil {
			return Snapshot{}, err
		}
		if err := ctx.Err(); err != nil {
			return Snapshot{}, err
		}
		return BuildSnapshot(rows, readAt)
	}
	return Snapshot{}, errors.New("Windows adapter inventory kept changing beyond five bounded reads; retry")
}
