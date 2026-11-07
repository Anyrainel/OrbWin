package main

import (
	"errors"
	"testing"
)

func TestPreDispatchClientCleanupBoundedAndUnknownLatched(t *testing.T) {
	var owned clientOwnership
	count := 0
	for i := 0; i < 100; i++ {
		if !owned.admissible() {
			t.Fatal("known failed-spawn cleanup leaked")
		}
		owned.retain(1)
		if e := owned.close(1, func() error { count++; return nil }); e != nil {
			t.Fatal(e)
		}
	}
	if count != 100 || len(owned.resources) != 0 {
		t.Fatal(count)
	}
	owned.retain(2)
	if e := owned.close(2, func() error { count++; return errors.New("unknown") }); e == nil {
		t.Fatal("close uncertainty lost")
	}
	if owned.admissible() || !owned.resources[2] {
		t.Fatal("unknown permitted replay")
	}
	_ = owned.close(2, func() error { count++; return nil })
	if count != 101 {
		t.Fatal("unknown close retried")
	}
}
func TestClientResourceBudgetRefusesBeforeDispatch(t *testing.T) {
	var owned clientOwnership
	for i := uintptr(1); i <= 16; i++ {
		if !owned.admissible() {
			t.Fatal(i)
		}
		owned.retain(i)
	}
	if owned.admissible() {
		t.Fatal("unbounded admission")
	}
}
