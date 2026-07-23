package pumpdump_test

import (
	"math"
	"runtime"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

/*
metricValues groups one signal's raw values by metric and market symbol.
*/
type metricValues = map[types.MetricType]map[string]float64

/*
marketOutcome retains both transition peaks and the latest closed volume bar.
*/
type marketOutcome struct {
	peak   metricValues
	latest metricValues
}

/*
marketProof names one semantic tape and any symbols isolated by that tape.
*/
type marketProof struct {
	name    string
	states  []tests.MarketState
	symbols []string
}

/*
metricRelation declares metrics compared across controlled market tapes.
*/
type metricRelation struct {
	left    string
	right   string
	metrics []types.MetricType
}

/*
TestMeasure proves pumpdump separates cadence, displacement, executed volume,
spread compression, rejection, direction, and symbol isolation through the
production boot graph. Every proof validates all eight metrics at both its peak
and latest new volume bar.
*/
func TestMeasure(t *testing.T) {
	// Mirror peak equality is scheduling-sensitive under parallel package load;
	// pin one P so MeasureLoop sees trade→book→ticker in lockstep with Drain.
	procs := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(procs)

	metrics := []types.MetricType{
		types.MetricRVOL,
		types.MetricPrecursor,
		types.MetricSpread,
		types.MetricCompression,
		types.MetricIgnition,
		types.MetricTrend,
		types.MetricExhaustion,
		types.MetricStrength,
	}
	families := []types.MetricType{
		types.MetricCompression,
		types.MetricIgnition,
		types.MetricTrend,
		types.MetricExhaustion,
	}
	priceFamilies := []types.MetricType{
		types.MetricPrecursor,
		types.MetricIgnition,
		types.MetricTrend,
		types.MetricExhaustion,
	}
	lift := []types.MetricType{
		types.MetricRVOL,
		types.MetricPrecursor,
		types.MetricIgnition,
		types.MetricTrend,
		types.MetricStrength,
	}
	priceLift := lift[1:]
	fastRejection := []tests.MarketState{
		tests.MarketStateFastPump,
		tests.MarketStateFastDump,
	}
	slowRejection := []tests.MarketState{
		tests.MarketStateFastPump,
		tests.MarketStateSlowDump,
	}
	reversal := []tests.MarketState{
		tests.MarketStateSlowDump,
		tests.MarketStateFastPump,
	}

	Convey("Given matched pump-cycle market tapes", t, func() {
		proofs := []marketProof{
			{"baseline", []tests.MarketState{tests.MarketStateBaseline}, nil},
			{"fast pump", []tests.MarketState{tests.MarketStateFastPump}, nil},
			{"fast dump", []tests.MarketState{tests.MarketStateFastDump}, nil},
			{"slow pump", []tests.MarketState{tests.MarketStateSlowPump}, nil},
			{"slow dump", []tests.MarketState{tests.MarketStateSlowDump}, nil},
			{"slow cadence lift", []tests.MarketState{tests.MarketStateSlowCadenceLift}, nil},
			{"small lift", []tests.MarketState{tests.MarketStateSmallLift}, nil},
			{"low-volume lift", []tests.MarketState{tests.MarketStateLowVolumeLift}, nil},
			{"absorption", []tests.MarketState{tests.MarketStateVolumeAbsorption}, nil},
			{"compression", []tests.MarketState{tests.MarketStateSpreadCompression}, nil},
			{"spread control", []tests.MarketState{tests.MarketStateSpreadControl}, nil},
			{"fast rejection", fastRejection, nil},
			{"slow rejection", slowRejection, nil},
			{"reversal", reversal, nil},
			{"isolated pump", []tests.MarketState{tests.MarketStateFastPump}, []string{"SIM1/USD"}},
		}
		outcomes := make(map[string]marketOutcome, len(proofs))
		symbols := []string{}

		for _, proof := range proofs {
			market := tests.NewMarket(t.Context(), 3)
			wired, err := stack.NewBooter(t.Context()).Test(market)
			So(err, ShouldBeNil)
			var thesis *types.Thesis
			So(market.Warmup(func() error {
				var err error
				thesis, err = wired.Observe()
				return err
			}), ShouldBeNil)
			measurements := []*types.Measurement{}
			maturity := make(map[string]float64, len(market.Symbols))

			for _, measurement := range thesis.Measurements {
				if measurement.Source == types.SourcePumpDump {
					maturity[measurement.Symbol] = measurement.Maturity
				}
			}

			for _, state := range proof.states[:len(proof.states)-1] {
				So(market.Transition(state, func() error {
					var err error
					thesis, err = wired.Observe()
					return err
				}, proof.symbols...), ShouldBeNil)
			}

			for _, measurement := range thesis.Measurements {
				if measurement.Source == types.SourcePumpDump {
					maturity[measurement.Symbol] = measurement.Maturity
				}
			}

			So(market.Transition(
				proof.states[len(proof.states)-1], func() error {
					var err error
					thesis, err = wired.Observe()

					if err != nil {
						return err
					}

					current := thesis.Measurements
					advanced := make(map[string]bool, len(market.Symbols))

					for _, measurement := range current {
						if measurement.Source != types.SourcePumpDump {
							continue
						}

						So(measurement.ValidateStruct(), ShouldBeNil)

						if measurement.Maturity > maturity[measurement.Symbol] {
							So(measurement.Validity.State, ShouldEqual, types.ValidityValid)
							advanced[measurement.Symbol] = true
							maturity[measurement.Symbol] = measurement.Maturity
						}

						if measurement.Raw == 0 {
							So(measurement.Normalized, ShouldBeNil)
							continue
						}

						So(measurement.Normalized, ShouldNotBeNil)
						So(math.IsNaN(*measurement.Normalized), ShouldBeFalse)
						So(math.IsInf(*measurement.Normalized, 0), ShouldBeFalse)
					}

					for _, measurement := range current {
						if measurement.Source == types.SourcePumpDump &&
							advanced[measurement.Symbol] {
							measurements = append(measurements, measurement)
						}
					}

					return nil
				}, proof.symbols...,
			), ShouldBeNil)

			outcomes[proof.name] = marketOutcome{
				peak: tests.PeakMeasurements(
					measurements, types.SourcePumpDump, metrics,
				),
				latest: tests.LatestMeasurements(
					measurements, types.SourcePumpDump, metrics,
				),
			}

			if len(symbols) == 0 {
				symbols = append(symbols, market.Symbols...)
			}

			So(wired.Close(), ShouldBeNil)
			market.Close()
		}

		Convey("It should distinguish every causal market regime", func() {
			for name, outcome := range outcomes {
				for _, values := range []metricValues{outcome.peak, outcome.latest} {
					for _, metric := range metrics {
						SoMsg(name+" "+string(metric), values[metric],
							ShouldHaveLength, len(symbols))
					}

					for _, symbol := range symbols {
						for _, metric := range metrics {
							value := values[metric][symbol]
							So(math.IsNaN(value), ShouldBeFalse)
							So(math.IsInf(value, 0), ShouldBeFalse)
							So(value, ShouldBeGreaterThanOrEqualTo, 0)
						}

						So(values[types.MetricRVOL][symbol], ShouldBeGreaterThan, 0)
						So(values[types.MetricSpread][symbol], ShouldBeGreaterThan, 0)
						strength := 0.0

						for _, family := range families {
							strength = max(strength, values[family][symbol])
						}

						So(values[types.MetricStrength][symbol], ShouldEqual, strength)
					}
				}
			}

			comparisons := []metricRelation{
				{"fast pump", "baseline", lift},
				{"slow pump", "baseline", lift},
				{"fast pump", "fast dump", priceLift},
				{"slow dump", "baseline", []types.MetricType{types.MetricExhaustion}},
				{"fast pump", "slow cadence lift", []types.MetricType{
					types.MetricRVOL, types.MetricIgnition, types.MetricTrend, types.MetricStrength,
				}},
				{"fast pump", "small lift", priceLift},
				{"fast pump", "low-volume lift", []types.MetricType{
					types.MetricRVOL, types.MetricIgnition, types.MetricStrength,
				}},
				{"fast pump", "absorption", priceLift},
				{"fast rejection", "fast pump", []types.MetricType{types.MetricExhaustion}},
				{"slow rejection", "fast pump", []types.MetricType{types.MetricExhaustion}},
				{"reversal", "slow dump", priceLift},
			}

			for _, relation := range comparisons {
				for _, metric := range relation.metrics {
					for _, symbol := range symbols {
						SoMsg(
							relation.left+" "+string(metric)+" > "+relation.right,
							outcomes[relation.left].peak[metric][symbol],
							ShouldBeGreaterThan,
							outcomes[relation.right].peak[metric][symbol],
						)
					}
				}
			}

			matches := []metricRelation{
				{"fast pump", "fast dump", []types.MetricType{
					types.MetricRVOL, types.MetricSpread,
				}},
				{"slow pump", "slow dump", []types.MetricType{
					types.MetricRVOL, types.MetricSpread,
				}},
				{"fast pump", "slow cadence lift", []types.MetricType{types.MetricPrecursor}},
				{"fast pump", "small lift", []types.MetricType{types.MetricRVOL}},
				{"fast pump", "absorption", []types.MetricType{types.MetricRVOL}},
				{"compression", "spread control", []types.MetricType{types.MetricRVOL}},
			}

			for _, relation := range matches {
				leftValues := []metricValues{
					outcomes[relation.left].peak,
					outcomes[relation.left].latest,
				}
				rightValues := []metricValues{
					outcomes[relation.right].peak,
					outcomes[relation.right].latest,
				}

				for index, left := range leftValues {
					for _, metric := range relation.metrics {
						for _, symbol := range symbols {
							So(left[metric][symbol], ShouldAlmostEqual,
								rightValues[index][metric][symbol])
						}
					}
				}
			}

			for _, symbol := range symbols {
				for _, name := range []string{"absorption", "compression", "spread control"} {
					for _, metric := range priceFamilies {
						So(outcomes[name].latest[metric][symbol], ShouldEqual, 0.0)
					}
				}

				So(outcomes["fast dump"].peak[types.MetricExhaustion][symbol],
					ShouldBeGreaterThan, 0)
				So(outcomes["compression"].peak[types.MetricCompression][symbol],
					ShouldBeGreaterThan,
					outcomes["spread control"].peak[types.MetricCompression][symbol])
				So(outcomes["compression"].latest[types.MetricCompression][symbol],
					ShouldBeGreaterThan,
					outcomes["spread control"].latest[types.MetricCompression][symbol])
			}

			isolated := outcomes["isolated pump"].peak

			for _, control := range symbols[1:] {
				for _, metric := range lift {
					So(isolated[metric][symbols[0]], ShouldBeGreaterThan,
						isolated[metric][control])
				}
			}
		})
	})
}

/*
BenchmarkMeasure exercises one full production tick against generated markets.
*/
func BenchmarkMeasure(b *testing.B) {
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
		}, tests.Consume(wired.Observe)); err != nil {
			b.Fatal(err)
		}
	}
}
