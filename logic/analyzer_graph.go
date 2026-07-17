package logic

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/types"
)

/*
composeGraphs inserts measurements into per-symbol evidence graphs and drops
graphs that produce no edges after Compose.
*/
func (analyzer *Analyzer) composeGraphs(thesis *types.Thesis) {
	graphsStarted := time.Now()

	for _, measurement := range thesis.Measurements {
		if measurement == nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"analyzer received a nil measurement",
				nil,
			))

			continue
		}

		value, found := thesis.Graphs.Load(measurement.Symbol)

		if !found {
			value = types.NewGraph(measurement.Symbol)
			thesis.Graphs.Store(measurement.Symbol, value)
		}

		evidenceGraph := value.(*types.Graph)

		if err := evidenceGraph.AddNode(measurement); err != nil {
			errnie.Error(err)
			continue
		}
	}

	thesis.Graphs.Range(func(key, value any) bool {
		evidenceGraph := value.(*types.Graph)
		evidenceGraph.Compose()

		if len(evidenceGraph.Edges()) == 0 {
			thesis.Graphs.Delete(key)
		}

		return true
	})

	errnie.Error(audit.Phase(analyzer.recorder, thesis.Tick, "graphs", map[string]any{
		"measurements": len(thesis.Measurements),
		"ns":           time.Since(graphsStarted).Nanoseconds(),
	}))
}
