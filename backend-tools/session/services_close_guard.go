package main

// Called ONLY by the actual producer's sole cleanup defer, whose direct recover
// observes body panic before any resource-close defer can run.
func finishServiceStartup(bodyPanic any, root, client, self func() error, keep func()) error {
	if bodyPanic != nil {
		keep()
		panic("local-services keeper returned")
	}
	for _, close := range []func() error{root, client, self} {
		if err := closeServiceStartupResource(close, keep); err != nil {
			return err
		}
	}
	return nil
}

// A keeper is an internal fixed producer operation, never a permission callback.
// Positive return is possible only after close itself positively returned.
func closeServiceStartupResource(close func() error, keep func()) (result error) {
	known, retaining := false, false
	defer func() {
		if !known && !retaining {
			retaining = true
			keep()
			panic("local-services keeper returned")
		}
	}()
	result = close()
	if result != nil {
		retaining = true
		keep()
		panic("local-services keeper returned")
	}
	known = true
	return nil
}
