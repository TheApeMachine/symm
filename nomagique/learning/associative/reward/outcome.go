package reward

import "time"

/* Mark is the serialized identity/value of one measured objective. */
type Mark struct {
	At      time.Time
	Version uint64
	Value   float64
}

/* Outcome is a Go projection; Ledger exclusively owns numerical accounting. */
type Outcome struct {
	From         Mark
	Through      Mark
	Elapsed      time.Duration
	Reward       float64
	TotalElapsed time.Duration
	TotalReward  float64
	PriorRate    float64
	Rate         float64
	Differential float64
	HasPriorRate bool
	HasRate      bool
	Transitions  uint64
}
