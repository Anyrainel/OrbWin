package lanprovider

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func observedFilterMetadata(t *testing.T) []FilterSchema {
	t.Helper()
	data, err := os.ReadFile("testdata/qualified-filter-metadata.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != "d2faeca5614803dadd6c07b2c81c251fc6c2686e7790b5cdd9d098b284ad65ca" {
		t.Fatal("root-observed filter metadata fixture changed")
	}
	var schemas []FilterSchema
	if json.Unmarshal(data, &schemas) != nil || len(schemas) != 14 {
		t.Fatal("root-observed schema fixture malformed")
	}
	return schemas
}

func TestPinnedFilterContractsMatchIndependentNativeMetadataReceipt(t *testing.T) {
	for i, schema := range observedFilterMetadata(t) {
		if err := validatePinnedFilterSchema(schema, i); err != nil {
			t.Fatalf("observed class contract %d: %v", i, err)
		}
	}
}

func TestPinnedFiltersRejectExtraChangedNegationIdentityAndSecurityMetadata(t *testing.T) {
	for _, change := range []struct {
		index  int
		mutate func(*FilterSchema)
	}{
		{0, func(v *FilterSchema) {
			v.Class.Properties = append(v.Class.Properties, Property{Name: "ExtraCondition", Type: 13, Flags: filterMutableMetadataFlags})
		}},
		{0, func(v *FilterSchema) {
			for i := range v.Class.Properties {
				if v.Class.Properties[i].Name == "IsNegated" {
					v.Class.Properties[i].Type = 13
				}
			}
		}},
		{2, func(v *FilterSchema) {
			for i := range v.Class.Properties {
				if v.Class.Properties[i].Name == "Name" {
					v.Class.Properties[i].Flags = filterReadOnlyMetadataFlags
				}
			}
		}},
		{4, func(v *FilterSchema) {
			for i := range v.Class.Properties {
				if v.Class.Properties[i].Name == "Protocol" {
					v.Class.Properties[i].Type = 5
				}
			}
		}},
		{5, func(v *FilterSchema) {
			for i := range v.Class.Properties {
				if v.Class.Properties[i].Name == "RemoteUsers" {
					v.Class.Properties[i].HasDefault = true
				}
			}
		}},
		{6, func(v *FilterSchema) {
			v.Class.Methods = append(v.Class.Methods, Method{Name: "Unexpected", ReturnType: 5})
		}},
		{7, func(v *FilterSchema) { v.References[0].Class = "MSFT_NetAddressFilter" }},
	} {
		v := observedFilterMetadata(t)[change.index]
		change.mutate(&v)
		if validatePinnedFilterSchema(v, change.index) == nil {
			t.Fatal("new or drifted filter conditions accepted")
		}
	}
}
