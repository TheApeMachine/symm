package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestAverageRadarAxes(t *testing.T) {
	Convey("Given per-symbol regime radar axes", t, func() {
		Convey("It should average each axis while skipping zero values", func() {
			averaged := AverageRadarAxes(map[string]map[string]float64{
				"BTC/EUR": {
					RegimeAxisVolatility: 0.8,
					RegimeAxisTrend:      0.6,
					RegimeAxisBullish:    0.6,
					RegimeAxisBearish:    0,
					RegimeAxisChoppiness: 0.2,
				},
				"ETH/EUR": {
					RegimeAxisVolatility: 0,
					RegimeAxisTrend:      0.4,
					RegimeAxisBullish:    0,
					RegimeAxisBearish:    0.4,
					RegimeAxisChoppiness: 0.8,
				},
			})

			So(averaged[RegimeAxisVolatility], ShouldEqual, 0.8)
			So(averaged[RegimeAxisTrend], ShouldEqual, 0.5)
			So(averaged[RegimeAxisBullish], ShouldEqual, 0.6)
			So(averaged[RegimeAxisBearish], ShouldEqual, 0.4)
			So(averaged[RegimeAxisChoppiness], ShouldEqual, 0.5)
		})

		Convey("It should leave an axis at zero when every symbol is zero on that axis", func() {
			averaged := AverageRadarAxes(map[string]map[string]float64{
				"BTC/EUR": {RegimeAxisTrend: 0.5},
				"ETH/EUR": {RegimeAxisTrend: 0},
			})

			So(averaged[RegimeAxisTrend], ShouldEqual, 0.5)
			So(averaged[RegimeAxisVolatility], ShouldEqual, 0)
		})
	})
}

func TestMajorityRegime(t *testing.T) {
	Convey("Given per-symbol regime features", t, func() {
		majority := MajorityRegime(map[string]RegimeFeatures{
			"BTC/EUR": {Regime: types.RegimeTrending},
			"ETH/EUR": {Regime: types.RegimeChoppy},
			"SOL/EUR": {Regime: types.RegimeTrending},
			"XRP/EUR": {Regime: types.RegimeNone},
		})

		Convey("It should ignore RegimeNone and pick the plurality winner", func() {
			So(majority, ShouldEqual, types.RegimeTrending)
		})
	})
}
