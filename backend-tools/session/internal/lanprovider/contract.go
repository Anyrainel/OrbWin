package lanprovider

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
)

const contractBudget = 256 << 10
const cdxmlNamespace = "http://schemas.microsoft.com/cmdlets-over-objects/2009/11"

var createParameters = map[string]string{
	"cim:OperationOption:PolicyStore": "string", "cim:OperationOption:GPOSession": "string", "InstanceID": "string", "ElementName": "string", "Description": "string", "RuleGroup": "string", "Enabled": "NetSecurity.Enabled", "Profiles": "NetSecurity.Profile", "Platforms": "string[]", "Direction": "NetSecurity.Direction", "Action": "NetSecurity.Action", "EdgeTraversalPolicy": "NetSecurity.EdgeTraversal", "LooseSourceMapping": "boolean", "LocalOnlyMapping": "boolean", "Owner": "string",
	"cim:OperationOption:LocalAddress": "string[]", "cim:OperationOption:RemoteAddress": "string[]", "cim:OperationOption:Protocol": "string", "cim:OperationOption:LocalPort": "string[]", "cim:OperationOption:RemotePort": "string[]", "cim:OperationOption:IcmpType": "string[]", "cim:OperationOption:DynamicTransport": "NetSecurity.DynamicTransport", "cim:OperationOption:Program": "string", "cim:OperationOption:Package": "string", "cim:OperationOption:Service": "string", "cim:OperationOption:InterfaceAlias": "System.Management.Automation.WildcardPattern[]", "cim:OperationOption:InterfaceType": "NetSecurity.InterfaceType", "cim:OperationOption:LocalUser": "string", "cim:OperationOption:RemoteUser": "string", "cim:OperationOption:RemoteMachine": "string", "cim:OperationOption:Authentication": "NetSecurity.Authentication", "cim:OperationOption:Encryption": "NetSecurity.Encryption", "cim:OperationOption:OverrideBlockRules": "boolean", "RemoteDynamicKeywordAddresses": "string[]", "PolicyAppId": "string", "PackageFamilyName": "string",
}

// ValidateInstalledContract parses only bounded, entity-free metadata. It never
// loads PowerShell, executes CDXML, follows external XML resources or invokes a
// provider. Returned hash identifies observed bytes, not a signature/trust grant.
func ValidateInstalledContract(data []byte) (string, error) {
	if len(data) == 0 || len(data) > contractBudget {
		return "", errors.New("installed firewall CDXML exceeds byte budget")
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	depth, tokens := 0, 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		tokens++
		if tokens > 16384 {
			return "", errors.New("installed CDXML token budget exceeded")
		}
		switch token := token.(type) {
		case xml.Directive:
			return "", errors.New("XML directives are unsupported")
		case xml.StartElement:
			depth++
			if depth > 32 || token.Name.Space != cdxmlNamespace {
				return "", errors.New("unqualified installed CDXML namespace or depth")
			}
		case xml.EndElement:
			depth--
		}
	}
	var document struct {
		XMLName xml.Name
		Classes []struct {
			Name   string `xml:"ClassName,attr"`
			Static struct {
				Cmdlets []struct {
					Metadata []struct {
						Verb string `xml:"Verb,attr"`
					} `xml:"CmdletMetadata"`
					Methods []struct {
						Name       string `xml:"MethodName,attr"`
						Parameters []struct {
							Name  string `xml:"ParameterName,attr"`
							Types []struct {
								Type string `xml:"PSType,attr"`
							} `xml:"Type"`
						} `xml:"Parameters>Parameter"`
					} `xml:"Method"`
				} `xml:"Cmdlet"`
			} `xml:"StaticCmdlets"`
		} `xml:"Class"`
	}
	if err := xml.Unmarshal(data, &document); err != nil {
		return "", err
	}
	if document.XMLName.Local != "PowerShellMetadata" || document.XMLName.Space != cdxmlNamespace || len(document.Classes) != 1 || document.Classes[0].Name != "root/standardcimv2/MSFT_NetFirewallRule" {
		return "", errors.New("installed CDXML class changed")
	}
	newCount := 0
	for _, cmdlet := range document.Classes[0].Static.Cmdlets {
		isNew := false
		for _, metadata := range cmdlet.Metadata {
			if metadata.Verb == "New" {
				isNew = true
			}
		}
		if !isNew {
			continue
		}
		newCount++
		if len(cmdlet.Metadata) != 1 || len(cmdlet.Methods) != 1 || cmdlet.Methods[0].Name != "cim:CreateInstance" {
			return "", errors.New("installed create method changed")
		}
		parameters := cmdlet.Methods[0].Parameters
		if len(parameters) != len(createParameters) {
			return "", errors.New("installed create parameter count changed")
		}
		seen := map[string]bool{}
		for _, parameter := range parameters {
			expected, known := createParameters[parameter.Name]
			if !known || seen[parameter.Name] || len(parameter.Types) != 1 || parameter.Types[0].Type != expected {
				return "", errors.New("installed create parameter type/name changed")
			}
			seen[parameter.Name] = true
		}
	}
	if newCount != 1 {
		return "", errors.New("installed create method is missing or ambiguous")
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
