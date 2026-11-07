package lanprofile

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"time"
)

const MaxAdapters = 128
const MaxConnections = 256
const MaxOutput = 64 << 10

type Profile struct {
	AdapterGUID         string `json:"adapterGuid"`
	Category            string `json:"category"`
	State               string `json:"state"`
	Reason              string `json:"reason"`
	ConnectedRecords    int    `json:"connectedRecords"`
	ActivationAvailable bool   `json:"activationAvailable"`
}
type Snapshot struct {
	Version             int       `json:"version"`
	ReadAt              string    `json:"readAt"`
	Source              string    `json:"source"`
	Complete            bool      `json:"complete"`
	ActivationAvailable bool      `json:"activationAvailable"`
	Profiles            []Profile `json:"profiles"`
}
type Record struct {
	AdapterGUID, ConnectionGUID, NetworkGUID string
	Connected, ReadComplete                  bool
	Category                                 uint32
}

var guidPattern = regexp.MustCompile(`^\{[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\}$`)

func validGUID(value string) bool {
	return value != "{00000000-0000-0000-0000-000000000000}" && guidPattern.MatchString(value)
}

func ValidateAdapters(guids []string) error {
	if len(guids) > MaxAdapters {
		return errors.New("profile observation exceeds 128 requested adapters")
	}
	seen := map[string]bool{}
	for _, guid := range guids {
		if !validGUID(guid) || seen[guid] {
			return errors.New("profile observation requires distinct canonical exact adapter GUIDs")
		}
		seen[guid] = true
	}
	return nil
}

func unknownSnapshot(guids []string, now time.Time) (Snapshot, error) {
	if err := ValidateAdapters(guids); err != nil {
		return Snapshot{}, err
	}
	if now.IsZero() {
		return Snapshot{}, errors.New("profile observation requires a timestamp")
	}
	result := Snapshot{Version: 1, ReadAt: now.UTC().Format(time.RFC3339Nano), Source: "windows-network-list-manager", Profiles: []Profile{}}
	for _, guid := range guids {
		result.Profiles = append(result.Profiles, Profile{AdapterGUID: guid, Category: "unknown", State: "unavailable", Reason: "Network profile could not be completely observed."})
	}
	sort.Slice(result.Profiles, func(i, j int) bool { return result.Profiles[i].AdapterGUID < result.Profiles[j].AdapterGUID })
	return result, nil
}

// BuildSnapshot joins copied NLM records only by exact adapter GUID. Incomplete
// or malformed scans never retain a partially observed Private category.
func BuildSnapshot(guids []string, records []Record, complete bool, now time.Time) (Snapshot, error) {
	result, err := unknownSnapshot(guids, now)
	if err != nil {
		return result, err
	}
	if !complete {
		return result, errors.New("network profile enumeration was incomplete")
	}
	if len(records) > MaxConnections {
		return result, errors.New("network profile connection budget exceeded")
	}
	seen := map[string]bool{}
	byAdapter := map[string][]Record{}
	for _, record := range records {
		if !validGUID(record.AdapterGUID) || !validGUID(record.ConnectionGUID) || seen[record.ConnectionGUID] || record.Connected && record.ReadComplete && !validGUID(record.NetworkGUID) {
			return result, errors.New("network profile enumeration contains malformed or duplicate identities")
		}
		seen[record.ConnectionGUID] = true
		byAdapter[record.AdapterGUID] = append(byAdapter[record.AdapterGUID], record)
	}
	for index := range result.Profiles {
		profile := &result.Profiles[index]
		matches := byAdapter[profile.AdapterGUID]
		connected := []Record{}
		for _, record := range matches {
			if record.Connected {
				connected = append(connected, record)
			}
		}
		profile.ConnectedRecords = len(connected)
		switch {
		case len(matches) == 0:
			profile.State = "missing"
			profile.Reason = "No Network List Manager connection matches this exact adapter GUID."
		case len(connected) == 0:
			profile.State = "disconnected"
			profile.Reason = "No connected Network List Manager record matches this adapter."
		case len(connected) > 1:
			profile.State = "ambiguous"
			profile.Reason = "Several connected records match this adapter; no category was selected."
		case !connected[0].ReadComplete || connected[0].Category > 2:
			profile.State = "unavailable"
			profile.Reason = "Connected network category was not completely supported/read."
		default:
			profile.State = "observed"
			profile.Category = []string{"public", "private", "domain"}[connected[0].Category]
			profile.Reason = "Windows category observed; exposure still requires separate explicit consent and firewall verification."
		}
	}
	result.Complete = true
	data, err := json.Marshal(result)
	if err != nil || len(data) > MaxOutput {
		unknown, _ := unknownSnapshot(guids, now)
		return unknown, errors.New("network profile snapshot exceeds output budget")
	}
	return result, nil
}

type connectionReader interface {
	Read(context.Context) (Record, error)
	Close()
}
type connectionIterator interface {
	Next(context.Context) (connectionReader, bool, error)
	Close()
}

func collectRecords(ctx context.Context, iterator connectionIterator) ([]Record, error) {
	defer iterator.Close()
	records := []Record{}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		connection, more, err := iterator.Next(ctx)
		if err != nil {
			if connection != nil {
				connection.Close()
			}
			return nil, err
		}
		if !more {
			if connection != nil {
				connection.Close()
				return nil, errors.New("malformed connection enumeration terminator")
			}
			return records, nil
		}
		if connection == nil {
			return nil, errors.New("connection enumeration returned a missing interface")
		}
		if len(records) >= MaxConnections {
			connection.Close()
			return nil, errors.New("network profile enumeration exceeds 256 connections")
		}
		record, err := connection.Read(ctx)
		connection.Close()
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
}
