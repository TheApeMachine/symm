package strategy

import (
	"sort"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
EpochJournal retains the typed logic evidence used during an opportunity's
lifecycle. The trading loop is its sole writer, so chronological slices are
sufficient and avoid introducing another coordination layer.
*/
type EpochJournal struct {
	entries map[string][]types.LogicEpoch
}

/*
NewEpochJournal creates an empty typed evidence history.
*/
func NewEpochJournal() *EpochJournal {
	return &EpochJournal{entries: make(map[string][]types.LogicEpoch)}
}

/*
Record validates a complete batch before appending independent epoch values.
Append order is causal availability order: an asynchronously drained feed may
legitimately deliver an older event time later, and preserving that regression
lets logic classify stale-relative evidence instead of terminating the runtime.
*/
func (journal *EpochJournal) Record(epochs ...types.LogicEpoch) error {
	if len(epochs) == 0 {
		return nil
	}

	for _, epoch := range epochs {
		if err := epoch.Validate(); err != nil {
			return errnie.Err(
				errnie.Validation,
				"strategy epoch journal: invalid logic epoch",
				err,
			)
		}
	}

	for _, epoch := range epochs {
		journal.entries[epoch.Symbol] = append(
			journal.entries[epoch.Symbol],
			epoch.Clone(),
		)
	}

	return nil
}

/*
Epochs returns one symbol's typed evidence in causal availability order without
exposing the journal's retained slices. Each epoch retains its independent
event time even when that time regresses across asynchronously drained feeds.
*/
func (journal *EpochJournal) Epochs(symbol string) []types.LogicEpoch {
	entries := journal.entries[symbol]
	epochs := make([]types.LogicEpoch, len(entries))

	for index, epoch := range entries {
		epochs[index] = epoch.Clone()
	}

	return epochs
}

/*
Symbols returns every symbol with typed logic history in deterministic order.
*/
func (journal *EpochJournal) Symbols() []string {
	symbols := make([]string, 0, len(journal.entries))

	for symbol := range journal.entries {
		symbols = append(symbols, symbol)
	}

	sort.Strings(symbols)

	return symbols
}
