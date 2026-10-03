package strategy

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/cognition"
)

type Evaluator struct {
	price  *broker.Price
	engine *cognition.Engine
}

func NewEvaluator(price *broker.Price, engine *cognition.Engine) *Evaluator {
	return &Evaluator{
		price:  price,
		engine: engine,
	}
}

/*
EvaluatePnL uses broker.Price to calculate the net economic return between
an entry price (ask) and an exit price (bid), accounting for venue fees and sizing.
*/
func (evaluator *Evaluator) EvaluatePnL(
	symbol string,
	entryAsk *decimal.Decimal,
	exitBid *decimal.Decimal,
) (float64, error) {
	if evaluator == nil || evaluator.price == nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation, "evaluator: price required", nil,
		))
	}

	if symbol == "" || entryAsk == nil || exitBid == nil || entryAsk.Sign() <= 0 || exitBid.Sign() <= 0 {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation, "evaluator: valid symbol, ask, and bid required", nil,
		))
	}

	evaluator.price.SetQuote(symbol, exitBid, entryAsk)

	cost, err := evaluator.price.AllocateEntry(symbol, evaluator.price.ReferenceCash())

	if err != nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation, "evaluator: allocate entry failed for "+symbol, err,
		))
	}

	if cost == nil || cost.Total == nil || cost.Total.Sign() <= 0 {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation, "evaluator: non-positive entry cost for "+symbol, nil,
		))
	}

	netProceeds, _, err := evaluator.price.Liquidate(symbol, cost.Quantity, exitBid)

	if err != nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation, "evaluator: liquidate failed for "+symbol, err,
		))
	}

	if netProceeds == nil {
		return 0, errnie.Error(errnie.Err(
			errnie.Validation, "evaluator: net proceeds missing for "+symbol, nil,
		))
	}

	profit := netProceeds.Sub(cost.Total)
	return profit.Div(cost.Total).Float64(), nil
}

/*
Train updates the cognition engine with a graded observation for the context.
Profitable outcomes receive the exact PnL; non-positive outcomes receive -1.0.
*/
func (evaluator *Evaluator) Train(context []byte, action cognition.Action, pnl float64) {
	if evaluator == nil || evaluator.engine == nil || len(context) == 0 {
		return
	}

	feedback := pnl

	if pnl <= 0 {
		feedback = -1.0
	}

	_, err := evaluator.engine.Train(context, []byte(action), feedback)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal, "evaluator: train failed", err,
		))
	}
}

/*
ObserveWait registers an intermediate prefix sequence as ActionWait in the trie.
*/
func (evaluator *Evaluator) ObserveWait(context []byte) {
	if evaluator == nil || evaluator.engine == nil || len(context) == 0 {
		return
	}

	_, err := evaluator.engine.Observe(cognition.Association{
		Context:  context,
		Class:    []byte(cognition.ActionWait),
		Feedback: 1.0,
		Graded:   true,
	})

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal, "evaluator: observe wait failed", err,
		))
	}
}
