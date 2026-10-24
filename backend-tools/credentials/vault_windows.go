//go:build windows

package main

import (
	"errors"

	"github.com/danieljoos/wincred"
)

type windowsVault struct{}

func fromWindows(value *wincred.Credential) entry {
	item := entry{key: value.TargetName, username: value.UserName, secret: string(value.CredentialBlob)}
	for _, attribute := range value.Attributes {
		if attribute.Keyword == "label" {
			item.label = string(attribute.Value)
		}
	}
	return item
}
func (windowsVault) get(key string) (entry, error) {
	value, err := wincred.GetGenericCredential(key)
	if errors.Is(err, wincred.ErrElementNotFound) {
		return entry{}, missing
	}
	if err != nil {
		return entry{}, err
	}
	return fromWindows(&value.Credential), nil
}
func (windowsVault) put(item entry) error {
	value := wincred.NewGenericCredential(item.key)
	value.UserName = item.username
	value.CredentialBlob = []byte(item.secret)
	value.Attributes = []wincred.CredentialAttribute{{Keyword: "label", Value: []byte(item.label)}}
	return value.Write()
}
func (windowsVault) remove(key string) error {
	value := wincred.NewGenericCredential(key)
	err := value.Delete()
	if errors.Is(err, wincred.ErrElementNotFound) {
		return nil
	}
	return err
}
func (windowsVault) list(filter string) ([]entry, error) {
	values, err := wincred.FilteredList(filter)
	if err != nil {
		return nil, err
	}
	result := make([]entry, 0, len(values))
	for _, value := range values {
		result = append(result, fromWindows(value))
	}
	return result, nil
}
