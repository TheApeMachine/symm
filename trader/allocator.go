package trader

import (
	"sort"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
)

type Allocator struct {
	fraction float64
	quote    string
}

func NewAllocator() *Allocator {
	return &Allocator{
		fraction: viper.GetFloat64("trading.sizing.base_fraction"),
		quote:    viper.GetString("market.quote_currency"),
	}
}

func (allocator *Allocator) Allowed(
	actions []*datura.Artifact, balances *datura.Artifact,
) []*datura.Artifact {
	sort.SliceStable(actions, func(first, second int) bool {
		firstScore := datura.Peek[float64](actions[first], "decision", "score")
		secondScore := datura.Peek[float64](actions[second], "decision", "score")

		return firstScore > secondScore
	})

	_ = datura.Peek[float64](balances, "asset", "balance")
	allowed := make([]*datura.Artifact, 0)

	for _, action := range actions {
		action.Poke(allocator.calculate(
			datura.Peek[float64](action, "confidence"),
		), "fraction").Poke(
			true, "allowed",
		)

		allowed = append(allowed, action)
	}

	return allowed
}

/*
fraction sizes an admitted entry by wallet policy:
base wallet risk scaled by playbook confidence.
*/
func (allocator *Allocator) calculate(confidence float64) float64 {
	size := allocator.fraction
	size *= confidence
	return size
}
