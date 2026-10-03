package cognition

import (
	"bytes"
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

	byClass := make(map[string][]rawCandidate)
	classes := make([]string, 0)

	for _, cand := range candidates {
		cls := strings.ToUpper(cand.className)

		if len(byClass[cls]) == 0 {
			classes = append(classes, cls)
		}

		byClass[cls] = append(byClass[cls], cand)
	}

	for _, cls := range classes {
		slices.SortFunc(byClass[cls], func(left, right rawCandidate) int {
			return cmp.Compare(right.count, left.count)
		})
	}

	topLimit := min(len(candidates), 64)
	topCandidates := make([]rawCandidate, 0, topLimit)
	candIdx := 0

	for len(topCandidates) < topLimit {
		addedAny := false

		for _, cls := range classes {
			list := byClass[cls]

			if candIdx < len(list) {
				topCandidates = append(topCandidates, list[candIdx])
				addedAny = true

				if len(topCandidates) >= topLimit {
					break
				}
			}
		}

		if !addedAny {
			break
		}

		candIdx++
	}

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

		// Path nodes are region frames only. ENTER/EXIT is a dedicated leaf —
		// never painted onto intermediate region nodes (shared prefixes would
		// otherwise show green ENTER mid-chain).
		currNode := rootNode
		var pathSoFar strings.Builder
		pathSoFar.WriteString("root")

		for _, regToken := range regionTokens {
			pathSoFar.WriteString("/")
			pathSoFar.WriteString(regToken)

			var foundChild *TrieNodeJSON

			for _, child := range currNode.Children {
				if !isActionLeaf(child) && len(child.Tokens) > 0 && child.Tokens[0] == regToken {
					foundChild = child
					break
				}
			}

			if foundChild == nil {
				foundChild = &TrieNodeJSON{
					ID:              pathSoFar.String(),
					TokenPrefix:     regToken,
					Probability:     cand.probability,
					StepProbability: cand.probability,
					Count:           0,
					Tokens:          []string{regToken},
					State:           "EVALUATED",
				}
				currNode.Children = append(currNode.Children, foundChild)
			}

			foundChild.Count += cand.count

			if cand.probability > foundChild.Probability {
				foundChild.Probability = cand.probability
				foundChild.StepProbability = cand.probability
			}

			currNode = foundChild
		}

		actionID := pathSoFar.String() + "/" + actionName
		var actionLeaf *TrieNodeJSON

		for _, child := range currNode.Children {
			if child.TokenPrefix == actionName && isActionLeaf(child) {
				actionLeaf = child
				break
			}
		}

		if actionLeaf == nil {
			actionLeaf = &TrieNodeJSON{
				ID:          actionID,
				TokenPrefix: actionName,
				Count:       0,
				Tokens:      []string{cand.className},
			}
			currNode.Children = append(currNode.Children, actionLeaf)
		}

		actionLeaf.Count += cand.count

		if cand.probability >= actionLeaf.Probability {
			actionLeaf.Probability = cand.probability
			actionLeaf.StepProbability = cand.probability
			actionLeaf.State = policyState
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
and formats each as [id,id,...] for the trie viz. Consecutive identical frames
collapse (change-point only). ENTER/EXIT stay on the leaf action node — they
are never emitted as region tokens.
*/
const litRegionsFrameCap = 3 // matches store.litRegionTokenSize (TRAINING.md N)

func regionFrames(context []byte) []string {
	if len(context) == 0 {
		return nil
	}

	if bytes.IndexByte(context, 'R') >= 0 || bytes.IndexByte(context, 'r') >= 0 {
		str := string(context)
		delims := func(r rune) bool {
			return r == '_' || r == '/' || r == 0 || r == ','
		}
		rawParts := strings.FieldsFunc(str, delims)
		var frames []string
		for _, part := range rawParts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			idStr := strings.TrimPrefix(strings.TrimPrefix(part, "R"), "r")
			num, err := strconv.Atoi(idStr)
			var frame string
			if err == nil && num > 0 {
				frame = fmt.Sprintf("[%d]", num)
			} else {
				frame = fmt.Sprintf("[%s]", part)
			}
			if len(frames) == 0 || frames[len(frames)-1] != frame {
				frames = append(frames, frame)
			}
		}
		return frames
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

			// Cap to designed LitRegions N. Longer frames are legacy/corrupt
			// signatures (pre-null-separator or pre-top-N) — keep first N IDs.
			if len(parts) > litRegionsFrameCap {
				parts = parts[:litRegionsFrameCap]
			}

			if len(parts) > 0 {
				frame := "[" + strings.Join(parts, ",") + "]"
				// Collapse consecutive identical frames (change-point only).
				if len(frames) == 0 || frames[len(frames)-1] != frame {
					frames = append(frames, frame)
				}
			}
		}

		start = index + 1
	}

	return frames
}

func isActionLeaf(node *TrieNodeJSON) bool {
	if node == nil {
		return false
	}

	switch strings.ToUpper(node.TokenPrefix) {
	case "ENTER", "EXIT", "WAIT":
		return true
	default:
		return false
	}
}
