package cmd

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
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
	price *broker.Price
	desk  *broker.Desk
	tick  int64
}

/* Step updates the quote provider before dependent signal producers run. */
func (node *learningTickNode) Step(envelope *types.Envelope) *types.Envelope {
	if envelope == nil || envelope.TypeID != types.EnvelopeTicker {
		return envelope
	}

	node.tick++
	envelope.Tick = node.tick
	node.price.Update(&envelope.TickerData)

	if node.desk == nil {
		return envelope
	}

	if err := node.desk.StepTicker(envelope.TickerData); err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "symm: desk ticker step", err))
	}

	envelope.Equity = node.desk.Equity()
	envelope.Positions = node.desk.OpenPositionWire()

	return envelope
}
