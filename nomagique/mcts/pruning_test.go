package mcts

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestCounterfactualEvidenceDecidesTheWinner is the test that would have caught
the original defect: bestChild gathered counterfactual experience and then
discarded it, ranking only on real rollout means.

The tree is built so the two rankings disagree. The Wait branch has the higher
raw rollout mean; the Enter branch has a lower rollout mean but strong
counterfactual evidence behind it. Ranking on MeanReward picks Wait; ranking
on BlendedValue picks Enter.
*/
func TestCounterfactualEvidenceDecidesTheWinner(t *testing.T) {
	Convey("Given two branches whose raw and blended rankings disagree", t, func() {
		root := &SearchNode{}

		wait := &SearchNode{Action: Wait, Parent: root, Visits: 4, Mean: 1.0}
		enter := &SearchNode{Action: Enter, Parent: root, Visits: 4, Mean: 0.5}

		// Enter never looked better in its own rollouts, but every rollout
		// that took Wait produced a counterfactual saying Enter would have
		// paid far more under the same market noise.
		enter.CounterfactualReward = 12
		enter.CounterfactualMass = 4

		root.Children = []*SearchNode{wait, enter}

		Convey("the raw rollout mean favors Wait", func() {
			So(wait.MeanReward(), ShouldBeGreaterThan, enter.MeanReward())
		})

		Convey("the blended value favors Enter", func() {
			// Enter blends 4 real visits at 0.5 with 4 virtual at 3.0.
			So(enter.BlendedValue(), ShouldEqual, 1.75)
			So(enter.BlendedValue(), ShouldBeGreaterThan, wait.BlendedValue())
		})

		Convey("bestChild selects on the counterfactual evidence", func() {
			best, found := bestChild(root)
			So(found, ShouldBeTrue)
			So(best.Action, ShouldEqual, Enter)
		})
	})
}

/*
TestUngroundedBranchCannotWin holds the line the decision must not cross: a
branch with overwhelming counterfactual support but zero real rollouts is
imagination, and imagination does not spend capital.
*/
func TestUngroundedBranchCannotWin(t *testing.T) {
	Convey("Given a branch supported only by counterfactuals", t, func() {
		root := &SearchNode{}

		grounded := &SearchNode{Action: Wait, Parent: root, Visits: 2, Mean: 0.1}
		imagined := &SearchNode{Action: Enter, Parent: root, Visits: 0}
		imagined.CounterfactualReward = 500
		imagined.CounterfactualMass = 10

		root.Children = []*SearchNode{grounded, imagined}

		Convey("its blended value is enormous", func() {
			So(imagined.BlendedValue(), ShouldBeGreaterThan, grounded.BlendedValue())
		})

		Convey("but the grounded branch still wins the decision", func() {
			best, found := bestChild(root)
			So(found, ShouldBeTrue)
			So(best.Action, ShouldEqual, Wait)
		})
	})
}

func TestCausalRejectionWithdrawsBranchFromSelection(t *testing.T) {
	Convey("Given a search with an armed rejection floor", t, func() {
		// The structural model condemns every action it is asked about.
		engine := &recordingEngine{expectation: -5, precision: 1}

		search := NewSearch(24, 0.5, 0.25, 41)
		search.Causal = engine
		search.CausalPolicy = EconomicCausalPolicy(4, 1, 8, true).
			WithRejectionFloor(0)

		result := search.Run(causalSearchState(), alwaysEstimable{})

		Convey("the search still returns a usable decision", func() {
			So(result.DecisionUnavailable, ShouldBeFalse)
			So(result.Tree, ShouldNotBeNil)
		})

		Convey("rejection never withdraws every branch", func() {
			viable := 0

			for _, child := range result.Tree.Children {
				if !child.Pruned {
					viable++
				}
			}

			So(viable, ShouldBeGreaterThan, 0)
		})
	})
}

func TestRejectionSparesUnidentifiedBranches(t *testing.T) {
	Convey("Given a structural model whose queries never identify", t, func() {
		engine := &recordingEngine{
			expectationErr: errAllQueriesFail,
			counterErr:     errAllQueriesFail,
		}

		search := NewSearch(24, 0.5, 0.25, 43)
		search.Causal = engine
		search.CausalPolicy = EconomicCausalPolicy(4, 1, 8, true).
			WithRejectionFloor(1000)

		result := search.Run(causalSearchState(), alwaysEstimable{})

		Convey("no branch is condemned on absent evidence", func() {
			for _, child := range result.Tree.Children {
				So(child.Pruned, ShouldBeFalse)
			}
		})

		Convey("the search still decides", func() {
			So(result.DecisionUnavailable, ShouldBeFalse)
		})
	})
}

func TestRejectionIsComparativeNotAbsolute(t *testing.T) {
	Convey("Given a declining tape where every expectation is negative", t, func() {
		node := &SearchNode{Visits: 10}

		// The cold-market trap: entering is a disaster, waiting is merely
		// mild friction. An absolute floor at zero would condemn both and
		// leave the search wedged with nothing to select.
		enter := &SearchNode{
			Action: Enter, Parent: node, Visits: 2,
			CausalExpectation: -500, CausalExpectationDefined: true,
		}
		wait := &SearchNode{
			Action: Wait, Parent: node, Visits: 2,
			CausalExpectation: -1, CausalExpectationDefined: true,
		}

		node.Children = []*SearchNode{enter, wait}

		search := NewSearch(4, 0.5, 0.25, 47)
		search.CausalPolicy = CausalPolicy{}.WithRejectionFloor(10)

		Convey("only the dominated branch is withdrawn", func() {
			So(search.prune(node), ShouldBeTrue)
			So(enter.Pruned, ShouldBeTrue)

			Convey("the safe action survives despite its negative expectation", func() {
				So(wait.Pruned, ShouldBeFalse)
			})
		})

		Convey("a second pass finds nothing left to withdraw", func() {
			search.prune(node)
			So(search.prune(node), ShouldBeFalse)
		})
	})
}

func TestRejectionSparesBranchesWithinTheMargin(t *testing.T) {
	Convey("Given two branches that are close together", t, func() {
		node := &SearchNode{Visits: 10}

		leader := &SearchNode{
			Action: Enter, Parent: node, Visits: 2,
			CausalExpectation: 5, CausalExpectationDefined: true,
		}
		near := &SearchNode{
			Action: Wait, Parent: node, Visits: 2,
			CausalExpectation: 1, CausalExpectationDefined: true,
		}

		node.Children = []*SearchNode{leader, near}

		search := NewSearch(4, 0.5, 0.25, 51)
		search.CausalPolicy = CausalPolicy{}.WithRejectionFloor(10)

		Convey("neither is withdrawn, because neither dominates decisively", func() {
			So(search.prune(node), ShouldBeFalse)
			So(leader.Pruned, ShouldBeFalse)
			So(near.Pruned, ShouldBeFalse)
		})
	})
}

func TestUnidentifiedBranchIsExploredNotCondemned(t *testing.T) {
	Convey("Given a dominant branch beside an unidentified one", t, func() {
		node := &SearchNode{Visits: 10}

		identified := &SearchNode{
			Action: Wait, Parent: node, Visits: 2,
			CausalExpectation: 100, CausalExpectationDefined: true,
		}
		unknown := &SearchNode{
			Action: Enter, Parent: node, Visits: 2,
			CausalExpectationDefined: false,
		}

		node.Children = []*SearchNode{identified, unknown}

		search := NewSearch(4, 0.5, 0.25, 59)
		search.CausalPolicy = CausalPolicy{}.WithRejectionFloor(1)

		Convey("absence of evidence is not a veto", func() {
			So(search.prune(node), ShouldBeFalse)
			So(unknown.Pruned, ShouldBeFalse)
		})
	})
}

func TestBestBranchIsNeverPruned(t *testing.T) {
	Convey("Given a zero margin, where every gap dominates", t, func() {
		node := &SearchNode{Visits: 10}

		leader := &SearchNode{
			Action: Wait, Parent: node, Visits: 2,
			CausalExpectation: -1, CausalExpectationDefined: true,
		}
		worse := &SearchNode{
			Action: Enter, Parent: node, Visits: 2,
			CausalExpectation: -2, CausalExpectationDefined: true,
		}

		node.Children = []*SearchNode{leader, worse}

		search := NewSearch(4, 0.5, 0.25, 61)
		search.CausalPolicy = CausalPolicy{}.WithRejectionFloor(0)

		Convey("the reference branch survives its own comparison", func() {
			So(search.prune(node), ShouldBeTrue)
			So(leader.Pruned, ShouldBeFalse)
			So(worse.Pruned, ShouldBeTrue)
		})
	})
}

func TestSelectionSurvivesAFullyPrunedNode(t *testing.T) {
	Convey("Given a node whose every child was withdrawn", t, func() {
		node := &SearchNode{Visits: 10}

		first := &SearchNode{
			Action: Enter, Parent: node, Visits: 1, Pruned: true,
			CausalExpectation: -5, CausalExpectationDefined: true,
		}
		second := &SearchNode{
			Action: Wait, Parent: node, Visits: 1, Pruned: true,
			CausalExpectation: -2, CausalExpectationDefined: true,
		}

		node.Children = []*SearchNode{first, second}

		search := NewSearch(4, 0.5, 0.25, 67)

		Convey("selection reinstates the least-condemned branch instead of panicking", func() {
			selected := search.ucbChild(node, nil)
			So(selected, ShouldNotBeNil)
			So(selected.Action, ShouldEqual, Wait)
			So(selected.Pruned, ShouldBeFalse)
		})
	})
}

func TestPrunedBranchIsNeverReselected(t *testing.T) {
	Convey("Given a pruned branch with an otherwise winning score", t, func() {
		node := &SearchNode{Visits: 10}

		pruned := &SearchNode{
			Action: Enter, Parent: node, Visits: 1, Mean: 1000, Pruned: true,
		}
		open := &SearchNode{Action: Wait, Parent: node, Visits: 1, Mean: 0}

		node.Children = []*SearchNode{pruned, open}

		search := NewSearch(4, 0.5, 0.25, 53)

		Convey("selection skips it despite the higher value", func() {
			So(search.ucbChild(node, nil).Action, ShouldEqual, Wait)
		})
	})
}
