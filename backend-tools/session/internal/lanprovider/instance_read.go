package lanprovider

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

// These source-fixed qualification selectors are not a product rule-discovery
// API or ownership evidence. No caller can supply a query, store or identifier.
const exactRuleSelector = "CoreNet-DNS-Out-UDP"
const exactRuleQuery = "SELECT InstanceID, CreationClassName, PolicyRuleName, SystemCreationClassName, SystemName FROM MSFT_NetFirewallRule WHERE InstanceID = 'CoreNet-DNS-Out-UDP'"
const persistentStore = "PersistentStore"

type filterFamily struct{ class, association string }

var filterFamilies = [...]filterFamily{
	{"MSFT_NetAddressFilter", "MSFT_NetFirewallRuleFilterByAddress"},
	{"MSFT_NetApplicationFilter", "MSFT_NetFirewallRuleFilterByApplication"},
	{"MSFT_NetInterfaceFilter", "MSFT_NetFirewallRuleFilterByInterface"},
	{"MSFT_NetInterfaceTypeFilter", "MSFT_NetFirewallRuleFilterByInterfaceType"},
	{"MSFT_NetProtocolPortFilter", "MSFT_NetFirewallRuleFilterByProtocolPort"},
	{"MSFT_NetNetworkLayerSecurityFilter", "MSFT_NetFirewallRuleFilterBySecurity"},
	{"MSFT_NetServiceFilter", "MSFT_NetFirewallRuleFilterByService"},
}

type ReferenceMetadata struct {
	Name  string `json:"name"`
	Class string `json:"class"`
}
type FilterSchema struct {
	Class      Class               `json:"class"`
	References []ReferenceMetadata `json:"references"`
}
type FilterSchemaSnapshot struct {
	Version               int            `json:"version"`
	ReadAt                string         `json:"readAt"`
	Source                string         `json:"source"`
	Complete              bool           `json:"complete"`
	Classes               []FilterSchema `json:"classes,omitempty"`
	SchemaReviewed        bool           `json:"schemaReviewed"`
	ActivationAvailable   bool           `json:"activationAvailable"`
	AtomicCreateQualified bool           `json:"atomicCreateQualified"`
}
type RuleKeySnapshot struct {
	Version               int    `json:"version"`
	ReadAt                string `json:"readAt"`
	Source                string `json:"source"`
	Selector              string `json:"selector"`
	PolicyStore           string `json:"policyStore"`
	Complete              bool   `json:"complete"`
	Status                string `json:"status"`
	Found                 bool   `json:"found"`
	KeyCount              int    `json:"keyCount"`
	RuleSchemaRechecked   bool   `json:"ruleSchemaRechecked"`
	ExactIDAndKeysMatched bool   `json:"exactIdAndKeysMatched"`
	RoundTrips            int    `json:"roundTrips"`
	Owned                 bool   `json:"owned"`
	ActivationAvailable   bool   `json:"activationAvailable"`
	AtomicSnapshot        bool   `json:"atomicSnapshot"`
	MutationAuthorized    bool   `json:"mutationAuthorized"`
}

// Private, non-serializable-by-JSON observed values. Do not log this structure,
// attach it to an error, persist it or expose it as a receipt/caller capability.
type observedRuleKeys struct{ instanceID, creationClassName, policyRuleName, systemCreationClassName, systemName string }

type cursorStep struct {
	hasValue, settled bool
	err               error
}
type readCursor interface {
	Next() cursorStep
	CopyCurrent(context.Context) error
	Cancel() error
	Close() error
}

// consumeOne owns a synchronous read through positive terminal consumption.
// After failure it only cancels once and drains/discards; an external one-shot
// process deadline remains mandatory if native calls/provider cleanup block.
func consumeOne(ctx context.Context, cursor readCursor) (found bool, err error) {
	settled, cancelled := false, false
	processed := 0
	defer func() {
		if settled {
			err = errors.Join(err, cursor.Close())
		}
		if err != nil {
			found = false
		}
	}()
	for !settled {
		if ctx.Err() != nil && err == nil {
			err = ctx.Err()
		}
		if err != nil && !cancelled {
			cancelled = true
			err = errors.Join(err, cursor.Cancel())
		}
		step := cursor.Next()
		settled = step.settled
		if processed < 5 {
			processed++
		}
		if step.err != nil && err == nil {
			err = step.err
		}
		if ctx.Err() != nil && err == nil {
			err = ctx.Err()
		}
		if processed > 4 && err == nil {
			err = errors.New("native read result budget exceeded")
		}
		if err == nil && step.hasValue {
			if found {
				err = errors.New("native exact read returned ambiguous results")
			} else {
				err = cursor.CopyCurrent(ctx)
				found = err == nil
			}
		}
	}
	if ctx.Err() != nil && err == nil {
		err = ctx.Err()
	}
	return found, err
}

var schemaIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)

func validateFilterSchema(value FilterSchema, index int) error {
	if index < 0 || index >= 14 {
		return errors.New("filter schema index unsupported")
	}
	family := filterFamilies[index%7]
	expected := family.class
	if index >= 7 {
		expected = family.association
	}
	c := value.Class
	if c.Name != expected || !strings.EqualFold(strings.ReplaceAll(c.Namespace, `\`, "/"), Namespace) || c.Provider != "wfascim" || len(c.Properties) > 64 || len(c.Methods) > 16 {
		return errors.New("fixed local filter schema identity unavailable")
	}
	seen := map[string]bool{}
	referenceProperties := map[string]bool{}
	for _, p := range c.Properties {
		if !schemaIdentifier.MatchString(p.Name) || seen[p.Name] || p.Type > 31 {
			return errors.New("filter schema property metadata malformed")
		}
		seen[p.Name] = true
		if p.Type == 14 {
			referenceProperties[p.Name] = true
		}
	}
	seen = map[string]bool{}
	for _, m := range c.Methods {
		if !schemaIdentifier.MatchString(m.Name) || seen[m.Name] || m.ReturnType > 31 {
			return errors.New("filter schema method metadata malformed")
		}
		seen[m.Name] = true
	}
	seen = map[string]bool{}
	for _, r := range value.References {
		if !referenceProperties[r.Name] || !schemaIdentifier.MatchString(r.Class) || seen[r.Name] {
			return errors.New("filter reference metadata malformed")
		}
		seen[r.Name] = true
	}
	if len(seen) != len(referenceProperties) {
		return errors.New("filter reference metadata incomplete")
	}
	if index >= 7 {
		if len(value.References) != 2 {
			return errors.New("association has unqualified reference ends")
		}
		ends := map[string]string{}
		for _, r := range value.References {
			ends[r.Name] = r.Class
		}
		if ends["GroupComponent"] != ClassName || ends["PartComponent"] != family.class {
			return errors.New("association reference direction differs from installed contract")
		}
		for _, p := range c.Properties {
			if p.Type == 14 && p.Flags&(1<<12) == 0 {
				return errors.New("association reference is not a native key")
			}
		}
	}
	return nil
}

type keySession interface {
	Query(context.Context) (readCursor, error)
	Read(context.Context, observedRuleKeys) (readCursor, error)
	CurrentKeys() observedRuleKeys
	Close() error
}

func keyRoundTrip(ctx context.Context, open func(context.Context) (keySession, error)) (out RuleKeySnapshot, err error) {
	out = RuleKeySnapshot{Version: 1, ReadAt: time.Now().UTC().Format(time.RFC3339Nano), Source: "windows-mi-fixed-rule-key-observation", Selector: exactRuleSelector, PolicyStore: persistentStore, Status: "unavailable"}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	session, err := open(ctx)
	if err != nil {
		return out, err
	}
	defer func() {
		err = errors.Join(err, session.Close())
		if err != nil {
			out.Complete = false
			out.Status = "unavailable"
			out.Found = false
			out.KeyCount = 0
			out.ExactIDAndKeysMatched = false
			out.RoundTrips = 0
		}
	}()
	cursor, err := session.Query(ctx)
	if err != nil {
		return out, err
	}
	found, err := consumeOne(ctx, cursor)
	if err != nil {
		return out, err
	}
	if !found {
		out.Complete = true
		out.Status = "not-found"
		return out, nil
	}
	keys := session.CurrentKeys()
	if keys.instanceID != exactRuleSelector {
		return out, errors.New("exact selector differs from native result")
	}
	for n := 0; n < 2; n++ {
		cursor, err = session.Read(ctx, keys)
		if err != nil {
			return out, err
		}
		found, err = consumeOne(ctx, cursor)
		if err != nil {
			return out, err
		}
		if !found || session.CurrentKeys() != keys {
			return out, errors.New("native full-key round-trip changed or disappeared")
		}
	}
	out.Complete = true
	out.Status = "observed"
	out.Found = true
	out.KeyCount = 4
	out.ExactIDAndKeysMatched = true
	out.RoundTrips = 2
	data, e := json.Marshal(out)
	if e != nil || len(data) > MaxOutput {
		return out, errors.New("sanitized key observation exceeds budget")
	}
	return out, nil
}
