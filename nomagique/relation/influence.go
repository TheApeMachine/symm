package relation

import (
	"math"
	"sort"
	"time"

	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
FitStatus is the explicit state of one Relation estimate. Invalid is not
zero: every failure state is distinct and observable.
*/
type FitStatus uint8

const (
	// FitOK means the estimate is defined.
	FitOK FitStatus = iota
	// FitNoSourceHistory means the Source coordinate has no retained observations.
	FitNoSourceHistory
	// FitNoTargetHistory means the Target coordinate has no retained observations.
	FitNoTargetHistory
	// FitControlUnavailable means an explicit control coordinate has no retained observations.
	FitControlUnavailable
	// FitNoPositiveLag means no positive candidate lag is resolvable from the observed cadence.
	FitNoPositiveLag
	// FitNoAlignedRows means no target observation could be aligned with all predictors.
	FitNoAlignedRows
	// FitInsufficientSupport means too few aligned rows for the parameter count.
	FitInsufficientSupport
	// FitRankDeficient means the design matrix lacks full column rank.
	FitRankDeficient
	// FitResidualVarianceUnavailable means no defined prequential residual step exists.
	FitResidualVarianceUnavailable
)

func (status FitStatus) String() string {
	switch status {
	case FitOK:
		return "ok"
	case FitNoSourceHistory:
		return "no_source_history"
	case FitNoTargetHistory:
		return "no_target_history"
	case FitControlUnavailable:
		return "control_unavailable"
	case FitNoPositiveLag:
		return "no_positive_lag"
	case FitNoAlignedRows:
		return "no_aligned_rows"
	case FitInsufficientSupport:
		return "insufficient_support"
	case FitRankDeficient:
		return "rank_deficient"
	case FitResidualVarianceUnavailable:
		return "residual_variance_unavailable"
	default:
		return "unknown"
	}
}

/*
LagPoint is one point of the lag-response surface: the predictive gain
measured at one candidate lag. Gains that are mathematically undefined are
absent, not zero.
*/
type LagPoint struct {
	Lag           time.Duration
	PredictiveGain *float64
	DefinedSteps  int
}

/*
Control is one explicit control coordinate with its own alignment lag. A
zero Lag aligns the control at the same cutoff as the Source (t - sourceLag).
Controls come from an explicit RelationPlan or CausalSchema; the estimator
never invents semantic controls.
*/
type Control struct {
	Coordinate Coordinate
	Lag        time.Duration
}

/*
InfluenceResult is the complete Relation output contract. Mathematically
undefined fields are nil pointers; undefined is never zero. Zero-valued
coefficients and zero PredictiveGain are valid measurements and remain
representable.
*/
type InfluenceResult struct {
	Source    Coordinate
	Target    Coordinate
	Controls  []Control
	From      time.Time
	At        time.Time
	SourceObservedAt time.Time
	TargetObservedAt time.Time
	SourceAge time.Duration

	Lag                 time.Duration
	LagResolution       time.Duration
	LagSearchSpan       time.Duration
	// LagSupportBound is the largest candidate lag the retained history can
	// support, derived from the target observation count, the parameter
	// count, and the observed cadence. It is provenance, not a fixed
	// constant.
	LagSupportBound     time.Duration
	LagCandidateCount   int
	LagSurface          []LagPoint

	Coefficient             *float64
	CoefficientVariance     *float64
	CoefficientSNR          *float64
	RestrictedResidualVariance *float64
	FullResidualVariance    *float64
	PredictiveGain          *float64
	EffectiveSampleCount    float64
	Maturity                float64

	EstimatorVersion string
	Epoch            uint64
	Status           FitStatus
	definedSteps     int
}

/*
Defined reports whether the estimate reached an OK state.
*/
func (result *InfluenceResult) Defined() bool {
	return result != nil && result.Status == FitOK
}

/*
CoefficientDefined reports whether the coefficient and its uncertainty are
identifiable.
*/
func (result *InfluenceResult) CoefficientDefined() bool {
	return result != nil && result.Coefficient != nil &&
		result.CoefficientVariance != nil && result.CoefficientSNR != nil
}

/*
InfluenceRequest is the explicit estimator input: exact Source, Target, and
Control coordinates plus the candidate lag domain.
*/
type InfluenceRequest struct {
	Source   Coordinate
	Target   Coordinate
	Controls []Control
	Lag      LagDomain
}

/*
InfluenceEstimator measures directed temporal predictive contribution between
coordinates. It never infers roles from names; every role is explicit in the
request. It never claims causality.
*/
type InfluenceEstimator struct {
	version string
}

/*
NewInfluenceEstimator builds the estimator. The version string is
provenance recorded in every result.
*/
func NewInfluenceEstimator(version string) *InfluenceEstimator {
	if version == "" {
		version = "prequential-linear-v1"
	}

	return &InfluenceEstimator{version: version}
}

/*
Version returns the estimator version string.
*/
func (estimator *InfluenceEstimator) Version() string {
	if estimator == nil {
		return "prequential-linear-v1"
	}

	return estimator.version
}

/*
Estimate measures the Influence of Source on Target under the explicit
controls and candidate lag domain, using only the store's resident
observational rings. The evaluation is causal/prequential: each target is
predicted by a model fitted strictly on earlier observations. Retained
history is read in place through resident ring views — never copied into
temporary slices.
*/
func (estimator *InfluenceEstimator) Estimate(
	store *ObservationStore,
	request InfluenceRequest,
) (*InfluenceResult, error) {
	if estimator == nil {
		estimator = NewInfluenceEstimator("")
	}

	if store == nil {
		return nil, errRelation("observation store required")
	}

	sourceView, sourceFound := store.ViewRing(request.Source)

	if !sourceFound || sourceView.Len() == 0 {
		if sourceFound {
			sourceView.Close()
		}

		return estimator.unavailable(request, FitNoSourceHistory), nil
	}

	targetView, targetFound := store.ViewRing(request.Target)

	if !targetFound || targetView.Len() == 0 {
		sourceView.Close()

		if targetFound {
			targetView.Close()
		}

		return estimator.unavailable(request, FitNoTargetHistory), nil
	}

	controlViews := make([]RingView, len(request.Controls))

	for index, control := range request.Controls {
		controlView, controlFound := store.ViewRing(control.Coordinate)

		if !controlFound || controlView.Len() == 0 {
			sourceView.Close()
			targetView.Close()

			for closeIndex := 0; closeIndex < index; closeIndex++ {
				controlViews[closeIndex].Close()
			}

			if controlFound {
				controlView.Close()
			}

			return estimator.unavailable(request, FitControlUnavailable), nil
		}

		controlViews[index] = controlView
	}

	defer func() {
		sourceView.Close()
		targetView.Close()

		for _, controlView := range controlViews {
			controlView.Close()
		}
	}()

	resolution, resolvable := deriveLagResolution(sourceView, targetView)

	if !resolvable || resolution <= 0 {
		return estimator.unavailable(request, FitNoPositiveLag), nil
	}

	searchSpan := targetView.At(targetView.Len() - 1).At.Sub(sourceView.At(0).At)

	if searchSpan <= 0 {
		return estimator.unavailable(request, FitNoPositiveLag), nil
	}

	// The retained history bounds the searchable lag domain: each
	// resolution step of lag consumes at least one target observation from
	// the alignment, and a fit needs more rows than parameters. The derived
	// bound is provenance, not a fixed constant.
	minRows := 4 + len(request.Controls)
	supportLagBound := time.Duration(max(0, targetView.Len()-minRows)) * resolution

	maxLag := request.Lag.MaxLag

	if maxLag <= 0 || maxLag > searchSpan {
		maxLag = searchSpan
	}

	if maxLag > supportLagBound {
		maxLag = supportLagBound
	}

	startLag := request.Lag.MinLag

	if startLag < resolution {
		startLag = resolution
	}

	candidates := lagCandidates(startLag, maxLag, resolution)
	best := (*InfluenceResult)(nil)
	surface := make([]LagPoint, 0, len(candidates))

	for _, lag := range candidates {
		candidate := estimator.estimateAtLag(
			request, sourceView, targetView, controlViews, lag, resolution, searchSpan,
		)

		if candidate == nil {
			surface = append(surface, LagPoint{Lag: lag})
			continue
		}

		surface = append(surface, LagPoint{
			Lag:            lag,
			PredictiveGain: candidate.PredictiveGain,
			DefinedSteps:   candidate.definedSteps,
		})

		if betterRelation(candidate, best) {
			best = candidate
		}
	}

	if best == nil {
		return estimator.unavailable(request, FitNoPositiveLag), nil
	}

	best.LagSurface = surface
	best.LagCandidateCount = len(candidates)
	best.LagSupportBound = supportLagBound
	best.EstimatorVersion = estimator.version
	return best, nil
}

/*
estimateAtLag runs the prequential restricted/full comparison at one lag. It
walks the resident target ring exactly twice (the prequential pass and the
final full fit), aligning predictors with per-series cursors on the resident
source/control/target rings — no history copy and no aligned-row
materialization.
*/
func (estimator *InfluenceEstimator) estimateAtLag(
	request InfluenceRequest,
	sourceView RingView,
	targetView RingView,
	controlViews []RingView,
	lag time.Duration,
	resolution time.Duration,
	searchSpan time.Duration,
) *InfluenceResult {
	restrictedParameters := 2 + len(request.Controls)
	fullParameters := 3 + len(request.Controls)
	restrictedAccumulator := statistic.NewRegressionAccumulator(restrictedParameters)
	fullAccumulator := statistic.NewRegressionAccumulator(fullParameters)
	restrictedResiduals := make([]float64, 0, targetView.Len())
	fullResiduals := make([]float64, 0, targetView.Len())
	rankDeficient := false
	rows := 0
	firstTarget := time.Time{}
	lastTarget := time.Time{}
	lastSource := time.Time{}

	// Reusable design row: [intercept, targetPast, controls..., source]. It
	// is reused across rows; no per-row slice is allocated.
	predictors := make([]float64, fullParameters)

	walkAligned(targetView, sourceView, controlViews, request.Controls, lag, func(target Observation, aligned []Observation) bool {
		rows++

		if rows == 1 {
			firstTarget = target.At
		}

		lastTarget = target.At
		lastSource = aligned[len(aligned)-1].At

		predictors[0] = 1
		predictors[1] = aligned[0].Raw

		for controlIndex := 0; controlIndex < len(request.Controls); controlIndex++ {
			predictors[2+controlIndex] = aligned[1+controlIndex].Raw
		}

		predictors[restrictedParameters] = aligned[len(aligned)-1].Raw

		// Prequential step: predict with models fitted strictly on earlier
		// rows, then incorporate the current row so it never trains the
		// model that scored it. The prediction runs on the recursive
		// least-squares state (O(p²), zero allocation) seeded from the exact
		// normal equations at the first non-singular design.
		restrictedPrediction, restrictedDefined := restrictedAccumulator.PrequentialPredict(predictors[:restrictedParameters])
		fullPrediction, fullDefined := fullAccumulator.PrequentialPredict(predictors[:fullParameters])

		// Warm-up steps (rows not exceeding parameters) are not defined and
		// not rank-deficient; a singular design with more rows than parameters
		// is rank deficiency. The RLS readiness mirrors the exact Fit Defined
		// gate, seeding false when the accumulated design is singular.
		if restrictedAccumulator.Rows() > restrictedParameters && !restrictedDefined {
			rankDeficient = true
		}

		if fullAccumulator.Rows() > fullParameters && !fullDefined {
			rankDeficient = true
		}

		if restrictedDefined && fullDefined {
			restrictedResiduals = append(restrictedResiduals, target.Raw-restrictedPrediction)
			fullResiduals = append(fullResiduals, target.Raw-fullPrediction)
		}

		restrictedAccumulator.PrequentialAdd(predictors[:restrictedParameters], target.Raw)
		fullAccumulator.PrequentialAdd(predictors[:fullParameters], target.Raw)

		return true
	})

	if rows == 0 {
		return nil
	}

	result := &InfluenceResult{
		Source:            request.Source,
		Target:            request.Target,
		Controls:          append([]Control(nil), request.Controls...),
		Lag:               lag,
		LagResolution:     resolution,
		LagSearchSpan:     searchSpan,
		LagCandidateCount: 1,
		Epoch:             request.Source.Epoch,
		Status:            FitOK,
	}

	result.definedSteps = len(restrictedResiduals)

	if rankDeficient {
		result.Status = FitRankDeficient
		return result
	}

	if len(restrictedResiduals) == 0 {
		result.Status = FitResidualVarianceUnavailable
		return result
	}

	restrictedVariance := meanSquares(restrictedResiduals)
	fullVariance := meanSquares(fullResiduals)
	result.RestrictedResidualVariance = &restrictedVariance
	result.FullResidualVariance = &fullVariance

	if gain := predictiveGain(restrictedVariance, fullVariance); gain != nil {
		result.PredictiveGain = gain
	}

	result.From = firstTarget
	result.At = lastTarget
	result.SourceObservedAt = lastSource
	result.TargetObservedAt = lastTarget
	result.SourceAge = lastTarget.Sub(lastSource)

	// Every aligned row carries unit weight, so the Kish effective sample
	// size equals the aligned row count exactly.
	effective := float64(rows)
	result.EffectiveSampleCount = effective

	if rows > 1 {
		result.Maturity = 1 - 1/effective
	}

	finalFit := fullFitViews(targetView, sourceView, controlViews, request.Controls, lag, fullParameters)

	if finalFit == nil || !finalFit.Defined {
		if rows <= restrictedParameters {
			result.Status = FitInsufficientSupport
			return result
		}

		result.Status = FitRankDeficient
		return result
	}

	sourceColumn := restrictedParameters
	coefficient := finalFit.Coefficients[sourceColumn]
	result.Coefficient = &coefficient

	if variance, found := finalFit.VarianceAt(sourceColumn); found &&
		!math.IsNaN(variance) && variance > 0 {
		result.CoefficientVariance = &variance
		snr := statistic.CoefficientSNR(coefficient, variance)
		result.CoefficientSNR = &snr
	}

	return result
}

/*
walkAligned visits every target observation that aligns with all predictors
in chronological order, reading exclusively from resident ring views with
per-series cursors. The visit callback receives the target and a reusable
slice of aligned observations ([targetPast, controls..., source]); the slice
must not be retained across calls. Nothing is materialized.
*/
func walkAligned(
	targetView RingView,
	sourceView RingView,
	controlViews []RingView,
	controls []Control,
	lag time.Duration,
	visit func(target Observation, aligned []Observation) bool,
) {
	seriesCount := 2 + len(controlViews)
	cursors := make([]int, seriesCount)

	for index := range cursors {
		cursors[index] = -1
	}

	aligned := make([]Observation, seriesCount)

	for targetIndex := 0; targetIndex < targetView.Len(); targetIndex++ {
		target := targetView.At(targetIndex)
		complete := true

		for seriesIndex := 0; seriesIndex < seriesCount; seriesIndex++ {
			seriesLag := lag

			if seriesIndex > 0 && seriesIndex <= len(controlViews) {
				controlLag := controls[seriesIndex-1].Lag

				if controlLag > 0 {
					seriesLag = controlLag
				}
			}

			cutoff := target.At.Add(-seriesLag)

			var history RingView

			switch {
			case seriesIndex == 0:
				history = targetView
			case seriesIndex <= len(controlViews):
				history = controlViews[seriesIndex-1]
			default:
				history = sourceView
			}

			predictor, found := newestAtOrBeforeView(history, &cursors[seriesIndex], cutoff)

			if !found {
				complete = false
				break
			}

			aligned[seriesIndex] = predictor
		}

		if !complete {
			continue
		}

		if !visit(target, aligned) {
			return
		}
	}
}

/*
fullFitViews fits the final full model over every aligned row from the
resident rings, in the same alignment used by the prequential pass.
*/
func fullFitViews(
	targetView RingView,
	sourceView RingView,
	controlViews []RingView,
	controls []Control,
	lag time.Duration,
	parameterCount int,
) *statistic.RegressionFit {
	accumulator := statistic.NewRegressionAccumulator(parameterCount)
	predictors := make([]float64, parameterCount)

	walkAligned(targetView, sourceView, controlViews, controls, lag, func(target Observation, aligned []Observation) bool {
		predictors[0] = 1
		predictors[1] = aligned[0].Raw

		for controlIndex := 0; controlIndex < len(controlViews); controlIndex++ {
			predictors[2+controlIndex] = aligned[1+controlIndex].Raw
		}

		predictors[parameterCount-1] = aligned[len(aligned)-1].Raw
		accumulator.Add(predictors, target.Raw)

		return true
	})

	fit := accumulator.Fit()
	return &fit
}

func (estimator *InfluenceEstimator) unavailable(
	request InfluenceRequest,
	status FitStatus,
) *InfluenceResult {
	return &InfluenceResult{
		Source:           request.Source,
		Target:           request.Target,
		Controls:         append([]Control(nil), request.Controls...),
		Epoch:            request.Source.Epoch,
		EstimatorVersion: estimator.version,
		Status:           status,
	}
}

/*
betterRelation ranks candidate lags by causal prequential predictive
performance: defined PredictiveGain first (higher is better), then more
defined prequential steps, then the smaller lag.
*/
func betterRelation(candidate *InfluenceResult, best *InfluenceResult) bool {
	if best == nil {
		return true
	}

	candidateGain := candidate.PredictiveGain != nil
	bestGain := best.PredictiveGain != nil

	if candidateGain != bestGain {
		return candidateGain
	}

	if candidateGain && bestGain {
		if *candidate.PredictiveGain != *best.PredictiveGain {
			return *candidate.PredictiveGain > *best.PredictiveGain
		}
	}

	if candidate.definedSteps != best.definedSteps {
		return candidate.definedSteps > best.definedSteps
	}

	return candidate.Lag < best.Lag
}

/*
predictiveGain computes log(Vr / Vf). It is defined only when both variances
are positive and finite; every degenerate case, including both-zero, is
serialized as undefined rather than infinite or NaN.
*/
func predictiveGain(restrictedVariance float64, fullVariance float64) *float64 {
	if restrictedVariance <= 0 || fullVariance <= 0 ||
		math.IsNaN(restrictedVariance) || math.IsNaN(fullVariance) ||
		math.IsInf(restrictedVariance, 0) || math.IsInf(fullVariance, 0) {
		return nil
	}

	gain := math.Log(restrictedVariance / fullVariance)
	return &gain
}

func meanSquares(residuals []float64) float64 {
	sum := 0.0

	for _, residual := range residuals {
		sum += residual * residual
	}

	return sum / float64(len(residuals))
}

/*
deriveLagResolution derives the minimum resolvable lag step from the observed
Source and Target cadence, using the slower typical cadence. Fixed bar counts
are never used as mathematical truth.
*/
func deriveLagResolution(sourceView RingView, targetView RingView) (time.Duration, bool) {
	sourceCadence := medianCadenceView(sourceView)
	targetCadence := medianCadenceView(targetView)

	if sourceCadence <= 0 {
		sourceCadence = targetCadence
	}

	if targetCadence <= 0 {
		targetCadence = sourceCadence
	}

	if sourceCadence <= 0 || targetCadence <= 0 {
		return 0, false
	}

	return max(sourceCadence, targetCadence), true
}

/*
medianCadenceView returns the median positive inter-observation gap of a
resident ring view.
*/
func medianCadenceView(view RingView) time.Duration {
	if view.Len() < 2 {
		return 0
	}

	gaps := make([]time.Duration, 0, view.Len()-1)

	for index := 1; index < view.Len(); index++ {
		gap := view.At(index).At.Sub(view.At(index - 1).At)

		if gap > 0 {
			gaps = append(gaps, gap)
		}
	}

	if len(gaps) == 0 {
		return 0
	}

	sort.Slice(gaps, func(left int, right int) bool {
		return gaps[left] < gaps[right]
	})

	return gaps[len(gaps)/2]
}

/*
lagCandidates enumerates the candidate lag times from start to maxLag at the
given resolution.
*/
func lagCandidates(start time.Duration, maxLag time.Duration, resolution time.Duration) []time.Duration {
	if start <= 0 || maxLag <= 0 || resolution <= 0 || start > maxLag {
		return nil
	}

	candidates := make([]time.Duration, 0)

	for lag := start; lag <= maxLag; lag += resolution {
		candidates = append(candidates, lag)
	}

	return candidates
}
