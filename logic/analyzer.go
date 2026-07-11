package logic

import (
	"strings"
	"sync"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/strategy"
)

type Analyzer struct {
	engine     *manifold.Engine
	resonances *sync.Map
	causals    *sync.Map
	theses     *sync.Map
	replay     *manifold.ReplayRecorder
	tree       *dmt.Tree
	uiHub      chan []byte
}

func NewAnalyzer(tree *dmt.Tree, uiHub chan []byte) *Analyzer {
	return &Analyzer{
		engine:     manifold.NewEngine(),
		resonances: &sync.Map{},
		causals:    &sync.Map{},
		theses:     &sync.Map{},
		replay:     manifold.NewReplayRecorder(),
		tree:       tree,
		uiHub:      uiHub,
	}
}

func (analyzer *Analyzer) Close() {
	analyzer.engine.Close()
}

/*
IngestLevel3 applies one authoritative L3 row to the population ledger and
advances the shared GPU field slot for that symbol.
*/
func (analyzer *Analyzer) IngestLevel3(
	row kraken.Level3Data,
	pricePrecision int,
	qtyPrecision int,
	book manifold.Level3Book,
) map[string]*strategy.Thesis {
	symbol := strings.TrimSpace(row.Symbol)

	if symbol == "" {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"logic analyzer: level3 symbol required",
			nil,
		))

		return nil
	}

	thesis := analyzer.thesisFor(symbol)
	slot, err := analyzer.engine.Admit(symbol, thesis)

	if err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "logic analyzer: field admission failed", err))
		return nil
	}

	if _, ok := analyzer.causals.Load(symbol); !ok {
		analyzer.causals.Store(symbol, NewCausal(thesis, analyzer.uiHub))
	}

	result := slot.Process(row, pricePrecision, qtyPrecision, book)
	thesis = result.Thesis

	if result.GasReady {
		result.ReplayPushed = analyzer.replay.Record(
			symbol,
			row,
			result.State,
			result.Accounting,
			result.CohortCount,
			result.OrderCount,
			result.DepositCount,
		)

		if _, ok := analyzer.resonances.Load(symbol); !ok {
			analyzer.resonances.Store(symbol, NewResonance(thesis, analyzer.uiHub))
		}

		resonanceFound, _ := analyzer.resonances.Load(symbol)
		resonance := resonanceFound.(*Resonance)
		thesis = resonance.Update()

		causalFound, _ := analyzer.causals.Load(symbol)
		causal := causalFound.(*Causal)
		thesis = causal.Update()
	}

	analyzer.theses.Store(symbol, thesis)

	return map[string]*strategy.Thesis{symbol: thesis}
}

func (analyzer *Analyzer) thesisFor(symbol string) *strategy.Thesis {
	found, ok := analyzer.theses.Load(symbol)

	if ok {
		return found.(*strategy.Thesis)
	}

	thesis := strategy.NewThesis()
	analyzer.theses.Store(symbol, thesis)

	return thesis
}
