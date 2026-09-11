package position

import (
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"slices"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/* Regulator serializes one position's orders and cumulative execution facts. */
type Regulator struct {
	mu            sync.RWMutex
	Guardian      *Guardian
	Holding       *types.Holding
	ID            string
	Recovered     bool
	Orders        Orders
	Surface       *types.ExecutionSurface
	At            time.Time
	LastExecution kraken.ExecutionData

	api      *websocket.API
	price    *broker.Price
	pending  *spot.AddOrderRequest
	result   spot.AddOrderResult
	executed kraken.ExecutionData
	exitID   string
	record   func(kraken.ExecutionData) error
}

/* Orders is the replaceable order transport; accounting stays on Regulator. */
type Orders interface {
	AddOrder(*spot.AddOrderRequest) (spot.AddOrderResult, error)
	CancelOrder(*spot.CancelOrderRequest) (spot.CancelResult, error)
}

func NewRegulator(
	api *websocket.API,
	price *broker.Price,
	symbol string,
	record func(kraken.ExecutionData) error,
) *Regulator {
	position := &Regulator{
		api:     api,
		Orders:  api,
		price:   price,
		record:  record,
		Holding: types.NewHolding(symbol),
	}
	return position
}

/*
Submit runs on Guardian. An exit waits for a canceled buy's terminal fill before
selling, so a late partial buy cannot leave inventory behind.
*/
func (position *Regulator) Submit(
	identity string,
	side broker.Direction,
	quantity *decimal.Decimal,
) error {
	if position.pending != nil {
		if side != broker.SELL || position.pending.Type != "buy" {
			return errnie.Error(&types.ExecutionRefusal{
				State:  "order pending",
				Detail: "position already has an outstanding order",
			})
		}
		position.exitID = identity

		for _, identity := range position.result.ID {
			if _, err := position.Orders.CancelOrder(&spot.CancelOrderRequest{
				TxID: identity,
			}); err != nil {
				return errnie.Error(errnie.Err(
					errnie.IO, "position: cancel pending buy before exit", err,
				))
			}
		}
		return nil
	}

	if side == broker.SELL && quantity.Cmp(position.Holding.Qty) > 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation, "position: sale exceeds held inventory", nil,
		))
	}
	normalized, err := position.api.Normalizer().FormatSize(
		position.Holding.Symbol, quantity,
	)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "position: venue size normalization failed", err,
		))
	}

	if normalized.Cmp(quantity) != 0 || normalized.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation, "position: quantity must match the venue increment", nil,
		))
	}
	position.pending = &spot.AddOrderRequest{
		ClOrdId:   identity,
		Pair:      position.Holding.Symbol,
		Type:      string(side),
		OrderType: "market",
		Volume:    normalized.String(),
	}
	position.executed = kraken.ExecutionData{
		CumQty:      decimal.NewFromInt64(0),
		CumCost:     decimal.NewFromInt64(0),
		FeeUsdEquiv: decimal.NewFromInt64(0),
	}
	position.result, err = position.Orders.AddOrder(position.pending)

	if err != nil {
		position.pending = nil
		return errnie.Error(errnie.Err(
			errnie.IO, "position: order submission failed", err,
		))
	}

	if position.ID == "" {
		position.ID = identity
	}
	position.mu.Lock()
	position.Holding.Status = types.PENDING
	position.mu.Unlock()
	return nil
}

/* Increase submits a buy order for the specified quantity. */
func (position *Regulator) Increase(identity string, quantity *decimal.Decimal) error {
	return position.Submit(identity, broker.BUY, quantity)
}

/* Reduce submits a sell order for the specified quantity. */
func (position *Regulator) Reduce(identity string, quantity *decimal.Decimal) error {
	return position.Submit(identity, broker.SELL, quantity)
}

/* Exit submits a sell order closing the complete remaining inventory. */
func (position *Regulator) Exit(identity string) error {
	position.mu.RLock()
	quantity := position.Holding.Qty
	position.mu.RUnlock()
	return position.Submit(identity, broker.SELL, quantity)
}

/* Pending returns the active working order, if any. */
func (position *Regulator) Pending() *spot.AddOrderRequest {
	position.mu.RLock()
	defer position.mu.RUnlock()
	return position.pending
}

/* Status returns the current lifecycle status of the position. */
func (position *Regulator) Status() types.Status {
	position.mu.RLock()
	defer position.mu.RUnlock()
	return position.Holding.Status
}

/* Apply accounts each cumulative increment once, including partial cancellations. */
func (position *Regulator) Apply(execution kraken.ExecutionData) error {
	if position.pending == nil || (execution.ClientOrderID != position.pending.ClOrdId && !slices.Contains(position.result.ID, execution.OrderID)) {
		return nil
	}

	if execution.CumQty != nil && execution.CumQty.Sign() > 0 {
		position.mu.Lock()
		err := position.price.ApplyFill(position.Holding, execution, position.executed)
		position.mu.Unlock()

		if err != nil {
			return err
		}
		position.executed = execution
		position.LastExecution = execution
	}

	if position.record != nil {
		if err := position.record(execution); err != nil {
			return err
		}
	}

	switch execution.OrderStatus {
	case "filled", "iceberg_filled", "canceled", "expired", "rejected":
		position.pending = nil
		position.executed = kraken.ExecutionData{}
	default:
		return nil
	}
	position.mu.Lock()
	position.Holding.Status = types.OPEN
	position.mu.Unlock()

	if position.Holding.Qty.Sign() == 0 {
		position.mu.Lock()
		position.Holding.Status = types.CLOSED
		position.mu.Unlock()
		return nil
	}

	if position.exitID != "" {
		identity := position.exitID
		position.exitID = ""
		return position.Submit(identity, broker.SELL, position.Holding.Qty)
	}
	return nil
}

/* Mark uses executable liquidation depth and never submits an order. */
func (position *Regulator) Mark(at time.Time) error {
	if position.Holding == nil || position.Holding.Qty == nil || position.Holding.Qty.Sign() == 0 || position.price == nil {
		return nil
	}
	surface, err := position.price.Surface(
		position.Holding.Symbol, position.Holding.Qty, at,
	)

	if err != nil || !surface.FullyExecutable {
		return err
	}
	position.mu.Lock()
	defer position.mu.Unlock()
	position.Holding.Mark = surface.ExecutableVWAP
	position.Surface = surface
	position.Holding.PnL = surface.ExecutableValue.Sub(position.Holding.Basis).Sub(position.Holding.EntryFee)
	basis := position.Holding.Basis.Add(position.Holding.EntryFee)

	if basis.Sign() > 0 {
		position.Holding.ReturnPct = position.Holding.PnL.Div(basis).Mul(decimal.NewFromInt64(100)).Float64()
	}
	return nil
}

/* Wire serializes the current holding at the UI boundary under its read lock. */
func (position *Regulator) Wire() *wire.PositionT {
	position.mu.RLock()
	defer position.mu.RUnlock()
	return &wire.PositionT{
		Status:  string(position.Holding.Status),
		Holding: types.HoldingWire(position.Holding),
	}
}
