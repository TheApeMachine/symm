package hawkes

import (
	"math"
	"time"
)

const (
	minFitEvents     = 8
	betaScanSteps    = 7
	criticalBranch   = 1.0
	localBetaSteps   = 5
	localMuSteps     = 5
	localBranchSteps = 5
	localScaleMin    = 0.6
	localScaleMax    = 1.4
	localBranchDelta = 0.08
	minBranching     = 0.05
)

/*
SideFit holds rolling MLE parameters for one trade side.
*/
type SideFit struct {
	mu        float64
	alpha     float64
	beta      float64
	branching float64
	intensity float64
}

/*
fitSide estimates mu, alpha, beta via warm-started local likelihood search.
*/
func fitSide(events []time.Time, horizon time.Time) SideFit {
	return fitSideWithPrior(events, horizon, SideFit{})
}

/*
fitSideWithPrior warm-starts from prior parameters and scans a local neighborhood.
*/
func fitSideWithPrior(events []time.Time, horizon time.Time, prior SideFit) SideFit {
	if len(events) < minFitEvents {
		return SideFit{}
	}

	medianGap := medianInterArrivalSec(events)

	if medianGap <= 0 {
		return SideFit{}
	}

	baseBeta := 1 / medianGap
	span := horizon.Sub(events[0]).Seconds()

	if span <= 0 {
		return SideFit{}
	}

	muStart := float64(len(events)) / span

	if priorValid(prior) {
		local := scanLocalGrid(events, horizon, prior, muStart, baseBeta)

		if local.mu > 0 {
			return local
		}
	}

	return scanFullGrid(events, horizon, muStart, baseBeta)
}

func priorValid(prior SideFit) bool {
	return prior.mu > 0 &&
		prior.beta > 0 &&
		prior.branching > 0 &&
		prior.branching < criticalBranch
}

func scanLocalGrid(
	events []time.Time,
	horizon time.Time,
	prior SideFit,
	muStart, baseBeta float64,
) SideFit {
	best := SideFit{mu: -1}
	bestLL := math.Inf(-1)

	betaScales := localScales(localBetaSteps)
	muScales := localScales(localMuSteps)
	branchOffsets := localBranchOffsets(localBranchSteps)

	for _, betaScale := range betaScales {
		beta := prior.beta * betaScale

		if beta <= 0 {
			continue
		}

		for _, muScale := range muScales {
			mu := prior.mu * muScale

			if mu <= 0 {
				continue
			}

			for _, branchOffset := range branchOffsets {
				branching := prior.branching + branchOffset

				if branching <= minBranching || branching >= criticalBranch {
					continue
				}

				alpha := branching * beta
				candidate := evaluateFit(events, horizon, mu, alpha, beta, branching)

				if candidate.fit.mu <= 0 || candidate.logLikelihood <= bestLL {
					continue
				}

				bestLL = candidate.logLikelihood
				best = candidate.fit
			}
		}
	}

	if best.mu > 0 {
		return best
	}

	return scanFullGrid(events, horizon, muStart, baseBeta)
}

func scanFullGrid(events []time.Time, horizon time.Time, muStart, baseBeta float64) SideFit {
	best := SideFit{mu: -1}
	bestLL := math.Inf(-1)

	for step := 0; step < betaScanSteps; step++ {
		betaScale := 0.25 + float64(step)*0.25
		beta := baseBeta * betaScale

		for muFactor := 0.2; muFactor <= 2.0; muFactor += 0.2 {
			mu := muStart * muFactor

			for branchStep := 1; branchStep < 14; branchStep++ {
				branching := float64(branchStep) / 15
				alpha := branching * beta
				candidate := evaluateFit(events, horizon, mu, alpha, beta, branching)

				if candidate.fit.mu <= 0 || candidate.logLikelihood <= bestLL {
					continue
				}

				bestLL = candidate.logLikelihood
				best = candidate.fit
			}
		}
	}

	return best
}

type fitCandidate struct {
	fit           SideFit
	logLikelihood float64
}

func evaluateFit(
	events []time.Time,
	horizon time.Time,
	mu, alpha, beta, branching float64,
) fitCandidate {
	logLikelihood := logLikelihood(events, horizon, mu, alpha, beta)

	if logLikelihood <= math.Inf(-1) {
		return fitCandidate{}
	}

	return fitCandidate{
		logLikelihood: logLikelihood,
		fit: SideFit{
			mu:        mu,
			alpha:     alpha,
			beta:      beta,
			branching: branching,
			intensity: intensityAt(events, horizon, mu, alpha, beta),
		},
	}
}

func localScales(steps int) []float64 {
	if steps <= 1 {
		return []float64{1}
	}

	scales := make([]float64, steps)
	stepSize := (localScaleMax - localScaleMin) / float64(steps-1)

	for index := 0; index < steps; index++ {
		scales[index] = localScaleMin + float64(index)*stepSize
	}

	return scales
}

func localBranchOffsets(steps int) []float64 {
	if steps <= 1 {
		return []float64{0}
	}

	center := (steps - 1) / 2
	offsets := make([]float64, steps)

	for index := 0; index < steps; index++ {
		offsets[index] = float64(index-center) * localBranchDelta
	}

	return offsets
}

func logLikelihood(events []time.Time, horizon time.Time, mu, alpha, beta float64) float64 {
	if len(events) == 0 || mu <= 0 || beta <= 0 || alpha < 0 {
		return math.Inf(-1)
	}

	span := horizon.Sub(events[0]).Seconds()

	if span <= 0 {
		return math.Inf(-1)
	}

	var logSum float64

	for eventIndex, eventTime := range events {
		lambda := intensityAt(events[:eventIndex], eventTime, mu, alpha, beta)

		if lambda <= 0 {
			return math.Inf(-1)
		}

		logSum += math.Log(lambda)
	}

	var support float64

	for _, eventTime := range events {
		remaining := horizon.Sub(eventTime).Seconds()

		if remaining > 0 {
			support += 1 - math.Exp(-beta*remaining)
		}
	}

	return logSum - mu*span - (alpha/beta)*support
}

func intensityAt(events []time.Time, at time.Time, mu, alpha, beta float64) float64 {
	lambda := mu

	for _, eventTime := range events {
		if !eventTime.Before(at) {
			continue
		}

		age := at.Sub(eventTime).Seconds()

		if age >= 0 {
			lambda += alpha * math.Exp(-beta*age)
		}
	}

	return lambda
}

func buySellAsymmetry(buyFit, sellFit SideFit) float64 {
	total := buyFit.intensity + sellFit.intensity

	if total <= 0 || buyFit.intensity <= sellFit.intensity {
		return 0
	}

	return (buyFit.intensity - sellFit.intensity) / total
}

func excitationConfidence(buyFit SideFit, asymmetry float64) float64 {
	if asymmetry <= 0 || buyFit.mu <= 0 || buyFit.intensity <= 0 {
		return 0
	}

	if buyFit.branching >= criticalBranch {
		return 0
	}

	ratio := buyFit.intensity / buyFit.mu

	if ratio <= 1 {
		return 0
	}

	return asymmetry * ratio
}

func medianInterArrivalSec(events []time.Time) float64 {
	if len(events) < 2 {
		return 0
	}

	gaps := make([]float64, 0, len(events)-1)

	for index := 1; index < len(events); index++ {
		gap := events[index].Sub(events[index-1]).Seconds()

		if gap > 0 {
			gaps = append(gaps, gap)
		}
	}

	if len(gaps) == 0 {
		return 0
	}

	return medianFloat(gaps)
}

func medianFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	cp := append([]float64(nil), values...)
	sortFloats(cp)

	return percentileSorted(cp, 0.5)
}

func percentileSorted(sorted []float64, quantile float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	if quantile <= 0 {
		return sorted[0]
	}

	if quantile >= 1 {
		return sorted[len(sorted)-1]
	}

	position := quantile * float64(len(sorted)-1)
	lowerIndex := int(math.Floor(position))
	upperIndex := int(math.Ceil(position))
	weight := position - float64(lowerIndex)

	return sorted[lowerIndex]*(1-weight) + sorted[upperIndex]*weight
}

func sortFloats(values []float64) {
	for index := 1; index < len(values); index++ {
		for inner := index; inner > 0 && values[inner] < values[inner-1]; inner-- {
			values[inner], values[inner-1] = values[inner-1], values[inner]
		}
	}
}

func confidenceFence(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	lower, upper := quartiles(values)
	spread := upper - lower

	if spread > 0 {
		return upper + spread + spread/2
	}

	return maxFloat(values)
}

func quartiles(values []float64) (lower, upper float64) {
	if len(values) == 0 {
		return 0, 0
	}

	cp := append([]float64(nil), values...)
	sortFloats(cp)

	return percentileSorted(cp, 0.25), percentileSorted(cp, 0.75)
}

func maxFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	peak := values[0]

	for _, value := range values[1:] {
		if value > peak {
			peak = value
		}
	}

	return peak
}

func crossSectionMedian(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	cp := append([]float64(nil), values...)
	sortFloats(cp)

	return percentileSorted(cp, 0.5)
}
