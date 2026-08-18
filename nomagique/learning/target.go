package learning

import "math"

/*
TargetTransform defines how a current reference value and a past reference value
are converted into a supervised training target y.
*/
type TargetTransform func(current float64, past float64) (float64, bool)

/*
DirectionalTarget classifies whether the reference signal moved up (+1) or down (-1).
If the absolute change is less than the deadband threshold, it returns 0.
*/
func DirectionalTarget(deadband float64) TargetTransform {
	return func(current, past float64) (float64, bool) {
		if !finite(current) || !finite(past) {
			return 0, false
		}
		diff := current - past
		if math.Abs(diff) <= deadband {
			return 0, true
		}
		if diff > 0 {
			return 1.0, true
		}
		return -1.0, true
	}
}

/*
BinaryTarget returns 1.0 if the reference signal increased, otherwise 0.0.
*/
func BinaryTarget() TargetTransform {
	return func(current, past float64) (float64, bool) {
		if !finite(current) || !finite(past) {
			return 0, false
		}
		if current > past {
			return 1.0, true
		}
		return 0.0, true
	}
}

/*
DeltaTarget returns the continuous difference (current - past).
*/
func DeltaTarget() TargetTransform {
	return func(current, past float64) (float64, bool) {
		if !finite(current) || !finite(past) {
			return 0, false
		}
		return current - past, true
	}
}

/*
RatioTarget returns relative change (current / past - 1.0).
*/
func RatioTarget() TargetTransform {
	return func(current, past float64) (float64, bool) {
		if !finite(current) || !finite(past) || past == 0 {
			return 0, false
		}
		return (current / past) - 1.0, true
	}
}
