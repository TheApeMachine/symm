package cognition

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
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
		context     []byte
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

		candidates = append(candidates, rawCandidate{
			keyBytes:    keyBytes,
			className:   string(classBytes),
			context:     append([]byte{}, contextBytes...),
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

	type scoredBranch struct {
		branch   TrieBranchJSON
		feasible FeasibleActionJSON
	}

	var collectedBranches []scoredBranch

	for _, cand := range topCandidates {
		// Training signatures are null-separated LitRegions frames (raw region
		// ID bytes). Reading them as packed uint64 tokens invented private spines.
		regionTokens := regionFrames(cand.context)

		actionName := strings.ToUpper(cand.className)
		policyState := "EVALUATED"

		if cand.probability > 0.5 {
			policyState = "POLICY CHOICE"
		}

		// Edges encode region tokens; nodes encode actions (WAIT, ENTER, EXIT).
		currNode := rootNode
		var pathSoFar strings.Builder
		pathSoFar.WriteString("root")

		for idx, regToken := range regionTokens {
			isLast := idx == len(regionTokens)-1
			nodeAction := "WAIT"
			nodeState := "EVALUATED"
			nodeProb := cand.probability
			nodeCount := cand.count

			if isLast {
				nodeAction = actionName
				nodeState = policyState
			}

			pathSoFar.WriteString("/")
			pathSoFar.WriteString(regToken)

			var foundChild *TrieNodeJSON

			for _, child := range currNode.Children {
				if len(child.Tokens) > 0 && child.Tokens[0] == regToken {
					foundChild = child
					break
				}
			}

			if foundChild == nil {
				foundChild = &TrieNodeJSON{
					ID:              pathSoFar.String(),
					TokenPrefix:     nodeAction,
					Probability:     nodeProb,
					StepProbability: nodeProb,
					Count:           nodeCount,
					Tokens:          []string{regToken},
					State:           nodeState,
				}
				currNode.Children = append(currNode.Children, foundChild)
			}

			if isLast {
				foundChild.Count += cand.count

				if cand.probability >= foundChild.Probability {
					foundChild.TokenPrefix = actionName
					foundChild.Probability = cand.probability
					foundChild.StepProbability = cand.probability
					foundChild.State = policyState
				}
			}

			currNode = foundChild
		}

		hash := fmt.Sprintf("0x%x:%s", cand.keyBytes, cand.className)
		associationBias := cand.probability - 0.5

		collectedBranches = append(collectedBranches, scoredBranch{
			branch: TrieBranchJSON{
				Hash:       hash,
				Depth:      len(regionTokens) + 1,
				Visits:     cand.count,
				MeanEdge:   associationBias,
				Confidence: cand.probability * 100.0,
				Policy:     actionName,
			},
			feasible: FeasibleActionJSON{
				Action:      cand.className,
				TokenPrefix: fmt.Sprintf("ROOT / [%s] -> %s", strings.Join(regionTokens, ", "), cand.className),
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

/*
regionFrames splits a training signature into null-separated LitRegions frames
and formats each as [id,id,...] for the trie viz. ENTER/EXIT stay on the leaf
action node — they are never emitted as region tokens.
*/
func regionFrames(context []byte) []string {
	if len(context) == 0 {
		return nil
	}

	var frames []string
	start := 0

	for index := 0; index <= len(context); index++ {
		if index < len(context) && context[index] != 0 {
			continue
		}

		if index > start {
			parts := make([]string, 0, index-start)

			for _, region := range context[start:index] {
				if region == 0 {
					continue
				}

				parts = append(parts, strconv.Itoa(int(region)))
			}

			if len(parts) > 0 {
				frames = append(frames, "["+strings.Join(parts, ",")+"]")
			}
		}

		start = index + 1
	}

	return frames
}
