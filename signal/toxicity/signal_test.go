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
	metrics := []types.MetricType{
		types.MetricTradeVolume,
		types.MetricFillVolume,
		types.MetricBestPrice,
		types.MetricTouchQuantity,
		types.MetricCancelledQuantity,
		types.MetricRetreatingQuantity,
	}
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
			So(market.Warmup(wired.Crypto.Step), ShouldBeNil)
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
					So(measurement.Raw, ShouldBeGreaterThan, 0)
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
			for _, proof := range proofs {
				outcome := outcomes[proof.name]

				for _, values := range []map[measurementKey]float64{
					outcome.peak,
					outcome.latest,
				} {
					for _, symbol := range symbols {
						for _, metric := range []types.MetricType{
							types.MetricBestPrice,
							types.MetricTouchQuantity,
						} {
							for _, side := range sides {
								So(values[measurementKey{metric, symbol, side}], ShouldBeGreaterThan, 0)
							}
						}

						tradeKey := measurementKey{
							types.MetricTradeVolume,
							symbol,
							types.SideNone,
						}
						_, hasTrade := values[tradeKey]
						So(hasTrade, ShouldEqual, proof.traded)
						fills := 0

						for _, side := range sides {
							if _, exists := values[measurementKey{
								types.MetricFillVolume,
								symbol,
								side,
							}]; exists {
								fills++
							}
						}

						if proof.traded {
							So(fills, ShouldBeGreaterThan, 0)
							continue
						}

						So(fills, ShouldEqual, 0)
					}
				}
			}

			for _, symbol := range symbols {
				pump := outcomes["fast pump"].active
				dump := outcomes["fast dump"].active
				absorption := outcomes["absorption"].active
				tradeKey := measurementKey{
					types.MetricTradeVolume,
					symbol,
					types.SideNone,
				}
				So(pump[tradeKey], ShouldEqual, dump[tradeKey])
				So(pump[tradeKey], ShouldEqual, absorption[tradeKey])
				So(
					pump[tradeKey],
					ShouldBeGreaterThan,
					outcomes["slow pump"].active[tradeKey],
				)
				So(
					pump[tradeKey],
					ShouldBeGreaterThan,
					outcomes["low-volume lift"].active[tradeKey],
				)

				for _, proof := range []struct {
					name        string
					fillSide    types.MeasurementSide
					retreatSide types.MeasurementSide
				}{
					{"fast pump", types.SideSell, types.SideSell},
					{"fast dump", types.SideBuy, types.SideBuy},
				} {
					active := outcomes[proof.name].active
					So(active[measurementKey{
						types.MetricFillVolume,
						symbol,
						proof.fillSide,
					}], ShouldBeGreaterThan, 0)
					So(active[measurementKey{
						types.MetricCancelledQuantity,
						symbol,
						proof.retreatSide,
					}], ShouldBeGreaterThan, 0)
					So(active[measurementKey{
						types.MetricRetreatingQuantity,
						symbol,
						proof.retreatSide,
					}], ShouldBeGreaterThan, 0)
				}

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

			for _, proof := range []struct {
				name string
				side types.MeasurementSide
			}{
				{"thin withdrawal", types.SideSell},
				{"bid retreat", types.SideBuy},
			} {
				for _, symbol := range symbols {
					for _, metric := range []types.MetricType{
						types.MetricCancelledQuantity,
						types.MetricRetreatingQuantity,
					} {
						So(outcomes[proof.name].peak[measurementKey{
							metric,
							symbol,
							proof.side,
						}], ShouldBeGreaterThan, 0)
					}
				}
			}

			for _, name := range []string{
				"loaded control",
				"spoof addition",
				"depth addition",
			} {
				for _, symbol := range symbols {
					for _, metric := range []types.MetricType{
						types.MetricCancelledQuantity,
						types.MetricRetreatingQuantity,
					} {
						for _, side := range sides {
							_, exists := outcomes[name].peak[measurementKey{
								metric,
								symbol,
								side,
							}]
							So(exists, ShouldBeFalse)
						}
					}
				}
			}

			So(metrics, ShouldHaveLength, 6)
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

	if err := market.Warmup(wired.Crypto.Step); err != nil {
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
		}, wired.Crypto.Step); err != nil {
			b.Fatal(err)
		}
	}
}
