//go:build windows

package sdkstate

import (
	"golang.org/x/sys/windows"
	"testing"
)

func TestSDKColdOwnerOriginHasNoPathOrTupleAdmission(t *testing.T) {
	// Structural refusals before native effects, not native qualification. A
	// copied launch tuple/empty code graph grants no mutation.
	for _, pair := range [][2]windows.Handle{{0, 0}, {windows.InvalidHandle, 0}, {0, windows.InvalidHandle}} {
		if transitionMappedImage(pair[0], pair[1]) == nil {
			t.Fatal("invalid borrowed owner handles admitted")
		}
	}
	a := &nativeColdAuthority{grant: transitionGrant{Parent: transitionProcess{PID: 19, Creation: 23, Image: [32]byte{1}}}}
	if a.recheckParentImage() == nil {
		t.Fatal("launch tuple without retained native parent/image graph admitted")
	}
}
