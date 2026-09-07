package learning

import (
	"time"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* Reward adapts Primitive marks to the canonical typed objective ledger. */
type Reward struct {
	core.PrimitiveError
	ledger  RewardLedger
	seed    core.Primitive
	current core.Primitive
}

/*
NewReward measures each mark in a finite delivery run. Marks contain at (int64
Unix nanoseconds), version (uint64), and value (float64). The typed ledger owns
all accounting; records exist only at the Primitive boundary.
*/
func NewReward() core.Primitive {
	return transport.NewMap(&Reward{seed: transport.NewIO(core.From(RewardOutcome{}))})
}

/* Next measures one mark and retains its immutable output until the next run. */
func (reward *Reward) Next(input core.Primitive) core.Primitive {
	result := core.Yield(reward.seed, input,
		func(previous RewardOutcome, fields map[string]core.Primitive) RewardOutcome {
			decoder := core.NewDecoder(fields)
			mark := RewardMark{
				At:      time.Unix(0, core.Decode[int64](decoder, "at")).UTC(),
				Version: core.Decode[uint64](decoder, "version"),
				Value:   core.Decode[float64](decoder, "value"),
			}

			if err := decoder.Error(); err != nil {
				reward.Error(err)
				return previous
			}

			outcome, err := reward.ledger.Measure(mark)
			reward.Error(err)
			return outcome
		}, reward,
	)

	if result == nil || reward.Error() != nil {
		return nil
	}

	outcome := core.To[RewardOutcome](result)
	reward.current = core.Record(map[string]any{
		"from": core.Record(map[string]any{
			"at": outcome.From.At.UnixNano(), "version": outcome.From.Version, "value": outcome.From.Value,
		}),
		"through": core.Record(map[string]any{
			"at": outcome.Through.At.UnixNano(), "version": outcome.Through.Version, "value": outcome.Through.Value,
		}),
		"elapsed": outcome.Elapsed.Seconds(), "total_elapsed": outcome.TotalElapsed.Seconds(),
		"reward": outcome.Reward, "total_reward": outcome.TotalReward,
		"prior_rate": outcome.PriorRate, "rate": outcome.Rate, "differential": outcome.Differential,
		"has_prior_rate": outcome.HasPriorRate, "has_rate": outcome.HasRate,
		"transitions": outcome.Transitions,
	})
	return reward.current
}

/* Read exposes the most recent measured record without advancing the ledger. */
func (reward *Reward) Read() any { return core.To[any](reward.current) }
