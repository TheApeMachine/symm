package statistic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolAlphaAA          = nomagique.MustIntern("alpha_aa")
	SymbolAlphaAB          = nomagique.MustIntern("alpha_ab")
	SymbolAlphaBA          = nomagique.MustIntern("alpha_ba")
	SymbolAlphaBB          = nomagique.MustIntern("alpha_bb")
	SymbolBeta             = nomagique.MustIntern("beta")
	SymbolSpectralRadius   = nomagique.MustIntern("spectral_radius")
	SymbolOffspringAA      = nomagique.MustIntern("offspring_aa")
	SymbolOffspringAB      = nomagique.MustIntern("offspring_ab")
	SymbolOffspringBA      = nomagique.MustIntern("offspring_ba")
	SymbolOffspringBB      = nomagique.MustIntern("offspring_bb")
	SymbolDescendantsAlpha = nomagique.MustIntern("descendants_alpha")
	SymbolDescendantsBeta  = nomagique.MustIntern("descendants_beta")
)

/*
Branching calculates a bivariate branching matrix, spectral radius, immediate
offspring, and total descendants.
*/
func Branching(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	beta, found := input.Get(SymbolBeta)

	if !found || beta <= 0 || math.IsNaN(beta) || math.IsInf(beta, 0) {
		return state, types.Frame{}, fmt.Errorf(
			"statistic: branching requires positive finite beta",
		)
	}

	alphaAA := inputValue(input, SymbolAlphaAA) / beta
	alphaAB := inputValue(input, SymbolAlphaAB) / beta
	alphaBA := inputValue(input, SymbolAlphaBA) / beta
	alphaBB := inputValue(input, SymbolAlphaBB) / beta

	if !finite(alphaAA, alphaAB, alphaBA, alphaBB) {
		return state, types.Frame{}, fmt.Errorf(
			"statistic: branching coefficients must be finite",
		)
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

	output := input
	output.Put(SymbolSpectralRadius, spectralRadius)
	output.Put(SymbolOffspringAA, alphaAA)
	output.Put(SymbolOffspringAB, alphaAB)
	output.Put(SymbolOffspringBA, alphaBA)
	output.Put(SymbolOffspringBB, alphaBB)
	output.Put(SymbolDescendantsAlpha, descendantsAlpha)
	output.Put(SymbolDescendantsBeta, descendantsBeta)

	return state, output, nil
}

func inputValue(input types.Frame, symbol nomagique.Symbol) float64 {
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
