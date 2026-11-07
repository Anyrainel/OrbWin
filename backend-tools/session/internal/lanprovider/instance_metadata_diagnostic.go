package lanprovider

import "fmt"

// This diagnostic accepts metadata only, never a value, key, provider message or
// instance. Unknown native property identifiers are replaced before storing or
// formatting them. ExpectedType=0 with KnownProperty=false means unavailable.
type instanceMetadataDiagnostic struct {
	Index         uint32 `json:"index"`
	Property      string `json:"property"`
	KnownProperty bool   `json:"knownProperty"`
	Type          uint32 `json:"type"`
	Flags         uint32 `json:"flags"`
	ExpectedType  uint32 `json:"expectedType"`
	ExpectedKey   bool   `json:"expectedKey"`
	Duplicate     bool   `json:"duplicate"`
}

func (d instanceMetadataDiagnostic) Error() string {
	return fmt.Sprintf("native instance metadata differs from qualified schema: index=%d property=%s knownProperty=%t type=%d flags=0x%08x expectedType=%d expectedKey=%t duplicate=%t", d.Index, d.Property, d.KnownProperty, d.Type, d.Flags, d.ExpectedType, d.ExpectedKey, d.Duplicate)
}

func sanitizedInstanceMetadataMismatch(index uint32, name string, kind, flags uint32, duplicate bool) error {
	contract, known := propertyContracts[name]
	if !known {
		name = "unknown"
	}
	return instanceMetadataDiagnostic{Index: index, Property: name, KnownProperty: known, Type: kind, Flags: flags, ExpectedType: contract.kind, ExpectedKey: contract.key, Duplicate: duplicate}
}
