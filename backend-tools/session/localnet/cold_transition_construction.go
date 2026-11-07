package localnet

import (
	"errors"
	"sync"
)

// The actual NewServiceRuntime call result, not an inferred nil holder. Every
// nil/error return in that callee has no remaining native registration owner.
// A nonnil/error partial registration has no actor but refuses this witness.
// The cold caller still seals issuers and
// checks ledger Finish to settle a possible partial state guard from LoadSettings.
type coldRuntimeConstruction struct {
	mu                   sync.Mutex
	attempted, completed bool
	runtime              *ServiceRuntime
	err                  error
}

func (c *coldRuntimeConstruction) construct(options RuntimeOptions) (*ServiceRuntime, error) {
	c.mu.Lock()
	if c.attempted {
		c.mu.Unlock()
		return nil, errors.New("cold runtime construction is one use")
	}
	c.attempted = true
	c.mu.Unlock()
	runtime, err := NewServiceRuntime(options) // actual callee, no replaceable callback
	c.mu.Lock()
	c.runtime, c.err, c.completed = runtime, err, true
	c.mu.Unlock()
	return runtime, err
}
func (c *coldRuntimeConstruction) failedBeforeActor() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.attempted && c.completed && c.runtime == nil && c.err != nil
}
