package types

/*
MCTSNodeTrace is one node of the search tree as it is shown to an operator.

It mirrors the search's own node statistics rather than summarizing them: the
point of showing a decision tree live is to see why a branch won, which means
seeing real rollout reward and virtual counterfactual evidence separately
rather than pre-blended into one number.
*/
type MCTSNodeTrace struct {
	Action                   string
	Depth                    int
	Visits                   int
	EffectiveVisits          float64
	MeanReward               float64
	RewardStd                float64
	BlendedValue             float64
	CounterfactualReward     float64
	CounterfactualMass       float64
	CounterfactualPrecision  float64
	CausalExpectation        float64
	CausalExpectationDefined bool
	Pruned                   bool
	Selected                 bool
	Children                 []MCTSNodeTrace
}

/*
MCTSBranchTrace is one root action's aggregate statistics.
*/
type MCTSBranchTrace struct {
	Action                   string
	Visits                   int
	MeanReward               float64
	RewardStd                float64
	BlendedValue             float64
	CounterfactualReward     float64
	CounterfactualMass       float64
	CounterfactualMean       float64
	EffectiveVisits          float64
	CausalExpectation        float64
	CausalExpectationDefined bool
	Pruned                   bool
}

/*
MCTSTrace is the search provenance for one decision round: what the search was
configured with, what it explored, and what it concluded.
*/
type MCTSTrace struct {
	Iterations           int
	Horizon              int
	ExplorationConstant  float64
	UncertaintyWeight    float64
	RecommendedAction    string
	ExpectedOutcome      float64
	OutcomeUncertainty   float64
	IdentificationStatus string
	DecisionUnavailable  bool
	TransitionSource     string
	Branches             []MCTSBranchTrace
	Tree                 *MCTSNodeTrace
	MaxDepth             int
	TotalNodes           int
}

/*
DeliberationTrace is the War Room's consensus for one decision round, retained
so an operator can see which readings shaped the odds the search then used.
*/
type DeliberationTrace struct {
	DominantMove  string
	Confidence    float64
	Participants  int
	Probabilities map[string]float64
	Vetoes        []string
	Synergies     []string
}

/*
DecisionTrace is the complete reasoning record behind one decision: the
council's deliberation and the causal search it fed.
*/
type DecisionTrace struct {
	Deliberation DeliberationTrace
	MCTS         MCTSTrace
}
