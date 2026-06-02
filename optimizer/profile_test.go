package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestProfilePrepareCache(t *testing.T) {
	convey.Convey("Given replay rows with mixed categories", t, func() {
		profile := Profile{ctx: context.Background()}
		profile.Add(perspectives.Measurement{
			Category:   perspectives.CategoryLaminar,
			SNR:        1,
			Confidence: 0.2,
		})
		profile.Add(perspectives.Measurement{
			Category:   perspectives.CategoryLaminar,
			SNR:        3,
			Confidence: 0.8,
		})
		profile.Add(perspectives.Measurement{
			Category:   perspectives.CategoryExhaustion,
			SNR:        2,
			Confidence: 0.5,
		})

		profile.PrepareCache()

		convey.Convey("It should serve quantiles from cached sorted values", func() {
			convey.So(
				profile.Quantile(
					perspectives.CategoryLaminar,
					perspectives.UnitSNR,
					0.5,
				),
				convey.ShouldEqual,
				3,
			)
			convey.So(profile.Categories(), convey.ShouldHaveLength, 2)
		})

		convey.Convey("It should count gate passes without rescanning rows", func() {
			convey.So(profile.GatePassCount(
				perspectives.CategoryLaminar,
				perspectives.UnitSNR,
				perspectives.ConditionIsGreaterThanOrEqual,
				2,
			), convey.ShouldEqual, 1)
		})
	})
}

func TestPrecompileTapeMatchesLiveReplay(t *testing.T) {
	convey.Convey("Given the same measurement tape", t, func() {
		ctx := context.Background()
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      2, Last: 110,
			},
		}
		branches := perspectives.BranchList{
			{
				Category:    perspectives.CategoryLaminar,
				Observation: perspectives.ObservationNotHolding,
				Condition:   perspectives.ConditionIsGreaterThanOrEqual,
				Unit:        perspectives.UnitSNR,
				Value:       1, ValueSet: true,
				Action: perspectives.Action{Type: perspectives.ActionLimit},
			},
			{
				Category:    perspectives.CategoryExhaustion,
				Observation: perspectives.ObservationHolding,
				Condition:   perspectives.ConditionIsGreaterThanOrEqual,
				Unit:        perspectives.UnitSNR,
				Value:       1, ValueSet: true,
				Action: perspectives.Action{Type: perspectives.ActionSettlePosition},
			},
		}
		tape := PrecompileTape(rows)
		live := NewReplaySimulation(ctx, branches, rows).Score()
		cached := NewReplaySimulationWithTape(ctx, branches, tape).Score()

		convey.Convey("It should score identically against a precompiled tape", func() {
			convey.So(cached, convey.ShouldEqual, live)
		})
	})
}
