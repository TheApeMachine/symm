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
AdvisorOpinion records one advisor's individual conclusion for a round: its top
chosen market state, probability mass, empirical credibility, and effective weighting.
*/
type AdvisorOpinion struct {
	Advisor     string  `json:"advisor"`
	State       string  `json:"state"`
	Probability float64 `json:"probability"`
	Credibility float64 `json:"credibility"`
	Weight      float64 `json:"weight"`

	// Classes is the advisor's whole reading, not just the class that happened
	// to lead it. A single top probability cannot say whether an advisor was
	// decisive or nearly split between two incompatible readings, and those are
	// different states to act on.
	Classes []AdvisorClass `json:"classes,omitempty"`

	// Maturity is the support behind the reading, floored the same way the
	// consensus weighting floors it, so the displayed weight can be traced back
	// to the three factors that produced it.
	Maturity float64 `json:"maturity"`

	// Contribution is the move mass this advisor alone placed, before
	// normalization. It is what turns the council from a list of opinions into
	// a visible sum.
	Contribution []AdvisorMoveMass `json:"contribution,omitempty"`

	// Unmapped names this advisor's classes that no projection rule accepted,
	// so their weight never reached the consensus. Empty in a correct build.
	Unmapped []string `json:"unmapped,omitempty"`

	// Unscored names classes this advisor declares but could not measure,
	// because their own evidence was incomplete. They took no part in the
	// distribution, so a reading drawn from two of five classes is visibly
	// different from one drawn from all five.
	Unscored []string `json:"unscored,omitempty"`

	// Lease is the clock window this reading is valid over, and ClockNow the
	// coordinate the council is at. A reading close to its expiry is a
	// different thing from a fresh one, and only these three numbers say which.
	Clock      string `json:"clock,omitempty"`
	LeaseFrom  uint64 `json:"leaseFrom"`
	LeaseUntil uint64 `json:"leaseUntil"`
	ClockNow   uint64 `json:"clockNow"`
}

/*
AdvisorClass is one class in an advisor's reading with the probability it
assigned to it.
*/
type AdvisorClass struct {
	State       string  `json:"state"`
	Probability float64 `json:"probability"`
}

/*
AdvisorMoveMass is the mass one advisor placed on one market move.
*/
type AdvisorMoveMass struct {
	Move string  `json:"move"`
	Mass float64 `json:"mass"`
}

/*
AdvisorSilence explains why an advisor contributed nothing to a round.

Silence had exactly one rendering and two entirely different causes: an advisor
whose evidence never completed, and an advisor that spoke and whose reading has
since expired. The first is a wiring fault that can persist unnoticed for as
long as the process runs — four of seven advisors were mute this way — and the
second is the ordinary rhythm of a slow instrument. A surface that cannot tell
them apart cannot report either.
*/
type AdvisorSilence struct {
	Advisor string `json:"advisor"`

	// Reason is "incomplete" when the advisor's declared evidence has never all
	// been present for this symbol, or "expired" when it published a reading
	// whose lease the clock has since passed.
	Reason string `json:"reason"`

	// Missing names the declared metric keys absent from the observation, for
	// the incomplete case. It is the advisor's own contract, unmet.
	Missing []string `json:"missing,omitempty"`

	// Declared is how many keys the advisor declares in total, so a reader can
	// see whether it is one key short or has nothing at all.
	Declared int `json:"declared"`

	// LeaseUntil and ClockNow date an expired reading against the clock.
	LeaseUntil uint64 `json:"leaseUntil,omitempty"`
	ClockNow   uint64 `json:"clockNow,omitempty"`
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
	Advisors      []AdvisorOpinion
	// Silent explains every advisor that contributed nothing this round.
	Silent []AdvisorSilence
	// UnmappedClasses names advisor classes no projection rule accepted, so
	// their weight never reached the consensus mass.
	UnmappedClasses []string
}

/*
DecisionTrace is the complete reasoning record behind one decision: the
council's deliberation and the causal search it fed.
*/
type DecisionTrace struct {
	Deliberation DeliberationTrace
	MCTS         MCTSTrace
}
