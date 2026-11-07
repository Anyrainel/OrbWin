package localnet

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"time"

	"orbwin.local/session/internal/sdkstate"
)

// CheckRestrictiveStartup is before ledger/runtime construction. The Root's
// original exclusive owner remains held across positive Guard retirement.
func CheckRestrictiveStartup(ctx context.Context, root *sdkstate.Root, daemon string, mode RestrictiveStartup) (result error) {
	if root == nil || !mode.valid() {
		return errRestricted
	}
	g, err := root.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		known := false
		defer func() {
			if !known {
				retainRestrictionGuard(root, g)
			}
		}()
		closed := g.Close()
		if closed != nil {
			retainRestrictionGuard(root, g)
		}
		known = true
		result = errors.Join(result, closed)
	}()

	names, err := g.List()
	if err != nil {
		return err
	}
	if mode.RequireNew {
		if len(names) != 0 {
			return errRestricted
		}
		return g.Check()
	}
	records := map[string][]byte{}
	for _, n := range names {
		budget := int64(settingsBudget)
		if n == "lan-identity.json" {
			budget = 2048
		} else if n != settingsName {
			return errRestricted
		}
		records[n], err = g.Read(n, budget)
		if err != nil {
			return err
		}
	}
	data := records[settingsName]
	var setting settingsRecord
	if strictJSON(data, &setting) != nil || setting.Version != 1 || setting.Daemon != daemon || setting.Revision == 0 || setting.Settings.Enabled || setting.Settings.validate() != nil {
		return errRestricted
	}
	actual, err := restrictedInventory(records)
	if err != nil || actual != mode.DisabledStateSHA256 {
		return errRestricted
	}
	return g.Check()
}

// Actor-only observation. Native Guard leases live until finishState positively
// settles them. No mutation can be interleaved by the same runtime actor.
func (runtime *ServiceRuntime) restrictedFingerprint() (out string, result error) {
	ledger := runtime.opts.Ledger
	ctx, cancel := context.WithTimeout(runtime.ctx, 5*time.Second)
	defer cancel()
	if err := ledger.lockState(ctx); err != nil {
		return "", err
	}
	defer ledger.finishState(&result)
	if ledger.stateStore == nil {
		return "", errRestricted
	}
	settings, err := ledger.readSettings()
	if err != nil || settings.Settings.Enabled || settings.Revision == 0 {
		return "", errRestricted
	}
	names, err := ledger.stateStore.Names()
	if err != nil {
		return "", err
	}
	records := map[string][]byte{}
	for _, n := range names {
		budget := int64(settingsBudget)
		if n == "lan-identity.json" {
			budget = 2048
		} else if n != settingsName {
			return "", errRestricted
		}
		records[n], err = ledger.readState(filepath.Join(ledger.directory, n), budget)
		if err != nil {
			return "", err
		}
	}
	return restrictedInventory(records)
}

// Own stack retains the actual original guard even if its Close partially ran.
func retainRestrictionGuard(root *sdkstate.Root, g *sdkstate.Guard) {
	fmt.Fprintln(os.Stderr, "Local-services startup state could not be settled; further setup is blocked.")
	for {
		time.Sleep(time.Minute)
		goruntime.KeepAlive(root)
		goruntime.KeepAlive(g)
	}
}
