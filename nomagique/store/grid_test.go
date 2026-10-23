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
	if RegionSide(0) != "enter" || RegionSide(2) != "enter" {
		t.Fatal("R00-R02 are the entry precursor")
	}

	if RegionSide(3) != "exit" || RegionSide(5) != "exit" {
		t.Fatal("R03-R05 are the exit stretch")
	}

	if RegionSide(7) != "" {
		t.Fatal("a regime region is neither an entry nor an exit")
	}
}
