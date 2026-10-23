package store

import (
	"context"
	"testing"
)

func TestFinalizeRegionsUsesEachSymbolsMidpoint(t *testing.T) {
	grid := NewGrid(context.Background())
	got := grid.finalizeRegions([]*Region{
		{Symbol: "BBB/USD", Key: 3, Brightness: 0.4},
		{Symbol: "AAA/USD", Key: 0, Brightness: 0.9},
		{Symbol: "BBB/USD", Key: 4, Brightness: 0.2},
		{Symbol: "AAA/USD", Key: 1, Brightness: 0.1},
	})

	if len(got) != 2 {
		t.Fatalf("kept %d regions, want 2 (each symbol's own upper half)", len(got))
	}

	if got[0].Symbol != "AAA/USD" || got[0].Key != 0 {
		t.Fatalf("first region = %s R%02d, want AAA/USD R00", got[0].Symbol, got[0].Key)
	}

	if got[1].Symbol != "BBB/USD" || got[1].Key != 3 {
		t.Fatalf("second region = %s R%02d, want BBB/USD R03", got[1].Symbol, got[1].Key)
	}
}

func TestSymbolSurpriseIsAbsoluteEnergyError(t *testing.T) {
	grid := NewGrid(context.Background())

	if _, ok := grid.SymbolSurprise("BTC/USD"); ok {
		t.Fatal("a symbol with no prior reported a surprise")
	}

	grid.Priors["book"] = map[string]*Prior{
		"BTC/USD": {Samples: 2, Prediction: 0.4, Energy: 0.1},
	}

	got, ok := grid.SymbolSurprise("BTC/USD")
	if !ok {
		t.Fatal("a prior with samples did not report a surprise")
	}

	if diff := got - 0.3; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("surprise = %v, want |0.1 - 0.4|", got)
	}

	if got < 0 {
		t.Fatal("surprise went negative")
	}

	grid.Priors["book"]["BTC/USD"] = &Prior{Samples: 1, Prediction: 0, Energy: 0.5}

	got, ok = grid.SymbolSurprise("BTC/USD")
	if !ok || got != 0.5 {
		t.Fatalf("empty prediction surprise = %v, %v; want the energy 0.5", got, ok)
	}
}

func TestRegionSide(t *testing.T) {
	if RegionSide(0) != "enter" || RegionSide(1) != "enter" {
		t.Fatal("R00 and R01 vote for enter")
	}

	if side, with := RegionVote(2); side != "enter" || with {
		t.Fatal("R02 is enter opposition and votes against enter")
	}

	if RegionSide(2) != "" {
		t.Fatal("R02 does not cast an enter vote")
	}

	if RegionSide(3) != "exit" || RegionSide(4) != "exit" {
		t.Fatal("R03 and R04 vote for exit")
	}

	if side, with := RegionVote(5); side != "exit" || with {
		t.Fatal("R05 is exit opposition and votes against exit")
	}

	if RegionSide(7) != "" {
		t.Fatal("an oracle region casts no side vote")
	}
}
