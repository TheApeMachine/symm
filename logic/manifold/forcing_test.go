package manifold

import (
	"context"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"
)

func testHawkesMeasurement(symbol string, buy, sell float64, at time.Time) *data.Measurement[float64] {
	return &data.Measurement[float64]{
		Label:  symbol,
		Source: "hawkes",
		At:     at,
		Metrics: map[string]data.Metric[float64]{
			"excitation_fraction:buy":  {Raw: buy},
			"excitation_fraction:sell": {Raw: sell},
		},
	}
}

func testForcingEnvelope(symbol string, buy, sell float64) *types.Envelope {
	envelope := types.NewEnvelope(types.EnvelopeTrade)
	envelope.Hawkes = testHawkesMeasurement(symbol, buy, sell, time.Unix(1, 0))

	return envelope
}

func testSolver() *Solver {
	return NewSolver(context.Background())
}

/*
TestForcingStateRecordedNotAdvanced proves the wiring boundary: a Trade event
with valid Hawkes excitation fractions records the forcing state but never
advances the physics field and never emits envelope.Manifold.
*/
func TestForcingStateRecordedNotAdvanced(t *testing.T) {
	Convey("Given a manifold solver", t, func() {
		solver := testSolver()
		defer solver.Close()

		Convey("trade records forcing without advancing", func() {
			envelope := testForcingEnvelope("SYM/USD", 0.3, 0.1)

			result := solver.Step(envelope)

			So(result.Manifold, ShouldBeNil)
			So(solver.forcing["SYM/USD"].buyExcitation, ShouldAlmostEqual, 0.3, 1e-6)
			So(solver.forcing["SYM/USD"].sellExcitation, ShouldAlmostEqual, 0.1, 1e-6)
		})

		Convey("no forcing observed defaults the symbol to unit baseline", func() {
			So(solver.latestForcing("UNSEEN/USD").buyExcitation, ShouldEqual, 0)
			So(solver.latestForcing("UNSEEN/USD").sellExcitation, ShouldEqual, 0)
		})

		Convey("a non-finite excitation update is rejected", func() {
			envelope := testForcingEnvelope("SYM/USD", math.Inf(1), 0.1)

			solver.Step(envelope)

			So(math.IsInf(float64(solver.forcing["SYM/USD"].buyExcitation), 0), ShouldBeFalse)
		})
	})
}

/*
TestForcingSideCorrectness proves the side-correct energy baseline: buy
excitation lifts asks, sell excitation lifts bids, and no forcing leaves both
sides at the unit baseline.
*/
func TestForcingSideCorrectness(t *testing.T) {
	Convey("Given the dataset order-state projection", t, func() {
		dataset := NewDataset()

		price, quantity, mid, scale := 10.0, 1.0, 10.0, 0.05
		grid := manifoldGrid()
		symbolIndex := uint32(1)
		ask := orderEntry{ask: true}
		bid := orderEntry{ask: false}

		Convey("no forcing leaves both sides at unit energy", func() {
			askState := dataset.orderState(ask, 0, 2, price, quantity, mid, scale, symbolIndex, grid, forcingState{})
			So(askState.Energy[0], ShouldAlmostEqual, 1.0)
			So(askState.Amp[0], ShouldAlmostEqual, 1.0)

			bidState := dataset.orderState(bid, 0, 2, price, quantity, mid, scale, symbolIndex, grid, forcingState{})
			So(bidState.Energy[0], ShouldAlmostEqual, 1.0)
		})

		Convey("buy excitation lifts ask energy but not bid energy", func() {
			forcing := forcingState{buyExcitation: 0.5, sellExcitation: 0.0}

			askState := dataset.orderState(ask, 0, 2, price, quantity, mid, scale, symbolIndex, grid, forcing)
			So(askState.Energy[0], ShouldAlmostEqual, 1.5)
			So(askState.Amp[0], ShouldAlmostEqual, float32(math.Sqrt(1.5)))

			bidState := dataset.orderState(bid, 0, 2, price, quantity, mid, scale, symbolIndex, grid, forcing)
			So(bidState.Energy[0], ShouldAlmostEqual, 1.0)
		})

		Convey("sell excitation lifts bid energy but not ask energy", func() {
			forcing := forcingState{buyExcitation: 0.0, sellExcitation: 0.4}

			bidState := dataset.orderState(bid, 0, 2, price, quantity, mid, scale, symbolIndex, grid, forcing)
			So(bidState.Energy[0], ShouldAlmostEqual, 1.4, 1e-6)

			askState := dataset.orderState(ask, 0, 2, price, quantity, mid, scale, symbolIndex, grid, forcing)
			So(askState.Energy[0], ShouldAlmostEqual, 1.0)
		})
	})
}
