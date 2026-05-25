package logic

import "testing"

func TestOr(t *testing.T) {
	if Or(1, 2, true) != 1 {
		t.Fatal("expected first value when true")
	}

	if Or(1, 2, false) != 2 {
		t.Fatal("expected second value when false")
	}
}
