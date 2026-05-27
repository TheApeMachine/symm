package engine

import "testing"

func TestValidExit(t *testing.T) {
	if ValidExit(Exit{}) {
		t.Fatal("expected empty exit to be rejected")
	}

	if !ValidExit(Exit{
		Symbol:  "BTC/EUR",
		Urgency: 0.9,
		Reason:  "book_thinning",
	}) {
		t.Fatal("expected complete exit to pass")
	}
}
