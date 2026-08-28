package category

import (
	"context"
	"math"
	"sort"
	"sync"
	"time"

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
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	categories    []types.CategoryType
	states        sync.Map
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
affinity, maturity, and the provenance identity.
*/
type evidenceItem struct {
	Affinity   float64
	Maturity   float64
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
func NewSolver(ctx context.Context, bus *runtime.Workspace) *Solver {
	ctx, cancel := context.WithCancel(ctx)

	categories := distinctCategories(types.CategorySchemas)

	solver := &Solver{
		ctx:        ctx,
		cancel:     cancel,
		categories: categories,
	}

	if bus != nil {
		runtime.Register(
			bus,
			func(measurement *data.Measurement[float64]) string {
				if measurement == nil {
					return ""
				}

				return measurement.Symbol()
			},
			func(measurement *data.Measurement[float64]) []types.Category {
				if measurement == nil {
					return nil
				}

				return solver.Step(measurement.ToTypesMeasurement())
			},
		)
	}

	return solver
}

func (solver *Solver) Name() string { return "category" }

func (solver *Solver) Error() error { return solver.err }

/*
Step consumes one measurement observation, updates the symbol's evidence
snapshot, and runs the schema classifier against the latest coordinates. It
replaces the current coordinate rather than accumulating votes: each
measurement is an observation of current state that sets or updates the
value of any coordinate it carries; it never appends another independent vote.
*/
func (solver *Solver) Step(measurement *nmtypes.Measurement) []types.Category {
	if measurement == nil || measurement.Symbol == "" {
		return nil
	}

	started := time.Now()
	defer func() {
		if solver.ObserveModule != nil {
			solver.ObserveModule("category", time.Since(started))
		}
	}()

	state := solver.symbolState(measurement.Symbol)
	solver.accumulate(state, measurement)

	categories, measured, err := solver.classify(measurement.Symbol, state)

	if err != nil {
		solver.err = err
		return nil
	}

	if !measured {
		return nil
	}

	return categories
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
func (solver *Solver) accumulate(state *categoryState, measurement *nmtypes.Measurement) {
	state.mu.Lock()
	defer state.mu.Unlock()

	for _, schema := range types.CategorySchemas {
		if string(schema.Source) != measurement.Source {
			continue
		}

		sample, exists := measurement.Metrics[schema.Metric]

		if !exists {
			continue
		}

		affinity := sample.Raw

		if sample.Normalized != nil {
			affinity = *sample.Normalized
		}

		if affinity <= 0 {
			// A non-positive affinity provides no positive support. It is
			// still a current reading of the coordinate, so it must not be
			// left as a stale positive vote; drop the coordinate.
			delete(state.coordinates, coordinate{
				Source: measurement.Source,
				Metric: schema.Metric,
			})

			continue
		}

		state.coordinates[coordinate{
			Source: measurement.Source,
			Metric: schema.Metric,
		}] = evidenceItem{
			Affinity:   affinity,
			Maturity:   measurement.Maturity,
			Supporting: string(schema.Source) + ":" + schema.Metric,
		}
	}
}

/*
classify builds the ranked category batch from the current evidence snapshot.
Strength per category is the geometric mean of its currently supporting
affinities; confidence is its symmetric-one-pseudocount evidence share across the
whole vocabulary; surprisal is -log2(confidence).
*/
func (solver *Solver) classify(
	symbol string,
	state *categoryState,
) ([]types.Category, bool, error) {
	byCategory, measured := solver.aggregate(state)

	if !measured {
		return nil, false, nil
	}

	strengths := make([]float64, len(solver.categories))

	for index, category := range solver.categories {
		items := byCategory[category]

		if len(items) == 0 {
			strengths[index] = 0
			continue
		}

		strengths[index] = categoryStrength(items)
	}

	categories := solver.buildBatch(symbol, strengths, byCategory)

	return categories, true, nil
}

/*
categoryStrength is the geometric mean of a category's current positive
affinities, expressed through the shared probability.Geomean primitive. The
solver lifts the affinities into a Frame of sample slots, steps the atomic
primitive, and projects the result back out.
*/
func categoryStrength(items []evidenceItem) float64 {
	frame := nmtypes.Frame{}

	for index, item := range items {
		frame.Put(nmtypes.MustSampleSymbol(index), item.Affinity)
	}

	result := nmtypes.Step(nomagique_probability.Geomean, frame)

	if result.Err != nil {
		return 0
	}

	return result.MustGet(nomagique_probability.SymbolResult)
}

/*
lift projects a strength vector into a Frame of generic sample slots, so the
probability primitives can reduce it without knowing category identity.
*/
func lift(strengths []float64) nmtypes.Frame {
	frame := nmtypes.Frame{}

	for index, strength := range strengths {
		frame.Put(nmtypes.MustSampleSymbol(index), strength)
	}

	return frame
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
	strengths []float64,
	byCategory map[types.CategoryType][]evidenceItem,
) []types.Category {
	count := len(solver.categories)

	evidence := lift(strengths)
	confidences := make([]float64, count)

	ambiguityFrame := nmtypes.Step(nomagique_probability.ShannonAmbiguity(), evidence)
	uncertainty := 0.0

	if ambiguityFrame.Err == nil {
		uncertainty = ambiguityFrame.MustGet(nomagique_probability.SymbolAmbiguity)
	}

	for index := range solver.categories {
		// Preselect the winner so EvidenceShare resolves this category's share.
		selected := evidence
		selected.Put(nomagique_probability.SymbolWinner, float64(index))

		result := nmtypes.Step(nomagique_probability.EvidenceShare(), selected)

		confidence := 1.0 / float64(count)

		if result.Err == nil {
			confidence = result.MustGet(nomagique_probability.SymbolConfidence)
		}

		confidences[index] = confidence
	}

	categories := make([]types.Category, 0, count)

	for index, category := range solver.categories {
		confidence := confidences[index]
		maturity := maturityOf(byCategory[category])

		categories = append(categories, types.Category{
			Symbol:      symbol,
			Type:        category,
			Confidence:  confidence,
			Surprisal:   -math.Log2(confidence),
			Strength:    strengths[index],
			Maturity:    maturity,
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

	return categories
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

/*
supportingIdentities returns the sorted, de-duplicated evidence coordinates that
supported the category.
*/
func supportingIdentities(items []evidenceItem) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(items))

	for _, item := range items {
		if seen[item.Supporting] {
			continue
		}

		seen[item.Supporting] = true
		result = append(result, item.Supporting)
	}

	sort.Strings(result)

	return result
}
