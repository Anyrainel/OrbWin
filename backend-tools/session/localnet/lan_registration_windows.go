//go:build windows && amd64

package localnet

import (
	"context"
	"errors"
	"strings"
	"time"

	"orbwin.local/session/internal/lanfirewall"
	"orbwin.local/session/internal/lanhelper"
	"orbwin.local/session/internal/sdkstate"
)

// Only the two actual managed-ledger constructors supply this graph. A plain
// filesystem ledger or interface-shaped fixture cannot register native LAN.
func nativeLANManagedRoot(options RuntimeOptions) (*sdkstate.Root, error) {
	ledger := options.Ledger
	if ledger == nil {
		return nil, ErrLANAdapterUnavailable
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.directory != managedStateLabel || ledger.closed || ledger.closeErr != nil {
		return nil, ErrLANAdapterUnavailable
	}
	var managed *managedServiceState
	switch store := ledger.stateStore.(type) {
	case *managedServiceState:
		if options.coldLANLeaf != nil {
			return nil, ErrLANAdapterUnavailable
		}
		managed = store
	case *coldManagedServiceState:
		if store == nil || store.closed {
			return nil, ErrLANAdapterUnavailable
		}
		managed, _ = store.serviceStateStore.(*managedServiceState)
		if managed == nil || options.coldLANLeaf == nil || options.coldLANLeaf.RootBacking() != managed.root {
			return nil, ErrLANAdapterUnavailable
		}
	default:
		return nil, ErrLANAdapterUnavailable
	}
	if managed == nil || managed.root == nil {
		return nil, ErrLANAdapterUnavailable
	}
	return managed.root, nil
}

func validateNativeLANRegistration(options RuntimeOptions) error {
	if options.LAN != nil || options.SelfIdentity == nil || !options.SelfIdentity.Valid() || options.SelfIdentity.ImageSHA256 != options.BinarySHA256 {
		return ErrLANAdapterUnavailable
	}
	installation := options.Installation
	if !validLANID(strings.ReplaceAll(installation, "-", "")) || (len(installation) != 32 && !(len(installation) == 36 && installation[8] == '-' && installation[13] == '-' && installation[18] == '-' && installation[23] == '-')) {
		return ErrLANAdapterUnavailable
	}
	root, err := nativeLANManagedRoot(options)
	if err != nil {
		return err
	}
	return root.Check() // Outside ledger.mu; LANInstallationID owns its own guard.
}

type nativeLANRegistrationClient interface {
	LANAdministrativeHelper
	ManagerIdentity() lanfirewall.CallerIdentity
	Installation() string
	WorkerSHA256() string
}

// Pure result settlement; the actual constructor below is not replaceable.
// A partial client's getters are deliberately never read on an error result.
func settleNativeLANRegistration(options RuntimeOptions, manager lanfirewall.CallerIdentity, installation string, helper nativeLANRegistrationClient, constructionErr error) (*LANAdapters, error) {
	if helper == nil {
		return nil, errors.Join(ErrLANAdapterUnavailable, constructionErr)
	}
	adapters := &LANAdapters{Installation: installation, Helper: helper}
	if constructionErr != nil {
		return adapters, constructionErr
	}
	if options.SelfIdentity == nil || manager.PID != options.SelfIdentity.PID || manager.CreationTime != options.SelfIdentity.CreationTime || manager.ImageSHA256 != options.BinarySHA256 || helper.ManagerIdentity() != manager || helper.Installation() != installation || !lanhelper.ValidSHA(helper.WorkerSHA256()) {
		failure := errors.New("actual native LAN registration differs from retained SDK identity")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if closeErr := helper.Close(ctx); closeErr != nil {
			return adapters, errors.Join(failure, closeErr)
		}
		return nil, failure
	}
	adapters.WorkerSHA256 = helper.WorkerSHA256()
	return adapters, nil
}

func newNativeLANRegistration(options RuntimeOptions) (*LANAdapters, error) {
	if err := validateNativeLANRegistration(options); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	manager, err := nativeLANManager(ctx)
	if err != nil {
		return nil, err
	}
	if manager.PID != options.SelfIdentity.PID || manager.CreationTime != options.SelfIdentity.CreationTime || manager.ImageSHA256 != options.BinarySHA256 {
		return nil, errors.New("native ordinary manager differs from the retained SDK")
	}
	// May initialize the same private denial/info record. It does not elevate,
	// launch a helper, register a provider rule or grant authority from that file.
	installation, err := options.Ledger.LANInstallationID(options.Installation, manager.UserSID)
	if err != nil {
		return nil, err
	}
	helper, err := lanhelper.NewAdministrativeClient(installation, options.BinarySHA256)
	if helper == nil {
		return nil, err
	} // Avoid a typed-nil interface as an owner.
	return settleNativeLANRegistration(options, manager, installation, helper, err)
}
