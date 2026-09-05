package category

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	nomagique_probability "github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
Solver converts heterogeneous signal measurements into a discrete market-regime
distribution for one symbol. It consumes Measurement objects from every signal,
maintains a per-symbol causal evidence state, evaluates the declared category
vocabulary, and publishes a ranked category batch whose first element is the
dominant regime token consumed by logic/cognition.

It is a pure interpretation stage: signals measure, Category resolves what the
measurements jointly support. It never predicts, never consults Cognition, and
never lets signal publication cadence count as extra evidence.
*/
type Solver struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	status     *runtime.Status
	categories []types.CategoryType
	states     sync.Map
	// version is the monotonic committed-classification revision. It is local
	// Category state, distinct from transport identity and venue event time.
	version       atomic.Uint64
	ObserveModule func(string, time.Duration)
}

/*
coordinate identifies one category-evidence input: the signal source plus the
metric name string, optionally side-qualified in the metric name. It is the unit
of latest-state replacement — one coordinate is one current vote.
*/
type coordinate struct {
	Source string
	Metric string
}

/*
evidenceItem is the current eligible state of one coordinate: its category
affinity, estimator maturity, and provenance identity. Freshness is one for a
resident latest observation: Category does not invent a universal wall-clock
expiry for heterogeneous metrics. Source-specific volume clocks and validity
facts remain measurements for semantic consumers.
*/
type evidenceItem struct {
	Affinity   float64
	Maturity   float64
	At         time.Time
	Freshness  float64
	Supporting string
}

/*
categoryState is one symbol's current evidence snapshot. It holds exactly one
current affinity per evidence coordinate, so memory is bounded by
O(symbols × schema coordinates) and publication frequency never inflates votes.

The coordinates map is guarded by mu. Step runs on runtime handlers whose lane
ownership does not guarantee single-goroutine access to one symbol — a symbol's
measurements can arrive on distinct producer rings, each with its own handler
group, so two goroutines may mutate and read the same state concurrently.
*/
type categoryState struct {
	mu          sync.Mutex
	coordinates map[coordinate]evidenceItem
}

/*
NewSolver creates the category solver. The declared vocabulary is the distinct
set of categories appearing in types.CategorySchemas, in deterministic
types.CategoryOrder order.
*/
func NewSolver(ctx context.Context) *Solver {
	ctx, cancel := context.WithCancel(ctx)

	categories := distinctCategories(types.CategorySchemas)

	solver := &Solver{
		ctx:        ctx,
		cancel:     cancel,
		status:     runtime.NewStatus().Transition(runtime.READY),
		categories: categories,
	}

	return solver
}

func (solver *Solver) Name() string { return "category" }

func (solver *Solver) Error() error { return solver.err }

/*
Step folds every signal measurement populated on this envelope into its
symbol's evidence snapshot. The envelope is one committed observation: all of
its measurements are applied before Category publishes one distribution.
*/
func (solver *Solver) Step(envelope *types.Envelope) *types.Envelope {
	if solver.err != nil {
		solver.cancel()

		return nil
	}

	if envelope == nil {
		return nil
	}

	measurements := envelope.SignalMeasurements()
	envelope.Categories = solver.stepMeasurements(measurements[:])

	if solver.err != nil {
		return nil
	}

	return envelope
}

/*
StepMeasurement consumes one measurement observation, updates the symbol's
evidence snapshot, and runs the schema classifier against the latest
coordinates. It replaces the current coordinate rather than accumulating
votes: each measurement is an observation of current state that sets or
updates the value of any coordinate it carries; it never appends another
independent vote.
*/
func (solver *Solver) StepMeasurement(measurement *data.Measurement[float64]) []types.Category {
	measurements := [1]*data.Measurement[float64]{measurement}

	return solver.stepMeasurements(measurements[:])
}

/*
stepMeasurements commits one runtime observation. Commit order is the causal
clock at this fan-in; measurement timestamps remain provenance and are only
compared inside the observation that produced them.
*/
func (solver *Solver) stepMeasurements(
	measurements []*data.Measurement[float64],
) []types.Category {
	if solver.err != nil {
		solver.cancel()

		return nil
	}

	if len(measurements) == 0 {
		return nil
	}

	started := time.Now()
	defer func() {
		if solver.ObserveModule != nil {
			solver.ObserveModule("category", time.Since(started))
		}
	}()

	var symbol string
	var at time.Time
	var state *categoryState
	var failure error
	var validCount int

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		if measurement.Err != nil {
			if failure == nil {
				failure = measurement.Err
			}

			continue
		}

		validCount++

		if measurement.Symbol() == "" || measurement.At.IsZero() {
			solver.fail("category: measurement symbol and event time required", nil)

			return nil
		}

		if symbol == "" {
			symbol = measurement.Symbol()
			at = measurement.At
			state = solver.symbolState(symbol)
		}

		if measurement.Symbol() != symbol || !measurement.At.Equal(at) {
			solver.fail("category: envelope requires one symbol and event time", nil)

			return nil
		}
	}

	if validCount == 0 && failure != nil {
		solver.fail("category: signal measurement failed", failure)

		return nil
	}

	if failure != nil {
		errnie.Warn(fmt.Sprintf(
			"[category] skipped failed signal measurement: %v", failure,
		))
	}

	if state == nil {
		return nil
	}

	state.mu.Lock()

	for _, measurement := range measurements {
		if measurement == nil || measurement.Err != nil {
			continue
		}

		if err := solver.accumulateLocked(state, measurement); err != nil {
			state.mu.Unlock()
			solver.fail("category: invalid measurement", err)

			return nil
		}
	}

	byCategory, measured := solver.aggregateLocked(state)
	state.mu.Unlock()

	if !measured {
		return nil
	}

	categories, err := solver.classify(symbol, at, byCategory)

	if err != nil {
		solver.fail("category: classification failed", err)

		return nil
	}

	solver.version.Add(1)

	return categories
}

/*
Version returns the monotonic committed-classification revision of Category's
shared evidence state. It is not an external observation identity.
*/
func (solver *Solver) Version() uint64 {
	if solver == nil {
		return 0
	}

	return solver.version.Load()
}

func (solver *Solver) symbolState(symbol string) *categoryState {
	loaded, _ := solver.states.LoadOrStore(symbol, &categoryState{
		coordinates: make(map[coordinate]evidenceItem),
	})

	return loaded.(*categoryState)
}

/*
accumulate replaces the current affinity of every coordinate the measurement
carries. Latest-state replacement means a repeated publication of the same
coordinate overwrites rather than accumulates, so arrival cadence is not
evidence.
*/
func (solver *Solver) accumulate(
	state *categoryState,
	measurement *data.Measurement[float64],
) error {
	state.mu.Lock()
	defer state.mu.Unlock()

	return solver.accumulateLocked(state, measurement)
}

func (solver *Solver) accumulateLocked(
	state *categoryState,
	measurement *data.Measurement[float64],
) error {
	if measurement == nil {
		return fmt.Errorf("measurement is required")
	}

	if !measurement.From.IsZero() && measurement.From.After(measurement.At) {
		return fmt.Errorf(
			"%s %s/%s interval begins at %s after event time %s",
			measurement.ID,
			measurement.Source,
			measurement.Symbol(),
			measurement.From.Format(time.RFC3339Nano),
			measurement.At.Format(time.RFC3339Nano),
		)
	}

	for _, schema := range types.CategorySchemas {
		if string(schema.Source) != measurement.Source {
			continue
		}

		sample, exists := measurement.Metrics[schema.Metric]

		if !exists {
			continue
		}

		key := coordinate{Source: measurement.Source, Metric: schema.Metric}
		affinity := sample.Raw

		if sample.Normalized != nil {
			affinity = *sample.Normalized
		}

		if affinity <= 0 {
			// A non-positive affinity provides no positive support. It is
			// still a current reading of the coordinate, so it must not be
			// left as a stale positive vote; drop the coordinate.
			delete(state.coordinates, key)

			continue
		}

		state.coordinates[key] = evidenceItem{
			Affinity:   affinity,
			Maturity:   measurement.Maturity,
			At:         measurement.At,
			Freshness:  1,
			Supporting: string(schema.Source) + ":" + schema.Metric,
		}
	}

	return nil
}

/*
classify builds the ranked category batch from the current evidence snapshot.
Strength per category is the geometric mean of its currently supporting
affinities; confidence is its symmetric-one-pseudocount evidence share across the
whole vocabulary; surprisal is -log2(confidence).
*/
func (solver *Solver) classify(
	symbol string,
	at time.Time,
	byCategory map[types.CategoryType][]evidenceItem,
) ([]types.Category, error) {
	strengths := make([]float64, len(solver.categories))

	for index, category := range solver.categories {
		items := byCategory[category]

		if len(items) == 0 {
			strengths[index] = 0
			continue
		}

		strength, err := categoryStrength(items)

		if err != nil {
			return nil, err
		}

		strengths[index] = strength
	}

	return solver.buildBatch(symbol, at, strengths, byCategory)
}

/*
categoryStrength is the geometric mean of a category's current positive
affinities, folded by the shared probability.Geomean reduction. The geometric
mean is the right aggregate here because affinities combine multiplicatively:
one near-zero affinity should drag the category's strength down rather than
being averaged away by its stronger siblings.
*/
func categoryStrength(items []evidenceItem) (float64, error) {
	affinities := make([]nmtypes.Scalar, len(items))

	for index, item := range items {
		affinities[index] = nmtypes.Scalar(item.Affinity)
	}

	return float64(nomagique_probability.Geomean(affinities)), nil
}

/*
lift converts a strength vector into the carrier the probability reductions
fold over, so they reduce it without knowing category identity.
*/
func lift(strengths []float64) []nmtypes.Scalar {
	values := make([]nmtypes.Scalar, len(strengths))

	for index, strength := range strengths {
		values[index] = nmtypes.Scalar(strength)
	}

	return values
}

/*
aggregate groups current evidence items by category, in a deterministic order
that mirrors the declared vocabulary.
*/
func (solver *Solver) aggregate(
	state *categoryState,
) (map[types.CategoryType][]evidenceItem, bool) {
	state.mu.Lock()
	defer state.mu.Unlock()

	return solver.aggregateLocked(state)
}

func (solver *Solver) aggregateLocked(
	state *categoryState,
) (map[types.CategoryType][]evidenceItem, bool) {

	byCategory := make(map[types.CategoryType][]evidenceItem)

	measured := false

	for _, schema := range types.CategorySchemas {
		item, found := state.coordinates[coordinate{
			Source: string(schema.Source),
			Metric: schema.Metric,
		}]

		if !found {
			continue
		}

		byCategory[schema.Category] = append(byCategory[schema.Category], item)
		measured = true
	}

	return byCategory, measured
}

/*
buildBatch assembles the ranked category batch: entry zero is the dominant
regime (highest confidence, CategoryOrder tie-break), alternatives follow in
descending confidence order, and every entry carries its strength, confidence,
maturity, surprising, supporting provenance, and the single distribution-level
uncertainty shared by the whole batch.
*/
func (solver *Solver) buildBatch(
	symbol string,
	at time.Time,
	strengths []float64,
	byCategory map[types.CategoryType][]evidenceItem,
) ([]types.Category, error) {
	count := len(solver.categories)

	evidence := lift(strengths)
	confidences := make([]float64, count)

	// The batch shares one uncertainty: how evenly the evidence is spread
	// across the whole declared vocabulary.
	uncertainty := float64(nomagique_probability.ShannonAmbiguity(evidence))

	for index := range solver.categories {
		confidences[index] = float64(
			nomagique_probability.EvidenceShare(evidence, index),
		)
	}

	categories := make([]types.Category, 0, count)

	for index, category := range solver.categories {
		confidence := confidences[index]
		maturity := maturityOf(byCategory[category])

		categories = append(categories, types.Category{
			At:          at,
			Symbol:      symbol,
			Type:        category,
			Confidence:  confidence,
			Surprisal:   -math.Log2(confidence),
			Strength:    strengths[index],
			Maturity:    maturity,
			Freshness:   freshnessOf(byCategory[category]),
			Uncertainty: uncertainty,
			Supporting:  supportingIdentities(byCategory[category]),
		})
	}

	sort.SliceStable(categories, func(left int, right int) bool {
		if categories[left].Confidence != categories[right].Confidence {
			return categories[left].Confidence > categories[right].Confidence
		}

		return types.CategoryOrderLess(categories[left].Type, categories[right].Type)
	})

	return categories, nil
}

func (solver *Solver) fail(message string, err error) {
	solver.err = errnie.Error(errnie.Err(errnie.Validation, message, err))
	solver.status.Transition(runtime.FATAL)
	solver.cancel()
}

/*
shannonAmbiguity returns the normalized Shannon entropy U = H / log2(K) over the
category evidence-share distribution, bounded to [0,1]. Low U means evidence
concentrates on few regimes; high U means the measurements do not distinguish
competing regimes. It is a distribution-level quantity, identical for every
category in one batch, and is not 1 - Confidence.
*/
func shannonAmbiguity(confidences []float64) float64 {
	if len(confidences) <= 1 {
		return 0
	}

	entropy := 0.0

	for _, probability := range confidences {
		if probability <= 0 {
			continue
		}

		entropy -= probability * math.Log2(probability)
	}

	maximum := math.Log2(float64(len(confidences)))

	if maximum <= 0 {
		return 0
	}

	ambiguity := entropy / maximum

	if ambiguity < 0 {
		return 0
	}

	if ambiguity > 1 {
		return 1
	}

	return ambiguity
}

func (solver *Solver) Close() error {
	solver.cancel()
	return nil
}

/*
distinctCategories returns the declared vocabulary in types.CategoryOrder
order, deduplicated, so the classifier indices are stable and deterministic.
*/
func distinctCategories(schemas []types.CategorySchema) []types.CategoryType {
	seen := make(map[types.CategoryType]bool)
	result := make([]types.CategoryType, 0)

	appendCategory := func(category types.CategoryType) {
		if seen[category] {
			return
		}

		seen[category] = true
		result = append(result, category)
	}

	for _, schema := range schemas {
		appendCategory(schema.Category)
	}

	for _, category := range types.CategoryOrder {
		appendCategory(category)
	}

	return result
}

/*
maturityOf returns the weakest estimator support among the supporting items, or
zero when nothing reports maturity. Zero maturity when items exists means none
reported it, kept distinct from a measured zero (which cannot happen for
positive affinity).
*/
func maturityOf(items []evidenceItem) float64 {
	if len(items) == 0 {
		return 0
	}

	maturity := 1.0

	for _, item := range items {
		if item.Maturity < maturity {
			maturity = item.Maturity
		}
	}

	return maturity
}

func freshnessOf(items []evidenceItem) float64 {
	if len(items) == 0 {
		return 0
	}

	freshness := 1.0

	for _, item := range items {
		if item.Freshness < freshness {
			freshness = item.Freshness
		}
	}

	return freshness
}

/*
supportingIdentities returns the sorted, de-duplicated evidence coordinates that
supported the category.
*/
func supportingIdentities(items []evidenceItem) []string {
	if len(items) == 0 {
		return nil
	}

	if len(items) == 1 {
		return []string{items[0].Supporting}
	}

	result := make([]string, 0, len(items))

	for _, item := range items {
		found := false

		for _, existing := range result {
			if existing == item.Supporting {
				found = true
				break
			}
		}

		if !found {
			result = append(result, item.Supporting)
		}
	}

	sort.Strings(result)

	return result
}
