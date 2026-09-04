package cmd

import (
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/types"
)

/*
tickNode applies one public ticker to the priority position path and stamps the
account state before analytical work begins.
*/
type tickNode struct {
	desk *broker.Desk
	tick atomic.Int64
}

func (node *tickNode) Step(envelope *types.Envelope) *types.Envelope {
	if envelope == nil || envelope.TypeID != types.EnvelopeTicker {
		return envelope
	}

	envelope.Tick = node.tick.Add(1)

	if err := node.desk.StepTicker(envelope.TickerData); err != nil {
		errnie.Error(errnie.Err(errnie.Internal, "symm: desk ticker step", err))
	}

	envelope.Equity = node.desk.Equity()
	envelope.Positions = node.desk.OpenPositionWire()

	return envelope
}

func (node *tickNode) StepBacklog(envelope *types.Envelope, backlog int64) *types.Envelope {
	return node.Step(envelope)
}

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

/*
deskContextNode forwards survived perspectives and cross-signal causative facts
from the enriched envelope into the broker desk's position guardian.
*/
type deskContextNode struct {
	desk *broker.Desk
}

func (node *deskContextNode) Step(envelope *types.Envelope) *types.Envelope {
	if node == nil || node.desk == nil || envelope == nil {
		return envelope
	}

	symbol := envelope.Key

	if symbol == "" {
		symbol = envelope.TickerData.Symbol
	}

	if symbol == "" {
		for _, perspective := range envelope.Perspectives {
			if perspective != nil && perspective.Symbol != "" {
				symbol = perspective.Symbol
				break
			}
		}
	}

	if symbol == "" {
		return envelope
	}

	for _, perspective := range envelope.Perspectives {
		_ = node.desk.StepPerspective(perspective)
	}

	causative := types.CausativeContext{
		ActivePerspectives: make(map[string]string),
	}

	for _, perspective := range envelope.Perspectives {
		causative.ActivePerspectives[perspective.Advisor] = string(perspective.TopClass())
	}

	if envelope.Hawkes != nil {
		if metric, ok := envelope.Hawkes.Metrics["branching_spectral_radius"]; ok {
			causative.HawkesBranchingRatio = metric.Raw
		}

		if metric, ok := envelope.Hawkes.Metrics["excitation_fraction:buy"]; ok {
			causative.HawkesExcitationBuy = metric.Raw
		}

		if metric, ok := envelope.Hawkes.Metrics["excitation_fraction:sell"]; ok {
			causative.HawkesExcitationSell = metric.Raw
		}
	}

	if envelope.Derivatives != nil {
		if metric, ok := envelope.Derivatives.Metrics["open_interest_growth_velocity"]; ok {
			causative.OIGrowthVelocity = metric.Raw
		}

		if metric, ok := envelope.Derivatives.Metrics["open_interest_growth_zscore"]; ok {
			causative.OIGrowthZScore = metric.Raw
		}
	}

	if envelope.Toxicity != nil {
		if metric, ok := envelope.Toxicity.Metrics["net_replenishment_fraction:bid"]; ok {
			causative.NetReplenishmentBid = metric.Raw
		}

		if metric, ok := envelope.Toxicity.Metrics["net_withdrawal_fraction:bid"]; ok {
			causative.NetWithdrawalBid = metric.Raw
		}
	}

	_ = node.desk.StepCausative(symbol, causative)

	return envelope
}
