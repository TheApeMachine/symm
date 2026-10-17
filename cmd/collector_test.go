package cmd

import "testing"

func TestTheCollectorDoesNotPaperTrade(t *testing.T) {
	if paperTrades(true, false) {
		t.Fatal("the collector places paper orders")
	}

	if !paperTrades(false, true) {
		t.Fatal("the dashboard does not place paper orders")
	}
}
