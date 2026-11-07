package lanprovider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func capturedContract(t *testing.T) []byte {
	t.Helper()
	data, e := os.ReadFile("testdata/qualified-cim-metadata.json")
	if e != nil {
		t.Fatal(e)
	}
	var captured struct{ InstalledCreateParameters []struct{ Name, Type string } }
	if e = json.Unmarshal(data, &captured); e != nil {
		t.Fatal(e)
	}
	var b strings.Builder
	b.WriteString(`<PowerShellMetadata xmlns="` + cdxmlNamespace + `"><Class ClassName="root/standardcimv2/MSFT_NetFirewallRule"><StaticCmdlets><Cmdlet><CmdletMetadata Verb="New"/><Method MethodName="cim:CreateInstance"><Parameters>`)
	for _, p := range captured.InstalledCreateParameters {
		fmt.Fprintf(&b, `<Parameter ParameterName="%s"><Type PSType="%s"/></Parameter>`, p.Name, p.Type)
	}
	b.WriteString(`</Parameters></Method></Cmdlet></StaticCmdlets></Class></PowerShellMetadata>`)
	return []byte(b.String())
}
func TestInstalledContractRefusesIncompleteOrBroadenedCustomOptions(t *testing.T) {
	good := capturedContract(t)
	if _, e := ValidateInstalledContract(good); e != nil {
		t.Fatal(e)
	}
	cases := [][]byte{bytes.Replace(good, []byte("cim:CreateInstance"), []byte("arbitrary:Run"), 1), bytes.Replace(good, []byte(`ParameterName="cim:OperationOption:LocalAddress"`), []byte(`ParameterName="LocalAddress"`), 1), bytes.Replace(good, []byte(`ParameterName="cim:OperationOption:Program"`), []byte(`ParameterName="cim:OperationOption:Protocol"`), 1), bytes.Replace(good, []byte(`PSType="System.Management.Automation.WildcardPattern[]"`), []byte(`PSType="string[]"`), 1), bytes.Replace(good, []byte(`xmlns="`+cdxmlNamespace+`"`), []byte(`xmlns="https://other.example/schema"`), 1), append([]byte(`<!DOCTYPE example SYSTEM "file:///private">`), good...), bytes.Repeat([]byte{' '}, contractBudget+1)}
	for i, bad := range cases {
		if _, e := ValidateInstalledContract(bad); e == nil {
			t.Fatalf("hostile contract %d accepted", i)
		}
	}
}
