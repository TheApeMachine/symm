package category

import (
	"fmt"
	"iter"
	"math"
	"slices"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Solver derives categories from the measurements every signal contributed this
tick. A category is a hypothesis about what the market is doing, and each
metric that carries affinity is typed evidence for or against it.
*/
type Solver struct {
	classifier *probability.Classifier
	categories []types.CategoryType
	api        *websocket.API
	recorder   *audit.Recorder
	ui         chan []byte
}

/*
NewSolver creates a new Solver for the category logic.
*/
func NewSolver(
	api *websocket.API,
	ui chan []byte,
	recorder *audit.Recorder,
) *Solver {
	categories := make([]types.CategoryType, 0, len(types.CategorySchemas))
	categoryNames := make([]string, 0, len(types.CategorySchemas))

	for _, schema := range types.CategorySchemas {
		if slices.Contains(categories, schema.Category) {
			continue
		}

		categories = append(categories, schema.Category)
		categoryNames = append(categoryNames, string(schema.Category))
	}

	return &Solver{
		classifier: probability.NewClassifier(
			probability.ClassifierSchema{Categories: categoryNames},
		),
		categories: categories,
		api:        api,
		recorder:   recorder,
		ui:         ui,
	}
}

func (solver *Solver) Name() string {
	return "category"
}

/*
Update scores the configured categories against the measurements this tick
carried and records one classified artifact per symbol.
Categories are the substrate the graph and the cognition tree are built from,
so they are derived before either runs.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	var classificationErr error

	thesis.Symbols.Range(func(key, value any) bool {
		symbolName, nameOK := key.(string)
		symbol, symbolOK := value.(*types.Symbol)

		if !nameOK || symbolName == "" || !symbolOK || symbol == nil {
			classificationErr = errnie.Error(errnie.Err(
				errnie.Internal,
				fmt.Sprintf(
					"category: failed to classify symbol - invalid key or value: %v, %v",
					key, value,
				),
				nil,
			))

			return false
		}

		categories, measured, err := solver.classify(
			symbolName,
			symbol.MarketMeasurements("category"),
		)

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				fmt.Sprintf(
					"category: failed to classify symbol %s - %v",
					symbolName, err,
				),
				err,
			))
			classificationErr = err
			return false
		}

		if measured {
			for index := range categories {
				categories[index].At = thesis.At
			}

			symbol.Categories.Store(symbolName, categories)
			return true
		}

		return true
	})

	if classificationErr != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"category: failed to classify categories - "+classificationErr.Error(),
			classificationErr,
		))
	}

	return nil
}

func (solver *Solver) classify(
	symbol string,
	measurements iter.Seq[*types.Measurement],
) ([]types.Category, bool, error) {
	evidence := make(map[types.CategoryType][]float64, len(solver.categories))
	supporting := make(map[types.CategoryType][]string, len(solver.categories))
	maturity := make(map[types.CategoryType]float64, len(solver.categories))
	maturitySet := make(map[types.CategoryType]bool, len(solver.categories))
	measured := false

	for measurement := range measurements {
		if measurement == nil {
			continue
		}

		measured = true

		for _, schema := range types.CategorySchemas {
			if schema.Source != measurement.Source {
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
	return nil
}
