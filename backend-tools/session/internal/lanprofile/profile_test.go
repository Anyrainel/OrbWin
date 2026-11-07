package lanprofile

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const adapterA = "{11111111-1111-1111-1111-111111111111}"
const adapterB = "{22222222-2222-2222-2222-222222222222}"

func recordFixture(index int) Record {
	return Record{AdapterGUID: adapterA, ConnectionGUID: fmt.Sprintf("{%08x-aaaa-aaaa-aaaa-aaaaaaaaaaaa}", index), NetworkGUID: "{33333333-3333-3333-3333-333333333333}", Connected: true, ReadComplete: true, Category: 1}
}

func TestProfileJoinUsesOnlyExactGUIDAndNeverAuthorizesExposure(t *testing.T) {
	for category, expected := range []string{"public", "private", "domain"} {
		record := recordFixture(1)
		record.Category = uint32(category)
		snapshot, err := BuildSnapshot([]string{adapterA, adapterB}, []Record{record}, true, time.Now())
		if err != nil || !snapshot.Complete || snapshot.ActivationAvailable || snapshot.Profiles[0].Category != expected || snapshot.Profiles[0].ActivationAvailable || snapshot.Profiles[1].Category != "unknown" || snapshot.Profiles[1].State != "missing" {
			t.Fatalf("category %d joined incorrectly: %+v %v", category, snapshot, err)
		}
	}
	for _, guid := range []string{"Private Ethernet", "11111111-1111-1111-1111-111111111111", strings.ToUpper("{aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa}"), "{00000000-0000-0000-0000-000000000000}"} {
		if _, err := BuildSnapshot([]string{guid}, nil, true, time.Now()); err == nil {
			t.Fatalf("noncanonical identity accepted: %q", guid)
		}
	}
	if _, err := BuildSnapshot([]string{adapterA, adapterA}, nil, true, time.Now()); err == nil {
		t.Fatal("duplicate adapter request accepted")
	}
}

func TestProfileAmbiguityMissingDisconnectedAndReadFailureStayUnknown(t *testing.T) {
	cases := []struct {
		name, state string
		records     []Record
	}{
		{"missing", "missing", nil},
		{"disconnected", "disconnected", []Record{{AdapterGUID: adapterA, ConnectionGUID: recordFixture(1).ConnectionGUID, ReadComplete: true}}},
		{"ambiguous", "ambiguous", []Record{recordFixture(1), recordFixture(2)}},
		{"unknown-category", "unavailable", func() []Record { r := recordFixture(1); r.Category = 99; return []Record{r} }()},
		{"incomplete-read", "unavailable", func() []Record { r := recordFixture(1); r.ReadComplete = false; return []Record{r} }()},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			snapshot, err := BuildSnapshot([]string{adapterA}, test.records, true, time.Now())
			if err != nil || snapshot.Profiles[0].Category != "unknown" || snapshot.Profiles[0].State != test.state || snapshot.Profiles[0].ActivationAvailable {
				t.Fatalf("uncertain category adopted: %+v %v", snapshot, err)
			}
		})
	}
	active := recordFixture(1)
	disconnected := recordFixture(2)
	disconnected.Connected = false
	snapshot, err := BuildSnapshot([]string{adapterA}, []Record{active, disconnected}, true, time.Now())
	if err != nil || snapshot.Profiles[0].Category != "private" || snapshot.Profiles[0].ConnectedRecords != 1 {
		t.Fatalf("historical disconnected record changed current unique category: %+v %v", snapshot, err)
	}
	for _, complete := range []bool{false, true} {
		records := []Record{recordFixture(1)}
		if complete {
			records = append(records, recordFixture(1))
		}
		snapshot, err := BuildSnapshot([]string{adapterA}, records, complete, time.Now())
		if err == nil || snapshot.Complete || snapshot.Profiles[0].Category != "unknown" {
			t.Fatalf("incomplete/duplicate scan retained partial Private: %+v %v", snapshot, err)
		}
	}
}

func TestProfileBudgetsAndMalformedNativeIDsRefusePartialResults(t *testing.T) {
	if _, err := BuildSnapshot(make([]string, MaxAdapters+1), nil, true, time.Now()); err == nil {
		t.Fatal("adapter limit ignored")
	}
	if snapshot, err := BuildSnapshot([]string{adapterA}, make([]Record, MaxConnections+1), true, time.Now()); err == nil || snapshot.Profiles[0].Category != "unknown" {
		t.Fatal("connection limit ignored")
	}
	for _, field := range []string{"adapter", "connection", "network"} {
		record := recordFixture(2)
		switch field {
		case "adapter":
			record.AdapterGUID = "broken"
		case "connection":
			record.ConnectionGUID = "broken"
		case "network":
			record.NetworkGUID = "broken"
		}
		snapshot, err := BuildSnapshot([]string{adapterA}, []Record{recordFixture(1), record}, true, time.Now())
		if err == nil || snapshot.Profiles[0].Category != "unknown" {
			t.Fatalf("malformed %s adopted partial profile", field)
		}
	}
}

type fixtureConnection struct {
	record Record
	err    error
	closed int
	cancel context.CancelFunc
}

func (connection *fixtureConnection) Read(context.Context) (Record, error) {
	if connection.cancel != nil {
		connection.cancel()
	}
	return connection.record, connection.err
}
func (connection *fixtureConnection) Close() { connection.closed++ }

type fixtureIterator struct {
	connections []*fixtureConnection
	next        int
	closed      int
	err         error
}

func (iterator *fixtureIterator) Next(context.Context) (connectionReader, bool, error) {
	if iterator.next == len(iterator.connections) {
		return nil, false, iterator.err
	}
	value := iterator.connections[iterator.next]
	iterator.next++
	return value, true, nil
}
func (iterator *fixtureIterator) Close() { iterator.closed++ }

func TestConnectionCollectionReleasesOnReadFailureCancellationAndOverflow(t *testing.T) {
	for _, scenario := range []string{"success", "read-failure", "enumeration-failure", "cancel", "overflow"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			iterator := &fixtureIterator{connections: []*fixtureConnection{{record: recordFixture(1)}}}
			switch scenario {
			case "read-failure":
				iterator.connections[0].err = errors.New("denied")
			case "enumeration-failure":
				iterator.err = errors.New("COM disconnected")
			case "cancel":
				iterator.connections[0].cancel = cancel
			case "overflow":
				for i := 1; i <= MaxConnections; i++ {
					iterator.connections = append(iterator.connections, &fixtureConnection{record: recordFixture(i + 1)})
				}
			}
			records, err := collectRecords(ctx, iterator)
			if scenario == "success" {
				if err != nil || len(records) != 1 {
					t.Fatalf("successful collection failed: %v", err)
				}
			} else if err == nil || len(records) != 0 {
				t.Fatalf("failed collection retained partial records: %v", err)
			}
			if iterator.closed != 1 {
				t.Fatal("enumerator not released exactly once")
			}
			for _, connection := range iterator.connections {
				if connection.closed != 1 {
					t.Fatalf("connection not released exactly once: %d", connection.closed)
				}
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	iterator := &fixtureIterator{}
	if _, err := collectRecords(ctx, iterator); err == nil || iterator.next != 0 || iterator.closed != 1 {
		t.Fatal("pre-cancelled iterator leaked/entered enumeration")
	}
}
