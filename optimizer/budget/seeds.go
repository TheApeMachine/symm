package budget

import (
	"sort"
	"strings"

	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/cooccurrence"
	"github.com/theapemachine/symm/optimizer/profile"
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
		perspectives.CategoryExhaustion,
		perspectives.CategoryActiveReversal,
		perspectives.CategoryThermalExhaustion,
		perspectives.CategoryFadedExhaustion,
		perspectives.CategoryMechanicalCollapse,
		perspectives.CategoryFragileExpansion,
		perspectives.CategoryAnchorStall,
	}

	// Protective exits are price-triggered, not signal-gated, so each seeds an
	// unconditional bracket armed on entry. Seeding one per type gives the search
	// real starting diversity instead of a settle-only monoculture.
	decisionProtectiveExits = []perspectives.ActionType{
		perspectives.ActionStopLoss,
		perspectives.ActionTakeProfit,
		perspectives.ActionTrailingStop,
	}
)

/*
protectiveExitSeed pairs an entry root with a single unconditional protective
exit (armed whenever holding). The replay ledger rests the trigger and fills it
on the price path, so no signal-category gate is needed.
*/
func protectiveExitSeed(
	entryWrapped perspectives.Branch, actionType perspectives.ActionType,
) perspectives.BranchList {
	return perspectives.BranchList{
		entryWrapped,
		perspectives.Branch{
			Observation: perspectives.ObservationHolding,
			Action:      perspectives.Action{Type: actionType},
		},
	}
}

/*
BuildDecisionSeedPlaybooks materializes DECISION.md templates that are reachable
on the measurement tape.
*/
func BuildDecisionSeedPlaybooks(
	profile *profile.Profile,
	index *cooccurrence.CoOccurrenceIndex,
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
				if !index.CategorySeen(exitCategory) {
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

			for _, protective := range decisionProtectiveExits {
				entryWrapped := perspectives.NestDenyGatesAroundEntry(
					entryRoot, decisionDenyPlaybook(profile, index),
				)
				playbooks = append(playbooks, protectiveExitSeed(entryWrapped, protective))
			}
		}
	}

	return playbooks
}

type profileNestedPair struct {
	outer   perspectives.CategoryType
	inner   perspectives.CategoryType
	support int
}

/*
BuildProfileNestedSeedPlaybooks materializes two-gate entry chains from categories
observed on the tape. These seed deepening when DECISION.md templates do not match.
*/
func BuildProfileNestedSeedPlaybooks(
	profile *profile.Profile,
	index *cooccurrence.CoOccurrenceIndex,
) []perspectives.BranchList {
	categories := profile.Categories()

	if len(categories) < 2 {
		return nil
	}

	pairs := rankedProfileNestedPairs(categories, index)
	playbooks := make([]perspectives.BranchList, 0, len(pairs)*len(categories))
	seen := make(map[string]struct{})

	for _, pair := range pairs {
		outerValue, _ := profile.InformativeGreaterEqualThreshold(
			pair.outer, perspectives.UnitSNR, 0.5,
		)
		entryRoot := perspectives.Branch{
			Category:    pair.outer,
			Observation: perspectives.ObservationNone,
			Condition:   perspectives.ConditionIsGreaterThanOrEqual,
			Unit:        perspectives.UnitSNR,
			Value:       outerValue,
			ValueSet:    true,
			Branches: []perspectives.Branch{
				gateBranch(
					profile, pair.inner, perspectives.ObservationNotHolding, perspectives.ActionLimit,
				),
			},
		}

		for _, exitCategory := range categories {
			if exitCategory == pair.inner {
				continue
			}

			exit := gateBranch(
				profile, exitCategory, perspectives.ObservationHolding, perspectives.ActionSettlePosition,
			)
			playbook := append(perspectives.BranchList{entryRoot}, exit)
			key := branchListKey(playbook)

			if _, ok := seen[key]; ok {
				continue
			}

			seen[key] = struct{}{}
			playbooks = append(playbooks, playbook)
		}

		for _, protective := range decisionProtectiveExits {
			playbook := protectiveExitSeed(entryRoot, protective)
			key := branchListKey(playbook)

			if _, ok := seen[key]; ok {
				continue
			}

			seen[key] = struct{}{}
			playbooks = append(playbooks, playbook)
		}
	}

	return playbooks
}

func rankedProfileNestedPairs(
	categories []perspectives.CategoryType,
	index *cooccurrence.CoOccurrenceIndex,
) []profileNestedPair {
	pairs := make([]profileNestedPair, 0, len(categories)*len(categories))

	for _, outer := range categories {
		for _, inner := range categories {
			if outer == inner {
				continue
			}

			support := 0

			if index != nil {
				support = index.ChainSupport([]perspectives.CategoryType{outer, inner})
			}

			pairs = append(pairs, profileNestedPair{
				outer:   outer,
				inner:   inner,
				support: support,
			})
		}
	}

	sort.SliceStable(pairs, func(leftIndex, rightIndex int) bool {
		left := pairs[leftIndex]
		right := pairs[rightIndex]

		if left.support != right.support {
			return left.support > right.support
		}

		if left.outer != right.outer {
			return left.outer < right.outer
		}

		return left.inner < right.inner
	})

	return pairs
}

func reachableEntryChainPrefixes(
	index *cooccurrence.CoOccurrenceIndex,
	chain []perspectives.CategoryType,
) [][]perspectives.CategoryType {
	if len(chain) == 0 {
		return nil
	}

	prefixes := make([][]perspectives.CategoryType, 0)

	for end := 1; end <= len(chain); end++ {
		prefix := chain[:end]

		if index.ChainReachabilityScore(prefix) <= 0 {
			continue
		}

		prefixes = append(prefixes, append([]perspectives.CategoryType(nil), prefix...))
	}

	return prefixes
}

func decisionDenyPlaybook(
	profile *profile.Profile,
	index *cooccurrence.CoOccurrenceIndex,
) perspectives.BranchList {
	deny := make(perspectives.BranchList, 0)

	for _, category := range decisionDenyCategories {
		if !index.CategorySeen(category) {
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
	profile *profile.Profile,
	chain []perspectives.CategoryType,
	observation perspectives.ObservationType,
	action perspectives.ActionType,
) perspectives.Branch {
	if len(chain) == 0 {
		return perspectives.Branch{}
	}

	leaf := gateBranch(profile, chain[len(chain)-1], observation, action)

	for index := len(chain) - 2; index >= 0; index-- {
		gateValue, _ := profile.InformativeGreaterEqualThreshold(
			chain[index], perspectives.UnitSNR, 0.5,
		)
		leaf = perspectives.Branch{
			Category:    chain[index],
			Observation: perspectives.ObservationNone,
			Condition:   perspectives.ConditionIsGreaterThanOrEqual,
			Unit:        perspectives.UnitSNR,
			Value:       gateValue,
			ValueSet:    true,
			Branches:    []perspectives.Branch{leaf},
		}
	}

	return leaf
}

func gateBranch(
	profile *profile.Profile,
	category perspectives.CategoryType,
	observation perspectives.ObservationType,
	action perspectives.ActionType,
) perspectives.Branch {
	value, _ := profile.InformativeGreaterEqualThreshold(category, perspectives.UnitSNR, 0.5)

	return perspectives.Branch{
		Category:    category,
		Observation: observation,
		Condition:   perspectives.ConditionIsGreaterThanOrEqual,
		Unit:        perspectives.UnitSNR,
		Value:       value,
		ValueSet:    true,
		Action:      perspectives.Action{Type: action},
	}
}

func branchListKey(branches perspectives.BranchList) string {
	canonical := perspectives.CanonicalPlaybookBranches(branches)
	parts := make([]string, 0, len(canonical))

	for _, branch := range canonical {
		parts = append(parts, string(branch.Category))
	}

	return strings.Join(parts, "|")
}
