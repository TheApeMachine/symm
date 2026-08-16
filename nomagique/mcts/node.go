package mcts

import "encoding/json"

/*
Node contains both search statistics and the decomposition required to audit why
a branch was selected. Parent and mutable state are excluded from JSON.
*/
type Node struct {
	State                   State
	Action                  float64
	Parent                  *Node
	Children                []*Node
	UntakenActions          []float64
	Visits                  int
	TotalReward             float64
	ObservedReward          float64
	CounterfactualReward    float64
	CounterfactualMass      float64
	CounterfactualPrecision float64
	Exploitation            float64
	Exploration             float64
	CausalExpectation       float64
	SelectionScore          float64
	SCMReady                bool
	SCMReason               string
	Selected                bool
	Principal               bool
	Depth                   int
}

/*
EffectiveVisits combines real rollout visits and precision-weighted virtual
experience without presenting counterfactual samples as observed visits.
*/
func (node *Node) EffectiveVisits() float64 {
	if node == nil {
		return 0
	}

	return float64(node.Visits) + node.CounterfactualMass
}

/*
MeanReward returns the precision-weighted value used for exploitation.
*/
func (node *Node) MeanReward() float64 {
	effectiveVisits := node.EffectiveVisits()

	if effectiveVisits == 0 {
		return 0
	}

	return node.TotalReward / effectiveVisits
}

/*
NodeTrace is the stable wire representation consumed by the reasoning UI.
*/
type NodeTrace struct {
	Action                  float64            `json:"action"`
	ActionName              string             `json:"actionName"`
	Depth                   int                `json:"depth"`
	Visits                  int                `json:"visits"`
	EffectiveVisits         float64            `json:"effectiveVisits"`
	ObservedReward          float64            `json:"observedReward"`
	CounterfactualReward    float64            `json:"counterfactualReward"`
	CounterfactualMass      float64            `json:"counterfactualMass"`
	CounterfactualPrecision float64            `json:"counterfactualPrecision"`
	TotalReward             float64            `json:"totalReward"`
	MeanReward              float64            `json:"meanReward"`
	Exploitation            float64            `json:"exploitation"`
	Exploration             float64            `json:"exploration"`
	CausalExpectation       float64            `json:"causalExpectation"`
	SelectionScore          float64            `json:"selectionScore"`
	SCMReady                bool               `json:"scmReady"`
	SCMReason               string             `json:"scmReason,omitempty"`
	Selected                bool               `json:"selected"`
	Principal               bool               `json:"principal"`
	State                   map[string]float64 `json:"state,omitempty"`
	Children                []NodeTrace        `json:"children,omitempty"`
}

/*
Trace creates an acyclic, named snapshot of the explored tree.
*/
func (node *Node) Trace() NodeTrace {
	trace := NodeTrace{
		Action:                  node.Action,
		ActionName:              ActionName(node.Action),
		Depth:                   node.Depth,
		Visits:                  node.Visits,
		EffectiveVisits:         node.EffectiveVisits(),
		ObservedReward:          node.ObservedReward,
		CounterfactualReward:    node.CounterfactualReward,
		CounterfactualMass:      node.CounterfactualMass,
		CounterfactualPrecision: node.CounterfactualPrecision,
		TotalReward:             node.TotalReward,
		MeanReward:              node.MeanReward(),
		Exploitation:            node.Exploitation,
		Exploration:             node.Exploration,
		CausalExpectation:       node.CausalExpectation,
		SelectionScore:          node.SelectionScore,
		SCMReady:                node.SCMReady,
		SCMReason:               node.SCMReason,
		Selected:                node.Selected,
		Principal:               node.Principal,
	}

	if graphState, supported := node.State.(*GraphState); supported {
		trace.State = FrameValues(graphState.ToFrame())
	}

	if len(node.Children) == 0 {
		return trace
	}

	trace.Children = make([]NodeTrace, len(node.Children))

	for childIndex, child := range node.Children {
		trace.Children[childIndex] = child.Trace()
	}

	return trace
}

/*
MarshalJSON publishes the inspectable trace instead of pointer-linked internals.
*/
func (node *Node) MarshalJSON() ([]byte, error) {
	return json.Marshal(node.Trace())
}

/*
Child returns the explored branch for one strategic intervention.
*/
func (node *Node) Child(action float64) *Node {
	if node == nil {
		return nil
	}

	for _, child := range node.Children {
		if child.Action == action {
			return child
		}
	}

	return nil
}

/*
MustChild returns an explored intervention branch and panics when the requested
branch is absent, because an incomplete search tree must not silently authorize
a decision.
*/
func (node *Node) MustChild(action float64) *Node {
	child := node.Child(action)

	if child == nil {
		panic("mcts: explored tree does not contain action " + ActionName(action))
	}

	return child
}
