// SPDX-FileCopyrightText: Copyright The Lima Authors
// SPDX-License-Identifier: Apache-2.0

package wsl2

import (
	"slices"
	"testing"
)

func TestWSLImportArgs(t *testing.T) {
	tests := []struct {
		name        string
		instanceDir string
		distroName  string
		want        []string
	}{
		{
			name:        "owned distribution",
			instanceDir: `C:\Users\test\.rdorbwin\lima\orbwin`,
			distroName:  "lima-orbwin",
			want: []string{
				"wsl.exe", "--import", "lima-orbwin",
				`C:\Users\test\.rdorbwin\lima\orbwin`,
				`C:\Users\test\.rdorbwin\lima\orbwin\basedisk`,
				"--version", "2",
			},
		},
		{
			name:        "spaces and Unicode remain single arguments",
			instanceDir: `D:\OrbWin 用户 Data\.rdorbwin\lima\orbwin`,
			distroName:  "lima-orbwin-test",
			want: []string{
				"wsl.exe", "--import", "lima-orbwin-test",
				`D:\OrbWin 用户 Data\.rdorbwin\lima\orbwin`,
				`D:\OrbWin 用户 Data\.rdorbwin\lima\orbwin\basedisk`,
				"--version", "2",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wslImportArgs(tt.instanceDir, tt.distroName); !slices.Equal(got, tt.want) {
				t.Fatalf("WSL import arguments = %#v, want %#v", got, tt.want)
			}
		})
	}
}
