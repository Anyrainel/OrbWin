package localnet

import (
	"context"
	"errors"

	"orbwin.local/session/internal/sdkstate"
)

const managedStateLabel = "@orbwin-managed-services"

// OpenManagedNamespaceLedger accepts only the native private-root capability.
// The directory label is never an OS path; production record access cannot fall
// through to the explicit plain-filesystem fixture constructor.
func OpenManagedNamespaceLedger(root *sdkstate.Root, daemon string) (*NamespaceLedger, error) {
	if root == nil || daemon == "" {
		return nil, errors.Join(errors.New("native private local-services root required"), sdkstate.ErrState)
	}
	if err := root.Check(); err != nil {
		return nil, err
	}
	return &NamespaceLedger{directory: managedStateLabel, daemon: daemon, stateStore: &managedServiceState{root: root}}, nil
}

type managedServiceState struct {
	root  *sdkstate.Root
	guard *sdkstate.Guard
}

func (s *managedServiceState) Begin(ctx context.Context) error {
	if s.guard != nil {
		return sdkstate.ErrBusy
	}
	g, err := s.root.Begin(ctx)
	s.guard = g
	return err
}
func (s *managedServiceState) Finish() error {
	if s.guard == nil {
		return nil
	}
	err := s.guard.Close()
	if err == nil {
		s.guard = nil
	}
	return err
}
func (s *managedServiceState) Check() error {
	if s.guard != nil {
		return s.guard.Check()
	}
	return s.root.Check()
}
func (s *managedServiceState) Read(name string, budget int64) ([]byte, error) {
	if s.guard == nil {
		return nil, sdkstate.ErrState
	}
	return s.guard.Read(name, budget)
}
func (s *managedServiceState) Create(name string, data []byte) error {
	if s.guard == nil {
		return sdkstate.ErrState
	}
	return s.guard.Create(name, data)
}
func (s *managedServiceState) Append(name string, before, extra []byte) error {
	if s.guard == nil {
		return sdkstate.ErrState
	}
	return s.guard.Append(name, before, extra)
}
func (s *managedServiceState) Replace(name string, before, data []byte) error {
	if s.guard == nil {
		return sdkstate.ErrState
	}
	return s.guard.Replace(name, before, data)
}
func (s *managedServiceState) Delete(name string, before []byte) error {
	if s.guard == nil {
		return sdkstate.ErrState
	}
	return s.guard.Delete(name, before)
}
func (s *managedServiceState) Names() ([]string, error) {
	if s.guard == nil {
		return nil, sdkstate.ErrState
	}
	return s.guard.List()
}
func (s *managedServiceState) Close() error {
	if s.guard != nil {
		if err := s.guard.Close(); err != nil {
			return err
		}
		s.guard = nil
	}
	return s.root.Close()
}
