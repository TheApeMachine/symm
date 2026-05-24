package asset

import "testing"

func TestNewPairSetsBaseAndQuote(t *testing.T) {
	pair := NewPair("BTC", "EUR")

	if pair.Base != "BTC" || pair.Quote != "EUR" {
		t.Fatalf("unexpected pair %+v", pair)
	}
}

func TestSymbolPrefersWsname(t *testing.T) {
	pair := Pair{Wsname: "BTC/EUR", Altname: "XBTEUR"}

	if Symbol(pair) != "BTC/EUR" {
		t.Fatalf("expected wsname, got %q", Symbol(pair))
	}
}

func TestSymbolFallsBackToAltname(t *testing.T) {
	pair := Pair{Altname: "XBTEUR"}

	if Symbol(pair) != "XBTEUR" {
		t.Fatalf("expected altname, got %q", Symbol(pair))
	}
}
