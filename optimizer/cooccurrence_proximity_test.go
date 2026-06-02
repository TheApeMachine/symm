package optimizer

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestProximityChainSupport(t *testing.T) {
	convey.Convey("Given categories that appear on adjacent ticks but not together", t, func() {
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar, SNR: 2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion, SNR: 2, Last: 110,
			},
		}
		index := NewCoOccurrenceIndex(PrecompileTape(rows), 2)

		convey.Convey("It should count near-miss proximity support", func() {
			convey.So(index.ChainReachable([]perspectives.CategoryType{
				perspectives.CategoryLaminar,
				perspectives.CategoryExhaustion,
			}), convey.ShouldBeFalse)
			convey.So(index.ProximityChainSupport([]perspectives.CategoryType{
				perspectives.CategoryLaminar,
				perspectives.CategoryExhaustion,
			}, 1), convey.ShouldEqual, 2)
		})

		convey.Convey("It should expose soft reachability through ChainReachability", func() {
			hard, nearMiss := index.ChainReachability([]perspectives.CategoryType{
				perspectives.CategoryLaminar,
				perspectives.CategoryExhaustion,
			}, 1)

			convey.So(hard, convey.ShouldBeFalse)
			convey.So(nearMiss, convey.ShouldBeTrue)
		})
	})
}

func TestMoveReachabilityTheoretical(t *testing.T) {
	convey.Convey("Given adjacent-but-not-simultaneous categories", t, func() {
		ctx := t.Context()
		profile := Profile{ctx: ctx}
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar, SNR: 2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion, SNR: 2, Last: 110,
			},
		}

		for _, row := range rows {
			profile.Add(row)
		}

		tape := PrecompileTape(rows)
		search := NewHybridTreeSearchWithTape(
			ctx, &profile, rows, tape, GuardOptions{}, nil, MCTSOptions{},
		)
		move := Move{
			category:    perspectives.CategoryExhaustion,
			observation: perspectives.ObservationNotHolding,
			condition:   perspectives.ConditionIsGreaterThanOrEqual,
			unit:        perspectives.UnitSNR,
			value:       1,
			action:      perspectives.ActionLimit,
		}

		convey.Convey("It should allow theoretical near-miss moves with UCT discount", func() {
			allowed, theoretical, discount := search.moveReachability(
				move, perspectives.BranchList{{
					Category:    perspectives.CategoryLaminar,
					Observation: perspectives.ObservationNotHolding,
					Condition:   perspectives.ConditionIsGreaterThanOrEqual,
					Unit:        perspectives.UnitSNR,
					Value:       1,
					ValueSet:    true,
					Action:      perspectives.Action{Type: perspectives.ActionLimit},
				}},
			)

			convey.So(allowed, convey.ShouldBeTrue)
			convey.So(theoretical, convey.ShouldBeTrue)
			convey.So(discount, convey.ShouldBeGreaterThan, 0)
			convey.So(discount, convey.ShouldBeLessThan, 1)
		})
	})
}
