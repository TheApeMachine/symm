package statistic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
JointDecayedEstimator is the single coherent causal estimator over one joint
multivariate state X = [x_0, ..., x_{k-1}] (log touch notionals per side and
log relative spread in Liquidity). One event-time decay weight alpha derives
every downstream fact — mean, covariance, scalar noise, z-scores, joint SNR and
effective support — so that no two consumers describe different histories.

Ordering is strictly causal (signal/liquidity/README.md §8):

	1. read pre-observation state (mu, covariance, weight moments);
	2. compute residual r = X - mu_{t-}, scalar noise (diag of covariance),
	   z-scores (r / noise when noise > 0), joint SNR (Cholesky over the
	   pre-observation covariance) and effective support N_eff;
	3. project (the caller's projector reads the emitted OBS slots);
	4. update mu, covariance and weight moments with X under alpha.

State slots (committed, pre-observation):
	state/mu/<j>
	state/cov/<i>/<j>
	state/weight_sum
	state/weight_sq_sum
	state/last_sec
	state/last_nsec

Observation slots (current):
	obs/residual/<j>
	obs/noise/<j>
	obs/zscore/<j>
	obs/neff
	obs/snr
	obs/ready
*/
type JointDecayedEstimator struct {
	prefix string
	count  int

	mu      []types.Symbol
	cov     [][]types.Symbol
	weightSum   types.Symbol
	weightSqSum types.Symbol
	spanHat     types.Symbol
	lastSec     types.Symbol
	lastNsec    types.Symbol

	residual []types.Symbol
	noise    []types.Symbol
	zscore   []types.Symbol

	neff  types.Symbol
	snr   types.Symbol
	ready types.Symbol
	horizon types.Symbol
}

/*
NewJointDecayedEstimator builds the slot table for a k-dimensional estimator.
*/
func NewJointDecayedEstimator(prefix string, count int) *JointDecayedEstimator {
	estimator := &JointDecayedEstimator{
		prefix:  prefix,
		count:   count,
		mu:      make([]types.Symbol, count),
		cov:     make([][]types.Symbol, count),
		residual: make([]types.Symbol, count),
		noise:   make([]types.Symbol, count),
		zscore:  make([]types.Symbol, count),
	}

	for index := range count {
		estimator.mu[index] = types.MustIntern(fmt.Sprintf("%s/state/mu/%d", prefix, index))
		estimator.cov[index] = make([]types.Symbol, count)
		estimator.residual[index] = types.MustIntern(fmt.Sprintf("%s/obs/residual/%d", prefix, index))
		estimator.noise[index] = types.MustIntern(fmt.Sprintf("%s/obs/noise/%d", prefix, index))
		estimator.zscore[index] = types.MustIntern(fmt.Sprintf("%s/obs/zscore/%d", prefix, index))

		for column := range count {
			estimator.cov[index][column] = types.MustIntern(fmt.Sprintf("%s/state/cov/%d/%d", prefix, index, column))
		}
	}

	estimator.weightSum = types.MustIntern(prefix + "/state/weight_sum")
	estimator.weightSqSum = types.MustIntern(prefix + "/state/weight_sq_sum")
	estimator.spanHat = types.MustIntern(prefix + "/state/span_hat")
	estimator.lastSec = types.MustIntern(prefix + "/state/last_sec")
	estimator.lastNsec = types.MustIntern(prefix + "/state/last_nsec")
	estimator.neff = types.MustIntern(prefix + "/obs/neff")
	estimator.snr = types.MustIntern(prefix + "/obs/snr")
	estimator.ready = types.MustIntern(prefix + "/obs/ready")
	estimator.horizon = types.MustIntern(prefix + "/obs/horizon")

	return estimator
}

/*
Primitive returns the step function. Every input observation must supply the
k values via InValues and the event time. alpha is the data-derived event-time
decay weight; a nil alpha returns 1 (first observation seeds the state).
*/
func (estimator *JointDecayedEstimator) Primitive(
	inValues []types.Symbol,
	alpha func(*types.Frame) float64,
) types.Primitive {
	return func(frame *types.Frame) {
		if len(inValues) != estimator.count {
			frame.Err = fmt.Errorf("statistic: joint estimator expects %d values, got %d", estimator.count, len(inValues))

			return
		}

		values := make([]float64, estimator.count)

		for index, slot := range inValues {
			value, found := frame.Get(slot)

			if !found {
				frame.Err = fmt.Errorf("statistic: joint estimator missing value slot %v", slot)

				return
			}

			values[index] = value
		}

		sec, hasSec := frame.Get(types.EventTimeSec)
		nsec, hasNsec := frame.Get(types.EventTimeNsec)

		if !hasSec || !hasNsec {
			frame.Err = fmt.Errorf("statistic: joint estimator requires event time")

			return
		}

		if nsec < 0 || nsec >= 1e9 {
			frame.Err = fmt.Errorf("statistic: joint estimator requires normalized nanoseconds")

			return
		}

		// Step 1: read pre-observation state.
		mu := make([]float64, estimator.count)
		hasMean := true

		for index := range estimator.count {
			value, found := frame.Get(estimator.mu[index])

			if !found {
				hasMean = false
				break
			}

			mu[index] = value
		}

		// Data-derived event-time decay weight: alpha = 1 - exp(-Δt·ln2 /
		// span_hat) where span_hat is the decayed inter-arrival cadence tracked
		// in state (grown by the data, no fixed window, no constant).
		previousSec, hasLastSec := frame.Get(estimator.lastSec)
		previousNsec, hasLastNsec := frame.Get(estimator.lastNsec)
		elapsed := 0.0

		if hasLastSec && hasLastNsec {
			elapsed = sec - previousSec + (nsec-previousNsec)*1e-9
		}

		spanHat, _ := frame.Get(estimator.spanHat)

		a := 1.0

		if hasMean && elapsed > 0 {
			if spanHat <= 0 {
				spanHat = elapsed
			}

			a = 1 - math.Exp(-elapsed*math.Ln2/spanHat)
		}

		// Step 2: pre-observation facts.
		if hasMean {
			residuals := make([]float64, estimator.count)

			for index := range estimator.count {
				residuals[index] = values[index] - mu[index]
				frame.Put(estimator.residual[index], residuals[index])

				noiseSq, _ := frame.Get(estimator.cov[index][index])

				if noiseSq > 0 {
					noise := math.Sqrt(noiseSq)

					frame.Put(estimator.noise[index], noise)
					frame.Put(estimator.zscore[index], residuals[index]/noise)
				}
			}

			// Joint SNR: (1/k) delta^T Sigma^{-1} delta over pre-observation
			// covariance, only when invertible and support sufficient.
			evaluateJointSNR(estimator, frame, residuals)
		}

		// Step 4: update state with alpha.
		gamma := 1 - a

		if hasMean {
			// Decayed mean and covariance (convex combination with gamma+a=1).
			for index := range estimator.count {
				r := values[index] - mu[index]
				frame.Put(estimator.mu[index], mu[index]+a*r)

				for column := range estimator.count {
					rc := values[column] - mu[column]
					cov, _ := frame.Get(estimator.cov[index][column])
					frame.Put(estimator.cov[index][column], cov+a*(r*rc-cov))
				}
			}

			// Weight moments: prior weights rescale by gamma, new weight enters
			// with weight a.
			oldSum, _ := frame.Get(estimator.weightSum)
			oldSumSq, _ := frame.Get(estimator.weightSqSum)
			frame.Put(estimator.weightSum, a+gamma*oldSum)
			frame.Put(estimator.weightSqSum, a*a+gamma*gamma*oldSumSq)

			// Decay the cadence estimate toward the latest gap.
			if elapsed > 0 {
				if spanHat <= 0 {
					spanHat = elapsed
				} else {
					spanHat += a * (elapsed - spanHat)
				}

				frame.Put(estimator.spanHat, spanHat)
			}
		} else {
			// First observation seeds the state.
			for index := range estimator.count {
				frame.Put(estimator.mu[index], values[index])
			}

			frame.Put(estimator.weightSum, 1)
			frame.Put(estimator.weightSqSum, 1)
			frame.Put(estimator.spanHat, 1)
		}

		frame.Put(estimator.lastSec, sec)
		frame.Put(estimator.lastNsec, nsec)

		// Effective support for maturity, and the derived regression horizon
		// (effective memory N_eff expressed as a time window: N_eff × cadence).
		sumW, _ := frame.Get(estimator.weightSum)
		sumW2, _ := frame.Get(estimator.weightSqSum)

		if sumW2 > 0 {
			neff := sumW*sumW/sumW2
			frame.Put(estimator.neff, neff)

			if horizon, found := frame.Get(estimator.spanHat); found && horizon > 0 {
				frame.Put(estimator.horizon, neff*horizon)
			}
		}
	}
}

/*
Residual returns the observation slot name of dimension j's divergence
(log(X_j) - mu_j), the pre-observation residual.
*/
func (estimator *JointDecayedEstimator) Residual(index int) types.Symbol {
	return estimator.residual[index]
}

/*
Noise returns the observation slot name of dimension j's scalar noise scale
(sqrt of the pre-observation variance).
*/
func (estimator *JointDecayedEstimator) Noise(index int) types.Symbol {
	return estimator.noise[index]
}

/*
ZScore returns the observation slot name of dimension j's z-score
(residual/noise), populated only when the noise is estimable and positive.
*/
func (estimator *JointDecayedEstimator) ZScore(index int) types.Symbol {
	return estimator.zscore[index]
}

/*
Neff returns the observation slot name carrying the effective support
N_eff = (Σw)²/Σ(w²).
*/
func (estimator *JointDecayedEstimator) Neff() types.Symbol {
	return estimator.neff
}

/*
SNR returns the observation slot name carrying the joint Mahalanobis SNR.
*/
func (estimator *JointDecayedEstimator) SNR() types.Symbol {
	return estimator.snr
}

/*
JointReady returns the observation slot name carrying the joint SNR readiness.
*/
func (estimator *JointDecayedEstimator) JointReady() types.Symbol {
	return estimator.ready
}

/*
Span returns the state slot name carrying the derived event-time cadence
estimate (seconds), used as the local-regression horizon so the divergence
velocity regresses over the same derived window as the estimator itself.
*/
func (estimator *JointDecayedEstimator) Span() types.Symbol {
	return estimator.spanHat
}

/*
Horizon returns the observation slot name carrying the derived regression
window (N_eff × cadence, seconds) — the estimator's effective memory expressed
as a time span, large enough that the local regression has supporting prior
observations within it.
*/
func (estimator *JointDecayedEstimator) Horizon() types.Symbol {
	return estimator.horizon
}

/*
evaluateJointSNR computes (1/k) delta^T Sigma^{-1} delta against the
pre-observation covariance via Cholesky, writing obs/snr and obs/ready only
when the covariance is invertible and has sufficient support. Absence of the
ready marker or ready == 0 means the SNR is undefined.
*/
func evaluateJointSNR(estimator *JointDecayedEstimator, frame *types.Frame, residuals []float64) {
	support, _ := frame.Get(estimator.weightSum)

	if support <= float64(estimator.count) {
		frame.Put(estimator.ready, 0)

		return
	}

	covariance := make([]float64, estimator.count*estimator.count)

	for row := range estimator.count {
		for column := range estimator.count {
			value, _ := frame.Get(estimator.cov[row][column])
			covariance[row*estimator.count+column] = value
		}
	}

	snr, distance, invertible := evaluateMahalanobisSNR(residuals, covariance, estimator.count)

	if !invertible {
		frame.Put(estimator.ready, 0)

		return
	}

	frame.Put(estimator.snr, snr)
	_ = distance
	frame.Put(estimator.ready, 1)
}
