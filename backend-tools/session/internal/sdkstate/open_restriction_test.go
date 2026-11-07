package sdkstate

import "testing"

func TestNativeFreshRestriction(t *testing.T) {
	if restrictNativeAdmission(newServiceOnly, false) == nil {
		t.Fatal("existing empty root accepted")
	}
	if restrictNativeAdmission(newServiceOnly, true) != nil {
		t.Fatal("actual missing/create path refused")
	}
	for _, fresh := range []bool{false, true} {
		if restrictNativeAdmission(ordinaryOpen, fresh) != nil {
			t.Fatal("ordinary behavior changed")
		}
	}
	if restrictNativeAdmission(openRestriction(9), true) == nil {
		t.Fatal("unknown profile")
	}
}
