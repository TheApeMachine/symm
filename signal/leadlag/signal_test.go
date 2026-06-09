package leadlag

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/numeric"
)

func TestSignalMeasureStall(t *testing.T) {
	Convey("Given a lead-lag signal", t, func() {
		signal := &Signal{
			transition: numeric.NewTransitionMatrix(5, 0.5),
		}

		measurement, err := signal.measureStall(0.6, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should classify an anchor stall on the unit interval", func() {
			So(err, ShouldBeNil)
			So(measurement.Source, ShouldEqual, logic.SourceLeadLag)
			So(measurement.Category, ShouldEqual, logic.CategoryAnchorStall)
			So(measurement.Confidence, ShouldAlmostEqual, 0.375, 0.0001)
		})
	})
}

func TestSignalMeasureFollower(t *testing.T) {
	Convey("Given a lead-lag signal with cross-section state", t, func() {
		t.Cleanup(viper.Reset)
		viper.Set("market.anchor_symbol", "BTC/EUR")

		crossSection := newCrossSection()
		leadLagSection = crossSection

		signal := &Signal{
			symbol:     "ETH/EUR",
			transition: numeric.NewTransitionMatrix(5, 0.5),
		}

		anchor := crossSection.ensure("BTC/EUR")
		follower := crossSection.ensure("ETH/EUR")

		Convey("When both series lack enough overlap", func() {
			anchor.observeTicker(50000, time.Now())
			follower.observeTicker(100, time.Now())

			measurement, err := signal.measureFollower(anchor, follower, time.Now())

			Convey("It should withhold the reading", func() {
				So(err, ShouldBeNil)
				So(measurement.Source, ShouldEqual, logic.SourceNone)
			})
		})

		Convey("When a warmed anchor moves independently from the follower", func() {
			start := time.Date(2026, 6, 3, 10, 30, 0, 0, time.UTC)

			for index := range minLagSamples {
				at := start.Add(time.Duration(index) * barInterval)
				anchor.observeTicker(100, at)
				follower.observeTicker(100.01, at)
				crossSection.anchorMove()
			}

			for range anchorMoveMinObs {
				crossSection.anchorMove()
			}

			origin := start.Add(time.Duration(minLagSamples) * barInterval)

			for index := range 18 {
				at := origin.Add(time.Duration(index) * barInterval)
				anchor.observeTicker(100+float64(index)*0.3, at)
				follower.observeTicker(100-float64(index)*0.35, at)
				crossSection.anchorMove()
			}

			finalAt := origin.Add(18 * barInterval)
			anchor.observeTicker(116, finalAt)
			follower.observeTicker(93.5, finalAt)
			move := crossSection.anchorMove()
			measurement, err := signal.measureFollower(anchor, follower, time.Now())

			Convey("It should clear the anchor move gate and classify decoupling", func() {
				So(err, ShouldBeNil)
				So(move.ready, ShouldBeTrue)
				So(move.moved, ShouldBeTrue)
				So(measurement.Source, ShouldEqual, logic.SourceLeadLag)
				So(measurement.Category, ShouldEqual, logic.CategoryDecoupledMove)
			})
		})
	})
}

func TestSymbolStatePriceSamples(t *testing.T) {
	Convey("Given ticker observations", t, func() {
		state := newCrossSection().ensure("BTC/EUR")
		start := time.Now()

		for index := range 20 {
			state.observeTicker(100+float64(index), start.Add(time.Duration(index)*ringSampleSpacing))
		}

		var buffer [priceHistoryCap]numeric.PriceSample

		Convey("It should retain enough samples for correlation", func() {
			So(len(state.priceSamplesInto(buffer[:0])), ShouldBeGreaterThanOrEqualTo, minLagSamples)
		})
	})
}

func TestSymbolStateCrossLagInsufficientData(t *testing.T) {
	Convey("Given sparse histories", t, func() {
		crossSection := newCrossSection()
		anchor := crossSection.ensure("BTC/EUR")
		follower := crossSection.ensure("ETH/EUR")
		now := time.Now()

		anchor.observeTicker(100, now)
		follower.observeTicker(200, now)

		_, _, ok := follower.crossLag(anchor)

		Convey("It should refuse to score lag without enough samples", func() {
			So(ok, ShouldBeFalse)
		})
	})
}

func TestSymbolStateContemporaneous(t *testing.T) {
	Convey("Given aligned price paths", t, func() {
		crossSection := newCrossSection()
		anchor := crossSection.ensure("BTC/EUR")
		follower := crossSection.ensure("ETH/EUR")
		start := time.Now()

		for index := range 20 {
			at := start.Add(time.Duration(index) * ringSampleSpacing)
			anchor.observeTicker(100+float64(index), at)
			follower.observeTicker(200+float64(index)*2, at)
		}

		correlation, ok := follower.contemporaneous(anchor)

		Convey("It should compute positive contemporaneous correlation", func() {
			So(ok, ShouldBeTrue)
			So(correlation, ShouldBeGreaterThan, 0.5)
		})
	})
}

func TestRecentPathMove(t *testing.T) {
	Convey("Given a flat anchor path across the lag window", t, func() {
		state := newCrossSection().ensure("BTC/EUR")
		start := time.Now()

		for index := range minLagSamples {
			state.observeTicker(50000, start.Add(time.Duration(index)*2*time.Minute))
		}

		move, ok := state.recentPathMove(time.Duration(maxLagBars) * barInterval)

		Convey("It should report a near-zero move", func() {
			So(ok, ShouldBeTrue)
			So(move, ShouldBeLessThan, 1e-6)
		})
	})
}

func TestMoveBaselineEvaluate(t *testing.T) {
	Convey("Given a warmed move baseline", t, func() {
		baseline := moveBaseline{
			minObs:  anchorMoveMinObs,
			alpha:   anchorMoveAlpha,
			minMove: anchorMoveMinLogRet,
		}

		for index := range anchorMoveMinObs {
			_, _, ready := baseline.evaluate(0.0001 + float64(index%2)*0.00005)
			So(ready, ShouldBeFalse)
		}

		Convey("It should classify a flat reading as stall with unit margin", func() {
			moved, margin, ready := baseline.evaluate(0.00001)
			So(ready, ShouldBeTrue)
			So(moved, ShouldBeFalse)
			So(margin, ShouldBeGreaterThan, 0)
			So(margin, ShouldBeLessThanOrEqualTo, 1)
		})
	})
}

func TestSignalMeasureTickAnchorStall(t *testing.T) {
	Convey("Given a flat anchor ticker path", t, func() {
		t.Cleanup(viper.Reset)
		viper.Set("market.anchor_symbol", "BTC/EUR")

		crossSection := newCrossSection()
		leadLagSection = crossSection
		start := time.Now().Add(-time.Duration(maxLagBars) * barInterval)

		for index := range anchorMoveMinObs + minLagSamples {
			crossSection.observePrice(
				"BTC/EUR",
				50000,
				start.Add(time.Duration(index)*2*time.Minute),
			)
			crossSection.anchorMove()
		}

		signal := NewSignal(
			"BTC/EUR",
			logic.NewEntity(logic.EntityTick),
		)

		measurement, err := signal.fromLag(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))

		Convey("It should publish anchor stall on the anchor symbol", func() {
			So(err, ShouldBeNil)
			So(measurement.Category, ShouldEqual, logic.CategoryAnchorStall)
			So(measurement.Symbol, ShouldEqual, "BTC/EUR")
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	crossSection := newCrossSection()
	leadLagSection = crossSection

	signal := &Signal{
		symbol:     "ETH/EUR",
		transition: numeric.NewTransitionMatrix(5, 0.5),
	}

	anchor := crossSection.ensure("BTC/EUR")
	follower := crossSection.ensure("ETH/EUR")
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

	for index := range minLagSamples {
		at := base.Add(time.Duration(index) * time.Minute)
		anchor.observeTicker(50000+float64(index), at)
		follower.observeTicker(100+float64(index), at.Add(2*time.Minute))
	}

	for range anchorMoveMinObs {
		crossSection.anchorMove()
	}

	b.ResetTimer()

	for b.Loop() {
		_, _ = signal.measureFollower(anchor, follower, time.Now())
	}
}
