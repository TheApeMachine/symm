package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFinalizeMeasurementUsesComparableSNR(t *testing.T) {
	Convey("Given a cold score series", t, func() {
		ResetPlaybookScoreFloors()

		measurement := Measurement{
			Symbol:   "ETH/EUR",
			Source:   SourceDepthFlow,
			Category: CategorySpoofTrap,
		}

		Convey("It should keep SNR at zero during warmup instead of leaking raw strength", func() {
			finalized := FinalizeMeasurement(measurement, 90, "level1")

			So(finalized.Strength, ShouldEqual, 90)
			So(finalized.SNR, ShouldEqual, 0)
		})

		Convey("It should report sigma units once the floor has history", func() {
			ResetPlaybookScoreFloors()

			for index := 0; index < 24; index++ {
				baseline := 0.15 + float64(index%6)*0.01
				_ = FinalizeMeasurement(measurement, baseline, "level1")
			}

			spike := FinalizeMeasurement(measurement, 0.85, "level1")

			So(spike.SNR, ShouldBeGreaterThan, 1)
		})
	})
}

func TestScoreSeriesKey(t *testing.T) {
	Convey("Given source category stream and symbol", t, func() {
		key := ScoreSeriesKey(SourceFluid, CategoryTurbulent, "re", "BTC/EUR")

		Convey("It should encode every segment with positional labels", func() {
			So(key, ShouldEqual, "fluid:turbulent:stream:re:symbol:BTC/EUR")
		})

		Convey("It should distinguish empty stream from empty symbol", func() {
			withSymbol := ScoreSeriesKey(SourceFluid, CategoryTurbulent, "", "BTC/EUR")
			withStream := ScoreSeriesKey(SourceFluid, CategoryTurbulent, "BTC", "")

			So(withSymbol, ShouldNotEqual, withStream)
		})
	})
}
