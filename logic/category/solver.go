package category

import (
	"context"
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/equation"
	nomagique_probability "github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/transport"
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
	*runtime.System
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
	coordinates atomic.Pointer[map[coordinate]evidenceItem]
}

func (state *categoryState) Coordinates() map[coordinate]evidenceItem {
	if state == nil {
		return nil
	}

	coordsPtr := state.coordinates.Load()

	if coordsPtr == nil {
		return nil
	}

	out := make(map[coordinate]evidenceItem, len(*coordsPtr))

	for key, val := range *coordsPtr {
		out[key] = val
	}

	return out
}

/*
NewSolver creates the category solver. The declared vocabulary is the distinct
set of categories appearing in types.CategorySchemas, in deterministic
types.CategoryOrder order.
*/
func NewSolver(ctx context.Context) *Solver {
	categories := distinctCategories(types.CategorySchemas)

	solver := &Solver{
		System:     runtime.NewSystem(ctx, "category"),
		categories: categories,
	}

	return solver
}

/*
Step reads all completed prior-stage signal outputs from the StageInput,
groups them by symbol, folds them into per-symbol evidence snapshots, and
writes the resulting category metrics onto the owned output measurement.
*/
func (solver *Solver) Step(input *runtime.StageInput, output *data.Measurement[float64]) *data.Measurement[float64] {
	if solver.Status() != runtime.READY {
		errnie.Warn(solver.Name() + ": Step called before READY; dropping event")
		return nil
	}

	if solver.Error() != nil {
		return nil
	}

	if input == nil {
		return nil
	}

	// Collect all prior-stage producer outputs (Stage 0 signal results).
	priorOutputs := input.AllPriorOutputs()

	// Also include the ingress measurement itself as a fallback for live slots
	// where signals wrote metrics directly onto the shared measurement.
	ingress := input.Ingress()
	sources := make([]*data.Measurement[float64], 0, len(priorOutputs)+1)
	if ingress != nil && ingress.Label != "" && ingress.Err == nil {
		sources = append(sources, ingress)
	}

	for _, prior := range priorOutputs {
		if prior != nil && prior.Label != "" && prior.Err == nil {
			sources = append(sources, prior)
		}
	}

	if len(sources) == 0 {
		return nil
	}

	bySymbol := make(map[string][]*data.Measurement[float64])

	for _, src := range sources {
		bySymbol[src.Label] = append(bySymbol[src.Label], src)
	}

	results := make([][]types.Category, 0, len(bySymbol))

	for _, symbol := range slices.Sorted(maps.Keys(bySymbol)) {
		categories := solver.stepMeasurements(bySymbol[symbol])

		if solver.Error() != nil {
			return nil
		}

		if len(categories) == 0 {
			continue
		}

		results = append(results, categories)
		output.Label = symbol
		output.At = categories[0].At
		maturity := categories[0].Maturity
		snr, snrDefined, estimated := 0.0, false, false
		if categories[0].Uncertainty > 0 {
			snr = categories[0].Confidence / categories[0].Uncertainty
			snrDefined = true
			estimated = true
		}
		output.SetQuality(maturity, snr, snrDefined, estimated)

		for _, cat := range categories {
			if cat.Type != "" {
				output.WriteMetric(string(cat.Type), cat.Confidence)
			}
		}
	}

	if len(results) == 0 {
		return nil
	}

	output.Result = results

	return output
}

func (solver *Solver) Register() *data.Measurement[float64] {
	metrics := make(map[string]data.Metric[float64], len(solver.categories))

	for _, catType := range solver.categories {
		name := string(catType)
		metrics[name] = data.NewMetric[float64](
			name, data.UnitDimensionless, data.TimescaleInstantaneous, 0, 1,
		)
	}

	measurement := data.NewMeasurement("category", metrics)
	measurement.SetMetadata("peer-interest", "*")

	return measurement
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
	if solver.Error() != nil {
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

		if measurement.Label == "" || measurement.At.IsZero() {
			solver.fail("category: measurement symbol and event time required", nil)

			return nil
		}

		if symbol == "" {
			symbol = measurement.Label
			at = measurement.At
			state = solver.symbolState(symbol)
		}

		if measurement.Label != symbol {
			solver.fail("category: envelope requires one symbol", nil)

			return nil
		}

		if measurement.At.After(at) {
			at = measurement.At
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

	var byCategory map[types.CategoryType][]evidenceItem
	var measured bool

	for {
		oldPtr := state.coordinates.Load()
		newCoords := make(map[coordinate]evidenceItem)

		if oldPtr != nil {
			for key, val := range *oldPtr {
				newCoords[key] = val
			}
		}

		skipped := 0

		for _, measurement := range measurements {
			if measurement == nil || measurement.Err != nil {
				continue
			}

			if err := solver.accumulateCoords(newCoords, measurement); err != nil {
				// Soft-skip inverted/out-of-order intervals (From after At) and
				// other coordinate rejects. Never FATAL: a late correlation peer
				// must not take Category READY->ERROR and starve Step with
				// before-READY floods.
				skipped++
				errnie.Warn(fmt.Sprintf(
					"[category] skipped invalid measurement: %v", err,
				))
				continue
			}
		}

		if skipped > 0 && len(newCoords) == 0 {
			// Every peer in this envelope was unusable; nothing to classify.
			return nil
		}

		if state.coordinates.CompareAndSwap(oldPtr, &newCoords) {
			byCategory, measured = solver.aggregateCoords(newCoords)
			break
		}
	}

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
	s := &categoryState{}
	initial := make(map[coordinate]evidenceItem)
	s.coordinates.Store(&initial)

	loaded, _ := solver.states.LoadOrStore(symbol, s)

	return loaded.(*categoryState)
}

func (solver *Solver) accumulate(
	state *categoryState,
	measurement *data.Measurement[float64],
) error {
	for {
		oldPtr := state.coordinates.Load()
		newCoords := make(map[coordinate]evidenceItem)

		if oldPtr != nil {
			for key, val := range *oldPtr {
				newCoords[key] = val
			}
		}

		if err := solver.accumulateCoords(newCoords, measurement); err != nil {
			return err
		}

		if state.coordinates.CompareAndSwap(oldPtr, &newCoords) {
			return nil
		}
	}
}

func (solver *Solver) accumulateCoords(
	coords map[coordinate]evidenceItem,
	measurement *data.Measurement[float64],
) error {
	if measurement == nil {
		return fmt.Errorf("measurement is required")
	}

	if !measurement.From.IsZero() && measurement.From.After(measurement.At) {
		return fmt.Errorf(
			"%d %s/%s interval begins at %s after event time %s",
			measurement.ID,
			measurement.Source,
			measurement.Label,
			measurement.From.Format(time.RFC3339Nano),
			measurement.At.Format(time.RFC3339Nano),
		)
	}

	for _, schema := range types.CategorySchemas {
		if string(schema.Source) != measurement.Source {
			continue
		}

		sample, exists := measurement.LookupMetric(schema.Metric)

		if !exists {
			continue
		}

		key := coordinate{Source: measurement.Source, Metric: schema.Metric}
		affinity := sample.Raw

		if sample.Normalized != nil {
			affinity = *sample.Normalized
		}

		if affinity < 0 {
			affinity = math.Abs(affinity)
		}

		if affinity == 0 {
			// Zero affinity provides no positive support. It is still a current reading
			// of the coordinate, so drop any prior vote.
			delete(coords, key)

			continue
		}

		coords[key] = evidenceItem{
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
	if len(items) == 0 {
		return 0, nil
	}

	affinities := make([]float64, len(items))

	for index, item := range items {
		affinities[index] = item.Affinity
	}

	strengthFold := nomagique_probability.NewGeomean()
	var strength float64

	for out := range strengthFold.Next(transport.NewValues(affinities...).Next(nil)) {
		strength = *(*float64)(out)
	}

	if err := strengthFold.Error(); err != nil {
		return 0, err
	}

	return strength, nil
}

/*
lift converts a strength vector into the carrier the probability reductions
fold over, so they reduce it without knowing category identity.
*/
func lift(strengths []float64) []float64 {
	return append([]float64(nil), strengths...)
}

func (solver *Solver) aggregate(
	state *categoryState,
) (map[types.CategoryType][]evidenceItem, bool) {
	coordsPtr := state.coordinates.Load()

	if coordsPtr == nil {
		return nil, false
	}

	return solver.aggregateCoords(*coordsPtr)
}

func (solver *Solver) aggregateCoords(
	coords map[coordinate]evidenceItem,
) (map[types.CategoryType][]evidenceItem, bool) {

	byCategory := make(map[types.CategoryType][]evidenceItem)

	measured := false

	for _, schema := range types.CategorySchemas {
		item, found := coords[coordinate{
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
	// Category specification section 20 assigns one symmetric pseudocount
	// to every declared category: P(c) = (strength(c)+1)/(sum(strength)+K).
	// Keep raw strength and maturity unchanged; this is competition prior mass.
	for index := range evidence {
		evidence[index] += 1
	}
	confidences := make([]float64, count)

	// The batch shares one uncertainty: how evenly the evidence is spread
	// across the whole declared vocabulary.
	uncertaintyFold := nomagique_probability.NewShannonAmbiguity()
	var uncertainty float64

	for out := range uncertaintyFold.Next(transport.NewValues(evidence...).Next(nil)) {
		uncertainty = *(*float64)(out)
	}

	err := uncertaintyFold.Error()

	if err != nil {
		return nil, err
	}

	for index := range solver.categories {
		selection := transport.NewEvaluate(equation.NewEvidenceShare(index))
		share := 0.0

		for out := range selection.Next(transport.NewOne(unsafe.Pointer(&evidence)).Next(nil)) {
			share = *(*float64)(out)
		}

		if err := selection.Error(); err != nil {
			return nil, err
		}

		confidences[index] = share
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

/*
fail records a hard Category invariant breach and transitions FATAL.
Coordinate-level rejects (inverted From/At, late peers) must soft-skip instead —
see stepMeasurements — so one bad signal cannot freeze the regime path.
*/
func (solver *Solver) fail(message string, err error) {
	solver.Error(errnie.Err(errnie.Validation, message, err))
	solver.Transition(runtime.FATAL)
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
