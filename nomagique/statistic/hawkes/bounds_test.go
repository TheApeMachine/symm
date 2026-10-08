package hawkes

import (
	"math"
	"testing"
)

func TestSoftplusInverseRoundTrips(testingT *testing.T) {
	for _, value := range []float64{0.001, 0.1, 1, 5, 25, 100} {
		free := inverseSoftplus(value)
		recovered := softplus(free)

		if math.Abs(recovered-value) > 1e-6*math.Max(1, value) {
			testingT.Fatalf("softplus(inverseSoftplus(%v)) = %v, want %v", value, recovered, value)
		}
	}
}

func TestSoftplusInversePanicsOnNonPositive(testingT *testing.T) {
	defer func() {
		if recover() == nil {
			testingT.Fatal("expected inverseSoftplus to panic on a non-positive value")
		}
	}()

	inverseSoftplus(0)
}

func TestSoftplusDerivativeMatchesFiniteDifference(testingT *testing.T) {
	const step = 1e-6

	for _, free := range []float64{-5, -1, 0, 1, 5, 25} {
		analytic := softplusDerivative(free)
		numeric := (softplus(free+step) - softplus(free-step)) / (2 * step)

		if math.Abs(analytic-numeric) > 1e-4*math.Max(1, math.Abs(numeric)) {
			testingT.Fatalf(
				"softplusDerivative(%v) = %v, finite-difference = %v",
				free, analytic, numeric,
			)
		}
	}
}

func TestLogParamBoundsEncodeDecodeStaysInBounds(testingT *testing.T) {
	context := fitContext{
		spanSec:        10,
		totalEvents:    20,
		eventsX:        10,
		eventsY:        10,
		medianGapSec:   0.5,
		betaCandidates: []float64{0.5, 1, 2},
		branchFloor:    0.01,
		branchCeiling:  0.9,
		localScales:    []float64{0.5},
	}
	bounds, err := context.logParamBounds()

	if err != nil {
		testingT.Fatalf("logParamBounds: %v", err)
	}

	free := bounds.encode([bivariateParamCount]float64{0, 0, 0, -2, -3, -3, -2})
	decoded := bounds.decode(free)

	for index := range decoded {
		if decoded[index] < bounds.lower[index] || decoded[index] > bounds.upper[index] {
			testingT.Fatalf(
				"decoded[%d] = %v outside bounds [%v, %v]",
				index, decoded[index], bounds.lower[index], bounds.upper[index],
			)
		}
	}
}

func TestLogParamBoundsEncodeDecodeRoundTripsWithinBounds(testingT *testing.T) {
	context := fitContext{
		spanSec:        10,
		totalEvents:    20,
		eventsX:        10,
		eventsY:        10,
		medianGapSec:   0.5,
		betaCandidates: []float64{0.5, 1, 2},
		branchFloor:    0.01,
		branchCeiling:  0.9,
		localScales:    []float64{0.5},
	}
	bounds, err := context.logParamBounds()

	if err != nil {
		testingT.Fatalf("logParamBounds: %v", err)
	}

	midpoint := [bivariateParamCount]float64{}

	for index := range midpoint {
		midpoint[index] = (bounds.lower[index] + bounds.upper[index]) / 2
	}

	free := bounds.encode(midpoint)
	decoded := bounds.decode(free)

	for index := range midpoint {
		if math.Abs(decoded[index]-midpoint[index]) > 1e-6 {
			testingT.Fatalf(
				"encode/decode round-trip mismatch at index %d: got %v, want %v",
				index, decoded[index], midpoint[index],
			)
		}
	}
}

func TestSoftplusJacobianMatchesFiniteDifference(testingT *testing.T) {
	context := fitContext{
		spanSec:        10,
		totalEvents:    20,
		eventsX:        10,
		eventsY:        10,
		medianGapSec:   0.5,
		betaCandidates: []float64{0.5, 1, 2},
		branchFloor:    0.01,
		branchCeiling:  0.9,
		localScales:    []float64{0.5},
	}
	bounds, err := context.logParamBounds()

	if err != nil {
		testingT.Fatalf("logParamBounds: %v", err)
	}

	free := bounds.encode([bivariateParamCount]float64{0, 0, 0, -2, -3, -3, -2})
	jacobian := bounds.softplusJacobian(free)
	const step = 1e-6

	for index := range free {
		perturbedUp := append([]float64(nil), free...)
		perturbedDown := append([]float64(nil), free...)
		perturbedUp[index] += step
		perturbedDown[index] -= step

		decodedUp := bounds.decode(perturbedUp)
		decodedDown := bounds.decode(perturbedDown)
		numeric := (decodedUp[index] - decodedDown[index]) / (2 * step)

		if math.Abs(jacobian[index]-numeric) > 1e-4*math.Max(1, math.Abs(numeric)) {
			testingT.Fatalf(
				"softplusJacobian[%d] = %v, finite-difference = %v",
				index, jacobian[index], numeric,
			)
		}
	}
}
