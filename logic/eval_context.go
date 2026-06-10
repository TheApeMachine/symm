package logic

import (
	"fmt"

	"github.com/spf13/viper"
)

/*
EvalContext resolves dynamic playbook thresholds against the live measurement window.
*/
type EvalContext struct {
	measurements []Measurement
	holdings     *Holdings
}

func NewEvalContext(measurements []Measurement, holdings *Holdings) *EvalContext {
	return &EvalContext{
		measurements: measurements,
		holdings:     holdings,
	}
}

func (evalContext *EvalContext) Resolve(ref string) (float64, error) {
	switch ref {
	case "confidence.regime_adjusted_baseline":
		return evalContext.regimeAdjustedConfidenceBaseline()
	case "surprise.regime_adjusted_baseline":
		return evalContext.regimeAdjustedSurpriseBaseline()
	default:
		return 0, fmt.Errorf("logic: unknown dynamic ref %q", ref)
	}
}

func (evalContext *EvalContext) regimeAdjustedConfidenceBaseline() (float64, error) {
	baseline := viper.GetFloat64("trading.entry.confidence_baseline")

	if baseline <= 0 {
		return 0, fmt.Errorf("logic: trading.entry.confidence_baseline must be positive")
	}

	scale := viper.GetFloat64("trading.entry.turbulence_confidence_scale")

	if scale <= 0 {
		return 0, fmt.Errorf("logic: trading.entry.turbulence_confidence_scale must be positive")
	}

	return baseline + evalContext.fluidTurbulence()*scale, nil
}

func (evalContext *EvalContext) regimeAdjustedSurpriseBaseline() (float64, error) {
	baseline := viper.GetFloat64("trading.entry.surprise_baseline")

	if baseline <= 0 {
		return 0, fmt.Errorf("logic: trading.entry.surprise_baseline must be positive")
	}

	scale := viper.GetFloat64("trading.entry.turbulence_surprise_scale")

	if scale <= 0 {
		return 0, fmt.Errorf("logic: trading.entry.turbulence_surprise_scale must be positive")
	}

	floor := baseline - evalContext.fluidTurbulence()*scale

	if floor <= 0 {
		return 0, fmt.Errorf("logic: regime-adjusted surprise floor must be positive")
	}

	return floor, nil
}

func (evalContext *EvalContext) fluidTurbulence() float64 {
	turbulence := 0.0

	for _, measurement := range evalContext.measurements {
		if measurement.Source != SourceFluid {
			continue
		}

		if measurement.Strength > turbulence {
			turbulence = measurement.Strength
		}
	}

	return turbulence
}
