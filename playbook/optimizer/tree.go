package optimizer

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// TreeRewrite summarizes how an optimizer report was applied to tree.yml.
type TreeRewrite struct {
	SequencesVisited int `json:"sequences_visited"`
	BranchesMatched  int `json:"branches_matched"`
	SequencesChanged int `json:"sequences_changed"`
}

type rankedRecommendation struct {
	key  actionKey
	rank int
}

type branchRank struct {
	rank    int
	matched bool
	class   string
}

type conditionFact struct {
	source   string
	category string
}

// RewriteTreeYAML applies optimizer rankings to playbook branch order.
func RewriteTreeYAML(input []byte, report Report) ([]byte, TreeRewrite, error) {
	ranks := rankedRecommendations(report)
	if len(ranks) == 0 {
		return nil, TreeRewrite{}, errors.New("playbook optimizer: report has no recommendations")
	}

	var document yaml.Node
	if err := yaml.Unmarshal(input, &document); err != nil {
		return nil, TreeRewrite{}, fmt.Errorf("playbook optimizer: decode tree yaml: %w", err)
	}
	if len(document.Content) == 0 {
		return nil, TreeRewrite{}, errors.New("playbook optimizer: empty tree yaml")
	}

	root := document.Content[0]
	branches := mappingValue(root, "branches")
	if branches == nil || branches.Kind != yaml.SequenceNode {
		return nil, TreeRewrite{}, errors.New("playbook optimizer: tree yaml missing branches sequence")
	}

	rewrite := TreeRewrite{}
	reorderSequence(branches, ranks, &rewrite)

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		_ = encoder.Close()
		return nil, TreeRewrite{}, fmt.Errorf("playbook optimizer: encode tree yaml: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, TreeRewrite{}, fmt.Errorf("playbook optimizer: close tree yaml encoder: %w", err)
	}

	return out.Bytes(), rewrite, nil
}

func rankedRecommendations(report Report) []rankedRecommendation {
	recs := make([]rankedRecommendation, 0, len(report.Recommendations))
	for idx, rec := range report.Recommendations {
		key := actionKey{
			Type:     normalizeKey(rec.Type),
			Source:   normalizeKey(rec.Source),
			Category: normalizeKey(rec.Category),
		}
		if key.Type == "" && key.Source == "" && key.Category == "" {
			continue
		}
		recs = append(recs, rankedRecommendation{key: key, rank: idx})
	}
	return recs
}

func reorderSequence(
	sequence *yaml.Node,
	recs []rankedRecommendation,
	rewrite *TreeRewrite,
) branchRank {
	if sequence == nil || sequence.Kind != yaml.SequenceNode {
		return branchRank{rank: math.MaxInt, class: "unknown"}
	}

	rewrite.SequencesVisited++
	ranked := make([]struct {
		node  *yaml.Node
		rank  branchRank
		index int
	}, 0, len(sequence.Content))

	for idx, branch := range sequence.Content {
		rank := rankBranch(branch, recs, rewrite)
		ranked = append(ranked, struct {
			node  *yaml.Node
			rank  branchRank
			index int
		}{node: branch, rank: rank, index: idx})
	}

	changed := false
	for start := 0; start < len(ranked); {
		end := start + 1
		for end < len(ranked) && ranked[end].rank.class == ranked[start].rank.class {
			end++
		}

		before := append([]*yaml.Node(nil), sequence.Content[start:end]...)
		sort.SliceStable(ranked[start:end], func(i, j int) bool {
			left := ranked[start+i]
			right := ranked[start+j]
			if left.rank.matched != right.rank.matched {
				return left.rank.matched
			}
			if left.rank.rank != right.rank.rank {
				return left.rank.rank < right.rank.rank
			}
			return left.index < right.index
		})
		for offset := start; offset < end; offset++ {
			sequence.Content[offset] = ranked[offset].node
		}
		for offset := range before {
			if before[offset] != sequence.Content[start+offset] {
				changed = true
				break
			}
		}
		start = end
	}

	if changed {
		rewrite.SequencesChanged++
	}

	best := branchRank{rank: math.MaxInt, class: "unknown"}
	for _, item := range ranked {
		best = mergeRank(best, item.rank)
	}
	return best
}

func rankBranch(
	branch *yaml.Node,
	recs []rankedRecommendation,
	rewrite *TreeRewrite,
) branchRank {
	action := mappingValue(branch, "action")
	group := mappingValue(branch, "condition_group")
	facts := conditionFacts(group)
	out := branchRank{rank: math.MaxInt, class: branchClass(action)}

	if action != nil {
		actionType := normalizeKey(mappingScalar(action, "type"))
		for _, rec := range recs {
			if !recommendationMatches(actionType, facts, rec.key) {
				continue
			}
			if rec.rank < out.rank {
				out.rank = rec.rank
				out.matched = true
			}
		}
		if out.matched {
			rewrite.BranchesMatched++
		}
	}

	children := mappingValue(branch, "branches")
	if children != nil && children.Kind == yaml.SequenceNode {
		childRank := reorderSequence(children, recs, rewrite)
		out = mergeRank(out, childRank)
		if out.class == "unknown" {
			out.class = childRank.class
		}
	}

	return out
}

func mergeRank(left, right branchRank) branchRank {
	if left.class == "unknown" && right.class != "" {
		left.class = right.class
	}
	if !left.matched || (right.matched && right.rank < left.rank) {
		if right.class == "" {
			right.class = left.class
		}
		return right
	}
	return left
}

func recommendationMatches(
	actionType string,
	facts []conditionFact,
	key actionKey,
) bool {
	if key.Type != "" && key.Type != actionType {
		return false
	}
	if key.Source == "" && key.Category == "" {
		return true
	}
	for _, fact := range facts {
		if key.Source != "" && key.Source != fact.source {
			continue
		}
		if key.Category != "" && key.Category != fact.category {
			continue
		}
		return true
	}
	return false
}

func conditionFacts(node *yaml.Node) []conditionFact {
	var facts []conditionFact
	walkYAML(node, func(current *yaml.Node) {
		if current == nil || current.Kind != yaml.MappingNode {
			return
		}
		left := mappingValue(current, "left")
		if left == nil {
			return
		}
		source := normalizeKey(mappingScalar(left, "source"))
		categoryNode := mappingValue(left, "category")
		category := normalizeKey(mappingScalar(categoryNode, "type"))
		if source == "" && category == "" {
			return
		}
		facts = append(facts, conditionFact{source: source, category: category})
	})
	return facts
}

func walkYAML(node *yaml.Node, visit func(*yaml.Node)) {
	if node == nil {
		return
	}
	visit(node)
	for _, child := range node.Content {
		walkYAML(child, visit)
	}
}

func branchClass(action *yaml.Node) string {
	if action == nil {
		return "unknown"
	}

	side := normalizeKey(mappingScalar(action, "side"))
	actionType := normalizeKey(mappingScalar(action, "type"))
	if side == "buy" {
		return "entry"
	}
	if side == "sell" || isExitActionType(actionType) {
		return "exit"
	}
	if actionType == "limit" || actionType == "market" || actionType == "iceberg" {
		return "entry"
	}
	return "unknown"
}

func isExitActionType(actionType string) bool {
	switch actionType {
	case "settle_position", "take_profit", "take_profit_limit", "stop_loss", "stop_loss_limit", "trailing_stop", "trailing_stop_limit":
		return true
	default:
		return false
	}
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for idx := 0; idx+1 < len(node.Content); idx += 2 {
		if node.Content[idx].Value == key {
			return node.Content[idx+1]
		}
	}
	return nil
}

func mappingScalar(node *yaml.Node, key string) string {
	value := mappingValue(node, key)
	if value == nil {
		return ""
	}
	return strings.TrimSpace(value.Value)
}
