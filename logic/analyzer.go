package logic

import (
	"strings"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
Analyzer coordinates the existing L3 manifold path with synchronous numerical
measurement composition. MeasurementAnalyzer is exposed as the composed
responsibility used by the trading loop, avoiding another forwarding method on
an already broad coordinator.
*/
type Analyzer struct {
	Measurements *MeasurementAnalyzer
	gate         stageGate
	engine       *manifold.Engine
	status       types.Status
	resonances   map[string]*Resonance
	causals      map[string]*Causal
	thesis       *strategy.Thesis
	replay       *manifold.ReplayRecorder
	uiHub        chan []byte
}

/*
stageGate keeps Analyzer dependent on readiness state rather than the Booter's
asynchronous UI publisher, which remains owned by system startup.
*/
type stageGate interface {
	Ready(system.StageType) bool
}

/*
NewAnalyzer constructs every synchronous analysis dependency. The analyzer is
ready immediately because no component performs deferred initialization.
*/
func NewAnalyzer(
	gate stageGate, uiHub chan []byte,
) *Analyzer {
	thesis := strategy.NewThesis()

	return &Analyzer{
		Measurements: NewMeasurementAnalyzer(thesis),
		gate:         gate,
		engine:       manifold.NewEngine(),
		status:       types.READY,
		resonances:   map[string]*Resonance{},
		causals:      map[string]*Causal{},
		thesis:       thesis,
		replay:       manifold.NewReplayRecorder(),
		uiHub:        uiHub,
	}
}

/*
Status reports whether Analyzer itself can accept evidence.
*/
func (analyzer *Analyzer) Status() types.Status {
	return analyzer.status
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
	if !analyzer.gate.Ready(system.StagePreflight) {
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
	if !analyzer.gate.Ready(system.StagePreflight) {
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
	if !analyzer.gate.Ready(system.StagePreflight) {
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

	if analyzer.causals[symbol] == nil {
		analyzer.causals[symbol] = NewCausal(symbol, analyzer.thesis, analyzer.uiHub)
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
	if analyzer.resonances[symbol] == nil {
		analyzer.resonances[symbol] = NewResonance(
			symbol,
			analyzer.thesis,
			analyzer.uiHub,
		)
	}

	analyzer.resonances[symbol].Update()
	analyzer.causals[symbol].Update()
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
	return analyzer.thesis
}
