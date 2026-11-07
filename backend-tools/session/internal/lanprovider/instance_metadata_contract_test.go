package lanprovider

import (
	"context"
	"testing"
)

func qualifiedRuleDeclaration(t *testing.T) ruleInstanceDeclaration {
	t.Helper()
	c := Class{Name: ClassName, Namespace: Namespace, Provider: "wfascim", Methods: []Method{{Name: "Enable", ReturnType: 5}, {Name: "Disable", ReturnType: 5}, {Name: "Rename", ReturnType: 5}, {Name: "CloneObject", ReturnType: 5}, {Name: "EnumerateFull", ReturnType: 5}}}
	for name, p := range propertyContracts {
		flags := uint32(4) | instanceNullFlag
		if p.readOnly {
			flags |= 1 << 21
		}
		if p.key {
			flags |= 1 << 12
		}
		c.Properties = append(c.Properties, Property{Name: name, Type: p.kind, Flags: flags})
	}
	d, err := bindRuleInstanceDeclaration(c)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestInstanceMetadataSeparatesOnlyDocumentedFieldStateFromFreshDeclaration(t *testing.T) {
	d := qualifiedRuleDeclaration(t)
	// The independently observed diagnostic: Caption, type13, flags0x22200004.
	selected, e := d.validate(0, "Caption", 13, 0x22200004, false)
	if e != nil || selected {
		t.Fatal("actual null/unmodified readonly field refused")
	}
	for name, p := range d.properties {
		base := p.Flags &^ instanceFieldStateFlags
		for _, state := range []uint32{0, instanceNullFlag, instanceNotModifiedFlag, instanceFieldStateFlags} {
			selected, e = d.validate(1, name, p.Type, base|state, false)
			identity := name == "InstanceID" || p.Flags&4096 != 0
			if identity && state&instanceNullFlag != 0 {
				if e == nil {
					t.Fatal("NULL key/ID accepted")
				}
			} else if e != nil || selected != identity {
				t.Fatal("field state was confused with declaration or identity")
			}
		}
	}
}

func TestInstanceMetadataRejectsEveryUndeclaredBitAndChangedIdentityContract(t *testing.T) {
	d := qualifiedRuleDeclaration(t)
	for name, p := range d.properties {
		base := p.Flags &^ instanceFieldStateFlags
		for bit := uint32(0); bit < 32; bit++ {
			mask := uint32(1) << bit
			if mask&instanceFieldStateFlags != 0 {
				continue
			}
			if _, e := d.validate(0, name, p.Type, base^mask, false); e == nil {
				t.Fatalf("declaration bit %d drift accepted", bit)
			}
		}
		if _, e := d.validate(0, name, p.Type+1, base, false); e == nil {
			t.Fatal("type drift accepted")
		}
		if _, e := d.validate(0, name, p.Type, base, true); e == nil {
			t.Fatal("duplicate field accepted")
		}
	}
	if _, e := d.validate(0, "unknown-private-name", 13, 0x22200004, false); e == nil {
		t.Fatal("unknown property accepted")
	}
	if _, e := (ruleInstanceDeclaration{}).validate(0, "Caption", 13, 0x22200004, false); e == nil {
		t.Fatal("missing fresh declaration accepted")
	}
	// Even a valid-but-different qualifier flavor must agree with the fresh
	// qualified declaration, not a hardcoded broad allowed-mask union.
	p := d.properties["Caption"]
	p.Flags |= 1 << 8
	d.properties["Caption"] = p
	if _, e := d.validate(0, "Caption", 13, 0x22200004, false); e == nil {
		t.Fatal("fresh qualifier flavor disagreement ignored")
	}
}

func TestMetadataStateDoesNotNormalizeEmptyIdentityToNull(t *testing.T) {
	d := qualifiedRuleDeclaration(t)
	p := d.properties["PolicyRuleName"]
	if selected, e := d.validate(0, p.Name, p.Type, (p.Flags&^instanceFieldStateFlags)|instanceNotModifiedFlag, false); e != nil || !selected {
		t.Fatal("non-null identity metadata refused")
	}
	// An explicitly copied empty string is retained as the actual key; no
	// substitution with InstanceID, NULL or a guessed key is permitted.
	keys := observedRuleKeys{instanceID: exactRuleSelector, creationClassName: "c", policyRuleName: "", systemCreationClassName: "s", systemName: "h"}
	s := &fakeKeySession{current: keys, query: oneCursor(nil), reads: []*fakeReadCursor{oneCursor(nil), oneCursor(nil)}}
	out, e := keyRoundTrip(context.Background(), func(context.Context) (keySession, error) { return s, nil })
	if e != nil || !out.Complete || out.RoundTrips != 2 || s.current.policyRuleName != "" {
		t.Fatal("empty key lost during stable observed-key round-trip")
	}
	if _, e := d.validate(0, p.Name, p.Type, p.Flags|instanceNotModifiedFlag, false); e == nil {
		t.Fatal("NULL key accepted as empty")
	}
}

func TestRuleInstanceDeclarationCannotBypassFreshCompleteSchemaGate(t *testing.T) {
	d := qualifiedRuleDeclaration(t)
	c := Class{Name: ClassName, Namespace: Namespace, Provider: "wfascim", Methods: []Method{{Name: "Enable", ReturnType: 5}, {Name: "Disable", ReturnType: 5}, {Name: "Rename", ReturnType: 5}, {Name: "CloneObject", ReturnType: 5}, {Name: "EnumerateFull", ReturnType: 5}}}
	for _, p := range d.properties {
		c.Properties = append(c.Properties, p)
	}
	if _, e := bindRuleInstanceDeclaration(c); e != nil {
		t.Fatal(e)
	}
	for _, change := range []func(*Class){
		func(c *Class) { c.Properties = c.Properties[:38] },
		func(c *Class) { c.Properties[0].Flags |= instanceNotModifiedFlag },
		func(c *Class) { c.Properties[0].Name = "unknown-private-name" },
		func(c *Class) { c.Provider = "unreviewed" },
	} {
		v := c
		v.Properties = append([]Property(nil), c.Properties...)
		change(&v)
		if _, e := bindRuleInstanceDeclaration(v); e == nil {
			t.Fatal("unqualified declaration bound for instance reads")
		}
	}
}
