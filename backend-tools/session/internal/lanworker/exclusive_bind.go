package lanworker

import (
	"errors"
	"net"
	"net/netip"
	"strconv"
)

type rawSocketControl interface{ Control(func(uintptr)) error }

// The actual raw socket Control and setsockopt adapter are native. This policy
// enforces one explicit private IPv4 tuple and fail-closed option application;
// the fake-control tests do not create sockets or invoke Windows APIs.
func exclusiveBind(network, address string, raw rawSocketControl, set func(uintptr) error) error {
	host, port, e := net.SplitHostPort(address)
	ip, ipErr := netip.ParseAddr(host)
	p, pErr := strconv.ParseUint(port, 10, 16)
	if (network != "tcp4" && network != "udp4") || e != nil || ipErr != nil || !ip.Is4() || !ip.IsPrivate() || ip.String() != host || pErr != nil || p == 0 || strconv.FormatUint(p, 10) != port || raw == nil || set == nil {
		return errors.New("exclusive bind requires exact reviewed private IPv4 tuple")
	}
	var optionErr error
	calls := 0
	e = raw.Control(func(fd uintptr) {
		calls++
		if calls == 1 {
			optionErr = set(fd)
		}
	})
	if e != nil || optionErr != nil || calls != 1 {
		return errors.New("Windows exclusive LAN bind option was not positively applied")
	}
	return nil
}
