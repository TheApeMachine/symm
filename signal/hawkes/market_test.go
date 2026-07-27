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
		So(market.Warmup(tests.Idle), ShouldBeNil)
	}

	outcome := marketOutcome{peak: make(map[evidenceKey]map[string]float64)}

	for index, state := range proof.states {
		capture := index == len(proof.states)-1
		err = market.Transition(state, func() error {
			thesis := wired.Thesis

			if capture {
				outcome.Capture(thesis)
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
			for _, symbol := range symbols {
				pump := outcomes["fast pump"].latest[symbol]
				twinMeasurement := outcomes[twin].latest[symbol]

				for _, key := range evidenceKeys {
					pumpSample, ok := pump.Sample(key.metric, key.side)
					So(ok, ShouldBeTrue)
					twinSample, twinOk := twinMeasurement.Sample(key.metric, key.side)
					So(twinOk, ShouldBeTrue)
					So(twinSample.Raw, ShouldEqual, pumpSample.Raw)
					So(outcomes[twin].peak[key][symbol],
						ShouldEqual, outcomes["fast pump"].peak[key][symbol])
				}
			}
		}

		for _, symbol := range symbols {
			pump := outcomes["fast pump"]
			dump := outcomes["fast dump"]
			slow := outcomes["slow cadence"]
			So(pump.Value(types.MetricEventCount, types.SideNone, symbol),
				ShouldEqual, dump.Value(types.MetricEventCount, types.SideNone, symbol))
			So(pump.Value(types.MetricArrivalRate, types.SideBuy, symbol)+
				pump.Value(types.MetricArrivalRate, types.SideSell, symbol),
				ShouldAlmostEqual,
				dump.Value(types.MetricArrivalRate, types.SideBuy, symbol)+
					dump.Value(types.MetricArrivalRate, types.SideSell, symbol))
			So(pump.Value(types.MetricArrivalRate, types.SideBuy, symbol),
				ShouldBeGreaterThan,
				pump.Value(types.MetricArrivalRate, types.SideSell, symbol))
			So(dump.Value(types.MetricArrivalRate, types.SideSell, symbol),
				ShouldBeGreaterThan,
				dump.Value(types.MetricArrivalRate, types.SideBuy, symbol))
			So(pump.Value(types.MetricConditionalIntensity, types.SideBuy, symbol),
				ShouldBeGreaterThan,
				pump.Value(types.MetricConditionalIntensity, types.SideSell, symbol))
			So(dump.Value(types.MetricConditionalIntensity, types.SideSell, symbol),
				ShouldBeGreaterThan,
				dump.Value(types.MetricConditionalIntensity, types.SideBuy, symbol))

			for _, side := range []types.MeasurementSide{types.SideNone, types.SideBuy, types.SideSell} {
				So(pump.Value(types.MetricEventCount, side, symbol),
					ShouldEqual, slow.Value(types.MetricEventCount, side, symbol))
			}

			for _, side := range []types.MeasurementSide{types.SideBuy, types.SideSell} {
				So(pump.Value(types.MetricArrivalRate, side, symbol),
					ShouldBeGreaterThan, slow.Value(types.MetricArrivalRate, side, symbol))
			}
			pumpFit := pump.Value(types.MetricHawkesPoissonDelta, types.SideNone, symbol)
			dumpFit := dump.Value(types.MetricHawkesPoissonDelta, types.SideNone, symbol)
			So(math.Max(pumpFit, dumpFit), ShouldBeLessThanOrEqualTo, 0.0)

			fastRejection := outcomes["fast rejection"]
			slowRejection := outcomes["slow rejection"]
			reversal := outcomes["reversal"]
			So(math.Abs(
				fastRejection.Value(types.MetricEventCount, types.SideBuy, symbol)-
					fastRejection.Value(types.MetricEventCount, types.SideSell, symbol),
			), ShouldBeLessThanOrEqualTo, 1.0)
			So(slowRejection.Value(types.MetricHawkesPoissonDelta, types.SideNone, symbol),
				ShouldBeGreaterThan, 0.0)
			So(fastRejection.Value(types.MetricHawkesPoissonDelta, types.SideNone, symbol),
				ShouldBeGreaterThan, 0.0)
			So(reversal.Value(types.MetricHawkesPoissonDelta, types.SideNone, symbol),
				ShouldBeGreaterThan, 0.0)

			for _, key := range evidenceKeys[:5] {
				So(reversal.Value(key.metric, key.side, symbol),
					ShouldAlmostEqual,
					slowRejection.Value(key.metric, key.side, symbol))
			}

			So(reversal.Value(types.MetricConditionalIntensity, types.SideBuy, symbol),
				ShouldBeGreaterThan,
				slowRejection.Value(types.MetricConditionalIntensity, types.SideBuy, symbol))
		}
	})

	Convey("Given a cold production boot", t, func() {
		outcome, symbols := (marketProof{
			name:   "readiness",
			states: []tests.MarketState{tests.MarketStateBaseline},
			warm:   cold,
		}).Run(t)
		firstModel := -1

		expected := len(symbols)

		for index, batch := range outcome.batches {
			measurement := batch[symbols[0]]
			intensity, ok := measurement.Sample(
				types.MetricConditionalIntensity, types.SideBuy,
			)

			if ok && intensity.Raw > 0 && firstModel < 0 {
				firstModel = index
			}

			So(batch, ShouldHaveLength, expected)
			So(outcome.rowCount[index], ShouldEqual, expected)
		}

		So(firstModel, ShouldBeGreaterThan, 0)
		So(firstModel, ShouldBeLessThan, len(outcome.batches))
		outcome.Prove(symbols, model)
	})
}
