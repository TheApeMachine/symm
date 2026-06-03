package types

/*
GuardOptions configures overfit rejection for branch-tree search.
*/
type GuardOptions struct {
	MaxReasoningSteps int
	ComplexityPenalty float64
	MinRoundTrips     int
	JitterEnabled     bool
	JitterFractions   []float64
	WalkForward       WalkForwardOptions
}

/*
WalkForwardOptions configures chronological train/test windows.
*/
type WalkForwardOptions struct {
	Enabled         bool
	TrainFraction   float64
	TestFraction    float64
	StepFraction    float64
	MinWinRate      float64
	MaxHoldoutDecay float64
}
