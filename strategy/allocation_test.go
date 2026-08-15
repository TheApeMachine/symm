package strategy

import (
	"slices"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

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
}

func TestAdmissionOrder(t *testing.T) {
	Convey("Given two candidates that differ on the regulated gates", t, func() {
		higherUtility := admissionCandidate("WEAK/USD", 0.04, 0.10)
		lowerUtility := admissionCandidate("STRONG/USD", 0.08, 0.01)

		Convey("It should prefer the higher net utility even when graph score is worse", func() {
			So(admissionOrder(lowerUtility, higherUtility), ShouldEqual, -1)
			So(admissionOrder(higherUtility, lowerUtility), ShouldEqual, 1)
		})
	})

	Convey("Given two candidates with the same net utility", t, func() {
		higherGraph := admissionCandidate("AAA/USD", 0.05, 0.90)
		lowerGraph := admissionCandidate("ZZZ/USD", 0.05, 0.10)

		Convey("It should prefer the higher graph score the regulator also gates on", func() {
			So(admissionOrder(higherGraph, lowerGraph), ShouldEqual, -1)
			So(admissionOrder(lowerGraph, higherGraph), ShouldEqual, 1)
		})
	})

	Convey("Given two candidates that the regulator cannot separate", t, func() {
		first := admissionCandidate("AAA/USD", 0.05, 0.50)
		second := admissionCandidate("BBB/USD", 0.05, 0.50)

		Convey("It should break the tie on symbol identity so the fill is stable", func() {
			So(admissionOrder(first, second), ShouldEqual, -1)
			So(admissionOrder(second, first), ShouldEqual, 1)
			So(admissionOrder(first, first), ShouldEqual, 0)
		})
	})
}

func TestAdmitBest(t *testing.T) {
	Convey("Given more passing entries than open slots", t, func() {
		weak := admissionCandidate("WEAK/USD", 0.02, 0.90)
		mid := admissionCandidate("MID/USD", 0.05, 0.10)
		strong := admissionCandidate("STRONG/USD", 0.09, 0.10)
		noise := types.NewDecision(types.ActionNothing, "NOISE/USD")
		noise.Utility = 1
		noise.GraphScore = 1

		Convey("It should fill only the best candidates regardless of arrival order", func() {
			for _, order := range [][]*types.Decision{
				{weak, mid, strong, noise},
				{strong, noise, weak, mid},
				{noise, mid, weak, strong},
				{mid, strong, noise, weak},
			} {
				clone := cloneAdmission(order)
				admitBest(clone, 2, 0, nil)
				entered := enteredSymbols(clone)
				So(entered, ShouldResemble, []string{"MID/USD", "STRONG/USD"})
				So(decisionBySymbol(clone, "WEAK/USD").Action, ShouldEqual, types.ActionNothing)
				So(decisionBySymbol(clone, "WEAK/USD").Reason,
					ShouldEqual, "planner: no position slot available for allocation")
				So(decisionBySymbol(clone, "WEAK/USD").Stoploss, ShouldBeNil)
				So(decisionBySymbol(clone, "NOISE/USD").Action, ShouldEqual, types.ActionNothing)
			}
		})
	})

	Convey("Given one slot and a later-arriving higher utility", t, func() {
		early := admissionCandidate("EARLY/USD", 0.03, 0.80)
		late := admissionCandidate("LATE/USD", 0.11, 0.20)

		Convey("It should give the slot to the later, better candidate", func() {
			admitBest([]*types.Decision{early, late}, 1, 0, nil)
			So(early.Action, ShouldEqual, types.ActionNothing)
			So(late.Action, ShouldEqual, types.ActionEnter)
		})
	})

	Convey("Given a held symbol that still posts the highest utility", t, func() {
		held := admissionCandidate("HELD/USD", 0.20, 0.90)
		challenger := admissionCandidate("FREE/USD", 0.04, 0.10)
		held.Stoploss = &types.Stoploss{}

		Convey("It should keep the open slot for a symbol that can actually fill it", func() {
			admitBest([]*types.Decision{held, challenger}, 1, 0, map[string]bool{
				"HELD/USD": true,
			})
			So(held.Action, ShouldEqual, types.ActionNothing)
			So(held.Reason, ShouldEqual, "planner: symbol already occupies a slot")
			So(held.Stoploss, ShouldBeNil)
			So(challenger.Action, ShouldEqual, types.ActionEnter)
		})
	})

	Convey("Given two entries for the same symbol", t, func() {
		first := admissionCandidate("DUP/USD", 0.08, 0.40)
		second := admissionCandidate("DUP/USD", 0.07, 0.90)
		other := admissionCandidate("OTHER/USD", 0.06, 0.10)

		Convey("It should spend at most one slot on that symbol", func() {
			admitBest([]*types.Decision{first, second, other}, 2, 0, nil)
			So(first.Action, ShouldEqual, types.ActionEnter)
			So(second.Action, ShouldEqual, types.ActionNothing)
			So(second.Reason, ShouldEqual, "planner: symbol already occupies a slot")
			So(other.Action, ShouldEqual, types.ActionEnter)
		})
	})

	Convey("Given no open slots", t, func() {
		candidate := admissionCandidate("ALONE/USD", 0.15, 0.90)

		Convey("It should refuse every candidate", func() {
			admitBest([]*types.Decision{candidate}, 0, 0, nil)
			So(candidate.Action, ShouldEqual, types.ActionNothing)
			So(candidate.Reason, ShouldEqual, "planner: no position slot available for allocation")
		})
	})

	Convey("Given equal utility and one better graph score", t, func() {
		lowEvidence := admissionCandidate("LOW/USD", 0.05, 0.10)
		highEvidence := admissionCandidate("HIGH/USD", 0.05, 0.80)

		Convey("It should fill with the regulator's graph ranking", func() {
			admitBest([]*types.Decision{lowEvidence, highEvidence}, 1, 0, nil)
			So(highEvidence.Action, ShouldEqual, types.ActionEnter)
			So(lowEvidence.Action, ShouldEqual, types.ActionNothing)
		})
	})

	Convey("Given equal utility and graph score", t, func() {
		zebra := admissionCandidate("ZZZ/USD", 0.05, 0.50)
		alpha := admissionCandidate("AAA/USD", 0.05, 0.50)

		Convey("It should fill deterministically regardless of presentation order", func() {
			admitBest([]*types.Decision{zebra, alpha}, 1, 0, nil)
			So(alpha.Action, ShouldEqual, types.ActionEnter)
			So(zebra.Action, ShouldEqual, types.ActionNothing)

			zebra = admissionCandidate("ZZZ/USD", 0.05, 0.50)
			alpha = admissionCandidate("AAA/USD", 0.05, 0.50)
			admitBest([]*types.Decision{alpha, zebra}, 1, 0, nil)
			So(alpha.Action, ShouldEqual, types.ActionEnter)
			So(zebra.Action, ShouldEqual, types.ActionNothing)
		})
	})

	Convey("Given a nil decision mixed into a live field", t, func() {
		live := admissionCandidate("LIVE/USD", 0.05, 0.50)

		Convey("It should ignore the hole and still fill the slot", func() {
			admitBest([]*types.Decision{nil, live, nil}, 1, 0, nil)
			So(live.Action, ShouldEqual, types.ActionEnter)
		})
	})

	Convey("Given a reserved opportunity after normal slots are gone", t, func() {
		normal := admissionCandidate("NORMAL/USD", 0.10, 0.20)
		reserve := admissionCandidate("RESERVE/USD", 0.03, 0.20)
		reserve.Opportunity = true
		overflow := admissionCandidate("OVERFLOW/USD", 0.02, 0.20)
		overflow.Opportunity = true

		Convey("It should spend the reserve only after ranking, and only on opportunities", func() {
			admitBest([]*types.Decision{overflow, reserve, normal}, 1, 1, nil)
			So(normal.Action, ShouldEqual, types.ActionEnter)
			So(reserve.Action, ShouldEqual, types.ActionEnter)
			So(overflow.Action, ShouldEqual, types.ActionNothing)
		})
	})
}

func TestVisibleAskQuantity(t *testing.T) {
	Convey("Given a cash-sized request larger than the visible ask", t, func() {
		requested := decimal.NewFromFloat64(2)

		Convey("It should keep only the quantity the ticker can fill", func() {
			capped := visibleAskQuantity(0.4, requested)

			So(capped.Float64(), ShouldAlmostEqual, 0.4, 1e-12)
			So(requested.Float64(), ShouldAlmostEqual, 2, 1e-12)
		})
	})

	Convey("Given a request already inside the visible ask", t, func() {
		requested := decimal.NewFromFloat64(0.2)

		Convey("It should leave the request unchanged", func() {
			So(visibleAskQuantity(1.5, requested), ShouldEqual, requested)
		})
	})

	Convey("Given no observable ask quantity", t, func() {
		requested := decimal.NewFromFloat64(2)

		Convey("It should leave the request for later executable pricing", func() {
			So(visibleAskQuantity(0, requested), ShouldEqual, requested)
			So(visibleAskQuantity(-1, requested), ShouldEqual, requested)
			So(visibleAskQuantity(1, nil), ShouldBeNil)
		})
	})
}

func TestOccupiedSymbols(t *testing.T) {
	Convey("Given no desk", t, func() {
		Convey("It should report an empty occupancy map", func() {
			So(occupiedSymbols(nil), ShouldResemble, map[string]bool{})
		})
	})
}

func admissionCandidate(symbol string, utility, graphScore float64) *types.Decision {
	decision := types.NewDecision(types.ActionEnter, symbol)
	decision.Utility = utility
	decision.GraphScore = graphScore
	decision.Stoploss = &types.Stoploss{}

	return decision
}

func cloneAdmission(decisions []*types.Decision) []*types.Decision {
	cloned := make([]*types.Decision, len(decisions))

	for index, decision := range decisions {
		if decision == nil {
			continue
		}

		copyDecision := *decision
		cloned[index] = &copyDecision
	}

	return cloned
}

func enteredSymbols(decisions []*types.Decision) []string {
	symbols := make([]string, 0, len(decisions))

	for _, decision := range decisions {
		if decision == nil || decision.Action != types.ActionEnter {
			continue
		}

		symbols = append(symbols, decision.Symbol)
	}

	slices.Sort(symbols)

	return symbols
}

func decisionBySymbol(decisions []*types.Decision, symbol string) *types.Decision {
	for _, decision := range decisions {
		if decision != nil && decision.Symbol == symbol {
			return decision
		}
	}

	return nil
}

func BenchmarkAllocationCalculate(b *testing.B) {
	decisions := []*types.Decision{
		types.NewDecision(types.ActionNothing, "BTC/USD"),
	}
	allocation := NewAllocation(b.Context(), nil)

	for b.Loop() {
		if err := allocation.Calculate(decisions); err != nil {
			b.Fatal(err)
		}
	}
}
