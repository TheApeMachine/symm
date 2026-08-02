package broker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Position is one lot shell owned by Desk. Order correlation uses request ID then
exchange order ID; unmatched executions buffer until the ack binds them.
Position subscribes to the market ticker topic for live mark and stoploss
updates, and to the account execution and add_order topics for fills and
order acks so it handles its own lifecycle without Desk absorbing the logic.
*/
type Position struct {
	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	subscriptions map[string]*types.Subscription[any]
	ui            chan []byte
	instrument    *Instrument
	price         *Price
	balance       *Balance
	pair          kraken.InstrumentPair
	ID            string                `json:"id"`
	EntryOrderID  string                `json:"entry_order_id,omitempty"`
	ExitOrderID   string                `json:"exit_order_id,omitempty"`
	Status        types.Status          `json:"status"`
	EntryOrder    *spot.AddOrderRequest `json:"entry_order"`
	ExitOrder     *spot.AddOrderRequest `json:"exit_order"`
	Holding       *types.Holding        `json:"holding"`
}

/*
NewPosition constructs one lot shell; Desk routes order and execution
rows initially but Position subscribes to the market ticker, account
executions, and account add_order topics so it responds to live
websocket messages directly.
*/
func NewPosition(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
	instrument *Instrument,
	price *Price,
	balance *Balance,
	pair kraken.InstrumentPair,
	qty *decimal.Decimal,
) *Position {
	errnie.Info("creating position for: " + pair.Symbol)
	ctx, cancel := context.WithCancel(ctx)

	position := &Position{
		ctx:    ctx,
		cancel: cancel,
		Status: types.INITIALIZING,
		api:    api,
		subscriptions: map[string]*types.Subscription[any]{
			"ticker":     api.Subscribe("ticker", types.NewSubscription[any]()),
			"executions": api.Subscribe("executions", types.NewSubscription[any]()),
			"add_order":  api.Subscribe("add_order", types.NewSubscription[any]()),
		},
		ui:         ui,
		instrument: instrument,
		price:      price,
		balance:    balance,
		pair:       pair,
		EntryOrder: &spot.AddOrderRequest{
			Type:      "buy",
			OrderType: "market",
			Volume:    qty.String(),
			Pair:      pair.Symbol,
			Validate:  true,
		},
		ExitOrder: &spot.AddOrderRequest{
			Type:      "sell",
			OrderType: "market",
			Volume:    qty.String(),
			Pair:      pair.Symbol,
			Validate:  true,
		},
		Holding: types.NewHolding(
			ctx,
			pair.Symbol,
			qty,
			price.Mark(pair.Symbol, SELL),
		),
	}

	position.run()
	position.Publish()

	return position
}

func (position *Position) run() {
	go func() {
		for {
			select {
			case <-position.ctx.Done():
				return
			case message := <-position.subscriptions["ticker"].Channel:
				ticker, ok := message.(*kraken.Ticker)

				if !ok {
					continue
				}

				position.onTicker(ticker)
			case message := <-position.subscriptions["executions"].Channel:
				execution, ok := message.(*kraken.Execution)

				if !ok || execution == nil {
					continue
				}

				position.onExecution(execution)
			}
		}
	}()
}

func (position *Position) snapshot() Position {
	position.mu.RLock()
	defer position.mu.RUnlock()

	out := *position
	out.mu = sync.RWMutex{}

	if position.Holding != nil {
		holding := *position.Holding

		if position.Holding.Stoploss != nil {
			stoploss := *position.Holding.Stoploss
			holding.Stoploss = &stoploss
		}

		out.Holding = &holding
	}

	if position.EntryOrder != nil {
		entryOrder := *position.EntryOrder
		out.EntryOrder = &entryOrder
	}

	if position.ExitOrder != nil {
		exitOrder := *position.ExitOrder
		out.ExitOrder = &exitOrder
	}

	return out
}

/*
Publish the position to the UI, which will automatically marshal the Holding
and its Stoploss into the JSON payload. For clarity, the balance is kept out
of this, as there must be a way to get that more accurate to reality, where
the exchange publishes the wallet state at the sensible moments. The paper
trading implementation we use is based on the kraken-cli, where under normal
use you would also not be manually managing the balances.
*/
func (position *Position) Publish() {
	out := datura.NewMap()
	out["positions"] = []Position{position.snapshot()}
	utils.Publish(position.ui, out)
}

/*
onTicker refreshes the mark cache for this position's holding and
lets the bound stoploss regulator evaluate the live bid path for
exit decisions.
*/
func (position *Position) onTicker(ticker *kraken.Ticker) {
	if ticker == nil {
		return
	}

	for _, row := range ticker.Data {
		if row.Symbol != position.pair.Symbol {
			continue
		}

		position.mu.Lock()

		if position.Holding != nil && position.Holding.Stoploss != nil {
			if err := position.Holding.Stoploss.Update(ticker); err != nil {
				errnie.Error(err)
			}
		}

		position.mu.Unlock()

		break
	}
}

func (position *Position) onExecution(execution *kraken.Execution) {
	for _, row := range execution.Data {
		if row.Symbol != position.pair.Symbol || !position.matches(row) {
			continue
		}

		position.mu.Lock()

		if row.Side == "buy" {
			position.applyEntry(row)
		}

		if row.Side == "sell" {
			position.applyExit(row)
		}

		closed := position.Status == types.CLOSED
		position.mu.Unlock()
		position.Publish()

		if closed {
			position.cancel()
		}
	}
}

func (position *Position) matches(execution kraken.ExecutionData) bool {
	position.mu.RLock()
	defer position.mu.RUnlock()

	if execution.ClientOrderID == position.ID {
		return true
	}

	return execution.OrderID != "" &&
		(execution.OrderID == position.EntryOrderID || execution.OrderID == position.ExitOrderID)
}

func (position *Position) applyEntry(execution kraken.ExecutionData) {
	filled := execution.CumQty != nil && execution.CumQty.Sign() > 0

	if !filled {
		filled = execution.LastQty != nil && execution.LastQty.Sign() > 0
	}

	if !filled {
		return
	}

	if execution.CumQty != nil && execution.CumQty.Sign() > 0 {
		position.Holding.Qty = execution.CumQty.Copy()
		position.Holding.SellableQty = execution.CumQty.Copy()
	}

	if execution.AvgPrice != nil && execution.AvgPrice.Sign() > 0 {
		position.Holding.EntryPrice = execution.AvgPrice.Copy()
		position.Holding.Mark = execution.AvgPrice.Copy()
	}

	if execution.FeeUsdEquiv != nil {
		position.Holding.EntryFee = execution.FeeUsdEquiv.Copy()
	}

	entryAt := execution.Timestamp

	if entryAt.IsZero() {
		entryAt = time.Now().UTC()
	}

	position.Holding.EntryAt = &entryAt
	position.Status = types.OPEN
	position.Holding.Status = types.OPEN
}

func (position *Position) applyExit(execution kraken.ExecutionData) {
	if execution.OrderStatus != "filled" {
		position.Status = types.PENDING
		position.Holding.Status = types.PENDING
		return
	}

	if execution.AvgPrice != nil && execution.AvgPrice.Sign() > 0 {
		position.Holding.ExitPrice = execution.AvgPrice.Copy()
	}

	if execution.FeeUsdEquiv != nil {
		position.Holding.ExitFee = execution.FeeUsdEquiv.Copy()
	}

	exitAt := execution.Timestamp

	if exitAt.IsZero() {
		exitAt = time.Now().UTC()
	}

	position.Holding.ExitAt = &exitAt
	position.Holding.SellableQty = decimal.NewFromInt64(0)
	position.Status = types.CLOSED
	position.Holding.Status = types.CLOSED
}

/*
Enter submits a market buy for its quantity and returns the transport error so
Desk cannot publish a false entry-submitted lifecycle.
*/
func (position *Position) Enter() (*Position, error) {
	result, err := position.api.AddOrder(position.EntryOrder)

	if err != nil {
		position.mu.Lock()
		position.Status = types.ERROR
		position.Holding.Status = types.ERROR
		position.mu.Unlock()

		return position, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	position.mu.Lock()

	if len(result.ID) > 0 {
		position.EntryOrderID = result.ID[0]
	}

	position.Status = types.PENDING
	position.Holding.Status = types.PENDING
	position.mu.Unlock()
	position.Publish()

	return position, nil
}

/*
Exit submits a market sell for the sellable ledger quantity.
*/
func (position *Position) Exit() error {
	position.mu.Lock()

	if position.Holding == nil || position.Holding.SellableQty == nil || position.Holding.SellableQty.Sign() <= 0 {
		position.mu.Unlock()

		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"position: no sellable quantity available",
			nil,
		))
	}

	position.ExitOrder.Volume = position.Holding.SellableQty.String()
	position.mu.Unlock()

	result, err := position.api.AddOrder(position.ExitOrder)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market sell order",
			err,
		))
	}

	position.mu.Lock()

	if len(result.ID) > 0 {
		position.ExitOrderID = result.ID[0]
	}

	position.Status = types.PENDING
	position.Holding.Status = types.PENDING
	position.mu.Unlock()
	position.Publish()

	return nil
}

/*
Close marks the lot closed once Desk drops it from the open map.
*/
func (position *Position) Close() (err error) {
	position.mu.Lock()
	defer position.mu.Unlock()

	if position.Status == types.CLOSED {
		return nil
	}

	if position.cancel != nil {
		position.cancel()
	}

	if position.Holding != nil {
		err = errors.Join(err, position.Holding.Close())
	}

	position.Status = types.CLOSED
	return errnie.Error(err)
}
