package optimizer

import (
	"bytes"
	"fmt"

	"github.com/theapemachine/symm/logic"
	"go.yaml.in/yaml/v3"
)

type Mutation struct {
	ID          float64
	Name        string
	Description string
	Apply       func([]byte) ([]byte, error)
}

func DefaultMutations() []Mutation {
	return []Mutation{
		minObservationMutation(1, "tighten_entries", 1),
		minObservationMutation(2, "loosen_entries", -1),
		disableEntrySourceMutation(3, logic.SourcePumpDump),
		disableEntrySourceMutation(4, logic.SourceCVD),
		disableEntrySourceMutation(5, logic.SourceHawkes),
		disableEntrySourceMutation(6, logic.SourceSentiment),
		disableEntrySourceMutation(7, logic.SourceFluid),
		disableEntrySourceMutation(8, logic.SourceToxicity),
		disableEntrySourceMutation(9, logic.SourceDepthFlow),
		disableEntrySourceMutation(10, logic.SourceLiquidity),
		disableEntrySourceMutation(11, logic.SourceExhaustion),
		disableEntrySourceMutation(12, logic.SourceCausal),
		disableEntrySourceMutation(13, logic.SourceManifold),
		disableEntrySourceMutation(14, logic.SourceCorrelation),
		disableEntrySourceMutation(15, logic.SourceLeadLag),
		entryActionTypeMutation(16, "prefer_limit_entries", logic.ActionLimit),
		entryActionTypeMutation(17, "prefer_market_entries", logic.ActionMarket),
		entryOpportunitySlotMutation(18, "reserve_opportunity_entries", true),
		entryOpportunitySlotMutation(19, "clear_opportunity_entries", false),
	}
}

func minObservationMutation(id float64, name string, delta int) Mutation {
	return Mutation{
		ID:          id,
		Name:        name,
		Description: fmt.Sprintf("adjust entry confirmation window by %+d observation", delta),
		Apply: func(input []byte) ([]byte, error) {
			tree, err := decodeTree(input)
			if err != nil {
				return nil, err
			}

			for _, branch := range tree.Branches {
				adjustEntryConfirmations(branch, delta)
			}

			return encodeTree(tree)
		},
	}
}

func disableEntrySourceMutation(id float64, source logic.SourceType) Mutation {
	return Mutation{
		ID:          id,
		Name:        "disable_entry_" + string(source),
		Description: "remove entry branches whose evidence depends on " + string(source),
		Apply: func(input []byte) ([]byte, error) {
			tree, err := decodeTree(input)
			if err != nil {
				return nil, err
			}

			tree.Branches = filterBranches(tree.Branches, source)

			return encodeTree(tree)
		},
	}
}

func entryActionTypeMutation(id float64, name string, actionType logic.ActionType) Mutation {
	return Mutation{
		ID:          id,
		Name:        name,
		Description: "set replayable buy entries to " + string(actionType),
		Apply: func(input []byte) ([]byte, error) {
			tree, err := decodeTree(input)
			if err != nil {
				return nil, err
			}

			for _, branch := range tree.Branches {
				walkEntryActions(branch, func(action *logic.Action) {
					action.Type = actionType
				})
			}

			return encodeTree(tree)
		},
	}
}

func entryOpportunitySlotMutation(id float64, name string, enabled bool) Mutation {
	return Mutation{
		ID:          id,
		Name:        name,
		Description: fmt.Sprintf("set buy entry opportunity_slot=%t", enabled),
		Apply: func(input []byte) ([]byte, error) {
			tree, err := decodeTree(input)
			if err != nil {
				return nil, err
			}

			for _, branch := range tree.Branches {
				walkEntryActions(branch, func(action *logic.Action) {
					action.OpportunitySlot = enabled
				})
			}

			return encodeTree(tree)
		},
	}
}

func decodeTree(input []byte) (*logic.Tree, error) {
	tree := &logic.Tree{}
	if err := yaml.Unmarshal(input, tree); err != nil {
		return nil, fmt.Errorf("decode playbook tree: %w", err)
	}

	return tree, nil
}

func encodeTree(tree *logic.Tree) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(tree); err != nil {
		return nil, fmt.Errorf("encode playbook tree: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

func adjustEntryConfirmations(branch *logic.Branch, delta int) {
	if branch == nil {
		return
	}

	if branchContainsEntry(branch) && branch.ConditionGroup != nil {
		next := branch.ConditionGroup.MinObservations + delta
		if next < 1 {
			next = 1
		}
		if next > 8 {
			next = 8
		}
		branch.ConditionGroup.MinObservations = next
	}

	for _, child := range branch.Branches {
		adjustEntryConfirmations(child, delta)
	}
}

func filterBranches(branches []*logic.Branch, source logic.SourceType) []*logic.Branch {
	filtered := make([]*logic.Branch, 0, len(branches))

	for _, branch := range branches {
		if branch == nil {
			continue
		}

		branch.Branches = filterBranches(branch.Branches, source)
		if branchContainsEntry(branch) && branchDependsOnSource(branch, source) {
			continue
		}

		filtered = append(filtered, branch)
	}

	return filtered
}

func branchContainsEntry(branch *logic.Branch) bool {
	if branch == nil {
		return false
	}
	if branch.Action != nil && branch.Action.Side == logic.SideBuy && !branch.Action.Type.IsExit() {
		return true
	}
	for _, child := range branch.Branches {
		if branchContainsEntry(child) {
			return true
		}
	}

	return false
}

func walkEntryActions(branch *logic.Branch, visit func(*logic.Action)) {
	if branch == nil {
		return
	}
	if branch.Action != nil && branch.Action.Side == logic.SideBuy && !branch.Action.Type.IsExit() {
		visit(branch.Action)
	}
	for _, child := range branch.Branches {
		walkEntryActions(child, visit)
	}
}

func branchDependsOnSource(branch *logic.Branch, source logic.SourceType) bool {
	if branch == nil {
		return false
	}
	if conditionGroupDependsOnSource(branch.ConditionGroup, source) {
		return true
	}
	if branch.Action != nil && branch.Action.ReasonSource == source {
		return true
	}
	for _, child := range branch.Branches {
		if branchDependsOnSource(child, source) {
			return true
		}
	}

	return false
}

func conditionGroupDependsOnSource(group *logic.ConditionGroup, source logic.SourceType) bool {
	if group == nil {
		return false
	}
	for _, condition := range group.Conditions {
		if condition.Left.Source == source || condition.Right.Source == source {
			return true
		}
	}
	for index := range group.Groups {
		if conditionGroupDependsOnSource(&group.Groups[index], source) {
			return true
		}
	}

	return false
}
