package lanhelper

import (
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"orbwin.local/session/internal/lanlease"
)

const PublicationBudget = 64 << 10

// ReadPublication returns the validated source time so a composed observation
// cannot refresh old publication evidence merely by completing a later query.
func ReadPublication(data []byte, want lanlease.Publication, now time.Time) (time.Time, error) {
	if e := DecodePublication(data, want, now); e != nil {
		return time.Time{}, e
	}
	var s PublicationSnapshot
	_ = json.Unmarshal(data, &s)
	read, _ := time.Parse(time.RFC3339Nano, s.ReadAt)
	return read, nil
}

type PublicationBinding struct {
	ContainerPort uint16 `json:"containerPort"`
	HostPort      uint16 `json:"hostPort"`
	Protocol      string `json:"protocol"`
	Generation    uint64 `json:"generation"`
	Published     bool   `json:"published"`
}
type PublicationSnapshot struct {
	OwnerInstance string               `json:"ownerInstance"`
	Version       int                  `json:"version"`
	ReadAt        string               `json:"readAt"`
	ContainerID   string               `json:"containerId"`
	StartedAt     string               `json:"startedAt"`
	Running       bool                 `json:"running"`
	Publications  []PublicationBinding `json:"publications"`
}

// A positive result is only a fresh bounded source observation. It cannot
// reserve a publication or prove it remains present after the response.
func DecodePublication(data []byte, want lanlease.Publication, now time.Time) error {
	s, _, e := ParsePublication(data, want.ContainerID, now)
	if e != nil {
		return e
	}
	if s.OwnerInstance != want.OwnerInstance || s.StartedAt != want.StartedAt || !s.Running {
		return errors.New("publication-v2 source is stale, missing or a different incarnation")
	}
	matches := 0
	for _, p := range s.Publications {
		if p.HostPort == want.HostPort && p.ContainerPort == want.ContainerPort && p.Protocol == want.Protocol && p.Generation == want.Generation && p.Published {
			matches++
		}
	}
	if matches != 1 {
		return errors.New("exact active publication generation was not observed")
	}
	return nil
}

// ParsePublication admits discovery only from the exact current owned source.
// It does not authorize activation; Review and native commit re-observe it.
func ParsePublication(data []byte, containerID string, now time.Time) (PublicationSnapshot, time.Time, error) {
	var empty PublicationSnapshot
	if len(data) == 0 || len(data) > PublicationBudget {
		return empty, time.Time{}, errors.New("publication-v2 snapshot exceeds bound")
	}
	var s PublicationSnapshot
	if strictShape(data) != nil || canonicalKeys(data, reflect.TypeOf(s)) != nil || json.Unmarshal(data, &s) != nil {
		return empty, time.Time{}, errors.New("publication-v2 closed schema differs")
	}
	read, e := time.Parse(time.RFC3339Nano, s.ReadAt)
	started, startedErr := time.Parse(time.RFC3339Nano, s.StartedAt)
	if !recordID(s.OwnerInstance) || s.Version != 2 || e != nil || read.IsZero() || now.Before(read) || now.Sub(read) > ObservationTTL || !ValidSHA(s.ContainerID) || s.ContainerID != containerID || startedErr != nil || started.IsZero() || len(s.Publications) > 256 {
		return empty, time.Time{}, errors.New("publication-v2 source is stale, missing or a different incarnation")
	}
	seen := make(map[[2]uint16]bool)
	for _, p := range s.Publications {
		kind := uint16(1)
		if p.Protocol == "udp" {
			kind = 2
		} else if p.Protocol != "tcp" {
			return empty, time.Time{}, errors.New("publication-v2 protocol differs")
		}
		key := [2]uint16{kind, p.HostPort}
		if p.HostPort == 0 || p.ContainerPort == 0 || seen[key] || (p.Published && p.Generation == 0) {
			return empty, time.Time{}, errors.New("publication-v2 ambiguous or incomplete binding")
		}
		seen[key] = true
	}
	return s, read, nil
}
