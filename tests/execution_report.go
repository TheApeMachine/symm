package tests

import (
	"fmt"

	testtypes "github.com/theapemachine/symm/tests/types"
)

/*
AutoFillOptions is the public execution configuration accepted by WithAutoFill.
*/
type AutoFillOptions = testtypes.ExecutionConfig

/*
OrderLifecycle records one venue order without conflating acknowledgement,
execution, and terminal state.
*/
type OrderLifecycle struct {
	OrderID      string   `json:"order_id"`
	ClientID     string   `json:"client_order_id"`
	Symbol       string   `json:"symbol"`
	Side         string   `json:"side"`
	Quantity     float64  `json:"quantity"`
	Executed     float64  `json:"executed"`
	State        string   `json:"state"`
	ExecutionIDs []string `json:"execution_ids"`
}

/*
MechanicsReport reports order-state and invariant outcomes independently from
profit or loss.
*/
type MechanicsReport struct {
	Submitted            int              `json:"submitted"`
	Acknowledged         int              `json:"acknowledged"`
	PartiallyFilled      int              `json:"partially_filled"`
	Filled               int              `json:"filled"`
	Canceled             int              `json:"canceled"`
	Rejected             int              `json:"rejected"`
	Expired              int              `json:"expired"`
	FalsePositiveEntries int              `json:"false_positive_entries"`
	Orders               []OrderLifecycle `json:"orders"`
	InvariantViolations  []string         `json:"invariant_violations"`
}

/*
EconomicsReport reports execution costs and realized economics.
*/
type EconomicsReport struct {
	OrderedQuantity  float64 `json:"ordered_quantity"`
	ExecutedQuantity float64 `json:"executed_quantity"`
	FillRatio        float64 `json:"fill_ratio"`
	GrossPnL         float64 `json:"gross_pnl"`
	Fees             float64 `json:"fees"`
	Slippage         float64 `json:"slippage"`
	NetPnL           float64 `json:"net_pnl"`
	MaximumDrawdown  float64 `json:"maximum_drawdown"`
}

/*
Report returns detached lifecycle and economics snapshots.
*/
func (model *executionModel) Report() (MechanicsReport, EconomicsReport) {
	mechanics := model.mechanics
	mechanics.Orders = make([]OrderLifecycle, len(model.orders))
	mechanics.InvariantViolations = append(
		[]string{}, model.mechanics.InvariantViolations...,
	)

	for index, order := range model.orders {
		state := "submitted"

		if order.acknowledged {
			state = "open"
		}

		if order.cumulativeQuantity > 0 {
			state = "partially_filled"
		}

		if order.terminal {
			state = order.terminalState
		}

		mechanics.Orders[index] = OrderLifecycle{
			OrderID:      order.order.ID,
			ClientID:     order.order.Request.ClOrdId,
			Symbol:       order.order.Request.Pair,
			Side:         order.order.Request.Type,
			Quantity:     order.order.Quantity,
			Executed:     order.cumulativeQuantity,
			State:        state,
			ExecutionIDs: append([]string{}, order.executionIDs...),
		}
	}

	return mechanics, model.ledger.Report()
}

/*
Validate checks cumulative order quantity, execution identity, and ledger
invariants independent of consumers.
*/
func (model *executionModel) Validate() error {
	seenExecutions := map[string]struct{}{}

	for _, order := range model.orders {
		if order.cumulativeQuantity > order.order.Quantity {
			return fmt.Errorf("simulator: order %s executed beyond its quantity", order.order.ID)
		}

		for _, executionID := range order.executionIDs {
			if _, exists := seenExecutions[executionID]; exists {
				return fmt.Errorf("simulator: duplicate execution identity %s", executionID)
			}

			seenExecutions[executionID] = struct{}{}
		}
	}

	return model.ledger.Validate()
}
