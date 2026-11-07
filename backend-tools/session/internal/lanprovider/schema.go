// Package lanprovider is a fixed, read-only local MI schema bridge. It does not
// authenticate a helper, authorize LAN ingress, or expose provider mutations.
package lanprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const Namespace = "root/StandardCimv2"
const ClassName = "MSFT_NetFirewallRule"
const MaxOutput = 64 << 10
const maxProperties = 64
const maxMethods = 16

type Property struct {
	Name       string `json:"name"`
	Type       uint32 `json:"type"`
	Flags      uint32 `json:"flags"`
	HasDefault bool   `json:"hasDefault"`
}
type Method struct {
	Name       string `json:"name"`
	ReturnType uint32 `json:"returnType"`
}
type Class struct {
	Name       string     `json:"name"`
	Namespace  string     `json:"namespace"`
	Provider   string     `json:"provider"`
	Properties []Property `json:"properties"`
	Methods    []Method   `json:"methods"`
}
type Snapshot struct {
	Version                 int    `json:"version"`
	ReadAt                  string `json:"readAt"`
	Source                  string `json:"source"`
	Complete                bool   `json:"complete"`
	ActivationAvailable     bool   `json:"activationAvailable"`
	AtomicCreateQualified   bool   `json:"atomicCreateQualified"`
	Class                   *Class `json:"class,omitempty"`
	InstalledContractSHA256 string `json:"installedContractSha256,omitempty"`
}

type propertyContract struct {
	kind          uint32
	readOnly, key bool
}

var propertyContracts = map[string]propertyContract{
	"Caption": {13, true, false}, "Description": {13, false, false}, "ElementName": {13, false, false}, "InstanceID": {13, true, false},
	"CommonName": {13, true, false}, "PolicyKeywords": {29, true, false}, "Enabled": {3, true, false}, "PolicyDecisionStrategy": {3, true, false},
	"PolicyRoles": {29, true, false}, "ConditionListType": {3, true, false}, "CreationClassName": {13, true, true}, "ExecutionStrategy": {3, true, false},
	"Mandatory": {0, true, false}, "PolicyRuleName": {13, true, true}, "Priority": {3, true, false}, "RuleUsage": {13, true, false},
	"SequencedActions": {3, true, false}, "SystemCreationClassName": {13, true, true}, "SystemName": {13, true, true},
	"Action": {3, false, false}, "Direction": {3, false, false}, "DisplayGroup": {13, true, false}, "DisplayName": {13, true, false},
	"EdgeTraversalPolicy": {3, false, false}, "EnforcementStatus": {19, false, false}, "LocalOnlyMapping": {0, false, false}, "LooseSourceMapping": {0, false, false},
	"Owner": {13, false, false}, "PackageFamilyName": {13, false, false}, "Platforms": {29, false, false}, "PolicyAppId": {13, false, false},
	"PolicyStoreSource": {13, false, false}, "PolicyStoreSourceType": {3, false, false}, "PrimaryStatus": {3, false, false}, "Profiles": {3, false, false},
	"RemoteDynamicKeywordAddresses": {29, false, false}, "RuleGroup": {13, false, false}, "Status": {13, false, false}, "StatusCode": {5, false, false},
}

// ValidateClass checks the qualified 39-property metadata contract. MI's raw
// flags are retained separately from CIM cmdlet flags; null/default metadata is
// observational, not silently translated into a mutation permission.
func ValidateClass(c Class) error {
	if c.Name != ClassName || !strings.EqualFold(strings.ReplaceAll(c.Namespace, `\`, "/"), Namespace) || c.Provider != "wfascim" || len(c.Properties) != 39 || len(c.Methods) != 5 {
		return errors.New("unqualified local firewall class identity or schema size")
	}
	seen := map[string]bool{}
	for _, p := range c.Properties {
		expected, ok := propertyContracts[p.Name]
		// Known property/qualifier flavor, key, readonly, null and MI version
		// bits only. A new native flag is schema drift, not silent permission.
		const knownFlags = 4 | (31 << 7) | (1 << 12) | (1 << 21) | (7 << 26) | (1 << 29)
		if !ok || seen[p.Name] || p.Type != expected.kind || p.Flags&4 == 0 || p.Flags & ^uint32(knownFlags) != 0 || (p.Flags&(1<<21) != 0) != expected.readOnly || (p.Flags&(1<<12) != 0) != expected.key {
			name := "unrecognized"
			if ok {
				name = p.Name
			}
			return fmt.Errorf("unqualified firewall property metadata: %s (type %d flags 0x%08x)", name, p.Type, p.Flags)
		}
		seen[p.Name] = true
	}
	methods := map[string]bool{"Enable": true, "Disable": true, "Rename": true, "CloneObject": true, "EnumerateFull": true}
	seen = map[string]bool{}
	for _, m := range c.Methods {
		if !methods[m.Name] || seen[m.Name] || m.ReturnType != 5 {
			return errors.New("unqualified firewall method metadata")
		}
		seen[m.Name] = true
	}
	return nil
}

// Borrowed metadata must be copied before advancing the MI operation. No native
// pointer is represented by these interfaces or retained in a returned snapshot.
type classReader interface {
	Read(context.Context) (Class, error)
}
type classResult struct {
	reader  classReader
	more    bool
	settled bool
	err     error
}
type classOperation interface {
	Next() classResult
	Cancel() error
	Close() error
}
type schemaConnection interface {
	Begin() (classOperation, error)
	Close() error
}

// observe is one-shot-process code: synchronous MI calls, cancellation draining
// and final Close may block. The process owner MUST enforce a hard external
// deadline. No goroutine is abandoned to fabricate timely in-process cleanup.
func observe(ctx context.Context, open func(context.Context) (schemaConnection, error), installed string) (snapshot Snapshot, err error) {
	snapshot = Snapshot{Version: 1, ReadAt: time.Now().UTC().Format(time.RFC3339Nano), Source: "windows-mi-local-class-schema"}
	if err = ctx.Err(); err != nil {
		return snapshot, err
	}
	connection, err := open(ctx)
	if err != nil {
		return snapshot, err
	}
	defer func() {
		err = errors.Join(err, connection.Close())
		if err != nil {
			snapshot.Class = nil
			snapshot.Complete = false
			snapshot.InstalledContractSHA256 = ""
		}
	}()
	op, err := connection.Begin()
	if err != nil {
		return snapshot, err
	}
	// Close only after a positively observed terminal result; MI documents that
	// closing earlier can hang. On a provider that never settles the outer process
	// deadline owns termination, rather than a detached native cleanup goroutine.
	settled := false
	defer func() {
		if settled {
			err = errors.Join(err, op.Close())
		}
	}()
	var copied *Class
	var failure error
	cancelled := false
	count := 0
	for !settled {
		if ctx.Err() != nil && failure == nil {
			failure = ctx.Err()
		}
		if failure != nil && !cancelled {
			cancelled = true
			failure = errors.Join(failure, op.Cancel())
		}
		result := op.Next()
		if count < 5 {
			count++
		}
		settled = result.settled
		if result.err != nil && failure == nil {
			failure = result.err
		}
		if ctx.Err() != nil && failure == nil {
			failure = ctx.Err()
		}
		if count > 4 && failure == nil {
			failure = errors.New("MI class result budget exceeded")
		}
		if failure == nil && result.reader != nil {
			if copied != nil {
				failure = errors.New("MI returned duplicate class metadata")
			} else {
				value, readErr := result.reader.Read(ctx)
				if readErr == nil {
					readErr = ValidateClass(value)
				}
				if readErr != nil {
					failure = readErr
				} else {
					value.Properties = append([]Property(nil), value.Properties...)
					value.Methods = append([]Method(nil), value.Methods...)
					copied = &value
				}
			}
		}
		// After failure, continue only draining borrowed results; do not parse,
		// retain partial schema, grow buffers, or treat cancellation as completion.
	}
	if ctx.Err() != nil && failure == nil {
		failure = ctx.Err()
	}
	if failure != nil {
		return snapshot, failure
	}
	if copied == nil {
		return snapshot, errors.New("MI returned no class metadata")
	}
	sort.Slice(copied.Properties, func(i, j int) bool { return copied.Properties[i].Name < copied.Properties[j].Name })
	sort.Slice(copied.Methods, func(i, j int) bool { return copied.Methods[i].Name < copied.Methods[j].Name })
	snapshot.Class = copied
	snapshot.InstalledContractSHA256 = installed
	snapshot.Complete = true
	bytes, marshalErr := json.Marshal(snapshot)
	if marshalErr != nil || len(bytes) > MaxOutput {
		return Snapshot{Version: 1, ReadAt: snapshot.ReadAt, Source: snapshot.Source}, errors.New("MI schema output budget exceeded")
	}
	return snapshot, nil
}
