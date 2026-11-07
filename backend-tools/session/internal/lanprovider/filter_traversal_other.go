//go:build !windows || !amd64

package lanprovider

import (
	"context"
	"errors"
)

func ObserveFilterTraversal(context.Context) (FilterTraversalSnapshot, error) {
	return FilterTraversalSnapshot{Version: 1, Source: "windows-mi-fixed-filter-traversal", Status: "unavailable"}, errors.New("fixed local MI traversal requires Windows amd64")
}
