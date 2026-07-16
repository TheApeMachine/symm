//go:build darwin && cgo

package manifold

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/nomagique/hawkes"
)

func TestSolverAdvance(t *testing.T) {
	viper.Set("market.l3_depth", 8)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	viper.Set("market.manifold_max_symbols", 8)

	Convey("Given Hawkes excitation", t, func() {
		solver, err := NewSolver(newTestBookSource("BTC/USD"))
		So(err, ShouldBeNil)
		defer solver.Close()

		at := time.Unix(1, 0)
		state, advanceErr := solver.advance(
			"BTC/USD",
			solverOutcome(at, 4, 2),
		)
		stale, staleErr := solver.advance(
			"BTC/USD",
			solverOutcome(at, 4, 2),
		)

		Convey("It should return a finite GPU readout", func() {
			So(advanceErr, ShouldBeNil)
			So(state.GasReady(), ShouldBeTrue)
			So(state.Epoch, ShouldEqual, 1)
			So(state.Source, ShouldEqual, "manifold")
			So(state.Reading.CoherenceMag2, ShouldBeGreaterThan, 0)
			So(len(state.Rho), ShouldBeGreaterThan, 0)
			So(len(state.PsiMag2), ShouldBeGreaterThan, 0)
			So(len(state.Particles), ShouldBeGreaterThan, 0)
			So(staleErr, ShouldBeNil)
			So(stale.GasReady(), ShouldBeFalse)
		})
	})
}

func TestSolverAdvanceWaitsForMarketState(t *testing.T) {
	viper.Set("market.l3_depth", 8)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	viper.Set("market.manifold_max_symbols", 8)

	Convey("Given Hawkes state before its authoritative L3 book", t, func() {
		solver, err := NewSolver(newTestBookSource("BTC/USD"))
		So(err, ShouldBeNil)
		defer solver.Close()

		state, advanceErr := solver.advance(
			"ETH/USD",
			solverOutcome(time.Unix(1, 0), 4, 2),
		)

		Convey("It should wait without consuming GPU symbol capacity", func() {
			So(advanceErr, ShouldBeNil)
			So(state.GasReady(), ShouldBeFalse)
			So(solver.symbols, ShouldBeEmpty)
		})
	})
}

func TestSolverAdvanceAppliesAbsoluteHawkesForcing(t *testing.T) {
	viper.Set("market.l3_depth", 8)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	viper.Set("market.manifold_max_symbols", 8)

	Convey("Given the same L3 state up to a 117-unit absolute arrival impulse", t, func() {
		quiet, quietErr := NewSolver(newTestBookSource("BTC/USD"))
		active, activeErr := NewSolver(newTestBookSource("BTC/USD"))
		So(quietErr, ShouldBeNil)
		So(activeErr, ShouldBeNil)
		defer quiet.Close()
		defer active.Close()

		at := time.Unix(1, 0)
		quietState, quietAdvanceErr := quiet.advance(
			"BTC/USD", solverOutcome(at, 2, 1),
		)
		activeState, activeAdvanceErr := active.advance(
			"BTC/USD", solverOutcome(at, 140, 70),
		)

		Convey("It should respond without encoding forcing into carrier coordinates", func() {
			So(quietAdvanceErr, ShouldBeNil)
			So(activeAdvanceErr, ShouldBeNil)
			So(quietState.GasReady(), ShouldBeTrue)
			So(activeState.GasReady(), ShouldBeTrue)
			So(activeState.Reading, ShouldNotResemble, quietState.Reading)
		})
	})
}

func TestSolverAdvanceReplacesOldestSymbolAtCapacity(t *testing.T) {
	viper.Set("market.l3_depth", 8)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	viper.Set("market.manifold_max_symbols", 1)

	Convey("Given more active symbols than the configured GPU working set", t, func() {
		solver, err := NewSolver(newTestBookSource("BTC/USD", "ETH/USD"))
		So(err, ShouldBeNil)
		defer solver.Close()

		_, firstErr := solver.advance(
			"BTC/USD",
			solverOutcome(time.Unix(1, 0), 4, 2),
		)
		state, secondErr := solver.advance(
			"ETH/USD",
			solverOutcome(time.Unix(2, 0), 4, 2),
		)

		Convey("It should replace the stale field and advance the current symbol", func() {
			So(firstErr, ShouldBeNil)
			So(secondErr, ShouldBeNil)
			So(state.GasReady(), ShouldBeTrue)
			So(solver.symbols, ShouldNotContainKey, "BTC/USD")
			So(solver.symbols, ShouldContainKey, "ETH/USD")
		})
	})
}

func solverOutcome(at time.Time, buyRate, sellRate float64) excitation.Outcome {
	return excitation.Outcome{
		At:              at,
		Horizon:         time.Second,
		EventCount:      8,
		BuyArrivalRate:  buyRate,
		SellArrivalRate: sellRate,
		Maturity:        0.75,
		Readiness: excitation.Readiness{
			Observation: true,
			Intensity:   true,
			HawkesFit:   true,
		},
		Fit: hawkes.BivariateFit{
			MuX:            buyRate,
			MuY:            sellRate,
			AlphaXX:        buyRate * 0.1,
			AlphaYY:        sellRate * 0.1,
			AlphaXY:        buyRate * 0.01,
			AlphaYX:        sellRate * 0.01,
			Beta:           2,
			IntensityX:     buyRate,
			IntensityY:     sellRate,
			SpectralRadius: 0.35,
		},
	}
}

func BenchmarkSolverAdvance(b *testing.B) {
	viper.Set("market.l3_depth", 8)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	viper.Set("market.manifold_max_symbols", 8)

	solver, err := NewSolver(newTestBookSource("BTC/USD"))

	if err != nil {
		b.Fatal(err)
	}

	defer solver.Close()

	at := time.Unix(1, 0)
	b.ReportAllocs()

	for b.Loop() {
		outcome := solverOutcome(at, 4, 2)

		if _, advanceErr := solver.advance("BTC/USD", outcome); advanceErr != nil {
			b.Fatal(advanceErr)
		}

		at = at.Add(time.Nanosecond)
	}
}
