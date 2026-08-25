package category

import (
	"context"
	"github.com/theapemachine/symm/nomagique/runtime"
	"math"
	"slices"
	"sync"

	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/audit"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
Solver derives categories from the measurements every signal contributed this
tick. A category is a hypothesis about what the market is doing, and each
metric that carries affinity is typed evidence for or against it.
*/
type Solver struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	thesis       *types.Thesis
	classifier   *probability.Classifier
	categories   []types.CategoryType
	recorder     *audit.Recorder
	categoriesCh *runtime.Channel[[]types.Category]
	states       sync.Map
}

/*
categoryState accumulates one symbol's category evidence between measurement
arrivals. Evidence is bounded per category so a hot symbol cannot grow
unbounded state; the freshest window drives the latest category reading.
*/
type categoryState struct {
	evidence    map[types.CategoryType][]float64
	supporting  map[types.CategoryType][]string
	maturity    map[types.CategoryType]float64
	maturitySet map[types.CategoryType]bool
}

/*
NewSolver creates a new Solver for the category logic.
*/
func NewSolver(
	ctx context.Context,
	bus *runtime.Workspace,
) *Solver {
	ctx, cancel := context.WithCancel(ctx)

	categories := make([]types.CategoryType, 0, len(types.CategorySchemas))
	categoryNames := make([]string, 0, len(types.CategorySchemas))

	for _, schema := range types.CategorySchemas {
		if slices.Contains(categories, schema.Category) {
			continue
		}

		categories = append(categories, schema.Category)
		categoryNames = append(categoryNames, string(schema.Category))
	}

	var thesis *types.Thesis
	if bus != nil {
		if shared, found := bus.Shared("thesis", ""); found {
			if t, ok := shared.(*types.Thesis); ok {
				thesis = t
			}
		}
	}

	solver := &Solver{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		classifier: probability.NewClassifier(
			probability.ClassifierSchema{Categories: categoryNames},
		),
		categories: categories,
	}

	solver.categoriesCh = runtime.ChannelOf[[]types.Category](
		bus, types.ChannelCategories,
		func(batch []types.Category) string {
			if len(batch) == 0 {
				return ""
			}
			return batch[0].Symbol
		},
	)
	runtime.ChannelOf[*nmtypes.Measurement](
		bus, types.ChannelMeasurements,
		func(measurement *nmtypes.Measurement) string { return measurement.Symbol },
	).Subscribe(solver.Name(), solver.Step)

	return solver
}

func (solver *Solver) Name() string {
	return "category"
}

func (solver *Solver) Error() error { return solver.err }

/*
Run consumes each symbol's measurement stream from its own input MapReduce (the
symbol Measurements column) and pushes the derived category batches to the
output MapReduce the downstream cognition and graph solvers consume. The
category stage never re-materializes the iterator; each measurement batch is
classified and forwarded inline.
*/
// Step folds one measurement into the symbol's category evidence and
// publishes the freshest category reading downstream.
func (solver *Solver) Step(measurement *nmtypes.Measurement) error {
	if measurement == nil || measurement.Symbol == "" {
		return nil
	}

	state := solver.symbolState(measurement.Symbol)
	solver.accumulate(state, measurement)

	categories, measured, err := solver.classify(measurement.Symbol, state)

	if err != nil {
		solver.err = err
		return err
	}

	if !measured || solver.categoriesCh == nil {
		return nil
	}

	for index := range categories {
		categories[index].At = solver.thesis.At
	}

	solver.categoriesCh.Publish(categories)

	return nil
}

func (solver *Solver) symbolState(symbol string) *categoryState {
	loaded, _ := solver.states.LoadOrStore(symbol, &categoryState{
		evidence:    make(map[types.CategoryType][]float64, len(solver.categories)),
		supporting:  make(map[types.CategoryType][]string, len(solver.categories)),
		maturity:    make(map[types.CategoryType]float64, len(solver.categories)),
		maturitySet: make(map[types.CategoryType]bool, len(solver.categories)),
	})

	return loaded.(*categoryState)
}

func (solver *Solver) accumulate(state *categoryState, measurement *nmtypes.Measurement) {
	for _, schema := range types.CategorySchemas {
		if string(schema.Source) != measurement.Source {
			continue
		}

		sample, exists := measurement.Metrics[schema.Metric]

		if !exists || sample.Normalized == nil || *sample.Normalized <= 0 {
			continue
		}

		state.evidence[schema.Category] = append(
			state.evidence[schema.Category], *sample.Normalized,
		)

		state.supporting[schema.Category] = append(
			state.supporting[schema.Category], string(schema.Source)+":"+schema.Metric,
		)

		if !state.maturitySet[schema.Category] {
			state.maturity[schema.Category] = measurement.Maturity
			state.maturitySet[schema.Category] = true
			continue
		}

		state.maturity[schema.Category] = min(
			state.maturity[schema.Category], measurement.Maturity,
		)
	}
}

func (solver *Solver) classify(
	symbol string,
	state *categoryState,
) ([]types.Category, bool, error) {
	evidence := state.evidence
	supporting := state.supporting
	maturity := state.maturity
	measured := len(evidence) > 0

	if !measured {
		return nil, false, nil
	}

	scores := make([]probability.CategoryScore, 0, len(solver.categories))
	scoreValues := make([]float64, 0, len(solver.categories))
	strengths := make(map[types.CategoryType]float64, len(solver.categories))
	maxStrength := 0.0

	for _, category := range solver.categories {
		strength := 0.0

		if len(evidence[category]) > 0 {
			var err error
			strength, err = probability.EvidenceGeomean(evidence[category]...)

			if err != nil {
				return nil, false, err
			}
		}

		strengths[category] = strength
		maxStrength = max(maxStrength, strength)
		scores = append(scores, probability.CategoryScore{
			Category: string(category),
			Score:    strength,
		})
		scoreValues = append(scoreValues, strength)
	}

	result, err := solver.classifier.Classify(probability.ClassifierInput{
		Scores:   scores,
		Strength: maxStrength,
	})

	if err != nil {
		return nil, false, err
	}

	categoryType := types.CategoryTypeNone

	if maxStrength > 0 {
		categoryType = solver.categories[int(result.Category)-1]
	}

	if categoryType == types.CategoryTypeNone {
		return []types.Category{{
			Symbol:     symbol,
			Type:       categoryType,
			Confidence: result.Confidence,
			Surprisal:  -math.Log2(result.Confidence),
		}}, true, nil
	}

	categories := make([]types.Category, 0, len(solver.categories))
	categories = append(categories, types.Category{
		Symbol:     symbol,
		Type:       categoryType,
		Confidence: result.Confidence,
		Surprisal:  -math.Log2(result.Confidence),
		Strength:   strengths[categoryType],
		Maturity:   maturity[categoryType],
		Supporting: supporting[categoryType],
	})

	for index, category := range solver.categories {
		if category == categoryType || strengths[category] <= 0 {
			continue
		}

		confidence, confidenceErr := probability.CategoryShareConfidence(
			scoreValues,
			index+1,
		)

		if confidenceErr != nil {
			return nil, false, confidenceErr
		}

		categories = append(categories, types.Category{
			Symbol:     symbol,
			Type:       category,
			Confidence: confidence,
			Surprisal:  -math.Log2(confidence),
			Strength:   strengths[category],
			Maturity:   maturity[category],
			Supporting: supporting[category],
		})
	}

	return categories, true, nil
}

/*
Close releases the solver. Categories are derived per tick and hold no
resources of their own.
*/
func (solver *Solver) Close() error {
	solver.cancel()

	return nil
}
