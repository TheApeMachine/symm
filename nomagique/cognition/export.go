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
	TokenPrefix     string          `json:"prefix"`
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
	TokenPrefix string  `json:"prefix"`
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
	state := op.state.Load()

	if state == nil || state.root == nil || state.root.Len() == 0 {
		return CognitionTreeExport{
			Root: &TrieNodeJSON{
				ID:          "root",
				TokenPrefix: "ROOT",
				Probability: 1.0,
				State:       "ESTIMATED",
			},
			Branches: []TrieBranchJSON{},
			Feasible: []FeasibleActionJSON{},
		}
	}

	rootTree := state.root
	currentStep := state.step
	iterator := rootTree.Root().Iterator()

	type rawCandidate struct {
		keyBytes    []byte
		className   string
		tokens      []uint64
		probability float64
		count       uint64
	}

	var candidates []rawCandidate

	for keyBytes, valBytes, found := iterator.Next(); found; keyBytes, valBytes, found = iterator.Next() {
		if len(valBytes) != WeightSize {
			continue
		}

		classBytes, contextBytes, validBasin := parseBasinKey(keyBytes)

		if !validBasin {
			continue
		}

		weight := decodeWeight(valBytes).effective(currentStep, op.decayFactor)

		if weight.Count == 0 {
			continue
		}

		tokens := extractTokens(contextBytes)
		candidates = append(candidates, rawCandidate{
			keyBytes:    keyBytes,
			className:   string(classBytes),
			tokens:      tokens,
			probability: weight.Probability,
			count:       weight.Count,
		})
	}

	slices.SortFunc(candidates, func(left, right rawCandidate) int {
		return cmp.Compare(right.count, left.count)
	})

	topLimit := min(len(candidates), 64)
	topCandidates := candidates[:topLimit]

	rootNode := &TrieNodeJSON{
		ID:          "root",
		TokenPrefix: "ROOT",
		Probability: 1.0,
		State:       "EVALUATED",
	}

	nodeIndex := make(map[string]*TrieNodeJSON, topLimit*4)
	nodeIndex["root"] = rootNode

	type scoredBranch struct {
		branch   TrieBranchJSON
		feasible FeasibleActionJSON
	}

	var collectedBranches []scoredBranch

	for _, cand := range topCandidates {
		tokens := cand.tokens
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
					TokenPrefix:     stepPrefix,
					Probability:     cand.probability,
					StepProbability: cand.probability,
					Count:           cand.count,
					Tokens:          tokenNames[:level+1],
					State:           "EVALUATED",
				}
				nodeIndex[stepID] = childNode
				currentNode.Children = append(currentNode.Children, childNode)
			}

			if exists {
				childNode.Count += cand.count

				if cand.probability > childNode.Probability {
					childNode.Probability = cand.probability
				}
			}

			currentPath = stepID
			currentNode = childNode
		}

		leafID := fmt.Sprintf("%s:%s", currentPath, cand.className)
		policyState := "EVALUATED"

		if cand.probability > 0.5 {
			policyState = "POLICY CHOICE"
		}

		leafNode := &TrieNodeJSON{
			ID:              leafID,
			TokenPrefix:     cand.className,
			Probability:     cand.probability,
			StepProbability: cand.probability,
			Count:           cand.count,
			Tokens:          append(tokenNames, cand.className),
			State:           policyState,
		}
		currentNode.Children = append(currentNode.Children, leafNode)

		hash := fmt.Sprintf("0x%x:%s", cand.keyBytes, cand.className)

		policyStr := "WAIT"

		if cand.className == "enter" {
			policyStr = "ENTER"
		}

		if cand.className == "exit" {
			policyStr = "EXIT"
		}

		associationBias := cand.probability - 0.5

		collectedBranches = append(collectedBranches, scoredBranch{
			branch: TrieBranchJSON{
				Hash:       hash,
				Depth:      len(tokens) + 1,
				Visits:     cand.count,
				MeanEdge:   associationBias,
				Confidence: cand.probability * 100.0,
				Policy:     policyStr,
			},
			feasible: FeasibleActionJSON{
				Action:      cand.className,
				TokenPrefix: fmt.Sprintf("ROOT / %s / %s", strings.Join(tokenNames, " / "), cand.className),
				Probability: cand.probability,
				State:       policyState,
			},
		})
	}

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
