package strategy

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/learning/associative/agent"
)

/*
	Evaluation owns the issue-time economics needed to grade a decision once a

durable trade leg closes. It reports tape opportunity return, not hypothetical
executable PnL. Actual wallet PnL remains on Balance and Holding.
*/
type Evaluation struct {
	Index, Entry                 int
	Baseline, Secured, Potential *decimal.Decimal
	Idle                         time.Duration
	CaptureFraction              float64
	Failure                      string
	*agent.Decision[Action]
	Trader                                               int
	Initial, Reference, Quantity, Cost, Fee, Opportunity *decimal.Decimal
	Through                                              time.Time
	Value                                                float64
	Complete                                             bool
	// Forced marks a decision that had no alternative. Its outcome describes
	// what the market did, not what the action was worth, so it is released
	// rather than trained.
	Forced bool
}

/*
	Resolve grades the decision against the excursion the tape actually

developed. Every action family reports the same unit: a fraction of original
account funding. Waiting and holding are symmetric readings of one leg return —
holding through a decline costs what waiting through it saved — so a falling
tape neither excuses waiting nor forgives holding. Entries and exits are graded
on the costs recorded at fill time, not on a hypothetical round trip.
*/
func (evaluation *Evaluation) Resolve(leg hindsight.Leg) bool {
	if evaluation.Complete || leg.Symbol != evaluation.Label || !leg.Through.After(evaluation.At) {
		return false
	}
	end := leg.End.SetScale(decimal.DefaultScale)
	value := decimal.NewFromInt64(0)

	switch evaluation.Action.Kind {
	case "wait":
		if evaluation.Opportunity == nil {
			break
		}

		if evaluation.Reference == nil {
			return false
		}

		// legReturn scaled by the notional the agent declined to commit. A
		// falling leg pays the drawdown avoided; a rising one charges the move
		// missed. Reference cancels between the return and the notional, so the
		// account fraction is one division of the excursion by initial funding.
		value = end.Sub(evaluation.Reference).Mul(evaluation.Opportunity).Mul(decimal.NewFromInt64(-1))
	case "hold":
		if evaluation.Reference == nil {
			return false
		}

		// The retained position captures the leg return over its own notional.
		value = end.Sub(evaluation.Reference).Mul(evaluation.Quantity)
	default:
		if evaluation.Cost == nil {
			return false
		}

		// Entries and increases realize the developed leg against their actual
		// recorded execution cost and fee.
		value = end.Mul(evaluation.Quantity).Sub(evaluation.Cost).Sub(evaluation.Fee)

		if evaluation.Action.Reduce {
			// Exits and reductions are graded against the alternative they gave
			// up: the proceeds booked, net of fee, less what the same inventory
			// would have been worth at the end of the leg.
			value = evaluation.Cost.Sub(evaluation.Fee).Sub(end.Mul(evaluation.Quantity))
		}
	}
	evaluation.Value = value.SetScale(decimal.DefaultScale).Div(evaluation.Initial).Float64()
	evaluation.Through, evaluation.Complete = leg.Through, true
	return true
}

/*
Row converts a graded decision into its outcomes table row.

The conversion lives here rather than in the tables package because tables must
not import strategy: hindsight and strategy both read those tables, so a
dependency in that direction would close a cycle.
*/
func (evaluation *Evaluation) Row(run string) tables.OutcomeRow {
	row := tables.OutcomeRow{
		Run:         run,
		Trader:      int32(evaluation.Trader),
		Through:     evaluation.Through,
		Value:       evaluation.Value,
		Complete:    evaluation.Complete,
		Forced:      evaluation.Forced,
		Initial:     evaluation.Initial,
		Reference:   evaluation.Reference,
		Quantity:    evaluation.Quantity,
		Cost:        evaluation.Cost,
		Fee:         evaluation.Fee,
		Opportunity: evaluation.Opportunity,
	}

	if evaluation.Decision == nil {
		return row
	}

	row.DecisionID = int64(evaluation.ID)
	row.Label = evaluation.Label
	row.At = evaluation.At
	row.ActionKind = evaluation.Action.Kind
	row.ActionPower = int32(evaluation.Action.Power)
	row.ActionReduce = evaluation.Action.Reduce
	row.Authority = evaluation.Authority
	row.Outcome = evaluation.Outcome

	for _, token := range evaluation.Context {
		row.Context = append(row.Context, int64(token))
	}

	return row
}

// Grade scores completed replay decisions using secured wallet profit. Potential
// is the best single round trip supported by this tape's quotes for the available
// quantity and initial funding; future quotes are used only after replay ends.
// Idle time discounts a gain by captured-span/(captured-span+idle).
// It never discounts a loss. A flat missed opportunity receives an idle debit;
// waiting on a tape without executable profit receives no fabricated reward.
func (evaluation *Evaluation) Grade(observations []hindsight.Observation, price *broker.Price) error {
	if len(observations) < 2 || evaluation.Initial == nil || evaluation.Initial.Sign() <= 0 || evaluation.Secured == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "evaluation: complete tape, funding and secured result required", nil))
	}
	span := observations[len(observations)-1].ReceivedAt.Sub(observations[0].ReceivedAt)

	if span <= 0 || evaluation.Idle < 0 || evaluation.Idle > span {
		return errnie.Error(errnie.Err(errnie.Validation, "evaluation: invalid captured duration or idle time", nil))
	}
	fee := price.FeeIfAvailable(observations[0].Symbol)

	if fee == nil || fee.Fee == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "evaluation: symbol fee required", nil))
	}
	quantity := evaluation.Quantity

	if quantity == nil || quantity.Sign() == 0 {
		quantity = evaluation.Opportunity
	}
	evaluation.Potential = nil
	var cheapest *decimal.Decimal

	for index, observation := range observations {
		if observation.Symbol != observations[0].Symbol {
			return errnie.Error(errnie.Err(errnie.Validation, "evaluation: mixed symbols on one tape", nil))
		}

		// Order is read from the capture coordinate, never from the receive
		// clock: a batched trade or touch frame decodes into several
		// observations that all carry that frame's single receive instant.
		if index > 0 && !observations[index-1].Before(observation) {
			return errnie.Error(errnie.Err(errnie.Validation, "evaluation: non-increasing capture order", nil))
		}

		// Idle time is measured against the receive clock below, so that clock
		// must at least not run backwards across the tape.
		if index > 0 && observation.ReceivedAt.Before(observations[index-1].ReceivedAt) {
			return errnie.Error(errnie.Err(errnie.Validation, "evaluation: receive clock runs backwards", nil))
		}

		if quantity == nil || quantity.Sign() <= 0 {
			continue // No executable size was available at the decision.
		}

		if cheapest != nil && observation.HasBid && decimal.NewFromFloat64(observation.BidQty).Cmp(quantity) >= 0 {
			proceeds := price.WithFee(observation.Symbol, decimal.NewFromFloat64(observation.Bid).Mul(quantity), broker.SELL)
			profit := proceeds.Sub(cheapest)

			if evaluation.Potential == nil || profit.Cmp(evaluation.Potential) > 0 {
				evaluation.Potential = profit
			}
		}

		if observation.HasAsk && decimal.NewFromFloat64(observation.AskQty).Cmp(quantity) >= 0 {
			cost := price.WithFee(observation.Symbol, decimal.NewFromFloat64(observation.Ask).Mul(quantity), broker.BUY)

			if cost.Cmp(evaluation.Initial) <= 0 && (cheapest == nil || cost.Cmp(cheapest) < 0) {
				cheapest = cost
			}
		}
	}
	evaluation.Value = evaluation.Secured.Div(evaluation.Initial).Float64()
	evaluation.Failure = "capital loss"
	idleFraction := float64(evaluation.Idle) / (float64(span) + float64(evaluation.Idle))

	if evaluation.Secured.Sign() > 0 {
		evaluation.Value *= 1 - idleFraction
		evaluation.Failure = "profit secured"
	}

	if evaluation.Secured.Sign() == 0 {
		evaluation.Failure = "no executable profit secured"

		if evaluation.Potential != nil && evaluation.Potential.Sign() > 0 {
			evaluation.Value = -evaluation.Potential.Div(evaluation.Initial).Float64() * idleFraction
			evaluation.Failure = "opportunity missed"
		}
	}

	if evaluation.Potential != nil && evaluation.Potential.Sign() > 0 {
		evaluation.CaptureFraction = evaluation.Secured.Div(evaluation.Potential).Float64()
	}
	evaluation.Through = observations[len(observations)-1].ReceivedAt
	evaluation.Complete = true
	return nil
}
