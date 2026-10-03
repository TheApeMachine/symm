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
)

func projectionSolver(t testing.TB) *Solver {
	t.Helper()
	book := broker.NewBook(context.Background(), spot.NewNormalizer())
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
		departures, batch := solver.project()
		So(departures, ShouldBeEmpty)
		So(batch.N, ShouldEqual, 4)
		sensorium.StatePool.Put(batch)
		Convey("A quiet book retains every resident", func() {
			departures, batch = solver.project()
			So(departures, ShouldBeEmpty)
			So(batch, ShouldBeNil)
			So(len(solver.loaded), ShouldEqual, 4)
		})
		Convey("Updating BTC preserves ETH, then a real BTC deletion removes one identity", func() {
			solver.markDirty("BTC/USD")
			departures, batch = solver.project()
			So(departures, ShouldBeEmpty)
			So(batch.N, ShouldEqual, 2)
			sensorium.StatePool.Put(batch)
			So(len(solver.loaded), ShouldEqual, 4)
			err := solver.book.Update(&kraken.Level3{Type: "update", Data: []kraken.Level3Data{{Symbol: "BTC/USD", Bids: []kraken.Level3Order{{OrderID: "BTC/USD-bid", LimitPrice: decimal.NewFromInt64(99), OrderQty: decimal.NewFromInt64(0), Timestamp: time.Unix(101, 0), Event: "delete"}}}}})
			So(err, ShouldBeNil)
			solver.markDirty("BTC/USD")
			departures, batch = solver.project()
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

func TestManifoldSolverPersistentFinalizer(t *testing.T) {
	Convey("Given a manifold solver", t, func() {
		solver := projectionSolver(t)

		Convey("The same symbol returns the identical persistent Finalizer instance", func() {
			finalizerBTC := solver.finalizer("BTC/USD")
			So(finalizerBTC, ShouldNotBeNil)

			finalizerBTC2 := solver.finalizer("BTC/USD")
			So(finalizerBTC2 == finalizerBTC, ShouldBeTrue)

			finalizerETH := solver.finalizer("ETH/USD")
			So(finalizerETH != finalizerBTC, ShouldBeTrue)
		})
	})
}

func BenchmarkSolverProject(b *testing.B) {
	solver := projectionSolver(b)
	for b.Loop() {
		solver.markDirty("BTC/USD")
		solver.markDirty("ETH/USD")
		_, batch := solver.project()
		if batch == nil {
			b.Fatal("missing projected state")
		}
		sensorium.StatePool.Put(batch)
	}
}
