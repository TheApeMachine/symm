package scan

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/profile"
	"github.com/theapemachine/symm/optimizer/progress"
	"github.com/theapemachine/symm/optimizer/replay"
	"github.com/theapemachine/symm/optimizer/types"
)

func TestScanSearchPooledScoringHaltsOnStagnation(t *testing.T) {
	convey.Convey("Given pooled scoring with reward stagnation enabled", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 1, 2, qpool.NewConfig())
		defer pool.Close()

		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2, Last: 100,
			},
		}
		measurementProfile := profile.NewProfile(ctx)
		measurementProfile.Add(rows[0])
		measurementProfile.PrepareCache()

		search := NewScanSearchWithTape(
			ctx,
			measurementProfile,
			rows,
			replay.PrecompileTape(rows),
			types.ScanOptions{
				Workers:   2,
				BeamWidth: 2,
				Pool:      pool,
			},
		)
		search.progress = progress.NewSearchProgress()
		search.haltPhaseOnStagnation = true

		entry := perspectives.BranchList{{
			Category:    perspectives.CategoryLaminar,
			Observation: perspectives.ObservationNotHolding,
			Condition:   perspectives.ConditionIsGreaterThanOrEqual,
			Unit:        perspectives.UnitSNR,
			Value:       1,
			ValueSet:    true,
			Action:      perspectives.Action{Type: perspectives.ActionLimit},
		}}
		exit := perspectives.BranchList{{
			Category:    perspectives.CategoryLaminar,
			Observation: perspectives.ObservationHolding,
			Condition:   perspectives.ConditionIsGreaterThanOrEqual,
			Unit:        perspectives.UnitSNR,
			Value:       1,
			ValueSet:    true,
			Action:      perspectives.Action{Type: perspectives.ActionMarket},
		}}
		branches := append(entry.Clone(), exit.Clone()...)

		before := search.candidates

		search.score(func(send func(scanCandidate) bool) {
			for range 32 {
				if !send(scanCandidate{branches: branches.Clone()}) {
					break
				}
			}
		})

		scored := search.candidates - before

		convey.Convey("It should stop after the stagnation window instead of scoring every candidate", func() {
			convey.So(scored, convey.ShouldBeLessThan, 32)
			convey.So(scored, convey.ShouldBeLessThanOrEqualTo, search.options.BeamWidth+search.options.Workers*2+1)
		})
	})
}

func TestEmitDeepeningExpansionsSkipsWhenStagnant(t *testing.T) {
	convey.Convey("Given a stagnant scan phase", t, func() {
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
		measurementProfile := profile.NewProfile(ctx)

		for _, row := range rows {
			measurementProfile.Add(row)
		}

		measurementProfile.PrepareCache()

		search := NewScanSearchWithTape(
			ctx,
			measurementProfile,
			rows,
			replay.PrecompileTape(rows),
			types.ScanOptions{Workers: 1, BeamWidth: 2},
		)
		searchProgress := progress.NewSearchProgress()
		search.progress = searchProgress
		for range searchProgress.StagnationLimit(2) {
			searchProgress.Record(0, 0, func(float64, int, float64, int) bool { return false })
		}
		search.haltPhaseOnStagnation = true

		survivors := []types.CandidateScore{{
			Branches: perspectives.BranchList{{
				Category: perspectives.CategoryLaminar,
				Branches: []perspectives.Branch{{
					Category:    perspectives.CategoryLaminar,
					Observation: perspectives.ObservationNotHolding,
					Action:      perspectives.Action{Type: perspectives.ActionLimit},
				}},
			}},
		}}
		gates := search.rankedEntryBranchers()
		emitted := 0

		search.emitDeepeningExpansions(func(scanCandidate) bool {
			emitted++

			return true
		}, survivors, gates)

		convey.Convey("It should not emit candidates when scoring is already halted", func() {
			convey.So(emitted, convey.ShouldEqual, 0)
		})
	})
}
