package cvd_test

import (
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
metricValues indexes one CVD snapshot by metric and simulated symbol.
*/
type metricValues = map[types.MetricType]map[string]float64

/*
marketOutcome retains the strongest and settled order-flow evidence from one
market tape so transient aggression cannot be confused with its final state.
*/
type marketOutcome struct {
	peak   metricValues
	latest metricValues
}

/*
TestCalculate proves CVD distinguishes drive, balance, absorption, starvation,
rejection, and isolated flow through the complete production boot graph.
*/
func TestCalculate(t *testing.T) {
	metrics := []types.MetricType{
		types.MetricAbsorption,
		types.MetricDrive,
		types.MetricBalance,
		types.MetricStarvation,
		types.MetricStrength,
		types.MetricNetFraction,
		types.MetricNet,
	}
	families := metrics[:4]

	Convey("Given directional, depleted, and adversarial flow tapes", t, func() {
		proofs := []struct {
			name    string
			states  []tests.MarketState
			symbols []string
		}{
			{"baseline", []tests.MarketState{tests.MarketStateBaseline}, nil},
			{"fast pump", []tests.MarketState{tests.MarketStateFastPump}, nil},
			{"slow cadence lift", []tests.MarketState{tests.MarketStateSlowCadenceLift}, nil},
			{"small lift", []tests.MarketState{tests.MarketStateSmallLift}, nil},
			{"slow pump", []tests.MarketState{tests.MarketStateSlowPump}, nil},
			{"fast dump", []tests.MarketState{tests.MarketStateFastDump}, nil},
			{"slow dump", []tests.MarketState{tests.MarketStateSlowDump}, nil},
			{"absorption", []tests.MarketState{tests.MarketStateVolumeAbsorption}, nil},
			{"low-volume lift", []tests.MarketState{tests.MarketStateLowVolumeLift}, nil},
			{"compression", []tests.MarketState{tests.MarketStateSpreadCompression}, nil},
			{"fast rejection", []tests.MarketState{
				tests.MarketStateFastPump, tests.MarketStateFastDump,
			}, nil},
			{"slow rejection", []tests.MarketState{
				tests.MarketStateFastPump, tests.MarketStateSlowDump,
			}, nil},
			{"reversal", []tests.MarketState{
				tests.MarketStateSlowDump, tests.MarketStateFastPump,
			}, nil},
			{"isolated pump", []tests.MarketState{tests.MarketStateFastPump}, []string{"SIM1/USD"}},
		}
		outcomes := make(map[string]marketOutcome, len(proofs))

		for _, proof := range proofs {
			market := tests.NewMarket(t.Context(), 3)
			wired, err := stack.NewBooter(t.Context()).Test(market)
			So(err, ShouldBeNil)
			So(market.Warmup(wired.Crypto.Step), ShouldBeNil)
			for _, state := range proof.states[:len(proof.states)-1] {
				So(market.Transition(state, wired.Crypto.Step, proof.symbols...), ShouldBeNil)
			}

			from := market.Now()
			measurements := []*types.Measurement{}
			So(market.Transition(
				proof.states[len(proof.states)-1], func() error {
					thesis, err := wired.Crypto.Tick()

					if err != nil {
						return err
					}

					for _, measurement := range thesis.Measurements {
						if measurement.Source == types.SourceCVD && measurement.At.After(from) {
							measurements = append(measurements, measurement)
						}
					}

					return nil
				}, proof.symbols...,
			), ShouldBeNil)
			So(measurements, ShouldNotBeEmpty)

			for _, measurement := range measurements {
				So(measurement.ValidateStruct(), ShouldBeNil)
				So(measurement.Stream, ShouldEqual, types.CVD)
				So(measurement.Subject, ShouldEqual, types.SubjectAggressorFlow)
				So(measurement.Validity.State, ShouldEqual, types.ValidityValid)
				So(measurement.Validity.Readiness, ShouldEqual,
					types.ReadinessObservation)
				So(measurement.Maturity, ShouldBeBetween, 0, 1)

				if measurement.Metric == types.MetricNet {
					So(measurement.Unit, ShouldEqual, types.UnitQuoteCurrency)
					So(measurement.Normalized, ShouldBeNil)
					continue
				}

				So(measurement.Unit, ShouldEqual, types.UnitDimensionless)

				if measurement.Raw == 0 {
					So(measurement.Normalized, ShouldBeNil)
					continue
				}

				So(measurement.Normalized, ShouldNotBeNil)
				So(*measurement.Normalized, ShouldEqual, measurement.Raw)
			}

			outcomes[proof.name] = marketOutcome{
				peak: utils.PeakMeasurements(measurements, types.SourceCVD, metrics),
				latest: utils.LatestMeasurements(
					measurements, types.SourceCVD, metrics,
				),
			}
			outcomes[proof.name].peak[types.MetricNet] =
				utils.PeakMagnitudeMeasurements(
					measurements,
					types.SourceCVD,
					[]types.MetricType{types.MetricNet},
				)[types.MetricNet]
			So(wired.Close(), ShouldBeNil)
			market.Close()
		}

		for _, outcome := range outcomes {
			for _, values := range []metricValues{outcome.peak, outcome.latest} {
				for _, metric := range metrics {
					So(values[metric], ShouldHaveLength, 3)
				}

				for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
					strength := 0.0

					for _, metric := range metrics {
						So(math.IsNaN(values[metric][symbol]), ShouldBeFalse)
						So(math.IsInf(values[metric][symbol], 0), ShouldBeFalse)
					}

					for _, metric := range families {
						So(values[metric][symbol], ShouldBeGreaterThanOrEqualTo, 0)
						strength = max(strength, values[metric][symbol])
					}

					So(values[types.MetricStrength][symbol], ShouldEqual, strength)
					So(values[types.MetricNetFraction][symbol], ShouldBeBetweenOrEqual, 0, 1)
				}
			}
		}

		expectations := []struct {
			name         string
			peak         types.MetricType
			latest       types.MetricType
			netDirection int
		}{
			{"baseline", types.MetricBalance, types.MetricBalance, 0},
			{"fast pump", types.MetricDrive, types.MetricDrive, 1},
			{"slow cadence lift", types.MetricDrive, types.MetricDrive, 1},
			{"small lift", types.MetricDrive, types.MetricDrive, 1},
			{"slow pump", types.MetricDrive, types.MetricStarvation, 1},
			{"fast dump", types.MetricDrive, types.MetricDrive, -1},
			{"slow dump", types.MetricDrive, types.MetricStarvation, -1},
			{"absorption", types.MetricAbsorption, types.MetricAbsorption, 1},
			{"low-volume lift", types.MetricDrive, types.MetricBalance, 1},
			{"compression", types.MetricBalance, types.MetricBalance, 0},
			{"fast rejection", types.MetricDrive, types.MetricStarvation, -1},
			{"slow rejection", types.MetricDrive, types.MetricStarvation, -1},
			{"reversal", types.MetricDrive, types.MetricDrive, 1},
		}

		for _, expectation := range expectations {
			outcome := outcomes[expectation.name]

			for _, phase := range []struct {
				values metricValues
				family types.MetricType
			}{
				{outcome.peak, expectation.peak},
				{outcome.latest, expectation.latest},
			} {
				for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
					So(phase.values[phase.family][symbol], ShouldBeGreaterThan, 0)

					for _, family := range families {
						if family != phase.family {
							So(phase.values[phase.family][symbol], ShouldBeGreaterThan,
								phase.values[family][symbol])
						}
					}
				}
			}

			if expectation.netDirection == 0 {
				continue
			}

			for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
				So(outcome.peak[types.MetricNet][symbol]*
					float64(expectation.netDirection), ShouldBeGreaterThan, 0)
				So(outcome.latest[types.MetricNet][symbol]*
					float64(expectation.netDirection), ShouldBeGreaterThan, 0)
			}
		}

		for _, comparison := range []struct {
			stronger string
			weaker   string
			metric   types.MetricType
			latest   bool
		}{
			{"fast pump", "absorption", types.MetricDrive, false},
			{"fast pump", "absorption", types.MetricDrive, true},
			{"absorption", "fast pump", types.MetricAbsorption, false},
			{"absorption", "fast pump", types.MetricAbsorption, true},
			{"fast pump", "low-volume lift", types.MetricNet, false},
			{"fast pump", "low-volume lift", types.MetricNet, true},
			{"fast pump", "small lift", types.MetricNet, false},
			{"fast pump", "small lift", types.MetricNet, true},
		} {
			stronger := outcomes[comparison.stronger].peak
			weaker := outcomes[comparison.weaker].peak

			if comparison.latest {
				stronger = outcomes[comparison.stronger].latest
				weaker = outcomes[comparison.weaker].latest
			}

			for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
				So(stronger[comparison.metric][symbol], ShouldBeGreaterThan,
					weaker[comparison.metric][symbol])
			}
		}

		for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
			for _, values := range []metricValues{
				outcomes["fast pump"].peak, outcomes["fast pump"].latest,
			} {
				So(values[types.MetricDrive][symbol], ShouldEqual,
					values[types.MetricNetFraction][symbol])
			}

			So(outcomes["absorption"].latest[types.MetricAbsorption][symbol],
				ShouldEqual, outcomes["absorption"].latest[types.MetricNetFraction][symbol])

			for _, name := range []string{"baseline", "compression", "low-volume lift"} {
				latest := outcomes[name].latest
				So(latest[types.MetricBalance][symbol], ShouldEqual,
					1-latest[types.MetricNetFraction][symbol])
			}

			for _, metric := range metrics {
				So(outcomes["fast pump"].peak[metric][symbol], ShouldEqual,
					outcomes["slow cadence lift"].peak[metric][symbol])
				So(outcomes["fast pump"].latest[metric][symbol], ShouldEqual,
					outcomes["slow cadence lift"].latest[metric][symbol])
			}

			for _, metric := range []types.MetricType{
				types.MetricDrive, types.MetricStrength, types.MetricNetFraction,
			} {
				So(outcomes["fast pump"].peak[metric][symbol], ShouldEqual,
					outcomes["fast dump"].peak[metric][symbol])
			}

			So(outcomes["baseline"].latest[types.MetricStarvation][symbol],
				ShouldEqual, 0.0)
			So(outcomes["compression"].latest[types.MetricStarvation][symbol],
				ShouldEqual, 0.0)
		}

		for _, phase := range []metricValues{
			outcomes["isolated pump"].peak, outcomes["isolated pump"].latest,
		} {
			So(phase[types.MetricDrive]["SIM1/USD"], ShouldBeGreaterThan, 0)
			So(phase[types.MetricNet]["SIM1/USD"], ShouldBeGreaterThan, 0)

			for _, control := range []string{"SIM2/USD", "SIM3/USD"} {
				So(phase[types.MetricDrive][control], ShouldEqual, 0.0)
				So(phase[types.MetricAbsorption][control], ShouldEqual, 0.0)
				So(phase[types.MetricBalance][control], ShouldEqual,
					phase[types.MetricStrength][control])
				So(phase[types.MetricDrive]["SIM1/USD"], ShouldBeGreaterThan,
					phase[types.MetricDrive][control])
			}
		}
	})

	Convey("Given sparse startup cuts before an executable touch exists", t, func() {
		signal := cvd.NewSignal(t.Context(), make(chan []byte, 1))
		observedAt := time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)
		trade := kraken.TradeData{
			Symbol:    "SIM1/USD",
			Side:      "buy",
			Price:     *decimal.NewFromFloat64(100.01),
			Qty:       10,
			Timestamp: observedAt,
		}

		Convey("A trade should wait rather than fabricate a midpoint", func() {
			measurements, err := signal.Calculate(&types.MarketFrame{
				Trades: []kraken.TradeData{trade},
			})

			So(err, ShouldBeNil)
			So(measurements, ShouldBeEmpty)
		})

		Convey("A later complete touch should support a trade-only cut", func() {
			_, err := signal.Calculate(&types.MarketFrame{
				Tickers: []kraken.TickerData{
					{
						Symbol:    "SIM1/USD",
						Bid:       decimal.NewFromFloat64(99.99),
						Ask:       decimal.NewFromFloat64(100.01),
						Timestamp: observedAt,
					},
				},
			})
			So(err, ShouldBeNil)

			measurements, err := signal.Calculate(&types.MarketFrame{
				Trades: []kraken.TradeData{trade},
			})

			So(err, ShouldBeNil)
			So(measurements, ShouldNotBeEmpty)
		})

		Convey("A present but crossed touch should remain an explicit error", func() {
			_, err := signal.Calculate(&types.MarketFrame{
				Tickers: []kraken.TickerData{
					{
						Symbol:    "SIM1/USD",
						Bid:       decimal.NewFromFloat64(100.01),
						Ask:       decimal.NewFromFloat64(99.99),
						Timestamp: observedAt,
					},
				},
			})

			So(err, ShouldNotBeNil)
		})
	})
}

/*
BenchmarkCalculate exercises repeated fixture-driven CVD transitions through
the complete production graph.
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
	b.ReportAllocs()

	for b.Loop() {
		if err := market.Apply(tests.MarketStep{
			Advance: 250 * time.Millisecond,
			Actions: []tests.MarketAction{
				{Kind: tests.MarketTrade, Symbol: "SIM1/USD", Side: "buy", Qty: 100},
				{Kind: tests.MarketRefill, Symbol: "SIM1/USD", Side: "sell", Qty: 100},
				{Kind: tests.MarketMoveMid, Symbol: "SIM1/USD", Ticks: 1},
			},
		}, wired.Crypto.Step); err != nil {
			b.Fatal(err)
		}
	}
}
