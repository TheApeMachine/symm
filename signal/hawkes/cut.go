package hawkes

import (
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/types"
)

/*
Cut is one Hawkes publish: the shared Thesis plus an immutable Outcome snapshot.
Analyzer must consume this snapshot, not the live Process, or queued cuts all
collapse onto the latest EventCount and manifold advances starve.
*/
type Cut struct {
	Thesis   *types.Thesis
	outcomes map[string]excitation.Outcome
	symbols  []string
}

/*
SharedThesis returns the Thesis this cut measured into.
*/
func (cut *Cut) SharedThesis() *types.Thesis {
	return cut.Thesis
}

/*
Symbols lists every symbol present in the snapshot.
*/
func (cut *Cut) Symbols() []string {
	if cut == nil {
		return nil
	}

	return append([]string(nil), cut.symbols...)
}

/*
Outcome returns the frozen Hawkes outcome for one symbol.
*/
func (cut *Cut) Outcome(symbol string) (excitation.Outcome, bool) {
	if cut == nil {
		return excitation.Outcome{}, false
	}

	outcome, ok := cut.outcomes[symbol]

	return outcome, ok
}

/*
cut freezes the current Process outcomes onto a publish payload.
*/
func (signal *Signal) cut(thesis *types.Thesis) *Cut {
	symbols := signal.process.Symbols()
	outcomes := make(map[string]excitation.Outcome, len(symbols))

	for _, symbol := range symbols {
		outcome, ok := signal.process.Outcome(symbol)

		if !ok {
			continue
		}

		outcomes[symbol] = outcome
	}

	return &Cut{
		Thesis:   thesis,
		outcomes: outcomes,
		symbols:  append([]string(nil), symbols...),
	}
}
