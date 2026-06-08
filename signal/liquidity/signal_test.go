package liquidity

import (
	"container/ring"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

func TestSignalMeasure(t *testing.T) {
	Convey("Given a cross-section with deep and thin peers", t, func() {
		crossSection := &crossSection{}
		crossSection.publishQuoteVol("COIN/EUR", 800)
		crossSection.publishQuoteVol("PEER/EUR", 900)

		signal := NewSignal(
			"ALT/EUR",
			logic.NewEntity(logic.EntityTick),
			ring.New(4),
			crossSection,
			2.0,
			0.5,
		)

		signal.measurements.Value = &krakenmarket.TickerUpdate{
			Symbol: "ALT/EUR",
			Last:   10,
			Volume: 125,
			Ask:    10.1,
			Bid:    9.9,
		}
		signal.measurements = signal.measurements.Next()

		measurement, err := signal.Measure(nil)

		Convey("It should publish robust liquidity", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceLiquidity)
			So(measurement.Category, ShouldEqual, logic.CategoryRobustLiquidity)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a peak-scarcity symbol", t, func() {
		crossSection := &crossSection{}
		crossSection.publishQuoteVol("DEEP/EUR", 1100)
		crossSection.publishQuoteVol("MID/EUR", 950)

		signal := NewSignal(
			"THIN/EUR",
			logic.NewEntity(logic.EntityTick),
			ring.New(4),
			crossSection,
			2.0,
			0.5,
		)

		signal.measurements.Value = &krakenmarket.TickerUpdate{
			Symbol: "THIN/EUR",
			Last:   5,
			Volume: 50,
			Ask:    5.1,
			Bid:    4.9,
		}
		signal.measurements = signal.measurements.Next()

		measurement, err := signal.Measure(nil)

		Convey("It should classify extreme scarcity", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryExtremeScarcity)
		})
	})

	Convey("Given fewer than two universe symbols", t, func() {
		crossSection := &crossSection{}
		signal := NewSignal(
			"SOLO/EUR",
			logic.NewEntity(logic.EntityTick),
			ring.New(4),
			crossSection,
			2.0,
			0.5,
		)

		signal.measurements.Value = &krakenmarket.TickerUpdate{
			Symbol: "SOLO/EUR",
			Last:   5,
			Volume: 100,
		}
		signal.measurements = signal.measurements.Next()

		measurement, err := signal.Measure(nil)

		Convey("It should withhold the reading", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryTypeNone)
		})
	})

	Convey("Given feedback for the same symbol", t, func() {
		crossSection := &crossSection{}
		signal := NewSignal(
			"ALT/EUR",
			logic.NewEntity(logic.EntityTrade),
			ring.New(4),
			crossSection,
			2.0,
			0.5,
		)
		feedback := market.NewFeedback("ALT/EUR", 0.5, 1.0, 0.2, 3)

		_, err := signal.Measure(feedback)

		Convey("It should apply tuning without error", func() {
			So(err, ShouldBeNil)
			So(signal.weights.Threshold, ShouldBeGreaterThan, 2.0)
		})
	})

	Convey("Given a wrong entity type in the ring", t, func() {
		crossSection := &crossSection{}
		signal := NewSignal(
			"ALT/EUR",
			logic.NewEntity(logic.EntityTrade),
			ring.New(2),
			crossSection,
			2.0,
			0.5,
		)

		signal.measurements.Value = &krakenmarket.TickerUpdate{Symbol: "ALT/EUR"}
		signal.measurements = signal.measurements.Next()

		_, err := signal.Measure(nil)

		Convey("It should return a type error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestSignalClassify(t *testing.T) {
	Convey("Given peer quote volumes", t, func() {
		signal := NewSignal(
			"ALT/EUR",
			logic.NewEntity(logic.EntityTick),
			ring.New(4),
			&crossSection{},
			2.0,
			0.5,
		)
		peers := []float64{800, 900, 1000, 1100}
		lower, upper := signal.quartiles(peers)

		Convey("It should map peer quartiles onto scarcity categories", func() {
			So(signal.classify(1200, lower, upper, false), ShouldEqual, logic.CategoryRobustLiquidity)
			So(signal.classify(950, lower, upper, false), ShouldEqual, logic.CategoryMedianDepth)
			So(signal.classify(500, lower, upper, true), ShouldEqual, logic.CategoryExtremeScarcity)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	crossSection := &crossSection{}

	for index := range 16 {
		symbol := fmt.Sprintf("SYM%d/EUR", index)
		crossSection.publishQuoteVol(symbol, float64(500+index*50))
	}

	signal := NewSignal(
		"SYM0/EUR",
		logic.NewEntity(logic.EntityTick),
		ring.New(4),
		crossSection,
		2.0,
		0.5,
	)

	signal.measurements.Value = &krakenmarket.TickerUpdate{
		Symbol: "SYM0/EUR",
		Last:   10,
		Volume: 125,
		Ask:    10.1,
		Bid:    9.9,
	}
	signal.measurements = signal.measurements.Next()

	b.ReportAllocs()

	for b.Loop() {
		_, err := signal.Measure(nil)

		if err != nil {
			b.Fatal(err)
		}
	}
}
