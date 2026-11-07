package sdkself

import (
	"context"
	"testing"
)

func TestNativeLANPhysicalClassificationBindsActualOperationClaim(t *testing.T) {
	one, two := NewNativeLANClaim(), NewNativeLANClaim()
	if one == two {
		t.Fatal("distinct live physical claims aliased")
	}
	noBirth := nativeLANNoBirthFor(one, context.Canceled)
	if !IsNativeLANNoBirth(noBirth, one) || IsNativeLANNoBirth(noBirth, two) || IsNativeLANNoBirth(context.Canceled, one) || IsNativeLANNoBirth(noBirth, nil) {
		t.Fatal("caller error or another operation forged no-birth classification")
	}
	noResume := &nativeLANNoResume{one, context.Canceled}
	if !IsNativeLANNoResume(noResume, one) || IsNativeLANNoResume(noResume, two) || IsNativeLANNoResume(context.Canceled, one) {
		t.Fatal("caller cancellation substituted for actual no-Resume operation")
	}
}
