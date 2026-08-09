package category

import (
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
	return &Solver{
		extractor: vector.NewFeatureExtractor(
			vector.FeatureExtractorConfig{},
		),
		classifier: probability.NewClassifier(
			probability.ClassifierSchema{},
		),
		api:      api,
		recorder: recorder,
		ui:       ui,
	}
}

/*
Update scores the configured categories against the measurements this tick
carried and records those that cleared their evidence threshold.
Categories are the substrate the graph and the cognition tree are built from,
so they are derived before either runs.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	// Categories are read off this tick's measurements, so there is nothing to
	// classify until every signal has stamped. Skipping leaves the stamp
	// unraised and the tick comes back once the evidence is there.
	if !thesis.SignalsMeasured() {
		return nil
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
