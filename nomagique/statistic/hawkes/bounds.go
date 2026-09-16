package hawkes

import (
	"fmt"
	"math"
)

const (
	softplusLinearAt = 20
	paramRatioFloor  = 1e-9
)

/*
logParamBounds owns the reversible transform between unconstrained optimizer
coordinates and data-derived parameter bounds. Keeping this responsibility
separate makes the likelihood optimizer operate only on valid model domains.
*/
type logParamBounds struct {
	lower [bivariateParamCount]float64
	upper [bivariateParamCount]float64
}

func (fitContext fitContext) logParamBounds() (logParamBounds, error) {
	betaMin := fitContext.betaCandidates[0]
	betaMax := fitContext.betaCandidates[len(fitContext.betaCandidates)-1]
	selfMax := fitContext.branchCeiling * selfBranchShareFromContext(fitContext)
	crossMax := fitContext.branchCeiling
	crossMin, err := crossBranchFloorFromContext(fitContext)

	if err != nil {
		return logParamBounds{}, err
	}

	if !(fitContext.spanSec > 0) {
		return logParamBounds{}, fmt.Errorf("hawkes: log param bounds require positive span")
	}

	minRate := 1 / fitContext.spanSec
	maxRate := float64(fitContext.totalEvents) / fitContext.spanSec

	return logParamBounds{
		lower: [bivariateParamCount]float64{
			logPositive(minRate),
			logPositive(minRate),
			math.Log(betaMin),
			logPositive(fitContext.branchFloor),
			logPositive(crossMin),
			logPositive(crossMin),
			logPositive(fitContext.branchFloor),
		},
		upper: [bivariateParamCount]float64{
			logPositive(maxRate),
			logPositive(maxRate),
			math.Log(betaMax),
			math.Log(selfMax),
			math.Log(crossMax),
			math.Log(crossMax),
			math.Log(selfMax),
		},
	}, nil
}

func (logParamBounds logParamBounds) decode(free []float64) [bivariateParamCount]float64 {
	params := [bivariateParamCount]float64{}

	for index := range free {
		span := logParamBounds.upper[index] - logParamBounds.lower[index]

		if span <= 0 {
			params[index] = logParamBounds.lower[index]
			continue
		}

		lift := softplus(free[index])
		params[index] = logParamBounds.lower[index] + span*lift/(1+lift)
	}

	return params
}

func (logParamBounds logParamBounds) encode(params [bivariateParamCount]float64) []float64 {
	free := make([]float64, bivariateParamCount)

	for index := range params {
		span := logParamBounds.upper[index] - logParamBounds.lower[index]

		if span <= 0 {
			continue
		}

		ratio := (params[index] - logParamBounds.lower[index]) / span
		ratio = math.Max(paramRatioFloor, math.Min(1-paramRatioFloor, ratio))
		free[index] = inverseSoftplus(ratio / (1 - ratio))
	}

	return free
}

func (logParamBounds logParamBounds) softplusJacobian(free []float64) [bivariateParamCount]float64 {
	jacobian := [bivariateParamCount]float64{}

	for index := range free {
		span := logParamBounds.upper[index] - logParamBounds.lower[index]

		if span <= 0 {
			continue
		}

		lift := softplus(free[index])
		jacobian[index] = span * softplusDerivative(free[index]) / ((1 + lift) * (1 + lift))
	}

	return jacobian
}

/*
crossBranchFloorFromContext derives the smallest cross-excitation branch the
optimizer is allowed to consider: a machine-epsilon-scaled fraction of the
data-derived branch floor, or (when that floor is degenerate) of the
observation's own span and event mass.
*/
func crossBranchFloorFromContext(context fitContext) (float64, error) {
	radicand := math.Nextafter(1, 2) - 1
	scale := math.Max(1, math.Abs(radicand))
	tolerance := 32 * radicand * scale

	if radicand < -tolerance {
		return 0, fmt.Errorf("hawkes: machine-epsilon radicand is negative beyond tolerance")
	}

	machineSqrt := math.Sqrt(math.Max(0, radicand))

	if context.branchFloor > 0 {
		return context.branchFloor * machineSqrt, nil
	}

	if !(context.spanSec > 0) || context.totalEvents <= 0 {
		return 0, fmt.Errorf("hawkes: cross-branch floor requires positive span and event mass")
	}

	return 1 / context.spanSec / float64(context.totalEvents), nil
}

func selfBranchShareFromContext(context fitContext) float64 {
	return arrivalTune{
		totalEvents: context.totalEvents,
		eventsX:     context.eventsX,
		eventsY:     context.eventsY,
	}.selfBranchShare()
}

func softplus(value float64) float64 {
	if value > softplusLinearAt {
		return value
	}

	argument := math.Exp(value)

	return math.Log1p(argument)
}

func inverseSoftplus(value float64) float64 {
	if value > softplusLinearAt {
		return value
	}

	if value <= 0 {
		panic("hawkes: inverseSoftplus argument must be strictly positive")
	}

	expm1 := math.Expm1(value)

	if expm1 <= 0 {
		panic("hawkes: inverseSoftplus log argument must be strictly positive")
	}

	return math.Log(expm1)
}

func softplusDerivative(value float64) float64 {
	if value > softplusLinearAt {
		return 1
	}

	if value < -softplusLinearAt {
		return math.Exp(value)
	}

	return 1 / (1 + math.Exp(-value))
}
