package hawkes

import (
	"math"
	"time"
)

const criticalBranch = 1.0

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

func (fit BivariateFit) computeSpectralRadius() float64 {
	if fit.Beta <= 0 {
		return math.Inf(1)
	}

	branchBB := fit.AlphaBB / fit.Beta
	branchBS := fit.AlphaBS / fit.Beta
	branchSB := fit.AlphaSB / fit.Beta
	branchSS := fit.AlphaSS / fit.Beta
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

func (fit BivariateFit) LogLikelihood(stream ArrivalStream, horizon time.Time) float64 {
	if fit.MuBuy <= 0 || fit.MuSell <= 0 || fit.Beta <= 0 {
		return math.Inf(-1)
	}

	if fit.AlphaBB < 0 || fit.AlphaBS < 0 || fit.AlphaSB < 0 || fit.AlphaSS < 0 {
		return math.Inf(-1)
	}

	marked := stream.Marked()

	if len(marked) == 0 {
		return math.Inf(-1)
	}

	span := stream.Span(horizon)

	if span <= 0 {
		return math.Inf(-1)
	}

	state := ExcitationState{}
	logSum, ok := state.LogLikelihoodSum(
		marked,
		fit.MuBuy, fit.MuSell,
		fit.AlphaBB, fit.AlphaBS, fit.AlphaSB, fit.AlphaSS,
		fit.Beta,
	)

	if !ok {
		return math.Inf(-1)
	}

	compensator := fit.compensator(stream, horizon, span)

	return logSum - compensator
}

func (fit BivariateFit) WithIntensitiesAt(stream ArrivalStream, horizon time.Time) BivariateFit {
	result := fit
	result.BuyIntensity = stream.buyIntensityAt(
		horizon, fit.MuBuy, fit.AlphaBB, fit.AlphaBS, fit.Beta,
	)
	result.SellIntensity = stream.sellIntensityAt(
		horizon, fit.MuSell, fit.AlphaSB, fit.AlphaSS, fit.Beta,
	)

	return result
}

func (fit BivariateFit) Evaluate(
	stream ArrivalStream,
	horizon time.Time,
	context FitContext,
	muBuy, muSell, alphaBB, alphaBS, alphaSB, alphaSS, beta float64,
) BivariateFit {
	candidate := BivariateFit{
		MuBuy:   muBuy,
		MuSell:  muSell,
		AlphaBB: alphaBB,
		AlphaBS: alphaBS,
		AlphaSB: alphaSB,
		AlphaSS: alphaSS,
		Beta:    beta,
	}
	candidate.SpectralRadius = candidate.computeSpectralRadius()

	if candidate.SpectralRadius >= criticalBranch ||
		candidate.SpectralRadius <= context.BranchFloor {
		return BivariateFit{}
	}

	if candidate.LogLikelihood(stream, horizon) <= math.Inf(-1) {
		return BivariateFit{}
	}

	return candidate.WithIntensitiesAt(stream, horizon)
}

func (fit BivariateFit) Asymmetry(sellSide bool) float64 {
	total := fit.BuyIntensity + fit.SellIntensity

	if total <= 0 {
		return 0
	}

	if sellSide {
		if fit.SellIntensity <= fit.BuyIntensity {
			return 0
		}

		return (fit.SellIntensity - fit.BuyIntensity) / total
	}

	if fit.BuyIntensity <= fit.SellIntensity {
		return 0
	}

	return (fit.BuyIntensity - fit.SellIntensity) / total
}

/*
Runway is the fitted kernel e-folding time: 1/β seconds until self-excitation
has decayed to 1/e of an impulse.
*/
func (fit BivariateFit) Runway() time.Duration {
	if fit.Beta <= 0 {
		return 0
	}

	return time.Duration((1 / fit.Beta) * float64(time.Second))
}

func (fit BivariateFit) ExcitationConfidence(
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

/*
Calibrated scales buy-side excitation from settled forecast feedback.
*/
func (fit BivariateFit) Calibrated(calibration float64) BivariateFit {
	if !fit.valid() {
		return fit
	}

	if calibration <= 0 {
		return fit.zeroBuyExcitationPrior()
	}

	if calibration == 1 {
		return fit
	}

	scaled := fit
	scaled.AlphaBB *= calibration
	scaled.AlphaBS *= calibration
	scaled.SpectralRadius = scaled.computeSpectralRadius()

	if scaled.SpectralRadius >= criticalBranch {
		scaled = scaled.ClampSubcritical()
	}

	if !scaled.valid() {
		return fit
	}

	excitation := fit.BuyIntensity - fit.MuBuy

	if excitation > 0 {
		scaled.BuyIntensity = fit.MuBuy + excitation*calibration
	}

	return scaled
}

func (fit BivariateFit) ClampSubcritical() BivariateFit {
	if fit.SpectralRadius <= 0 || fit.SpectralRadius >= criticalBranch {
		return fit
	}

	factor := criticalBranch / fit.SpectralRadius

	if factor >= 1 {
		return fit
	}

	clamped := fit
	clamped.AlphaBB *= factor
	clamped.AlphaBS *= factor
	clamped.AlphaSB *= factor
	clamped.AlphaSS *= factor
	clamped.SpectralRadius = clamped.computeSpectralRadius()

	return clamped
}

func (fit BivariateFit) zeroBuyExcitationPrior() BivariateFit {
	zeroed := fit
	zeroed.AlphaBB = 0
	zeroed.AlphaBS = 0
	zeroed.BuyIntensity = fit.MuBuy
	zeroed.SpectralRadius = zeroed.computeSpectralRadius()

	return zeroed
}

func (fit BivariateFit) compensator(
	stream ArrivalStream,
	horizon time.Time,
	span float64,
) float64 {
	beta := fit.Beta
	buySupport, sellSupport := stream.kernelSupport(horizon, beta)

	buyIntegral := fit.MuBuy*span +
		(fit.AlphaBB/beta)*buySupport +
		(fit.AlphaBS/beta)*sellSupport
	sellIntegral := fit.MuSell*span +
		(fit.AlphaSB/beta)*buySupport +
		(fit.AlphaSS/beta)*sellSupport

	return buyIntegral + sellIntegral
}

func (fit BivariateFit) withCrossZeroed() BivariateFit {
	if fit.AlphaBS <= 0 && fit.AlphaSB <= 0 {
		return fit
	}

	restricted := fit
	restricted.AlphaBS = 0
	restricted.AlphaSB = 0
	restricted.SpectralRadius = restricted.computeSpectralRadius()

	return restricted
}
