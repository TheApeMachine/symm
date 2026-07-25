package exhaust_test

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

var exhaustMetrics = []types.MetricType{
	types.MetricMechanical, types.MetricThermal, types.MetricFragile,
	types.MetricReversal, types.MetricUrgency, types.MetricStrength,
	types.MetricValue, types.MetricCategory,
}

var heldSides = []types.MeasurementSide{types.SideBuy, types.SideSell}

var simulatedSymbols = []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"}

/*
measurementKey preserves metric, held side, and symbol while keeping outcome
storage flat.
*/
type measurementKey struct {
	metric types.MetricType
	side   types.MeasurementSide
	symbol string
}

/*
epochKey identifies one complete side-specific measurement set.
*/
type epochKey struct {
	side   types.MeasurementSide
	symbol string
	at     time.Time
}

/*
marketOutcome retains transition-local peaks, terminal values, and complete
epochs without collapsing long-position and short-position evidence.
*/
type marketOutcome struct {
	peak     map[measurementKey]float64
	latest   map[measurementKey]float64
	latestAt map[measurementKey]time.Time
	epochs   map[epochKey]map[types.MetricType]float64
	valid    bool
}

/*
observe records one exhaustion measurement in each view used by the proof.
*/
func (outcome *marketOutcome) observe(measurement *types.Measurement) {
	if measurement == nil || measurement.Source != types.SourceExhaustion {
		return
	}

	baseValid := !measurement.At.IsZero() &&
		measurement.ObservedFrom.Equal(measurement.At) &&
		measurement.Maturity > 0 && measurement.Maturity < 1 &&
		measurement.Validity.State == types.ValidityValid &&
		measurement.Validity.Readiness == types.ReadinessObservation

	measurement.EachMetric(func(
		metric types.MetricType,
		side types.MeasurementSide,
		sample types.MetricSample,
	) bool {
		normalized := sample.Raw == 0 && sample.Normalized == nil

		if sample.Raw > 0 && sample.Normalized != nil {
			normalized = !math.IsNaN(*sample.Normalized) &&
				!math.IsInf(*sample.Normalized, 0)
		}

		outcome.valid = outcome.valid &&
			baseValid &&
			(side == types.SideBuy || side == types.SideSell) &&
			sample.Unit == types.UnitDimensionless &&
			!math.IsNaN(sample.Raw) &&
			!math.IsInf(sample.Raw, 0) &&
			sample.Raw >= 0 && normalized

		key := measurementKey{metric, side, measurement.Symbol}
		peak, exists := outcome.peak[key]

		if !exists || sample.Raw > peak {
			outcome.peak[key] = sample.Raw
		}

		if latest, exists := outcome.latestAt[key]; !exists || !measurement.At.Before(latest) {
			outcome.latest[key] = sample.Raw
			outcome.latestAt[key] = measurement.At
		}

		epoch := epochKey{side, measurement.Symbol, measurement.At}

		if outcome.epochs[epoch] == nil {
			outcome.epochs[epoch] = map[types.MetricType]float64{}
		}

		outcome.epochs[epoch][metric] = sample.Raw

		return true
	})
}

/*
TestCalculate proves the production graph emits complete, side-preserving
exhaustion evidence and distinguishes matched causal market regimes.
*/
func TestCalculate(t *testing.T) {
	Convey("Given directional, rejecting, and book-decay market tapes", t, func() {
		proofs := []struct {
			name   string
			states []tests.MarketState
		}{
			{"baseline", []tests.MarketState{tests.MarketStateBaseline}},
			{"fast pump", []tests.MarketState{tests.MarketStateFastPump}},
			{"slow pump", []tests.MarketState{tests.MarketStateSlowPump}},
			{"fast dump", []tests.MarketState{tests.MarketStateFastDump}},
			{"slow dump", []tests.MarketState{tests.MarketStateSlowDump}},
			{"absorption", []tests.MarketState{tests.MarketStateVolumeAbsorption}},
			{"low-volume lift", []tests.MarketState{tests.MarketStateLowVolumeLift}},
			{"slow cadence lift", []tests.MarketState{tests.MarketStateSlowCadenceLift}},
			{"small lift", []tests.MarketState{tests.MarketStateSmallLift}},
			{"compression", []tests.MarketState{tests.MarketStateSpreadCompression}},
			{"spread control", []tests.MarketState{tests.MarketStateSpreadControl}},
			{"thin", []tests.MarketState{tests.MarketStateThinLiquidity}},
			{"loaded", []tests.MarketState{tests.MarketStateLoadedLiquidity}},
			{"retreat", []tests.MarketState{tests.MarketStateLiquidityRetreat}},
			{"spoof", []tests.MarketState{tests.MarketStateSpoofLiquidity}},
			{"thinning", []tests.MarketState{tests.MarketStateDepthThinning}},
			{"fast rejection", []tests.MarketState{
				tests.MarketStateFastPump,
				tests.MarketStateFastDump,
			}},
			{"slow rejection", []tests.MarketState{
				tests.MarketStateFastPump,
				tests.MarketStateSlowDump,
			}},
			{"reversal", []tests.MarketState{
				tests.MarketStateSlowDump,
				tests.MarketStateFastPump,
			}},
		}
		outcomes := make(map[string]marketOutcome, len(proofs))

		for _, proof := range proofs {
			market := tests.NewMarket(t.Context(), 3)
			wired, err := stack.NewBooter(t.Context()).Test(market)
			So(err, ShouldBeNil)

			So(market.Warmup(tests.Idle), ShouldBeNil)

			for _, state := range proof.states[:len(proof.states)-1] {
				So(market.Transition(state, tests.Idle), ShouldBeNil)
			}

			signal := exhaustionOf(wired.Signals)
			book := signal.Subscribe("book")
			trade := signal.Subscribe("trade")
			measurements := []*types.Measurement{}

			So(market.Transition(proof.states[len(proof.states)-1], func() error {
				drainExhaust(book, trade, &measurements)
				return nil
			}), ShouldBeNil)
			drainExhaust(book, trade, &measurements)

			So(measurements, ShouldNotBeEmpty)

			outcome := marketOutcome{
				peak:     map[measurementKey]float64{},
				latest:   map[measurementKey]float64{},
				latestAt: map[measurementKey]time.Time{},
				epochs:   map[epochKey]map[types.MetricType]float64{},
				valid:    true,
			}

			for _, measurement := range measurements {
				outcome.observe(measurement)
			}

			outcomes[proof.name] = outcome
			So(wired.Close(), ShouldBeNil)
			market.Close()
		}

		Convey("It should validate every metric at every transition epoch", func() {
			for _, outcome := range outcomes {
				So(outcome.valid, ShouldBeTrue)

				for _, side := range heldSides {
					for _, symbol := range simulatedSymbols {
						for _, metric := range exhaustMetrics {
							key := measurementKey{metric, side, symbol}
							_, hasPeak := outcome.peak[key]
							_, hasLatest := outcome.latest[key]
							So(hasPeak, ShouldBeTrue)
							So(hasLatest, ShouldBeTrue)
							So(math.IsNaN(outcome.peak[key]), ShouldBeFalse)
							So(math.IsInf(outcome.peak[key], 0), ShouldBeFalse)
							So(math.IsNaN(outcome.latest[key]), ShouldBeFalse)
							So(math.IsInf(outcome.latest[key], 0), ShouldBeFalse)
						}
					}
				}
				for _, values := range outcome.epochs {
					So(values, ShouldHaveLength, len(exhaustMetrics))
					mechanical := values[types.MetricMechanical]
					thermal := values[types.MetricThermal]
					fragile := values[types.MetricFragile]
					reversal := values[types.MetricReversal]
					urgency := values[types.MetricUrgency]
					category := values[types.MetricCategory]
					strongest := math.Max(
						math.Max(mechanical, thermal),
						math.Max(fragile, reversal),
					)
					So(urgency, ShouldEqual, values[types.MetricStrength])
					So(urgency, ShouldEqual, values[types.MetricValue])
					So(urgency, ShouldBeLessThanOrEqualTo, strongest)
					So(category, ShouldEqual, math.Trunc(category))
					So(category, ShouldBeBetweenOrEqual, 0, 4)

					switch int(category) {
					case 0:
						So(strongest, ShouldEqual, 0)
						So(urgency, ShouldEqual, 0)
					case 1:
						So(mechanical, ShouldEqual, strongest)
					case 2:
						So(fragile, ShouldEqual, strongest)
					case 3:
						So(thermal, ShouldEqual, strongest)
					case 4:
						So(reversal, ShouldEqual, strongest)
					}
				}
			}

			for _, name := range []string{"absorption", "compression"} {
				for _, side := range heldSides {
					for _, symbol := range simulatedSymbols {
						for _, metric := range exhaustMetrics {
							key := measurementKey{metric, side, symbol}
							So(outcomes[name].latest[key], ShouldEqual, 0)
						}
					}
				}
			}

			for _, name := range []string{"loaded", "spoof", "thinning"} {
				for _, side := range heldSides {
					for _, symbol := range simulatedSymbols {
						for _, metric := range []types.MetricType{
							types.MetricMechanical,
							types.MetricThermal,
							types.MetricFragile,
						} {
							key := measurementKey{metric, side, symbol}
							So(outcomes[name].latest[key], ShouldEqual, 0)
						}
					}
				}
			}

			for _, symbol := range simulatedSymbols {
				buyThermal := measurementKey{types.MetricThermal, types.SideBuy, symbol}
				sellThermal := measurementKey{types.MetricThermal, types.SideSell, symbol}
				So(outcomes["fast pump"].peak[sellThermal], ShouldBeGreaterThan,
					outcomes["fast pump"].peak[buyThermal])
				So(outcomes["fast pump"].peak[sellThermal], ShouldBeGreaterThan,
					outcomes["baseline"].peak[sellThermal])
				So(outcomes["fast pump"].peak[sellThermal], ShouldBeGreaterThan,
					outcomes["low-volume lift"].peak[sellThermal])
				So(outcomes["fast pump"].peak[sellThermal], ShouldBeGreaterThan,
					outcomes["small lift"].peak[sellThermal])
				So(outcomes["slow pump"].peak[sellThermal], ShouldBeGreaterThan,
					outcomes["slow pump"].peak[buyThermal])
				So(outcomes["slow cadence lift"].peak[sellThermal],
					ShouldBeGreaterThan,
					outcomes["slow cadence lift"].peak[buyThermal])
				So(outcomes["fast dump"].peak[buyThermal], ShouldBeGreaterThan,
					outcomes["fast dump"].peak[sellThermal])
				So(outcomes["fast dump"].peak[buyThermal], ShouldBeGreaterThan,
					outcomes["baseline"].peak[buyThermal])
				So(outcomes["slow dump"].peak[buyThermal], ShouldBeGreaterThan,
					outcomes["slow dump"].peak[sellThermal])
				So(outcomes["fast rejection"].peak[buyThermal],
					ShouldBeGreaterThan,
					outcomes["fast rejection"].peak[sellThermal])
				So(outcomes["fast rejection"].peak[buyThermal],
					ShouldBeGreaterThan,
					outcomes["slow rejection"].peak[buyThermal])
				So(outcomes["reversal"].peak[sellThermal], ShouldBeGreaterThan,
					outcomes["reversal"].peak[buyThermal])

				for _, name := range []string{
					"fast pump", "slow pump", "fast dump", "slow dump",
					"low-volume lift", "slow cadence lift", "small lift",
					"fast rejection", "slow rejection", "reversal",
				} {
					for _, side := range heldSides {
						So(outcomes[name].peak[measurementKey{
							types.MetricMechanical, side, symbol,
						}], ShouldEqual, 0)
						So(outcomes[name].peak[measurementKey{
							types.MetricReversal, side, symbol,
						}], ShouldEqual, 0)
					}
				}

				for _, side := range heldSides {
					thinMechanical := measurementKey{types.MetricMechanical, side, symbol}
					So(outcomes["thin"].latest[thinMechanical],
						ShouldBeGreaterThan, 0)
					So(outcomes["thin"].latest[measurementKey{
						types.MetricCategory, side, symbol,
					}], ShouldBeIn, float64(1), float64(4))
					So(outcomes["thin"].latest[measurementKey{
						types.MetricFragile, side, symbol,
					}], ShouldEqual, 0)
					So(outcomes["thin"].latest[measurementKey{
						types.MetricThermal, side, symbol,
					}], ShouldEqual, 0)
					So(outcomes["retreat"].latest[thinMechanical],
						ShouldBeGreaterThan, 0)
					So(outcomes["retreat"].latest[measurementKey{
						types.MetricFragile, side, symbol,
					}], ShouldBeGreaterThan, 0)
					// Spread control widens then settles; Fragile must appear
					// during the widen cycle (peak), not necessarily remain on
					// the terminal settle observation.
					So(outcomes["spread control"].peak[measurementKey{
						types.MetricFragile, side, symbol,
					}], ShouldBeGreaterThan, 0)
				}

				buyReversal := outcomes["retreat"].latest[measurementKey{
					types.MetricReversal, types.SideBuy, symbol,
				}]
				buyCategory := outcomes["retreat"].latest[measurementKey{
					types.MetricCategory, types.SideBuy, symbol,
				}]
				So(buyReversal, ShouldBeGreaterThanOrEqualTo, 0)
				So(buyCategory, ShouldBeIn, float64(1), float64(4))

				So(outcomes["retreat"].latest[measurementKey{
					types.MetricReversal, types.SideSell, symbol,
				}], ShouldEqual, 0)
				So(outcomes["retreat"].latest[measurementKey{
					types.MetricUrgency, types.SideBuy, symbol,
				}], ShouldBeGreaterThan,
					outcomes["retreat"].latest[measurementKey{
						types.MetricUrgency, types.SideSell, symbol,
					}])
			}
		})
	})

	Convey("Given an invalid trade row", t, func() {
		signal := exhaust.NewSignal(t.Context(), nil)

		Convey("It should skip unusable rows and return no measurements", func() {
			rows, err := signal.Calculate(
				nil,
				[]kraken.TradeData{{Symbol: "SIM1/USD"}},
				nil,
			)
			So(err, ShouldBeNil)
			So(rows, ShouldBeEmpty)
		})
	})
}

/*
BenchmarkCalculate measures exhaustion through a complete production market
tick with matched book replenishment.
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

	if err := market.Warmup(func() error { return nil }); err != nil {
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
					Qty:    100,
				},
				{
					Kind:   tests.MarketRefill,
					Symbol: "SIM1/USD",
					Side:   "sell",
					Qty:    100,
				},
				{
					Kind:   tests.MarketMoveMid,
					Symbol: "SIM1/USD",
					Ticks:  1,
				},
			},
		}, func() error { return nil }); err != nil {
			b.Fatal(err)
		}
	}
}

/*
exhaustionOf returns the boot-wired exhaustion signal.
*/
func exhaustionOf(signals []types.Signal) *exhaust.Signal {
	for _, signal := range signals {
		if named, ok := signal.(*exhaust.Signal); ok {
			return named
		}
	}

	panic("exhaust signal missing from boot")
}

/*
drainExhaust waits for book/trade publishes from the exhaustion signal, then
drains until idle. Ticker is not subscribed: Calculate ignores ticker rows, so
a ticker subscribe never emits and cannot form a drain barrier.
*/
func drainExhaust(
	book *types.Subscription[any],
	trade *types.Subscription[any],
	into *[]*types.Measurement,
) {
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()

	for {
		select {
		case message := <-book.Channel:
			*into = append(*into, exhaustionFrom(message)...)

			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}

			timer.Reset(50 * time.Millisecond)
		case message := <-trade.Channel:
			*into = append(*into, exhaustionFrom(message)...)

			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}

			timer.Reset(50 * time.Millisecond)
		case <-timer.C:
			return
		}
	}
}

/*
exhaustionFrom reads SourceExhaustion measurements off a signal publish.
*/
func exhaustionFrom(message any) []*types.Measurement {
	switch published := message.(type) {
	case []*types.Measurement:
		out := make([]*types.Measurement, 0, len(published))

		for _, measurement := range published {
			if measurement.Source == types.SourceExhaustion {
				out = append(out, measurement)
			}
		}

		return out
	case *types.Thesis:
		out := make([]*types.Measurement, 0, len(published.Measurements))

		for _, measurement := range published.Measurements {
			if measurement.Source == types.SourceExhaustion {
				out = append(out, measurement)
			}
		}

		return out
	default:
		return nil
	}
}
