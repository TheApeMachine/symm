package reasoning

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func rootDepth(root reasoning.Thought) int {
	return forestDepth([]reasoning.Thought{root})
}

// anyPredicate reports whether any predicate anywhere in the forest matches.
func anyPredicate(forest []reasoning.Thought, match func(reasoning.Predicate) bool) bool {
	var checkPredicate func(predicate reasoning.Predicate) bool
	checkPredicate = func(predicate reasoning.Predicate) bool {
		if match(predicate) {
			return true
		}

		for _, operand := range predicate.All {
			if checkPredicate(operand) {
				return true
			}
		}

		for _, operand := range predicate.Any {
			if checkPredicate(operand) {
				return true
			}
		}

		return predicate.Not != nil && checkPredicate(*predicate.Not)
	}

	var walk func(nodes []reasoning.Thought) bool
	walk = func(nodes []reasoning.Thought) bool {
		for index := range nodes {
			if checkPredicate(nodes[index].When) {
				return true
			}

			if walk(nodes[index].Then) {
				return true
			}
		}

		return false
	}

	return walk(forest)
}

func twoBranchForest() []reasoning.Thought {
	return []reasoning.Thought{
		{When: allOf(notHolding(), signalAtLeast(types.CategoryVerticalIgnition, 1.0)), Do: reasoning.Act{Type: reasoning.ActionMarket}},
		{When: allOf(notHolding(), signalAtLeast(types.CategoryCoiledCompression, 1.0)), Do: reasoning.Act{Type: reasoning.ActionMarket}},
		{When: holding(), Do: reasoning.Act{Type: reasoning.ActionTrailingStop, Offset: 0.02}},
	}
}

func TestTemporalizeDeepensEachBranchIndependently(t *testing.T) {
	Convey("Given a forest with two parallel entry branches", t, func() {
		vocab := DeriveVocabulary(ignitionRows())
		forest := twoBranchForest()

		So(rootDepth(forest[0]), ShouldEqual, 1)
		So(rootDepth(forest[1]), ShouldEqual, 1)

		Convey("temporalize produces neighbours that deepen EACH branch on its own", func() {
			neighbors := temporalizeEntry(forest, vocab)

			deepenedFirst := false
			deepenedSecond := false

			for _, neighbor := range neighbors {
				first := rootDepth(neighbor[0])
				second := rootDepth(neighbor[1])

				if first == 2 && second == 1 {
					deepenedFirst = true
				}

				if second == 2 && first == 1 {
					deepenedSecond = true
				}
			}

			// This is the fix for the critical finding: depth is no longer confined
			// to branch 0 — both strategy branches can grow their own temporal chain.
			So(deepenedFirst, ShouldBeTrue)
			So(deepenedSecond, ShouldBeTrue)
		})
	})
}

func TestNeighborsIncludeCrossedUpAndLifecycleExit(t *testing.T) {
	Convey("Given a seed and its neighbours", t, func() {
		vocab := DeriveVocabulary(ignitionRows())
		seed := Seeds(vocab)[0]
		neighbors := Neighbors(seed, vocab)

		Convey("Some neighbour waits for the signal to cross up (an edge, not a level)", func() {
			found := false
			for _, neighbor := range neighbors {
				if anyPredicate(neighbor, func(p reasoning.Predicate) bool {
					return p.Op == reasoning.ComparisonCrossedUp
				}) {
					found = true
				}
			}
			So(found, ShouldBeTrue)
		})

		Convey("Some neighbour adds a lifecycle exit (settle once the move has ended)", func() {
			found := false
			for _, neighbor := range neighbors {
				if anyPredicate(neighbor, func(p reasoning.Predicate) bool {
					return p.Subject == reasoning.SubjectPosition && p.Lifecycle == types.ObservationHasEnded
				}) {
					found = true
				}
			}
			So(found, ShouldBeTrue)
		})
	})
}

func TestNeighborsIncludeVersusNotAndTimeStop(t *testing.T) {
	Convey("Given a seed and a two-signal vocabulary", t, func() {
		rows := []types.Measurement{
			{Symbol: "BTC/EUR", Category: types.CategoryVerticalIgnition, SNR: 1.5, Last: 100},
			{Symbol: "BTC/EUR", Category: types.CategoryCoiledCompression, SNR: 1.2, Last: 101},
		}
		vocab := DeriveVocabulary(rows)
		seed := Seeds(vocab)[0]
		neighbors := Neighbors(seed, vocab)

		hasVersus := false
		hasNot := false
		hasTimeStop := false

		for _, neighbor := range neighbors {
			if anyPredicate(neighbor, func(p reasoning.Predicate) bool { return p.Versus != nil }) {
				hasVersus = true
			}
			if anyPredicate(neighbor, func(p reasoning.Predicate) bool { return p.Not != nil }) {
				hasNot = true
			}
			if anyPredicate(neighbor, func(p reasoning.Predicate) bool { return p.Subject == reasoning.SubjectElapsed }) {
				hasTimeStop = true
			}
		}

		Convey("It can express metric-to-metric, negation, and a time-stop", func() {
			So(hasVersus, ShouldBeTrue)   // signal-above-signal
			So(hasNot, ShouldBeTrue)      // avoid another signal
			So(hasTimeStop, ShouldBeTrue) // settle after elapsed minutes
		})
	})
}

func TestMinRoundTripsDiscountAppliesWhenSet(t *testing.T) {
	Convey("Given a profitable but low-trade tape and a high MinRoundTrips floor", t, func() {
		testconfig.Load(t)

		rows := rallyTape()

		result, err := Search(context.Background(), rows, frictionlessCosts(), SearchConfig{
			BeamWidth:     6,
			MaxRounds:     6,
			Patience:      3,
			MinRoundTrips: 50, // far above what this short tape can trade
		})
		So(err, ShouldBeNil)

		Convey("The winner is profitable but trades far fewer than the floor", func() {
			So(result.Best.Return, ShouldBeGreaterThan, 0)
			So(result.Best.Trades, ShouldBeLessThan, 50)
		})

		Convey("Its score is discounted below full velocity credit by the trade shortfall", func() {
			tradeFactor := float64(result.Best.Trades) / 50.0

			So(tradeFactor, ShouldBeLessThan, 1.0)
			So(result.Best.Score, ShouldBeGreaterThan, 0)

			impliedFullCredit := result.Best.Score / tradeFactor

			So(impliedFullCredit, ShouldBeGreaterThanOrEqualTo, result.Best.RealizedEUR)
		})
	})
}

func BenchmarkNeighbors(b *testing.B) {
	vocab := DeriveVocabulary(ignitionRows())
	forest := Seeds(vocab)[0]

	for b.Loop() {
		_ = Neighbors(forest, vocab)
	}
}

func BenchmarkTemporalizeEntry(b *testing.B) {
	vocab := DeriveVocabulary(ignitionRows())
	forest := Seeds(vocab)[0]

	for b.Loop() {
		_ = temporalizeEntry(forest, vocab)
	}
}
