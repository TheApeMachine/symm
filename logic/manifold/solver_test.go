package manifold

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
	"github.com/theapemachine/symm/types"
)

func projectionSolver(t testing.TB) *Solver {
	t.Helper()
	// The book keys levels at each pair's precision, so the pairs are known.
	normalizer := spot.NewNormalizer()
	normalizer.Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{"BTC": {AltName: "BTC"}, "ETH": {AltName: "ETH"}, "USD": {AltName: "USD"}},
		NewPairs: map[string]spot.AssetPair{
			"BTCUSD": {WSName: "BTC/USD", Base: "BTC", Quote: "USD", LotMultiplier: 1, TickSize: decimal.NewFromInt64(1)},
			"ETHUSD": {WSName: "ETH/USD", Base: "ETH", Quote: "USD", LotMultiplier: 1, TickSize: decimal.NewFromInt64(1)},
		},
	})
	book := broker.NewBook(context.Background(), normalizer)
	for _, symbol := range []string{"BTC/USD", "ETH/USD"} {
		err := book.Update(&kraken.Level3{Type: "snapshot", Data: []kraken.Level3Data{{Symbol: symbol,
			Bids: []kraken.Level3Order{{OrderID: symbol + "-bid", LimitPrice: decimal.NewFromInt64(99), OrderQty: decimal.NewFromInt64(1), Timestamp: time.Unix(100, 0), Event: "add"}},
			Asks: []kraken.Level3Order{{OrderID: symbol + "-ask", LimitPrice: decimal.NewFromInt64(101), OrderQty: decimal.NewFromInt64(2), Timestamp: time.Unix(100, 0), Event: "add"}},
		}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	return &Solver{book: book, dataset: NewDataset(), loaded: make(map[int64]string)}
}

func TestSolverProject(t *testing.T) {
	Convey("Only symbols refreshed from the venue may evict their resident orders", t, func() {
		solver := projectionSolver(t)
		departures, batch, _ := solver.project()
		So(departures, ShouldBeEmpty)
		So(batch.N, ShouldEqual, 4)
		sensorium.StatePool.Put(batch)
		Convey("A quiet book retains every resident", func() {
			departures, batch, _ = solver.project()
			So(departures, ShouldBeEmpty)
			So(batch, ShouldBeNil)
			So(len(solver.loaded), ShouldEqual, 4)
		})
		Convey("Updating BTC preserves ETH, then a real BTC deletion removes one identity", func() {
			solver.markDirty("BTC/USD")
			departures, batch, _ = solver.project()
			So(departures, ShouldBeEmpty)
			So(batch.N, ShouldEqual, 2)
			sensorium.StatePool.Put(batch)
			So(len(solver.loaded), ShouldEqual, 4)
			err := solver.book.Update(&kraken.Level3{Type: "update", Data: []kraken.Level3Data{{Symbol: "BTC/USD", Bids: []kraken.Level3Order{{OrderID: "BTC/USD-bid", LimitPrice: decimal.NewFromInt64(99), OrderQty: decimal.NewFromInt64(0), Timestamp: time.Unix(101, 0), Event: "delete"}}}}})
			So(err, ShouldBeNil)
			solver.markDirty("BTC/USD")
			departures, batch, _ = solver.project()
			So(len(departures), ShouldEqual, 1)
			So(batch.N, ShouldEqual, 1)
			sensorium.StatePool.Put(batch)
			So(len(solver.loaded), ShouldEqual, 3)
			quiet := 0
			for _, symbol := range solver.loaded {
				if symbol == "ETH/USD" {
					quiet++
				}
			}
			So(quiet, ShouldEqual, 2)
		})
	})
}

func TestHostSymbolIsTheSharedField(t *testing.T) {
	previous := types.Focus()
	t.Cleanup(func() { types.SetFocus(previous) })

	types.SetFocus("ETH/USD")
	if got := hostSymbol(nil); got != "ETH/USD" {
		t.Fatalf("hostSymbol() = %q, want the focus", got)
	}

	types.SetFocus("")
	if got := hostSymbol(nil); got != "" {
		t.Fatalf("hostSymbol() = %q, want the unlabeled field", got)
	}

	if got := hostSymbol(map[int64]string{1: "BTC/USD"}); got != "BTC/USD" {
		t.Fatalf("hostSymbol(loaded) = %q, want a resident symbol", got)
	}
}
