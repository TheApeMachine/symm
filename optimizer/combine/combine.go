package combine

import (
	"sort"
	"strconv"
	"strings"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
Scored pairs a discovered strategy playbook with its standalone replay score.
*/
type Scored struct {
	Branches perspectives.BranchList
	Score    float64
}

/*
Greedy is the second search round: it composes several distinct single-strategy
playbooks into one multi-entry playbook, where each strategy stays an independent
gated entry subtree (the evaluator fires the deepest reachable entry per tick).

The pool is first reduced to the best representative per entry family, ordered by
standalone score, then merged one at a time. A strategy is kept only when adding
it strictly raises the JOINT replay score from scoreFn, so combination never
regresses below the best single strategy and only admits strategies that pull
their weight together. Returns the canonical combined playbook, or nil for an
empty pool.
*/
func Greedy(
	pool []Scored, scoreFn func(perspectives.BranchList) float64,
) perspectives.BranchList {
	families := bestPerFamily(pool)

	if len(families) == 0 {
		return nil
	}

	sort.SliceStable(families, func(left, right int) bool {
		return families[left].Score > families[right].Score
	})

	combined := perspectives.CanonicalPlaybookBranches(families[0].Branches)
	best := scoreFn(combined)

	for _, candidate := range families[1:] {
		trial := perspectives.CanonicalPlaybookBranches(
			unionStrategies(combined, candidate.Branches),
		)

		trialScore := scoreFn(trial)

		if trialScore > best {
			combined = trial
			best = trialScore
		}
	}

	return combined
}

/*
bestPerFamily keeps the highest-scoring playbook per entry-category family so two
variants of the same strategy never both enter the merge.
*/
func bestPerFamily(pool []Scored) []Scored {
	best := make(map[string]Scored)

	for _, candidate := range pool {
		if len(perspectives.FindAllEntryIndices(candidate.Branches)) == 0 {
			continue
		}

		key := familyKey(candidate.Branches)
		existing, ok := best[key]

		if !ok || candidate.Score > existing.Score {
			best[key] = candidate
		}
	}

	families := make([]Scored, 0, len(best))

	for _, candidate := range best {
		families = append(families, candidate)
	}

	return families
}

/*
unionStrategies merges the entry roots and exit branches of two playbooks,
de-duplicating identical subtrees. Distinct entries become sibling strategies;
shared exits collapse to one.
*/
func unionStrategies(left, right perspectives.BranchList) perspectives.BranchList {
	merged := make(perspectives.BranchList, 0)
	seen := make(map[string]struct{})

	add := func(branch perspectives.Branch) {
		key := fingerprint(branch)

		if _, ok := seen[key]; ok {
			return
		}

		seen[key] = struct{}{}
		merged = append(merged, branch.Clone())
	}

	for _, list := range []perspectives.BranchList{left, right} {
		entries := entrySet(list)

		for index := range entries {
			add(list[index])
		}

		for index, branch := range list {
			if _, isEntry := entries[index]; isEntry {
				continue
			}

			if branch.Observation == perspectives.ObservationHolding {
				add(branch)
			}
		}
	}

	return merged
}

func entrySet(branches perspectives.BranchList) map[int]struct{} {
	set := make(map[int]struct{})

	for _, index := range perspectives.FindAllEntryIndices(branches) {
		set[index] = struct{}{}
	}

	return set
}

/*
familyKey identifies a strategy by the sorted set of categories on its entry
paths, so different gating or entry triggers count as different strategies.
*/
func familyKey(branches perspectives.BranchList) string {
	categories := make(map[perspectives.CategoryType]struct{})

	for index := range entrySet(branches) {
		collectCategories(branches[index], categories)
	}

	return joinCategories(categories)
}

func collectCategories(
	branch perspectives.Branch, into map[perspectives.CategoryType]struct{},
) {
	if branch.Category != perspectives.CategoryTypeNone {
		into[branch.Category] = struct{}{}
	}

	for _, child := range branch.Branches {
		collectCategories(child, into)
	}
}

func joinCategories(set map[perspectives.CategoryType]struct{}) string {
	parts := make([]string, 0, len(set))

	for category := range set {
		parts = append(parts, string(category))
	}

	sort.Strings(parts)

	return strings.Join(parts, ",")
}

/*
fingerprint is a stable structural signature of a branch subtree, used to
de-duplicate identical entries or exits during a union.
*/
func fingerprint(branch perspectives.Branch) string {
	var builder strings.Builder

	writeFingerprint(&builder, branch)

	return builder.String()
}

func writeFingerprint(builder *strings.Builder, branch perspectives.Branch) {
	builder.WriteString(string(branch.Category))
	builder.WriteByte('|')
	builder.WriteString(strconv.Itoa(int(branch.Observation)))
	builder.WriteByte('|')
	builder.WriteString(strconv.Itoa(int(branch.Condition)))
	builder.WriteByte('|')
	builder.WriteString(strconv.Itoa(int(branch.Unit)))
	builder.WriteByte('|')
	builder.WriteString(strconv.FormatFloat(branch.Value, 'g', -1, 64))
	builder.WriteByte('|')
	builder.WriteString(strconv.Itoa(int(branch.Action.Type)))
	builder.WriteByte('(')

	for _, child := range branch.Branches {
		writeFingerprint(builder, child)
		builder.WriteByte(';')
	}

	builder.WriteByte(')')
}
