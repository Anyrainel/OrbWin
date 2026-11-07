// SPDX-FileCopyrightText: Copyright The Lima Authors
// SPDX-License-Identifier: Apache-2.0

package wsl2

import (
	"path/filepath"

	"github.com/lima-vm/lima/v2/pkg/limatype/filenames"
)

// wslImportArgs selects WSL 2 for this new distribution without changing the host default.
func wslImportArgs(instanceDir, distroName string) []string {
	return []string{
		"wsl.exe",
		"--import",
		distroName,
		instanceDir,
		filepath.Join(instanceDir, filenames.BaseDiskLegacy),
		"--version",
		"2",
	}
}
