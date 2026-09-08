package cognition

// LookaheadPath is one scored future branch trajectory.
type LookaheadPath struct {
	Sequence string
	Score    float64
}

/*
Evaluation is the full information-theoretic readout of an evaluated context.
*/
type Evaluation struct {
	// Attractor Basin Classification
	WinnerClass string
	RunnerUp    string
	Confidence  float64
	Contrast    float64 // Difference in bits between winner and runner-up

	// Information Content & Uncertainty
	Surprisal float64 // -log2 P of the context transition
	Ambiguity float64 // Normalized Shannon entropy in [0, 1] via nomagique/probability
	IsBreak   bool    // True if Surprisal >= SurprisalBreakBits

	// Predictive Lookahead
	Lookahead []LookaheadPath
}
