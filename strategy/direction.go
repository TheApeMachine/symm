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
over the calibrated ticker-step reach observed for this symbol. probabilityUp and
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
	archetype    types.OpportunityArchetype
	phase        types.OpportunityPhase
	horizonSteps int
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
	observedOrder uint64
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
	issuedOrdinal uint64
	reference     float64
	horizonSteps  int
	score         float64
	context       forecastContext
	present       bool
}

type directionalState struct {
	features               map[featureKey]*predictorFeature
	groupsByName           map[string]int
	groups                 []evidenceGroup
	returns                map[int]*returnHead
	pending                pendingReturn
	observationOrdinal     uint64
	lastAdvanceObservation uint64
	tickerOrdinal          uint64
	horizonSteps           int
	opportunity            types.OpportunityCandidate
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

	state := &directionalState{
		features:     make(map[featureKey]*predictorFeature),
		groupsByName: make(map[string]int),
		returns:      make(map[int]*returnHead),
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
		if err := state.observe(
			featureKey{family: "measurement", source: measurement.Source, metric: metricName},
			metric.Raw,
			quality,
			measurement.At,
			semanticFeatureUse(measurement.Source, metricName),
		); err != nil {
			return err
		}
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
advance resolves a prior forecast only after its frozen ticker-step horizon,
then queries the current executable-return posterior and issues one new bounded
pending observation when necessary.
*/
func (predictor *directionalPredictor) advance(
	symbol string,
	at time.Time,
	executableBid float64,
) (*directionalForecast, error) {
	if symbol == "" || at.IsZero() || executableBid <= 0 {
		return nil, fmt.Errorf(
			"strategy: forecast requires symbol, event time, and positive executable bid",
		)
	}

	state, err := predictor.state(symbol)

	if err != nil {
		return nil, err
	}

	priorObservation := state.lastAdvanceObservation

	state.lastAdvanceObservation = state.observationOrdinal
	state.tickerOrdinal++

	if err := state.resolve(executableBid); err != nil {
		return nil, err
	}

	forecast := &directionalForecast{
		symbol:       symbol,
		at:           at,
		status:       "missing-active-opportunity",
		horizonSteps: state.horizonSteps,
		opportunity:  state.opportunity,
	}
	forecast.estimabilityFeatures = state.currentFeatureCount(
		featureEstimability, priorObservation,
	)
	forecast.executionFeatures = state.currentFeatureCount(
		featureExecution, priorObservation,
	)
	forecast.reviewFeatures = state.currentFeatureCount(
		featureReview, priorObservation,
	)

	context, active := state.context()

	if !active {
		return forecast, nil
	}

	if state.horizonSteps <= 0 {
		forecast.status = "missing-adaptive-horizon"
		return forecast, nil
	}

	score, features := state.aggregate(context, priorObservation)
	forecast.directionalFeatures = features

	returns, err := predictor.returnHead(state, state.horizonSteps)

	if err != nil {
		return nil, err
	}

	forecast.calibration = returns.outcomes
	output, err := returns.predict(score)

	if err != nil {
		return nil, err
	}

	forecast.output = output
	forecast.status = "awaiting-return-distribution"

	if output.Ready && output.DegreesOfFreedom <= 1 {
		forecast.status = "return-distribution-mean-undefined"
	}

	if output.Ready && output.DegreesOfFreedom > 1 {
		distribution := distuv.StudentsT{
			Mu: output.Value, Sigma: output.Scale, Nu: output.DegreesOfFreedom,
		}
		forecast.expectedLogReturn = output.Value
		forecast.probabilityUp = 1 - distribution.CDF(0)
		forecast.status = "missing-execution-boundary"
	}

	state.issue(
		priorObservation, executableBid, context, score,
	)

	return forecast, nil
}

/*
observeExecutionBoundary applies current fee, spread, and impact economics to a
forecast after its ticker-step state transition has already completed.
*/
func (forecast *directionalForecast) observeExecutionBoundary(
	executableBid float64,
	breakEven float64,
) error {
	if executableBid <= 0 || breakEven <= 0 {
		return fmt.Errorf("strategy: positive executable bid and break-even required")
	}

	forecast.breakEvenLogReturn = math.Log(breakEven / executableBid)

	if !forecast.output.Ready || forecast.output.DegreesOfFreedom <= 1 {
		return nil
	}

	distribution := distuv.StudentsT{
		Mu:    forecast.output.Value,
		Sigma: forecast.output.Scale,
		Nu:    forecast.output.DegreesOfFreedom,
	}
	forecast.probabilityProfitable = 1 - distribution.CDF(forecast.breakEvenLogReturn)
	forecast.ready = true
	forecast.status = "adaptive-return-distribution-ready"

	return nil
}
