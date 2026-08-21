package category

import (
	"context"
	"iter"
	"math"
	"slices"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/transport"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
Solver derives categories from the measurements every signal contributed this
tick. A category is a hypothesis about what the market is doing, and each
metric that carries affinity is typed evidence for or against it.
*/
type Solver struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	thesis     *types.Thesis
	classifier *probability.Classifier
	categories []types.CategoryType
	api        *websocket.API
	recorder   *audit.Recorder
	ui         *transport.MapReduce[*types.UIFrame]
	work       *transport.Consumer[*types.Symbol]
	pool       *types.SymbolPool
}

/*
NewSolver creates a new Solver for the category logic.
*/
func NewSolver(
	ctx context.Context,
	thesis *types.Thesis,
	api *websocket.API,
	ui *transport.MapReduce[*types.UIFrame],
	recorder *audit.Recorder,
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

	solver := &Solver{
		ctx:    ctx,
		cancel: cancel,
		thesis: thesis,
		classifier: probability.NewClassifier(
			probability.ClassifierSchema{Categories: categoryNames},
		),
		categories: categories,
		api:        api,
		recorder:   recorder,
		ui:         ui,
		pool:       types.NewSymbolPool(types.ShardWorkers()),
	}
	solver.work = transport.NewConsumer[*types.Symbol](solver.Name(), solver.consume)
	thesis.Work(types.SourceCategory).Register(solver.work)

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
func (solver *Solver) consume() {
	go func() {
		defer func() {
			if err := solver.pool.Error(); err != nil {
				solver.err = err
			}

			solver.thesis.Fail(solver.err)
		}()

		for symbol := range solver.thesis.Work(types.SourceCategory).Drain(
			solver.work, nil,
		) {
			select {
			case <-solver.ctx.Done():
				solver.pool.CaptureError(solver.ctx.Err())
				return
			default:
			}

			if symbol == nil {
				continue
			}

			symbolName := symbol.Symbol

			solver.pool.Submit(symbolName, func() {
				if err := solver.consumeSymbol(symbol); err != nil {
					solver.pool.CaptureError(errnie.Error(errnie.Err(
						errnie.Internal,
						"category: failed to classify symbol",
						err,
					).With("symbol", symbolName)))
				}
			})
		}
	}()
}

func (solver *Solver) consumeSymbol(symbol *types.Symbol) error {
	consumer := symbol.MeasurementConsumers[types.MeasurementConsumerCategory]

	if symbol.Measurements.Length(consumer) == 0 {
		return nil
	}

	categories, measured, err := solver.classify(
		symbol.Symbol,
		symbol.MarketMeasurements(consumer),
	)

	if err != nil {
		return err
	}

	if !measured {
		return nil
	}

	for index := range categories {
		categories[index].At = solver.thesis.At
	}

	symbol.Categories.Push(categories)

	return nil
}

func (solver *Solver) classify(
	symbol string,
	measurements iter.Seq[*nmtypes.Measurement],
) ([]types.Category, bool, error) {
	evidence := make(map[types.CategoryType][]float64, len(solver.categories))
	supporting := make(map[types.CategoryType][]string, len(solver.categories))
	maturity := make(map[types.CategoryType]float64, len(solver.categories))
	maturitySet := make(map[types.CategoryType]bool, len(solver.categories))
	measured := false

	for measurement := range measurements {
		measured = true

		for _, schema := range types.CategorySchemas {
			if string(schema.Source) != measurement.Source {
				continue
			}

			metricKey := types.MetricKey(schema.Metric, schema.Side)
			sample, exists := measurement.Metrics[metricKey]

			if !exists || sample.Normalized == nil || *sample.Normalized <= 0 {
				continue
			}

			evidence[schema.Category] = append(
				evidence[schema.Category], *sample.Normalized,
			)

			supporting[schema.Category] = append(
				supporting[schema.Category], string(schema.Source)+":"+metricKey,
			)

			if !maturitySet[schema.Category] {
				maturity[schema.Category] = measurement.Maturity
				maturitySet[schema.Category] = true
				continue
			}

			maturity[schema.Category] = min(
				maturity[schema.Category], measurement.Maturity,
			)
		}
	}

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

	if solver.pool != nil {
		solver.pool.Close()
	}

	return nil
}
