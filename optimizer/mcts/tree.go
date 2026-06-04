package mcts

import (
	"encoding/binary"
	"hash/fnv"
	"io"
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
	bestKey := nodeFingerprint(best)

	for _, child := range node.children[1:] {
		score := tree.UCT(node, child)

		switch {
		case score > bestScore:
			best, bestScore, bestKey = child, score, nodeFingerprint(child)
		case score == bestScore:
			// UCT ties — notably all unvisited children at +Inf — would otherwise
			// fall to append order, which mirrors searchEntryActions and pins
			// selection to the first-listed action (ActionLimit). Break the tie on
			// a stable branch fingerprint instead, so no action is systematically
			// favoured.
			if key := nodeFingerprint(child); key < bestKey {
				best, bestKey = child, key
			}
		}
	}

	return best
}

/*
nodeFingerprint is a stable, order-independent hash of a node's branches used to
break UCT ties deterministically without favouring move-enumeration order.
*/
func nodeFingerprint(node *Node) uint64 {
	hash := fnv.New64a()
	writeBranchListFingerprint(hash, node.branches)

	return hash.Sum64()
}

func writeBranchListFingerprint(writer io.Writer, branches perspectives.BranchList) {
	var scratch [8]byte

	for _, branch := range branches {
		_, _ = writer.Write([]byte(branch.Category))
		_, _ = writer.Write([]byte{
			byte(branch.Observation),
			byte(branch.Regime),
			byte(branch.Condition),
			byte(branch.Unit),
			byte(branch.Action.Type),
		})
		binary.LittleEndian.PutUint64(scratch[:], math.Float64bits(branch.Value))
		_, _ = writer.Write(scratch[:])
		writeBranchListFingerprint(writer, branch.Branches)
	}
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
