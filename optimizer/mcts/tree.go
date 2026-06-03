package mcts

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
Node is one position in the branch-construction search tree.
*/
type Node struct {
	branches    perspectives.BranchList
	parent      *Node
	children    []*Node
	visits      int
	value       float64
	untried     []Move
	uctDiscount float64
}

/*
Tree runs UCT selection, expansion, and backpropagation over MCTS nodes.
*/
type Tree struct {
	Root              *Node
	explorationWeight float64
	moves             *Moves
	heuristic         *Heuristic
}

func NewTree(
	explorationWeight float64,
	moves *Moves,
	heuristic *Heuristic,
) *Tree {
	return &Tree{
		Root:              &Node{branches: perspectives.BranchList{}},
		explorationWeight: explorationWeight,
		moves:             moves,
		heuristic:         heuristic,
	}
}

func (tree *Tree) Select() *Node {
	node := tree.Root

	for len(node.children) > 0 && len(node.untried) == 0 {
		node = tree.bestChild(node)
	}

	return node
}

func (tree *Tree) Expand(node *Node) *Node {
	if len(node.untried) == 0 {
		return node
	}

	moveIndex := tree.heuristic.SampleMoveIndex(node.untried, node.branches)
	move := node.untried[moveIndex]
	node.untried[moveIndex] = node.untried[len(node.untried)-1]
	node.untried = node.untried[:len(node.untried)-1]

	childBranches := tree.moves.Apply(node.branches, move)

	child := &Node{
		branches:    childBranches,
		parent:      node,
		untried:     tree.moves.Available(childBranches),
		uctDiscount: move.uctDiscount,
	}

	node.children = append(node.children, child)

	return child
}

func (tree *Tree) Backpropagate(node *Node, reward float64) {
	for current := node; current != nil; current = current.parent {
		current.visits++
		current.value += reward
	}
}

func (tree *Tree) UCT(parent, child *Node) float64 {
	if child.visits == 0 {
		return math.Inf(1)
	}

	exploit := child.value / float64(child.visits)
	explore := tree.explorationWeight * math.Sqrt(
		math.Log(float64(parent.visits))/float64(child.visits),
	)
	discount := child.uctDiscount

	if discount <= 0 {
		discount = 1
	}

	return (exploit + explore) * discount
}

func (tree *Tree) bestChild(node *Node) *Node {
	best := node.children[0]
	bestScore := tree.UCT(node, best)

	for _, child := range node.children[1:] {
		score := tree.UCT(node, child)

		if score > bestScore {
			best = child
			bestScore = score
		}
	}

	return best
}

func (node *Node) ChildCount() int {
	return len(node.children)
}

func (node *Node) FirstChild() *Node {
	if len(node.children) == 0 {
		return nil
	}

	return node.children[0]
}

func (node *Node) Visits() int {
	return node.visits
}

func (tree *Tree) ensureUntried() {
	if len(tree.Root.children) == 0 && len(tree.Root.untried) == 0 {
		tree.Root.untried = tree.moves.Available(tree.Root.branches)
	}
}
