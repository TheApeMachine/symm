package learning

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
DirectionalTarget composes a finite nonnegative deadband and the sign of a delta.
*/
func DirectionalTarget(deadband float64) types.Value[[2]float64, float64] {
	return func(sample [2]float64) float64 {
		delta := sample[0] - sample[1]
		if math.Abs(delta) > deadband {
			return math.Copysign(1, delta)
		}
		return 0.0
	}
}

/*
BinaryTarget classifies an increase without inventing a new numeric rule.
*/
func BinaryTarget() types.Value[[2]float64, float64] {
	return func(sample [2]float64) float64 {
		if sample[0] > sample[1] {
			return 1.0
		}
		return 0.0
	}
}

/*
IdentityTarget selects the finite current value.
*/
func IdentityTarget() types.Value[[2]float64, float64] {
	return func(sample [2]float64) float64 {
		return sample[0]
	}
}

/*
DeltaTarget returns the observed current-minus-past difference.
*/
func DeltaTarget() types.Value[[2]float64, float64] {
	return func(sample [2]float64) float64 {
		return sample[0] - sample[1]
	}
}

/*
RatioTarget is the relative change, with an explicit nonzero past domain.
*/
func RatioTarget() types.Value[[2]float64, float64] {
	return func(sample [2]float64) float64 {
		if sample[1] == 0 {
			return math.NaN()
		}
		return sample[0]/sample[1] - 1
	}
}
