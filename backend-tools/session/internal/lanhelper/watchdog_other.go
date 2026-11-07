//go:build !windows

package lanhelper

func StartReadOnlyObservationCap() func() bool { return func() bool { return true } }
