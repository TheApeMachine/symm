package statistic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolAlphaAA          = types.MustIntern("alpha_aa")
	SymbolAlphaAB          = types.MustIntern("alpha_ab")
	SymbolAlphaBA          = types.MustIntern("alpha_ba")
	SymbolAlphaBB          = types.MustIntern("alpha_bb")
	SymbolBeta             = types.MustIntern("beta")
	SymbolSpectralRadius   = types.MustIntern("spectral_radius")
	SymbolOffspringAA      = types.MustIntern("offspring_aa")
	SymbolOffspringAB      = types.MustIntern("offspring_ab")
	SymbolOffspringBA      = types.MustIntern("offspring_ba")
	SymbolOffspringBB      = types.MustIntern("offspring_bb")
	SymbolDescendantsAlpha = types.MustIntern("descendants_alpha")
	SymbolDescendantsBeta  = types.MustIntern("descendants_beta")
)

/*
Branching calculates a bivariate branching matrix, spectral radius, immediate
offspring, and total descendants.
*/
func Branching(input types.Frame) types.Frame {
	beta, found := input.Get(SymbolBeta)

	if !found || beta <= 0 || math.IsNaN(beta) || math.IsInf(beta, 0) {
		input.Err = fmt.Errorf(
			"statistic: branching requires positive finite beta",
		)

		return input
	}

	alphaAA := inputValue(input, SymbolAlphaAA) / beta
	alphaAB := inputValue(input, SymbolAlphaAB) / beta
	alphaBA := inputValue(input, SymbolAlphaBA) / beta
	alphaBB := inputValue(input, SymbolAlphaBB) / beta

	if !finite(alphaAA, alphaAB, alphaBA, alphaBB) {
		input.Err = fmt.Errorf(
			"statistic: branching coefficients must be finite",
		)

		return input
	}

	trace := alphaAA + alphaBB
	determinant := alphaAA*alphaBB - alphaAB*alphaBA
	discriminant := trace*trace - 4*determinant

	if discriminant < 0 {
		discriminant = 0
	}

	spectralRadius := (trace + math.Sqrt(discriminant)) / 2
	determinantIdentity := (1-alphaAA)*(1-alphaBB) - alphaAB*alphaBA
	descendantsAlpha := 1.0
	descendantsBeta := 1.0

	if determinantIdentity > 1e-9 {
		descendantsAlpha = (1 - alphaBB + alphaAB) / determinantIdentity
		descendantsBeta = (alphaBA + 1 - alphaAA) / determinantIdentity
	}

	input.Put(SymbolSpectralRadius, spectralRadius)
	input.Put(SymbolOffspringAA, alphaAA)
	input.Put(SymbolOffspringAB, alphaAB)
	input.Put(SymbolOffspringBA, alphaBA)
	input.Put(SymbolOffspringBB, alphaBB)
	input.Put(SymbolDescendantsAlpha, descendantsAlpha)
	input.Put(SymbolDescendantsBeta, descendantsBeta)

	return input
}

func inputValue(input types.Frame, symbol types.Symbol) float64 {
	value, _ := input.Get(symbol)

	return value
}

func finite(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}

	return true
}
