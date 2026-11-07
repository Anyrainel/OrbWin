// SPDX-License-Identifier: Apache-2.0
// OrbWin adaptation: defer the Windows host home until native environment projection.

package limayaml

import (
	"os"
	"runtime"
)

// Preserve upstream's eager cached home on other hosts. A Windows fixed-role
// process cannot use inherited USERPROFILE before its native setup admission.
func initialHostHome() string {
	if runtime.GOOS == "windows" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return home
}

// Ordinary template DATA only. The owning Go service projects the Windows
// environment before calling the actual template evaluator. Failure is an
// error, never an inherited fallback, cached value or native capability.
func hostTemplateHome(legacy string) (string, error) {
	if runtime.GOOS == "windows" {
		return os.UserHomeDir()
	}
	return legacy, nil
}
