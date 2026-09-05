package learning

import (
	"time"

	"github.com/theapemachine/errnie"
)

/*
RewardMark observes a cumulative numerical objective. Differences in Value are
rewards; Value may increase, decrease or remain unchanged. The caller defines
the objective and keeps its units and reference consistent across samples.

At is the sample's production time, not its delivery time. Version is its
monotonically increasing producer version. One ledger observes one sequence.
*/
type RewardMark struct {
	At      time.Time `json:"at"`
	Version uint64    `json:"version"`
	Value   float64   `json:"value"`
}

/*
RewardOutcome separates observed reward from differential learning feedback.
The rate is cumulative reward divided by actual elapsed seconds, not the
average of per-transition rates. Differential is Reward minus PriorRate times
Elapsed.Seconds(), using only the rate known before this transition.

HasPriorRate and HasRate distinguish unavailable rates from measured zero.
Differential is usable only when HasPriorRate is true. A negative prior rate
can make a negative reward positive relative feedback; Reward retains the
original sign. Rates describe the observed sequence and do not estimate the
outcomes of alternatives which have never been observed.
*/
type RewardOutcome struct {
	From         RewardMark    `json:"from"`
	Through      RewardMark    `json:"through"`
	Elapsed      time.Duration `json:"elapsedNs"`
	Reward       float64       `json:"reward"`
	TotalReward  float64       `json:"totalReward"`
	TotalElapsed time.Duration `json:"totalElapsedNs"`
	PriorRate    float64       `json:"priorRate"`
	HasPriorRate bool          `json:"hasPriorRate"`
	Differential float64       `json:"differential"`
	Rate         float64       `json:"rate"`
	HasRate      bool          `json:"hasRate"`
	Transitions  uint64        `json:"transitions"`
}

/*
RewardLedger measures changes and elapsed-time rates for one numerical
objective. The first mark establishes the reference; later marks report its
changes. Identical redelivery is idempotent. Cumulative values preserve total
reward across skipped intermediate samples without reconstructing them.

The caller serializes Measure and keeps separate sequences in separate ledgers.
This is an average-reward measurement, not an action selector, value function
or exploration mechanism. An unchanged objective has zero reward.
*/
type RewardLedger struct {
	last    RewardMark
	initial RewardMark
	outcome RewardOutcome
}

/*
Measure observes one producer sample. A newer version at the same timestamp
can change the objective without inventing elapsed time.
Malformed identities and backwards production time leave the ledger unchanged.
*/
func (ledger *RewardLedger) Measure(mark RewardMark) (RewardOutcome, error) {
	if mark.At.IsZero() || mark.Version == 0 {
		return RewardOutcome{}, errnie.Err(
			errnie.Validation, "reward: producer time and version are required", nil,
		)
	}

	if ledger.last.Version == 0 {
		ledger.last, ledger.initial = mark, mark
		ledger.outcome = RewardOutcome{From: mark, Through: mark}

		return ledger.outcome, nil
	}

	if mark.Version == ledger.last.Version && mark.At.Equal(ledger.last.At) &&
		mark.Value == ledger.last.Value {
		return ledger.outcome, nil
	}

	if mark.Version <= ledger.last.Version || mark.At.Before(ledger.last.At) {
		return RewardOutcome{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"reward: sample version must advance and production time cannot move backwards",
			nil,
		))
	}

	outcome := RewardOutcome{
		From:         ledger.last,
		Through:      mark,
		Elapsed:      mark.At.Sub(ledger.last.At),
		Reward:       mark.Value - ledger.last.Value,
		TotalElapsed: mark.At.Sub(ledger.initial.At),
		PriorRate:    ledger.outcome.Rate,
		HasPriorRate: ledger.outcome.HasRate,
		Transitions:  ledger.outcome.Transitions + 1,
	}
	outcome.TotalReward = mark.Value - ledger.initial.Value

	if outcome.HasPriorRate {
		outcome.Differential = outcome.Reward - outcome.PriorRate*outcome.Elapsed.Seconds()
	}

	if outcome.TotalElapsed > 0 {
		outcome.HasRate = true
		outcome.Rate = outcome.TotalReward / outcome.TotalElapsed.Seconds()
	}

	ledger.last, ledger.outcome = mark, outcome

	return outcome, nil
}
