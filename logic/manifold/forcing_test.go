package manifold

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
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

/*
TestForcingAdvanceConcurrent proves the lock split between recordForcing (Trade
path) and advance (Level3 physics path). The two now guard separate mutexes, so
a Trade forcing update is never serialized behind a full field advance. Running
both paths concurrently must be race-free (enforced by -race) and must leave the
resident forcing coherent: every forcing write is a whole forcingState, read as a
snapshot under the read lock.
*/
func TestForcingAdvanceConcurrent(t *testing.T) {
	Convey("Given a manifold solver stepped from many goroutines", t, func() {
		solver := testSolver()
		defer solver.Close()

		level3 := types.NewEnvelope(types.EnvelopeLevel3)
		level3.Level3Data = kraken.Level3Data{
			Symbol:    "SYM/USD",
			Timestamp: time.Now(),
			Bids: []kraken.Level3Order{
				{Event: "add", OrderID: "b1", LimitPrice: decimal.NewFromFloat64(10.0), OrderQty: decimal.NewFromFloat64(1.0), Timestamp: time.Now()},
			},
			Asks: []kraken.Level3Order{
				{Event: "add", OrderID: "a1", LimitPrice: decimal.NewFromFloat64(10.1), OrderQty: decimal.NewFromFloat64(1.5), Timestamp: time.Now()},
			},
		}

		trade := testForcingEnvelope("SYM/USD", 0.3, 0.1)

		var wait sync.WaitGroup

		for index := 0; index < 64; index++ {
			wait.Add(1)

			go func(iteration int) {
				defer wait.Done()

				if iteration%2 == 0 {
					solver.Step(trade)
					return
				}

				solver.Step(level3)
			}(index)
		}

		wait.Wait()

		Convey("resident forcing reflects the last recorded Trade state", func() {
			solver.forcingMu.RLock()
			defer solver.forcingMu.RUnlock()

			So(solver.forcing["SYM/USD"].buyExcitation, ShouldAlmostEqual, 0.3, 1e-6)
			So(solver.forcing["SYM/USD"].sellExcitation, ShouldAlmostEqual, 0.1, 1e-6)
		})
	})
}
