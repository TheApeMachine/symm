package ui

import "testing"

func TestEventIsCritical(t *testing.T) {
	if !eventIsCritical("trade_exit") {
		t.Fatal("trade_exit should be critical")
	}

	if !eventIsCritical("status") {
		t.Fatal("status should be critical")
	}

	if eventIsCritical("price_tick") {
		t.Fatal("price_tick should not be critical")
	}
}
