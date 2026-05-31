package perspectives

import (
	"math"
	"math/rand"
	"time"
)

/*
MutateDocument returns a randomized playbook document for replay tuning. It
mutates both numeric thresholds and sibling composition while preserving validity.
*/
func MutateDocument(source Document, random *rand.Rand) Document {
	if random == nil {
		random = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	document := cloneDocument(source)

	for playbookIndex := range document.Playbooks {
		playbook := &document.Playbooks[playbookIndex]
		playbook.Entry = mutateBranchList(playbook.Entry, random)
		playbook.Deny = mutateBranchList(playbook.Deny, random)
		playbook.Exit = mutateBranchList(playbook.Exit, random)
		playbook.Deny = pruneEntryShadowingDenies(playbook.Deny, playbook.Entry)
	}

	return document
}

func cloneDocument(source Document) Document {
	document := Document{
		Version:   source.Version,
		Playbooks: make([]PlaybookSpec, len(source.Playbooks)),
	}

	for index, playbook := range source.Playbooks {
		document.Playbooks[index] = PlaybookSpec{
			Name:   playbook.Name,
			Regime: playbook.Regime,
			Policy: playbook.Policy,
			Deny:   cloneBranchList(playbook.Deny),
			Entry:  cloneBranchList(playbook.Entry),
			Exit:   cloneBranchList(playbook.Exit),
		}
	}

	return document
}

/*
CloneDocument returns a deep copy for storing the tuning incumbent without aliasing
search mutations.
*/
func CloneDocument(source Document) Document {
	return cloneDocument(source)
}

func cloneBranchList(branches []BranchSpec) []BranchSpec {
	if branches == nil {
		return nil
	}

	out := make([]BranchSpec, len(branches))

	for index, branch := range branches {
		out[index] = cloneBranch(branch)
	}

	return out
}

func cloneBranch(branch BranchSpec) BranchSpec {
	out := branch
	out.Any = cloneBranchList(branch.Any)
	out.All = cloneBranchList(branch.All)
	out.Branches = cloneBranchList(branch.Branches)

	if branch.Value != nil {
		value := *branch.Value
		out.Value = &value
	}

	return out
}

func pruneEntryShadowingDenies(
	deny []BranchSpec,
	entry []BranchSpec,
) []BranchSpec {
	if len(deny) == 0 {
		return deny
	}

	entryCategories := branchCategorySet(entry)

	if len(entryCategories) == 0 {
		return deny
	}

	return pruneDenyCategories(deny, entryCategories)
}

func branchCategorySet(branches []BranchSpec) map[string]struct{} {
	categories := make(map[string]struct{})

	for _, branch := range branches {
		collectBranchSpecCategories(branch, categories)
	}

	return categories
}

func collectBranchSpecCategories(
	branch BranchSpec,
	categories map[string]struct{},
) {
	if branch.Category != "" {
		categories[cleanName(branch.Category)] = struct{}{}
	}

	for _, child := range branch.Any {
		collectBranchSpecCategories(child, categories)
	}

	for _, child := range branch.All {
		collectBranchSpecCategories(child, categories)
	}

	for _, child := range branch.Branches {
		collectBranchSpecCategories(child, categories)
	}
}

func pruneDenyCategories(
	branches []BranchSpec,
	entryCategories map[string]struct{},
) []BranchSpec {
	out := make([]BranchSpec, 0, len(branches))

	for _, branch := range branches {
		if branch.Category != "" {
			if _, blocksEntry := entryCategories[cleanName(branch.Category)]; blocksEntry {
				continue
			}
		}

		branch.Any = pruneDenyCategories(branch.Any, entryCategories)
		branch.All = pruneDenyCategories(branch.All, entryCategories)
		branch.Branches = pruneDenyCategories(branch.Branches, entryCategories)
		out = append(out, branch)
	}

	return out
}

func mutateBranchList(branches []BranchSpec, random *rand.Rand) []BranchSpec {
	if len(branches) == 0 {
		return branches
	}

	out := make([]BranchSpec, 0, len(branches))

	for _, branch := range branches {
		if len(branches) > 1 && random.Float64() < 0.20 {
			continue
		}

		out = append(out, mutateBranch(branch, random))
	}

	if len(out) == 0 {
		out = append(out, mutateBranch(branches[random.Intn(len(branches))], random))
	}

	return out
}

func mutateBranch(branch BranchSpec, random *rand.Rand) BranchSpec {
	branch.Any = mutateBranchList(branch.Any, random)
	branch.All = mutateBranchList(branch.All, random)
	branch.Branches = mutateBranchList(branch.Branches, random)

	if branch.Metric == MetricScoreCostRatio {
		branch.Value = floatPtr(quantized(random, 0.70, 2.50, 0.10))

		return branch
	}

	if branch.Category != "" && random.Float64() < 0.35 {
		branch.Value = floatPtr(quantized(random, 0.70, 1.80, 0.05))
	}

	return branch
}

func quantized(random *rand.Rand, min float64, max float64, step float64) float64 {
	value := min + random.Float64()*(max-min)

	if step <= 0 {
		return value
	}

	return math.Round(value/step) * step
}

func floatPtr(value float64) *float64 {
	return &value
}
