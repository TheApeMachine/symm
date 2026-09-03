package equation

import (
	"math"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

/*
JointEstimator tracks a 3-dimensional online decayed joint distribution.
It derives causal running mean, covariance, noise scale, z-scores, effective
support N_eff, and Mahalanobis SNR without heap allocations, global frame slots,
or magic constants.
*/
type JointEstimator struct {
	mu          [3]float64
	cov         [3][3]float64
	preMu       [3]float64
	residuals   [3]float64
	noise       [3]float64
	zscore      [3]float64
	hasNoise    [3]bool
	snr         float64
	hasSNR      bool

	weightSum   float64
	weightSqSum float64
	spanHat     float64
	lastSec     float64
	lastNsec    float64
	hasMean     bool
	hasTime     bool
}

/*
Step evaluates the pre-observation residuals, noise, z-scores, and SNR,
then updates the online joint state using the causal event-time decay weight alpha.
Returns the joint SNR if defined, or 0.
*/
func (estimator *JointEstimator) Step(values [3]float64, sec, nsec float64) nmtypes.Number {
	elapsed := 0.0

	if estimator.hasTime {
		elapsed = sec - estimator.lastSec + (nsec-estimator.lastNsec)*1e-9
	}

	// Pre-observation facts
	estimator.hasSNR = false
	estimator.snr = 0

	for index := 0; index < 3; index++ {
		estimator.hasNoise[index] = false
		estimator.noise[index] = 0
		estimator.zscore[index] = 0
	}

	if estimator.hasMean {
		estimator.preMu = estimator.mu

		for index := 0; index < 3; index++ {
			estimator.residuals[index] = values[index] - estimator.mu[index]
			noiseSq := estimator.cov[index][index]

			if noiseSq > 0 {
				noise := math.Sqrt(noiseSq)
				estimator.noise[index] = noise
				estimator.zscore[index] = estimator.residuals[index] / noise
				estimator.hasNoise[index] = true
			}
		}

		if estimator.weightSqSum > 0 {
			preNeff := estimator.weightSum * estimator.weightSum / estimator.weightSqSum

			if preNeff > 3.0 {
				snr, invertible := computeCholeskySNR(estimator.residuals, estimator.cov)

				if invertible {
					estimator.snr = snr
					estimator.hasSNR = true
				}
			}
		}
	}

	// Calculate decay weight alpha
	a := 1.0

	if estimator.hasMean {
		if elapsed <= 0 {
			a = 0.0
		} else {
			if estimator.spanHat <= 0 {
				estimator.spanHat = elapsed
			}

			a = 1.0 - math.Exp(-elapsed*math.Ln2/estimator.spanHat)
		}
	}

	gamma := 1.0 - a

	// Update state
	if estimator.hasMean {
		for index := 0; index < 3; index++ {
			r := values[index] - estimator.mu[index]
			estimator.mu[index] += a * r

			for column := 0; column < 3; column++ {
				rc := values[column] - estimator.mu[column]
				estimator.cov[index][column] = gamma*estimator.cov[index][column] + a*gamma*(r*rc)
			}
		}

		estimator.weightSum = a + gamma*estimator.weightSum
		estimator.weightSqSum = a*a + gamma*gamma*estimator.weightSqSum

		if elapsed > 0 && estimator.spanHat <= 0 {
			estimator.spanHat = elapsed
		} else if elapsed > 0 {
			estimator.spanHat += a * (elapsed - estimator.spanHat)
		}
	} else {
		estimator.mu = values
		estimator.weightSum = 1.0
		estimator.weightSqSum = 1.0
		estimator.hasMean = true
	}

	estimator.lastSec = sec
	estimator.lastNsec = nsec
	estimator.hasTime = true

	return nmtypes.Number(estimator.snr)
}

func (estimator *JointEstimator) HasMean() bool {
	return estimator.hasMean
}

func (estimator *JointEstimator) Mean(index int) float64 {
	return estimator.mu[index]
}

func (estimator *JointEstimator) Baseline(index int) float64 {
	return math.Exp(estimator.preMu[index])
}

func (estimator *JointEstimator) Residual(index int) float64 {
	return estimator.residuals[index]
}

func (estimator *JointEstimator) Ratio(index int) float64 {
	return math.Exp(estimator.residuals[index])
}

func (estimator *JointEstimator) Noise(index int) (float64, bool) {
	return estimator.noise[index], estimator.hasNoise[index]
}

func (estimator *JointEstimator) ZScore(index int) (float64, bool) {
	return estimator.zscore[index], estimator.hasNoise[index]
}

func (estimator *JointEstimator) SNR() (float64, bool) {
	return estimator.snr, estimator.hasSNR
}

func (estimator *JointEstimator) NEff() float64 {
	if estimator.weightSqSum <= 0 {
		return 0
	}

	return estimator.weightSum * estimator.weightSum / estimator.weightSqSum
}

func (estimator *JointEstimator) Horizon() float64 {
	neff := estimator.NEff()

	if neff <= 0 || estimator.spanHat <= 0 {
		return 0
	}

	return neff * estimator.spanHat
}

func computeCholeskySNR(residuals [3]float64, cov [3][3]float64) (float64, bool) {
	var l [3][3]float64

	for i := 0; i < 3; i++ {
		for j := 0; j <= i; j++ {
			sum := 0.0

			for k := 0; k < j; k++ {
				sum += l[i][k] * l[j][k]
			}

			if i == j {
				val := cov[i][i] - sum

				if val <= 0 || math.IsNaN(val) {
					return 0, false
				}

				l[i][j] = math.Sqrt(val)
			} else {
				if l[j][j] <= 0 {
					return 0, false
				}

				l[i][j] = (cov[i][j] - sum) / l[j][j]
			}
		}
	}

	var y [3]float64

	for i := 0; i < 3; i++ {
		sum := 0.0

		for k := 0; k < i; k++ {
			sum += l[i][k] * y[k]
		}

		if l[i][i] <= 0 {
			return 0, false
		}

		y[i] = (residuals[i] - sum) / l[i][i]
	}

	distSq := y[0]*y[0] + y[1]*y[1] + y[2]*y[2]

	return distSq / 3.0, true
}
