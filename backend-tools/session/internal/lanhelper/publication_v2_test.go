package lanhelper

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"orbwin.local/session/internal/lanlease"
)

func TestPublicationV2RefusesStaleIncarnationGenerationAndAmbiguity(t *testing.T) {
	now := time.Now().UTC()
	want := lanlease.Publication{OwnerInstance: strings.Repeat("7", 32), ContainerID: strings.Repeat("a", 64), StartedAt: now.Add(-time.Minute).Format(time.RFC3339Nano), HostPort: 58080, ContainerPort: 8080, Protocol: "tcp", Generation: 7}
	valid := PublicationSnapshot{OwnerInstance: strings.Repeat("7", 32), Version: 2, ReadAt: now.Format(time.RFC3339Nano), ContainerID: want.ContainerID, StartedAt: want.StartedAt, Running: true, Publications: []PublicationBinding{{ContainerPort: 8080, HostPort: 58080, Protocol: "tcp", Generation: 7, Published: true}}}
	for _, tc := range []struct {
		name   string
		mutate func(*PublicationSnapshot)
		ok     bool
	}{
		{"valid", func(*PublicationSnapshot) {}, true},
		{"old version", func(s *PublicationSnapshot) { s.Version = 1 }, false},
		{"owner restarted same counter", func(s *PublicationSnapshot) { s.OwnerInstance = strings.Repeat("8", 32) }, false},
		{"owner missing", func(s *PublicationSnapshot) { s.OwnerInstance = "" }, false},
		{"stale", func(s *PublicationSnapshot) { s.ReadAt = now.Add(-6 * time.Second).Format(time.RFC3339Nano) }, false},
		{"future", func(s *PublicationSnapshot) { s.ReadAt = now.Add(time.Second).Format(time.RFC3339Nano) }, false},
		{"restarted", func(s *PublicationSnapshot) { s.StartedAt = now.Format(time.RFC3339Nano) }, false},
		{"generation changed", func(s *PublicationSnapshot) { s.Publications[0].Generation++ }, false},
		{"listener absent", func(s *PublicationSnapshot) { s.Publications[0].Published = false }, false},
		{"duplicate tuple", func(s *PublicationSnapshot) { s.Publications = append(s.Publications, s.Publications[0]) }, false},
		{"target stopped", func(s *PublicationSnapshot) { s.Running = false }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := valid
			s.Publications = append([]PublicationBinding(nil), valid.Publications...)
			tc.mutate(&s)
			data, _ := json.Marshal(s)
			if (DecodePublication(data, want, now) == nil) != tc.ok {
				t.Fatal("wrong source acceptance")
			}
		})
	}
	data, _ := json.Marshal(valid)
	for _, bad := range []string{strings.Replace(string(data), `"version"`, `"Version"`, 1), strings.Replace(string(data), `"hostPort"`, `"HostPort"`, 1), strings.Replace(string(data), `"version":2`, `"version":2,"version":2`, 1), string(data) + `{}`} {
		if DecodePublication([]byte(bad), want, now) == nil {
			t.Fatal("closed source schema bypass")
		}
	}
}
