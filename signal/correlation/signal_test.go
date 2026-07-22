package correlation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

type metricValues = map[types.MetricType]map[string]float64

/*
marketOutcome retains transition peaks and settlement for cohort-relation claims.
*/
type marketOutcome struct {
	peak   metricValues
	latest metricValues
}

/*
TestCalculate proves correlation distinguishes cohort herd, isolated alpha, and
stress through the production boot graph on fixture tapes.
*/
func TestCalculate(t *testing.T) {
	metrics := []types.MetricType{
		types.MetricCorrelation,
		types.MetricSigned,
		types.MetricRelativeEnergy,
		types.MetricHerdScore,
		types.MetricAlphaScore,
		types.MetricNoiseScore,
		types.MetricStressScore,
		types.MetricPeakScore,
		types.MetricStrength,
	}
	symbols := []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"}

	Convey("Given materially distinct cohort and divergence tapes", t, func() {
		proofs := []struct {
			name   string
			states []tests.MarketState
			focus  []string
		}{
			{"baseline", []tests.MarketState{tests.MarketStateBaseline}, nil},
			{"fast pump", []tests.MarketState{tests.MarketStateFastPump}, nil},
			{"slow pump", []tests.MarketState{tests.MarketStateSlowPump}, nil},
			{"fast dump", []tests.MarketState{tests.MarketStateFastDump}, nil},
			{"slow dump", []tests.MarketState{tests.MarketStateSlowDump}, nil},
			{"absorption", []tests.MarketState{tests.MarketStateVolumeAbsorption}, nil},
			{"compression", []tests.MarketState{tests.MarketStateSpreadCompression}, nil},
			{
				"isolated pump", []tests.MarketState{tests.MarketStateFastPump},
				[]string{"SIM1/USD"},
			},
			{
				"isolated dump", []tests.MarketState{tests.MarketStateFastDump},
				[]string{"SIM1/USD"},
			},
			{"leader follower", []tests.MarketState{tests.MarketStateLeaderFollower}, nil},
			{"adverse divergence", []tests.MarketState{tests.MarketStateAdverseDivergence}, nil},
			{"fast rejection", []tests.MarketState{
				tests.MarketStateFastPump, tests.MarketStateFastDump,
			}, nil},
			{"slow rejection", []tests.MarketState{
				tests.MarketStateFastPump, tests.MarketStateSlowDump,
			}, nil},
			{"reversal", []tests.MarketState{
				tests.MarketStateSlowDump, tests.MarketStateFastPump,
			}, nil},
		}
		outcomes := make(map[string]marketOutcome, len(proofs))

		for _, proof := range proofs {
			market := tests.NewMarket(t.Context(), len(symbols))
			wired, err := stack.NewBooter(t.Context()).Test(market)
			So(err, ShouldBeNil)
			So(market.Warmup(tests.Consume(wired.Crypto.Tick)), ShouldBeNil)
			for _, state := range proof.states[:len(proof.states)-1] {
				So(market.Transition(state,
					tests.Consume(wired.Crypto.Tick), proof.focus...), ShouldBeNil)
			}

			cuts := 0
			missingScores := 0
			measurements := []*types.Measurement{}
			So(market.Transition(proof.states[len(proof.states)-1], func() error {
				cuts++
				thesis, err := wired.Crypto.Tick()

				if err != nil {
					return err
				}

				stepMetrics := make(map[string]map[types.MetricType]bool, len(symbols))

				for _, measurement := range thesis.Measurements {
					if measurement.Source != types.SourceCorrelation {
						continue
					}

					So(measurement.ValidateStruct(), ShouldBeNil)
					So(math.IsNaN(measurement.Raw), ShouldBeFalse)
					So(math.IsInf(measurement.Raw, 0), ShouldBeFalse)
					So(measurement.At, ShouldResemble, market.Now())
					So(measurement.Stream, ShouldEqual, types.Correlation)
					So(measurement.Unit, ShouldEqual, types.UnitDimensionless)
					So(measurement.Validity, ShouldResemble, types.MeasurementValidity{
						State:     types.ValidityValid,
						Readiness: types.ReadinessObservation,
					})

					switch measurement.Metric {
					case types.MetricSigned:
						So(measurement.Raw, ShouldBeBetweenOrEqual, -1.0, 1.0)
					case types.MetricRelativeEnergy:
						So(measurement.Raw, ShouldBeGreaterThanOrEqualTo, 0.0)
					case types.MetricCorrelation, types.MetricHerdScore,
						types.MetricAlphaScore, types.MetricNoiseScore,
						types.MetricStressScore, types.MetricPeakScore,
						types.MetricStrength:
						So(measurement.Raw, ShouldBeBetweenOrEqual, 0.0, 1.0)
					default:
						t.Fatalf("unexpected correlation metric %q", measurement.Metric)
					}

					if stepMetrics[measurement.Symbol] == nil {
						stepMetrics[measurement.Symbol] = make(map[types.MetricType]bool)
					}

					So(stepMetrics[measurement.Symbol][measurement.Metric], ShouldBeFalse)
					stepMetrics[measurement.Symbol][measurement.Metric] = true
					measurements = append(measurements, measurement)
				}

				expectedSymbols := len(symbols)

				if proof.name == "absorption" && cuts == 10 {
					// Both SIM1 peer relations are exactly null at this fixture cut,
					// so the zero-strength correlation contract emits no row.
					expectedSymbols--
					missingScores++
					So(stepMetrics["SIM1/USD"], ShouldBeNil)
				}

				So(stepMetrics, ShouldHaveLength, expectedSymbols)

				for _, observed := range stepMetrics {
					So(len(observed), ShouldEqual, len(metrics))
				}

				return nil
			}, proof.focus...), ShouldBeNil)
			So(measurements, ShouldHaveLength,
				(cuts*len(symbols)-missingScores)*len(metrics))

			outcomes[proof.name] = marketOutcome{
				peak: utils.PeakMeasurements(
					measurements, types.SourceCorrelation, metrics,
				),
				latest: utils.LatestMeasurements(
					measurements, types.SourceCorrelation, metrics,
				),
			}
			So(wired.Close(), ShouldBeNil)
			market.Close()
		}

		Convey("Then the scores separate herd, alpha, lag, and adverse stress", func() {
			So(len(outcomes), ShouldEqual, len(proofs))
			baseline := outcomes["baseline"].latest

			for _, outcome := range outcomes {
				for _, symbol := range symbols {
					latest := outcome.latest
					signed := latest[types.MetricSigned][symbol]
					correlation := latest[types.MetricCorrelation][symbol]
					relativeEnergy := latest[types.MetricRelativeEnergy][symbol]
					excess := math.Max(0, relativeEnergy-1)
					deficit := math.Max(0, 1-relativeEnergy)
					herd := math.Max(0, signed) / (1 + excess)
					alpha := excess / (1 + excess) / (1 + math.Max(0, signed))
					noise := math.Max(0, 1-correlation) / (1 + excess + deficit)
					stress := math.Max(0, -signed)
					strength := max(max(herd, alpha), max(noise, stress))

					So(latest[types.MetricHerdScore][symbol], ShouldEqual, herd)
					So(latest[types.MetricAlphaScore][symbol], ShouldEqual, alpha)
					So(latest[types.MetricNoiseScore][symbol], ShouldEqual, noise)
					So(latest[types.MetricStressScore][symbol], ShouldEqual, stress)
					So(latest[types.MetricStrength][symbol], ShouldEqual, strength)
					So(latest[types.MetricPeakScore][symbol], ShouldEqual, strength)
					So(outcome.peak[types.MetricPeakScore][symbol], ShouldEqual,
						outcome.peak[types.MetricStrength][symbol])
				}
			}

			for _, name := range []string{
				"fast pump", "slow pump", "fast dump", "slow dump",
				"fast rejection", "slow rejection", "reversal",
			} {
				cohort := outcomes[name].latest

				for _, symbol := range symbols {
					So(cohort[types.MetricSigned][symbol]/
						math.Abs(cohort[types.MetricSigned][symbol]), ShouldEqual, 1.0)
					So(cohort[types.MetricStressScore][symbol], ShouldEqual, 0)
					So(cohort[types.MetricCorrelation][symbol], ShouldBeGreaterThan,
						baseline[types.MetricCorrelation][symbol])
					So(cohort[types.MetricHerdScore][symbol], ShouldBeGreaterThan,
						baseline[types.MetricHerdScore][symbol])
					So(cohort[types.MetricNoiseScore][symbol], ShouldBeLessThan,
						baseline[types.MetricNoiseScore][symbol])
				}
			}

			for _, name := range []string{"absorption", "compression"} {
				quiet := outcomes[name].latest

				for _, symbol := range symbols {
					for _, cohortName := range []string{"fast pump", "fast dump"} {
						cohort := outcomes[cohortName].latest
						So(quiet[types.MetricCorrelation][symbol], ShouldBeLessThan,
							cohort[types.MetricCorrelation][symbol])
						So(quiet[types.MetricHerdScore][symbol], ShouldBeLessThan,
							cohort[types.MetricHerdScore][symbol])
						So(quiet[types.MetricNoiseScore][symbol], ShouldBeGreaterThan,
							cohort[types.MetricNoiseScore][symbol])
					}
				}
			}

			for isolatedName, cohortName := range map[string]string{
				"isolated pump": "fast pump",
				"isolated dump": "fast dump",
			} {
				isolated := outcomes[isolatedName].latest
				cohort := outcomes[cohortName].latest
				subject := "SIM1/USD"

				So(isolated[types.MetricAlphaScore][subject], ShouldBeGreaterThan,
					cohort[types.MetricAlphaScore][subject])
				So(isolated[types.MetricHerdScore][subject], ShouldBeLessThan,
					cohort[types.MetricHerdScore][subject])
				So(isolated[types.MetricRelativeEnergy][subject], ShouldBeGreaterThan, 1)

				for _, peer := range symbols[1:] {
					So(isolated[types.MetricAlphaScore][peer], ShouldEqual, 0)
					So(isolated[types.MetricHerdScore][peer], ShouldEqual, 0)
					So(isolated[types.MetricRelativeEnergy][peer], ShouldBeLessThan, 1)
					So(isolated[types.MetricCorrelation][subject], ShouldBeLessThan,
						isolated[types.MetricCorrelation][peer])
				}
			}

			leader := outcomes["leader follower"]

			for _, symbol := range symbols {
				So(leader.latest[types.MetricSigned][symbol]/
					math.Abs(leader.latest[types.MetricSigned][symbol]), ShouldEqual, 1.0)
				So(leader.latest[types.MetricStressScore][symbol], ShouldEqual, 0)
			}

			So(leader.peak[types.MetricRelativeEnergy]["SIM1/USD"], ShouldBeGreaterThan,
				leader.peak[types.MetricRelativeEnergy]["SIM2/USD"])
			So(leader.peak[types.MetricRelativeEnergy]["SIM1/USD"], ShouldBeGreaterThan,
				leader.peak[types.MetricRelativeEnergy]["SIM3/USD"])
			So(leader.peak[types.MetricAlphaScore]["SIM1/USD"], ShouldBeGreaterThan,
				leader.peak[types.MetricAlphaScore]["SIM2/USD"])
			So(leader.peak[types.MetricAlphaScore]["SIM1/USD"], ShouldBeGreaterThan,
				leader.peak[types.MetricAlphaScore]["SIM3/USD"])
			So(leader.latest[types.MetricAlphaScore]["SIM1/USD"], ShouldEqual, 0)
			So(leader.latest[types.MetricAlphaScore]["SIM2/USD"], ShouldEqual, 0)
			So(leader.latest[types.MetricAlphaScore]["SIM3/USD"], ShouldEqual,
				leader.latest[types.MetricStrength]["SIM3/USD"])

			adverse := outcomes["adverse divergence"]
			subject := "SIM1/USD"
			So(adverse.latest[types.MetricSigned][subject], ShouldEqual,
				-adverse.latest[types.MetricCorrelation][subject])
			So(adverse.latest[types.MetricHerdScore][subject], ShouldEqual, 0)
			So(adverse.latest[types.MetricStressScore][subject], ShouldEqual,
				adverse.latest[types.MetricCorrelation][subject])

			for _, peer := range symbols[1:] {
				So(adverse.latest[types.MetricSigned][peer]/
					math.Abs(adverse.latest[types.MetricSigned][peer]), ShouldEqual, 1.0)
				So(adverse.latest[types.MetricStressScore][peer], ShouldEqual, 0)
				So(adverse.latest[types.MetricCorrelation][subject], ShouldBeLessThan,
					adverse.latest[types.MetricCorrelation][peer])
				So(adverse.peak[types.MetricAlphaScore][subject], ShouldBeGreaterThan,
					adverse.peak[types.MetricAlphaScore][peer])
				So(adverse.peak[types.MetricStressScore][subject], ShouldBeGreaterThan,
					adverse.peak[types.MetricStressScore][peer])
			}
		})
	})
}

/*
BenchmarkCalculate measures correlation through the production Tick path.
*/
func BenchmarkCalculate(b *testing.B) {
	market := tests.NewMarket(b.Context(), 3)
	wired, err := stack.NewBooter(b.Context()).Test(market)

	if err != nil {
		b.Fatal(err)
	}

	defer wired.Close()
	defer market.Close()

	if err := market.Warmup(tests.Consume(wired.Crypto.Tick)); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for b.Loop() {
		if err := market.Transition(tests.MarketStateFastPump, func() error {
			_, err := wired.Crypto.Tick()
			return err
		}); err != nil {
			b.Fatal(err)
		}
	}
}
