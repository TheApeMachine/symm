package system

import (
	"fmt"

	"github.com/spf13/viper"
)

/*
Detector configures how detect bounds an excursion's precursor and where it
places the executable entry and exit. Neither has a default in code: the
configuration must state both (cmd/cfg/config.yml).

MinPrecursorTicks is the fewest ticks between A and B an emitted excursion
may have once A is clamped to the epoch's first stored tick.

ExecutionLeadFraction pulls the entry back from B and the exit back from C
by this fraction of the precursor's length in ticks, so an order placed at
the learned point can fill before the move it targets.
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

/*
Validate rejects a detector configuration that is missing or out of range.
*/
func (detector *Detector) Validate() error {
	if detector == nil {
		return fmt.Errorf("detector configuration is missing")
	}

	if detector.MinPrecursorTicks < 1 {
		return fmt.Errorf("detector.min_precursor_ticks %d must be at least 1", detector.MinPrecursorTicks)
	}

	if detector.ExecutionLeadFraction < 0 || detector.ExecutionLeadFraction >= 1 {
		return fmt.Errorf("detector.execution_lead_fraction %v must be in [0, 1)", detector.ExecutionLeadFraction)
	}

	return nil
}
