package depthflow

import (
	"container/ring"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
)

func TestSignalMeasure(t *testing.T) {
	Convey("Given a bid-heavy book", t, func() {
		crossSection := &crossSection{}
		crossSection.publishTradePressure("BTC/EUR", 0.8)

		signal := NewSignal(
			"BTC/EUR",
			logic.NewEntity(logic.EntityBook),
			ring.New(4),
			crossSection,
			2.0,
			0.5,
		)

		signal.measurements.Value = &krakenmarket.Book{
			Symbol: "BTC/EUR",
			Type:   krakenmarket.BookSnapshot,
			Bids: []krakenmarket.BookLevel{
				{Price: 99, Qty: 10},
				{Price: 98, Qty: 20},
			},
			Asks: []krakenmarket.BookLevel{
				{Price: 101, Qty: 1},
				{Price: 102, Qty: 1},
			},
		}
		signal.measurements = signal.measurements.Next()

		measurement, err := signal.Measure(nil)

		Convey("It should classify loaded imbalance", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceDepthFlow)
			So(measurement.Category, ShouldEqual, logic.CategoryLoadedImbalance)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given deep bid wall with bearish touch", t, func() {
		crossSection := &crossSection{}

		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityBook),
			ring.New(4),
			crossSection,
			2.0,
			0.5,
		)

		signal.measurements.Value = &krakenmarket.Book{
			Symbol: "ETH/EUR",
			Type:   krakenmarket.BookSnapshot,
			Bids: []krakenmarket.BookLevel{
				{Price: 49, Qty: 1},
				{Price: 48, Qty: 30},
			},
			Asks: []krakenmarket.BookLevel{
				{Price: 51, Qty: 8},
				{Price: 52, Qty: 8},
			},
		}
		signal.measurements = signal.measurements.Next()

		measurement, err := signal.Measure(nil)

		Convey("It should classify spoof trap", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategorySpoofTrap)
		})
	})

	Convey("Given trade pressure update", t, func() {
		crossSection := &crossSection{}

		signal := NewSignal(
			"SOL/EUR",
			logic.NewEntity(logic.EntityTrade),
			ring.New(4),
			crossSection,
			2.0,
			0.5,
		)

		signal.measurements.Value = &krakenmarket.TradeUpdate{
			Symbol: "SOL/EUR",
			Side:   "buy",
			Price:  25,
			Qty:    3,
		}
		signal.measurements = signal.measurements.Next()

		signal.measurements.Value = &krakenmarket.TradeUpdate{
			Symbol: "SOL/EUR",
			Side:   "buy",
			Price:  25.1,
			Qty:    2,
		}
		signal.measurements = signal.measurements.Next()

		measurement, err := signal.Measure(nil)

		Convey("It should publish trade pressure without category", func() {
			So(err, ShouldBeNil)
			So(measurement.Symbol, ShouldEqual, "SOL/EUR")
			So(crossSection.tradePressureFor("SOL/EUR"), ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	crossSection := &crossSection{}
	signal := NewSignal(
		"BTC/EUR",
		logic.NewEntity(logic.EntityBook),
		ring.New(64),
		crossSection,
		2.0,
		0.5,
	)

	signal.measurements.Value = &krakenmarket.Book{
		Symbol: "BTC/EUR",
		Type:   krakenmarket.BookSnapshot,
		Bids: []krakenmarket.BookLevel{
			{Price: 99, Qty: 10},
			{Price: 98, Qty: 20},
		},
		Asks: []krakenmarket.BookLevel{
			{Price: 101, Qty: 1},
			{Price: 102, Qty: 1},
		},
	}
	signal.measurements = signal.measurements.Next()

	b.ResetTimer()

	for b.Loop() {
		_, _ = signal.Measure(nil)
	}
}
