package leadlag_test

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
TestCalculate proves lead-lag reports complete temporal evidence for cohort and
isolated moves through the complete production boot graph.
*/
func TestCalculate(t *testing.T) {
	metrics := []types.MetricType{
		types.MetricCorrelation,
		types.MetricSignedCorrelation,
		types.MetricSignedContempCorrelation,
		types.MetricSignedLagCorrelation,
		types.MetricLagFraction,
		types.MetricSampleSupport,
		types.MetricInefficient,
		types.MetricSync,
		types.MetricDecoupled,
		types.MetricStall,
		types.MetricStrength,
	}
	families := []types.MetricType{
		types.MetricInefficient,
		types.MetricSync,
		types.MetricDecoupled,
		types.MetricStall,
	}

	Convey("Given cohort and isolated market tapes", t, func() {
		outcomes := map[string]map[types.MetricType]map[string]float64{}

		for _, proof := range []struct {
			name    string
			symbols []string
		}{
			{"cohort pump", nil},
			{"isolated pump", []string{"SIM1/USD"}},
		} {
			market := tests.NewMarket(t.Context(), 3)
			So(market.Bootstrap(), ShouldBeNil)
			wired, err := stack.NewBooter(t.Context()).Test(market)
			So(err, ShouldBeNil)
			So(market.Warmup(wired.Crypto.Tick), ShouldBeNil)
			measurements := []*types.Measurement{}
			So(market.Transition(tests.MarketStateFastPump, func() error {
				err := wired.Crypto.Tick()
				measurements = append(measurements, wired.Crypto.Thesis().Measurements...)
				return err
			}, proof.symbols...), ShouldBeNil)
			outcomes[proof.name] = utils.PeakMeasurements(
				measurements,
				types.SourceLeadLag,
				metrics,
			)
			wired.Close()
			market.Close()
		}

		for _, values := range outcomes {
			for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
				strength := 0.0

				for _, metric := range metrics {
					So(values[metric], ShouldContainKey, symbol)
					So(math.IsNaN(values[metric][symbol]), ShouldBeFalse)
				}

				So(values[types.MetricCorrelation][symbol], ShouldBeBetweenOrEqual, 0, 1)
				So(values[types.MetricLagFraction][symbol], ShouldBeBetweenOrEqual, 0, 1)
				So(values[types.MetricSampleSupport][symbol], ShouldBeBetweenOrEqual, 0, 1)

				for _, metric := range families {
					So(values[metric][symbol], ShouldBeGreaterThanOrEqualTo, 0)
					strength = max(strength, values[metric][symbol])
				}

				So(values[types.MetricStrength][symbol], ShouldEqual, strength)
			}
		}

		for _, symbol := range []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"} {
			So(outcomes["cohort pump"][types.MetricSync][symbol], ShouldBeGreaterThan, 0)
		}
		So(outcomes["isolated pump"][types.MetricStrength]["SIM1/USD"], ShouldBeGreaterThan, 0)
	})
}
