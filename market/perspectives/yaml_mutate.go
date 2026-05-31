package perspectives

import (
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

	return float64(int(value/step+0.5)) * step
}

func floatPtr(value float64) *float64 {
	return &value
}
