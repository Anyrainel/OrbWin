//go:build windows

package sdkstate

import "testing"

func TestColdLeafMasterMissingNativeBackingCannotCloseOrConsumeAttempt(t *testing.T) {
	for _, master := range []*nativeColdLANMaster{{}, {closePrepared: true}, {unknown: true, closePrepared: true}} {
		if master.closeKnown() == nil || master.closeAttempted || master.closed || master.prepareOwnerRelease() == nil {
			t.Fatal("empty master or copied phase flag gained a native close sequence")
		}
	}
}
