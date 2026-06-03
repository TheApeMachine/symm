package mcts

import (
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/cooccurrence"
	"github.com/theapemachine/symm/optimizer/playbook"
	"github.com/theapemachine/symm/optimizer/profile"
)

/*
Moves enumerates and applies branch expansions for MCTS.
*/
type Moves struct {
	profile           *profile.Profile
	coOccurrence      *cooccurrence.CoOccurrenceIndex
	maxThresholds     int
	maxReasoningSteps int
	cached            []Move
}

func NewMoves(
	profile *profile.Profile,
	coOccurrence *cooccurrence.CoOccurrenceIndex,
	maxThresholds int,
	maxReasoningSteps int,
) *Moves {
	moves := &Moves{
		profile:           profile,
		coOccurrence:      coOccurrence,
		maxThresholds:     maxThresholds,
		maxReasoningSteps: maxReasoningSteps,
	}
	moves.cached = moves.Generate()

	return moves
}

func (moves *Moves) Cached() []Move {
	return moves.cached
}

func (moves *Moves) Generate() []Move {
	categories := moves.profile.Categories()
	generated := make([]Move, 0)

	for _, category := range categories {
		for _, observation := range searchObservations {
			actions := moves.actions(observation)

			for _, regime := range searchRegimes {
				for _, unit := range searchUnits {
					values := moves.profile.AdaptiveValues(
						category,
						unit,
						moves.maxThresholds,
					)

					for _, condition := range searchConditions {
						for _, value := range values {
							for _, action := range actions {
								generated = append(generated, Move{
									category:    category,
									observation: observation,
									regime:      regime,
									condition:   condition,
									unit:        unit,
									value:       value,
									action:      action,
								})
							}
						}
					}
				}
			}
		}
	}

	return generated
}

func (moves *Moves) Available(branches perspectives.BranchList) []Move {
	if playbook.ReasoningDepth(branches) >= moves.maxReasoningSteps {
		return nil
	}

	return moves.reachable(moves.cached, branches)
}

func (moves *Moves) Apply(
	branches perspectives.BranchList, move Move,
) perspectives.BranchList {
	branch := moves.branchFromMove(move)

	if len(branches) == 0 {
		return perspectives.BranchList{branch}
	}

	switch move.observation {
	case perspectives.ObservationNone:
		if perspectives.FindEntryIndex(branches) >= 0 {
			nested, ok := playbook.NestGateUnderEntry(branches, branch)

			if ok {
				return nested
			}
		}

		return branches.Clone()
	case perspectives.ObservationNotHolding:
		return playbook.AppendEntryPathSibling(branches, branch)
	case perspectives.ObservationHolding:
		return playbook.AppendExitSibling(branches, branch)
	default:
		return append(branches.Clone(), branch)
	}
}

func (moves *Moves) actions(
	observation perspectives.ObservationType,
) []perspectives.ActionType {
	switch observation {
	case perspectives.ObservationNotHolding:
		return searchEntryActions
	case perspectives.ObservationHolding:
		return searchExitActions
	default:
		return []perspectives.ActionType{perspectives.ActionNone}
	}
}

func (moves *Moves) branchFromMove(move Move) perspectives.Branch {
	return perspectives.Branch{
		Category:    move.category,
		Observation: move.observation,
		Regime:      move.regime,
		Condition:   move.condition,
		Unit:        move.unit,
		Value:       move.value,
		ValueSet:    true,
		Action: perspectives.Action{
			Type: move.action,
		},
	}
}

func (moves *Moves) reachable(
	candidates []Move, branches perspectives.BranchList,
) []Move {
	reachable := make([]Move, 0, len(candidates))

	for _, move := range candidates {
		allowed, theoretical, uctDiscount := moves.moveReachability(move, branches)

		if !allowed {
			continue
		}

		if !moves.moveCompatible(branches, move) {
			continue
		}

		move.theoretical = theoretical
		move.uctDiscount = uctDiscount
		reachable = append(reachable, move)
	}

	return reachable
}

func (moves *Moves) MoveReachability(
	move Move, branches perspectives.BranchList,
) (allowed bool, theoretical bool, uctDiscount float64) {
	return moves.moveReachability(move, branches)
}

func (moves *Moves) moveReachability(
	move Move, branchList perspectives.BranchList,
) (allowed bool, theoretical bool, uctDiscount float64) {
	if moves.profile.CategoryCount(move.category) == 0 {
		return false, false, 0
	}

	if moves.profile.GatePassCount(
		move.category, move.unit, move.condition, move.value,
	) <= 0 {
		return false, false, 0
	}

	if moves.coOccurrence == nil {
		return true, false, 1
	}

	chain := moves.moveChainCategories(move, branchList)
	reachScore := moves.coOccurrence.ChainReachabilityScore(chain)

	if reachScore <= 0 {
		return false, false, 0
	}

	theoretical = reachScore < 1

	return true, theoretical, reachScore
}

func (moves *Moves) moveChainCategories(
	move Move, branches perspectives.BranchList,
) []perspectives.CategoryType {
	switch move.observation {
	case perspectives.ObservationNone:
		return append(cooccurrence.EntryPathCategories(branches), move.category)
	case perspectives.ObservationNotHolding:
		if len(branches) == 0 {
			return []perspectives.CategoryType{move.category}
		}

		return append(cooccurrence.EntryPathCategories(branches), move.category)
	case perspectives.ObservationHolding:
		return []perspectives.CategoryType{move.category}
	default:
		return append(cooccurrence.CategoriesInBranchList(branches), move.category)
	}
}

func (moves *Moves) moveCompatible(
	branches perspectives.BranchList, move Move,
) bool {
	if len(branches) == 0 {
		return true
	}

	branch := moves.branchFromMove(move)

	switch move.observation {
	case perspectives.ObservationNone:
		if entryIndex := perspectives.FindEntryIndex(branches); entryIndex >= 0 {
			entry := branches[entryIndex]

			if anchor, ok := playbook.LastEntryChainGate(entry); ok {
				return playbook.IsBranchCompatible(anchor, branch)
			}
		}

		if anchor, ok := playbook.LastGateBranch(branches); ok {
			return playbook.IsBranchCompatible(anchor, branch)
		}

		return true
	default:
		return true
	}
}

func (moves *Moves) chainReachabilityScore(
	move Move, branches perspectives.BranchList,
) float64 {
	if moves.coOccurrence == nil {
		return 1
	}

	chain := moves.moveChainCategories(move, branches)

	return moves.coOccurrence.ChainReachabilityScore(chain)
}

func (moves *Moves) chainSupport(
	move Move, branches perspectives.BranchList,
) int {
	if moves.coOccurrence == nil {
		return 0
	}

	chain := moves.moveChainCategories(move, branches)

	return moves.coOccurrence.ChainSupport(chain)
}
