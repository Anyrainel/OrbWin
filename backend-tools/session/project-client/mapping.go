package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"path"
	"regexp"
	"strings"
	"unicode"
)

type mappingDocument struct {
	Services map[string]mappingService  `json:"services"`
	Volumes  map[string]mappingExternal `json:"volumes"`
}
type mappingService struct {
	Volumes []mappingMount `json:"volumes"`
}
type mappingMount struct {
	Type     string        `json:"type"`
	Source   string        `json:"source"`
	Target   string        `json:"target"`
	ReadOnly bool          `json:"read_only"`
	Volume   mappingNoCopy `json:"volume"`
}
type mappingNoCopy struct {
	NoCopy bool `json:"nocopy"`
}
type mappingExternal struct {
	External bool   `json:"external"`
	Name     string `json:"name"`
}

func checkedMapping(contents string) (mappingDocument, error) {
	var value mappingDocument
	if len(contents) > 256<<10 {
		return value, errors.New("mapping draft exceeds 256 KiB")
	}
	decoder := json.NewDecoder(bytes.NewBufferString(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return value, errors.New("unexpected mapping input")
	}
	valid := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)
	if len(value.Services) == 0 || len(value.Services) > 128 || len(value.Volumes) == 0 || len(value.Volumes) > 128 {
		return value, errors.New("invalid bounded mapping selection")
	}
	used := map[string]bool{}
	destinations := map[string]bool{}
	count := 0
	for name, service := range value.Services {
		if !valid.MatchString(name) || len(service.Volumes) == 0 {
			return value, errors.New("invalid mapped service")
		}
		targets := map[string]bool{}
		for _, mount := range service.Volumes {
			count++
			_, exists := value.Volumes[mount.Source]
			if count > 256 || mount.Type != "volume" || !mount.Volume.NoCopy || !exists || !strings.HasPrefix(mount.Source, "orbwin_migration_") || !valid.MatchString(mount.Source) || !strings.HasPrefix(mount.Target, "/") || mount.Target == "/" || len(mount.Target) > 4096 || path.Clean(mount.Target) != mount.Target || strings.ContainsFunc(mount.Target, unicode.IsControl) || strings.Contains(strings.ReplaceAll(mount.Target, "$$", ""), "$") || targets[mount.Target] {
				return value, errors.New("unsupported or repeated mapped mount")
			}
			targets[mount.Target] = true
			used[mount.Source] = true
		}
	}
	for alias, volume := range value.Volumes {
		if !used[alias] || !volume.External || !valid.MatchString(volume.Name) || destinations[volume.Name] {
			return value, errors.New("distinct existing external volumes are required")
		}
		destinations[volume.Name] = true
	}
	return value, nil
}
