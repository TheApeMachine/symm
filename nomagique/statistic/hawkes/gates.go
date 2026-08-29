package hawkes

import (
	"errors"
	"math"
	"sort"

	"gonum.org/v1/gonum/stat"
)

/*
fitGates carries series-local saturation and frenzy thresholds derived from
fit history.
*/
type fitGates struct {
	saturationRadius float64
	frenzyAsymmetry  float64
}

/*
ready reports whether both gates were derived from sufficient history.
*/
func (gates fitGates) ready() bool {
	return gates.saturationRadius > 0 && gates.frenzyAsymmetry > 0
}

/*
fitGatesFromHistory derives saturation and frenzy gates from observed fit
statistics.
*/
func fitGatesFromHistory(spectralRadii, asymmetries []float64) (fitGates, bool) {
	_, longWindow, err := resolveWindows(make([]float64, len(spectralRadii)), 0, 0)

	if err != nil || len(spectralRadii) < longWindow || len(asymmetries) < longWindow {
		return fitGates{}, false
	}

	upperRank := 1 - 1/float64(longWindow)
	lowerRank := 1 / float64(longWindow)
	saturationRadius, err := quantileFromHistory(spectralRadii, upperRank)

	if err != nil {
		return fitGates{}, false
	}

	absAsymmetries := make([]float64, len(asymmetries))

	for index, asymmetry := range asymmetries {
		absAsymmetries[index] = math.Abs(asymmetry)
	}

	frenzyAsymmetry, err := quantileFromHistory(absAsymmetries, lowerRank)

	if err != nil {
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

func quantileFromHistory(history []float64, percentile float64) (float64, error) {
	if len(history) == 0 {
		return 0, errors.New("hawkes-gates: quantile requires history")
	}

	sorted := append([]float64(nil), history...)
	sort.Float64s(sorted)

	for _, value := range sorted {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, errors.New("hawkes-gates: quantile sample is non-finite")
		}
	}

	return stat.Quantile(percentile, stat.LinInterp, sorted, nil), nil
}
