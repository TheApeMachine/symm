package cmd

import (
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/* learningTickNode owns ticker sequencing and the shared observable quote cache. */
type learningTickNode struct {
	price *broker.Price
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
	return envelope
}
