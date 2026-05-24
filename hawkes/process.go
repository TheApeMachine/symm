package hawkes

import (
	"math"
	"time"

	"github.com/theapemachine/symm/stats"
)

type eventSide int

const (
	sideBuy eventSide = iota
	sideSell
)

/*
markedEvent is one trade arrival tagged by aggressor side.
*/
type markedEvent struct {
	at   time.Time
	side eventSide
}

/*
BivariateFit holds joint buy/sell Hawkes MLE parameters and horizon intensities.

	λ_buy(t)  = μ_buy  + Σ α_bb exp(-β(t-t_i)) + Σ α_bs exp(-β(t-t_j))
	λ_sell(t) = μ_sell + Σ α_sb exp(-β(t-t_i)) + Σ α_ss exp(-β(t-t_j))
*/
type BivariateFit struct {
	MuBuy          float64
	MuSell         float64
	AlphaBB        float64
	AlphaBS        float64
	AlphaSB        float64
	AlphaSS        float64
	Beta           float64
	BuyIntensity   float64
	SellIntensity  float64
	SpectralRadius float64
}

func (fit BivariateFit) valid() bool {
	return fit.MuBuy > 0 &&
		fit.MuSell > 0 &&
		fit.Beta > 0 &&
		fit.AlphaBB >= 0 &&
		fit.AlphaBS >= 0 &&
		fit.AlphaSB >= 0 &&
		fit.AlphaSS >= 0 &&
		fit.SpectralRadius > 0 &&
		fit.SpectralRadius < criticalBranch
}

func mergeMarkedEvents(buyEvents, sellEvents []time.Time) []markedEvent {
	marked := make([]markedEvent, 0, len(buyEvents)+len(sellEvents))

	for _, eventTime := range buyEvents {
		marked = append(marked, markedEvent{at: eventTime, side: sideBuy})
	}

	for _, eventTime := range sellEvents {
		marked = append(marked, markedEvent{at: eventTime, side: sideSell})
	}

	if len(marked) < 2 {
		return marked
	}

	for left := 1; left < len(marked); left++ {
		current := marked[left]
		insertAt := left

		for insertAt > 0 && marked[insertAt-1].at.After(current.at) {
			marked[insertAt] = marked[insertAt-1]
			insertAt--
		}

		marked[insertAt] = current
	}

	return marked
}

func windowSpan(marked []markedEvent, horizon time.Time) float64 {
	if len(marked) == 0 {
		return 0
	}

	span := horizon.Sub(marked[0].at).Seconds()

	if span <= 0 {
		return 0
	}

	return span
}

func buyIntensityAt(
	buyEvents, sellEvents []time.Time,
	at time.Time,
	muBuy, alphaBB, alphaBS, beta float64,
) float64 {
	lambda := muBuy

	for _, eventTime := range buyEvents {
		if !eventTime.Before(at) {
			continue
		}

		age := at.Sub(eventTime).Seconds()

		if age >= 0 {
			lambda += alphaBB * math.Exp(-beta*age)
		}
	}

	for _, eventTime := range sellEvents {
		if !eventTime.Before(at) {
			continue
		}

		age := at.Sub(eventTime).Seconds()

		if age >= 0 {
			lambda += alphaBS * math.Exp(-beta*age)
		}
	}

	return lambda
}

func sellIntensityAt(
	buyEvents, sellEvents []time.Time,
	at time.Time,
	muSell, alphaSB, alphaSS, beta float64,
) float64 {
	lambda := muSell

	for _, eventTime := range buyEvents {
		if !eventTime.Before(at) {
			continue
		}

		age := at.Sub(eventTime).Seconds()

		if age >= 0 {
			lambda += alphaSB * math.Exp(-beta*age)
		}
	}

	for _, eventTime := range sellEvents {
		if !eventTime.Before(at) {
			continue
		}

		age := at.Sub(eventTime).Seconds()

		if age >= 0 {
			lambda += alphaSS * math.Exp(-beta*age)
		}
	}

	return lambda
}

func kernelSupport(events []time.Time, horizon time.Time, beta float64) float64 {
	var support float64

	for _, eventTime := range events {
		remaining := horizon.Sub(eventTime).Seconds()

		if remaining > 0 {
			support += 1 - math.Exp(-beta*remaining)
		}
	}

	return support
}

func bivariateCompensator(
	buyEvents, sellEvents []time.Time,
	horizon time.Time,
	muBuy, muSell, alphaBB, alphaBS, alphaSB, alphaSS, beta, span float64,
) float64 {
	buySupport := kernelSupport(buyEvents, horizon, beta)
	sellSupport := kernelSupport(sellEvents, horizon, beta)

	buyIntegral := muBuy*span +
		(alphaBB/beta)*buySupport +
		(alphaBS/beta)*sellSupport
	sellIntegral := muSell*span +
		(alphaSB/beta)*buySupport +
		(alphaSS/beta)*sellSupport

	return buyIntegral + sellIntegral
}

func bivariateLogLikelihood(
	buyEvents, sellEvents []time.Time,
	horizon time.Time,
	muBuy, muSell, alphaBB, alphaBS, alphaSB, alphaSS, beta float64,
) float64 {
	if muBuy <= 0 || muSell <= 0 || beta <= 0 {
		return math.Inf(-1)
	}

	if alphaBB < 0 || alphaBS < 0 || alphaSB < 0 || alphaSS < 0 {
		return math.Inf(-1)
	}

	marked := mergeMarkedEvents(buyEvents, sellEvents)

	if len(marked) == 0 {
		return math.Inf(-1)
	}

	span := windowSpan(marked, horizon)

	if span <= 0 {
		return math.Inf(-1)
	}

	buySoFar := make([]time.Time, 0, len(buyEvents))
	sellSoFar := make([]time.Time, 0, len(sellEvents))
	var logSum float64

	for _, event := range marked {
		var lambda float64

		switch event.side {
		case sideBuy:
			lambda = buyIntensityAt(
				buySoFar, sellSoFar, event.at,
				muBuy, alphaBB, alphaBS, beta,
			)
			buySoFar = append(buySoFar, event.at)
		case sideSell:
			lambda = sellIntensityAt(
				buySoFar, sellSoFar, event.at,
				muSell, alphaSB, alphaSS, beta,
			)
			sellSoFar = append(sellSoFar, event.at)
		}

		if lambda <= 0 {
			return math.Inf(-1)
		}

		logSum += math.Log(lambda)
	}

	compensator := bivariateCompensator(
		buyEvents, sellEvents, horizon,
		muBuy, muSell, alphaBB, alphaBS, alphaSB, alphaSS, beta, span,
	)

	return logSum - compensator
}

func spectralRadius(alphaBB, alphaBS, alphaSB, alphaSS, beta float64) float64 {
	if beta <= 0 {
		return math.Inf(1)
	}

	branchBB := alphaBB / beta
	branchBS := alphaBS / beta
	branchSB := alphaSB / beta
	branchSS := alphaSS / beta
	trace := branchBB + branchSS
	determinant := branchBB*branchSS - branchBS*branchSB
	discriminant := trace*trace - 4*determinant

	if discriminant < 0 {
		modulus := math.Sqrt(-discriminant)
		realPart := trace / 2
		imagPart := modulus / 2

		return math.Sqrt(realPart*realPart + imagPart*imagPart)
	}

	rootHigh := (trace + math.Sqrt(discriminant)) / 2
	rootLow := (trace - math.Sqrt(discriminant)) / 2

	return math.Max(math.Abs(rootHigh), math.Abs(rootLow))
}

func evaluateBivariateFit(
	buyEvents, sellEvents []time.Time,
	horizon time.Time,
	context FitContext,
	muBuy, muSell, alphaBB, alphaBS, alphaSB, alphaSS, beta float64,
) BivariateFit {
	spectral := spectralRadius(alphaBB, alphaBS, alphaSB, alphaSS, beta)

	if spectral >= criticalBranch || spectral <= context.BranchFloor {
		return BivariateFit{}
	}

	logLikelihood := bivariateLogLikelihood(
		buyEvents, sellEvents, horizon,
		muBuy, muSell, alphaBB, alphaBS, alphaSB, alphaSS, beta,
	)

	if logLikelihood <= math.Inf(-1) {
		return BivariateFit{}
	}

	return BivariateFit{
		MuBuy:   muBuy,
		MuSell:  muSell,
		AlphaBB: alphaBB,
		AlphaBS: alphaBS,
		AlphaSB: alphaSB,
		AlphaSS: alphaSS,
		Beta:    beta,
		BuyIntensity: buyIntensityAt(
			buyEvents, sellEvents, horizon,
			muBuy, alphaBB, alphaBS, beta,
		),
		SellIntensity: sellIntensityAt(
			buyEvents, sellEvents, horizon,
			muSell, alphaSB, alphaSS, beta,
		),
		SpectralRadius: spectral,
	}
}

func sellBuyAsymmetry(fit BivariateFit) float64 {
	total := fit.BuyIntensity + fit.SellIntensity

	if total <= 0 || fit.SellIntensity <= fit.BuyIntensity {
		return 0
	}

	return (fit.SellIntensity - fit.BuyIntensity) / total
}

func buySellAsymmetry(fit BivariateFit) float64 {
	total := fit.BuyIntensity + fit.SellIntensity

	if total <= 0 || fit.BuyIntensity <= fit.SellIntensity {
		return 0
	}

	return (fit.BuyIntensity - fit.SellIntensity) / total
}

/*
excitationRunway is the fitted kernel e-folding time: 1/β seconds until
self-excitation has decayed to 1/e of an impulse.
*/
func excitationRunway(fit BivariateFit) time.Duration {
	if fit.Beta <= 0 {
		return 0
	}

	return time.Duration((1 / fit.Beta) * float64(time.Second))
}

func excitationConfidence(
	fit BivariateFit,
	asymmetry float64,
	baselineFence float64,
	sellSide bool,
) float64 {
	if asymmetry <= 0 || fit.SpectralRadius >= criticalBranch {
		return 0
	}

	if sellSide {
		if fit.MuSell <= 0 || fit.SellIntensity <= 0 {
			return 0
		}

		ratio := fit.SellIntensity / fit.MuSell

		if ratio <= baselineFence {
			return 0
		}

		return asymmetry * ratio
	}

	if fit.MuBuy <= 0 || fit.BuyIntensity <= 0 {
		return 0
	}

	ratio := fit.BuyIntensity / fit.MuBuy

	if ratio <= baselineFence {
		return 0
	}

	return asymmetry * ratio
}

func excitationConfidenceLegacy(fit BivariateFit, asymmetry float64, baselineFence float64) float64 {
	return excitationConfidence(fit, asymmetry, baselineFence, false)
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

	return stats.Median(gaps)
}

func mergedMedianGap(buyEvents, sellEvents []time.Time) float64 {
	marked := mergeMarkedEvents(buyEvents, sellEvents)

	if len(marked) < 2 {
		return 0
	}

	times := make([]time.Time, len(marked))

	for index, event := range marked {
		times[index] = event.at
	}

	return medianInterArrivalSec(times)
}

func confidenceFence(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	lower, upper := stats.Quartiles(values)
	spread := upper - lower

	if spread > 0 {
		return upper + spread + spread/2
	}

	return stats.Max(values)
}

func crossSectionMedian(values []float64) float64 {
	return stats.CrossSectionMedian(values)
}
