package mcts

import (
	"errors"
	"math"
	"math/rand"
	"time"
)

type CausalMCTS struct {
	CausalEngine CausalEngine
	C            float64 // Exploration constant (traditional UCT)
	CausalAlpha  float64 // Scaling factor for the interventional bias
	MinRows      int     // Minimum historical rows required for SCM fitting
	TreatmentCol int     // Index of Action in the tabular vector
	TargetCol    int     // Index of Reward in the tabular vector
	ControlCols  []int   // Confounding feature indices
	Features     []int   // Global SCM features
	LinearFit    bool    // Toggle for SCM linearity
	Seed         int64
	rng          *rand.Rand
}

func NewCausalMCTS(
	engine CausalEngine,
	c, alpha float64,
	minRows, treatment, target int,
	controls, features []int,
	linear bool,
) *CausalMCTS {
	seed := time.Now().UnixNano()

	return &CausalMCTS{
		CausalEngine: engine,
		C:            c,
		CausalAlpha:  alpha,
		MinRows:      minRows,
		TreatmentCol: treatment,
		TargetCol:    target,
		ControlCols:  controls,
		Features:     features,
		LinearFit:    linear,
		Seed:         seed,
		rng:          rand.New(rand.NewSource(seed)),
	}
}

// Search executes MCTS iterations and returns the explored tree's root
// together with the recommended best action, so a caller can inspect every
// branch actually visited rather than only the one selected.
func (mcts *CausalMCTS) Search(rootState State, iterations int, historicalData [][]float64) (*Node, float64, error) {
	root := &Node{
		State:          rootState,
		UntakenActions: rootState.GetPossibleActions(),
	}

	// Work on a copy of historical data to append simulation steps dynamically
	localHistory := make([][]float64, len(historicalData))
	for i, row := range historicalData {
		localHistory[i] = append([]float64(nil), row...)
	}

	for range iterations {
		// 1. Selection: Traverses tree using UCT + Causal Intervention Bias
		selected := mcts.selectNode(root, localHistory)

		// 2. Expansion: Appends a child representing an untaken action
		expanded := mcts.expandNode(selected)

		// 3. Simulation: Performs a rollout to a terminal state
		reward, trajectory := mcts.simulate(expanded)

		// Incorporate the simulation experience into our SCM history
		for _, stateVec := range trajectory {
			localHistory = append(localHistory, stateVec)
		}

		// 4. Backpropagation: Propagates actual rewards up and counterfactual rewards to siblings
		mcts.causalBackpropagate(expanded, reward, trajectory, localHistory)
	}

	if len(root.Children) == 0 {
		return nil, 0, errors.New("mcts: zero paths explored during search")
	}

	// Select the action associated with the most visited child (robust child strategy)
	bestAction := 0.0
	maxVisits := -1
	for _, child := range root.Children {
		if child.Visits > maxVisits {
			maxVisits = child.Visits
			bestAction = child.Action
		}
	}

	return root, bestAction, nil
}

// selectNode descends the tree using our modified selection equation.
func (mcts *CausalMCTS) selectNode(node *Node, history [][]float64) *Node {
	curr := node
	for len(curr.Children) > 0 && len(curr.UntakenActions) == 0 {
		curr = mcts.bestChild(curr, history)
	}
	return curr
}

// bestChild evaluates UCT score augmented with do-calculus.
func (mcts *CausalMCTS) bestChild(node *Node, history [][]float64) *Node {
	var best *Node
	bestScore := math.Inf(-1)

	for _, child := range node.Children {
		if child.Visits == 0 {
			return child
		}

		if node.Visits <= 0 || child.Visits <= 0 {
			continue
		}

		exploitation := child.TotalReward / float64(child.Visits)
		exploration := mcts.C * math.Sqrt(math.Log(float64(node.Visits))/float64(child.Visits))
		uct := exploitation + exploration

		// Query our causal engine for E[Target | do(Action)]
		causalBias := 0.0
		if len(history) >= mcts.MinRows {
			val, err := mcts.CausalEngine.DoExpectation(
				history,
				mcts.TargetCol,
				mcts.MinRows,
				mcts.TreatmentCol,
				interventionLevel(child),
				mcts.ControlCols,
			)
			if err == nil && !math.IsNaN(val) && !math.IsInf(val, 0) {
				causalBias = val
			}
		}

		score := uct + mcts.CausalAlpha*causalBias
		if score > bestScore {
			bestScore = score
			best = child
		}
	}

	return best
}

// expandNode instantiates a new child node in the search tree.
func (mcts *CausalMCTS) expandNode(node *Node) *Node {
	if len(node.UntakenActions) == 0 {
		return node
	}

	// Pop an untaken action
	action := node.UntakenActions[len(node.UntakenActions)-1]
	node.UntakenActions = node.UntakenActions[:len(node.UntakenActions)-1]

	nextState := node.State.ApplyAction(action)
	child := &Node{
		State:          nextState,
		Action:         action,
		Parent:         node,
		UntakenActions: nextState.GetPossibleActions(),
	}
	node.Children = append(node.Children, child)
	return child
}

// simulate rolls out the state until a terminal condition or horizon limit.
func (mcts *CausalMCTS) simulate(node *Node) (float64, [][]float64) {
	currState := node.State
	trajectory := [][]float64{currState.ToVector()}

	// Soft limit to prevent infinite loops in cyclical environments
	maxRolloutDepth := 100
	depth := 0

	for !currState.IsTerminal() && depth < maxRolloutDepth {
		actions := currState.GetPossibleActions()
		if len(actions) == 0 {
			break
		}

		// Random rollout policy
		action := actions[mcts.rng.Intn(len(actions))]
		currState = currState.ApplyAction(action)
		trajectory = append(trajectory, currState.ToVector())
		depth++
	}

	return currState.GetReward(), trajectory
}

// causalBackpropagate updates visited nodes and runs counterfactual reasoning.
func (mcts *CausalMCTS) causalBackpropagate(leaf *Node, reward float64, trajectory [][]float64, history [][]float64) {
	curr := leaf
	trajectoryIdx := len(trajectory) - 1

	for curr != nil {
		curr.Visits++
		curr.TotalReward += reward

		// Perform abduction, action, and prediction on untaken sibling choices
		if curr.Parent != nil && len(history) >= mcts.MinRows && trajectoryIdx >= 0 {
			actualRow := trajectory[trajectoryIdx]

			for _, sibling := range curr.Parent.Children {
				if sibling == curr {
					continue
				}

				// Infer what reward would have occurred if sibling's action was taken
				virtualReward, noise, err := mcts.CausalEngine.AbductiveCounterfactual(
					history,
					mcts.TargetCol,
					mcts.MinRows,
					mcts.Features,
					mcts.LinearFit,
					actualRow,
					mcts.TreatmentCol,
					interventionLevel(sibling),
				)

				if err == nil && !math.IsNaN(virtualReward) && !math.IsInf(virtualReward, 0) {
					// Scale virtual reward by SCM reconstruction precision
					precision := 1.0 / (1.0 + math.Abs(noise))

					if precision > 1.0 {
						precision = 1.0
					}

					sibling.Visits++
					sibling.TotalReward += virtualReward * precision
				}
			}
		}

		curr = curr.Parent
		trajectoryIdx--
	}
}

// Helper to access node action parameters cleanly
func (n *Node) Rotor() [1]float64 {
	return [1]float64{n.Action}
}
