package manifold

import (
	"time"

	"github.com/spf13/viper"
)

const (
	Valid = "valid"

	// BaselineEpsilon is the adaptive baseline floor for resonance normalization.
	BaselineEpsilon = 1e-12
)

/*
DefaultBaselineHalflife is the event-time scale resonance uses when deriving
adaptive baselines from observed manifold readouts.
*/
func DefaultBaselineHalflife() time.Duration {
	halflife := viper.GetDuration("market.baseline_halflife")

	if halflife <= 0 {
		return 30 * time.Second
	}

	return halflife
}
