package market

import "github.com/theapemachine/symm/market/perspectives"

/*
MostUrgentExit picks the highest-priority exit from parallel playbook verdicts.
*/
func MostUrgentExit(decisions []Decision) *perspectives.ActionType {
	var best *perspectives.ActionType
	bestRank := 0

	for _, verdict := range decisions {
		if !perspectives.IsExitAction(verdict.Action) {
			continue
		}

		rank := perspectives.ExitUrgency(verdict.Action)

		if rank > bestRank {
			action := verdict.Action
			best = &action
			bestRank = rank
		}
	}

	return best
}
