package main

import (
	"strings"
	"testing"
)

const mappingFixture = `{"services":{"web":{"volumes":[{"type":"volume","source":"orbwin_migration_test_1","target":"/data/$${literal}","read_only":true,"volume":{"nocopy":true}}]}},"volumes":{"orbwin_migration_test_1":{"external":true,"name":"restored"}}}`

func TestMappingSchemaRefusesUnrelatedConfiguration(t *testing.T) {
	if _, err := checkedMapping(mappingFixture); err != nil {
		t.Fatal(err)
	}
	for _, contents := range []string{strings.Replace(mappingFixture, `"external":true`, `"external":false`, 1), strings.Replace(mappingFixture, `"nocopy":true`, `"nocopy":false`, 1), strings.Replace(mappingFixture, `/data/$${literal}`, `/data/${literal}`, 1), strings.Replace(mappingFixture, `"volumes":[`, `"command":["sh"],"volumes":[`, 1), mappingFixture + `{}`, strings.Repeat(" ", (256<<10)+1)} {
		if _, err := checkedMapping(contents); err == nil {
			t.Fatal("unsupported mapping accepted")
		}
	}
}
