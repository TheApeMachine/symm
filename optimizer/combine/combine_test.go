package combine

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func strategy(entryCategory perspectives.CategoryType, value float64) perspectives.BranchList {
	return perspectives.BranchList{
		{
			Category: entryCategory, Observation: perspectives.ObservationNotHolding,
			Condition: perspectives.ConditionIsGreaterThanOrEqual,
			Unit:      perspectives.UnitSNR, Value: value, ValueSet: true,
			Action: perspectives.Action{Type: perspectives.ActionLimit},
		},
		{
			Category: perspectives.CategoryActiveReversal, Observation: perspectives.ObservationHolding,
			Condition: perspectives.ConditionIsGreaterThanOrEqual,
			Unit:      perspectives.UnitSNR, Value: 1, ValueSet: true,
			Action: perspectives.Action{Type: perspectives.ActionSettlePosition},
		},
	}
}

// distinctEntryScore rewards a tree for each distinct entry-category family it
// holds — a stand-in for "more complementary strategies earn more joint return".
func distinctEntryScore(branches perspectives.BranchList) float64 {
	seen := make(map[perspectives.CategoryType]struct{})

	for _, index := range perspectives.FindAllEntryIndices(branches) {
		collectCategories(branches[index], seen)
	}

	return float64(len(seen))
}

func TestGreedyCombinesDistinctStrategies(t *testing.T) {
	convey.Convey("Given three distinct-family strategies", t, func() {
		pool := []Scored{
			{Branches: strategy(perspectives.CategoryLaminar, 1), Score: 3},
			{Branches: strategy(perspectives.CategoryExhaustion, 1), Score: 2},
			{Branches: strategy(perspectives.CategoryActiveReversal, 1), Score: 1},
		}

		combined := Greedy(pool, distinctEntryScore)

		convey.Convey("All three become sibling entries when each improves joint score", func() {
			convey.So(len(perspectives.FindAllEntryIndices(combined)), convey.ShouldEqual, 3)
			convey.So(perspectives.IsCanonicalPlaybook(combined), convey.ShouldBeTrue)
		})
	})
}

func TestGreedyRejectsNonImproving(t *testing.T) {
	convey.Convey("Given a scorer that only ever values the first strategy", t, func() {
		pool := []Scored{
			{Branches: strategy(perspectives.CategoryLaminar, 1), Score: 3},
			{Branches: strategy(perspectives.CategoryExhaustion, 1), Score: 2},
		}

		flatScore := func(perspectives.BranchList) float64 { return 1.0 }
		combined := Greedy(pool, flatScore)

		convey.Convey("Only the best single strategy survives (no regression)", func() {
			convey.So(len(perspectives.FindAllEntryIndices(combined)), convey.ShouldEqual, 1)
		})
	})
}

func BenchmarkGreedy(b *testing.B) {
	families := []perspectives.CategoryType{
		perspectives.CategoryLaminar,
		perspectives.CategoryExhaustion,
		perspectives.CategoryActiveReversal,
		perspectives.CategoryToxicBluff,
	}

	pool := make([]Scored, 0, len(families))

	for index, category := range families {
		pool = append(pool, Scored{
			Branches: strategy(category, 1),
			Score:    float64(len(families) - index),
		})
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = Greedy(pool, distinctEntryScore)
	}
}

func TestGreedyDedupesSameFamily(t *testing.T) {
	convey.Convey("Given two variants of the same entry family", t, func() {
		pool := []Scored{
			{Branches: strategy(perspectives.CategoryLaminar, 1), Score: 5},
			{Branches: strategy(perspectives.CategoryLaminar, 2), Score: 4},
			{Branches: strategy(perspectives.CategoryExhaustion, 1), Score: 1},
		}

		combined := Greedy(pool, distinctEntryScore)

		convey.Convey("The family collapses to its best representative, two entries total", func() {
			convey.So(len(perspectives.FindAllEntryIndices(combined)), convey.ShouldEqual, 2)
		})
	})
}
