package optimizer

import (
	"math"
)

func (search *TreeSearch) selectNode() *Node {
	node := search.root

	for len(node.children) > 0 && len(node.untried) == 0 {
		node = search.bestChild(node)
	}

	return node
}

func (search *TreeSearch) bestChild(node *Node) *Node {
	best := node.children[0]
	bestScore := search.uct(node, best)

	for _, child := range node.children[1:] {
		score := search.uct(node, child)

		if score > bestScore {
			best = child
			bestScore = score
		}
	}

	return best
}

func (search *TreeSearch) uct(parent, child *Node) float64 {
	if child.visits == 0 {
		return math.Inf(1)
	}

	exploit := child.value / float64(child.visits)
	explore := search.explorationWeight * math.Sqrt(
		math.Log(float64(parent.visits))/float64(child.visits),
	)

	return exploit + explore
}

func (search *TreeSearch) expand(node *Node) *Node {
	if len(node.untried) == 0 {
		return node
	}

	moveIndex := search.sampleMoveIndex(node.untried, node.branches)
	move := node.untried[moveIndex]
	node.untried[moveIndex] = node.untried[len(node.untried)-1]
	node.untried = node.untried[:len(node.untried)-1]

	childBranches := search.applyMove(node.branches, move)

	child := &Node{
		branches: childBranches,
		parent:   node,
		untried:  search.moves(childBranches),
	}

	node.children = append(node.children, child)

	return child
}

func (search *TreeSearch) backpropagate(node *Node, reward float64) {
	for current := node; current != nil; current = current.parent {
		current.visits++
		current.value += reward
	}
}
