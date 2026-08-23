package mcts

import (
	"fmt"
	"math"
	"math/rand"
	"slices"
	"time"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
CausalMCTS performs UCT selection augmented by interventional expectation and
precision-weighted counterfactual sibling updates.
*/
type CausalMCTS struct {
	CausalEngine        CausalEngine
	ExplorationConstant float64
	CausalAlpha         float64
	MinRows             int
	TreatmentCol        int
	TargetCol           int
	ControlCols         []int
	Features            []int
	LinearFit           bool
	Seed                int64
	random              *rand.Rand
	err                 error
}

/*
NewCausalMCTS preserves the former constructor boundary while validating the
configuration before search. The eight parameters are exploration constant,
causal alpha, minimum rows, treatment column, target column, control columns,
feature columns, and linear-fit mode.
*/
func NewCausalMCTS(
	engine CausalEngine,
	parameters ...interface{},
) *CausalMCTS {
	search := &CausalMCTS{CausalEngine: engine}

	if len(parameters) != 8 {
		search.err = fmt.Errorf(
			"mcts: constructor requires 8 parameters; received %d",
			len(parameters),
		)
		return search
	}

	explorationConstant, explorationSupported := parameters[0].(float64)
	causalAlpha, alphaSupported := parameters[1].(float64)
	minimumRows, minimumSupported := parameters[2].(int)
	treatmentColumn, treatmentSupported := parameters[3].(int)
	targetColumn, targetSupported := parameters[4].(int)
	controlColumns, controlsSupported := parameters[5].([]int)
	features, featuresSupported := parameters[6].([]int)
	linearFit, linearSupported := parameters[7].(bool)

	if !explorationSupported || !alphaSupported || !minimumSupported ||
		!treatmentSupported || !targetSupported || !controlsSupported ||
		!featuresSupported || !linearSupported {
		search.err = fmt.Errorf(
			"mcts: constructor parameters do not match the causal search contract",
		)
		return search
	}

	return NewCausalMCTSWithSeed(
		engine,
		explorationConstant,
		causalAlpha,
		minimumRows,
		treatmentColumn,
		targetColumn,
		controlColumns,
		features,
		linearFit,
		time.Now().UnixNano(),
	)
}

/*
NewCausalMCTSWithSeed creates a deterministic search engine for replay and tests.
*/
func NewCausalMCTSWithSeed(
	engine CausalEngine,
	explorationConstant float64,
	causalAlpha float64,
	minimumRows int,
	treatmentColumn int,
	targetColumn int,
	controlColumns []int,
	features []int,
	linearFit bool,
	seed int64,
) *CausalMCTS {
	return &CausalMCTS{
		CausalEngine:        engine,
		ExplorationConstant: explorationConstant,
		CausalAlpha:         causalAlpha,
		MinRows:             minimumRows,
		TreatmentCol:        treatmentColumn,
		TargetCol:           targetColumn,
		ControlCols:         slices.Clone(controlColumns),
		Features:            slices.Clone(features),
		LinearFit:           linearFit,
		Seed:                seed,
		random:              rand.New(rand.NewSource(seed)),
	}
}

/*
Search explores strategic interventions. historicalData must contain observed
market rows only; simulated trajectories are never inserted into the SCM fit.
*/
func (search *CausalMCTS) Search(
	rootState State,
	iterations int,
	historicalData [][]float64,
) (*Node, float64, error) {
	if search.err != nil {
		return nil, 0, search.err
	}

	if rootState == nil {
		return nil, 0, fmt.Errorf("mcts: root state is nil")
	}

	if iterations < 1 {
		return nil, 0, fmt.Errorf("mcts: iterations must be positive")
	}

	if search.CausalEngine == nil {
		return nil, 0, fmt.Errorf("mcts: causal engine is nil")
	}

	if graphState, supported := rootState.(*GraphState); supported {
		if err := graphState.Err(); err != nil {
			return nil, 0, err
		}
	}

	observations, err := search.observationalHistory(rootState, historicalData)

	if err != nil {
		return nil, 0, err
	}

	root := &Node{
		State:          rootState,
		UntakenActions: slices.Clone(rootState.GetPossibleActions()),
		SCMReason:      search.readinessReason(observations),
	}
	root.SCMReady = len(observations) >= search.MinRows

	for iteration := 0; iteration < iterations; iteration++ {
		selected, err := search.selectNode(root, observations)

		if err != nil {
			return nil, 0, err
		}

		expanded, err := search.expandNode(selected)

		if err != nil {
			return nil, 0, err
		}

		reward, trajectory, err := search.simulate(expanded)

		if err != nil {
			return nil, 0, err
		}

		if err := search.causalBackpropagate(
			expanded, reward, trajectory, observations,
		); err != nil {
			return nil, 0, err
		}
	}

	if len(root.Children) == 0 {
		return nil, 0, fmt.Errorf("mcts: zero strategic paths explored")
	}

	bestChild := robustChild(root.Children)
	bestChild.Selected = true
	markPrincipalVariation(bestChild)
	return root, bestChild.Action, nil
}

/*
SearchFrames accepts named observational frames and preserves the same search
contract as Search.
*/
func (search *CausalMCTS) SearchFrames(
	rootState State,
	iterations int,
	history []types.Frame,
) (*Node, float64, error) {
	rows := make([][]float64, 0, len(history))

	for historyIndex, frame := range history {
		row, err := FrameToRow(frame)

		if err != nil {
			return nil, 0, fmt.Errorf(
				"mcts: history frame %d is invalid: %w",
				historyIndex, err,
			)
		}

		rows = append(rows, row)
	}

	return search.Search(rootState, iterations, rows)
}

func (search *CausalMCTS) observationalHistory(
	rootState State,
	historicalData [][]float64,
) ([][]float64, error) {
	observations := make([][]float64, 0, len(historicalData))

	for rowIndex, row := range historicalData {
		if search.TargetCol < 0 || search.TargetCol >= len(row) ||
			search.TreatmentCol < 0 || search.TreatmentCol >= len(row) {
			return nil, fmt.Errorf(
				"mcts: observational row %d does not contain treatment %d and target %d",
				rowIndex, search.TreatmentCol, search.TargetCol,
			)
		}

		observations = append(observations, slices.Clone(row))
	}

	graphState, supported := rootState.(*GraphState)

	if !supported {
		return observations, nil
	}

	for _, row := range graphState.History() {
		observations = append(observations, slices.Clone(row))
	}

	return observations, nil
}

func (search *CausalMCTS) readinessReason(history [][]float64) string {
	if len(history) >= search.MinRows {
		return "observational SCM fitted"
	}

	return fmt.Sprintf(
		"observational SCM warming: %d/%d rows",
		len(history), search.MinRows,
	)
}

func (search *CausalMCTS) selectNode(
	node *Node,
	history [][]float64,
) (*Node, error) {
	current := node

	for len(current.Children) > 0 && len(current.UntakenActions) == 0 {
		best, err := search.bestChild(current, history)

		if err != nil {
			return nil, err
		}

		current = best
	}

	return current, nil
}

func (search *CausalMCTS) bestChild(
	node *Node,
	history [][]float64,
) (*Node, error) {
	if len(node.Children) == 0 {
		return nil, fmt.Errorf("mcts: cannot select from an empty branch")
	}

	bestScore := math.Inf(-1)
	var best *Node

	for _, child := range node.Children {
		if child.EffectiveVisits() == 0 {
			return child, nil
		}

		if node.Visits < 1 {
			return nil, fmt.Errorf("mcts: parent visits must be positive for UCT")
		}

		child.Exploitation = child.MeanReward()
		child.Exploration = search.ExplorationConstant * math.Sqrt(
			math.Log(float64(node.Visits))/child.EffectiveVisits(),
		)
		child.CausalExpectation = 0
		child.SCMReady = len(history) >= search.MinRows
		child.SCMReason = search.readinessReason(history)

		if child.SCMReady {
			expectation, err := search.CausalEngine.DoExpectation(
				history,
				search.TargetCol,
				search.MinRows,
				search.TreatmentCol,
				child.Action,
				search.ControlCols,
			)

			if err != nil {
				return nil, fmt.Errorf(
					"mcts: do(%s) expectation failed: %w",
					ActionName(child.Action), err,
				)
			}

			child.CausalExpectation = expectation
		}

		child.SelectionScore = child.Exploitation + child.Exploration +
			search.CausalAlpha*child.CausalExpectation

		if child.SelectionScore <= bestScore {
			continue
		}

		bestScore = child.SelectionScore
		best = child
	}

	if best == nil {
		return nil, fmt.Errorf("mcts: no selectable strategic child")
	}

	return best, nil
}

func (search *CausalMCTS) expandNode(node *Node) (*Node, error) {
	if len(node.UntakenActions) == 0 {
		return node, nil
	}

	actionIndex := len(node.UntakenActions) - 1
	action := node.UntakenActions[actionIndex]
	node.UntakenActions = node.UntakenActions[:actionIndex]
	nextState := node.State.ApplyAction(action)

	if graphState, supported := nextState.(*GraphState); supported {
		if err := graphState.Err(); err != nil {
			return nil, err
		}
	}

	child := &Node{
		State:          nextState,
		Action:         action,
		Parent:         node,
		UntakenActions: slices.Clone(nextState.GetPossibleActions()),
		Depth:          node.Depth + 1,
	}
	node.Children = append(node.Children, child)
	return child, nil
}

func (search *CausalMCTS) simulate(
	node *Node,
) (float64, [][]float64, error) {
	currentState := node.State
	trajectory := make([][]float64, 0)

	for !currentState.IsTerminal() {
		actions := currentState.GetPossibleActions()

		if len(actions) == 0 {
			break
		}

		action := actions[search.random.Intn(len(actions))]
		currentState = currentState.ApplyAction(action)

		if graphState, supported := currentState.(*GraphState); supported {
			if err := graphState.Err(); err != nil {
				return 0, nil, err
			}
		}

		row := currentState.ToVector()

		if len(row) == 0 {
			return 0, nil, fmt.Errorf("mcts: simulated state produced an empty SCM row")
		}

		trajectory = append(trajectory, row)
	}

	if len(trajectory) == 0 {
		row := currentState.ToVector()

		if len(row) > 0 {
			trajectory = append(trajectory, row)
		}
	}

	return currentState.GetReward(), trajectory, nil
}

func (search *CausalMCTS) causalBackpropagate(
	leaf *Node,
	reward float64,
	trajectory [][]float64,
	history [][]float64,
) error {
	current := leaf
	trajectoryIndex := len(trajectory) - 1

	for current != nil {
		current.Visits++
		current.ObservedReward += reward
		current.TotalReward += reward
		current.SCMReady = len(history) >= search.MinRows
		current.SCMReason = search.readinessReason(history)

		if current.Parent != nil && current.SCMReady && trajectoryIndex >= 0 {
			actualRow := trajectory[trajectoryIndex]

			for _, sibling := range current.Parent.Children {
				if sibling == current {
					continue
				}

				counterfactualReward, noise, err := search.CausalEngine.AbductiveCounterfactual(
					history,
					search.TargetCol,
					search.MinRows,
					search.Features,
					search.LinearFit,
					actualRow,
					search.TreatmentCol,
					sibling.Action,
				)

				if err != nil {
					return fmt.Errorf(
						"mcts: counterfactual do(%s) failed: %w",
						ActionName(sibling.Action), err,
					)
				}

				precision := 1 / (1 + math.Abs(noise))
				weightedReward := counterfactualReward * precision
				sibling.CounterfactualReward += weightedReward
				sibling.CounterfactualMass += precision
				sibling.CounterfactualPrecision = precision
				sibling.TotalReward += weightedReward
				sibling.SCMReady = true
				sibling.SCMReason = "counterfactual sibling update"
			}
		}

		current = current.Parent

		if trajectoryIndex > 0 {
			trajectoryIndex--
		}
	}

	return nil
}

func robustChild(children []*Node) *Node {
	best := children[0]

	for _, child := range children[1:] {
		if child.Visits > best.Visits {
			best = child
			continue
		}

		if child.Visits == best.Visits && child.MeanReward() > best.MeanReward() {
			best = child
		}
	}

	return best
}

func markPrincipalVariation(node *Node) {
	current := node

	for current != nil {
		current.Principal = true

		if len(current.Children) == 0 {
			return
		}

		current = robustChild(current.Children)
	}
}
