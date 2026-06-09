package pumpdump

import (
	"testing"

	"time"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

func TestSignalRecord(t *testing.T) {
	Convey("Given a new signal", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			4,
			2.0,
			0.5,
			3,
			0,
		)

		So(signal.Record(&krakenmarket.TradeUpdate{Symbol: "ETH/EUR", Price: 100, Qty: 1}), ShouldBeTrue)
		So(signal.Record(&krakenmarket.TradeUpdate{Symbol: "ETH/EUR", Price: 101, Qty: 1}), ShouldBeTrue)

		Convey("It should count down warmup without scanning the ring", func() {
			So(signal.warmupRemaining, ShouldEqual, 2)
			So(signal.WarmupFilled(), ShouldEqual, 2)
		})
	})
}

func TestSignalMeasure(t *testing.T) {
	Convey("Given trade samples with a volume spike", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			8,
			2.0,
			0.5,
			3,
			0,
		)

		trades := []*krakenmarket.TradeUpdate{
			{Symbol: "ETH/EUR", Price: 100, Qty: 1},
			{Symbol: "ETH/EUR", Price: 101, Qty: 1},
			{Symbol: "ETH/EUR", Price: 102, Qty: 1},
			{Symbol: "ETH/EUR", Price: 103, Qty: 1},
			{Symbol: "ETH/EUR", Price: 104, Qty: 20},
			{Symbol: "ETH/EUR", Price: 105, Qty: 20},
			{Symbol: "ETH/EUR", Price: 106, Qty: 20},
		}

		for _, trade := range trades {
			signal.Record(trade)
		}

		measurement, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should classify without error", func() {
			So(err, ShouldBeNil)
			So(measurement.Symbol, ShouldEqual, "ETH/EUR")
			So(measurement.Source, ShouldEqual, logic.SourcePumpDump)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
			So(measurement.Category, ShouldNotEqual, logic.CategoryTypeNone)
			So(measurement.ObservedAt, ShouldEqual, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		})
	})

	Convey("Given feedback for the same symbol", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			8,
			2.0,
			0.5,
			3,
			0,
		)
		feedback := market.NewFeedback("ETH/EUR", 0.5, 1.0, 0.2, 3)

		_, err := signal.Measure(feedback, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should apply tuning without error", func() {
			So(err, ShouldBeNil)
			So(signal.weights.Threshold, ShouldBeGreaterThan, 2.0)
		})
	})

	Convey("Given folded book snapshots with tightening spread", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityBook),
			8,
			2.0,
			0.5,
			3,
			0,
		)

		snapshot := &krakenmarket.Book{
			Bids: []krakenmarket.BookLevel{{Price: 99, Qty: 8}},
			Asks: []krakenmarket.BookLevel{{Price: 101, Qty: 4}},
		}
		snapshot.SetEnvelopeType("snapshot")

		updates := []*krakenmarket.Book{
			{
				Bids: []krakenmarket.BookLevel{{Price: 100, Qty: 8}},
				Asks: []krakenmarket.BookLevel{{Price: 100.2, Qty: 4}},
			},
			{
				Bids: []krakenmarket.BookLevel{{Price: 100, Qty: 12}},
				Asks: []krakenmarket.BookLevel{{Price: 100.1, Qty: 6}},
			},
		}

		for _, frame := range updates {
			frame.SetEnvelopeType("update")
		}

		frames := append([]*krakenmarket.Book{snapshot}, updates...)

		for _, frame := range frames {
			signal.Record(frame)
		}

		measurement, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should measure spread compression without error", func() {
			So(err, ShouldBeNil)
			So(measurement.Symbol, ShouldEqual, "ETH/EUR")
			So(measurement.Spread, ShouldBeGreaterThan, 0)
			So(measurement.Category, ShouldNotEqual, logic.CategoryTypeNone)
		})
	})

	Convey("Given a wrong entity type in the ring", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
			8,
			2.0,
			0.5,
			3,
			0,
		)

		signal.Record(&krakenmarket.TickerUpdate{Symbol: "ETH/EUR"})

		_, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should return a type error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	signal := NewSignal(
		"ETH/EUR",
		logic.NewEntity(logic.EntityTrade),
		8,
		2.0,
		0.5,
		3,
		0,
	)

	for index := range 32 {
		signal.Record(&krakenmarket.TradeUpdate{
			Symbol: "ETH/EUR",
			Price:  100 + float64(index),
			Qty:    float64(index%5) + 1,
		})
	}

	b.ReportAllocs()

	for b.Loop() {
		_, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		if err != nil {
			b.Fatal(err)
		}
	}
}
