package trader

import (
	"strings"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/manifold"
)

/*
recoveringDelta reports whether a row belongs to a symbol waiting for a fresh
snapshot. Deltas from the failed continuity interval cannot repair local state.
*/
func (level3 *Level3) recoveringDelta(row kraken.Level3Data) bool {
	_, recovering := level3.recovering[row.Symbol]
	return recovering && !strings.EqualFold(row.Type, "snapshot")
}

/*
recover enters one per-symbol recovery transition, resets local state, and
requests one fresh Kraken snapshot. Further failures cannot create a request
storm while the symbol remains in this state.
*/
func (level3 *Level3) recover(symbol string, reason manifold.InvalidReason) {
	if reason == manifold.Valid {
		return
	}

	if _, recovering := level3.recovering[symbol]; recovering {
		return
	}

	level3.recovering[symbol] = reason
	level3.scheduler.Remove(symbol)
	level3.book.Reset(symbol)

	if err := level3.instrument.RefreshLevel3(symbol); err != nil {
		level3.fail(err)
	}
}

/*
recovered clears recovery only after a fresh snapshot has validated and become
schedulable.
*/
func (level3 *Level3) recovered(row kraken.Level3Data) {
	if !strings.EqualFold(row.Type, "snapshot") {
		return
	}

	delete(level3.recovering, row.Symbol)
}
