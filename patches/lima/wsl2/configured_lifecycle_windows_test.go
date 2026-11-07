// SPDX-FileCopyrightText: Copyright The Lima Authors
// SPDX-License-Identifier: Apache-2.0
//go:build windows

package wsl2

import (
	"github.com/lima-vm/lima/v2/pkg/limatype"
	"testing"
)

// Compile-only during native pause: importing this package runs the selected
// upstream WSL registration initializer, even though this body is inert.
func TestConfiguredLifecycleDoesNotMutatePrototype(t *testing.T) {
	prototype := &LimaWslDriver{vSockPort: 42, virtioPort: "fixed"}
	a, b := &limatype.Instance{SSHLocalPort: 100}, &limatype.Instance{SSHLocalPort: 200}
	first, second := prototype.configuredFor(a), prototype.configuredFor(b)
	first.startEntered = true
	first.workers.seal()
	if prototype.Instance != nil || prototype.startEntered || first == second || first.Instance != a || second.Instance != b || second.startEntered || second.workers.stopping || first.vSockPort != 42 || second.SSHLocalPort != 200 {
		t.Fatal("prototype/live state copied")
	}
}
