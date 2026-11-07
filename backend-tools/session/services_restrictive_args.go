package main

import (
	"encoding/hex"
	"errors"
)

// Restriction suffixes are closed startup conditions, never target selectors.
func parseServiceRestriction(args []string) ([]string, int, string, error) {
	if len(args) > 0 && args[len(args)-1] == "--require-new-sdk-state" {
		return checkedRestrictionBase(args[:len(args)-1], 1, "")
	}
	if len(args) >= 2 && args[len(args)-2] == "--require-disabled-sdk-state" {
		value := args[len(args)-1]
		b, err := hex.DecodeString(value)
		if err != nil || len(b) != 32 || hex.EncodeToString(b) != value {
			return nil, 0, "", errors.New("invalid disabled SDK state fingerprint")
		}
		return checkedRestrictionBase(args[:len(args)-2], 2, value)
	}
	return args, 0, "", nil // Existing arity validation rejects malformed suffixes.
}

func checkedRestrictionBase(args []string, mode int, sha string) ([]string, int, string, error) {
	if len(args) != 6 && len(args) != 8 {
		return nil, 0, "", errors.New("invalid restrictive local-services arguments")
	}
	for _, arg := range args {
		if arg == "--require-new-sdk-state" || arg == "--require-disabled-sdk-state" {
			return nil, 0, "", errors.New("duplicate local-services restriction")
		}
	}
	return args, mode, sha, nil
}
