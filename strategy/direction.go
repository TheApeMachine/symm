package strategy

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning"
)

/*
directionalConfig states the statistical policies of the streaming predictor.
*/
type directionalConfig struct {
	initialVariance       float64
	forgettingFactor      float64
	calibrationConfidence float64
}

/*
directionalForecast is a pair of calibrated classifications. probabilityUp is
P(the next ticker observation is higher). probabilityProfitable is P(the next
executable bid clears the entry break-even known when this forecast was issued).
Neither field estimates a profit amount.
*/
type directionalForecast struct {
	symbol                   string
	at                       time.Time
	probabilityUp            float64
	probabilityProfitable    float64
	directionReady           bool
	profitabilityReady       bool
	directionSkillLowerBound float64
	profitSkillLowerBound    float64
	directionCalibration     uint64
	profitCalibration        uint64
	directionFeatures        int
	profitFeatures           int
	directionOutput          learning.RLSOutput
	profitOutput             learning.RLSOutput
}

type featureKey struct {
	family string
	source string
	metric string
}

type predictorFeature struct {
	value          float64
	quality        float64
	observed       bool
	pendingValue   float64
	pendingQuality float64
	pendingUp      bool
	pendingProfit  bool
	up             weightedAssociation
	profit         weightedAssociation
}

type directionalState struct {
	mu             sync.Mutex
	features       map[featureKey]*predictorFeature
	direction      *binaryHead
	profitability  *binaryHead
	priorReference float64
	priorBreakEven float64
	profitPending  bool
}

/*
directionalPredictor learns one per-metric relationship to each binary outcome,
then calibrates the bounded aggregate prequentially. Measurements are consumed
once; only sufficient statistics and the one unresolved feature value remain.
*/
type directionalPredictor struct {
	config directionalConfig
	states sync.Map
}

func newDirectionalPredictor(config directionalConfig) (*directionalPredictor, error) {
	if config.initialVariance <= 0 {
		return nil, fmt.Errorf("strategy: positive directional initial variance required")
	}

	if config.forgettingFactor <= 0 || config.forgettingFactor > 1 {
		return nil, fmt.Errorf("strategy: directional forgetting factor must be in (0, 1]")
	}

	if config.calibrationConfidence <= 0.5 || config.calibrationConfidence >= 1 {
		return nil, fmt.Errorf("strategy: directional calibration confidence must be in (0.5, 1)")
	}

	return &directionalPredictor{config: config}, nil
}

func (predictor *directionalPredictor) state(symbol string) (*directionalState, error) {
	if loaded, found := predictor.states.Load(symbol); found {
		return loaded.(*directionalState), nil
	}

	direction, err := newBinaryHead(predictor.config)

	if err != nil {
		return nil, err
	}

	profitability, err := newBinaryHead(predictor.config)

	if err != nil {
		return nil, err
	}

	candidate := &directionalState{
		features:      make(map[featureKey]*predictorFeature),
		direction:     direction,
		profitability: profitability,
	}
	actual, _ := predictor.states.LoadOrStore(symbol, candidate)

	return actual.(*directionalState), nil
}

/*
observeMeasurement gives every projected metric one update of its own resident
predictive sufficient statistics.
*/
func (predictor *directionalPredictor) observeMeasurement(measurement *data.Measurement[float64]) error {
	if measurement == nil {
		return nil
	}

	if measurement.Err != nil {
		return fmt.Errorf("strategy: measurement %s failed: %w", measurement.ID, measurement.Err)
	}

	state, err := predictor.state(measurement.Label)

	if err != nil {
		return err
	}

	quality := measurement.Maturity

	if measurement.SNRDefined {
		quality *= measurement.SNR / (1 + measurement.SNR)
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	for metricName, metric := range measurement.Metrics {
		if err := state.observe(featureKey{family: "measurement", source: measurement.Source, metric: metricName}, metric.Raw, quality); err != nil {
			return err
		}
	}

	return nil
}

/*
observe stores one metric fact for the symbol. A non-finite value is not a
valid observation: it would otherwise poison every aggregate score and make
the recursive least squares classifier misfire on a NaN feature (see rls
predictive, which strictly requires finite features). Reject it here so the
erroneous producer surfaces in Planner.Step's log with the identity of the
metric that fed it, instead of collapsing the whole symbol's forecast.
*/
func (state *directionalState) observe(key featureKey, value, quality float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf(
			"strategy: feature %s/%s/%s must be finite, got %v",
			key.family,
			key.source,
			key.metric,
			value,
		)
	}

	feature := state.features[key]

	if feature == nil {
		feature = &predictorFeature{}
		state.features[key] = feature
	}

	feature.value = value
	feature.quality = quality
	feature.observed = true

	return nil
}

func (state *directionalState) aggregate(profitability bool) (float64, int) {
	numerator := 0.0
	denominator := 0.0
	count := 0

	for _, feature := range state.features {
		if !feature.observed || feature.quality <= 0 {
			continue
		}

		association := feature.up

		if profitability {
			association = feature.profit
		}

		evidence, maturity, ready := association.evidence(feature.value)

		if !ready {
			continue
		}

		weight := maturity * feature.quality
		numerator += weight * evidence
		denominator += weight
		count++
	}

	if denominator <= 0 {
		return 0, 0
	}

	return numerator / denominator, count
}

/*
advance resolves the prior ticker classifications, then issues the current
classification from all observations resident for the symbol.
*/
func (predictor *directionalPredictor) advance(
	symbol string,
	at time.Time,
	reference float64,
	executableBid float64,
	breakEven *float64,
) (*directionalForecast, error) {
	state, err := predictor.state(symbol)

	if err != nil {
		return nil, err
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.priorReference > 0 {
		up := reference > state.priorReference
		profit := state.profitPending && executableBid > state.priorBreakEven

		for _, feature := range state.features {
			if feature.pendingUp {
				feature.up.observe(feature.pendingValue, signedOutcome(up), feature.pendingQuality)
			}

			if feature.pendingProfit && state.profitPending {
				feature.profit.observe(feature.pendingValue, signedOutcome(profit), feature.pendingQuality)
			}
		}

		if err := state.direction.resolve(up); err != nil {
			return nil, err
		}

		if state.profitPending {
			if err := state.profitability.resolve(profit); err != nil {
				return nil, err
			}
		}
	}

	directionScore, directionFeatures := state.aggregate(false)
	directionOutput, probabilityUp, directionReady, directionSkill, err := state.direction.issue(directionScore)

	if err != nil {
		return nil, err
	}

	profitScore, profitFeatures := state.aggregate(true)
	profitOutput := learning.RLSOutput{}
	probabilityProfit := 0.5
	profitReady := false
	profitSkill := 0.0

	if breakEven != nil {
		profitOutput, probabilityProfit, profitReady, profitSkill, err = state.profitability.issue(profitScore)

		if err != nil {
			return nil, err
		}
	}

	for _, feature := range state.features {
		if !feature.observed {
			continue
		}

		feature.pendingValue = feature.value
		feature.pendingQuality = feature.quality
		feature.pendingUp = true
		feature.pendingProfit = breakEven != nil
	}

	state.priorReference = reference
	state.profitPending = breakEven != nil

	if breakEven != nil {
		state.priorBreakEven = *breakEven
	}

	return &directionalForecast{
		symbol:                   symbol,
		at:                       at,
		probabilityUp:            probabilityUp,
		probabilityProfitable:    probabilityProfit,
		directionReady:           directionReady && directionFeatures > 0,
		profitabilityReady:       profitReady && profitFeatures > 0,
		directionSkillLowerBound: directionSkill,
		profitSkillLowerBound:    profitSkill,
		directionCalibration:     state.direction.skill.count,
		profitCalibration:        state.profitability.skill.count,
		directionFeatures:        directionFeatures,
		profitFeatures:           profitFeatures,
		directionOutput:          directionOutput,
		profitOutput:             profitOutput,
	}, nil
}

func signedOutcome(positive bool) float64 {
	if positive {
		return 1
	}

	return -1
}
