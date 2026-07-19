//go:build darwin && cgo

package manifold

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/nomagique/hawkes"
	"github.com/theapemachine/symm/types"
)

/*
staticHawkesSource exposes one deterministic excitation outcome to Solver tests.
*/
type staticHawkesSource struct {
	symbols  []string
	outcomes map[string]excitation.Outcome
}

/*
Symbols returns the configured test symbol.
*/
func (source staticHawkesSource) Symbols() []string {
	return source.symbols
}

/*
Outcome returns the configured outcome for its test symbol.
*/
func (source staticHawkesSource) Outcome(symbol string) (excitation.Outcome, bool) {
	outcome, found := source.outcomes[symbol]

	return outcome, found
}

func TestSolverUpdate(t *testing.T) {
	viper.Set("market.l3_depth", 8)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)

	Convey("Given observed arrival intensity before Hawkes fit", t, func() {
		solver, err := NewSolver(newTestBookSource("BTC/USD"))
		So(err, ShouldBeNil)
		defer solver.Close()

		outcome := solverOutcome(time.Unix(1, 0), 4, 2)
		outcome.Readiness.HawkesFit = false
		outcome.Fit = hawkes.BivariateFit{}
		thesis := types.NewThesis(nil, nil)

		err = solver.Update(thesis, staticHawkesSource{
			symbols: []string{"BTC/USD"},
			outcomes: map[string]excitation.Outcome{
				"BTC/USD": outcome,
			},
		})
		state, found := thesis.Manifold.Load("BTC/USD")

		Convey("It should start the field from empirical forcing", func() {
			So(err, ShouldBeNil)
			So(found, ShouldBeTrue)
			So(state.(State).GasReady(), ShouldBeTrue)
		})
	})

	Convey("Given multiple booked intensity leaders", t, func() {
		solver, err := NewSolver(newTestBookSource("BTC/USD", "ETH/USD"))
		So(err, ShouldBeNil)
		defer solver.Close()

		thesis := types.NewThesis(nil, nil)
		bitcoin := solverOutcome(time.Unix(1, 0), 2, 1)
		bitcoin.Readiness.HawkesFit = false
		bitcoin.Fit = hawkes.BivariateFit{}
		ether := solverOutcome(time.Unix(1, 0), 8, 4)
		ether.Readiness.HawkesFit = false
		ether.Fit = hawkes.BivariateFit{}
		err = solver.Update(thesis, staticHawkesSource{
			symbols: []string{"BTC/USD", "ETH/USD"},
			outcomes: map[string]excitation.Outcome{
				"BTC/USD": bitcoin,
				"ETH/USD": ether,
			},
		})
		_, bitcoinFound := thesis.Manifold.Load("BTC/USD")
		_, etherFound := thesis.Manifold.Load("ETH/USD")

		Convey("It should advance every booked leader", func() {
			So(err, ShouldBeNil)
			So(bitcoinFound, ShouldBeTrue)
			So(etherFound, ShouldBeTrue)
			So(solver.symbols, ShouldHaveLength, 2)
		})
	})

}

func TestSolverAdvance(t *testing.T) {
	viper.Set("market.l3_depth", 8)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)

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
			So(stale.GasReady(), ShouldBeTrue)
			So(stale.Epoch, ShouldEqual, state.Epoch)
			So(stale.Rho, ShouldResemble, state.Rho)
		})
	})
}

func TestSolverUpdateRepublishesQuietManifold(t *testing.T) {
	viper.Set("market.l3_depth", 8)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)

	Convey("Given a GasReady field and a quiet Hawkes outcome", t, func() {
		books := newTestBookSource("BTC/USD")
		solver, err := NewSolver(books)
		So(err, ShouldBeNil)
		defer solver.Close()

		at := time.Unix(1, 0)
		outcome := solverOutcome(at, 4, 2)
		first := types.NewThesis(nil, nil)
		So(solver.Update(first, staticHawkesSource{
			symbols:  []string{"BTC/USD"},
			outcomes: map[string]excitation.Outcome{"BTC/USD": outcome},
		}), ShouldBeNil)

		value, found := first.Manifold.Load("BTC/USD")
		So(found, ShouldBeTrue)
		So(value.(State).GasReady(), ShouldBeTrue)

		second := types.NewThesis(nil, nil)
		So(solver.Update(second, staticHawkesSource{
			symbols:  []string{"BTC/USD"},
			outcomes: map[string]excitation.Outcome{"BTC/USD": outcome},
		}), ShouldBeNil)

		Convey("Then the next Thesis still carries the manifold field", func() {
			replay, found := second.Manifold.Load("BTC/USD")
			So(found, ShouldBeTrue)
			So(replay.(State).GasReady(), ShouldBeTrue)
			So(replay.(State).Epoch, ShouldEqual, value.(State).Epoch)
		})
	})
}

func TestSolverAdvanceWaitsForMarketState(t *testing.T) {
	viper.Set("market.l3_depth", 8)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)

	Convey("Given Hawkes state before its authoritative L3 book", t, func() {
		solver, err := NewSolver(newTestBookSource("BTC/USD"))
		So(err, ShouldBeNil)
		defer solver.Close()

		state, advanceErr := solver.advance(
			"ETH/USD",
			solverOutcome(time.Unix(1, 0), 4, 2),
		)

		Convey("It should wait without allocating a Field", func() {
			So(advanceErr, ShouldBeNil)
			So(state.GasReady(), ShouldBeFalse)
			So(solver.symbols, ShouldBeEmpty)
		})
	})
}

func TestSolverAdvanceAppliesAbsoluteHawkesForcing(t *testing.T) {
	viper.Set("market.l3_depth", 8)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)

	Convey("Given the same L3 state up to a 117-unit absolute arrival impulse", t, func() {
		solver, solverErr := NewSolver(newTestBookSource("BTC/USD"))
		So(solverErr, ShouldBeNil)
		defer solver.Close()

		at := time.Unix(1, 0)
		quietState, quietAdvanceErr := solver.advance(
			"BTC/USD", solverOutcome(at, 2, 1),
		)
		activeState, activeAdvanceErr := solver.advance(
			"BTC/USD", solverOutcome(at.Add(time.Second), 140, 70),
		)

		Convey("It should respond without encoding forcing into carrier coordinates", func() {
			So(quietAdvanceErr, ShouldBeNil)
			So(activeAdvanceErr, ShouldBeNil)
			So(quietState.GasReady(), ShouldBeTrue)
			So(activeState.GasReady(), ShouldBeTrue)
			gasDelta := math.Abs(activeState.Reading.GuidanceSpeed-quietState.Reading.GuidanceSpeed) +
				math.Abs(activeState.Reading.PressureGradX-quietState.Reading.PressureGradX) +
				math.Abs(activeState.Reading.Divergence-quietState.Reading.Divergence) +
				math.Abs(activeState.Reading.CoherenceMag2-quietState.Reading.CoherenceMag2)
			for row := range quietState.Rho {
				for col := range quietState.Rho[row] {
					gasDelta += math.Abs(activeState.Rho[row][col] - quietState.Rho[row][col])
				}
			}
			So(gasDelta, ShouldBeGreaterThan, 0)
		})
	})
}

func TestSolverAdvanceAdaptsToForcing(t *testing.T) {
	viper.Set("market.l3_depth", 8)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)

	Convey("Given arrival forcing whose carrier speed exceeds the configured step", t, func() {
		solver, err := NewSolver(newTestBookSource("BTC/USD"))
		So(err, ShouldBeNil)
		defer solver.Close()

		state, advanceErr := solver.advance(
			"BTC/USD",
			solverOutcome(time.Unix(1, 0), 1_000_000, 500_000),
		)

		Convey("It should derive a stable advective step and keep the gas finite", func() {
			So(advanceErr, ShouldBeNil)
			So(state.GasReady(), ShouldBeTrue)
		})
	})
}

func TestSolverAdvanceKeepsMultipleSymbols(t *testing.T) {
	viper.Set("market.l3_depth", 8)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)

	Convey("Given successive advances on distinct booked symbols", t, func() {
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
		restored, restoredErr := solver.advance(
			"BTC/USD",
			solverOutcome(time.Unix(3, 0), 4, 2),
		)

		Convey("It should keep every Field resident", func() {
			So(firstErr, ShouldBeNil)
			So(secondErr, ShouldBeNil)
			So(restoredErr, ShouldBeNil)
			So(state.GasReady(), ShouldBeTrue)
			So(restored.GasReady(), ShouldBeTrue)
			So(restored.Epoch, ShouldEqual, 2)
			So(solver.symbols["BTC/USD"].handle, ShouldNotBeNil)
			So(solver.symbols["ETH/USD"].handle, ShouldNotBeNil)
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

func TestSolverSharedEngineFields(t *testing.T) {
	viper.Set("market.l3_depth", 8)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)

	Convey("Given one shared Metal engine", t, func() {
		solver, err := NewSolver(newTestBookSource("BTC/USD", "ETH/USD"))
		So(err, ShouldBeNil)
		defer solver.Close()

		So(solver.engine, ShouldNotBeNil)

		bitcoin, bitcoinErr := solver.advance(
			"BTC/USD",
			solverOutcome(time.Unix(1, 0), 4, 2),
		)
		ether, etherErr := solver.advance(
			"ETH/USD",
			solverOutcome(time.Unix(1, 0), 8, 4),
		)

		Convey("It should keep two resident Fields on the same engine", func() {
			So(bitcoinErr, ShouldBeNil)
			So(etherErr, ShouldBeNil)
			So(bitcoin.GasReady(), ShouldBeTrue)
			So(ether.GasReady(), ShouldBeTrue)
			So(solver.symbols, ShouldHaveLength, 2)
			So(solver.symbols["BTC/USD"].handle.ResidentBytes(), ShouldBeGreaterThan, 0)
			So(solver.symbols["ETH/USD"].handle.ResidentBytes(), ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkSolverAdvance(b *testing.B) {
	viper.Set("market.l3_depth", 8)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)

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

/*
TestSolverProductionBoot constructs the shared engine at the runtime 64³ grid —
the path that previously SIGSEGV'd on start.
*/
func TestSolverProductionBoot(t *testing.T) {
	viper.Set("market.l3_depth", 10)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)

	solver, err := NewSolver(newTestBookSource("BTC/USD"))

	if err != nil {
		t.Fatalf("NewSolver: %v", err)
	}

	defer solver.Close()

	if solver.engine == nil {
		t.Fatal("engine missing")
	}
}
