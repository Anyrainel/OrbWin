package lanprovider

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMetadataDiagnosticDiscardsUnknownIdentifiersBeforeStorageOrFormatting(t *testing.T) {
	for _, secret := range []string{"private-host-secret", "PolicyRuleName\nprivate-key-value", "SystemName\x00private-value", "instance-path\\host\\private", "<provider-error-with-user-data>"} {
		err := sanitizedInstanceMetadataMismatch(3, secret, 13, 0x20201004, false)
		data, e := json.Marshal(err)
		if e != nil || strings.Contains(string(data), secret) || strings.Contains(err.Error(), secret) {
			t.Fatal("unknown native metadata escaped sanitization")
		}
		diagnostic, ok := err.(instanceMetadataDiagnostic)
		if !ok || diagnostic.Property != "unknown" || diagnostic.KnownProperty || diagnostic.ExpectedKey || diagnostic.ExpectedType != 0 || diagnostic.Index != 3 || diagnostic.Type != 13 || diagnostic.Flags != 0x20201004 {
			t.Fatal("numeric metadata or explicit unknown state lost")
		}
	}
}

func TestMetadataDiagnosticPreservesOnlyReviewedPropertyIdentityAndExpectedContract(t *testing.T) {
	for name, contract := range propertyContracts {
		err := sanitizedInstanceMetadataMismatch(2, name, 5, 0x20200004, true)
		diagnostic := err.(instanceMetadataDiagnostic)
		if !diagnostic.KnownProperty || diagnostic.Property != name || diagnostic.ExpectedType != contract.kind || diagnostic.ExpectedKey != contract.key || !diagnostic.Duplicate {
			t.Fatal("known metadata diagnostic lost reviewed expected contract")
		}
		if len(err.Error()) > 300 {
			t.Fatal("diagnostic exceeds fixed metadata budget")
		}
	}
}
