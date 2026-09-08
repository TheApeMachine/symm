package strategy

import (
 "time"

 "github.com/krakenfx/api-go/v2/pkg/decimal"
 "github.com/theapemachine/symm/hindsight"
 "github.com/theapemachine/symm/nomagique/learning/associative/agent"
)

/* Evaluation owns the issue-time economics needed to grade a decision once a
durable trade leg closes. It reports tape opportunity return, not hypothetical
executable PnL. Actual wallet PnL remains on Balance and Holding. */
type Evaluation struct {
 *agent.Decision[Action]
 Trader int
 Initial, Reference, Quantity, Cost, Fee, Opportunity *decimal.Decimal
 Through time.Time
 Value float64
 Complete bool
}

/* Resolve measures buys against their actual cost, sales against the retained
inventory alternative, holds against their issue price, and waits against the
positive opportunity they left unused. A flat or falling tape does not penalize
waiting. All values use fractions of original account funding. */
func (evaluation *Evaluation) Resolve(leg hindsight.Leg) bool {
 if evaluation.Complete || leg.Symbol != evaluation.Label || !leg.Through.After(evaluation.At) {
  return false
 }
 value := decimal.NewFromInt64(0)

 switch evaluation.Action.Kind {
 case "wait":
  if evaluation.Opportunity == nil { break }

  if evaluation.Reference == nil { return false }
  gain := leg.End.Sub(evaluation.Reference)

  if gain.Sign() > 0 { value = gain.Mul(evaluation.Opportunity).Mul(decimal.NewFromInt64(-1)) }
 case "hold":
  if evaluation.Reference == nil { return false }
  value = leg.End.Sub(evaluation.Reference).Mul(evaluation.Quantity)
 default:
  if evaluation.Cost == nil { return false }
  value = leg.End.SetScale(decimal.DefaultScale).Mul(evaluation.Quantity).Sub(evaluation.Cost).Sub(evaluation.Fee)

  if evaluation.Action.Reduce {
   value = evaluation.Cost.Sub(evaluation.Fee).Sub(leg.End.SetScale(decimal.DefaultScale).Mul(evaluation.Quantity))
  }
 }
 evaluation.Value = value.SetScale(decimal.DefaultScale).Div(evaluation.Initial).Float64()
 evaluation.Through, evaluation.Complete = leg.Through, true
 return true
}
