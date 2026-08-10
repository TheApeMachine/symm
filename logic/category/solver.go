package category

import (
	"slices"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/nomagique/vector"
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
	extractor  *vector.FeatureExtractor
	classifier *probability.Classifier
	inputs     []string
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
	inputs := make([]string, len(types.CategorySchemas))
	categories := make([]types.CategoryType, 0, len(types.CategorySchemas))
	categoryNames := make([]string, 0, len(types.CategorySchemas))

	for index, schema := range types.CategorySchemas {
		inputs[index] = string(schema.Source) + ":" + types.MetricKey(
			schema.Metric,
			schema.Side,
		)

		if slices.Contains(categories, schema.Category) {
			continue
		}

		categories = append(categories, schema.Category)
		categoryNames = append(categoryNames, string(schema.Category))
	}

	return &Solver{
		extractor: vector.NewFeatureExtractor(
			vector.FeatureExtractorConfig{
				FeatureScopeConfig: vector.FeatureScopeConfig{
					Root:   ".",
					Inputs: inputs,
				},
			},
		),
		classifier: probability.NewClassifier(
			probability.ClassifierSchema{Categories: categoryNames},
		),
		inputs:     inputs,
		categories: categories,
		api:        api,
		recorder:   recorder,
		ui:         ui,
	}
}

/*
Update scores the configured categories against the measurements this tick
carried and records one classified artifact per symbol.
Categories are the substrate the graph and the cognition tree are built from,
so they are derived before either runs.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	if thesis.Stamped(types.SourceCategories) || !thesis.SignalsMeasured() {
		return nil
	}

	var err error

	thesis.Symbols.Range(func(key, value any) bool {
		symbol := value.(*types.Symbol)

		for _, measurement := range symbol.Measurements {
			for _, schema := range types.CategorySchemas {
				if schema.Source != measurement.Source {
					continue
				}

				category, err := solver.classifier.Classify(probability.ClassifierInput{
					Scores: []probability.CategoryScore{{
						Category: string(schema.Category),
						Score:    measurement.Uncertainty.Confidence,
					}},
				})

				if err != nil {
					err = err
					return false
				}

				thesis.Categories.Store(symbol, []probability.ScoreResult{category})

				return true
			}
		}

		return true
	})

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"category: failed to classify categories - "+err.Error(),
			err,
		))
	}

	thesis.Stamp(types.SourceCategories)

	return nil
}

/*
Close releases the solver. Categories are derived per tick and hold no
resources of their own.
*/
func (solver *Solver) Close() error {
	return nil
}
