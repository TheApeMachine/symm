package mcts

import (
	"bytes"
	"context"
	"io"
	"math"
	"os"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/profile"
	"github.com/theapemachine/symm/optimizer/types"
)

func TestNormalizeReward(t *testing.T) {
	convey.Convey("Given replay PnL scores", t, func() {
		convey.Convey("It should map zero return to 0.5", func() {
			convey.So(normalizeReward(0, 1), convey.ShouldAlmostEqual, 0.5, 0.0001)
		})

		convey.Convey("It should map positive return above 0.5", func() {
			convey.So(normalizeReward(0.10, 1), convey.ShouldBeGreaterThan, 0.5)
		})

		convey.Convey("It should map negative return below 0.5", func() {
			convey.So(normalizeReward(-0.10, 1), convey.ShouldBeLessThan, 0.5)
		})
	})
}

func TestTreeSearchCachesMoves(t *testing.T) {
	convey.Convey("Given a hybrid tree search", t, func() {
		ctx := context.Background()
		profile := profile.NewProfile(ctx)
		profile.Add(perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Source:   perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      2,
			Last:     100,
		})

		search := NewHybridTreeSearch(
			ctx,
			profile,
			profile.Rows(),
			types.GuardOptions{},
			nil,
			Options{Iterations: 1},
			nil,
		)

		convey.Convey("It should reuse the pre-generated move list", func() {
			convey.So(len(search.Moves().Cached()), convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestTreeSearchRunProgressLogging(t *testing.T) {
	convey.Convey("Given a hybrid tree search run", t, func() {
		ctx := context.Background()
		profile := profile.NewProfile(ctx)
		profile.Add(perspectives.Measurement{
			Symbol:   "BTC/EUR",
			Source:   perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      2,
			Last:     100,
		})

		search := NewHybridTreeSearch(
			ctx,
			profile,
			profile.Rows(),
			types.GuardOptions{},
			[]types.CandidateScore{{
				Branches: perspectives.BranchList{{
					Category:    perspectives.CategoryLaminar,
					Observation: perspectives.ObservationNotHolding,
					Condition:   perspectives.ConditionIsGreaterThanOrEqual,
					Unit:        perspectives.UnitSNR,
					Value:       1,
					ValueSet:    true,
					Action:      perspectives.Action{Type: perspectives.ActionLimit},
				}},
			}},
			Options{Iterations: 3},
			nil,
		)

		original := os.Stderr
		reader, writer, err := os.Pipe()
		convey.So(err, convey.ShouldBeNil)

		os.Stderr = writer
		_ = search.Run()
		writer.Close()
		os.Stderr = original

		var buffer bytes.Buffer
		_, _ = io.Copy(&buffer, reader)
		output := buffer.String()

		convey.Convey("It should log seeding and rollout progress phases", func() {
			convey.So(output, convey.ShouldContainSubstring, "mcts seeding 1 roots")
			convey.So(output, convey.ShouldContainSubstring, "mcts seeding finished")
			convey.So(output, convey.ShouldContainSubstring, "mcts rollouts starting (3 iterations)")
			convey.So(output, convey.ShouldContainSubstring, "mcts rollouts 1/3")
			convey.So(output, convey.ShouldContainSubstring, "mcts rollouts finished (3 iterations")
		})
	})
}

func TestTreeSearchScoreBranches(t *testing.T) {
	convey.Convey("Given replayable branches", t, func() {
		ctx := context.Background()
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar, SNR: 2, Last: 100,
			},
		}
		search := NewHybridTreeSearch(
			ctx,
			profile.NewProfile(ctx),
			rows,
			types.GuardOptions{},
			nil,
			Options{Iterations: 1},
			nil,
		)

		branches := perspectives.BranchList{{
			Category:    perspectives.CategoryLaminar,
			Observation: perspectives.ObservationNotHolding,
			Condition:   perspectives.ConditionIsGreaterThanOrEqual,
			Unit:        perspectives.UnitSNR,
			Value:       1, ValueSet: true,
			Action: perspectives.Action{Type: perspectives.ActionLimit},
		}}

		score := search.scoreBranches(branches)

		convey.Convey("It should score branches through replay simulation", func() {
			convey.So(score, convey.ShouldBeGreaterThan, -1)
			convey.So(math.IsNaN(score), convey.ShouldBeFalse)
		})
	})
}
