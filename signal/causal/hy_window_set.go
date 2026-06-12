package causal

import (
	"fmt"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/symm/market"
)

func newHYWindowSet() *correlation.WindowSet {
	return correlation.NewWindowSet(contagionSlowCapacity())
}

func contagionTierWindows() correlation.TierWindows {
	fastWindow, mediumWindow, slowWindow := contagionWindowsFromAdaptation()

	return correlation.TierWindows{
		Fast:   fastWindow,
		Medium: mediumWindow,
		Slow:   slowWindow,
	}
}

func contagionSlowCapacity() int {
	capacity := viper.GetViper().GetInt("signals.causal.contagion_window_slow_max")

	if capacity > 0 {
		return capacity
	}

	return contagionWindow()
}

func contagionWindowsFromAdaptation() (fastWindow, mediumWindow, slowWindow int) {
	adaptation, err := market.LoadAdaptation()

	if err != nil {
		errnie.Debug(fmt.Sprintf(
			"causal: LoadAdaptation failed, falling back to static windows: %s",
			err.Error(),
		))

		return contagionWindowFast(), contagionWindowMedium(), contagionWindowSlow()
	}

	return adaptation.ContagionWindows()
}

func contagionWindow() int {
	window := viper.GetViper().GetInt("signals.causal.contagion_window")

	if window > 0 {
		return window
	}

	return 128
}

func contagionWindowFast() int {
	window := viper.GetViper().GetInt("signals.causal.contagion_window_fast")

	if window > 0 {
		return window
	}

	return contagionWindow() / 8
}

func contagionWindowMedium() int {
	window := viper.GetViper().GetInt("signals.causal.contagion_window_medium")

	if window > 0 {
		return window
	}

	return contagionWindow() / 2
}

func contagionWindowSlow() int {
	window := viper.GetViper().GetInt("signals.causal.contagion_window_slow")

	if window > 0 {
		return window
	}

	return contagionWindow()
}

func contagionAdaptiveSigma() float64 {
	sigma := viper.GetViper().GetFloat64("signals.causal.contagion_adaptive_sigma")

	if sigma > 0 {
		return sigma
	}

	return 2
}

func contagionVolatilityResetSigma() float64 {
	sigma := viper.GetViper().GetFloat64("signals.causal.contagion_volatility_reset_sigma")

	if sigma > 0 {
		return sigma
	}

	return 5
}
