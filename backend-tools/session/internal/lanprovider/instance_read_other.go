//go:build !windows || !amd64

package lanprovider

import (
	"context"
	"errors"
)

func ObserveFilterSchemas(context.Context) (FilterSchemaSnapshot, error) {
	return FilterSchemaSnapshot{Version: 1, Source: "windows-mi-fixed-filter-schemas"}, errors.New("fixed local MI reads require Windows amd64")
}
func ObserveRuleKeyRoundTrip(context.Context) (RuleKeySnapshot, error) {
	return RuleKeySnapshot{Version: 1, Source: "windows-mi-fixed-rule-key-observation", Status: "unavailable"}, errors.New("fixed local MI reads require Windows amd64")
}
