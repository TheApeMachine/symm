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
	thesis     *strategy.Thesis
	replay     *manifold.ReplayRecorder
	tree       *dmt.Tree
	uiHub      chan []byte
}

func NewAnalyzer(tree *dmt.Tree, uiHub chan []byte) *Analyzer {
	return &Analyzer{
		engine:     manifold.NewEngine(),
		resonances: &sync.Map{},
		causals:    &sync.Map{},
		thesis:     strategy.NewThesis(),
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
) {
	symbol := strings.TrimSpace(row.Symbol)

	if symbol == "" {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"logic analyzer: level3 symbol required",
			nil,
		))

		return
	}

	thesis := analyzer.thesis
	slot, err := analyzer.engine.Admit(symbol, thesis)

	if err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "logic analyzer: field admission failed", err))
		return
	}

	if _, ok := analyzer.causals.Load(symbol); !ok {
		analyzer.causals.Store(symbol, NewCausal(symbol, thesis, analyzer.uiHub))
	}

	result := slot.Process(row, pricePrecision, qtyPrecision, book)
	// We don't overwrite analyzer.thesis with result.Thesis because it's the same pointer.

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
			analyzer.resonances.Store(symbol, NewResonance(symbol, thesis, analyzer.uiHub))
		}

		resonanceFound, _ := analyzer.resonances.Load(symbol)
		resonance := resonanceFound.(*Resonance)
		resonance.Update()

		causalFound, _ := analyzer.causals.Load(symbol)
		causal := causalFound.(*Causal)
		causal.Update()
	}
}

/*
PendingThesis returns the single multi-symbol thesis accumulated since the last call,
and resets the analyzer's thesis for the next tick.
*/
func (analyzer *Analyzer) PendingThesis() *strategy.Thesis {
	thesis := analyzer.thesis
	analyzer.thesis = strategy.NewThesis()
	return thesis
}
