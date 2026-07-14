package logic

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
Analyzer coordinates the composed analysis responsibilities after every signal
has measured the current Thesis. Level3 owns book-derived analysis while the
Analyzer builds each symbol's evidence topology with Gonum.
*/
type Analyzer struct {
	gate   stageGate
	status types.Status
	level3 *Level3
}

/*
stageGate keeps Analyzer dependent on readiness state rather than the Booter's
asynchronous UI publisher, which remains owned by system startup.
*/
type stageGate interface {
	Ready(system.StageType) bool
}

/*
NewAnalyzer composes the book processor required by the analysis stage and is
ready immediately because neither responsibility initializes asynchronously.
*/
func NewAnalyzer(
	ctx context.Context,
	gate stageGate,
	source level3Source,
) *Analyzer {
	return &Analyzer{
		gate:   gate,
		status: types.READY,
		level3: NewLevel3(ctx, source),
	}
}

/*
Initialize marks the synchronous Analyzer ready for Thesis processing.
*/
func (analyzer *Analyzer) Initialize() error {
	errnie.Info("initializing analyzer")
	analyzer.status = types.READY

	return nil
}

/*
Status reports whether Analyzer itself can accept evidence.
*/
func (analyzer *Analyzer) Status() types.Status {
	return analyzer.status
}

/*
Close releases the composed Level3 processor and its field resources.
*/
func (analyzer *Analyzer) Close() {
	if analyzer.level3 != nil {
		analyzer.level3.Close()
	}
}

/*
Update delegates book analysis after signals measure, then composes every
measurement—including Level3 resonance evidence—into its symbol Gonum graph.
*/
func (analyzer *Analyzer) Update(thesis *types.Thesis) {
	if analyzer.level3 != nil && analyzer.gate.Ready(system.StagePreflight) {
		analyzer.level3.Update(thesis)
	}

	for _, measurement := range thesis.Measurements {
		if measurement == nil || measurement.Symbol == "" {
			continue
		}

		evidenceGraph := thesis.Graphs[measurement.Symbol]

		if evidenceGraph == nil {
			evidenceGraph = types.NewGraph(measurement.Symbol)
			thesis.Graphs[measurement.Symbol] = evidenceGraph
		}

		evidenceGraph.AddNode(measurement)
	}

	for _, evidenceGraph := range thesis.Graphs {
		evidenceGraph.Compose()
	}
}
