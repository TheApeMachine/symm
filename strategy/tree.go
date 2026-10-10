package strategy

import (
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/theapemachine/symm/ui"
)

/*
treeCache builds the learning dashboard's view of the stored key space once:
Step matches against the keys as they stood at startup, so the tree cannot
change during a run.
*/
type treeCache struct {
	once   sync.Once
	export ui.CognitionTreeExport
}

/*
CognitionTree exports the stored token paths as the radix tree Step walks:
each node is a run of tokens shared by every path below it, its count the
number of stored paths through it, and its probability that count over its
parent's. A node whose paths all end in one action is where Step could act
(subject to the minimum path confidence), shown as POLICY CHOICE; a node
whose paths still disagree is ESTIMATED; a stored path's action is EVALUATED.
Feasible lists each action's share of all stored paths.
*/
func (training *Training) CognitionTree() ui.CognitionTreeExport {
	training.tree.once.Do(func() { training.tree.export = keyTree(training.keys) })

	return training.tree.export
}

type keyNode struct {
	tokens   []string
	count    uint64
	actions  map[string]uint64
	children map[string]*keyNode
}

func keyTree(keys []string) ui.CognitionTreeExport {
	export := ui.CognitionTreeExport{
		Branches: []ui.TrieBranchJSON{},
		Feasible: []ui.FeasibleActionJSON{},
	}

	if len(keys) == 0 {
		return export
	}

	root := &keyNode{actions: map[string]uint64{}, children: map[string]*keyNode{}}
	totals := map[string]uint64{}

	for _, key := range keys {
		segments := strings.Split(key, "/")
		action := strings.TrimSuffix(segments[len(segments)-1], ".json")
		path := append(segments[:len(segments)-1:len(segments)-1], action)
		totals[action]++

		node := root
		node.count++
		node.actions[action]++

		for _, segment := range path {
			child, ok := node.children[segment]

			if !ok {
				child = &keyNode{tokens: []string{segment}, actions: map[string]uint64{}, children: map[string]*keyNode{}}
				node.children[segment] = child
			}

			child.count++
			child.actions[action]++
			node = child
		}
	}

	next := 0
	export.Root = emitNode(compress(root), nil, root.count, &next)

	actions := make([]string, 0, len(totals))

	for action := range totals {
		actions = append(actions, action)
	}

	slices.SortFunc(actions, func(left, right string) int {
		return int(totals[right]) - int(totals[left])
	})

	for rank, action := range actions {
		export.Feasible = append(export.Feasible, ui.FeasibleActionJSON{
			Rank:        rank + 1,
			Action:      action,
			Probability: float64(totals[action]) / float64(root.count),
			State:       "EVALUATED",
		})
	}

	return export
}

/*
compress folds every single-child chain into one node, so the tree has at
most one internal node per branching point.
*/
func compress(node *keyNode) *keyNode {
	for len(node.children) == 1 && len(node.tokens) > 0 {
		var only *keyNode

		for _, child := range node.children {
			only = child
		}

		node.tokens = append(node.tokens, only.tokens...)
		node.children = only.children
	}

	for key, child := range node.children {
		node.children[key] = compress(child)
	}

	return node
}

func emitNode(node *keyNode, prefix []string, parentCount uint64, next *int) *ui.TrieNodeJSON {
	path := append(slices.Clone(prefix), node.tokens...)
	state := "ESTIMATED"

	switch {
	case len(node.children) == 0:
		state = "EVALUATED"
	case len(node.actions) == 1:
		state = "POLICY CHOICE"
	}

	json := &ui.TrieNodeJSON{
		ID:          strconv.Itoa(*next),
		TokenPrefix: strings.Join(path, "/"),
		Probability: float64(node.count) / float64(parentCount),
		Count:       node.count,
		Tokens:      slices.Clone(node.tokens),
		State:       state,
	}
	*next++

	keys := make([]string, 0, len(node.children))

	for key := range node.children {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	for _, key := range keys {
		json.Children = append(json.Children, emitNode(node.children[key], path, node.count, next))
	}

	return json
}
