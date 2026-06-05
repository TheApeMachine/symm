package causal

import (
	"github.com/spf13/viper"
)

/*
hyWindowSet holds fast, medium, and slow Hayashi-Yoshida return accumulators so
contagion can react to phase transitions without a single static lookback.
*/
type hyWindowSet struct {
	fast   *hyReturns
	medium *hyReturns
	slow   *hyReturns
}

func newHYWindowSet() *hyWindowSet {
	return &hyWindowSet{
		fast:   newHYReturns(contagionWindowFast()),
		medium: newHYReturns(contagionWindowMedium()),
		slow:   newHYReturns(contagionWindowSlow()),
	}
}

func (windowSet *hyWindowSet) Observe(nanos int64, price float64) {
	windowSet.fast.Observe(nanos, price)
	windowSet.medium.Observe(nanos, price)
	windowSet.slow.Observe(nanos, price)
}

func (windowSet *hyWindowSet) clone() *hyWindowSet {
	if windowSet == nil {
		return nil
	}

	return &hyWindowSet{
		fast:   windowSet.fast.clone(),
		medium: windowSet.medium.clone(),
		slow:   windowSet.slow.clone(),
	}
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
