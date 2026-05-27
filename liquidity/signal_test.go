package liquidity

import (
	"context"
	"sync"
	"testing"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/numeric/learned"
)

func storeLiquiditySymbol(liquidity *Liquidity, symbol string, state *symbolState) {
	liquidity.symbols.Store(symbol, state)
}

func newTestLiquidity() *Liquidity {
	return &Liquidity{
		symbols:     sync.Map{},
		requested:   sync.Map{},
		belowMedian: adaptive.NewBelowMedian(),
		peak:        adaptive.NewPeak(),
	}
}

func TestLiquidityMeasure(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewLiquidity(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	storeLiquiditySymbol(signal, "LOW/EUR", &symbolState{
		pair:          asset.Pair{Wsname: "LOW/EUR"},
		dailyQuoteVol: 100,
		forecast:      learned.NewForecast(0),
	})
	storeLiquiditySymbol(signal, "MID/EUR", &symbolState{
		pair:          asset.Pair{Wsname: "MID/EUR"},
		dailyQuoteVol: 200,
		forecast:      learned.NewForecast(0),
	})
	storeLiquiditySymbol(signal, "HIGH/EUR", &symbolState{
		pair:          asset.Pair{Wsname: "HIGH/EUR"},
		dailyQuoteVol: 300,
		forecast:      learned.NewForecast(0),
	})

	lowFound := false
	highFound := false

	for measurement := range signal.Measure() {
		if measurement.Pairs[0].Wsname == "LOW/EUR" {
			lowFound = true
		}

		if measurement.Pairs[0].Wsname == "HIGH/EUR" {
			highFound = true
		}

		if measurement.Source != liquiditySource || measurement.Confidence <= 0 ||
			measurement.Confidence > 1 {
			t.Fatalf("unexpected measurement: %+v", measurement)
		}
	}

	if !lowFound {
		t.Fatal("expected low-volume symbol to measure")
	}

	if highFound {
		t.Fatal("expected high-volume symbol to be excluded")
	}
}

func TestLiquidityMeasureMinPeerGuard(t *testing.T) {
	signal := newTestLiquidity()

	storeLiquiditySymbol(signal, "LOW/EUR", &symbolState{
		pair: asset.Pair{Wsname: "LOW/EUR"}, dailyQuoteVol: 100, forecast: learned.NewForecast(0),
	})
	storeLiquiditySymbol(signal, "MID/EUR", &symbolState{
		pair: asset.Pair{Wsname: "MID/EUR"}, dailyQuoteVol: 200, forecast: learned.NewForecast(0),
	})

	for range signal.Measure() {
		t.Fatal("expected min peer guard to fail with one peer")
	}
}

func BenchmarkLiquidityMeasure(b *testing.B) {
	signal := newTestLiquidity()

	for index, symbol := range []string{"A/EUR", "B/EUR", "C/EUR", "D/EUR"} {
		storeLiquiditySymbol(signal, symbol, &symbolState{
			pair:          asset.Pair{Wsname: symbol},
			dailyQuoteVol: 100 + float64(index)*50,
			forecast:      learned.NewForecast(0),
		})
	}

	b.ReportAllocs()

	for b.Loop() {
		for range signal.Measure() {
		}
	}
}
