package conditions

import (
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	executionfixture "github.com/theapemachine/symm/tests/fixtures/execution"
	orderackfixture "github.com/theapemachine/symm/tests/fixtures/orderack"
	"github.com/theapemachine/symm/types"
)

/*
EntryFill converts one submitted synthetic buy into the private Kraken facts a
filled venue order produces: acknowledgement, execution, and authoritative
wallet snapshot. Monetary arithmetic remains decimal; float conversion occurs
only for Kraken's wire quantity field.
*/
func EntryFill(
	request []byte,
	decision types.Decision,
	cashBefore *decimal.Decimal,
) []tests.Frame {
	order := &kraken.MarketOrder{}

	if err := sonic.Unmarshal(request, order); err != nil {
		panic(errnie.Err(errnie.Validation, "conditions: order request invalid", err))
	}

	if order.Method != "add_order" || order.Params.Side != "buy" ||
		order.Params.Symbol != decision.Symbol || order.Params.OrderQty <= 0 {
		panic(errnie.Err(errnie.Validation, "conditions: buy order does not match decision", nil))
	}

	if decision.ProposedQuantity == nil || decision.ProposedNotional == nil ||
		decision.ReferencePrice == nil || cashBefore == nil {
		panic(errnie.Err(errnie.Validation, "conditions: complete decimal fill facts required", nil))
	}

	base, quote, found := strings.Cut(decision.Symbol, "/")

	if !found || base == "" || quote == "" ||
		cashBefore.Cmp(decision.ProposedNotional) < 0 {
		panic(errnie.Err(errnie.Validation, "conditions: fill wallet facts invalid", nil))
	}

	cost := decision.ReferencePrice.Mul(decision.ProposedQuantity)
	fee := decision.ProposedNotional.Sub(cost)

	if fee.Sign() < 0 || decision.At.IsZero() {
		panic(errnie.Err(errnie.Validation, "conditions: fill economics invalid", nil))
	}

	const orderID = "synthetic-entry-1"
	execution := executionfixture.Frame(executionfixture.Options{
		OrderID:     orderID,
		ExecID:      "synthetic-fill-1",
		Symbol:      decision.Symbol,
		Side:        "buy",
		LastQty:     decision.ProposedQuantity.String(),
		LastPrice:   decision.ReferencePrice.String(),
		Cost:        cost.String(),
		OrderStatus: "filled",
		CumQty:      decision.ProposedQuantity.String(),
		CumCost:     cost.String(),
		AvgPrice:    decision.ReferencePrice.String(),
		FeeUsdEquiv: fee.String(),
		Timestamp:   decision.At.UTC().Format(time.RFC3339Nano),
	})
	cashAfter := cashBefore.Sub(decision.ProposedNotional)
	balance := tests.MarshalFrame(map[string]any{
		"channel":  "balances",
		"type":     "snapshot",
		"sequence": 2,
		"data": []map[string]any{
			{
				"asset":     quote,
				"balance":   cashAfter.String(),
				"available": cashAfter.String(),
				"reserved":  "0",
			},
			{
				"asset":     base,
				"balance":   decision.ProposedQuantity.String(),
				"available": decision.ProposedQuantity.String(),
				"reserved":  "0",
			},
		},
	})

	return []tests.Frame{
		{
			Channel: "add_order",
			Payload: orderackfixture.Frame(orderackfixture.Options{
				ReqID:   order.ReqID,
				OrderID: orderID,
				Success: true,
			}),
		},
		{
			Channel: "executions",
			Payload: execution,
		},
		{
			Channel: "balances",
			Payload: balance,
		},
	}
}

/*
ExitFill converts one submitted synthetic sell into a matched acknowledgement,
execution, and full-wallet snapshot with the asset removed. Proceeds and fees
remain decimal so the fixture cannot hide cash drift behind float arithmetic.
*/
func ExitFill(
	request []byte,
	decision types.Decision,
	cashBefore *decimal.Decimal,
	fee *decimal.Decimal,
) []tests.Frame {
	order := &kraken.MarketOrder{}

	if err := sonic.Unmarshal(request, order); err != nil {
		panic(errnie.Err(errnie.Validation, "conditions: exit order request invalid", err))
	}

	if order.Method != "add_order" || order.Params.Side != "sell" ||
		order.Params.Symbol != decision.Symbol || order.Params.OrderQty <= 0 {
		panic(errnie.Err(errnie.Validation, "conditions: sell order does not match decision", nil))
	}

	if decision.ProposedQuantity == nil || decision.ReferencePrice == nil ||
		cashBefore == nil || fee == nil || fee.Sign() < 0 || decision.At.IsZero() {
		panic(errnie.Err(errnie.Validation, "conditions: complete decimal exit facts required", nil))
	}

	_, quote, found := strings.Cut(decision.Symbol, "/")

	if !found || quote == "" {
		panic(errnie.Err(errnie.Validation, "conditions: exit wallet facts invalid", nil))
	}

	const orderID = "synthetic-exit-1"
	proceeds := decision.ReferencePrice.Mul(decision.ProposedQuantity)
	execution := executionfixture.Frame(executionfixture.Options{
		OrderID:     orderID,
		ExecID:      "synthetic-exit-fill-1",
		Symbol:      decision.Symbol,
		Side:        "sell",
		LastQty:     decision.ProposedQuantity.String(),
		LastPrice:   decision.ReferencePrice.String(),
		Cost:        proceeds.String(),
		OrderStatus: "filled",
		CumQty:      decision.ProposedQuantity.String(),
		CumCost:     proceeds.String(),
		AvgPrice:    decision.ReferencePrice.String(),
		FeeUsdEquiv: fee.String(),
		Timestamp:   decision.At.UTC().Format(time.RFC3339Nano),
	})
	cashAfter := cashBefore.Add(proceeds).Sub(fee)
	balance := tests.MarshalFrame(map[string]any{
		"channel":  "balances",
		"type":     "snapshot",
		"sequence": 3,
		"data": []map[string]any{{
			"asset":     quote,
			"balance":   cashAfter.String(),
			"available": cashAfter.String(),
			"reserved":  "0",
		}},
	})

	return []tests.Frame{
		{
			Channel: "add_order",
			Payload: orderackfixture.Frame(orderackfixture.Options{
				ReqID:   order.ReqID,
				OrderID: orderID,
				Success: true,
			}),
		},
		{
			Channel: "executions",
			Payload: execution,
		},
		{
			Channel: "balances",
			Payload: balance,
		},
	}
}
