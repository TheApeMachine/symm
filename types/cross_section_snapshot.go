package types

import "time"

/*
CrossSectionSnapshot is one immutable, market-wide aggregation emitted by the
CrossSection stage for every ticker observation. Cross-symbol signals consume
this snapshot instead of scanning the whole universe themselves, so breadth,
leadership, and cohort statistics are computed exactly once per tick.
*/
type CrossSectionSnapshot struct {
	At              time.Time         `json:"at"`
	Breadth         float64           `json:"breadth"`
	MedianReturn    float64           `json:"medianReturn"`
	MedianMagnitude float64           `json:"medianMagnitude"`
	LeaderSymbol    string            `json:"leaderSymbol"`
	LeaderReturn    float64           `json:"leaderReturn"`
	LeaderPath      []float64         `json:"leaderPath,omitempty"`
	MedianSpread    float64           `json:"medianSpread"`
	MedianDepth     float64           `json:"medianDepth"`
	Returns         map[string]float64 `json:"returns"`
}
