package logic

import (
	"strings"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
Analyzer coordinates the L3 manifold path with resonance and causal logic.
Operational state flows directly between stages; durable graphs and forecasts
are appended to the tick Thesis passed in by the planner.
*/
type Analyzer struct {
	gate       stageGate
	engine     *manifold.Engine
	status     types.Status
	resonances map[string]*Resonance
	causals    map[string]*Causal
	replay     *manifold.ReplayRecorder
	uiHub      chan []byte
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
	return &Analyzer{
		gate:       gate,
		engine:     manifold.NewEngine(),
		status:     types.READY,
		resonances: map[string]*Resonance{},
		causals:    map[string]*Causal{},
		replay:     manifold.NewReplayRecorder(),
		uiHub:      uiHub,
	}
}

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

func (analyzer *Analyzer) Close() {
	analyzer.engine.Close()
}

/*
Update runs logic stages against the current tick thesis after signals measure.
*/
func (analyzer *Analyzer) Update(thesis *types.Thesis) {
	symbols := make(map[string]struct{})

	for _, measurement := range thesis.Measurements {
		if measurement != nil && measurement.Symbol != "" {
			symbols[measurement.Symbol] = struct{}{}
		}
	}

	for symbol := range symbols {
		analyzer.composeGraph(thesis, symbol)

		composer := CategoryComposer{}
		composer.Compose(thesis, symbol)
	}
}

func (analyzer *Analyzer) composeGraph(thesis *types.Thesis, symbol string) {
	graph := types.NewGraph(symbol)

	for _, measurement := range thesis.Measurements {
		graph.AddNode(measurement)
	}

	graph.Compose()
	thesis.Graphs[symbol] = graph
}

/*
IngestLevel3 preserves the synchronous ingest contract by observing the row and
immediately advancing its symbol when the population is ready.
*/
func (analyzer *Analyzer) IngestLevel3(
	thesis *types.Thesis,
	row kraken.Level3Data,
	pricePrecision int,
	qtyPrecision int,
	book manifold.Level3Book,
) {
	if !analyzer.gate.Ready(system.StagePreflight) {
		return
	}

	result := analyzer.ObserveLevel3(thesis, row, pricePrecision, qtyPrecision, book)

	if result.AdvanceReady {
		analyzer.AdvanceLevel3(thesis, row.Symbol)
	}
}

/*
ObserveLevel3 applies one authoritative L3 row without doing GPU work. The
typed result preserves the reason an observation cannot be scheduled.
*/
func (analyzer *Analyzer) ObserveLevel3(
	thesis *types.Thesis,
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
	analyzer.handle(symbol, thesis, result)

	return result
}

/*
AdvanceLevel3 evolves one admitted symbol from its latest accumulated
population and publishes the resulting typed state.
*/
func (analyzer *Analyzer) AdvanceLevel3(thesis *types.Thesis, symbol string) {
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

	analyzer.handle(symbol, thesis, slot.Advance())
}

func (analyzer *Analyzer) admit(symbol string) (*manifold.Slot, bool) {
	slot, err := analyzer.engine.Admit(symbol)

	if err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "logic analyzer: field admission failed", err))
		return nil, false
	}

	if analyzer.resonances[symbol] == nil {
		analyzer.resonances[symbol] = NewResonance(
			symbol, analyzer.uiHub, analyzer.engine.Halflife(),
		)
	}

	if analyzer.causals[symbol] == nil {
		analyzer.causals[symbol] = NewCausal(symbol, analyzer.uiHub)
	}

	return slot, true
}

func (analyzer *Analyzer) handle(
	symbol string,
	thesis *types.Thesis,
	result manifold.ProcessResult,
) {
	if result.StateProduced {
		analyzer.publish(result.State)
	}

	if result.Forecast != nil {
		thesis.Forecasts = append(thesis.Forecasts, *result.Forecast)
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

	if resonance := analyzer.resonances[symbol]; resonance != nil {
		thesis.Measurements = append(
			thesis.Measurements,
			resonance.Update(result.State)...,
		)
	}

	if causal := analyzer.causals[symbol]; causal != nil {
		hypothesis, produced, err := causal.Update(result.State)

		if err != nil {
			errnie.Error(err)
			return
		}

		if produced {
			thesis.Hypotheses = append(thesis.Hypotheses, hypothesis)
		}
	}
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
