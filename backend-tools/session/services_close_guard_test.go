package main

import (
	"errors"
	"testing"
)

func TestProducerDeferObservesPanicBeforeClose(t *testing.T) {
	for _, scenario := range []string{"body-panic", "root-panic", "root-error", "client-panic", "client-error", "positive"} {
		t.Run(scenario, func(t *testing.T) {
			original := &struct {
				closes [3]int
				suffix int
			}{}
			kept := 0
			closeRole := func(role int) func() error {
				return func() error {
					original.closes[role]++
					if scenario == []string{"root-panic", "client-panic", "self-panic"}[role] {
						panic("controlled close")
					}
					if scenario == []string{"root-error", "client-error", "self-error"}[role] {
						return errors.New("controlled close")
					}
					return nil
				}
			}
			func() {
				defer func() {
					if recovered := recover(); recovered != nil && recovered != "keeper" {
						t.Fatalf("unexpected panic: %v", recovered)
					}
				}()
				func() {
					// Same real Go defer/recover order as the production function.
					defer func() {
						_ = finishServiceStartup(recover(), closeRole(0), closeRole(1), closeRole(2), func() { kept++; panic("keeper") })
					}()
					if scenario == "body-panic" {
						panic("body")
					}
				}()
				original.suffix++
			}()
			want := [3]int{1, 1, 1}
			switch scenario {
			case "body-panic":
				want = [3]int{}
			case "root-panic", "root-error":
				want = [3]int{1, 0, 0}
			case "client-panic", "client-error":
				want = [3]int{1, 1, 0}
			}
			if original.closes != want {
				t.Fatalf("original close counts %v want %v", original.closes, want)
			}
			if scenario == "positive" {
				if kept != 0 || original.suffix != 1 {
					t.Fatal("positive cleanup")
				}
			} else if kept != 1 || original.suffix != 0 {
				t.Fatal("keeper or dependent suffix", kept, original.suffix)
			}
		})
	}
}

func TestStartupCloseStopsSuffixAndRetainsOriginal(t *testing.T) {
	for _, panicClose := range []bool{false, true} {
		t.Run(map[bool]string{false: "error", true: "panic"}[panicClose], func(t *testing.T) {
			original := &struct{ prefix, failed, suffix int }{}
			kept := false
			func() {
				defer func() { _ = recover() }()
				original.prefix++ // already cached positive predecessor
				_ = closeServiceStartupResource(func() error {
					original.failed++
					if panicClose {
						panic("controlled")
					}
					return errors.New("controlled close")
				}, func() {
					kept = true
					if original.prefix != 1 || original.failed != 1 || original.suffix != 0 {
						t.Fatal("original partial graph lost")
					}
					panic("controlled keeper")
				})
				original.suffix++
			}()
			if !kept || original.failed != 1 || original.suffix != 0 {
				t.Fatal("cleanup suffix ran")
			}
		})
	}
	closed := 0
	if closeServiceStartupResource(func() error { closed++; return nil }, func() { t.Fatal("positive close retained") }) != nil || closed != 1 {
		t.Fatal("positive close")
	}
}
