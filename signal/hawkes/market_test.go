package hawkes_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
marketProof describes one complete production-boot market replay.
*/
type marketProof struct {
	name   string
	states []tests.MarketState
	warm   bool
	fitted bool
}

const warm, model = true, true
const cold = false

/*
Run boots the real graph and captures only the final semantic transition.
*/
func (proof marketProof) Run(t *testing.T) (marketOutcome, []string) {
	market := tests.NewMarket(t.Context(), 3)
	wired, err := stack.NewBooter(t.Context()).Test(market)
	So(err, ShouldBeNil)
	defer func() {
		So(wired.Close(), ShouldBeNil)
	}()
	defer market.Close()

	if proof.warm {
		So(market.Warmup(tests.Consume(wired.Observe)), ShouldBeNil)
	}

	outcome := marketOutcome{peak: make(evidenceValues)}

	for index, state := range proof.states {
		capture := index == len(proof.states)-1
		err = market.Transition(state, func() error {
			thesis, err := wired.Observe()

			if err != nil {
				return err
			}

			if capture {
				outcome.Capture(thesis.Measurements)
			}

			return nil
		})
		So(err, ShouldBeNil)
	}

	return outcome, market.Symbols
}

/*
TestCalculate proves marked-arrival behavior through the production boot graph.
*/
func TestCalculate(t *testing.T) {
	Convey("Given deterministic marked-arrival market tapes", t, func() {
		proofs := []marketProof{
			{"baseline", []tests.MarketState{tests.MarketStateBaseline}, warm, model},
			{"fast pump", []tests.MarketState{tests.MarketStateFastPump}, warm, model},
			{"fast dump", []tests.MarketState{tests.MarketStateFastDump}, warm, model},
			{"slow pump", []tests.MarketState{tests.MarketStateSlowPump}, warm, model},
			{"slow dump", []tests.MarketState{tests.MarketStateSlowDump}, warm, model},
			{"absorption", []tests.MarketState{tests.MarketStateVolumeAbsorption}, warm, model},
			{"low volume", []tests.MarketState{tests.MarketStateLowVolumeLift}, warm, model},
			{"small lift", []tests.MarketState{tests.MarketStateSmallLift}, warm, model},
			{"slow cadence", []tests.MarketState{tests.MarketStateSlowCadenceLift}, warm, model},
			{"compression", []tests.MarketState{tests.MarketStateSpreadCompression}, warm, model},
			{"fast rejection", []tests.MarketState{
				tests.MarketStateFastPump, tests.MarketStateFastDump,
			}, warm, model},
			{"slow rejection", []tests.MarketState{
				tests.MarketStateFastPump, tests.MarketStateSlowDump,
			}, warm, model},
			{"reversal", []tests.MarketState{
				tests.MarketStateSlowDump, tests.MarketStateFastPump,
			}, warm, model},
		}
		outcomes := make(map[string]marketOutcome, len(proofs))
		var symbols []string

		for _, proof := range proofs {
			outcomes[proof.name], symbols = proof.Run(t)
			outcomes[proof.name].Prove(symbols, proof.fitted)
		}

		for _, twin := range []string{"absorption", "low volume", "small lift"} {
			for identity, pump := range outcomes["fast pump"].latest {
				So(outcomes[twin].latest[identity].Raw, ShouldEqual, pump.Raw)
				So(outcomes[twin].peak[identity].Raw,
					ShouldEqual, outcomes["fast pump"].peak[identity].Raw)
			}
		}

		for _, symbol := range symbols {
			pump := outcomes["fast pump"]
			dump := outcomes["fast dump"]
			slow := outcomes["slow cadence"]
			So(pump.latest.Value(types.MetricEventCount, types.SideNone, symbol),
				ShouldEqual, dump.latest.Value(types.MetricEventCount, types.SideNone, symbol))
			So(pump.latest.Value(types.MetricArrivalRate, types.SideBuy, symbol)+
				pump.latest.Value(types.MetricArrivalRate, types.SideSell, symbol),
				ShouldAlmostEqual,
				dump.latest.Value(types.MetricArrivalRate, types.SideBuy, symbol)+
					dump.latest.Value(types.MetricArrivalRate, types.SideSell, symbol))
			So(pump.latest.Value(types.MetricArrivalRate, types.SideBuy, symbol),
				ShouldBeGreaterThan,
				pump.latest.Value(types.MetricArrivalRate, types.SideSell, symbol))
			So(dump.latest.Value(types.MetricArrivalRate, types.SideSell, symbol),
				ShouldBeGreaterThan,
				dump.latest.Value(types.MetricArrivalRate, types.SideBuy, symbol))
			So(pump.latest.Value(types.MetricConditionalIntensity, types.SideBuy, symbol),
				ShouldBeGreaterThan,
				pump.latest.Value(types.MetricConditionalIntensity, types.SideSell, symbol))
			So(dump.latest.Value(types.MetricConditionalIntensity, types.SideSell, symbol),
				ShouldBeGreaterThan,
				dump.latest.Value(types.MetricConditionalIntensity, types.SideBuy, symbol))

			for _, side := range []types.MeasurementSide{types.SideNone, types.SideBuy, types.SideSell} {
				So(pump.latest.Value(types.MetricEventCount, side, symbol),
					ShouldEqual, slow.latest.Value(types.MetricEventCount, side, symbol))
			}

			for _, side := range []types.MeasurementSide{types.SideBuy, types.SideSell} {
				So(pump.latest.Value(types.MetricArrivalRate, side, symbol),
					ShouldBeGreaterThan, slow.latest.Value(types.MetricArrivalRate, side, symbol))
			}
			pumpFit := pump.latest.Value(types.MetricHawkesPoissonDelta, types.SideNone, symbol)
			dumpFit := dump.latest.Value(types.MetricHawkesPoissonDelta, types.SideNone, symbol)
			So(math.Max(pumpFit, dumpFit), ShouldBeLessThanOrEqualTo, 0.0)

			fastRejection := outcomes["fast rejection"]
			slowRejection := outcomes["slow rejection"]
			reversal := outcomes["reversal"]
			So(math.Abs(
				fastRejection.latest.Value(types.MetricEventCount, types.SideBuy, symbol)-
					fastRejection.latest.Value(types.MetricEventCount, types.SideSell, symbol),
			), ShouldBeLessThanOrEqualTo, 1.0)
			So(slowRejection.latest.Value(types.MetricHawkesPoissonDelta, types.SideNone, symbol),
				ShouldBeGreaterThan, 0.0)
			So(fastRejection.latest.Value(types.MetricHawkesPoissonDelta, types.SideNone, symbol),
				ShouldBeGreaterThan, 0.0)
			So(reversal.latest.Value(types.MetricHawkesPoissonDelta, types.SideNone, symbol),
				ShouldBeGreaterThan, 0.0)

			for _, key := range evidenceKeys[:5] {
				So(reversal.latest.Value(key.metric, key.side, symbol),
					ShouldAlmostEqual,
					slowRejection.latest.Value(key.metric, key.side, symbol))
			}

			So(reversal.latest.Value(types.MetricConditionalIntensity, types.SideBuy, symbol),
				ShouldBeGreaterThan,
				slowRejection.latest.Value(types.MetricConditionalIntensity, types.SideBuy, symbol))
		}
	})

	Convey("Given a cold production boot", t, func() {
		outcome, symbols := (marketProof{
			name:   "readiness",
			states: []tests.MarketState{tests.MarketStateBaseline},
			warm:   cold,
		}).Run(t)
		firstModel := -1

		for index, batch := range outcome.batches {
			_, fitted := batch[evidenceIdentity{
				evidenceKey: evidenceKey{
					metric: types.MetricConditionalIntensity,
					side:   types.SideBuy,
				},
				symbol: symbols[0],
			}]
			expected := 5 * len(symbols)

			if fitted {
				expected = len(evidenceKeys) * len(symbols)

				if firstModel < 0 {
					firstModel = index
				}
			}

			So(batch, ShouldHaveLength, expected)
			So(outcome.rows[index], ShouldEqual, expected)
		}

		So(firstModel, ShouldBeGreaterThan, 0)
		So(firstModel, ShouldBeLessThan, len(outcome.batches))
		outcome.Prove(symbols, model)
	})
}
