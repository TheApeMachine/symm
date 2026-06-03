package mcts

import (
	"math"
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestTreeUCT(t *testing.T) {
	convey.Convey("Given a parent with explored children", t, func() {
		tree := &Tree{explorationWeight: 1.4}
		parent := &Node{visits: 10}
		strongChild := &Node{visits: 5, value: 3}
		weakChild := &Node{visits: 5, value: 1}

		strongScore := tree.UCT(parent, strongChild)
		weakScore := tree.UCT(parent, weakChild)

		convey.Convey("It should prefer higher average reward", func() {
			convey.So(strongScore, convey.ShouldBeGreaterThan, weakScore)
		})
	})

	convey.Convey("Given an unvisited child", t, func() {
		tree := &Tree{explorationWeight: 1.4}
		parent := &Node{visits: 3}
		child := &Node{visits: 0}

		convey.Convey("It should treat unvisited nodes as infinitely attractive", func() {
			convey.So(tree.UCT(parent, child), convey.ShouldEqual, math.Inf(1))
		})
	})
}

func TestTreeBackpropagate(t *testing.T) {
	convey.Convey("Given a leaf node chain", t, func() {
		root := &Node{}
		child := &Node{parent: root}
		leaf := &Node{parent: child}
		tree := &Tree{}

		tree.Backpropagate(leaf, 0.75)

		convey.Convey("It should accumulate reward up the tree", func() {
			convey.So(leaf.visits, convey.ShouldEqual, 1)
			convey.So(leaf.value, convey.ShouldAlmostEqual, 0.75, 0.0001)
			convey.So(child.visits, convey.ShouldEqual, 1)
			convey.So(root.visits, convey.ShouldEqual, 1)
		})
	})
}

func TestTreeBestChild(t *testing.T) {
	convey.Convey("Given multiple children with different UCT scores", t, func() {
		tree := &Tree{explorationWeight: 1.0}
		parent := &Node{visits: 20}
		first := &Node{visits: 4, value: 1}
		second := &Node{visits: 4, value: 3}
		parent.children = []*Node{first, second}

		best := tree.bestChild(parent)

		convey.Convey("It should pick the highest UCT child", func() {
			convey.So(best, convey.ShouldEqual, second)
		})
	})
}
