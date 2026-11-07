package lanprovider

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type FilterTraversalSnapshot struct {
	Version                int    `json:"version"`
	ReadAt                 string `json:"readAt"`
	Source                 string `json:"source"`
	Selector               string `json:"selector"`
	PolicyStore            string `json:"policyStore"`
	Complete               bool   `json:"complete"`
	Status                 string `json:"status"`
	SchemaRechecked        bool   `json:"schemaRechecked"`
	Families               int    `json:"families"`
	Passes                 int    `json:"passes"`
	ForwardReads           int    `json:"forwardReads"`
	ReverseReads           int    `json:"reverseReads"`
	RuleFullReads          int    `json:"ruleFullReads"`
	TypedValuesStable      bool   `json:"typedValuesStable"`
	ReverseIdentityMatched bool   `json:"reverseIdentityMatched"`
	Owned                  bool   `json:"owned"`
	ActivationAvailable    bool   `json:"activationAvailable"`
	AtomicSnapshot         bool   `json:"atomicSnapshot"`
	MutationAuthorized     bool   `json:"mutationAuthorized"`
}

type filterTraversalSession interface {
	Select(context.Context) (readCursor, error)
	ReadRule(context.Context, observedRuleKeys) (readCursor, error)
	Forward(context.Context, observedRuleKeys, int) (readCursor, error)
	Reverse(context.Context, observedFilterKeys, int) (readCursor, error)
	CurrentRuleKeys() observedRuleKeys
	CurrentRule() typedInstance
	CurrentFilter() typedInstance
	Close() error
}

func observeFilterTraversal(ctx context.Context, open func(context.Context) (filterTraversalSession, error)) (out FilterTraversalSnapshot, err error) {
	out = FilterTraversalSnapshot{Version: 1, ReadAt: time.Now().UTC().Format(time.RFC3339Nano), Source: "windows-mi-fixed-filter-traversal", Selector: exactRuleSelector, PolicyStore: persistentStore, Status: "unavailable"}
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
			out.Families = 0
			out.Passes = 0
			out.ForwardReads = 0
			out.ReverseReads = 0
			out.RuleFullReads = 0
			out.TypedValuesStable = false
			out.ReverseIdentityMatched = false
		}
	}()
	cursor, err := session.Select(ctx)
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
	keys := session.CurrentRuleKeys()
	if keys.instanceID != exactRuleSelector {
		return out, errors.New("fixed traversal selector differs")
	}
	cursor, err = session.ReadRule(ctx, keys)
	if err != nil {
		return out, err
	}
	found, err = consumeOne(ctx, cursor)
	if err != nil {
		return out, err
	}
	if !found || session.CurrentRuleKeys() != keys {
		return out, errors.New("typed rule disappeared or changed keys")
	}
	baselineRule := session.CurrentRule()
	out.RuleFullReads++
	var firstFilters [7]typedInstance
	for pass := 0; pass < 2; pass++ {
		for family := 0; family < 7; family++ {
			cursor, err = session.Forward(ctx, keys, family)
			if err != nil {
				return out, fmt.Errorf("fixed traversal forward family %d: %w", family, err)
			}
			found, err = consumeOne(ctx, cursor)
			if err != nil {
				return out, fmt.Errorf("fixed traversal forward family %d: %w", family, err)
			}
			if !found {
				return out, fmt.Errorf("fixed filter association absent at family %d", family)
			}
			filter := session.CurrentFilter()
			filterKeys, e := filter.filterKeys(family)
			if e != nil {
				return out, e
			}
			if pass == 0 {
				firstFilters[family] = filter
			} else if !sameTypedInstance(firstFilters[family], filter) {
				return out, fmt.Errorf("typed filter values changed across passes at family %d", family)
			}
			out.ForwardReads++
			cursor, err = session.Reverse(ctx, filterKeys, family)
			if err != nil {
				return out, fmt.Errorf("fixed traversal reverse family %d: %w", family, err)
			}
			found, err = consumeOne(ctx, cursor)
			if err != nil {
				return out, fmt.Errorf("fixed traversal reverse family %d: %w", family, err)
			}
			if !found || session.CurrentRuleKeys() != keys {
				return out, fmt.Errorf("reverse filter association identity differs or disappeared at family %d", family)
			}
			out.ReverseReads++
		}
		cursor, err = session.ReadRule(ctx, keys)
		if err != nil {
			return out, err
		}
		found, err = consumeOne(ctx, cursor)
		if err != nil {
			return out, err
		}
		if !found || session.CurrentRuleKeys() != keys || !sameTypedInstance(baselineRule, session.CurrentRule()) {
			return out, errors.New("full typed rule values changed during traversal")
		}
		out.RuleFullReads++
		out.Passes++
	}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	out.Complete = true
	out.Status = "observed"
	out.Families = 7
	out.TypedValuesStable = true
	out.ReverseIdentityMatched = true
	return out, nil
}
