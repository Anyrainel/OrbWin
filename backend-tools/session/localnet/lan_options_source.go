package localnet

import (
	"context"
	"errors"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/moby/moby/client"
	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanobserve"
	"orbwin.local/session/internal/lanprofile"
)

const maxLANOptionsContainers = 64
const maxLANOptionsPublications = 256

// NativeLANObservation is observation-only. Production obtains it from the
// exact SDK's fixed read-only child command, not renderer JSON or a pipe claim.
type NativeLANObservation struct {
	Interfaces lanobserve.Snapshot `json:"interfaces"`
	Profiles   lanprofile.Snapshot `json:"profiles"`
}

// ObserveNativeLAN runs ONLY in the bounded read-only diagnostic child. Its
// synchronous Windows calls cannot be interrupted by a Go context; the owning
// process enforces the outer wall-clock cap and positive child wait.
func ObserveNativeLAN(ctx context.Context) (NativeLANObservation, error) {
	interfaces, err := lanobserve.Observe(ctx)
	if err != nil {
		return NativeLANObservation{}, err
	}
	guids := make([]string, 0, len(interfaces.Adapters))
	for _, adapter := range interfaces.Adapters {
		guids = append(guids, adapter.GUID)
	}
	profiles, err := lanprofile.Observe(ctx, guids)
	return NativeLANObservation{Interfaces: interfaces, Profiles: profiles}, err
}

type lanOptionsTarget struct{ id, name string }
type lanOptionsSource struct {
	verify      func(context.Context) error
	targets     func(context.Context) ([]lanOptionsTarget, error)
	publication func(context.Context, string) ([]byte, error)
	native      func(context.Context) (NativeLANObservation, error)
	helperReady bool
}

func (runtime *ServiceRuntime) lanOptionsSource() lanOptionsSource {
	cli, daemon := runtime.opts.Client, runtime.opts.Ledger.daemon
	helperReady := false
	if runtime.opts.LAN != nil && runtime.opts.LAN.Helper != nil && fullID(runtime.opts.LAN.WorkerSHA256) {
		_, helperReady = runtime.opts.LAN.Helper.(lanReviewPersistence)
	}
	return lanOptionsSource{
		verify: func(ctx context.Context) error {
			info, err := cli.Info(ctx, client.InfoOptions{})
			if err != nil || info.Info.ID != daemon || info.Info.OSType != "linux" {
				return errors.New("owned publication source identity is unconfirmed")
			}
			return nil
		},
		targets: func(ctx context.Context) ([]lanOptionsTarget, error) {
			listed, err := cli.ContainerList(ctx, client.ContainerListOptions{All: false, Limit: maxLANOptionsContainers + 1})
			if err != nil || len(listed.Items) > maxLANOptionsContainers {
				return nil, errors.New("LAN publication inventory unavailable or over budget")
			}
			result := make([]lanOptionsTarget, 0, len(listed.Items))
			seen := map[string]bool{}
			for _, item := range listed.Items {
				if !fullID(item.ID) || seen[item.ID] {
					return nil, errors.New("LAN publication inventory identity differs")
				}
				seen[item.ID] = true
				name := item.ID[:12]
				if len(item.Names) > 0 {
					name = strings.TrimPrefix(item.Names[0], "/")
				}
				if len(name) > 256 || strings.ContainsAny(name, "\x00\r\n") {
					return nil, errors.New("LAN workload display name exceeds bounds")
				}
				result = append(result, lanOptionsTarget{item.ID, name})
			}
			return result, nil
		},
		publication: func(ctx context.Context, id string) ([]byte, error) { return readLANPublication(ctx, cli, daemon, id) },
		native: func(ctx context.Context) (NativeLANObservation, error) {
			if runtime.opts.coldLANLeaf != nil {
				return readColdNativeLANChild(ctx, runtime.opts.coldLANLeaf)
			}
			return readNativeLANChild(ctx, runtime.opts.BinarySHA256)
		},
		helperReady: helperReady,
	}
}

func collectLANOptions(ctx context.Context, source lanOptionsSource, now func() time.Time) (LANOptions, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	start := now()
	result := LANOptions{Version: 1, ReadAt: start.UTC().Format(time.RFC3339Nano), ExpiresAt: start.Add(5 * time.Second).UTC().Format(time.RFC3339Nano), Complete: true,
		HelperAvailable: source.helperReady, Publications: []LANPublicationChoice{}, Interfaces: []LANInterfaceChoice{}}
	if !source.helperReady {
		result.HelperUnavailableReason = "helper-unregistered"
	}
	oldest := start
	readNative, nativeErr := source.native(ctx)
	if nativeErr == nil {
		result.Interfaces, nativeErr = buildLANInterfaceChoices(readNative, now())
		for _, raw := range []string{readNative.Interfaces.ReadAt, readNative.Profiles.ReadAt} {
			stamp, err := time.Parse(time.RFC3339Nano, raw)
			if err != nil || stamp.IsZero() || now().Before(stamp) || now().Sub(stamp) > 5*time.Second {
				nativeErr = errors.New("LAN native observation expired")
			} else if stamp.Before(oldest) {
				oldest = stamp
			}
		}
	}
	if nativeErr != nil {
		result.Complete = false
		result.Interfaces = []LANInterfaceChoice{}
	}
	if err := source.verify(ctx); err != nil {
		result.Complete, result.SourceUnavailableReason = false, "publication-source-unconfirmed"
		return finalizeLANOptions(result, oldest, now()), err
	}
	targets, err := source.targets(ctx)
	if err != nil || len(targets) > maxLANOptionsContainers {
		result.Complete, result.SourceUnavailableReason = false, "publication-source-unconfirmed"
		return finalizeLANOptions(result, oldest, now()), errors.New("LAN publication inventory unavailable or over budget")
	}
	for _, target := range targets {
		data, err := source.publication(ctx, target.id)
		if err != nil {
			result.Complete = false
			if errors.Is(err, errLANPublicationUnavailable) {
				result.SourceUnavailableReason = "publication-v2-unavailable"
			} else if result.SourceUnavailableReason == "" {
				result.SourceUnavailableReason = "publication-source-unconfirmed"
			}
			continue
		}
		snapshot, stamp, err := lanhelper.ParsePublication(data, target.id, now())
		if err != nil {
			result.Complete = false
			result.SourceUnavailableReason = "publication-source-unconfirmed"
			continue
		}
		if stamp.Before(oldest) {
			oldest = stamp
		}
		for _, binding := range snapshot.Publications {
			if len(result.Publications) >= maxLANOptionsPublications {
				result.Complete, result.SourceUnavailableReason = false, "publication-source-unconfirmed"
				break
			}
			choice := LANPublicationChoice{ContainerID: target.id, ContainerName: target.name, StartedAt: snapshot.StartedAt, HostPort: binding.HostPort, ContainerPort: binding.ContainerPort, Protocol: binding.Protocol, Available: snapshot.Running && binding.Published}
			if !choice.Available {
				choice.AvailabilityReason = "publication-inactive"
			}
			result.Publications = append(result.Publications, choice)
		}
	}
	if err = source.verify(ctx); err != nil || ctx.Err() != nil {
		result.Complete, result.SourceUnavailableReason = false, "publication-source-unconfirmed"
	}
	result.SourceAvailable = result.SourceUnavailableReason == "" && len(result.Publications) > 0
	if len(result.Publications) == 0 && result.SourceUnavailableReason == "" {
		result.SourceUnavailableReason = "no-published-services"
	}
	return finalizeLANOptions(result, oldest, now()), errors.Join(nativeErr, err, ctx.Err())
}

func finalizeLANOptions(result LANOptions, oldest, now time.Time) LANOptions {
	result.ReadAt, result.ExpiresAt = oldest.UTC().Format(time.RFC3339Nano), oldest.Add(5*time.Second).UTC().Format(time.RFC3339Nano)
	if !now.Before(oldest.Add(5 * time.Second)) {
		result.Complete, result.SourceAvailable = false, false
		result.SourceUnavailableReason = "observation-expired"
	}
	if !result.Complete {
		for i := range result.Publications {
			result.Publications[i].Available, result.Publications[i].AvailabilityReason = false, "publication-source-unconfirmed"
		}
		for i := range result.Interfaces {
			result.Interfaces[i].Available, result.Interfaces[i].AvailabilityReason = false, "interface-unconfirmed"
		}
	}
	sort.Slice(result.Publications, func(i, j int) bool {
		a, b := result.Publications[i], result.Publications[j]
		if a.ContainerID != b.ContainerID {
			return a.ContainerID < b.ContainerID
		}
		if a.Protocol != b.Protocol {
			return a.Protocol < b.Protocol
		}
		return a.HostPort < b.HostPort
	})
	return result
}

func buildLANInterfaceChoices(observation NativeLANObservation, now time.Time) ([]LANInterfaceChoice, error) {
	interfaces, profiles := observation.Interfaces, observation.Profiles
	if interfaces.Version != 1 || interfaces.Source != "windows-ip-helper" || profiles.Version != 1 || profiles.Source != "windows-network-list-manager" || !profiles.Complete || len(interfaces.Adapters) > lanobserve.MaxAdapters || len(profiles.Profiles) > lanprofile.MaxAdapters {
		return nil, errors.New("complete bounded native interface/profile evidence required")
	}
	result := make([]LANInterfaceChoice, 0, len(interfaces.Adapters))
	for _, adapter := range interfaces.Adapters {
		choice := LANInterfaceChoice{InterfaceGUID: adapter.GUID, InterfaceLUID: adapter.LUID, Alias: adapter.Alias, Profile: "unknown", PrivateAddresses: []string{}, AvailabilityReason: "interface-unconfirmed"}
		for _, profile := range profiles.Profiles {
			if profile.AdapterGUID == adapter.GUID && profile.State == "observed" && profile.ConnectedRecords == 1 && (profile.Category == "private" || profile.Category == "domain" || profile.Category == "public") {
				choice.Profile = profile.Category
			}
		}
		eligible := []string{}
		for _, address := range adapter.Addresses {
			ip, err := netip.ParseAddr(address.Address)
			if err != nil || !ip.Is4() || !ip.IsPrivate() || ip.String() != address.Address || address.DADState != 4 {
				continue
			}
			choice.PrivateAddresses = append(choice.PrivateAddresses, address.Address)
			scope := lanfirewall.Scope{Protocol: "tcp", LocalAddress: address.Address, LocalPort: 1, InterfaceGUID: adapter.GUID, InterfaceLUID: adapter.LUID, Profile: choice.Profile,
				ClientNetworks: []string{ip.String() + "/32"}, WorkerSHA256: strings.Repeat("0", 64)}
			if _, err := lanfirewall.BindInterface(scope, interfaces, profiles, now); err == nil {
				eligible = append(eligible, address.Address)
				choice.Available, choice.AvailabilityReason = true, ""
			}
		}
		if choice.Available {
			choice.PrivateAddresses = eligible
		}
		if choice.Profile == "public" {
			choice.Available, choice.AvailabilityReason = false, "public-profile-refused"
		}
		sort.Strings(choice.PrivateAddresses)
		result = append(result, choice)
	}
	return result, nil
}
