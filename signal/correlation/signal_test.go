package correlation_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
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

	Convey("Given cohort, isolated, and adverse market tapes", t, func() {
		proofs := []struct {
			name  string
			state tests.MarketState
			focus []string
		}{
			{"baseline", tests.MarketStateBaseline, nil},
			{"fast pump", tests.MarketStateFastPump, nil},
			{
				"isolated pump", tests.MarketStateFastPump,
				[]string{"SIM1/USD"},
			},
		}
		outcomes := make(map[string]marketOutcome, len(proofs))

		for _, proof := range proofs {
			market := tests.NewMarket(t.Context(), len(symbols))
			wired, err := stack.NewBooter(t.Context()).Test(market)
			So(err, ShouldBeNil)
			So(market.Warmup(tests.Idle), ShouldBeNil)
			measurements := []*types.Measurement{}
			So(market.Transition(proof.state, func() error {
				thesis := wired.Thesis

			thesis.Measurements.Range(func(_, value any) bool {
				for _, measurement := range value.([]*types.Measurement) {
					if measurement.Source != types.SourceCorrelation {
						continue
					}

					So(measurement.ValidateStruct(), ShouldBeNil)

					measurement.EachMetric(func(
						_ types.MetricType, _ types.MeasurementSide, sample types.MetricSample,
					) bool {
						So(math.IsNaN(sample.Raw), ShouldBeFalse)
						return true
					})
					measurements = append(measurements, measurement)
				}
				return true
			})

				return nil
			}, proof.focus...), ShouldBeNil)

			outcomes[proof.name] = marketOutcome{
				peak: tests.PeakMeasurements(
					measurements, types.SourceCorrelation, metrics,
				),
				latest: tests.LatestMeasurements(
					measurements, types.SourceCorrelation, metrics,
				),
			}
			So(wired.Close(), ShouldBeNil)
			market.Close()
		}

		Convey("Then cohort lift lights herd above baseline", func() {
			pump := outcomes["fast pump"].peak
			baseline := outcomes["baseline"].peak

			for _, symbol := range symbols {
				So(pump[types.MetricHerdScore][symbol], ShouldBeGreaterThan, 0)
				So(
					pump[types.MetricHerdScore][symbol],
					ShouldBeGreaterThan,
					baseline[types.MetricHerdScore][symbol],
				)
			}
		})

		Convey("Then isolated lift lights alpha on the subject", func() {
			isolated := outcomes["isolated pump"].peak
			So(isolated[types.MetricAlphaScore]["SIM1/USD"], ShouldBeGreaterThan, 0)
			So(
				isolated[types.MetricAlphaScore]["SIM1/USD"],
				ShouldBeGreaterThan,
				outcomes["fast pump"].peak[types.MetricAlphaScore]["SIM1/USD"],
			)
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

	if err := market.Warmup(tests.Idle); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for b.Loop() {
		if err := market.Transition(tests.MarketStateFastPump, func() error {
			_ = wired.Thesis
			return err
		}); err != nil {
			b.Fatal(err)
		}
	}
}
