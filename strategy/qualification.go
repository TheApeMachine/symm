package strategy

import (
	"fmt"
	"slices"
	"strings"

	"github.com/theapemachine/nomagique/mcts"
	logicgraph "github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/types"
)

/*
predictiveReadiness reports the predictive coder's current calibration state.
It is diagnostic evidence and may qualify the emergency reserve lane, but it is
not a general entry veto: ordinary admission is owned by the explicit policy.
*/
func predictiveReadiness(graph *logicgraph.Graph) (bool, string) {
	if graph == nil {
		return false, "market graph unavailable"
	}

	if !graph.TaskSkillReady {
		return false, "task skill is still calibrating"
	}

	if graph.TaskSkill < 0.5 {
		return false, "task skill is below the zero-return baseline"
	}

	if graph.Forecast == nil || !graph.Forecast.Ready {
		return false, "no calibrated regime-transition posterior is published"
	}

	if graph.ForecastHorizon < 1 {
		return false, "no transition horizon is statistically supported"
	}

	return true, "baseline-or-better task skill with a supported transition horizon"
}

/*
reserveQualification protects reserve slots for abrupt, one-horizon
opportunities. A broad structural opportunity remains visible through
OpportunityType, but it cannot consume emergency capacity unless predictive
coding independently supports the shortest transition and the graph has
classified an actual sudden-pump opportunity.
*/
func reserveQualification(
	opportunityType types.OpportunityType,
	predictiveReady bool,
	horizon int,
) (bool, string) {
	if !predictiveReady {
		return false, "predictive coder is not ready"
	}

	if horizon != 1 {
		return false, "reserve lane requires the shortest supported horizon"
	}

	switch opportunityType {
	case types.OpportunitySuddenPump:
		return true, "sudden-pump precursor with one-horizon predictive support"
	default:
		return false, "structural opportunity is not an emergency reserve setup"
	}
}

/*
graphOpportunityType names a precursor family only when one of the graph's
supporting category nodes already carries that semantics. It does not infer an
opportunity type from price movement or from a generic positive score.
*/
func graphOpportunityType(graph *logicgraph.Graph) string {
	if graph == nil || graph.DecisionTarget == "" {
		return ""
	}

	preferred := map[types.CategoryType]bool{
		types.CoiledCompression: true,
		types.VerticalIgnition:  true,
		types.RiskOnSurge:       true,
		types.InefficientLag:    true,
		types.HiddenAbsorption:  true,
		types.DecoupledAlpha:    true,
		types.EndogenousAlpha:   true,
		types.LiquidityVacuum:   true,
		types.ExtremeScarcity:   true,
	}

	best := types.CategoryTypeNone
	bestMass := 0.0

	for _, edge := range graph.Edges {
		if edge == nil || edge.To != graph.DecisionTarget ||
			edge.Relation != logicgraph.RelationSupports {
			continue
		}

		node := graph.Nodes[edge.From]

		if node == nil || node.Kind != logicgraph.KindCategory {
			continue
		}

		categoryValue, _ := node.Metadata["type"].(string)
		category := types.CategoryType(categoryValue)

		if !preferred[category] {
			continue
		}

		mass := edge.Weight * edge.Confidence

		if mass > bestMass {
			best = category
			bestMass = mass
		}
	}

	return string(best)
}

func decisionTrace(
	graph *logicgraph.Graph,
	root *mcts.Node,
	recommended float64,
	iterations int,
) *types.DecisionTrace {
	summary := graph.OpportunitySummary()
	branches := make([]types.DecisionMCTSBranch, 0, len(root.Children))
	roots := graph.Roots()

	for _, branch := range root.Children {
		meanReward := 0.0

		if branch.Visits > 0 {
			meanReward = branch.TotalReward / float64(branch.Visits)
		}

		branches = append(branches, types.DecisionMCTSBranch{
			Action:     portfolioActionLabel(root, roots, branch.Action),
			Visits:     branch.Visits,
			MeanReward: meanReward,
		})
	}

	slices.SortFunc(branches, func(left, right types.DecisionMCTSBranch) int {
		if left.Visits == right.Visits {
			return 0
		}

		if left.Visits > right.Visits {
			return -1
		}

		return 1
	})

	return &types.DecisionTrace{
		Hypothesis:       summary.Hypothesis,
		GraphSupports:    summary.Support,
		GraphContradicts: summary.Contradiction,
		GraphConditions:  summary.Conditions,
		ThesisBalance:    summary.Balance,
		ThesisConfidence: summary.Confidence,
		MCTS: types.DecisionMCTSTrace{
			Iterations:        iterations,
			Branches:          branches,
			RecommendedAction: portfolioActionLabel(root, roots, recommended),
			Tree:              root,
		},
	}
}

/*
portfolioActionLabel names one branch action depending on what kind of tree
produced it. Portfolio-tree actions address (symbol, intervention) pairs while
single-symbol graph trees address graph roots, so both stay honest in the same
trace contract.
*/
func portfolioActionLabel(
	root *mcts.Node,
	roots []string,
	action float64,
) string {
	state, portfolio := root.State.(*PortfolioState)

	if !portfolio {
		return graphActionLabel(roots, action)
	}

	if action == portfolioDoneAction {
		return "done"
	}

	index, intervened := decodePortfolioAction(action)

	if index < 0 || index >= len(state.legs) {
		return "done"
	}

	symbol := state.legs[index].Symbol

	switch {
	case action == portfolioEnterReference(index):
		return symbol + ":enter"
	case action == portfolioHoldReference(index):
		return symbol + ":hold"
	case !intervened:
		return symbol + ":done"
	default:
		return fmt.Sprintf("%s:%g", symbol, action)
	}
}

func graphActionLabel(roots []string, action float64) string {
	index := int(action)

	if index >= 0 && index < len(roots) && action == float64(index) {
		return roots[index]
	}

	return fmt.Sprintf("root[%g]", action)
}

/*
decisionTracePortfolio builds the observable trace for a held-lot exit decision
produced by the portfolio search: just the branches that touched this symbol.
*/
func decisionTracePortfolio(root *mcts.Node, symbol string) *types.DecisionTrace {
	branches := make([]types.DecisionMCTSBranch, 0, len(root.Children))

	for _, branch := range root.Children {
		label := portfolioActionLabel(root, nil, branch.Action)

		if !strings.Contains(label, symbol+":") {
			continue
		}

		meanReward := 0.0

		if branch.Visits > 0 {
			meanReward = branch.TotalReward / float64(branch.Visits)
		}

		branches = append(branches, types.DecisionMCTSBranch{
			Action:     label,
			Visits:     branch.Visits,
			MeanReward: meanReward,
		})
	}

	return &types.DecisionTrace{
		Hypothesis: "held:" + symbol + ":continuation",
		MCTS: types.DecisionMCTSTrace{
			Branches: branches,
			Tree:     root,
		},
	}
}

func isExcludedSymbol(symbol string) bool {
	base := symbol

	if index := strings.Index(symbol, "/"); index != -1 {
		base = symbol[:index]
	}

	switch strings.ToUpper(strings.TrimSpace(base)) {
	case "USD", "EUR", "GBP", "AUD", "CAD", "CHF", "JPY", "NZD",
		"USDT", "USDC", "DAI", "PYUSD", "FDUSD", "TUSD", "USDG",
		"USDE", "EURT", "EURC", "GUSD", "BUSD", "FRAX", "LUSD",
		"CUSD", "USD0", "USDS", "RLUSD", "UST":
		return true
	default:
		return false
	}
}
