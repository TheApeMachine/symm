package exhaust

import (
	"testing"

	"time"

	. "github.com/smartystreets/goconvey/convey"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/numeric/adaptive"
	floatring "github.com/theapemachine/symm/ring"
)

func thinningBook(symbol string, bidDepth float64, askPrice float64) *krakenmarket.BookUpdate {
	return &krakenmarket.BookUpdate{
		Symbol: symbol,
		Type:   "snapshot",
		Bids:   []krakenmarket.BookLevel{{Price: 100, Qty: bidDepth}},
		Asks:   []krakenmarket.BookLevel{{Price: askPrice, Qty: bidDepth * 0.5}},
	}
}

func TestSignalMeasure(t *testing.T) {
	Convey("Given deteriorating long-side book history", t, func() {
		exhaustSection = newCrossSection(24)
		symbol := "ETH/EUR"

		for index := range 8 {
			depth := 20.0 - float64(index)*2
			askPrice := 101.0 + float64(index)*0.5
			exhaustSection.observeBook(symbol, thinningBook(symbol, depth, askPrice))
		}

		signal := NewSignal(
			symbol,
			logic.NewEntity(logic.EntityBook),
		)

		signal.Record(thinningBook(symbol, 4, 105))

		measurement, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should publish an exhaustion reading", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceExhaustion)
			So(measurement.Symbol, ShouldEqual, symbol)
			So(measurement.Price, ShouldBeGreaterThan, 0)
			So(measurement.Strength, ShouldBeGreaterThan, 0)
			So(measurement.Category, ShouldNotEqual, logic.CategoryTypeNone)
			So(measurement.Confidence, ShouldBeGreaterThan, 0)
			So(measurement.ObservedAt, ShouldEqual, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		})
	})

	Convey("Given smoothed pressure fade on the long side", t, func() {
		exhaustSection = newCrossSection(24)
		state := exhaustSection.ensure("BTC/EUR")
		state.pressureEMA = adaptive.NewEMA(0)

		for _, sign := range []float64{1, 1, 1, 1, 1, -1, -1, -1} {
			smoothed, err := state.pressureEMA.Next(0, sign)
			So(err, ShouldBeNil)
			state.pressures.Push(smoothed)
		}

		state.lastPrice = 100

		signal := NewSignal(
			"BTC/EUR",
			logic.NewEntity(logic.EntityTrade),
		)

		measurement, err := signal.fromFeatures(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should classify thermal exhaustion from pressure fade", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryThermalExhaustion)
		})
	})

	Convey("Given insufficient decay features", t, func() {
		signal := NewSignal(
			"SOL/EUR",
			logic.NewEntity(logic.EntityBook),
		)

		measurement, err := signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should withhold until history is populated", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryTypeNone)
		})
	})
}

func TestDepthTrend(t *testing.T) {
	Convey("Given shrinking depth samples", t, func() {
		samples := floatring.NewFloatRing(24)

		for _, value := range []float64{10, 10, 10, 10, 8, 6} {
			samples.Push(value)
		}

		signal := NewSignal("X/EUR", logic.NewEntity(logic.EntityBook))

		Convey("It should report positive thinning trend", func() {
			So(signal.depthTrend(samples), ShouldBeGreaterThan, 0)
		})
	})
}

func TestExitScorePicksStrongerSide(t *testing.T) {
	Convey("Given ask-side thinning stronger than bid-side", t, func() {
		history := featureState{
			bidDepths:  floatring.NewFloatRing(24),
			askDepths:  floatring.NewFloatRing(24),
			spreads:    floatring.NewFloatRing(24),
			pressures:  floatring.NewFloatRing(24),
			imbalances: floatring.NewFloatRing(24),
			densities:  floatring.NewFloatRing(24),
			lastPrice:  100,
		}

		for _, value := range []float64{10, 10, 10, 10, 9, 9} {
			history.bidDepths.Push(value)
			history.spreads.Push(4)
			history.pressures.Push(0.2)
			history.imbalances.Push(0.1)
			history.densities.Push(8)
		}

		for _, value := range []float64{10, 10, 10, 10, 8, 2} {
			history.askDepths.Push(value)
		}

		signal := NewSignal("ETH/EUR", logic.NewEntity(logic.EntityBook))
		longUrgency, _, _ := signal.exitScore(history, 1)
		shortUrgency, shortCategory, _ := signal.exitScore(history, -1)

		Convey("It should let the stronger short-side score win", func() {
			So(shortUrgency, ShouldBeGreaterThan, longUrgency)
			So(shortCategory, ShouldEqual, logic.CategoryMechanicalCollapse)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	exhaustSection = newCrossSection(24)
	symbol := "ETH/EUR"

	for index := range 12 {
		depth := 20.0 - float64(index)
		exhaustSection.observeBook(symbol, thinningBook(symbol, depth, 101+float64(index)*0.25))
	}

	signal := NewSignal(
		symbol,
		logic.NewEntity(logic.EntityBook),
	)

	signal.Record(thinningBook(symbol, 6, 104))

	b.ResetTimer()

	for b.Loop() {
		_, _ = signal.Measure(nil, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	}
}
