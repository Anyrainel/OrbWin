//go:build windows

package main

import (
	"golang.org/x/sys/windows"
	"testing"
)

func TestDirectoryShareAccessParticipatesWithoutListing(t *testing.T) {
	// MS-FSA sharing checks only consider these data/execute/delete access bits.
	// Metadata READ_CONTROL/READ_ATTRIBUTES/SYNCHRONIZE alone cannot exclude a
	// DELETE open. FILE_TRAVERSE shares FILE_EXECUTE's value for directories.
	participates := uint32(windows.FILE_READ_DATA | windows.FILE_EXECUTE | windows.FILE_WRITE_DATA | windows.FILE_APPEND_DATA | windows.DELETE)
	for _, base := range []uint32{windows.READ_CONTROL | windows.FILE_READ_ATTRIBUTES, windows.READ_CONTROL | windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE} {
		if base&participates != 0 {
			t.Fatal("metadata fixture already participates")
		}
		actual := directoryShareAccess(base)
		if actual&participates != windows.FILE_EXECUTE || actual&windows.FILE_TRAVERSE == 0 {
			t.Fatal("directory open still cannot participate in sharing")
		}
		if actual&windows.FILE_LIST_DIRECTORY != 0 || actual&(windows.FILE_WRITE_DATA|windows.FILE_APPEND_DATA|windows.DELETE) != 0 {
			t.Fatal("directory retention widened to listing or mutation")
		}
		if actual & ^uint32(windows.FILE_TRAVERSE) != base {
			t.Fatal("unrelated directory access mask changed")
		}
	}
	// The snapshot enumerator already needs LIST_DIRECTORY (READ_DATA alias).
	// Its existing mask participates independently; no ordinary ancestry needs
	// listing authority to achieve the same sharing participation.
	snapshot := uint32(windows.READ_CONTROL | windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE | windows.FILE_LIST_DIRECTORY)
	if snapshot&participates == 0 {
		t.Fatal("snapshot enumeration mask unexpectedly metadata-only")
	}
}
