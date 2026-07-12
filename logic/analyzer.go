package logic

import (
	"strings"
	"sync"
	"sync/atomic"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

type Analyzer struct {
	booter     *system.Booter
	engine     *manifold.Engine
	status     atomic.Value
	resonances *sync.Map
	causals    *sync.Map
	thesis     *strategy.Thesis
	replay     *manifold.ReplayRecorder
	tree       *dmt.Tree
	uiHub      chan []byte
}

func NewAnalyzer(
	booter *system.Booter, tree *dmt.Tree, uiHub chan []byte,
) *Analyzer {
	return &Analyzer{
		booter:     booter,
		engine:     manifold.NewEngine(),
		status:     atomic.Value{},
		resonances: &sync.Map{},
		causals:    &sync.Map{},
		thesis:     strategy.NewThesis(),
		replay:     manifold.NewReplayRecorder(),
		tree:       tree,
		uiHub:      uiHub,
	}
}

func (analyzer *Analyzer) Status() types.Status {
	return analyzer.status.Load().(types.Status)
}

func (analyzer *Analyzer) Close() {
	analyzer.engine.Close()
}

/*
IngestLevel3 preserves the synchronous ingest contract by observing the row and
immediately advancing its symbol when the population is ready.
*/
func (analyzer *Analyzer) IngestLevel3(
	row kraken.Level3Data,
	pricePrecision int,
	qtyPrecision int,
	book manifold.Level3Book,
) {
	if !analyzer.booter.Ready(system.StagePreflight) {
		return
	}

	result := analyzer.ObserveLevel3(row, pricePrecision, qtyPrecision, book)

	if result.AdvanceReady {
		analyzer.AdvanceLevel3(row.Symbol)
	}
}

/*
ObserveLevel3 applies one authoritative L3 row without doing GPU work. The
typed result preserves the reason an observation cannot be scheduled.
*/
func (analyzer *Analyzer) ObserveLevel3(
	row kraken.Level3Data,
	pricePrecision int,
	qtyPrecision int,
	book manifold.Level3Book,
) manifold.ProcessResult {
	if !analyzer.booter.Ready(system.StagePreflight) {
		return manifold.ProcessResult{}
	}

	symbol := strings.TrimSpace(row.Symbol)

	if symbol == "" {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"logic analyzer: level3 symbol required",
			nil,
		))

		return manifold.ProcessResult{}
	}

	slot, ready := analyzer.admit(symbol)

	if !ready {
		return manifold.ProcessResult{}
	}

	result := slot.Observe(row, pricePrecision, qtyPrecision, book)
	analyzer.handle(symbol, result)

	return result
}

/*
AdvanceLevel3 evolves one admitted symbol from its latest accumulated
population and publishes the resulting typed state.
*/
func (analyzer *Analyzer) AdvanceLevel3(symbol string) {
	if !analyzer.booter.Ready(system.StagePreflight) {
		return
	}

	symbol = strings.TrimSpace(symbol)
	slot, ok := analyzer.engine.Slot(symbol)

	if !ok {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"logic analyzer: manifold slot not admitted",
			nil,
		))

		return
	}

	analyzer.handle(symbol, slot.Advance())
}

func (analyzer *Analyzer) admit(symbol string) (*manifold.Slot, bool) {
	slot, err := analyzer.engine.Admit(symbol, analyzer.thesis)

	if err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "logic analyzer: field admission failed", err))
		return nil, false
	}

	if _, ok := analyzer.causals.Load(symbol); !ok {
		analyzer.causals.Store(symbol, NewCausal(symbol, analyzer.thesis, analyzer.uiHub))
	}

	return slot, true
}

func (analyzer *Analyzer) handle(symbol string, result manifold.ProcessResult) {
	if result.StateProduced {
		analyzer.publish(result.State)
	}

	if !result.GasReady {
		return
	}

	if !analyzer.replay.Record(
		symbol,
		result.Observation,
		result.State,
		result.Accounting,
		result.CohortCount,
		result.OrderCount,
		result.DepositCount,
	) {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"logic analyzer: replay frame dropped",
			nil,
		))
	}

	analyzer.update(symbol)
}

func (analyzer *Analyzer) update(symbol string) {
	if _, ok := analyzer.resonances.Load(symbol); !ok {
		analyzer.resonances.Store(
			symbol,
			NewResonance(symbol, analyzer.thesis, analyzer.uiHub),
		)
	}

	resonanceFound, _ := analyzer.resonances.Load(symbol)
	resonanceFound.(*Resonance).Update()
	causalFound, _ := analyzer.causals.Load(symbol)
	causalFound.(*Causal).Update()
}

/*
publish emits the typed manifold state without remapping it into a UI DTO.
The top-level key is only the websocket route used by the frontend store.
*/
func (analyzer *Analyzer) publish(state manifold.State) {
	if analyzer.uiHub == nil {
		return
	}

	select {
	case analyzer.uiHub <- datura.Map[any]{"manifold": state}.Marshal():
	default:
		errnie.Error(errnie.Err(
			errnie.IO,
			"logic analyzer: UI channel full while publishing manifold state",
			nil,
		))
	}
}

/*
PendingThesis returns the live multi-symbol thesis shared by the manifold,
resonance, causal, and strategy stages.
*/
func (analyzer *Analyzer) PendingThesis() *strategy.Thesis {
	if !analyzer.booter.Ready(system.StageReady) {
		return nil
	}

	return analyzer.thesis
}
