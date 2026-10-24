package strategy

/*
TickMetrics is one workspace tick's aggregate for the Longtail dashboard.
Counts come only from training/step frames Evaluate and appendEmitReadout publish.
*/
type TickMetrics struct {
	Symbols           int     `json:"symbols"`
	GatePass          int     `json:"gatePass"`  // regime.posture letter changed
	GateTotal         int     `json:"gateTotal"` // symbol steps this tick
	LitTicks          int     `json:"litTicks"`  // steps with non-empty grid lit
	Blocked           int     `json:"blocked"`   // lit but no new letter
	BlockedBrightness int     `json:"blockedBrightness"`
	BlockedDuplicate  int     `json:"blockedDuplicate"` // same letter as the previous step
	BlockedVeto       int     `json:"blockedVeto"`      // risk brighter than posture
	BlockedEmpty      int     `json:"blockedEmpty"`     // a role band was dark
	PolicyChecks      int     `json:"policyChecks"`
	PolicyStored      int     `json:"policyStored"` // live path key exists in explore/policy tree
	OutcomeEnter      int     `json:"outcomeEnter"`
	OutcomeExit       int     `json:"outcomeExit"`
	ExploreEnter      int     `json:"exploredEnter"`
	VenueOpen         int     `json:"venueOpen"`
	VirtualExcursion  int     `json:"virtualExcursion"`
	DevelopedPaths    int     `json:"developedPaths"`
	PromotedPaths     int     `json:"promotedPaths"`
	PrunedPaths       int     `json:"prunedPaths"`
	MeanThreshold     float64 `json:"meanThreshold"`
	ExitProfit        int     `json:"exitProfit"`
	ExitStop          int     `json:"exitStop"`
	ExitBranch        int     `json:"exitBranch"`
	// Legacy keys kept at zero so old clients do not read stale fiction.
	FrictionPass    int     `json:"frictionPass"`
	FrictionTotal   int     `json:"frictionTotal"`
	PathDrift       int     `json:"pathDrift"`
	PathCompared    int     `json:"pathCompared"`
	CanonicalTicks  int     `json:"canonicalTicks"`
	PolicyMatches   int     `json:"policyMatches"`
	ConfirmedEnter  int     `json:"confirmedEnter"`
	MeanMaturity    float64 `json:"meanMaturity"`
	MeanSurprise    float64 `json:"meanSurprise"`
	SurpriseSamples int     `json:"surpriseSamples"`
}

/*
LearningSnapshot is the session's path book, copied onto each longtail tick.
*/
type LearningSnapshot struct {
	Developed     int
	Promoted      int
	Pruned        int
	MeanThreshold float64
	ExitProfit    int
	ExitStop      int
	ExitBranch    int
}
