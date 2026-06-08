package numeric

import "math"

/*
ClassifierWeights holds softmax logits coefficients for a four-way classifier.
*/
type ClassifierWeights struct {
	Threshold float64
	WIgnVol   float64
	WIgnPrec  float64
	WCoilComp float64
	WCoilPrec float64
	WOrgPrec  float64
	WOrgComp  float64
	WOrgVol   float64
	WExVol    float64
	WExPrec   float64
}

func DefaultClassifierWeights(threshold float64) ClassifierWeights {
	return ClassifierWeights{
		Threshold: threshold,
		WIgnVol:   0.6,
		WIgnPrec:  0.4,
		WCoilComp: 0.7,
		WCoilPrec: 0.3,
		WOrgPrec:  0.5,
		WOrgComp:  0.3,
		WOrgVol:   0.2,
		WExVol:    0.5,
		WExPrec:   0.5,
	}
}

func (weights *ClassifierWeights) Scores(
	rvol, precursor, compression float64,
) []float64 {
	return []float64{
		rvol*weights.WIgnVol + precursor*weights.WIgnPrec,
		compression*weights.WCoilComp + (1.0-precursor)*weights.WCoilPrec,
		precursor*weights.WOrgPrec + (1.0-compression)*weights.WOrgComp + rvol*weights.WOrgVol,
		(1.0-rvol)*weights.WExVol + (1.0-precursor)*weights.WExPrec,
	}
}

func (weights *ClassifierWeights) Strength(rvol, precursor float64) float64 {
	return rvol*weights.WIgnVol + precursor*weights.WIgnPrec
}

func (weights *ClassifierWeights) clamp() {
	weights.WIgnVol = clamp(weights.WIgnVol, 0.1, 1.5)
	weights.WIgnPrec = clamp(weights.WIgnPrec, 0.1, 1.5)
	weights.WCoilComp = clamp(weights.WCoilComp, 0.1, 1.5)
	weights.WCoilPrec = clamp(weights.WCoilPrec, 0.1, 1.5)
	weights.WOrgPrec = clamp(weights.WOrgPrec, 0.1, 1.5)
	weights.WOrgComp = clamp(weights.WOrgComp, 0.1, 1.5)
	weights.WOrgVol = clamp(weights.WOrgVol, 0.1, 1.5)
	weights.WExVol = clamp(weights.WExVol, 0.1, 1.5)
	weights.WExPrec = clamp(weights.WExPrec, 0.1, 1.5)
}

func clamp(value, lower, upper float64) float64 {
	return math.Min(math.Max(value, lower), upper)
}
