package reasoning

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func rootDepth(root perspectives.Thought) int {
	return forestDepth([]perspectives.Thought{root})
}

// anyPredicate reports whether any predicate anywhere in the forest matches.
func anyPredicate(forest []perspectives.Thought, match func(perspectives.Predicate) bool) bool {
	var checkPredicate func(predicate perspectives.Predicate) bool
	checkPredicate = func(predicate perspectives.Predicate) bool {
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

	var walk func(nodes []perspectives.Thought) bool
	walk = func(nodes []perspectives.Thought) bool {
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

func twoBranchForest() []perspectives.Thought {
	return []perspectives.Thought{
		{When: allOf(notHolding(), signalAtLeast(perspectives.CategoryVerticalIgnition, 1.0)), Do: perspectives.Act{Type: perspectives.ActionMarket}},
		{When: allOf(notHolding(), signalAtLeast(perspectives.CategoryCoiledCompression, 1.0)), Do: perspectives.Act{Type: perspectives.ActionMarket}},
		{When: holding(), Do: perspectives.Act{Type: perspectives.ActionTrailingStop, Offset: 0.02}},
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
				if anyPredicate(neighbor, func(p perspectives.Predicate) bool {
					return p.Op == perspectives.ComparisonCrossedUp
				}) {
					found = true
				}
			}
			So(found, ShouldBeTrue)
		})

		Convey("Some neighbour adds a lifecycle exit (settle once the move has ended)", func() {
			found := false
			for _, neighbor := range neighbors {
				if anyPredicate(neighbor, func(p perspectives.Predicate) bool {
					return p.Subject == perspectives.SubjectPosition && p.Lifecycle == perspectives.ObservationHasEnded
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
		rows := []perspectives.Measurement{
			{Symbol: "BTC/EUR", Category: perspectives.CategoryVerticalIgnition, SNR: 1.5, Last: 100},
			{Symbol: "BTC/EUR", Category: perspectives.CategoryCoiledCompression, SNR: 1.2, Last: 101},
		}
		vocab := DeriveVocabulary(rows)
		seed := Seeds(vocab)[0]
		neighbors := Neighbors(seed, vocab)

		hasVersus := false
		hasNot := false
		hasTimeStop := false

		for _, neighbor := range neighbors {
			if anyPredicate(neighbor, func(p perspectives.Predicate) bool { return p.Versus != nil }) {
				hasVersus = true
			}
			if anyPredicate(neighbor, func(p perspectives.Predicate) bool { return p.Not != nil }) {
				hasNot = true
			}
			if anyPredicate(neighbor, func(p perspectives.Predicate) bool { return p.Subject == perspectives.SubjectElapsed }) {
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
		rows := rallyTape()

		result := Search(context.Background(), rows, frictionlessCosts(), SearchConfig{
			BeamWidth:     6,
			MaxRounds:     6,
			Patience:      3,
			MinRoundTrips: 50, // far above what this short tape can trade
		})

		Convey("The winner is profitable but trades far fewer than the floor", func() {
			So(result.Best.Return, ShouldBeGreaterThan, 0)
			So(result.Best.Trades, ShouldBeLessThan, 50)
		})

		Convey("Its score is the return discounted by the trade shortfall", func() {
			expected := result.Best.Return * float64(result.Best.Trades) / 50.0
			So(result.Best.Score, ShouldAlmostEqual, expected, 1e-9)
			So(result.Best.Score, ShouldBeLessThan, result.Best.Return)
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
