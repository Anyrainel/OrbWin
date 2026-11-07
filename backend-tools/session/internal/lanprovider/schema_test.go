package lanprovider

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func capturedClass(t *testing.T) Class {
	t.Helper()
	data, e := os.ReadFile("testdata/qualified-cim-metadata.json")
	if e != nil {
		t.Fatal(e)
	}
	var raw struct {
		ClassName, Provider string
		Properties          []struct{ Name, Type, Flags string }
		Methods             []struct{ Name, ReturnType string }
	}
	if e = json.Unmarshal(data, &raw); e != nil {
		t.Fatal(e)
	}
	out := Class{Name: raw.ClassName, Namespace: Namespace, Provider: raw.Provider}
	types := map[string]uint32{"Boolean": 0, "UInt16": 3, "UInt32": 5, "String": 13, "UInt16Array": 19, "StringArray": 29}
	for _, p := range raw.Properties {
		kind, ok := types[p.Type]
		if !ok {
			t.Fatal(p.Type)
		}
		flags := uint32(4)
		if strings.Contains(p.Flags, "ReadOnly") {
			flags |= 1 << 21
		}
		if strings.Contains(p.Flags, "Key") {
			flags |= 1 << 12
		}
		out.Properties = append(out.Properties, Property{Name: p.Name, Type: kind, Flags: flags, HasDefault: !strings.Contains(p.Flags, "NullValue")})
	}
	for _, m := range raw.Methods {
		out.Methods = append(out.Methods, Method{Name: m.Name, ReturnType: types[m.ReturnType]})
	}
	return out
}
func cloneClass(c Class) Class {
	c.Properties = append([]Property(nil), c.Properties...)
	c.Methods = append([]Method(nil), c.Methods...)
	return c
}
func TestClassContractRejectsSemanticDrift(t *testing.T) {
	base := capturedClass(t)
	if e := ValidateClass(base); e != nil {
		t.Fatal(e)
	}
	for name, change := range map[string]func(*Class){"unknown-property": func(c *Class) { c.Properties[0].Name = "Command" }, "case-alias": func(c *Class) { c.Properties[0].Name = "caption" }, "duplicate": func(c *Class) { c.Properties[1] = c.Properties[0] }, "type": func(c *Class) { c.Properties[0].Type = 29 }, "readonly": func(c *Class) { c.Properties[0].Flags &^= 1 << 21 }, "property-bit": func(c *Class) { c.Properties[0].Flags &^= 4 }, "key": func(c *Class) { c.Properties[0].Flags |= 1 << 12 }, "missing": func(c *Class) { c.Properties = c.Properties[:38] }, "provider": func(c *Class) { c.Provider = "other" }, "namespace": func(c *Class) { c.Namespace = "remote/root/StandardCimv2" }, "method-type": func(c *Class) { c.Methods[0].ReturnType = 13 }, "method": func(c *Class) { c.Methods[0].Name = "Create" }} {
		t.Run(name, func(t *testing.T) {
			bad := cloneClass(base)
			change(&bad)
			if ValidateClass(bad) == nil {
				t.Fatal("schema drift accepted")
			}
		})
	}
}

type fakeReader struct {
	class Class
	err   error
	reads int
}

func (r *fakeReader) Read(context.Context) (Class, error) { r.reads++; return r.class, r.err }

type fakeOperation struct {
	results                []classResult
	index, cancels, closes int
	events                 *[]string
	onNext                 func(int)
	closeErr               error
}

func (o *fakeOperation) Next() classResult {
	if o.onNext != nil {
		o.onNext(o.index)
	}
	if o.index >= len(o.results) {
		panic("operation did not consume final result")
	}
	r := o.results[o.index]
	o.index++
	if r.settled {
		*o.events = append(*o.events, "terminal")
	}
	return r
}
func (o *fakeOperation) Cancel() error {
	o.cancels++
	*o.events = append(*o.events, "cancel")
	return nil
}
func (o *fakeOperation) Close() error {
	o.closes++
	*o.events = append(*o.events, "op-close")
	return o.closeErr
}

type fakeConnection struct {
	op                 *fakeOperation
	events             *[]string
	closes             int
	beginErr, closeErr error
}

func (c *fakeConnection) Begin() (classOperation, error) { return c.op, c.beginErr }
func (c *fakeConnection) Close() error {
	c.closes++
	*c.events = append(*c.events, "connection-close")
	return c.closeErr
}
func TestBorrowedClassCopiedBeforeAdvanceAndClosedAfterTerminal(t *testing.T) {
	events := []string{}
	class := capturedClass(t)
	reader := &fakeReader{class: class}
	op := &fakeOperation{events: &events, results: []classResult{{reader: reader, more: true}, {settled: true}}, onNext: func(i int) {
		if i == 1 {
			class.Properties[0].Name = "overwritten"
		}
	}}
	connection := &fakeConnection{op: op, events: &events}
	got, e := observe(context.Background(), func(context.Context) (schemaConnection, error) { return connection, nil }, "fixture")
	if e != nil || !got.Complete || got.ActivationAvailable || got.AtomicCreateQualified || got.Class == nil {
		t.Fatalf("read-only schema failed: %+v %v", got, e)
	}
	if got.Class.Properties[0].Name == "overwritten" || reader.reads != 1 || op.cancels != 0 || !reflect.DeepEqual(events, []string{"terminal", "op-close", "connection-close"}) {
		t.Fatalf("borrowed lifetime/cleanup: %+v %+v", events, got)
	}
}
func TestCancelledResultIsDrainedWithoutReadingOrPartialSuccess(t *testing.T) {
	events := []string{}
	ctx, cancel := context.WithCancel(context.Background())
	reader := &fakeReader{class: capturedClass(t)}
	op := &fakeOperation{events: &events, results: []classResult{{reader: reader, more: true}, {reader: reader, more: true}, {settled: true}}, onNext: func(i int) {
		if i == 0 {
			cancel()
		}
	}}
	connection := &fakeConnection{op: op, events: &events}
	got, e := observe(ctx, func(context.Context) (schemaConnection, error) { return connection, nil }, "fixture")
	if !errors.Is(e, context.Canceled) || got.Complete || got.Class != nil || reader.reads != 0 || op.cancels != 1 || op.index != 3 || op.closes != 1 || connection.closes != 1 {
		t.Fatalf("cancelled native owner leaked/partially succeeded: %+v %v %+v", got, e, events)
	}
}
func TestSchemaFaultDrainsAndNeverParsesLaterBorrowedResults(t *testing.T) {
	events := []string{}
	bad := &fakeReader{class: capturedClass(t), err: errors.New("bad schema")}
	later := &fakeReader{class: capturedClass(t)}
	op := &fakeOperation{events: &events, results: []classResult{{reader: bad, more: true}, {reader: later, more: true}, {settled: true}}}
	connection := &fakeConnection{op: op, events: &events}
	got, e := observe(context.Background(), func(context.Context) (schemaConnection, error) { return connection, nil }, "fixture")
	if e == nil || got.Class != nil || got.Complete || later.reads != 0 || op.cancels != 1 || op.index != 3 || !reflect.DeepEqual(events, []string{"cancel", "terminal", "op-close", "connection-close"}) {
		t.Fatalf("fault did not drain before close: %+v %v", events, e)
	}
}
func TestNativeErrorBudgetAndCleanupErrorsCannotProduceCompleteSchema(t *testing.T) {
	for _, mode := range []string{"api-error", "duplicates", "result-budget", "op-close", "connection-close", "begin"} {
		t.Run(mode, func(t *testing.T) {
			events := []string{}
			reader := &fakeReader{class: capturedClass(t)}
			op := &fakeOperation{events: &events, results: []classResult{{reader: reader, more: true}, {settled: true}}}
			connection := &fakeConnection{op: op, events: &events}
			switch mode {
			case "api-error":
				op.results = []classResult{{err: errors.New("native failure")}, {settled: true}}
			case "duplicates":
				op.results = []classResult{{reader: reader, more: true}, {reader: reader, more: true}, {settled: true}}
			case "result-budget":
				op.results = []classResult{{more: true}, {more: true}, {more: true}, {more: true}, {more: true}, {settled: true}}
			case "op-close":
				op.closeErr = errors.New("close failed")
			case "connection-close":
				connection.closeErr = errors.New("close failed")
			case "begin":
				connection.beginErr = errors.New("start refused")
			}
			got, e := observe(context.Background(), func(context.Context) (schemaConnection, error) { return connection, nil }, "fixture")
			if e == nil || got.Complete || got.Class != nil || connection.closes != 1 {
				t.Fatalf("failure produced success: %+v %v", got, e)
			}
			if mode != "begin" && op.closes != 1 {
				t.Fatal("operation not closed after terminal")
			}
		})
	}
}
func TestPreCancelledSchemaDoesNotOpenNativeState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	got, e := observe(ctx, func(context.Context) (schemaConnection, error) { called = true; return nil, nil }, "")
	if !errors.Is(e, context.Canceled) || called || got.Complete {
		t.Fatal("cancelled operation opened provider state")
	}
}

func TestUnknownNativePropertyFlagRefusesSchema(t *testing.T) {
	c := capturedClass(t)
	c.Properties[0].Flags |= 1 << 31
	if ValidateClass(c) == nil {
		t.Fatal("unqualified native property flag accepted")
	}
}
