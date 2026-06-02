package optimizer

import (
	"github.com/theapemachine/symm/market/perspectives"
)

/*
Decision seed chains follow DECISION.md stage ordering: systemic filter, origin,
quality, timing, then exit alternatives. Only chains reachable on the tape are scored.
*/
var (
	decisionDenyCategories = []perspectives.CategoryType{
		perspectives.CategoryToxicBluff,
		perspectives.CategorySaturation,
		perspectives.CategoryTurbulent,
		perspectives.CategoryLiquidityShock,
		perspectives.CategoryMechanicalCollapse,
		perspectives.CategorySystemicBeta,
		perspectives.CategorySystemicHerd,
		perspectives.CategorySpoofTrap,
		perspectives.CategorySystemicSlump,
	}

	decisionEntryChains = [][]perspectives.CategoryType{
		{
			perspectives.CategoryRiskOnSurge,
			perspectives.CategoryEndogenousAlpha,
			perspectives.CategoryFrenzy,
			perspectives.CategoryAggressiveDrive,
		},
		{
			perspectives.CategoryRiskOnSurge,
			perspectives.CategoryEndogenousAlpha,
			perspectives.CategoryLaminar,
			perspectives.CategoryAggressiveDrive,
		},
		{
			perspectives.CategoryRiskOnSurge,
			perspectives.CategoryEndogenousAlpha,
			perspectives.CategoryInertial,
			perspectives.CategoryAggressiveDrive,
		},
		{
			perspectives.CategoryRiskOnSurge,
			perspectives.CategoryEndogenousAlpha,
			perspectives.CategoryHardSupport,
			perspectives.CategoryLoadedImbalance,
		},
		{
			perspectives.CategoryDecoupledMove,
			perspectives.CategoryInefficientLag,
		},
		{
			perspectives.CategoryDecoupledAlpha,
			perspectives.CategoryInefficientLag,
		},
		{
			perspectives.CategoryHiddenAbsorption,
			perspectives.CategoryAggressiveDrive,
		},
		{
			perspectives.CategoryLaminar,
			perspectives.CategoryFrenzy,
		},
		{
			perspectives.CategoryInertial,
			perspectives.CategoryFrenzy,
		},
		{
			perspectives.CategoryVerticalIgnition,
		},
		{
			perspectives.CategoryCoiledCompression,
		},
		{
			perspectives.CategoryExtremeScarcity,
			perspectives.CategoryVerticalIgnition,
		},
	}

	decisionExitCategories = []perspectives.CategoryType{
		perspectives.CategoryActiveReversal,
		perspectives.CategoryThermalExhaustion,
		perspectives.CategoryFadedExhaustion,
		perspectives.CategoryMechanicalCollapse,
		perspectives.CategoryFragileExpansion,
		perspectives.CategoryAnchorStall,
	}
)

/*
BuildDecisionSeedPlaybooks materializes DECISION.md templates that are reachable
on the measurement tape.
*/
func BuildDecisionSeedPlaybooks(
	profile *Profile,
	index *CoOccurrenceIndex,
) []perspectives.BranchList {
	if index == nil {
		return nil
	}

	playbooks := make([]perspectives.BranchList, 0)

	for _, chain := range decisionEntryChains {
		prefixes := reachableEntryChainPrefixes(index, chain)

		for _, prefix := range prefixes {
			if len(prefix) == 0 {
				continue
			}

			entryRoot := branchFromCategoryChain(
				profile, prefix, perspectives.ObservationNotHolding, perspectives.ActionLimit,
			)

			for _, exitCategory := range decisionExitCategories {
				if !index.categorySeen(exitCategory) {
					continue
				}

				exit := gateBranch(
					profile, exitCategory, perspectives.ObservationHolding, perspectives.ActionSettlePosition,
				)
				entryWrapped := perspectives.NestDenyGatesAroundEntry(
					entryRoot, decisionDenyPlaybook(profile, index),
				)
				playbook := append(perspectives.BranchList{entryWrapped}, exit)
				playbooks = append(playbooks, playbook)
			}
		}
	}

	return playbooks
}

func reachableEntryChainPrefixes(
	index *CoOccurrenceIndex,
	chain []perspectives.CategoryType,
) [][]perspectives.CategoryType {
	if len(chain) == 0 {
		return nil
	}

	prefixes := make([][]perspectives.CategoryType, 0)

	for end := 1; end <= len(chain); end++ {
		prefix := chain[:end]

		if !index.ChainReachable(prefix) {
			continue
		}

		prefixes = append(prefixes, append([]perspectives.CategoryType(nil), prefix...))
	}

	return prefixes
}

func decisionDenyPlaybook(
	profile *Profile,
	index *CoOccurrenceIndex,
) perspectives.BranchList {
	deny := make(perspectives.BranchList, 0)

	for _, category := range decisionDenyCategories {
		if !index.categorySeen(category) {
			continue
		}

		deny = append(
			deny,
			gateBranch(profile, category, perspectives.ObservationNone, perspectives.ActionNone),
		)
	}

	return deny
}

func branchFromCategoryChain(
	profile *Profile,
	chain []perspectives.CategoryType,
	observation perspectives.ObservationType,
	action perspectives.ActionType,
) perspectives.Branch {
	if len(chain) == 0 {
		return perspectives.Branch{}
	}

	leaf := gateBranch(profile, chain[len(chain)-1], observation, action)

	for index := len(chain) - 2; index >= 0; index-- {
		leaf = perspectives.Branch{
			Category:    chain[index],
			Observation: perspectives.ObservationNone,
			Condition:   perspectives.ConditionIsGreaterThanOrEqual,
			Unit:        perspectives.UnitSNR,
			Value:       profile.Quantile(chain[index], perspectives.UnitSNR, 0.5),
			ValueSet:    true,
			Branches:    []perspectives.Branch{leaf},
		}
	}

	return leaf
}

func gateBranch(
	profile *Profile,
	category perspectives.CategoryType,
	observation perspectives.ObservationType,
	action perspectives.ActionType,
) perspectives.Branch {
	return perspectives.Branch{
		Category:    category,
		Observation: observation,
		Condition:   perspectives.ConditionIsGreaterThanOrEqual,
		Unit:        perspectives.UnitSNR,
		Value:       profile.Quantile(category, perspectives.UnitSNR, 0.5),
		ValueSet:    true,
		Action:      perspectives.Action{Type: action},
	}
}
