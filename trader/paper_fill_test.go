package trader

import (
	"testing"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
)

func TestPaperSimulatedFillAcceptsPartialCoverage(t *testing.T) {
	config.System.PaperMinFillCoverage = 0.5
	config.System.MinCostEUR = 1

	levels := []market.BookLevel{
		{Price: 100, Volume: 1},
	}

	fill, baseQty, proceeds, err := paperSimulatedFill(
		"buy",
		100,
		0,
		100,
		99.9,
		100.1,
		nil,
		levels,
	)

	if err != nil {
		t.Fatal(err)
	}

	if fill <= 0 || baseQty <= 0 {
		t.Fatalf("expected fill, got fill=%v base=%v", fill, baseQty)
	}

	if proceeds < 50 {
		t.Fatalf("expected partial proceeds >= 50, got %v", proceeds)
	}

	config.System.PaperMinFillCoverage = 1
}

func TestPaperShouldRejectWhenRateSet(t *testing.T) {
	config.System.PaperOrderRejectRate = 1
	defer func() {
		config.System.PaperOrderRejectRate = 0
	}()

	if !paperShouldReject() {
		t.Fatal("expected rejection at rate 1")
	}
}
