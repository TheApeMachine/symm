package ui

/*
TrieNodeJSON is one node in the region trie.
*/
type TrieNodeJSON struct {
	ID          string          `json:"id"`
	TokenPrefix string          `json:"prefix"`
	Token       string          `json:"token,omitempty"`
	Action      string          `json:"action,omitempty"`
	Count       uint64          `json:"count,omitempty"`
	Children    []*TrieNodeJSON `json:"children,omitempty"`
}

/*
TrieBranchJSON is the wire structure for a cognitive radix branch.
*/
type TrieBranchJSON struct {
	Hash       string  `json:"hash"`
	Depth      int     `json:"depth"`
	Visits     int     `json:"visits"`
	MeanEdge   float64 `json:"meanEdge"`
	Confidence float64 `json:"confidence"`
	Policy     string  `json:"policy"`
}

/*
CognitionTreeExport is the wire structure consumed by the learning dashboard.
*/
type CognitionTreeExport struct {
	Keys     []string         `json:"keys,omitempty"`
	Root     *TrieNodeJSON    `json:"root,omitempty"`
	Branches []TrieBranchJSON `json:"branches"`
}
