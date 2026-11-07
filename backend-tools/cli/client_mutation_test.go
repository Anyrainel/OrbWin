package main

import (
	"errors"
	"reflect"
	"testing"
)

func TestPrivateMutationPublicationAndCleanupFaultSuffix(t *testing.T) {
	for _, shape := range [][]string{
		{"rename", "close-stage", "reopen", "native-id-sd", "body", "old-name-absent"},
		{"disposition", "close-delete-cap", "name-absent"},
	} {
		for fault := -1; fault < len(shape); fault++ {
			state := &privateMutationState{}
			calls, quarantined := []string{}, false
			e := checkedPrivateMutation(state, func() error {
				if !state.admissible() {
					t.Fatal("pre-effect admission already armed")
				}
				calls = append(calls, "admit")
				return nil
			}, func() error {
				if state.admissible() {
					t.Fatal("native call before actual owner armed")
				}
				calls = append(calls, shape[0])
				if fault == 0 {
					return errors.New("native result unknown")
				}
				return nil
			}, func() error {
				for i, step := range shape[1:] {
					if state.admissible() {
						t.Fatal("suffix lost live attempt")
					}
					calls = append(calls, step)
					if fault == i+1 {
						return errors.New("checked suffix unknown")
					}
				}
				return nil
			}, func() error { quarantined = true; calls = append(calls, "fence"); return errClientConfigRecovery })
			if fault < 0 {
				if e != nil || quarantined || !state.admissible() {
					t.Fatalf("verified success: %v", e)
				}
				if !reflect.DeepEqual(calls, append([]string{"admit"}, shape...)) {
					t.Fatal(calls)
				}
			} else {
				if e == nil || !quarantined || state.admissible() {
					t.Fatal("unknown released admission")
				}
				want := append(append([]string{"admit"}, shape[:fault+1]...), "fence")
				if !reflect.DeepEqual(calls, want) {
					t.Fatalf("follow-on suffix ran: %v vs %v", calls, want)
				}
				if checkedPrivateMutation(state, func() error { t.Fatal("retried unknown owner"); return nil }, func() error { t.Fatal("retried native call"); return nil }, func() error { t.Fatal("retried suffix"); return nil }, func() error { quarantined = true; return errClientConfigRecovery }) == nil {
					t.Fatal("unknown attempt became success")
				}
			}
		}
	}
}

func TestPrivateMutationPreEffectRefusalAndPanic(t *testing.T) {
	state := &privateMutationState{}
	if checkedPrivateMutation(state, func() error { return errors.New("known pre-effect refusal") }, func() error { t.Fatal("native effect after refusal"); return nil }, func() error { t.Fatal("suffix after refusal"); return nil }, func() error { t.Fatal("invented mutation uncertainty"); return nil }) == nil || !state.admissible() {
		t.Fatal("pre-effect refusal armed owner")
	}
	for _, at := range []string{"native", "close", "reopen", "identity", "body", "absence"} {
		t.Run(at, func(t *testing.T) {
			state := &privateMutationState{}
			quarantined := false
			func() {
				defer func() {
					if recover() == nil {
						t.Fatal("panic hidden as success")
					}
				}()
				_ = checkedPrivateMutation(state, func() error { return nil }, func() error {
					if at == "native" {
						panic("native call lost")
					}
					return nil
				}, func() error { panic(at) }, func() error { quarantined = true; return errClientConfigRecovery })
			}()
			if !quarantined || state.admissible() {
				t.Fatal("panic released actual mutation attempt")
			}
		})
	}
	state = &privateMutationState{}
	if checkedPrivateMutation(state, func() error { return nil }, func() error { return errors.New("unknown") }, func() error { t.Fatal("suffix after unknown"); return nil }, func() error { return nil }) == nil || state.admissible() {
		t.Fatal("bad fence callback converted unknown to success")
	}
}
