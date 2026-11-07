//go:build windows

package lanobserve

import (
	"context"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func nativeFixture() (nativeBuffer, *windows.IpAdapterAddresses, *windows.IpAdapterUnicastAddress) {
	buffer := make(nativeBuffer, 4096)
	adapter := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buffer[0]))
	adapter.Length = uint32(unsafe.Sizeof(*adapter))
	adapter.IfIndex = 7
	adapter.Ipv6IfIndex = 7
	adapter.Luid = 1234
	adapter.IfType = 6
	adapter.OperStatus = 1
	copy(buffer[1024:], []byte("{11111111-1111-1111-1111-111111111111}\x00"))
	adapter.AdapterName = &buffer[1024]
	address := (*windows.IpAdapterUnicastAddress)(unsafe.Pointer(&buffer[1536]))
	address.Length = uint32(unsafe.Sizeof(*address))
	address.OnLinkPrefixLength = 24
	address.DadState = 4
	adapter.FirstUnicastAddress = address
	socket := (*windows.RawSockaddrInet4)(unsafe.Pointer(&buffer[2048]))
	socket.Family = windows.AF_INET
	socket.Addr = [4]byte{192, 168, 4, 10}
	address.Address.Sockaddr = (*syscall.RawSockaddrAny)(unsafe.Pointer(socket))
	address.Address.SockaddrLength = int32(unsafe.Sizeof(*socket))
	return buffer, adapter, address
}

func TestDecodeNativeBufferChecksEveryLinkedPointerAndLength(t *testing.T) {
	buffer, _, _ := nativeFixture()
	rows, err := decodeNativeAdapters(context.Background(), buffer, func(uint64) (RawAdapter, error) { return rawFixture(), nil })
	if err != nil || len(rows) != 1 || len(rows[0].Addresses) != 1 || rows[0].Addresses[0].Address != "192.168.4.10" {
		t.Fatalf("native copied fixture failed: %+v %v", rows, err)
	}
	cases := map[string]func(nativeBuffer, *windows.IpAdapterAddresses, *windows.IpAdapterUnicastAddress){
		"adapter-cycle": func(_ nativeBuffer, a *windows.IpAdapterAddresses, _ *windows.IpAdapterUnicastAddress) { a.Next = a },
		"foreign-adapter-pointer": func(_ nativeBuffer, a *windows.IpAdapterAddresses, _ *windows.IpAdapterUnicastAddress) {
			a.Next = &windows.IpAdapterAddresses{}
		},
		"short-adapter": func(_ nativeBuffer, a *windows.IpAdapterAddresses, _ *windows.IpAdapterUnicastAddress) { a.Length = 8 },
		"oversized-adapter": func(_ nativeBuffer, a *windows.IpAdapterAddresses, _ *windows.IpAdapterUnicastAddress) {
			a.Length = 999999
		},
		"foreign-name": func(_ nativeBuffer, a *windows.IpAdapterAddresses, _ *windows.IpAdapterUnicastAddress) {
			a.AdapterName = new(byte)
		},
		"unterminated-name": func(b nativeBuffer, a *windows.IpAdapterAddresses, _ *windows.IpAdapterUnicastAddress) {
			for i := 1024; i < 1088; i++ {
				b[i] = 'a'
			}
		},
		"foreign-address": func(_ nativeBuffer, a *windows.IpAdapterAddresses, _ *windows.IpAdapterUnicastAddress) {
			a.FirstUnicastAddress = &windows.IpAdapterUnicastAddress{}
		},
		"address-cycle": func(_ nativeBuffer, _ *windows.IpAdapterAddresses, u *windows.IpAdapterUnicastAddress) { u.Next = u },
		"short-address": func(_ nativeBuffer, _ *windows.IpAdapterAddresses, u *windows.IpAdapterUnicastAddress) { u.Length = 1 },
		"foreign-socket": func(_ nativeBuffer, _ *windows.IpAdapterAddresses, u *windows.IpAdapterUnicastAddress) {
			u.Address.Sockaddr = &syscall.RawSockaddrAny{}
		},
		"negative-socket-length": func(_ nativeBuffer, _ *windows.IpAdapterAddresses, u *windows.IpAdapterUnicastAddress) {
			u.Address.SockaddrLength = -1
		},
		"short-socket": func(_ nativeBuffer, _ *windows.IpAdapterAddresses, u *windows.IpAdapterUnicastAddress) {
			u.Address.SockaddrLength = 2
		},
		"unsupported-family": func(_ nativeBuffer, _ *windows.IpAdapterAddresses, u *windows.IpAdapterUnicastAddress) {
			u.Address.Sockaddr.Addr.Family = 999
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			buffer, adapter, address := nativeFixture()
			mutate(buffer, adapter, address)
			if rows, err := decodeNativeAdapters(context.Background(), buffer, func(uint64) (RawAdapter, error) { return rawFixture(), nil }); err == nil || len(rows) != 0 {
				t.Fatalf("malformed native chain accepted: %+v %v", rows, err)
			}
		})
	}
}

func TestDecodeNativeBufferRefusesAdapterDriftAndCancellation(t *testing.T) {
	buffer, _, _ := nativeFixture()
	calls := 0
	_, err := decodeNativeAdapters(context.Background(), buffer, func(uint64) (RawAdapter, error) {
		calls++
		row := rawFixture()
		if calls == 2 {
			row.LUID++
		}
		return row, nil
	})
	if err == nil || calls != 2 {
		t.Fatal("adapter drift after address copying accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	if _, err := decodeNativeAdapters(ctx, buffer, func(uint64) (RawAdapter, error) { called = true; return rawFixture(), nil }); err == nil || called {
		t.Fatal("cancelled observation entered native reads")
	}
}
