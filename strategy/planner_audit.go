package strategy

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/types"
)

/*
auditDecisions records the compact per-symbol strategy evidence needed to audit
why a thesis was held, entered, rotated, or rejected on this tick.
*/
func (planner *Planner) auditDecisions(thesis *types.Thesis) {
	if planner == nil || thesis == nil {
		return
	}

	for _, decision := range thesis.Decisions {
		if decision.Symbol == "" {
			continue
		}

		lifecycle, _ := lifecycleState(thesis, decision.Symbol)
		errnie.Error(audit.StrategyDecision(
			planner.recorder,
			thesis.Tick,
			lifecycle,
			decision,
		))
	}
}

/*
decisionCounts summarizes current thesis actions for the decide_end phase row.
*/
func decisionCounts(decisions []types.Decision) map[string]any {
	counts := map[string]int{}

	for _, decision := range decisions {
		counts[string(decision.Action)]++
	}

	summary := map[string]any{
		"decisions": len(decisions),
	}

	for action, count := range counts {
		summary[action] = count
	}

	return summary
}
