package strategy

import (
	"math"

	"github.com/theapemachine/symm/nomagique/learning"
	"gonum.org/v1/gonum/stat/distuv"
)

/* weightedAssociation retains the sufficient statistics for one feature/outcome relationship. */
type weightedAssociation struct {
	weightSum   float64
	weightSqSum float64
	meanValue   float64
	meanOutcome float64
	valueM2     float64
	outcomeM2   float64
	coMoment    float64
}

func (association *weightedAssociation) observe(value, outcome, weight float64) {
	if weight <= 0 {
		return
	}

	nextWeight := association.weightSum + weight
	valueDelta := value - association.meanValue
	outcomeDelta := outcome - association.meanOutcome
	association.meanValue += weight / nextWeight * valueDelta
	association.meanOutcome += weight / nextWeight * outcomeDelta
	association.valueM2 += weight * valueDelta * (value - association.meanValue)
	association.outcomeM2 += weight * outcomeDelta * (outcome - association.meanOutcome)
	association.coMoment += weight * valueDelta * (outcome - association.meanOutcome)
	association.weightSum = nextWeight
	association.weightSqSum += weight * weight
}

func (association weightedAssociation) evidence(value float64) (float64, float64, bool) {
	if association.weightSum <= 0 || association.weightSqSum <= 0 ||
		association.valueM2 <= 0 || association.outcomeM2 <= 0 {
		return 0, 0, false
	}

	effectiveSupport := association.weightSum * association.weightSum / association.weightSqSum

	if effectiveSupport <= 1 {
		return 0, 0, false
	}

	correlation := association.coMoment / math.Sqrt(association.valueM2*association.outcomeM2)
	variance := association.valueM2 / association.weightSum
	standardized := (value - association.meanValue) / math.Sqrt(variance)
	maturity := 1 - 1/effectiveSupport

	return math.Tanh(correlation * standardized), maturity, true
}

type skillEstimate struct {
	count uint64
	mean  float64
	m2    float64
}

func (estimate *skillEstimate) observe(value float64) {
	estimate.count++
	delta := value - estimate.mean
	estimate.mean += delta / float64(estimate.count)
	estimate.m2 += delta * (value - estimate.mean)
}

func (estimate skillEstimate) lowerBound(confidence float64) (float64, bool) {
	if estimate.count < 2 || estimate.m2 <= 0 {
		return 0, false
	}

	degrees := float64(estimate.count - 1)
	variance := estimate.m2 / degrees
	standardError := math.Sqrt(variance / float64(estimate.count))
	distribution := distuv.StudentsT{Mu: estimate.mean, Sigma: standardError, Nu: degrees}

	return distribution.Quantile(1 - confidence), true
}

/* binaryHead calibrates one bounded evidence score against a binary outcome. */
type binaryHead struct {
	learner               *learning.RLS
	skill                 skillEstimate
	outcomes              uint64
	positive              uint64
	pendingScore          float64
	pendingProbability    float64
	pendingBaseline       float64
	pendingReady          bool
	hasPending            bool
	calibrationConfidence float64
}

func newBinaryHead(config directionalConfig) (*binaryHead, error) {
	learner, err := learning.NewRLS(learning.RLSConfig{
		Dimension:        1,
		InitialVariance:  config.initialVariance,
		ForgettingFactor: config.forgettingFactor,
	})

	if err != nil {
		return nil, err
	}

	return &binaryHead{
		learner:               learner,
		calibrationConfidence: config.calibrationConfidence,
	}, nil
}

func (head *binaryHead) resolve(positive bool) error {
	if !head.hasPending {
		return nil
	}

	target := -1.0
	actual := 0.0

	if positive {
		target = 1
		actual = 1
		head.positive++
	}

	if _, err := head.learner.Observe(learning.RLSSample{
		Features: []float64{head.pendingScore},
		Target:   target,
	}); err != nil {
		return err
	}

	if head.pendingReady {
		modelError := actual - head.pendingProbability
		baselineError := actual - head.pendingBaseline
		head.skill.observe(baselineError*baselineError - modelError*modelError)
	}

	head.outcomes++
	head.hasPending = false

	return nil
}

func (head *binaryHead) issue(score float64) (learning.RLSOutput, float64, bool, float64, error) {
	output, err := head.learner.Predict([]float64{score})

	if err != nil {
		return learning.RLSOutput{}, 0, false, 0, err
	}

	probability := 0.5

	if output.Ready {
		distribution := distuv.StudentsT{
			Mu:    output.Value,
			Sigma: output.Scale,
			Nu:    output.DegreesOfFreedom,
		}
		probability = 1 - distribution.CDF(0)
	}

	baseline := 0.5

	if head.outcomes > 0 {
		baseline = float64(head.positive) / float64(head.outcomes)
	}

	head.pendingScore = score
	head.pendingProbability = probability
	head.pendingBaseline = baseline
	head.pendingReady = output.Ready
	head.hasPending = true

	lowerBound, calibrated := head.skill.lowerBound(head.calibrationConfidence)

	return output, probability, output.Ready && calibrated && lowerBound > 0, lowerBound, nil
}
