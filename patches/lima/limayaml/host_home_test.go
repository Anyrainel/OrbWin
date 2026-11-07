// SPDX-License-Identifier: Apache-2.0
// OrbWin regression: only stdlib environment DATA, no Lima/native initializers.

package limayaml

import (
	"runtime"
	"testing"
)

func TestHostHomeWindowsDefersUntilProjectionAndDoesNotCache(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows environment contract")
	}
	t.Setenv("USERPROFILE", "")
	if initialHostHome() != "" {
		t.Fatal("Windows eager home captured")
	}
	if _, err := hostTemplateHome("stale inherited home"); err == nil {
		t.Fatal("missing projection silently used legacy value")
	}
	for _, home := range []string{`C:\owned-one\home`, `D:\owned-two\home`} {
		t.Setenv("USERPROFILE", home)
		actual, err := hostTemplateHome("stale inherited home")
		if err != nil || actual != home {
			t.Fatal("current projection was not observed", actual, err)
		}
		if initialHostHome() != "" {
			t.Fatal("Windows eager path queried projected environment")
		}
	}
	t.Setenv("USERPROFILE", "")
	if _, err := hostTemplateHome("previous complete value"); err == nil {
		t.Fatal("removed projection reused a previous home")
	}
}

func TestHostHomeOtherHostsKeepEagerCache(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-Windows upstream behavior")
	}
	t.Setenv("HOME", "/ordinary-initial-home")
	cached := initialHostHome()
	t.Setenv("HOME", "/later-host-home")
	actual, err := hostTemplateHome(cached)
	if err != nil || actual != "/ordinary-initial-home" {
		t.Fatal("upstream cached home behavior changed", actual, err)
	}
}
