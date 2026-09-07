package cmd

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/broker/position"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

/*
level3Node sends one book epoch directly to the agent's resident execution
surface before analytical stages process it.
*/
type level3Node struct {
	learner *strategy.Agent
}

func (node level3Node) Step(envelope *types.Envelope) *types.Envelope {
	if envelope == nil || envelope.TypeID != types.EnvelopeLevel3 || node.learner == nil {
		return envelope
	}

	value, found := node.learner.Positions.Load(envelope.Level3Data.Symbol)

	if !found {
		return envelope
	}

	if err := value.(*position.Regulator).Guardian.Publish(envelope.Level3Data); err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "symm: position level3 step", err))
	}

	return envelope
}

/*
executionNode sends a confirmed private execution to the agent's position owner.
*/
type executionNode struct {
	learner *strategy.Agent
}

func (node executionNode) Step(envelope *types.Envelope) *types.Envelope {
	if envelope == nil || envelope.TypeID != types.EnvelopeExecution || node.learner == nil {
		return envelope
	}

	value, found := node.learner.Positions.Load(envelope.ExecutionData.Symbol)

	if !found {
		return envelope
	}

	if err := value.(*position.Regulator).Guardian.Publish(envelope.ExecutionData); err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "symm: position execution step", err))
	}

	return envelope
}

/*
cvdQuoteProvider reads the agent's resident top of book for trade-response facts.
*/
func cvdQuoteProvider(price *broker.Price) func(string) (*decimal.Decimal, *decimal.Decimal) {
	return func(symbol string) (*decimal.Decimal, *decimal.Decimal) {
		if price == nil {
			return nil, nil
		}

		tick := price.Tick(symbol)

		if tick == nil {
			return nil, nil
		}

		return tick.Bid, tick.Ask
	}
}
