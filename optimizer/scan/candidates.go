package scan

import (
	"sort"

	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/beam"
	"github.com/theapemachine/symm/optimizer/budget"
)

func (search *ScanSearch) rankedEntryBranchers() []perspectives.Branch {
	branchers := search.branchers()

	if len(branchers) <= search.options.BeamWidth {
		return branchers
	}

	type rankedBrancher struct {
		branch perspectives.Branch
		score  float64
		passes int
	}

	ranked := make([]rankedBrancher, 0, len(branchers))
	byCategory := make(map[perspectives.CategoryType][]rankedBrancher)

	for _, brancher := range branchers {
		passes := search.profile.GatePassCount(
			brancher.Category,
			brancher.Unit,
			brancher.Condition,
			brancher.Value,
		)
		entry := rankedBrancher{
			branch: brancher,
			score: search.profile.GateSelectivityScore(
				brancher.Category,
				brancher.Unit,
				brancher.Condition,
				brancher.Value,
			),
			passes: passes,
		}

		ranked = append(ranked, entry)
		byCategory[brancher.Category] = append(byCategory[brancher.Category], entry)
	}

	less := func(left, right rankedBrancher) bool {
		if left.score != right.score {
			return left.score > right.score
		}

		return left.passes > right.passes
	}

	for category := range byCategory {
		sort.Slice(byCategory[category], func(leftIndex, rightIndex int) bool {
			return less(byCategory[category][leftIndex], byCategory[category][rightIndex])
		})
	}

	sort.Slice(ranked, func(leftIndex, rightIndex int) bool {
		return less(ranked[leftIndex], ranked[rightIndex])
	})

	limited := make([]perspectives.Branch, 0, search.options.BeamWidth)
	seen := make(map[string]struct{}, search.options.BeamWidth)
	categories := search.profile.Categories()

	for layer := 0; len(limited) < search.options.BeamWidth; layer++ {
		progress := false

		for _, category := range categories {
			candidates := byCategory[category]

			if layer >= len(candidates) {
				continue
			}

			key := beam.BranchFingerprint(candidates[layer].branch)

			if _, ok := seen[key]; ok {
				continue
			}

			limited = append(limited, candidates[layer].branch)
			seen[key] = struct{}{}
			progress = true

			if len(limited) == search.options.BeamWidth {
				return limited
			}
		}

		if !progress {
			break
		}
	}

	for _, candidate := range ranked {
		if len(limited) == search.options.BeamWidth {
			break
		}

		key := beam.BranchFingerprint(candidate.branch)

		if _, ok := seen[key]; ok {
			continue
		}

		limited = append(limited, candidate.branch)
		seen[key] = struct{}{}
	}

	return limited
}

func (search *ScanSearch) emitDecisionSeeds(send func(scanCandidate) bool) {
	for _, playbook := range budget.BuildDecisionSeedPlaybooks(search.profile, search.coOccurrence) {
		if !send(scanCandidate{
			branches: playbook,
			group:    actionGroupEntry,
		}) {
			return
		}
	}

	for _, playbook := range budget.BuildProfileNestedSeedPlaybooks(search.profile, search.coOccurrence) {
		if !send(scanCandidate{
			branches: playbook,
			group:    actionGroupEntry,
		}) {
			return
		}
	}
}

func (search *ScanSearch) actionBranches() []scanCandidate {
	candidates := make([]scanCandidate, 0)

	for _, branch := range search.branches(perspectives.ObservationNotHolding) {
		for _, actionType := range searchEntryActions {
			if actionType == perspectives.ActionNone {
				continue
			}

			next := branch
			next.Action = perspectives.Action{Type: actionType}
			candidates = append(candidates, scanCandidate{
				branches: perspectives.BranchList{next},
				group:    actionGroupEntry,
			})
		}
	}

	for _, branch := range search.branches(perspectives.ObservationHolding) {
		for _, actionType := range searchExitActions {
			if actionType == perspectives.ActionNone {
				continue
			}

			next := branch
			next.Action = perspectives.Action{Type: actionType}
			candidates = append(candidates, scanCandidate{
				branches: perspectives.BranchList{next},
				group:    actionGroupExit,
			})
		}
	}

	return candidates
}

func (search *ScanSearch) branchers() []perspectives.Branch {
	return search.branches(perspectives.ObservationNone)
}

func (search *ScanSearch) branches(
	observation perspectives.ObservationType,
) []perspectives.Branch {
	categories := search.profile.Categories()
	branches := make([]perspectives.Branch, 0)

	for _, category := range categories {
		for _, unit := range searchUnits {
			values := search.profile.Values(
				category,
				unit,
				search.options.MaxThresholds,
			)

			for _, condition := range searchConditions {
				for _, value := range values {
					// Skip vacuous gates (e.g. snr >= 0) that fire on every or
					// no reading of the category — they carry no signal.
					if !search.profile.IsInformativeGate(category, unit, condition, value) {
						continue
					}

					branches = append(branches, perspectives.Branch{
						Category:    category,
						Observation: observation,
						Condition:   condition,
						Unit:        unit,
						Value:       value,
						ValueSet:    true,
					})
				}
			}
		}
	}

	return branches
}

func (search *ScanSearch) groupCandidates(
	candidates []scanCandidate,
	group actionGroup,
) []scanCandidate {
	grouped := make([]scanCandidate, 0, search.options.BeamWidth)

	for _, candidate := range candidates {
		if candidate.group != group {
			continue
		}

		grouped = append(grouped, candidate)

		if len(grouped) == search.options.BeamWidth {
			return grouped
		}
	}

	return grouped
}
