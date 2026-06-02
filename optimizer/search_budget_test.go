package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestDeriveSearchBudget(t *testing.T) {
	convey.Convey("Given a profile and replay tape", t, func() {
		profile := Profile{ctx: context.Background()}
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar, SNR: 1, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceSentiment,
				Category: perspectives.CategoryRiskOnSurge, SNR: 2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion, SNR: 3, Last: 110,
			},
		}

		for _, row := range rows {
			profile.Add(row)
		}

		tape := PrecompileTape(rows)
		budget := DeriveSearchBudget(&profile, tape, 2)

		convey.Convey("It should derive limits from tape breadth and workers", func() {
			convey.So(budget.BeamWidth, convey.ShouldEqual, 6)
			convey.So(budget.MaxReasoningSteps, convey.ShouldEqual, 3)
			convey.So(budget.MinChainSupport, convey.ShouldBeGreaterThanOrEqualTo, 2)
			convey.So(budget.ReentryTickCooldown, convey.ShouldBeGreaterThan, 0)
			convey.So(budget.ComplexityPenalty, convey.ShouldEqual, 0)
		})
	})
}

func TestDeriveMeasurementSampleCap(t *testing.T) {
	convey.Convey("Given a large capture file row count", t, func() {
		cap := DeriveMeasurementSampleCap(1000000, 8)

		convey.Convey("It should subsample with sqrt scaling", func() {
			convey.So(cap, convey.ShouldBeLessThan, 1000000)
			convey.So(cap, convey.ShouldBeGreaterThan, 1000)
		})
	})
}

func TestDeriveReentryTickCooldown(t *testing.T) {
	convey.Convey("Given tick and category counts", t, func() {
		convey.Convey("It should scale cooldown with tape density", func() {
			convey.So(deriveReentryTickCooldown(1000, 10), convey.ShouldEqual, 10)
			convey.So(deriveReentryTickCooldown(5, 3), convey.ShouldEqual, 1)
		})
	})
}
