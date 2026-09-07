package learning

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
RewardLedger retains the initial and latest objective marks and their measured
outcome. It owns cumulative differences and elapsed-time rates without retaining
evaluation graphs. Producer timestamps retain their monotonic clock component
for ordering and intervals; output timestamps preserve the producer's wall time.
The caller serializes Measure.
*/
type RewardLedger struct {
	initial RewardMark
	last    RewardMark
	outcome RewardOutcome
}

/*
Measure accepts identified objective marks in producer order. Identical
redelivery is idempotent; rewritten or regressed marks cannot mutate evidence.
A same-instant newer version contributes reward without inventing elapsed time.
*/
func (ledger *RewardLedger) Measure(mark RewardMark) (RewardOutcome, error) {
	if mark.Version == 0 || mark.Version < ledger.last.Version ||
		(ledger.last.Version != 0 && mark.At.Before(ledger.last.At)) {
		return RewardOutcome{}, fmt.Errorf("%w: reward mark regressed or has no version: mark=%+v last=%+v", core.ErrDomain, mark, ledger.last)
	}

	if mark.Version == ledger.last.Version {
		if !mark.At.Equal(ledger.last.At) || mark.Value != ledger.last.Value {
			return RewardOutcome{}, fmt.Errorf("%w: reward mark was rewritten: mark=%+v last=%+v", core.ErrDomain, mark, ledger.last)
		}

		return ledger.outcome, nil
	}

	through := mark
	through.At = through.At.UTC()
	outcome := RewardOutcome{From: through, Through: through}

	if ledger.last.Version == 0 {
		ledger.initial, ledger.last, ledger.outcome = mark, mark, outcome
		return outcome, nil
	}

	outcome.From = ledger.outcome.Through
	outcome.Elapsed = mark.At.Sub(ledger.last.At)
	outcome.TotalElapsed = mark.At.Sub(ledger.initial.At)
	outcome.Reward = mark.Value - ledger.last.Value
	outcome.TotalReward = mark.Value - ledger.initial.Value
	outcome.PriorRate = ledger.outcome.Rate
	outcome.HasPriorRate = ledger.outcome.HasRate
	outcome.HasRate = outcome.TotalElapsed > 0
	outcome.Transitions = ledger.outcome.Transitions + 1

	if outcome.HasRate {
		outcome.Rate = outcome.TotalReward / outcome.TotalElapsed.Seconds()
	}

	if outcome.HasPriorRate {
		outcome.Differential = outcome.Reward - outcome.PriorRate*outcome.Elapsed.Seconds()
	}

	ledger.last, ledger.outcome = mark, outcome
	return outcome, nil
}
