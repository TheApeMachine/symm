package mcts

import (
	"fmt"
	"slices"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Graph is intentionally unconstrained. Search requires the explicit
InterventionGraph capability and no longer treats visual graph traversal as a
market transition model.
*/
type Graph any

/*
InterventionGraph bridges a knowledge graph into semantic market state.
ReasoningHistory must return observational rows only; simulations are separate.
*/
type InterventionGraph interface {
	ReasoningFrame() types.Frame
	ReasoningHistory() []types.Frame
	ReasoningKey() string
	ApplyReasoningIntervention(
		state types.Frame,
		action float64,
	) (types.Frame, error)
}

/*
GraphState is a compatibility name for a market-intervention rollout state.
The graph contributes context and dynamics but actions never select node IDs.
*/
type GraphState struct {
	graph        InterventionGraph
	frame        types.Frame
	observations []types.Frame
	key          string
	err          error
}

/*
NewGraphState creates an intervention state. An optional horizon limit preserves
old call sites while allowing graph-derived horizons by default.
*/
func NewGraphState(graph Graph, horizonLimit ...int) *GraphState {
	interventionGraph, supported := graph.(InterventionGraph)

	if !supported {
		return &GraphState{
			err: fmt.Errorf(
				"mcts: graph does not implement the market intervention model",
			),
		}
	}

	frame := interventionGraph.ReasoningFrame()

	if len(horizonLimit) > 0 {
		frame.Put(SymbolMaximumHorizon, float64(horizonLimit[0]))
	}

	state := &GraphState{
		graph:        interventionGraph,
		frame:        frame,
		observations: slices.Clone(interventionGraph.ReasoningHistory()),
		key:          interventionGraph.ReasoningKey(),
	}
	state.err = ValidateReasoningFrame(frame)
	return state
}

func (graphState *GraphState) GetPossibleActions() []float64 {
	if graphState == nil || graphState.err != nil || graphState.IsTerminal() {
		return nil
	}

	position, _ := graphState.frame.Get(SymbolPosition)

	if position == 0 {
		return []float64{ActionWait, ActionEnter}
	}

	return []float64{ActionExit, ActionWait, ActionScale}
}

func (graphState *GraphState) ApplyAction(action float64) State {
	if graphState == nil {
		return &GraphState{err: fmt.Errorf("mcts: cannot apply an action to a nil graph state")}
	}

	nextState := &GraphState{
		graph:        graphState.graph,
		frame:        graphState.frame,
		observations: slices.Clone(graphState.observations),
		key:          graphState.key,
		err:          graphState.err,
	}

	if nextState.err != nil {
		return nextState
	}

	allowed := false

	for _, possible := range graphState.GetPossibleActions() {
		if possible != action {
			continue
		}

		allowed = true
		break
	}

	if !allowed {
		nextState.err = fmt.Errorf(
			"mcts: %s is not valid for the current position",
			ActionName(action),
		)
		return nextState
	}

	frame, err := graphState.graph.ApplyReasoningIntervention(
		graphState.frame, action,
	)

	if err != nil {
		nextState.err = err
		return nextState
	}

	nextState.frame = frame
	nextState.err = ValidateReasoningFrame(frame)
	return nextState
}

func (graphState *GraphState) IsTerminal() bool {
	if graphState == nil || graphState.err != nil {
		return true
	}

	horizon, _ := graphState.frame.Get(SymbolHorizon)
	maximumHorizon, _ := graphState.frame.Get(SymbolMaximumHorizon)
	return horizon >= maximumHorizon
}

func (graphState *GraphState) GetReward() float64 {
	if graphState == nil {
		return 0
	}

	reward, _ := graphState.frame.Get(SymbolTarget)
	return reward
}

func (graphState *GraphState) ToVector() []float64 {
	if graphState == nil {
		return nil
	}

	row, err := FrameToRow(graphState.frame)

	if err != nil {
		return nil
	}

	return row
}

/*
ToFrame returns the named market state for UI inspection.
*/
func (graphState *GraphState) ToFrame() types.Frame {
	if graphState == nil {
		return types.Frame{}
	}

	return graphState.frame
}

/*
History returns copied observational SCM rows supplied by the graph adapter.
*/
func (graphState *GraphState) History() [][]float64 {
	if graphState == nil {
		return nil
	}

	rows := make([][]float64, 0, len(graphState.observations))

	for _, observation := range graphState.observations {
		row, err := FrameToRow(observation)

		if err != nil {
			continue
		}

		rows = append(rows, row)
	}

	return rows
}

/*
HistoryKey identifies the observational stream without embedding key routing in
reducers or transition state.
*/
func (graphState *GraphState) HistoryKey() string {
	if graphState == nil {
		return ""
	}

	return graphState.key
}

/*
Err exposes transition validation failures to the search engine.
*/
func (graphState *GraphState) Err() error {
	if graphState == nil {
		return fmt.Errorf("mcts: graph state is nil")
	}

	return graphState.err
}

/*
Current returns the most recently applied strategic intervention name.
*/
func (graphState *GraphState) Current() string {
	if graphState == nil {
		return ""
	}

	action, found := graphState.frame.Get(SymbolTreatment)

	if !found {
		return ""
	}

	return ActionName(action)
}

/*
Depth returns the current simulated horizon as an integer.
*/
func (graphState *GraphState) Depth() int {
	if graphState == nil {
		return 0
	}

	horizon, _ := graphState.frame.Get(SymbolHorizon)
	return int(horizon)
}

/*
HorizonLimit returns the graph-derived terminal rollout horizon.
*/
func (graphState *GraphState) HorizonLimit() int {
	if graphState == nil {
		return 0
	}

	maximumHorizon, _ := graphState.frame.Get(SymbolMaximumHorizon)
	return int(maximumHorizon)
}
