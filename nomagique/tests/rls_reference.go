// Test-only independent copy of the supplied square-root RLS implementation.
package tests

import (
	"fmt"
	"math"
	"sync"
)

/*
referenceRLSConfig configures recursive least squares.
*/
type referenceRLSConfig struct {
	Dimension        int
	InitialVariance  float64
	ForgettingFactor float64
}

/*
referenceRLSSample carries one feature vector and target.
*/
type referenceRLSSample struct {
	Features []float64
	Target   float64
}

/*
referenceRLSOutput reports the hot-path prediction and scalar update diagnostics.
Coefficient and covariance matrices are available only through Snapshot so the
streaming path stays linear in feature dimension.
*/
type referenceRLSOutput struct {
	Value            float64
	Scale            float64
	DegreesOfFreedom float64
	Ready            bool
	Innovation       float64
	Reset            bool
}

/*
referenceRLSSnapshot retains coefficients and the reconstructed covariance for inspection.
*/
type referenceRLSSnapshot struct {
	Beta               []float64
	Covariance         []float64
	CovarianceDiagonal []float64
}

/*
referenceRLSObserveOutput reports diagnostics from a training step that does not forecast.
*/
type referenceRLSObserveOutput struct {
	Innovation float64
	Reset      bool
}

/*
referenceRLS is an online square-root recursive-least-squares learner.
*/
type referenceRLS struct {
	config  referenceRLSConfig
	session *referenceRlsSession
	mu      sync.RWMutex
}

/*
referenceNewRLS returns a typed referenceRLS learner.
*/
func referenceNewRLS(config referenceRLSConfig) (*referenceRLS, error) {
	learner := &referenceRLS{
		config: config,
	}

	session, err := learner.loadSession()

	if err != nil {
		return nil, err
	}

	learner.session = session

	return learner, nil
}

/*
Measure predicts with the retained coefficients, then observes the target so
the returned Value is a true prior forecast rather than a post-hoc fit.
*/
func (rls *referenceRLS) Measure(sample referenceRLSSample) (referenceRLSOutput, error) {
	if rls == nil || rls.session == nil {
		return referenceRLSOutput{}, fmt.Errorf("learning: rls session required")
	}

	rls.mu.Lock()
	defer rls.mu.Unlock()

	prediction, err := rls.session.predictive(sample.Features)

	if err != nil {
		return referenceRLSOutput{}, fmt.Errorf("learning: rls predict failed: %w", err)
	}

	observed, err := rls.session.observe(sample.Features, sample.Target)

	if err != nil {
		return referenceRLSOutput{}, fmt.Errorf("learning: rls observe failed: %w", err)
	}

	return referenceRLSOutput{
		Value:            prediction.value,
		Scale:            prediction.scale,
		DegreesOfFreedom: prediction.degreesOfFreedom,
		Ready:            prediction.ready,
		Innovation:       observed.Innovation,
		Reset:            observed.Reset,
	}, nil
}

/*
Observe updates retained coefficients from one labeled sample without forecasting.
*/
func (rls *referenceRLS) Observe(sample referenceRLSSample) (referenceRLSObserveOutput, error) {
	if rls == nil || rls.session == nil {
		return referenceRLSObserveOutput{}, fmt.Errorf("learning: rls session required")
	}

	rls.mu.Lock()
	defer rls.mu.Unlock()

	observed, err := rls.session.observe(sample.Features, sample.Target)

	if err != nil {
		return referenceRLSObserveOutput{}, fmt.Errorf("learning: rls observe failed: %w", err)
	}

	return observed, nil
}

/*
Predict evaluates features against the retained coefficients without observing a
target. This keeps a live forecast strictly prior to the outcome used to train
the next step.
*/
func (rls *referenceRLS) Predict(features []float64) (referenceRLSOutput, error) {
	if rls == nil || rls.session == nil {
		return referenceRLSOutput{}, fmt.Errorf("learning: rls session required")
	}

	rls.mu.RLock()
	defer rls.mu.RUnlock()

	prediction, err := rls.session.predictive(features)

	if err != nil {
		return referenceRLSOutput{}, fmt.Errorf("learning: rls predict failed: %w", err)
	}

	return referenceRLSOutput{
		Value:            prediction.value,
		Scale:            prediction.scale,
		DegreesOfFreedom: prediction.degreesOfFreedom,
		Ready:            prediction.ready,
	}, nil
}

/*
PredictSum evaluates the posterior predictive distribution of the sum of
multiple future targets. The feature rows share one coefficient posterior, so
their covariance is retained rather than treating the forecasts as independent.
*/
func (rls *referenceRLS) PredictSum(featureRows [][]float64) (referenceRLSOutput, error) {
	if rls == nil || rls.session == nil {
		return referenceRLSOutput{}, fmt.Errorf("learning: rls session required")
	}

	rls.mu.RLock()
	defer rls.mu.RUnlock()

	prediction, err := rls.session.predictiveSum(featureRows)

	if err != nil {
		return referenceRLSOutput{}, fmt.Errorf("learning: rls predict sum failed: %w", err)
	}

	return referenceRLSOutput{
		Value:            prediction.value,
		Scale:            prediction.scale,
		DegreesOfFreedom: prediction.degreesOfFreedom,
		Ready:            prediction.ready,
	}, nil
}

/*
Snapshot copies coefficients and the reconstructed covariance for diagnostics.
*/
func (rls *referenceRLS) Snapshot() (referenceRLSSnapshot, error) {
	if rls == nil || rls.session == nil {
		return referenceRLSSnapshot{}, fmt.Errorf("learning: rls session required")
	}

	rls.mu.RLock()
	defer rls.mu.RUnlock()

	return rls.session.snapshot(), nil
}

/*
copyCoefficients copies the fitted linear model without reconstructing the
covariance matrix. The intercept is returned separately because callers such as
the resonance task head store it outside their dense weight row.
*/
func (rls *referenceRLS) copyCoefficients(weights []float64) (float64, error) {
	if rls == nil || rls.session == nil {
		return 0, fmt.Errorf("learning: rls session required")
	}

	rls.mu.RLock()
	defer rls.mu.RUnlock()

	if len(weights) != rls.session.dimension {
		return 0, fmt.Errorf(
			"learning: rls expected %d coefficient slots, got %d",
			rls.session.dimension,
			len(weights),
		)
	}

	copy(weights, rls.session.beta[1:])

	return rls.session.beta[0], nil
}

type referenceRlsSession struct {
	dimension        int
	initialVariance  float64
	forgettingFactor float64
	beta             []float64
	root             []float64
	design           []float64
	factor           []float64
	gain             []float64
	noiseShape       float64
	noiseScale       float64
}

type referenceRlsPrediction struct {
	value            float64
	scale            float64
	degreesOfFreedom float64
	ready            bool
}

func (rls *referenceRLS) loadSession() (*referenceRlsSession, error) {
	config := rls.config

	if config.ForgettingFactor == 0 {
		config.ForgettingFactor = 1
	}

	if config.Dimension <= 0 {
		return nil, fmt.Errorf("learning: rls dimension must be positive")
	}

	if config.InitialVariance <= 0 {
		return nil, fmt.Errorf("learning: rls initial variance must be positive")
	}

	if config.ForgettingFactor <= 0 || config.ForgettingFactor > 1 {
		return nil, fmt.Errorf("learning: rls forgetting factor must be in (0,1]")
	}

	size := config.Dimension + 1
	session := &referenceRlsSession{
		dimension:        config.Dimension,
		initialVariance:  config.InitialVariance,
		forgettingFactor: config.ForgettingFactor,
		beta:             make([]float64, size),
		root:             make([]float64, size*size),
		design:           make([]float64, size),
		factor:           make([]float64, size),
		gain:             make([]float64, size),
	}
	session.resetState()

	return session, nil
}

func (session *referenceRlsSession) resetState() {
	size := session.dimension + 1
	scale := math.Sqrt(session.initialVariance)

	for index := range session.beta {
		session.beta[index] = 0
	}

	for index := range session.root {
		session.root[index] = 0
	}

	for row := 0; row < size; row++ {
		session.root[row*size+row] = scale
	}

	session.noiseShape = 0
	session.noiseScale = 0
}

func (session *referenceRlsSession) observe(
	features []float64,
	target float64,
) (referenceRLSObserveOutput, error) {
	innovation, err := session.observeOnce(features, target)

	if err == nil {
		return referenceRLSObserveOutput{Innovation: innovation}, nil
	}

	session.resetState()

	innovation, retry := session.observeOnce(features, target)

	if retry != nil {
		session.resetState()

		return referenceRLSObserveOutput{Reset: true}, fmt.Errorf(
			"learning: rls observe failed after state reset: %w",
			retry,
		)
	}

	return referenceRLSObserveOutput{
		Innovation: innovation,
		Reset:      true,
	}, nil
}

func (session *referenceRlsSession) observeOnce(features []float64, target float64) (float64, error) {
	if !referenceRLSFinite(target) {
		return 0, fmt.Errorf("learning: rls target must be finite")
	}

	if len(features) != session.dimension {
		return 0, fmt.Errorf(
			"learning: rls expected %d features, got %d",
			session.dimension,
			len(features),
		)
	}

	size := session.dimension + 1
	design := session.design
	design[0] = 1

	for index, feature := range features {
		if !referenceRLSFinite(feature) {
			return 0, fmt.Errorf("learning: rls feature[%d] must be finite", index)
		}

		design[index+1] = feature
	}

	factor := session.factor

	for row := range size {
		sum := 0.0

		for col := range size {
			sum += session.root[col*size+row] * design[col]
		}

		factor[row] = sum
	}

	alpha := session.forgettingFactor

	for index := range size {
		alpha += factor[index] * factor[index]
	}

	if alpha <= 0 || !referenceRLSFinite(alpha) {
		return 0, fmt.Errorf("learning: rls denominator must be positive")
	}

	prediction := 0.0

	for index := range size {
		prediction += session.beta[index] * design[index]
	}

	innovation := target - prediction

	if !referenceRLSFinite(innovation) {
		return 0, fmt.Errorf("learning: rls innovation must be finite")
	}

	gain := session.gain

	for row := range size {
		sum := 0.0

		for col := range size {
			sum += session.root[row*size+col] * factor[col]
		}

		gain[row] = sum / alpha
		session.beta[row] += gain[row] * innovation

		if !referenceRLSFinite(session.beta[row]) {
			return 0, fmt.Errorf("learning: rls coefficient must stay finite")
		}
	}

	lambda := session.forgettingFactor
	rootLambda := math.Sqrt(lambda)
	gammaDenom := alpha + rootLambda*math.Sqrt(alpha)

	if gammaDenom <= 0 || !referenceRLSFinite(gammaDenom) {
		return 0, fmt.Errorf("learning: rls square-root update denominator invalid")
	}

	gamma := 1 / gammaDenom
	scale := 1.0

	if lambda < 1 {
		scale = 1 / rootLambda
	}

	for row := range size {
		scaledGain := gamma * gain[row] * alpha

		for col := range size {
			updated := session.root[row*size+col] - scaledGain*factor[col]
			session.root[row*size+col] = scale * updated

			if !referenceRLSFinite(session.root[row*size+col]) {
				return 0, fmt.Errorf("learning: rls square-root factor must stay finite")
			}
		}
	}

	session.noiseShape = lambda*session.noiseShape + 0.5
	session.noiseScale = lambda*session.noiseScale + 0.5*innovation*innovation/alpha

	return innovation, nil
}

/*
predictive evaluates the Student-t posterior predictive distribution retained by
recursive least squares. The coefficient covariance contributes design leverage;
the normalized prequential innovations contribute observation noise. With no
arbitrary noise prior, uncertainty remains unavailable until outcomes identify a
strictly positive residual scale.
*/
func (session *referenceRlsSession) predictive(features []float64) (referenceRlsPrediction, error) {
	if len(features) != session.dimension {
		return referenceRlsPrediction{}, fmt.Errorf(
			"learning: rls expected %d features, got %d",
			session.dimension,
			len(features),
		)
	}

	forecast := session.beta[0]

	for index, feature := range features {
		if !referenceRLSFinite(feature) {
			return referenceRlsPrediction{}, fmt.Errorf(
				"learning: rls feature[%d] must be finite",
				index,
			)
		}

		forecast += session.beta[index+1] * feature
	}

	if !referenceRLSFinite(forecast) {
		return referenceRlsPrediction{}, fmt.Errorf("learning: rls forecast must be finite")
	}

	prediction := referenceRlsPrediction{value: forecast}

	if !(session.noiseShape > 0) || !(session.noiseScale > 0) {
		return prediction, nil
	}

	size := session.dimension + 1
	leverage := 1.0

	for row := range size {
		factor := session.root[row]

		for featureIndex, feature := range features {
			factor += session.root[(featureIndex+1)*size+row] * feature
		}

		leverage += factor * factor
	}

	variance := session.noiseScale / session.noiseShape * leverage

	if !(variance > 0) || !referenceRLSFinite(variance) {
		return referenceRlsPrediction{}, fmt.Errorf(
			"learning: rls predictive variance must be finite and positive",
		)
	}

	prediction.scale = math.Sqrt(variance)
	prediction.degreesOfFreedom = 2 * session.noiseShape
	prediction.ready = true

	return prediction, nil
}

/*
predictiveSum preserves coefficient covariance across a collection of future
design rows. Conditional observation errors remain independent, contributing
one noise variance per row.
*/
func (session *referenceRlsSession) predictiveSum(
	featureRows [][]float64,
) (referenceRlsPrediction, error) {
	if len(featureRows) == 0 {
		return referenceRlsPrediction{}, fmt.Errorf(
			"learning: rls predictive sum requires feature rows",
		)
	}

	size := session.dimension + 1
	aggregateDesign := make([]float64, size)
	aggregateDesign[0] = float64(len(featureRows))

	for rowIndex, features := range featureRows {
		if len(features) != session.dimension {
			return referenceRlsPrediction{}, fmt.Errorf(
				"learning: rls feature row %d expected %d features, got %d",
				rowIndex,
				session.dimension,
				len(features),
			)
		}

		for featureIndex, feature := range features {
			if !referenceRLSFinite(feature) {
				return referenceRlsPrediction{}, fmt.Errorf(
					"learning: rls feature row %d[%d] must be finite",
					rowIndex,
					featureIndex,
				)
			}

			aggregateDesign[featureIndex+1] += feature
		}
	}

	forecast := 0.0

	for index, design := range aggregateDesign {
		forecast += session.beta[index] * design
	}

	prediction := referenceRlsPrediction{value: forecast}

	if !(session.noiseShape > 0) || !(session.noiseScale > 0) {
		return prediction, nil
	}

	leverage := float64(len(featureRows))

	for col := range size {
		factor := 0.0

		for row := range size {
			factor += aggregateDesign[row] * session.root[row*size+col]
		}

		leverage += factor * factor
	}

	variance := session.noiseScale / session.noiseShape * leverage

	if !(variance > 0) || !referenceRLSFinite(variance) {
		return referenceRlsPrediction{}, fmt.Errorf(
			"learning: rls predictive sum variance must be finite and positive",
		)
	}

	prediction.scale = math.Sqrt(variance)
	prediction.degreesOfFreedom = 2 * session.noiseShape
	prediction.ready = true

	return prediction, nil
}

func (session *referenceRlsSession) snapshot() referenceRLSSnapshot {
	size := session.dimension + 1
	covariance := make([]float64, size*size)
	diagonal := make([]float64, size)

	for row := range size {
		for col := range size {
			sum := 0.0

			for index := 0; index < size; index++ {
				sum += session.root[row*size+index] * session.root[col*size+index]
			}

			covariance[row*size+col] = sum
		}

		diagonal[row] = covariance[row*size+row]
	}

	return referenceRLSSnapshot{
		Beta:               append([]float64(nil), session.beta...),
		Covariance:         covariance,
		CovarianceDiagonal: diagonal,
	}
}

func referenceRLSFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
