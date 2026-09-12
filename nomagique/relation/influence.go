package relation

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"sort"
	"time"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
viewRing drives the store's ring command and returns the read-locked view and
whether the coordinate is registered.
*/
func viewRing(store core.Primitive, coordinate Coordinate) (RingView, bool) {
	command := StoreCommand{Ring: &RingRequest{Coordinate: coordinate}}
	var result StoreResult

	for out := range store.Next(transport.NewOne(unsafe.Pointer(&command)).Next(nil)) {
		result = *(*StoreResult)(out)
	}

	return result.Ring, result.Found
}

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

/*
String renders the fit status for logs and telemetry. Enum rendering follows
house style (statistic and data payloads carry their own String).
*/
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
	Lag            time.Duration
	PredictiveGain *float64
	DefinedSteps   int
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
	Source           Coordinate
	Target           Coordinate
	Controls         []Control
	From             time.Time
	At               time.Time
	SourceObservedAt time.Time
	TargetObservedAt time.Time
	SourceAge        time.Duration

	Lag           time.Duration
	LagResolution time.Duration
	LagSearchSpan time.Duration
	// LagSupportBound is the largest candidate lag the retained history can
	// support, derived from the target observation count, the parameter
	// count, and the observed cadence. It is provenance, not a fixed
	// constant.
	LagSupportBound   time.Duration
	LagCandidateCount int
	LagSurface        []LagPoint

	Coefficient                *float64
	CoefficientVariance        *float64
	CoefficientSNR             *float64
	RestrictedResidualVariance *float64
	FullResidualVariance       *float64
	PredictiveGain             *float64
	EffectiveSampleCount       float64
	Maturity                   float64

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
InfluenceHistory carries the read-locked resident ring views the estimate
walks: the Source ring, the Target ring, and one ring per explicit control.
An unregistered coordinate yields a zero view, whose length is zero; the
estimator reports the matching unavailable status rather than inventing
history.
*/
type InfluenceHistory struct {
	Source   RingView
	Target   RingView
	Controls []RingView
}

/*
InfluenceRequest is the explicit estimator input: exact Source, Target, and
Control coordinates, the resident ring views to walk, and the candidate lag
domain.
*/
type InfluenceRequest struct {
	Source   Coordinate
	Target   Coordinate
	Controls []Control
	History  InfluenceHistory
	Lag      LagDomain
}

/*
Influence measures directed temporal predictive contribution between
coordinates. It never infers roles from names; every role is explicit in the
request. It never claims causality.

The evaluation is causal/prequential: each target is predicted by a model
fitted strictly on earlier observations. Retained history is read in place
through the request's ring views — never copied into temporary slices — and
every found view is closed when the estimate completes.
*/
type Influence struct {
	err     error
	version string
	out     *InfluenceResult
}

/*
NewInfluence builds the estimator Primitive. The version string is provenance
recorded in every result; an empty version is recorded as a domain failure
and every stream over the primitive yields nothing.
*/
func NewInfluence(version string) core.Primitive {
	if version == "" {
		return &Influence{
			err: fmt.Errorf(
				"%w: relation: influence estimator requires a version",
				core.ErrDomain,
			),
		}
	}

	return &Influence{version: version}
}

/*
Next receives *InfluenceRequest payloads and yields a *InfluenceResult for
each: an unavailable result when the request's history cannot support an
estimate, otherwise the best prequential candidate-lag estimate.
*/
func (op *Influence) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if op.err != nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			request := (*InfluenceRequest)(arriving)
			op.out = op.estimate(request)

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *Influence) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
estimate measures the Influence of Source on Target under the explicit
controls and candidate lag domain, using only the request's resident ring
views. The evaluation is causal/prequential: each target is predicted by a
model fitted strictly on earlier observations.
*/
func (op *Influence) estimate(request *InfluenceRequest) *InfluenceResult {
	sourceView := request.History.Source
	targetView := request.History.Target
	controlViews := request.History.Controls

	defer func() {
		sourceView.Close()
		targetView.Close()

		for _, controlView := range controlViews {
			controlView.Close()
		}
	}()

	if sourceView.Len() == 0 {
		return op.unavailable(request, FitNoSourceHistory)
	}

	if targetView.Len() == 0 {
		return op.unavailable(request, FitNoTargetHistory)
	}

	for _, controlView := range controlViews {
		if controlView.Len() == 0 {
			return op.unavailable(request, FitControlUnavailable)
		}
	}

	resolution, resolvable := deriveLagResolution(sourceView, targetView)

	if !resolvable || resolution <= 0 {
		return op.unavailable(request, FitNoPositiveLag)
	}

	searchSpan := targetView.At(targetView.Len() - 1).At.Sub(sourceView.At(0).At)

	if searchSpan <= 0 {
		return op.unavailable(request, FitNoPositiveLag)
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

	// scratch is built once per estimate and reused across every candidate
	// lag: the residual/predictor/cursor/aligned buffers are reused slices.
	// A candidate search routinely walks tens of lags per estimate cycle
	// across hundreds of candidate pairs per tick, so a fresh set of
	// buffers per lag was a direct multiplier on process-wide allocation
	// pressure.
	scratch := newEstimateScratch(2+len(request.Controls), targetView.Len())

	for _, lag := range candidates {
		candidate := op.estimateAtLag(
			request, sourceView, targetView, controlViews, lag, resolution, searchSpan, scratch,
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
		return op.unavailable(request, FitNoPositiveLag)
	}

	best.LagSurface = surface
	best.LagCandidateCount = len(candidates)
	best.LagSupportBound = supportLagBound
	best.EstimatorVersion = op.version
	return best
}

/*
estimateScratch holds every reusable buffer estimateAtLag/fullFitViews need
across a single estimate's candidate-lag search: the residual and predictor
slices, and the per-series cursor/aligned-observation buffers the aligned
walk scans with. seriesCount is 2 + len(Controls) (target-past, source, one
slot per control); parameterCount is fullParameters (restrictedParameters +
1) since that upper-bounds the predictor row.
*/
type estimateScratch struct {
	restrictedResiduals []float64
	fullResiduals       []float64
	predictors          []float64
	fullFitPredictors   []float64
	cursors             []int
	aligned             []Observation
}

/*
newEstimateScratch allocates one estimateScratch sized for seriesCount
series (2 + len(Controls)) and an expected row capacity, so the residual
slices rarely need to grow during the search.
*/
func newEstimateScratch(seriesCount int, expectedRows int) *estimateScratch {
	fullParameters := seriesCount + 1

	return &estimateScratch{
		restrictedResiduals: make([]float64, 0, expectedRows),
		fullResiduals:       make([]float64, 0, expectedRows),
		predictors:          make([]float64, fullParameters),
		fullFitPredictors:   make([]float64, fullParameters),
		cursors:             make([]int, seriesCount),
		aligned:             make([]Observation, seriesCount),
	}
}

/*
singlePointer presents one payload pointer as a one-element run.
*/
func singlePointer(value unsafe.Pointer) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		yield(value)
	}
}

/*
foldRegressionRow drives one design row through a regression accumulator
Primitive and reports the reading the row produced.
*/
func foldRegressionRow(
	accumulator core.Primitive,
	row *statistic.RegressionRow,
) (statistic.RegressionReading, bool) {
	var reading statistic.RegressionReading

	for out := range accumulator.Next(singlePointer(unsafe.Pointer(row))) {
		reading = *(*statistic.RegressionReading)(out)
	}

	if err := accumulator.Error(); err != nil {
		return reading, false
	}

	return reading, true
}

/*
estimateAtLag runs the prequential restricted/full comparison at one lag. It
walks the resident target ring exactly twice (the prequential pass and the
final full fit), aligning predictors with per-series cursors on the resident
source/control/target rings — no history copy and no aligned-row
materialization. scratch's buffers are reused across every candidate lag in
the enclosing search; the accumulators are fresh Primitives per lag.
*/
func (op *Influence) estimateAtLag(
	request *InfluenceRequest,
	sourceView RingView,
	targetView RingView,
	controlViews []RingView,
	lag time.Duration,
	resolution time.Duration,
	searchSpan time.Duration,
	scratch *estimateScratch,
) *InfluenceResult {
	restrictedParameters := 2 + len(request.Controls)
	fullParameters := 3 + len(request.Controls)

	restrictedAccumulator := statistic.NewRegressionAccumulator(restrictedParameters)
	fullAccumulator := statistic.NewRegressionAccumulator(fullParameters)

	restrictedResiduals := scratch.restrictedResiduals[:0]
	fullResiduals := scratch.fullResiduals[:0]
	rankDeficient := false
	rows := 0
	firstTarget := time.Time{}
	lastTarget := time.Time{}
	lastSource := time.Time{}

	// Reusable design row: [intercept, targetPast, controls..., source]. It
	// is reused across rows and across lags; no per-row or per-lag slice is
	// allocated.
	predictors := scratch.predictors

	walkAligned(targetView, sourceView, controlViews, request.Controls, lag, scratch.cursors, scratch.aligned, func(target Observation, aligned []Observation) bool {
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
		restrictedRow := statistic.RegressionRow{Predictors: predictors[:restrictedParameters], Target: target.Raw}
		restrictedReading, restrictedOK := foldRegressionRow(restrictedAccumulator, &restrictedRow)

		if !restrictedOK {
			op.Error(restrictedAccumulator.Error())
			return false
		}

		fullRow := statistic.RegressionRow{Predictors: predictors[:fullParameters], Target: target.Raw}
		fullReading, fullOK := foldRegressionRow(fullAccumulator, &fullRow)

		if !fullOK {
			op.Error(fullAccumulator.Error())
			return false
		}

		// Warm-up steps (rows not exceeding parameters) are not defined and
		// not rank-deficient; a singular design with more rows than parameters
		// is rank deficiency. The RLS readiness mirrors the exact Fit Defined
		// gate, seeding false when the accumulated design is singular. The
		// reading's fit counts the row just incorporated, so the pre-add row
		// count is one less.
		if restrictedReading.Fit.Observations-1 > restrictedParameters && !restrictedReading.PredictionDefined {
			rankDeficient = true
		}

		if fullReading.Fit.Observations-1 > fullParameters && !fullReading.PredictionDefined {
			rankDeficient = true
		}

		if restrictedReading.PredictionDefined && fullReading.PredictionDefined {
			restrictedResiduals = append(restrictedResiduals, target.Raw-restrictedReading.Prediction)
			fullResiduals = append(fullResiduals, target.Raw-fullReading.Prediction)
		}

		return true
	})

	// append may have grown the residual slices past their starting
	// capacity; write the (possibly reallocated) backing arrays back so the
	// next candidate lag reuses the larger capacity instead of scratch
	// reverting to its original, smaller allocation.
	scratch.restrictedResiduals = restrictedResiduals
	scratch.fullResiduals = fullResiduals

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

	finalFit := fullFitViews(targetView, sourceView, controlViews, request.Controls, lag, fullParameters, scratch)

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

	if sourceColumn >= 0 && sourceColumn < finalFit.Parameters &&
		len(finalFit.CoefficientVariance) == finalFit.Parameters {
		variance := finalFit.CoefficientVariance[sourceColumn]

		if !math.IsNaN(variance) && variance > 0 {
			result.CoefficientVariance = &variance

			snr, err := coefficientSNR(coefficient, variance)

			if err != nil {
				op.Error(err)
				return nil
			}

			result.CoefficientSNR = &snr
		}
	}

	return result
}

/*
walkAligned visits every target observation that aligns with all predictors
in chronological order, reading exclusively from resident ring views with
per-series cursors. The visit callback receives the target and a reusable
slice of aligned observations ([targetPast, controls..., source]); the slice
must not be retained across calls. Nothing is materialized. cursors and
aligned are the caller's reusable per-series buffers (sized 2 +
len(controlViews)); they are reset in place on every call so the same
allocation serves every candidate lag in a search.
*/
func walkAligned(
	targetView RingView,
	sourceView RingView,
	controlViews []RingView,
	controls []Control,
	lag time.Duration,
	cursors []int,
	aligned []Observation,
	visit func(target Observation, aligned []Observation) bool,
) {
	seriesCount := 2 + len(controlViews)

	for index := 0; index < seriesCount; index++ {
		cursors[index] = -1
	}

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

			predictor, found := newestAtOrBefore(history, &cursors[seriesIndex], cutoff)

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
resident rings, in the same alignment used by the prequential pass. It reuses
scratch's predictor buffer, the same as the prequential pass, so the final
fit adds no allocations on top of the search that already ran at this lag.
*/
func fullFitViews(
	targetView RingView,
	sourceView RingView,
	controlViews []RingView,
	controls []Control,
	lag time.Duration,
	parameterCount int,
	scratch *estimateScratch,
) *statistic.RegressionFit {
	accumulator := statistic.NewRegressionAccumulator(parameterCount)
	predictors := scratch.fullFitPredictors
	var fit statistic.RegressionFit

	walkAligned(targetView, sourceView, controlViews, controls, lag, scratch.cursors, scratch.aligned, func(target Observation, aligned []Observation) bool {
		predictors[0] = 1
		predictors[1] = aligned[0].Raw

		for controlIndex := 0; controlIndex < len(controlViews); controlIndex++ {
			predictors[2+controlIndex] = aligned[1+controlIndex].Raw
		}

		predictors[parameterCount-1] = aligned[len(aligned)-1].Raw

		row := statistic.RegressionRow{Predictors: predictors[:parameterCount], Target: target.Raw}
		reading, ok := foldRegressionRow(accumulator, &row)

		if !ok {
			return false
		}

		fit = reading.Fit

		return true
	})

	if err := accumulator.Error(); err != nil {
		return nil
	}

	return &fit
}

/*
unavailable synthesizes the explicit unavailable result for one request.
*/
func (op *Influence) unavailable(
	request *InfluenceRequest,
	status FitStatus,
) *InfluenceResult {
	return &InfluenceResult{
		Source:           request.Source,
		Target:           request.Target,
		Controls:         append([]Control(nil), request.Controls...),
		Epoch:            request.Source.Epoch,
		EstimatorVersion: op.version,
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

/*
meanSquares returns the mean of squared residuals.
*/
func meanSquares(residuals []float64) float64 {
	sum := 0.0

	for _, residual := range residuals {
		sum += residual * residual
	}

	return sum / float64(len(residuals))
}

/*
coefficientSNR drives the statistic layer's coefficient SNR Primitive for one
coefficient/variance pair.
*/
func coefficientSNR(coefficient float64, variance float64) (float64, error) {
	operation := statistic.NewCoefficientSNR()
	var snr float64

	for out := range operation.Next(singlePointer(unsafe.Pointer(&statistic.CoefficientSNRPair{
		Coefficient: coefficient,
		Variance:    variance,
	}))) {
		snr = *(*float64)(out)
	}

	if err := operation.Error(); err != nil {
		return 0, fmt.Errorf("relation: coefficient snr: %w", err)
	}

	return snr, nil
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
