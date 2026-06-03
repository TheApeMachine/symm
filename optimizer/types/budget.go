package types

/*
SearchBudget holds scan, guard, MCTS, and simulation limits derived from the
measurement tape and profile. Nothing here is user-tunable.
*/
type SearchBudget struct {
	BeamWidth                  int
	CandidateLimit             int
	MaxThresholds              int
	MaxReasoningSteps          int
	MeasurementSampleCap       int
	HybridSeedCount            int
	MCTSIterations             int
	MCTSSeedPriorVisits        int
	MinChainSupport            int
	BeamPruneFactor            int
	MaxGatesPerSurvivor        int
	MaxWidensPerSurvivor       int
	ReentryTickCooldown        int
	MinRoundTrips              int
	ComplexityPenalty          float64
	ExplorationWeight          float64
	MCTSRewardScale            float64
	NearMissTickJitter         int
	TheoreticalUCTDiscount     float64
	AdversarialRolloutInterval int
	AdversarialRolloutFraction float64
}

func (budget SearchBudget) IsZero() bool {
	return budget.BeamWidth <= 0
}
