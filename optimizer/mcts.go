package optimizer

import (
	"math"
	"math/rand"

	"github.com/theapemachine/symm/market/perspectives"
)

const (
	defaultMCTSIterations = 256
	explorationWeight     = 1.41
)

/*
Node is one position in the branch-construction search tree.
*/
type Node struct {
	branches perspectives.BranchList
	parent   *Node
	children []*Node
	visits   int
	value    float64
	untried  []Move
}

/*
TreeSearch runs MCTS over perspective branch registries.
*/
type TreeSearch struct {
	profile    *Profile
	evaluate   func(perspectives.BranchList) float64
	rng        *rand.Rand
	root       *Node
	best       *Node
	bestScore  float64
	iterations int
}

func (tuner *Tuner) newTreeSearch() *TreeSearch {
	search := &TreeSearch{
		profile:    &tuner.profile,
		evaluate:   tuner.evaluator(),
		rng:        rand.New(rand.NewSource(tuner.seed)),
		iterations: defaultMCTSIterations,
	}

	search.root = &Node{
		branches: perspectives.BranchList{},
	}
	search.root.untried = search.moves(search.root.branches)

	return search
}

/*
Run finds the most profitable branch registry for the replay profile.
*/
func (search *TreeSearch) Run() perspectives.BranchList {
	for iteration := 0; iteration < search.iterations; iteration++ {
		node := search.selectNode()
		child := search.expand(node)
		reward := search.rollout(child)
		search.backpropagate(child, reward)
	}

	if search.best == nil {
		return search.root.branches.Clone()
	}

	return search.best.branches.Clone()
}

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
	explore := explorationWeight * math.Sqrt(
		math.Log(float64(parent.visits))/float64(child.visits),
	)

	return exploit + explore
}

func (search *TreeSearch) expand(node *Node) *Node {
	if len(node.untried) == 0 {
		return node
	}

	last := len(node.untried) - 1
	move := node.untried[last]
	node.untried = node.untried[:last]

	childBranches := search.applyMove(node.branches, move)

	child := &Node{
		branches: childBranches,
		parent:   node,
		untried:  search.moves(childBranches),
	}

	node.children = append(node.children, child)

	return child
}

func (search *TreeSearch) rollout(node *Node) float64 {
	branches := node.branches.Clone()
	targetDepth := len(search.profile.Categories()) * maxBranchDepth

	for len(branches) < targetDepth {
		moves := search.moves(branches)

		if len(moves) == 0 {
			break
		}

		move := moves[search.rng.Intn(len(moves))]
		branches = search.applyMove(branches, move)
	}

	score := search.evaluate(branches)

	if len(branches) > 0 && (search.best == nil || score > search.bestScore) {
		search.bestScore = score
		search.best = &Node{branches: branches.Clone(), value: score}
	}

	return score
}

func (search *TreeSearch) backpropagate(node *Node, reward float64) {
	for current := node; current != nil; current = current.parent {
		current.visits++
		current.value += reward
	}
}
