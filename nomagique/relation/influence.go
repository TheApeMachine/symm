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

	maxLag := request.Lag.MaxLag

	if maxLag <= 0 || maxLag > searchSpan {
		maxLag = searchSpan
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

	effective := statistic.EffectiveSampleSize(equalWeights(len(rows)))
	result.EffectiveSampleCount = effective
	result.Maturity = statistic.KishMaturity(equalWeights(len(rows)))

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
	coefficientVariance := finalFit.CoefficientVariance[sourceColumn]
	result.Coefficient = &coefficient

	if !math.IsNaN(coefficientVariance) && coefficientVariance > 0 {
		variance := coefficientVariance
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
		Source:   request.Source,
		Target:   request.Target,
		Controls: append([]Control(nil), request.Controls...),
		Status:   status,
	}
}

func (result *InfluenceResult) definedStepsCount() int {
	if result == nil {
		return 0
	}

	return result.definedSteps
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

	if candidate.definedStepsCount() != best.definedStepsCount() {
		return candidate.definedStepsCount() > best.definedStepsCount()
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
cutoff. The index cursor is advanced monotonically because history is
chronological and cutoffs are non-decreasing across target observations.
*/
func newestAtOrBefore(
	history []Observation,
	cursor *int,
	cutoff time.Time,
) (Observation, bool) {
	best := -1

	for *cursor < len(history) && !history[*cursor].At.After(cutoff) {
		best = *cursor
		*cursor++
	}

	if best < 0 {
		return Observation{}, false
	}

	return history[best], true
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
	restricted := make([]float64, 0, len(rows))
	full := make([]float64, 0, len(rows))
	rankDeficient := false

	for index := range rows {
		restrictedResidual, restrictedDefined, restrictedDeficient := predictRestricted(rows, index, controlCount)

		if restrictedDeficient {
			rankDeficient = true
		}

		if !restrictedDefined {
			continue
		}

		fullResidual, fullDefined, fullDeficient := predictFull(rows, index, controlCount)

		if fullDeficient {
			rankDeficient = true
		}

		if !fullDefined {
			continue
		}

		restricted = append(restricted, restrictedResidual)
		full = append(full, fullResidual)
	}

	return restricted, full, rankDeficient
}

/*
predictRestricted fits TargetLater ← TargetPast + ControlsPast on rows
strictly before the index and predicts the row at the index. The boolean
reports whether the fit was identifiable; a separate flag reports rank
deficiency (identifiable rows but singular design).
*/
func predictRestricted(rows []alignedRow, index int, controlCount int) (float64, bool, bool) {
	parameterCount := 2 + controlCount

	if index <= parameterCount {
		return 0, false, false
	}

	design := make([]float64, 0, index*parameterCount)
	targets := make([]float64, 0, index)

	for rowIndex := 0; rowIndex < index; rowIndex++ {
		appendPredictorRow(&design, rows[rowIndex], controlCount, false)
		targets = append(targets, rows[rowIndex].target.Raw)
	}

	fit := statistic.FitOLS(design, targets, parameterCount)

	if !fit.Defined {
		return 0, false, true
	}

	return predictRow(fit, rows[index], controlCount, false), true, false
}

/*
predictFull additionally fits the lagged Source and predicts the same row.
*/
func predictFull(rows []alignedRow, index int, controlCount int) (float64, bool, bool) {
	parameterCount := 3 + controlCount

	if index <= parameterCount {
		return 0, false, false
	}

	design := make([]float64, 0, index*parameterCount)
	targets := make([]float64, 0, index)

	for rowIndex := 0; rowIndex < index; rowIndex++ {
		appendPredictorRow(&design, rows[rowIndex], controlCount, true)
		targets = append(targets, rows[rowIndex].target.Raw)
	}

	fit := statistic.FitOLS(design, targets, parameterCount)

	if !fit.Defined {
		return 0, false, true
	}

	return predictRow(fit, rows[index], controlCount, true), true, false
}

/*
appendPredictorRow appends one row's predictors: intercept, TargetPast,
ControlsPast, and optionally SourcePast.
*/
func appendPredictorRow(
	design *[]float64,
	row alignedRow,
	controlCount int,
	withSource bool,
) {
	*design = append(*design, 1, row.targetPast.Raw)

	for _, control := range row.controls {
		*design = append(*design, control.Raw)
	}

	if withSource {
		*design = append(*design, row.source.Raw)
	}
}

/*
predictRow evaluates the fitted model on one aligned row.
*/
func predictRow(
	fit statistic.OLSFit,
	row alignedRow,
	controlCount int,
	withSource bool,
) float64 {
	predicted := fit.Coefficients[0] + fit.Coefficients[1]*row.targetPast.Raw

	for index, control := range row.controls {
		predicted += fit.Coefficients[2+index] * control.Raw
	}

	if withSource {
		predicted += fit.Coefficients[2+controlCount] * row.source.Raw
	}

	return row.target.Raw - predicted
}

/*
fullFit fits the final full model over all aligned rows and returns it.
*/
func fullFit(rows []alignedRow, controlCount int) *statistic.OLSFit {
	parameterCount := 3 + controlCount

	if len(rows) <= parameterCount {
		return nil
	}

	design := make([]float64, 0, len(rows)*parameterCount)
	targets := make([]float64, 0, len(rows))

	for _, row := range rows {
		appendPredictorRow(&design, row, controlCount, true)
		targets = append(targets, row.target.Raw)
	}

	fit := statistic.FitOLS(design, targets, parameterCount)
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
