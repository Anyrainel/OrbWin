package localnet

import (
	"context"
	"errors"
	"time"

	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/lanlease"
)

// Read-only native/publication work owns the same bounded diagnostic query
// slot as options. Only its immutable result is enqueued to the settings actor.
func (runtime *ServiceRuntime) ReviewLAN(ctx context.Context, revision uint64, selection LANSelection) (LANReview, error) {
	var input lanReviewRecord
	err := runtime.readLANQuery(ctx, func(ctx context.Context) error {
		adapters := runtime.opts.LAN
		if adapters == nil || adapters.Helper == nil || !fullID(adapters.WorkerSHA256) {
			return ErrLANAdapterUnavailable
		}
		if _, ok := adapters.Helper.(lanReviewPersistence); !ok {
			return ErrLANAdapterUnavailable
		}
		canonical, err := canonicalLANSelection(selection, adapters.WorkerSHA256)
		if err != nil {
			return err
		}
		source := runtime.lanOptionsSource()
		if err = source.verify(ctx); err != nil {
			return err
		}
		data, err := source.publication(ctx, canonical.ContainerID)
		if err != nil {
			return err
		}
		publication, _, err := lanhelper.ParsePublication(data, canonical.ContainerID, time.Now())
		if err != nil {
			return err
		}
		if !publication.Running || publication.StartedAt != canonical.StartedAt {
			return errors.New("selected workload incarnation changed")
		}
		generation := uint64(0)
		for _, binding := range publication.Publications {
			if binding.Published && binding.Protocol == canonical.Protocol && binding.HostPort == canonical.HostPort && binding.ContainerPort == canonical.ContainerPort {
				generation = binding.Generation
			}
		}
		if generation == 0 {
			return errors.New("exact selected publication is unavailable")
		}
		native, err := source.native(ctx)
		if err != nil {
			return err
		}
		scope := lanfirewall.Scope{Protocol: canonical.Protocol, LocalAddress: canonical.LocalAddress, LocalPort: canonical.HostPort, InterfaceGUID: canonical.InterfaceGUID, InterfaceLUID: canonical.InterfaceLUID, Profile: canonical.Profile, ClientNetworks: canonical.ClientNetworks, WorkerSHA256: adapters.WorkerSHA256}
		if _, err = lanfirewall.BindInterface(scope, native.Interfaces, native.Profiles, time.Now()); err != nil {
			return err
		}
		manager := adapters.Manager
		if manager == nil {
			manager = nativeLANManager
		}
		identity, err := manager(ctx)
		if err != nil {
			return err
		}
		lanInstallation, err := runtime.opts.Ledger.LANInstallationID(runtime.opts.Installation, identity.UserSID)
		if err != nil {
			return err
		}
		if adapters.Installation != "" && adapters.Installation != lanInstallation {
			return errors.New("native helper registration belongs to a different LAN owner namespace")
		}
		transaction, err := lanRandomID()
		if err != nil {
			return err
		}
		nonce, err := lanRandomID()
		if err != nil {
			return err
		}
		nonceTail, err := lanRandomID()
		if err != nil {
			return err
		}
		nonce += nonceTail
		input.review.Selection = canonical
		input.stage = lanhelper.StageRequest{Base: lanfirewall.Request{Version: 1, Operation: "create", Installation: lanInstallation, UserSID: identity.UserSID, Transaction: transaction, Nonce: nonce, CallerPID: identity.PID, CallerCreationTime: identity.CreationTime, CallerImageSHA256: identity.ImageSHA256, Scope: scope}, Publication: lanlease.Publication{OwnerInstance: publication.OwnerInstance, Daemon: runtime.opts.Ledger.daemon, ContainerID: canonical.ContainerID, StartedAt: canonical.StartedAt, Generation: generation, HostPort: canonical.HostPort, ContainerPort: canonical.ContainerPort, Protocol: canonical.Protocol}}
		return source.verify(ctx)
	})
	if err != nil {
		return LANReview{}, err
	}
	reply, err := runtime.request(ctx, runtimeCommand{kind: "lan-owner-review", revision: revision, lanReviewInput: &input})
	if reply.lanReview == nil {
		return LANReview{}, err
	}
	return *reply.lanReview, err
}
