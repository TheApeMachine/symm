package depthflow_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
TestCalculate proves bookflow measures loaded, thin, and retreating fixture
books through the complete production boot graph.
*/
func TestCalculate(t *testing.T) {
	metrics := []types.MetricType{
		types.MetricLoadedScore,
		types.MetricSpoofScore,
		types.MetricThinScore,
		types.MetricNeutralScore,
		types.MetricStrength,
		types.MetricValue,
	}
	families := metrics[:4]

	Convey("Given production-booted book conditions", t, func() {
		for _, proof := range []struct {
			name   string
			state  tests.MarketState
			metric types.MetricType
		}{
			{"loaded touch", tests.MarketStateLoadedLiquidity, types.MetricLoadedScore},
			{"directional book", tests.MarketStateFastPump, types.MetricNeutralScore},
		} {
			market := tests.NewMarket(t.Context(), 3)
			So(market.Bootstrap(), ShouldBeNil)
			wired, err := stack.NewBooter(t.Context()).Test(market)
			So(err, ShouldBeNil)
			So(market.Warmup(wired.Crypto.Tick), ShouldBeNil)
			subject := market.Symbols[0]
			measurements := []*types.Measurement{}
			So(market.Transition(proof.state, func() error {
				err := wired.Crypto.Tick()
				measurements = append(measurements, wired.Crypto.Thesis().Measurements...)
				return err
			}, subject), ShouldBeNil)
			values := utils.PeakMeasurements(
				measurements,
				types.SourceDepthFlow,
				metrics,
			)
			for _, metric := range metrics {
				So(values[metric], ShouldContainKey, subject)
			}

			for _, metric := range families {
				So(values[metric][subject], ShouldBeGreaterThanOrEqualTo, 0)
			}

			So(values[proof.metric][subject], ShouldBeGreaterThan, 0)
			So(values[types.MetricStrength][subject], ShouldBeGreaterThan, 0)
			So(values[types.MetricValue][subject], ShouldEqual,
				values[types.MetricStrength][subject])
			wired.Close()
			market.Close()
		}
	})
}
