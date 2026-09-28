package cognition

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"slices"
	"strings"
)

/*
TrieNodeJSON represents one node in the interactive cognitive radix tree.
*/
type TrieNodeJSON struct {
	ID              string          `json:"id"`
	Prefix          string          `json:"prefix"`
	Probability     float64         `json:"probability"`
	StepProbability float64         `json:"stepProbability,omitempty"`
	Count           uint64          `json:"count"`
	Tokens          []string        `json:"tokens,omitempty"`
	State           string          `json:"state,omitempty"` // EVALUATED | POLICY CHOICE | ESTIMATED
	Children        []*TrieNodeJSON `json:"children,omitempty"`
}

/*
TrieBranchJSON represents one active branch in the radix trie memory.
*/
type TrieBranchJSON struct {
	Hash       string  `json:"hash"`
	Depth      int     `json:"depth"`
	Visits     uint64  `json:"visits"`
	MeanEdge   float64 `json:"meanEdge"`
	Confidence float64 `json:"confidence"`
	Policy     string  `json:"policy"`
}

/*
FeasibleActionJSON represents one candidate action at the evaluated impulse.
*/
type FeasibleActionJSON struct {
	Rank        int     `json:"rank"`
	Action      string  `json:"action"`
	Prefix      string  `json:"prefix"`
	Probability float64 `json:"probability"`
	State       string  `json:"state"`
}

/*
CognitionTreeExport is the wire structure consumed by the learning dashboard.
*/
type CognitionTreeExport struct {
	Root     *TrieNodeJSON        `json:"root"`
	Branches []TrieBranchJSON     `json:"branches"`
	Feasible []FeasibleActionJSON `json:"feasible"`
}

/*
TreeExport traverses the immutable radix trie and produces a hierarchical tree
and active branch roster for UI visualization without synthesizing fake data.
*/
func (op *Engine) TreeExport() CognitionTreeExport {
	rootTree := op.root.Load()

	if rootTree == nil || rootTree.Len() == 0 {
		return CognitionTreeExport{
			Root: &TrieNodeJSON{
				ID:          "root",
				Prefix:      "ROOT",
				Probability: 1.0,
				State:       "ESTIMATED",
			},
			Branches: []TrieBranchJSON{},
			Feasible: []FeasibleActionJSON{},
		}
	}

	currentStep := op.stepCounter.Load()
	iterator := rootTree.Root().Iterator()

	rootNode := &TrieNodeJSON{
		ID:          "root",
		Prefix:      "ROOT",
		Probability: 1.0,
		State:       "EVALUATED",
	}

	nodeIndex := make(map[string]*TrieNodeJSON)
	nodeIndex["root"] = rootNode

	type scoredBranch struct {
		branch   TrieBranchJSON
		feasible FeasibleActionJSON
	}

	var collectedBranches []scoredBranch

	for keyBytes, valBytes, found := iterator.Next(); found; keyBytes, valBytes, found = iterator.Next() {
		if len(valBytes) != WeightSize {
			continue
		}

		classBytes, contextBytes, validBasin := parseBasinKey(keyBytes)

		if !validBasin {
			continue
		}

		className := string(classBytes)
		weight := decodeWeight(valBytes).effective(currentStep, op.decayFactor)

		tokens := extractTokens(contextBytes)
		tokenNames := make([]string, len(tokens))

		for idx, tokenVal := range tokens {
			tokenNames[idx] = fmt.Sprintf("0x%04x", tokenVal&0xffff)
		}

		currentPath := "root"
		currentNode := rootNode

		for level, tokenVal := range tokens {
			stepID := fmt.Sprintf("%s:%04x", currentPath, tokenVal&0xffff)
			childNode, exists := nodeIndex[stepID]

			if !exists {
				stepPrefix := fmt.Sprintf("t-%04x", tokenVal&0xffff)

				if level == 0 {
					stepPrefix = fmt.Sprintf("scope-%04x", tokenVal&0xffff)
				}

				childNode = &TrieNodeJSON{
					ID:              stepID,
					Prefix:          stepPrefix,
					Probability:     weight.Probability,
					StepProbability: weight.Probability,
					Count:           weight.Count,
					Tokens:          tokenNames[:level+1],
					State:           "EVALUATED",
				}
				nodeIndex[stepID] = childNode
				currentNode.Children = append(currentNode.Children, childNode)
			}

			if exists {
				childNode.Count += weight.Count

				if weight.Probability > childNode.Probability {
					childNode.Probability = weight.Probability
				}
			}

			currentPath = stepID
			currentNode = childNode
		}

		leafID := fmt.Sprintf("%s:%s", currentPath, className)
		policyState := "EVALUATED"

		if weight.Probability > 0.5 {
			policyState = "POLICY CHOICE"
		}

		leafNode := &TrieNodeJSON{
			ID:              leafID,
			Prefix:          className,
			Probability:     weight.Probability,
			StepProbability: weight.Probability,
			Count:           weight.Count,
			Tokens:          append(tokenNames, className),
			State:           policyState,
		}
		currentNode.Children = append(currentNode.Children, leafNode)

		hash := "0x0000"

		if len(tokens) > 0 {
			hash = fmt.Sprintf("0x%06x", tokens[len(tokens)-1]&0xffffff)
		}

		policyStr := "WAIT"

		if className == "enter" {
			policyStr = "ENTER"
		}

		if className == "exit" {
			policyStr = "EXIT"
		}

		meanEdge := (weight.Probability - 0.5) * 20.0

		collectedBranches = append(collectedBranches, scoredBranch{
			branch: TrieBranchJSON{
				Hash:       hash,
				Depth:      len(tokens) + 1,
				Visits:     weight.Count,
				MeanEdge:   meanEdge,
				Confidence: weight.Probability * 100.0,
				Policy:     policyStr,
			},
			feasible: FeasibleActionJSON{
				Action:      className,
				Prefix:      fmt.Sprintf("ROOT / %s / %s", strings.Join(tokenNames, " / "), className),
				Probability: weight.Probability,
				State:       policyState,
			},
		})
	}

	slices.SortFunc(collectedBranches, func(left, right scoredBranch) int {
		return cmp.Compare(right.branch.Visits, left.branch.Visits)
	})

	maxBranches := min(len(collectedBranches), 16)
	branches := make([]TrieBranchJSON, maxBranches)
	feasible := make([]FeasibleActionJSON, maxBranches)

	for idx := 0; idx < maxBranches; idx++ {
		branches[idx] = collectedBranches[idx].branch
		feasible[idx] = collectedBranches[idx].feasible
		feasible[idx].Rank = idx + 1
	}

	return CognitionTreeExport{
		Root:     rootNode,
		Branches: branches,
		Feasible: feasible,
	}
}

func extractTokens(contextBytes []byte) []uint64 {
	tokenCount := len(contextBytes) / 8

	if tokenCount == 0 {
		return nil
	}

	tokens := make([]uint64, tokenCount)

	for idx := 0; idx < tokenCount; idx++ {
		tokens[idx] = binary.BigEndian.Uint64(contextBytes[idx*8 : (idx+1)*8])
	}

	return tokens
}
