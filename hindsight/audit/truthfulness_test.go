package audit

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestTruthfulnessAudit(t *testing.T) {
	Convey("Given honest market measurements", t, func() {
		now := time.Now()
		honestTrade := data.NewMeasurement(
			1, "BTC/USD", "spot:trade", 1, 1,
			&data.StringEntry{Key: "side", Value: "buy"},
		)
		honestTrade.At = now
		honestTrade = honestTrade.Write(
			data.NewMetric("price", 60000.0, data.UnitPrice, data.TimescaleInstantaneous),
			data.NewMetric("qty", 1.5, data.UnitQuantity, data.TimescaleInstantaneous),
		)

		honestCVD := data.NewMeasurement(1, "BTC/USD", "cvd", 1, 1)
		honestCVD = honestCVD.Write(
			data.NewMetric("signed_net_fraction", 0.75, data.UnitDimensionless, data.TimescaleTick),
		)

		Convey("When auditing truthful measurements", func() {
			report := AnalyzeTruthfulness([]*data.Measurement{honestTrade, honestCVD})

			So(report.TotalChecked, ShouldEqual, 2)
			So(report.ViolationsCount, ShouldEqual, 0)
			So(report.ZeroFilledMidpoints, ShouldEqual, 0)
			So(report.Passed, ShouldBeTrue)
		})

		Convey("When auditing fraudulent zero-filled midpoints and non-positive prices", func() {
			badBook := data.NewMeasurement(1, "BTC/USD", "depthflow", 2, 2)
			badBook = badBook.Write(
				data.NewMetric("midpoint", 0.0, data.UnitPrice, data.TimescaleTick),
			)

			badTrade := data.NewMeasurement(
				1, "BTC/USD", "spot:trade", 3, 3,
				&data.StringEntry{Key: "side", Value: "invalid_side"},
			)
			badTrade = badTrade.Write(
				data.NewMetric("price", -100.0, data.UnitPrice, data.TimescaleInstantaneous),
				data.NewMetric("qty", 0.0, data.UnitQuantity, data.TimescaleInstantaneous),
			)

			badCVD := data.NewMeasurement(1, "BTC/USD", "cvd", 4, 4)
			badCVD = badCVD.Write(
				data.NewMetric("signed_net_fraction", 2.5, data.UnitDimensionless, data.TimescaleTick),
			)

			report := AnalyzeTruthfulness([]*data.Measurement{badBook, badTrade, badCVD})

			So(report.TotalChecked, ShouldEqual, 3)
			So(report.ViolationsCount, ShouldBeGreaterThanOrEqualTo, 3)
			So(report.ZeroFilledMidpoints, ShouldEqual, 1)
			So(report.Passed, ShouldBeFalse)
		})
	})
}
