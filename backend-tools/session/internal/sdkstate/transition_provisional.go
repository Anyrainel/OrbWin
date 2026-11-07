package sdkstate

// Correlation for an actual process handle duplicated by the TEMP native
// launcher into this SDK before the selected candidate's first Resume. This
// record alone is never a supervisor, native handle constructor or grant.
type ColdSupervisorRecord struct {
	Process  uint64
	PID      uint32
	Creation uint64
	Session  uint32
	Image    [32]byte
}

// A query process handle in the actual provisional Rdd master's table. Only
// the native transition/control adopter may consume it; this data is no grant.
type ColdLeafMasterRecord struct{ Process uint64 }

func (r ColdSupervisorRecord) Valid() bool {
	return r.Process != 0 && r.Process != uint64(^uintptr(0)) && uint64(uintptr(r.Process)) == r.Process && r.PID != 0 && r.Creation != 0 && r.Session != 0 && r.Image != [32]byte{}
}
