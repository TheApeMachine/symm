package causal

import (
	"fmt"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/symm/market"
)

func newHYWindowSet() *correlation.WindowSet {
	config := loadRuntimeConfig()
	capacity := config.ContagionSlowMax

	if capacity > 0 {
		return correlation.NewWindowSet(capacity)
	}

	return correlation.NewWindowSet(config.ContagionWindow)
}

func contagionTierWindows() correlation.TierWindows {
	fastWindow, mediumWindow, slowWindow := contagionWindowsFromAdaptation()

	return correlation.TierWindows{
		Fast:   fastWindow,
		Medium: mediumWindow,
		Slow:   slowWindow,
	}
}

func contagionWindowsFromAdaptation() (fastWindow, mediumWindow, slowWindow int) {
	adaptation, err := market.LoadAdaptation()

	if err != nil {
		errnie.Debug(fmt.Sprintf(
			"causal: LoadAdaptation failed, falling back to static windows: %s",
			err.Error(),
		))

		return staticContagionWindows()
	}

	return adaptation.ContagionWindows()
}

func staticContagionWindows() (fastWindow, mediumWindow, slowWindow int) {
	config := loadRuntimeConfig()

	return config.ContagionWindowFast, config.ContagionWindowMedium, config.ContagionWindowSlow
}
