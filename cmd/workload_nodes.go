package cmd

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/*
level3Node sends one book epoch directly to the Desk's resident execution
surface before analytical stages process it.
*/
type level3Node struct {
	desk *broker.Desk
}

func (node level3Node) Step(envelope *types.Envelope) *types.Envelope {
	if envelope == nil || envelope.TypeID != types.EnvelopeLevel3 || node.desk == nil {
		return envelope
	}

	if err := node.desk.StepLevel3Epoch(envelope.Level3Data, uint64(envelope.Stream.Epoch)); err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "symm: desk level3 step", err))
	}

	return envelope
}

/*
executionNode sends a confirmed private execution to the Desk's position owner.
*/
type executionNode struct {
	desk *broker.Desk
}

func (node executionNode) Step(envelope *types.Envelope) *types.Envelope {
	if envelope == nil || envelope.TypeID != types.EnvelopeExecution || node.desk == nil {
		return envelope
	}

	if err := node.desk.StepExecution(envelope.ExecutionData); err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "symm: desk execution step", err))
	}

	return envelope
}

/*
cvdQuoteProvider reads the Desk's resident top of book for trade-response facts.
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
