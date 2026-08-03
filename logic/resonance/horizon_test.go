package resonance

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
driveSolver runs the solver over a stream of resolvable ticker epochs and
returns it, so a test can inspect the reach it earned.

Each tick carries its own ticker timestamp, because the task head only resolves
a supervised sample when the market epoch advances. A stream that repeats one
epoch teaches the head nothing however long it runs.
*/
func driveSolver(ticks int) *Solver {
	solver := NewSolver(make(chan []byte, 1), nil)
	normalized := 0.5

	for tick := range ticks {
		thesis := types.NewThesis()
		thesis.Measurements.Store(types.SourceLiquidity, []*types.Measurement{{
			Source: types.SourceLiquidity,
			Symbol: "BTC/USD",
			Metrics: map[string]types.MetricSample{
				"reading": {Normalized: &normalized},
			},
		}})

		thesis.Tickers.Store("BTC/USD", kraken.TickerData{
			Symbol:    "BTC/USD",
			Bid:       decimal.NewFromFloat64(100 + float64(tick)*0.01),
			Ask:       decimal.NewFromFloat64(100.02 + float64(tick)*0.01),
			Timestamp: time.Unix(int64(tick), 0),
		})

		solver.Update(thesis)
	}

	return solver
}

/*
TestHorizonExtendsWhilePrecisionHolds pins the behaviour the predictive coding
stage exists to provide: a forecast window that grows as far as the head can
support and gives way when it cannot.

A window fixed at one step makes the whole rollout dead weight, and a window
fixed at its ceiling claims reach the head never earned. Both were live at
different points: the ceiling was multiplied by an uncalibrated confidence that
evaluated to approximately zero, so the horizon was one on essentially every
tick regardless of how well the network was predicting.
*/
func TestHorizonExtendsWhilePrecisionHolds(t *testing.T) {
	Convey("Given a head fed resolvable epochs until its precision settles", t, func() {
		solver := driveSolver(400)
		state := solver.state("BTC/USD")
		precision, hasPrecision := state.manifold.TaskPrecision()

		t.Logf("samples %d, precision %.3f, reach %d",
			state.targetSamples, precision, state.horizonReach)

		Convey("Then the head reports a precision it can be judged on", func() {
			So(hasPrecision, ShouldBeTrue)
			So(precision, ShouldBeGreaterThan, 0)
		})

		Convey("Then the reach grows well past a single step", func() {
			So(state.targetSamples, ShouldBeGreaterThan, 0)
			So(state.horizonReach, ShouldBeGreaterThan, 1)
			So(state.horizonReach, ShouldBeLessThanOrEqualTo, solver.maxHorizon)
		})

		Convey("Then the published horizon does not outrun that reach", func() {
			/*
				Reach is what the head earned; the published horizon is that
				reach after retention and confidence have capped it. Temporal
				contraction may honestly reduce even a fully confident rollout
				to one step, so earned reach is an upper bound, not a promise.
			*/
			horizon := solver.horizonFor(state, 1.0)

			So(horizon, ShouldBeGreaterThanOrEqualTo, 1)
			So(horizon, ShouldBeLessThanOrEqualTo, state.horizonReach)
		})
	})
}

func TestHorizonConfidenceCapsReach(t *testing.T) {
	Convey("Given a solver that has already earned multi-step reach", t, func() {
		solver := driveSolver(400)
		state := solver.state("BTC/USD")
		precision, hasPrecision := state.manifold.TaskPrecision()

		So(hasPrecision, ShouldBeTrue)
		So(precision, ShouldBeGreaterThan, 0)
		So(state.horizonReach, ShouldBeGreaterThan, 1)

		Convey("Then middling confidence caps the earned reach itself", func() {
			reach := state.horizonReach
			confidence := 0.4
			horizon := solver.horizonFor(state, confidence)
			confidenceCap := max(1, int(float64(reach)*confidence))

			So(horizon, ShouldBeLessThan, reach)
			So(horizon, ShouldBeGreaterThanOrEqualTo, 1)
			So(horizon, ShouldBeLessThanOrEqualTo, confidenceCap)
		})
	})
}

/*
TestHorizonRetractsFasterThanItGrows pins the asymmetry between earning reach
and losing it.

Claiming reach the head cannot support prices trades off a forecast that was
never made, while claiming too little only forgoes edge. The controller must
therefore give ground faster than it takes it.
*/
func TestHorizonRetractsFasterThanItGrows(t *testing.T) {
	Convey("Given a solver holding its full reach", t, func() {
		solver := NewSolver(make(chan []byte, 1), nil)
		state := solver.state("BTC/USD")
		state.horizonReach = solver.maxHorizon

		growthTicks := solver.maxHorizon / horizonGrowthStep
		retractionTicks := 0

		for state.horizonReach > 1 {
			state.horizonReach = max(1, int(
				float64(state.horizonReach)*horizonRetractionFactor,
			))

			retractionTicks++

			So(retractionTicks, ShouldBeLessThan, 100)
		}

		t.Logf("earned over %d ticks, surrendered in %d", growthTicks, retractionTicks)

		Convey("Then reach is surrendered faster than it is earned", func() {
			So(retractionTicks, ShouldBeLessThan, growthTicks)
		})
	})
}

/*
TestHorizonStartsShortWithoutSamples pins that reach is earned rather than
assumed. A head that has resolved no supervised sample has no basis for any
claim about the future.
*/
func TestHorizonStartsShortWithoutSamples(t *testing.T) {
	Convey("Given a solver whose head has resolved nothing", t, func() {
		solver := driveSolver(1)
		state := solver.state("BTC/USD")

		_, hasPrecision := state.manifold.TaskPrecision()

		Convey("Then it claims the shortest horizon", func() {
			So(hasPrecision, ShouldBeFalse)
			So(solver.horizonFor(state, 1.0), ShouldEqual, 1)
			So(state.horizonReach, ShouldEqual, 1)
		})
	})
}
