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
TrieBranchJSON is kept for wire compatibility.
*/
type TrieBranchJSON struct{}

/*
CognitionTreeExport is the wire structure consumed by the learning dashboard.
*/
type CognitionTreeExport struct {
	Keys     []string         `json:"keys,omitempty"`
	Root     *TrieNodeJSON    `json:"root,omitempty"`
	Branches []TrieBranchJSON `json:"branches"`
}

func (export CognitionTreeExport) normalized() CognitionTreeExport {
	if export.Branches == nil {
		export.Branches = []TrieBranchJSON{}
	}

	return export
}
