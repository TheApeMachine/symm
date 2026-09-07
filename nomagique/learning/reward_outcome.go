package learning

import (
	"github.com/theapemachine/symm/nomagique/core"
	"time"
)

/* RewardMark is the serialized identity/value of one measured objective. */
type RewardMark struct {
	At      time.Time
	Version uint64
	Value   float64
}

/* RewardOutcome is a Go projection; RewardLedger exclusively owns numerical accounting. */
type RewardOutcome struct {
	From         RewardMark
	Through      RewardMark
	Elapsed      time.Duration
	Reward       float64
	TotalElapsed time.Duration
	TotalReward  float64
	PriorRate    float64
	Rate         float64
	Differential float64
	HasPriorRate bool
	HasRate      bool
	Transitions  uint64
}

/* ProjectReward converts exact event timestamps and graph rates to the wire-facing DTO. */
func ProjectReward(fields map[string]core.Primitive) (RewardOutcome, error) {
	decoder := core.NewDecoder(fields)
	from := core.NewDecoder(core.Decode[map[string]core.Primitive](decoder, "from"))
	through := core.NewDecoder(core.Decode[map[string]core.Primitive](decoder, "through"))
	outcome := RewardOutcome{
		From:         RewardMark{At: time.Unix(0, core.Decode[int64](from, "at")).UTC(), Version: core.Decode[uint64](from, "version"), Value: core.Decode[float64](from, "value")},
		Through:      RewardMark{At: time.Unix(0, core.Decode[int64](through, "at")).UTC(), Version: core.Decode[uint64](through, "version"), Value: core.Decode[float64](through, "value")},
		Reward:       core.Decode[float64](decoder, "reward"),
		TotalReward:  core.Decode[float64](decoder, "total_reward"),
		PriorRate:    core.Decode[float64](decoder, "prior_rate"),
		Rate:         core.Decode[float64](decoder, "rate"),
		Differential: core.Decode[float64](decoder, "differential"),
		HasPriorRate: core.Decode[bool](decoder, "has_prior_rate"),
		HasRate:      core.Decode[bool](decoder, "has_rate"),
		Transitions:  core.Decode[uint64](decoder, "transitions"),
	}
	// Sub uses the exact nanoseconds, rather than rounding a float-seconds interval.
	outcome.Elapsed = outcome.Through.At.Sub(outcome.From.At)
	outcome.TotalElapsed = time.Duration(core.Decode[float64](decoder, "total_elapsed") * float64(time.Second))
	if err := decoder.Error(); err != nil {
		return RewardOutcome{}, err
	}
	if err := from.Error(); err != nil {
		return RewardOutcome{}, err
	}
	if err := through.Error(); err != nil {
		return RewardOutcome{}, err
	}
	return outcome, nil
}
