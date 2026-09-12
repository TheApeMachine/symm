package cognition

/*
LookaheadPath is one scored future branch trajectory.
*/
type LookaheadPath struct {
	Sequence string
	Score    float64
}

/*
ClassCandidate carries evidence recalled for one specific action or category class.
*/
type ClassCandidate struct {
	Name        string
	Probability float64
	Support     uint64
	Order       int
}

/*
Evaluation is the full information-theoretic readout of an evaluated context.
It is plain wire payload: no methods.
*/
type Evaluation struct {
	// What is being asked about, and the trie clock it was read under. Both
	// travel with the reading so a caller never has to pair an answer back up
	// with the question by position.
	Context []byte
	Step    uint64

	// How much evidence stands behind the leading class, which is what
	// separates a strong association from one coincidence of the same shape.
	Support uint64

	// Attractor Basin Classification
	WinnerClass string
	RunnerUp    string
	Confidence  float64
	Contrast    float64 // Difference in bits between winner and runner-up
	Candidates  []ClassCandidate

	// Information Content & Uncertainty
	Surprisal float64 // -log2 P of the context transition
	Ambiguity float64 // Normalized Shannon entropy in [0, 1] via nomagique/probability
	IsBreak   bool    // True if Surprisal >= SurprisalBreakBits

	// Predictive Lookahead
	Lookahead []LookaheadPath
}
