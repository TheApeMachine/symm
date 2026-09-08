package reward

import (
	"time"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

/* Primitive adapts Primitive marks to the canonical typed objective ledger. */
type Primitive struct {
	core.PrimitiveError
	ledger  Ledger
	seed    core.Primitive
	current core.Primitive
}

/*
New measures each mark in a finite delivery run. Marks contain at (int64
Unix nanoseconds), version (uint64), and value (float64). The typed ledger owns
all accounting; records exist only at the Primitive boundary.
*/
func New() core.Primitive {
	return transport.NewMap(&Primitive{seed: transport.NewIO(core.From(Outcome{}))})
}

/* Next measures one mark and retains its immutable output until the next run. */
func (reward *Primitive) Next(input core.Primitive) core.Primitive {
	result := core.Yield(reward.seed, input,
		func(previous Outcome, fields map[string]core.Primitive) Outcome {
			decoder := core.NewDecoder(fields)
			mark := Mark{
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

	outcome := core.To[Outcome](result)
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
func (reward *Primitive) Read() any { return core.To[any](reward.current) }
