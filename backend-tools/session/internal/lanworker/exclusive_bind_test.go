package lanworker

import (
	"errors"
	"testing"
)

func TestExclusiveLANDatagramBindRetainsExactIPv4Policy(t *testing.T) {
	if exclusiveBind("udp4", "192.168.4.10:58080", fakeSocketControl{calls: 1}, func(uintptr) error { return nil }) != nil {
		t.Fatal("reviewed UDP exclusive bind refused")
	}
	if exclusiveBind("udp6", "[::]:58080", fakeSocketControl{calls: 1}, func(uintptr) error { t.Fatal("unsupported family reached native option"); return nil }) == nil {
		t.Fatal("unsupported UDP family accepted")
	}
}

type fakeSocketControl struct {
	err   error
	calls int
}

func (r fakeSocketControl) Control(f func(uintptr)) error {
	for i := 0; i < r.calls; i++ {
		f(77)
	}
	return r.err
}
func TestExclusiveLANBindOptionFailurePreventsBind(t *testing.T) {
	for _, tc := range []struct {
		name      string
		raw       fakeSocketControl
		optionErr error
		want      bool
	}{
		{"success", fakeSocketControl{calls: 1}, nil, true},
		{"raw unavailable", fakeSocketControl{err: errors.New("closed")}, nil, false},
		{"unsupported option", fakeSocketControl{calls: 1}, errors.New("unsupported"), false},
		{"callback absent", fakeSocketControl{}, nil, false},
		{"unexpected repeated callback", fakeSocketControl{calls: 2}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := exclusiveBind("tcp4", "192.168.4.10:58080", tc.raw, func(fd uintptr) error {
				if fd != 77 {
					t.Fatal("different socket")
				}
				return tc.optionErr
			})
			if (e == nil) != tc.want {
				t.Fatalf("acceptance %v", e)
			}
		})
	}
	for _, address := range []string{"0.0.0.0:58080", "127.0.0.1:58080", "[::]:58080", "192.168.4.10:0", "192.168.4.10:058080"} {
		calls := 0
		if exclusiveBind("tcp4", address, fakeSocketControl{calls: 1}, func(uintptr) error { calls++; return nil }) == nil || calls != 0 {
			t.Fatalf("unreviewed tuple %q reached socket option", address)
		}
	}
}
