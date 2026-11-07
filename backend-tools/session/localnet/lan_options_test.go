package localnet

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanobserve"
	"orbwin.local/session/internal/lanprofile"
)

func lanOptionsFixture(t *testing.T) (lanOptionsSource, time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 25, 20, 0, 0, 0, time.UTC)
	guid := "{11111111-1111-1111-1111-111111111111}"
	interfaces, err := lanobserve.BuildSnapshot([]lanobserve.RawAdapter{{GUID: guid, LUID: 42, Index: 7, IPv4Index: 7, Alias: "Ethernet", Type: 6, OperationalStatus: 1, Hardware: true,
		Addresses: []lanobserve.Address{{Address: "192.168.4.10", PrefixLength: 24, DADState: 4}}}}, now.Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := lanprofile.BuildSnapshot([]string{guid}, []lanprofile.Record{{AdapterGUID: guid, ConnectionGUID: "{22222222-2222-2222-2222-222222222222}", NetworkGUID: "{33333333-3333-3333-3333-333333333333}", Connected: true, ReadComplete: true, Category: 1}}, true, now.Add(-time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return lanOptionsSource{
		verify: func(context.Context) error { return nil }, targets: func(context.Context) ([]lanOptionsTarget, error) {
			return []lanOptionsTarget{{strings.Repeat("a", 64), "app"}}, nil
		},
		publication: func(_ context.Context, id string) ([]byte, error) {
			return json.Marshal(lanhelper.PublicationSnapshot{OwnerInstance: strings.Repeat("7", 32), Version: 2, ReadAt: now.Add(-2 * time.Second).Format(time.RFC3339Nano), ContainerID: id, StartedAt: now.Add(-time.Hour).Format(time.RFC3339Nano), Running: true,
				Publications: []lanhelper.PublicationBinding{{ContainerPort: 8080, HostPort: 58080, Protocol: "tcp", Generation: 1 << 60, Published: true}}})
		},
		native: func(context.Context) (NativeLANObservation, error) {
			return NativeLANObservation{interfaces, profiles}, nil
		},
	}, now
}

func TestLANOptionsIndependentHelperGateAndOldestObservation(t *testing.T) {
	source, now := lanOptionsFixture(t)
	options, err := collectLANOptions(context.Background(), source, func() time.Time { return now })
	if err != nil || !options.Complete || !options.SourceAvailable || options.HelperAvailable || options.HelperUnavailableReason != "helper-unregistered" || len(options.Publications) != 1 || !options.Publications[0].Available || len(options.Interfaces) != 1 || !options.Interfaces[0].Available {
		t.Fatalf("options %+v error %v", options, err)
	}
	if options.ReadAt != now.Add(-2*time.Second).Format(time.RFC3339Nano) || options.ExpiresAt != now.Add(3*time.Second).Format(time.RFC3339Nano) {
		t.Fatal("older source was refreshed by later completion")
	}
	data, err := json.Marshal(LANReview{PublicationGeneration: 1 << 60})
	if err != nil || !strings.Contains(string(data), `"publicationGeneration":"1152921504606846976"`) {
		t.Fatal("generation crossed JS as an unsafe number")
	}
}

func TestLANOptionsIncompleteSourcesNeverOfferActivationChoices(t *testing.T) {
	for _, kind := range []string{"404", "partial", "changed-daemon", "stale", "budget"} {
		t.Run(kind, func(t *testing.T) {
			source, now := lanOptionsFixture(t)
			switch kind {
			case "404":
				source.publication = func(context.Context, string) ([]byte, error) { return nil, errLANPublicationUnavailable }
			case "partial":
				source.targets = func(context.Context) ([]lanOptionsTarget, error) {
					return []lanOptionsTarget{{strings.Repeat("a", 64), "a"}, {strings.Repeat("b", 64), "b"}}, nil
				}
				original := source.publication
				source.publication = func(ctx context.Context, id string) ([]byte, error) {
					if id[0] == 'b' {
						return nil, errors.New("source unavailable")
					}
					return original(ctx, id)
				}
			case "changed-daemon":
				calls := 0
				source.verify = func(context.Context) error {
					calls++
					if calls > 1 {
						return errors.New("foreign")
					}
					return nil
				}
			case "stale":
				original := source.publication
				source.publication = func(ctx context.Context, id string) ([]byte, error) {
					raw, e := original(ctx, id)
					return []byte(strings.ReplaceAll(string(raw), now.Add(-2*time.Second).Format(time.RFC3339Nano), now.Add(-6*time.Second).Format(time.RFC3339Nano))), e
				}
			case "budget":
				source.targets = func(context.Context) ([]lanOptionsTarget, error) { return make([]lanOptionsTarget, 65), nil }
			}
			options, _ := collectLANOptions(context.Background(), source, func() time.Time { return now })
			if options.Complete || options.SourceAvailable || options.SourceUnavailableReason == "" {
				t.Fatalf("unconfirmed source became available: %+v", options)
			}
			for _, p := range options.Publications {
				if p.Available {
					t.Fatal("partial publication remained eligible")
				}
			}
			for _, i := range options.Interfaces {
				if i.Available {
					t.Fatal("partial combined observation remained eligible")
				}
			}
		})
	}
}

func TestLANOptionsPublicAndAmbiguousInterfaceAreVisibleUnavailable(t *testing.T) {
	for _, kind := range []string{"public", "duplicate-address", "duplicate-alias"} {
		t.Run(kind, func(t *testing.T) {
			source, now := lanOptionsFixture(t)
			original := source.native
			source.native = func(ctx context.Context) (NativeLANObservation, error) {
				o, e := original(ctx)
				switch kind {
				case "public":
					o.Profiles.Profiles[0].Category = "public"
				case "duplicate-address", "duplicate-alias":
					a := o.Interfaces.Adapters[0]
					a.GUID = "{44444444-4444-4444-4444-444444444444}"
					a.LUID = "99"
					a.Index = 9
					a.IPv4Index = 9
					if kind == "duplicate-alias" {
						a.Addresses = []lanobserve.Address{{Address: "192.168.4.11", PrefixLength: 24, DADState: 4}}
					} else {
						a.Alias = "Other"
					}
					o.Interfaces.Adapters = append(o.Interfaces.Adapters, a)
				}
				return o, e
			}
			options, e := collectLANOptions(context.Background(), source, func() time.Time { return now })
			if e != nil || len(options.Interfaces) == 0 {
				t.Fatal("expected visible native choices", e)
			}
			for _, choice := range options.Interfaces {
				if choice.Available {
					t.Fatal("ambiguous/public interface offered", choice)
				}
			}
		})
	}
}

func TestLANOptionsQuerySlotSurvivesRPCDeadlineUntilOwnedReadJoins(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	r := &ServiceRuntime{ctx: context.Background(), opts: RuntimeOptions{lanOptionsReader: func(context.Context) (LANOptions, error) {
		calls.Add(1)
		close(entered)
		<-release
		return LANOptions{Version: 1}, nil
	}}}
	done := make(chan struct{})
	go func() { defer close(done); _, _ = r.ReadLANOptions(context.Background()) }()
	<-entered
	if _, e := r.ReadLANOptions(context.Background()); e == nil || calls.Load() != 1 {
		t.Fatal("second observation queued/started")
	}
	closed := make(chan struct{})
	go func() { r.closeLANQueries(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("shutdown abandoned active read")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	<-done
	<-closed
	if _, e := r.ReadLANOptions(context.Background()); e == nil {
		t.Fatal("new read after shutdown")
	}
}

func TestLANOptionsRequestClosedShape(t *testing.T) {
	if e := validateLANIPC([]byte(`{"version":1,"operation":"lan-options"}`), "lan-options"); e != nil {
		t.Fatal(e)
	}
	for _, raw := range []string{`{"version":1,"operation":"lan-options","revision":1}`, `{"version":1,"Version":1,"operation":"lan-options"}`, `{"version":1,"operation":"lan-options","operation":"lan-options"}`, `{"version":1,"operation":"lan-options","lanSelection":null}`, `{"version":1,"operation":"lan-options"} {}`, `{"operation":"lan-options"}`} {
		if validateLANIPC([]byte(raw), "lan-options") == nil {
			t.Fatal("invalid LAN shape admitted", raw)
		}
	}
}
