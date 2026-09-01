package strategy

import (
	"fmt"
	"math"
	"time"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/signal"
	"github.com/theapemachine/symm/types"
	"gonum.org/v1/gonum/stat/distuv"
)

/* directionalConfig states the statistical policies of the streaming predictor. */
type directionalConfig struct {
	initialVariance  float64
	forgettingFactor float64
}

/*
directionalForecast is a posterior distribution of executable-bid log return
over the activity clock observed for this symbol. probabilityUp and
probabilityProfitable are two queries against that one distribution.
*/
type directionalForecast struct {
	symbol                string
	at                    time.Time
	probabilityUp         float64
	probabilityProfitable float64
	expectedLogReturn     float64
	breakEvenLogReturn    float64
	ready                 bool
	status                string
	horizon               time.Duration
	horizonSteps          int
	calibration           uint64
	directionalFeatures   int
	estimabilityFeatures  int
	executionFeatures     int
	reviewFeatures        int
	output                learning.RLSOutput
	opportunity           types.OpportunityCandidate
}

type featureKey struct {
	family string
	source string
	metric string
}

type forecastContext struct {
	archetype types.OpportunityArchetype
	phase     types.OpportunityPhase
}

type featureUse uint8

const (
	featureContext featureUse = iota + 1
	featureEstimability
	featureExecution
	featureReview
)

type predictorFeature struct {
	value         float64
	quality       float64
	observedAt    time.Time
	observed      bool
	group         int
	associations  map[forecastContext]*weightedAssociation
	pendingValue  float64
	pendingWeight float64
	pending       bool
	use           featureUse
}

type evidenceGroup struct {
	numerator   float64
	denominator float64
	features    int
}

type pendingReturn struct {
	issuedAt  time.Time
	reference float64
	horizon   time.Duration
	score     float64
	context   forecastContext
	present   bool
}

type intervalMean struct {
	last  time.Time
	count uint64
	mean  time.Duration
}

type directionalState struct {
	features     map[featureKey]*predictorFeature
	groupsByName map[string]int
	groups       []evidenceGroup
	returns      *returnHead
	pending      pendingReturn
	horizon      time.Duration
	intervals    intervalMean
	opportunity  types.OpportunityCandidate
}

/*
directionalPredictor learns feature relationships within an opportunity
archetype and phase. It retains sufficient statistics and one unresolved
adaptive-horizon forecast per symbol; no event history is accumulated.
*/
type directionalPredictor struct {
	config directionalConfig
	states map[string]*directionalState
}

func newDirectionalPredictor(config directionalConfig) (*directionalPredictor, error) {
	if config.initialVariance <= 0 {
		return nil, fmt.Errorf("strategy: positive directional initial variance required")
	}

	if config.forgettingFactor <= 0 || config.forgettingFactor > 1 {
		return nil, fmt.Errorf("strategy: directional forgetting factor must be in (0, 1]")
	}

	return &directionalPredictor{
		config: config,
		states: make(map[string]*directionalState),
	}, nil
}

func (predictor *directionalPredictor) state(symbol string) (*directionalState, error) {
	if state := predictor.states[symbol]; state != nil {
		return state, nil
	}

	returns, err := newReturnHead(predictor.config)

	if err != nil {
		return nil, err
	}

	state := &directionalState{
		features:     make(map[featureKey]*predictorFeature),
		groupsByName: make(map[string]int),
		returns:      returns,
	}
	predictor.states[symbol] = state

	return state, nil
}

/* observeMeasurement routes one metric according to its declared semantic role. */
func (predictor *directionalPredictor) observeMeasurement(
	measurement *data.Measurement[float64],
) error {
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

	for metricName, metric := range measurement.Metrics {
		if measurement.Source == "pumpdump" && metricName == "volume_bar_duration" {
			state.observeHorizon(metric.Raw)
		}

		state.observe(
			featureKey{family: "measurement", source: measurement.Source, metric: metricName},
			metric.Raw,
			quality,
			measurement.At,
			semanticFeatureUse(measurement.Source, metricName),
		)
	}

	return nil
}

func semanticFeatureUse(source, metric string) featureUse {
	semantics, found := signal.Lookup(source, metric)

	if !found {
		return featureReview
	}

	if semantics.Status == "MIGRATE_AND_REMOVE" || semantics.Role == "DEPRECATE_REDUNDANT" {
		return featureReview
	}

	switch semantics.Role {
	case "ESTIMABILITY":
		return featureEstimability
	case "EXECUTION_CONTEXT":
		return featureExecution
	default:
		return featureContext
	}
}

/*
advance resolves a prior forecast only after its own activity-clock horizon,
then queries the current executable-return posterior and issues one new bounded
pending observation when necessary.
*/
func (predictor *directionalPredictor) advance(
	symbol string,
	at time.Time,
	executableBid float64,
	breakEven *float64,
) (*directionalForecast, error) {
	state, err := predictor.state(symbol)

	if err != nil {
		return nil, err
	}

	state.intervals.observe(at)

	if err := state.resolve(at, executableBid); err != nil {
		return nil, err
	}

	forecast := &directionalForecast{
		symbol:      symbol,
		at:          at,
		status:      "missing-active-opportunity",
		horizon:     state.horizon,
		calibration: state.returns.outcomes,
		opportunity: state.opportunity,
	}
	forecast.estimabilityFeatures = state.currentFeatureCount(featureEstimability, at)
	forecast.executionFeatures = state.currentFeatureCount(featureExecution, at)
	forecast.reviewFeatures = state.currentFeatureCount(featureReview, at)

	context, active := state.context()

	if !active {
		return forecast, nil
	}

	if state.horizon <= 0 {
		forecast.status = "missing-adaptive-horizon"
		return forecast, nil
	}

	forecast.horizonSteps = state.intervals.steps(state.horizon)
	score, features := state.aggregate(context, at, state.horizon)
	forecast.directionalFeatures = features

	output, err := state.returns.predict(score)

	if err != nil {
		return nil, err
	}

	forecast.output = output
	forecast.expectedLogReturn = output.Value
	forecast.status = "awaiting-return-distribution"

	if breakEven != nil && executableBid > 0 && *breakEven > 0 {
		forecast.breakEvenLogReturn = math.Log(*breakEven / executableBid)
	}

	if output.Ready && breakEven != nil {
		distribution := distuv.StudentsT{
			Mu: output.Value, Sigma: output.Scale, Nu: output.DegreesOfFreedom,
		}
		forecast.probabilityUp = 1 - distribution.CDF(0)
		forecast.probabilityProfitable = 1 - distribution.CDF(forecast.breakEvenLogReturn)
		forecast.ready = true
		forecast.status = "adaptive-return-distribution-ready"
	}

	state.issue(at, executableBid, context, score)

	return forecast, nil
}
