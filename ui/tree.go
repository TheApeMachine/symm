package ui

/*
TrieNodeJSON is one node in the learning dashboard radix tree.
*/
type TrieNodeJSON struct {
	ID              string          `json:"id"`
	TokenPrefix     string          `json:"prefix"`
	Probability     float64         `json:"probability"`
	StepProbability float64         `json:"stepProbability,omitempty"`
	Count           uint64          `json:"count"`
	Tokens          []string        `json:"tokens,omitempty"`
	State           string          `json:"state,omitempty"`
	Children        []*TrieNodeJSON `json:"children,omitempty"`
}

/*
TrieBranchJSON is one active branch in the association trie.
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
FeasibleActionJSON is one candidate action at the evaluated context.
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
normalized returns the export with empty lists instead of nil, so the wire
carries [] rather than null for a tree that has not grown yet.
*/
func (export CognitionTreeExport) normalized() CognitionTreeExport {
	if export.Branches == nil {
		export.Branches = []TrieBranchJSON{}
	}

	if export.Feasible == nil {
		export.Feasible = []FeasibleActionJSON{}
	}

	return export
}
