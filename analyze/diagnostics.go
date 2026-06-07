package analyze

import (
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
DiagnosticThresholds parameterises the signal health battery. Defaults match the
original static constants; regime and config overlays widen or tighten per context.
*/
type DiagnosticThresholds struct {
	DeadDispersion     float64
	FlickerCrossing    float64
	FlickerAutocorr    float64
	SmoothAutocorr     float64
	FlatExcursionRatio float64
	UnstableSwitchRate float64
}

/*
DefaultDiagnosticThresholds returns the baseline battery before regime adjustment.
*/
func DefaultDiagnosticThresholds() DiagnosticThresholds {
	return DiagnosticThresholds{
		DeadDispersion:     deadDispersion,
		FlickerCrossing:    flickerCrossing,
		FlickerAutocorr:    flickerAutocorr,
		SmoothAutocorr:     smoothAutocorr,
		FlatExcursionRatio: flatExcursionRatio,
		UnstableSwitchRate: unstableSwitchRate,
	}
}

/*
DiagnosticThresholdsForRegime calibrates verdict cutoffs to the prevailing market regime.
*/
func DiagnosticThresholdsForRegime(regime types.Regime) DiagnosticThresholds {
	thresholds := DiagnosticThresholdsFromConfig()

	switch regime {
	case types.RegimeChoppy:
		thresholds.FlickerCrossing *= 1.25
		thresholds.FlatExcursionRatio *= 1.1
	case types.RegimeBearish, types.RegimeDead:
		thresholds.FlickerCrossing *= 0.65
		thresholds.SmoothAutocorr *= 1.1
	case types.RegimeBullish, types.RegimeTrending:
		thresholds.FlickerCrossing *= 1.15
		thresholds.FlatExcursionRatio *= 0.95
	default:
	}

	return thresholds
}

/*
DiagnosticThresholdsFromConfig reads signals.diagnostics from strategy config.
*/
func DiagnosticThresholdsFromConfig() DiagnosticThresholds {
	thresholds := DefaultDiagnosticThresholds()
	prefix := "signals.diagnostics"

	if value := viper.GetFloat64(prefix + ".dead_dispersion"); value > 0 {
		thresholds.DeadDispersion = value
	}

	if value := viper.GetFloat64(prefix + ".flicker_crossing"); value > 0 {
		thresholds.FlickerCrossing = value
	}

	if value := viper.GetFloat64(prefix + ".flicker_autocorr"); value > 0 {
		thresholds.FlickerAutocorr = value
	}

	if value := viper.GetFloat64(prefix + ".smooth_autocorr"); value > 0 {
		thresholds.SmoothAutocorr = value
	}

	if value := viper.GetFloat64(prefix + ".flat_excursion_ratio"); value > 0 {
		thresholds.FlatExcursionRatio = value
	}

	if value := viper.GetFloat64(prefix + ".unstable_switch_rate"); value > 0 {
		thresholds.UnstableSwitchRate = value
	}

	return thresholds
}
