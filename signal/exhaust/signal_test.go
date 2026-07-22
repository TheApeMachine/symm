package exhaust_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
TestCalculate proves an advancing tape and its rejection produce complete
exhaustion state through the production boot graph.
*/
func TestCalculate(t *testing.T) {
	metrics := []types.MetricType{
		types.MetricMechanical,
		types.MetricThermal,
		types.MetricFragile,
		types.MetricReversal,
		types.MetricUrgency,
		types.MetricStrength,
		types.MetricValue,
		types.MetricCategory,
	}

	Convey("Given a production-booted advancing market", t, func() {
		market := tests.NewMarket(t.Context(), 3)
		So(market.Bootstrap(), ShouldBeNil)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() { wired.Close(); market.Close() })
		So(market.Warmup(wired.Crypto.Tick), ShouldBeNil)
		So(market.Transition(tests.MarketStateFastPump, wired.Crypto.Tick), ShouldBeNil)
		advance := utils.LatestMeasurements(
			wired.Crypto.Thesis().Measurements,
			types.SourceExhaustion,
			metrics,
		)

		Convey("When the advance rejects", func() {
			measurements := []*types.Measurement{}
			So(market.Transition(tests.MarketStateFastDump, func() error {
				err := wired.Crypto.Tick()
				measurements = append(measurements, wired.Crypto.Thesis().Measurements...)
				return err
			}), ShouldBeNil)
			rejection := utils.PeakMeasurements(
				measurements,
				types.SourceExhaustion,
				metrics,
			)

			for _, symbol := range market.Symbols {
				for _, values := range []map[types.MetricType]map[string]float64{
					advance,
					rejection,
				} {
					for _, metric := range metrics {
						So(values[metric], ShouldContainKey, symbol)
					}

					for _, metric := range metrics {
						So(values[metric][symbol], ShouldBeGreaterThanOrEqualTo, 0)
					}
				}

				So(rejection[types.MetricReversal][symbol], ShouldBeGreaterThan,
					advance[types.MetricReversal][symbol])
				So(rejection[types.MetricUrgency][symbol], ShouldEqual,
					rejection[types.MetricStrength][symbol])
				So(rejection[types.MetricValue][symbol], ShouldEqual,
					rejection[types.MetricStrength][symbol])
			}
		})
	})
}
