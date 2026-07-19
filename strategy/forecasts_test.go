package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestSelectForecastsPrefersNewerEpoch(t *testing.T) {
	Convey("Given two forecasts for one symbol", t, func() {
		rows := []types.Forecasts{
			{Symbol: "MATIC/USD", SourceEpoch: 1, Ready: true, Calibrated: true, FrictionReady: true, ExpectedReturn: 0.01},
			{Symbol: "MATIC/USD", SourceEpoch: 2, Ready: true, Calibrated: true, FrictionReady: true, ExpectedReturn: 0.09},
		}

		Convey("When selectForecasts runs", func() {
			selected := selectForecasts(rows)

			Convey("Then the newer epoch wins", func() {
				So(selected["MATIC/USD"].SourceEpoch, ShouldEqual, 2)
				So(selected["MATIC/USD"].ExpectedReturn, ShouldEqual, 0.09)
			})
		})
	})
}
