package fluid

import (
	"context"
	"testing"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/kraken/market"
)

func TestFluidSymbolMeasure(t *testing.T) {
	state := NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"})
	state.bids = []market.BookLevel{{Price: 10, Volume: 80}}
	state.asks = []market.BookLevel{{Price: 10.01, Volume: 20}}
	state.spreadBPS = 10
	state.buyPressure, _ = state.pressure.Next(0, 0.8)

	for range 6 {
		measurement, ok := state.Measure()

		if ok && measurement.Confidence > 0 {
			return
		}
	}

	t.Fatal("expected fluid measurement from imbalanced book")
}

func TestFluidTickAppliesBook(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
	t.Cleanup(func() { pool.Close() })

	signal := NewFluid(ctx, pool)
	t.Cleanup(func() { _ = signal.Close() })

	signal.symbols["ALT/EUR"] = NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"})

	pool.CreateBroadcastGroup("book", 0).Send(&qpool.QValue[any]{
		Value: market.BookLevelsDelta{
			Symbol: "ALT/EUR",
			BidOK:  true,
			AskOK:  true,
			Bids:   []market.BookLevel{{Price: 10, Volume: 50}},
			Asks:   []market.BookLevel{{Price: 10.02, Volume: 40}},
		},
	})

	if err := signal.Tick(); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if len(signal.symbols["ALT/EUR"].bids) != 1 || signal.symbols["ALT/EUR"].spreadBPS <= 0 {
		t.Fatalf("expected book state, got bids=%d spread=%v",
			len(signal.symbols["ALT/EUR"].bids), signal.symbols["ALT/EUR"].spreadBPS)
	}
}

func BenchmarkFluidMeasure(b *testing.B) {
	signal := NewFluid(context.Background(), nil)
	signal.symbols["ALT/EUR"] = NewFluidSymbol(asset.Pair{Wsname: "ALT/EUR"})
	signal.symbols["ALT/EUR"].bids = []market.BookLevel{{Price: 10, Volume: 70}}
	signal.symbols["ALT/EUR"].asks = []market.BookLevel{{Price: 10.01, Volume: 30}}
	signal.symbols["ALT/EUR"].spreadBPS = 10

	b.ReportAllocs()

	for b.Loop() {
		for range signal.Measure() {
		}
	}
}
