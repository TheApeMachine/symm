package cmd

import (
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/broker/position"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

/*
learningTickNode owns ticker sequencing, the shared observable quote cache and
the account state every surface reads.

The desk is stepped whether or not the agent has earned the right to trade. An
operator watching a calibrating agent is still watching a real account, and the
balance, unrealized and equity readings in the terminal come from here — a run
that only stamped the quote cache left them with no producer at all.
*/
type learningTickNode struct {
	price   *broker.Price
	learner *strategy.Agent
	tick    int64
}

/* Step updates the quote provider before dependent signal producers run. */
func (node *learningTickNode) Step(envelope *types.Envelope) *types.Envelope {
	if envelope == nil || envelope.TypeID != types.EnvelopeTicker {
		return envelope
	}

	node.tick++
	envelope.Tick = node.tick
	node.price.Update(&envelope.TickerData)

	if node.learner == nil {
		return envelope
	}

	envelope.Equity = node.learner.Balance.Reading.Load()
	node.learner.Positions.Range(func(_, value any) bool {
		published := value.(*position.Regulator).Wire()

		if published.Status != string(types.CLOSED) {
			envelope.Positions = append(envelope.Positions, published)
		}
		return true
	})

	return envelope
}
