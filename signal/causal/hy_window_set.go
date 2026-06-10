package causal

import (
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market"
)

/*
hyWindowSet holds one Hayashi-Yoshida return accumulator and materializes fast,
medium, and slow views at read time from the shared adaptation controller.
*/
type hyWindowSet struct {
	series *hyReturns
	fast   *hyReturns
	medium *hyReturns
	slow   *hyReturns
}

func newHYWindowSet() *hyWindowSet {
	return &hyWindowSet{
		series: newHYReturns(contagionSlowCapacity()),
	}
}

func (windowSet *hyWindowSet) Observe(nanos int64, price float64) {
	if windowSet == nil || windowSet.series == nil {
		return
	}

	windowSet.series.Observe(nanos, price)
}

func (windowSet *hyWindowSet) clone() *hyWindowSet {
	if windowSet == nil || windowSet.series == nil {
		return nil
	}

	fastWindow, mediumWindow, slowWindow := contagionWindowsFromAdaptation()

	return &hyWindowSet{
		series: windowSet.series,
		fast:   windowSet.series.cloneTail(fastWindow),
		medium: windowSet.series.cloneTail(mediumWindow),
		slow:   windowSet.series.cloneTail(slowWindow),
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
