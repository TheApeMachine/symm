package pumpdump

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/internal/testconfig"
)

func setPumpDumpTestConfig() {
	testconfig.SeedCompactRegime()
}

func seedTrades(signal *Signal, symbol string, base time.Time, trades []*krakenmarket.TradeUpdate) {
	for index, trade := range trades {
		update := *trade
		update.Symbol = symbol
		update.Timestamp = base.Add(time.Duration(index) * time.Millisecond)
		signal.Record(&update)
	}
}

func seedBooks(signal *Signal, symbol string, base time.Time, frames []*krakenmarket.BookUpdate) {
	for index, frame := range frames {
		update := *frame
		update.Symbol = symbol
		update.Timestamp = base.Add(time.Duration(index) * time.Millisecond)
		signal.Record(&update)
	}
}

func TestSignalRecord(t *testing.T) {
	Convey("Given a new signal", t, func() {
		setPumpDumpTestConfig()

		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
		)

		So(signal.Record(&krakenmarket.TradeUpdate{Symbol: "ETH/EUR", Price: 100, Qty: 1}), ShouldBeTrue)
		So(signal.Record(&krakenmarket.TradeUpdate{Symbol: "ETH/EUR", Price: 101, Qty: 1}), ShouldBeTrue)

		Convey("It should count down warmup without scanning the ring", func() {
			capacity := market.MustSignalMeasurementCapacity()

			So(signal.warmupRemaining, ShouldEqual, capacity-2)
			So(signal.WarmupFilled(), ShouldEqual, 2)
		})
	})
}

func TestSignalMeasure(t *testing.T) {
	setPumpDumpTestConfig()
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	measureAt := eventAt.Add(time.Second)

	Convey("Given trade samples with a volume spike", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
		)

		seedTrades(signal, "ETH/EUR", eventAt, []*krakenmarket.TradeUpdate{
			{Price: 100, Qty: 1},
			{Price: 101, Qty: 1},
			{Price: 102, Qty: 1},
			{Price: 103, Qty: 1},
			{Price: 104, Qty: 20},
			{Price: 105, Qty: 20},
			{Price: 106, Qty: 20},
		})

		measurement, err := signal.Measure(nil, measureAt)

		Convey("It should classify without error", func() {
			So(err, ShouldBeNil)
			So(measurement.Symbol, ShouldEqual, "ETH/EUR")
			So(measurement.Source, ShouldEqual, logic.SourcePumpDump)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
			So(measurement.Category, ShouldNotEqual, logic.CategoryTypeNone)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
			So(measurement.ObservedAt, ShouldEqual, measureAt)
		})
	})

	Convey("Given feedback for the same symbol", t, func() {
		viper.Set("signals.pumpdump.surprise.weights.threshold", 2.0)

		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
		)
		feedback := market.NewFeedback("ETH/EUR", 0.5, 1.0, 0.2, 3)

		seedTrades(signal, "ETH/EUR", eventAt, []*krakenmarket.TradeUpdate{
			{Price: 100, Qty: 1},
			{Price: 101, Qty: 1},
			{Price: 102, Qty: 1},
			{Price: 103, Qty: 1},
		})

		_, err := signal.Measure(feedback, measureAt)

		Convey("It should apply tuning without error", func() {
			So(err, ShouldBeNil)
			So(signal.weights.Threshold, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given book frames with valid touch spread", t, func() {
		signal := NewSignal(
			"BTC/EUR",
			logic.NewEntity(logic.EntityBook),
		)

		seedBooks(signal, "BTC/EUR", eventAt, []*krakenmarket.BookUpdate{
			{
				Bids: []krakenmarket.BookLevel{{Price: 99, Qty: 8}},
				Asks: []krakenmarket.BookLevel{{Price: 101, Qty: 4}},
			},
			{
				Bids: []krakenmarket.BookLevel{{Price: 100, Qty: 8}},
				Asks: []krakenmarket.BookLevel{{Price: 100.2, Qty: 4}},
			},
			{
				Bids: []krakenmarket.BookLevel{{Price: 100, Qty: 12}},
				Asks: []krakenmarket.BookLevel{{Price: 100.1, Qty: 6}},
			},
			{
				Bids: []krakenmarket.BookLevel{{Price: 100, Qty: 12}},
				Asks: []krakenmarket.BookLevel{{Price: 100.05, Qty: 6}},
			},
		})

		measurement, err := signal.Measure(nil, measureAt)

		Convey("It should measure spread compression without error", func() {
			So(err, ShouldBeNil)
			So(measurement.Symbol, ShouldEqual, "BTC/EUR")
			So(measurement.Spread, ShouldBeGreaterThan, 0)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given folded book snapshots with tightening spread", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityBook),
		)

		seedBooks(signal, "ETH/EUR", eventAt, []*krakenmarket.BookUpdate{
			{
				Bids: []krakenmarket.BookLevel{{Price: 99, Qty: 8}},
				Asks: []krakenmarket.BookLevel{{Price: 101, Qty: 4}},
			},
			{
				Bids: []krakenmarket.BookLevel{{Price: 100, Qty: 8}},
				Asks: []krakenmarket.BookLevel{{Price: 100.2, Qty: 4}},
			},
			{
				Bids: []krakenmarket.BookLevel{{Price: 100, Qty: 12}},
				Asks: []krakenmarket.BookLevel{{Price: 100.1, Qty: 6}},
			},
		})

		measurement, err := signal.Measure(nil, measureAt)

		Convey("It should measure spread compression without error", func() {
			So(err, ShouldBeNil)
			So(measurement.Symbol, ShouldEqual, "ETH/EUR")
			So(measurement.Spread, ShouldBeGreaterThan, 0)
			So(measurement.Category, ShouldNotEqual, logic.CategoryTypeNone)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a long silence before a thin print", t, func() {
		viper.Set("signals.pumpdump.window", time.Minute)

		signal := NewSignal(
			"ALT/EUR",
			logic.NewEntity(logic.EntityTrade),
		)

		signal.Record(&krakenmarket.TradeUpdate{
			Symbol:    "ALT/EUR",
			Price:     1,
			Qty:       1000,
			Timestamp: eventAt,
		})

		signal.Record(&krakenmarket.TradeUpdate{
			Symbol:    "ALT/EUR",
			Price:     1,
			Qty:       1000,
			Timestamp: eventAt.Add(time.Second),
		})

		signal.Record(&krakenmarket.TradeUpdate{
			Symbol:    "ALT/EUR",
			Price:     1,
			Qty:       5,
			Timestamp: eventAt.Add(10 * time.Minute),
		})

		Convey("It should decay stale volume context instead of phantom-spiking", func() {
			So(signal.lastRvol, ShouldBeLessThan, 0.1)
		})
	})

	Convey("Given a wrong entity type in the ring", t, func() {
		signal := NewSignal(
			"ETH/EUR",
			logic.NewEntity(logic.EntityTrade),
		)

		signal.Record(&krakenmarket.TickerUpdate{Symbol: "ETH/EUR"})

		_, err := signal.Measure(nil, measureAt)

		Convey("It should return a type error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	setPumpDumpTestConfig()
	eventAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	measureAt := eventAt.Add(time.Second)

	signal := NewSignal(
		"ETH/EUR",
		logic.NewEntity(logic.EntityTrade),
	)

	for index := range 32 {
		signal.Record(&krakenmarket.TradeUpdate{
			Symbol:    "ETH/EUR",
			Price:     100 + float64(index),
			Qty:       float64(index%5) + 1,
			Timestamp: eventAt.Add(time.Duration(index) * time.Millisecond),
		})
	}

	b.ReportAllocs()

	for b.Loop() {
		_, err := signal.Measure(nil, measureAt)

		if err != nil {
			b.Fatal(err)
		}
	}
}
