package budget

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/profile"
	"github.com/theapemachine/symm/optimizer/replay"
)

func TestDeriveMaxReasoningSteps(t *testing.T) {
	convey.Convey("Given a profile and replay tape", t, func() {
		measurementProfile := profile.NewProfile(context.Background())
		rows := []perspectives.Measurement{
			{Symbol: "BTC/EUR", Source: perspectives.SourceFluid, Category: perspectives.CategoryLaminar, SNR: 2, Last: 100},
			{Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion, Category: perspectives.CategoryExhaustion, SNR: 3, Last: 110},
		}

		for _, row := range rows {
			measurementProfile.Add(row)
		}

		tape := replay.PrecompileTape(rows)
		steps := deriveMaxReasoningSteps(tape, measurementProfile)

		convey.Convey("It should bound reasoning depth from tape structure", func() {
			convey.So(steps, convey.ShouldBeGreaterThanOrEqualTo, 1)
			convey.So(steps, convey.ShouldBeLessThanOrEqualTo, 6)
		})
	})
}

func TestDeriveReentryTickCooldownFromTape(t *testing.T) {
	convey.Convey("Given tick and category counts", t, func() {
		convey.Convey("It should scale cooldown with tape density", func() {
			convey.So(deriveReentryTickCooldown(1000, 10), convey.ShouldEqual, 10)
		})
	})
}

func TestDeriveMinRoundTrips(t *testing.T) {
	convey.Convey("Given the minimum-round-trips significance floor", t, func() {
		convey.Convey("It floors small tapes so single-trade flukes can't win", func() {
			convey.So(deriveMinRoundTrips(100), convey.ShouldEqual, minRoundTripFloor)
		})

		convey.Convey("It caps large tapes instead of demanding ever more trades", func() {
			// sqrt(30467)/3 ~= 59, which without the cap would force the search
			// toward the busiest, most friction-heavy strategies.
			convey.So(deriveMinRoundTrips(30467), convey.ShouldEqual, minRoundTripCeil)
			convey.So(deriveMinRoundTrips(1_000_000), convey.ShouldEqual, minRoundTripCeil)
		})

		convey.Convey("It scales within the band for mid-size tapes", func() {
			trips := deriveMinRoundTrips(6090) // sqrt/3 ~= 26
			convey.So(trips, convey.ShouldBeBetweenOrEqual, minRoundTripFloor, minRoundTripCeil)
		})
	})
}

func TestDeriveMeasurementSampleCapFromFile(t *testing.T) {
	convey.Convey("Given row count and workers", t, func() {
		cap := deriveMeasurementSampleCap(10000, 8)

		convey.Convey("It should subsample large files", func() {
			convey.So(cap, convey.ShouldBeLessThan, 10000)
			convey.So(cap, convey.ShouldBeGreaterThan, 100)
		})
	})
}
