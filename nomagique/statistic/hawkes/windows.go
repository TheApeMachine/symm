package hawkes

import (
	"errors"
	"math"

	"gonum.org/v1/gonum/stat"
)

/*
windowSet reports resolved short, long, and return-lag window counts.
*/
type windowSet struct {
	shortWindow int
	longWindow  int
	returnLag   int
	targetLong  int
}

/*
resolveWindows returns short and long windows for imperative call sites,
deriving both adaptively from history when no explicit hints are given.
*/
func resolveWindows(history []float64, shortHint, longHint int) (shortWindow, longWindow int, err error) {
	set, err := resolveWindowSet(history, shortHint, longHint, 0)

	if err != nil {
		return 0, 0, err
	}

	return set.shortWindow, set.longWindow, nil
}

/*
resolveWindowSet resolves all window counts from history and optional hints.
*/
func resolveWindowSet(history []float64, shortHint, longHint, returnLagHint int) (windowSet, error) {
	sampleCount := len(history)
	shortWindow := shortHint
	longWindow := longHint

	if shortWindow > 0 && longWindow > 0 {
		return buildWindowSet(shortWindow, longWindow, returnLagHint), nil
	}

	if sampleCount <= 0 {
		return windowSet{}, errors.New("hawkes-windows: history or explicit hints required")
	}

	for _, value := range history {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return windowSet{}, errors.New("hawkes-windows: history value must be finite")
		}
	}

	if shortWindow <= 0 {
		shortWindow = max(1, int(math.Ceil(math.Sqrt(float64(sampleCount)))))
	}

	if longWindow <= 0 {
		longWindow = adaptiveLongWindow(history, shortWindow)
	}

	targetLong := longWindow

	if longHint <= 0 {
		targetLong = adaptiveLongWindow(history, shortWindow)
	}

	set := buildWindowSet(shortWindow, longWindow, returnLagHint)
	set.targetLong = targetLong

	return set, nil
}

func buildWindowSet(shortWindow, longWindow, returnLagHint int) windowSet {
	returnLag := returnLagHint

	if returnLag <= 0 {
		returnLag = max(1, int(math.Ceil(math.Sqrt(float64(longWindow)))))

		if longWindow > 1 {
			returnLag = min(returnLag, longWindow-1)
		}
	}

	return windowSet{
		shortWindow: shortWindow,
		longWindow:  longWindow,
		returnLag:   returnLag,
		targetLong:  longWindow,
	}
}

/*
adaptiveLongWindow needs at least two short-scale spans so the current regime
can be compared with an independent preceding span; the span widens with the
history's own coefficient of variation.
*/
func adaptiveLongWindow(history []float64, shortWindow int) int {
	sampleCount := len(history)
	spread := 0.0

	if sampleCount >= 2 {
		mean := stat.Mean(history, nil)

		if mean > 0 && !math.IsNaN(mean) && !math.IsInf(mean, 0) {
			std := stat.StdDev(history, nil)

			if std > 0 && !math.IsNaN(std) && !math.IsInf(std, 0) {
				spread = std / math.Abs(mean)
			}
		}
	}

	longWindow := int(math.Ceil(float64(shortWindow) * (2.0 + spread)))

	if longWindow <= shortWindow {
		longWindow = shortWindow + 1
	}

	if longWindow > sampleCount {
		longWindow = sampleCount
	}

	return longWindow
}
