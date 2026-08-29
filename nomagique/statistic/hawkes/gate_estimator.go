package hawkes

import (
	"math"
	"slices"

	"gonum.org/v1/gonum/stat"
)

/*
fitGateEstimator derives symbol-local gates while reusing quantile storage.
*/
type fitGateEstimator struct {
	window      []float64
	radii       []float64
	asymmetries []float64
}

/*
newFitGateEstimator returns a reusable Hawkes gate estimator.
*/
func newFitGateEstimator() *fitGateEstimator {
	return &fitGateEstimator{}
}

/*
measure derives the same gates as fitGatesFromHistory without transient
slices.
*/
func (estimator *fitGateEstimator) measure(
	spectralRadii, asymmetries []float64,
) (fitGates, bool) {
	estimator.window = slices.Grow(estimator.window[:0], len(spectralRadii))
	estimator.window = estimator.window[:len(spectralRadii)]
	_, longWindow, err := resolveWindows(estimator.window, 0, 0)

	if err != nil || len(spectralRadii) < longWindow || len(asymmetries) < longWindow {
		return fitGates{}, false
	}

	upperRank := 1 - 1/float64(longWindow)
	lowerRank := 1 / float64(longWindow)
	saturationRadius, ok := estimator.quantile(spectralRadii, upperRank, false, &estimator.radii)

	if !ok {
		return fitGates{}, false
	}

	frenzyAsymmetry, ok := estimator.quantile(asymmetries, lowerRank, true, &estimator.asymmetries)

	if !ok {
		return fitGates{}, false
	}

	if saturationRadius <= 0 {
		saturationRadius = criticalBranch
	}

	if frenzyAsymmetry <= 0 {
		frenzyAsymmetry = 1
	}

	return fitGates{
		saturationRadius: saturationRadius,
		frenzyAsymmetry:  frenzyAsymmetry,
	}, true
}

func (estimator *fitGateEstimator) quantile(
	history []float64,
	percentile float64,
	absolute bool,
	scratch *[]float64,
) (float64, bool) {
	*scratch = slices.Grow((*scratch)[:0], len(history))

	for _, value := range history {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, false
		}

		if absolute {
			value = math.Abs(value)
		}

		*scratch = append(*scratch, value)
	}

	slices.Sort(*scratch)

	return stat.Quantile(percentile, stat.LinInterp, *scratch, nil), true
}
