package trader

import (
	"fmt"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestCryptoPublishEnginePulse(t *testing.T) {
	convey.Convey("Given readings with entry candidates", t, func() {
		crypto := newTestCrypto()

		for index := range 4 {
			crypto.record(traderMeasurement(
				fmt.Sprintf("COIN%d/EUR", index),
				perspectives.SourceCVD,
				perspectives.CategoryAggressiveDrive,
				2.0,
			))
		}

		crypto.record(traderMeasurement(
			"LEADER/EUR",
			perspectives.SourceCVD,
			perspectives.CategoryAggressiveDrive,
			4.0,
		))
		crypto.refreshCrossSection()
		crypto.publishEnginePulse()

		snapshot := crypto.ensureCrossSection()

		convey.Convey("It should expose cross-section forecast aggregates", func() {
			convey.So(snapshot.ForecastSymbols, convey.ShouldBeGreaterThan, 0)
			convey.So(snapshot.AvgPredictionMult, convey.ShouldBeGreaterThan, 0)
		})
	})
}
