package logic

import (
	"strings"
	"sync"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

type Analyzer struct {
	theses     *sync.Map
	manifolds  *sync.Map
	resonances *sync.Map
	causals    *sync.Map
	tree       *dmt.Tree
	uiHub      chan []byte
}

func NewAnalyzer(tree *dmt.Tree, uiHub chan []byte) *Analyzer {
	analyzer := &Analyzer{
		theses:     &sync.Map{},
		manifolds:  &sync.Map{},
		resonances: &sync.Map{},
		causals:    &sync.Map{},
		tree:       tree,
		uiHub:      uiHub,
	}

	return analyzer
}

func (analyzer *Analyzer) Close() {
	analyzer.manifolds.Range(func(key, value any) bool {
		manifold := value.(*Manifold)
		manifold.Close()
		return true
	})

	analyzer.resonances.Range(func(key, value any) bool {
		resonance := value.(*Resonance)
		resonance.Close()
		return true
	})
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

	for symbol, rows := range grouped {
		found, _ := analyzer.theses.LoadOrStore(symbol, strategy.NewThesis())
		thesis := found.(*strategy.Thesis)

		if _, ok := analyzer.manifolds.Load(symbol); !ok {
			analyzer.theses.Store(symbol, thesis)
			analyzer.manifolds.Store(symbol, NewManifold(thesis, analyzer.tree, analyzer.uiHub))
			analyzer.causals.Store(symbol, NewCausal(thesis, analyzer.uiHub))
		}

		thesis.AddEvidence("symbol", symbol)

		manifoldFound, _ := analyzer.manifolds.Load(symbol)
		manifold := manifoldFound.(*Manifold)
		thesis = manifold.Update(rows)

		if _, ok := thesis.Evidence("manifold"); ok {
			if _, ok := analyzer.resonances.Load(symbol); !ok {
				analyzer.resonances.Store(symbol, NewResonance(thesis, analyzer.uiHub))
			}

			resonanceFound, _ := analyzer.resonances.Load(symbol)
			resonance := resonanceFound.(*Resonance)
			thesis = resonance.Update()
		}

		causalFound, _ := analyzer.causals.Load(symbol)
		causal := causalFound.(*Causal)
		thesis = causal.Update()
		analyzer.theses.Store(symbol, thesis)
	}

	theses := map[string]*strategy.Thesis{}

	analyzer.theses.Range(func(key, value any) bool {
		thesis := value.(*strategy.Thesis)
		theses[key.(string)] = thesis
		return true
	})

	return theses
}
