package scan

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/profile"
	"github.com/theapemachine/symm/optimizer/types"
)

func TestScanSearchBeamScoresClone(t *testing.T) {
	convey.Convey("Given recorded beam scores", t, func() {
		ctx := context.Background()
		search := NewScanSearch(ctx, profile.NewProfile(ctx), nil, types.ScanOptions{Workers: 1})
		search.beamScores = []types.CandidateScore{{
			Candidate: 1,
			Score:     10,
			Branches: perspectives.BranchList{{
				Category: perspectives.CategoryLaminar,
			}},
		}}

		cloned := search.BeamScoresClone()

		convey.Convey("It should deep-clone branch lists", func() {
			convey.So(len(cloned), convey.ShouldEqual, 1)
			convey.So(cloned[0].Score, convey.ShouldEqual, 10)
			cloned[0].Branches[0].Category = perspectives.CategoryExhaustion
			convey.So(search.beamScores[0].Branches[0].Category, convey.ShouldEqual, perspectives.CategoryLaminar)
		})
	})
}

func TestScanSearchEvaluateRaw(t *testing.T) {
	convey.Convey("Given replayable branches", t, func() {
		ctx := context.Background()
		rows := []perspectives.Measurement{
			{Symbol: "BTC/EUR", Source: perspectives.SourceFluid, Category: perspectives.CategoryLaminar, SNR: 2, Last: 100},
		}
		search := NewScanSearch(ctx, profile.NewProfile(ctx), rows, types.ScanOptions{Workers: 1})
		branches := perspectives.BranchList{{
			Category:    perspectives.CategoryLaminar,
			Observation: perspectives.ObservationNotHolding,
			Condition:   perspectives.ConditionIsGreaterThanOrEqual,
			Unit:        perspectives.UnitSNR,
			Value:       1, ValueSet: true,
			Action: perspectives.Action{Type: perspectives.ActionLimit},
		}}

		score := search.evaluateRaw(branches)

		convey.Convey("It should score branches without guard adjustment", func() {
			convey.So(score, convey.ShouldBeGreaterThan, -1)
		})
	})
}

func TestScanSearchBest(t *testing.T) {
	convey.Convey("Given recorded beam scores", t, func() {
		ctx := context.Background()
		search := NewScanSearch(ctx, profile.NewProfile(ctx), nil, types.ScanOptions{Workers: 1})
		search.bestBranch = perspectives.BranchList{{
			Category: perspectives.CategoryLaminar,
		}}

		best := search.best()

		convey.Convey("It should return the leading branch list", func() {
			convey.So(len(best), convey.ShouldEqual, 1)
			convey.So(best[0].Category, convey.ShouldEqual, perspectives.CategoryLaminar)
		})
	})
}

func TestRecordBeamAmortizesPruning(t *testing.T) {
	convey.Convey("Given a scan search with a small beam width", t, func() {
		ctx := context.Background()
		search := NewScanSearch(ctx, profile.NewProfile(ctx), nil, types.ScanOptions{
			BeamWidth: 1,
			Budget:    types.SearchBudget{BeamWidth: 1, BeamPruneFactor: 4},
		})

		for candidate := 1; candidate <= 5; candidate++ {
			search.recordBeam(types.CandidateScore{
				Candidate:     candidate,
				Score:         float64(candidate) * -0.001,
				AdjustedScore: float64(candidate) * -0.001,
				ClosedTrades:  1,
				Branches: perspectives.BranchList{
					{Category: perspectives.CategoryLaminar},
				},
			})
		}

		convey.Convey("It should prune once the buffer exceeds four times the beam width", func() {
			convey.So(len(search.beamScores), convey.ShouldBeLessThanOrEqualTo, 1)
		})
	})
}
