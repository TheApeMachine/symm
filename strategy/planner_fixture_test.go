package strategy

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/types"
)

/*
testPlanner builds a Decide-only Planner with nil broker dependencies so admit
and rotate unit tests exercise ranking without instrument or transport wiring.
*/
func testPlanner(signals ...types.Signal) *Planner {
	return NewPlanner(
		context.Background(),
		nil,
		nil,
		nil,
		nil,
		nil,
		signals,
		nil,
		nil,
		nil,
	)
}

/*
testHolding builds an open pointer lot for Thesis.Holdings fixtures.
*/
func testHolding(symbol string, qty, mark float64) *types.Holding {
	return &types.Holding{
		Symbol: symbol,
		Qty:    decimal.NewFromFloat64(qty),
		Mark:   decimal.NewFromFloat64(mark),
		Status: types.OPEN,
	}
}
