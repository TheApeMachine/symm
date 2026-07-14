package logic

import (
	"sort"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
Analyzer composes every signal measurement on the current Thesis into
symbol-local evidence graphs. Its existing L3 path may add measurements,
forecasts, and hypotheses to that same Thesis without defining the rest of the
analysis pipeline around one market-data source.
*/
type Analyzer struct {
	gate       stageGate
	engine     *manifold.Engine
	status     types.Status
	resonances map[string]*Resonance
	causals    map[string]*Causal
	replay     *manifold.ReplayRecorder
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
func NewAnalyzer(gate stageGate) *Analyzer {
	return &Analyzer{
		gate:       gate,
		engine:     manifold.NewEngine(),
		status:     types.READY,
		resonances: map[string]*Resonance{},
		causals:    map[string]*Causal{},
		replay:     manifold.NewReplayRecorder(),
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
Close releases the existing L3 engine owned by the Analyzer.
*/
func (analyzer *Analyzer) Close() {
	analyzer.engine.Close()
}

/*
Update runs logic stages against the current tick thesis after signals measure.
*/
func (analyzer *Analyzer) Update(thesis *types.Thesis) {
	if value, exists := thesis.Signals.Load("level3"); exists {
		rows, ok := value.([]kraken.Level3Data)

		if !ok {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"logic analyzer: level3 signal has invalid type",
				nil,
			))
		}

		if ok {
			analyzer.IngestLevel3(thesis, rows)
		}
	}

	for _, measurement := range thesis.Measurements {
		if measurement == nil || measurement.Symbol == "" {
			continue
		}

		graph := thesis.Graphs[measurement.Symbol]

		if graph == nil {
			graph = types.NewGraph(measurement.Symbol)
			thesis.Graphs[measurement.Symbol] = graph
		}

		graph.AddNode(measurement)
	}

	for _, graph := range thesis.Graphs {
		graph.Compose()
	}
}

/*
IngestLevel3 applies every authoritative row in the tick, then advances each
mutated symbol once so a transport burst becomes one coherent field epoch.
*/
func (analyzer *Analyzer) IngestLevel3(
	thesis *types.Thesis,
	rows []kraken.Level3Data,
) {
	if !analyzer.gate.Ready(system.StagePreflight) {
		return
	}

	mutated := map[string]*manifold.Slot{}

	for _, row := range rows {
		symbol := strings.TrimSpace(row.Symbol)

		if symbol == "" {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"logic analyzer: level3 symbol required",
				nil,
			))

			continue
		}

		slot, ready := analyzer.admit(symbol)

		if !ready {
			continue
		}

		result := slot.Observe(row)

		if result.StateProduced {
			thesis.Manifold = append(thesis.Manifold, result.State)
		}

		if result.AdvanceReady {
			mutated[symbol] = slot
		}
	}

	symbols := make([]string, 0, len(mutated))

	for symbol := range mutated {
		symbols = append(symbols, symbol)
	}

	sort.Strings(symbols)

	for _, symbol := range symbols {
		result := mutated[symbol].Advance()
		resonance, causal := analyzer.handle(symbol, thesis, result)

		if result.StateProduced {
			thesis.Manifold = append(thesis.Manifold, result.State)
		}

		if resonance != nil {
			thesis.Resonance = append(thesis.Resonance, *resonance)
		}

		if causal != nil {
			thesis.Causal = append(thesis.Causal, *causal)
		}
	}
}

/*
admit obtains the existing L3 slot for a symbol and initializes its persistent
resonance and causal models when the symbol first reaches that path.
*/
func (analyzer *Analyzer) admit(symbol string) (*manifold.Slot, bool) {
	slot, err := analyzer.engine.Admit(symbol)

	if err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "logic analyzer: field admission failed", err))
		return nil, false
	}

	if analyzer.resonances[symbol] == nil {
		analyzer.resonances[symbol] = NewResonance(
			symbol, analyzer.engine.Halflife(),
		)
	}

	if analyzer.causals[symbol] == nil {
		analyzer.causals[symbol] = NewCausal(symbol)
	}

	return slot, true
}

/*
handle appends the durable outputs produced by one L3 advance to the same
Thesis used by the measurement-wide analysis path.
*/
func (analyzer *Analyzer) handle(
	symbol string,
	thesis *types.Thesis,
	result manifold.ProcessResult,
) (*ResonanceOutcome, *CausalOutcome) {
	var resonanceOutcome *ResonanceOutcome
	var causalOutcome *CausalOutcome

	if result.Forecast != nil {
		thesis.Forecasts = append(thesis.Forecasts, *result.Forecast)

		if err := thesis.Transition(
			symbol, types.LifecycleShaped, result.Forecast.At,
		); err != nil {
			errnie.Error(err)
		}
	}

	if !result.GasReady {
		return nil, nil
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
		measurements, outcome := resonance.Update(result.State)
		thesis.Measurements = append(
			thesis.Measurements,
			measurements...,
		)

		resonanceOutcome = outcome
	}

	if causal := analyzer.causals[symbol]; causal != nil {
		hypothesis, outcome, err := causal.Update(result.State)

		if err != nil {
			errnie.Error(err)
			return resonanceOutcome, nil
		}

		if outcome != nil {
			thesis.Hypotheses = append(thesis.Hypotheses, hypothesis)
		}

		causalOutcome = outcome
	}

	return resonanceOutcome, causalOutcome
}
