package strategy

import (
	"slices"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func economicCandidate(symbol string, expectedOutcome float64, visits float64) *types.Decision {
	decision := types.NewDecision(types.ActionEnter, symbol)
	alternativesOf(decision)["economic:expected_outcome"] = expectedOutcome
	alternativesOf(decision)["economic:visits"] = visits
	return decision
}

func TestAllocationCalculate(t *testing.T) {
	Convey("Given a decision that does not enter", t, func() {
		decision := types.NewDecision(types.ActionNothing, "BTC/USD")
		allocation := NewAllocation(t.Context(), nil)

		err := allocation.Calculate([]*types.Decision{decision})

		Convey("It should leave the decision untouched without broker state", func() {
			So(err, ShouldBeNil)
			So(decision.Action, ShouldEqual, types.ActionNothing)
		})
	})

	Convey("Given an enter decision without a broker desk", t, func() {
		decision := types.NewDecision(types.ActionEnter, "BTC/USD")
		allocation := NewAllocation(t.Context(), nil)

		err := allocation.Calculate([]*types.Decision{decision})

		Convey("It should report that a broker desk is required", func() {
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "broker desk required")
		})
	})
}

func TestEconomicOrder(t *testing.T) {
	Convey("Given two candidates with different expected economic outcomes", t, func() {
		higher := economicCandidate("HIGH/USD", 0.08, 10)
		lower := economicCandidate("LOW/USD", 0.02, 20)

		Convey("It should prefer the higher expected economic outcome", func() {
			So(economicOrder(higher, lower), ShouldEqual, -1)
			So(economicOrder(lower, higher), ShouldEqual, 1)
		})
	})

	Convey("Given two candidates with equal expected outcome", t, func() {
		moreVisits := economicCandidate("AAA/USD", 0.05, 40)
		fewerVisits := economicCandidate("ZZZ/USD", 0.05, 10)

		Convey("It should prefer the higher search visits", func() {
			So(economicOrder(moreVisits, fewerVisits), ShouldEqual, -1)
		})
	})

	Convey("Given two fully tied candidates", t, func() {
		first := economicCandidate("AAA/USD", 0.05, 10)
		second := economicCandidate("BBB/USD", 0.05, 10)

		Convey("It should break the tie on symbol identity so the fill is stable", func() {
			So(economicOrder(first, second), ShouldEqual, -1)
			So(economicOrder(second, first), ShouldEqual, 1)
			So(economicOrder(first, first), ShouldEqual, 0)
		})
	})
}

func TestAdmitBest(t *testing.T) {
	Convey("Given more entering candidates than open slots", t, func() {
		weak := economicCandidate("WEAK/USD", 0.02, 10)
		mid := economicCandidate("MID/USD", 0.05, 10)
		strong := economicCandidate("STRONG/USD", 0.09, 10)
		noise := types.NewDecision(types.ActionNothing, "NOISE/USD")

		Convey("It should fill only the best economic candidates regardless of arrival order", func() {
			for _, order := range [][]*types.Decision{
				{weak, mid, strong, noise},
				{strong, noise, weak, mid},
				{noise, mid, weak, strong},
				{mid, strong, noise, weak},
			} {
				clone := cloneDecisions(order)
				admitBest(clone, 2, 0, nil)
				entered := enteredSymbols(clone)
				So(entered, ShouldResemble, []string{"MID/USD", "STRONG/USD"})
				So(decisionBySymbol(clone, "WEAK/USD").Action, ShouldEqual, types.ActionNothing)
				So(decisionBySymbol(clone, "NOISE/USD").Action, ShouldEqual, types.ActionNothing)
			}
		})
	})

	Convey("Given an occupied symbol", t, func() {
		candidate := economicCandidate("HELD/USD", 0.09, 10)
		occupied := map[string]bool{"HELD/USD": true}

		Convey("It should not consume a slot for an already-held symbol", func() {
			admitBest([]*types.Decision{candidate}, 1, 0, occupied)
			So(candidate.Action, ShouldEqual, types.ActionNothing)
			So(candidate.Reason, ShouldContainSubstring, "already occupies")
		})
	})

	Convey("Given more candidates than slots plus reserve", t, func() {
		best := economicCandidate("BEST/USD", 0.10, 10)
		second := economicCandidate("SECOND/USD", 0.06, 10)
		third := economicCandidate("THIRD/USD", 0.01, 10)

		Convey("Reserve capacity is filled by economic rank too", func() {
			clone := []*types.Decision{third, best, second}
			admitBest(clone, 1, 1, nil)
			So(decisionBySymbol(clone, "BEST/USD").Action, ShouldEqual, types.ActionEnter)
			So(decisionBySymbol(clone, "SECOND/USD").Action, ShouldEqual, types.ActionEnter)
			So(decisionBySymbol(clone, "THIRD/USD").Action, ShouldEqual, types.ActionNothing)
		})
	})
}

func cloneDecisions(decisions []*types.Decision) []*types.Decision {
	cloned := make([]*types.Decision, len(decisions))

	for index, decision := range decisions {
		clonedDecision := *decision
		cloned[index] = &clonedDecision
	}

	return cloned
}

func enteredSymbols(decisions []*types.Decision) []string {
	entered := make([]string, 0)

	for _, decision := range decisions {
		if decision.Action == types.ActionEnter {
			entered = append(entered, decision.Symbol)
		}
	}

	slices.Sort(entered)
	return entered
}

func decisionBySymbol(decisions []*types.Decision, symbol string) *types.Decision {
	for _, decision := range decisions {
		if decision.Symbol == symbol {
			return decision
		}
	}

	return nil
}
