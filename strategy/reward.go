package strategy

import (
	"github.com/theapemachine/symm/hindsight"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/learning"
)

/*
EquityMark is an account valuation after paid execution costs. Actual marks use
the broker's authoritative valuation; virtual marks require displayed liquidation
depth and fees. Equity and NetFunding use the same currency.
NetFunding is cumulative signed external funding in the producer session;
trading cash movements and fees are not funding. HasFunding distinguishes a
known zero from unavailable funding information.

At and Version identify the valuation at its producer. They must not be
substituted with the time or version of a later delivery carrying this mark.
*/
type EquityMark = hindsight.AccountMark

/*
AccountReward projects one account's valuations into a numerical objective.
It owns the funding adjustment and account-mark identity; the composed learner
owns reward differences and elapsed-time rates. Reward and TotalReward in its
output are net profit in the account currency. Negative equity remains a loss.

The first mark establishes capital and funding references. Skipped valuations
do not lose intervening funding adjustments. Each virtual account and producer
session requires its own AccountReward; the caller serializes Measure.
*/
type AccountReward struct {
	initial EquityMark
	last    EquityMark
	ledger  learning.RewardLedger
}

/*
Measure removes external funding from equity changes before numerical learning.
Unknown funding and rewritten account marks are errors, even when a rewrite
would leave the numerical objective unchanged. Invalid marks leave both the
account reference and learner unchanged.
*/
func (reward *AccountReward) Measure(mark EquityMark) (learning.RewardOutcome, error) {
	if !mark.HasFunding {
		return learning.RewardOutcome{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"account reward: known net funding is required",
			nil,
		))
	}

	if reward.last.Version != 0 && mark.Version == reward.last.Version &&
		(!mark.At.Equal(reward.last.At) || mark.Equity != reward.last.Equity ||
			mark.NetFunding != reward.last.NetFunding) {

		return learning.RewardOutcome{}, errnie.Error(errnie.Err(
			errnie.Validation, "account reward: a producer valuation cannot be rewritten", nil,
		))
	}

	initial := reward.initial

	if reward.last.Version == 0 {
		initial = mark
	}

	outcome, err := reward.ledger.Measure(learning.RewardMark{
		At: mark.At, Version: mark.Version,
		Value: (mark.Equity - initial.Equity) - (mark.NetFunding - initial.NetFunding),
	})

	if err != nil {
		return learning.RewardOutcome{}, errnie.Error(errnie.Err(
			errnie.Internal,
			"account reward: failed to measure valuation",
			err,
		))
	}

	reward.initial, reward.last = initial, mark

	return outcome, nil
}
