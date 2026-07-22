package toxicity_test

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
measurementKey preserves the side identity that distinguishes resting bid
liquidity from resting ask liquidity.
*/
type measurementKey struct {
	metric types.MetricType
	symbol string
	side   types.MeasurementSide
}

/*
marketOutcome retains transition peaks, settlement state, and the strongest
trade epoch for exact event-side assertions.
*/
type marketOutcome struct {
	peak   map[measurementKey]float64
	latest map[measurementKey]float64
	active map[measurementKey]float64
}

/*
marketProof names one semantic tape and whether it contains public executions.
*/
type marketProof struct {
	name   string
	state  tests.MarketState
	traded bool
}

/*
TestCalculate proves exact touch execution, cancellation, and retreat through
the production boot graph without collapsing bid and ask evidence together.
*/
func TestCalculate(t *testing.T) {
	sides := []types.MeasurementSide{types.SideBuy, types.SideSell}

	Convey("Given execution, withdrawal, and adversarial Level3 tapes", t, func() {
		proofs := []marketProof{
			{"baseline", tests.MarketStateBaseline, true},
			{"fast pump", tests.MarketStateFastPump, true},
			{"slow cadence lift", tests.MarketStateSlowCadenceLift, true},
			{"small lift", tests.MarketStateSmallLift, true},
			{"slow pump", tests.MarketStateSlowPump, true},
			{"fast dump", tests.MarketStateFastDump, true},
			{"slow dump", tests.MarketStateSlowDump, true},
			{"absorption", tests.MarketStateVolumeAbsorption, true},
			{"low-volume lift", tests.MarketStateLowVolumeLift, true},
			{"compression", tests.MarketStateSpreadCompression, true},
			{"spread control", tests.MarketStateSpreadControl, true},
			{"thin withdrawal", tests.MarketStateThinLiquidity, false},
			{"loaded control", tests.MarketStateLoadedLiquidity, false},
			{"bid retreat", tests.MarketStateLiquidityRetreat, false},
			{"spoof addition", tests.MarketStateSpoofLiquidity, false},
			{"depth addition", tests.MarketStateDepthThinning, false},
		}
		outcomes := make(map[string]marketOutcome, len(proofs))
		symbols := []string{}

		for _, proof := range proofs {
			market := tests.NewMarket(t.Context(), 3)
			wired, err := stack.NewBooter(t.Context()).Test(market)
			So(err, ShouldBeNil)
			So(market.Warmup(tests.Consume(wired.Crypto.Tick)), ShouldBeNil)
			measurements := []*types.Measurement{}
			So(market.Transition(proof.state, func() error {
				thesis, err := wired.Crypto.Tick()

				if err != nil {
					return err
				}

				for _, measurement := range thesis.Measurements {
					if measurement.Source != types.SourceToxicity {
						continue
					}

					So(measurement.ValidateStruct(), ShouldBeNil)
					So(measurement.Validity.State, ShouldNotEqual, types.ValidityInvalid)
					So(math.IsNaN(measurement.Raw), ShouldBeFalse)
					So(math.IsInf(measurement.Raw, 0), ShouldBeFalse)
					So(measurement.Stream, ShouldEqual, types.Toxicity)
					So(measurement.Scale.Kind, ShouldEqual, types.ScaleObservationWindow)
					So(measurement.Metric, ShouldBeIn,
						types.MetricTradeVolume,
						types.MetricFillVolume,
						types.MetricBestPrice,
						types.MetricTouchQuantity,
						types.MetricCancelledQuantity,
						types.MetricRetreatingQuantity,
					)

					measurements = append(measurements, measurement)
				}

				return nil
			}), ShouldBeNil)

			outcome := marketOutcome{
				peak:   map[measurementKey]float64{},
				latest: map[measurementKey]float64{},
				active: map[measurementKey]float64{},
			}
			activeAt := map[string]time.Time{}
			activeVolume := map[string]float64{}
			latestAt := time.Time{}

			for _, measurement := range measurements {
				key := measurementKey{
					metric: measurement.Metric,
					symbol: measurement.Symbol,
					side:   measurement.Side,
				}

				if measurement.Raw > outcome.peak[key] {
					outcome.peak[key] = measurement.Raw
				}

				if measurement.At.After(latestAt) {
					latestAt = measurement.At
					clear(outcome.latest)
				}

				if measurement.At.Equal(latestAt) {
					outcome.latest[key] = measurement.Raw
				}

				if measurement.Metric == types.MetricTradeVolume &&
					measurement.Raw > activeVolume[measurement.Symbol] {
					activeVolume[measurement.Symbol] = measurement.Raw
					activeAt[measurement.Symbol] = measurement.At
				}
			}

			for _, measurement := range measurements {
				if !measurement.At.Equal(activeAt[measurement.Symbol]) {
					continue
				}

				outcome.active[measurementKey{
					metric: measurement.Metric,
					symbol: measurement.Symbol,
					side:   measurement.Side,
				}] = measurement.Raw
			}

			outcomes[proof.name] = outcome

			if len(symbols) == 0 {
				symbols = append(symbols, market.Symbols...)
			}

			So(wired.Close(), ShouldBeNil)
			market.Close()
		}

		Convey("It should retain complete side-specific Level3 evidence", func() {
			expectedTouches := map[string][2]float64{
				"thin withdrawal": {10_000, 10},
				"loaded control":  {50_170, 10_000},
				"spoof addition":  {10_000, 50_000},
			}

			for _, proof := range proofs {
				outcome := outcomes[proof.name]
				touch := [2]float64{10_000, 10_000}

				if expected, exists := expectedTouches[proof.name]; exists {
					touch = expected
				}

				for _, symbol := range symbols {
					values := outcome.latest
					bidPrice := values[measurementKey{types.MetricBestPrice, symbol, types.SideBuy}]
					askPrice := values[measurementKey{types.MetricBestPrice, symbol, types.SideSell}]
					So(bidPrice, ShouldBeLessThan, askPrice)
					bidQuantity := values[measurementKey{types.MetricTouchQuantity, symbol, types.SideBuy}]
					askQuantity := values[measurementKey{types.MetricTouchQuantity, symbol, types.SideSell}]
					So(bidQuantity, ShouldEqual, touch[0])
					So(askQuantity, ShouldEqual, touch[1])

					_, hasTrade := values[measurementKey{
						types.MetricTradeVolume, symbol, types.SideNone,
					}]
					So(hasTrade, ShouldEqual, proof.traded)
					fillCount := 0

					for _, side := range sides {
						if _, exists := values[measurementKey{
							types.MetricFillVolume, symbol, side,
						}]; exists {
							fillCount++
						}
					}

					So(fillCount == 1, ShouldEqual, proof.traded)
				}
			}

			activeVolumes := map[string]float64{
				"fast pump":         100,
				"slow cadence lift": 100,
				"small lift":        100,
				"slow pump":         30,
				"fast dump":         100,
				"slow dump":         30,
			}
			activeSides := map[string]types.MeasurementSide{
				"fast pump":         types.SideSell,
				"slow cadence lift": types.SideSell,
				"small lift":        types.SideSell,
				"slow pump":         types.SideSell,
				"fast dump":         types.SideBuy,
				"slow dump":         types.SideBuy,
			}

			for name, expectedVolume := range activeVolumes {
				for _, symbol := range symbols {
					active := outcomes[name].active
					side := activeSides[name]
					opposite := types.SideBuy

					if side == types.SideBuy {
						opposite = types.SideSell
					}

					tradeKey := measurementKey{types.MetricTradeVolume, symbol, types.SideNone}
					fillKey := measurementKey{types.MetricFillVolume, symbol, side}
					oppositeFillKey := measurementKey{types.MetricFillVolume, symbol, opposite}
					retreatKey := measurementKey{types.MetricRetreatingQuantity, symbol, side}
					oppositeRetreatKey := measurementKey{
						types.MetricRetreatingQuantity, symbol, opposite,
					}
					So(active[tradeKey], ShouldEqual, expectedVolume)
					_, hasFill := active[fillKey]
					So(hasFill, ShouldBeTrue)
					_, hasOppositeFill := active[oppositeFillKey]
					So(hasOppositeFill, ShouldBeFalse)
					So(active[retreatKey], ShouldEqual, 10_000)
					_, hasOppositeRetreat := active[oppositeRetreatKey]
					So(hasOppositeRetreat, ShouldBeFalse)

					for _, touchSide := range sides {
						_, hasCancellation := active[measurementKey{
							types.MetricCancelledQuantity, symbol, touchSide,
						}]
						So(hasCancellation, ShouldBeFalse)
					}
				}
			}

			for _, symbol := range symbols {
				tradeKey := measurementKey{types.MetricTradeVolume, symbol, types.SideNone}
				pump := outcomes["fast pump"].active
				dump := outcomes["fast dump"].active
				absorption := outcomes["absorption"].active
				pumpFillKey := measurementKey{types.MetricFillVolume, symbol, types.SideSell}
				dumpFillKey := measurementKey{types.MetricFillVolume, symbol, types.SideBuy}
				So(absorption[tradeKey], ShouldEqual, 100)
				So(absorption[pumpFillKey], ShouldEqual, pump[pumpFillKey])
				cadenceFill := outcomes["slow cadence lift"].active[pumpFillKey]
				smallFill := outcomes["small lift"].active[pumpFillKey]
				So(cadenceFill, ShouldEqual, pump[pumpFillKey])
				So(smallFill, ShouldEqual, pump[pumpFillKey])
				slowPumpPrice := outcomes["slow pump"].active[pumpFillKey] / 30
				slowDumpPrice := outcomes["slow dump"].active[dumpFillKey] / 30
				So(slowPumpPrice, ShouldAlmostEqual, pump[pumpFillKey]/100, 1e-12)
				So(slowDumpPrice, ShouldAlmostEqual, dump[dumpFillKey]/100, 1e-12)

				for _, metric := range []types.MetricType{
					types.MetricCancelledQuantity,
					types.MetricRetreatingQuantity,
				} {
					for _, side := range sides {
						_, exists := absorption[measurementKey{metric, symbol, side}]
						So(exists, ShouldBeFalse)
					}
				}
			}

			for _, symbol := range symbols {
				thin := outcomes["thin withdrawal"].peak
				So(thin[measurementKey{
					types.MetricCancelledQuantity, symbol, types.SideSell,
				}], ShouldEqual, 9_990)
				So(thin[measurementKey{
					types.MetricRetreatingQuantity, symbol, types.SideSell,
				}], ShouldEqual, 9_990)

				retreat := outcomes["bid retreat"].peak
				So(retreat[measurementKey{
					types.MetricRetreatingQuantity, symbol, types.SideBuy,
				}], ShouldEqual, 10_000)

				for _, key := range []measurementKey{
					{types.MetricCancelledQuantity, symbol, types.SideBuy},
					{types.MetricRetreatingQuantity, symbol, types.SideBuy},
				} {
					_, exists := thin[key]
					So(exists, ShouldBeFalse)
				}

				for _, key := range []measurementKey{
					{types.MetricCancelledQuantity, symbol, types.SideBuy},
					{types.MetricCancelledQuantity, symbol, types.SideSell},
					{types.MetricRetreatingQuantity, symbol, types.SideSell},
				} {
					_, exists := retreat[key]
					So(exists, ShouldBeFalse)
				}
			}

			for _, name := range []string{
				"loaded control",
				"spoof addition",
				"depth addition",
			} {
				for _, symbol := range symbols {
					for _, key := range []measurementKey{
						{types.MetricCancelledQuantity, symbol, types.SideBuy},
						{types.MetricCancelledQuantity, symbol, types.SideSell},
						{types.MetricRetreatingQuantity, symbol, types.SideBuy},
						{types.MetricRetreatingQuantity, symbol, types.SideSell},
					} {
						_, exists := outcomes[name].peak[key]
						So(exists, ShouldBeFalse)
					}
				}
			}
		})
	})
}

/*
BenchmarkCalculate exercises exact fill and retreat attribution through one
complete production planner tick.
*/
func BenchmarkCalculate(b *testing.B) {
	market := tests.NewMarket(b.Context(), 3)

	wired, err := stack.NewBooter(b.Context()).Test(market)

	if err != nil {
		b.Fatal(err)
	}

	defer func() {
		if err := wired.Close(); err != nil {
			b.Fatal(err)
		}
	}()
	defer market.Close()

	if err := market.Warmup(tests.Consume(wired.Crypto.Tick)); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if err := market.Apply(tests.MarketStep{
			Advance: 250 * time.Millisecond,
			Actions: []tests.MarketAction{
				{
					Kind:   tests.MarketTrade,
					Symbol: "SIM1/USD",
					Side:   "buy",
					Qty:    10,
				},
				{
					Kind:   tests.MarketRefill,
					Symbol: "SIM1/USD",
					Side:   "sell",
					Qty:    10,
				},
				{
					Kind:   tests.MarketMoveMid,
					Symbol: "SIM1/USD",
					Ticks:  1,
				},
			},
		}, tests.Consume(wired.Crypto.Tick)); err != nil {
			b.Fatal(err)
		}
	}
}
