package strategy

import (
	"sort"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
Thesis is the current multi-symbol migration carrier shared by the existing
manifold runtime. Compatibility evidence remains a bounded live view, while
typed logic epochs and decisions are append-only. It is not yet the specified
one-symbol lifecycle; stating that boundary prevents this bridge from being
mistaken for the final PostMortem record.
*/
type Thesis struct {
	evidence  *EvidenceBook
	epochs    *EpochJournal
	decisions *DecisionJournal
}

/*
NewThesis creates empty compatibility evidence, typed epoch, and decision
histories for the active in-process lifecycle.
*/
func NewThesis() *Thesis {
	return &Thesis{
		evidence:  NewEvidenceBook(),
		epochs:    NewEpochJournal(),
		decisions: NewDecisionJournal(),
	}
}

/*
AddEvidence records the current snapshot for an unmigrated or non-measurement
stage. Numerical signal history uses RecordEpochs so source-key replacement
cannot discard distinct metrics from one observation.
*/
func (thesis *Thesis) AddEvidence(symbol string, key string, snapshot any) {
	thesis.evidence.Update(symbol, *NewEvidence(key, symbol, snapshot))
}

/*
Symbols returns all symbols currently tracked in the thesis.
*/
func (thesis *Thesis) Symbols() []string {
	evidenceSymbols := thesis.evidence.Symbols()
	epochSymbols := thesis.epochs.Symbols()

	if len(epochSymbols) == 0 {
		return evidenceSymbols
	}

	if len(evidenceSymbols) == 0 {
		return epochSymbols
	}

	symbolSet := make(map[string]struct{})

	for _, symbol := range evidenceSymbols {
		symbolSet[symbol] = struct{}{}
	}

	for _, symbol := range epochSymbols {
		symbolSet[symbol] = struct{}{}
	}

	symbols := make([]string, 0, len(symbolSet))

	for symbol := range symbolSet {
		symbols = append(symbols, symbol)
	}

	sort.Strings(symbols)

	return symbols
}

/*
Values returns the values for a given Symbol.
*/
func (thesis *Thesis) Values(symbol string) ([]*Evidence, error) {
	loaded, ok := thesis.evidence.Values(symbol)

	if !ok {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound, "symbol not found", nil,
		))
	}

	return loaded, nil
}

/*
Evidence returns the snapshot for a given symbol and key.
*/
func (thesis *Thesis) Evidence(symbol string, key string) (any, bool) {
	return thesis.evidence.Latest(symbol, key)
}

/*
RecordEpochs appends one validated availability-ordered batch of numerical
evidence to the Thesis while each epoch retains its exact market event time.
*/
func (thesis *Thesis) RecordEpochs(epochs []types.LogicEpoch) error {
	return thesis.epochs.Record(epochs...)
}

/*
Epochs returns one symbol's immutable numerical evidence history.
*/
func (thesis *Thesis) Epochs(symbol string) []types.LogicEpoch {
	return thesis.epochs.Epochs(symbol)
}

/*
RecordDecision appends one completed strategy evaluation to the Thesis.
Decisions stay chronological while active evidence remains a bounded latest-value
view, because a postmortem needs the former and live planning needs the latter.
*/
func (thesis *Thesis) RecordDecision(decision Decision) (bool, error) {
	return thesis.decisions.Record(decision)
}

/*
Evaluated reports whether the Thesis already contains an evaluation for a
forecast epoch, preventing a cached forecast from generating repeated actions.
*/
func (thesis *Thesis) Evaluated(
	symbol string,
	forecast types.Forecasts,
) bool {
	return thesis.decisions.Evaluated(symbol, forecast)
}

/*
Decisions returns one symbol's immutable decision history in evaluation order.
*/
func (thesis *Thesis) Decisions(symbol string) []Decision {
	return thesis.decisions.Decisions(symbol)
}

/*
RemoveEvidence invalidates one current stage output.
*/
func (thesis *Thesis) RemoveEvidence(symbol string, key string) {
	thesis.evidence.Delete(symbol, key)
}
