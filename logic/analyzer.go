package logic

import (
	"strings"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/ui"
)

type Analyzer struct {
	theses     map[string]*strategy.Thesis
	manifolds  map[string]*Manifold
	resonances map[string]*Resonance
	causals    map[string]*Causal
	tree       *dmt.Tree
	uiHub      *ui.Hub
}

func NewAnalyzer(thesis *strategy.Thesis, tree *dmt.Tree, uiHub *ui.Hub) *Analyzer {
	analyzer := &Analyzer{
		theses:     map[string]*strategy.Thesis{},
		manifolds:  map[string]*Manifold{},
		resonances: map[string]*Resonance{},
		causals:    map[string]*Causal{},
		tree:       tree,
		uiHub:      uiHub,
	}

	if thesis != nil {
		analyzer.theses[""] = thesis
	}

	return analyzer
}

func (analyzer *Analyzer) Close() {
	for _, manifold := range analyzer.manifolds {
		manifold.Close()
	}

	for _, resonance := range analyzer.resonances {
		resonance.Close()
	}
}

/*
Update turns measurements into particles that "surf" on the phase-directed pilot-wave
driven by the oscillator field underneath the compressed gas fluid.
*/
func (analyzer *Analyzer) Update(
	measurements []*types.Measurement,
) map[string]*strategy.Thesis {
	grouped := map[string][]*types.Measurement{}

	for _, measurement := range measurements {
		symbol := strings.TrimSpace(measurement.Symbol)

		if symbol == "" {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"logic analyzer: measurement symbol required",
				nil,
			))

			continue
		}

		measurement.Symbol = symbol
		grouped[symbol] = append(grouped[symbol], measurement)
	}

	theses := map[string]*strategy.Thesis{}

	for symbol, rows := range grouped {
		thesis := analyzer.theses[symbol]

		if thesis == nil && len(grouped) == 1 {
			thesis = analyzer.theses[""]

			if thesis != nil {
				delete(analyzer.theses, "")
			}
		}

		if thesis == nil {
			thesis = strategy.NewThesis()
		}

		if analyzer.manifolds[symbol] == nil {
			analyzer.theses[symbol] = thesis
			analyzer.manifolds[symbol] = NewManifold(thesis, analyzer.tree)
			analyzer.causals[symbol] = NewCausal(thesis)
		}

		thesis.AddEvidence("symbol", symbol)
		thesis = analyzer.manifolds[symbol].Update(rows)

		if _, ok := thesis.Evidence("manifold"); ok {
			if analyzer.resonances[symbol] == nil {
				analyzer.resonances[symbol] = NewResonance(thesis)
			}

			thesis = analyzer.resonances[symbol].Update()
		}

		thesis = analyzer.causals[symbol].Update()
		analyzer.Publish(symbol, rows, thesis)
		theses[symbol] = thesis
	}

	return theses
}
