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
	"github.com/theapemachine/symm/nomagique/physics/sensorium"
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
		message := kraken.Level3Data{
			Symbol: "SYM/USD",
			Bids: []kraken.Level3Order{
				{
					Event:      "add",
					OrderID:    "bid",
					LimitPrice: decimal.NewFromFloat64(10),
					OrderQty:   decimal.NewFromFloat64(1),
				},
			},
			Asks: []kraken.Level3Order{
				{
					Event:      "add",
					OrderID:    "ask",
					LimitPrice: decimal.NewFromFloat64(10.1),
					OrderQty:   decimal.NewFromFloat64(1),
				},
			},
		}
		project := func(forcing forcingState) (
			askEnergy, askAmplitude, bidEnergy float32,
		) {
			for state := range dataset.Step(message, forcing) {
				if state.TokenIDs[0]&1 == 1 {
					askEnergy = state.Energy[0]
					askAmplitude = state.Amp[0]
				}

				if state.TokenIDs[0]&1 == 0 {
					bidEnergy = state.Energy[0]
				}

				sensorium.StatePool.Put(state)
			}

			return
		}

		// Energy is the order's own size in its resident frame, lifted by the
		// excitation on its side. With no forcing observed, both sides carry
		// exactly what their size is worth and neither is favoured.
		Convey("no forcing leaves both sides at their size energy", func() {
			askEnergy, askAmplitude, bidEnergy := project(forcingState{})

			So(askEnergy, ShouldBeGreaterThan, 0)
			So(askEnergy, ShouldAlmostEqual, bidEnergy)
			So(float64(askAmplitude), ShouldAlmostEqual,
				math.Sqrt(float64(askEnergy)), 1e-6)
		})

		// Excitation is a fraction above the order's own size energy, so what
		// matters is that it lifts the side aggressive flow is hitting and
		// leaves the other side where its size put it.
		Convey("buy excitation lifts ask energy but not bid energy", func() {
			restingAsk, _, restingBid := project(forcingState{})
			forcing := forcingState{buyExcitation: 0.5, sellExcitation: 0.0}
			askEnergy, askAmplitude, bidEnergy := project(forcing)

			So(float64(askEnergy), ShouldAlmostEqual,
				float64(restingAsk)*1.5, 1e-5)
			So(float64(askAmplitude), ShouldAlmostEqual,
				math.Sqrt(float64(askEnergy)), 1e-6)
			So(bidEnergy, ShouldAlmostEqual, restingBid)
		})

		Convey("sell excitation lifts bid energy but not ask energy", func() {
			restingAsk, _, restingBid := project(forcingState{})
			forcing := forcingState{buyExcitation: 0.0, sellExcitation: 0.4}
			askEnergy, _, bidEnergy := project(forcing)

			So(float64(bidEnergy), ShouldAlmostEqual,
				float64(restingBid)*1.4, 1e-5)
			So(askEnergy, ShouldAlmostEqual, restingAsk)
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

		level3Data := kraken.Level3Data{
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

				level3 := types.NewEnvelope(types.EnvelopeLevel3)
				level3.Level3Data = level3Data
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
