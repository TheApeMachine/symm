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
controls and candidate lag domain, using only the store's retained
observational history. The evaluation is causal/prequential: each target is
predicted by a model fitted strictly on earlier observations.
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

	sourceHistory := store.History(request.Source)

	if len(sourceHistory) == 0 {
		return estimator.unavailable(request, FitNoSourceHistory), nil
	}

	targetHistory := store.History(request.Target)

	if len(targetHistory) == 0 {
		return estimator.unavailable(request, FitNoTargetHistory), nil
	}

	controlHistories := make([][]Observation, len(request.Controls))

	for index, control := range request.Controls {
		history := store.History(control.Coordinate)

		if len(history) == 0 {
			return estimator.unavailable(request, FitControlUnavailable), nil
		}

		controlHistories[index] = history
	}

	resolution, resolvable := deriveLagResolution(sourceHistory, targetHistory)

	if !resolvable || resolution <= 0 {
		return estimator.unavailable(request, FitNoPositiveLag), nil
	}

	searchSpan := targetHistory[len(targetHistory)-1].At.Sub(sourceHistory[0].At)

	if searchSpan <= 0 {
		return estimator.unavailable(request, FitNoPositiveLag), nil
	}

	// The retained history bounds the searchable lag domain: each
	// resolution step of lag consumes at least one target observation from
	// the alignment, and a fit needs more rows than parameters. The derived
	// bound is provenance, not a fixed constant.
	minRows := 4 + len(request.Controls)
	supportLagBound := time.Duration(max(0, len(targetHistory)-minRows)) * resolution

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
			request, sourceHistory, targetHistory, controlHistories, lag, resolution, searchSpan,
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
estimateAtLag runs the prequential restricted/full comparison at one lag.
*/
func (estimator *InfluenceEstimator) estimateAtLag(
	request InfluenceRequest,
	sourceHistory []Observation,
	targetHistory []Observation,
	controlHistories [][]Observation,
	lag time.Duration,
	resolution time.Duration,
	searchSpan time.Duration,
) *InfluenceResult {
	rows := alignAtLag(targetHistory, sourceHistory, controlHistories, request.Controls, lag)

	if len(rows) == 0 {
		return nil
	}

	parameterCount := 2 + len(request.Controls)
	result := &InfluenceResult{
		Source:          request.Source,
		Target:          request.Target,
		Controls:        append([]Control(nil), request.Controls...),
		Lag:             lag,
		LagResolution:   resolution,
		LagSearchSpan:   searchSpan,
		LagCandidateCount: 1,
		Epoch:           request.Source.Epoch,
		Status:          FitOK,
	}

	restrictedResiduals, fullResiduals, rankDeficient := prequentialResiduals(rows, len(request.Controls))
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

	result.From = rows[0].target.At
	result.At = rows[len(rows)-1].target.At
	result.SourceObservedAt = rows[len(rows)-1].source.At
	result.TargetObservedAt = rows[len(rows)-1].target.At
	result.SourceAge = result.TargetObservedAt.Sub(result.SourceObservedAt)

	weights := equalWeights(len(rows))
	effective := statistic.EffectiveSampleSize(weights)
	result.EffectiveSampleCount = effective
	result.Maturity = statistic.KishMaturity(weights)

	finalFit := fullFit(rows, len(request.Controls))

	if finalFit == nil || !finalFit.Defined {
		if len(rows) <= parameterCount {
			result.Status = FitInsufficientSupport
			return result
		}

		result.Status = FitRankDeficient
		return result
	}

	sourceColumn := parameterCount
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
alignedRow is one target observation aligned with its lagged predictors.
*/
type alignedRow struct {
	target    Observation
	source    Observation
	controls  []Observation
	targetPast Observation
}

/*
alignAtLag aligns every target observation at time t with the newest valid
Source and Control observations no later than their cutoffs. The Source
cutoff is t - lag; each control cutoff is t - control.Lag when positive,
otherwise t - lag. Future observations never enter a row. Only target
observations with all predictors aligned are retained, and they are returned
in chronological order.
*/
func alignAtLag(
	targetHistory []Observation,
	sourceHistory []Observation,
	controlHistories [][]Observation,
	controls []Control,
	lag time.Duration,
) []alignedRow {
	series := make([]LaggedSeries, 0, 2+len(controlHistories))
	series = append(series, LaggedSeries{Observations: targetHistory, Lag: lag})

	for index, history := range controlHistories {
		controlLag := controls[index].Lag

		if controlLag <= 0 {
			controlLag = lag
		}

		series = append(series, LaggedSeries{Observations: history, Lag: controlLag})
	}

	series = append(series, LaggedSeries{Observations: sourceHistory, Lag: lag})
	aligned := AlignLagged(targetHistory, series)
	rows := make([]alignedRow, 0, len(aligned))

	for _, row := range aligned {
		rows = append(rows, alignedRow{
			target:     row.Target,
			targetPast: row.Predictors[0],
			controls:   row.Predictors[1 : 1+len(controlHistories)],
			source:     row.Predictors[len(row.Predictors)-1],
		})
	}

	return rows
}

/*
newestAtOrBefore returns the newest observation in history at or before
cutoff. The cursor remains positioned on the last matched observation (a
negative value means no match has ever been recorded): repeated calls with
non-decreasing cutoffs re-scan only entries after the previous match, and a
call whose cutoff reaches no newer entry returns the previously matched
observation. When no observation has ever matched, the result is not-found.
The precondition is that history is chronological and cutoffs are
non-decreasing across calls, which the alignment paths guarantee.
*/
func newestAtOrBefore(
	history []Observation,
	cursor *int,
	cutoff time.Time,
) (Observation, bool) {
	best := -1
	start := 0

	if cursor != nil && *cursor >= 0 {
		start = *cursor
	}

	for index := start; index < len(history) && !history[index].At.After(cutoff); index++ {
		best = index
	}

	if best >= 0 {
		if cursor != nil {
			*cursor = best
		}

		return history[best], true
	}

	if cursor == nil || *cursor < 0 {
		return Observation{}, false
	}

	return history[*cursor], true
}

/*
prequentialResiduals evaluates restricted and full predictors prequentially:
each row's residual is produced by a model fitted strictly on earlier rows,
and only then is the row incorporated. The current target never trains the
model that scores it. Steps whose fit is not identifiable contribute no
residual. A step whose design matrix lacks full rank marks the whole estimate
rank-deficient rather than silently regularized.
*/
func prequentialResiduals(rows []alignedRow, controlCount int) ([]float64, []float64, bool) {
	restrictedParameters := 2 + controlCount
	fullParameters := 3 + controlCount
	restrictedAccumulator := statistic.NewRegressionAccumulator(restrictedParameters)
	fullAccumulator := statistic.NewRegressionAccumulator(fullParameters)
	restricted := make([]float64, 0, len(rows))
	full := make([]float64, 0, len(rows))
	rankDeficient := false

	for _, row := range rows {
		restrictedFit := restrictedAccumulator.Fit()
		fullFit := fullAccumulator.Fit()

		// Warm-up steps (rows not exceeding parameters) are not defined and
		// not rank-deficient; a singular fit with more rows than parameters
		// is rank deficiency.
		if restrictedAccumulator.Rows() > restrictedParameters && !restrictedFit.Defined {
			rankDeficient = true
		}

		if fullAccumulator.Rows() > fullParameters && !fullFit.Defined {
			rankDeficient = true
		}

		if restrictedFit.Defined && fullFit.Defined {
			restrictedPrediction, _ := restrictedFit.Predict(predictorsFor(row, controlCount, false))
			fullPrediction, _ := fullFit.Predict(predictorsFor(row, controlCount, true))
			restricted = append(restricted, row.target.Raw-restrictedPrediction)
			full = append(full, row.target.Raw-fullPrediction)
		}

		// Incorporate the current row only after it was predicted, so it
		// never trains the model that scored it.
		restrictedAccumulator.Add(predictorsFor(row, controlCount, false), row.target.Raw)
		fullAccumulator.Add(predictorsFor(row, controlCount, true), row.target.Raw)
	}

	return restricted, full, rankDeficient
}

/*
predictorsFor builds one design row: intercept, TargetPast, ControlsPast,
and optionally SourcePast.
*/
func predictorsFor(row alignedRow, controlCount int, withSource bool) []float64 {
	predictors := make([]float64, 0, 3+controlCount)
	predictors = append(predictors, 1, row.targetPast.Raw)

	for _, control := range row.controls {
		predictors = append(predictors, control.Raw)
	}

	if withSource {
		predictors = append(predictors, row.source.Raw)
	}

	return predictors
}

/*
fullFit fits the final full model over all aligned rows from the
incrementally accumulated moments.
*/
func fullFit(rows []alignedRow, controlCount int) *statistic.RegressionFit {
	parameterCount := 3 + controlCount
	accumulator := statistic.NewRegressionAccumulator(parameterCount)

	for _, row := range rows {
		accumulator.Add(predictorsFor(row, controlCount, true), row.target.Raw)
	}

	fit := accumulator.Fit()
	return &fit
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

func equalWeights(count int) []float64 {
	weights := make([]float64, count)

	for index := range weights {
		weights[index] = 1
	}

	return weights
}

/*
deriveLagResolution derives the minimum resolvable lag step from the observed
Source and Target cadence, using the slower typical cadence. Fixed bar counts
are never used as mathematical truth.
*/
func deriveLagResolution(sourceHistory []Observation, targetHistory []Observation) (time.Duration, bool) {
	sourceCadence := medianCadence(sourceHistory)
	targetCadence := medianCadence(targetHistory)

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
medianCadence returns the median positive inter-observation gap of a series.
*/
func medianCadence(history []Observation) time.Duration {
	gaps := make([]time.Duration, 0, len(history)-1)

	for index := 1; index < len(history); index++ {
		gap := history[index].At.Sub(history[index-1].At)

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
