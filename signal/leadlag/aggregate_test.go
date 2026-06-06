package leadlag

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestAggregateLeadlagReadings(t *testing.T) {
	Convey("Given per-symbol lead-lag readings", t, func() {
		aggregate, ok := aggregateLeadlagReadings([]leadlagGaugeReading{
			{
				measurement: types.Measurement{
					Category:   types.CategoryAnchorStall,
					Confidence: 0.8,
					SNR:        1.2,
					Strength:   0,
				},
				standout: 0.4,
			},
			{
				measurement: types.Measurement{
					Category:   types.CategoryDecoupledMove,
					Confidence: 0.4,
					SNR:        0,
					Strength:   0.6,
				},
				standout: 0.2,
			},
		})

		Convey("It should average non-zero telemetry and keep the clearest category", func() {
			So(ok, ShouldBeTrue)
			So(aggregate.measurement.Symbol, ShouldEqual, leadlagMarketSymbol)
			So(aggregate.measurement.Category, ShouldEqual, types.CategoryAnchorStall)
			So(aggregate.measurement.Confidence, ShouldAlmostEqual, 0.6, 0.0001)
			So(aggregate.measurement.SNR, ShouldEqual, 1.2)
			So(aggregate.measurement.Strength, ShouldEqual, 0.6)
		})
	})
}
