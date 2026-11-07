//go:build windows

package wsltrust

import (
	"context"
	"errors"
	"io"
	"testing"
)

func TestGuestIssuerActualHostSealsBeforeItsNativeLaunchEdge(t *testing.T) {
	count := 0
	host := &Host{admission: &guestAdmission{}, run: func(context.Context, []string, io.Reader, int) ([]byte, error) {
		count++
		return nil, errors.New("injected native launch uncertainty")
	}}
	seal, err := host.SealGuestIssuer()
	if err != nil || seal.NoGuestIssued() != nil {
		t.Fatal("unused actual Host issuer failed sealing")
	}
	if _, err := host.runOwned(context.Background(), []string{"--help"}, nil, 64); err == nil || count != 0 {
		t.Fatal("sealed Host reached native launcher")
	}
	host = &Host{admission: &guestAdmission{}}
	host.run = func(context.Context, []string, io.Reader, int) ([]byte, error) {
		host.admission.mu.Lock()
		before := host.admission.issued == 1 && host.admission.active == 1
		host.admission.mu.Unlock()
		if !before {
			t.Fatal("native launcher ran before admission was recorded")
		}
		return nil, errors.New("injected native transport loss")
	}
	if _, err := host.runOwned(context.Background(), []string{"--help"}, nil, 64); err == nil {
		t.Fatal("injected uncertainty vanished")
	}
	seal, err = host.SealGuestIssuer()
	if err != nil || seal.NoGuestIssued() == nil {
		t.Fatal("returned/failed native call erased possible guest dispatch")
	}
}
