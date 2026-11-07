package lanprovider

import "errors"

// Exact class metadata observed in root's 97f5b7b6 read-only receipt. These are
// class declaration flags (including NULL/default absence), NOT the still-open
// rule/filter INSTANCE getter flags. No flag translation grants mutation rights.
const filterReadOnlyMetadataFlags uint32 = 0x20200004
const filterKeyMetadataFlags uint32 = 0x20201004
const filterMutableMetadataFlags uint32 = 0x20000004
const filterAssociationMetadataFlags uint32 = 0x20001004

var commonFilterProperties = map[string]propertyContract{
	"Caption": {13, true, false}, "Description": {13, true, false}, "ElementName": {13, true, false}, "InstanceID": {13, true, false},
	"CommunicationStatus": {3, true, false}, "DetailedStatus": {3, true, false}, "HealthState": {3, true, false}, "InstallDate": {12, true, false},
	"Name": {13, true, true}, "OperatingStatus": {3, true, false}, "OperationalStatus": {19, true, false}, "PrimaryStatus": {3, true, false},
	"Status": {13, true, false}, "StatusDescriptions": {29, true, false}, "CreationClassName": {13, true, true}, "IsNegated": {0, true, false},
	"SystemCreationClassName": {13, true, true}, "SystemName": {13, true, true},
}

var familyFilterProperties = [...]map[string]uint32{
	{"LocalAddress": 29, "RemoteAddress": 29},
	{"AppPath": 13, "Package": 13},
	{"InterfaceAlias": 29},
	{"InterfaceType": 5},
	{"DynamicTransport": 5, "IcmpType": 29, "LocalPort": 29, "Protocol": 13, "RemotePort": 29},
	{"Authentication": 3, "Encryption": 3, "LocalUsers": 13, "OverrideBlockRules": 0, "RemoteMachines": 13, "RemoteUsers": 13},
	{"ServiceName": 13},
}

// validatePinnedFilterSchema is for a future fresh schema gate. Stage 1 still
// reports schemaReviewed=false until this new contract is separately reviewed.
// It neither reads values nor qualifies any instance as supported LAN scope.
func validatePinnedFilterSchema(value FilterSchema, index int) error {
	if err := validateFilterSchema(value, index); err != nil {
		return err
	}
	if index >= 7 {
		if len(value.Class.Properties) != 2 || len(value.Class.Methods) != 0 {
			return errors.New("pinned association schema size differs")
		}
		for _, p := range value.Class.Properties {
			if (p.Name != "GroupComponent" && p.Name != "PartComponent") || p.Type != 14 || p.Flags != filterAssociationMetadataFlags || p.HasDefault {
				return errors.New("pinned association property metadata differs")
			}
		}
		return nil
	}
	additional := familyFilterProperties[index]
	if len(value.Class.Properties) != len(commonFilterProperties)+len(additional) {
		return errors.New("pinned filter property count differs")
	}
	for _, p := range value.Class.Properties {
		kind, flags := uint32(0), uint32(0)
		if c, ok := commonFilterProperties[p.Name]; ok {
			kind, flags = c.kind, filterReadOnlyMetadataFlags
			if c.key {
				flags = filterKeyMetadataFlags
			}
		} else if k, ok := additional[p.Name]; ok {
			kind, flags = k, filterMutableMetadataFlags
		} else {
			return errors.New("pinned filter contains unknown property")
		}
		if p.Type != kind || p.Flags != flags || p.HasDefault {
			return errors.New("pinned filter property metadata differs")
		}
	}
	if index == 0 {
		if len(value.Class.Methods) != 1 || value.Class.Methods[0].Name != "QueryIsolationType" || value.Class.Methods[0].ReturnType != 5 {
			return errors.New("pinned address filter method metadata differs")
		}
	} else if len(value.Class.Methods) != 0 {
		return errors.New("pinned filter contains unexpected method metadata")
	}
	return nil
}
