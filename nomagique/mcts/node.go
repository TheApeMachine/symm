package mcts

import "math"

/*
SearchNode is one node of the economic search tree. It accumulates only
economic rollout outcomes; no unrelated signal scores are injected into node
reward after causal simulation.
*/
type SearchNode struct {
	State         State
	Action        Action
	Parent        *SearchNode
	Children      []*SearchNode
	UntakenActions []Action
	Visits        int
	TotalReward   float64
	SumSquares    float64
	Depth         int
}

/*
MeanReward is the average economic reward observed at this node.
*/
func (node *SearchNode) MeanReward() float64 {
	if node == nil || node.Visits == 0 {
		return 0
	}

	return node.TotalReward / float64(node.Visits)
}

/*
RewardStandardDeviation is the sample standard deviation of the economic
reward, used for outcome uncertainty. It is the real reward dispersion, never
a pseudo-precision transform.
*/
func (node *SearchNode) RewardStandardDeviation() float64 {
	if node == nil || node.Visits < 2 {
		return 0
	}

	variance := node.SumSquares/float64(node.Visits) - node.MeanReward()*node.MeanReward()

	if variance < 0 {
		variance = 0
	}

	return math.Sqrt(variance)
}

/*
StandardError is the standard error of the mean reward.
*/
func (node *SearchNode) StandardError() float64 {
	if node == nil || node.Visits == 0 {
		return 0
	}

	return node.RewardStandardDeviation() / math.Sqrt(float64(node.Visits))
}
