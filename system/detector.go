package system

import "github.com/spf13/viper"

/*
Detector configures how detect bounds an excursion's precursor and where it
places the executable entry and exit. Neither has a default in code: the
configuration must state both (cmd/cfg/config.yml).

MinPrecursorTicks is the fewest ticks between A and B an emitted excursion
may have once A is clamped to the epoch's first stored tick.

ExecutionLeadFraction pulls recorded B and C (and entry/exit) back by this
fraction of the A→B precursor length in ticks, so orders placed on those
points can fill before the move they target.
*/
type Detector struct {
	MinPrecursorTicks     int64
	ExecutionLeadFraction float64
}

func NewDetector() *Detector {
	return &Detector{
		MinPrecursorTicks:     viper.GetInt64("detector.min_precursor_ticks"),
		ExecutionLeadFraction: viper.GetFloat64("detector.execution_lead_fraction"),
	}
}
