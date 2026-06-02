package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestDeriveMaxReasoningSteps(t *testing.T) {
	convey.Convey("Given a profile and replay tape", t, func() {
		profile := Profile{ctx: context.Background()}
		rows := []perspectives.Measurement{
			{Symbol: "BTC/EUR", Source: perspectives.SourceFluid, Category: perspectives.CategoryLaminar, SNR: 2, Last: 100},
			{Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion, Category: perspectives.CategoryExhaustion, SNR: 3, Last: 110},
		}

		for _, row := range rows {
			profile.Add(row)
		}

		tape := PrecompileTape(rows)
		steps := deriveMaxReasoningSteps(tape, &profile)

		convey.Convey("It should bound reasoning depth from tape structure", func() {
			convey.So(steps, convey.ShouldBeGreaterThanOrEqualTo, 1)
			convey.So(steps, convey.ShouldBeLessThanOrEqualTo, 6)
		})
	})
}

func TestDeriveReentryTickCooldownFromTape(t *testing.T) {
	convey.Convey("Given dense and sparse tapes", t, func() {
		convey.Convey("It should increase cooldown with tick density", func() {
			convey.So(deriveReentryTickCooldown(1000, 10), convey.ShouldEqual, 10)
			convey.So(deriveReentryTickCooldown(5, 3), convey.ShouldEqual, 1)
		})
	})
}

func TestDeriveMeasurementSampleCapFromFile(t *testing.T) {
	convey.Convey("Given a large row count and worker count", t, func() {
		cap := deriveMeasurementSampleCap(1000000, 8)

		convey.Convey("It should subsample with sqrt scaling", func() {
			convey.So(cap, convey.ShouldBeLessThan, 1000000)
			convey.So(cap, convey.ShouldBeGreaterThan, 1000)
		})
	})
}
