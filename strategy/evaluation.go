package strategy

import (
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
